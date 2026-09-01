package brief

import (
	"slices"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// The relations a related entry carries. They are the reader's answer to "why
// am I being shown this document", which a hop count cannot give: the subject a
// clause speaks to, the other clauses speaking to it, the exception recorded in
// either direction, and the requirement an option leans on.
const (
	RelationAbout      = "about"
	RelationSameTopic  = "same topic"
	RelationExcepts    = "excepts"
	RelationExceptedBy = "excepted by"
	RelationInterop    = "interop"
)

// relatedEntries assembles the normative neighbourhood of one document: what it
// is about, the binding clauses about the same subjects, the exceptions
// recorded in either direction and the requirements its options lean on.
//
// It is empty for a configuration that declares none of these edges, which is
// every corpus that is not a standard, so a brief under the adr preset is the
// brief it always was. The entries are kept whether or not they bind — the
// subject of a clause is a definition, not a decision, and an exception is
// worth reading exactly where it does not bind on its own.
func relatedEntries(g *model.Graph, cfg config.Config, opts Options, taken, binding map[model.ID]bool, id model.ID) ([]Entry, error) {
	// The named relations come before the shared subject, because a document is
	// reported under the first group that claims it and the specific relation
	// is the one worth reading: an exception is about this pair of clauses,
	// where sharing a subject is about the whole group.
	groups := []struct {
		relation string
		ids      []model.ID
	}{
		{RelationAbout, topicsOf(g, cfg, id)},
		{RelationExcepts, neighbours(g, cfg, config.EdgeExcepts, id, false)},
		{RelationExceptedBy, neighbours(g, cfg, config.EdgeExcepts, id, true)},
		{RelationInterop, neighbours(g, cfg, config.EdgeInterop, id, false)},
		{RelationSameTopic, sameTopic(g, cfg, binding, id)},
	}
	out := []Entry{}
	for _, group := range groups {
		for _, neighbor := range group.ids {
			if taken[neighbor] {
				continue
			}
			taken[neighbor] = true
			e, err := entry(g, opts.Section, neighbor)
			if err != nil {
				return nil, err
			}
			e.Relation = group.relation
			out = append(out, e)
		}
	}
	return out, nil
}

// neighbours returns the documents one declared edge type reaches from a
// document, or reaches it from, sorted and known to the corpus. An edge type
// the configuration does not declare reaches nothing.
func neighbours(g *model.Graph, cfg config.Config, t model.EdgeType, id model.ID, inbound bool) []model.ID {
	if _, declared := cfg.Edge(t); !declared {
		return nil
	}
	adj := graph.Adjacency(g, t)
	if inbound {
		adj = graph.Reverse(g, t)
	}
	known := make([]model.ID, 0, len(adj[id]))
	for _, neighbor := range adj[id] {
		if _, ok := g.Node(neighbor); ok {
			known = append(known, neighbor)
		}
	}
	return known
}

func topicsOf(g *model.Graph, cfg config.Config, id model.ID) []model.ID {
	return neighbours(g, cfg, config.EdgeAbout, id, false)
}

// sameTopic names the binding clauses that speak to a subject this document
// speaks to. Binding alone, because a clause nothing holds in force says
// nothing this one has to agree with — which is the set modality_conflict
// compares, so the brief shows what the check looked at.
func sameTopic(g *model.Graph, cfg config.Config, binding map[model.ID]bool, id model.ID) []model.ID {
	topics := topicsOf(g, cfg, id)
	if len(topics) == 0 {
		return nil
	}
	speakers := graph.Reverse(g, config.EdgeAbout)
	found := []model.ID{}
	for _, topic := range topics {
		for _, clause := range speakers[topic] {
			if clause == id || !binding[clause] {
				continue
			}
			if _, ok := g.Node(clause); ok {
				found = append(found, clause)
			}
		}
	}
	slices.Sort(found)
	return slices.Compact(found)
}

// suppressed reports the conflicts about this document that a recorded
// exception defeats, one line each. They are the reading a reader needs to make
// sense of a permission standing beside a prohibition, and they appear nowhere
// else: validate leaves them out unless asked.
func suppressed(g *model.Graph, cfg config.Config, id model.ID) []string {
	lines := []string{}
	for _, c := range graph.ModalityConflicts(g, cfg) {
		if !c.Suppressed || (c.A != id && c.B != id) {
			continue
		}
		lines = append(lines, c.Suppression())
	}
	return lines
}
