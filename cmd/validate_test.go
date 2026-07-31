package cmd

import (
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/render"
)

func TestValidateTextReport(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		wantExit int
		findings []string
		summary  string
	}{
		{
			name:     "valid corpus reports no findings",
			fixture:  "ok-basic",
			wantExit: 0,
			summary:  "OK: 6 docs, 4 typed edges, no cycles",
		},
		{
			name:     "MADR corpus warns about the unstructured supersedes",
			fixture:  "ok-madr",
			wantExit: 0,
			findings: []string{"WARN unstructured_supersedes 0002:"},
			summary:  "OK: 4 docs, 3 typed edges, no cycles",
		},
		{
			name:     "superseded orphan warns without failing",
			fixture:  "superseded-orphan",
			wantExit: 0,
			findings: []string{"WARN superseded_orphan 0001:"},
			summary:  "OK: 2 docs, 0 typed edges, no cycles",
		},
		{
			name:     "fan-in corpus is valid",
			fixture:  "fan-in",
			wantExit: 0,
			summary:  "OK: 3 docs, 2 typed edges, no cycles",
		},
		{
			name:     "depends-on corpus is valid",
			fixture:  "depends-impact",
			wantExit: 0,
			summary:  "OK: 4 docs, 3 typed edges, no cycles",
		},
		{
			name:     "supersedes cycle fails",
			fixture:  "cycle",
			wantExit: 1,
			findings: []string{"ERROR cycle 0001:"},
		},
		{
			name:     "dangling reference fails",
			fixture:  "dangling",
			wantExit: 1,
			findings: []string{"ERROR dangling_ref 0002:"},
		},
		{
			name:     "duplicate identifier fails",
			fixture:  "id-collision",
			wantExit: 1,
			findings: []string{"ERROR id_collision 0004:"},
		},
		{
			name:     "undecodable frontmatter fails",
			fixture:  "invalid-yaml",
			wantExit: 1,
			findings: []string{"ERROR invalid_frontmatter 0002:"},
		},
		{
			name:     "status drift fails",
			fixture:  "status-drift",
			wantExit: 1,
			findings: []string{"ERROR status_drift 0001:"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, "validate", "--dir", fixture(t, tt.fixture))
			assertExit(t, got, tt.wantExit)
			assertPrefixes(t, "findings", findingLines(got.stdout), tt.findings)
			if got.stderr != "" {
				t.Errorf("stderr = %q, want empty: validate reports through stdout", got.stderr)
			}
			all := lines(got.stdout)
			if tt.summary == "" {
				if strings.Contains(got.stdout, "OK:") {
					t.Errorf("stdout = %q, want no success summary for a failing corpus", got.stdout)
				}
				return
			}
			if len(all) == 0 || all[len(all)-1] != tt.summary {
				t.Errorf("summary line = %q, want %q", got.stdout, tt.summary)
			}
		})
	}
}

func TestValidateDanglingReferenceNamesTheMissingDocument(t *testing.T) {
	got := run(t, "validate", "--dir", fixture(t, "dangling"))
	assertExit(t, got, 1)
	detail := strings.Join(findingLines(got.stdout), "\n")
	for _, want := range []string{"0009", "supersedes"} {
		if !strings.Contains(detail, want) {
			t.Errorf("finding %q does not mention %q", detail, want)
		}
	}
}

func TestValidateReportsAStatusStringNamingAnUnknownDocument(t *testing.T) {
	// The most likely MADR authoring slip: a successor that does not exist yet.
	dir := writeDocs(t, map[string]string{
		"0001-a-decision.md": "---\ntitle: A decision\nstatus: superseded by 0099\ndate: 2025-01-01\n---\n\n# A decision\n",
		"0002-another.md":    "---\ntitle: Another decision\nstatus: accepted\ndate: 2025-02-01\n---\n\n# Another decision\n",
	})

	got := run(t, "validate", "--dir", dir)

	assertExit(t, got, 1)
	detail := strings.Join(findingLines(got.stdout), "\n")
	if !strings.Contains(detail, "ERROR dangling_ref 0001") {
		t.Errorf("findings = %q, want a dangling reference on 0001", detail)
	}
	if !strings.Contains(detail, "0099") {
		t.Errorf("findings = %q, want the missing document named", detail)
	}
}

