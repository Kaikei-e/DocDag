package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/render"
)

func linkKeys(doc render.NodeLink) []string {
	keys := make([]string, 0, len(doc.Links))
	for _, l := range doc.Links {
		keys = append(keys, fmt.Sprintf("%s|%s|%s|%s", l.Source, l.Type, l.Target, l.Origin))
	}
	slices.Sort(keys)
	return keys
}

func nodeIDs(doc render.NodeLink) []model.ID {
	ids := make([]model.ID, 0, len(doc.Nodes))
	for _, n := range doc.Nodes {
		ids = append(ids, n.ID)
	}
	return ids
}

func TestExportNodeLinkJSON(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		args    []string
		nodes   []model.ID
		links   []string
	}{
		{
			name:    "typed layer only",
			fixture: "ok-basic",
			nodes:   []model.ID{"0001", "0002", "0003", "0004", "0005", "0006"},
			links: []string{
				"0002|supersedes|0001|structured",
				"0004|depends-on|0003|structured",
				"0004|supersedes|0002|structured",
				"0005|depends-on|0003|structured",
			},
		},
		{
			name:    "reference layer overlaid",
			fixture: "ok-basic",
			args:    []string{"--include-refs"},
			nodes:   []model.ID{"0001", "0002", "0003", "0004", "0005", "0006"},
			links: []string{
				"0001||0002|reference",
				"0002|supersedes|0001|structured",
				"0002||0001|reference",
				"0004|depends-on|0003|structured",
				"0004|supersedes|0002|structured",
				"0004||0003|reference",
				"0005|depends-on|0003|structured",
				"0005||0003|reference",
			},
		},
		{
			name:    "derived edges are exported as supersedes",
			fixture: "ok-madr",
			nodes:   []model.ID{"0001", "0002", "0003", "0004"},
			links: []string{
				"0003|depends-on|0001|structured",
				"0003|supersedes|0002|derived",
				"0004|depends-on|0003|structured",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"export", "--format", "json"}, tt.args...)
			args = append(args, "--dir", fixture(t, tt.fixture))
			got := run(t, args...)
			assertExit(t, got, 0)
			doc := decodeJSON[render.NodeLink](t, got.stdout)
			if !slices.Equal(nodeIDs(doc), tt.nodes) {
				t.Errorf("nodes = %v, want %v in ascending order", nodeIDs(doc), tt.nodes)
			}
			if !slices.Equal(linkKeys(doc), tt.links) {
				t.Errorf("links = %v, want %v", linkKeys(doc), tt.links)
			}
		})
	}
}

func TestExportConnectedAndEdgeFilters(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		nodes []model.ID
		links []string
	}{
		{
			name:  "only documents a typed edge touches",
			args:  []string{"--connected"},
			nodes: []model.ID{"0001", "0002", "0003", "0004", "0005"},
			links: []string{
				"0002|supersedes|0001|structured",
				"0004|depends-on|0003|structured",
				"0004|supersedes|0002|structured",
				"0005|depends-on|0003|structured",
			},
		},
		{
			name:  "one edge type keeps every document",
			args:  []string{"--edge", "depends-on"},
			nodes: []model.ID{"0001", "0002", "0003", "0004", "0005", "0006"},
			links: []string{
				"0004|depends-on|0003|structured",
				"0005|depends-on|0003|structured",
			},
		},
		{
			name:  "the edge filter narrows the connected set",
			args:  []string{"--connected", "--edge", "supersedes"},
			nodes: []model.ID{"0001", "0002", "0004"},
			links: []string{
				"0002|supersedes|0001|structured",
				"0004|supersedes|0002|structured",
			},
		},
		{
			name:  "a repeated edge filter unions the types",
			args:  []string{"--connected", "--edge", "supersedes", "--edge", "depends-on"},
			nodes: []model.ID{"0001", "0002", "0003", "0004", "0005"},
			links: []string{
				"0002|supersedes|0001|structured",
				"0004|depends-on|0003|structured",
				"0004|supersedes|0002|structured",
				"0005|depends-on|0003|structured",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args := append([]string{"export", "--format", "json"}, tt.args...)
			args = append(args, "--dir", fixture(t, "ok-basic"))
			got := run(t, args...)
			assertExit(t, got, 0)
			doc := decodeJSON[render.NodeLink](t, got.stdout)
			if !slices.Equal(nodeIDs(doc), tt.nodes) {
				t.Errorf("nodes = %v, want %v", nodeIDs(doc), tt.nodes)
			}
			if !slices.Equal(linkKeys(doc), tt.links) {
				t.Errorf("links = %v, want %v", linkKeys(doc), tt.links)
			}
		})
	}
}

