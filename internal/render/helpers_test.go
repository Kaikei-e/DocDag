package render

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
)

var errWriterClosed = errors.New("render: writer is closed")

// failingWriter proves renderers surface write failures instead of swallowing
// them behind a nil error.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errWriterClosed }

func testNode(id, title, status, path string) *model.Node {
	return &model.Node{
		ID:     model.ID(id),
		Path:   path,
		Title:  title,
		Status: status,
		Attrs:  map[string]any{config.DefaultStatusField: status},
	}
}

func testEdge(from, to string, t model.EdgeType) model.Edge {
	return model.Edge{From: model.ID(from), To: model.ID(to), Type: t, Origin: model.OriginStructured}
}

func testRefEdge(from, to string) model.Edge {
	return model.Edge{From: model.ID(from), To: model.ID(to), Origin: model.OriginReference}
}

func testGraph(nodes []*model.Node, edges, refEdges []model.Edge) *model.Graph {
	g := &model.Graph{
		Nodes:    make(map[model.ID]*model.Node, len(nodes)),
		Edges:    edges,
		RefEdges: refEdges,
	}
	for _, n := range nodes {
		g.Nodes[n.ID] = n
	}
	return g
}

// testOKBasicGraph mirrors testdata/fixtures/ok-basic. Edges are listed out of
// order on purpose: the renderers own the ordering, not the caller.
func testOKBasicGraph() *model.Graph {
	return testGraph(
		[]*model.Node{
			testNode("0001", "Poll every feed on a fixed five-minute interval", "superseded", "000001.md"),
			testNode("0002", "Poll feeds with adaptive backoff", "superseded", "000002.md"),
			testNode("0003", "Use a message queue for ingestion hand-off", "accepted", "000003.md"),
			testNode("0004", "Schedule feed polling from the ingestion queue", "accepted", "000004.md"),
			testNode("0005", "Deduplicate articles by canonical URL", "accepted", "000005.md"),
			testNode("0006", "Reject binary attachments during ingestion", "rejected", "000006.md"),
		},
		[]model.Edge{
			testEdge("0005", "0003", config.EdgeDependsOn),
			testEdge("0002", "0001", config.EdgeSupersedes),
			testEdge("0004", "0002", config.EdgeSupersedes),
			testEdge("0004", "0003", config.EdgeDependsOn),
		},
		[]model.Edge{
			testRefEdge("0005", "0003"),
			testRefEdge("0002", "0001"),
			testRefEdge("0004", "0003"),
			testRefEdge("0001", "0002"),
		},
	)
}

// testFanInGraph mirrors testdata/fixtures/fan-in: two documents superseded by
// one successor, no reference layer.
func testFanInGraph() *model.Graph {
	return testGraph(
		[]*model.Node{
			testNode("0003", "Authenticate every client with short-lived signed tokens", "accepted", "0003-authenticate-with-signed-tokens.md"),
			testNode("0001", "Authenticate browsers with session cookies", "superseded", "0001-authenticate-with-session-cookies.md"),
			testNode("0002", "Authenticate integrations with API keys", "superseded", "0002-authenticate-with-api-keys.md"),
		},
		[]model.Edge{
			testEdge("0003", "0002", config.EdgeSupersedes),
			testEdge("0003", "0001", config.EdgeSupersedes),
		},
		nil,
	)
}

func testEmptyGraph() *model.Graph {
	return testGraph(nil, nil, nil)
}

type testRenderer func(w io.Writer, g *model.Graph, opts Options) error

func testRenderers() map[string]testRenderer {
	return map[string]testRenderer{
		"mermaid": Mermaid,
		"dot":     DOT,
		"json":    NodeLinkJSON,
	}
}

func testRender(t *testing.T, fn testRenderer, g *model.Graph, opts Options) string {
	t.Helper()
	var buf bytes.Buffer
	if err := fn(&buf, g, opts); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func testAssertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name)
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	if got != string(want) {
		t.Errorf("rendering does not match %s\ngot:\n%s\nwant:\n%s", path, got, want)
	}
}
