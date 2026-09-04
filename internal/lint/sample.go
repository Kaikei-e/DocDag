package lint

import (
	"regexp/syntax"
	"slices"
	"strings"

	"github.com/Kaikei-e/DocDag/config"
)

// sampleID returns an identifier a declared pattern accepts: the shortest one
// for index 0, and a variation of it after that, so a fixture holding two
// documents of one kind can name them both.
//
// The walk over the pattern is what makes a generated fixture possible at all:
// a kind's identity is a regular expression, and a document has to be named
// something the expression admits before any rule can be run against it.
func sampleID(pattern string, index int) (string, bool) {
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return "", false
	}
	compiled, err := config.IDPattern(pattern)
	if err != nil {
		return "", false
	}
	sample := minimalMatch(parsed.Simplify())
	if !compiled.MatchString(sample) {
		return "", false
	}
	if index == 0 {
		return sample, true
	}
	for _, candidate := range variations(sample, index) {
		if compiled.MatchString(candidate) {
			return candidate, true
		}
	}
	return "", false
}

// variations offers spellings of an identifier that differ from it in the last
// character, which is where a pattern's variable part almost always is.
func variations(sample string, index int) []string {
	runes := []rune(sample)
	out := []string{}
	for at := len(runes) - 1; at >= 0; at-- {
		replacement, ok := bump(runes[at], index)
		if !ok {
			continue
		}
		candidate := slices.Clone(runes)
		candidate[at] = replacement
		out = append(out, string(candidate))
	}
	return out
}

// bump moves one character along its own alphabet, and reports the characters
// it has no alphabet for.
func bump(r rune, index int) (rune, bool) {
	switch {
	case r >= '0' && r <= '9':
		return '0' + rune(index%10), true
	case r >= 'a' && r <= 'z':
		return 'a' + rune(index%26), true
	case r >= 'A' && r <= 'Z':
		return 'A' + rune(index%26), true
	}
	return 0, false
}

// minimalMatch returns the shortest string a parsed pattern accepts, preferring
// a letter to a digit inside a character class so a generated identifier reads
// like one somebody would have written.
func minimalMatch(re *syntax.Regexp) string {
	switch re.Op {
	case syntax.OpLiteral:
		return string(re.Rune)
	case syntax.OpCharClass:
		return string(classRune(re.Rune))
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		return "a"
	case syntax.OpCapture, syntax.OpPlus:
		return minimalMatch(re.Sub[0])
	case syntax.OpConcat:
		var b strings.Builder
		for _, sub := range re.Sub {
			b.WriteString(minimalMatch(sub))
		}
		return b.String()
	case syntax.OpAlternate:
		if len(re.Sub) == 0 {
			return ""
		}
		return minimalMatch(re.Sub[0])
	case syntax.OpRepeat:
		return strings.Repeat(minimalMatch(re.Sub[0]), re.Min)
	}
	return ""
}

// classRune picks the character a class is sampled with.
func classRune(ranges []rune) rune {
	for _, wanted := range []rune{'a', '0', 'A'} {
		for i := 0; i+1 < len(ranges); i += 2 {
			if wanted >= ranges[i] && wanted <= ranges[i+1] {
				return wanted
			}
		}
	}
	if len(ranges) > 0 {
		return ranges[0]
	}
	return 'a'
}
