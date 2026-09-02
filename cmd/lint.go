package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/lint"
	"github.com/Kaikei-e/DocDag/internal/parse"
	"github.com/Kaikei-e/DocDag/internal/render"
	"github.com/Kaikei-e/DocDag/internal/vcs"
)

// The lint flags. --all is the word `context` already spells "everything you
// have", declared beside that command.
const (
	flagCorpus   = "corpus"
	flagFixtures = "fixtures"
	flagStrict   = "strict"
	flagSince    = "since"
)

// exitLintWarn is what a lint run exits with when it found warnings and no
// errors. It is the one command with a code of its own: a configuration that
// lints with warnings is neither a failure nor nothing, and a repository
// decides for itself whether to gate on it — with --strict, or by reading the
// code.
const exitLintWarn = 2

// newLintCmd builds `docdag lint`, which reports on the configuration rather
// than on the documents. It is never run by `validate`: the health of a
// configuration and the state of a corpus have different lifecycles, and a lint
// warning on every pull request is a warning nobody reads.
func newLintCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Report contradictions, vacuity and untested rules in the configuration",
		Long: "docdag lint reads docdag.yaml and reports the rules and projections that\n" +
			"cannot fire, that fire everywhere, that say what another rule already says,\n" +
			"and — with --corpus and --fixtures — the ones the vault never fires and the\n" +
			"ones whose own fixtures disagree with them.",
		Args: cobra.NoArgs,
		RunE: runLint,
	}
	cmd.Flags().Bool(flagCorpus, false, "also evaluate the rules and projections against the current vault")
	cmd.Flags().String(flagFixtures, "", "also run the per-rule fixtures under this directory")
	cmd.Flags().Bool(flagAll, false, "run every layer, reading fixtures from "+lint.DefaultFixtureDir+"/")
	cmd.Flags().Bool(flagStrict, false, "exit 1 on warnings as well as errors")
	cmd.Flags().String(flagSince, "", "also evaluate the corpus at <rev> and report what started or stopped firing")
	addAsOfFlag(cmd, "the day HEAD was committed on")
	addAtFlag(cmd)
	return cmd
}

func runLint(cmd *cobra.Command, _ []string) error {
	format, err := outputFormat(cmd, formatText, formatJSON, formatGitHub, formatRDJSON)
	if err != nil {
		return err
	}
	opts, run, err := lintOptions(cmd)
	if err != nil {
		return err
	}
	defer run.close()
	findings, err := lint.Run(opts)
	if err != nil {
		return ioErr(err)
	}
	summary := lint.Summarize(findings)
	out := cmd.OutOrStdout()
	header := render.Header{PresetVersion: opts.Config.PresetVersion, AsOf: graph.AsOfDay(opts.AsOf), At: run.at}
	// Only a run that read the corpus asked a question a day could change the
	// answer to: layer 1 reads the configuration alone, and a configuration
	// means the same thing on every day.
	day := ""
	if opts.Corpus != nil {
		day = reportedAsOf(opts.Config, opts.AsOf)
	}
	switch format {
	case formatJSON:
		err = render.LintJSON(out, findings, summary, header)
	case formatGitHub:
		err = render.LintGitHub(out, findings, summary, day)
	case formatRDJSON:
		err = render.LintRDJSON(out, findings, summary)
	default:
		err = render.LintText(out, findings, summary, day)
	}
	if err != nil {
		return ioErr(err)
	}
	switch {
	case summary.Errors > 0:
		return &cliError{code: exitFailure}
	case summary.Warnings > 0 && run.strict:
		return &cliError{code: exitFailure}
	case summary.Warnings > 0:
		return &cliError{code: exitLintWarn}
	}
	return nil
}

// lintRun is what the caller has to hold on to beside the options: whether a
// warning fails the run, the revision the corpus was read from, and the
// temporary tree --at left behind.
type lintRun struct {
	strict bool
	at     string
	corpus *corpus
}

func (r lintRun) close() { r.corpus.close() }

