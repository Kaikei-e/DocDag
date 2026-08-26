package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/render"
	"github.com/Kaikei-e/DocDag/internal/vcs"
)

const flagImmutableSince = "immutable-since"

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
			g, cfg, err := loadGraph(cmd)
			if err != nil {
				return err
			}
			findings := graph.Validate(g, cfg)
			history, err := immutableFindings(cmd, cfg)
			if err != nil {
				return err
			}
			if len(history) > 0 {
				findings = append(findings, history...)
				graph.SortFindings(findings)
			}
			summary := graph.Summarize(g, findings)
			out := cmd.OutOrStdout()
			switch format {
			case formatJSON:
				err = render.FindingsJSON(out, findings, summary)
			case formatGitHub:
				err = render.FindingsGitHub(out, findings, summary)
			case formatRDJSON:
				err = render.FindingsRDJSON(out, findings, summary)
			default:
				err = render.FindingsText(out, findings, summary)
			}
			if err != nil {
				return ioErr(err)
			}
			if summary.Errors > 0 {
				return &cliError{code: exitFailure}
			}
			return nil
		},
	}
	cmd.Flags().String(flagImmutableSince, "", "check that documents closed at <rev> only grew since")
	return cmd
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
