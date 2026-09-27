package graph

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/parse"
	"github.com/Kaikei-e/DocDag/model"
)

// CheckSections enforces required document body sections and their ordering.
func CheckSections(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	for _, id := range g.NodeIDs() {
		n := g.Nodes[id]
		spec, ok := cfg.KindSections(n.Kind)
		if !ok || len(spec.Required) == 0 {
			continue
		}
		if spec.When != nil && len(spec.When.Status) > 0 {
			matchesStatus := false
			for _, s := range spec.When.Status {
				if strings.EqualFold(s, n.Status) {
					matchesStatus = true
					break
				}
			}
			if !matchesStatus {
				continue
			}
		}
		src, err := os.ReadFile(n.Path)
		if err != nil {
			continue
		}
		_, body, ok := parse.SplitFrontmatter(src)
		firstBodyLine := 1
		if ok {
			firstBodyLine = 1 + bytes.Count(src[:len(src)-len(body)], []byte("\n"))
		}
		headings := parse.Headings(string(body))

		present := make([]bool, len(spec.Required))
		for _, h := range headings {
			if idx := matchRequiredSection(h, spec.Required, spec.Level); idx >= 0 {
				present[idx] = true
			}
		}

		for i, req := range spec.Required {
			if !present[i] {
				findings = append(findings, model.Finding{
					Severity: cfg.Severity(model.RuleMissingSection),
					Rule:     model.RuleMissingSection,
					ID:       n.ID,
					Detail:   fmt.Sprintf("missing required section %q", req),
					Location: model.Location{Path: n.Path, Line: firstBodyLine},
				})
			}
		}

		if spec.Ordered {
			maxIndex := -1
			for _, h := range headings {
				idx := matchRequiredSection(h, spec.Required, spec.Level)
				if idx < 0 {
					continue
				}
				if idx < maxIndex {
					fileLine := firstBodyLine + h.Line - 1
					findings = append(findings, model.Finding{
						Severity: cfg.Severity(model.RuleSectionOrder),
						Rule:     model.RuleSectionOrder,
						ID:       n.ID,
						Detail:   fmt.Sprintf("section %q is out of order", h.Text),
						Location: model.Location{Path: n.Path, Line: fileLine},
					})
				} else {
					maxIndex = idx
				}
			}
		}
	}
	return findings
}

func matchRequiredSection(h parse.Heading, required []string, level int) int {
	if level != 0 && h.Level != level {
		return -1
	}
	normHeading := parse.NormalizeHeading(h.Text)
	for i, entry := range required {
		for _, alt := range strings.Split(entry, "|") {
			if normHeading == parse.NormalizeHeading(alt) {
				return i
			}
		}
	}
	return -1
}
