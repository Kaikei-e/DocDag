package config

import "github.com/Kaikei-e/DocDag/internal/model"

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

// Normalize extracts the digit run from ref and pads it to the display width.
func (n ADRNormalizer) Normalize(ref string) (model.ID, bool) { return "", false }

// MatchesFilename reports whether name is `NNNN.md` or `NNNN-kebab-title.md`
// with a digit run of width 3 to 6.
func (n ADRNormalizer) MatchesFilename(name string) bool { return false }

// Width reports the display width used when padding identifiers.
func (n ADRNormalizer) Width() int { return n.Pad }

// Normalizer returns the identifier normalizer for this configuration.
func (c Config) Normalizer() IDNormalizer { return ADRNormalizer{Pad: c.IDWidth} }
