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
			format, err := outputFormat(cmd, formatText, formatJSON)
			if err != nil {
				return err
			}
			g, cfg, err := loadGraph(cmd)
			if err != nil {
				return err
			}
			// The binding count and the chain-depth distribution are supersedes
			// statistics; without that edge type they would be fiction.
			if err := requireSupersedes(cfg); err != nil {
				return err
			}
			stats := graph.ComputeStats(g, cfg)
			out := cmd.OutOrStdout()
			switch format {
			case formatJSON:
				err = render.StatsJSON(out, stats)
			default:
				err = render.StatsText(out, stats)
			}
			if err != nil {
				return ioErr(err)
			}
			return nil
		},
	}
}
