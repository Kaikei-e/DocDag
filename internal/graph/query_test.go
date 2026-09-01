package graph

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name  string
		graph *model.Graph
		id    string
		want  []string
	}{
		{
			name: "a document nothing supersedes resolves to itself",
			graph: testGraph(
				[]*model.Node{testNode("0001", config.StatusAccepted)},
				nil,
				nil,
			),
			id:   "0001",
			want: []string{"0001"},
		},
		{
			name:  "a chain resolves to the sink",
			graph: testSupersedesChain(),
			id:    "0001",
			want:  []string{"0003"},
		},
		{
			name:  "a mid-chain document resolves to the same sink",
			graph: testSupersedesChain(),
			id:    "0002",
			want:  []string{"0003"},
		},
		{
			name:  "the sink resolves to itself",
			graph: testSupersedesChain(),
			id:    "0003",
			want:  []string{"0003"},
		},
		{
			name: "fan-in converges on the single successor",
			graph: testGraph(
				[]*model.Node{
					testNode("0001", config.StatusSuperseded),
					testNode("0002", config.StatusSuperseded),
					testNode("0003", config.StatusAccepted),
				},
				[]model.Edge{
					testEdge("0003", "0001", config.EdgeSupersedes),
					testEdge("0003", "0002", config.EdgeSupersedes),
				},
				nil,
			),
			id:   "0001",
			want: []string{"0003"},
		},
		{
			name:  "a diamond reports the shared sink once",
			graph: testSupersedesDiamond(),
			id:    "0001",
			want:  []string{"0004"},
		},
		{
			name: "two independent successors are both reported, sorted",
			graph: testGraph(
				[]*model.Node{
					testNode("0001", config.StatusSuperseded),
					testNode("0002", config.StatusAccepted),
					testNode("0003", config.StatusAccepted),
				},
				[]model.Edge{
					testEdge("0003", "0001", config.EdgeSupersedes),
					testEdge("0002", "0001", config.EdgeSupersedes),
				},
				nil,
			),
			id:   "0001",
			want: []string{"0002", "0003"},
		},
		{
			name: "edges of another type are not followed",
			graph: testGraph(
				[]*model.Node{testNode("0001", config.StatusAccepted), testNode("0002", config.StatusAccepted)},
				[]model.Edge{testEdge("0002", "0001", config.EdgeDependsOn)},
				nil,
			),
			id:   "0001",
			want: []string{"0001"},
		},
		{
			name: "derived successors are followed like structured ones",
			graph: testGraph(
				[]*model.Node{testNode("0001", config.StatusSuperseded), testNode("0002", config.StatusAccepted)},
				[]model.Edge{testDerivedEdge("0002", "0001", config.EdgeSupersedes)},
				nil,
			),
			id:   "0001",
			want: []string{"0002"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Resolve(tt.graph, model.ID(tt.id), config.EdgeSupersedes)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			testAssertIDs(t, "Resolve", got, testIDs(tt.want...))
		})
	}
}

func TestResolveErrors(t *testing.T) {
	t.Run("an unknown id is reported", func(t *testing.T) {
		_, err := Resolve(testSupersedesChain(), "0099", config.EdgeSupersedes)

		if !errors.Is(err, model.ErrUnknownID) {
			t.Fatalf("error = %v, want %v", err, model.ErrUnknownID)
		}
	})

	t.Run("a cycle is reported instead of walked forever", func(t *testing.T) {
		g := testSupersedesCycle()

		var err error
		testMustNotHang(t, 5*time.Second, func() { _, err = Resolve(g, "0001", config.EdgeSupersedes) })

		if !errors.Is(err, model.ErrCycle) {
			t.Fatalf("error = %v, want %v", err, model.ErrCycle)
		}
	})
}

func TestResolveOnDeepChain(t *testing.T) {
	const n = 10000
	g := testDeepChainGraph(n)

	var (
		got []model.ID
		err error
	)
	testMustNotHang(t, 30*time.Second, func() {
		got, err = Resolve(g, model.ID(testChainID(0)), config.EdgeSupersedes)
	})

	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	testAssertIDs(t, "Resolve", got, testIDs(testChainID(n-1)))
}

