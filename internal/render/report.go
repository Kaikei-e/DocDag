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
// that reads it can tell a shape change from a content change. Version 2 heads
// the report with the preset revision the corpus was checked under.
const ReportSchemaVersion = 2

// Tool identity carried by the machine-readable report formats.
const (
	toolName = "docdag"
	toolURL  = "https://github.com/Kaikei-e/DocDag"
)

// Header is what a machine-readable report carries beside its findings: the
// revision of the preset the corpus is written against, the day the
// time-dependent checks were given for, and the revision the documents were
// read from. Each is left out where there is nothing to say, and together they
// are what makes a report reproducible: the same documents, read at the same
// revision, asked about the same day, answer the same way.
type Header struct {
	PresetVersion int
	AsOf          string
	At            string
}

// Report is the JSON shape of `docdag validate`.
type Report struct {
	SchemaVersion int             `json:"schema_version"`
	PresetVersion int             `json:"preset_version,omitempty"`
	AsOf          string          `json:"as_of,omitempty"`
	At            string          `json:"at,omitempty"`
	Findings      []model.Finding `json:"findings"`
	Summary       model.Summary   `json:"summary"`
}

// FindingsText writes one line per finding, errors first, then a summary line.
// asOf is the day the run asked about, written only where the corpus has an
// answer that depends on it; the closing line carries it, so a report a person
// reads says which day it is about without a line in front of the findings.
func FindingsText(w io.Writer, findings []model.Finding, summary model.Summary, asOf string) error {
	out := &errWriter{w: w}
	for _, f := range findings {
		out.printf("%s%s %s%s: %s\n", locationPrefix(f.Location), strings.ToUpper(string(f.Severity)), f.Rule, subject(f.ID), f.Detail)
		if f.Fix != "" {
			out.printf("  fix: %s\n", f.Fix)
		}
	}
	writeSummary(out, summary, asOf)
	if out.err != nil {
		return fmt.Errorf("write findings: %w", out.err)
	}
	return nil
}

// subject renders the identifier a finding is filed against, as the space and
// name that follow the rule. A finding about a file that yields no identifier
// at all has none to name, and an empty one would leave a gap the reader has to
// wonder about.
func subject(id model.ID) string {
	if id == "" {
		return ""
	}
	return " " + id.String()
}

