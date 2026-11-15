package cmd

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/render"
	"github.com/Kaikei-e/DocDag/model"
)

const (
	flagAncestors   = "ancestors"
	flagDescendants = "descendants"
	flagEdge        = "edge"
	flagBinding     = "binding"
	flagWhere       = "where"
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
	cmd.Flags().StringArray(flagWhere, nil, "filter results by <key>=<op>:<value> (repeatable)")
	addAsOfFlag(cmd, "today")
	addAtFlag(cmd)
	addFieldsFlag(cmd)
	return cmd
}

func runQuery(cmd *cobra.Command, args []string) error {
	format, err := outputFormat(cmd, formatText, formatJSON)
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
	whereRaw, err := flags.GetStringArray(flagWhere)
	if err != nil {
		return usageErr("%v", err)
	}
	where, err := parseWhereFlags(whereRaw)
	if err != nil {
		return err
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
	if err := validateWhereKeys(where, cfg); err != nil {
		return err
	}
	out := cmd.OutOrStdout()

	if binding {
		if err := requireSupersedes(cfg); err != nil {
			return err
		}
		ids := graph.BindingSet(g, cfg, asOf)
		if len(where) > 0 {
			ids = filterWhere(ids, graph.MatchAttrs(g, cfg, where, asOf), func(id model.ID) model.ID { return id })
		}
		base := render.Records(g, ids)
		if format == formatJSON {
			err = render.RecordsJSON(out, withAllColumns(g, cfg, base, asOf))
		} else {
			// The binding set has a default column set of its own, which a caller
			// naming columns replaces like any other default.
			if !flags.Changed(flagFields) {
				fields = bindingColumns(cfg)
			}
			err = render.RecordsText(out, withColumns(g, cfg, base, fields, asOf), fields)
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
	if len(where) > 0 {
		results = filterWhere(results, graph.MatchAttrs(g, cfg, where, asOf), func(r graph.QueryResult) model.ID { return r.ID })
	}
	base := render.QueryRecords(g, results)
	if format == formatJSON {
		err = render.RecordsJSON(out, withAllColumns(g, cfg, base, asOf))
	} else {
		err = render.RecordsText(out, withColumns(g, cfg, base, fields, asOf), fields)
	}
	if err != nil {
		return ioErr(err)
	}
	return nil
}

const whereOpsHelp = "eq, not, contains, not_contains, subset_of"

func parseWhereFlags(raws []string) ([]graph.AttrFilter, error) {
	filters := make([]graph.AttrFilter, 0, len(raws))
	for _, raw := range raws {
		f, err := parseWhereExpr(raw)
		if err != nil {
			return nil, err
		}
		filters = append(filters, f)
	}
	return filters, nil
}

func parseWhereExpr(raw string) (graph.AttrFilter, error) {
	key, rest, hasEq := strings.Cut(raw, "=")
	if !hasEq {
		return graph.AttrFilter{}, usageErr("invalid --%s %q: want key=op:value (ops: %s)", flagWhere, raw, whereOpsHelp)
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return graph.AttrFilter{}, usageErr("invalid --%s %q: empty key, want key=op:value (ops: %s)", flagWhere, raw, whereOpsHelp)
	}
	op, val, hasColon := strings.Cut(rest, ":")
	if !hasColon {
		return graph.AttrFilter{}, usageErr("invalid --%s %q: want key=op:value (ops: %s)", flagWhere, raw, whereOpsHelp)
	}
	cond, ok := parseWhereOp(op, val)
	if !ok {
		return graph.AttrFilter{}, usageErr("invalid --%s %q: unknown op %q, want key=op:value (ops: %s)", flagWhere, raw, op, whereOpsHelp)
	}
	return graph.AttrFilter{Key: key, Want: cond}, nil
}

func parseWhereOp(op, val string) (config.AttrCondition, bool) {
	switch op {
	case "eq":
		return config.AttrCondition{Eq: &val}, true
	case "not":
		return config.AttrCondition{Not: &val}, true
	case "contains":
		return config.AttrCondition{Contains: &val}, true
	case "not_contains":
		return config.AttrCondition{NotContains: &val}, true
	case "subset_of":
		items := make([]string, 0)
		for _, item := range strings.Split(val, ",") {
			item = strings.TrimSpace(item)
			if item != "" {
				items = append(items, item)
			}
		}
		return config.AttrCondition{SubsetOf: items}, true
	default:
		return config.AttrCondition{}, false
	}
}

func validateWhereKeys(filters []graph.AttrFilter, cfg config.Config) error {
	if len(filters) == 0 {
		return nil
	}
	allowed := allowedWhereKeys(cfg)
	for _, f := range filters {
		if !slices.Contains(allowed, f.Key) {
			return usageErr("unknown --%s key %q (allowed: %s)", flagWhere, f.Key, strings.Join(allowed, ", "))
		}
	}
	return nil
}

func allowedWhereKeys(cfg config.Config) []string {
	keys := []string{cfg.EffectiveStatus()}
	add := func(name string) {
		if !slices.Contains(keys, name) {
			keys = append(keys, name)
		}
	}
	for _, name := range cfg.DeclaredFields() {
		add(name)
	}
	for _, name := range cfg.ProjectionNames() {
		add(name)
	}
	add(config.AttrInForce)
	return keys
}

func filterWhere[T any](items []T, match func(model.ID) bool, idOf func(T) model.ID) []T {
	out := make([]T, 0, len(items))
	for _, item := range items {
		if match(idOf(item)) {
			out = append(out, item)
		}
	}
	return out
}
