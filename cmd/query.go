package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/render"
)

const (
	flagAncestors   = "ancestors"
	flagDescendants = "descendants"
	flagEdge        = "edge"
	flagBinding     = "binding"
)

func newQueryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "query [ref]",
		Short: "List the documents reachable from a reference over typed edges",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runQuery,
	}
	cmd.Flags().Bool(flagAncestors, false, "walk typed edges backwards")
	cmd.Flags().Bool(flagDescendants, false, "walk typed edges forwards (default)")
	cmd.Flags().String(flagEdge, "", "restrict the walk to one edge type")
	cmd.Flags().Bool(flagIncludeRefs, false, "overlay reference-layer neighbours")
	cmd.Flags().Bool(flagBinding, false, "list every binding document")
	addFieldsFlag(cmd)
	return cmd
}

func runQuery(cmd *cobra.Command, args []string) error {
	format, err := outputFormat(cmd, formatText, formatJSON)
	if err != nil {
		return err
	}
	fields, err := recordFields(cmd)
	if err != nil {
		return err
	}
	flags := cmd.Flags()
	binding, err := flags.GetBool(flagBinding)
	if err != nil {
		return usageErr("%v", err)
	}
	ancestors, err := flags.GetBool(flagAncestors)
	if err != nil {
		return usageErr("%v", err)
	}
	descendants, err := flags.GetBool(flagDescendants)
	if err != nil {
		return usageErr("%v", err)
	}
	if ancestors && descendants {
		return usageErr("--%s and --%s are mutually exclusive", flagAncestors, flagDescendants)
	}
	includeRefs, err := flags.GetBool(flagIncludeRefs)
	if err != nil {
		return usageErr("%v", err)
	}
	edge, err := flags.GetString(flagEdge)
	if err != nil {
		return usageErr("%v", err)
	}
	if binding && len(args) > 0 {
		return usageErr("--%s takes no reference", flagBinding)
	}
	if !binding && len(args) == 0 {
		return usageErr("query needs a reference or --%s", flagBinding)
	}
	// The binding set is not a walk, so a walk flag alongside it asks for
	// something the command cannot do.
	if binding && (ancestors || descendants || includeRefs || edge != "") {
		return usageErr("--%s takes none of --%s, --%s, --%s and --%s",
			flagBinding, flagAncestors, flagDescendants, flagEdge, flagIncludeRefs)
	}

	g, cfg, err := loadGraph(cmd)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()

	if binding {
		if err := requireSupersedes(cfg); err != nil {
			return err
		}
		records := render.Records(g, graph.BindingSet(g, cfg))
		if format == formatJSON {
			err = render.RecordsJSON(out, records)
		} else {
			err = render.RecordsText(out, records, fields)
		}
		if err != nil {
			return ioErr(err)
		}
		return nil
	}

	opts := graph.QueryOptions{Direction: graph.DirectionDescendants, IncludeRefs: includeRefs}
	if ancestors {
		opts.Direction = graph.DirectionAncestors
	}
	if edge != "" {
		if _, ok := cfg.Edge(model.EdgeType(edge)); !ok {
			return usageErr("unknown edge type %q", edge)
		}
		opts.Types = []model.EdgeType{model.EdgeType(edge)}
	}
	id, err := normalize(g, cfg, args[0])
	if err != nil {
		return err
	}
	results, err := graph.Query(g, id, opts)
	if err != nil {
		return domainErr("query %s: %v", id, err)
	}
	records := render.QueryRecords(g, results)
	if format == formatJSON {
		err = render.RecordsJSON(out, records)
	} else {
		err = render.RecordsText(out, records, fields)
	}
	if err != nil {
		return ioErr(err)
	}
	return nil
}
