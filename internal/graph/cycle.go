package graph

import (
	"slices"
	"strings"

	"github.com/Kaikei-e/DocDag/internal/model"
)

// Search colours of the iterative depth-first walks in this package.
const (
	colorWhite = iota
	colorGray
	colorBlack
)

// visitFrame is one entry of an explicit depth-first stack: the node and how
// many of its neighbours have been expanded.
type visitFrame struct {
	id   model.ID
	next int
}

// FindCycle returns a closed cycle path such as [A B A], or nil when the
// adjacency is acyclic. Iterative three-colour search: document corpora are
// shallow but a recursive walk would blow the stack on a pathological chain.
func FindCycle(adj map[model.ID][]model.ID) []model.ID {
	color := make(map[model.ID]int, len(adj))
	for _, root := range sortedIDs(adj) {
		if color[root] != colorWhite {
			continue
		}
		if cycle := walkForCycle(adj, color, root); len(cycle) > 0 {
			return cycle
		}
	}
	return nil
}

func walkForCycle(adj map[model.ID][]model.ID, color map[model.ID]int, root model.ID) []model.ID {
	color[root] = colorGray
	stack := []visitFrame{{id: root}}
	for len(stack) > 0 {
		frame := &stack[len(stack)-1]
		neighbors := adj[frame.id]
		if frame.next >= len(neighbors) {
			color[frame.id] = colorBlack
			stack = stack[:len(stack)-1]
			continue
		}
		next := neighbors[frame.next]
		frame.next++
		switch color[next] {
		case colorGray:
			return closedPath(stack, next)
		case colorWhite:
			color[next] = colorGray
			stack = append(stack, visitFrame{id: next})
		}
	}
	return nil
}

func closedPath(stack []visitFrame, at model.ID) []model.ID {
	start := 0
	for i := range stack {
		if stack[i].id == at {
			start = i
			break
		}
	}
	path := make([]model.ID, 0, len(stack)-start+1)
	for _, frame := range stack[start:] {
		path = append(path, frame.id)
	}
	return append(path, at)
}

// FindCycles returns one cycle path per strongly connected component with a
// cycle, in deterministic order.
func FindCycles(adj map[model.ID][]model.ID) [][]model.ID {
	var cycles [][]model.ID
	for _, component := range cyclicComponents(adj) {
		if cycle := FindCycle(inducedSubgraph(adj, component)); len(cycle) > 0 {
			cycles = append(cycles, cycle)
		}
	}
	return cycles
}

// cyclicComponents returns the strongly connected components that contain a
// cycle, each sorted, ordered by their lowest member.
func cyclicComponents(adj map[model.ID][]model.ID) [][]model.ID {
	index := make(map[model.ID]int, len(adj))
	low := make(map[model.ID]int, len(adj))
	onStack := make(map[model.ID]bool, len(adj))
	var (
		pending    []model.ID
		components [][]model.ID
		counter    int
	)

	for _, root := range sortedIDs(adj) {
		if _, seen := index[root]; seen {
			continue
		}
		index[root], low[root] = counter, counter
		counter++
		pending = append(pending, root)
		onStack[root] = true

		work := []visitFrame{{id: root}}
		for len(work) > 0 {
			frame := &work[len(work)-1]
			neighbors := adj[frame.id]
			if frame.next < len(neighbors) {
				next := neighbors[frame.next]
				frame.next++
				if _, seen := index[next]; !seen {
					index[next], low[next] = counter, counter
					counter++
					pending = append(pending, next)
					onStack[next] = true
					work = append(work, visitFrame{id: next})
					continue
				}
				if onStack[next] && index[next] < low[frame.id] {
					low[frame.id] = index[next]
				}
				continue
			}

			id := frame.id
			work = work[:len(work)-1]
			if len(work) > 0 {
				parent := work[len(work)-1].id
				if low[id] < low[parent] {
					low[parent] = low[id]
				}
			}
			if low[id] != index[id] {
				continue
			}
			var component []model.ID
			for {
				member := pending[len(pending)-1]
				pending = pending[:len(pending)-1]
				onStack[member] = false
				component = append(component, member)
				if member == id {
					break
				}
			}
			components = append(components, component)
		}
	}

	cyclic := make([][]model.ID, 0, len(components))
	for _, component := range components {
		if len(component) == 1 && !slices.Contains(adj[component[0]], component[0]) {
			continue
		}
		slices.Sort(component)
		cyclic = append(cyclic, component)
	}
	slices.SortFunc(cyclic, func(a, b []model.ID) int {
		return strings.Compare(string(a[0]), string(b[0]))
	})
	return cyclic
}

func inducedSubgraph(adj map[model.ID][]model.ID, members []model.ID) map[model.ID][]model.ID {
	inside := make(map[model.ID]bool, len(members))
	for _, id := range members {
		inside[id] = true
	}
	sub := make(map[model.ID][]model.ID, len(members))
	for _, id := range members {
		var neighbors []model.ID
		for _, next := range adj[id] {
			if inside[next] {
				neighbors = append(neighbors, next)
			}
		}
		sub[id] = neighbors
	}
	return sub
}

func sortedIDs(adj map[model.ID][]model.ID) []model.ID {
	ids := make([]model.ID, 0, len(adj))
	for id := range adj {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
