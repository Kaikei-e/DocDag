package graph

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/parse"
	"github.com/Kaikei-e/DocDag/model"
)

func retractDoc(id string, fm map[string]any, body string) *parse.Document {
	keyLines := make(map[string]int, len(fm))
	line := 2
	for k := range fm {
		keyLines[k] = line
		line++
	}
	return &parse.Document{
		ID:              model.ID(id),
		Path:            "docs/adr/" + id + ".md",
		Frontmatter:     fm,
		Body:            body,
		FrontmatterLine: 1,
		KeyLines:        keyLines,
	}
}

func TestApplyRetractions(t *testing.T) {
	t.Run("accepted record retracts a structured edge", func(t *testing.T) {
		cfg := config.ADRPreset()
		docs := []*parse.Document{
			retractDoc("0001", map[string]any{
				"title":      "New",
				"status":     "accepted",
				"supersedes": []any{"0002", "0003"},
			}, ""),
			retractDoc("0002", map[string]any{"title": "Unrelated", "status": "accepted"}, ""),
			retractDoc("0003", map[string]any{"title": "Replaced", "status": "superseded"}, ""),
			retractDoc("0004", map[string]any{
				"title":  "Correction",
				"status": "accepted",
				"retracts": []any{
					map[string]any{"from": "0001", "edge": "supersedes", "to": "0002"},
				},
			}, ""),
		}

		g := Build(docs, cfg)

		if len(g.Edges) != 1 || g.Edges[0].From != "0001" || g.Edges[0].To != "0003" {
			t.Fatalf("Edges = %+v, want only 0001 -> 0003", g.Edges)
		}
		wantRetractions := []model.Retraction{{From: "0001", To: "0002", Type: config.EdgeSupersedes, By: "0004"}}
		if !slices.Equal(g.Retractions, wantRetractions) {
			t.Fatalf("Retractions = %+v, want %+v", g.Retractions, wantRetractions)
		}
	})

	t.Run("accepted record retracts a derived edge for the same pair", func(t *testing.T) {
		cfg := config.ADRPreset()
		docs := []*parse.Document{
			retractDoc("0001", map[string]any{"title": "Bulk", "status": "accepted"}, ""),
			retractDoc("0002", map[string]any{"title": "Target", "status": "superseded by 0001"}, ""),
			retractDoc("0003", map[string]any{
				"title":  "Correction",
				"status": "accepted",
				"retracts": []any{
					map[string]any{"from": "0001", "edge": "supersedes", "to": "0002"},
				},
			}, ""),
		}

		g := Build(docs, cfg)

		if len(g.Edges) != 0 {
			t.Fatalf("Edges = %+v, want derived edge dropped by retraction", g.Edges)
		}
		if len(g.Retractions) != 1 {
			t.Fatalf("Retractions = %+v, want 1", g.Retractions)
		}
	})

	t.Run("proposed and withdrawn records have no effect while superseded still holds", func(t *testing.T) {
		cfg := config.ADRPreset()
		for _, tc := range []struct {
			name   string
			status string
			want   int
		}{
			{name: "proposed", status: "proposed", want: 1},
			{name: "withdrawn", status: "withdrawn", want: 1},
			{name: "superseded", status: "superseded", want: 0},
		} {
			t.Run(tc.name, func(t *testing.T) {
				docs := []*parse.Document{
					retractDoc("0001", map[string]any{
						"title":      "New",
						"status":     "accepted",
						"supersedes": []any{"0002"},
					}, ""),
					retractDoc("0002", map[string]any{"title": "Old", "status": "accepted"}, ""),
					retractDoc("0003", map[string]any{
						"title":  "Correction",
						"status": tc.status,
						"retracts": []any{
							map[string]any{"from": "0001", "edge": "supersedes", "to": "0002"},
						},
					}, ""),
				}

				g := Build(docs, cfg)

				if len(g.Edges) != tc.want {
					t.Fatalf("Edges = %+v, want %d edge(s)", g.Edges, tc.want)
				}
				if got := len(g.Retractions); (tc.want == 0) != (got == 1) {
					t.Fatalf("Retractions = %+v, want effective=%v", g.Retractions, tc.want == 0)
				}
			})
		}
	})

	t.Run("respects custom status_field", func(t *testing.T) {
		cfg := config.ADRPreset()
		cfg.StatusField = "state"
		docs := []*parse.Document{
			retractDoc("0001", map[string]any{"title": "New", "state": "accepted", "supersedes": []any{"0002"}}, ""),
			retractDoc("0002", map[string]any{"title": "Old", "state": "accepted"}, ""),
			retractDoc("0003", map[string]any{
				"title": "Correction",
				"state": "accepted",
				"retracts": []any{
					map[string]any{"from": "0001", "edge": "supersedes", "to": "0002"},
				},
			}, ""),
		}

		g := Build(docs, cfg)

		if len(g.Edges) != 0 {
			t.Fatalf("Edges = %+v, want retracted using custom status_field", g.Edges)
		}
	})
}

