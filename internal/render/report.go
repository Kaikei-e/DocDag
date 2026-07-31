package render

import (
	"io"

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
	return model.ErrNotImplemented
}

// FindingsJSON writes the findings and the summary as a JSON report.
func FindingsJSON(w io.Writer, findings []model.Finding, summary model.Summary) error {
	return model.ErrNotImplemented
}

// IDsText writes one identifier per line.
func IDsText(w io.Writer, ids []model.ID) error { return model.ErrNotImplemented }

// IDsJSON writes identifiers as a JSON array.
func IDsJSON(w io.Writer, ids []model.ID) error { return model.ErrNotImplemented }

// QueryText writes one query result per line, marking reference-layer hits.
func QueryText(w io.Writer, results []graph.QueryResult) error { return model.ErrNotImplemented }

// QueryJSON writes query results as JSON.
func QueryJSON(w io.Writer, results []graph.QueryResult) error { return model.ErrNotImplemented }

// StatsText writes the corpus statistics as an aligned text report.
func StatsText(w io.Writer, s graph.Statistics) error { return model.ErrNotImplemented }

// StatsJSON writes the corpus statistics as JSON.
func StatsJSON(w io.Writer, s graph.Statistics) error { return model.ErrNotImplemented }
