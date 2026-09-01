package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/render"
	"github.com/Kaikei-e/DocDag/internal/vcs"
)

const (
	flagTouching       = "touching"
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
			showSuppressed, err := cmd.Flags().GetBool(flagShowSuppressed)
			if err != nil {
				return usageErr("%v", err)
			}
			g, cfg, err := loadGraph(cmd)
			if err != nil {
				return err
			}
			// The day is read here rather than inside the checks, so one run
			// compares every sunset against one date and a test can name it.
			findings := graph.Validate(g, cfg, time.Now())
			history, err := immutableFindings(cmd, cfg)
			if err != nil {
				return err
			}
			if len(history) > 0 {
				findings = append(findings, history...)
				graph.SortFindings(findings)
			}
			findings = graph.Suggest(findings, g, cfg)
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
			switch format {
			case formatJSON:
				err = render.FindingsJSON(out, reported, summary, cfg.PresetVersion)
			case formatGitHub:
				err = render.FindingsGitHub(out, reported, summary)
			case formatRDJSON:
				err = render.FindingsRDJSON(out, reported, summary)
			default:
				err = render.FindingsText(out, reported, summary)
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
	// The history check reads one documents directory under one identity rule.
	// A multi-kind corpus has neither, and answering over the wrong directory
	// would report an append-only violation nobody committed.
	if cfg.Multikind() {
		return nil, ioErr(fmt.Errorf("--%s does not read a multi-kind corpus yet: %w", flagImmutableSince, model.ErrInvalidConfig))
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