func TestAncestorsAndDescendants(t *testing.T) {
	g := testMixedFixture()

	tests := []struct {
		name      string
		direction Direction
		id        string
		types     []model.EdgeType
		want      []string
	}{
		{
			name:      "descendants follow every typed edge forwards",
			direction: DirectionDescendants,
			id:        "0004",
			want:      []string{"0001", "0002", "0003"},
		},
		{
			name:      "ancestors follow every typed edge backwards",
			direction: DirectionAncestors,
			id:        "0002",
			want:      []string{"0003", "0004"},
		},
		{
			name:      "descendants filter by edge type",
			direction: DirectionDescendants,
			id:        "0004",
			types:     []model.EdgeType{config.EdgeDependsOn},
			want:      []string{"0003"},
		},
		{
			name:      "ancestors filter by edge type",
			direction: DirectionAncestors,
			id:        "0002",
			types:     []model.EdgeType{config.EdgeSupersedes},
			want:      []string{"0003"},
		},
		{
			name:      "a sink has no descendants",
			direction: DirectionDescendants,
			id:        "0001",
			want:      nil,
		},
		{
			name:      "a source has no ancestors",
			direction: DirectionAncestors,
			id:        "0004",
			want:      nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var (
				got []model.ID
				err error
			)
			if tt.direction == DirectionAncestors {
				got, err = Ancestors(g, model.ID(tt.id), tt.types...)
			} else {
				got, err = Descendants(g, model.ID(tt.id), tt.types...)
			}
			if err != nil {
				t.Fatalf("query: %v", err)
			}
			testAssertIDs(t, "reachable", got, testIDs(tt.want...))
		})
	}
}

func TestAncestorsAndDescendantsEdgeCases(t *testing.T) {
	t.Run("an unknown id is reported by both directions", func(t *testing.T) {
		g := testMixedFixture()

		if _, err := Ancestors(g, "0099"); !errors.Is(err, model.ErrUnknownID) {
			t.Errorf("Ancestors error = %v, want %v", err, model.ErrUnknownID)
		}
		if _, err := Descendants(g, "0099"); !errors.Is(err, model.ErrUnknownID) {
			t.Errorf("Descendants error = %v, want %v", err, model.ErrUnknownID)
		}
	})

	t.Run("a cycle terminates and never reports the start node", func(t *testing.T) {
		g := testSupersedesCycle()

		var (
			got []model.ID
			err error
		)
		testMustNotHang(t, 5*time.Second, func() { got, err = Descendants(g, "0001") })

		if err != nil {
			t.Fatalf("Descendants: %v", err)
		}
		testAssertIDs(t, "Descendants", got, testIDs("0002", "0003"))
	})

	t.Run("results are sorted and exclude the start node", func(t *testing.T) {
		g := testSupersedesDiamond()

		got, err := Descendants(g, "0004", config.EdgeSupersedes)
		if err != nil {
			t.Fatalf("Descendants: %v", err)
		}
		testAssertIDs(t, "Descendants", got, testIDs("0001", "0002", "0003"))
	})

	t.Run("edges to unknown documents are not reachable results", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0002", config.StatusAccepted)},
			[]model.Edge{testEdge("0002", "0099", config.EdgeSupersedes)},
			nil,
		)

		got, err := Descendants(g, "0002")
		if err != nil {
			t.Fatalf("Descendants: %v", err)
		}
		testAssertIDs(t, "Descendants", got, nil)
	})
}

