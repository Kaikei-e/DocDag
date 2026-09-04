package cmd

import (
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/render"
	"github.com/Kaikei-e/DocDag/model"
)

const flagFields = "fields"

// addFieldsFlag declares the field selection a listing command answers in.
func addFieldsFlag(cmd *cobra.Command) {
	cmd.Flags().String(flagFields, render.FieldID,
		"comma-separated text columns: "+strings.Join(render.FieldNames, ", ")+
			", plus the configured projections and declared fields (json carries them all)")
}

// recordFields reads the requested text columns, in the order they were asked
// for. A configured projection is a column too: its value is derived rather
// than written down, but a listing reads it like any other field. So is a key
// the configuration declares under fields: the corpus has said that key is part
// of what a document of that kind is, which is what makes it worth a column.
func recordFields(cmd *cobra.Command, cfg config.Config) ([]string, error) {
	raw, err := cmd.Flags().GetString(flagFields)
	if err != nil {
		return nil, usageErr("%v", err)
	}
	allowed := allowedFields(cfg)
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

func allowedFields(cfg config.Config) []string {
	allowed := append(slices.Clone(render.FieldNames), cfg.ProjectionNames()...)
	return append(allowed, cfg.DeclaredFields()...)
}

// bindingColumns is the default column set of a binding listing: the identifier,
// and beside it the modality where the configuration declares one. A set that
// spans the modalities is unreadable without the column — a permission and a
// prohibition are both binding, and the listing would show two identifiers and
// no way to tell them apart — and a corpus that declares no modality has one
// column, exactly as it always had.
func bindingColumns(cfg config.Config) []string {
	if slices.Contains(cfg.DeclaredFields(), config.FieldModality) {
		return []string{render.FieldID, config.FieldModality}
	}
	return []string{render.FieldID}
}

// withColumns fills in the derived and declared columns a listing was asked
// for. Both are computed over the whole graph, so a listing that names neither
// pays nothing for them.
func withColumns(g *model.Graph, cfg config.Config, records []render.Record, fields []string, asOf time.Time) []render.Record {
	projections, declared := []string{}, []string{}
	for _, name := range fields {
		switch {
		case cfgDeclaresProjection(cfg, name):
			projections = append(projections, name)
		case slices.Contains(cfg.DeclaredFields(), name):
			declared = append(declared, name)
		}
	}
	if len(projections) > 0 {
		records = render.WithProjections(records, projections, graph.EvalProjections(g, cfg, asOf))
	}
	if len(declared) > 0 {
		records = render.WithFields(records, declared, g)
	}
	return records
}

// cfgDeclaresProjection reports whether a column name is a projection, which
// wins over a frontmatter key of the same name for the reason the evaluator
// gives: a derived value must not be taken back by writing the key down.
func cfgDeclaresProjection(cfg config.Config, name string) bool {
	_, ok := cfg.Projection(name)
	return ok
}
