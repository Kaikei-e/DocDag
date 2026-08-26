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

// Link is one reference-layer link found in a document body. Line is 1-based
// and relative to the body. Reference links are never validated and never
// constrain the DAG.
type Link struct {
	Target string
	Alias  string
	Kind   LinkKind
	Line   int
}

// linkPattern matches both syntaxes at once so links come back in the order
// they appear; the wikilink alternative is tried first at every position.
var linkPattern = regexp.MustCompile(`\[\[([^\[\]]*)\]\]|\[([^\]\n]*)\]\(([^)\s]*)\)`)

// schemePattern matches the leading scheme of an absolute URL.
var schemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.\-]*:`)

// Links extracts `[[target]]`, `[[target|alias]]` and relative Markdown links
// from a document body, in order of appearance. A link written inside a fenced
// code block or an inline code span is an example, not a reference, and is
// skipped.
func Links(body string) []Link {
	var (
		links  []Link
		code   = codeSpans(body)
		cursor int
		line   = 1
	)
	for _, m := range linkPattern.FindAllStringSubmatchIndex(body, -1) {
		line += strings.Count(body[cursor:m[0]], "\n")
		cursor = m[0]
		if code[m[0]] {
			continue
		}
		if m[2] >= 0 {
			target, alias, _ := strings.Cut(body[m[2]:m[3]], "|")
			target, alias = strings.TrimSpace(target), strings.TrimSpace(alias)
			if target == "" {
				continue
			}
			links = append(links, Link{Target: target, Alias: alias, Kind: LinkWiki, Line: line})
			continue
		}
		target := body[m[6]:m[7]]
		if !isDocumentTarget(target) {
			continue
		}
		links = append(links, Link{Target: target, Alias: strings.TrimSpace(body[m[4]:m[5]]), Kind: LinkMarkdown, Line: line})
	}
	return links
}

// codeSpans marks every byte of body that sits inside a fenced code block or an
// inline code span.
func codeSpans(body string) []bool {
	inside := make([]bool, len(body)+1)
	fence := ""
	for start := 0; start < len(body); {
		end, next := len(body), len(body)
		if at := strings.IndexByte(body[start:], '\n'); at >= 0 {
			end, next = start+at, start+at+1
		}
		text := strings.TrimSuffix(body[start:end], "\r")
		switch opening := openingFence(text); {
		case fence != "":
			mark(inside, start, end)
			if closesFence(text, fence) {
				fence = ""
			}
		case opening != "":
			fence = opening
			mark(inside, start, end)
		default:
			markInlineSpans(inside, body, start, end)
		}
		start = next
	}
	return inside
}

// openingFence returns the fence marker a line opens, or the empty string. Up
// to three leading spaces still open a block.
func openingFence(line string) string {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return ""
	}
	for _, marker := range []byte{'`', '~'} {
		run := runLength(trimmed, marker)
		if run < 3 {
			continue
		}
		// An info string may follow a backtick fence but may not contain a
		// backtick, which would make the line an inline span instead.
		if marker == '`' && strings.Contains(trimmed[run:], "`") {
			continue
		}
		return trimmed[:run]
	}
	return ""
}

// closesFence reports whether a line closes an open fence: the same marker,
// at least as long, and nothing else on the line.
func closesFence(line, fence string) bool {
	trimmed := strings.TrimSpace(strings.TrimLeft(line, " "))
	run := runLength(trimmed, fence[0])
	return run >= len(fence) && run == len(trimmed)
}

func runLength(s string, c byte) int {
	n := 0
	for n < len(s) && s[n] == c {
		n++
	}
	return n
}

// markInlineSpans marks the inline code spans of one line. A backtick run opens
// a span that only a run of the same length closes; an unclosed run is prose.
func markInlineSpans(inside []bool, body string, start, end int) {
	for at := start; at < end; {
		if body[at] != '`' {
			at++
			continue
		}
		open := runLength(body[at:end], '`')
		closing := closingRun(body, at+open, end, open)
		if closing < 0 {
			at += open
			continue
		}
		mark(inside, at, closing+open)
		at = closing + open
	}
}

// closingRun returns the offset of the next backtick run of exactly length n,
// or -1 when the span stays open to the end of the line.
func closingRun(body string, from, end, n int) int {
	for at := from; at < end; {
		if body[at] != '`' {
			at++
			continue
		}
		run := runLength(body[at:end], '`')
		if run == n {
			return at
		}
		at += run
	}
	return -1
}

func mark(inside []bool, start, end int) {
	for i := start; i < end; i++ {
		inside[i] = true
	}
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
