package graph

import (
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/parse"
	"github.com/Kaikei-e/DocDag/model"
)

func testRequireMentionConfig(severity string) config.Config {
	cfg := config.ADRPreset()
	cfg.Edges[0].RequireMention = severity
	return cfg
}

func TestCheckUnmentionedEdges(t *testing.T) {
	t.Run("fires for an unmentioned target at the edge key line", func(t *testing.T) {
		cfg := testRequireMentionConfig(string(model.SeverityWarn))
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "superseded"}, "Original decision."),
			testDoc("0002", map[string]any{"status": "accepted", "supersedes": []any{"0001"}}, "New decision without citation."),
		}

		g := Build(docs, cfg)
		got := CheckUnmentionedEdges(g, cfg)

		f := testAssertSingleFinding(t, got, model.RuleUnmentionedEdge, model.SeverityWarn, "0002")
		wantDetail := `supersedes target "0001" is never referenced in the body`
		if f.Detail != wantDetail {
			t.Errorf("detail = %q, want %q", f.Detail, wantDetail)
		}
		if f.Location.Path != "0002.md" || f.Location.Line != docs[1].KeyLines["supersedes"] {
			t.Errorf("location = %+v, want 0002.md:%d", f.Location, docs[1].KeyLines["supersedes"])
		}
		if len(f.Related) != 1 || f.Related[0].Path != "0001.md" {
			t.Errorf("related = %+v, want target 0001.md", f.Related)
		}
	})

	t.Run("silent when the body has a wikilink to the target", func(t *testing.T) {
		cfg := testRequireMentionConfig(string(model.SeverityError))
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "superseded"}, "Original."),
			testDoc("0002", map[string]any{"status": "accepted", "supersedes": []any{"0001"}}, "Replaces [[0001|the old record]].\n"),
		}

		g := Build(docs, cfg)
		if got := CheckUnmentionedEdges(g, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("silent when the body has a markdown link to the target", func(t *testing.T) {
		cfg := testRequireMentionConfig(string(model.SeverityError))
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "superseded"}, "Original."),
			testDoc("0002", map[string]any{"status": "accepted", "supersedes": []any{"0001"}}, "Replaces [ADR-0001](0001-original.md).\n"),
		}

		g := Build(docs, cfg)
		if got := CheckUnmentionedEdges(g, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("frontmatter-only link never counts even when references.scan includes frontmatter", func(t *testing.T) {
		cfg := testRequireMentionConfig(string(model.SeverityError))
		cfg.References.Scan = []string{config.ScanBody, config.ScanFrontmatter}
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "superseded"}, "Original."),
			testDoc("0002", map[string]any{
				"status":     "accepted",
				"supersedes": []any{"0001"},
				"relates":    "[[0001]]",
			}, "No citation in body.\n"),
		}

		g := Build(docs, cfg)
		testAssertSingleFinding(t, CheckUnmentionedEdges(g, cfg), model.RuleUnmentionedEdge, model.SeverityError, "0002")
	})

	t.Run("works when references.dangling is off and references.scan omits body", func(t *testing.T) {
		cfg := testRequireMentionConfig(string(model.SeverityWarn))
		cfg.References.Dangling = config.ReferencesOff
		cfg.References.Scan = []string{config.ScanFrontmatter}
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "superseded"}, "Original."),
			testDoc("0002", map[string]any{"status": "accepted", "supersedes": []any{"0001"}}, "Cites [[0001]].\n"),
			testDoc("0003", map[string]any{"status": "accepted", "supersedes": []any{"0001"}}, "Cites nothing.\n"),
		}

		g := Build(docs, cfg)
		testAssertSingleFinding(t, CheckUnmentionedEdges(g, cfg), model.RuleUnmentionedEdge, model.SeverityWarn, "0003")
	})

	t.Run("respects custom references.pattern", func(t *testing.T) {
		cfg := testRequireMentionConfig(string(model.SeverityError))
		cfg.References.Pattern = `^(?i)RFC-(\d+)$`
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "superseded"}, "Original."),
			testDoc("0002", map[string]any{"status": "accepted", "supersedes": []any{"0001"}}, "Mentions [[0001]] which does not match pattern.\n"),
			testDoc("0003", map[string]any{"status": "accepted", "supersedes": []any{"0001"}}, "Mentions [[RFC-0001]] which matches.\n"),
		}

		g := Build(docs, cfg)
		testAssertSingleFinding(t, CheckUnmentionedEdges(g, cfg), model.RuleUnmentionedEdge, model.SeverityError, "0002")
	})

	t.Run("silent when require_mention is omitted", func(t *testing.T) {
		cfg := config.ADRPreset()
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "superseded"}, "Original."),
			testDoc("0002", map[string]any{"status": "accepted", "supersedes": []any{"0001"}}, "No citation.\n"),
		}

		g := Build(docs, cfg)
		if got := CheckUnmentionedEdges(g, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("uses configured warn vs error severity", func(t *testing.T) {
		for _, sev := range []model.Severity{model.SeverityWarn, model.SeverityError} {
			cfg := testRequireMentionConfig(string(sev))
			docs := []*parse.Document{
				testDoc("0001", map[string]any{"status": "superseded"}, "Original."),
				testDoc("0002", map[string]any{"status": "accepted", "supersedes": []any{"0001"}}, "No citation.\n"),
			}

			g := Build(docs, cfg)
			testAssertSingleFinding(t, CheckUnmentionedEdges(g, cfg), model.RuleUnmentionedEdge, sev, "0002")
		}
	})

	t.Run("skips dangling targets, derived edges, and inverse mirror entries", func(t *testing.T) {
		cfg := testRequireMentionConfig(string(model.SeverityError))
		cfg.Edges[0].Inverse = "superseded_by"
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "superseded by 0002"}, "Original."),
			testDoc("0002", map[string]any{"status": "accepted", "supersedes": []any{"0099"}}, "No citation.\n"),
			testDoc("0003", map[string]any{"status": "superseded", "superseded_by": []any{"0004"}}, "No citation.\n"),
			testDoc("0004", map[string]any{"status": "accepted", "supersedes": []any{"0003"}}, "Cites [[0003]].\n"),
		}

		g := Build(docs, cfg)
		if got := CheckUnmentionedEdges(g, cfg); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("passes through touching filter", func(t *testing.T) {
		cfg := testRequireMentionConfig(string(model.SeverityWarn))
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "superseded"}, "Original."),
			testDoc("0002", map[string]any{"status": "accepted", "supersedes": []any{"0001"}}, "No citation.\n"),
			testDoc("0003", map[string]any{"status": "accepted"}, "Unrelated."),
		}

		g := Build(docs, cfg)
		findings := Validate(g, cfg, testAsOf)
		if len(testFindingsFor(findings, model.RuleUnmentionedEdge)) != 1 {
			t.Fatalf("findings = %+v, want one unmentioned_edge", findings)
		}

		kept := Touching(findings, g, []string{"0002.md"})
		if len(testFindingsFor(kept, model.RuleUnmentionedEdge)) != 1 {
			t.Errorf("kept = %+v, want unmentioned_edge kept when 0002.md is touched", kept)
		}

		filtered := Touching(findings, g, []string{"0003.md"})
		if len(testFindingsFor(filtered, model.RuleUnmentionedEdge)) != 0 {
			t.Errorf("filtered = %+v, want unmentioned_edge filtered out when only 0003.md is touched", filtered)
		}
	})

	t.Run("reverse direction edge checks the declaring document body", func(t *testing.T) {
		cfg := config.ADRPreset()
		cfg.Edges[0].Direction = config.DirectionReverse
		cfg.Edges[0].RequireMention = string(model.SeverityWarn)
		docs := []*parse.Document{
			testDoc("0001", map[string]any{"status": "accepted"}, "Original."),
			testDoc("0002", map[string]any{"status": "superseded", "supersedes": []any{"0001"}}, "No mention of target.\n"),
		}

		g := Build(docs, cfg)
		f := testAssertSingleFinding(t, CheckUnmentionedEdges(g, cfg), model.RuleUnmentionedEdge, model.SeverityWarn, "0002")
		if !strings.Contains(f.Detail, `"0001"`) {
			t.Errorf("detail = %q, want target 0001 named", f.Detail)
		}
	})
}
