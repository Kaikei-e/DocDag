// Package cmd wires the docdag CLI: a cobra command tree over the document
// graph. It is the only place that maps errors onto process exit codes.
package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

// Exit codes:
//
//	0 = success (warnings allowed)
//	1 = domain failure (validation errors, unknown reference, cycle)
//	2 = usage error (bad arguments or flags)
//	3 = I/O or configuration error
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
	exitIO      = 3
)

// Global flag names and output formats.
const (
	flagDir         = "dir"
	flagConfig      = "config"
	flagFormat      = "format"
	flagIncludeRefs = "include-refs"

	formatText    = "text"
	formatJSON    = "json"
	formatGitHub  = "github"
	formatRDJSON  = "rdjson"
	formatMermaid = "mermaid"
	formatDOT     = "dot"
)

// cliError carries an exit code through cobra's RunE path. An empty msg means
// the command already printed its own diagnostics.
type cliError struct {
	code int
	msg  string
}

func (e *cliError) Error() string { return e.msg }

func domainErr(format string, a ...any) *cliError {
	return &cliError{code: exitFailure, msg: fmt.Sprintf(format, a...)}
}

func usageErr(format string, a ...any) *cliError {
	return &cliError{code: exitUsage, msg: fmt.Sprintf(format, a...)}
}

func ioErr(err error) *cliError {
	return &cliError{code: exitIO, msg: err.Error()}
}

// exitCode maps a RunE error onto a process exit code. Anything cobra itself
// produced (unknown flag, wrong argument count) is a usage error.
func exitCode(err error) int {
	if err == nil {
		return exitOK
	}
	var ce *cliError
	if errors.As(err, &ce) {
		return ce.code
	}
	return exitUsage
}

func validFormat(format string, allowed ...string) error {
	for _, a := range allowed {
		if format == a {
			return nil
		}
	}
	return usageErr("invalid --%s %q (allowed: %v)", flagFormat, format, allowed)
}

// outputFormat reads the output format and holds it to the set the calling
// command answers in.
func outputFormat(cmd *cobra.Command, allowed ...string) (string, error) {
	format, err := cmd.Flags().GetString(flagFormat)
	if err != nil {
		return "", usageErr("%v", err)
	}
	if err := validFormat(format, allowed...); err != nil {
		return "", err
	}
	return format, nil
}

// effectiveConfig resolves preset, config file and flags into one config.
func effectiveConfig(cmd *cobra.Command) (config.Config, error) {
	dir, err := cmd.Flags().GetString(flagDir)
	if err != nil {
		return config.Config{}, usageErr("%v", err)
	}
	path, err := cmd.Flags().GetString(flagConfig)
	if err != nil {
		return config.Config{}, usageErr("%v", err)
	}
	root, err := os.Getwd()
	if err != nil {
		return config.Config{}, ioErr(fmt.Errorf("working directory: %w", err))
	}
	cfg, err := config.Resolve(config.Options{Root: root, Dir: dir, ConfigPath: path})
	if err != nil {
		return config.Config{}, ioErr(fmt.Errorf("resolve configuration: %w", err))
	}
	return cfg, nil
}

// loadGraph resolves the configuration, parses the documents directory and
// builds the two-layer graph.
func loadGraph(cmd *cobra.Command) (*model.Graph, config.Config, error) {
	cfg, err := effectiveConfig(cmd)
	if err != nil {
		return nil, config.Config{}, err
	}
	docs, err := parse.Dir(cfg.Dir, cfg)
	if err != nil {
		return nil, cfg, ioErr(err)
	}
	root, err := os.Getwd()
	if err != nil {
		return nil, cfg, ioErr(fmt.Errorf("working directory: %w", err))
	}
	// Findings name files the way the caller would type them, not the way
	// discovery happened to spell them.
	parse.Localize(docs, root)
	return graph.Build(docs, cfg), cfg, nil
}

// requireSupersedes refuses the commands that are defined over the supersedes
// edge type when the configuration does not declare it. Walking an edge set
// that cannot exist would report every document as current, at exit 0.
func requireSupersedes(cfg config.Config) error {
	if _, ok := cfg.Edge(config.EdgeSupersedes); ok {
		return nil
	}
	return ioErr(fmt.Errorf("this command needs the %q edge type, which the configuration does not declare: %w",
		config.EdgeSupersedes, model.ErrInvalidConfig))
}

// normalize maps a raw reference onto a node identifier that exists.
func normalize(g *model.Graph, cfg config.Config, ref string) (model.ID, error) {
	id, ok := cfg.Normalizer().Normalize(ref)
	if !ok {
		return "", domainErr("unrecognized reference %q", ref)
	}
	if _, ok := g.Node(id); !ok {
		return "", domainErr("unknown document %s", id)
	}
	return id, nil
}

// version is stamped by the release build via -ldflags "-X ...cmd.version=".
var version = "dev"

// newRootCmd builds a fresh command tree, so every test gets an isolated one.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "docdag",
		Short:         "Extract, validate and query a typed graph of Markdown documents",
		Long:          "docdag builds a typed directed graph from Markdown documents with YAML\nfrontmatter, enforces DAG invariants on it and answers graph queries.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// The command set is the contract; cobra's generated completion command is
	// not part of it.
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().String(flagDir, "", "documents directory (overrides config and discovery)")
	root.PersistentFlags().String(flagConfig, "", "path to docdag.yaml")
	root.PersistentFlags().String(flagFormat, formatText, "output format: text|json, plus github|rdjson on validate")
	root.AddCommand(
		newValidateCmd(),
		newResolveCmd(),
		newQueryCmd(),
		newExportCmd(),
		newStatsCmd(),
		newNewCmd(),
	)
	return root
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	return executeWith(os.Args[1:], os.Stdout, os.Stderr)
}

// executeWith is Execute with injectable arguments and streams so the exit-code
// mapping is testable.
func executeWith(args []string, out, errW io.Writer) int {
	root := newRootCmd()
	root.SetOut(out)
	root.SetErr(errW)
	root.SetArgs(args)
	err := root.Execute()
	if err != nil && err.Error() != "" {
		fmt.Fprintf(errW, "Error: %v\n", err)
	}
	return exitCode(err)
}
