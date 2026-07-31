package cmd

import (
	"math"
	"reflect"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
)

func TestStatsJSON(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		want    graph.Statistics
	}{
		{
			name:    "supersedes chain and dependencies",
			fixture: "ok-basic",
			want: graph.Statistics{
				Documents: 6,
				Edges: []graph.EdgeCount{
					{Type: config.EdgeSupersedes, Count: 2},
					{Type: config.EdgeDependsOn, Count: 2},
				},
				Binding:    3,
				ChainDepth: []graph.DepthCount{{Depth: 0, Count: 4}, {Depth: 1, Count: 1}, {Depth: 2, Count: 1}},
				Orphans:    1,
				OrphanRate: 1.0 / 6.0,
				TopReferenced: []graph.ReferenceCount{
					{ID: "0003", Count: 2},
					{ID: "0001", Count: 1},
					{ID: "0002", Count: 1},
				},
			},
		},
		{
			name:    "fan-in has no reference layer",
			fixture: "fan-in",
			want: graph.Statistics{
				Documents: 3,
				Edges: []graph.EdgeCount{
					{Type: config.EdgeSupersedes, Count: 2},
					{Type: config.EdgeDependsOn, Count: 0},
				},
				Binding:    1,
				ChainDepth: []graph.DepthCount{{Depth: 0, Count: 2}, {Depth: 1, Count: 1}},
				Orphans:    0,
				OrphanRate: 0,
			},
		},
		{
			name:    "derived edges and markdown links count",
			fixture: "ok-madr",
			want: graph.Statistics{
				Documents: 4,
				Edges: []graph.EdgeCount{
					{Type: config.EdgeSupersedes, Count: 1},
					{Type: config.EdgeDependsOn, Count: 2},
				},
				Binding:       2,
				ChainDepth:    []graph.DepthCount{{Depth: 0, Count: 3}, {Depth: 1, Count: 1}},
				Orphans:       0,
				OrphanRate:    0,
				TopReferenced: []graph.ReferenceCount{{ID: "0001", Count: 1}},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, "stats", "--format", "json", "--dir", fixture(t, tt.fixture))
			assertExit(t, got, 0)
			stats := decodeJSON[graph.Statistics](t, got.stdout)

			if stats.Documents != tt.want.Documents {
				t.Errorf("documents = %d, want %d", stats.Documents, tt.want.Documents)
			}
			if !reflect.DeepEqual(stats.Edges, tt.want.Edges) {
				t.Errorf("edges = %+v, want %+v in declaration order", stats.Edges, tt.want.Edges)
			}
			if stats.Binding != tt.want.Binding {
				t.Errorf("binding = %d, want %d", stats.Binding, tt.want.Binding)
			}
			if !reflect.DeepEqual(stats.ChainDepth, tt.want.ChainDepth) {
				t.Errorf("chain depth = %+v, want %+v", stats.ChainDepth, tt.want.ChainDepth)
			}
			if stats.Orphans != tt.want.Orphans {
				t.Errorf("orphans = %d, want %d", stats.Orphans, tt.want.Orphans)
			}
			if math.Abs(stats.OrphanRate-tt.want.OrphanRate) > 1e-9 {
				t.Errorf("orphan rate = %v, want %v", stats.OrphanRate, tt.want.OrphanRate)
			}
			if len(stats.TopReferenced) != len(tt.want.TopReferenced) {
				t.Fatalf("top referenced = %+v, want %+v", stats.TopReferenced, tt.want.TopReferenced)
			}
			for i, want := range tt.want.TopReferenced {
				if stats.TopReferenced[i] != want {
					t.Errorf("top referenced %d = %+v, want %+v", i, stats.TopReferenced[i], want)
				}
			}
		})
	}
}

func TestStatsTopReferencedIsCapped(t *testing.T) {
	// Twelve referenced documents, so the ranking has more entries than the cap.
	files := map[string]string{}
	for i := 1; i <= 12; i++ {
		id := padTestID(i)
		target := padTestID(i + 100)
		files[id+"-source.md"] = "---\ntitle: Source " + id + "\nstatus: accepted\ndate: 2025-01-01\n---\n\n# Source\n\nSee [[" + target + "]].\n"
		files[target+"-target.md"] = "---\ntitle: Target " + target + "\nstatus: accepted\ndate: 2025-01-02\n---\n\n# Target\n"
	}

	dir := writeDocs(t, files)
	got := run(t, "stats", "--format", "json", "--dir", dir)
	assertExit(t, got, 0)
	stats := decodeJSON[graph.Statistics](t, got.stdout)
	if len(stats.TopReferenced) != graph.TopReferencedLimit {
		t.Errorf("top referenced holds %d entries, want the cap of %d", len(stats.TopReferenced), graph.TopReferencedLimit)
	}
}

func padTestID(n int) string {
	digits := []byte("0000")
	for i := len(digits) - 1; i >= 0 && n > 0; i-- {
		digits[i] = byte('0' + n%10)
		n /= 10
	}
	return string(digits)
}

func TestStatsText(t *testing.T) {
	got := run(t, "stats", "--dir", fixture(t, "ok-basic"))
	assertExit(t, got, 0)
	if len(lines(got.stdout)) == 0 {
		t.Error("stats printed nothing")
	}
	if got.stderr != "" {
		t.Errorf("stderr = %q, want empty", got.stderr)
	}
}

func TestStatsRejectsAnUnknownFormat(t *testing.T) {
	got := run(t, "stats", "--format", "mermaid", "--dir", fixture(t, "ok-basic"))
	assertExit(t, got, 2)
}
