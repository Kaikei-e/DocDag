package brief

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/parse"
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
		{
			name:    "a child heading followed by a fenced code block returns the child heading text",
			body:    "## 決定\n\n### D1. 辺仕様に target を追加する\n\n```yaml\nedges:\n  - name: enforces\n```\n\n- prose after block\n",
			section: "決定",
			want:    "D1. 辺仕様に target を追加する",
		},
		{
			name:    "a child heading followed by prose returns heading and first paragraph",
			body:    "## 決定\n\n### D1. 辺仕様に target を追加する\n\n対象文書に対する条件を宣言する。\n次の行。\n\n二段落目。\n",
			section: "決定",
			want:    "D1. 辺仕様に target を追加する\n対象文書に対する条件を宣言する。\n次の行。",
		},
		{
			name:    "a child heading followed by another heading returns only the child heading",
			body:    "## 決定\n\n### D1. First ###\n\n### D2. Second\n\nBody.\n",
			section: "決定",
			want:    "D1. First",
		},
		{
			name:    "a child heading followed by tilde fence stops before fence and ignores headings inside fence",
			body:    "## 決定\n\n### D1. Tilde fence\n\n~~~md\n# fake\n~~~\n",
			section: "決定",
			want:    "D1. Tilde fence",
		},
		{
			name:    "an H3 section returns H4 child heading and prose",
			body:    "### Decision\n\n#### Option A\n\nUse Option A.\n",
			section: "Decision",
			want:    "Option A\nUse Option A.",
		},
		{
			name:    "an H3 section stops empty before a sibling H3 or parent H2",
			body:    "### Decision\n\n## Consequences\n\nLater.\n",
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

func TestDefaultExcerptFallbacks(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "English MADR prefers Decision Outcome",
			body: "# Title\n\n## Decision Drivers\n\n- cost\n\n## Decision Outcome\n\nChosen option: S3.\n",
			want: "Chosen option: S3.",
		},
		{
			name: "Japanese decision section with child heading and code block",
			body: "# 0002\n\n## ステータス\n\nAccepted\n\n## コンテキスト\n\n背景。\n\n## 決定\n\n### D1. 辺仕様に target を追加する\n\n```yaml\nedges: []\n```\n",
			want: "D1. 辺仕様に target を追加する",
		},
		{
			name: "Japanese summary fallback headings",
			body: "# 0001\n\n## 結論\n\n結論の段落。\n",
			want: "結論の段落。",
		},
		{
			name: "English Summary heading fallback",
			body: "# Note\n\n## Summary\n\nShort summary.\n",
			want: "Short summary.",
		},
		{
			name: "spec clause without H2 returns lead paragraph",
			body: "# Every claim carries evidence\n\nA report states what it measured and how.\n\nSecond paragraph.\n",
			want: "A report states what it measured and how.",
		},
		{
			name: "lead paragraph skips fenced code blocks",
			body: "# Title\n\n```yaml\n# fake heading\nkey: val\n```\n\nActual lead paragraph.\n",
			want: "Actual lead paragraph.",
		},
		{
			name: "custom H2 fallback skips metadata headings",
			body: "# Title\n\n## Status\n\nAccepted\n\n## Date\n\n2026-01-01\n\n## ステータス\n\nAccepted\n\n## Rationale\n\nBecause latency matters.\n",
			want: "Because latency matters.",
		},
		{
			name: "lead paragraph before metadata-only headings",
			body: "# Title\n\nIntro summary.\n\n## Status\n\nAccepted\n",
			want: "Intro summary.",
		},
		{
			name: "only metadata headings returns empty instead of status value",
			body: "# Title\n\n## Status\n\nAccepted\n\n## Metadata\n\nOwner: team\n",
			want: "",
		},
		{
			name: "empty body with only title returns empty",
			body: "# Title\n",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DefaultExcerpt(tt.body); got != tt.want {
				t.Errorf("DefaultExcerpt = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildExplicitSectionIsStrictWhileEmptyUsesDefaultExcerpt(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "0001-japanese.md")
	content := "---\ntitle: Japanese decision\nstatus: accepted\ndate: 2026-09-01\n---\n\n# 0001\n\n## 決定\n\n### D1. 決定の内容\n\n```yaml\nkey: value\n```\n"
	if err := os.WriteFile(docPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	cfg := config.ADRPreset()
	cfg.Dir = dir
	docs, err := parse.Dir(dir, cfg)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	g := graph.Build(docs, cfg)

	fallback, err := Build(g, cfg, "0001", Options{Depth: 1})
	if err != nil {
		t.Fatalf("Build default: %v", err)
	}
	if fallback.Ref.Excerpt != "D1. 決定の内容" {
		t.Errorf("default excerpt = %q, want %q", fallback.Ref.Excerpt, "D1. 決定の内容")
	}

	strict, err := Build(g, cfg, "0001", Options{Depth: 1, Section: "Nonexistent"})
	if err != nil {
		t.Fatalf("Build strict: %v", err)
	}
	if strict.Ref.Excerpt != "" {
		t.Errorf("strict excerpt = %q, want empty when explicit section is absent", strict.Ref.Excerpt)
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

func TestBuildSurfacesRetractionsOnInvolvedDocuments(t *testing.T) {
	g, cfg := testCorpus(t, "ok-madr")
	g.Retractions = []model.Retraction{
		{From: "0003", To: "0002", Type: config.EdgeSupersedes, By: "0004"},
	}

	want := Retraction{Edge: config.EdgeSupersedes, From: "0003", To: "0002", By: "0004"}
	for _, id := range []model.ID{"0003", "0002", "0004"} {
		b, err := Build(g, cfg, id, Options{Depth: 1})
		if err != nil {
			t.Fatalf("Build(%s): %v", id, err)
		}
		if len(b.Retracted) != 1 || b.Retracted[0] != want {
			t.Errorf("Build(%s).Retracted = %+v, want [%+v]", id, b.Retracted, want)
		}
	}

	unaffected, err := Build(g, cfg, "0001", Options{Depth: 1})
	if err != nil {
		t.Fatalf("Build(0001): %v", err)
	}
	if len(unaffected.Retracted) != 0 {
		t.Errorf("Build(0001).Retracted = %v, want empty", unaffected.Retracted)
	}
}
