package graph

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

func writeTestDoc(t *testing.T, dir, filename, content string) string {
	t.Helper()
	p := filepath.Join(dir, filename)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile %s: %v", filename, err)
	}
	return p
}

func TestCheckSections(t *testing.T) {
	t.Run("opt-in: no sections spec reports nothing", func(t *testing.T) {
		dir := t.TempDir()
		p := writeTestDoc(t, dir, "0001.md", "---\ntitle: T\nstatus: accepted\n---\n\nBody without sections.\n")
		g := model.NewGraph()
		g.Nodes["0001"] = &model.Node{ID: "0001", Path: p, Status: "accepted"}
		cfg := config.ADRPreset()

		got := CheckSections(g, cfg)
		if len(got) != 0 {
			t.Fatalf("got %d findings, want none: %+v", len(got), got)
		}
	})

	t.Run("missing required section reports at first body line", func(t *testing.T) {
		dir := t.TempDir()
		// Frontmatter is 4 lines: 1: ---, 2: title, 3: status, 4: ---
		// Line 5 is empty, line 6 is # Title, line 7 is empty, line 8 is ## Context
		content := "---\ntitle: T\nstatus: accepted\n---\n\n# Title\n\n## Context\n\nSome context.\n"
		p := writeTestDoc(t, dir, "0001.md", content)
		g := model.NewGraph()
		g.Nodes["0001"] = &model.Node{ID: "0001", Path: p, Status: "accepted"}

		cfg := config.ADRPreset()
		cfg.Sections = &config.SectionsSpec{
			Required: []string{"Context", "Decision", "Consequences"},
		}

		got := CheckSections(g, cfg)
		if len(got) != 2 {
			t.Fatalf("got %d findings, want 2 (Decision and Consequences missing): %+v", len(got), got)
		}
		for _, f := range got {
			if f.Rule != model.RuleMissingSection {
				t.Errorf("rule = %q, want %q", f.Rule, model.RuleMissingSection)
			}
			if f.Severity != model.SeverityError {
				t.Errorf("severity = %q, want %q", f.Severity, model.SeverityError)
			}
			if f.Location.Line != 5 {
				t.Errorf("location line = %d, want 5 (first body line)", f.Location.Line)
			}
		}
	})

	t.Run("alternatives with pipe match either heading", func(t *testing.T) {
		dir := t.TempDir()
		content := "---\ntitle: T\nstatus: accepted\n---\n\n## Context and Problem Statement\n\nContext...\n\n## Decision Outcome:\n\nDecision...\n"
		p := writeTestDoc(t, dir, "0001.md", content)
		g := model.NewGraph()
		g.Nodes["0001"] = &model.Node{ID: "0001", Path: p, Status: "accepted"}

		cfg := config.ADRPreset()
		cfg.Sections = &config.SectionsSpec{
			Required: []string{
				"Context|Context and Problem Statement",
				"Decision|Decision Outcome",
			},
		}

		got := CheckSections(g, cfg)
		if len(got) != 0 {
			t.Fatalf("got %d findings, want none: %+v", len(got), got)
		}
	})

	t.Run("ordered false allows out of order headings", func(t *testing.T) {
		dir := t.TempDir()
		content := "---\ntitle: T\nstatus: accepted\n---\n\n## Decision\n\nDecision text.\n\n## Context\n\nContext text.\n"
		p := writeTestDoc(t, dir, "0001.md", content)
		g := model.NewGraph()
		g.Nodes["0001"] = &model.Node{ID: "0001", Path: p, Status: "accepted"}

		cfg := config.ADRPreset()
		cfg.Sections = &config.SectionsSpec{
			Required: []string{"Context", "Decision"},
			Ordered:  false,
		}

		got := CheckSections(g, cfg)
		if len(got) != 0 {
			t.Fatalf("got %d findings, want none: %+v", len(got), got)
		}
	})

	t.Run("ordered true reports heading out of order at heading line", func(t *testing.T) {
		dir := t.TempDir()
		// Line 1: ---
		// Line 2: title: T
		// Line 3: status: accepted
		// Line 4: ---
		// Line 5: (empty)
		// Line 6: ## Decision
		// Line 7: (empty)
		// Line 8: ## Context
		content := "---\ntitle: T\nstatus: accepted\n---\n\n## Decision\n\n## Context\n"
		p := writeTestDoc(t, dir, "0001.md", content)
		g := model.NewGraph()
		g.Nodes["0001"] = &model.Node{ID: "0001", Path: p, Status: "accepted"}

		cfg := config.ADRPreset()
		cfg.Sections = &config.SectionsSpec{
			Required: []string{"Context", "Decision"},
			Ordered:  true,
		}

		got := CheckSections(g, cfg)
		if len(got) != 1 {
			t.Fatalf("got %d findings, want 1: %+v", len(got), got)
		}
		f := got[0]
		if f.Rule != model.RuleSectionOrder {
			t.Errorf("rule = %q, want %q", f.Rule, model.RuleSectionOrder)
		}
		if f.Severity != model.SeverityError {
			t.Errorf("severity = %q, want %q", f.Severity, model.SeverityError)
		}
		if f.Location.Line != 8 {
			t.Errorf("location line = %d, want 8 (line of Context)", f.Location.Line)
		}
		if !strings.Contains(f.Detail, "Context") {
			t.Errorf("detail = %q, want it to contain Context", f.Detail)
		}
	})

	t.Run("when status filter skips non-matching statuses", func(t *testing.T) {
		dir := t.TempDir()
		// Both documents lack Context
		p1 := writeTestDoc(t, dir, "0001.md", "---\ntitle: 1\nstatus: proposed\n---\n\n## Decision\n")
		p2 := writeTestDoc(t, dir, "0002.md", "---\ntitle: 2\nstatus: accepted\n---\n\n## Decision\n")

		g := model.NewGraph()
		g.Nodes["0001"] = &model.Node{ID: "0001", Path: p1, Status: "proposed"}
		g.Nodes["0002"] = &model.Node{ID: "0002", Path: p2, Status: "accepted"}

		cfg := config.ADRPreset()
		cfg.Sections = &config.SectionsSpec{
			Required: []string{"Context", "Decision"},
			When:     &config.SectionsWhen{Status: []string{"accepted"}},
		}

		got := CheckSections(g, cfg)
		if len(got) != 1 {
			t.Fatalf("got %d findings, want 1 (0002 only): %+v", len(got), got)
		}
		if got[0].ID != "0002" {
			t.Errorf("finding ID = %q, want 0002", got[0].ID)
		}
	})

	t.Run("headings inside code fences are ignored", func(t *testing.T) {
		dir := t.TempDir()
		content := "---\ntitle: T\nstatus: accepted\n---\n\n```markdown\n## Context\n```\n\n## Decision\n"
		p := writeTestDoc(t, dir, "0001.md", content)
		g := model.NewGraph()
		g.Nodes["0001"] = &model.Node{ID: "0001", Path: p, Status: "accepted"}

		cfg := config.ADRPreset()
		cfg.Sections = &config.SectionsSpec{
			Required: []string{"Context", "Decision"},
		}

		got := CheckSections(g, cfg)
		if len(got) != 1 {
			t.Fatalf("got %d findings, want 1 (Context missing): %+v", len(got), got)
		}
		if got[0].Rule != model.RuleMissingSection || !strings.Contains(got[0].Detail, "Context") {
			t.Errorf("unexpected finding: %+v", got[0])
		}
	})

	t.Run("level filter requires specific heading level", func(t *testing.T) {
		dir := t.TempDir()
		content := "---\ntitle: T\nstatus: accepted\n---\n\n# Context\n\n### Decision\n"
		p := writeTestDoc(t, dir, "0001.md", content)
		g := model.NewGraph()
		g.Nodes["0001"] = &model.Node{ID: "0001", Path: p, Status: "accepted"}

		cfg := config.ADRPreset()
		cfg.Sections = &config.SectionsSpec{
			Required: []string{"Context", "Decision"},
			Level:    2,
		}

		got := CheckSections(g, cfg)
		if len(got) != 2 {
			t.Fatalf("got %d findings, want 2 (both H2 missing): %+v", len(got), got)
		}
	})
}