// subjectDetail is what an annotation says: the identifier the finding is
// filed against and its detail, or the detail alone where there is no
// identifier to name.
func subjectDetail(f model.Finding) string {
	if f.ID == "" {
		return f.Detail
	}
	return f.ID.String() + ": " + f.Detail
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
// reassuring summary — only the day it was asked about, where that day decides
// anything.
func writeSummary(out *errWriter, summary model.Summary, asOf string) {
	if summary.Errors > 0 {
		if asOf != "" {
			out.printf("as of %s\n", asOf)
		}
		return
	}
	line := fmt.Sprintf("OK: %d docs, %d typed edges, no cycles", summary.Documents, summary.Edges)
	if asOf != "" {
		line += ", as of " + asOf
	}
	out.printf("%s\n", line)
}

// FindingsJSON writes the findings and the summary as a JSON report, headed by
// the schema version and everything the header says about the run.
func FindingsJSON(w io.Writer, findings []model.Finding, summary model.Summary, header Header) error {
	report := Report{
		SchemaVersion: ReportSchemaVersion,
		PresetVersion: header.PresetVersion,
		AsOf:          header.AsOf,
		At:            header.At,
		Findings:      findings,
		Summary:       summary,
	}
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
func FindingsGitHub(w io.Writer, findings []model.Finding, summary model.Summary, asOf string) error {
	out := &errWriter{w: w}
	writeAnnotations(out, findings)
	writeSummary(out, summary, asOf)
	if out.err != nil {
		return fmt.Errorf("write findings: %w", out.err)
	}
	return nil
}

// writeAnnotations writes the workflow command every finding is annotated with,
// whichever report is being written.
func writeAnnotations(out *errWriter, findings []model.Finding) {
	for _, f := range findings {
		level := githubLevel(f.Severity)
		properties := []string{"file=" + escapeProperty(f.Location.Path)}
		if f.Location.Line > 0 {
			properties = append(properties, fmt.Sprintf("line=%d", f.Location.Line))
		}
		if f.Location.Column > 0 {
			properties = append(properties, fmt.Sprintf("col=%d", f.Location.Column))
		}
		properties = append(properties, "title="+escapeProperty(f.Rule))
		out.printf("::%s %s::%s\n", level, strings.Join(properties, ","), escapeData(subjectDetail(f)))
	}
}

// githubLevel names the annotation level one severity is rendered at. An info
// finding is a notice: the workflow command set has exactly that word for
// something a reader should see and no job should fail on.
func githubLevel(s model.Severity) string {
	switch s {
	case model.SeverityWarn:
		return "warning"
	case model.SeverityInfo:
		return "notice"
	}
	return "error"
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
	// The result-level severity is the default for a diagnostic that carries
	// none, so it follows the strongest finding in the report.
	worst := model.SeverityError
	if summary.Errors == 0 && summary.Warnings > 0 {
		worst = model.SeverityWarn
	}
	return writeRDJSON(w, findings, worst)
}

// writeRDJSON writes a diagnostic result, whichever report is being written.
func writeRDJSON(w io.Writer, findings []model.Finding, worst model.Severity) error {
	result := rdjson{
		Source:      rdjsonSource{Name: toolName, URL: toolURL},
		Severity:    rdjsonSeverity(worst),
		Diagnostics: make([]rdjsonDiagnostic, 0, len(findings)),
	}
	for _, f := range findings {
		d := rdjsonDiagnostic{
			Message:  f.Rule + subject(f.ID) + ": " + f.Detail,
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
	switch s {
	case model.SeverityWarn:
		return "WARNING"
	case model.SeverityInfo:
		return "INFO"
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

// PlanSchemaVersion is the version of the JSON plan `docdag new --dry-run`
// writes.
const PlanSchemaVersion = 1

// Plan is what `docdag new` would write, reported instead of written. Exists
// marks the path as a document the corpus already holds, and is always written
// out: a consumer has to be able to read "nothing would be written" too.
type Plan struct {
	SchemaVersion int           `json:"schema_version"`
	ID            model.ID      `json:"id"`
	Path          string        `json:"path"`
	Exists        bool          `json:"exists"`
	Rewrites      []PlanRewrite `json:"rewrites"`
}

// PlanRewrite is one status change a plan applies to a superseded document.
type PlanRewrite struct {
	Path   string `json:"path"`
	Status string `json:"status"`
}

// CreationPlan writes a plan as a line per action, or as the JSON object a
// machine reads it from.
func CreationPlan(w io.Writer, plan Plan, field string, asJSON bool) error {
	plan.SchemaVersion = PlanSchemaVersion
	if plan.Rewrites == nil {
		plan.Rewrites = []PlanRewrite{}
	}
	if asJSON {
		if err := writeJSON(w, plan); err != nil {
			return fmt.Errorf("write plan: %w", err)
		}
		return nil
	}
	out := &errWriter{w: w}
	verb := "create"
	if plan.Exists {
		verb = "exists"
	}
	out.printf("%s %s %s\n", verb, plan.ID, plan.Path)
	for _, r := range plan.Rewrites {
		out.printf("rewrite %s %s: %s\n", r.Path, field, r.Status)
	}
	if out.err != nil {
		return fmt.Errorf("write plan: %w", out.err)
	}
	return nil
}

// StatsText writes the corpus statistics as an aligned text report. asOf is the
// day the counts are about, written as a row of its own only where the corpus
// has counts that depend on it.
func StatsText(w io.Writer, s graph.Statistics, asOf string) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if asOf != "" {
		fmt.Fprintf(tw, "as of\t%s\n", asOf)
	}
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
	for _, m := range s.Modalities {
		fmt.Fprintf(tw, "modality %s\t%d\n", m.Modality, m.Count)
	}
	for _, t := range s.Topics {
		fmt.Fprintf(tw, "clauses about %s\t%d\n", t.Topic, t.Clauses)
	}
	if len(s.Topics) > 0 {
		fmt.Fprintf(tw, "suppressed conflicts\t%d\n", s.SuppressedConflicts)
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("write statistics: %w", err)
	}
	return nil
}

// StatsReport is the JSON shape of `docdag stats`: the counts, headed by the
// day and the revision they are about. The statistics are embedded rather than
// nested, so the fields a consumer already reads stay where they were.
type StatsReport struct {
	AsOf string `json:"as_of,omitempty"`
	At   string `json:"at,omitempty"`
	graph.Statistics
}

// StatsJSON writes the corpus statistics as JSON.
func StatsJSON(w io.Writer, s graph.Statistics, header Header) error {
	if err := writeJSON(w, StatsReport{AsOf: header.AsOf, At: header.At, Statistics: s}); err != nil {
		return fmt.Errorf("write statistics: %w", err)
	}
	return nil
}

// noValue stands in for a column a report has no answer for, so every row has
// the same number of columns and a reader never has to count separators.
const noValue = "-"

// FieldUsageText writes the per-field usage as an aligned table. It carries a
// header row, unlike the degree statistics: four columns of numbers and dates
// say nothing about themselves.
func FieldUsageText(w io.Writer, usage []graph.FieldUsage) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "field\tdocuments\tlast change\tdeprecated\n")
	for _, u := range usage {
		deprecated := noValue
		if u.Deprecated {
			deprecated = "yes"
		}
		last := u.LastChange
		if last == "" {
			last = noValue
		}
		fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n", u.Field, u.Documents, last, deprecated)
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("write field usage: %w", err)
	}
	return nil
}

// FieldUsageReport is the JSON shape of `docdag stats --fields`: an object
// rather than a bare array, so a later revision can head the rows with
// something without moving them.
type FieldUsageReport struct {
	Fields []graph.FieldUsage `json:"fields"`
}

// FieldUsageJSON writes the per-field usage as JSON.
func FieldUsageJSON(w io.Writer, usage []graph.FieldUsage) error {
	if usage == nil {
		usage = []graph.FieldUsage{}
	}
	if err := writeJSON(w, FieldUsageReport{Fields: usage}); err != nil {
		return fmt.Errorf("write field usage: %w", err)
	}
	return nil
}
