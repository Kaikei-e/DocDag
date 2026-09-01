package graph

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

func TestBuildNodes(t *testing.T) {
	cfg := config.ADRPreset()

	t.Run("nodes carry the recognized frontmatter attributes", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0001", map[string]any{
				"title":  "Store events in an append-only table",
				"status": "accepted",
				"date":   "2025-01-02",
			}, ""),
		}

		g := Build(docs, cfg)
		n, ok := g.Nodes["0001"]
		if !ok {
			t.Fatalf("node 0001 is missing: %v", g.Nodes)
		}
		if n.Title != "Store events in an append-only table" {
			t.Errorf("title = %q", n.Title)
		}
		if n.Status != "accepted" {
			t.Errorf("status = %q, want accepted", n.Status)
		}
		if n.Date != "2025-01-02" {
			t.Errorf("date = %q, want 2025-01-02", n.Date)
		}
		if n.Path != "0001.md" {
			t.Errorf("path = %q, want 0001.md", n.Path)
		}
	})

	t.Run("a document with invalid frontmatter is still a node", func(t *testing.T) {
		docs := []*parse.Document{
			{
				Path:           "0001.md",
				Name:           "0001.md",
				ID:             "0001",
				HasFrontmatter: true,
				MatchesPattern: true,
				Err:            errInvalidFixtureFrontmatter,
			},
		}

		g := Build(docs, cfg)
		if _, ok := g.Nodes["0001"]; !ok {
			t.Fatalf("node 0001 is missing: %v", g.Nodes)
		}
	})

	t.Run("a document without frontmatter matching the filename pattern is still a node", func(t *testing.T) {
		docs := []*parse.Document{
			{
				Path:           "0001-no-frontmatter.md",
				Name:           "0001-no-frontmatter.md",
				ID:             "0001",
				Body:           "# No frontmatter here\n",
				HasFrontmatter: false,
				MatchesPattern: true,
			},
		}

		g := Build(docs, cfg)
		n, ok := g.Nodes["0001"]
		if !ok {
			t.Fatalf("node 0001 is missing: %v", g.Nodes)
		}
		if n.Status != "" {
			t.Errorf("status = %q, want empty", n.Status)
		}
	})
}

