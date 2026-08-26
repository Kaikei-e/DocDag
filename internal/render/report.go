package render

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// ReportSchemaVersion is the version of the JSON validation report. A consumer
// that reads it can tell a shape change from a content change.
const ReportSchemaVersion = 1

// Tool identity carried by the machine-readable report formats.
const (
	toolName = "docdag"
	toolURL  = "https://github.com/Kaikei-e/DocDag"
)

// Report is the JSON shape of `docdag validate`.
type Report struct {
	SchemaVersion int             `json:"schema_version"`
	Findings      []model.Finding `json:"findings"`
	Summary       model.Summary   `json:"summary"`
}

// FindingsText writes one line per finding, errors first, then a summary line.
func FindingsText(w io.Writer, findings []model.Finding, summary model.Summary) error {
	out := &errWriter{w: w}
	for _, f := range findings {
		out.printf("%s%s %s %s: %s\n", locationPrefix(f.Location), strings.ToUpper(string(f.Severity)), f.Rule, f.ID, f.Detail)
		if f.Fix != "" {
			out.printf("  fix: %s\n", f.Fix)
		}
	}
	writeSummary(out, summary)
	if out.err != nil {
		return fmt.Errorf("write findings: %w", out.err)
	}
	return nil
}

// locationPrefix renders "<path>:<line>: ", dropping whichever part is unknown
// so an editor can still jump to what is left.
func locationPrefix(loc model.Location) string {
	switch {
	case loc.Path == "":
		return ""
	case loc.Line == 0:
		return loc.Path + ": "
	}
	return fmt.Sprintf("%s:%d: ", loc.Path, loc.Line)
}

// writeSummary appends the closing line. A corpus that failed gets no
// reassuring summary.
func writeSummary(out *errWriter, summary model.Summary) {
	if summary.Errors == 0 {
		out.printf("OK: %d docs, %d typed edges, no cycles\n", summary.Documents, summary.Edges)
	}
}

// FindingsJSON writes the findings and the summary as a JSON report.
func FindingsJSON(w io.Writer, findings []model.Finding, summary model.Summary) error {
	report := Report{SchemaVersion: ReportSchemaVersion, Findings: findings, Summary: summary}
	if report.Findings == nil {
		report.Findings = []model.Finding{}
	}
	if err := writeJSON(w, report); err != nil {
		return fmt.Errorf("write findings: %w", err)
	}
	return nil
}

// FindingsGitHub writes one GitHub Actions workflow command per finding, then
// the text summary. A workflow step renders at most ten annotations, so the
// summary is what a reader sees when a corpus fails widely.
func FindingsGitHub(w io.Writer, findings []model.Finding, summary model.Summary) error {
	out := &errWriter{w: w}
	for _, f := range findings {
		level := "error"
		if f.Severity == model.SeverityWarn {
			level = "warning"
		}
		properties := []string{"file=" + escapeProperty(f.Location.Path)}
		if f.Location.Line > 0 {
			properties = append(properties, fmt.Sprintf("line=%d", f.Location.Line))
		}
		if f.Location.Column > 0 {
			properties = append(properties, fmt.Sprintf("col=%d", f.Location.Column))
		}
		properties = append(properties, "title="+escapeProperty(f.Rule))
		out.printf("::%s %s::%s\n", level, strings.Join(properties, ","), escapeData(f.ID.String()+": "+f.Detail))
	}
	writeSummary(out, summary)
	if out.err != nil {
		return fmt.Errorf("write findings: %w", out.err)
	}
	return nil
}

// escapeData and escapeProperty apply the escaping the workflow command parser
// expects, so a path holding a separator does not truncate an annotation.
func escapeData(value string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(value)
}

func escapeProperty(value string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A", ":", "%3A", ",", "%2C").Replace(value)
}

// rdjson mirrors the reviewdog diagnostic format; the JSON names are the ones
// its proto declares.
type rdjson struct {
	Source      rdjsonSource       `json:"source"`
	Severity    string             `json:"severity"`
	Diagnostics []rdjsonDiagnostic `json:"diagnostics"`
}

type rdjsonSource struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type rdjsonDiagnostic struct {
	Message          string          `json:"message"`
	Location         rdjsonLocation  `json:"location"`
	Severity         string          `json:"severity"`
	Code             rdjsonCode      `json:"code"`
	RelatedLocations []rdjsonRelated `json:"related_locations,omitempty"`
}

type rdjsonLocation struct {
	Path  string       `json:"path"`
	Range *rdjsonRange `json:"range,omitempty"`
}

type rdjsonRange struct {
	Start rdjsonPosition `json:"start"`
}

type rdjsonPosition struct {
	Line   int `json:"line"`
	Column int `json:"column,omitempty"`
}

type rdjsonCode struct {
	Value string `json:"value"`
}

type rdjsonRelated struct {
	Location rdjsonLocation `json:"location"`
}

// FindingsRDJSON writes the findings in the reviewdog diagnostic format, which
// carries no summary: it is read by a machine.
func FindingsRDJSON(w io.Writer, findings []model.Finding, summary model.Summary) error {
	result := rdjson{
		Source:      rdjsonSource{Name: toolName, URL: toolURL},
		Severity:    rdjsonSeverity(model.SeverityError),
		Diagnostics: make([]rdjsonDiagnostic, 0, len(findings)),
	}
	// The result-level severity is the default for a diagnostic that carries
	// none, so it follows the strongest finding in the report.
	if summary.Errors == 0 && summary.Warnings > 0 {
		result.Severity = rdjsonSeverity(model.SeverityWarn)
	}
	for _, f := range findings {
		d := rdjsonDiagnostic{
			Message:  fmt.Sprintf("%s %s: %s", f.Rule, f.ID, f.Detail),
			Location: rdjsonLocationOf(f.Location),
			Severity: rdjsonSeverity(f.Severity),
			Code:     rdjsonCode{Value: f.Rule},
		}
		for _, related := range f.Related {
			d.RelatedLocations = append(d.RelatedLocations, rdjsonRelated{Location: rdjsonLocationOf(related)})
		}
		result.Diagnostics = append(result.Diagnostics, d)
	}
	if err := writeJSON(w, result); err != nil {
		return fmt.Errorf("write findings: %w", err)
	}
	return nil
}

func rdjsonLocationOf(loc model.Location) rdjsonLocation {
	out := rdjsonLocation{Path: loc.Path}
	if loc.Line > 0 {
		out.Range = &rdjsonRange{Start: rdjsonPosition{Line: loc.Line, Column: loc.Column}}
	}
	return out
}

func rdjsonSeverity(s model.Severity) string {
	if s == model.SeverityWarn {
		return "WARNING"
	}
	return "ERROR"
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
