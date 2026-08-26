package cmd

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
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
			findings: []string{"0002-store-thumbnails-on-the-local-disk.md:3: WARN unstructured_supersedes 0002:"},
			summary:  "OK: 4 docs, 3 typed edges, no cycles",
		},
		{
			name:     "superseded orphan warns without failing",
			fixture:  "superseded-orphan",
			wantExit: 0,
			findings: []string{"0001-run-migrations-during-deploy.md:3: WARN superseded_orphan 0001:"},
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
			findings: []string{"0001-encrypt-backups-with-a-shared-key.md:4: ERROR cycle 0001:"},
		},
		{
			name:     "dangling reference fails",
			fixture:  "dangling",
			wantExit: 1,
			findings: []string{"0002-ship-logs-to-a-central-collector.md:4: ERROR dangling_ref 0002:"},
		},
		{
			name:     "duplicate identifier fails",
			fixture:  "id-collision",
			wantExit: 1,
			findings: []string{"0004-a.md:1: ERROR id_collision 0004:"},
		},
		{
			name:     "undecodable frontmatter fails",
			fixture:  "invalid-yaml",
			wantExit: 1,
			findings: []string{"0002-negotiate-api-versions-by-header.md:2: ERROR invalid_frontmatter 0002:"},
		},
		{
			name:     "an edge key that names nothing fails",
			fixture:  "empty-edge",
			wantExit: 1,
			findings: []string{
				"0002-stream-audit-events-to-append-only-storage.md:4: ERROR empty_edge 0002:",
				"0002-stream-audit-events-to-append-only-storage.md:5: ERROR empty_edge 0002:",
				"0001-write-audit-events-to-the-database.md:3: WARN superseded_orphan 0001:",
			},
		},
		{
			name:     "a withdrawn document is in the vocabulary and binds nothing",
			fixture:  "withdrawn",
			wantExit: 0,
			summary:  "OK: 2 docs, 0 typed edges, no cycles",
		},
		{
			name:     "status drift fails",
			fixture:  "status-drift",
			wantExit: 1,
			findings: []string{"0001-serve-images-from-the-application-server.md:3: ERROR status_drift 0001:"},
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
	if !strings.Contains(detail, "0001-a-decision.md:3: ERROR dangling_ref 0001") {
		t.Errorf("findings = %q, want a dangling reference on 0001", detail)
	}
	if !strings.Contains(detail, "0099") {
		t.Errorf("findings = %q, want the missing document named", detail)
	}
}

func TestValidateSuggestsAFixUnderTheFinding(t *testing.T) {
	got := run(t, "validate", "--dir", fixture(t, "dangling"))

	assertExit(t, got, 1)
	all := lines(got.stdout)
	if len(all) != 2 {
		t.Fatalf("stdout = %q, want the finding and its fix", got.stdout)
	}
	if all[1] != "  fix: did you mean 0001?" {
		t.Errorf("fix line = %q, want the nearest document suggested", all[1])
	}
}

func TestValidateJSONCarriesTheFix(t *testing.T) {
	got := run(t, "validate", "--format", "json", "--dir", fixture(t, "status-drift"))

	assertExit(t, got, 1)
	report := decodeJSON[render.Report](t, got.stdout)
	if len(report.Findings) != 1 {
		t.Fatalf("findings = %+v, want one", report.Findings)
	}
	if !strings.HasPrefix(report.Findings[0].Fix, "set status: superseded in ") {
		t.Errorf("fix = %q, want the status change spelled out", report.Findings[0].Fix)
	}
}

func TestValidateRejectsAFrontmatterReferenceThatIsNotIdentityShaped(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"0001-a-decision.md": "---\ntitle: A decision\nstatus: superseded\ndate: 2025-01-01\n---\n\n# A decision\n",
		"0002-another.md":    "---\ntitle: Another decision\nstatus: accepted\nsupersedes:\n  - see 0001\ndate: 2025-02-01\n---\n\n# Another decision\n",
	})

	got := run(t, "validate", "--dir", dir)

	assertExit(t, got, 1)
	assertPrefixes(t, "findings", findingLines(got.stdout), []string{
		"0002-another.md:4: ERROR invalid_ref 0002:",
		"0001-a-decision.md:3: WARN superseded_orphan 0001:",
	})
}

