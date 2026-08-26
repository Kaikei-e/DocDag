package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// Fields a listing can be printed with.
const (
	FieldID     = "id"
	FieldTitle  = "title"
	FieldStatus = "status"
	FieldPath   = "path"
)

// FieldNames lists every field a listing answers in, in the order a record
// carries them.
var FieldNames = []string{FieldID, FieldTitle, FieldStatus, FieldPath}

// Record is one document in a listing. Reference marks a hit reached through
// the reference layer rather than a typed edge.
type Record struct {
	ID        model.ID `json:"id"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	Path      string   `json:"path"`
	Reference bool     `json:"reference,omitempty"`
}

func (r Record) field(name string) string {
	switch name {
	case FieldTitle:
		return r.Title
	case FieldStatus:
		return r.Status
	case FieldPath:
		return r.Path
	}
	return r.ID.String()
}

// Records describes documents by identifier.
func Records(g *model.Graph, ids []model.ID) []Record {
	out := make([]Record, 0, len(ids))
	for _, id := range ids {
		out = append(out, newRecord(g, id))
	}
	return out
}

// QueryRecords describes query results, marking the reference-layer hits.
func QueryRecords(g *model.Graph, results []graph.QueryResult) []Record {
	out := make([]Record, 0, len(results))
	for _, r := range results {
		record := newRecord(g, r.ID)
		record.Reference = r.Layer == graph.LayerReference
		out = append(out, record)
	}
	return out
}

func newRecord(g *model.Graph, id model.ID) Record {
	n, ok := g.Node(id)
	if !ok {
		return Record{ID: id}
	}
	return Record{ID: n.ID, Title: n.Title, Status: n.Status, Path: n.Path}
}

// RecordsText writes one record per line, the named fields separated by tabs.
// With no fields it writes identifiers alone.
func RecordsText(w io.Writer, records []Record, fields []string) error {
	if len(fields) == 0 {
		fields = []string{FieldID}
	}
	out := &errWriter{w: w}
	values := make([]string, 0, len(fields))
	for _, r := range records {
		values = values[:0]
		for _, name := range fields {
			values = append(values, r.field(name))
		}
		line := strings.Join(values, "\t")
		if r.Reference {
			line += " (reference)"
		}
		out.printf("%s\n", line)
	}
	if out.err != nil {
		return fmt.Errorf("write records: %w", out.err)
	}
	return nil
}

// RecordsJSON writes records as a JSON array of objects.
func RecordsJSON(w io.Writer, records []Record) error {
	if records == nil {
		records = []Record{}
	}
	if err := writeJSON(w, records); err != nil {
		return fmt.Errorf("write records: %w", err)
	}
	return nil
}