func TestExportRejectsAnUnknownEdgeType(t *testing.T) {
	got := run(t, "export", "--edge", "relates-to", "--dir", fixture(t, "ok-basic"))

	assertExit(t, got, 2)
	if !strings.Contains(got.stderr, "relates-to") {
		t.Errorf("stderr = %q, want it to name the unknown edge type", got.stderr)
	}
}

func TestExportConnectedInEveryFormat(t *testing.T) {
	dir := fixture(t, "ok-basic")
	for _, format := range []string{"mermaid", "dot", "json"} {
		t.Run(format, func(t *testing.T) {
			got := run(t, "export", "--format", format, "--connected", "--dir", dir)
			assertExit(t, got, 0)
			if strings.Contains(got.stdout, "0006") {
				t.Errorf("%s output keeps a document no typed edge touches:\n%s", format, got.stdout)
			}
		})
	}
}

func TestExportMermaidIsTheDefault(t *testing.T) {
	got := run(t, "export", "--dir", fixture(t, "ok-basic"))
	assertExit(t, got, 0)
	ls := lines(got.stdout)
	if len(ls) == 0 || strings.TrimSpace(ls[0]) != "graph LR" {
		t.Fatalf("first line = %q, want %q", got.stdout, "graph LR")
	}
	for _, want := range []string{"supersedes", "depends-on"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("mermaid output does not label %q: %s", want, got.stdout)
		}
	}
}

func TestExportDOT(t *testing.T) {
	got := run(t, "export", "--format", "dot", "--dir", fixture(t, "ok-basic"))
	assertExit(t, got, 0)
	if !strings.Contains(got.stdout, "digraph") {
		t.Errorf("DOT output does not open a digraph: %s", got.stdout)
	}
}

func TestExportIsDeterministic(t *testing.T) {
	dir := fixture(t, "ok-basic")
	first := run(t, "export", "--dir", dir)
	assertExit(t, first, 0)
	second := run(t, "export", "--dir", dir)
	if first.stdout != second.stdout {
		t.Errorf("export output is not deterministic:\nfirst:\n%s\nsecond:\n%s", first.stdout, second.stdout)
	}
}

func TestExportOutFlag(t *testing.T) {
	dir := fixture(t, "ok-basic")
	stdout := run(t, "export", "--format", "json", "--out", "-", "--dir", dir)
	assertExit(t, stdout, 0)
	if stdout.stdout == "" {
		t.Fatal("--out - wrote nothing to stdout")
	}

	path := filepath.Join(t.TempDir(), "graph.json")
	toFile := run(t, "export", "--format", "json", "--out", path, "--dir", dir)
	assertExit(t, toFile, 0)
	if toFile.stdout != "" {
		t.Errorf("stdout = %q, want empty when --out names a file", toFile.stdout)
	}
	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(written) != stdout.stdout {
		t.Errorf("file content differs from the stdout rendering:\nfile:\n%s\nstdout:\n%s", written, stdout.stdout)
	}
}

func TestExportUnwritableTargetExitsThree(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent", "graph.json")
	got := run(t, "export", "--out", path, "--dir", fixture(t, "ok-basic"))
	assertExit(t, got, 3)
}