func TestValidateLeavesProseThatIsNotIdentityShapedAlone(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"0001-a-decision.md": "---\ntitle: A decision\nstatus: accepted\ndate: 2025-01-01\n---\n\n" +
			"# A decision\n\nSee [[3days-recap]], [[upstream]] and [[tool.uv.index]].\n\n```\n[[0099]]\n```\n",
	})

	got := run(t, "validate", "--dir", dir)

	assertExit(t, got, 0)
	assertPrefixes(t, "findings", findingLines(got.stdout), nil)
}

const testAcceptedDocument = "---\ntitle: Serve images from the application\nstatus: accepted\ndate: 2025-01-01\n---\n\n" +
	"# Serve images from the application\n\n## Decision Outcome\n\nThe application resizes and serves images itself.\n"

// gitRepo commits an ADR corpus so a history check has a revision to compare
// against. Identity comes from flags, so a runner without a git identity works.
func gitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	dir := writeDocs(t, files)
	git(t, dir, "init", "--quiet")
	git(t, dir, "add", "-A")
	git(t, dir,
		"-c", "user.name=DocDag Test",
		"-c", "user.email=test@example.test",
		"-c", "commit.gpgsign=false",
		"commit", "--quiet", "-m", "the first revision")
	return dir
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "nonexistent-gitconfig"), "GIT_CONFIG_NOSYSTEM=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
}

func TestValidateImmutableSince(t *testing.T) {
	corpus := map[string]string{"docs/adr/0001-serve-images-from-the-application.md": testAcceptedDocument}

	t.Run("an untouched history passes", func(t *testing.T) {
		t.Chdir(gitRepo(t, corpus))

		got := run(t, "validate", "--immutable-since", "HEAD")

		assertExit(t, got, 0)
		assertPrefixes(t, "findings", findingLines(got.stdout), nil)
	})

	t.Run("a rewritten accepted document fails", func(t *testing.T) {
		dir := gitRepo(t, corpus)
		t.Chdir(dir)
		rewritten := strings.Replace(testAcceptedDocument, "The application resizes", "A CDN resizes", 1)
		if err := os.WriteFile(filepath.Join(dir, "docs", "adr", "0001-serve-images-from-the-application.md"), []byte(rewritten), 0o600); err != nil {
			t.Fatalf("rewrite: %v", err)
		}

		got := run(t, "validate", "--immutable-since", "HEAD")

		assertExit(t, got, 1)
		assertPrefixes(t, "findings", findingLines(got.stdout), []string{
			"0001-serve-images-from-the-application.md:11: ERROR immutable_violation 0001:",
		})
	})

	t.Run("the check is off unless it is asked for", func(t *testing.T) {
		dir := gitRepo(t, corpus)
		t.Chdir(dir)
		rewritten := strings.Replace(testAcceptedDocument, "The application resizes", "A CDN resizes", 1)
		if err := os.WriteFile(filepath.Join(dir, "docs", "adr", "0001-serve-images-from-the-application.md"), []byte(rewritten), 0o600); err != nil {
			t.Fatalf("rewrite: %v", err)
		}

		got := run(t, "validate")

		assertExit(t, got, 0)
		assertPrefixes(t, "findings", findingLines(got.stdout), nil)
	})

	t.Run("a corpus outside a repository is a configuration error", func(t *testing.T) {
		t.Chdir(writeDocs(t, corpus))

		got := run(t, "validate", "--immutable-since", "HEAD")

		assertExit(t, got, 3)
		if got.stderr == "" {
			t.Error("stderr is empty, want a diagnostic naming the missing repository")
		}
	})
}

func TestValidateRuleAlternativesAndNegation(t *testing.T) {
	dir := fixture(t, "any-of")

	got := run(t, "validate", "--dir", dir, "--config", filepath.Join(dir, "docdag.yaml"))

	assertExit(t, got, 0)
	assertPrefixes(t, "findings", findingLines(got.stdout), []string{
		"0001-run-nightly-reports-on-the-primary.md:3: WARN unexplained_retirement 0001:",
	})
}

func TestValidateRulesOverListAttributes(t *testing.T) {
	dir := fixture(t, "list-attrs")

	got := run(t, "validate", "--dir", dir, "--config", filepath.Join(dir, "docdag.yaml"))

	assertExit(t, got, 1)
	assertPrefixes(t, "findings", findingLines(got.stdout), []string{
		"0001-store-secrets-in-the-orchestrator.md:7: ERROR security_review_missing 0001:",
		"0002-serve-static-assets-from-the-app.md:4: WARN retired_tags_only 0002:",
	})
}

