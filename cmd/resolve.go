package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/render"
)

func newResolveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resolve <ref>",
		Short: "Print the documents that currently supersede a reference",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := outputFormat(cmd)
			if err != nil {
				return err
			}
			g, cfg, err := loadGraph(cmd)
			if err != nil {
				return err
			}
			id, err := normalize(g, cfg, args[0])
			if err != nil {
				return err
			}
			ids, err := graph.Resolve(g, id, config.EdgeSupersedes)
			if err != nil {
				return domainErr("%v", err)
			}
			out := cmd.OutOrStdout()
			if format == formatJSON {
				err = render.IDsJSON(out, ids)
			} else {
				err = render.IDsText(out, ids)
			}
			if err != nil {
				return ioErr(err)
			}
			return nil
		},
	}
}
