package graph

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/parse"
	"github.com/Kaikei-e/DocDag/model"
)

func diffTestBuild(t *testing.T, cfg config.Config, files map[string]string) *model.Graph {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	cfg.Dir = filepath.Join(root, "docs", "adr")
	docs, err := parse.Documents(cfg)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	parse.Localize(docs, root)
	return Build(docs, cfg)
}

func TestDiffIdenticalGraphs(t *testing.T) {
	cfg := config.ADRPreset()
	files := map[string]string{
		"docs/adr/0001-first.md":  "---\ntitle: First\nstatus: accepted\ndate: 2025-01-01\n---\n\n# First\n",
		"docs/adr/0002-second.md": "---\ntitle: Second\nstatus: proposed\ndate: 2025-01-02\n---\n\n# Second\n",
	}
	base := diffTestBuild(t, cfg, files)
	target := diffTestBuild(t, cfg, files)

	d := Diff(base, target, cfg, time.Time{})
	if len(d.Edges.Added) != 0 || len(d.Edges.Removed) != 0 {
		t.Errorf("edges = %+v, want empty", d.Edges)
	}
	if len(d.Status) != 0 {
		t.Errorf("status = %+v, want empty", d.Status)
	}
	if len(d.Documents.Added) != 0 || len(d.Documents.Removed) != 0 {
		t.Errorf("documents = %+v, want empty", d.Documents)
	}
	wantBinding := BindingDelta{Before: 1, After: 1, Delta: 0}
	if d.Binding != wantBinding {
		t.Errorf("binding = %+v, want %+v", d.Binding, wantBinding)
	}
}

func TestDiffTracksEdgesStatusesDocumentsAndBinding(t *testing.T) {
	cfg := config.ADRPreset()
	base := diffTestBuild(t, cfg, map[string]string{
		"docs/adr/0001-base.md":    "---\ntitle: Base\nstatus: accepted\ndate: 2025-01-01\n---\n\n# Base\n",
		"docs/adr/0002-old.md":     "---\ntitle: Old\nstatus: accepted\ndepends-on:\n  - \"0001\"\ndate: 2025-01-02\n---\n\n# Old\n",
		"docs/adr/0003-removed.md": "---\ntitle: Removed\nstatus: accepted\ndate: 2025-01-03\n---\n\n# Removed\n",
	})
	target := diffTestBuild(t, cfg, map[string]string{
		"docs/adr/0001-base.md": "---\ntitle: Base\nstatus: superseded\ndate: 2025-01-01\n---\n\n# Base\n",
		"docs/adr/0002-old.md":  "---\ntitle: Old\nstatus: accepted\ndate: 2025-01-02\n---\n\n# Old\n",
		"docs/adr/0004-new.md":  "---\ntitle: New\nstatus: accepted\nsupersedes:\n  - \"0001\"\ndepends-on:\n  - \"0002\"\ndate: 2025-01-04\n---\n\n# New\nSee [[0001]].\n",
	})
	d := Diff(base, target, cfg, time.Time{})

	wantAddedEdges := []EdgeChange{
		{Type: "depends-on", From: "0004", To: "0002", Location: model.Location{Path: "docs/adr/0004-new.md", Line: 6}},
		{Type: "supersedes", From: "0004", To: "0001", Location: model.Location{Path: "docs/adr/0004-new.md", Line: 4}},
	}
	if !reflect.DeepEqual(d.Edges.Added, wantAddedEdges) {
		t.Errorf("added edges = %+v, want %+v", d.Edges.Added, wantAddedEdges)
	}

	wantRemovedEdges := []EdgeChange{
		{Type: "depends-on", From: "0002", To: "0001", Location: model.Location{Path: "docs/adr/0002-old.md"}},
	}
	if !reflect.DeepEqual(d.Edges.Removed, wantRemovedEdges) {
		t.Errorf("removed edges = %+v, want %+v", d.Edges.Removed, wantRemovedEdges)
	}

	wantStatus := []StatusChange{
		{ID: "0001", Path: "docs/adr/0001-base.md", From: "accepted", To: "superseded", Location: model.Location{Path: "docs/adr/0001-base.md", Line: 3}},
	}
	if !reflect.DeepEqual(d.Status, wantStatus) {
		t.Errorf("status = %+v, want %+v", d.Status, wantStatus)
	}

	wantAddedDocs := []DocumentChange{
		{ID: "0004", Path: "docs/adr/0004-new.md", Status: "accepted", Location: model.Location{Path: "docs/adr/0004-new.md", Line: 3}},
	}
	if !reflect.DeepEqual(d.Documents.Added, wantAddedDocs) {
		t.Errorf("added docs = %+v, want %+v", d.Documents.Added, wantAddedDocs)
	}

	wantRemovedDocs := []DocumentChange{
		{ID: "0003", Path: "docs/adr/0003-removed.md", Status: "accepted", Location: model.Location{Path: "docs/adr/0003-removed.md"}},
	}
	if !reflect.DeepEqual(d.Documents.Removed, wantRemovedDocs) {
		t.Errorf("removed docs = %+v, want %+v", d.Documents.Removed, wantRemovedDocs)
	}

	wantBinding := BindingDelta{Before: 3, After: 2, Delta: -1}
	if d.Binding != wantBinding {
		t.Errorf("binding = %+v, want %+v", d.Binding, wantBinding)
	}
}

