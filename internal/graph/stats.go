package graph

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
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

// FieldUsage is how one frontmatter field is used across the corpus: how many
// documents write it, whether the configuration retired it, and the day a
// document that writes it last changed. LastChange is empty where no repository
// answered — a report says what it knows rather than failing for want of git.
type FieldUsage struct {
	Field      string `json:"field"`
	Documents  int    `json:"documents"`
	Deprecated bool   `json:"deprecated"`
	LastChange string `json:"last_change,omitempty"`
}

// ComputeFieldUsage summarises the frontmatter fields the corpus writes, plus
// the ones it declares and nobody writes: a migration is finished exactly when
// a retired field's count reaches zero, so that row has to outlive the last
// document that carried it. changed maps a document path onto the day it last
// changed; a path it does not hold contributes no date.
func ComputeFieldUsage(g *model.Graph, cfg config.Config, changed map[string]string) []FieldUsage {
	counts := make(map[string]int, len(g.Nodes))
	last := make(map[string]string, len(g.Nodes))
	for _, id := range g.NodeIDs() {
		n := g.Nodes[id]
		day := changed[n.Path]
		// The node's attributes are the frontmatter as the corpus reads it, so
		// on a multi-kind corpus the kind the directory answered is among them
		// whether or not the document wrote it down.
		for key := range n.Attrs {
			counts[key]++
			// ISO 8601 days sort chronologically, so the largest is the latest.
			if day > last[key] {
				last[key] = day
			}
		}
	}
	for _, name := range cfg.DeclaredFields() {
		if _, counted := counts[name]; !counted {
			counts[name] = 0
		}
	}
	usage := make([]FieldUsage, 0, len(counts))
	for _, name := range slices.Sorted(maps.Keys(counts)) {
		usage = append(usage, FieldUsage{
			Field:      name,
			Documents:  counts[name],
			Deprecated: cfg.FieldDeprecated(name),
			LastChange: last[name],
		})
	}
	// The rarest fields are the ones a removal decision is about, so the table
	// counts down towards them; ties read alphabetically.
	slices.SortFunc(usage, func(a, b FieldUsage) int {
		return cmp.Or(b.Documents-a.Documents, strings.Compare(a.Field, b.Field))
	})
	return usage
}

// TopicCount is how many clauses speak to one subject. It is what a corpus
// watches its topic granularity with: a subject a paragraph defines carries a
// handful of clauses, and one carrying dozens is a subject that says too little
// to compare clauses under.
type TopicCount struct {
	Topic   model.ID `json:"topic"`
	Clauses int      `json:"clauses"`
}

// ModalityCount is how many documents state one modality. Every declared value
// is a row, at zero where nobody states it: a standard with no MUST_NOT is a
// fact about the standard, and a missing row would read as a corpus that has
// not been asked.
type ModalityCount struct {
	Modality string `json:"modality"`
	Count    int    `json:"count"`
}

// Statistics is the degree-based corpus summary. Every field is computable in
// O(V+E); nothing here needs a full path enumeration.
//
// The last three answer only for a corpus that declares the modality vocabulary
// and the subject edge, and are left out of the report entirely otherwise.
type Statistics struct {
	Documents           int              `json:"documents"`
	Edges               []EdgeCount      `json:"edges"`
	Binding             int              `json:"binding"`
	ChainDepth          []DepthCount     `json:"chain_depth"`
	Orphans             int              `json:"orphans"`
	OrphanRate          float64          `json:"orphan_rate"`
	TopReferenced       []ReferenceCount `json:"top_referenced"`
	Topics              []TopicCount     `json:"topics,omitempty"`
	Modalities          []ModalityCount  `json:"modalities,omitempty"`
	SuppressedConflicts int              `json:"suppressed_conflicts,omitempty"`
}

// TopReferencedLimit caps the reference-layer in-degree ranking.
const TopReferencedLimit = 10

