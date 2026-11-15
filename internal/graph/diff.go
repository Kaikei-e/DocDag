package graph

import (
	"cmp"
	"maps"
	"slices"
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

// GraphDiff holds the typed-edge, status, document, and binding-set
// differences between a base graph and a target graph.
type GraphDiff struct {
	Edges     EdgeDiff
	Status    []StatusChange
	Documents DocumentDiff
	Binding   BindingDelta
}

// EdgeDiff lists typed edges added and removed between base and target.
type EdgeDiff struct {
	Added   []EdgeChange `json:"added"`
	Removed []EdgeChange `json:"removed"`
}

// EdgeChange records an added or removed typed edge and its declaring location.
type EdgeChange struct {
	Type     model.EdgeType `json:"type"`
	From     model.ID       `json:"from"`
	To       model.ID       `json:"to"`
	Location model.Location `json:"-"`
}

// StatusChange records a document present on both sides whose status changed.
type StatusChange struct {
	ID       model.ID       `json:"id"`
	Path     string         `json:"path"`
	From     string         `json:"from"`
	To       string         `json:"to"`
	Location model.Location `json:"-"`
}

// DocumentDiff lists documents added to or removed from the corpus.
type DocumentDiff struct {
	Added   []DocumentChange `json:"added"`
	Removed []DocumentChange `json:"removed"`
}

// DocumentChange records an added or removed document and its status.
type DocumentChange struct {
	ID       model.ID       `json:"id"`
	Path     string         `json:"path"`
	Status   string         `json:"status"`
	Location model.Location `json:"-"`
}

// BindingDelta records the size of the binding set on each side and its signed delta.
type BindingDelta struct {
	Before int `json:"before"`
	After  int `json:"after"`
	Delta  int `json:"delta"`
}

// Diff compares base and target graphs under cfg on asOf, returning typed-edge
// additions and removals (excluding reference-layer edges), status changes on
// shared documents, document additions and removals, and the binding-set delta.
func Diff(base, target *model.Graph, cfg config.Config, asOf time.Time) GraphDiff {
	baseEdges := edgeMap(base.Edges)
	targetEdges := edgeMap(target.Edges)

	addedEdges := []EdgeChange{}
	for k, e := range targetEdges {
		if _, ok := baseEdges[k]; ok {
			continue
		}
		addedEdges = append(addedEdges, EdgeChange{
			Type:     e.Type,
			From:     e.From,
			To:       e.To,
			Location: diffEdgeLocation(target, cfg, e, true),
		})
	}
	slices.SortFunc(addedEdges, compareEdgeChange)

	removedEdges := []EdgeChange{}
	for k, e := range baseEdges {
		if _, ok := targetEdges[k]; ok {
			continue
		}
		loc := diffEdgeLocation(target, cfg, e, false)
		if loc.Path == "" {
			loc = diffEdgeLocation(base, cfg, e, false)
		}
		removedEdges = append(removedEdges, EdgeChange{
			Type:     e.Type,
			From:     e.From,
			To:       e.To,
			Location: loc,
		})
	}
	slices.SortFunc(removedEdges, compareEdgeChange)

	statusKey := statusField(cfg)
	statusChanges := []StatusChange{}
	addedDocs := []DocumentChange{}
	for _, id := range slices.Sorted(maps.Keys(target.Nodes)) {
		tn := target.Nodes[id]
		if bn, ok := base.Nodes[id]; ok {
			if bn.Status != tn.Status {
				statusChanges = append(statusChanges, StatusChange{
					ID:       id,
					Path:     tn.Path,
					From:     bn.Status,
					To:       tn.Status,
					Location: tn.Location(statusKey),
				})
			}
			continue
		}
		addedDocs = append(addedDocs, DocumentChange{
			ID:       id,
			Path:     tn.Path,
			Status:   tn.Status,
			Location: tn.Location(statusKey),
		})
	}

	removedDocs := []DocumentChange{}
	for _, id := range slices.Sorted(maps.Keys(base.Nodes)) {
		if _, ok := target.Nodes[id]; ok {
			continue
		}
		bn := base.Nodes[id]
		removedDocs = append(removedDocs, DocumentChange{
			ID:       id,
			Path:     bn.Path,
			Status:   bn.Status,
			Location: model.Location{Path: bn.Path},
		})
	}

	before := len(BindingSet(base, cfg, asOf))
	after := len(BindingSet(target, cfg, asOf))

	return GraphDiff{
		Edges: EdgeDiff{
			Added:   addedEdges,
			Removed: removedEdges,
		},
		Status: statusChanges,
		Documents: DocumentDiff{
			Added:   addedDocs,
			Removed: removedDocs,
		},
		Binding: BindingDelta{
			Before: before,
			After:  after,
			Delta:  after - before,
		},
	}
}

func edgeMap(edges []model.Edge) map[edgeKey]model.Edge {
	out := make(map[edgeKey]model.Edge, len(edges))
	for _, e := range edges {
		out[edgeKey{from: e.From, to: e.To, t: e.Type}] = e
	}
	return out
}

func diffEdgeLocation(g *model.Graph, cfg config.Config, e model.Edge, withLine bool) model.Location {
	id := declaringNodeID(cfg, e)
	n, ok := g.Nodes[id]
	if !ok {
		return model.Location{}
	}
	if !withLine {
		return model.Location{Path: n.Path}
	}
	return edgeLocation(cfg, n, e)
}

func declaringNodeID(cfg config.Config, e model.Edge) model.ID {
	if e.Origin == model.OriginDerived {
		for _, d := range cfg.DerivedEdges {
			if model.EdgeType(d.Edge) == e.Type {
				if d.Direction == config.DirectionReverse {
					return e.To
				}
				return e.From
			}
		}
	}
	if spec, ok := cfg.Edge(e.Type); ok && spec.Direction == config.DirectionReverse {
		return e.To
	}
	return e.From
}

func compareEdgeChange(a, b EdgeChange) int {
	return cmp.Or(
		cmp.Compare(a.Type, b.Type),
		cmp.Compare(a.From, b.From),
		cmp.Compare(a.To, b.To),
	)
}
