package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/render"
)

func newValidateCmd() *cobra.Command {
	return &cobra.Command{
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
			findings := graph.Suggest(graph.Validate(g, cfg), g, cfg)
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
}
