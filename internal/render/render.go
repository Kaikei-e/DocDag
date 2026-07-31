// Package render writes graphs, findings and statistics in the output formats
// the CLI offers. Every renderer sorts before writing: output is a contract.
package render

import (
	"io"

	"github.com/Kaikei-e/DocDag/internal/model"
)

// LabelLimit is the maximum rendered node label width; longer labels are cut to
// LabelLimit-3 characters and suffixed with an ellipsis.
const LabelLimit = 40

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
func Label(n *model.Node, limit int) string { return "" }

// Mermaid renders the typed graph as a `graph LR` diagram.
func Mermaid(w io.Writer, g *model.Graph, opts Options) error { return model.ErrNotImplemented }

// DOT renders the typed graph as a Graphviz digraph.
func DOT(w io.Writer, g *model.Graph, opts Options) error { return model.ErrNotImplemented }

// NodeLinkJSON renders the typed graph as node-link JSON.
func NodeLinkJSON(w io.Writer, g *model.Graph, opts Options) error {
	return model.ErrNotImplemented
}