func TestBuildTypedEdges(t *testing.T) {
	cfg := config.ADRPreset()

	t.Run("a forward edge points from the containing document to the reference", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "superseded"}, ""),
			testDoc("0002", map[string]any{
				"status":     "accepted",
				"supersedes": []any{"0001"},
			}, ""),
		}

		g := Build(docs, cfg)
		want := []model.Edge{testEdge("0002", "0001", config.EdgeSupersedes)}
		if !slices.EqualFunc(g.Edges, want, model.Edge.Equal) {
			t.Fatalf("edges = %+v, want %+v", g.Edges, want)
		}
	})

	t.Run("each declared edge key produces its own edge type", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "accepted"}, ""),
			testDoc("0002", map[string]any{
				"status":     "accepted",
				"depends-on": []any{"0001"},
			}, ""),
		}

		g := Build(docs, cfg)
		want := []model.Edge{testEdge("0002", "0001", config.EdgeDependsOn)}
		if !slices.EqualFunc(g.Edges, want, model.Edge.Equal) {
			t.Fatalf("edges = %+v, want %+v", g.Edges, want)
		}
	})

	t.Run("references are normalized before they become edges", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "superseded"}, ""),
			testDoc("0002", map[string]any{
				"status":     "accepted",
				"supersedes": []any{"ADR-1", "1", "0001"},
			}, ""),
		}

		g := Build(docs, cfg)
		want := []model.Edge{testEdge("0002", "0001", config.EdgeSupersedes)}
		if !slices.EqualFunc(g.Edges, want, model.Edge.Equal) {
			t.Fatalf("edges = %+v, want %+v (three spellings of one reference are one edge)", g.Edges, want)
		}
	})

	t.Run("a reverse direction edge spec flips the edge", func(t *testing.T) {
		reversed := config.ADRPreset()
		reversed.Edges = []config.EdgeSpec{{
			Name:      config.EdgeSupersedes.String(),
			Key:       "superseded-by",
			Acyclic:   true,
			Direction: config.DirectionReverse,
		}}
		reversed.DerivedEdges = nil
		docs := []*parse.Document{
			testDoc("0002", map[string]any{
				"status":        "superseded",
				"superseded-by": []any{"0003"},
			}, ""),
			testDoc("0003", map[string]any{"status": "accepted"}, ""),
		}

		g := Build(docs, reversed)
		want := []model.Edge{testEdge("0003", "0002", config.EdgeSupersedes)}
		if !slices.EqualFunc(g.Edges, want, model.Edge.Equal) {
			t.Fatalf("edges = %+v, want %+v", g.Edges, want)
		}
	})

	t.Run("an edge to an unknown document is kept for the dangling check", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0002", map[string]any{
				"status":     "accepted",
				"supersedes": []any{"0099"},
			}, ""),
		}

		g := Build(docs, cfg)
		want := []model.Edge{testEdge("0002", "0099", config.EdgeSupersedes)}
		if !slices.EqualFunc(g.Edges, want, model.Edge.Equal) {
			t.Fatalf("edges = %+v, want %+v", g.Edges, want)
		}
	})

	t.Run("a reference that names no identity is rejected", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0002", map[string]any{
				"status":     "accepted",
				"supersedes": []any{"a-reference-without-digits"},
			}, ""),
		}

		g := Build(docs, cfg)
		f := testAssertSingleFinding(t, g.Findings, model.RuleInvalidRef, model.SeverityError, "0002")
		if !strings.Contains(f.Detail, "a-reference-without-digits") {
			t.Errorf("detail = %q, want it to name the unresolvable reference", f.Detail)
		}
	})

	t.Run("an edge entry that is not a reference at all is recorded as dangling", func(t *testing.T) {
		// An unquoted wikilink decodes as a nested sequence rather than a string.
		docs := []*parse.Document{
			testDoc("0002", map[string]any{
				"status":     "accepted",
				"supersedes": []any{[]any{uint64(1)}},
			}, ""),
		}

		g := Build(docs, cfg)
		if len(g.Edges) != 0 {
			t.Fatalf("edges = %+v, want none", g.Edges)
		}
		f := testAssertSingleFinding(t, g.Findings, model.RuleDanglingRef, model.SeverityError, "0002")
		if !strings.Contains(f.Detail, config.EdgeSupersedes.String()) {
			t.Errorf("detail = %q, want it to name the edge type", f.Detail)
		}
	})

	t.Run("typed edges are sorted deterministically whatever the document order", func(t *testing.T) {
		frontmatter := map[string]map[string]any{
			"0001": {"status": "superseded"},
			"0002": {"status": "superseded", "supersedes": []any{"0001"}},
			"0003": {"status": "accepted", "supersedes": []any{"0002"}, "depends-on": []any{"0001"}},
		}
		order := func(ids ...string) []*parse.Document {
			docs := make([]*parse.Document, 0, len(ids))
			for _, id := range ids {
				docs = append(docs, testDoc(id, frontmatter[id], ""))
			}
			return docs
		}
		want := []model.Edge{
			testEdge("0002", "0001", config.EdgeSupersedes),
			testEdge("0003", "0001", config.EdgeDependsOn),
			testEdge("0003", "0002", config.EdgeSupersedes),
		}

		ascending := Build(order("0001", "0002", "0003"), cfg)
		shuffled := Build(order("0003", "0001", "0002"), cfg)
		if !slices.EqualFunc(ascending.Edges, want, model.Edge.Equal) {
			t.Fatalf("edges = %+v, want %+v (sorted by from, type, to)", ascending.Edges, want)
		}
		if !slices.EqualFunc(shuffled.Edges, ascending.Edges, model.Edge.Equal) {
			t.Fatalf("document order changed the edge order: %+v vs %+v", shuffled.Edges, ascending.Edges)
		}
	})
}

