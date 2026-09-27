package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/render"
	"github.com/Kaikei-e/DocDag/model"
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
	got := run(t, "resolve", "0002", "--format", "json", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	records := decodeJSON[[]render.Record](t, got.stdout)
	if len(records) != 1 {
		t.Fatalf("records = %+v, want one successor", records)
	}
	r := records[0]
	if r.ID != model.ID("0003") || r.Title != "Store thumbnails in object storage" || r.Status != "accepted" {
		t.Errorf("record = %+v, want the successor described", r)
	}
	if filepath.Base(r.Path) != "0003-store-thumbnails-in-object-storage.md" {
		t.Errorf("path = %q, want the successor's file", r.Path)
	}
}

func TestResolveFieldsSelectTheTextColumns(t *testing.T) {
	dir := fixture(t, "ok-madr")

	bare := run(t, "resolve", "0002", "--dir", dir)
	assertExit(t, bare, 0)
	assertLines(t, "resolve", lines(bare.stdout), []string{"0003"})

	got := run(t, "resolve", "0002", "--fields", "id,status,title", "--dir", dir)

	assertExit(t, got, 0)
	assertLines(t, "resolve", lines(got.stdout), []string{"0003\taccepted\tStore thumbnails in object storage"})
}

func TestResolveNeverPrintsADocumentTheCorpusDoesNotHold(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"0001-a-decision.md": "---\ntitle: A decision\nstatus: superseded by 0099\ndate: 2025-01-01\n---\n\n# A decision\n",
	})

	got := run(t, "resolve", "0001", "--dir", dir)

	assertExit(t, got, 0)
	assertLines(t, "resolve", lines(got.stdout), []string{"0001"})
}

const testReplacesConfig = `edges:
  - name: replaces
    key: replaces
    acyclic: true
    direction: forward
rules:
  - name: replaced_must_not_be_accepted
    severity: error
    when:
      inbound: replaces
      attr:
        status:
          eq: accepted
    message: "a replaced decision cannot stay accepted"
derived_edges:
  - field: status
    pattern: "(?i)^replaced by\\s+(\\S+)"
    edge: replaces
    direction: reverse
`

// testReplacesCorpus configures a corpus whose only edge type is "replaces",
// so the commands defined over supersedes have nothing to walk.
func testReplacesCorpus(t *testing.T) string {
	t.Helper()
	dir := writeDocs(t, map[string]string{
		"docdag.yaml":                     testReplacesConfig,
		"docs/adr/0001-first.md":          "---\ntitle: First\nstatus: superseded\ndate: 2025-01-01\n---\n\n# First\n",
		"docs/adr/0002-replacement.md":    "---\ntitle: Replacement\nstatus: accepted\nreplaces:\n  - \"0001\"\ndate: 2025-02-01\n---\n\n# Replacement\n",
		"docs/adr/0003-independent.md":    "---\ntitle: Independent\nstatus: accepted\ndate: 2025-03-01\n---\n\n# Independent\n",
		"docs/adr/0004-also-accepted.md":  "---\ntitle: Also accepted\nstatus: accepted\ndate: 2025-04-01\n---\n\n# Also accepted\n",
		"docs/adr/0005-still-proposed.md": "---\ntitle: Still proposed\nstatus: proposed\ndate: 2025-05-01\n---\n\n# Still proposed\n",
	})
	t.Chdir(dir)
	return dir
}

