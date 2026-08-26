package render

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
)

func TestLabel(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		title string
		limit int
		want  string
	}{
		{
			name:  "a label below the limit is left alone",
			id:    "0002",
			title: "Poll feeds with adaptive backoff",
			limit: LabelLimit,
			want:  "0002 Poll feeds with adaptive backoff",
		},
		{
			name:  "a label of exactly the limit is left alone",
			id:    "0001",
			title: "Use a message queue for ingestion h",
			limit: LabelLimit,
			want:  "0001 Use a message queue for ingestion h",
		},
		{
			name:  "one character over the limit is cut to 37 plus an ellipsis",
			id:    "0001",
			title: "Use a message queue for ingestion he",
			limit: LabelLimit,
			want:  "0001 Use a message queue for ingestio...",
		},
		{
			name:  "double quotes become apostrophes",
			id:    "0001",
			title: `Adopt the "strangler fig" pattern`,
			limit: LabelLimit,
			want:  "0001 Adopt the 'strangler fig' pattern",
		},
		{
			name:  "quotes are replaced before the label is cut",
			id:    "0001",
			title: `Adopt the "strangler fig" pattern for the ingestion path`,
			limit: LabelLimit,
			want:  "0001 Adopt the 'strangler fig' patter...",
		},
		{
			name:  "a title spanning lines collapses onto one",
			id:    "0001",
			title: "Use a message queue\nfor ingestion\n",
			limit: LabelLimit,
			want:  "0001 Use a message queue for ingestion",
		},
		{
			name:  "runs of whitespace collapse to single spaces",
			id:    "0001",
			title: "Adopt   the\tstrangler fig",
			limit: LabelLimit,
			want:  "0001 Adopt the strangler fig",
		},
		{
			name:  "the limit counts runes rather than bytes",
			id:    "0001",
			title: "メッセージキューを採用して取り込みを分離する案について",
			limit: LabelLimit,
			want:  "0001 メッセージキューを採用して取り込みを分離する案について",
		},
		{
			name:  "a non-positive limit falls back to LabelLimit",
			id:    "0001",
			title: "Use a message queue for ingestion he",
			limit: 0,
			want:  "0001 Use a message queue for ingestio...",
		},
		{
			name:  "a custom limit is honoured",
			id:    "0001",
			title: "Poll feeds with adaptive backoff",
			limit: 12,
			want:  "0001 Poll...",
		},
		{
			name:  "a document without a title renders as its identifier",
			id:    "0001",
			title: "",
			limit: LabelLimit,
			want:  "0001",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Label(testNode(tt.id, tt.title, "accepted", tt.id+".md"), tt.limit)
			if got != tt.want {
				t.Errorf("Label = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMermaidGolden(t *testing.T) {
	tests := []struct {
		name   string
		graph  *model.Graph
		opts   Options
		golden string
	}{
		{
			name:   "the typed layer of ok-basic",
			graph:  testOKBasicGraph(),
			golden: "ok-basic.mmd",
		},
		{
			name:   "ok-basic with the reference layer overlaid",
			graph:  testOKBasicGraph(),
			opts:   Options{IncludeRefs: true},
			golden: "ok-basic-refs.mmd",
		},
		{
			name:   "the fan-in of two superseded documents",
			graph:  testFanInGraph(),
			golden: "fan-in.mmd",
		},
		{
			name:   "ok-basic without the documents no typed edge touches",
			graph:  testOKBasicGraph(),
			opts:   Options{Connected: true},
			golden: "ok-basic-connected.mmd",
		},
		{
			name:   "ok-basic restricted to one edge type",
			graph:  testOKBasicGraph(),
			opts:   Options{Edges: []model.EdgeType{config.EdgeDependsOn}},
			golden: "ok-basic-depends-on.mmd",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testAssertGolden(t, tt.golden, testRender(t, Mermaid, tt.graph, tt.opts))
		})
	}
}

func TestMermaidHeader(t *testing.T) {
	got := testRender(t, Mermaid, testOKBasicGraph(), Options{})
	first, _, _ := strings.Cut(got, "\n")
	if first != "graph LR" {
		t.Errorf("first line = %q, want %q", first, "graph LR")
	}
}

func TestDOTGolden(t *testing.T) {
	tests := []struct {
		name   string
		graph  *model.Graph
		opts   Options
		golden string
	}{
		{
			name:   "the typed layer of ok-basic",
			graph:  testOKBasicGraph(),
			golden: "ok-basic.dot",
		},
		{
			name:   "ok-basic with the reference layer overlaid",
			graph:  testOKBasicGraph(),
			opts:   Options{IncludeRefs: true},
			golden: "ok-basic-refs.dot",
		},
		{
			name:   "the fan-in of two superseded documents",
			graph:  testFanInGraph(),
			golden: "fan-in.dot",
		},
		{
			name:   "ok-basic without the documents no typed edge touches",
			graph:  testOKBasicGraph(),
			opts:   Options{Connected: true},
			golden: "ok-basic-connected.dot",
		},
		{
			name:   "ok-basic restricted to one edge type",
			graph:  testOKBasicGraph(),
			opts:   Options{Edges: []model.EdgeType{config.EdgeDependsOn}},
			golden: "ok-basic-depends-on.dot",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testAssertGolden(t, tt.golden, testRender(t, DOT, tt.graph, tt.opts))
		})
	}
}

func TestNodeLinkJSONGolden(t *testing.T) {
	tests := []struct {
		name   string
		graph  *model.Graph
		opts   Options
		golden string
	}{
		{
			name:   "the typed layer of ok-basic",
			graph:  testOKBasicGraph(),
			golden: "ok-basic.json",
		},
		{
			name:   "ok-basic with the reference layer overlaid",
			graph:  testOKBasicGraph(),
			opts:   Options{IncludeRefs: true},
			golden: "ok-basic-refs.json",
		},
		{
			name:   "the fan-in of two superseded documents",
			graph:  testFanInGraph(),
			golden: "fan-in.json",
		},
		{
			name:   "ok-basic without the documents no typed edge touches",
			graph:  testOKBasicGraph(),
			opts:   Options{Connected: true},
			golden: "ok-basic-connected.json",
		},
		{
			name:   "ok-basic restricted to one edge type",
			graph:  testOKBasicGraph(),
			opts:   Options{Edges: []model.EdgeType{config.EdgeDependsOn}},
			golden: "ok-basic-depends-on.json",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testAssertGolden(t, tt.golden, testRender(t, NodeLinkJSON, tt.graph, tt.opts))
		})
	}
}

