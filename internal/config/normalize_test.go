package config

import (
	"testing"

	"github.com/Kaikei-e/DocDag/internal/model"
)

func TestADRNormalizerNormalize(t *testing.T) {
	tests := []struct {
		name string
		pad  int
		ref  string
		want model.ID
		ok   bool
	}{
		{name: "a bare number", pad: 4, ref: "339", want: "0339", ok: true},
		{name: "a prefixed reference", pad: 4, ref: "ADR-339", want: "0339", ok: true},
		{name: "an already padded reference", pad: 4, ref: "000339", want: "0339", ok: true},
		{name: "a file stem with a title", pad: 4, ref: "0339-use-postgres", want: "0339", ok: true},
		{name: "a file name", pad: 4, ref: "0339-use-postgres.md", want: "0339", ok: true},
		{name: "a relative link target", pad: 4, ref: "../adr/0339-use-postgres.md", want: "0339", ok: true},
		{name: "a link target with a fragment", pad: 4, ref: "0339-use-postgres.md#decision-outcome", want: "0339", ok: true},
		{name: "the first digit run is the identity", pad: 4, ref: "0007-use-http2.md", want: "0007", ok: true},
		{name: "a digit in a directory component is not the identity", pad: 4, ref: "docs/2024/0339-use-postgres.md", want: "0339", ok: true},
		{name: "a version-qualified reference", pad: 4, ref: "v2/0003", want: "0003", ok: true},
		{name: "a lowercase prefix", pad: 4, ref: "adr-339", want: "0339", ok: true},
		{name: "a spaced prefix", pad: 4, ref: "ADR 339", want: "0339", ok: true},
		{name: "surrounding whitespace", pad: 4, ref: "  339  ", want: "0339", ok: true},
		{name: "one", pad: 4, ref: "1", want: "0001", ok: true},
		{name: "zero", pad: 4, ref: "0", want: "0000", ok: true},
		{name: "more digits than the width are not truncated", pad: 4, ref: "12345", want: "12345", ok: true},
		{name: "width six pads to six", pad: 6, ref: "339", want: "000339", ok: true},
		{name: "width six on a kebab file name", pad: 6, ref: "0339-use-postgres", want: "000339", ok: true},
		{name: "width six leaves a six-digit reference alone", pad: 6, ref: "000001", want: "000001", ok: true},
		{name: "a zero width does not pad", pad: 0, ref: "339", want: "339", ok: true},
		{name: "a reference without digits", pad: 4, ref: "use-postgres"},
		{name: "a reference that names nothing", pad: 4, ref: "a-reference-without-digits"},
		{name: "an empty reference", pad: 4},
		{name: "whitespace only", pad: 4, ref: "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ADRNormalizer{Pad: tt.pad}.Normalize(tt.ref)

			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

func TestNormalizeIsOneIdentityPerSpelling(t *testing.T) {
	spellings := []string{"339", "ADR-339", "000339", "0339-use-postgres"}

	t.Run("every spelling of one reference is one node", func(t *testing.T) {
		norm := ADRNormalizer{Pad: 4}

		for _, ref := range spellings {
			got, ok := norm.Normalize(ref)
			if !ok {
				t.Fatalf("Normalize(%q) failed, want an identifier", ref)
			}
			if got != "0339" {
				t.Errorf("Normalize(%q) = %q, want 0339", ref, got)
			}
		}
	})

	t.Run("the display width follows the configuration", func(t *testing.T) {
		norm := ADRNormalizer{Pad: 6}

		for _, ref := range spellings {
			got, ok := norm.Normalize(ref)
			if !ok {
				t.Fatalf("Normalize(%q) failed, want an identifier", ref)
			}
			if got != "000339" {
				t.Errorf("Normalize(%q) = %q, want 000339", ref, got)
			}
		}
	})

	t.Run("renaming the title suffix does not change identity", func(t *testing.T) {
		norm := ADRNormalizer{Pad: 4}
		before, _ := norm.Normalize("0339-use-postgres.md")
		after, _ := norm.Normalize("0339-use-postgresql-instead.md")

		if before != after {
			t.Fatalf("identity changed with the title: %q vs %q", before, after)
		}
	})
}

func TestADRNormalizerMatchesFilename(t *testing.T) {
	tests := []struct {
		name string
		file string
		want bool
	}{
		{name: "four digits", file: "0001.md", want: true},
		{name: "three digits", file: "001.md", want: true},
		{name: "six digits", file: "000001.md", want: true},
		{name: "six digits at full width", file: "123456.md", want: true},
		{name: "digits with a kebab title", file: "0001-use-postgres.md", want: true},
		{name: "six digits with a kebab title", file: "000339-use-postgres.md", want: true},
		{name: "a title suffix may carry capitals", file: "0001-Use-Postgres.md", want: true},
		{name: "a title suffix may carry digits", file: "0007-use-http2.md", want: true},
		{name: "two digits is too narrow", file: "01.md"},
		{name: "seven digits is too wide", file: "1234567.md"},
		{name: "an index file", file: "README.md"},
		{name: "a prefixed name", file: "adr-0001.md"},
		{name: "an underscore separator", file: "0001_use_postgres.md"},
		{name: "another extension", file: "0001.txt"},
		{name: "a long extension", file: "0001-use-postgres.markdown"},
		{name: "no extension", file: "0001"},
		{name: "an empty name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (ADRNormalizer{Pad: 4}).MatchesFilename(tt.file); got != tt.want {
				t.Fatalf("MatchesFilename(%q) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}

func TestADRNormalizerWidth(t *testing.T) {
	if got := (ADRNormalizer{Pad: 6}).Width(); got != 6 {
		t.Fatalf("Width = %d, want 6", got)
	}
}

func TestConfigNormalizer(t *testing.T) {
	t.Run("the configured width reaches the normalizer", func(t *testing.T) {
		cfg := ADRPreset()
		cfg.IDWidth = 6
		norm := cfg.Normalizer()

		if got := norm.Width(); got != 6 {
			t.Errorf("Width = %d, want 6", got)
		}
		got, ok := norm.Normalize("339")
		if !ok || got != "000339" {
			t.Fatalf("Normalize(339) = %q (ok=%v), want 000339", got, ok)
		}
	})

	t.Run("the preset width is four", func(t *testing.T) {
		got, ok := ADRPreset().Normalizer().Normalize("339")

		if !ok || got != "0339" {
			t.Fatalf("Normalize(339) = %q (ok=%v), want 0339", got, ok)
		}
	})
}