// ComputeStats summarises the graph on one day: what is binding, and how many
// conflicts a recorded exception defeats, are answers about a moment wherever a
// kind declares a period.
func ComputeStats(g *model.Graph, cfg config.Config, asOf time.Time) Statistics {
	stats := Statistics{
		Documents:           len(g.Nodes),
		Edges:               edgeCounts(g, cfg),
		Binding:             len(BindingSet(g, cfg, asOf)),
		ChainDepth:          chainDepths(g),
		TopReferenced:       topReferenced(g),
		Topics:              topicCounts(g, cfg),
		Modalities:          modalityCounts(g, cfg),
		SuppressedConflicts: suppressedConflicts(g, cfg, asOf),
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

// topicCounts ranks the subjects by how many clauses speak to them, the busiest
// first and ties alphabetically, so the report opens on the subject whose
// granularity is most likely wrong. A declared subject nobody speaks to is a
// row at zero: it is either a subject the standard has not reached yet or one
// it has left behind, and both are worth seeing.
func topicCounts(g *model.Graph, cfg config.Config) []TopicCount {
	counts := map[model.ID]int{}
	spec, declared := cfg.Edge(config.EdgeAbout)
	if !declared {
		return []TopicCount{}
	}
	for _, id := range g.NodeIDs() {
		if slices.Contains(spec.To, g.Nodes[id].Kind) {
			counts[id] = 0
		}
	}
	for _, e := range g.EdgesOfType(model.EdgeType(spec.Name)) {
		if _, known := g.Node(e.To); !known {
			continue
		}
		counts[e.To]++
	}
	ranked := make([]TopicCount, 0, len(counts))
	for _, id := range slices.Sorted(maps.Keys(counts)) {
		ranked = append(ranked, TopicCount{Topic: id, Clauses: counts[id]})
	}
	slices.SortStableFunc(ranked, func(a, b TopicCount) int { return b.Clauses - a.Clauses })
	return ranked
}

// modalityCounts is the distribution of the declared modality vocabulary over
// the documents that state one, in the order the vocabulary declares them. A
// value outside the vocabulary is counted after them, alphabetically: it is an
// unknown_field_value, and leaving it out of the report would hide a document
// the count is missing.
func modalityCounts(g *model.Graph, cfg config.Config) []ModalityCount {
	declared := modalityVocabulary(cfg)
	if len(declared) == 0 {
		return []ModalityCount{}
	}
	counts := make(map[string]int, len(declared))
	for _, value := range declared {
		counts[value] = 0
	}
	for _, id := range g.NodeIDs() {
		if value, written := g.Nodes[id].Attr(config.FieldModality); written {
			counts[value]++
		}
	}
	distribution := make([]ModalityCount, 0, len(counts))
	for _, value := range declared {
		distribution = append(distribution, ModalityCount{Modality: value, Count: counts[value]})
	}
	for _, value := range slices.Sorted(maps.Keys(counts)) {
		if !slices.Contains(declared, value) {
			distribution = append(distribution, ModalityCount{Modality: value, Count: counts[value]})
		}
	}
	return distribution
}

// modalityVocabulary returns the values a modality field declares, over every
// kind that declares one, in declaration order. Two kinds declaring different
// vocabularies is a corpus with two vocabularies, and the report is over the
// union.
func modalityVocabulary(cfg config.Config) []string {
	values := []string{}
	for _, kind := range append([]string{""}, cfg.KindNames()...) {
		spec, declared := cfg.Field(kind, config.FieldModality)
		if !declared {
			continue
		}
		for _, value := range spec.OneOf {
			if !slices.Contains(values, value) {
				values = append(values, value)
			}
		}
	}
	return values
}

// suppressedConflicts counts the conflicts a recorded exception defeats. It is
// the number a corpus watches: exceptions are decisions, and a standard growing
// them faster than it grows clauses is one being written around.
func suppressedConflicts(g *model.Graph, cfg config.Config, asOf time.Time) int {
	count := 0
	for _, c := range ModalityConflicts(g, cfg, asOf) {
		if c.Suppressed {
			count++
		}
	}
	return count
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