func TestNodeLinkJSONDecodes(t *testing.T) {
	got := testRender(t, NodeLinkJSON, testOKBasicGraph(), Options{})

	var doc NodeLink
	if err := json.Unmarshal([]byte(got), &doc); err != nil {
		t.Fatalf("decode node-link JSON %q: %v", got, err)
	}

	ids := make([]model.ID, 0, len(doc.Nodes))
	for _, n := range doc.Nodes {
		ids = append(ids, n.ID)
	}
	wantIDs := []model.ID{"0001", "0002", "0003", "0004", "0005", "0006"}
	if !slices.Equal(ids, wantIDs) {
		t.Errorf("nodes = %v, want %v in ascending order", ids, wantIDs)
	}

	links := make([]string, 0, len(doc.Links))
	for _, l := range doc.Links {
		links = append(links, strings.Join([]string{l.Source.String(), l.Type.String(), l.Target.String(), string(l.Origin)}, "|"))
	}
	wantLinks := []string{
		"0002|supersedes|0001|structured",
		"0004|depends-on|0003|structured",
		"0004|supersedes|0002|structured",
		"0005|depends-on|0003|structured",
	}
	if !slices.Equal(links, wantLinks) {
		t.Errorf("links = %v, want %v sorted by source, type and target", links, wantLinks)
	}
}

