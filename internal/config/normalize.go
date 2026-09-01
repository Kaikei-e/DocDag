package config

import (
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/Kaikei-e/DocDag/internal/model"
)

// DefaultReferencePattern is the shape a body link target must have to be an
// identity reference at all. Capture group 1 is the digit run.
const DefaultReferencePattern = `^(?i)(?:adr-?)?(\d{3,6})$`

var (
	// idShape is a reference whose whole text is an identity: an optional ADR
	// prefix, a digit run and an optional file name suffix.
	idShape = regexp.MustCompile(`^(?i)(?:adr[-_ ]?)?[0-9]+(-[^/\\]*?)?(\.md)?$`)
	// documentLink is the file name shape a Markdown link target must have to
	// name a managed document.
	documentLink = regexp.MustCompile(`^[0-9]{3,6}(-[^/\\]*)?\.md$`)
	// wikilink is an Obsidian-style wrapper around a reference, with an optional
	// alias or anchor after it.
	wikilink = regexp.MustCompile(`^\[\[([^\[\]|#]*)(?:[|#][^\[\]]*)?\]\]$`)

	defaultReference = regexp.MustCompile(DefaultReferencePattern)
	referencePattern sync.Map
)

// IDShaped reports whether the whole reference names an identity, so that prose
// such as "see 0042" never counts as one. An Obsidian wikilink is unwrapped
// first: a corpus that writes `[[0042]]` in frontmatter means 0042.
func IDShaped(ref string) bool {
	ref = unwrap(ref)
	return ref != "" && idShape.MatchString(ref)
}

// unwrap returns the reference a frontmatter value names, with an Obsidian
// wikilink wrapper and its alias removed.
func unwrap(ref string) string {
	ref = strings.TrimSpace(ref)
	if inside := wikilink.FindStringSubmatch(ref); inside != nil {
		return strings.TrimSpace(inside[1])
	}
	return ref
}

// IsDocumentLink reports whether a Markdown link target names a managed
// document file rather than some other Markdown file in the repository.
func IsDocumentLink(target string) bool {
	file, _, _ := strings.Cut(strings.TrimSpace(target), "#")
	return documentLink.MatchString(path.Base(file))
}

// IsReference reports whether a body link target is an identity reference under
// references.pattern.
func (c Config) IsReference(target string) bool {
	target, _, _ = strings.Cut(strings.TrimSpace(target), "#")
	return target != "" && c.referenceShape().MatchString(target)
}

// referenceShape compiles references.pattern once per process; Validate has
// already rejected a pattern that does not compile.
func (c Config) referenceShape() *regexp.Regexp {
	expr := c.References.Pattern
	if expr == "" {
		return defaultReference
	}
	if cached, ok := referencePattern.Load(expr); ok {
		compiled, ok := cached.(*regexp.Regexp)
		if !ok {
			return defaultReference
		}
		return compiled
	}
	compiled, err := regexp.Compile(expr)
	if err != nil {
		referencePattern.Store(expr, nil)
		return defaultReference
	}
	referencePattern.Store(expr, compiled)
	return compiled
}

// IDNormalizer turns raw references and filenames into node identity. Presets
// supply the implementation; the engine treats the result as opaque.
type IDNormalizer interface {
	// Normalize maps a reference token onto a node identifier.
	Normalize(ref string) (model.ID, bool)
	// MatchesFilename reports whether a file name denotes a managed document.
	MatchesFilename(name string) bool
	// Width reports the display width used when padding identifiers.
	Width() int
}

// ADRNormalizer implements ADR identity: the digit run inside a reference or
// filename is the identity, zero-padded to a fixed display width.
type ADRNormalizer struct {
	Pad int
}

var (
	adrDigits   = regexp.MustCompile(`[0-9]+`)
	adrFilename = regexp.MustCompile(`^[0-9]{3,6}(-[A-Za-z0-9-]+)?\.md$`)
)

