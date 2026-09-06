package graph

import (
	"slices"
	"testing"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/parse"
	"github.com/Kaikei-e/DocDag/model"
)

// TestPaddingMismatch is about the identity rules that do not pad. Where a
// kind's `id:` pattern is the canonical spelling, `UZ-V-11` and `UZ-V-011` are
// two identifiers and only one of them exists, so a reference of the wrong
// width is told what it meant instead of being reported as a name nobody wrote.
func TestPaddingMismatch(t *testing.T) {
	cfg := testKindsConfig()
	subject := func(supersedes any) []*parse.Document {
		return []*parse.Document{
			testKindDoc("clause", "UZ-V-011", map[string]any{"status": config.StatusSuperseded}),
			testKindDoc("clause", "UZ-V-012", map[string]any{"status": config.StatusAccepted, "supersedes": supersedes}),
		}
	}

	t.Run("a reference of the wrong width names the document it meant", func(t *testing.T) {
		g := Build(subject([]any{"UZ-V-11"}), cfg)

		f := testAssertSingleFinding(t, g.Findings, model.RulePaddingMismatch, model.SeverityError, "UZ-V-012")
		want := `supersedes reference "UZ-V-11" does not name a document; did you mean "UZ-V-011"?`
		if f.Detail != want {
			t.Errorf("detail = %q, want %q", f.Detail, want)
		}
		if want := "write supersedes: UZ-V-011"; f.Fix != want {
			t.Errorf("fix = %q, want %q", f.Fix, want)
		}
		if len(g.Edges) != 0 {
			t.Errorf("edges = %+v, want none: the document the author did not name must not be linked", g.Edges)
		}
	})

	t.Run("a wikilink of the wrong width is unwrapped first", func(t *testing.T) {
		g := Build(subject([]any{"[[UZ-V-11]]"}), cfg)

		testAssertSingleFinding(t, g.Findings, model.RulePaddingMismatch, model.SeverityError, "UZ-V-012")
	})

	t.Run("a reference that names a document is left alone", func(t *testing.T) {
		g := Build(subject([]any{"UZ-V-011"}), cfg)

		if len(g.Findings) != 0 {
			t.Fatalf("findings = %+v, want none", g.Findings)
		}
		if want := []model.Edge{testEdge("UZ-V-012", "UZ-V-011", "supersedes")}; !slices.EqualFunc(g.Edges, want, model.Edge.Equal) {
			t.Fatalf("edges = %+v, want %+v", g.Edges, want)
		}
	})

	t.Run("a reference no width of which names a document stays what it was", func(t *testing.T) {
		g := Build(subject([]any{"UZ-V-99"}), cfg)

		testAssertSingleFinding(t, g.Findings, model.RuleInvalidRef, model.SeverityError, "UZ-V-012")
	})

	t.Run("a width two documents both answer to suggests nothing", func(t *testing.T) {
		wide := testKindsConfig()
		wide.Kinds["clause"] = config.KindSpec{Dir: "spec/clauses", ID: `^UZ-V-\d{1,4}$`, Closed: true}
		docs := []*parse.Document{
			testKindDoc("clause", "UZ-V-011", map[string]any{"status": config.StatusAccepted}),
			testKindDoc("clause", "UZ-V-11", map[string]any{"status": config.StatusAccepted}),
			testKindDoc("clause", "UZ-V-012", map[string]any{"status": config.StatusAccepted, "supersedes": []any{"UZ-V-0011"}}),
		}

		g := Build(docs, wide)

		if got := testFindingsFor(g.Findings, model.RulePaddingMismatch); len(got) != 0 {
			t.Fatalf("findings = %+v, want none: two documents answer to that width and neither is the answer", got)
		}
	})
}

// TestPaddingIsIdentityUnderTheDigitRunRules holds the line the README draws:
// where identity is the digit run, `11`, `0011` and `ADR-11` are one document
// and always were, so nothing is reported about a width there.
func TestPaddingIsIdentityUnderTheDigitRunRules(t *testing.T) {
	cfg := config.ADRPreset()
	for _, ref := range []string{"11", "0011", "000011", "ADR-11"} {
		t.Run(ref, func(t *testing.T) {
			docs := []*parse.Document{
				testDoc("0011", map[string]any{"status": config.StatusSuperseded}, ""),
				testDoc("0012", map[string]any{"status": config.StatusAccepted, "supersedes": []any{ref}}, ""),
			}

			g := Build(docs, cfg)

			if len(g.Findings) != 0 {
				t.Fatalf("findings = %+v, want none", g.Findings)
			}
			if want := []model.Edge{testEdge("0012", "0011", "supersedes")}; !slices.EqualFunc(g.Edges, want, model.Edge.Equal) {
				t.Fatalf("edges = %+v, want %+v", g.Edges, want)
			}
		})
	}
}

func TestUnpadded(t *testing.T) {
	tests := []struct{ token, want string }{
		{token: "0011", want: "11"},
		{token: "11", want: "11"},
		{token: "UZ-V-011", want: "UZ-V-11"},
		{token: "0000", want: "0"},
		{token: "conform/tls-version", want: "conform/tls-version"},
		{token: "", want: ""},
	}
	for _, tt := range tests {
		if got := unpadded(tt.token); got != tt.want {
			t.Errorf("unpadded(%q) = %q, want %q", tt.token, got, tt.want)
		}
	}
}