func TestQuery(t *testing.T) {
	g := testQueryFixture()

	t.Run("typed results carry the typed layer", func(t *testing.T) {
		got, err := Query(g, "0002", QueryOptions{Direction: DirectionDescendants})
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		want := []QueryResult{{ID: "0001", Layer: LayerTyped}}
		if !slices.Equal(got, want) {
			t.Fatalf("Query = %+v, want %+v", got, want)
		}
	})

	t.Run("ancestors direction walks the other way", func(t *testing.T) {
		got, err := Query(g, "0002", QueryOptions{Direction: DirectionAncestors})
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		want := []QueryResult{
			{ID: "0003", Layer: LayerTyped},
			{ID: "0004", Layer: LayerTyped},
		}
		if !slices.Equal(got, want) {
			t.Fatalf("Query = %+v, want %+v", got, want)
		}
	})

	t.Run("edge type filter applies to the typed layer", func(t *testing.T) {
		got, err := Query(g, "0002", QueryOptions{
			Direction: DirectionAncestors,
			Types:     []model.EdgeType{config.EdgeSupersedes},
		})
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		want := []QueryResult{{ID: "0003", Layer: LayerTyped}}
		if !slices.Equal(got, want) {
			t.Fatalf("Query = %+v, want %+v", got, want)
		}
	})

	t.Run("reference neighbours are overlaid only when asked for", func(t *testing.T) {
		got, err := Query(g, "0002", QueryOptions{Direction: DirectionDescendants, IncludeRefs: true})
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		want := []QueryResult{
			{ID: "0001", Layer: LayerTyped},
			{ID: "0004", Layer: LayerReference},
		}
		if !slices.Equal(got, want) {
			t.Fatalf("Query = %+v, want %+v (typed results first, then reference, each sorted)", got, want)
		}
	})

	t.Run("a document reached through both layers is reported once as typed", func(t *testing.T) {
		got, err := Query(g, "0002", QueryOptions{Direction: DirectionDescendants, IncludeRefs: true})
		if err != nil {
			t.Fatalf("Query: %v", err)
		}
		seen := map[model.ID]int{}
		for _, r := range got {
			seen[r.ID]++
		}
		if seen["0001"] != 1 {
			t.Fatalf("0001 appears %d times in %+v, want once", seen["0001"], got)
		}
	})

	t.Run("an unknown id is reported", func(t *testing.T) {
		if _, err := Query(g, "0099", QueryOptions{Direction: DirectionDescendants}); !errors.Is(err, model.ErrUnknownID) {
			t.Fatalf("error = %v, want %v", err, model.ErrUnknownID)
		}
	})

	t.Run("an unset direction is an error rather than a silent default", func(t *testing.T) {
		if _, err := Query(g, "0002", QueryOptions{}); err == nil {
			t.Fatal("error = nil, want a direction error")
		}
	})
}

func TestReferenceNeighbors(t *testing.T) {
	g := testQueryFixture()

	t.Run("outbound links are neighbours", func(t *testing.T) {
		testAssertIDs(t, "ReferenceNeighbors(0002)", ReferenceNeighbors(g, "0002"), testIDs("0001", "0004"))
	})

	t.Run("inbound backlinks are neighbours too", func(t *testing.T) {
		testAssertIDs(t, "ReferenceNeighbors(0004)", ReferenceNeighbors(g, "0004"), testIDs("0002"))
	})

	t.Run("duplicate links collapse", func(t *testing.T) {
		dup := testGraph(
			[]*model.Node{testNode("0001", config.StatusAccepted), testNode("0002", config.StatusAccepted)},
			nil,
			[]model.Edge{testRefEdge("0002", "0001"), testRefEdge("0002", "0001"), testRefEdge("0001", "0002")},
		)

		testAssertIDs(t, "ReferenceNeighbors(0002)", ReferenceNeighbors(dup, "0002"), testIDs("0001"))
	})

	t.Run("a document without reference links has no neighbours", func(t *testing.T) {
		testAssertIDs(t, "ReferenceNeighbors(0003)", ReferenceNeighbors(g, "0003"), nil)
	})
}

