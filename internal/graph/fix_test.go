package graph

import (
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

func testSuggestionGraph() *model.Graph {
	return testGraph(
		[]*model.Node{
			testNode("0001", config.StatusAccepted),
			testNode("0002", config.StatusSuperseded),
			testNode("0003", config.StatusAccepted),
			testNode("0042", config.StatusAccepted),
		},
		[]model.Edge{testDerivedEdge("0003", "0002", config.EdgeSupersedes)},
		nil,
	)
}

func testSuggest(t *testing.T, f model.Finding) string {
	t.Helper()
	suggested := Suggest([]model.Finding{f}, testSuggestionGraph(), config.ADRPreset(), testAsOf)
	if len(suggested) != 1 {
		t.Fatalf("suggested = %+v, want one finding back", suggested)
	}
	return suggested[0].Fix
}

func TestSuggestNamesWhatToType(t *testing.T) {
	tests := []struct {
		name    string
		finding model.Finding
		want    string
	}{
		{
			name: "a dangling reference names the nearest documents",
			finding: model.Finding{
				Rule:     model.RuleDanglingRef,
				ID:       "0001",
				Detail:   `supersedes reference "0002x" does not name a document`,
				Location: model.Location{Path: "0001.md", Line: 4},
			},
			want: "did you mean 0002, 0003 or 0042?",
		},
		{
			name: "a reference-layer dangling link is suggested for too",
			finding: model.Finding{
				Rule:   "dangling_reference",
				ID:     "0001",
				Detail: `reference "0041" does not name a document`,
			},
			want: "did you mean 0042, 0003 or 0002?",
		},
		{
			name: "a drifted status names the value and the file",
			finding: model.Finding{
				Rule:     model.RuleStatusDrift,
				ID:       "0001",
				Location: model.Location{Path: "docs/adr/0001.md", Line: 3},
			},
			want: "set status: superseded in docs/adr/0001.md",
		},
		{
			name:    "an orphaned superseded status offers both ways out",
			finding: model.Finding{Rule: model.RuleSupersededOrphan, ID: "0002"},
			want:    "declare supersedes: 0002 in the replacing document, or set status: withdrawn",
		},
		{
			name:    "an unstructured supersedes names the document that should declare it",
			finding: model.Finding{Rule: model.RuleUnstructuredSupersedes, ID: "0002"},
			want:    "declare supersedes: 0002 in 0003",
		},
		{
			name:    "an unknown status lists the vocabulary",
			finding: model.Finding{Rule: model.RuleUnknownStatus, ID: "0001"},
			want:    "use one of: proposed, accepted, rejected, deprecated, superseded, withdrawn",
		},
		{
			name:    "an absent frontmatter block says what to write",
			finding: model.Finding{Rule: model.RuleMissingFrontmatter, ID: "0001"},
			want:    "add a YAML frontmatter block with title and status",
		},
		{
			name:    "a cycle says which edges are candidates",
			finding: model.Finding{Rule: model.RuleCycle, ID: "0001"},
			want:    "remove one of the listed edges",
		},
		{
			name:    "a collision has no mechanical fix",
			finding: model.Finding{Rule: model.RuleIDCollision, ID: "0004"},
			want:    "",
		},
		{
			name:    "undecodable frontmatter has no mechanical fix",
			finding: model.Finding{Rule: model.RuleInvalidFrontmatter, ID: "0002"},
			want:    "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := testSuggest(t, tt.finding); got != tt.want {
				t.Errorf("fix = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSuggestNamesAtMostThreeCandidates(t *testing.T) {
	got := testSuggest(t, model.Finding{
		Rule:   model.RuleDanglingRef,
		ID:     "0001",
		Detail: `supersedes reference "0009" does not name a document`,
	})

	if strings.Count(got, ",") > 1 {
		t.Errorf("fix = %q, want at most three candidates", got)
	}
}

func TestSuggestLeavesAReferenceItCannotReadAlone(t *testing.T) {
	got := testSuggest(t, model.Finding{
		Rule:   model.RuleDanglingRef,
		ID:     "0001",
		Detail: `supersedes reference "upstream" does not name a document`,
	})

	if got != "" {
		t.Errorf("fix = %q, want none for a reference that carries no number", got)
	}
}

func TestSuggestFollowsTheConfiguredVocabulary(t *testing.T) {
	cfg := config.ADRPreset()
	cfg.StatusField = "state"
	findings := Suggest([]model.Finding{
		{Rule: model.RuleStatusDrift, ID: "0001", Location: model.Location{Path: "0001.md"}},
		{Rule: model.RuleMissingFrontmatter, ID: "0002"},
	}, testSuggestionGraph(), cfg, testAsOf)

	if findings[0].Fix != "set state: superseded in 0001.md" {
		t.Errorf("fix = %q, want the configured status field", findings[0].Fix)
	}
	if !strings.HasSuffix(findings[1].Fix, "title and state") {
		t.Errorf("fix = %q, want the configured status field", findings[1].Fix)
	}
}