func TestDiffShowsRetractedEdgeAsRemovedAndIgnoresRefEdges(t *testing.T) {
	cfg := config.ADRPreset()
	base := diffTestBuild(t, cfg, map[string]string{
		"docs/adr/0001-orig.md": "---\ntitle: Orig\nstatus: accepted\ndate: 2025-01-01\n---\n\n# Orig\n",
		"docs/adr/0002-sup.md":  "---\ntitle: Sup\nstatus: accepted\nsupersedes:\n  - \"0001\"\ndate: 2025-01-02\n---\n\n# Sup\n",
	})
	target := diffTestBuild(t, cfg, map[string]string{
		"docs/adr/0001-orig.md":    "---\ntitle: Orig\nstatus: accepted\ndate: 2025-01-01\n---\n\n# Orig\nSee [[0002]].\n",
		"docs/adr/0002-sup.md":     "---\ntitle: Sup\nstatus: accepted\nsupersedes:\n  - \"0001\"\ndate: 2025-01-02\n---\n\n# Sup\n",
		"docs/adr/0003-retract.md": "---\ntitle: Retract\nstatus: accepted\ndate: 2025-01-03\nretracts:\n  - from: \"0002\"\n    edge: supersedes\n    to: \"0001\"\n---\n\n# Retract\n",
	})
	d := Diff(base, target, cfg, time.Time{})

	if len(d.Edges.Added) != 0 {
		t.Errorf("added edges = %+v, want empty (reference edges must not be compared)", d.Edges.Added)
	}
	wantRemoved := []EdgeChange{
		{Type: "supersedes", From: "0002", To: "0001", Location: model.Location{Path: "docs/adr/0002-sup.md"}},
	}
	if !reflect.DeepEqual(d.Edges.Removed, wantRemoved) {
		t.Errorf("removed edges = %+v, want %+v", d.Edges.Removed, wantRemoved)
	}
	// Before: 0002 (1); After: 0001, 0002, 0003 (3).
	wantBinding := BindingDelta{Before: 1, After: 3, Delta: 2}
	if d.Binding != wantBinding {
		t.Errorf("binding = %+v, want %+v", d.Binding, wantBinding)
	}
}

func TestDiffRespectsAsOfForBindingDelta(t *testing.T) {
	cfg := config.ADRPreset()
	cfg.Period = &config.PeriodSpec{From: config.KeyDate}
	cfg.Projections = []config.ProjectionSpec{
		{
			Name: config.ProjectionAcceptedUnsuperseded,
			When: config.Condition{
				NotInbound: config.EdgeSupersedes.String(),
				Attr: map[string]config.AttrCondition{
					config.DefaultStatusField: {Eq: ptr(config.StatusAccepted)},
					config.AttrInForce:        {Eq: ptr(ProjectionTrue)},
				},
			},
		},
	}

	base := diffTestBuild(t, cfg, map[string]string{
		"docs/adr/0001-first.md": "---\ntitle: First\nstatus: accepted\ndate: 2026-01-01\n---\n\n# First\n",
	})
	target := diffTestBuild(t, cfg, map[string]string{
		"docs/adr/0001-first.md":  "---\ntitle: First\nstatus: accepted\ndate: 2026-01-01\n---\n\n# First\n",
		"docs/adr/0002-future.md": "---\ntitle: Future\nstatus: accepted\ndate: 2027-01-01\n---\n\n# Future\n",
	})

	beforeBegin := Diff(base, target, cfg, time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if beforeBegin.Binding != (BindingDelta{Before: 1, After: 1, Delta: 0}) {
		t.Errorf("binding before begin = %+v, want 1 -> 1 (0)", beforeBegin.Binding)
	}

	afterBegin := Diff(base, target, cfg, time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if afterBegin.Binding != (BindingDelta{Before: 1, After: 2, Delta: 1}) {
		t.Errorf("binding after begin = %+v, want 1 -> 2 (+1)", afterBegin.Binding)
	}
}

func ptr(s string) *string { return &s }
