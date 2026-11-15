package render

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/model"
)

func testSampleDiff() graph.GraphDiff {
	return graph.GraphDiff{
		Edges: graph.EdgeDiff{
			Added: []graph.EdgeChange{{
				Type:     config.EdgeSupersedes,
				From:     "0940",
				To:       "0877",
				Location: model.Location{Path: "docs/ADR/0940-new.md", Line: 4},
			}},
			Removed: []graph.EdgeChange{{
				Type:     config.EdgeDependsOn,
				From:     "0012",
				To:       "0003",
				Location: model.Location{Path: "docs/ADR/0012-old.md"},
			}},
		},
		Status: []graph.StatusChange{{
			ID:       "0877",
			Path:     "docs/ADR/0877-mid.md",
			From:     "accepted",
			To:       "superseded",
			Location: model.Location{Path: "docs/ADR/0877-mid.md", Line: 3},
		}},
		Documents: graph.DocumentDiff{
			Added: []graph.DocumentChange{{
				ID:       "0990",
				Path:     "docs/ADR/0990-extra.md",
				Status:   "accepted",
				Location: model.Location{Path: "docs/ADR/0990-extra.md", Line: 2},
			}},
			Removed: []graph.DocumentChange{{
				ID:       "0005",
				Path:     "docs/ADR/0005-gone.md",
				Status:   "accepted",
				Location: model.Location{Path: "docs/ADR/0005-gone.md"},
			}},
		},
		Binding: graph.BindingDelta{Before: 944, After: 890, Delta: -54},
	}
}

func TestDiffGolden(t *testing.T) {
	d := testSampleDiff()
	header := Header{
		PresetVersion: config.ADRPreset().PresetVersion,
		AsOf:          testAsOfDay,
	}

	t.Run("text", func(t *testing.T) {
		var buf bytes.Buffer
		if err := DiffText(&buf, d); err != nil {
			t.Fatalf("DiffText: %v", err)
		}
		testAssertGolden(t, "diff.txt", buf.String())
	})

	t.Run("json", func(t *testing.T) {
		var buf bytes.Buffer
		if err := DiffJSON(&buf, d, header); err != nil {
			t.Fatalf("DiffJSON: %v", err)
		}
		testAssertGolden(t, "diff.json", buf.String())
	})

	t.Run("github", func(t *testing.T) {
		var buf bytes.Buffer
		if err := DiffGitHub(&buf, d); err != nil {
			t.Fatalf("DiffGitHub: %v", err)
		}
		testAssertGolden(t, "diff.github", buf.String())
	})
}

func TestDiffEmpty(t *testing.T) {
	d := graph.GraphDiff{Binding: graph.BindingDelta{Before: 12, After: 12, Delta: 0}}

	t.Run("text prints only the binding line", func(t *testing.T) {
		var buf bytes.Buffer
		if err := DiffText(&buf, d); err != nil {
			t.Fatalf("DiffText: %v", err)
		}
		if got, want := buf.String(), "binding: 12 -> 12 (0)\n"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("github prints only the binding line", func(t *testing.T) {
		var buf bytes.Buffer
		if err := DiffGitHub(&buf, d); err != nil {
			t.Fatalf("DiffGitHub: %v", err)
		}
		if got, want := buf.String(), "binding: 12 -> 12 (0)\n"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("json encodes empty arrays rather than nulls", func(t *testing.T) {
		var buf bytes.Buffer
		if err := DiffJSON(&buf, graph.GraphDiff{}, Header{}); err != nil {
			t.Fatalf("DiffJSON: %v", err)
		}
		var raw map[string]any
		if err := json.Unmarshal(buf.Bytes(), &raw); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		for _, wantArray := range []string{`"added": []`, `"removed": []`, `"status": []`} {
			if !strings.Contains(buf.String(), wantArray) {
				t.Errorf("JSON output missing %s:\n%s", wantArray, buf.String())
			}
		}
	})
}

func TestDiffFormatsPositiveBindingDeltaWithPlusSign(t *testing.T) {
	d := graph.GraphDiff{Binding: graph.BindingDelta{Before: 5, After: 8, Delta: 3}}
	var buf bytes.Buffer
	if err := DiffText(&buf, d); err != nil {
		t.Fatalf("DiffText: %v", err)
	}
	if got, want := buf.String(), "binding: 5 -> 8 (+3)\n"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDiffGitHubEscapesPathPropertiesAndDropsMissingFile(t *testing.T) {
	d := graph.GraphDiff{
		Edges: graph.EdgeDiff{
			Added: []graph.EdgeChange{{
				Type:     config.EdgeSupersedes,
				From:     "0002",
				To:       "0001",
				Location: model.Location{Path: "docs/a:b,c.md", Line: 5},
			}},
			Removed: []graph.EdgeChange{{
				Type: config.EdgeDependsOn,
				From: "0003",
				To:   "0001",
			}},
		},
		Binding: graph.BindingDelta{Before: 1, After: 1, Delta: 0},
	}
	var buf bytes.Buffer
	if err := DiffGitHub(&buf, d); err != nil {
		t.Fatalf("DiffGitHub: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3:\n%s", len(lines), buf.String())
	}
	if want := "::notice file=docs/a%3Ab%2Cc.md,line=5,title=docdag diff::+ supersedes 0002 -> 0001"; lines[0] != want {
		t.Errorf("line 0 = %q, want %q", lines[0], want)
	}
	if want := "::notice title=docdag diff::- depends-on 0003 -> 0001"; lines[1] != want {
		t.Errorf("line 1 = %q, want %q", lines[1], want)
	}
}

func TestDiffSurfacesWriteErrors(t *testing.T) {
	d := testSampleDiff()
	for name, fn := range map[string]func() error{
		"text":   func() error { return DiffText(failingWriter{}, d) },
		"github": func() error { return DiffGitHub(failingWriter{}, d) },
		"json":   func() error { return DiffJSON(failingWriter{}, d, Header{}) },
	} {
		t.Run(name, func(t *testing.T) {
			if err := fn(); !errors.Is(err, errWriterClosed) {
				t.Errorf("got err = %v, want %v", err, errWriterClosed)
			}
		})
	}
}
