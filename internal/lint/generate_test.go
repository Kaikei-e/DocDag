package lint

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

var testDay = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

func TestSampleID(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		index   int
		want    string
	}{
		{name: "a literal and a digit run", pattern: config.IDClause, index: 0, want: "UZ-A-000"},
		{name: "the next identifier bumps the last digit", pattern: config.IDClause, index: 1, want: "UZ-A-001"},
		{name: "a pattern carrying a slash", pattern: config.IDConform, index: 0, want: "conform/a"},
		{name: "the next one bumps the last letter", pattern: config.IDConform, index: 1, want: "conform/b"},
		{name: "a four-digit sequence", pattern: config.IDDeviation, index: 0, want: "dev-0000"},
		{name: "a nested pattern", pattern: config.IDMeasure, index: 0, want: "interp/UZ-A-000@0000-00-00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := sampleID(tt.pattern, tt.index)

			if !ok {
				t.Fatalf("sampleID(%q, %d): no sample, want %q", tt.pattern, tt.index, tt.want)
			}
			if got != tt.want {
				t.Errorf("sampleID(%q, %d) = %q, want %q", tt.pattern, tt.index, got, tt.want)
			}
			pattern, err := config.IDPattern(tt.pattern)
			if err != nil || !pattern.MatchString(got) {
				t.Errorf("sampleID = %q, want an identifier the pattern accepts", got)
			}
		})
	}

	t.Run("a pattern nothing can be sampled from", func(t *testing.T) {
		if got, ok := sampleID(`^[[:alpha:]{`, 0); ok {
			t.Errorf("sampleID = %q, want no sample from a pattern that does not parse", got)
		}
	})
}

// TestGenerateRoundTrip generates a fixture from a rule and then reads it back
// with the layer that checks fixtures: what the generator writes has to be what
// the checker accepts, or the command is a trap.
func TestGenerateRoundTrip(t *testing.T) {
	root := t.TempDir()
	cfg := testConfig(testDriftRule())

	skeleton, err := Generate(cfg, "drift", root, filepath.Join(root, DefaultFixtureDir), testDay)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	written, err := skeleton.Apply()
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(written) != len(skeleton.Files) {
		t.Fatalf("wrote %d of %d files", len(written), len(skeleton.Files))
	}
	for _, file := range skeleton.Files {
		if strings.Contains(string(file.Content), "TODO") {
			t.Errorf("%s holds a TODO, want a rule this simple to be generated whole:\n%s", file.Path, file.Content)
		}
	}

	findings, firable, err := Fixtures(testFixtureOptions(cfg, root, filepath.Join(root, DefaultFixtureDir)))
	if err != nil {
		t.Fatalf("Fixtures: %v", err)
	}
	if len(findings) > 0 {
		t.Errorf("findings = %s, want the generated fixture to pass", formatFindings(findings))
	}
	if !firable["drift"] {
		t.Error("firable = false, want the generated ruleid corpus to fire the rule")
	}
}

// TestGenerateKindFixture generates against the spec preset, where a document
// has a kind, an identifier its kind's pattern accepts and a directory of its
// own inside the fixture.
func TestGenerateKindFixture(t *testing.T) {
	root := t.TempDir()
	cfg := config.SpecPreset()
	for name, spec := range cfg.Kinds {
		spec.Dir = filepath.Join(root, spec.Dir)
		cfg.Kinds[name] = spec
	}
	fixtures := filepath.Join(root, DefaultFixtureDir)

	skeleton, err := Generate(cfg, model.RuleOrphanMust, root, fixtures, testDay)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, err := skeleton.Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	subject := filepath.Join(fixtures, model.RuleOrphanMust, FixtureFires, "spec", "clauses", "UZ-A-000.md")
	content, err := os.ReadFile(subject)
	if err != nil {
		t.Fatalf("read the generated clause: %v", err)
	}
	for _, want := range []string{"kind: clause", "modality: MUST", "status: accepted", "id: UZ-A-000"} {
		if !strings.Contains(string(content), want) {
			t.Errorf("generated clause:\n%s\nwant it to carry %q", content, want)
		}
	}

	findings, _, err := Fixtures(testFixtureOptions(cfg, root, fixtures))
	if err != nil {
		t.Fatalf("Fixtures: %v", err)
	}
	if len(findings) > 0 {
		t.Errorf("findings = %s, want the generated fixture to pass", formatFindings(findings))
	}
}

// TestGenerateAnnotatesWhatItCannotDo covers the honest half of the generator:
// a condition it cannot satisfy is still written, and says what is left.
func TestGenerateAnnotatesWhatItCannotDo(t *testing.T) {
	root := t.TempDir()
	cfg := testConfig(testRule("wide_open", model.SeverityWarn, config.Condition{
		Attr: map[string]config.AttrCondition{"tags": {SubsetOf: []string{"a", "b"}}},
	}))

	skeleton, err := Generate(cfg, "wide_open", root, filepath.Join(root, DefaultFixtureDir), testDay)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	if !slices.ContainsFunc(skeleton.Files, func(f SkeletonFile) bool { return strings.Contains(string(f.Content), "TODO") }) {
		t.Errorf("files = %v, want a note saying what the generator could not answer", skeleton.Files)
	}
}

func TestGenerateRefusesWhatItCannotName(t *testing.T) {
	root := t.TempDir()

	t.Run("a rule nothing declares", func(t *testing.T) {
		_, err := Generate(testConfig(), "nowhere", root, root, testDay)

		if err == nil {
			t.Fatal("Generate: no error, want a rule that is not configured to be refused")
		}
	})

	t.Run("a rule that can never fire", func(t *testing.T) {
		cfg := testConfig(testRule("dead", model.SeverityError, config.Condition{
			Inbound:    config.EdgeCondition{Edge: "supersedes"},
			NotInbound: "supersedes",
		}))

		_, err := Generate(cfg, "dead", root, root, testDay)

		if err == nil {
			t.Fatal("Generate: no error, want a rule with no satisfiable alternative to be refused")
		}
	})
}

// TestApplyKeepsWhatIsThere makes a second generation safe to run: a fixture
// somebody has edited is never overwritten.
func TestApplyKeepsWhatIsThere(t *testing.T) {
	root := t.TempDir()
	cfg := testConfig(testDriftRule())
	fixtures := filepath.Join(root, DefaultFixtureDir)

	skeleton, err := Generate(cfg, "drift", root, fixtures, testDay)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if _, err := skeleton.Apply(); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	edited := "---\ntitle: Edited by hand\nstatus: accepted\n---\n"
	if err := os.WriteFile(skeleton.Files[0].Path, []byte(edited), 0o644); err != nil {
		t.Fatalf("edit the fixture: %v", err)
	}

	written, err := skeleton.Apply()
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	if len(written) != 0 {
		t.Errorf("wrote %v, want a second run to write nothing", written)
	}
	content, err := os.ReadFile(skeleton.Files[0].Path)
	if err != nil {
		t.Fatalf("read the fixture: %v", err)
	}
	if string(content) != edited {
		t.Errorf("content = %q, want the hand-written fixture kept", content)
	}
}
