package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/render"
	"github.com/Kaikei-e/DocDag/internal/vcs"
	"github.com/Kaikei-e/DocDag/model"
)

const (
	flagTouching       = "touching"
	flagIfManaged      = "if-managed"
	flagImmutableSince = "immutable-since"
	flagShowSuppressed = "show-suppressed"
)

func newValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Run the structural checks and the configured rules",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			format, err := outputFormat(cmd, formatText, formatJSON, formatGitHub, formatRDJSON)
			if err != nil {
				return err
			}
			touching, err := cmd.Flags().GetStringArray(flagTouching)
			if err != nil {
				return usageErr("%v", err)
			}
			ifManaged, err := cmd.Flags().GetBool(flagIfManaged)
			if err != nil {
				return usageErr("%v", err)
			}
			if ifManaged && len(touching) == 0 {
				return usageErr("--%s requires --%s", flagIfManaged, flagTouching)
			}
			showSuppressed, err := cmd.Flags().GetBool(flagShowSuppressed)
			if err != nil {
				return usageErr("%v", err)
			}
			if ifManaged {
				cfg, err := resolveEffectiveConfig(cmd)
				if err != nil {
					if errors.Is(err, model.ErrNoDocuments) {
						return nil
					}
					var ce *cliError
					if errors.As(err, &ce) {
						return err
					}
					return ioErr(err)
				}
				if !touchesManagedDir(touching, cfg) {
					return nil
				}
			}
			asOf, err := asOfCommitted(cmd)
			if err != nil {
				return err
			}
			c, err := loadCorpus(cmd)
			if err != nil {
				return err
			}
			defer c.close()
			g, cfg := c.graph, c.cfg
			// The day is read here rather than inside the checks, so one run
			// compares every sunset and every period against one date and a
			// test can name it.
			findings := graph.Validate(g, cfg, asOf)
			history, err := immutableFindings(cmd, cfg)
			if err != nil {
				return err
			}
			if len(history) > 0 {
				findings = append(findings, history...)
				graph.SortFindings(findings)
			}
			findings = graph.Suggest(findings, g, cfg, asOf)
			// The exit code answers for the corpus, never for the filter: a
			// report narrowed to one file must not turn a failure into a pass.
			// Summarize leaves out the suppressed findings whether or not they
			// are being shown, so asking to see them cannot fail a build.
			summary := graph.Summarize(g, findings)
			if !showSuppressed {
				findings = shown(findings)
			}
			reported := findings
			if len(touching) > 0 {
				reported = graph.Touching(findings, g, touching)
			}
			out := cmd.OutOrStdout()
			header := render.Header{PresetVersion: cfg.PresetVersion, AsOf: graph.AsOfDay(asOf), At: c.at}
			day := reportedAsOf(cfg, asOf)
			switch format {
			case formatJSON:
				err = render.FindingsJSON(out, reported, summary, header)
			case formatGitHub:
				err = render.FindingsGitHub(out, reported, summary, day)
			case formatRDJSON:
				err = render.FindingsRDJSON(out, reported, summary)
			default:
				// Suppress the trailing "as of" line when --touching hid every
				// finding on a failing corpus so stdout stays empty, matching
				// corpora without period:.
				if len(touching) > 0 && len(reported) == 0 && summary.Errors > 0 {
					day = ""
				}
				err = render.FindingsText(out, reported, summary, day)
			}
			if err != nil {
				return ioErr(err)
			}
			if hidden := len(findings) - len(reported); hidden > 0 {
				fmt.Fprintf(cmd.ErrOrStderr(), "(%d %s hidden)\n", hidden, plural(hidden, "finding"))
			}
			if summary.Errors > 0 {
				return &cliError{code: exitFailure}
			}
			return nil
		},
	}
	cmd.Flags().StringArray(flagTouching, nil, "report only the findings about these files or directories (repeatable)")
	cmd.Flags().Bool(flagIfManaged, false, "exit 0 silently when --touching names no managed document")
	addAsOfFlag(cmd, "the day HEAD was committed on")
	addAtFlag(cmd)
	cmd.Flags().String(flagImmutableSince, "", "check that documents closed at <rev> only grew since")
	cmd.Flags().Bool(flagShowSuppressed, false, "also report the findings a recorded exception suppresses")
	return cmd
}

