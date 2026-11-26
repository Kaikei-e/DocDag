package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/render"
)

// execPrime executes the prime subcommand against an isolated root command,
// attaching newPrimeCmd() if cmd/root.go has not wired it yet.
func execPrime(t *testing.T, args ...string) runResult {
	t.Helper()
	root := newRootCmd()
	hasPrime := false
	for _, c := range root.Commands() {
		if c.Name() == "prime" {
			hasPrime = true
			break
		}
	}
	if !hasPrime {
		root.AddCommand(newPrimeCmd())
	}
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(append([]string{"prime"}, args...))
	err := root.Execute()
	if err != nil && err.Error() != "" {
		fmt.Fprintf(&errOut, "Error: %v\n", err)
	}
	return runResult{code: exitCode(err), stdout: out.String(), stderr: errOut.String()}
}

func TestPrimeADRPassing(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"docs/adr/0001-first.md":  "---\ntitle: First decision\nstatus: superseded\ndate: 2025-01-01\n---\n\n# First\n",
		"docs/adr/0002-second.md": "---\ntitle: Second decision\nstatus: accepted\nsupersedes:\n  - \"0001\"\ndate: 2025-02-01\n---\n\n# Second\n",
	})
	t.Chdir(dir)

	t.Run("text", func(t *testing.T) {
		got := execPrime(t)
		assertExit(t, got, 0)
		if got.stderr != "" {
			t.Errorf("stderr = %q, want empty", got.stderr)
		}
		for _, want := range []string{
			"docdag validate: OK: 2 docs, 1 typed edges, no cycles",
			"corpus: docs/adr (preset: adr, 2 docs, 1 binding)",
			"edges: depends-on, supersedes",
			"Prefer docdag over Glob/Grep/Read on managed docs",
			"  docdag context <ref> (e.g. docdag context 0002)",
			"  docdag query --binding --fields id,title,status",
			"  docdag resolve <ref> --fields id,title,status",
			"  docdag query <ref> --ancestors --fields id,title,status",
			`  docdag new "<title>" [--supersedes <ref>]`,
			"  docdag validate --touching <path>",
			"  docdag diff origin/main",
		} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("stdout missing %q\ngot:\n%s", want, got.stdout)
			}
		}
		if strings.Contains(got.stdout, "run docdag validate to see the findings") {
			t.Errorf("passing corpus unexpectedly asks to run validate: %s", got.stdout)
		}
		if strings.Contains(got.stdout, "--as-of YYYY-MM-DD") {
			t.Errorf("adr preset unexpectedly mentions --as-of: %s", got.stdout)
		}

		// Verify determinism.
		again := execPrime(t)
		assertExit(t, again, 0)
		if again.stdout != got.stdout {
			t.Errorf("prime output is non-deterministic:\nfirst:\n%s\nsecond:\n%s", got.stdout, again.stdout)
		}
	})

	t.Run("json", func(t *testing.T) {
		got := execPrime(t, "--format", "json")
		assertExit(t, got, 0)
		report := decodeJSON[render.PrimeReport](t, got.stdout)
		if report.SchemaVersion != 1 {
			t.Errorf("schema_version = %d, want 1", report.SchemaVersion)
		}
		if report.Preset != "adr" || report.PresetVersion == 0 {
			t.Errorf("preset = %q v%d, want adr", report.Preset, report.PresetVersion)
		}
		if !slices.Equal(report.Dirs, []string{"docs/adr"}) {
			t.Errorf("dirs = %v, want [docs/adr]", report.Dirs)
		}
		if !slices.Equal(report.Edges, []string{"depends-on", "supersedes"}) {
			t.Errorf("edges = %v, want [depends-on supersedes]", report.Edges)
		}
		if !report.HasSupersedes || report.HasPeriods || report.HasModality {
			t.Errorf("flags = supersedes:%v periods:%v modality:%v", report.HasSupersedes, report.HasPeriods, report.HasModality)
		}
		if report.Documents != 2 || report.Binding == nil || *report.Binding != 1 {
			t.Errorf("counts = docs:%d binding:%v", report.Documents, report.Binding)
		}
		if !report.Validation.OK || report.Validation.Errors != 0 || report.Validation.Summary != "OK: 2 docs, 1 typed edges, no cycles" {
			t.Errorf("validation = %+v", report.Validation)
		}
		if report.SampleRef != "0002" {
			t.Errorf("sample_ref = %q, want 0002", report.SampleRef)
		}
		if len(report.Commands) == 0 {
			t.Error("commands is empty")
		}
	})
}