// Normalize extracts the digit run from ref and pads it to the display width.
// Leading zeros are stripped textually, so an identifier wider than the machine
// integer range still normalizes. Directory components are dropped first: a
// digit in a path prefix names no document.
func (n ADRNormalizer) Normalize(ref string) (model.ID, bool) {
	token := strings.TrimSpace(ref)
	if cut := strings.LastIndexAny(token, `/\`); cut >= 0 {
		token = token[cut+1:]
	}
	digits := adrDigits.FindString(token)
	if digits == "" {
		return "", false
	}
	id := strings.TrimLeft(digits, "0")
	if id == "" {
		id = "0"
	}
	if missing := n.Pad - len(id); missing > 0 {
		id = strings.Repeat("0", missing) + id
	}
	return model.ID(id), true
}

// MatchesFilename reports whether name is `NNNN.md` or `NNNN-kebab-title.md`
// with a digit run of width 3 to 6.
func (n ADRNormalizer) MatchesFilename(name string) bool { return adrFilename.MatchString(name) }

// Width reports the display width used when padding identifiers.
func (n ADRNormalizer) Width() int { return n.Pad }

// DigitRunNormalizer is the identity of a kind that declares no id pattern: the
// digit-run rules, held to the reference shape a single-kind corpus holds them
// to. The shape matters inside a union in a way it does not on its own: the
// digit-run rules read a digit out of any text at all, so without the shape
// test a kind declaring no pattern would quietly claim every reference the
// other kinds rejected, "see 0042" included.
type DigitRunNormalizer struct {
	ADRNormalizer
}

// Normalize maps a reference onto the padded digit run, provided the whole
// reference names an identity.
func (n DigitRunNormalizer) Normalize(ref string) (model.ID, bool) {
	if !IDShaped(ref) {
		return "", false
	}
	return n.ADRNormalizer.Normalize(ref)
}

// PatternNormalizer implements declared identity: a reference is an identifier
// exactly when the whole of it matches the kind's pattern, and the reference as
// written is the identity. There is nothing to pad and nothing to extract — the
// pattern is the canonical spelling — so the only thing done to a reference is
// the wikilink unwrapping every identity path does, because a corpus that
// writes `[[UZ-V-001]]` in frontmatter means UZ-V-001 whatever its kind.
type PatternNormalizer struct {
	Pattern *regexp.Regexp
}

// Normalize reports the reference as its own identity when the pattern accepts
// it whole. A directory component is not stripped the way the digit-run rules
// strip one: a pattern may itself carry slashes, as `^conform/[a-z0-9-]+$`
// does, and cutting at the last slash would take the identity apart.
func (n PatternNormalizer) Normalize(ref string) (model.ID, bool) {
	token := unwrap(ref)
	if token == "" || n.Pattern == nil || !n.Pattern.MatchString(token) {
		return "", false
	}
	return model.ID(token), true
}

// MatchesFilename reports whether a file name carries an identity of this kind,
// which is its stem matching the pattern. A pattern carrying a slash matches no
// stem at all, so documents of such a kind take their identity from the
// frontmatter id key instead.
func (n PatternNormalizer) MatchesFilename(name string) bool {
	stem, isMarkdown := strings.CutSuffix(name, markdownExt)
	if !isMarkdown {
		return false
	}
	_, ok := n.Normalize(stem)
	return ok
}

// Width reports the display width used when padding identifiers, which a
// declared pattern never does: it is the identifier's own spelling.
func (n PatternNormalizer) Width() int { return 0 }

// markdownExt is the extension a document file carries.
const markdownExt = ".md"

// kindNormalizer is one kind's normalizer under the kind's name, so a union can
// report which kind accepted a reference.
type kindNormalizer struct {
	name string
	IDNormalizer
}

// UnionNormalizer resolves a reference against several kinds, the first that
// accepts it deciding. The order is a property of the configuration rather than
// of a map iteration: two kinds whose patterns overlap must normalize a
// reference the same way on every run, or a corpus would change shape between
// two runs of the same command.
type UnionNormalizer struct {
	kinds []kindNormalizer
}

// Normalize maps a reference onto the identity of the first kind that accepts
// it.
func (n UnionNormalizer) Normalize(ref string) (model.ID, bool) {
	for _, kind := range n.kinds {
		if id, ok := kind.Normalize(ref); ok {
			return id, true
		}
	}
	return "", false
}

// MatchesFilename reports whether any kind reads this file name as one of its
// documents.
func (n UnionNormalizer) MatchesFilename(name string) bool {
	return slices.ContainsFunc(n.kinds, func(kind kindNormalizer) bool { return kind.MatchesFilename(name) })
}

// Width reports no display width: the kinds pad differently, and a union has no
// single answer.
func (n UnionNormalizer) Width() int { return 0 }

// Kind names the kind whose normalizer accepts a reference, which is the kind a
// reference to an absent document would have named.
func (n UnionNormalizer) Kind(ref string) (string, bool) {
	for _, kind := range n.kinds {
		if _, ok := kind.Normalize(ref); ok {
			return kind.name, true
		}
	}
	return "", false
}

// idPatterns caches the compiled kind patterns for the life of the process, the
// way the reference pattern is cached: Validate has already compiled each one,
// and a normalizer is built once per reference-heavy loop.
var idPatterns sync.Map

// IDPattern compiles one kind's id pattern. The pattern describes a whole
// identifier, so it is anchored at both ends whether or not it was written that
// way: a pattern matching part of a reference would make `see UZ-V-001` an
// identifier, which is the mistake IDShaped exists to prevent.
func IDPattern(expr string) (*regexp.Regexp, error) {
	if cached, ok := idPatterns.Load(expr); ok {
		compiled, ok := cached.(*regexp.Regexp)
		if !ok {
			return nil, fmt.Errorf("invalid identifier pattern %q", expr)
		}
		return compiled, nil
	}
	compiled, err := regexp.Compile(`^(?:` + expr + `)$`)
	if err != nil {
		idPatterns.Store(expr, nil)
		return nil, err
	}
	idPatterns.Store(expr, compiled)
	return compiled, nil
}

// KindNormalizer returns the identity rules of one kind: its declared pattern,
// or the digit-run rules every document had before kinds existed when it
// declares none.
func (c Config) KindNormalizer(name string) IDNormalizer {
	spec, ok := c.Kind(name)
	if !ok || spec.ID == "" {
		return DigitRunNormalizer{ADRNormalizer{Pad: c.IDWidth}}
	}
	pattern, err := IDPattern(spec.ID)
	if err != nil {
		// Validate rejects a pattern that does not compile, so this is a
		// configuration nobody could have resolved; identity by digit run is
		// the least surprising answer left.
		return DigitRunNormalizer{ADRNormalizer{Pad: c.IDWidth}}
	}
	return PatternNormalizer{Pattern: pattern}
}

// Normalizer returns the identifier normalizer for this configuration: the
// digit-run rules of a single-kind corpus, or every kind's rules in sorted kind
// order when the configuration declares kinds.
func (c Config) Normalizer() IDNormalizer {
	if !c.Multikind() {
		return ADRNormalizer{Pad: c.IDWidth}
	}
	return c.unionNormalizer(nil)
}

// EdgeNormalizer returns the normalizer that resolves the references written
// under one edge key. An edge that names the kinds it may point at has those
// tried first: where two kinds' patterns both accept a reference, the one the
// edge declares is the one it meant. The other kinds still follow, so a
// reference to a document of the wrong kind resolves and is reported as the
// edge_kind_mismatch it is, rather than degrading into "not an identifier".
func (c Config) EdgeNormalizer(spec EdgeSpec) IDNormalizer {
	if !c.Multikind() {
		return ADRNormalizer{Pad: c.IDWidth}
	}
	// A reverse edge reads its key as naming the source, so the kinds a
	// reference under it may have are the edge's from kinds.
	preferred := spec.To
	if spec.Direction == DirectionReverse {
		preferred = spec.From
	}
	return c.unionNormalizer(preferred)
}

// unionNormalizer builds the union over every declared kind, sorted by name,
// with the preferred kinds moved in front of the rest in their own sorted
// order.
func (c Config) unionNormalizer(preferred []string) UnionNormalizer {
	first := make([]string, 0, len(preferred))
	for _, name := range c.KindNames() {
		if slices.Contains(preferred, name) {
			first = append(first, name)
		}
	}
	union := UnionNormalizer{kinds: make([]kindNormalizer, 0, len(c.Kinds))}
	for _, name := range append(first, c.KindNames()...) {
		if slices.ContainsFunc(union.kinds, func(kind kindNormalizer) bool { return kind.name == name }) {
			continue
		}
		union.kinds = append(union.kinds, kindNormalizer{name: name, IDNormalizer: c.KindNormalizer(name)})
	}
	return union
}

// IDShaped reports whether a reference names an identity under this
// configuration: the digit-run shape of a single-kind corpus, or a shape one of
// the declared kinds accepts. It is the question "could this ever name a
// document", which is what separates an invalid_ref from a dangling one.
func (c Config) IDShaped(ref string) bool {
	if !c.Multikind() {
		return IDShaped(ref)
	}
	_, ok := c.Normalizer().Normalize(ref)
	return ok
}
