package cmd

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/render"
)

const flagFields = "fields"

// addFieldsFlag declares the field selection a listing command answers in.
func addFieldsFlag(cmd *cobra.Command) {
	cmd.Flags().String(flagFields, render.FieldID,
		"comma-separated text columns: "+strings.Join(render.FieldNames, ", ")+
			", plus the configured projections (json carries them all)")
}

// recordFields reads the requested text columns, in the order they were asked
// for. A configured projection is a column too: its value is derived rather
// than written down, but a listing reads it like any other field.
func recordFields(cmd *cobra.Command, cfg config.Config) ([]string, error) {
	raw, err := cmd.Flags().GetString(flagFields)
	if err != nil {
		return nil, usageErr("%v", err)
	}
	allowed := append(slices.Clone(render.FieldNames), cfg.ProjectionNames()...)
	fields := []string{}
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !slices.Contains(allowed, name) {
			return nil, usageErr("unknown field %q (allowed: %s)", name, strings.Join(allowed, ", "))
		}
		fields = append(fields, name)
	}
	if len(fields) == 0 {
		return []string{render.FieldID}, nil
	}
	return fields, nil
}

// withProjections fills in the projection columns a listing was asked for. The
// projections are evaluated over the whole graph, so a listing that names none
// pays nothing for them.
func withProjections(g *model.Graph, cfg config.Config, records []render.Record, fields []string) []render.Record {
	names := []string{}
	for _, name := range fields {
		if _, ok := cfg.Projection(name); ok {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return records
	}
	return render.WithProjections(records, names, graph.EvalProjections(g, cfg))
}