func TestRetractionClearsDownstreamChecksAndPreservesBinding(t *testing.T) {
	cfg := config.ADRPreset()
	cfg.Edges[0].Inverse = "superseded_by"

	makeDocs := func(withRetraction bool) []*parse.Document {
		docs := []*parse.Document{
			retractDoc("0001", map[string]any{
				"title":      "Bulk edit",
				"status":     "accepted",
				"date":       "2026-01-01",
				"supersedes": []any{"0002"},
			}, ""),
			retractDoc("0002", map[string]any{
				"title":         "Still in force",
				"status":        "accepted",
				"date":          "2025-12-01",
				"superseded_by": []any{"0001"},
			}, "Superseded by [[0001]]\n"),
		}
		if withRetraction {
			docs = append(docs, retractDoc("0003", map[string]any{
				"title":  "Retract 0001->0002",
				"status": "accepted",
				"date":   "2026-02-01",
				"retracts": []any{
					map[string]any{"from": "0001", "edge": "supersedes", "to": "0002"},
				},
			}, ""))
		}
		return docs
	}

	t.Run("without retraction status_drift fires", func(t *testing.T) {
		g := Build(makeDocs(false), cfg)
		findings := Validate(g, cfg, time.Time{})
		var driftCount int
		for _, f := range findings {
			if f.Rule == model.RuleStatusDrift {
				driftCount++
			}
		}
		if driftCount == 0 {
			t.Fatalf("findings = %+v, want status_drift without retraction", findings)
		}
	})

	t.Run("with retraction no status_drift or inverse_mismatch, resolve answers self, and 0002 stays binding", func(t *testing.T) {
		g := Build(makeDocs(true), cfg)
		findings := Validate(g, cfg, time.Time{})
		for _, f := range findings {
			if f.Rule == model.RuleStatusDrift || f.Rule == model.RuleInverseMismatch || f.Rule == model.RuleInvalidRetraction {
				t.Errorf("unexpected finding = %+v", f)
			}
		}

		resolved, err := Resolve(g, "0002", config.EdgeSupersedes)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if len(resolved) != 1 || resolved[0] != "0002" {
			t.Errorf("Resolve(0002) = %+v, want [0002]", resolved)
		}
		if !slices.Contains(BindingSet(g, cfg, time.Time{}), model.ID("0002")) {
			t.Errorf("BindingSet does not contain 0002")
		}
	})
}

