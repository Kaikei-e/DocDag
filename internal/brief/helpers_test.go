package brief

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

// testCorpus loads a fixture the way the CLI does, so an excerpt comes from the
// file a reader would open.
func testCorpus(t *testing.T, fixture string) (*model.Graph, config.Config) {
	t.Helper()
	cfg := config.ADRPreset()
	cfg.Dir = filepath.Join("..", "..", "testdata", "fixtures", fixture)
	docs, err := parse.Dir(cfg.Dir, cfg)
	if err != nil {
		t.Fatalf("parse %s: %v", cfg.Dir, err)
	}
	root, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	parse.Localize(docs, root)
	return graph.Build(docs, cfg), cfg
}

func testBuild(t *testing.T, fixture, ref string, opts Options) *Brief {
	t.Helper()
	g, cfg := testCorpus(t, fixture)
	b, err := Build(g, cfg, model.ID(ref), opts)
	if err != nil {
		t.Fatalf("Build %s: %v", ref, err)
	}
	return b
}

// testEntries flattens a brief into the order the budget is spent in.
func testEntries(b *Brief) []Entry {
	out := []Entry{b.Ref}
	out = append(out, b.ResolvesTo...)
	out = append(out, b.Ancestors...)
	return append(out, b.Descendants...)
}

// testExcerptRun counts the excerpts a tightened brief kept and dropped, and
// fails when a kept one follows a dropped one.
func testExcerptRun(t *testing.T, full, tight *Brief) (kept, dropped int) {
	t.Helper()
	before, after := testEntries(full), testEntries(tight)
	if len(before) != len(after) {
		t.Fatalf("tightened brief has %d entries, want %d: a budget degrades entries, it does not drop them", len(after), len(before))
	}
	for i := range before {
		switch {
		case before[i].Excerpt == "":
			continue
		case after[i].Excerpt == "":
			dropped++
		case dropped > 0:
			t.Fatalf("entry %d kept its excerpt after %d were dropped", i, dropped)
		default:
			kept++
		}
	}
	return kept, dropped
}

func testIDs(entries []Entry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.ID.String())
	}
	return out
}

func testAssertIDs(t *testing.T, what string, got []Entry, want []string) {
	t.Helper()
	ids := testIDs(got)
	if len(ids) != len(want) {
		t.Fatalf("%s = %v, want %v", what, ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("%s = %v, want %v", what, ids, want)
		}
	}
}