func TestValidateStructuralSeverityEscalation(t *testing.T) {
	files := map[string]string{
		"docs/adr/0001-with-frontmatter.md": "---\ntitle: With frontmatter\nstatus: accepted\ndate: 2025-01-01\n---\n\n# With frontmatter\n",
		"docs/adr/0002-bare.md":             "# Bare\n\nThis file matches the document filename pattern but carries no frontmatter.\n",
	}

	t.Run("a missing frontmatter block is a warning by default", func(t *testing.T) {
		t.Chdir(writeDocs(t, files))

		got := run(t, "validate")

		assertExit(t, got, 0)
		assertPrefixes(t, "findings", findingLines(got.stdout), []string{"0002-bare.md:1: WARN missing_frontmatter 0002:"})
	})

	t.Run("an escalated check fails the build", func(t *testing.T) {
		escalated := maps.Clone(files)
		escalated["docdag.yaml"] = "structural:\n  missing_frontmatter: error\n"
		t.Chdir(writeDocs(t, escalated))

		got := run(t, "validate")

		assertExit(t, got, 1)
		assertPrefixes(t, "findings", findingLines(got.stdout), []string{"0002-bare.md:1: ERROR missing_frontmatter 0002:"})
	})

	t.Run("lowering a check is a configuration error", func(t *testing.T) {
		lowered := maps.Clone(files)
		lowered["docdag.yaml"] = "structural:\n  cycle: warn\n"
		t.Chdir(writeDocs(t, lowered))

		got := run(t, "validate")

		assertExit(t, got, 3)
		if !strings.Contains(got.stderr, "cycle") {
			t.Errorf("stderr = %q, want it to name the check", got.stderr)
		}
	})
}

func TestValidateUnionAcyclicity(t *testing.T) {
	dir := fixture(t, "union-cycle")

	t.Run("each edge type on its own is acyclic", func(t *testing.T) {
		got := run(t, "validate", "--dir", dir)

		assertExit(t, got, 0)
		assertPrefixes(t, "findings", findingLines(got.stdout), nil)
	})

	t.Run("the union of the acyclic edge types is not", func(t *testing.T) {
		got := run(t, "validate", "--dir", dir, "--config", filepath.Join(dir, "docdag.yaml"))

		assertExit(t, got, 1)
		assertPrefixes(t, "findings", findingLines(got.stdout), []string{
			"0001-keep-sessions-in-the-database.md:4: ERROR cycle 0001:",
		})
		detail := strings.Join(findingLines(got.stdout), "\n")
		for _, want := range []string{"supersedes", "depends-on"} {
			if !strings.Contains(detail, want) {
				t.Errorf("finding %q does not name the edge type %q", detail, want)
			}
		}
	})
}

func TestValidateUnionCycleBesideASingleTypeCycle(t *testing.T) {
	dir := fixture(t, "union-cycle-shadowed")

	got := run(t, "validate", "--dir", dir, "--config", filepath.Join(dir, "docdag.yaml"))

	assertExit(t, got, 1)
	assertPrefixes(t, "findings", findingLines(got.stdout), []string{
		"0001-rate-limit-in-the-gateway.md:4: ERROR cycle 0001:",
		"0001-rate-limit-in-the-gateway.md:4: ERROR cycle 0001:",
	})
	detail := strings.Join(findingLines(got.stdout), "\n")
	for _, want := range []string{"supersedes cycle: 0001 -> 0002 -> 0001", "cycle over supersedes, depends-on: 0001 -> 0002 -> 0003 -> 0001"} {
		if !strings.Contains(detail, want) {
			t.Errorf("findings %q do not carry %q", detail, want)
		}
	}
}

func TestValidateEdgeCardinality(t *testing.T) {
	dir := fixture(t, "cardinality")

	t.Run("unbounded by default", func(t *testing.T) {
		got := run(t, "validate", "--dir", dir)

		assertExit(t, got, 0)
		assertPrefixes(t, "findings", findingLines(got.stdout), nil)
	})

	t.Run("a bound the corpus breaks is an error on the document that carries it", func(t *testing.T) {
		got := run(t, "validate", "--dir", dir, "--config", filepath.Join(dir, "docdag.yaml"))

		assertExit(t, got, 1)
		assertPrefixes(t, "findings", findingLines(got.stdout), []string{
			"0001-rate-limit-per-api-key.md:3: ERROR cardinality 0001:",
		})
	})
}

