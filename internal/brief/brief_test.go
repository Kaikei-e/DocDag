package brief

import (
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

func TestSectionTakesTheFirstParagraphUnderTheHeading(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		section string
		want    string
	}{
		{
			name:    "an exact heading",
			body:    "# Title\n\n## Decision\n\nWe chose the boring option.\n\nAnd then some more.\n",
			section: "Decision",
			want:    "We chose the boring option.",
		},
		{
			name:    "the MADR outcome heading beats a sibling sharing the prefix",
			body:    "## Decision Drivers\n\n- cost\n\n## Decision Outcome\n\nChosen option: the bucket.\n",
			section: "Decision",
			want:    "Chosen option: the bucket.",
		},
		{
			name:    "an H3 counts",
			body:    "### Decision\n\nA smaller heading still names the section.\n",
			section: "Decision",
			want:    "A smaller heading still names the section.",
		},
		{
			name:    "the match is case insensitive",
			body:    "## CONSEQUENCES\n\nThe bill grows.\n",
			section: "consequences",
			want:    "The bill grows.",
		},
		{
			name:    "a paragraph keeps its own line breaks",
			body:    "## Decision\n\nOne line.\nAnd its continuation.\n\nA second paragraph.\n",
			section: "Decision",
			want:    "One line.\nAnd its continuation.",
		},
		{
			name:    "a following heading ends the paragraph",
			body:    "## Decision\nStraight after the heading.\n## Consequences\n\nLater.\n",
			section: "Decision",
			want:    "Straight after the heading.",
		},
		{
			name:    "a heading inside a fenced block is not a heading",
			body:    "## Context\n\n```md\n## Decision\n\nNot this one.\n```\n\n## Decision\n\nThis one.\n",
			section: "Decision",
			want:    "This one.",
		},
		{
			name:    "an H1 is not a section",
			body:    "# Decision\n\nThe document title is not a section.\n",
			section: "Decision",
			want:    "",
		},
		{
			name:    "an absent section has no paragraph",
			body:    "## Context\n\nNothing was decided.\n",
			section: "Decision",
			want:    "",
		},
		{
			name:    "an empty section has no paragraph",
			body:    "## Decision\n\n## Consequences\n\nLater.\n",
			section: "Decision",
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Section(tt.body, tt.section); got != tt.want {
				t.Errorf("Section = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildReportsTheReferenceItself(t *testing.T) {
	b := testBuild(t, "ok-madr", "0002", Options{Depth: 1})

	if b.SchemaVersion != SchemaVersion {
		t.Errorf("schemaVersion = %d, want %d", b.SchemaVersion, SchemaVersion)
	}
	// The brief is headed by the revision the corpus was read under, so a
	// caller can pin what it was handed.
	if b.PresetVersion != config.ADRPresetVersion {
		t.Errorf("presetVersion = %d, want the preset's %d", b.PresetVersion, config.ADRPresetVersion)
	}
	if b.Ref.ID != model.ID("0002") {
		t.Errorf("ref = %+v, want 0002", b.Ref)
	}
	if b.Ref.Title != "Store thumbnails on the local disk" {
		t.Errorf("ref title = %q, want the frontmatter title", b.Ref.Title)
	}
	if b.Ref.Status != "superseded" {
		t.Errorf("ref status = %q, want the projected status", b.Ref.Status)
	}
	if !strings.HasPrefix(b.Ref.Excerpt, "Chosen option: a directory on the local disk") {
		t.Errorf("ref excerpt = %q, want the decision outcome paragraph", b.Ref.Excerpt)
	}
}

func TestBuildResolvesASupersededReference(t *testing.T) {
	b := testBuild(t, "ok-madr", "0002", Options{Depth: 1})

	testAssertIDs(t, "resolvesTo", b.ResolvesTo, []string{"0003"})
	// The successor is already reported as the resolution, so the neighbourhood
	// must not spend the budget on it twice.
	testAssertIDs(t, "ancestors", b.Ancestors, nil)
	testAssertIDs(t, "descendants", b.Descendants, nil)
}

func TestBuildLeavesACurrentReferenceUnresolved(t *testing.T) {
	b := testBuild(t, "ok-madr", "0004", Options{Depth: 1})

	testAssertIDs(t, "resolvesTo", b.ResolvesTo, nil)
	testAssertIDs(t, "descendants", b.Descendants, []string{"0003"})
}

func TestBuildWalksToTheRequestedDepth(t *testing.T) {
	shallow := testBuild(t, "ok-madr", "0001", Options{Depth: 1})
	testAssertIDs(t, "ancestors at depth 1", shallow.Ancestors, []string{"0003"})

	deep := testBuild(t, "ok-madr", "0001", Options{Depth: 2, All: true})
	testAssertIDs(t, "ancestors at depth 2", deep.Ancestors, []string{"0003", "0004"})
}

func TestBuildKeepsOnlyBindingNeighboursUnlessAskedForAll(t *testing.T) {
	binding := testBuild(t, "ok-madr", "0001", Options{Depth: 2})
	testAssertIDs(t, "ancestors", binding.Ancestors, []string{"0003"})

	all := testBuild(t, "ok-madr", "0001", Options{Depth: 2, All: true})
	testAssertIDs(t, "ancestors", all.Ancestors, []string{"0003", "0004"})
}

func TestBuildRestrictsTheWalkToTheGivenEdgeTypes(t *testing.T) {
	b := testBuild(t, "ok-madr", "0001", Options{Depth: 2, All: true, Types: []model.EdgeType{"supersedes"}})

	testAssertIDs(t, "ancestors", b.Ancestors, nil)
}

func TestBuildSpendsTheWholeBudgetWhenItIsEnough(t *testing.T) {
	opts := Options{Depth: 2, All: true}
	full := testBuild(t, "ok-madr", "0001", opts)
	if full.Budget.Degraded != 0 {
		t.Fatalf("degraded = %d, want none without a limit", full.Budget.Degraded)
	}

	opts.Budget = full.Budget.Used
	exact := testBuild(t, "ok-madr", "0001", opts)

	if exact.Budget.Degraded != 0 {
		t.Errorf("degraded = %d, want none: the budget covers the report exactly", exact.Budget.Degraded)
	}
	if exact.Budget.Used != full.Budget.Used {
		t.Errorf("used = %d, want %d", exact.Budget.Used, full.Budget.Used)
	}
}

func TestBuildDegradesTheEntriesThatDoNotFitTheBudget(t *testing.T) {
	opts := Options{Depth: 2, All: true}
	full := testBuild(t, "ok-madr", "0001", opts)

	opts.Budget = full.Budget.Used - 1
	tight := testBuild(t, "ok-madr", "0001", opts)

	if tight.Budget.Limit != opts.Budget {
		t.Errorf("limit = %d, want %d", tight.Budget.Limit, opts.Budget)
	}
	if tight.Budget.Used > opts.Budget {
		t.Errorf("used = %d, want it inside the budget of %d", tight.Budget.Used, opts.Budget)
	}
	if tight.Budget.Degraded != 1 {
		t.Fatalf("degraded = %d, want the one entry that did not fit", tight.Budget.Degraded)
	}
	if tight.Ref.Excerpt != full.Ref.Excerpt {
		t.Error("the reference lost its excerpt, want the budget spent on it first")
	}
	// The budget drops whole excerpts from the end: a half sentence is worse
	// than a one-line entry.
	kept, dropped := testExcerptRun(t, full, tight)
	if dropped != 1 || kept != len(testEntries(full))-1 {
		t.Errorf("kept %d excerpts and dropped %d, want the last one dropped", kept, dropped)
	}
}

func TestBuildRejectsAnUnknownReference(t *testing.T) {
	g, cfg := testCorpus(t, "ok-madr")

	if _, err := Build(g, cfg, model.ID("0099"), Options{Depth: 1}); err == nil {
		t.Fatal("err = nil, want an unknown reference reported")
	}
}

func TestEntryLineNamesTheDocument(t *testing.T) {
	e := Entry{ID: "0002", Title: "Store thumbnails", Status: "superseded", Path: "docs/adr/0002-store.md"}

	want := "0002  Store thumbnails  [superseded]  docs/adr/0002-store.md"
	if got := e.Line(); got != want {
		t.Errorf("Line = %q, want %q", got, want)
	}
}
