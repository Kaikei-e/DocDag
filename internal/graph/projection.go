package graph

import (
	"slices"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// A projection reads as a boolean attribute: the string "true" where it holds
// and "false" where it does not, so attr: {enforced: {eq: "true"}} and
// attr: {enforced: {not: "true"}} both say what they look like.
const (
	ProjectionTrue  = "true"
	ProjectionFalse = "false"
)

// ProjectionValue renders one projection result the way an attribute clause,
// and a listing column, read it.
func ProjectionValue(held bool) string {
	if held {
		return ProjectionTrue
	}
	return ProjectionFalse
}

// Projections holds the evaluated projections of one graph: for every declared
// projection, the documents it holds for.
type Projections struct {
	names []string
	held  map[string]map[model.ID]bool
}

// Names returns the declared projections in configuration order.
func (p Projections) Names() []string {
	return slices.Clone(p.names)
}

// Declares reports whether a name is a projection at all, which is what makes
// it a virtual attribute rather than a frontmatter key.
func (p Projections) Declares(name string) bool {
	_, ok := p.held[name]
	return ok
}

// Holds reports whether a projection holds for one document. A name no
// configuration declares holds nowhere.
func (p Projections) Holds(name string, id model.ID) bool {
	return p.held[name][id]
}

// Set lists the documents a projection holds for, sorted.
func (p Projections) Set(name string) []model.ID {
	ids := make([]model.ID, 0, len(p.held[name]))
	for id := range p.held[name] {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// EvalProjections evaluates every configured projection over every document, in
// dependency order, so a projection that reads another as an attribute sees the
// evaluated value rather than an absent one.
func EvalProjections(g *model.Graph, cfg config.Config) Projections {
	return evalProjections(g, cfg, newEdgeIndex(g))
}

// evalProjections is EvalProjections over an edge index the caller already
// built, so evaluating rules and projections together indexes the graph once.
func evalProjections(g *model.Graph, cfg config.Config, ix edgeIndex) Projections {
	p := Projections{
		names: cfg.ProjectionNames(),
		held:  make(map[string]map[model.ID]bool, len(cfg.Projections)),
	}
	// Every declared projection is a virtual attribute from the first
	// evaluation onwards, whether or not it holds anywhere and whether or not
	// the order below could reach it: shadowing must not depend on evaluation.
	for _, name := range p.names {
		p.held[name] = map[model.ID]bool{}
	}
	if len(p.names) == 0 {
		return p
	}
	// The context shares the maps this loop fills, so each projection reads the
	// results of the ones it depends on.
	ctx := evalContext{g: g, ix: ix, projected: p}
	for _, spec := range projectionOrder(cfg) {
		held := make(map[model.ID]bool)
		for _, id := range g.NodeIDs() {
			if ctx.matchProjection(spec, id) {
				held[id] = true
			}
		}
		p.held[spec.Name] = held
	}
	return p
}

// matchProjection reports whether a projection holds for one document: its when
// block holds, or one of its alternatives does.
func (e evalContext) matchProjection(spec config.ProjectionSpec, id model.ID) bool {
	return slices.ContainsFunc(spec.Whens(), func(when config.Condition) bool {
		return e.match(when, id)
	})
}

// projectionOrder returns the projections in dependency order, so a projection
// that reads another as an attribute is evaluated after it. Configuration
// validation rejects a reference cycle; a cyclic configuration that reaches
// here anyway leaves its members out of the order rather than looping, and they
// hold nowhere. Ties keep declaration order, so the walk is deterministic.
func projectionOrder(cfg config.Config) []config.ProjectionSpec {
	pending := make(map[string]int, len(cfg.Projections))
	dependants := make(map[string][]string, len(cfg.Projections))
	for _, spec := range cfg.Projections {
		for _, key := range spec.AttrKeys() {
			if key == spec.Name {
				continue
			}
			if _, ok := cfg.Projection(key); !ok {
				continue
			}
			pending[spec.Name]++
			dependants[key] = append(dependants[key], spec.Name)
		}
	}

	ordered := make([]config.ProjectionSpec, 0, len(cfg.Projections))
	ready := make([]string, 0, len(cfg.Projections))
	for _, spec := range cfg.Projections {
		if pending[spec.Name] == 0 {
			ready = append(ready, spec.Name)
		}
	}
	for len(ready) > 0 {
		name := ready[0]
		ready = ready[1:]
		spec, ok := cfg.Projection(name)
		if !ok {
			continue
		}
		ordered = append(ordered, spec)
		for _, dependant := range dependants[name] {
			pending[dependant]--
			if pending[dependant] == 0 {
				ready = append(ready, dependant)
			}
		}
	}
	return ordered
}