func TestValidateInverseKeysMustAgreeWithTheEdges(t *testing.T) {
	dir := fixture(t, "inverse-mismatch")

	got := run(t, "validate", "--dir", dir, "--config", filepath.Join(dir, "docdag.yaml"))

	assertExit(t, got, 1)
	assertPrefixes(t, "findings", findingLines(got.stdout), []string{
		"0001-authorize-with-a-shared-service-token.md:3: ERROR inverse_mismatch 0001:",
		"0003-pin-the-ca-bundle-in-every-image.md:4: ERROR inverse_mismatch 0003:",
	})
}

func TestValidateReferenceLayerIsOptIn(t *testing.T) {
	dir := fixture(t, "dangling-reference")

	t.Run("unconfigured, the reference layer never fails a build", func(t *testing.T) {
		got := run(t, "validate", "--dir", dir)

		assertExit(t, got, 0)
		assertPrefixes(t, "findings", findingLines(got.stdout), nil)
	})

	t.Run("configured, a link that names no document is a finding", func(t *testing.T) {
		got := run(t, "validate", "--dir", dir, "--config", filepath.Join(dir, "docdag.yaml"))

		assertExit(t, got, 1)
		assertPrefixes(t, "findings", findingLines(got.stdout), []string{
			"0002-retry-failed-jobs-with-backoff.md:31: ERROR dangling_reference 0002:",
		})
	})
}

func TestValidateRejectsAStatusThatOnlyOpensWithAVocabularyWord(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"0001-a-decision.md": "---\ntitle: A decision\nstatus: accepted by the architecture board\ndate: 2025-01-01\n---\n\n# A decision\n",
	})

	got := run(t, "validate", "--dir", dir)

	assertExit(t, got, 1)
	assertPrefixes(t, "findings", findingLines(got.stdout), []string{"0001-a-decision.md:3: ERROR unknown_status 0001:"})
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

func TestValidateOrdersFindingsBySeverityThenLocation(t *testing.T) {
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
		"0002-beta.md:4: ERROR dangling_ref 0002:",
		"0003-gamma.md:3: ERROR status_drift 0003:",
		"0005-epsilon.md:4: ERROR dangling_ref 0005:",
		"0001-alpha.md:3: WARN superseded_orphan 0001:",
	})

	again := run(t, "validate", "--dir", dir)
	if again.stdout != got.stdout {
		t.Errorf("validate output is not deterministic:\nfirst:\n%s\nsecond:\n%s", got.stdout, again.stdout)
	}
}

// testMixedCorpus holds one problem per document plus one document with none,
// so a filtered report can be told from an unfiltered one.
func testMixedCorpus(t *testing.T) string {
	t.Helper()
	return writeDocs(t, map[string]string{
		"0001-alpha.md": "---\ntitle: Alpha\nstatus: superseded\ndate: 2025-01-01\n---\n\n# Alpha\n",
		"0002-beta.md":  "---\ntitle: Beta\nstatus: accepted\nsupersedes:\n  - 0009\ndate: 2025-01-02\n---\n\n# Beta\n",
		"0003-gamma.md": "---\ntitle: Gamma\nstatus: accepted\ndate: 2025-01-03\n---\n\n# Gamma\n",
		"0004-delta.md": "---\ntitle: Delta\nstatus: accepted\nsupersedes:\n  - 0003\ndate: 2025-01-04\n---\n\n# Delta\n",
		"0007-zeta.md":  "---\ntitle: Zeta\nstatus: accepted\ndate: 2025-01-07\n---\n\n# Zeta\n",
	})
}

func TestValidateTouchingReportsOnlyTheFindingsAboutThePath(t *testing.T) {
	dir := testMixedCorpus(t)

	got := run(t, "validate", "--touching", filepath.Join(dir, "0002-beta.md"), "--dir", dir)

	assertExit(t, got, 1)
	assertPrefixes(t, "findings", findingLines(got.stdout), []string{"0002-beta.md:4: ERROR dangling_ref 0002:"})
	if !strings.Contains(got.stderr, "(2 findings hidden)") {
		t.Errorf("stderr = %q, want the hidden findings counted", got.stderr)
	}
}

func TestValidateTouchingKeepsTheNeighboursOfTheChangedDocument(t *testing.T) {
	dir := testMixedCorpus(t)

	got := run(t, "validate", "--touching", filepath.Join(dir, "0004-delta.md"), "--dir", dir)

	assertExit(t, got, 1)
	assertPrefixes(t, "findings", findingLines(got.stdout), []string{"0003-gamma.md:3: ERROR status_drift 0003:"})
}

