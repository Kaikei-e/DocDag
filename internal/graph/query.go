package graph

import (
	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// Direction selects which way a reachability query walks the typed edges.
type Direction string

// Query directions.
const (
	DirectionAncestors   Direction = "ancestors"
	DirectionDescendants Direction = "descendants"
)

// Layer marks which graph layer produced a query result.
type Layer string

// Result layers.
const (
	LayerTyped     Layer = "typed"
	LayerReference Layer = "reference"
)

// QueryOptions parameterises a reachability query.
type QueryOptions struct {
	Direction   Direction
	Types       []model.EdgeType
	IncludeRefs bool
}

// QueryResult is one reachable document and the layer it was reached through.
type QueryResult struct {
	ID    model.ID `json:"id"`
	Layer Layer    `json:"layer"`
}

// Resolve walks forward along the reverse edges of t to the current sink
// documents. A document with no successors resolves to itself. It reports
// model.ErrUnknownID for an absent id and model.ErrCycle on a cyclic walk.
func Resolve(g *model.Graph, id model.ID, t model.EdgeType) ([]model.ID, error) {
	return nil, model.ErrNotImplemented
}

// Ancestors returns every document reachable from id by walking typed edges
// backwards, sorted and excluding id itself.
func Ancestors(g *model.Graph, id model.ID, types ...model.EdgeType) ([]model.ID, error) {
	return nil, model.ErrNotImplemented
}

// Descendants returns every document reachable from id by walking typed edges
// forwards, sorted and excluding id itself.
func Descendants(g *model.Graph, id model.ID, types ...model.EdgeType) ([]model.ID, error) {
	return nil, model.ErrNotImplemented
}

// ReferenceNeighbors returns the reference-layer neighbours of id, sorted.
func ReferenceNeighbors(g *model.Graph, id model.ID) []model.ID { return nil }

// Query runs a reachability query and overlays reference-layer neighbours when
// asked for them.
func Query(g *model.Graph, id model.ID, opts QueryOptions) ([]QueryResult, error) {
	return nil, model.ErrNotImplemented
}

// Binding reports whether a document is currently binding: its status is the
// configured accepted value and no document supersedes it.
func Binding(g *model.Graph, cfg config.Config, id model.ID) bool { return false }

// BindingSet lists every binding document, sorted.
func BindingSet(g *model.Graph, cfg config.Config) []model.ID { return nil }
