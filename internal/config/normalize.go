package config

import (
	"path"
	"regexp"
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

	defaultReference = regexp.MustCompile(DefaultReferencePattern)
	referencePattern sync.Map
)

// IDShaped reports whether the whole reference names an identity, so that prose
// such as "see 0042" never counts as one.
func IDShaped(ref string) bool {
	ref = strings.TrimSpace(ref)
	return ref != "" && idShape.MatchString(ref)
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

// Normalizer returns the identifier normalizer for this configuration.
func (c Config) Normalizer() IDNormalizer { return ADRNormalizer{Pad: c.IDWidth} }