func TestPrimeADRFailingExitsZeroAndReportsFailed(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"docs/adr/0001-bad.md":       "---\ntitle: Bad\nstatus: accepted\nsupersedes:\n  - \"0099\"\ndate: 2025-01-01\n---\n\n# Bad\n",
		"docs/adr/0002-orphan.md":    "---\ntitle: Orphan\nstatus: superseded\ndate: 2025-01-02\n---\n\n# Orphan\n",
		"docs/adr/0003-second.md":    "---\ntitle: Second\nstatus: accepted\ndate: 2025-01-03\n---\n\n# Second\n",
		"docs/adr/0004-dangling2.md": "---\ntitle: Dangling 2\nstatus: accepted\ndepends-on:\n  - \"0088\"\ndate: 2025-01-04\n---\n\n# Dangling 2\n",
	})
	t.Chdir(dir)

	got := execPrime(t)
	assertExit(t, got, 0)
	if !strings.Contains(got.stdout, "docdag validate: FAILED: 2 errors, 1 warning\n") {
		t.Errorf("stdout missing FAILED line:\n%s", got.stdout)
	}
	if !strings.Contains(got.stdout, "run docdag validate to see the findings\n") {
		t.Errorf("stdout missing follow-up validate hint:\n%s", got.stdout)
	}

	gotJSON := execPrime(t, "--format", "json")
	assertExit(t, gotJSON, 0)
	report := decodeJSON[render.PrimeReport](t, gotJSON.stdout)
	if report.Validation.OK {
		t.Error("validation.ok = true, want false")
	}
	if report.Validation.Errors != 2 || report.Validation.Warnings != 1 {
		t.Errorf("validation counts = %+v, want 2 errors, 1 warning", report.Validation)
	}
	if report.Validation.Summary != "FAILED: 2 errors, 1 warning" {
		t.Errorf("validation.summary = %q", report.Validation.Summary)
	}
}

func TestPrimeSpecPreset(t *testing.T) {
	dir := copyFixtureTree(t, "spec-vault")
	t.Chdir(dir)

	got := execPrime(t, "--as-of", "2026-06-01")
	assertExit(t, got, 0)
	for _, want := range []string{
		"corpus: 8 kinds (preset: spec,",
		"kinds: clause=spec/clauses, conform=spec/conform, deviation=spec/deviations,",
		"edges: about, counterexample, deviates-from, enforces, excepts, interop, measures, premise, rationale, supersedes",
		"fields: expires, in_force_from, in_force_until, modality, retired_on, test",
		"  docdag query --binding --fields id,modality,title,status",
		`  docdag new "<title>" --kind clause [--id <id>]`,
		"  add --as-of YYYY-MM-DD to query/validate for another day",
		"as of 2026-06-01",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q\ngot:\n%s", want, got.stdout)
		}
	}

	gotJSON := execPrime(t, "--as-of", "2026-06-01", "--format", "json")
	assertExit(t, gotJSON, 0)
	report := decodeJSON[render.PrimeReport](t, gotJSON.stdout)
	if report.Preset != "spec" {
		t.Errorf("preset = %q, want spec", report.Preset)
	}
	if report.AsOf != "2026-06-01" {
		t.Errorf("as_of = %q, want 2026-06-01", report.AsOf)
	}
	if !report.HasPeriods || !report.HasModality || !report.HasSupersedes {
		t.Errorf("flags = periods:%v modality:%v supersedes:%v", report.HasPeriods, report.HasModality, report.HasSupersedes)
	}
	if len(report.Kinds) != 8 {
		t.Errorf("len(kinds) = %d, want 8", len(report.Kinds))
	}
	if report.Kinds[0].Name != "clause" || report.Kinds[0].Dir != "spec/clauses" || report.Kinds[0].IDPattern == "" {
		t.Errorf("first kind = %+v", report.Kinds[0])
	}
}

