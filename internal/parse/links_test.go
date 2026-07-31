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
			want: []Link{{Target: "000002", Kind: LinkWiki}},
		},
		{
			name: "an aliased wikilink keeps both halves",
			body: "See [[0001|the first decision]].\n",
			want: []Link{{Target: "0001", Alias: "the first decision", Kind: LinkWiki}},
		},
		{
			name: "wikilink whitespace is trimmed",
			body: "See [[ 0001 | the first decision ]].\n",
			want: []Link{{Target: "0001", Alias: "the first decision", Kind: LinkWiki}},
		},
		{
			name: "a relative markdown link",
			body: "The cache agreed in [the thumbnail cache decision](0001-cache-rendered-thumbnails.md) needs a home.\n",
			want: []Link{{Target: "0001-cache-rendered-thumbnails.md", Alias: "the thumbnail cache decision", Kind: LinkMarkdown}},
		},
		{
			name: "a dot-relative markdown link",
			body: "See [the queue decision](./0002-hand-off-through-a-queue.md).\n",
			want: []Link{{Target: "./0002-hand-off-through-a-queue.md", Alias: "the queue decision", Kind: LinkMarkdown}},
		},
		{
			name: "a parent-relative markdown link",
			body: "See [the storage decision](../adr/0003-store-in-object-storage.md).\n",
			want: []Link{{Target: "../adr/0003-store-in-object-storage.md", Alias: "the storage decision", Kind: LinkMarkdown}},
		},
		{
			name: "a fragment stays part of the target",
			body: "See [the outcome](0004-expire-after-thirty-days.md#decision-outcome).\n",
			want: []Link{{Target: "0004-expire-after-thirty-days.md#decision-outcome", Alias: "the outcome", Kind: LinkMarkdown}},
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
				{Target: "0002", Kind: LinkWiki},
				{Target: "0001-cache.md", Alias: "the cache", Kind: LinkMarkdown},
				{Target: "0002", Alias: "again", Kind: LinkWiki},
			},
		},
		{
			name: "a target the engine cannot normalize is still returned",
			body: "See [[not-a-document]].\n",
			want: []Link{{Target: "not-a-document", Kind: LinkWiki}},
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
