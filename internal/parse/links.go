package parse

// LinkKind distinguishes the reference-layer link syntaxes DocDag recognizes.
type LinkKind string

// Recognized reference-layer link kinds.
const (
	LinkWiki     LinkKind = "wikilink"
	LinkMarkdown LinkKind = "markdown"
)

// Link is one reference-layer link found in a document body. Reference links
// are never validated and never constrain the DAG.
type Link struct {
	Target string
	Alias  string
	Kind   LinkKind
}

// Links extracts `[[target]]`, `[[target|alias]]` and relative Markdown links
// from a document body, in order of appearance.
func Links(body string) []Link { return nil }
