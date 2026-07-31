package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/render"
)

func newStatsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Report degree-based statistics for the document corpus",
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
			stats := graph.ComputeStats(g, cfg)
			out := cmd.OutOrStdout()
			if format == formatJSON {
				err = render.StatsJSON(out, stats)
			} else {
				err = render.StatsText(out, stats)
			}
			if err != nil {
				return ioErr(err)
			}
			return nil
		},
	}
}