// testOverlayGraph carries the same pair as both a typed and a reference edge,
// so the two layers have to stay distinguishable in the rendering.
func testOverlayGraph() *model.Graph {
	return testGraph(
		[]*model.Node{
			testNode("0001", "Alpha", "superseded", "0001.md"),
			testNode("0002", "Beta", "accepted", "0002.md"),
		},
		[]model.Edge{testEdge("0002", "0001", config.EdgeSupersedes)},
		[]model.Edge{testRefEdge("0002", "0001")},
	)
}

func TestReferenceOverlayIsDistinctFromTypedEdges(t *testing.T) {
	opts := Options{IncludeRefs: true}

	t.Run("mermaid marks reference edges with a dotted arrow and no label", func(t *testing.T) {
		got := testRender(t, Mermaid, testOverlayGraph(), opts)
		for _, want := range []string{"  0002 -->|supersedes| 0001\n", "  0002 -.-> 0001\n"} {
			if !strings.Contains(got, want) {
				t.Errorf("mermaid output is missing %q:\n%s", want, got)
			}
		}
	})

	t.Run("dot marks reference edges as dotted and unlabelled", func(t *testing.T) {
		got := testRender(t, DOT, testOverlayGraph(), opts)
		for _, want := range []string{
			"  \"0002\" -> \"0001\" [label=\"supersedes\"];\n",
			"  \"0002\" -> \"0001\" [style=dotted];\n",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("dot output is missing %q:\n%s", want, got)
			}
		}
	})

	t.Run("json marks reference edges with an empty type and the reference origin", func(t *testing.T) {
		var doc NodeLink
		if err := json.Unmarshal([]byte(testRender(t, NodeLinkJSON, testOverlayGraph(), opts)), &doc); err != nil {
			t.Fatalf("decode node-link JSON: %v", err)
		}
		want := []NodeLinkEdge{
			{Source: "0002", Target: "0001", Type: config.EdgeSupersedes, Origin: model.OriginStructured},
			{Source: "0002", Target: "0001", Type: "", Origin: model.OriginReference},
		}
		if !slices.Equal(doc.Links, want) {
			t.Errorf("links = %+v, want %+v", doc.Links, want)
		}
	})
}

func TestReferenceLayerIsOmittedByDefault(t *testing.T) {
	tests := []struct {
		format   string
		rendered testRenderer
		unwanted string
	}{
		{format: "mermaid", rendered: Mermaid, unwanted: "-.->"},
		{format: "dot", rendered: DOT, unwanted: "style=dotted"},
		{format: "json", rendered: NodeLinkJSON, unwanted: string(model.OriginReference)},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			got := testRender(t, tt.rendered, testOKBasicGraph(), Options{})
			if strings.Contains(got, tt.unwanted) {
				t.Errorf("%s output carries the reference layer without IncludeRefs (%q):\n%s", tt.format, tt.unwanted, got)
			}
		})
	}
}

func TestConnectedIsDecidedByTypedEdgesAlone(t *testing.T) {
	// 0002 is reachable only over the reference layer, so --connected must
	// drop it whether or not the reference layer is drawn.
	g := testGraph(
		[]*model.Node{
			testNode("0001", "Emit structured logs as JSON", "accepted", "0001.md"),
			testNode("0002", "Ship logs to the aggregator", "accepted", "0002.md"),
			testNode("0003", "Sample debug logs", "superseded", "0003.md"),
		},
		[]model.Edge{testEdge("0001", "0003", config.EdgeSupersedes)},
		[]model.Edge{testRefEdge("0001", "0002")},
	)
	for format, fn := range testRenderers() {
		t.Run(format, func(t *testing.T) {
			got := testRender(t, fn, g, Options{Connected: true, IncludeRefs: true})
			if strings.Contains(got, "0002") {
				t.Errorf("%s output keeps a document no typed edge touches:\n%s", format, got)
			}
			for _, want := range []string{"0001", "0003"} {
				if !strings.Contains(got, want) {
					t.Errorf("%s output dropped connected document %s:\n%s", format, want, got)
				}
			}
		})
	}
}