func TestBuildDerivedEdges(t *testing.T) {
	cfg := config.ADRPreset()

	t.Run("a derived edge points from the referenced document to the containing one", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0002", map[string]any{"status": "superseded by 0003"}, ""),
			testDoc("0003", map[string]any{"status": "accepted"}, ""),
		}

		g := Build(docs, cfg)
		want := []model.Edge{testDerivedEdge("0003", "0002", config.EdgeSupersedes)}
		if !slices.EqualFunc(g.Edges, want, model.Edge.Equal) {
			t.Fatalf("edges = %+v, want %+v (the referenced document is the new one)", g.Edges, want)
		}
	})

	t.Run("the hyphenated spelling is derived too", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0002", map[string]any{"status": "Superseded-by 0003"}, ""),
			testDoc("0003", map[string]any{"status": "accepted"}, ""),
		}

		g := Build(docs, cfg)
		want := []model.Edge{testDerivedEdge("0003", "0002", config.EdgeSupersedes)}
		if !slices.EqualFunc(g.Edges, want, model.Edge.Equal) {
			t.Fatalf("edges = %+v, want %+v", g.Edges, want)
		}
	})

	t.Run("the derived status is projected onto the node", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0002", map[string]any{"status": "superseded by 0003"}, ""),
			testDoc("0003", map[string]any{"status": "accepted"}, ""),
		}

		g := Build(docs, cfg)
		n, ok := g.Nodes["0002"]
		if !ok {
			t.Fatalf("node 0002 is missing: %v", g.Nodes)
		}
		if n.Status != config.StatusSuperseded {
			t.Fatalf("status = %q, want %q", n.Status, config.StatusSuperseded)
		}
	})

	t.Run("a derived edge agreeing with a structured edge is not duplicated", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0002", map[string]any{"status": "superseded by 0003"}, ""),
			testDoc("0003", map[string]any{
				"status":     "accepted",
				"supersedes": []any{"0002"},
			}, ""),
		}

		g := Build(docs, cfg)
		want := []model.Edge{testEdge("0003", "0002", config.EdgeSupersedes)}
		if !slices.EqualFunc(g.Edges, want, model.Edge.Equal) {
			t.Fatalf("edges = %+v, want %+v (agreement collapses to the structured edge)", g.Edges, want)
		}
	})
}

func TestBuildReferenceLayer(t *testing.T) {
	cfg := config.ADRPreset()

	t.Run("body links become reference edges", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "superseded"}, ""),
			testDoc("0002", map[string]any{"status": "accepted"}, "Replaces [[0001]] entirely.\n"),
		}

		g := Build(docs, cfg)
		want := []model.Edge{testRefEdge("0002", "0001")}
		if !slices.EqualFunc(g.RefEdges, want, model.Edge.Equal) {
			t.Fatalf("reference edges = %+v, want %+v", g.RefEdges, want)
		}
		if len(g.Edges) != 0 {
			t.Fatalf("typed edges = %+v, want none (the reference layer never constrains the DAG)", g.Edges)
		}
	})

	t.Run("links to unmanaged documents are dropped without a finding", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "accepted"}, "See [[0099]] and [[not-a-document]].\n"),
		}

		g := Build(docs, cfg)
		if len(g.RefEdges) != 0 {
			t.Fatalf("reference edges = %+v, want none", g.RefEdges)
		}
		if len(g.Findings) != 0 {
			t.Fatalf("findings = %+v, want none (the reference layer is never validated)", g.Findings)
		}
	})
}

func TestBuildReferenceLayerTakesOnlyIdentityShapedLinks(t *testing.T) {
	cfg := config.ADRPreset()
	docs := []*parse.Document{
		testDoc("0001", map[string]any{"status": "accepted"}, ""),
		testDoc("0002", map[string]any{"status": "accepted"},
			"Unlike [[1]], [[upstream]] and [[tool.uv.index]], only [[0001]] names a document.\n"),
	}

	g := Build(docs, cfg)

	want := []model.Edge{testRefEdge("0002", "0001")}
	if !slices.EqualFunc(g.RefEdges, want, model.Edge.Equal) {
		t.Fatalf("reference edges = %+v, want %+v", g.RefEdges, want)
	}
}