func TestBinding(t *testing.T) {
	cfg := config.ADRPreset()
	g := testGraph(
		[]*model.Node{
			testNode("0001", config.StatusSuperseded),
			testNode("0002", config.StatusAccepted),
			testNode("0003", config.StatusProposed),
			testNode("0004", "Accepted"),
			testNode("0005", config.StatusAccepted),
		},
		[]model.Edge{
			testEdge("0002", "0001", config.EdgeSupersedes),
			testEdge("0002", "0005", config.EdgeSupersedes),
			testEdge("0004", "0003", config.EdgeDependsOn),
		},
		nil,
	)

	tests := []struct {
		name string
		id   string
		want bool
	}{
		{name: "accepted without inbound supersedes is binding", id: "0002", want: true},
		{name: "superseded is not binding", id: "0001", want: false},
		{name: "proposed is not binding", id: "0003", want: false},
		{name: "the status comparison is case-insensitive", id: "0004", want: true},
		{name: "accepted with an inbound supersedes edge is not binding", id: "0005", want: false},
		{name: "an unknown id is not binding", id: "0099", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Binding(g, cfg, model.ID(tt.id)); got != tt.want {
				t.Fatalf("Binding(%s) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}

	t.Run("BindingSet lists every binding document sorted", func(t *testing.T) {
		testAssertIDs(t, "BindingSet", BindingSet(g, cfg), testIDs("0002", "0004"))
	})

	t.Run("BindingSet is empty when nothing is accepted", func(t *testing.T) {
		none := testGraph([]*model.Node{testNode("0001", config.StatusProposed)}, nil, nil)

		testAssertIDs(t, "BindingSet", BindingSet(none, cfg), nil)
	})

	t.Run("a status that only opens with a vocabulary word is not binding", func(t *testing.T) {
		prose := testGraph([]*model.Node{testNode("0001", "accepted by the architecture board")}, nil, nil)

		testAssertIDs(t, "BindingSet", BindingSet(prose, cfg), nil)
	})
}

func TestResolveNeverNamesADocumentTheCorpusDoesNotHold(t *testing.T) {
	t.Run("a reference to an unknown successor resolves to the document itself", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0007", config.StatusSuperseded)},
			[]model.Edge{testDerivedEdge("0099", "0007", config.EdgeSupersedes)},
			nil,
		)

		got, err := Resolve(g, "0007", config.EdgeSupersedes)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		testAssertIDs(t, "Resolve(0007)", got, testIDs("0007"))
	})

	t.Run("a chain stops at its last known document", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{testNode("0001", config.StatusSuperseded), testNode("0002", config.StatusSuperseded)},
			[]model.Edge{
				testEdge("0002", "0001", config.EdgeSupersedes),
				testDerivedEdge("0099", "0002", config.EdgeSupersedes),
			},
			nil,
		)

		got, err := Resolve(g, "0001", config.EdgeSupersedes)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		testAssertIDs(t, "Resolve(0001)", got, testIDs("0002"))
	})
}

func testSupersedesChain() *model.Graph {
	return testGraph(
		[]*model.Node{
			testNode("0001", config.StatusSuperseded),
			testNode("0002", config.StatusSuperseded),
			testNode("0003", config.StatusAccepted),
		},
		[]model.Edge{
			testEdge("0002", "0001", config.EdgeSupersedes),
			testEdge("0003", "0002", config.EdgeSupersedes),
		},
		nil,
	)
}

// testSupersedesDiamond has 0001 replaced twice and both replacements replaced
// by 0004, so resolution has to deduplicate the converged sink.
func testSupersedesDiamond() *model.Graph {
	return testGraph(
		[]*model.Node{
			testNode("0001", config.StatusSuperseded),
			testNode("0002", config.StatusSuperseded),
			testNode("0003", config.StatusSuperseded),
			testNode("0004", config.StatusAccepted),
		},
		[]model.Edge{
			testEdge("0002", "0001", config.EdgeSupersedes),
			testEdge("0003", "0001", config.EdgeSupersedes),
			testEdge("0004", "0002", config.EdgeSupersedes),
			testEdge("0004", "0003", config.EdgeSupersedes),
		},
		nil,
	)
}

func testSupersedesCycle() *model.Graph {
	return testGraph(
		[]*model.Node{
			testNode("0001", config.StatusSuperseded),
			testNode("0002", config.StatusSuperseded),
			testNode("0003", config.StatusSuperseded),
		},
		[]model.Edge{
			testEdge("0001", "0002", config.EdgeSupersedes),
			testEdge("0002", "0003", config.EdgeSupersedes),
			testEdge("0003", "0001", config.EdgeSupersedes),
		},
		nil,
	)
}

// testMixedFixture chains supersedes 0003 -> 0002 -> 0001 and hangs a depends-on
// edge 0004 -> 0003 off it, so edge-type filters have something to filter.
func testMixedFixture() *model.Graph {
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
		nil,
	)
}

