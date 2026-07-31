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
			format, err := outputFormat(cmd)
			if err != nil {
				return err
			}
			g, cfg, err := loadGraph(cmd)
			if err != nil {
				return err
			}
			findings := graph.Validate(g, cfg)
			summary := graph.Summarize(g, findings)
			out := cmd.OutOrStdout()
			if format == formatJSON {
				err = render.FindingsJSON(out, findings, summary)
			} else {
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
