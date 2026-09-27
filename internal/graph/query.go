package graph

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
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
//
// It reads every declared successor. A caller that has a configuration and a
// day in hand wants ResolveAt, which stops where a period says the replacement
// has not taken effect yet.
func Resolve(g *model.Graph, id model.ID, t model.EdgeType) ([]model.ID, error) {
	// A successor the corpus does not hold is a dangling reference, reported by
	// the structural checks: resolution stops there rather than answering with
	// an identifier that names no document.
	return resolveOver(g, retainKnown(g, Reverse(g, t)), id)
}

// PendingSuccessor is a successor that was not followed during resolution
// because its status is not currently binding (e.g. proposed, rejected, withdrawn)
// or its period has not yet begun.
type PendingSuccessor struct {
	ID          model.ID `json:"id"`
	Predecessor model.ID `json:"predecessor"`
	Status      string   `json:"status"`
}

type pendingKey struct {
	id          model.ID
	predecessor model.ID
}

// ResolveAt walks the lineage to the documents that stand in for a reference on
// one day. A successor replaces its predecessor only once somebody has accepted
// it and its own period has begun: until then the predecessor is what binds,
// and resolve says so — which is what keeps it answering the same set `--binding`
// does.
func ResolveAt(g *model.Graph, cfg config.Config, id model.ID, t model.EdgeType, asOf time.Time) ([]model.ID, error) {
	ids, _, err := ResolveWithPending(g, cfg, id, t, asOf)
	return ids, err
}

// ResolveWithPending walks the lineage along reverse edges of t from id to the
// current binding documents, stopping before any successor whose status is not
// binding-eligible (e.g. proposed, rejected, withdrawn) or not in force on asOf.
// Skipped successors are reported in the returned PendingSuccessor slice.
func ResolveWithPending(g *model.Graph, cfg config.Config, id model.ID, t model.EdgeType, asOf time.Time) ([]model.ID, []PendingSuccessor, error) {
	if _, ok := g.Nodes[id]; !ok {
		return nil, nil, fmt.Errorf("resolve %s: %w", id, model.ErrUnknownID)
	}

	adj := retainKnown(g, Reverse(g, t))
	adj = sortNeighbors(adj)

	// Detect cycles reachable from id along the lineage.
	color := make(map[model.ID]int, len(adj))
	color[id] = colorGray
	cycleStack := []visitFrame{{id: id}}
	for len(cycleStack) > 0 {
		frame := &cycleStack[len(cycleStack)-1]
		neighbors := adj[frame.id]
		if frame.next >= len(neighbors) {
			color[frame.id] = colorBlack
			cycleStack = cycleStack[:len(cycleStack)-1]
			continue
		}
		next := neighbors[frame.next]
		frame.next++
		switch color[next] {
		case colorGray:
			return nil, nil, fmt.Errorf("resolve %s through %s: %w", id, next, model.ErrCycle)
		case colorWhite:
			color[next] = colorGray
			cycleStack = append(cycleStack, visitFrame{id: next})
		}
	}

	periods := EvalPeriods(g, cfg, asOf)

	statusOf := func(nodeID model.ID) string {
		n, ok := g.Nodes[nodeID]
		if !ok {
			return "unknown"
		}
		status, ok := canonicalKindStatus(cfg, n.Kind, n.Status)
		if !ok || status == "" {
			status = strings.TrimSpace(n.Status)
		}
		if status == "" {
			status = "unknown"
		}
		return status
	}

	isAcceptedInForce := func(nodeID model.ID) bool {
		n, ok := g.Nodes[nodeID]
		if !ok {
			return false
		}
		status, ok := canonicalKindStatus(cfg, n.Kind, n.Status)
		return ok && strings.EqualFold(status, config.StatusAccepted) && periods.InForce(nodeID)
	}

	memoBinding := make(map[model.ID]bool)
	var leadsToBindingSink func(u model.ID) bool
	leadsToBindingSink = func(u model.ID) bool {
		if res, ok := memoBinding[u]; ok {
			return res
		}
		n, ok := g.Nodes[u]
		if !ok {
			memoBinding[u] = false
			return false
		}
		if isAcceptedInForce(u) {
			memoBinding[u] = true
			return true
		}
		status, _ := canonicalKindStatus(cfg, n.Kind, n.Status)
		if strings.EqualFold(status, config.StatusSuperseded) {
			for _, next := range adj[u] {
				if leadsToBindingSink(next) {
					memoBinding[u] = true
					return true
				}
			}
		}
		memoBinding[u] = false
		return false
	}

	validSuccessors := make(map[model.ID][]model.ID, len(adj))
	pendingMap := make(map[pendingKey]PendingSuccessor)

	stack := []model.ID{id}
	visited := map[model.ID]bool{id: true}
	for len(stack) > 0 {
		curr := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		var valid []model.ID
		for _, next := range adj[curr] {
			if leadsToBindingSink(next) {
				valid = append(valid, next)
				if !visited[next] {
					visited[next] = true
					stack = append(stack, next)
				}
			} else {
				k := pendingKey{id: next, predecessor: curr}
				if _, seen := pendingMap[k]; !seen {
					st := statusOf(next)
					n, ok := g.Nodes[next]
					if ok {
						cst, _ := canonicalKindStatus(cfg, n.Kind, n.Status)
						if strings.EqualFold(cst, config.StatusAccepted) && !periods.InForce(next) {
							st = "not in force"
						}
					}
					pendingMap[k] = PendingSuccessor{
						ID:          next,
						Predecessor: curr,
						Status:      st,
					}
				}
			}
		}
		validSuccessors[curr] = valid
	}

	resolved, err := resolveOver(g, validSuccessors, id)
	if err != nil {
		return nil, nil, err
	}

	pending := make([]PendingSuccessor, 0, len(pendingMap))
	for _, p := range pendingMap {
		pending = append(pending, p)
	}
	slices.SortFunc(pending, func(a, b PendingSuccessor) int {
		if c := strings.Compare(a.ID.String(), b.ID.String()); c != 0 {
			return c
		}
		return strings.Compare(a.Predecessor.String(), b.Predecessor.String())
	})

	return resolved, pending, nil
}

