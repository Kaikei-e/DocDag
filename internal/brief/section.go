package brief

import "strings"

var defaultSectionFallbacks = []string{
	DefaultSection,
	"決定",
	"結論",
	"Summary",
	"概要",
}

// Section returns the first paragraph under the H2 or H3 that names the wanted
// section, verbatim. When a deeper child heading immediately follows the
// matched heading, it returns that child heading's text together with its
// direct first paragraph when present. It returns an empty string when the
// document carries no such heading or when the next heading at the same or
// higher level follows with no content between.
func Section(body, want string) string {
	lines := splitBodyLines(body)
	best, at, matchedLevel := 0, -1, 0
	fence := ""
	for i, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		if nextFence, inside := stepFence(line, fence); inside {
			fence = nextFence
			continue
		}
		level, heading, ok := parseHeading(line)
		if !ok || level < 2 || level > 3 || heading == "" {
			continue
		}
		rank := sectionRank(heading, want)
		if rank < 0 || (at >= 0 && rank >= best) {
			continue
		}
		best, at, matchedLevel = rank, i, level
	}
	if at < 0 {
		return ""
	}
	return sectionContent(lines[at+1:], matchedLevel)
}

// DefaultExcerpt returns the default summary excerpt for a document when the
// caller did not ask for a specific section. It checks Decision (with MADR
// Decision Outcome preference), common decision/summary headings, the first
// non-metadata H2/H3 section, or the lead paragraph when no H2/H3 headings
// exist or none produce content.
func DefaultExcerpt(body string) string {
	for _, want := range defaultSectionFallbacks {
		if got := Section(body, want); got != "" {
			return got
		}
	}
	lines := splitBodyLines(body)
	type headingPos struct {
		at    int
		level int
		text  string
	}
	var headings []headingPos
	fence := ""
	for i, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		if nextFence, inside := stepFence(line, fence); inside {
			fence = nextFence
			continue
		}
		level, text, ok := parseHeading(line)
		if !ok || level < 2 || level > 3 || text == "" {
			continue
		}
		headings = append(headings, headingPos{at: i, level: level, text: text})
	}
	if len(headings) == 0 {
		return leadParagraph(lines)
	}
	for _, h := range headings {
		if isMetadataHeading(h.text) {
			continue
		}
		if got := sectionContent(lines[h.at+1:], h.level); got != "" {
			return got
		}
	}
	return leadParagraph(lines)
}

func splitBodyLines(body string) []string {
	return strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
}

func stepFence(line, fence string) (next string, inside bool) {
	if fence != "" {
		if closesFence(line, fence) {
			return "", true
		}
		return fence, true
	}
	if opening := openingFence(line); opening != "" {
		return opening, true
	}
	return "", false
}

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
		if marker == '`' && strings.Contains(trimmed[run:], "`") {
			continue
		}
		return trimmed[:run]
	}
	return ""
}

func closesFence(line, fence string) bool {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return false
	}
	trimmed = strings.TrimRight(trimmed, " \t")
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

func parseHeading(line string) (level int, text string, ok bool) {
	trimmed := strings.TrimLeft(line, " ")
	if len(line)-len(trimmed) > 3 {
		return 0, "", false
	}
	for level < len(trimmed) && trimmed[level] == '#' {
		level++
	}
	if level < 1 || level > 6 {
		return 0, "", false
	}
	rest := trimmed[level:]
	if len(rest) > 0 && rest[0] != ' ' && rest[0] != '\t' {
		return 0, "", false
	}
	text = strings.TrimSpace(strings.TrimRight(rest, "# \t"))
	return level, text, true
}

// sectionRank scores how well a heading names the wanted section, lower being
// better, or reports -1 for no match. MADR writes the decision under
// "<section> Outcome" and its alternatives under headings sharing that prefix,
// so the outcome outranks the prefix match.
func sectionRank(heading, want string) int {
	switch {
	case strings.EqualFold(heading, want):
		return 0
	case strings.EqualFold(heading, want+" Outcome"):
		return 1
	case len(heading) > len(want) && strings.EqualFold(heading[:len(want)], want):
		return 2
	}
	return -1
}

func sectionContent(lines []string, matchedLevel int) string {
	fence := ""
	for i, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		if nextFence, inside := stepFence(line, fence); inside {
			fence = nextFence
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if level, heading, ok := parseHeading(line); ok {
			if level <= matchedLevel {
				return ""
			}
			if heading == "" {
				continue
			}
			if sub := directProse(lines[i+1:]); sub != "" {
				return heading + "\n" + sub
			}
			return heading
		}
		return directProse(lines[i:])
	}
	return ""
}

func directProse(lines []string) string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	if start >= len(lines) {
		return ""
	}
	end := start
	for end < len(lines) {
		line := strings.TrimRight(lines[end], " \t")
		if strings.TrimSpace(line) == "" {
			break
		}
		if _, _, ok := parseHeading(line); ok {
			break
		}
		if openingFence(line) != "" {
			break
		}
		end++
	}
	return strings.TrimRight(strings.Join(lines[start:end], "\n"), " \t\n")
}

func leadParagraph(lines []string) string {
	fence := ""
	for i, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		if nextFence, inside := stepFence(line, fence); inside {
			fence = nextFence
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		if level, _, ok := parseHeading(line); ok {
			// Stop before H2/H3 sections so a document whose only sections are
			// metadata headings does not leak metadata values as its lead text.
			if level == 2 || level == 3 {
				return ""
			}
			continue
		}
		return directProse(lines[i:])
	}
	return ""
}

func isMetadataHeading(heading string) bool {
	normalized := strings.ToLower(strings.TrimSpace(strings.TrimRight(heading, ":：")))
	switch normalized {
	case "status", "ステータス", "date", "日付", "metadata", "メタデータ":
		return true
	}
	return false
}
