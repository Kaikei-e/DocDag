package graph

import (
	"slices"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/internal/model"
)

func TestFindCycle(t *testing.T) {
	tests := []struct {
		name string
		adj  map[string][]string
		want []string
	}{
		{
			name: "empty graph is acyclic",
			adj:  map[string][]string{},
		},
		{
			name: "isolated node is acyclic",
			adj:  map[string][]string{"a": nil},
		},
		{
			name: "chain is acyclic",
			adj:  map[string][]string{"a": {"b"}, "b": {"c"}, "c": nil},
		},
		{
			name: "diamond is acyclic",
			adj:  map[string][]string{"a": {"b", "c"}, "b": {"d"}, "c": {"d"}, "d": nil},
		},
		{
			name: "edges to nodes without an entry are acyclic",
			adj:  map[string][]string{"a": {"missing"}},
		},
		{
			name: "self loop is a cycle",
			adj:  map[string][]string{"a": {"a"}},
			want: []string{"a", "a"},
		},
		{
			name: "two node cycle",
			adj:  map[string][]string{"a": {"b"}, "b": {"a"}},
			want: []string{"a", "b", "a"},
		},
		{
			name: "three node cycle",
			adj:  map[string][]string{"a": {"b"}, "b": {"c"}, "c": {"a"}},
			want: []string{"a", "b", "c", "a"},
		},
		{
			name: "cycle behind an acyclic prefix reports only the cycle",
			adj:  map[string][]string{"a": {"b"}, "b": {"c"}, "c": {"d"}, "d": {"b"}},
			want: []string{"b", "c", "d", "b"},
		},
		{
			name: "the lowest reachable cycle wins across disjoint cycles",
			adj:  map[string][]string{"a": {"b"}, "b": {"a"}, "y": {"z"}, "z": {"y"}},
			want: []string{"a", "b", "a"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adj := testAdj(tt.adj)
			got := FindCycle(adj)

			if tt.want == nil {
				if len(got) != 0 {
					t.Fatalf("FindCycle = %v, want no cycle", got)
				}
				return
			}
			testAssertIDs(t, "FindCycle", got, testIDs(tt.want...))
			testAssertClosedCyclePath(t, adj, got)
		})
	}
}

func TestFindCycleIsDeterministic(t *testing.T) {
	adj := testAdj(map[string][]string{
		"a": {"b", "c"},
		"b": {"d"},
		"c": {"d"},
		"d": {"b"},
	})

	first := FindCycle(adj)
	if len(first) == 0 {
		t.Fatalf("FindCycle = %v, want a cycle", first)
	}
	for i := 0; i < 50; i++ {
		if got := FindCycle(adj); !slices.Equal(got, first) {
			t.Fatalf("run %d returned %v, want %v (map iteration order must not leak)", i, got, first)
		}
	}
}

func TestFindCycleOnDeepChain(t *testing.T) {
	const n = 10000

	t.Run("a ten thousand node chain is acyclic and does not overflow the stack", func(t *testing.T) {
		adj := testDeepChain(n, false)

		var got []model.ID
		testMustNotHang(t, 30*time.Second, func() { got = FindCycle(adj) })

		if len(got) != 0 {
			t.Fatalf("FindCycle = %v, want no cycle", got)
		}
	})

	t.Run("a ten thousand node loop reports the whole cycle", func(t *testing.T) {
		adj := testDeepChain(n, true)

		var got []model.ID
		testMustNotHang(t, 30*time.Second, func() { got = FindCycle(adj) })

		if len(got) != n+1 {
			t.Fatalf("cycle length = %d, want %d", len(got), n+1)
		}
		if got[0] != model.ID(testChainID(0)) || got[len(got)-1] != got[0] {
			t.Fatalf("cycle = %v...%v, want a closed path starting at %s", got[0], got[len(got)-1], testChainID(0))
		}
		testAssertClosedCyclePath(t, adj, got)
	})
}

func TestFindCycles(t *testing.T) {
	t.Run("acyclic graph reports nothing", func(t *testing.T) {
		got := FindCycles(testAdj(map[string][]string{"a": {"b"}, "b": {"c"}, "c": nil}))

		if len(got) != 0 {
			t.Fatalf("FindCycles = %v, want none", got)
		}
	})

	t.Run("one cycle per strongly connected component in deterministic order", func(t *testing.T) {
		adj := testAdj(map[string][]string{
			"a": {"b"},
			"b": {"a"},
			"m": {"n"},
			"n": nil,
			"y": {"z"},
			"z": {"y"},
		})

		got := FindCycles(adj)

		if len(got) != 2 {
			t.Fatalf("FindCycles = %v, want 2 cycles", got)
		}
		testAssertIDs(t, "FindCycles[0]", got[0], testIDs("a", "b", "a"))
		testAssertIDs(t, "FindCycles[1]", got[1], testIDs("y", "z", "y"))
		for _, cycle := range got {
			testAssertClosedCyclePath(t, adj, cycle)
		}
	})

	t.Run("one component with a cycle reports it once", func(t *testing.T) {
		adj := testAdj(map[string][]string{
			"a": {"b"},
			"b": {"c"},
			"c": {"b"},
		})

		got := FindCycles(adj)

		if len(got) != 1 {
			t.Fatalf("FindCycles = %v, want 1 cycle", got)
		}
		testAssertIDs(t, "FindCycles[0]", got[0], testIDs("b", "c", "b"))
	})

	t.Run("repeated runs agree", func(t *testing.T) {
		adj := testAdj(map[string][]string{
			"a": {"b"},
			"b": {"a"},
			"y": {"z"},
			"z": {"y"},
		})

		first := FindCycles(adj)
		for i := 0; i < 20; i++ {
			got := FindCycles(adj)
			if len(got) != len(first) {
				t.Fatalf("run %d returned %d cycles, want %d", i, len(got), len(first))
			}
			for j := range got {
				if !slices.Equal(got[j], first[j]) {
					t.Fatalf("run %d cycle %d = %v, want %v", i, j, got[j], first[j])
				}
			}
		}
	})
}

func testDeepChain(n int, closeLoop bool) map[model.ID][]model.ID {
	adj := make(map[model.ID][]model.ID, n)
	for i := 0; i < n-1; i++ {
		adj[model.ID(testChainID(i))] = testIDs(testChainID(i + 1))
	}
	if closeLoop {
		adj[model.ID(testChainID(n-1))] = testIDs(testChainID(0))
	} else {
		adj[model.ID(testChainID(n-1))] = nil
	}
	return adj
}

func testAssertClosedCyclePath(t *testing.T, adj map[model.ID][]model.ID, cycle []model.ID) {
	t.Helper()
	if len(cycle) < 2 {
		t.Fatalf("cycle = %v, want at least two entries", cycle)
	}
	if cycle[0] != cycle[len(cycle)-1] {
		t.Fatalf("cycle = %v, want a closed path", cycle)
	}
	for i := 0; i+1 < len(cycle); i++ {
		if !slices.Contains(adj[cycle[i]], cycle[i+1]) {
			t.Fatalf("cycle step %s -> %s is not an edge", cycle[i], cycle[i+1])
		}
	}
	seen := make(map[model.ID]bool, len(cycle))
	for _, id := range cycle[:len(cycle)-1] {
		if seen[id] {
			t.Fatalf("cycle = %v, want each node once before it closes", cycle)
		}
		seen[id] = true
	}
}
