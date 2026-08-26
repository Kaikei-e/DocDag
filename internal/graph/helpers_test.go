package graph

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

var errInvalidFixtureFrontmatter = errors.New("yaml: fixture frontmatter does not decode")

func testNode(id, status string) *model.Node {
	return testNodeAttrs(id, status, nil)
}

// testNodeAttrs mirrors the status onto both the field and the attribute map so
// a rule reading the attribute and code reading the field observe one value.
// Every recognized key gets its own line so a finding's location is legible.
func testNodeAttrs(id, status string, attrs map[string]string) *model.Node {
	n := &model.Node{
		ID:     model.ID(id),
		Path:   id + ".md",
		Title:  "Decision " + id,
		Status: status,
		Date:   "2025-01-01",
		Attrs:  map[string]any{},
		Line:   testFrontmatterLine,
		KeyLines: map[string]int{
			"title":      testTitleLine,
			"status":     testStatusLine,
			"supersedes": testSupersedesLine,
			"depends-on": testDependsOnLine,
		},
	}
	if status != "" {
		n.Attrs[config.DefaultStatusField] = status
	}
	for k, v := range attrs {
		n.Attrs[k] = v
	}
	return n
}

// Frontmatter lines every test node carries, so a location assertion names a
// key rather than a number nobody can trace.
const (
	testFrontmatterLine = 1
	testTitleLine       = 2
	testStatusLine      = 3
	testSupersedesLine  = 4
	testDependsOnLine   = 5
	testBodyLine        = 8
)

func testNodeLocation(id string, line int) model.Location {
	return model.Location{Path: id + ".md", Line: line}
}

func testEdge(from, to string, t model.EdgeType) model.Edge {
	return model.Edge{From: model.ID(from), To: model.ID(to), Type: t, Origin: model.OriginStructured}
}

func testDerivedEdge(from, to string, t model.EdgeType) model.Edge {
	return model.Edge{From: model.ID(from), To: model.ID(to), Type: t, Origin: model.OriginDerived}
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

func testDoc(id string, frontmatter map[string]any, body string) *parse.Document {
	doc := &parse.Document{
		Path:            id + ".md",
		Name:            id + ".md",
		ID:              model.ID(id),
		Frontmatter:     frontmatter,
		Body:            body,
		HasFrontmatter:  true,
		MatchesPattern:  true,
		FrontmatterLine: testFrontmatterLine,
		BodyLine:        testBodyLine,
		KeyLines:        make(map[string]int, len(frontmatter)),
	}
	for i, key := range slices.Sorted(maps.Keys(frontmatter)) {
		doc.KeyLines[key] = testFrontmatterLine + 1 + i
	}
	return doc
}

func testIDs(raw ...string) []model.ID {
	out := make([]model.ID, 0, len(raw))
	for _, r := range raw {
		out = append(out, model.ID(r))
	}
	return out
}

func testAdj(pairs map[string][]string) map[model.ID][]model.ID {
	out := make(map[model.ID][]model.ID, len(pairs))
	for from, tos := range pairs {
		out[model.ID(from)] = testIDs(tos...)
	}
	return out
}

func testAttrEq(v string) config.AttrCondition {
	value := v
	return config.AttrCondition{Eq: &value}
}

func testAttrNot(v string) config.AttrCondition {
	value := v
	return config.AttrCondition{Not: &value}
}

func testAssertIDs(t *testing.T, what string, got, want []model.ID) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

func testAssertAdjacency(t *testing.T, got, want map[model.ID][]model.ID) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("adjacency has %d entries, want %d: %v", len(got), len(want), got)
	}
	for id, wantNeighbors := range want {
		gotNeighbors, ok := got[id]
		if !ok {
			t.Fatalf("adjacency is missing an entry for %s: %v", id, got)
		}
		if !slices.Equal(gotNeighbors, wantNeighbors) {
			t.Fatalf("adjacency[%s] = %v, want %v", id, gotNeighbors, wantNeighbors)
		}
	}
}

func testFindingsFor(findings []model.Finding, rule string) []model.Finding {
	out := []model.Finding{}
	for _, f := range findings {
		if f.Rule == rule {
			out = append(out, f)
		}
	}
	return out
}

func testFindingIDs(findings []model.Finding, rule string) []model.ID {
	out := []model.ID{}
	for _, f := range testFindingsFor(findings, rule) {
		out = append(out, f.ID)
	}
	return out
}

func testRuleNames(findings []model.Finding) []string {
	out := []string{}
	for _, f := range findings {
		out = append(out, f.Rule)
	}
	return out
}

func testSeverityRank(s model.Severity) int {
	if s == model.SeverityError {
		return 0
	}
	return 1
}

func testFindingKey(f model.Finding) string {
	return fmt.Sprintf("%d\x00%s\x00%08d\x00%s\x00%s\x00%s",
		testSeverityRank(f.Severity), f.Location.Path, f.Location.Line, f.Rule, f.ID, f.Detail)
}

func testAssertFindings(t *testing.T, what string, got, want []model.Finding) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s = %+v, want %+v", what, got, want)
	}
}

func testAssertSortedFindings(t *testing.T, findings []model.Finding) {
	t.Helper()
	for i := 1; i < len(findings); i++ {
		if testFindingKey(findings[i-1]) > testFindingKey(findings[i]) {
			t.Fatalf("findings out of order at index %d: %+v precedes %+v", i, findings[i-1], findings[i])
		}
	}
}

func testAssertSingleFinding(t *testing.T, findings []model.Finding, rule string, severity model.Severity, id model.ID) model.Finding {
	t.Helper()
	matches := testFindingsFor(findings, rule)
	if len(matches) != 1 {
		t.Fatalf("got %d %s findings, want 1: %+v", len(matches), rule, findings)
	}
	if matches[0].Severity != severity {
		t.Fatalf("%s severity = %q, want %q", rule, matches[0].Severity, severity)
	}
	if matches[0].ID != id {
		t.Fatalf("%s id = %q, want %q", rule, matches[0].ID, id)
	}
	return matches[0]
}

// testMustNotHang guards the cycle-safety claims: a walk that loops forever is a
// timeout, not a wrong answer, so it needs its own deadline.
func testMustNotHang(t *testing.T, within time.Duration, fn func()) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn()
	}()
	select {
	case <-done:
	case <-time.After(within):
		t.Fatalf("call did not return within %s", within)
	}
}

func testChainID(i int) string {
	return fmt.Sprintf("n%05d", i)
}