func TestPrimeCustomFieldsAndRequiredSections(t *testing.T) {
	cfg := `preset: adr
dir: docs/ADR
fields:
  affected_services:
    type: list
  tags:
    type: list
  legacy_owner:
    deprecated: true
sections:
  required: ["Context", "Decision"]
`
	dir := writeDocs(t, map[string]string{
		"docdag.yaml":          cfg,
		"docs/ADR/0001-api.md": "---\ntitle: API gateway\nstatus: accepted\ndate: 2025-01-01\naffected_services: [gateway]\ntags: [edge]\n---\n\n## Context\nNeed routing.\n\n## Decision\nUse gateway.\n",
	})
	t.Chdir(dir)

	got := execPrime(t)
	assertExit(t, got, 0)
	for _, want := range []string{
		"fields: affected_services, tags",
		"required sections: Context, Decision",
		"  docdag query --binding --where affected_services=contains:<val> --fields id,title,affected_services",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout missing %q\ngot:\n%s", want, got.stdout)
		}
	}
	if strings.Contains(got.stdout, "legacy_owner") {
		t.Errorf("deprecated field leaked into stdout:\n%s", got.stdout)
	}

	gotJSON := execPrime(t, "--format", "json")
	assertExit(t, gotJSON, 0)
	report := decodeJSON[render.PrimeReport](t, gotJSON.stdout)
	if !slices.Equal(report.Fields, []string{"affected_services", "tags"}) {
		t.Errorf("fields = %v, want [affected_services tags]", report.Fields)
	}
	if !slices.Equal(report.RequiredSections, []string{"Context", "Decision"}) {
		t.Errorf("required_sections = %v, want [Context Decision]", report.RequiredSections)
	}
}

func TestPrimeWithoutSupersedesEdge(t *testing.T) {
	cfg := `dir: docs
rules: []
derived_edges: []
edges:
  - name: depends-on
    key: depends-on
    acyclic: true
    direction: forward
`
	dir := writeDocs(t, map[string]string{
		"docdag.yaml":    cfg,
		"docs/0001-a.md": "---\ntitle: Alpha\nstatus: accepted\ndate: 2025-01-01\n---\n\n# Alpha\n",
		"docs/0002-b.md": "---\ntitle: Beta\nstatus: accepted\ndepends-on:\n  - \"0001\"\ndate: 2025-01-02\n---\n\n# Beta\n",
	})
	t.Chdir(dir)

	got := execPrime(t)
	assertExit(t, got, 0)
	if !strings.Contains(got.stdout, "corpus: docs (preset: adr, 2 docs)\n") {
		t.Errorf("corpus line = %q", got.stdout)
	}
	if strings.Contains(got.stdout, "binding") {
		t.Errorf("unexpected binding mention when supersedes absent:\n%s", got.stdout)
	}
	if strings.Contains(got.stdout, "--binding") || strings.Contains(got.stdout, "docdag resolve") || strings.Contains(got.stdout, "--supersedes") {
		t.Errorf("unexpected supersedes commands when supersedes is not declared:\n%s", got.stdout)
	}
	if !strings.Contains(got.stdout, `  docdag new "<title>"`) {
		t.Errorf("stdout missing plain new command:\n%s", got.stdout)
	}

	gotJSON := execPrime(t, "--format", "json")
	assertExit(t, gotJSON, 0)
	report := decodeJSON[render.PrimeReport](t, gotJSON.stdout)
	if report.HasSupersedes {
		t.Error("has_supersedes = true, want false")
	}
	if report.Binding != nil {
		t.Errorf("binding = %v, want nil/omitted", report.Binding)
	}
	if strings.Contains(gotJSON.stdout, `"binding"`) {
		t.Errorf("json contains binding key when supersedes absent:\n%s", gotJSON.stdout)
	}
	if report.SampleRef != "0001" {
		t.Errorf("sample_ref = %q, want 0001", report.SampleRef)
	}

	gotHint := execPrime(t, "--hint", "docs/0001-a.md")
	assertExit(t, gotHint, 0)
	if strings.Contains(gotHint.stdout, "--binding") {
		t.Errorf("hint unexpectedly mentions --binding when supersedes absent: %q", gotHint.stdout)
	}
	if !strings.Contains(gotHint.stdout, "docdag context docs/0001-a.md") {
		t.Errorf("hint missing docdag context command: %q", gotHint.stdout)
	}
}

