package brief

import "strings"

// Section returns the first paragraph under the H2 or H3 that names the wanted
// section, verbatim. It returns an empty string when the document carries no
// such heading or nothing follows it.
func Section(body, want string) string {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	best, at := 0, -1
	var fence byte
	for i, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		if marker := fenceMarker(line); marker != 0 {
			switch {
			case fence == 0:
				fence = marker
			case fence == marker:
				fence = 0
			}
			continue
		}
		if fence != 0 {
			continue
		}
		heading, ok := headingText(line)
		if !ok {
			continue
		}
		rank := sectionRank(heading, want)
		if rank < 0 || (at >= 0 && rank >= best) {
			continue
		}
		best, at = rank, i
	}
	if at < 0 {
		return ""
	}
	return firstParagraph(lines[at+1:])
}

func fenceMarker(line string) byte {
	trimmed := strings.TrimLeft(line, " ")
	switch {
	case strings.HasPrefix(trimmed, "```"):
		return '`'
	case strings.HasPrefix(trimmed, "~~~"):
		return '~'
	}
	return 0
}

// headingText returns the text of an H2 or H3. An H1 is the document title, not
// a section of it.
func headingText(line string) (string, bool) {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level < 2 || level > 3 || level == len(line) || line[level] != ' ' {
		return "", false
	}
	return strings.TrimSpace(strings.TrimRight(line[level+1:], "# ")), true
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

func firstParagraph(lines []string) string {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	end := start
	for end < len(lines) {
		if strings.TrimSpace(lines[end]) == "" || strings.HasPrefix(lines[end], "#") {
			break
		}
		end++
	}
	return strings.TrimRight(strings.Join(lines[start:end], "\n"), " \t\n")
}
