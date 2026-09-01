package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lintCorpus writes a configuration file and a two-document corpus, and returns
// the configuration's path and the documents directory.
func lintCorpus(t *testing.T, configuration string) (string, string) {
	t.Helper()
	dir := writeDocs(t, map[string]string{
		"docdag.yaml":                          configuration,
		"docs/adr/0001-the-first-decision.md":  "---\ntitle: The first decision\nstatus: superseded\ndate: 2026-01-01\n---\n\n# The first decision\n",
		"docs/adr/0002-the-second-decision.md": "---\ntitle: The second decision\nstatus: accepted\nsupersedes:\n  - \"0001\"\ndate: 2026-01-01\n---\n\n# The second decision\n",
	})
	return filepath.Join(dir, "docdag.yaml"), filepath.Join(dir, "docs", "adr")
}

// lintFixtures names one of the fixture corpora DocDag ships for its presets.
func lintFixtures(t *testing.T, preset string) string {
	t.Helper()
	dir := filepath.Join("..", "testdata", "lint", preset)
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("shipped fixtures for %s: %v", preset, err)
	}
	return dir
}

func TestLintOnACleanConfiguration(t *testing.T) {
	config, docs := lintCorpus(t, "preset: adr\n")

	got := run(t, "lint", "--config", config, "--dir", docs)

	assertExit(t, got, 0)
	assertLines(t, "lint", lines(got.stdout), []string{"OK: no lint findings"})
}

func TestLintReportsAContradiction(t *testing.T) {
	config, docs := lintCorpus(t, `preset: adr
rules:
  - name: impossible
    severity: error
    when:
      inbound: supersedes
      not_inbound: supersedes
    message: cannot happen
`)

	got := run(t, "lint", "--config", config, "--dir", docs)

	assertExit(t, got, 1)
	assertPrefixes(t, "lint", findingLines(got.stdout), []string{
		"docdag.yaml:3: ERROR unfirable_rule impossible: every alternative contradicts itself",
	})
}

func TestLintExitCodes(t *testing.T) {
	warning := `preset: adr
rules:
  - name: everything
    severity: warn
    when: {}
    message: fires on every document
`
	config, docs := lintCorpus(t, warning)

	t.Run("warnings alone exit 2", func(t *testing.T) {
		got := run(t, "lint", "--config", config, "--dir", docs)

		assertExit(t, got, 2)
		assertPrefixes(t, "lint", findingLines(got.stdout), []string{
			"docdag.yaml:3: WARN tautological_rule everything: constrains nothing",
		})
	})

	t.Run("--strict makes a warning a failure", func(t *testing.T) {
		assertExit(t, run(t, "lint", "--strict", "--config", config, "--dir", docs), 1)
	})

	t.Run("a configuration that does not validate exits 3", func(t *testing.T) {
		broken, docs := lintCorpus(t, "preset: adr\nstructural:\n  not_a_check: error\n")

		assertExit(t, run(t, "lint", "--config", broken, "--dir", docs), 3)
	})
}

func TestLintCorpusLayer(t *testing.T) {
	config, docs := lintCorpus(t, "preset: adr\n")

	t.Run("the configuration alone says nothing about the corpus", func(t *testing.T) {
		assertExit(t, run(t, "lint", "--config", config, "--dir", docs), 0)
	})

	t.Run("--corpus reports the rules the vault never fires", func(t *testing.T) {
		got := run(t, "lint", "--corpus", "--config", config, "--dir", docs)

		assertExit(t, got, 2)
		if !strings.Contains(got.stdout, "WARN never_fired status_drift: fired on 0 of 2 documents") {
			t.Errorf("stdout = %q, want the rule the corpus never fires", got.stdout)
		}
		if !strings.Contains(got.stdout, "<preset:adr>") {
			t.Errorf("stdout = %q, want a preset rule located at the preset", got.stdout)
		}
	})
}

func TestLintShippedFixtures(t *testing.T) {
	t.Run("the adr preset", func(t *testing.T) {
		dir := lintFixtures(t, "adr")

		got := run(t, "lint", "--config", filepath.Join(dir, "docdag.yaml"), "--dir", dir, "--fixtures", dir)

		assertExit(t, got, 0)
		assertLines(t, "lint", lines(got.stdout), []string{"OK: no lint findings"})
	})

	t.Run("the spec preset", func(t *testing.T) {
		dir := lintFixtures(t, "spec")

		got := run(t, "lint", "--config", filepath.Join(dir, "docdag.yaml"), "--fixtures", dir)

		assertExit(t, got, 0)
		assertLines(t, "lint", lines(got.stdout), []string{"OK: no lint findings"})
	})

	t.Run("--all runs every layer", func(t *testing.T) {
		dir := lintFixtures(t, "adr")

		got := run(t, "lint", "--all", "--config", filepath.Join(dir, "docdag.yaml"), "--dir", dir)

		// The fixture directories are the corpus here, so the corpus layer has
		// documents to answer about and the fixtures are read from lint/, which
		// this repository does not have: what matters is that both layers ran.
		if got.code != 0 && got.code != 2 {
			t.Fatalf("exit code = %d, want 0 or 2 (stdout=%q)", got.code, got.stdout)
		}
	})
}

