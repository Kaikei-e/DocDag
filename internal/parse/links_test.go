package parse

import (
	"slices"
	"testing"
)

func TestLinks(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []Link
	}{
		{
			name: "a bare wikilink",
			body: "See [[000002]] for the replacement.\n",
			want: []Link{{Target: "000002", Kind: LinkWiki, Line: 1}},
		},
		{
			name: "an aliased wikilink keeps both halves",
			body: "See [[0001|the first decision]].\n",
			want: []Link{{Target: "0001", Alias: "the first decision", Kind: LinkWiki, Line: 1}},
		},
		{
			name: "wikilink whitespace is trimmed",
			body: "See [[ 0001 | the first decision ]].\n",
			want: []Link{{Target: "0001", Alias: "the first decision", Kind: LinkWiki, Line: 1}},
		},
		{
			name: "a relative markdown link",
			body: "The cache agreed in [the thumbnail cache decision](0001-cache-rendered-thumbnails.md) needs a home.\n",
			want: []Link{{Target: "0001-cache-rendered-thumbnails.md", Alias: "the thumbnail cache decision", Kind: LinkMarkdown, Line: 1}},
		},
		{
			name: "a dot-relative markdown link",
			body: "See [the queue decision](./0002-hand-off-through-a-queue.md).\n",
			want: []Link{{Target: "./0002-hand-off-through-a-queue.md", Alias: "the queue decision", Kind: LinkMarkdown, Line: 1}},
		},
		{
			name: "a parent-relative markdown link",
			body: "See [the storage decision](../adr/0003-store-in-object-storage.md).\n",
			want: []Link{{Target: "../adr/0003-store-in-object-storage.md", Alias: "the storage decision", Kind: LinkMarkdown, Line: 1}},
		},
		{
			name: "a fragment stays part of the target",
			body: "See [the outcome](0004-expire-after-thirty-days.md#decision-outcome).\n",
			want: []Link{{Target: "0004-expire-after-thirty-days.md#decision-outcome", Alias: "the outcome", Kind: LinkMarkdown, Line: 1}},
		},
		{
			name: "an absolute URL is not a document link",
			body: "See [the specification](https://example.test/rfc/0001.md).\n",
		},
		{
			name: "a root-absolute path is not a relative link",
			body: "See [the decision](/docs/adr/0001-a-decision.md).\n",
		},
		{
			name: "a non-markdown target is not a document link",
			body: "See [the diagram](diagram.png) and [the sheet](notes.txt).\n",
		},
		{
			name: "an anchor-only link is not a document link",
			body: "See [the outcome](#decision-outcome).\n",
		},
		{
			name: "links come back in order of appearance",
			body: "First [[0002]], then [the cache](0001-cache.md), then [[0002|again]].\n",
			want: []Link{
				{Target: "0002", Kind: LinkWiki, Line: 1},
				{Target: "0001-cache.md", Alias: "the cache", Kind: LinkMarkdown, Line: 1},
				{Target: "0002", Alias: "again", Kind: LinkWiki, Line: 1},
			},
		},
		{
			name: "a target the engine cannot normalize is still returned",
			body: "See [[not-a-document]].\n",
			want: []Link{{Target: "not-a-document", Kind: LinkWiki, Line: 1}},
		},
		{
			name: "an empty wikilink is ignored",
			body: "Empty [[]] and blank [[   ]].\n",
		},
		{
			name: "an unclosed wikilink is ignored",
			body: "Unclosed [[0001 and nothing else.\n",
		},
		{
			name: "prose without links has none",
			body: "# A decision\n\nNothing links anywhere from here.\n",
		},
		{
			name: "an empty body has none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Links(tt.body); !slices.Equal(got, tt.want) {
				t.Fatalf("Links = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLinksCarryTheLineTheyWereWrittenOn(t *testing.T) {
	body := "# A decision\n\nIt replaces [[0001]].\n\nIt also depends on [the queue](0002-queue.md)\nand on [[0003|the store]].\n"

	want := []Link{
		{Target: "0001", Kind: LinkWiki, Line: 3},
		{Target: "0002-queue.md", Alias: "the queue", Kind: LinkMarkdown, Line: 5},
		{Target: "0003", Alias: "the store", Kind: LinkWiki, Line: 6},
	}
	if got := Links(body); !slices.Equal(got, want) {
		t.Fatalf("Links = %+v, want %+v", got, want)
	}
}

func TestLinksSkipCode(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []Link
	}{
		{
			name: "a backtick fence hides its links",
			body: "Before [[0001]].\n\n```\nSee [[0002]] and [the queue](0003-queue.md).\n```\n\nAfter [[0004]].\n",
			want: []Link{
				{Target: "0001", Kind: LinkWiki, Line: 1},
				{Target: "0004", Kind: LinkWiki, Line: 7},
			},
		},
		{
			name: "a tilde fence hides its links",
			body: "~~~\n[[0002]]\n~~~\n[[0004]]\n",
			want: []Link{{Target: "0004", Kind: LinkWiki, Line: 4}},
		},
		{
			name: "an info string does not close the fence",
			body: "```yaml\nsupersedes: [[0002]]\n```\n[[0004]]\n",
			want: []Link{{Target: "0004", Kind: LinkWiki, Line: 4}},
		},
		{
			name: "a longer fence is not closed by a shorter one",
			body: "````\n```\n[[0002]]\n````\n[[0004]]\n",
			want: []Link{{Target: "0004", Kind: LinkWiki, Line: 5}},
		},
		{
			name: "an unterminated fence hides the rest of the body",
			body: "[[0001]]\n```\n[[0002]]\n[[0003]]\n",
			want: []Link{{Target: "0001", Kind: LinkWiki, Line: 1}},
		},
		{
			name: "an inline code span hides its links",
			body: "Write `[[0002]]` to link, as in [[0001]].\n",
			want: []Link{{Target: "0001", Kind: LinkWiki, Line: 1}},
		},
		{
			name: "an inline span closes on a backtick run of its own length",
			body: "``a ` b [[0002]]`` then [[0001]].\n",
			want: []Link{{Target: "0001", Kind: LinkWiki, Line: 1}},
		},
		{
			name: "an unclosed inline span does not hide the line",
			body: "A stray ` backtick and [[0001]].\n",
			want: []Link{{Target: "0001", Kind: LinkWiki, Line: 1}},
		},
		{
			name: "an indented fence still opens a block",
			body: "  ```\n  [[0002]]\n  ```\n[[0004]]\n",
			want: []Link{{Target: "0004", Kind: LinkWiki, Line: 4}},
		},
		{
			name: "a markdown link inside a code span is hidden too",
			body: "Use `[the cache](0001-cache.md)` in prose.\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Links(tt.body); !slices.Equal(got, tt.want) {
				t.Fatalf("Links = %+v, want %+v", got, tt.want)
			}
		})
	}
}
