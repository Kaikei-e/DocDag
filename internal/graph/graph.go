// Package graph builds the two-layer document graph and answers every question
// asked of it: invariants, reachability, resolution and degree statistics.
package graph

import (
	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

// Build assembles the typed constraint layer and the untyped reference layer
// from parsed documents, recording structural findings it observes on the way.
func Build(docs []*parse.Document, cfg config.Config) (*model.Graph, error) {
	return nil, model.ErrNotImplemented
}

// Adjacency returns from-to adjacency over the given edge types, or over every
// typed edge when none are given. Neighbour lists are sorted.
func Adjacency(g *model.Graph, types ...model.EdgeType) map[model.ID][]model.ID { return nil }

// Reverse returns to-from adjacency over the given edge types, or over every
// typed edge when none are given. Neighbour lists are sorted.
func Reverse(g *model.Graph, types ...model.EdgeType) map[model.ID][]model.ID { return nil }

// ReferenceAdjacency returns adjacency over the reference layer only.
func ReferenceAdjacency(g *model.Graph) map[model.ID][]model.ID { return nil }