func testQueryFixture() *model.Graph {
	g := testMixedFixture()
	g.RefEdges = []model.Edge{
		testRefEdge("0002", "0004"),
		testRefEdge("0002", "0001"),
	}
	return g
}

func testDeepChainGraph(n int) *model.Graph {
	nodes := make([]*model.Node, 0, n)
	edges := make([]model.Edge, 0, n-1)
	for i := 0; i < n; i++ {
		nodes = append(nodes, testNode(testChainID(i), config.StatusSuperseded))
	}
	for i := 0; i+1 < n; i++ {
		edges = append(edges, testEdge(testChainID(i+1), testChainID(i), config.EdgeSupersedes))
	}
	return testGraph(nodes, edges, nil)
}

func TestBindingSetAnswersTheBuiltInDefinition(t *testing.T) {
	// The binding set moved from code into the ADR preset's projection. The two
	// have to agree on every corpus the repository carries, or a preset change
	// nobody asked for went out with it.
	cfg := config.ADRPreset()
	for _, name := range testFixtureNames(t) {
		t.Run(name, func(t *testing.T) {
			g, _ := testFixtureGraph(t, name)

			written := bindingByStatus(g, cfg)
			want := []model.ID{}
			for _, id := range g.NodeIDs() {
				if written[id] {
					want = append(want, id)
				}
			}

			testAssertIDs(t, "BindingSet", BindingSet(g, cfg), want)
			for _, id := range g.NodeIDs() {
				if got := Binding(g, cfg, id); got != written[id] {
					t.Fatalf("Binding(%s) = %v, want %v", id, got, written[id])
				}
			}
		})
	}
}

func TestBindingWithoutABindingProjection(t *testing.T) {
	g := testGraph(
		[]*model.Node{
			testNode("0001", config.StatusSuperseded),
			testNode("0002", config.StatusAccepted),
			testNode("0003", config.StatusProposed),
		},
		[]model.Edge{testEdge("0002", "0001", config.EdgeSupersedes)},
		nil,
	)

	t.Run("a configuration that cleared its projections falls back", func(t *testing.T) {
		cleared := config.ADRPreset()
		cleared.Projections = []config.ProjectionSpec{}

		testAssertIDs(t, "BindingSet", BindingSet(g, cleared), testIDs("0002"))
		if !Binding(g, cleared, "0002") || Binding(g, cleared, "0001") {
			t.Fatal("Binding disagrees with the built-in definition")
		}
	})

	t.Run("a configuration naming no binding falls back", func(t *testing.T) {
		unnamed := config.ADRPreset()
		unnamed.Binding = ""

		testAssertIDs(t, "BindingSet", BindingSet(g, unnamed), testIDs("0002"))
	})

	t.Run("a projection of its own replaces the definition", func(t *testing.T) {
		own := config.ADRPreset()
		own.Projections = []config.ProjectionSpec{{
			Name: "proposed_only",
			When: config.Condition{Attr: map[string]config.AttrCondition{
				config.DefaultStatusField: testAttrEq(config.StatusProposed),
			}},
		}}
		own.Binding = "proposed_only"

		testAssertIDs(t, "BindingSet", BindingSet(g, own), testIDs("0003"))
		if !Binding(g, own, "0003") || Binding(g, own, "0002") {
			t.Fatal("Binding does not follow the configured projection")
		}
	})
}

func TestBindingCountsAnInboundEdgeFromAnUnknownDocument(t *testing.T) {
	// A "superseded by 0099" status derives an inbound supersedes edge from a
	// document nobody wrote. The dangling reference is a finding of its own;
	// what the edge says about this document stands either way.
	g := testGraph(
		[]*model.Node{testNode("0001", config.StatusAccepted)},
		[]model.Edge{testDerivedEdge("0099", "0001", config.EdgeSupersedes)},
		nil,
	)
	cfg := config.ADRPreset()

	if Binding(g, cfg, "0001") {
		t.Fatal("Binding = true, want an inbound supersedes from an unknown document to un-bind it")
	}
	if got := BindingSet(g, cfg); len(got) != 0 {
		t.Fatalf("BindingSet = %v, want none", got)
	}
}
