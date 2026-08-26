package cmd

import (
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/model"
)

func queryResults(t *testing.T, payload string) []graph.QueryResult {
	t.Helper()
	return decodeJSON[[]graph.QueryResult](t, payload)
}

func assertResults(t *testing.T, got []graph.QueryResult, want []graph.QueryResult) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("results = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("result %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func typed(ids ...model.ID) []graph.QueryResult {
	out := make([]graph.QueryResult, 0, len(ids))
	for _, id := range ids {
		out = append(out, graph.QueryResult{ID: id, Layer: graph.LayerTyped})
	}
	return out
}

func TestQueryReachability(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		args    []string
		want    []graph.QueryResult
	}{
		{
			name:    "ancestors are the impacted documents",
			fixture: "depends-impact",
			args:    []string{"0001", "--ancestors"},
			want:    typed("0002", "0003", "0004"),
		},
		{
			name:    "descendants are the dependencies",
			fixture: "depends-impact",
			args:    []string{"0003", "--descendants"},
			want:    typed("0001", "0002"),
		},
		{
			name:    "descendants are the default direction",
			fixture: "depends-impact",
			args:    []string{"0003"},
			want:    typed("0001", "0002"),
		},
		{
			name:    "a dependency has no descendants",
			fixture: "depends-impact",
			args:    []string{"0001", "--descendants"},
			want:    nil,
		},
		{
			name:    "every typed edge is walked by default",
			fixture: "ok-basic",
			args:    []string{"0004", "--descendants"},
			want:    typed("0001", "0002", "0003"),
		},
		{
			name:    "edge filter keeps the supersedes chain only",
			fixture: "ok-basic",
			args:    []string{"0004", "--descendants", "--edge", "supersedes"},
			want:    typed("0001", "0002"),
		},
		{
			name:    "edge filter keeps the dependencies only",
			fixture: "ok-basic",
			args:    []string{"0004", "--descendants", "--edge", "depends-on"},
			want:    typed("0003"),
		},
		{
			name:    "ancestors over every typed edge",
			fixture: "ok-basic",
			args:    []string{"0003", "--ancestors"},
			want:    typed("0004", "0005"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"query"}, tt.args...)
			args = append(args, "--format", "json", "--dir", fixture(t, tt.fixture))
			got := run(t, args...)
			assertExit(t, got, 0)
			assertResults(t, queryResults(t, got.stdout), tt.want)
		})
	}
}

func TestQueryTextPrintsOneIdentifierPerLine(t *testing.T) {
	got := run(t, "query", "0001", "--ancestors", "--dir", fixture(t, "depends-impact"))
	assertExit(t, got, 0)
	assertLines(t, "query", lines(got.stdout), []string{"0002", "0003", "0004"})
}

func TestQueryIncludeRefsOverlaysTheReferenceLayer(t *testing.T) {
	dir := fixture(t, "ok-basic")

	without := run(t, "query", "0001", "--descendants", "--format", "json", "--dir", dir)
	assertExit(t, without, 0)
	assertResults(t, queryResults(t, without.stdout), nil)

	with := run(t, "query", "0001", "--descendants", "--include-refs", "--format", "json", "--dir", dir)
	assertExit(t, with, 0)
	assertResults(t, queryResults(t, with.stdout), []graph.QueryResult{
		{ID: "0002", Layer: graph.LayerReference},
	})
}

func TestQueryBindingSkipsAWithdrawnDecision(t *testing.T) {
	got := run(t, "query", "--binding", "--dir", fixture(t, "withdrawn"))

	assertExit(t, got, 0)
	assertLines(t, "binding", lines(got.stdout), []string{"0002"})
}

func TestQueryBinding(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		want    []string
	}{
		{name: "accepted documents nobody supersedes", fixture: "ok-basic", want: []string{"0003", "0004", "0005"}},
		{name: "one binding document after a fan-in", fixture: "fan-in", want: []string{"0003"}},
		{name: "derived supersedes removes the binding", fixture: "ok-madr", want: []string{"0001", "0003"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, "query", "--binding", "--dir", fixture(t, tt.fixture))
			assertExit(t, got, 0)
			assertLines(t, "binding", lines(got.stdout), tt.want)
		})
	}
}

func TestQueryBindingJSON(t *testing.T) {
	got := run(t, "query", "--binding", "--format", "json", "--dir", fixture(t, "fan-in"))
	assertExit(t, got, 0)
	ids := decodeJSON[[]model.ID](t, got.stdout)
	if len(ids) != 1 || ids[0] != model.ID("0003") {
		t.Errorf("ids = %v, want [0003]", ids)
	}
}

func TestQueryArgumentErrors(t *testing.T) {
	dir := fixture(t, "ok-basic")
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "neither reference nor binding", args: []string{"query"}, want: "binding"},
		{name: "binding with a reference", args: []string{"query", "--binding", "0001"}, want: "binding"},
		{name: "both directions", args: []string{"query", "0001", "--ancestors", "--descendants"}, want: "mutually exclusive"},
		{name: "unknown edge type", args: []string{"query", "0001", "--edge", "relates-to"}, want: "unknown edge type"},
		{name: "binding with a direction", args: []string{"query", "--binding", "--ancestors"}, want: "binding"},
		{name: "binding with an edge filter", args: []string{"query", "--binding", "--edge", "supersedes"}, want: "binding"},
		{name: "binding with the reference layer", args: []string{"query", "--binding", "--include-refs"}, want: "binding"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, append(tt.args, "--dir", dir)...)
			assertExit(t, got, 2)
			if !strings.Contains(got.stderr, tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", got.stderr, tt.want)
			}
		})
	}
}

func TestQueryUnknownReference(t *testing.T) {
	got := run(t, "query", "0099", "--descendants", "--dir", fixture(t, "ok-basic"))
	assertExit(t, got, 1)
	if !strings.Contains(got.stderr, "unknown document") {
		t.Errorf("stderr = %q, want it to contain %q", got.stderr, "unknown document")
	}
}
