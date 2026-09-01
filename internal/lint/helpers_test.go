package lint

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// testStatuses is the vocabulary the single-kind tests are written against: the
// ADR preset's, so a test can name a value without inventing one.
var testStatuses = []string{
	config.StatusProposed,
	config.StatusAccepted,
	config.StatusRejected,
	config.StatusSuperseded,
	config.StatusWithdrawn,
}

func testEq(v string) config.AttrCondition       { return config.AttrCondition{Eq: &v} }
func testNot(v string) config.AttrCondition      { return config.AttrCondition{Not: &v} }
func testContains(v string) config.AttrCondition { return config.AttrCondition{Contains: &v} }
func testInt(v int) *int                         { return &v }

// testConfig is a small single-kind configuration: two edges, the ADR status
// vocabulary, and whatever rules a test hands it.
func testConfig(rules ...config.Rule) config.Config {
	return config.Config{
		Preset:       config.PresetADR,
		Dir:          "docs/adr",
		IDWidth:      config.DefaultIDWidth,
		StatusField:  config.DefaultStatusField,
		StatusValues: slices.Clone(testStatuses),
		Edges: []config.EdgeSpec{
			{Name: "supersedes", Key: "supersedes", Acyclic: true, Direction: config.DirectionForward},
			{Name: "depends-on", Key: "depends-on", Direction: config.DirectionForward, MaxInbound: 2},
		},
		Rules: rules,
	}
}

// testRule is one rule under a name no preset uses, so a test never has to
// think about the built-in fixes or the preset fixtures.
func testRule(name string, severity model.Severity, when config.Condition) config.Rule {
	return config.Rule{Name: name, Severity: severity, When: when, Message: "reports " + name}
}

// testAttr is the condition on the status field a test writes most often.
func testAttr(key string, want config.AttrCondition) config.Condition {
	return config.Condition{Attr: map[string]config.AttrCondition{key: want}}
}

// testLint runs layer 1 over a configuration with no configuration file, so
// every finding is located at the preset's virtual path.
func testLint(cfg config.Config) []model.Finding {
	return Inherent(cfg, NewLocator("", cfg.Preset))
}

// findingFor returns the finding of one name filed against one subject.
func findingFor(findings []model.Finding, rule, id string) (model.Finding, bool) {
	for _, f := range findings {
		if f.Rule == rule && f.ID == model.ID(id) {
			return f, true
		}
	}
	return model.Finding{}, false
}

// assertFinding fails unless exactly one finding of a name was filed against a
// subject, at the wanted severity and with a detail holding the wanted phrase.
func assertFinding(t *testing.T, findings []model.Finding, rule, id string, severity model.Severity, phrase string) model.Finding {
	t.Helper()
	f, ok := findingFor(findings, rule, id)
	if !ok {
		t.Fatalf("%s %s: no finding, got %s", rule, id, formatFindings(findings))
	}
	if f.Severity != severity {
		t.Errorf("%s %s: severity %q, want %q", rule, id, f.Severity, severity)
	}
	if phrase != "" && !strings.Contains(f.Detail, phrase) {
		t.Errorf("%s %s: detail %q, want it to hold %q", rule, id, f.Detail, phrase)
	}
	return f
}

// assertNoFinding fails when a name was reported at all.
func assertNoFinding(t *testing.T, findings []model.Finding, rule string) {
	t.Helper()
	for _, f := range findings {
		if f.Rule == rule {
			t.Fatalf("%s: reported %s, want nothing", rule, formatFindings(findings))
		}
	}
}

func formatFindings(findings []model.Finding) string {
	if len(findings) == 0 {
		return "no findings"
	}
	lines := make([]string, 0, len(findings))
	for _, f := range findings {
		lines = append(lines, string(f.Severity)+" "+f.Rule+" "+f.ID.String()+": "+f.Detail)
	}
	return "[" + strings.Join(lines, "; ") + "]"
}

// testCorpus writes a corpus of Markdown files into a temporary directory and
// reports the directory. Each entry is a file name and its content.
func testCorpus(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create %s: %v", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return root
}

// testDocument is one ADR-shaped document, as a fixture writes it.
func testDocument(title, status string, keys ...string) string {
	lines := []string{"---", "title: " + title, "status: " + status}
	lines = append(lines, keys...)
	lines = append(lines, "date: 2026-01-01", "---", "", "# "+title, "")
	return strings.Join(lines, "\n")
}