// resolveOver is the lineage walk both spellings share: an iterative
// depth-first search that reports the sinks it reaches and refuses a walk that
// closes a loop.
func resolveOver(g *model.Graph, successors map[model.ID][]model.ID, id model.ID) ([]model.ID, error) {
	if _, ok := g.Nodes[id]; !ok {
		return nil, fmt.Errorf("resolve %s: %w", id, model.ErrUnknownID)
	}
	color := make(map[model.ID]int, len(successors))
	sinks := make(map[model.ID]bool)
	color[id] = colorGray
	stack := []visitFrame{{id: id}}
	for len(stack) > 0 {
		frame := &stack[len(stack)-1]
		neighbors := successors[frame.id]
		if frame.next >= len(neighbors) {
			if len(neighbors) == 0 {
				sinks[frame.id] = true
			}
			color[frame.id] = colorBlack
			stack = stack[:len(stack)-1]
			continue
		}
		next := neighbors[frame.next]
		frame.next++
		switch color[next] {
		case colorGray:
			return nil, fmt.Errorf("resolve %s through %s: %w", id, next, model.ErrCycle)
		case colorWhite:
			color[next] = colorGray
			stack = append(stack, visitFrame{id: next})
		}
	}

	resolved := make([]model.ID, 0, len(sinks))
	for sink := range sinks {
		resolved = append(resolved, sink)
	}
	slices.Sort(resolved)
	return resolved, nil
}

// Ancestors returns every document reachable from id by walking typed edges
// backwards, sorted and excluding id itself.
func Ancestors(g *model.Graph, id model.ID, types ...model.EdgeType) ([]model.ID, error) {
	return reachable(g, id, Reverse(g, types...), DirectionAncestors)
}

// Descendants returns every document reachable from id by walking typed edges
// forwards, sorted and excluding id itself.
func Descendants(g *model.Graph, id model.ID, types ...model.EdgeType) ([]model.ID, error) {
	return reachable(g, id, Adjacency(g, types...), DirectionDescendants)
}

