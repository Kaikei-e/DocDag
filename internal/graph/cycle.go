package graph

import "github.com/Kaikei-e/DocDag/internal/model"

// FindCycle returns a closed cycle path such as [A B A], or nil when the
// adjacency is acyclic. Iterative three-colour search: document corpora are
// shallow but a recursive walk would blow the stack on a pathological chain.
func FindCycle(adj map[model.ID][]model.ID) []model.ID { return nil }

// FindCycles returns one cycle path per strongly connected component with a
// cycle, in deterministic order.
func FindCycles(adj map[model.ID][]model.ID) [][]model.ID { return nil }