// lintOptions reads the flags into one lint run: which layers to answer in,
// where the configuration file is, and where the corpus and the fixtures are
// read from.
func lintOptions(cmd *cobra.Command) (lint.Options, lintRun, error) {
	flags := cmd.Flags()
	readCorpus, err := flags.GetBool(flagCorpus)
	if err != nil {
		return lint.Options{}, lintRun{}, usageErr("%v", err)
	}
	fixtures, err := flags.GetString(flagFixtures)
	if err != nil {
		return lint.Options{}, lintRun{}, usageErr("%v", err)
	}
	all, err := flags.GetBool(flagAll)
	if err != nil {
		return lint.Options{}, lintRun{}, usageErr("%v", err)
	}
	run := lintRun{}
	if run.strict, err = flags.GetBool(flagStrict); err != nil {
		return lint.Options{}, lintRun{}, usageErr("%v", err)
	}
	since, err := flags.GetString(flagSince)
	if err != nil {
		return lint.Options{}, lintRun{}, usageErr("%v", err)
	}
	asOf, err := asOfCommitted(cmd)
	if err != nil {
		return lint.Options{}, lintRun{}, err
	}
	if all {
		readCorpus = true
		if fixtures == "" {
			fixtures = lint.DefaultFixtureDir
		}
	}
	// A revision to compare against is a corpus question, so naming one asks
	// for the corpus layer whether or not the flag beside it was written.
	if since != "" {
		readCorpus = true
	}

	root, err := os.Getwd()
	if err != nil {
		return lint.Options{}, lintRun{}, ioErr(fmt.Errorf("working directory: %w", err))
	}
	cfg, err := effectiveConfig(cmd)
	if err != nil {
		return lint.Options{}, lintRun{}, err
	}
	path, err := configFile(cmd, root)
	if err != nil {
		return lint.Options{}, lintRun{}, err
	}
	opts := lint.Options{
		Config:   cfg,
		Locator:  lint.NewLocator(path, cfg.Preset),
		Fixtures: fixtures,
		Since:    since,
		Root:     corpusRoot(root, path),
		Reported: root,
		AsOf:     asOf,
	}
	if readCorpus {
		read, err := loadCorpus(cmd)
		if err != nil {
			return lint.Options{}, lintRun{}, err
		}
		run.corpus, run.at = read, read.at
		opts.Corpus = read.graph
	}
	if since == "" {
		return opts, run, nil
	}
	repo, err := vcs.Open(root)
	if err != nil {
		run.close()
		return lint.Options{}, lintRun{}, ioErr(fmt.Errorf("--%s: %w", flagSince, err))
	}
	opts.Repo = repo
	return opts, run, nil
}

// runNewFixture writes the ruleid/ and ok/ skeleton of one rule, derived from
// the rule's own condition. It lives beside `lint` rather than beside `new`
// because what it writes is a lint fixture and what it reads is the same
// expansion lint reasons with — the documents that satisfy one alternative of
// the condition, and the same documents with one literal of it broken.
func runNewFixture(cmd *cobra.Command, rule, format string) error {
	root, err := os.Getwd()
	if err != nil {
		return ioErr(fmt.Errorf("working directory: %w", err))
	}
	cfg, err := effectiveConfig(cmd)
	if err != nil {
		return err
	}
	path, err := configFile(cmd, root)
	if err != nil {
		return err
	}
	fixtures, err := cmd.Flags().GetString(flagFixtures)
	if err != nil {
		return usageErr("%v", err)
	}
	skeleton, err := lint.Generate(cfg, rule, corpusRoot(root, path), fixtures, time.Now())
	if err != nil {
		return domainErr("%v", err)
	}
	written, err := skeleton.Apply()
	if err != nil {
		return ioErr(err)
	}
	for i, file := range written {
		written[i] = parse.LocalPath(root, file)
	}
	if err := render.FixturePaths(cmd.OutOrStdout(), written, format == formatJSON); err != nil {
		return ioErr(err)
	}
	return nil
}

// configFile reports the configuration file a run reads, named the way the
// caller would type it, and the empty string where the corpus configures
// nothing at all — there every rule comes from the preset, and that is where a
// finding about one is reported.
func configFile(cmd *cobra.Command, root string) (string, error) {
	path, err := cmd.Flags().GetString(flagConfig)
	if err != nil {
		return "", usageErr("%v", err)
	}
	if path == "" {
		candidate := filepath.Join(root, config.DefaultConfigFile)
		if _, err := os.Stat(candidate); err != nil {
			return "", nil
		}
		path = candidate
	}
	return parse.LocalPath(root, path), nil
}

// corpusRoot is the directory a configuration describes its corpus from: the
// one holding the configuration file, and the working directory where there is
// no file. It is what a fixture's kind directories are rerooted against, so a
// fixture holds the same layout the corpus does.
func corpusRoot(root, path string) string {
	if path == "" {
		return root
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return root
	}
	return filepath.Dir(absolute)
}
