package graph

import (
	"slices"
	"strings"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// EdgeCount is the number of typed edges of one type.
type EdgeCount struct {
	Type  model.EdgeType `json:"type"`
	Count int            `json:"count"`
}

// DepthCount is the number of documents whose supersedes chain has one depth.
type DepthCount struct {
	Depth int `json:"depth"`
	Count int `json:"count"`
}

// ReferenceCount is the reference-layer in-degree of one document.
type ReferenceCount struct {
	ID    model.ID `json:"id"`
	Count int      `json:"count"`
}

// Statistics is the degree-based corpus summary. Every field is computable in
// O(V+E); nothing here needs a full path enumeration.
type Statistics struct {
	Documents     int              `json:"documents"`
	Edges         []EdgeCount      `json:"edges"`
	Binding       int              `json:"binding"`
	ChainDepth    []DepthCount     `json:"chain_depth"`
	Orphans       int              `json:"orphans"`
	OrphanRate    float64          `json:"orphan_rate"`
	TopReferenced []ReferenceCount `json:"top_referenced"`
}

// TopReferencedLimit caps the reference-layer in-degree ranking.
const TopReferencedLimit = 10

// ComputeStats summarises the graph.
func ComputeStats(g *model.Graph, cfg config.Config) Statistics {
	stats := Statistics{
		Documents:     len(g.Nodes),
		Edges:         edgeCounts(g, cfg),
		Binding:       len(BindingSet(g, cfg)),
		ChainDepth:    chainDepths(g),
		TopReferenced: topReferenced(g),
	}

	connected := make(map[model.ID]bool, len(g.Nodes))
	for _, e := range g.Edges {
		connected[e.From] = true
		connected[e.To] = true
	}
	for id := range g.Nodes {
		if !connected[id] {
			stats.Orphans++
		}
	}
	if len(g.Nodes) > 0 {
		stats.OrphanRate = float64(stats.Orphans) / float64(len(g.Nodes))
	}
	return stats
}

func edgeCounts(g *model.Graph, cfg config.Config) []EdgeCount {
	counts := make(map[model.EdgeType]int, len(cfg.Edges))
	for _, e := range g.Edges {
		counts[e.Type]++
	}
	declared := make([]EdgeCount, 0, len(cfg.Edges))
	for _, spec := range cfg.Edges {
		t := model.EdgeType(spec.Name)
		declared = append(declared, EdgeCount{Type: t, Count: counts[t]})
	}
	return declared
}

// chainDepths counts documents by the length of the longest supersedes chain
// starting at them. A cyclic edge contributes nothing rather than looping.
func chainDepths(g *model.Graph) []DepthCount {
	adj := retainKnown(g, Adjacency(g, config.EdgeSupersedes))

	depth := make(map[model.ID]int, len(g.Nodes))
	color := make(map[model.ID]int, len(g.Nodes))
	for _, root := range g.NodeIDs() {
		if color[root] != colorWhite {
			continue
		}
		color[root] = colorGray
		stack := []visitFrame{{id: root}}
		for len(stack) > 0 {
			frame := &stack[len(stack)-1]
			neighbors := adj[frame.id]
			if frame.next < len(neighbors) {
				next := neighbors[frame.next]
				frame.next++
				if color[next] == colorWhite {
					color[next] = colorGray
					stack = append(stack, visitFrame{id: next})
				}
				continue
			}
			id := frame.id
			longest := 0
			for _, next := range neighbors {
				if color[next] != colorBlack {
					continue
				}
				if reach := depth[next] + 1; reach > longest {
					longest = reach
				}
			}
			depth[id] = longest
			color[id] = colorBlack
			stack = stack[:len(stack)-1]
		}
	}

	counts := make(map[int]int, len(g.Nodes))
	for id := range g.Nodes {
		counts[depth[id]]++
	}
	distribution := make([]DepthCount, 0, len(counts))
	for d, count := range counts {
		distribution = append(distribution, DepthCount{Depth: d, Count: count})
	}
	slices.SortFunc(distribution, func(a, b DepthCount) int { return a.Depth - b.Depth })
	return distribution
}

func topReferenced(g *model.Graph) []ReferenceCount {
	counts := make(map[model.ID]int, len(g.RefEdges))
	for _, e := range g.RefEdges {
		counts[e.To]++
	}
	ranked := make([]ReferenceCount, 0, len(counts))
	for id, count := range counts {
		ranked = append(ranked, ReferenceCount{ID: id, Count: count})
	}
	slices.SortFunc(ranked, func(a, b ReferenceCount) int {
		if c := b.Count - a.Count; c != 0 {
			return c
		}
		return strings.Compare(string(a.ID), string(b.ID))
	})
	if len(ranked) > TopReferencedLimit {
		ranked = ranked[:TopReferencedLimit]
	}
	return ranked
}
