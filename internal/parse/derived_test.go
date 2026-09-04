package parse

import (
	"slices"
	"testing"

	"github.com/Kaikei-e/DocDag/config"
)

func TestMatchDerived(t *testing.T) {
	spec := config.ADRPreset().DerivedEdges[0]
	tests := []struct {
		name  string
		value string
		want  string
		ok    bool
	}{
		{name: "the MADR spelling", value: "superseded by 0003", want: "0003", ok: true},
		{name: "the match is case-insensitive", value: "Superseded By 0003", want: "0003", ok: true},
		{name: "the hyphenated spelling", value: "superseded-by 0003", want: "0003", ok: true},
		{name: "the fully hyphenated spelling", value: "superseded-by-0003", want: "0003", ok: true},
		{name: "a prefixed reference is captured as written", value: "SUPERSEDED-BY ADR-0003", want: "ADR-0003", ok: true},
		{name: "only the first token is the reference", value: "superseded by 0003 (see also 0004)", want: "0003", ok: true},
		{name: "a plain status does not match", value: "superseded"},
		{name: "another status does not match", value: "accepted"},
		{name: "an empty value does not match"},
		{name: "the pattern is anchored at the start", value: "originally superseded by 0003"},
		{name: "a missing reference does not match", value: "superseded by"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := MatchDerived(tt.value, spec)

			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("MatchDerived(%q) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}

	t.Run("a pattern that does not compile matches nothing", func(t *testing.T) {
		broken := config.DerivedEdgeSpec{Field: "status", Pattern: "([0-9", Edge: "supersedes", Direction: config.DirectionReverse}

		if got, ok := MatchDerived("superseded by 0003", broken); ok {
			t.Fatalf("MatchDerived = %q, %v, want no match", got, ok)
		}
	})

	t.Run("a pattern without a capture group matches nothing", func(t *testing.T) {
		noGroup := config.DerivedEdgeSpec{Field: "status", Pattern: "(?i)^superseded", Edge: "supersedes", Direction: config.DirectionReverse}

		if got, ok := MatchDerived("superseded by 0003", noGroup); ok {
			t.Fatalf("MatchDerived = %q, %v, want no match", got, ok)
		}
	})
}

func TestDerived(t *testing.T) {
	cfg := config.ADRPreset()
	spec := cfg.DerivedEdges[0]

	t.Run("a MADR status string yields one derived edge", func(t *testing.T) {
		doc := testDoc(map[string]any{"title": "Store thumbnails on the local disk", "status": "superseded by 0003"})
		want := []DerivedEdge{{Spec: spec, Field: "status", Value: "superseded by 0003", Target: "0003"}}

		if got := Derived(doc, cfg); !slices.Equal(got, want) {
			t.Fatalf("Derived = %+v, want %+v", got, want)
		}
	})

	t.Run("the derived edge runs from the referenced document to the containing one", func(t *testing.T) {
		doc := testDoc(map[string]any{"status": "superseded by 0003"})

		got := Derived(doc, cfg)
		if len(got) != 1 {
			t.Fatalf("Derived = %+v, want one edge", got)
		}
		if got[0].Spec.Direction != config.DirectionReverse {
			t.Errorf("direction = %q, want %q (the referenced document is the new one)", got[0].Spec.Direction, config.DirectionReverse)
		}
		if got[0].Spec.Edge != config.EdgeSupersedes.String() {
			t.Errorf("edge = %q, want %q", got[0].Spec.Edge, config.EdgeSupersedes)
		}
	})

	t.Run("the hyphenated spelling derives too", func(t *testing.T) {
		doc := testDoc(map[string]any{"status": "Superseded-by 0003"})
		want := []DerivedEdge{{Spec: spec, Field: "status", Value: "Superseded-by 0003", Target: "0003"}}

		if got := Derived(doc, cfg); !slices.Equal(got, want) {
			t.Fatalf("Derived = %+v, want %+v", got, want)
		}
	})

	t.Run("an ordinary status derives nothing", func(t *testing.T) {
		doc := testDoc(map[string]any{"status": "accepted"})

		if got := Derived(doc, cfg); len(got) != 0 {
			t.Fatalf("Derived = %+v, want none", got)
		}
	})

	t.Run("a document without frontmatter derives nothing", func(t *testing.T) {
		doc := testDoc(nil)

		if got := Derived(doc, cfg); len(got) != 0 {
			t.Fatalf("Derived = %+v, want none", got)
		}
	})

	t.Run("a configuration without derived edges derives nothing", func(t *testing.T) {
		plain := config.ADRPreset()
		plain.DerivedEdges = nil
		doc := testDoc(map[string]any{"status": "superseded by 0003"})

		if got := Derived(doc, plain); len(got) != 0 {
			t.Fatalf("Derived = %+v, want none", got)
		}
	})

	t.Run("every configured pattern is applied in declaration order", func(t *testing.T) {
		replaces := config.DerivedEdgeSpec{
			Field:     "note",
			Pattern:   `(?i)^replaces\s+(\S+)`,
			Edge:      config.EdgeSupersedes.String(),
			Direction: config.DirectionForward,
		}
		twoSpecs := config.ADRPreset()
		twoSpecs.DerivedEdges = append(twoSpecs.DerivedEdges, replaces)
		doc := testDoc(map[string]any{"status": "superseded by 0003", "note": "replaces 0001"})
		want := []DerivedEdge{
			{Spec: spec, Field: "status", Value: "superseded by 0003", Target: "0003"},
			{Spec: replaces, Field: "note", Value: "replaces 0001", Target: "0001"},
		}

		if got := Derived(doc, twoSpecs); !slices.Equal(got, want) {
			t.Fatalf("Derived = %+v, want %+v", got, want)
		}
	})

	t.Run("a field the document does not carry derives nothing", func(t *testing.T) {
		other := config.ADRPreset()
		other.DerivedEdges = []config.DerivedEdgeSpec{{
			Field:     "note",
			Pattern:   `(?i)^replaces\s+(\S+)`,
			Edge:      config.EdgeSupersedes.String(),
			Direction: config.DirectionForward,
		}}
		doc := testDoc(map[string]any{"status": "superseded by 0003"})

		if got := Derived(doc, other); len(got) != 0 {
			t.Fatalf("Derived = %+v, want none", got)
		}
	})
}