func TestBuildReportsAnEdgeKeyThatNamesNothing(t *testing.T) {
	cfg := config.ADRPreset()
	tests := []struct {
		name  string
		value any
		want  bool
	}{
		{name: "a null value", want: true},
		{name: "an empty list", value: []any{}, want: true},
		{name: "a list of empty items", value: []any{"", "   "}, want: true},
		{name: "an empty scalar", value: "", want: true},
		{name: "a populated list", value: []any{"0001"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs := []*parse.Document{
				testDoc("0001", map[string]any{"status": "accepted"}, ""),
				testDoc("0002", map[string]any{"status": "accepted", "supersedes": tt.value}, ""),
			}

			g := Build(docs, cfg)

			found := testFindingsFor(g.Findings, model.RuleEmptyEdge)
			if !tt.want {
				if len(found) != 0 {
					t.Fatalf("findings = %+v, want none", found)
				}
				return
			}
			f := testAssertSingleFinding(t, g.Findings, model.RuleEmptyEdge, model.SeverityError, "0002")
			if f.Location != testNodeLocation("0002", docs[1].KeyLines["supersedes"]) {
				t.Errorf("location = %+v, want the line of the empty key", f.Location)
			}
			if !strings.Contains(f.Detail, "supersedes") {
				t.Errorf("detail = %q, want the key named", f.Detail)
			}
		})
	}

	t.Run("an absent key is not an empty one", func(t *testing.T) {
		docs := []*parse.Document{testDoc("0001", map[string]any{"status": "accepted"}, "")}

		if g := Build(docs, cfg); len(g.Findings) != 0 {
			t.Fatalf("findings = %+v, want none", g.Findings)
		}
	})
}

func TestBuildReportsDanglingReferences(t *testing.T) {
	withDangling := func(mode string) config.Config {
		cfg := config.ADRPreset()
		cfg.References.Dangling = mode
		return cfg
	}

	t.Run("the reference layer is unvalidated by default", func(t *testing.T) {
		docs := []*parse.Document{testDoc("0001", map[string]any{"status": "accepted"}, "See [[0099]].\n")}

		g := Build(docs, config.ADRPreset())

		if len(g.Findings) != 0 {
			t.Fatalf("findings = %+v, want none", g.Findings)
		}
	})

	t.Run("an enabled corpus reports the missing target at its line", func(t *testing.T) {
		docs := []*parse.Document{testDoc("0001", map[string]any{"status": "accepted"}, "First line.\n\nSee [[0099]].\n")}

		g := Build(docs, withDangling(string(model.SeverityError)))

		f := testAssertSingleFinding(t, g.Findings, model.RuleDanglingReference, model.SeverityError, "0001")
		want := model.Location{Path: "0001.md", Line: testBodyLine + 2}
		if f.Location != want {
			t.Errorf("location = %+v, want %+v", f.Location, want)
		}
		if !strings.Contains(f.Detail, "0099") || !strings.Contains(f.Detail, string(parse.LinkWiki)) {
			t.Errorf("detail = %q, want the link kind and the missing target", f.Detail)
		}
	})

	t.Run("the configured mode sets the severity", func(t *testing.T) {
		docs := []*parse.Document{testDoc("0001", map[string]any{"status": "accepted"}, "See [[0099]].\n")}

		g := Build(docs, withDangling(string(model.SeverityWarn)))

		testAssertSingleFinding(t, g.Findings, model.RuleDanglingReference, model.SeverityWarn, "0001")
	})

	t.Run("a markdown link to a missing document is reported too", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "accepted"}, "See [the queue](0099-hand-off-through-a-queue.md).\n"),
		}

		g := Build(docs, withDangling(string(model.SeverityError)))

		f := testAssertSingleFinding(t, g.Findings, model.RuleDanglingReference, model.SeverityError, "0001")
		if !strings.Contains(f.Detail, string(parse.LinkMarkdown)) {
			t.Errorf("detail = %q, want the markdown link kind", f.Detail)
		}
	})

	t.Run("each link to one missing target is its own finding", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "accepted"},
				"See [[0099]].\n\nAnd [the other](0099-a-decision.md).\n"),
		}

		g := Build(docs, withDangling(string(model.SeverityError)))

		findings := testFindingsFor(g.Findings, model.RuleDanglingReference)
		if len(findings) != 2 {
			t.Fatalf("findings = %+v, want one per link", findings)
		}
		if findings[0].Location.Line == findings[1].Location.Line {
			t.Errorf("findings = %+v, want the two lines kept apart", findings)
		}
	})

	t.Run("one link written twice on a line is one finding", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "accepted"}, "See [[0099]] and [[0099]] again.\n"),
		}

		g := Build(docs, withDangling(string(model.SeverityError)))

		testAssertSingleFinding(t, g.Findings, model.RuleDanglingReference, model.SeverityError, "0001")
	})

	t.Run("prose that names no identity is never dangling", func(t *testing.T) {
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "accepted"},
				"See [[upstream]], [[3days-recap]] and `[[0099]]`.\n"),
		}

		g := Build(docs, withDangling(string(model.SeverityError)))

		if len(g.Findings) != 0 {
			t.Fatalf("findings = %+v, want none", g.Findings)
		}
	})

	t.Run("frontmatter is scanned only when asked for", func(t *testing.T) {
		frontmatter := map[string]any{"status": "accepted", "relates": "see [[0099]]"}
		cfg := withDangling(string(model.SeverityError))

		if g := Build([]*parse.Document{testDoc("0001", frontmatter, "")}, cfg); len(g.Findings) != 0 {
			t.Fatalf("findings = %+v, want none while only the body is scanned", g.Findings)
		}

		cfg.References.Scan = []string{config.ScanBody, config.ScanFrontmatter}
		doc := testDoc("0001", frontmatter, "")
		g := Build([]*parse.Document{doc}, cfg)

		f := testAssertSingleFinding(t, g.Findings, model.RuleDanglingReference, model.SeverityError, "0001")
		if f.Location.Line != doc.KeyLines["relates"] {
			t.Errorf("location = %+v, want the line of the key holding the link", f.Location)
		}
	})
}

