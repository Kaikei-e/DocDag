package graph

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

// paddingIndex answers one question about a reference that names no document:
// would a zero-padding variant of it have named one? It holds every identifier
// in the corpus under its own spelling with the leading zeros of its trailing
// digit run removed, so `0011` and `11` meet under the key `11` and a reference
// written either way finds the document the other names.
//
// Where identity is the digit run — the ADR rules, and any kind that declares
// no `id:` pattern — padding is not a spelling but the identity itself, and
// `11`, `0011` and `ADR-11` already normalize to one document; there the index
// is asked nothing, because nothing failed to resolve. It answers for the kinds
// whose `id:` pattern is the canonical spelling, where `UZ-V-11` and `UZ-V-011`
// are two different identifiers and only one of them exists.
type paddingIndex map[string][]model.ID

// lastDigits matches the trailing digit run of a token, which is where an
// identifier carries its zero padding: `0011`, `UZ-V-011`, `adr-0011`.
var lastDigits = regexp.MustCompile(`[0-9]+$`)

// newPaddingIndex indexes every document of a graph by its unpadded spelling.
func newPaddingIndex(g *model.Graph) paddingIndex {
	index := make(paddingIndex, len(g.Nodes))
	for id := range g.Nodes {
		key := unpadded(id.String())
		index[key] = append(index[key], id)
	}
	for key := range index {
		slices.Sort(index[key])
	}
	return index
}

// meant reports the document a zero-padding variant of ref names, and whether
// there is exactly one such document. A reference that already names a document
// as written is not a mismatch, and a spelling two documents both answer to is
// not a suggestion anyone could act on.
func (x paddingIndex) meant(g *model.Graph, ref string) (model.ID, bool) {
	token := config.Unwrap(ref)
	if token == "" {
		return "", false
	}
	if _, known := g.Node(model.ID(token)); known {
		return "", false
	}
	candidates := x[unpadded(token)]
	if len(candidates) != 1 || candidates[0].String() == token {
		return "", false
	}
	return candidates[0], true
}

// unpadded strips the leading zeros from a token's trailing digit run, leaving
// one zero where the run is nothing but zeros. It is the spelling `0011` and
// `11` share, and the spelling `UZ-V-011` shares with `UZ-V-11`.
func unpadded(token string) string {
	run := lastDigits.FindStringIndex(token)
	if run == nil {
		return token
	}
	digits := strings.TrimLeft(token[run[0]:], "0")
	if digits == "" {
		digits = "0"
	}
	return token[:run[0]] + digits
}

// paddingMismatch reports a reference that names no document as written while a
// zero-padding variant of it names exactly one, which is the mistake that would
// otherwise be reported as "does not name a document" with three guesses after
// it. It is an error rather than a suggestion because the author meant a
// document that exists and wrote its identifier the wrong width; the edge is
// not built, so nothing resolves to the document they did not name either.
func paddingMismatch(cfg config.Config, index paddingIndex, g *model.Graph, owner model.ID, loc model.Location, key string, t model.EdgeType, ref string) (model.Finding, bool) {
	meant, ok := index.meant(g, ref)
	if !ok {
		return model.Finding{}, false
	}
	return model.Finding{
		Severity: cfg.Severity(model.RulePaddingMismatch),
		Rule:     model.RulePaddingMismatch,
		ID:       owner,
		Detail:   fmt.Sprintf("%s reference %q does not name a document; did you mean %q?", t, ref, meant),
		Location: loc,
		Fix:      fmt.Sprintf("write %s: %s", key, meant),
	}, true
}
