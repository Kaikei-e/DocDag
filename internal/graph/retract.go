package graph

import (
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/parse"
	"github.com/Kaikei-e/DocDag/model"
)

// applyRetractions validates every document's retracts declarations against the
// pre-retraction edge set, collects effective retractions from agreed records,
// and removes those edges from records before g.Edges is assembled.
func applyRetractions(g *model.Graph, docs []*parse.Document, cfg config.Config, records map[edgeKey]edgeRecord) []model.Finding {
	var findings []model.Finding
	var effective []model.Retraction
	toDrop := make(map[edgeKey]bool)
	seen := make(map[model.Retraction]bool)

	for _, doc := range docs {
		raw, present := doc.Frontmatter[config.KeyRetracts]
		if !present {
			continue
		}
		items, ok := raw.([]any)
		if !ok {
			findings = append(findings, invalidRetraction(cfg, doc,
				"retracts must be a list of mappings with from, edge, to"))
			continue
		}
		active := retractionEffective(g.Nodes[doc.ID].Status)
		for _, item := range items {
			r, detail := parseRetraction(item, cfg, g, records)
			if detail != "" {
				findings = append(findings, invalidRetraction(cfg, doc, detail))
				continue
			}
			r.By = doc.ID
			if !active {
				continue
			}
			toDrop[edgeKey{from: r.From, to: r.To, t: r.Type}] = true
			if !seen[r] {
				seen[r] = true
				effective = append(effective, r)
			}
		}
	}

	for k := range toDrop {
		delete(records, k)
	}
	slices.SortFunc(effective, compareRetraction)
	g.Retractions = effective
	return findings
}

func retractionEffective(status string) bool {
	return strings.EqualFold(status, config.StatusAccepted) || strings.EqualFold(status, config.StatusSuperseded)
}

func parseRetraction(item any, cfg config.Config, g *model.Graph, records map[edgeKey]edgeRecord) (model.Retraction, string) {
	mapping, ok := item.(map[string]any)
	if !ok {
		return model.Retraction{}, "retraction entry must be a mapping with from, edge, to"
	}
	for _, key := range slices.Sorted(maps.Keys(mapping)) {
		if key != "from" && key != "edge" && key != "to" {
			return model.Retraction{}, fmt.Sprintf("retraction entry has unexpected key %q", key)
		}
	}
	values := make(map[string]string, 3)
	for _, key := range []string{"from", "edge", "to"} {
		raw, present := mapping[key]
		if !present {
			return model.Retraction{}, fmt.Sprintf("retraction entry is missing required key %q", key)
		}
		val, isScalar := parse.Scalar(raw)
		val = strings.TrimSpace(val)
		if !isScalar || val == "" {
			return model.Retraction{}, fmt.Sprintf("retraction entry key %q must be a non-empty string", key)
		}
		values[key] = val
	}

	edgeName := model.EdgeType(values["edge"])
	spec, declared := cfg.Edge(edgeName)
	if !declared {
		return model.Retraction{}, fmt.Sprintf("retracts names unknown edge %q", edgeName)
	}
	fromID, detail := resolveRetractionEndpoint(values["from"], "from", cfg, cfg.EndpointNormalizer(spec.From), g)
	if detail != "" {
		return model.Retraction{}, detail
	}
	toID, detail := resolveRetractionEndpoint(values["to"], "to", cfg, cfg.EndpointNormalizer(spec.To), g)
	if detail != "" {
		return model.Retraction{}, detail
	}
	if _, exists := records[edgeKey{from: fromID, to: toID, t: edgeName}]; !exists {
		return model.Retraction{}, fmt.Sprintf("retracts names %s %s -> %s, but %s declares no such edge", edgeName, fromID, toID, fromID)
	}
	return model.Retraction{From: fromID, To: toID, Type: edgeName}, ""
}

func resolveRetractionEndpoint(raw, role string, cfg config.Config, norm config.IDNormalizer, g *model.Graph) (model.ID, string) {
	if !cfg.IDShaped(raw) {
		return "", fmt.Sprintf("retracts %s %q is not an identifier", role, raw)
	}
	id, ok := norm.Normalize(raw)
	if !ok {
		return "", fmt.Sprintf("retracts %s %q is not an identifier", role, raw)
	}
	if _, known := g.Nodes[id]; !known {
		return "", fmt.Sprintf("retracts %s %q names no document", role, raw)
	}
	return id, ""
}

func invalidRetraction(cfg config.Config, doc *parse.Document, detail string) model.Finding {
	return model.Finding{
		Severity: cfg.Severity(model.RuleInvalidRetraction),
		Rule:     model.RuleInvalidRetraction,
		ID:       doc.ID,
		Detail:   detail,
		Location: model.Locate(doc.Path, doc.FrontmatterLine, doc.KeyLines, config.KeyRetracts),
	}
}

func compareRetraction(a, b model.Retraction) int {
	return cmp.Or(
		cmp.Compare(a.Type, b.Type),
		cmp.Compare(a.From, b.From),
		cmp.Compare(a.To, b.To),
		cmp.Compare(a.By, b.By),
	)
}
