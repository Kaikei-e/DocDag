package parse

import (
	"strings"
)

// Heading is one ATX heading found in a document body. Line is 1-based and
// relative to the body.
type Heading struct {
	Text  string
	Level int
	Line  int
}

// Headings extracts ATX headings (# through ######) from a document body, in
// order of appearance. A heading inside a fenced code block is ignored.
func Headings(body string) []Heading {
	var (
		headings []Heading
		fence    string
		line     = 1
	)
	for start := 0; start < len(body); {
		end, next := len(body), len(body)
		if at := strings.IndexByte(body[start:], '\n'); at >= 0 {
			end, next = start+at, start+at+1
		}
		text := strings.TrimSuffix(body[start:end], "\r")
		switch opening := openingFence(text); {
		case fence != "":
			if closesFence(text, fence) {
				fence = ""
			}
		case opening != "":
			fence = opening
		default:
			if h, ok := parseHeading(text, line); ok {
				headings = append(headings, h)
			}
		}
		line++
		start = next
	}
	return headings
}

// parseHeading checks if line is an ATX heading (1 to 6 '#' characters preceded
// by up to 3 spaces and followed by whitespace or EOL).
func parseHeading(line string, lineNumber int) (Heading, bool) {
	trimmed := strings.TrimLeft(line, " ")
	indent := len(line) - len(trimmed)
	if indent > 3 {
		return Heading{}, false
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level < 1 || level > 6 {
		return Heading{}, false
	}
	rest := trimmed[level:]
	if len(rest) > 0 && rest[0] != ' ' && rest[0] != '\t' {
		return Heading{}, false
	}
	text := stripClosingHashes(rest)
	return Heading{
		Text:  text,
		Level: level,
		Line:  lineNumber,
	}, true
}

func stripClosingHashes(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasSuffix(s, "#") {
		return s
	}
	i := len(s) - 1
	for i >= 0 && s[i] == '#' {
		i--
	}
	if i < 0 {
		return ""
	}
	if s[i] == ' ' || s[i] == '\t' {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// NormalizeHeading trims whitespace, trailing colons, and folds case for
// heading matching.
func NormalizeHeading(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, ":")
	s = strings.TrimSpace(s)
	return strings.ToLower(s)
}
