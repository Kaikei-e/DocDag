package graph

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// Rule names and vocabulary this pass knows a remedy for but does not own.
const (
	ruleDanglingReference = "dangling_reference"
	statusWithdrawn       = "withdrawn"
)

// suggestionCandidates is how many existing documents a "did you mean" names.
const suggestionCandidates = 3

// quotedRef lifts the reference a dangling finding is about out of its detail.
var quotedRef = regexp.MustCompile(`"([^"]*)"`)

// Suggest fills in the Fix of every finding it recognizes. The checks say what
// is wrong; this says what to type, as a pass over a finished report so a check
// never has to carry a remedy.
func Suggest(findings []model.Finding, g *model.Graph, cfg config.Config) []model.Finding {
	for i := range findings {
		findings[i].Fix = suggestion(findings[i], g, cfg)
	}
	return findings
}

func suggestion(f model.Finding, g *model.Graph, cfg config.Config) string {
	switch f.Rule {
	case model.RuleDanglingRef, ruleDanglingReference:
		return didYouMean(g, f.Detail, f.ID)
	case model.RuleStatusDrift:
		return fmt.Sprintf("set %s: %s in %s", statusField(cfg), config.StatusSuperseded, f.Location.Path)
	case model.RuleSupersededOrphan:
		return fmt.Sprintf("declare %s: %s in the replacing document, or set %s: %s",
			supersedesKey(cfg), f.ID, statusField(cfg), statusWithdrawn)
	case model.RuleUnstructuredSupersedes:
		return declareEdge(g, cfg, f.ID)
	case model.RuleUnknownStatus:
		if len(cfg.StatusValues) == 0 {
			return ""
		}
		return "use one of: " + strings.Join(cfg.StatusValues, ", ")
	case model.RuleMissingFrontmatter:
		return "add a YAML frontmatter block with title and " + statusField(cfg)
	case model.RuleCycle:
		return "remove one of the listed edges"
	}
	return ""
}

// didYouMean names the existing documents closest to the reference that named
// none, which is what a mistyped or renumbered identifier looks like. The
// document holding the reference is never one of them: nothing refers to itself.
func didYouMean(g *model.Graph, detail string, self model.ID) string {
	match := quotedRef.FindStringSubmatch(detail)
	if match == nil {
		return ""
	}
	want, ok := number(match[1])
	if !ok {
		return ""
	}
	candidates := make([]model.ID, 0, len(g.Nodes))
	distances := make(map[model.ID]int, len(g.Nodes))
	for id := range g.Nodes {
		value, ok := number(id.String())
		if !ok || id == self {
			continue
		}
		candidates = append(candidates, id)
		distances[id] = abs(value - want)
	}
	if len(candidates) == 0 {
		return ""
	}
	slices.SortFunc(candidates, func(a, b model.ID) int {
		return cmp.Or(cmp.Compare(distances[a], distances[b]), cmp.Compare(a, b))
	})
	return fmt.Sprintf("did you mean %s?", joinAlternatives(candidates[:min(suggestionCandidates, len(candidates))]))
}

// number reads the digit run of a reference, which is what a document's
// identity is made of.
func number(ref string) (int, bool) {
	start := strings.IndexFunc(ref, isDigit)
	if start < 0 {
		return 0, false
	}
	end := start
	for end < len(ref) && isDigit(rune(ref[end])) {
		end++
	}
	value, err := strconv.Atoi(ref[start:end])
	if err != nil {
		return 0, false
	}
	return value, true
}

func isDigit(r rune) bool { return r >= '0' && r <= '9' }

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func joinAlternatives(ids []model.ID) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, id.String())
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + " or " + parts[len(parts)-1]
}

// declareEdge names the document that should carry the edge a field value only
// implies, and the key it belongs under.
func declareEdge(g *model.Graph, cfg config.Config, owner model.ID) string {
	for _, e := range g.Edges {
		if e.Origin != model.OriginDerived || (e.From != owner && e.To != owner) {
			continue
		}
		spec, ok := cfg.Edge(e.Type)
		if !ok {
			continue
		}
		holder, value := e.From, e.To
		if spec.Direction == config.DirectionReverse {
			holder, value = e.To, e.From
		}
		return fmt.Sprintf("declare %s: %s in %s", spec.Key, value, holder)
	}
	return ""
}

func supersedesKey(cfg config.Config) string {
	if spec, ok := cfg.Edge(config.EdgeSupersedes); ok {
		return spec.Key
	}
	return config.EdgeSupersedes.String()
}