func TestValidateTouchingExitsOnTheWholeCorpus(t *testing.T) {
	dir := testMixedCorpus(t)

	got := run(t, "validate", "--touching", filepath.Join(dir, "0007-zeta.md"), "--dir", dir)

	assertExit(t, got, 1)
	assertPrefixes(t, "findings", findingLines(got.stdout), nil)
	if !strings.Contains(got.stderr, "(3 findings hidden)") {
		t.Errorf("stderr = %q, want every finding counted as hidden", got.stderr)
	}
}

func TestValidateTouchingTakesADirectoryAndSeveralPaths(t *testing.T) {
	dir := testMixedCorpus(t)

	whole := run(t, "validate", "--touching", dir, "--dir", dir)
	assertExit(t, whole, 1)
	if whole.stderr != "" {
		t.Errorf("stderr = %q, want nothing hidden when the whole directory is named", whole.stderr)
	}

	two := run(t, "validate",
		"--touching", filepath.Join(dir, "0001-alpha.md"),
		"--touching", filepath.Join(dir, "0002-beta.md"),
		"--dir", dir)

	assertExit(t, two, 1)
	assertPrefixes(t, "findings", findingLines(two.stdout), []string{
		"0002-beta.md:4: ERROR dangling_ref 0002:",
		"0001-alpha.md:3: WARN superseded_orphan 0001:",
	})
}

func TestValidateUnknownStatus(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"0001-known-status.md":   "---\ntitle: Known status\nstatus: accepted\ndate: 2025-01-01\n---\n\n# Known status\n",
		"0002-unknown-status.md": "---\ntitle: Unknown status\nstatus: retired\ndate: 2025-01-02\n---\n\n# Unknown status\n",
		"0003-absent-status.md":  "---\ntitle: Absent status\ndate: 2025-01-03\n---\n\n# Absent status\n",
	})

	got := run(t, "validate", "--dir", dir)
	assertExit(t, got, 1)
	assertPrefixes(t, "findings", findingLines(got.stdout), []string{"0002-unknown-status.md:3: ERROR unknown_status 0002:"})
}

func TestValidateMissingFrontmatter(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"0001-with-frontmatter.md": "---\ntitle: With frontmatter\nstatus: accepted\ndate: 2025-01-01\n---\n\n# With frontmatter\n",
		"0002-bare.md":             "# Bare\n\nThis file matches the document filename pattern but carries no frontmatter.\n",
		"notes.md":                 "# Notes\n\nThis file is not a managed document and is skipped entirely.\n",
	})

	got := run(t, "validate", "--dir", dir)
	assertExit(t, got, 0)
	assertPrefixes(t, "findings", findingLines(got.stdout), []string{"0002-bare.md:1: WARN missing_frontmatter 0002:"})
	want := "OK: 2 docs, 0 typed edges, no cycles"
	if ls := lines(got.stdout); len(ls) == 0 || ls[len(ls)-1] != want {
		t.Errorf("summary line = %q, want %q: unmanaged files are skipped", got.stdout, want)
	}
}

func TestValidateJSONReportCarriesLocations(t *testing.T) {
	got := run(t, "validate", "--format", "json", "--dir", fixture(t, "status-drift"))

	assertExit(t, got, 1)
	report := decodeJSON[render.Report](t, got.stdout)
	if report.SchemaVersion != render.ReportSchemaVersion {
		t.Errorf("schema_version = %d, want %d", report.SchemaVersion, render.ReportSchemaVersion)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("findings = %+v, want one", report.Findings)
	}
	loc := report.Findings[0].Location
	if filepath.Base(loc.Path) != "0001-serve-images-from-the-application-server.md" {
		t.Errorf("location path = %q, want the drifted document", loc.Path)
	}
	if loc.Line != 3 {
		t.Errorf("location line = %d, want the status key line", loc.Line)
	}
}

func TestValidateJSONReportRelatesTheCollidingFiles(t *testing.T) {
	got := run(t, "validate", "--format", "json", "--dir", fixture(t, "id-collision"))

	assertExit(t, got, 1)
	report := decodeJSON[render.Report](t, got.stdout)
	if len(report.Findings) != 1 {
		t.Fatalf("findings = %+v, want one", report.Findings)
	}
	f := report.Findings[0]
	if filepath.Base(f.Location.Path) != "0004-a.md" {
		t.Errorf("location = %+v, want the lexically first path", f.Location)
	}
	if len(f.Related) != 1 || filepath.Base(f.Related[0].Path) != "0004-b.md" {
		t.Errorf("related = %+v, want the colliding peer", f.Related)
	}
}

