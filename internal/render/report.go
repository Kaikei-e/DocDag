package render

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// Report is the JSON shape of `docdag validate`.
type Report struct {
	Findings []model.Finding `json:"findings"`
	Summary  model.Summary   `json:"summary"`
}

// FindingsText writes one line per finding, errors first, then a summary line.
func FindingsText(w io.Writer, findings []model.Finding, summary model.Summary) error {
	out := &errWriter{w: w}
	for _, f := range findings {
		out.printf("%s %s %s: %s\n", strings.ToUpper(string(f.Severity)), f.Rule, f.ID, f.Detail)
	}
	// A corpus that failed gets no reassuring summary line.
	if summary.Errors == 0 {
		out.printf("OK: %d docs, %d typed edges, no cycles\n", summary.Documents, summary.Edges)
	}
	if out.err != nil {
		return fmt.Errorf("write findings: %w", out.err)
	}
	return nil
}

// FindingsJSON writes the findings and the summary as a JSON report.
func FindingsJSON(w io.Writer, findings []model.Finding, summary model.Summary) error {
	report := Report{Findings: findings, Summary: summary}
	if report.Findings == nil {
		report.Findings = []model.Finding{}
	}
	if err := writeJSON(w, report); err != nil {
		return fmt.Errorf("write findings: %w", err)
	}
	return nil
}

// IDsText writes one identifier per line.
func IDsText(w io.Writer, ids []model.ID) error {
	out := &errWriter{w: w}
	for _, id := range ids {
		out.printf("%s\n", id)
	}
	if out.err != nil {
		return fmt.Errorf("write identifiers: %w", out.err)
	}
	return nil
}

// IDsJSON writes identifiers as a JSON array.
func IDsJSON(w io.Writer, ids []model.ID) error {
	if ids == nil {
		ids = []model.ID{}
	}
	if err := writeJSON(w, ids); err != nil {
		return fmt.Errorf("write identifiers: %w", err)
	}
	return nil
}

// CreatedPath writes the path of a created document, as a bare line or as the
// JSON object every command answers with under --format json.
func CreatedPath(w io.Writer, path string, asJSON bool) error {
	if asJSON {
		if err := writeJSON(w, map[string]string{"path": path}); err != nil {
			return fmt.Errorf("write created path: %w", err)
		}
		return nil
	}
	out := &errWriter{w: w}
	out.printf("%s\n", path)
	if out.err != nil {
		return fmt.Errorf("write created path: %w", out.err)
	}
	return nil
}

// QueryText writes one query result per line, marking reference-layer hits.
func QueryText(w io.Writer, results []graph.QueryResult) error {
	out := &errWriter{w: w}
	for _, r := range results {
		if r.Layer == graph.LayerReference {
			out.printf("%s (reference)\n", r.ID)
			continue
		}
		out.printf("%s\n", r.ID)
	}
	if out.err != nil {
		return fmt.Errorf("write query results: %w", out.err)
	}
	return nil
}

// QueryJSON writes query results as JSON.
func QueryJSON(w io.Writer, results []graph.QueryResult) error {
	if results == nil {
		results = []graph.QueryResult{}
	}
	if err := writeJSON(w, results); err != nil {
		return fmt.Errorf("write query results: %w", err)
	}
	return nil
}

// StatsText writes the corpus statistics as an aligned text report.
func StatsText(w io.Writer, s graph.Statistics) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "documents\t%d\n", s.Documents)
	fmt.Fprintf(tw, "binding\t%d\n", s.Binding)
	fmt.Fprintf(tw, "orphans\t%d (%.1f%%)\n", s.Orphans, s.OrphanRate*100)
	for _, e := range s.Edges {
		fmt.Fprintf(tw, "%s edges\t%d\n", e.Type, e.Count)
	}
	for _, d := range s.ChainDepth {
		fmt.Fprintf(tw, "chain depth %d\t%d\n", d.Depth, d.Count)
	}
	for _, r := range s.TopReferenced {
		fmt.Fprintf(tw, "references to %s\t%d\n", r.ID, r.Count)
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("write statistics: %w", err)
	}
	return nil
}

// StatsJSON writes the corpus statistics as JSON.
func StatsJSON(w io.Writer, s graph.Statistics) error {
	if err := writeJSON(w, s); err != nil {
		return fmt.Errorf("write statistics: %w", err)
	}
	return nil
}
