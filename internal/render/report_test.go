package render

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

// testReport validates a fixture corpus the way the CLI does, so the golden
// files pin what a user actually sees.
func testReport(t *testing.T, fixture string) ([]model.Finding, model.Summary) {
	t.Helper()
	cfg := config.ADRPreset()
	cfg.Dir = filepath.Join("..", "..", "testdata", "fixtures", fixture)
	docs, err := parse.Dir(cfg.Dir, cfg)
	if err != nil {
		t.Fatalf("parse %s: %v", cfg.Dir, err)
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	parse.Localize(docs, root)
	g := graph.Build(docs, cfg)
	findings := graph.Suggest(graph.Validate(g, cfg, testAsOf), g, cfg, testAsOf)
	return findings, graph.Summarize(g, findings)
}

// testPresetFindingsJSON writes the report the way the CLI does under the ADR
// preset, so the golden files pin the header a user actually sees.
func testPresetFindingsJSON(w io.Writer, findings []model.Finding, summary model.Summary) error {
	return FindingsJSON(w, findings, summary,
		Header{PresetVersion: config.ADRPreset().PresetVersion, AsOf: testAsOfDay})
}

// testPresetFindingsText and testPresetFindingsGitHub write the two reports a
// person reads. Neither carries an as-of line: the ADR preset declares no
// period, so its answers do not depend on the day and saying one would be
// noise.
func testPresetFindingsText(w io.Writer, findings []model.Finding, summary model.Summary) error {
	return FindingsText(w, findings, summary, "")
}

func testPresetFindingsGitHub(w io.Writer, findings []model.Finding, summary model.Summary) error {
	return FindingsGitHub(w, findings, summary, "")
}

func TestReportGolden(t *testing.T) {
	writers := map[string]func(io.Writer, []model.Finding, model.Summary) error{
		"txt":    testPresetFindingsText,
		"json":   testPresetFindingsJSON,
		"github": testPresetFindingsGitHub,
		"rdjson": FindingsRDJSON,
	}
	for _, fixture := range []string{"ok-madr", "status-drift"} {
		for ext, write := range writers {
			t.Run(fixture+"."+ext, func(t *testing.T) {
				findings, summary := testReport(t, fixture)

				var buf bytes.Buffer
				if err := write(&buf, findings, summary); err != nil {
					t.Fatalf("write report: %v", err)
				}
				testAssertGolden(t, "report-"+fixture+"."+ext, buf.String())

				var again bytes.Buffer
				if err := write(&again, findings, summary); err != nil {
					t.Fatalf("write report: %v", err)
				}
				if again.String() != buf.String() {
					t.Errorf("report is not deterministic:\nfirst:\n%s\nsecond:\n%s", buf.String(), again.String())
				}
			})
		}
	}
}

func TestFindingsTextLineFormat(t *testing.T) {
	tests := []struct {
		name    string
		finding model.Finding
		want    string
	}{
		{
			name: "a located finding names the path and the line",
			finding: model.Finding{
				Severity: model.SeverityError,
				Rule:     model.RuleStatusDrift,
				ID:       "0001",
				Detail:   "has inbound supersedes but status is not superseded",
				Location: model.Location{Path: "docs/adr/0001-a.md", Line: 3},
			},
			want: "docs/adr/0001-a.md:3: ERROR status_drift 0001: has inbound supersedes but status is not superseded",
		},
		{
			name: "an unknown line leaves the path alone",
			finding: model.Finding{
				Severity: model.SeverityWarn,
				Rule:     model.RuleSupersededOrphan,
				ID:       "0002",
				Detail:   "nothing supersedes it",
				Location: model.Location{Path: "docs/adr/0002-b.md"},
			},
			want: "docs/adr/0002-b.md: WARN superseded_orphan 0002: nothing supersedes it",
		},
		{
			name: "a finding without a path keeps the bare form",
			finding: model.Finding{
				Severity: model.SeverityError,
				Rule:     model.RuleCycle,
				ID:       "0003",
				Detail:   "supersedes cycle",
			},
			want: "ERROR cycle 0003: supersedes cycle",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := FindingsText(&buf, []model.Finding{tt.finding}, model.Summary{Errors: 1}, ""); err != nil {
				t.Fatalf("FindingsText: %v", err)
			}
			got := strings.TrimRight(buf.String(), "\n")
			if got != tt.want {
				t.Fatalf("line = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindingsJSONCarriesTheSchemaVersion(t *testing.T) {
	var buf bytes.Buffer
	if err := FindingsJSON(&buf, nil, model.Summary{Documents: 1}, Header{PresetVersion: 3}); err != nil {
		t.Fatalf("FindingsJSON: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(buf.String()), "{\n  \"schema_version\": 2,\n  \"preset_version\": 3,") {
		t.Fatalf("report = %s, want the schema version and the preset version first", buf.String())
	}
	report := Report{}
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if report.SchemaVersion != ReportSchemaVersion {
		t.Errorf("schemaVersion = %d, want %d", report.SchemaVersion, ReportSchemaVersion)
	}
	if report.PresetVersion != 3 {
		t.Errorf("presetVersion = %d, want the configured 3", report.PresetVersion)
	}
	if report.Findings == nil {
		t.Error("findings = null, want an empty array")
	}
}

func TestFindingsJSONLeavesOutAnUnversionedPreset(t *testing.T) {
	// A configuration that names no preset version has none to report, and a
	// consumer pinning one must not read a zero as a revision.
	var buf bytes.Buffer
	if err := FindingsJSON(&buf, nil, model.Summary{Documents: 1}, Header{}); err != nil {
		t.Fatalf("FindingsJSON: %v", err)
	}
	if strings.Contains(buf.String(), "preset_version") {
		t.Fatalf("report = %s, want no preset_version", buf.String())
	}
}

func TestFindingsGitHubWorkflowCommands(t *testing.T) {
	tests := []struct {
		name    string
		finding model.Finding
		want    string
	}{
		{
			name: "an error with a full position",
			finding: model.Finding{
				Severity: model.SeverityError,
				Rule:     model.RuleInvalidFrontmatter,
				ID:       "0002",
				Detail:   "mapping value is not allowed in this context",
				Location: model.Location{Path: "docs/adr/0002-b.md", Line: 4, Column: 9},
			},
			want: "::error file=docs/adr/0002-b.md,line=4,col=9,title=invalid_frontmatter::0002: mapping value is not allowed in this context",
		},
		{
			name: "a warning without a column",
			finding: model.Finding{
				Severity: model.SeverityWarn,
				Rule:     model.RuleSupersededOrphan,
				ID:       "0001",
				Detail:   "nothing supersedes it",
				Location: model.Location{Path: "docs/adr/0001-a.md", Line: 3},
			},
			want: "::warning file=docs/adr/0001-a.md,line=3,title=superseded_orphan::0001: nothing supersedes it",
		},
		{
			name: "an unknown position drops both properties",
			finding: model.Finding{
				Severity: model.SeverityError,
				Rule:     model.RuleCycle,
				ID:       "0003",
				Detail:   "supersedes cycle",
				Location: model.Location{Path: "docs/adr/0003-c.md"},
			},
			want: "::error file=docs/adr/0003-c.md,title=cycle::0003: supersedes cycle",
		},
		{
			name: "separators inside a value are escaped",
			finding: model.Finding{
				Severity: model.SeverityError,
				Rule:     model.RuleIDCollision,
				ID:       "0004",
				Detail:   "shares its identifier with a,b.md",
				Location: model.Location{Path: "docs/a,b:c.md", Line: 1},
			},
			want: "::error file=docs/a%2Cb%3Ac.md,line=1,title=id_collision::0004: shares its identifier with a,b.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := FindingsGitHub(&buf, []model.Finding{tt.finding}, model.Summary{Errors: 1}, ""); err != nil {
				t.Fatalf("FindingsGitHub: %v", err)
			}
			got := strings.TrimRight(buf.String(), "\n")
			if got != tt.want {
				t.Fatalf("command = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFindingsGitHubEndsWithTheTextSummary(t *testing.T) {
	summary := model.Summary{Documents: 4, Edges: 3, Warnings: 1}
	var annotations, text bytes.Buffer
	findings := []model.Finding{{
		Severity: model.SeverityWarn,
		Rule:     model.RuleSupersededOrphan,
		ID:       "0001",
		Detail:   "nothing supersedes it",
		Location: model.Location{Path: "0001-a.md", Line: 3},
	}}
	if err := FindingsGitHub(&annotations, findings, summary, ""); err != nil {
		t.Fatalf("FindingsGitHub: %v", err)
	}
	if err := FindingsText(&text, findings, summary, ""); err != nil {
		t.Fatalf("FindingsText: %v", err)
	}
	last := func(s string) string {
		parts := strings.Split(strings.TrimRight(s, "\n"), "\n")
		return parts[len(parts)-1]
	}
	if last(annotations.String()) != last(text.String()) {
		t.Fatalf("summary = %q, want the text summary %q", last(annotations.String()), last(text.String()))
	}
}

func TestFindingsRDJSONShape(t *testing.T) {
	findings := []model.Finding{{
		Severity: model.SeverityError,
		Rule:     model.RuleIDCollision,
		ID:       "0004",
		Detail:   "shares its identifier with 0004-b.md",
		Location: model.Location{Path: "0004-a.md", Line: 1, Column: 2},
		Related:  []model.Location{{Path: "0004-b.md", Line: 1}},
	}}

	var buf bytes.Buffer
	if err := FindingsRDJSON(&buf, findings, model.Summary{Errors: 1}); err != nil {
		t.Fatalf("FindingsRDJSON: %v", err)
	}

	var result struct {
		Source struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"source"`
		Severity    string `json:"severity"`
		Diagnostics []struct {
			Message  string `json:"message"`
			Severity string `json:"severity"`
			Location struct {
				Path  string `json:"path"`
				Range struct {
					Start struct {
						Line   int `json:"line"`
						Column int `json:"column"`
					} `json:"start"`
				} `json:"range"`
			} `json:"location"`
			Code struct {
				Value string `json:"value"`
			} `json:"code"`
			RelatedLocations []struct {
				Location struct {
					Path  string `json:"path"`
					Range struct {
						Start struct {
							Line int `json:"line"`
						} `json:"start"`
					} `json:"range"`
				} `json:"location"`
			} `json:"related_locations"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("decode rdjson %s: %v", buf.String(), err)
	}
	if result.Source.Name != "docdag" || result.Source.URL == "" {
		t.Errorf("source = %+v, want the tool named with its url", result.Source)
	}
	if result.Severity != "ERROR" {
		t.Errorf("severity = %q, want ERROR", result.Severity)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v, want one", result.Diagnostics)
	}
	d := result.Diagnostics[0]
	if d.Message != "id_collision 0004: shares its identifier with 0004-b.md" {
		t.Errorf("message = %q, want the rule, the id and the detail", d.Message)
	}
	if d.Severity != "ERROR" {
		t.Errorf("diagnostic severity = %q, want ERROR", d.Severity)
	}
	if d.Code.Value != model.RuleIDCollision {
		t.Errorf("code = %q, want the rule name", d.Code.Value)
	}
	if d.Location.Path != "0004-a.md" || d.Location.Range.Start.Line != 1 || d.Location.Range.Start.Column != 2 {
		t.Errorf("location = %+v, want the finding position", d.Location)
	}
	if len(d.RelatedLocations) != 1 || d.RelatedLocations[0].Location.Path != "0004-b.md" {
		t.Errorf("relatedLocations = %+v, want the other file", d.RelatedLocations)
	}
}

func TestFindingsRDJSONSeverityFollowsTheStrongestFinding(t *testing.T) {
	warn := []model.Finding{{Severity: model.SeverityWarn, Rule: model.RuleSupersededOrphan, ID: "0001"}}

	var buf bytes.Buffer
	if err := FindingsRDJSON(&buf, warn, model.Summary{Warnings: 1}); err != nil {
		t.Fatalf("FindingsRDJSON: %v", err)
	}
	if !strings.Contains(buf.String(), `"severity": "WARNING"`) {
		t.Fatalf("report = %s, want WARNING", buf.String())
	}
}

func TestReportRenderersPropagateWriterErrors(t *testing.T) {
	findings := []model.Finding{{Severity: model.SeverityError, Rule: model.RuleCycle, ID: "0001", Location: model.Location{Path: "0001.md"}}}
	renderers := map[string]func(io.Writer, []model.Finding, model.Summary) error{
		"text":   testPresetFindingsText,
		"json":   testPresetFindingsJSON,
		"github": testPresetFindingsGitHub,
		"rdjson": FindingsRDJSON,
	}
	for name, write := range renderers {
		t.Run(name, func(t *testing.T) {
			if err := write(failingWriter{}, findings, model.Summary{}); err == nil {
				t.Fatal("err = nil, want the write failure surfaced")
			}
		})
	}
}

func TestFindingsAboutAFileWithoutAnIdentifier(t *testing.T) {
	// A file that yields no identifier is filed against no document, so the
	// renderers name the rule and the detail and leave no gap between them.
	findings := []model.Finding{{
		Severity: model.SeverityError,
		Rule:     model.RuleIDMismatch,
		Detail:   `"README" is not an identifier of kind "clause"`,
		Location: model.Location{Path: "spec/clauses/README.md", Line: 1},
	}}
	summary := model.Summary{Documents: 1, Errors: 1}

	tests := []struct {
		name  string
		write func(io.Writer, []model.Finding, model.Summary) error
		want  string
	}{
		{
			name:  "text",
			write: testPresetFindingsText,
			want:  `spec/clauses/README.md:1: ERROR id_mismatch: "README" is not an identifier of kind "clause"`,
		},
		{
			name:  "github",
			write: testPresetFindingsGitHub,
			want:  `title=id_mismatch::"README" is not an identifier of kind "clause"`,
		},
		{
			name:  "rdjson",
			write: FindingsRDJSON,
			want:  `id_mismatch: \"README\" is not an identifier of kind \"clause\"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := tt.write(&out, findings, summary); err != nil {
				t.Fatalf("write: %v", err)
			}

			if !strings.Contains(out.String(), tt.want) {
				t.Fatalf("report = %q, want it to contain %q", out.String(), tt.want)
			}
		})
	}
}

func TestFieldUsageText(t *testing.T) {
	usage := []graph.FieldUsage{
		{Field: "status", Documents: 6, LastChange: "2026-03-04"},
		{Field: "owner", Documents: 2, Deprecated: true, LastChange: "2026-01-02"},
		{Field: "team", Documents: 0, Deprecated: true},
	}

	var buf bytes.Buffer
	if err := FieldUsageText(&buf, usage); err != nil {
		t.Fatalf("FieldUsageText: %v", err)
	}

	want := []string{
		"field   documents  last change  deprecated",
		"status  6          2026-03-04   -",
		"owner   2          2026-01-02   yes",
		"team    0          -            yes",
	}
	got := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(got) != len(want) {
		t.Fatalf("report =\n%s\nwant %d lines", buf.String(), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFieldUsageRenderersPropagateWriterErrors(t *testing.T) {
	usage := []graph.FieldUsage{{Field: "owner", Documents: 2, Deprecated: true}}
	renderers := map[string]func(io.Writer, []graph.FieldUsage) error{
		"text": FieldUsageText,
		"json": FieldUsageJSON,
	}
	for name, write := range renderers {
		t.Run(name, func(t *testing.T) {
			if err := write(failingWriter{}, usage); err == nil {
				t.Fatal("err = nil, want the write failure surfaced")
			}
		})
	}
}

func TestFieldUsageJSONWritesAnEmptyArrayRatherThanNull(t *testing.T) {
	var buf bytes.Buffer
	if err := FieldUsageJSON(&buf, nil); err != nil {
		t.Fatalf("FieldUsageJSON: %v", err)
	}
	if !strings.Contains(buf.String(), `"fields": []`) {
		t.Fatalf("report = %s, want an empty array", buf.String())
	}
}