func TestEdgeFilterNarrowsTheConnectedSet(t *testing.T) {
	opts := Options{Connected: true, Edges: []model.EdgeType{config.EdgeSupersedes}}

	var doc NodeLink
	if err := json.Unmarshal([]byte(testRender(t, NodeLinkJSON, testOKBasicGraph(), opts)), &doc); err != nil {
		t.Fatalf("decode node-link JSON: %v", err)
	}

	ids := make([]model.ID, 0, len(doc.Nodes))
	for _, n := range doc.Nodes {
		ids = append(ids, n.ID)
	}
	if want := []model.ID{"0001", "0002", "0004"}; !slices.Equal(ids, want) {
		t.Errorf("nodes = %v, want %v", ids, want)
	}
	want := []NodeLinkEdge{
		{Source: "0002", Target: "0001", Type: config.EdgeSupersedes, Origin: model.OriginStructured},
		{Source: "0004", Target: "0002", Type: config.EdgeSupersedes, Origin: model.OriginStructured},
	}
	if !slices.Equal(doc.Links, want) {
		t.Errorf("links = %+v, want %+v", doc.Links, want)
	}
}

func TestRenderingIsDeterministic(t *testing.T) {
	for format, fn := range testRenderers() {
		t.Run(format, func(t *testing.T) {
			first := testRender(t, fn, testOKBasicGraph(), Options{IncludeRefs: true})
			for i := range 20 {
				got := testRender(t, fn, testOKBasicGraph(), Options{IncludeRefs: true})
				if got != first {
					t.Fatalf("%s rendering %d differs from the first:\ngot:\n%s\nwant:\n%s", format, i, got, first)
				}
			}
		})
	}
}

func TestRenderEmptyGraph(t *testing.T) {
	tests := []struct {
		format   string
		rendered testRenderer
		want     string
	}{
		{format: "mermaid", rendered: Mermaid, want: "graph LR\n"},
		{format: "dot", rendered: DOT, want: "digraph docdag {\n  rankdir=LR;\n}\n"},
		{format: "json", rendered: NodeLinkJSON, want: "{\n  \"nodes\": [],\n  \"links\": []\n}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.format, func(t *testing.T) {
			got := testRender(t, tt.rendered, testEmptyGraph(), Options{IncludeRefs: true})
			if got != tt.want {
				t.Errorf("%s of an empty graph = %q, want %q", tt.format, got, tt.want)
			}
		})
	}
}

func TestRenderSkipsEdgesWithUnknownEndpoints(t *testing.T) {
	g := testGraph(
		[]*model.Node{testNode("0001", "Emit structured logs as JSON", "accepted", "0001.md")},
		[]model.Edge{testEdge("0001", "0009", config.EdgeSupersedes)},
		[]model.Edge{testRefEdge("0001", "0009")},
	)
	for format, fn := range testRenderers() {
		t.Run(format, func(t *testing.T) {
			got := testRender(t, fn, g, Options{IncludeRefs: true})
			if strings.Contains(got, "0009") {
				t.Errorf("%s output renders an edge to a document that is not in the graph:\n%s", format, got)
			}
		})
	}
}

func TestRenderPropagatesWriterErrors(t *testing.T) {
	for format, fn := range testRenderers() {
		t.Run(format, func(t *testing.T) {
			err := fn(failingWriter{}, testOKBasicGraph(), Options{})
			if !errors.Is(err, errWriterClosed) {
				t.Errorf("%s error = %v, want it to wrap %v", format, err, errWriterClosed)
			}
		})
	}
}