func reachable(g *model.Graph, id model.ID, adj map[model.ID][]model.ID, direction Direction) ([]model.ID, error) {
	if _, ok := g.Nodes[id]; !ok {
		return nil, fmt.Errorf("%s of %s: %w", direction, id, model.ErrUnknownID)
	}

	seen := map[model.ID]bool{id: true}
	found := []model.ID{}
	stack := []model.ID{id}
	for len(stack) > 0 {
		current := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		for _, next := range adj[current] {
			if seen[next] {
				continue
			}
			seen[next] = true
			stack = append(stack, next)
			if _, known := g.Nodes[next]; known {
				found = append(found, next)
			}
		}
	}
	slices.Sort(found)
	return found, nil
}

// ReferenceNeighbors returns the reference-layer neighbours of id, sorted.
func ReferenceNeighbors(g *model.Graph, id model.ID) []model.ID {
	neighbors := []model.ID{}
	for _, e := range g.RefEdges {
		switch {
		case e.From == id && e.To != id:
			neighbors = append(neighbors, e.To)
		case e.To == id && e.From != id:
			neighbors = append(neighbors, e.From)
		}
	}
	slices.Sort(neighbors)
	return slices.Compact(neighbors)
}

// Query runs a reachability query and overlays reference-layer neighbours when
// asked for them.
func Query(g *model.Graph, id model.ID, opts QueryOptions) ([]QueryResult, error) {
	var (
		reached []model.ID
		err     error
	)
	switch opts.Direction {
	case DirectionAncestors:
		reached, err = Ancestors(g, id, opts.Types...)
	case DirectionDescendants:
		reached, err = Descendants(g, id, opts.Types...)
	default:
		return nil, fmt.Errorf("query %s: direction %q is neither %s nor %s", id, opts.Direction, DirectionAncestors, DirectionDescendants)
	}
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", id, err)
	}

	results := make([]QueryResult, 0, len(reached))
	seen := map[model.ID]bool{id: true}
	for _, r := range reached {
		seen[r] = true
		results = append(results, QueryResult{ID: r, Layer: LayerTyped})
	}
	if !opts.IncludeRefs {
		return results, nil
	}
	for _, r := range ReferenceNeighbors(g, id) {
		if seen[r] {
			continue
		}
		seen[r] = true
		results = append(results, QueryResult{ID: r, Layer: LayerReference})
	}
	return results, nil
}

// Binding reports whether a document is binding on the day asked about: it
// satisfies the projection the configuration names under binding. A caller
// asking about many documents wants BindingSet, which evaluates the
// projections once.
func Binding(g *model.Graph, cfg config.Config, id model.ID, asOf time.Time) bool {
	spec, ok := cfg.BindingProjection()
	if !ok {
		return bindingByStatus(g, cfg)[id]
	}
	return EvalProjections(g, cfg, asOf).Holds(spec.Name, id)
}

// BindingSet lists every document binding on the day asked about, sorted.
func BindingSet(g *model.Graph, cfg config.Config, asOf time.Time) []model.ID {
	spec, ok := cfg.BindingProjection()
	if ok {
		return EvalProjections(g, cfg, asOf).Set(spec.Name)
	}
	held := bindingByStatus(g, cfg)
	binding := []model.ID{}
	for _, id := range g.NodeIDs() {
		if held[id] {
			binding = append(binding, id)
		}
	}
	return binding
}

// bindingByStatus is the binding projection written in code: accepted, and
// superseded by nothing. It answers for a configuration that resolves no
// binding projection — one that cleared the preset's with an explicit empty
// list, or that names none — because a corpus still has a current set, and
// reporting every document as current would be worse than an opinion. Every
// preset-derived configuration resolves a projection instead: the merge carries
// the preset's binding down to it.
func bindingByStatus(g *model.Graph, cfg config.Config) map[model.ID]bool {
	superseded := make(map[model.ID]bool)
	for _, e := range g.EdgesOfType(config.EdgeSupersedes) {
		superseded[e.To] = true
	}
	binding := make(map[model.ID]bool, len(g.Nodes))
	for id, n := range g.Nodes {
		if superseded[id] {
			continue
		}
		status, _ := canonicalKindStatus(cfg, n.Kind, n.Status)
		binding[id] = strings.EqualFold(status, config.StatusAccepted)
	}
	return binding
}