func TestSupersedesCommandsRefuseAConfigurationWithoutThatEdgeType(t *testing.T) {
	// Walking an edge type nobody declared would answer "current" for every
	// document, at exit 0. That is worse than refusing.
	testReplacesCorpus(t)

	if got := run(t, "validate"); got.code != 0 {
		t.Fatalf("validate exit = %d, want 0 (stdout=%q stderr=%q)", got.code, got.stdout, got.stderr)
	}
	for _, args := range [][]string{
		{"resolve", "0001"},
		{"query", "--binding"},
		{"stats"},
	} {
		t.Run(args[0], func(t *testing.T) {
			got := run(t, args...)

			assertExit(t, got, 3)
			if !strings.Contains(got.stderr, "supersedes") {
				t.Errorf("stderr = %q, want it to name the missing edge type", got.stderr)
			}
			if got.stdout != "" {
				t.Errorf("stdout = %q, want empty", got.stdout)
			}
		})
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

func TestResolveWithPendingSuccessor(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"0001-record-architecture-decisions.md": "---\ntitle: Record architecture decisions\nstatus: accepted\ndate: 2025-01-01\n---\n\n# Record architecture decisions\n",
		"0002-use-markdown-adrs.md":             "---\ntitle: Use Markdown ADRs\nstatus: proposed\nsupersedes:\n  - 0001\ndate: 2025-02-01\n---\n\n# Use Markdown ADRs\n",
	})

	t.Run("text output prints the predecessor and notes pending successor on stderr", func(t *testing.T) {
		got := run(t, "resolve", "0001", "--dir", dir)
		assertExit(t, got, 0)
		assertLines(t, "resolve", lines(got.stdout), []string{"0001"})
		wantNote := "note: 0002 supersedes 0001 but is proposed; not yet binding"
		if !strings.Contains(got.stderr, wantNote) {
			t.Errorf("stderr = %q, want it to contain %q", got.stderr, wantNote)
		}
	})

	t.Run("json output includes pending list", func(t *testing.T) {
		got := run(t, "resolve", "0001", "--format", "json", "--dir", dir)
		assertExit(t, got, 0)
		type resolveRecordJSON struct {
			render.Record
			Pending []graph.PendingSuccessor `json:"pending"`
		}
		records := decodeJSON[[]resolveRecordJSON](t, got.stdout)
		if len(records) != 1 {
			t.Fatalf("records = %+v, want one record", records)
		}
		if records[0].ID != "0001" {
			t.Errorf("id = %q, want 0001", records[0].ID)
		}
		if len(records[0].Pending) != 1 {
			t.Fatalf("pending = %+v, want 1 pending successor", records[0].Pending)
		}
		p := records[0].Pending[0]
		if p.ID != "0002" || p.Predecessor != "0001" || p.Status != "proposed" {
			t.Errorf("pending = %+v, want 0002 superseding 0001 with status proposed", p)
		}
	})
}

func TestResolveProposedSupersededByProposed(t *testing.T) {
	// Observed in consumer corpus: ADR 000970 (proposed) superseded by 000974 (proposed)
	dir := writeDocs(t, map[string]string{
		"docdag.yaml":      "id_width: 6\n",
		"000970-first.md":  "---\ntitle: First proposed\nstatus: proposed\ndate: 2025-01-01\n---\n\n# First proposed\n",
		"000974-second.md": "---\ntitle: Second proposed\nstatus: proposed\nsupersedes:\n  - 000970\ndate: 2025-02-01\n---\n\n# Second proposed\n",
	})

	got := run(t, "resolve", "000970", "--config", filepath.Join(dir, "docdag.yaml"), "--dir", dir)
	assertExit(t, got, 0)
	assertLines(t, "resolve", lines(got.stdout), []string{"000970"})
	wantNote := "note: 000974 supersedes 000970 but is proposed; not yet binding"
	if !strings.Contains(got.stderr, wantNote) {
		t.Errorf("stderr = %q, want note %q", got.stderr, wantNote)
	}
}

func TestResolveChainAcceptedToProposed(t *testing.T) {
	dir := writeDocs(t, map[string]string{
		"0001-first.md":  "---\ntitle: First\nstatus: superseded\ndate: 2025-01-01\n---\n\n# First\n",
		"0002-second.md": "---\ntitle: Second\nstatus: accepted\nsupersedes:\n  - 0001\ndate: 2025-02-01\n---\n\n# Second\n",
		"0003-third.md":  "---\ntitle: Third\nstatus: proposed\nsupersedes:\n  - 0002\ndate: 2025-03-01\n---\n\n# Third\n",
	})

	t.Run("resolving head stops at accepted middle document", func(t *testing.T) {
		got := run(t, "resolve", "0001", "--dir", dir)
		assertExit(t, got, 0)
		assertLines(t, "resolve", lines(got.stdout), []string{"0002"})
		wantNote := "note: 0003 supersedes 0002 but is proposed; not yet binding"
		if !strings.Contains(got.stderr, wantNote) {
			t.Errorf("stderr = %q, want note %q", got.stderr, wantNote)
		}
	})

	t.Run("resolving accepted middle document stops at itself", func(t *testing.T) {
		got := run(t, "resolve", "0002", "--dir", dir)
		assertExit(t, got, 0)
		assertLines(t, "resolve", lines(got.stdout), []string{"0002"})
		wantNote := "note: 0003 supersedes 0002 but is proposed; not yet binding"
		if !strings.Contains(got.stderr, wantNote) {
			t.Errorf("stderr = %q, want note %q", got.stderr, wantNote)
		}
	})
}