func TestValidateRejectsAStatusThatOnlyOpensWithAVocabularyWord(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"0001-a-decision.md": "---\ntitle: A decision\nstatus: accepted by the architecture board\ndate: 2025-01-01\n---\n\n# A decision\n",
	})

	got := run(t, "validate", "--dir", dir)

	assertExit(t, got, 1)
	assertPrefixes(t, "findings", findingLines(got.stdout), []string{"ERROR unknown_status 0001:"})
}

func TestValidateIgnoresFilesThatAreNotManagedDocuments(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"0001-a-decision.md": "---\ntitle: A decision\nstatus: accepted\ndate: 2025-01-01\n---\n\n# A decision\n",
		"template-v2.md":     "---\ntitle: Template\nstatus: proposed\n---\n\n# Template\n",
		"notes-2024.md":      "---\ntitle: Notes\nstatus: accepted\n---\n\n# Notes\n",
	})

	got := run(t, "validate", "--dir", dir)

	assertExit(t, got, 0)
	want := "OK: 1 docs, 0 typed edges, no cycles"
	if ls := lines(got.stdout); len(ls) == 0 || ls[len(ls)-1] != want {
		t.Errorf("summary line = %q, want %q", got.stdout, want)
	}
}

func TestValidateCycleReportsThePath(t *testing.T) {
	got := run(t, "validate", "--dir", fixture(t, "cycle"))
	assertExit(t, got, 1)
	detail := strings.Join(findingLines(got.stdout), "\n")
	for _, want := range []string{"0001", "0002", "0003"} {
		if !strings.Contains(detail, want) {
			t.Errorf("cycle finding %q does not mention %q", detail, want)
		}
	}
}

func TestValidateJSONReport(t *testing.T) {
	type want struct {
		exit     int
		findings []model.Finding
		summary  model.Summary
	}
	tests := []struct {
		name    string
		fixture string
		want    want
	}{
		{
			name:    "valid corpus",
			fixture: "ok-basic",
			want: want{
				exit:    0,
				summary: model.Summary{Documents: 6, Edges: 4},
			},
		},
		{
			name:    "warning only corpus",
			fixture: "superseded-orphan",
			want: want{
				exit: 0,
				findings: []model.Finding{
					{Severity: model.SeverityWarn, Rule: model.RuleSupersededOrphan, ID: "0001"},
				},
				summary: model.Summary{Documents: 2, Edges: 0, Warnings: 1},
			},
		},
		{
			name:    "derived edge corpus",
			fixture: "ok-madr",
			want: want{
				exit: 0,
				findings: []model.Finding{
					{Severity: model.SeverityWarn, Rule: model.RuleUnstructuredSupersedes, ID: "0002"},
				},
				summary: model.Summary{Documents: 4, Edges: 3, Warnings: 1},
			},
		},
		{
			name:    "cyclic corpus",
			fixture: "cycle",
			want: want{
				exit: 1,
				findings: []model.Finding{
					{Severity: model.SeverityError, Rule: model.RuleCycle, ID: "0001"},
				},
				summary: model.Summary{Documents: 3, Edges: 3, Errors: 1, Cycles: 1},
			},
		},
		{
			name:    "drifted status corpus",
			fixture: "status-drift",
			want: want{
				exit: 1,
				findings: []model.Finding{
					{Severity: model.SeverityError, Rule: model.RuleStatusDrift, ID: "0001"},
				},
				summary: model.Summary{Documents: 2, Edges: 1, Errors: 1},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, "validate", "--format", "json", "--dir", fixture(t, tt.fixture))
			assertExit(t, got, tt.want.exit)
			report := decodeJSON[render.Report](t, got.stdout)
			if len(report.Findings) != len(tt.want.findings) {
				t.Fatalf("findings = %+v, want %d entries", report.Findings, len(tt.want.findings))
			}
			for i, wantFinding := range tt.want.findings {
				gotFinding := report.Findings[i]
				if gotFinding.Severity != wantFinding.Severity || gotFinding.Rule != wantFinding.Rule || gotFinding.ID != wantFinding.ID {
					t.Errorf("finding %d = %+v, want severity %q rule %q id %q",
						i, gotFinding, wantFinding.Severity, wantFinding.Rule, wantFinding.ID)
				}
				if gotFinding.Detail == "" {
					t.Errorf("finding %d has an empty detail", i)
				}
			}
			if report.Summary != tt.want.summary {
				t.Errorf("summary = %+v, want %+v", report.Summary, tt.want.summary)
			}
		})
	}
}

