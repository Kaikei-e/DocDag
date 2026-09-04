package graph

import (
	"path/filepath"
	"testing"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

func testTouchingGraph() *model.Graph {
	return testGraph(
		[]*model.Node{
			testNode("0001", config.StatusSuperseded),
			testNode("0002", config.StatusAccepted),
			testNode("0003", config.StatusAccepted),
		},
		[]model.Edge{testEdge("0002", "0001", config.EdgeSupersedes)},
		nil,
	)
}

func testTouchingFindings() []model.Finding {
	return []model.Finding{
		{Rule: model.RuleStatusDrift, ID: "0001", Location: model.Location{Path: "0001.md", Line: 3}},
		{Rule: model.RuleDanglingRef, ID: "0002", Location: model.Location{Path: "0002.md", Line: 4}},
		{Rule: model.RuleUnknownStatus, ID: "0003", Location: model.Location{Path: "0003.md", Line: 3}},
		{
			Rule: model.RuleIDCollision, ID: "0003",
			Location: model.Location{Path: "0003-other.md", Line: 1},
			Related:  []model.Location{{Path: "0003.md", Line: 1}},
		},
	}
}

func testAssertTouching(t *testing.T, got []model.Finding, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("findings = %+v, want %v", got, want)
	}
	for i := range want {
		if got[i].Location.Path != want[i] {
			t.Errorf("finding %d = %+v, want the one at %s", i, got[i], want[i])
		}
	}
}

func TestTouchingKeepsTheFindingsAboutThePaths(t *testing.T) {
	got := Touching(testTouchingFindings(), testTouchingGraph(), []string{"0003.md"})

	// The collision is filed against another file but relates to this one.
	testAssertTouching(t, got, []string{"0003.md", "0003-other.md"})
}

func TestTouchingKeepsTheFindingsAboutATypedEdgeNeighbour(t *testing.T) {
	// Changing 0002 is what puts 0001 out of step, so its finding is part of
	// the same question.
	got := Touching(testTouchingFindings(), testTouchingGraph(), []string{"0002.md"})

	testAssertTouching(t, got, []string{"0001.md", "0002.md"})
}

func TestTouchingStopsAtTheFirstHop(t *testing.T) {
	g := testGraph(
		[]*model.Node{
			testNode("0001", config.StatusSuperseded),
			testNode("0002", config.StatusSuperseded),
			testNode("0003", config.StatusAccepted),
		},
		[]model.Edge{
			testEdge("0002", "0001", config.EdgeSupersedes),
			testEdge("0003", "0002", config.EdgeSupersedes),
		},
		nil,
	)
	findings := []model.Finding{
		{Rule: model.RuleStatusDrift, ID: "0001", Location: model.Location{Path: "0001.md"}},
		{Rule: model.RuleStatusDrift, ID: "0002", Location: model.Location{Path: "0002.md"}},
		{Rule: model.RuleStatusDrift, ID: "0003", Location: model.Location{Path: "0003.md"}},
	}

	got := Touching(findings, g, []string{"0001.md"})

	testAssertTouching(t, got, []string{"0001.md", "0002.md"})
}

func TestTouchingAcceptsADirectory(t *testing.T) {
	dir := t.TempDir()
	findings := []model.Finding{
		{Rule: model.RuleStatusDrift, ID: "0001", Location: model.Location{Path: filepath.Join(dir, "0001.md")}},
		{Rule: model.RuleStatusDrift, ID: "0002", Location: model.Location{Path: "elsewhere/0002.md"}},
	}

	got := Touching(findings, testGraph(nil, nil, nil), []string{dir})

	testAssertTouching(t, got, []string{filepath.Join(dir, "0001.md")})
}

func TestTouchingKeepsNothingWhenNoPathMatches(t *testing.T) {
	got := Touching(testTouchingFindings(), testTouchingGraph(), []string{"0009.md"})

	testAssertTouching(t, got, nil)
}
