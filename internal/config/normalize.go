package config

import (
	"regexp"
	"strings"

	"github.com/Kaikei-e/DocDag/internal/model"
)

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
