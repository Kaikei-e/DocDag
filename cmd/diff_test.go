package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/render"
)

func TestDiffComparesWorkingTreeAgainstBaseRevision(t *testing.T) {
	t.Run("reports added edge, status change, added document, and binding delta", func(t *testing.T) {
		dir := gitRepo(t, map[string]string{
			"docs/adr/0001-use-sqlite.md": "---\ntitle: Use SQLite\nstatus: accepted\ndate: 2025-01-01\n---\n\n# Use SQLite\n",
			"docs/adr/0002-use-queue.md":  "---\ntitle: Use queue\nstatus: accepted\ndepends-on:\n  - \"0001\"\ndate: 2025-02-01\n---\n\n# Use queue\n",
		})
		t.Chdir(dir)

		if err := os.WriteFile(filepath.Join(dir, "docs/adr/0001-use-sqlite.md"),
			[]byte("---\ntitle: Use SQLite\nstatus: superseded\ndate: 2025-01-01\n---\n\n# Use SQLite\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "docs/adr/0003-use-postgres.md"),
			[]byte("---\ntitle: Use Postgres\nstatus: accepted\nsupersedes:\n  - \"0001\"\ndate: 2025-03-01\n---\n\n# Use Postgres\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		got := run(t, "diff", "HEAD")

		assertExit(t, got, 0)
		assertLines(t, "diff text", lines(got.stdout), []string{
			"+ supersedes 0003 -> 0001",
			"~ status     0001: accepted -> superseded",
			"+ document   0003 [accepted]",
			"binding: 2 -> 2 (0)",
		})
	})

	t.Run("reports only the binding line when nothing differs", func(t *testing.T) {
		dir := gitRepo(t, map[string]string{
			"docs/adr/0001-use-sqlite.md": "---\ntitle: Use SQLite\nstatus: accepted\ndate: 2025-01-01\n---\n\n# Use SQLite\n",
		})
		t.Chdir(dir)

		got := run(t, "diff", "HEAD")

		assertExit(t, got, 0)
		assertLines(t, "diff text", lines(got.stdout), []string{"binding: 1 -> 1 (0)"})
	})

	t.Run("compares two revisions with --at and evaluates --as-of on both sides", func(t *testing.T) {
		dir := gitRepoAt(t, "2026-01-01", map[string]string{
			"docdag.yaml": testPeriodConfig,
			"docs/adr/0001-serve-images-from-the-application.md": testCurrentDocument,
		})
		t.Chdir(dir)
		if err := os.WriteFile(filepath.Join(dir, "docs/adr/0002-serve-images-from-a-cdn.md"),
			[]byte(testFutureDocument), 0o644); err != nil {
			t.Fatal(err)
		}
		git(t, dir, "add", "-A")
		gitCommit(t, dir, "2026-02-01", "add future CDN decision")
		// Mutate the working tree to ensure --at reads HEAD rather than disk.
		if err := os.Remove(filepath.Join(dir, "docs/adr/0002-serve-images-from-a-cdn.md")); err != nil {
			t.Fatal(err)
		}

		got := run(t, "diff", "HEAD~1", "--at", "HEAD", "--as-of", "2027-01-01", "--format", "json")

		assertExit(t, got, 0)
		var report render.DiffReport
		if err := json.Unmarshal([]byte(got.stdout), &report); err != nil {
			t.Fatalf("unmarshal diff JSON: %v\n%s", err, got.stdout)
		}
		if report.SchemaVersion != render.DiffSchemaVersion {
			t.Errorf("schema_version = %d, want %d", report.SchemaVersion, render.DiffSchemaVersion)
		}
		if report.AsOf != "2027-01-01" || report.At != "HEAD" {
			t.Errorf("header = (as_of=%q, at=%q), want (2027-01-01, HEAD)", report.AsOf, report.At)
		}
		if len(report.Edges.Added) != 1 || len(report.Documents.Added) != 1 {
			t.Fatalf("unexpected diff contents: %+v", report)
		}
		if report.Binding.Before != 1 || report.Binding.After != 1 || report.Binding.Delta != 0 {
			t.Errorf("binding = %+v, want {Before:1 After:1 Delta:0}", report.Binding)
		}
	})

	t.Run("renders github annotations anchored to changed files", func(t *testing.T) {
		dir := gitRepo(t, map[string]string{
			"docs/adr/0001-use-sqlite.md": "---\ntitle: Use SQLite\nstatus: accepted\ndate: 2025-01-01\n---\n\n# Use SQLite\n",
		})
		t.Chdir(dir)
		if err := os.WriteFile(filepath.Join(dir, "docs/adr/0002-use-postgres.md"),
			[]byte("---\ntitle: Use Postgres\nstatus: accepted\nsupersedes:\n  - \"0001\"\ndate: 2025-02-01\n---\n\n# Use Postgres\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		got := run(t, "diff", "HEAD", "--format", "github")

		assertExit(t, got, 0)
		ls := lines(got.stdout)
		if len(ls) != 3 {
			t.Fatalf("got %d lines, want 3:\n%s", len(ls), got.stdout)
		}
		if !strings.HasPrefix(ls[0], "::notice file=docs/adr/0002-use-postgres.md,line=") {
			t.Errorf("annotation 0 = %q, want file/line notice", ls[0])
		}
		if !strings.Contains(ls[0], "title=docdag diff::+ supersedes 0002 -> 0001") {
			t.Errorf("annotation 0 = %q, want supersedes detail", ls[0])
		}
		if ls[2] != "binding: 1 -> 1 (0)" {
			t.Errorf("summary line = %q, want binding: 1 -> 1 (0)", ls[2])
		}
	})
}

func TestDiffFailuresMatchQueryAt(t *testing.T) {
	t.Run("unknown revision exits 3 with --at error wording", func(t *testing.T) {
		dir := gitRepo(t, map[string]string{
			"docs/adr/0001-use-sqlite.md": "---\ntitle: Use SQLite\nstatus: accepted\ndate: 2025-01-01\n---\n\n# Use SQLite\n",
		})
		t.Chdir(dir)

		gotDiff := run(t, "diff", "no-such-ref")
		gotQuery := run(t, "query", "--binding", "--at", "no-such-ref")

		assertExit(t, gotDiff, exitIO)
		if gotDiff.stderr != gotQuery.stderr {
			t.Errorf("diff stderr = %q, want %q", gotDiff.stderr, gotQuery.stderr)
		}
	})

	t.Run("outside a git repository exits 3 with --at error wording", func(t *testing.T) {
		dir := writeDocs(t, map[string]string{
			"docs/adr/0001-use-sqlite.md": "---\ntitle: Use SQLite\nstatus: accepted\ndate: 2025-01-01\n---\n\n# Use SQLite\n",
		})
		t.Chdir(dir)

		gotDiff := run(t, "diff", "HEAD")
		gotQuery := run(t, "query", "--binding", "--at", "HEAD")

		assertExit(t, gotDiff, exitIO)
		if gotDiff.stderr != gotQuery.stderr {
			t.Errorf("diff stderr = %q, want %q", gotDiff.stderr, gotQuery.stderr)
		}
	})

	t.Run("empty corpus on the base side exits 3", func(t *testing.T) {
		dir := writeDocs(t, map[string]string{
			"README.md": "hello\n",
		})
		git(t, dir, "init", "--quiet")
		git(t, dir, "add", "-A")
		gitCommit(t, dir, "2026-01-01", "no managed documents")
		if err := os.MkdirAll(filepath.Join(dir, "docs/adr"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "docs/adr/0001-use-sqlite.md"),
			[]byte("---\ntitle: Use SQLite\nstatus: accepted\ndate: 2025-01-01\n---\n\n# Use SQLite\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Chdir(dir)

		got := run(t, "diff", "HEAD")

		assertExit(t, got, exitIO)
	})
}
