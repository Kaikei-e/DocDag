package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/render"
)

// testHit is a query result reduced to what a fixture-independent expectation
// can name: the document and the layer it was reached through.
type testHit struct {
	id        model.ID
	reference bool
}

func queryResults(t *testing.T, payload string) []testHit {
	t.Helper()
	records := decodeJSON[[]render.Record](t, payload)
	out := make([]testHit, 0, len(records))
	for _, r := range records {
		out = append(out, testHit{id: r.ID, reference: r.Reference})
	}
	return out
}

func assertResults(t *testing.T, got []testHit, want []testHit) {
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

func typed(ids ...model.ID) []testHit {
	out := make([]testHit, 0, len(ids))
	for _, id := range ids {
		out = append(out, testHit{id: id})
	}
	return out
}

func TestQueryReachability(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		args    []string
		want    []testHit
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
	assertResults(t, queryResults(t, with.stdout), []testHit{
		{id: "0002", reference: true},
	})
}

func TestQueryBindingJSONCarriesEveryField(t *testing.T) {
	got := run(t, "query", "--binding", "--format", "json", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	records := decodeJSON[[]render.Record](t, got.stdout)
	if len(records) != 2 {
		t.Fatalf("records = %+v, want two binding documents", records)
	}
	want := render.Record{ID: "0001", Title: "Cache rendered thumbnails", Status: "accepted"}
	got0 := records[0]
	if got0.ID != want.ID || got0.Title != want.Title || got0.Status != want.Status {
		t.Errorf("record = %+v, want %+v", got0, want)
	}
	if filepath.Base(got0.Path) != "0001-cache-rendered-thumbnails.md" {
		t.Errorf("path = %q, want the document file", got0.Path)
	}
	if got0.Reference {
		t.Errorf("record = %+v, want no reference marker on a binding document", got0)
	}
}

func TestQueryFieldsSelectTheTextColumns(t *testing.T) {
	dir := fixture(t, "ok-madr")

	bare := run(t, "query", "--binding", "--dir", dir)
	assertExit(t, bare, 0)
	assertLines(t, "binding", lines(bare.stdout), []string{"0001", "0003"})

	got := run(t, "query", "--binding", "--fields", "id,title,status", "--dir", dir)

	assertExit(t, got, 0)
	assertLines(t, "binding", lines(got.stdout), []string{
		"0001\tCache rendered thumbnails\taccepted",
		"0003\tStore thumbnails in object storage\taccepted",
	})
}

func TestQueryFieldsKeepTheGivenOrder(t *testing.T) {
	got := run(t, "query", "0004", "--descendants", "--fields", "status,id", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	assertLines(t, "query", lines(got.stdout), []string{"accepted\t0003"})
}

func TestQueryFieldsMarkAReferenceLayerHit(t *testing.T) {
	got := run(t, "query", "0001", "--descendants", "--include-refs", "--fields", "id,title", "--dir", fixture(t, "ok-basic"))

	assertExit(t, got, 0)
	all := lines(got.stdout)
	if len(all) != 1 || !strings.HasSuffix(all[0], " (reference)") {
		t.Errorf("query = %q, want the reference-layer hit marked", got.stdout)
	}
}

func TestUnknownFieldIsAUsageError(t *testing.T) {
	for _, args := range [][]string{
		{"query", "--binding", "--fields", "id,colour"},
		{"resolve", "0001", "--fields", "colour"},
	} {
		t.Run(args[0], func(t *testing.T) {
			got := run(t, append(args, "--dir", fixture(t, "ok-basic"))...)

			assertExit(t, got, 2)
			if !strings.Contains(got.stderr, "unknown field") {
				t.Errorf("stderr = %q, want an unknown field diagnostic", got.stderr)
			}
		})
	}
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
	records := decodeJSON[[]render.Record](t, got.stdout)
	if len(records) != 1 || records[0].ID != model.ID("0003") {
		t.Errorf("records = %+v, want [0003]", records)
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
