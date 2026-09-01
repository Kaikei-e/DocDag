package graph

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
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

func TestComputeFieldUsage(t *testing.T) {
	cfg := config.ADRPreset()
	cfg.Fields = map[string]config.FieldSpec{
		"owner": {Deprecated: true, MigrateTo: "owned-by"},
		"team":  {Deprecated: true},
	}
	g := testGraph(
		[]*model.Node{
			testNodeAttrs("0001", config.StatusAccepted, map[string]any{"owner": "platform"}),
			testNodeAttrs("0002", config.StatusAccepted, map[string]any{"owner": "payments", "tags": []any{"legacy"}}),
			testNodeAttrs("0003", config.StatusAccepted, nil),
		},
		nil, nil,
	)
	changed := map[string]string{"0001.md": "2026-03-04", "0002.md": "2026-01-02"}

	got := ComputeFieldUsage(g, cfg, changed)

	t.Run("fields count down, ties alphabetically", func(t *testing.T) {
		want := []string{"status", "owner", "tags", "team"}
		names := make([]string, 0, len(got))
		for _, u := range got {
			names = append(names, u.Field)
		}
		if !slices.Equal(names, want) {
			t.Fatalf("fields = %v, want %v", names, want)
		}
	})

	t.Run("a written field carries its count and its latest change", func(t *testing.T) {
		owner := testFieldUsage(t, got, "owner")

		if owner.Documents != 2 {
			t.Errorf("documents = %d, want 2", owner.Documents)
		}
		if owner.LastChange != "2026-03-04" {
			t.Errorf("last change = %q, want the most recent of the two", owner.LastChange)
		}
		if !owner.Deprecated {
			t.Error("deprecated = false, want the retirement flagged")
		}
	})

	t.Run("a declared field nobody writes is still a row", func(t *testing.T) {
		// A migration is finished exactly when the count reaches zero, so the
		// row has to outlive the last document that carried it.
		team := testFieldUsage(t, got, "team")

		if team.Documents != 0 || team.LastChange != "" {
			t.Fatalf("team = %+v, want an empty row", team)
		}
	})

	t.Run("a field nobody declared is counted and not flagged", func(t *testing.T) {
		tags := testFieldUsage(t, got, "tags")

		if tags.Documents != 1 || tags.Deprecated {
			t.Fatalf("tags = %+v, want one document and no retirement", tags)
		}
	})

	t.Run("a corpus outside a repository reports counts without dates", func(t *testing.T) {
		for _, u := range ComputeFieldUsage(g, cfg, nil) {
			if u.LastChange != "" {
				t.Fatalf("%s last change = %q, want none where git answered nothing", u.Field, u.LastChange)
			}
		}
	})
}

// TestComputeStatsOverAStandard covers what a corpus of clauses adds to the
// degree report: how the subjects are cut, at what strengths the standard
// speaks, and how many conflicts it is carrying an exception for.
func TestComputeStatsOverAStandard(t *testing.T) {
	cfg := config.SpecPreset()
	g := Build([]*parse.Document{
		testTopicDoc(testTopic),
		testTopicDoc(testOtherTopic),
		testClause("UZ-V-001", config.ModalityMAY, []string{testTopic}, map[string]any{
			config.EdgeExcepts.String(): []any{
				map[string]any{"ref": "UZ-V-002", config.AttrScope: "only where the run is calibrated"},
			},
		}),
		testClause("UZ-V-002", config.ModalitySHOULDNOT, []string{testTopic}, nil),
		testClause("UZ-V-003", config.ModalitySHOULD, []string{testOtherTopic}, nil),
	}, cfg)

	stats := ComputeStats(g, cfg)

	t.Run("the subjects rank by the clauses hanging off them", func(t *testing.T) {
		want := []TopicCount{{Topic: testTopic, Clauses: 2}, {Topic: testOtherTopic, Clauses: 1}}
		if !slices.Equal(stats.Topics, want) {
			t.Errorf("topics = %+v, want %+v", stats.Topics, want)
		}
	})

	t.Run("every declared modality is a row, at zero where nobody states it", func(t *testing.T) {
		want := []ModalityCount{
			{Modality: config.ModalityMUST, Count: 0},
			{Modality: config.ModalityMUSTNOT, Count: 0},
			{Modality: config.ModalitySHOULD, Count: 1},
			{Modality: config.ModalitySHOULDNOT, Count: 1},
			{Modality: config.ModalityMAY, Count: 1},
		}
		if !slices.Equal(stats.Modalities, want) {
			t.Errorf("modalities = %+v, want %+v", stats.Modalities, want)
		}
	})

	t.Run("the conflicts an exception answers are counted", func(t *testing.T) {
		if stats.SuppressedConflicts != 1 {
			t.Errorf("suppressed conflicts = %d, want the one the excepts edge answers", stats.SuppressedConflicts)
		}
	})

	t.Run("a corpus without the vocabulary reports none of it", func(t *testing.T) {
		adr := ComputeStats(testStatsFixture(), config.ADRPreset())
		if len(adr.Topics) != 0 || len(adr.Modalities) != 0 || adr.SuppressedConflicts != 0 {
			t.Errorf("stats = %+v, want no subject or modality rows: the adr preset declares neither", adr)
		}
	})
}

func testFieldUsage(t *testing.T, usage []FieldUsage, field string) FieldUsage {
	t.Helper()
	for _, u := range usage {
		if u.Field == field {
			return u
		}
	}
	t.Fatalf("field %q is not in %+v", field, usage)
	return FieldUsage{}
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
