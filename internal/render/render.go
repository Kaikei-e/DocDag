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

// Options control graph rendering. An empty Edges list keeps every typed edge;
// Connected keeps only the documents a rendered typed edge touches.
type Options struct {
	IncludeRefs bool
	Connected   bool
	Edges       []model.EdgeType
	LabelLimit  int
}

// NodeLinkNode is one document in the node-link JSON export.
type NodeLinkNode struct {
	ID     model.ID `json:"id"`
	Title  string   `json:"title"`
	Status string   `json:"status"`
	Path   string   `json:"path"`
}

// NodeLinkEdge is one edge in the node-link JSON export. Attrs holds the edge's
// attributes and is omitted for an edge that carries none, so a corpus whose
// edges declare no attributes exports exactly what it always did.
type NodeLinkEdge struct {
	Source model.ID          `json:"source"`
	Target model.ID          `json:"target"`
	Type   model.EdgeType    `json:"type"`
	Origin model.Origin      `json:"origin"`
	Attrs  map[string]string `json:"attrs,omitempty"`
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
	v := newView(g, opts)
	out := &errWriter{w: w}
	out.printf("graph LR\n")
	for _, n := range v.nodes {
		out.printf("  %s[\"%s\"]\n", n.ID, Label(n, opts.LabelLimit))
	}
	for _, e := range v.typed {
		out.printf("  %s -->|%s| %s\n", e.From, e.Type, e.To)
	}
	for _, e := range v.refs {
		out.printf("  %s -.-> %s\n", e.From, e.To)
	}
	if out.err != nil {
		return fmt.Errorf("render mermaid: %w", out.err)
	}
	return nil
}

// DOT renders the typed graph as a Graphviz digraph.
func DOT(w io.Writer, g *model.Graph, opts Options) error {
	v := newView(g, opts)
	out := &errWriter{w: w}
	out.printf("digraph docdag {\n")
	out.printf("  rankdir=LR;\n")
	for _, n := range v.nodes {
		out.printf("  \"%s\" [label=\"%s\"];\n", n.ID, Label(n, opts.LabelLimit))
	}
	for _, e := range v.typed {
		out.printf("  \"%s\" -> \"%s\" [label=\"%s\"];\n", e.From, e.To, e.Type)
	}
	for _, e := range v.refs {
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
	v := newView(g, opts)
	doc := NodeLink{
		Nodes: make([]NodeLinkNode, 0, len(v.nodes)),
		Links: []NodeLinkEdge{},
	}
	for _, n := range v.nodes {
		doc.Nodes = append(doc.Nodes, NodeLinkNode{ID: n.ID, Title: n.Title, Status: n.Status, Path: n.Path})
	}
	for _, e := range v.typed {
		doc.Links = append(doc.Links, NodeLinkEdge{Source: e.From, Target: e.To, Type: e.Type, Origin: e.Origin, Attrs: e.Attrs})
	}
	for _, e := range v.refs {
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

// view is what one rendering shows: the nodes and edges left after edge-type
// and connectivity filtering, in the order they are written.
type view struct {
	nodes []*model.Node
	typed []model.Edge
	refs  []model.Edge
}

func newView(g *model.Graph, opts Options) view {
	v := view{typed: ofTypes(drawableEdges(g, g.Edges), opts.Edges)}
	slices.SortFunc(v.typed, func(a, b model.Edge) int {
		return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.Type, b.Type), cmp.Compare(a.To, b.To))
	})

	shown := make(map[model.ID]bool, len(g.Nodes))
	if opts.Connected {
		for _, e := range v.typed {
			shown[e.From], shown[e.To] = true, true
		}
	} else {
		for id := range g.Nodes {
			shown[id] = true
		}
	}
	for _, id := range slices.Sorted(maps.Keys(g.Nodes)) {
		if shown[id] {
			v.nodes = append(v.nodes, g.Nodes[id])
		}
	}

	if !opts.IncludeRefs {
		return v
	}
	for _, e := range drawableEdges(g, g.RefEdges) {
		if shown[e.From] && shown[e.To] {
			v.refs = append(v.refs, e)
		}
	}
	slices.SortFunc(v.refs, func(a, b model.Edge) int {
		return cmp.Or(cmp.Compare(a.From, b.From), cmp.Compare(a.To, b.To))
	})
	return v
}

// drawableEdges drops edges whose endpoints are not documents the corpus
// holds: a diagram must not invent documents.
func drawableEdges(g *model.Graph, edges []model.Edge) []model.Edge {
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

// ofTypes keeps the edges of the requested types; an empty request keeps all.
func ofTypes(edges []model.Edge, types []model.EdgeType) []model.Edge {
	if len(types) == 0 {
		return edges
	}
	out := make([]model.Edge, 0, len(edges))
	for _, e := range edges {
		if slices.Contains(types, e.Type) {
			out = append(out, e)
		}
	}
	return out
}
