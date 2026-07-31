package graph

import (
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
func ComputeStats(g *model.Graph, cfg config.Config) Statistics { return Statistics{} }