// shown drops the findings the corpus has already answered. They are computed
// either way — the check has to pair the documents before it can know one is
// suppressed — and left out of the report unless the reader asks, so the
// exception is a decision recorded once rather than a line read every day.
func shown(findings []model.Finding) []model.Finding {
	out := make([]model.Finding, 0, len(findings))
	for _, f := range findings {
		if f.Suppressed {
			continue
		}
		out = append(out, f)
	}
	return out
}

func plural(n int, noun string) string {
	if n == 1 {
		return noun
	}
	return noun + "s"
}

// immutableFindings runs the append-only history check when a revision names
// one. A corpus outside a repository cannot answer it, which is a setup error
// rather than a failing corpus.
func immutableFindings(cmd *cobra.Command, cfg config.Config) ([]model.Finding, error) {
	rev, err := cmd.Flags().GetString(flagImmutableSince)
	if err != nil {
		return nil, usageErr("%v", err)
	}
	if rev == "" {
		return nil, nil
	}
	// The history check compares the working tree against a base revision, and
	// --at replaces the working tree with a revision of its own: asking for both
	// is asking the check to compare two revisions it never read.
	if at, err := revision(cmd); err != nil {
		return nil, err
	} else if at != "" {
		return nil, usageErr("--%s reads the working tree, so it takes no --%s", flagImmutableSince, flagAt)
	}
	// The history check reads one documents directory under one identity rule,
	// or every kind that declares append_only: true. A multi-kind corpus with
	// none leaves the check nothing to read without guessing, so it refuses
	// rather than comparing the wrong files.
	if cfg.Multikind() && len(cfg.AppendOnlyKinds()) == 0 {
		return nil, ioErr(fmt.Errorf("--%s reads only kinds with append_only: true: %w", flagImmutableSince, model.ErrInvalidConfig))
	}
	repo, err := vcs.Open(cfg.Dir)
	if err != nil {
		return nil, ioErr(fmt.Errorf("--%s: %w", flagImmutableSince, err))
	}
	root, err := os.Getwd()
	if err != nil {
		return nil, ioErr(fmt.Errorf("working directory: %w", err))
	}
	findings, err := graph.CheckImmutable(repo, cfg, rev, root)
	if err != nil {
		return nil, ioErr(fmt.Errorf("--%s: %w", flagImmutableSince, err))
	}
	return findings, nil
}

// touchesManagedDir reports whether any path in paths names a Markdown file
// directly inside a resolved corpus directory, or a directory containing one,
// matching parse.Documents' non-recursive .md collection.
func touchesManagedDir(paths []string, cfg config.Config) bool {
	dirs := corpusDirs(cfg)
	if len(dirs) == 0 {
		return false
	}
	normDirs := make([]string, 0, len(dirs))
	for _, d := range dirs {
		normDirs = append(normDirs, normalizeDir(d))
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			dirNorm := normalizeDir(abs)
			prefix := dirNorm + string(filepath.Separator)
			for _, d := range normDirs {
				if d == dirNorm || strings.HasPrefix(d, prefix) {
					return true
				}
			}
			continue
		}
		if filepath.Ext(abs) != ".md" {
			continue
		}
		if slices.Contains(normDirs, normalizeDir(filepath.Dir(abs))) {
			return true
		}
	}
	return false
}

// normalizeDir returns a cleaned, symlink-resolved path for dir, walking up to
// the nearest existing ancestor when dir itself does not exist on disk yet.
func normalizeDir(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return filepath.Clean(dir)
	}
	cur := abs
	var tail []string
	for {
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			for i := len(tail) - 1; i >= 0; i-- {
				real = filepath.Join(real, tail[i])
			}
			return filepath.Clean(real)
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return filepath.Clean(abs)
		}
		tail = append(tail, filepath.Base(cur))
		cur = parent
	}
}
