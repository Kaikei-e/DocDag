package model

import (
	"slices"
	"testing"
)

func testGraph() *Graph {
	g := NewGraph()
	for _, n := range []*Node{
		{ID: "0003", Path: "0003.md", Title: "Third", Status: "accepted"},
		{ID: "0001", Path: "0001.md", Title: "First", Status: "superseded"},
		{ID: "0002", Path: "0002.md", Title: "Second", Status: "superseded"},
	} {
		g.Nodes[n.ID] = n
	}
	g.Edges = []Edge{
		{From: "0002", To: "0001", Type: "supersedes", Origin: OriginStructured},
		{From: "0003", To: "0002", Type: "supersedes", Origin: OriginDerived},
		{From: "0003", To: "0001", Type: "depends-on", Origin: OriginStructured},
	}
	return g
}

func TestNodeAttr(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]any
		key   string
		want  string
		ok    bool
	}{
		{name: "a string attribute", attrs: map[string]any{"status": "accepted"}, key: "status", want: "accepted", ok: true},
		{name: "an empty string is present", attrs: map[string]any{"status": ""}, key: "status", want: "", ok: true},
		{name: "an integer stringifies", attrs: map[string]any{"revision": 7}, key: "revision", want: "7", ok: true},
		{name: "an unsigned integer stringifies", attrs: map[string]any{"revision": uint64(7)}, key: "revision", want: "7", ok: true},
		{name: "a boolean stringifies", attrs: map[string]any{"draft": true}, key: "draft", want: "true", ok: true},
		{name: "a float stringifies", attrs: map[string]any{"ratio": 1.5}, key: "ratio", want: "1.5", ok: true},
		{name: "a list is not a scalar", attrs: map[string]any{"supersedes": []any{"0001"}}, key: "supersedes"},
		{name: "a mapping is not a scalar", attrs: map[string]any{"meta": map[string]any{"k": "v"}}, key: "meta"},
		{name: "an explicit null is absent", attrs: map[string]any{"status": nil}, key: "status"},
		{name: "a missing key is absent", attrs: map[string]any{"status": "accepted"}, key: "state"},
		{name: "keys are case-sensitive", attrs: map[string]any{"Status": "accepted"}, key: "status"},
		{name: "a node without attributes", key: "status"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &Node{ID: "0001", Attrs: tt.attrs}

			got, ok := n.Attr(tt.key)

			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if got != tt.want {
				t.Errorf("Attr(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestGraphNode(t *testing.T) {
	g := testGraph()

	t.Run("a known identifier", func(t *testing.T) {
		n, ok := g.Node("0002")

		if !ok {
			t.Fatal("Node(0002) reported absent, want the document")
		}
		if n.Title != "Second" {
			t.Errorf("title = %q, want Second", n.Title)
		}
	})

	t.Run("an unknown identifier", func(t *testing.T) {
		if n, ok := g.Node("0099"); ok {
			t.Fatalf("Node(0099) = %+v, want absent", n)
		}
	})

	t.Run("an empty graph", func(t *testing.T) {
		if n, ok := NewGraph().Node("0001"); ok {
			t.Fatalf("Node(0001) = %+v, want absent", n)
		}
	})
}

func TestGraphNodeIDs(t *testing.T) {
	tests := []struct {
		name string
		g    *Graph
		want []ID
	}{
		{name: "identifiers come back ascending whatever the insertion order", g: testGraph(), want: []ID{"0001", "0002", "0003"}},
		{name: "an empty graph has no identifiers", g: NewGraph(), want: []ID{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.g.NodeIDs(); !slices.Equal(got, tt.want) {
				t.Fatalf("NodeIDs = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("repeated calls agree", func(t *testing.T) {
		g := testGraph()

		if first, second := g.NodeIDs(), g.NodeIDs(); !slices.Equal(first, second) {
			t.Fatalf("NodeIDs = %v then %v, want one order", first, second)
		}
	})
}

func TestGraphEdgesOfType(t *testing.T) {
	g := testGraph()
	tests := []struct {
		name string
		t    EdgeType
		want []Edge
	}{
		{
			name: "one type",
			t:    "supersedes",
			want: []Edge{
				{From: "0002", To: "0001", Type: "supersedes", Origin: OriginStructured},
				{From: "0003", To: "0002", Type: "supersedes", Origin: OriginDerived},
			},
		},
		{
			name: "another type",
			t:    "depends-on",
			want: []Edge{{From: "0003", To: "0001", Type: "depends-on", Origin: OriginStructured}},
		},
		{name: "an empty type is every typed edge", t: "", want: g.Edges},
		{name: "an undeclared type has no edges", t: "relates-to", want: []Edge{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := g.EdgesOfType(tt.t); !slices.Equal(got, tt.want) {
				t.Fatalf("EdgesOfType(%q) = %+v, want %+v", tt.t, got, tt.want)
			}
		})
	}

	t.Run("the graph keeps its own edge slice", func(t *testing.T) {
		clone := g.EdgesOfType("")
		clone[0] = Edge{From: "9999", To: "9999", Type: "supersedes"}

		if g.Edges[0].From != "0002" {
			t.Fatalf("edges[0] = %+v, want the graph unchanged", g.Edges[0])
		}
	})
}

func TestSeverityRank(t *testing.T) {
	tests := []struct {
		name string
		s    Severity
		want int
	}{
		{name: "errors come first", s: SeverityError, want: 0},
		{name: "warnings come second", s: SeverityWarn, want: 1},
		{name: "an unknown severity sorts last", s: "notice", want: 2},
		{name: "an empty severity sorts last", want: 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Rank(); got != tt.want {
				t.Fatalf("Rank(%q) = %d, want %d", tt.s, got, tt.want)
			}
		})
	}
}

func TestIDAndEdgeTypeString(t *testing.T) {
	if got := ID("0001").String(); got != "0001" {
		t.Errorf("ID.String = %q, want 0001", got)
	}
	if got := EdgeType("supersedes").String(); got != "supersedes" {
		t.Errorf("EdgeType.String = %q, want supersedes", got)
	}
}
