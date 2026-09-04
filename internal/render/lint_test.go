package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/lint"
	"github.com/Kaikei-e/DocDag/model"
)

// testLintFindings is one finding of each severity, which is what separates a
// lint report from a validation one: info never fails a build.
func testLintFindings() []model.Finding {
	return []model.Finding{
		{
			Severity: model.SeverityError,
			Rule:     lint.FindingUnfirableRule,
			ID:       "impossible",
			Detail:   "every alternative contradicts itself",
			Location: model.Location{Path: "docdag.yaml", Line: 12},
			Fix:      "drop the rule",
		},
		{
			Severity: model.SeverityWarn,
			Rule:     lint.FindingNeverFired,
			ID:       "deviation_pressure",
			Detail:   "fired on 0 of 128 clause documents",
			Location: model.Location{Path: "docdag.yaml", Line: 41},
		},
		{
			Severity: model.SeverityInfo,
			Rule:     lint.FindingUnusedEdgeInCorpus,
			ID:       "measures",
			Detail:   "is declared and the corpus holds no edge of it",
			Location: model.Location{Path: "<preset:spec>"},
		},
	}
}

func TestLintText(t *testing.T) {
	findings := testLintFindings()
	var out bytes.Buffer

	if err := LintText(&out, findings, lint.Summarize(findings), ""); err != nil {
		t.Fatalf("LintText: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"docdag.yaml:12: ERROR unfirable_rule impossible: every alternative contradicts itself",
		"  fix: drop the rule",
		"docdag.yaml:41: WARN never_fired deviation_pressure: fired on 0 of 128 clause documents",
		"<preset:spec>: INFO unused_edge_in_corpus measures:",
		"1 error, 1 warning, 1 info",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report:\n%s\nwant it to hold %q", got, want)
		}
	}
}

func TestLintTextOnACleanConfiguration(t *testing.T) {
	var out bytes.Buffer

	if err := LintText(&out, nil, lint.Summary{}, ""); err != nil {
		t.Fatalf("LintText: %v", err)
	}

	if got := out.String(); got != "OK: no lint findings\n" {
		t.Errorf("report = %q, want the line a clean configuration ends with", got)
	}
}

func TestLintJSON(t *testing.T) {
	findings := testLintFindings()
	var out bytes.Buffer

	if err := LintJSON(&out, findings, lint.Summarize(findings), Header{PresetVersion: 2}); err != nil {
		t.Fatalf("LintJSON: %v", err)
	}

	var report LintReport
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("decode: %v: %s", err, out.String())
	}
	if report.Kind != LintKind || report.SchemaVersion != LintSchemaVersion {
		t.Errorf("header = %q %d, want the lint kind at version %d", report.Kind, report.SchemaVersion, LintSchemaVersion)
	}
	if report.PresetVersion != 2 {
		t.Errorf("preset_version = %d, want the revision the caller named", report.PresetVersion)
	}
	if got := report.Summary; got != (lint.Summary{Errors: 1, Warnings: 1, Infos: 1}) {
		t.Errorf("summary = %+v, want one of each severity", got)
	}
	if len(report.Findings) != len(findings) {
		t.Errorf("findings = %d, want %d", len(report.Findings), len(findings))
	}
}

func TestLintJSONWritesAnEmptyList(t *testing.T) {
	var out bytes.Buffer

	if err := LintJSON(&out, nil, lint.Summary{}, Header{PresetVersion: 0}); err != nil {
		t.Fatalf("LintJSON: %v", err)
	}

	if !strings.Contains(out.String(), `"findings": []`) {
		t.Errorf("report = %s, want an empty list rather than null", out.String())
	}
	if strings.Contains(out.String(), "preset_version") {
		t.Errorf("report = %s, want no preset revision where the configuration names none", out.String())
	}
}

// TestLintGitHubLevels pins the one thing the annotation format has to get
// right about a lint report: an info finding is a notice, which fails nothing.
func TestLintGitHubLevels(t *testing.T) {
	findings := testLintFindings()
	var out bytes.Buffer

	if err := LintGitHub(&out, findings, lint.Summarize(findings), ""); err != nil {
		t.Fatalf("LintGitHub: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"::error file=docdag.yaml,line=12,title=unfirable_rule::impossible: ",
		"::warning file=docdag.yaml,line=41,title=never_fired::",
		"::notice file=<preset%3Aspec>,title=unused_edge_in_corpus::",
		"1 error, 1 warning, 1 info",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report:\n%s\nwant it to hold %q", got, want)
		}
	}
}

func TestLintRDJSONSeverity(t *testing.T) {
	tests := []struct {
		name     string
		findings []model.Finding
		want     string
	}{
		{name: "an error report", findings: testLintFindings(), want: "ERROR"},
		{name: "a report of warnings", findings: testLintFindings()[1:], want: "WARNING"},
		{name: "a report of facts", findings: testLintFindings()[2:], want: "INFO"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			if err := LintRDJSON(&out, tt.findings, lint.Summarize(tt.findings)); err != nil {
				t.Fatalf("LintRDJSON: %v", err)
			}

			var result struct {
				Severity    string `json:"severity"`
				Diagnostics []struct {
					Severity string `json:"severity"`
				} `json:"diagnostics"`
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatalf("decode: %v: %s", err, out.String())
			}
			if result.Severity != tt.want {
				t.Errorf("severity = %q, want %q", result.Severity, tt.want)
			}
			if len(result.Diagnostics) != len(tt.findings) {
				t.Errorf("diagnostics = %d, want %d", len(result.Diagnostics), len(tt.findings))
			}
		})
	}
}

func TestFixturePaths(t *testing.T) {
	paths := []string{"lint/drift/ruleid/0001-a.md", "lint/drift/ok/0001-a.md"}

	t.Run("one path per line", func(t *testing.T) {
		var out bytes.Buffer

		if err := FixturePaths(&out, paths, false); err != nil {
			t.Fatalf("FixturePaths: %v", err)
		}

		if got := out.String(); got != strings.Join(paths, "\n")+"\n" {
			t.Errorf("output = %q, want one path per line", got)
		}
	})

	t.Run("as JSON", func(t *testing.T) {
		var out bytes.Buffer

		if err := FixturePaths(&out, nil, true); err != nil {
			t.Fatalf("FixturePaths: %v", err)
		}

		if !strings.Contains(out.String(), `"paths": []`) {
			t.Errorf("output = %s, want an empty list rather than null", out.String())
		}
	})
}