func TestBuildRejectsAFrontmatterReferenceThatIsNotIdentityShaped(t *testing.T) {
	cfg := config.ADRPreset()
	docs := []*parse.Document{
		testDoc("0001", map[string]any{"status": "accepted"}, ""),
		testDoc("0002", map[string]any{
			"status":     "accepted",
			"supersedes": []any{"see 0001"},
		}, ""),
	}

	g := Build(docs, cfg)

	f := testAssertSingleFinding(t, g.Findings, model.RuleInvalidRef, model.SeverityError, "0002")
	if !strings.Contains(f.Detail, "see 0001") {
		t.Errorf("detail = %q, want the reference as written", f.Detail)
	}
	if len(g.Edges) != 0 {
		t.Fatalf("edges = %+v, want none: the reference names no identity", g.Edges)
	}
}

func TestAdjacency(t *testing.T) {
	g := testTypedFixture()

	t.Run("defaults to every typed edge type", func(t *testing.T) {
		testAssertAdjacency(t, Adjacency(g), testAdj(map[string][]string{
			"0001": {},
			"0002": {"0001"},
			"0003": {"0002"},
			"0004": {"0003"},
		}))
	})

	t.Run("filters by edge type", func(t *testing.T) {
		testAssertAdjacency(t, Adjacency(g, config.EdgeDependsOn), testAdj(map[string][]string{
			"0001": {},
			"0002": {},
			"0003": {},
			"0004": {"0003"},
		}))
	})

	t.Run("neighbour lists are sorted and deduplicated", func(t *testing.T) {
		fanOut := testGraph(
			[]*model.Node{testNode("0001", "accepted"), testNode("0002", "superseded"), testNode("0003", "superseded")},
			[]model.Edge{
				testEdge("0001", "0003", config.EdgeSupersedes),
				testEdge("0001", "0002", config.EdgeSupersedes),
				testDerivedEdge("0001", "0002", config.EdgeSupersedes),
			},
			nil,
		)

		testAssertIDs(t, "Adjacency(0001)", Adjacency(fanOut)["0001"], testIDs("0002", "0003"))
	})

	t.Run("reference edges are excluded", func(t *testing.T) {
		refsOnly := testGraph(
			[]*model.Node{testNode("0001", "accepted"), testNode("0002", "accepted")},
			nil,
			[]model.Edge{testRefEdge("0002", "0001")},
		)

		testAssertIDs(t, "Adjacency(0002)", Adjacency(refsOnly)["0002"], nil)
	})

	t.Run("an edge to an unknown document stays in the neighbour list", func(t *testing.T) {
		dangling := testGraph(
			[]*model.Node{testNode("0002", "accepted")},
			[]model.Edge{testEdge("0002", "0099", config.EdgeSupersedes)},
			nil,
		)

		testAssertIDs(t, "Adjacency(0002)", Adjacency(dangling)["0002"], testIDs("0099"))
	})
}