func TestValidateGitHubFormat(t *testing.T) {
	got := run(t, "validate", "--format", "github", "--dir", fixture(t, "status-drift"))

	assertExit(t, got, 1)
	all := lines(got.stdout)
	if len(all) != 1 {
		t.Fatalf("stdout = %q, want one annotation and no summary for a failing corpus", got.stdout)
	}
	for _, want := range []string{
		"::error file=",
		"0001-serve-images-from-the-application-server.md",
		"line=3",
		"title=status_drift",
		"::0001: has inbound supersedes but status is not superseded",
	} {
		if !strings.Contains(all[0], want) {
			t.Errorf("annotation = %q, want it to contain %q", all[0], want)
		}
	}
	if strings.Contains(all[0], "col=") {
		t.Errorf("annotation = %q, want an unknown column omitted", all[0])
	}
}

func TestValidateGitHubFormatEndsWithTheSummary(t *testing.T) {
	got := run(t, "validate", "--format", "github", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	all := lines(got.stdout)
	if len(all) != 2 {
		t.Fatalf("stdout = %q, want one annotation and the summary", got.stdout)
	}
	if !strings.HasPrefix(all[0], "::warning file=") {
		t.Errorf("annotation = %q, want a warning workflow command", all[0])
	}
	if all[1] != "OK: 4 docs, 3 typed edges, no cycles" {
		t.Errorf("summary = %q, want the text summary", all[1])
	}
}

func TestValidateRDJSONFormat(t *testing.T) {
	type diagnostic struct {
		Message  string `json:"message"`
		Severity string `json:"severity"`
		Location struct {
			Path  string `json:"path"`
			Range struct {
				Start struct {
					Line int `json:"line"`
				} `json:"start"`
			} `json:"range"`
		} `json:"location"`
		Code struct {
			Value string `json:"value"`
		} `json:"code"`
	}
	type result struct {
		Source struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"source"`
		Severity    string       `json:"severity"`
		Diagnostics []diagnostic `json:"diagnostics"`
	}

	got := run(t, "validate", "--format", "rdjson", "--dir", fixture(t, "status-drift"))

	assertExit(t, got, 1)
	decoded := decodeJSON[result](t, got.stdout)
	if decoded.Source.Name != "docdag" {
		t.Errorf("source = %+v, want docdag", decoded.Source)
	}
	if decoded.Severity != "ERROR" {
		t.Errorf("severity = %q, want ERROR", decoded.Severity)
	}
	if len(decoded.Diagnostics) != 1 {
		t.Fatalf("diagnostics = %+v, want one", decoded.Diagnostics)
	}
	d := decoded.Diagnostics[0]
	if d.Code.Value != model.RuleStatusDrift {
		t.Errorf("code = %q, want the rule name", d.Code.Value)
	}
	if d.Message != "status_drift 0001: has inbound supersedes but status is not superseded" {
		t.Errorf("message = %q, want the rule, the id and the detail", d.Message)
	}
	if filepath.Base(d.Location.Path) != "0001-serve-images-from-the-application-server.md" {
		t.Errorf("path = %q, want the drifted document", d.Location.Path)
	}
	if d.Location.Range.Start.Line != 3 {
		t.Errorf("line = %d, want the status key line", d.Location.Range.Start.Line)
	}
}

func TestValidateRDJSONReportsAnEmptyCorpusAsAnEmptyDiagnosticList(t *testing.T) {
	got := run(t, "validate", "--format", "rdjson", "--dir", fixture(t, "ok-basic"))

	assertExit(t, got, 0)
	if !strings.Contains(got.stdout, `"diagnostics": []`) {
		t.Errorf("stdout = %q, want an empty diagnostic list", got.stdout)
	}
}

func TestTheReportFormatsBelongToValidateAlone(t *testing.T) {
	okBasic := fixture(t, "ok-basic")
	for _, format := range []string{"github", "rdjson"} {
		for _, args := range [][]string{
			{"resolve", "0001"},
			{"query", "--binding"},
			{"stats"},
		} {
			t.Run(format+" "+args[0], func(t *testing.T) {
				got := run(t, append(append([]string{}, args...), "--format", format, "--dir", okBasic)...)

				assertExit(t, got, 2)
				if !strings.Contains(got.stderr, "invalid --format") {
					t.Errorf("stderr = %q, want an invalid format diagnostic", got.stderr)
				}
			})
		}
	}
}
