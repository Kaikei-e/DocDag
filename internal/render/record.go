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
// the reference layer rather than a typed edge. Projections carries the derived
// columns a listing was asked for and Fields the frontmatter ones the
// configuration declares; both are empty unless a column asked for them.
type Record struct {
	ID          model.ID          `json:"id"`
	Title       string            `json:"title"`
	Status      string            `json:"status"`
	Path        string            `json:"path"`
	Reference   bool              `json:"reference,omitempty"`
	Projections map[string]string `json:"projections,omitempty"`
	Fields      map[string]string `json:"fields,omitempty"`
}

// noField is what a column stands in with where the document writes nothing: a
// row has to keep its shape, and an empty cell in a tab-separated line is a
// column a reader has to count to find.
const noField = "-"

func (r Record) field(name string) string {
	switch name {
	case FieldTitle:
		return r.Title
	case FieldStatus:
		return r.Status
	case FieldPath:
		return r.Path
	case FieldID:
		return r.ID.String()
	}
	if value, ok := r.Projections[name]; ok {
		return value
	}
	if value, ok := r.Fields[name]; ok {
		return value
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

// WithProjections fills the named projection columns into a listing, in place,
// and returns it. A projection is derived rather than written down, so a record
// carries it as the text an attribute clause reads it as.
func WithProjections(records []Record, names []string, projections graph.Projections) []Record {
	if len(names) == 0 {
		return records
	}
	for i := range records {
		values := make(map[string]string, len(names))
		for _, name := range names {
			values[name] = graph.ProjectionValue(projections.Holds(name, records[i].ID))
		}
		records[i].Projections = values
	}
	return records
}

// WithFields fills the named frontmatter columns into a listing, in place, and
// returns it. A key the document does not write, or writes as a list, has no
// scalar to show and reads as the placeholder: the column is about what one
// document says under one key.
func WithFields(records []Record, names []string, g *model.Graph) []Record {
	if len(names) == 0 {
		return records
	}
	for i := range records {
		values := make(map[string]string, len(names))
		for _, name := range names {
			values[name] = noField
			if n, ok := g.Node(records[i].ID); ok {
				if value, written := n.Attr(name); written {
					values[name] = value
				}
			}
		}
		records[i].Fields = values
	}
	return records
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