func TestReverse(t *testing.T) {
	g := testTypedFixture()

	t.Run("inverts every typed edge", func(t *testing.T) {
		testAssertAdjacency(t, Reverse(g), testAdj(map[string][]string{
			"0001": {"0002"},
			"0002": {"0003"},
			"0003": {"0004"},
			"0004": {},
		}))
	})

	t.Run("filters by edge type", func(t *testing.T) {
		testAssertAdjacency(t, Reverse(g, config.EdgeSupersedes), testAdj(map[string][]string{
			"0001": {"0002"},
			"0002": {"0003"},
			"0003": {},
			"0004": {},
		}))
	})

	t.Run("fan-in lists every predecessor sorted", func(t *testing.T) {
		fanIn := testGraph(
			[]*model.Node{testNode("0001", "superseded"), testNode("0002", "accepted"), testNode("0003", "accepted")},
			[]model.Edge{
				testEdge("0003", "0001", config.EdgeSupersedes),
				testEdge("0002", "0001", config.EdgeSupersedes),
			},
			nil,
		)

		testAssertIDs(t, "Reverse(0001)", Reverse(fanIn)["0001"], testIDs("0002", "0003"))
	})

	t.Run("an unknown target still gets an entry", func(t *testing.T) {
		dangling := testGraph(
			[]*model.Node{testNode("0002", "accepted")},
			[]model.Edge{testEdge("0002", "0099", config.EdgeSupersedes)},
			nil,
		)

		testAssertIDs(t, "Reverse(0099)", Reverse(dangling)["0099"], testIDs("0002"))
	})
}

func TestReferenceAdjacency(t *testing.T) {
	t.Run("maps reference edges from source to target", func(t *testing.T) {
		g := testTypedFixture()

		testAssertIDs(t, "ReferenceAdjacency(0003)", ReferenceAdjacency(g)["0003"], testIDs("0001"))
		testAssertIDs(t, "ReferenceAdjacency(0004)", ReferenceAdjacency(g)["0004"], testIDs("0001"))
	})

	t.Run("neighbour lists are sorted and deduplicated", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0001", "accepted"), testNode("0002", "accepted"), testNode("0003", "accepted")},
			nil,
			[]model.Edge{
				testRefEdge("0003", "0002"),
				testRefEdge("0003", "0001"),
				testRefEdge("0003", "0002"),
			},
		)

		testAssertIDs(t, "ReferenceAdjacency(0003)", ReferenceAdjacency(g)["0003"], testIDs("0001", "0002"))
	})

	t.Run("typed edges are excluded", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0001", "superseded"), testNode("0002", "accepted")},
			[]model.Edge{testEdge("0002", "0001", config.EdgeSupersedes)},
			nil,
		)

		testAssertIDs(t, "ReferenceAdjacency(0002)", ReferenceAdjacency(g)["0002"], nil)
	})
}

// testTypedFixture is a two-layer graph: a supersedes chain 0003 -> 0002 ->
// 0001, a depends-on edge 0004 -> 0003, and two reference links onto 0001.
func testTypedFixture() *model.Graph {
	return testGraph(
		[]*model.Node{
			testNode("0001", config.StatusSuperseded),
			testNode("0002", config.StatusSuperseded),
			testNode("0003", config.StatusAccepted),
			testNode("0004", config.StatusAccepted),
		},
		[]model.Edge{
			testEdge("0002", "0001", config.EdgeSupersedes),
			testEdge("0003", "0002", config.EdgeSupersedes),
			testEdge("0004", "0003", config.EdgeDependsOn),
		},
		[]model.Edge{
			testRefEdge("0003", "0001"),
			testRefEdge("0004", "0001"),
		},
	)
}