func TestPrimeIfPresent(t *testing.T) {
	t.Run("empty dir without corpus exits 0 silently", func(t *testing.T) {
		t.Chdir(t.TempDir())
		got := execPrime(t, "--if-present")
		assertExit(t, got, 0)
		if got.stdout != "" || got.stderr != "" {
			t.Errorf("got stdout=%q stderr=%q, want silence", got.stdout, got.stderr)
		}
	})

	t.Run("broken docdag.yaml fails closed exit 3", func(t *testing.T) {
		dir := writeDocs(t, map[string]string{
			"docdag.yaml": "edges: 3\n",
		})
		t.Chdir(dir)
		got := execPrime(t, "--if-present")
		assertExit(t, got, 3)
		if got.stderr == "" {
			t.Error("stderr is empty, want configuration error")
		}
	})

	t.Run("explicit empty dir fails closed exit 3", func(t *testing.T) {
		dir := writeDocs(t, map[string]string{
			"docdag.yaml": "dir: docs/adr\n",
		})
		if err := os.MkdirAll(filepath.Join(dir, "docs", "adr"), 0o750); err != nil {
			t.Fatal(err)
		}
		t.Chdir(dir)
		got := execPrime(t, "--if-present")
		assertExit(t, got, 3)
		if !strings.Contains(got.stderr, "no documents found") {
			t.Errorf("stderr = %q, want empty corpus error", got.stderr)
		}
	})
}

func TestPrimeHint(t *testing.T) {
	t.Run("adr corpus files, dirs, globs, exempt, unrelated, and json", func(t *testing.T) {
		// Deliberately include a broken ADR so we also prove --hint never parses documents.
		dir := writeDocs(t, map[string]string{
			"docdag.yaml":                 "preset: adr\ndir: docs/adr\ntemplate: docs/adr/custom-tpl.md\nfields:\n  tags:\n    type: list\n",
			"docs/adr/0001-first.md":      "---\ntitle: Broken\nstatus: accepted\nsupersedes:\n  - \"0099\"\n---\n",
			"docs/adr/README.md":          "# Readme\n",
			"docs/adr/index.md":           "# Index\n",
			"docs/adr/template.md":        "# Template\n",
			"docs/adr/custom-tpl.md":      "# Custom template\n",
			"docs/adr/_scratch.md":        "# Scratch\n",
			"docs/adr/.hidden.md":         "# Hidden\n",
			"docs/adr/sub/0002-nested.md": "# Nested\n",
			"docs/adr/notes.txt":          "notes\n",
			"README.md":                   "# Root\n",
			"main.go":                     "package main\n",
		})
		t.Chdir(dir)

		// Managed existing and non-existing .md files
		for _, path := range []string{"docs/adr/0001-first.md", "docs/adr/0099-new.md", filepath.Join(dir, "docs", "adr", "0001-first.md")} {
			got := execPrime(t, "--if-present", "--hint", path)
			assertExit(t, got, 0)
			if !strings.Contains(got.stdout, "Managed DocDag document: run 'docdag context docs/adr/") {
				t.Errorf("hint(%q) stdout = %q", path, got.stdout)
			}
			if !strings.Contains(got.stdout, "--where tags=contains:<val>") {
				t.Errorf("hint(%q) missing declared field filter tip: %q", path, got.stdout)
			}
			if lines := strings.Split(strings.TrimSuffix(got.stdout, "\n"), "\n"); len(lines) > 2 {
				t.Errorf("hint(%q) exceeded 2 lines (%d): %q", path, len(lines), got.stdout)
			}
		}

		// Managed directory, ancestor directory, trailing slash, and globs
		for _, tc := range []struct {
			hint     string
			wantPath string
		}{
			{hint: "docs/adr", wantPath: "docs/adr"},
			{hint: "docs/adr/", wantPath: "docs/adr"},
			{hint: "docs", wantPath: "docs"},
			{hint: "docs/adr/*.md", wantPath: "docs/adr"},
			{hint: "docs/adr/**/*.md", wantPath: "docs/adr"},
			{hint: "docs/adr/000*.md", wantPath: "docs/adr"},
			{hint: "docs/**/*.md", wantPath: "docs"},
		} {
			got := execPrime(t, "--if-present", "--hint", tc.hint, "--format", "json")
			assertExit(t, got, 0)
			h := decodeJSON[render.PrimeHint](t, got.stdout)
			if !h.Managed || h.Kind != "dir" || h.Path != tc.wantPath {
				t.Errorf("hint(%q) = %+v, want managed=true kind=dir path=%q", tc.hint, h, tc.wantPath)
			}
			if !strings.Contains(h.Hint, "Managed DocDag directory:") || !strings.Contains(h.Hint, "--where tags=contains:<val>") {
				t.Errorf("hint(%q) text = %q", tc.hint, h.Hint)
			}
		}

		// JSON for file
		gotFileJSON := execPrime(t, "--if-present", "--hint", "docs/adr/0001-first.md", "--format", "json")
		assertExit(t, gotFileJSON, 0)
		hf := decodeJSON[render.PrimeHint](t, gotFileJSON.stdout)
		if !hf.Managed || hf.Kind != "file" || hf.Path != "docs/adr/0001-first.md" {
			t.Errorf("file json = %+v", hf)
		}

		// Silent non-matches: exempt files, nested/non-md, unrelated files, repo root, root-wide globs
		for _, silent := range []string{
			"docs/adr/README.md",
			"docs/adr/readme.md",
			"docs/adr/index.md",
			"docs/adr/template.md",
			"docs/adr/custom-tpl.md",
			"docs/adr/_scratch.md",
			"docs/adr/.hidden.md",
			"docs/adr/sub/0002-nested.md",
			"docs/adr/notes.txt",
			"README.md",
			"main.go",
			".",
			"./",
			"..",
			"/",
			dir,
			"*.md",
			"**/*.md",
			"**/*.go",
			"./*.md",
			"",
		} {
			got := execPrime(t, "--if-present", "--hint", silent)
			assertExit(t, got, 0)
			if got.stdout != "" || got.stderr != "" {
				t.Errorf("hint(%q) got stdout=%q stderr=%q, want silence", silent, got.stdout, got.stderr)
			}
		}
	})

	t.Run("spec corpus file, dir, and glob", func(t *testing.T) {
		dir := writeDocs(t, map[string]string{
			"docdag.yaml":              "preset: spec\n",
			"spec/clauses/UZ-V-001.md": "---\ntitle: Broken\nkind: clause\nmodality: MUST\nstatus: accepted\n---\n",
		})
		t.Chdir(dir)

		gotFile := execPrime(t, "--if-present", "--hint", "spec/clauses/UZ-V-001.md")
		assertExit(t, gotFile, 0)
		if !strings.Contains(gotFile.stdout, "docdag context UZ-V-001") {
			t.Errorf("spec file hint = %q, want docdag context UZ-V-001", gotFile.stdout)
		}
		if !strings.Contains(gotFile.stdout, "--fields id,modality,title,status") {
			t.Errorf("spec file hint = %q, want modality in binding fields", gotFile.stdout)
		}

		for _, g := range []string{"spec/clauses/*.md", "spec/**/*.md", "spec/clauses", "spec", "spec/deviations"} {
			gotDir := execPrime(t, "--if-present", "--hint", g, "--format", "json")
			assertExit(t, gotDir, 0)
			h := decodeJSON[render.PrimeHint](t, gotDir.stdout)
			if !h.Managed || h.Kind != "dir" {
				t.Errorf("spec hint(%q) = %+v", g, h)
			}
		}
	})

	t.Run("hint without corpus respects --if-present and fails closed on broken config", func(t *testing.T) {
		empty := t.TempDir()
		t.Chdir(empty)
		assertExit(t, execPrime(t, "--if-present", "--hint", "README.md"), 0)
		assertExit(t, execPrime(t, "--hint", "README.md"), 3)

		broken := writeDocs(t, map[string]string{"docdag.yaml": "edges: 3\n", "README.md": "# x\n"})
		t.Chdir(broken)
		assertExit(t, execPrime(t, "--if-present", "--hint", "README.md"), 3)
	})
}

