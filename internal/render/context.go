package render

import (
	"fmt"
	"io"
	"strings"

	"github.com/Kaikei-e/DocDag/internal/brief"
)

// Group headings, shared by the text and Markdown briefs so a reader moving
// between them reads the same words.
const (
	groupRef         = "ref"
	groupResolvesTo  = "resolves to"
	groupAncestors   = "ancestors"
	groupDescendants = "descendants"
)

// ContextText writes a brief as an indented outline: one line per document,
// its excerpt beneath it.
func ContextText(w io.Writer, b *brief.Brief) error {
	out := &errWriter{w: w}
	contextGroupText(out, groupRef, []brief.Entry{b.Ref})
	contextGroupText(out, resolvesToHeading(b), b.ResolvesTo)
	contextGroupText(out, groupAncestors, b.Ancestors)
	contextGroupText(out, groupDescendants, b.Descendants)
	if b.Budget.Degraded > 0 {
		out.printf("budget: %d tokens, %d used, %d entries without an excerpt\n",
			b.Budget.Limit, b.Budget.Used, b.Budget.Degraded)
	}
	if out.err != nil {
		return fmt.Errorf("write context: %w", out.err)
	}
	return nil
}

// resolvesToHeading says in one line why a resolution is being reported at all.
func resolvesToHeading(b *brief.Brief) string {
	return fmt.Sprintf("%s (%s is %s)", groupResolvesTo, b.Ref.ID, b.Ref.Status)
}

func contextGroupText(out *errWriter, heading string, entries []brief.Entry) {
	if len(entries) == 0 {
		return
	}
	out.printf("%s\n", heading)
	for _, e := range entries {
		out.printf("  %s\n", e.Line())
		if e.Excerpt != "" {
			out.printf("%s\n", indent(e.Excerpt, "    "))
		}
	}
	out.printf("\n")
}

func indent(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

// ContextJSON writes a brief as JSON.
func ContextJSON(w io.Writer, b *brief.Brief) error {
	if err := writeJSON(w, b); err != nil {
		return fmt.Errorf("write context: %w", err)
	}
	return nil
}

// ContextMarkdown writes a brief as a Markdown document, so it can be pasted
// into a review or an issue as it stands.
func ContextMarkdown(w io.Writer, b *brief.Brief) error {
	out := &errWriter{w: w}
	contextEntryMarkdown(out, "#", b.Ref)
	contextGroupMarkdown(out, "Resolves to", b.ResolvesTo)
	contextGroupMarkdown(out, "Ancestors", b.Ancestors)
	contextGroupMarkdown(out, "Descendants", b.Descendants)
	if b.Budget.Degraded > 0 {
		out.printf("Budget: %d tokens, %d used, %d entries without an excerpt.\n",
			b.Budget.Limit, b.Budget.Used, b.Budget.Degraded)
	}
	if out.err != nil {
		return fmt.Errorf("write context: %w", out.err)
	}
	return nil
}

func contextGroupMarkdown(out *errWriter, heading string, entries []brief.Entry) {
	if len(entries) == 0 {
		return
	}
	out.printf("## %s\n\n", heading)
	for _, e := range entries {
		contextEntryMarkdown(out, "###", e)
	}
}

func contextEntryMarkdown(out *errWriter, level string, e brief.Entry) {
	out.printf("%s %s %s\n\n", level, e.ID, e.Title)
	out.printf("- status: %s\n- path: %s\n\n", e.Status, e.Path)
	if e.Excerpt != "" {
		out.printf("%s\n\n", e.Excerpt)
	}
}
