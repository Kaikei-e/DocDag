package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/render"
)

const flagTouching = "touching"

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
			g, cfg, err := loadGraph(cmd)
			if err != nil {
				return err
			}
			findings := graph.Suggest(graph.Validate(g, cfg), g, cfg)
			// The exit code answers for the corpus, never for the filter: a
			// report narrowed to one file must not turn a failure into a pass.
			summary := graph.Summarize(g, findings)
			reported := findings
			if len(touching) > 0 {
				reported = graph.Touching(findings, g, touching)
			}
			out := cmd.OutOrStdout()
			switch format {
			case formatJSON:
				err = render.FindingsJSON(out, reported, summary)
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
				fmt.Fprintf(cmd.ErrOrStderr(), "(%d findings hidden)\n", hidden)
			}
			if summary.Errors > 0 {
				return &cliError{code: exitFailure}
			}
			return nil
		},
	}
	cmd.Flags().StringArray(flagTouching, nil, "report only the findings about these files or directories (repeatable)")
	return cmd
}
