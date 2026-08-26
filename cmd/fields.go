package cmd

import (
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Kaikei-e/DocDag/internal/render"
)

const flagFields = "fields"

// addFieldsFlag declares the field selection a listing command answers in.
func addFieldsFlag(cmd *cobra.Command) {
	cmd.Flags().String(flagFields, render.FieldID,
		"comma-separated text columns: "+strings.Join(render.FieldNames, ", ")+" (json carries them all)")
}

// recordFields reads the requested text columns, in the order they were asked
// for.
func recordFields(cmd *cobra.Command) ([]string, error) {
	raw, err := cmd.Flags().GetString(flagFields)
	if err != nil {
		return nil, usageErr("%v", err)
	}
	fields := []string{}
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if !slices.Contains(render.FieldNames, name) {
			return nil, usageErr("unknown field %q (allowed: %s)", name, strings.Join(render.FieldNames, ", "))
		}
		fields = append(fields, name)
	}
	if len(fields) == 0 {
		return []string{render.FieldID}, nil
	}
	return fields, nil
}
