package cmd

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/render"
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

// testFieldDocs is a two-document corpus whose frontmatter carries one retired
// field and one nobody declared.
func testFieldDocs() map[string]string {
	return map[string]string{
		"docs/adr/0001-name-a-service-owner.md": "---\ntitle: Name a service owner\nstatus: accepted\nowner: platform\ndate: 2025-01-01\n---\n\n# Name a service owner\n",
		"docs/adr/0002-tag-decisions.md":        "---\ntitle: Tag decisions\nstatus: accepted\nowner: payments\ntags: [legacy]\ndate: 2025-02-01\n---\n\n# Tag decisions\n",
		"docdag.yaml":                           "preset_version: 3\ndir: docs/adr\nfields:\n  owner: {deprecated: true, since: 2, migrate_to: owned-by}\n  team: {deprecated: true}\n",
	}
}

func testStatsFieldsRow(t *testing.T, out, field string) string {
	t.Helper()
	for _, line := range lines(out) {
		if strings.HasPrefix(line, field+" ") || line == field {
			return line
		}
	}
	t.Fatalf("field %q is not in the report %q", field, out)
	return ""
}

func TestStatsFieldsText(t *testing.T) {
	dir := writeDocs(t, testFieldDocs())
	t.Chdir(dir)

	got := run(t, "stats", "--fields")

	assertExit(t, got, 0)
	all := lines(got.stdout)
	if len(all) == 0 || !strings.HasPrefix(all[0], "field ") {
		t.Fatalf("report = %q, want a header row naming the columns", got.stdout)
	}
	for _, want := range []string{"documents", "last change", "deprecated"} {
		if !strings.Contains(all[0], want) {
			t.Errorf("header = %q, want it to name %q", all[0], want)
		}
	}
	owner := testStatsFieldsRow(t, got.stdout, "owner")
	if !strings.Contains(owner, "2") || !strings.Contains(owner, "yes") {
		t.Errorf("owner row = %q, want two documents and the retirement flagged", owner)
	}
	// Outside a repository there is no day to report, and the report says so
	// rather than failing.
	if !strings.Contains(owner, "-") {
		t.Errorf("owner row = %q, want a dash where git answered nothing", owner)
	}
	if row := testStatsFieldsRow(t, got.stdout, "team"); !strings.Contains(row, "0") {
		t.Errorf("team row = %q, want the declared field nobody writes counted at zero", row)
	}
	if row := testStatsFieldsRow(t, got.stdout, "tags"); strings.Contains(row, "yes") {
		t.Errorf("tags row = %q, want an undeclared field left unflagged", row)
	}
}

func TestStatsFieldsJSON(t *testing.T) {
	dir := writeDocs(t, testFieldDocs())
	t.Chdir(dir)

	got := run(t, "stats", "--fields", "--format", "json")

	assertExit(t, got, 0)
	report := decodeJSON[render.FieldUsageReport](t, got.stdout)
	byName := map[string]graph.FieldUsage{}
	for _, u := range report.Fields {
		byName[u.Field] = u
	}
	want := map[string]graph.FieldUsage{
		"owner": {Field: "owner", Documents: 2, Deprecated: true},
		"tags":  {Field: "tags", Documents: 1},
		"team":  {Field: "team", Documents: 0, Deprecated: true},
	}
	for field, u := range want {
		if !reflect.DeepEqual(byName[field], u) {
			t.Errorf("%s = %+v, want %+v", field, byName[field], u)
		}
	}
	if _, listed := byName["title"]; !listed {
		t.Errorf("fields = %+v, want the keys nobody declared counted too", report.Fields)
	}
}

func TestStatsFieldsDatesTheDocumentsFromGit(t *testing.T) {
	dir := gitRepo(t, testFieldDocs())
	t.Chdir(dir)

	got := run(t, "stats", "--fields")

	assertExit(t, got, 0)
	day := time.Now().Format("2006-01-02")
	owner := testStatsFieldsRow(t, got.stdout, "owner")
	if !strings.Contains(owner, day) {
		t.Errorf("owner row = %q, want the commit day %s", owner, day)
	}
	if row := testStatsFieldsRow(t, got.stdout, "team"); strings.Contains(row, day) {
		t.Errorf("team row = %q, want no day for a field no document writes", row)
	}
}

func TestStatsFieldsAnswersACorpusWithoutSupersedes(t *testing.T) {
	// The field report is about frontmatter, not degrees, so the edge type the
	// degree statistics need is beside the point.
	files := testFieldDocs()
	files["docdag.yaml"] = "dir: docs/adr\nedges: []\nrules: []\nderived_edges: []\nfields:\n  owner: {deprecated: true}\n"
	dir := writeDocs(t, files)
	t.Chdir(dir)

	assertExit(t, run(t, "stats", "--fields"), 0)
	assertExit(t, run(t, "stats"), 3)
}

func TestStatsWithoutFieldsIsUnchanged(t *testing.T) {
	dir := fixture(t, "ok-basic")
	plain := run(t, "stats", "--dir", dir)
	explicit := run(t, "stats", "--fields=false", "--dir", dir)

	assertExit(t, plain, 0)
	if plain.stdout != explicit.stdout {
		t.Errorf("stats = %q, want the degree report unchanged by the flag's default", plain.stdout)
	}
	if strings.Contains(plain.stdout, "last change") {
		t.Errorf("stats = %q, want no field report without --fields", plain.stdout)
	}
}
