package render

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/model"
)

// DiffSchemaVersion is the version of the JSON diff report.
const DiffSchemaVersion = 1

const diffTitle = "docdag diff"

// DiffReport is the JSON shape of `docdag diff`.
type DiffReport struct {
	SchemaVersion int                  `json:"schema_version"`
	PresetVersion int                  `json:"preset_version,omitempty"`
	AsOf          string               `json:"as_of,omitempty"`
	At            string               `json:"at,omitempty"`
	Edges         graph.EdgeDiff       `json:"edges"`
	Status        []graph.StatusChange `json:"status"`
	Documents     graph.DocumentDiff   `json:"documents"`
	Binding       graph.BindingDelta   `json:"binding"`
}

type diffEntry struct {
	loc  model.Location
	line string
}

// DiffText writes each graph change as an aligned line, ending with the
// binding-set delta. When nothing differs, only the binding line is printed.
func DiffText(w io.Writer, d graph.GraphDiff) error {
	out := &errWriter{w: w}
	for _, entry := range diffEntries(d) {
		out.printf("%s\n", entry.line)
	}
	out.printf("%s\n", bindingLine(d.Binding))
	if out.err != nil {
		return fmt.Errorf("write diff: %w", out.err)
	}
	return nil
}

// DiffGitHub writes one GitHub Actions notice annotation per change, followed
// by the binding-set delta as a plain summary line.
func DiffGitHub(w io.Writer, d graph.GraphDiff) error {
	out := &errWriter{w: w}
	for _, entry := range diffEntries(d) {
		writeAnnotation(out, "notice", entry.loc, diffTitle, entry.line)
	}
	out.printf("%s\n", bindingLine(d.Binding))
	if out.err != nil {
		return fmt.Errorf("write diff: %w", out.err)
	}
	return nil
}

// DiffJSON writes the graph diff as a versioned JSON report.
func DiffJSON(w io.Writer, d graph.GraphDiff, header Header) error {
	report := DiffReport{
		SchemaVersion: DiffSchemaVersion,
		PresetVersion: header.PresetVersion,
		AsOf:          header.AsOf,
		At:            header.At,
		Edges:         nonNilEdgeDiff(d.Edges),
		Status:        nonNilSlice(d.Status),
		Documents:     nonNilDocumentDiff(d.Documents),
		Binding:       d.Binding,
	}
	if err := writeJSON(w, report); err != nil {
		return fmt.Errorf("write diff: %w", err)
	}
	return nil
}

func nonNilEdgeDiff(e graph.EdgeDiff) graph.EdgeDiff {
	return graph.EdgeDiff{
		Added:   nonNilSlice(e.Added),
		Removed: nonNilSlice(e.Removed),
	}
}

func nonNilDocumentDiff(d graph.DocumentDiff) graph.DocumentDiff {
	return graph.DocumentDiff{
		Added:   nonNilSlice(d.Added),
		Removed: nonNilSlice(d.Removed),
	}
}

func nonNilSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func diffEntries(d graph.GraphDiff) []diffEntry {
	type rawEntry struct {
		op    string
		label string
		rest  string
		loc   model.Location
	}
	total := len(d.Edges.Added) + len(d.Edges.Removed) + len(d.Status) + len(d.Documents.Added) + len(d.Documents.Removed)
	if total == 0 {
		return nil
	}
	raw := make([]rawEntry, 0, total)
	for _, e := range d.Edges.Added {
		raw = append(raw, rawEntry{"+", e.Type.String(), fmt.Sprintf("%s -> %s", e.From, e.To), e.Location})
	}
	for _, e := range d.Edges.Removed {
		raw = append(raw, rawEntry{"-", e.Type.String(), fmt.Sprintf("%s -> %s", e.From, e.To), e.Location})
	}
	for _, s := range d.Status {
		raw = append(raw, rawEntry{"~", "status", fmt.Sprintf("%s: %s -> %s", s.ID, s.From, s.To), s.Location})
	}
	for _, doc := range d.Documents.Added {
		raw = append(raw, rawEntry{"+", "document", fmt.Sprintf("%s [%s]", doc.ID, doc.Status), doc.Location})
	}
	for _, doc := range d.Documents.Removed {
		raw = append(raw, rawEntry{"-", "document", fmt.Sprintf("%s [%s]", doc.ID, doc.Status), doc.Location})
	}

	var buf bytes.Buffer
	tw := tabwriter.NewWriter(&buf, 0, 0, 1, ' ', 0)
	for _, r := range raw {
		fmt.Fprintf(tw, "%s %s\t%s\n", r.op, r.label, r.rest)
	}
	_ = tw.Flush()

	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	entries := make([]diffEntry, len(raw))
	for i, r := range raw {
		entries[i] = diffEntry{loc: r.loc, line: lines[i]}
	}
	return entries
}

func bindingLine(b graph.BindingDelta) string {
	return fmt.Sprintf("binding: %d -> %d (%s)", b.Before, b.After, formatDelta(b.Delta))
}

func formatDelta(delta int) string {
	if delta > 0 {
		return fmt.Sprintf("+%d", delta)
	}
	return fmt.Sprintf("%d", delta)
}
