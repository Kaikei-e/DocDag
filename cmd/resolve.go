package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/render"
)

type resolveRecord struct {
	render.Record
	Pending []graph.PendingSuccessor `json:"pending,omitempty"`
}

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
			ids, pending, err := graph.ResolveWithPending(g, cfg, id, config.EdgeSupersedes, asOf)
			if err != nil {
				return domainErr("%v", err)
			}
			out := cmd.OutOrStdout()
			records := withColumns(g, cfg, render.Records(g, ids), fields, asOf)
			if format == formatJSON {
				resolveRecords := make([]resolveRecord, len(records))
				for i, r := range records {
					resolveRecords[i] = resolveRecord{Record: r}
				}
				if len(resolveRecords) > 0 && len(pending) > 0 {
					if len(resolveRecords) == 1 {
						resolveRecords[0].Pending = pending
					} else {
						for _, p := range pending {
							matched := false
							for i := range resolveRecords {
								if p.Predecessor == resolveRecords[i].ID {
									resolveRecords[i].Pending = append(resolveRecords[i].Pending, p)
									matched = true
								}
							}
							if !matched {
								for i := range resolveRecords {
									resolveRecords[i].Pending = append(resolveRecords[i].Pending, p)
								}
							}
						}
					}
				}
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				enc.SetEscapeHTML(false)
				err = enc.Encode(resolveRecords)
			} else {
				err = render.RecordsText(out, records, fields)
				if err != nil {
					return ioErr(err)
				}
				for _, p := range pending {
					fmt.Fprintf(cmd.ErrOrStderr(), "note: %s supersedes %s but is %s; not yet binding\n", p.ID, p.Predecessor, p.Status)
				}
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
