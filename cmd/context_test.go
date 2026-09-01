package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/brief"
)

func TestContextTextReportsTheReferenceAndItsResolution(t *testing.T) {
	got := run(t, "context", "0002", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	assertPrefixes(t, "context", lines(got.stdout), []string{
		"ref",
		"  0002  Store thumbnails on the local disk  [superseded]  ",
		"    Chosen option: a directory on the local disk, sharded by the first two characters",
		"    of the cache key.",
		"resolves to (0002 is superseded)",
		"  0003  Store thumbnails in object storage  [accepted]  ",
		"    Chosen option: object storage behind a read-through layer, keeping the",
		"    content-addressed key from the caching decision unchanged.",
	})
	if got.stderr != "" {
		t.Errorf("stderr = %q, want empty", got.stderr)
	}
}

func TestContextTextGroupsTheNeighbourhood(t *testing.T) {
	got := run(t, "context", "0001", "--depth", "2", "--all", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	assertPrefixes(t, "context", lines(got.stdout), []string{
		"ref",
		"  0001  Cache rendered thumbnails  [accepted]  ",
		"    Chosen option: cache rendered thumbnails keyed by source URL and requested size.",
		"    The key is content-addressed, so an entry is never stale and never invalidated.",
		"ancestors",
		"  0003  Store thumbnails in object storage  [accepted]  ",
		"    Chosen option: object storage behind a read-through layer, keeping the",
		"    content-addressed key from the caching decision unchanged.",
		"  0004  Expire thumbnails after thirty days  [proposed]  ",
		"    Proposed: a lifecycle rule on the bucket, so expiry is configuration rather than",
		"    code.",
	})
}

func TestContextKeepsOnlyBindingNeighboursByDefault(t *testing.T) {
	got := run(t, "context", "0001", "--depth", "2", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	if strings.Contains(got.stdout, "0004") {
		t.Errorf("context = %q, want the proposed document left out without --all", got.stdout)
	}
	if !strings.Contains(got.stdout, "0003") {
		t.Errorf("context = %q, want the binding ancestor kept", got.stdout)
	}
}

func TestContextRestrictsTheWalkToAnEdgeType(t *testing.T) {
	got := run(t, "context", "0001", "--depth", "2", "--all", "--edge", "supersedes", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	if strings.Contains(got.stdout, "ancestors") {
		t.Errorf("context = %q, want no ancestors over supersedes alone", got.stdout)
	}
}

func TestContextReadsTheRequestedSection(t *testing.T) {
	got := run(t, "context", "0002", "--section", "Consequences", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	if !strings.Contains(got.stdout, "Nothing new to operate") {
		t.Errorf("context = %q, want the consequences paragraph", got.stdout)
	}
	if strings.Contains(got.stdout, "Chosen option") {
		t.Errorf("context = %q, want only the requested section", got.stdout)
	}
}

func TestContextBudgetDegradesEntriesToOneLine(t *testing.T) {
	got := run(t, "context", "0002", "--budget", "1", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	for _, line := range lines(got.stdout) {
		if strings.HasPrefix(line, "    ") {
			t.Errorf("line %q survived a budget of one token, want the one-line form", line)
		}
	}
	if !strings.Contains(got.stdout, "budget: 1 token") {
		t.Errorf("context = %q, want the spent budget reported", got.stdout)
	}
}

func TestContextJSONCarriesTheWholeBrief(t *testing.T) {
	got := run(t, "context", "0002", "--format", "json", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	b := decodeJSON[brief.Brief](t, got.stdout)
	if b.SchemaVersion != brief.SchemaVersion {
		t.Errorf("schema_version = %d, want %d", b.SchemaVersion, brief.SchemaVersion)
	}
	if b.Ref.ID != "0002" || b.Ref.Status != "superseded" {
		t.Errorf("ref = %+v, want the superseded document", b.Ref)
	}
	if len(b.ResolvesTo) != 1 || b.ResolvesTo[0].ID != "0003" {
		t.Errorf("resolves_to = %+v, want 0003", b.ResolvesTo)
	}
	if b.Budget.Limit != 2000 {
		t.Errorf("budget limit = %d, want the default of 2000 tokens", b.Budget.Limit)
	}
	if b.Budget.Used == 0 {
		t.Error("budget used = 0, want the tokens the report costs")
	}
	for _, key := range []string{`"ancestors"`, `"descendants"`} {
		if !strings.Contains(got.stdout, key) {
			t.Errorf("report = %q, want the %s key present even when empty", got.stdout, key)
		}
	}
}

func TestContextMarkdown(t *testing.T) {
	got := run(t, "context", "0002", "--format", "md", "--dir", fixture(t, "ok-madr"))

	assertExit(t, got, 0)
	for _, want := range []string{
		"# 0002 Store thumbnails on the local disk",
		"- status: superseded",
		"## Resolves to",
		"### 0003 Store thumbnails in object storage",
		"Chosen option: object storage behind a read-through layer, keeping the",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("markdown = %q, want it to contain %q", got.stdout, want)
		}
	}
}

func TestContextIsDeterministic(t *testing.T) {
	dir := fixture(t, "ok-madr")
	first := run(t, "context", "0001", "--depth", "2", "--all", "--dir", dir)
	second := run(t, "context", "0001", "--depth", "2", "--all", "--dir", dir)

	if first.stdout != second.stdout {
		t.Errorf("context output is not deterministic:\nfirst:\n%s\nsecond:\n%s", first.stdout, second.stdout)
	}
}

func TestContextFailures(t *testing.T) {
	dir := fixture(t, "ok-madr")
	tests := []struct {
		name string
		args []string
		want int
	}{
		{name: "unknown document", args: []string{"context", "0099"}, want: 1},
		{name: "unrecognized reference", args: []string{"context", "not-a-reference"}, want: 1},
		{name: "no reference", args: []string{"context"}, want: 2},
		{name: "unknown edge type", args: []string{"context", "0001", "--edge", "relates-to"}, want: 2},
		{name: "report format", args: []string{"context", "0001", "--format", "github"}, want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := run(t, append(tt.args, "--dir", dir)...)

			assertExit(t, got, tt.want)
			if got.stderr == "" {
				t.Error("stderr is empty, want a diagnostic")
			}
			if got.stdout != "" {
				t.Errorf("stdout = %q, want empty on failure", got.stdout)
			}
		})
	}
}

func TestContextReadsAMultiKindCorpus(t *testing.T) {
	got := run(t, "context", "UZ-V-001", "--config", filepath.Join(fixture(t, "kinds"), "docdag.yaml"))

	assertExit(t, got, 0)
	// One brief over three kinds: the clause, the conformance test that
	// enforces it and the deviation recorded against it.
	assertPrefixes(t, "context", lines(got.stdout), []string{
		"ref",
		"  UZ-V-001  Every claim carries evidence  [accepted]  ",
		"ancestors",
		"  UZ-V-006  Thresholds are declared before the run  [accepted]  ",
		"  conform/uz-v-001  Check that every claim carries evidence  [accepted]  ",
		"  dev-0001  The importer reports counts without inputs  [accepted]  ",
	})
}