func TestPrimeUsageErrorsAndRendererErrors(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"docs/adr/0001-first.md": "---\ntitle: First\nstatus: accepted\ndate: 2025-01-01\n---\n\n# First\n",
	})
	t.Chdir(dir)

	assertExit(t, execPrime(t, "extra-arg"), 2)
	assertExit(t, execPrime(t, "--format", "yaml"), 2)
	assertExit(t, execPrime(t, "--as-of", "not-a-date"), 2)

	errW := primeFailingWriter{}
	if err := render.PrimeText(errW, render.PrimeReport{}); !errors.Is(err, errPrimeWrite) {
		t.Errorf("PrimeText err = %v", err)
	}
	if err := render.PrimeJSON(errW, render.PrimeReport{}); !errors.Is(err, errPrimeWrite) {
		t.Errorf("PrimeJSON err = %v", err)
	}
	if err := render.PrimeHintText(errW, render.PrimeHint{Hint: "x"}); !errors.Is(err, errPrimeWrite) {
		t.Errorf("PrimeHintText err = %v", err)
	}
	if err := render.PrimeHintJSON(errW, render.PrimeHint{Hint: "x"}); !errors.Is(err, errPrimeWrite) {
		t.Errorf("PrimeHintJSON err = %v", err)
	}
}

var errPrimeWrite = errors.New("write failed")

type primeFailingWriter struct{}

func (primeFailingWriter) Write([]byte) (int, error) { return 0, errPrimeWrite }
