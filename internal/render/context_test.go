package render

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/brief"
)

func testBrief() *brief.Brief {
	return &brief.Brief{
		SchemaVersion: brief.SchemaVersion,
		Ref: brief.Entry{
			ID: "0002", Title: "Store thumbnails on disk", Status: "superseded", Path: "docs/adr/0002-store.md",
			Excerpt: "Chosen option: the local disk.\nSharded by the first two characters.",
		},
		ResolvesTo:  []brief.Entry{{ID: "0003", Title: "Store thumbnails in a bucket", Status: "accepted", Path: "docs/adr/0003-bucket.md"}},
		Ancestors:   []brief.Entry{},
		Descendants: []brief.Entry{},
		Budget:      brief.Budget{Limit: 2000, Used: 40},
	}
}

func TestContextTextOmitsEmptyGroups(t *testing.T) {
	var buf bytes.Buffer
	if err := ContextText(&buf, testBrief()); err != nil {
		t.Fatalf("ContextText: %v", err)
	}

	got := buf.String()
	for _, want := range []string{
		"ref\n  0002  Store thumbnails on disk  [superseded]  docs/adr/0002-store.md\n",
		"    Chosen option: the local disk.\n    Sharded by the first two characters.\n",
		"resolves to (0002 is superseded)\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("context = %q, want it to contain %q", got, want)
		}
	}
	for _, unwanted := range []string{"ancestors", "descendants", "budget:"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("context = %q, want no %q group", got, unwanted)
		}
	}
}

func TestContextTextReportsASpentBudget(t *testing.T) {
	b := testBrief()
	b.Budget.Degraded = 2

	var buf bytes.Buffer
	if err := ContextText(&buf, b); err != nil {
		t.Fatalf("ContextText: %v", err)
	}

	want := "budget: 2000 tokens, 40 used, 2 entries without an excerpt\n"
	if !strings.HasSuffix(buf.String(), want) {
		t.Errorf("context = %q, want it to end with %q", buf.String(), want)
	}
}

func TestContextRenderersPropagateWriterErrors(t *testing.T) {
	renderers := map[string]func(io.Writer, *brief.Brief) error{
		"text":     ContextText,
		"json":     ContextJSON,
		"markdown": ContextMarkdown,
	}
	for name, write := range renderers {
		t.Run(name, func(t *testing.T) {
			if err := write(failingWriter{}, testBrief()); err == nil {
				t.Fatal("err = nil, want the write failure surfaced")
			}
		})
	}
}

func TestContextRendersTheNormativeNeighbourhood(t *testing.T) {
	b := testBrief()
	b.Related = []brief.Entry{
		{ID: "topic/thumbnails", Title: "Where thumbnails live", Path: "spec/topics/thumbnails.md", Relation: brief.RelationAbout},
		{ID: "0004", Title: "Thumbnails may be cached in memory", Status: "accepted", Path: "spec/clauses/0004.md", Relation: brief.RelationExcepts},
	}
	b.Suppressed = []string{"suppressed by excepts 0004 -> 0002 (scope: only under memory pressure)"}

	t.Run("text names the relation on the line and the suppression under its own heading", func(t *testing.T) {
		var buf bytes.Buffer
		if err := ContextText(&buf, b); err != nil {
			t.Fatalf("ContextText: %v", err)
		}

		got := buf.String()
		for _, want := range []string{
			"related\n  topic/thumbnails  Where thumbnails live  []  spec/topics/thumbnails.md  (about)\n",
			"  0004  Thumbnails may be cached in memory  [accepted]  spec/clauses/0004.md  (excepts)\n",
			"suppressed\n  suppressed by excepts 0004 -> 0002 (scope: only under memory pressure)\n",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("context = %q, want it to contain %q", got, want)
			}
		}
	})

	t.Run("markdown carries the same two", func(t *testing.T) {
		var buf bytes.Buffer
		if err := ContextMarkdown(&buf, b); err != nil {
			t.Fatalf("ContextMarkdown: %v", err)
		}

		got := buf.String()
		for _, want := range []string{"## Related", "- relation: about", "## Suppressed", "- suppressed by excepts 0004 -> 0002"} {
			if !strings.Contains(got, want) {
				t.Errorf("context = %q, want it to contain %q", got, want)
			}
		}
	})

	t.Run("a brief without them renders neither heading", func(t *testing.T) {
		var buf bytes.Buffer
		if err := ContextText(&buf, testBrief()); err != nil {
			t.Fatalf("ContextText: %v", err)
		}

		for _, unwanted := range []string{"related", "suppressed"} {
			if strings.Contains(buf.String(), unwanted) {
				t.Errorf("context = %q, want no %q group", buf.String(), unwanted)
			}
		}
	})
}