func TestValidateOrdersFindingsBySeverityRuleThenID(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"0001-alpha.md":   "---\ntitle: Alpha\nstatus: superseded\ndate: 2025-01-01\n---\n\n# Alpha\n",
		"0002-beta.md":    "---\ntitle: Beta\nstatus: accepted\nsupersedes:\n  - 0009\ndate: 2025-01-02\n---\n\n# Beta\n",
		"0003-gamma.md":   "---\ntitle: Gamma\nstatus: accepted\ndate: 2025-01-03\n---\n\n# Gamma\n",
		"0004-delta.md":   "---\ntitle: Delta\nstatus: accepted\nsupersedes:\n  - 0003\ndate: 2025-01-04\n---\n\n# Delta\n",
		"0005-epsilon.md": "---\ntitle: Epsilon\nstatus: accepted\nsupersedes:\n  - 0008\ndate: 2025-01-05\n---\n\n# Epsilon\n",
	})

	got := run(t, "validate", "--dir", dir)
	assertExit(t, got, 1)
	assertPrefixes(t, "findings", findingLines(got.stdout), []string{
		"ERROR dangling_ref 0002:",
		"ERROR dangling_ref 0005:",
		"ERROR status_drift 0003:",
		"WARN superseded_orphan 0001:",
	})

	again := run(t, "validate", "--dir", dir)
	if again.stdout != got.stdout {
		t.Errorf("validate output is not deterministic:\nfirst:\n%s\nsecond:\n%s", got.stdout, again.stdout)
	}
}

func TestValidateUnknownStatus(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"0001-known-status.md":   "---\ntitle: Known status\nstatus: accepted\ndate: 2025-01-01\n---\n\n# Known status\n",
		"0002-unknown-status.md": "---\ntitle: Unknown status\nstatus: retired\ndate: 2025-01-02\n---\n\n# Unknown status\n",
		"0003-absent-status.md":  "---\ntitle: Absent status\ndate: 2025-01-03\n---\n\n# Absent status\n",
	})

	got := run(t, "validate", "--dir", dir)
	assertExit(t, got, 1)
	assertPrefixes(t, "findings", findingLines(got.stdout), []string{"ERROR unknown_status 0002:"})
}

func TestValidateMissingFrontmatter(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"0001-with-frontmatter.md": "---\ntitle: With frontmatter\nstatus: accepted\ndate: 2025-01-01\n---\n\n# With frontmatter\n",
		"0002-bare.md":             "# Bare\n\nThis file matches the document filename pattern but carries no frontmatter.\n",
		"notes.md":                 "# Notes\n\nThis file is not a managed document and is skipped entirely.\n",
	})

	got := run(t, "validate", "--dir", dir)
	assertExit(t, got, 0)
	assertPrefixes(t, "findings", findingLines(got.stdout), []string{"WARN missing_frontmatter 0002:"})
	want := "OK: 2 docs, 0 typed edges, no cycles"
	if ls := lines(got.stdout); len(ls) == 0 || ls[len(ls)-1] != want {
		t.Errorf("summary line = %q, want %q: unmanaged files are skipped", got.stdout, want)
	}
}