func TestInvalidRetractionFindings(t *testing.T) {
	cfg := config.ADRPreset()

	baseDocs := func(retractsVal any) []*parse.Document {
		return []*parse.Document{
			retractDoc("0001", map[string]any{
				"title":      "One",
				"status":     "accepted",
				"date":       "2026-01-01",
				"supersedes": []any{"0002"},
			}, ""),
			retractDoc("0002", map[string]any{
				"title":  "Two",
				"status": "superseded",
				"date":   "2026-01-01",
			}, ""),
			retractDoc("0003", map[string]any{
				"title":    "Correction",
				"status":   "proposed",
				"date":     "2026-01-02",
				"retracts": retractsVal,
			}, ""),
		}
	}

	tests := []struct {
		name       string
		retracts   any
		wantDetail string
	}{
		{name: "not a list", retracts: "0001", wantDetail: "list of mappings"},
		{name: "nil value", retracts: nil, wantDetail: "list of mappings"},
		{name: "entry not a mapping", retracts: []any{"0001"}, wantDetail: "must be a mapping"},
		{name: "missing from", retracts: []any{map[string]any{"edge": "supersedes", "to": "0002"}}, wantDetail: `missing required key "from"`},
		{name: "missing edge", retracts: []any{map[string]any{"from": "0001", "to": "0002"}}, wantDetail: `missing required key "edge"`},
		{name: "missing to", retracts: []any{map[string]any{"from": "0001", "edge": "supersedes"}}, wantDetail: `missing required key "to"`},
		{name: "unexpected key", retracts: []any{map[string]any{"from": "0001", "edge": "supersedes", "to": "0002", "why": "oops"}}, wantDetail: `unexpected key "why"`},
		{name: "non-scalar value", retracts: []any{map[string]any{"from": []any{"0001"}, "edge": "supersedes", "to": "0002"}}, wantDetail: "non-empty string"},
		{name: "empty value", retracts: []any{map[string]any{"from": "0001", "edge": "supersedes", "to": "   "}}, wantDetail: "non-empty string"},
		{name: "undeclared edge", retracts: []any{map[string]any{"from": "0001", "edge": "replaces", "to": "0002"}}, wantDetail: `unknown edge "replaces"`},
		{name: "from not id-shaped", retracts: []any{map[string]any{"from": "see 0001", "edge": "supersedes", "to": "0002"}}, wantDetail: "not an identifier"},
		{name: "to not existing doc", retracts: []any{map[string]any{"from": "0001", "edge": "supersedes", "to": "0099"}}, wantDetail: "names no document"},
		{name: "from not existing doc", retracts: []any{map[string]any{"from": "0099", "edge": "supersedes", "to": "0002"}}, wantDetail: "names no document"},
		{name: "edge does not exist between documents", retracts: []any{map[string]any{"from": "0002", "edge": "supersedes", "to": "0001"}}, wantDetail: "supersedes 0002 -> 0001, but 0002 declares no such edge"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs := baseDocs(tt.retracts)
			g := Build(docs, cfg)

			var got []model.Finding
			for _, f := range g.Findings {
				if f.Rule == model.RuleInvalidRetraction {
					got = append(got, f)
				}
			}
			if len(got) != 1 {
				t.Fatalf("invalid_retraction findings = %+v, want 1", got)
			}
			if got[0].ID != "0003" {
				t.Errorf("finding ID = %q, want 0003", got[0].ID)
			}
			if got[0].Severity != model.SeverityError {
				t.Errorf("severity = %q, want error", got[0].Severity)
			}
			if got[0].Location.Line != docs[2].KeyLines[config.KeyRetracts] {
				t.Errorf("line = %d, want retracts key line %d", got[0].Location.Line, docs[2].KeyLines[config.KeyRetracts])
			}
			if !strings.Contains(got[0].Detail, tt.wantDetail) {
				t.Errorf("detail = %q, want it to contain %q", got[0].Detail, tt.wantDetail)
			}
		})
	}

	t.Run("valid retraction produces no finding", func(t *testing.T) {
		g := Build(baseDocs([]any{map[string]any{"from": "[[0001]]", "edge": "supersedes", "to": "0002"}}), cfg)
		for _, f := range g.Findings {
			if f.Rule == model.RuleInvalidRetraction {
				t.Fatalf("unexpected finding = %+v", f)
			}
		}
	})

	t.Run("closed kind accepts retracts without unknown_field", func(t *testing.T) {
		closedCfg := config.ADRPreset()
		closedCfg.Kinds = map[string]config.KindSpec{
			"clause": {
				Dir:    "spec/clauses",
				Closed: true,
				ID:     `^UZ-\d{4}$`,
			},
		}
		closedCfg.Edges = []config.EdgeSpec{
			{Name: "supersedes", Key: "supersedes", Acyclic: true, Direction: config.DirectionForward},
		}
		docs := []*parse.Document{
			{
				ID:       "UZ-0001",
				Kind:     "clause",
				Path:     "spec/clauses/UZ-0001.md",
				KeyLines: map[string]int{"kind": 2, "status": 3, "retracts": 4},
				Frontmatter: map[string]any{
					"kind":   "clause",
					"status": "accepted",
					"retracts": []any{
						map[string]any{"from": "UZ-0001", "edge": "supersedes", "to": "UZ-0001"},
					},
				},
			},
		}
		for _, f := range CheckDocuments(docs, closedCfg) {
			if f.Rule == model.RuleUnknownField {
				t.Fatalf("unexpected unknown_field for retracts: %+v", f)
			}
		}
	})
}
