package render

import (
	"fmt"
	"io"
	"slices"
	"strings"
)

// PrimeSchemaVersion is the version of the JSON brief `docdag prime` writes.
const PrimeSchemaVersion = 1

// PrimeKind describes one document kind in a multi-kind corpus brief.
type PrimeKind struct {
	Name      string `json:"name"`
	Dir       string `json:"dir"`
	IDPattern string `json:"id_pattern,omitempty"`
}

// PrimeValidation summarizes corpus validation health for `docdag prime`.
type PrimeValidation struct {
	OK       bool   `json:"ok"`
	Errors   int    `json:"errors"`
	Warnings int    `json:"warnings"`
	Cycles   int    `json:"cycles"`
	Summary  string `json:"summary"`
}

// PrimeCommand pairs an agent intent with the tailored docdag invocation.
type PrimeCommand struct {
	Intent  string `json:"intent"`
	Command string `json:"command"`
}

// PrimeReport is the JSON shape of `docdag prime`.
type PrimeReport struct {
	SchemaVersion    int             `json:"schema_version"`
	Preset           string          `json:"preset,omitempty"`
	PresetVersion    int             `json:"preset_version,omitempty"`
	AsOf             string          `json:"as_of,omitempty"`
	At               string          `json:"at,omitempty"`
	Dirs             []string        `json:"dirs"`
	Kinds            []PrimeKind     `json:"kinds,omitempty"`
	Edges            []string        `json:"edges,omitempty"`
	Fields           []string        `json:"fields,omitempty"`
	Projections      []string        `json:"projections,omitempty"`
	RequiredSections []string        `json:"required_sections,omitempty"`
	HasPeriods       bool            `json:"has_periods"`
	HasSupersedes    bool            `json:"has_supersedes"`
	HasModality      bool            `json:"has_modality"`
	Documents        int             `json:"documents"`
	Binding          *int            `json:"binding,omitempty"`
	Validation       PrimeValidation `json:"validation"`
	SampleRef        string          `json:"sample_ref,omitempty"`
	Commands         []PrimeCommand  `json:"commands"`
}

// PrimeHint is the JSON and text payload of `docdag prime --hint <path>`.
type PrimeHint struct {
	Managed bool   `json:"managed"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Hint    string `json:"hint"`
}

// PrimeText writes the compact, corpus-adaptive brief for coding agents.
func PrimeText(w io.Writer, r PrimeReport) error {
	out := &errWriter{w: w}
	out.printf("docdag validate: %s\n", r.Validation.Summary)
	if r.Validation.Errors > 0 {
		out.printf("run docdag validate to see the findings\n")
	}
	out.printf("corpus: %s\n", primeCorpusSummary(r))
	if len(r.Kinds) > 0 {
		out.printf("kinds: %s\n", primeKindsLine(r.Kinds))
	}
	if len(r.Edges) > 0 {
		edges := slices.Clone(r.Edges)
		slices.Sort(edges)
		out.printf("edges: %s\n", strings.Join(edges, ", "))
	}
	if len(r.Fields) > 0 {
		fields := slices.Clone(r.Fields)
		slices.Sort(fields)
		out.printf("fields: %s\n", strings.Join(fields, ", "))
	}
	if len(r.RequiredSections) > 0 {
		out.printf("required sections: %s\n", strings.Join(r.RequiredSections, ", "))
	}
	out.printf("Prefer docdag over Glob/Grep/Read on managed docs (one call replaces reading the directory):\n")
	for _, cmd := range r.Commands {
		switch {
		case cmd.Intent == "context" && cmd.Command == "docdag context <ref>" && r.SampleRef != "":
			out.printf("  %s (e.g. docdag context %s)\n", cmd.Command, r.SampleRef)
		case cmd.Intent == "impact":
			out.printf("  %s  # impact\n", cmd.Command)
		default:
			out.printf("  %s\n", cmd.Command)
		}
	}
	if r.HasPeriods {
		out.printf("  add --as-of YYYY-MM-DD to query/validate for another day\n")
	}
	if out.err != nil {
		return fmt.Errorf("write prime report: %w", out.err)
	}
	return nil
}

func primeCorpusSummary(r PrimeReport) string {
	var meta []string
	if r.Preset != "" {
		meta = append(meta, "preset: "+r.Preset)
	}
	meta = append(meta, fmt.Sprintf("%d docs", r.Documents))
	if r.Binding != nil {
		meta = append(meta, fmt.Sprintf("%d binding", *r.Binding))
	}
	target := strings.Join(r.Dirs, ", ")
	if len(r.Kinds) > 0 {
		target = fmt.Sprintf("%d kinds", len(r.Kinds))
	}
	return fmt.Sprintf("%s (%s)", target, strings.Join(meta, ", "))
}

func primeKindsLine(kinds []PrimeKind) string {
	parts := make([]string, 0, len(kinds))
	for _, k := range kinds {
		parts = append(parts, k.Name+"="+k.Dir)
	}
	return strings.Join(parts, ", ")
}

// PrimeJSON writes the corpus brief as a versioned JSON report.
func PrimeJSON(w io.Writer, r PrimeReport) error {
	r.SchemaVersion = PrimeSchemaVersion
	if r.Dirs == nil {
		r.Dirs = []string{}
	}
	if r.Commands == nil {
		r.Commands = []PrimeCommand{}
	}
	if err := writeJSON(w, r); err != nil {
		return fmt.Errorf("write prime report: %w", err)
	}
	return nil
}

// PrimeHintText writes a single-path guidance tip as plain text.
func PrimeHintText(w io.Writer, h PrimeHint) error {
	out := &errWriter{w: w}
	out.printf("%s\n", h.Hint)
	if out.err != nil {
		return fmt.Errorf("write prime hint: %w", out.err)
	}
	return nil
}

// PrimeHintJSON writes a single-path guidance tip as JSON.
func PrimeHintJSON(w io.Writer, h PrimeHint) error {
	if err := writeJSON(w, h); err != nil {
		return fmt.Errorf("write prime hint: %w", err)
	}
	return nil
}