func TestBuildNodesCarryFrontmatterPositions(t *testing.T) {
	docs := []*parse.Document{testDoc("0001", map[string]any{"status": "accepted", "title": "First"}, "")}

	g := Build(docs, config.ADRPreset())
	n, ok := g.Node("0001")
	if !ok {
		t.Fatal("node 0001 is missing")
	}
	if n.Line != docs[0].FrontmatterLine {
		t.Errorf("line = %d, want %d", n.Line, docs[0].FrontmatterLine)
	}
	for key, want := range docs[0].KeyLines {
		if got := n.KeyLines[key]; got != want {
			t.Errorf("keyLines[%s] = %d, want %d", key, got, want)
		}
	}
}

func TestBuildLocatesAnUnresolvableReference(t *testing.T) {
	docs := []*parse.Document{
		testDoc("0002", map[string]any{
			"status":     "accepted",
			"supersedes": []any{"a-reference-without-digits"},
		}, ""),
	}

	g := Build(docs, config.ADRPreset())
	f := testAssertSingleFinding(t, g.Findings, model.RuleInvalidRef, model.SeverityError, "0002")
	want := model.Location{Path: "0002.md", Line: docs[0].KeyLines["supersedes"]}
	if f.Location != want {
		t.Errorf("location = %+v, want %+v", f.Location, want)
	}
}

