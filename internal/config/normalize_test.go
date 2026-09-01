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

func TestIDShaped(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want bool
	}{
		{name: "a padded identifier", ref: "0042", want: true},
		{name: "a bare number", ref: "42", want: true},
		{name: "a prefixed identifier", ref: "ADR-0042", want: true},
		{name: "a lowercase prefix", ref: "adr0042", want: true},
		{name: "a file name", ref: "0042-title.md", want: true},
		{name: "a file stem", ref: "0042-title", want: true},
		{name: "prose around a reference", ref: "see 0042"},
		{name: "a reference in a sentence", ref: "0042 and 0043"},
		{name: "a wikilink", ref: "[[0042]]", want: true},
		{name: "an aliased wikilink", ref: "[[0042|the caching decision]]", want: true},
		{name: "a slug that opens with digits", ref: "3days-recap"},
		{name: "a dotted name", ref: "tool.uv.index"},
		{name: "a word", ref: "upstream"},
		{name: "a path", ref: "docs/adr/0042.md"},
		{name: "a wikilink around a word", ref: "[[upstream]]"},
		{name: "an unclosed wikilink", ref: "[[0042"},
		{name: "an empty reference"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IDShaped(tt.ref); got != tt.want {
				t.Fatalf("IDShaped(%q) = %v, want %v", tt.ref, got, tt.want)
			}
		})
	}
}

func TestIsDocumentLink(t *testing.T) {
	tests := []struct {
		name   string
		target string
		want   bool
	}{
		{name: "a managed file name", target: "0042-title.md", want: true},
		{name: "a bare managed file name", target: "000042.md", want: true},
		{name: "a dot-relative target", target: "./0042-title.md", want: true},
		{name: "a parent-relative target", target: "../adr/0042-title.md", want: true},
		{name: "a title suffix with any punctuation", target: "0042-title_v2.md", want: true},
		{name: "a narrow digit run", target: "42-title.md"},
		{name: "a wide digit run", target: "1234567-title.md"},
		{name: "an unmanaged file", target: "README.md"},
		{name: "a slug that opens with digits", target: "3days-recap.md"},
		{name: "an empty target"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsDocumentLink(tt.target); got != tt.want {
				t.Fatalf("IsDocumentLink(%q) = %v, want %v", tt.target, got, tt.want)
			}
		})
	}
}

