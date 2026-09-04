package lint_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/goccy/go-yaml"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/lint"
)

// TestCheckMatchesAfterPresetRoundTrip holds lint.Check to the same findings
// before and after a YAML round trip of each built-in preset, across the
// layers the preset's fixtures and (for spec) an empty vault exercise.
func TestCheckMatchesAfterPresetRoundTrip(t *testing.T) {
	root := filepath.Join("..")
	for _, tt := range []struct {
		name     string
		cfg      config.Config
		vault    string
		fixtures string
	}{
		{
			name:     "adr",
			cfg:      config.ADRPreset(),
			fixtures: filepath.Join(root, "testdata", "lint", "adr"),
		},
		{
			name:     "spec",
			cfg:      config.SpecPreset(),
			vault:    root,
			fixtures: filepath.Join(root, "testdata", "lint", "spec"),
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			src, err := yaml.Marshal(tt.cfg)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			path := filepath.Join(t.TempDir(), config.DefaultConfigFile)
			if err := os.WriteFile(path, src, 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			got, err := config.Load(path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			wantFindings, err := lint.Check(tt.cfg, tt.vault, tt.fixtures)
			if err != nil {
				t.Fatalf("Check preset: %v", err)
			}
			gotFindings, err := lint.Check(got, tt.vault, tt.fixtures)
			if err != nil {
				t.Fatalf("Check round-trip: %v", err)
			}
			if !reflect.DeepEqual(gotFindings, wantFindings) {
				t.Fatalf("findings after round-trip = %+v, want %+v", gotFindings, wantFindings)
			}
		})
	}
}

func TestCheckInherentOnly(t *testing.T) {
	findings, err := lint.Check(config.ADRPreset(), "", "")
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("findings = %+v, want none from the ADR preset alone", findings)
	}
}