func TestBuildAcrossKinds(t *testing.T) {
	cfg := testKindsConfig()

	t.Run("a node carries its kind, on the node and as an attribute", func(t *testing.T) {
		docs := []*parse.Document{testKindDoc("clause", "UZ-V-001", map[string]any{"status": "accepted"})}

		g := Build(docs, cfg)

		n, ok := g.Node("UZ-V-001")
		if !ok {
			t.Fatalf("node UZ-V-001 is missing: %v", g.Nodes)
		}
		if n.Kind != "clause" {
			t.Errorf("kind = %q, want clause", n.Kind)
		}
		if got, ok := n.Attr(config.KeyKind); !ok || got != "clause" {
			t.Errorf("attr kind = %q (ok=%v), want clause", got, ok)
		}
	})

	t.Run("the directory's kind wins over a frontmatter key that disagrees", func(t *testing.T) {
		// The disagreement is the kind_mismatch finding; what rules read has to
		// be the kind the document was actually parsed as.
		docs := []*parse.Document{testKindDoc("clause", "UZ-V-001", map[string]any{"kind": "conform", "status": "accepted"})}

		g := Build(docs, cfg)

		if got, _ := g.Nodes["UZ-V-001"].Attr(config.KeyKind); got != "clause" {
			t.Fatalf("attr kind = %q, want the directory's answer", got)
		}
	})

	t.Run("a single-kind corpus leaves a written kind alone", func(t *testing.T) {
		docs := []*parse.Document{testDoc("0001", map[string]any{"kind": "decision", "status": "accepted"}, "")}

		g := Build(docs, config.ADRPreset())

		n := g.Nodes["0001"]
		if n.Kind != "" {
			t.Errorf("kind = %q, want none on a single-kind corpus", n.Kind)
		}
		if got, _ := n.Attr(config.KeyKind); got != "decision" {
			t.Errorf("attr kind = %q, want the frontmatter value untouched", got)
		}
	})

	t.Run("a file without an identity is no node at all", func(t *testing.T) {
		stray := testKindDoc("clause", "", nil)
		stray.Identity = "README"

		g := Build([]*parse.Document{stray}, cfg)

		if len(g.Nodes) != 0 {
			t.Fatalf("nodes = %v, want none: the file yielded no identifier", g.Nodes)
		}
		if len(testFindingsFor(g.Findings, model.RuleIDMismatch)) != 1 {
			t.Fatalf("findings = %+v, want the id_mismatch", g.Findings)
		}
	})

	t.Run("an edge resolves a reference of another kind", func(t *testing.T) {
		docs := []*parse.Document{
			testKindDoc("clause", "UZ-V-001", map[string]any{"status": "accepted"}),
			testKindDoc("conform", "conform/check", map[string]any{"status": "accepted", "enforces": []any{"UZ-V-001"}}),
		}

		g := Build(docs, cfg)

		want := []model.Edge{testEdge("conform/check", "UZ-V-001", "enforces")}
		if !slices.EqualFunc(g.Edges, want, func(a, b model.Edge) bool { return a.Equal(b) }) {
			t.Fatalf("edges = %+v, want %+v", g.Edges, want)
		}
	})

	t.Run("a reference no kind accepts is not an identifier", func(t *testing.T) {
		docs := []*parse.Document{
			testKindDoc("conform", "conform/check", map[string]any{"status": "accepted", "enforces": []any{"see UZ-V-001"}}),
		}

		g := Build(docs, cfg)

		testAssertSingleFinding(t, g.Findings, model.RuleInvalidRef, model.SeverityError, "conform/check")
	})

	t.Run("a reference of the wrong kind still resolves, and is a kind mismatch", func(t *testing.T) {
		// Reporting it as "not an identifier" would hide what is actually
		// wrong: the reference names a document, of a kind the edge cannot
		// reach.
		docs := []*parse.Document{
			testKindDoc("deviation", "dev-0001", map[string]any{"status": "accepted"}),
			testKindDoc("conform", "conform/check", map[string]any{"status": "accepted", "enforces": []any{"dev-0001"}}),
		}

		g := Build(docs, cfg)
		findings := Validate(g, cfg, time.Time{})

		if len(g.Edges) != 1 {
			t.Fatalf("edges = %+v, want the edge to have resolved", g.Edges)
		}
		testAssertSingleFinding(t, findings, model.RuleEdgeKindMismatch, model.SeverityError, "conform/check")
	})

	t.Run("a kind keeps its own status vocabulary", func(t *testing.T) {
		vocabulary := testKindsConfig()
		vocabulary.Kinds["clause"] = config.KindSpec{Dir: "spec/clauses", ID: `^UZ-[A-Z]-\d{3}$`, StatusValues: []string{"trial", "accepted"}}
		docs := []*parse.Document{
			testKindDoc("clause", "UZ-V-001", map[string]any{"status": "proposed"}),
			testKindDoc("deviation", "dev-0001", map[string]any{"status": "proposed"}),
		}

		findings := Validate(Build(docs, vocabulary), vocabulary, time.Time{})

		// The clause vocabulary does not carry proposed; the deviation kind
		// declares none and inherits the top-level one, which does.
		f := testAssertSingleFinding(t, findings, model.RuleUnknownStatus, model.SeverityError, "UZ-V-001")
		if !strings.Contains(f.Detail, "trial, accepted") {
			t.Errorf("detail = %q, want it to name the kind's own vocabulary", f.Detail)
		}
	})
}

func TestKindIsReadableByRulesAndProjections(t *testing.T) {
	cfg := testKindsConfig()
	cfg.Projections = []config.ProjectionSpec{
		testProjection("is_clause", config.Condition{Attr: map[string]config.AttrCondition{config.KeyKind: testAttrEq("clause")}}),
	}
	cfg.Rules = []config.Rule{
		{
			Name:     "orphan_test",
			Severity: model.SeverityWarn,
			When: config.Condition{
				Attr:        map[string]config.AttrCondition{config.KeyKind: testAttrEq("conform")},
				NotOutbound: "enforces",
			},
			Message: "enforces no clause",
		},
	}
	docs := []*parse.Document{
		testKindDoc("clause", "UZ-V-001", map[string]any{"status": "accepted"}),
		testKindDoc("conform", "conform/check", map[string]any{"status": "accepted"}),
	}
	g := Build(docs, cfg)

	t.Run("a projection reads the kind", func(t *testing.T) {
		testAssertIDs(t, "is_clause", EvalProjections(g, cfg).Set("is_clause"), testIDs("UZ-V-001"))
	})

	t.Run("a rule reads the kind", func(t *testing.T) {
		testAssertSingleFinding(t, EvalRules(g, cfg), "orphan_test", model.SeverityWarn, "conform/check")
	})
}
