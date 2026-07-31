package cmd

import (
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/model"
)

func TestResolveWalksToTheCurrentDocument(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		ref     string
		want    []string
	}{
		{name: "chain head resolves to the sink", fixture: "ok-basic", ref: "0001", want: []string{"0004"}},
		{name: "bare number is a reference", fixture: "ok-basic", ref: "1", want: []string{"0004"}},
		{name: "prefixed reference is normalized", fixture: "ok-basic", ref: "ADR-000001", want: []string{"0004"}},
		{name: "middle of the chain resolves forward", fixture: "ok-basic", ref: "0002", want: []string{"0004"}},
		{name: "sink resolves to itself", fixture: "ok-basic", ref: "0004", want: []string{"0004"}},
		{name: "document nobody supersedes resolves to itself", fixture: "ok-basic", ref: "0006", want: []string{"0006"}},
		{name: "first fan-in branch converges", fixture: "fan-in", ref: "0001", want: []string{"0003"}},
		{name: "second fan-in branch converges", fixture: "fan-in", ref: "0002", want: []string{"0003"}},
		{name: "derived supersedes is followed", fixture: "ok-madr", ref: "0002", want: []string{"0003"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, "resolve", tt.ref, "--dir", fixture(t, tt.fixture))
			assertExit(t, got, 0)
			assertLines(t, "resolve", lines(got.stdout), tt.want)
			if got.stderr != "" {
				t.Errorf("stderr = %q, want empty", got.stderr)
			}
		})
	}
}

func TestResolvePrintsEverySuccessorSorted(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"0001-original.md":        "---\ntitle: Original\nstatus: superseded\ndate: 2025-01-01\n---\n\n# Original\n",
		"0002-replacement-one.md": "---\ntitle: Replacement one\nstatus: accepted\nsupersedes:\n  - 0001\ndate: 2025-01-02\n---\n\n# Replacement one\n",
		"0003-replacement-two.md": "---\ntitle: Replacement two\nstatus: accepted\nsupersedes:\n  - 0001\ndate: 2025-01-03\n---\n\n# Replacement two\n",
	})

	got := run(t, "resolve", "0001", "--dir", dir)
	assertExit(t, got, 0)
	assertLines(t, "resolve", lines(got.stdout), []string{"0002", "0003"})
}

func TestResolveJSON(t *testing.T) {
	got := run(t, "resolve", "0001", "--format", "json", "--dir", fixture(t, "ok-basic"))
	assertExit(t, got, 0)
	ids := decodeJSON[[]model.ID](t, got.stdout)
	if len(ids) != 1 || ids[0] != model.ID("0004") {
		t.Errorf("ids = %v, want [0004]", ids)
	}
}

func TestResolveFailures(t *testing.T) {
	tests := []struct {
		name    string
		fixture string
		ref     string
		want    string
	}{
		{name: "unknown document", fixture: "ok-basic", ref: "0099", want: "unknown document"},
		{name: "unrecognized reference", fixture: "ok-basic", ref: "not-a-reference", want: "unrecognized reference"},
		{name: "cycle is reported", fixture: "cycle", ref: "0001", want: "cycle"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, "resolve", tt.ref, "--dir", fixture(t, tt.fixture))
			assertExit(t, got, 1)
			if !strings.Contains(got.stderr, tt.want) {
				t.Errorf("stderr = %q, want it to contain %q", got.stderr, tt.want)
			}
			if got.stdout != "" {
				t.Errorf("stdout = %q, want empty on failure", got.stdout)
			}
		})
	}
}
