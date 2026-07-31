package graph

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
)

func TestComputeStats(t *testing.T) {
	cfg := config.ADRPreset()
	got := ComputeStats(testStatsFixture(), cfg)

	t.Run("documents are counted", func(t *testing.T) {
		if got.Documents != 5 {
			t.Fatalf("documents = %d, want 5", got.Documents)
		}
	})

	t.Run("edges are counted per declared type in declaration order", func(t *testing.T) {
		want := []EdgeCount{
			{Type: config.EdgeSupersedes, Count: 2},
			{Type: config.EdgeDependsOn, Count: 1},
		}

		if !slices.Equal(got.Edges, want) {
			t.Fatalf("edges = %+v, want %+v", got.Edges, want)
		}
	})

	t.Run("binding counts accepted documents nothing supersedes", func(t *testing.T) {
		if got.Binding != 2 {
			t.Fatalf("binding = %d, want 2 (0003 and 0004)", got.Binding)
		}
	})

	t.Run("chain depth distributes the longest supersedes chain per document", func(t *testing.T) {
		want := []DepthCount{
			{Depth: 0, Count: 3},
			{Depth: 1, Count: 1},
			{Depth: 2, Count: 1},
		}

		if !slices.Equal(got.ChainDepth, want) {
			t.Fatalf("chain depth = %+v, want %+v", got.ChainDepth, want)
		}
	})

	t.Run("orphans are documents without any typed edge", func(t *testing.T) {
		if got.Orphans != 1 {
			t.Fatalf("orphans = %d, want 1 (0005)", got.Orphans)
		}
		if got.OrphanRate != 0.2 {
			t.Fatalf("orphan rate = %v, want 0.2", got.OrphanRate)
		}
	})

	t.Run("top referenced ranks the reference layer in-degree", func(t *testing.T) {
		want := []ReferenceCount{
			{ID: "0001", Count: 2},
			{ID: "0003", Count: 1},
		}

		if !slices.Equal(got.TopReferenced, want) {
			t.Fatalf("top referenced = %+v, want %+v", got.TopReferenced, want)
		}
	})
}

func TestComputeStatsTopReferenced(t *testing.T) {
	cfg := config.ADRPreset()

	t.Run("ties break on the identifier", func(t *testing.T) {
		g := testGraph(
			[]*model.Node{
				testNode("0001", config.StatusAccepted),
				testNode("0002", config.StatusAccepted),
				testNode("0003", config.StatusAccepted),
				testNode("0004", config.StatusAccepted),
				testNode("0005", config.StatusAccepted),
			},
			nil,
			[]model.Edge{
				testRefEdge("0001", "0004"),
				testRefEdge("0002", "0004"),
				testRefEdge("0001", "0003"),
				testRefEdge("0002", "0003"),
				testRefEdge("0001", "0005"),
			},
		)
		want := []ReferenceCount{
			{ID: "0003", Count: 2},
			{ID: "0004", Count: 2},
			{ID: "0005", Count: 1},
		}

		if got := ComputeStats(g, cfg).TopReferenced; !slices.Equal(got, want) {
			t.Fatalf("top referenced = %+v, want %+v (never-referenced documents are omitted)", got, want)
		}
	})

	t.Run("the ranking is capped", func(t *testing.T) {
		const targets = 12
		nodes := []*model.Node{}
		refs := []model.Edge{}
		for i := 1; i <= targets; i++ {
			nodes = append(nodes, testNode(testStatsID(1000+i), config.StatusAccepted))
			nodes = append(nodes, testNode(testStatsID(2000+i), config.StatusAccepted))
		}
		for target := 1; target <= targets; target++ {
			for source := 1; source <= target; source++ {
				refs = append(refs, testRefEdge(testStatsID(2000+source), testStatsID(1000+target)))
			}
		}
		g := testGraph(nodes, nil, refs)

		got := ComputeStats(g, cfg).TopReferenced

		if len(got) != TopReferencedLimit {
			t.Fatalf("top referenced has %d entries, want %d", len(got), TopReferencedLimit)
		}
		for i, rc := range got {
			wantID := model.ID(testStatsID(1000 + targets - i))
			wantCount := targets - i
			if rc.ID != wantID || rc.Count != wantCount {
				t.Fatalf("top referenced[%d] = %+v, want {%s %d}", i, rc, wantID, wantCount)
			}
		}
	})
}

func TestComputeStatsEmptyGraph(t *testing.T) {
	cfg := config.ADRPreset()

	got := ComputeStats(testGraph(nil, nil, nil), cfg)

	if got.Documents != 0 || got.Binding != 0 || got.Orphans != 0 {
		t.Fatalf("stats = %+v, want an empty corpus", got)
	}
	if got.OrphanRate != 0 {
		t.Fatalf("orphan rate = %v, want 0 for an empty corpus", got.OrphanRate)
	}
	want := []EdgeCount{
		{Type: config.EdgeSupersedes, Count: 0},
		{Type: config.EdgeDependsOn, Count: 0},
	}
	if !slices.Equal(got.Edges, want) {
		t.Fatalf("edges = %+v, want %+v", got.Edges, want)
	}
	if len(got.ChainDepth) != 0 || len(got.TopReferenced) != 0 {
		t.Fatalf("stats = %+v, want no depth or reference entries", got)
	}
}

func TestComputeStatsOnCyclicGraph(t *testing.T) {
	cfg := config.ADRPreset()

	var got Statistics
	testMustNotHang(t, 5*time.Second, func() { got = ComputeStats(testSupersedesCycle(), cfg) })

	if got.Documents != 3 {
		t.Fatalf("documents = %d, want 3", got.Documents)
	}
}

// testStatsFixture chains 0003 -> 0002 -> 0001 by supersedes, hangs a depends-on
// edge off it, leaves 0005 unconnected and links three reference-layer edges.
func testStatsFixture() *model.Graph {
	return testGraph(
		[]*model.Node{
			testNode("0001", config.StatusSuperseded),
			testNode("0002", config.StatusSuperseded),
			testNode("0003", config.StatusAccepted),
			testNode("0004", config.StatusAccepted),
			testNode("0005", config.StatusProposed),
		},
		[]model.Edge{
			testEdge("0002", "0001", config.EdgeSupersedes),
			testEdge("0003", "0002", config.EdgeSupersedes),
			testEdge("0004", "0003", config.EdgeDependsOn),
		},
		[]model.Edge{
			testRefEdge("0002", "0001"),
			testRefEdge("0003", "0001"),
			testRefEdge("0004", "0003"),
		},
	)
}

func testStatsID(n int) string {
	return fmt.Sprintf("%04d", n)
}
