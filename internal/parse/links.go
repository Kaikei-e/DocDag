package parse

import (
	"path"
	"regexp"
	"strings"
)

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

// linkPattern matches both syntaxes at once so links come back in the order
// they appear; the wikilink alternative is tried first at every position.
var linkPattern = regexp.MustCompile(`\[\[([^\[\]]*)\]\]|\[([^\]\n]*)\]\(([^)\s]*)\)`)

// schemePattern matches the leading scheme of an absolute URL.
var schemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]*:`)

// Links extracts `[[target]]`, `[[target|alias]]` and relative Markdown links
// from a document body, in order of appearance.
func Links(body string) []Link {
	var links []Link
	for _, m := range linkPattern.FindAllStringSubmatchIndex(body, -1) {
		if m[2] >= 0 {
			target, alias, _ := strings.Cut(body[m[2]:m[3]], "|")
			target, alias = strings.TrimSpace(target), strings.TrimSpace(alias)
			if target == "" {
				continue
			}
			links = append(links, Link{Target: target, Alias: alias, Kind: LinkWiki})
			continue
		}
		target := body[m[6]:m[7]]
		if !isDocumentTarget(target) {
			continue
		}
		links = append(links, Link{Target: target, Alias: strings.TrimSpace(body[m[4]:m[5]]), Kind: LinkMarkdown})
	}
	return links
}

// isDocumentTarget reports whether a Markdown link target is a relative path to
// another managed document rather than a URL, an anchor or an asset.
func isDocumentTarget(target string) bool {
	if target == "" || strings.HasPrefix(target, "/") || strings.HasPrefix(target, "#") {
		return false
	}
	if schemePattern.MatchString(target) {
		return false
	}
	file, _, _ := strings.Cut(target, "#")
	return path.Ext(file) == markdownExt
}
