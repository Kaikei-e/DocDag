package lint

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kaikei-e/DocDag/config"
)

// testConfigFile writes a configuration file whose line numbers a test can
// name, and returns its path.
func testConfigFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), config.DefaultConfigFile)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write the configuration: %v", err)
	}
	return path
}

func TestLocator(t *testing.T) {
	// Every line is counted from 1, so a test can name what the reader sees.
	path := testConfigFile(t, `preset: spec
status_values:
  - proposed
  - accepted
kinds:
  clause:
    dir: spec/clauses
edges:
  - name: enforces
    key: enforces
projections:
  - name: enforced
    when:
      inbound: enforces
rules:
  - name: orphan_must
    severity: error
path_constraints:
  - name: no_stale
    path: [enforces]
`)
	loc := NewLocator(path, config.PresetSpec)

	tests := []struct {
		name    string
		section string
		entry   string
		want    int
	}{
		{name: "a rule", section: SectionRules, entry: "orphan_must", want: 16},
		{name: "a projection", section: SectionProjections, entry: "enforced", want: 12},
		{name: "an edge", section: SectionEdges, entry: "enforces", want: 9},
		{name: "a path constraint", section: SectionPathConstraints, entry: "no_stale", want: 19},
		{name: "a kind", section: SectionKinds, entry: "clause", want: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name+" is located on its own line", func(t *testing.T) {
			got := loc.Locate(tt.section, tt.entry)

			if got.Path != path {
				t.Errorf("path = %q, want the configuration file %q", got.Path, path)
			}
			if got.Line != tt.want {
				t.Errorf("line = %d, want %d", got.Line, tt.want)
			}
		})
	}

	t.Run("a section is located on its own key", func(t *testing.T) {
		if got := loc.Section(SectionStatusValues); got.Line != 2 {
			t.Errorf("line = %d, want the status_values key on line 2", got.Line)
		}
	})

	t.Run("an entry the file does not write falls back to the section", func(t *testing.T) {
		got := loc.Locate(SectionRules, "somebody_elses_rule")

		if got.Path != path || got.Line != 15 {
			t.Errorf("location = %v, want the rules key the file does write", got)
		}
	})

	t.Run("a section the file does not write is the preset's", func(t *testing.T) {
		got := loc.Locate(SectionEdges, "supersedes")

		if got.Path != path {
			t.Errorf("path = %q, want the file that writes the edges section", got.Path)
		}
		if got := loc.Section("derived_edges"); got.Path != PresetPath(config.PresetSpec) || got.Line != 0 {
			t.Errorf("location = %v, want the preset's virtual path", got)
		}
	})
}

func TestLocatorWithoutAFile(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		preset string
		want   string
	}{
		{name: "a corpus that configures nothing", preset: config.PresetADR, want: "<preset:adr>"},
		{name: "the spec preset", preset: config.PresetSpec, want: "<preset:spec>"},
		{name: "a configuration naming no preset", want: "<preset:adr>"},
		{name: "a file that is not there", path: filepath.Join(t.TempDir(), "missing.yaml"), preset: config.PresetADR, want: "<preset:adr>"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loc := NewLocator(tt.path, tt.preset)

			got := loc.Locate(SectionRules, "status_drift")

			if got.Path != tt.want {
				t.Errorf("path = %q, want %q", got.Path, tt.want)
			}
			if got.Line != 0 {
				t.Errorf("line = %d, want no line for a rule with no file to be on", got.Line)
			}
		})
	}
}

func TestLocatorReadsNothingFromABrokenFile(t *testing.T) {
	path := testConfigFile(t, "rules:\n  - name: [unterminated\n")

	loc := NewLocator(path, config.PresetADR)

	if got := loc.Locate(SectionRules, "anything"); got.Path != PresetPath(config.PresetADR) {
		t.Errorf("location = %v, want a file lint cannot read to fall back on the preset", got)
	}
}
