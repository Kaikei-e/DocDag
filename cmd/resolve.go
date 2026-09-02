package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/render"
)

func newResolveCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "resolve <ref>",
		Short: "Print the documents that currently supersede a reference",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, err := outputFormat(cmd, formatText, formatJSON)
			if err != nil {
				return err
			}
			asOf, err := asOfToday(cmd)
			if err != nil {
				return err
			}
			c, err := loadCorpus(cmd)
			if err != nil {
				return err
			}
			defer c.close()
			g, cfg := c.graph, c.cfg
			fields, err := recordFields(cmd, cfg)
			if err != nil {
				return err
			}
			if err := requireSupersedes(cfg); err != nil {
				return err
			}
			id, err := normalize(g, cfg, args[0])
			if err != nil {
				return err
			}
			ids, err := graph.ResolveAt(g, cfg, id, config.EdgeSupersedes, asOf)
			if err != nil {
				return domainErr("%v", err)
			}
			out := cmd.OutOrStdout()
			records := withColumns(g, cfg, render.Records(g, ids), fields, asOf)
			if format == formatJSON {
				err = render.RecordsJSON(out, records)
			} else {
				err = render.RecordsText(out, records, fields)
			}
			if err != nil {
				return ioErr(err)
			}
			return nil
		},
	}
	addAsOfFlag(cmd, "today")
	addAtFlag(cmd)
	addFieldsFlag(cmd)
	return cmd
}