func TestConfigIsReference(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		target  string
		want    bool
	}{
		{name: "a padded identifier", target: "000042", want: true},
		{name: "a four digit identifier", target: "0042", want: true},
		{name: "a three digit identifier", target: "042", want: true},
		{name: "a prefixed identifier", target: "ADR-0042", want: true},
		{name: "an unhyphenated prefix", target: "adr0042", want: true},
		{name: "a single digit is not wide enough", target: "1"},
		{name: "a file stem carrying a title", target: "0042-title"},
		{name: "a slug that opens with digits", target: "3days-recap"},
		{name: "a word", target: "upstream"},
		{name: "a dotted name", target: "tool.uv.index"},
		{name: "prose", target: "see 0042"},
		{name: "an empty target"},
		{name: "a widened pattern accepts a title", pattern: `^(\d{3,6})(?:-.*)?$`, target: "0042-title", want: true},
		{name: "a widened pattern still rejects a word", pattern: `^(\d{3,6})(?:-.*)?$`, target: "upstream"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := ADRPreset()
			cfg.References.Pattern = tt.pattern
			if got := cfg.IsReference(tt.target); got != tt.want {
				t.Fatalf("IsReference(%q) = %v, want %v", tt.target, got, tt.want)
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

func TestPatternNormalizer(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		ref     string
		want    model.ID
		ok      bool
	}{
		{name: "a prefixed identifier", pattern: `^UZ-[A-Z]-\d{3}$`, ref: "UZ-V-001", want: "UZ-V-001", ok: true},
		{name: "surrounding whitespace", pattern: `^UZ-[A-Z]-\d{3}$`, ref: "  UZ-V-001  ", want: "UZ-V-001", ok: true},
		{name: "a wikilink", pattern: `^UZ-[A-Z]-\d{3}$`, ref: "[[UZ-V-001]]", want: "UZ-V-001", ok: true},
		{name: "an aliased wikilink", pattern: `^UZ-[A-Z]-\d{3}$`, ref: "[[UZ-V-001|the evidence clause]]", want: "UZ-V-001", ok: true},
		{name: "an identifier carrying a slash", pattern: `^conform/[a-z0-9-]+$`, ref: "conform/uz-v-001", want: "conform/uz-v-001", ok: true},
		{name: "the pattern is not padded onto a width", pattern: `^dev-\d{4}$`, ref: "dev-0007", want: "dev-0007", ok: true},
		{name: "prose around an identifier", pattern: `^UZ-[A-Z]-\d{3}$`, ref: "see UZ-V-001"},
		{name: "an unanchored pattern still reads the whole reference", pattern: `UZ-[A-Z]-\d{3}`, ref: "see UZ-V-001"},
		{name: "a file name is not an identifier", pattern: `^UZ-[A-Z]-\d{3}$`, ref: "UZ-V-001.md"},
		{name: "a directory prefix is not stripped", pattern: `^UZ-[A-Z]-\d{3}$`, ref: "spec/clauses/UZ-V-001"},
		{name: "another kind's identifier", pattern: `^UZ-[A-Z]-\d{3}$`, ref: "dev-0007"},
		{name: "an empty reference", pattern: `^UZ-[A-Z]-\d{3}$`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, err := IDPattern(tt.pattern)
			if err != nil {
				t.Fatalf("IDPattern(%q): %v", tt.pattern, err)
			}

			got, ok := PatternNormalizer{Pattern: pattern}.Normalize(tt.ref)

			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("Normalize(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

func TestPatternNormalizerMatchesFilename(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		file    string
		want    bool
	}{
		{name: "a stem that is an identifier", pattern: `^UZ-[A-Z]-\d{3}$`, file: "UZ-V-001.md", want: true},
		{name: "a stem carrying a title", pattern: `^UZ-[A-Z]-\d{3}$`, file: "UZ-V-001-evidence.md"},
		{name: "another extension", pattern: `^UZ-[A-Z]-\d{3}$`, file: "UZ-V-001.txt"},
		{name: "an index file", pattern: `^UZ-[A-Z]-\d{3}$`, file: "README.md"},
		// No file name can carry a slash, so a kind whose identifiers do takes
		// its identity from the frontmatter instead.
		{name: "a pattern no file name can satisfy", pattern: `^conform/[a-z0-9-]+$`, file: "uz-v-001.md"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pattern, err := IDPattern(tt.pattern)
			if err != nil {
				t.Fatalf("IDPattern(%q): %v", tt.pattern, err)
			}

			if got := (PatternNormalizer{Pattern: pattern}).MatchesFilename(tt.file); got != tt.want {
				t.Fatalf("MatchesFilename(%q) = %v, want %v", tt.file, got, tt.want)
			}
		})
	}
}

func TestConfigKindNormalizer(t *testing.T) {
	cfg := ADRPreset()
	cfg.Kinds = testKinds()

	t.Run("a kind with an id pattern reads that pattern", func(t *testing.T) {
		got, ok := cfg.KindNormalizer("clause").Normalize("UZ-V-001")

		if !ok || got != "UZ-V-001" {
			t.Fatalf("Normalize(UZ-V-001) = %q (ok=%v), want the identifier itself", got, ok)
		}
	})

	t.Run("a kind with an id pattern rejects a digit run", func(t *testing.T) {
		if got, ok := cfg.KindNormalizer("clause").Normalize("339"); ok {
			t.Fatalf("Normalize(339) = %q, want no identifier under a declared pattern", got)
		}
	})

	t.Run("a kind without one keeps the digit-run rules", func(t *testing.T) {
		got, ok := cfg.KindNormalizer("pm").Normalize("ADR-339")

		if !ok || got != "0339" {
			t.Fatalf("Normalize(ADR-339) = %q (ok=%v), want the padded digit run", got, ok)
		}
	})
}

func TestConfigNormalizerAcrossKinds(t *testing.T) {
	cfg := ADRPreset()
	cfg.Kinds = testKinds()

	t.Run("every kind's identifiers resolve", func(t *testing.T) {
		norm := cfg.Normalizer()
		for ref, want := range map[string]model.ID{
			"UZ-V-001":         "UZ-V-001",
			"conform/uz-v-001": "conform/uz-v-001",
			"339":              "0339",
		} {
			got, ok := norm.Normalize(ref)
			if !ok || got != want {
				t.Errorf("Normalize(%q) = %q (ok=%v), want %q", ref, got, ok, want)
			}
		}
	})

	t.Run("a reference no kind accepts resolves to nothing", func(t *testing.T) {
		if got, ok := cfg.Normalizer().Normalize("uz-v-001"); ok {
			t.Fatalf("Normalize(uz-v-001) = %q, want no identifier", got)
		}
	})

	t.Run("overlapping kinds are tried in sorted name order", func(t *testing.T) {
		// Both kinds accept the same reference, so the answer has to come from
		// the configuration's order rather than from a map iteration.
		overlapping := ADRPreset()
		overlapping.Kinds = map[string]KindSpec{
			"beta":  {Dir: "beta", ID: `^X-\d+$`},
			"alpha": {Dir: "alpha", ID: `^X-\d+$`},
		}
		norm, ok := overlapping.Normalizer().(UnionNormalizer)
		if !ok {
			t.Fatalf("Normalizer = %T, want a union over the kinds", overlapping.Normalizer())
		}

		if kind, ok := norm.Kind("X-7"); !ok || kind != "alpha" {
			t.Fatalf("Kind(X-7) = %q (ok=%v), want the first kind in sorted order", kind, ok)
		}
	})

	t.Run("an edge resolves its references against the kinds it points at first", func(t *testing.T) {
		overlapping := ADRPreset()
		overlapping.Kinds = map[string]KindSpec{
			"beta":  {Dir: "beta", ID: `^X-\d+$`},
			"alpha": {Dir: "alpha", ID: `^X-\d+$`},
		}
		spec := EdgeSpec{Name: "enforces", Key: "enforces", Direction: DirectionForward, To: []string{"beta"}}

		norm, ok := overlapping.EdgeNormalizer(spec).(UnionNormalizer)
		if !ok {
			t.Fatalf("EdgeNormalizer = %T, want a union over the kinds", overlapping.EdgeNormalizer(spec))
		}
		if kind, ok := norm.Kind("X-7"); !ok || kind != "beta" {
			t.Fatalf("Kind(X-7) = %q (ok=%v), want the kind the edge points at", kind, ok)
		}
	})

	t.Run("a reverse edge prefers the kinds its key names", func(t *testing.T) {
		// A reverse key names the source of the edge it declares, so the kinds
		// a reference under it may have are the edge's from kinds.
		overlapping := ADRPreset()
		overlapping.Kinds = map[string]KindSpec{
			"beta":  {Dir: "beta", ID: `^X-\d+$`},
			"alpha": {Dir: "alpha", ID: `^X-\d+$`},
		}
		spec := EdgeSpec{Name: "supersedes", Key: "superseded-by", Direction: DirectionReverse, From: []string{"beta"}, To: []string{"alpha"}}

		norm, ok := overlapping.EdgeNormalizer(spec).(UnionNormalizer)
		if !ok {
			t.Fatalf("EdgeNormalizer = %T, want a union over the kinds", overlapping.EdgeNormalizer(spec))
		}
		if kind, ok := norm.Kind("X-7"); !ok || kind != "beta" {
			t.Fatalf("Kind(X-7) = %q (ok=%v), want the kind the reverse key names", kind, ok)
		}
	})

	t.Run("a corpus without kinds keeps the digit-run normalizer", func(t *testing.T) {
		norm := ADRPreset().EdgeNormalizer(EdgeSpec{Name: "supersedes", Key: "supersedes", Direction: DirectionForward})

		if got, ok := norm.Normalize("339"); !ok || got != "0339" {
			t.Fatalf("Normalize(339) = %q (ok=%v), want 0339", got, ok)
		}
	})
}

func TestConfigIDShaped(t *testing.T) {
	t.Run("a corpus without kinds asks the digit-run rules", func(t *testing.T) {
		cfg := ADRPreset()

		if !cfg.IDShaped("ADR-0042") || cfg.IDShaped("UZ-V-001") {
			t.Fatal("IDShaped disagrees with the single-kind rules")
		}
	})

	t.Run("a corpus with kinds asks every kind", func(t *testing.T) {
		cfg := ADRPreset()
		cfg.Kinds = testKinds()

		for _, ref := range []string{"UZ-V-001", "conform/uz-v-001", "ADR-339"} {
			if !cfg.IDShaped(ref) {
				t.Errorf("IDShaped(%q) = false, want a kind to accept it", ref)
			}
		}
		for _, ref := range []string{"see UZ-V-001", "uz-v-001", ""} {
			if cfg.IDShaped(ref) {
				t.Errorf("IDShaped(%q) = true, want no kind to accept it", ref)
			}
		}
	})
}

func TestIDPatternRejectsWhatDoesNotCompile(t *testing.T) {
	if _, err := IDPattern("^UZ-([A-Z]$"); err == nil {
		t.Fatal("IDPattern accepted a pattern that does not compile")
	}
}

func TestDigitRunNormalizerInsideAUnion(t *testing.T) {
	// A kind that declares no pattern reads the digit run, but only out of a
	// reference that is wholly an identity: inside a union it would otherwise
	// claim every reference the pattern kinds rejected.
	norm := DigitRunNormalizer{ADRNormalizer{Pad: 4}}

	got, ok := norm.Normalize("ADR-339")
	if !ok || got != "0339" {
		t.Fatalf("Normalize(ADR-339) = %q (ok=%v), want 0339", got, ok)
	}
	for _, ref := range []string{"see 0042", "uz-v-001", "0042 and 0043"} {
		if id, ok := norm.Normalize(ref); ok {
			t.Errorf("Normalize(%q) = %q, want no identifier", ref, id)
		}
	}
}
