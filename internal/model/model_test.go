package model

import (
	"encoding/json"
	"slices"
	"strings"
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

func TestNodeAttrList(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]any
		key   string
		want  []string
		ok    bool
	}{
		{
			name:  "a list of strings",
			attrs: map[string]any{"tags": []any{"security", "storage"}},
			key:   "tags",
			want:  []string{"security", "storage"},
			ok:    true,
		},
		{
			name:  "a scalar is a one element list",
			attrs: map[string]any{"tags": "security"},
			key:   "tags",
			want:  []string{"security"},
			ok:    true,
		},
		{
			name:  "list items stringify like scalars",
			attrs: map[string]any{"revisions": []any{1, true}},
			key:   "revisions",
			want:  []string{"1", "true"},
			ok:    true,
		},
		{
			name:  "an item that is not a scalar is dropped",
			attrs: map[string]any{"tags": []any{"security", []any{"nested"}}},
			key:   "tags",
			want:  []string{"security"},
			ok:    true,
		},
		{name: "an empty list is present", attrs: map[string]any{"tags": []any{}}, key: "tags", want: []string{}, ok: true},
		{name: "a mapping is not a list", attrs: map[string]any{"meta": map[string]any{"k": "v"}}, key: "meta"},
		{name: "an explicit null is absent", attrs: map[string]any{"tags": nil}, key: "tags"},
		{name: "a missing key is absent", attrs: map[string]any{"tags": []any{"security"}}, key: "labels"},
		{name: "a node without attributes", key: "tags"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := &Node{ID: "0001", Attrs: tt.attrs}

			got, ok := n.AttrList(tt.key)

			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v", ok, tt.ok)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("AttrList(%q) = %q, want %q", tt.key, got, tt.want)
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

func TestNodeLocation(t *testing.T) {
	n := &Node{
		ID:       "0001",
		Path:     "docs/adr/0001-a.md",
		Line:     1,
		KeyLines: map[string]int{"title": 2, "status": 3, "supersedes": 4},
	}
	tests := []struct {
		name string
		keys []string
		want Location
	}{
		{
			name: "no candidate key falls back to the opening delimiter",
			want: Location{Path: "docs/adr/0001-a.md", Line: 1},
		},
		{
			name: "a present key wins",
			keys: []string{"status"},
			want: Location{Path: "docs/adr/0001-a.md", Line: 3},
		},
		{
			name: "the first present candidate wins",
			keys: []string{"amends", "supersedes", "status"},
			want: Location{Path: "docs/adr/0001-a.md", Line: 4},
		},
		{
			name: "no candidate is present at all",
			keys: []string{"amends"},
			want: Location{Path: "docs/adr/0001-a.md", Line: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := n.Location(tt.keys...); got != tt.want {
				t.Fatalf("Location(%v) = %+v, want %+v", tt.keys, got, tt.want)
			}
		})
	}
}

func TestNodeLocationWithoutPositions(t *testing.T) {
	n := &Node{ID: "0001", Path: "0001.md"}

	if got := n.Location("status"); got != (Location{Path: "0001.md"}) {
		t.Fatalf("Location = %+v, want only the path", got)
	}
}

func TestFindingJSONShape(t *testing.T) {
	f := Finding{
		Severity: SeverityError,
		Rule:     RuleStatusDrift,
		ID:       "0001",
		Detail:   "drifted",
		Location: Location{Path: "0001.md", Line: 3},
		Related:  []Location{{Path: "0002.md", Line: 4}},
	}

	payload, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, want := range []string{
		`"severity":"error"`,
		`"rule":"status_drift"`,
		`"id":"0001"`,
		`"detail":"drifted"`,
		`"location":{"path":"0001.md","line":3}`,
		`"related":[{"path":"0002.md","line":4}]`,
	} {
		if !strings.Contains(string(payload), want) {
			t.Errorf("finding JSON = %s, want it to contain %s", payload, want)
		}
	}
	if strings.Contains(string(payload), `"fix"`) {
		t.Errorf("finding JSON = %s, want an empty fix omitted", payload)
	}

	bare, err := json.Marshal(Finding{Location: Location{Path: "0001.md"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(bare), `"line"`) || strings.Contains(string(bare), `"column"`) {
		t.Errorf("finding JSON = %s, want an unknown line and column omitted", bare)
	}
	if strings.Contains(string(bare), `"related"`) {
		t.Errorf("finding JSON = %s, want an empty related list omitted", bare)
	}
}
