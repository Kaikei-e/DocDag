// Package render writes graphs, findings and statistics in the output formats
// the CLI offers. Every renderer sorts before writing: output is a contract.
package render

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/Kaikei-e/DocDag/internal/model"
)

// LabelLimit is the maximum rendered node label width; longer labels are cut to
// LabelLimit-3 characters and suffixed with an ellipsis.
const LabelLimit = 40

const labelEllipsis = "..."

// Options control graph rendering.
type Options struct {
	IncludeRefs bool
	LabelLimit  int
}

// NodeLinkNode is one document in the node-link JSON export.
type NodeLinkNode struct {
	ID     model.ID `json:"id"`
	Title  string   `json:"title"`
	Status string   `json:"status"`
	Path   string   `json:"path"`
}

// NodeLinkEdge is one edge in the node-link JSON export.
type NodeLinkEdge struct {
	Source model.ID       `json:"source"`
	Target model.ID       `json:"target"`
	Type   model.EdgeType `json:"type"`
	Origin model.Origin   `json:"origin"`
}

// NodeLink is the node-link JSON document.
type NodeLink struct {
	Nodes []NodeLinkNode `json:"nodes"`
	Links []NodeLinkEdge `json:"links"`
}

// Label formats a node label as "<id> <title>", truncated to limit characters
// with double quotes replaced by single quotes.
func Label(n *model.Node, limit int) string {
	if limit <= 0 {
		limit = LabelLimit
	}
	text := n.ID.String()
	if n.Title != "" {
		text += " " + n.Title
	}
	// A title written as a YAML block scalar carries line breaks, and a label
	// spanning lines is not valid mermaid.
	text = strings.Join(strings.Fields(text), " ")
	text = strings.ReplaceAll(text, `"`, "'")

	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:max(limit-len(labelEllipsis), 0)]) + labelEllipsis
}

// Mermaid renders the typed graph as a `graph LR` diagram.
func Mermaid(w io.Writer, g *model.Graph, opts Options) error {
	out := &errWriter{w: w}
	out.printf("graph LR\n")
	for _, n := range sortedNodes(g) {
		out.printf("  %s[\"%s\"]\n", n.ID, Label(n, opts.LabelLimit))
	}
	for _, e := range typedEdges(g) {
		out.printf("  %s -->|%s| %s\n", e.From, e.Type, e.To)
	}
	for _, e := range referenceEdges(g, opts) {
		out.printf("  %s -.-> %s\n", e.From, e.To)
	}
	if out.err != nil {
		return fmt.Errorf("render mermaid: %w", out.err)
	}
	return nil
}

// DOT renders the typed graph as a Graphviz digraph.
func DOT(w io.Writer, g *model.Graph, opts Options) error {
	out := &errWriter{w: w}
	out.printf("digraph docdag {\n")
	out.printf("  rankdir=LR;\n")
	for _, n := range sortedNodes(g) {
		out.printf("  \"%s\" [label=\"%s\"];\n", n.ID, Label(n, opts.LabelLimit))
	}
	for _, e := range typedEdges(g) {
		out.printf("  \"%s\" -> \"%s\" [label=\"%s\"];\n", e.From, e.To, e.Type)
	}
	for _, e := range referenceEdges(g, opts) {
		out.printf("  \"%s\" -> \"%s\" [style=dotted];\n", e.From, e.To)
	}
	out.printf("}\n")
	if out.err != nil {
		return fmt.Errorf("render dot: %w", out.err)
	}
	return nil
}

// NodeLinkJSON renders the typed graph as node-link JSON.
func NodeLinkJSON(w io.Writer, g *model.Graph, opts Options) error {
	nodes := sortedNodes(g)
	doc := NodeLink{
		Nodes: make([]NodeLinkNode, 0, len(nodes)),
		Links: []NodeLinkEdge{},
	}
	for _, n := range nodes {
		doc.Nodes = append(doc.Nodes, NodeLinkNode{ID: n.ID, Title: n.Title, Status: n.Status, Path: n.Path})
	}
	for _, e := range typedEdges(g) {
		doc.Links = append(doc.Links, NodeLinkEdge{Source: e.From, Target: e.To, Type: e.Type, Origin: e.Origin})
	}
	for _, e := range referenceEdges(g, opts) {
		doc.Links = append(doc.Links, NodeLinkEdge{Source: e.From, Target: e.To, Type: e.Type, Origin: e.Origin})
	}
	if err := writeJSON(w, doc); err != nil {
		return fmt.Errorf("render node-link json: %w", err)
	}
	return nil
}

// errWriter collects the first write failure so renderers stay linear.
type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) printf(format string, a ...any) {
	if e.err != nil {
		return
	}
	_, e.err = fmt.Fprintf(e.w, format, a...)
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	// Nothing here is embedded in HTML, and an escaped edge arrow makes a
	// finding unreadable to the person tailing the report.
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}

func sortedNodes(g *model.Graph) []*model.Node {
	out := make([]*model.Node, 0, len(g.Nodes))
	for _, id := range slices.Sorted(maps.Keys(g.Nodes)) {
		out = append(out, g.Nodes[id])
	}
	return out
}

func typedEdges(g *model.Graph) []model.Edge {
	out := connectedEdges(g, g.Edges)
	slices.SortFunc(out, func(a, b model.Edge) int {
		return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.Type, b.Type), cmp.Compare(a.To, b.To))
	})
	return out
}

func referenceEdges(g *model.Graph, opts Options) []model.Edge {
	if !opts.IncludeRefs {
		return nil
	}
	out := connectedEdges(g, g.RefEdges)
	slices.SortFunc(out, func(a, b model.Edge) int {
		return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To))
	})
	return out
}

// connectedEdges drops edges whose endpoints are not rendered nodes: a diagram
// must not invent documents the corpus does not hold.
func connectedEdges(g *model.Graph, edges []model.Edge) []model.Edge {
	out := make([]model.Edge, 0, len(edges))
	for _, e := range edges {
		if _, ok := g.Nodes[e.From]; !ok {
			continue
		}
		if _, ok := g.Nodes[e.To]; !ok {
			continue
		}
		out = append(out, e)
	}
	return out
}