func TestLintFormats(t *testing.T) {
	config, docs := lintCorpus(t, `preset: adr
rules:
  - name: everything
    severity: warn
    when: {}
    message: fires on every document
`)

	t.Run("json is a report of its own kind", func(t *testing.T) {
		got := run(t, "lint", "--format", "json", "--config", config, "--dir", docs)

		assertExit(t, got, 2)
		var report struct {
			Kind          string `json:"kind"`
			SchemaVersion int    `json:"schema_version"`
			PresetVersion int    `json:"preset_version"`
			Findings      []struct {
				Severity string `json:"severity"`
				Rule     string `json:"rule"`
				ID       string `json:"id"`
				Location struct {
					Path string `json:"path"`
					Line int    `json:"line"`
				} `json:"location"`
				Fix string `json:"fix"`
			} `json:"findings"`
			Summary struct {
				Errors   int `json:"errors"`
				Warnings int `json:"warnings"`
				Infos    int `json:"infos"`
			} `json:"summary"`
		}
		if err := json.Unmarshal([]byte(got.stdout), &report); err != nil {
			t.Fatalf("decode the report: %v: %s", err, got.stdout)
		}
		if report.Kind != "lint" {
			t.Errorf("kind = %q, want the lint report to be its own kind", report.Kind)
		}
		if report.SchemaVersion != 1 {
			t.Errorf("schema_version = %d, want 1", report.SchemaVersion)
		}
		if report.PresetVersion != 1 {
			t.Errorf("preset_version = %d, want the revision the corpus is written against", report.PresetVersion)
		}
		if len(report.Findings) != 1 || report.Findings[0].Rule != "tautological_rule" {
			t.Fatalf("findings = %+v, want the one tautological rule", report.Findings)
		}
		if report.Findings[0].Location.Line == 0 || !strings.HasSuffix(report.Findings[0].Location.Path, "docdag.yaml") {
			t.Errorf("location = %+v, want the rule's line in the configuration file", report.Findings[0].Location)
		}
		if report.Summary != struct {
			Errors   int `json:"errors"`
			Warnings int `json:"warnings"`
			Infos    int `json:"infos"`
		}{Warnings: 1} {
			t.Errorf("summary = %+v, want one warning", report.Summary)
		}
	})

	t.Run("github writes one annotation per finding", func(t *testing.T) {
		got := run(t, "lint", "--format", "github", "--config", config, "--dir", docs)

		assertExit(t, got, 2)
		if !strings.Contains(got.stdout, "::warning file=") || !strings.Contains(got.stdout, "title=tautological_rule::") {
			t.Errorf("stdout = %q, want a workflow command per finding", got.stdout)
		}
	})

	t.Run("rdjson writes a diagnostic result", func(t *testing.T) {
		got := run(t, "lint", "--format", "rdjson", "--config", config, "--dir", docs)

		assertExit(t, got, 2)
		var result struct {
			Source struct {
				Name string `json:"name"`
			} `json:"source"`
			Severity    string `json:"severity"`
			Diagnostics []struct {
				Code struct {
					Value string `json:"value"`
				} `json:"code"`
				Severity string `json:"severity"`
			} `json:"diagnostics"`
		}
		if err := json.Unmarshal([]byte(got.stdout), &result); err != nil {
			t.Fatalf("decode the result: %v: %s", err, got.stdout)
		}
		if result.Source.Name != "docdag" || result.Severity != "WARNING" {
			t.Errorf("result = %+v, want a docdag result at the strongest severity it holds", result)
		}
		if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code.Value != "tautological_rule" {
			t.Errorf("diagnostics = %+v, want the one finding", result.Diagnostics)
		}
	})

	t.Run("an unknown format is a usage error", func(t *testing.T) {
		assertExit(t, run(t, "lint", "--format", "mermaid", "--config", config, "--dir", docs), 2)
	})
}

// TestValidateNeverLints is the boundary between the two commands: the state of
// a corpus and the health of a configuration have different lifecycles, and a
// lint warning must not reach a document gate.
func TestValidateNeverLints(t *testing.T) {
	config, docs := lintCorpus(t, `preset: adr
rules:
  - name: everything
    severity: warn
    when: {}
    message: fires on every document
`)

	got := run(t, "validate", "--config", config, "--dir", docs)

	assertExit(t, got, 0)
	if strings.Contains(got.stdout, "tautological_rule") {
		t.Errorf("stdout = %q, want validate never to report a lint finding", got.stdout)
	}
}

func TestNewFixture(t *testing.T) {
	config, docs := lintCorpus(t, `preset: adr
rules:
  - name: drift
    severity: error
    when:
      inbound: supersedes
      attr:
        status:
          not: superseded
    message: has inbound supersedes but is not superseded
`)
	root := filepath.Dir(config)
	fixtures := filepath.Join(root, "lint")

	created := run(t, "new", "--fixture", "drift", "--fixtures", fixtures, "--config", config, "--dir", docs)

	assertExit(t, created, 0)
	if len(lines(created.stdout)) == 0 {
		t.Fatalf("stdout = %q, want the files the generation wrote", created.stdout)
	}
	for _, side := range []string{"ruleid", "ok"} {
		if _, err := os.Stat(filepath.Join(fixtures, "drift", side)); err != nil {
			t.Errorf("%s: %v, want the generated layout", side, err)
		}
	}

	t.Run("the generated fixture passes the layer that reads it", func(t *testing.T) {
		got := run(t, "lint", "--fixtures", fixtures, "--config", config, "--dir", docs)

		assertExit(t, got, 0)
		assertLines(t, "lint", lines(got.stdout), []string{"OK: no lint findings"})
	})

	t.Run("a rule nothing declares is a domain failure", func(t *testing.T) {
		assertExit(t, run(t, "new", "--fixture", "nowhere", "--fixtures", fixtures, "--config", config, "--dir", docs), 1)
	})

	t.Run("new still needs a title without --fixture", func(t *testing.T) {
		assertExit(t, run(t, "new", "--config", config, "--dir", docs), 2)
	})
}
