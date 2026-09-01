package vcs

import (
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// testRepo builds a git repository holding files, committed as one revision.
// Identity comes from flags so the suite runs on a machine without a git
// identity, such as a fresh CI runner.
func testRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	dir := t.TempDir()
	testGit(t, dir, "init", "--quiet")
	testWrite(t, dir, files)
	testGit(t, dir, "add", "-A")
	testCommit(t, dir, "the first revision")
	return dir
}

func testGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "nonexistent-gitconfig"), "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func testCommit(t *testing.T, dir, message string) {
	t.Helper()
	testGit(t, dir,
		"-c", "user.name=DocDag Test",
		"-c", "user.email=test@example.test",
		"-c", "commit.gpgsign=false",
		"commit", "--quiet", "-m", message)
}

func testWrite(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatalf("mkdir for %s: %v", name, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}

func TestOpen(t *testing.T) {
	dir := testRepo(t, map[string]string{"docs/adr/0001-a-decision.md": "# A decision\n"})

	t.Run("a subdirectory finds the working tree root", func(t *testing.T) {
		repo, err := Open(filepath.Join(dir, "docs", "adr"))
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		want, err := filepath.EvalSymlinks(dir)
		if err != nil {
			t.Fatalf("EvalSymlinks: %v", err)
		}
		got, err := filepath.EvalSymlinks(repo.Root())
		if err != nil {
			t.Fatalf("EvalSymlinks: %v", err)
		}
		if got != want {
			t.Errorf("Root = %q, want %q", got, want)
		}
	})

	t.Run("a directory outside a repository is an error", func(t *testing.T) {
		if _, err := Open(t.TempDir()); err == nil {
			t.Fatal("Open succeeded outside a repository, want an error")
		}
	})
}

func TestRepoMergeBase(t *testing.T) {
	dir := testRepo(t, map[string]string{"docs/adr/0001-a-decision.md": "# A decision\n"})
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	head := strings.TrimSpace(testGit(t, dir, "rev-parse", "HEAD"))

	t.Run("a revision that is an ancestor is its own merge base", func(t *testing.T) {
		testWrite(t, dir, map[string]string{"docs/adr/0002-another.md": "# Another\n"})
		testGit(t, dir, "add", "-A")
		testCommit(t, dir, "the second revision")

		if got := repo.MergeBase(head); got != head {
			t.Fatalf("MergeBase = %q, want %q", got, head)
		}
	})

	t.Run("a revision git cannot resolve comes back unchanged", func(t *testing.T) {
		if got := repo.MergeBase("no-such-revision"); got != "no-such-revision" {
			t.Fatalf("MergeBase = %q, want the revision as given", got)
		}
	})
}

func TestRepoChanges(t *testing.T) {
	dir := testRepo(t, map[string]string{
		"docs/adr/0001-a-decision.md": "# A decision\n",
		"docs/adr/0002-another.md":    "# Another\n",
		"README.md":                   "# Readme\n",
	})
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	head := strings.TrimSpace(testGit(t, dir, "rev-parse", "HEAD"))

	testWrite(t, dir, map[string]string{
		"docs/adr/0001-a-decision.md": "# A decision\n\nMore.\n",
		"README.md":                   "# Readme\n\nMore.\n",
	})
	if err := os.Remove(filepath.Join(dir, "docs", "adr", "0002-another.md")); err != nil {
		t.Fatalf("remove: %v", err)
	}

	t.Run("the working tree is compared against the revision", func(t *testing.T) {
		got, err := repo.Changes(head, filepath.Join(dir, "docs", "adr"))
		if err != nil {
			t.Fatalf("Changes: %v", err)
		}
		want := []Change{
			{Status: 'M', Path: "docs/adr/0001-a-decision.md"},
			{Status: 'D', Path: "docs/adr/0002-another.md"},
		}
		slices.SortFunc(got, func(a, b Change) int { return strings.Compare(a.Path, b.Path) })
		if !slices.Equal(got, want) {
			t.Fatalf("Changes = %+v, want %+v", got, want)
		}
	})

	t.Run("an untracked file is listed on its own", func(t *testing.T) {
		testWrite(t, dir, map[string]string{"docs/adr/0003-new.md": "# New\n"})

		got, err := repo.Untracked(filepath.Join(dir, "docs", "adr"))
		if err != nil {
			t.Fatalf("Untracked: %v", err)
		}
		if !slices.Equal(got, []string{"docs/adr/0003-new.md"}) {
			t.Fatalf("Untracked = %v, want the new document", got)
		}
	})
}

func TestRepoLastChanged(t *testing.T) {
	dir := testRepo(t, map[string]string{
		"docs/adr/0001-a-decision.md": "# A decision\n",
		"docs/adr/0002-another.md":    "# Another\n",
		"README.md":                   "# Readme\n",
	})
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	first := testCommittedDay(t, dir, "HEAD")
	// A second commit touching one of the two documents, dated later than the
	// first whatever day the suite runs on, so the walk has two days to choose
	// between for that path and one for the other.
	testWrite(t, dir, map[string]string{"docs/adr/0001-a-decision.md": "# A decision\n\nMore.\n"})
	testGit(t, dir, "add", "-A")
	testCommitAt(t, dir, "the second revision", "2099-01-02T12:00:00+00:00")

	t.Run("each file reports the day it last changed", func(t *testing.T) {
		got, err := repo.LastChanged(filepath.Join(dir, "docs", "adr"))
		if err != nil {
			t.Fatalf("LastChanged: %v", err)
		}
		want := map[string]string{
			"docs/adr/0001-a-decision.md": "2099-01-02",
			"docs/adr/0002-another.md":    first,
		}
		if !maps.Equal(got, want) {
			t.Fatalf("LastChanged = %v, want %v", got, want)
		}
	})

	t.Run("a directory nobody asked about is left out", func(t *testing.T) {
		got, err := repo.LastChanged(filepath.Join(dir, "docs", "adr"))
		if err != nil {
			t.Fatalf("LastChanged: %v", err)
		}
		if _, listed := got["README.md"]; listed {
			t.Fatalf("LastChanged = %v, want the pathspec to hold it to the documents", got)
		}
	})

	t.Run("naming no directory asks about the whole tree", func(t *testing.T) {
		got, err := repo.LastChanged()
		if err != nil {
			t.Fatalf("LastChanged: %v", err)
		}
		if got["README.md"] != first {
			t.Fatalf("LastChanged = %v, want the README dated too", got)
		}
	})

	t.Run("a path outside the tree is an error rather than an empty answer", func(t *testing.T) {
		if _, err := repo.LastChanged(t.TempDir()); err == nil {
			t.Fatal("LastChanged succeeded outside the working tree, want an error")
		}
	})
}

func TestRepoRel(t *testing.T) {
	dir := testRepo(t, map[string]string{"docs/adr/0001-a-decision.md": "# A decision\n"})
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Run("a file is named the way git names it", func(t *testing.T) {
		got, err := repo.Rel(filepath.Join(dir, "docs", "adr", "0001-a-decision.md"))
		if err != nil {
			t.Fatalf("Rel: %v", err)
		}
		if got != "docs/adr/0001-a-decision.md" {
			t.Fatalf("Rel = %q, want the repository-relative slash-separated path", got)
		}
	})

	t.Run("the working tree root is the current directory", func(t *testing.T) {
		got, err := repo.Rel(dir)
		if err != nil {
			t.Fatalf("Rel: %v", err)
		}
		if got != "." {
			t.Fatalf("Rel = %q, want .", got)
		}
	})
}

// testCommitAt commits with the dates pinned, so a test can say which day a
// file changed on whatever day it runs. Git reads them from the environment:
// there is no configuration key for a commit's date.
func testCommitAt(t *testing.T, dir, message, date string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir,
		"-c", "user.name=DocDag Test",
		"-c", "user.email=test@example.test",
		"-c", "commit.gpgsign=false",
		"commit", "--quiet", "-m", message)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL="+filepath.Join(dir, "nonexistent-gitconfig"),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_DATE="+date,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v: %s", err, out)
	}
}

func testCommittedDay(t *testing.T, dir, rev string) string {
	t.Helper()
	return strings.TrimSpace(testGit(t, dir, "show", "-s", "--format=%cs", rev))
}

func TestRepoFile(t *testing.T) {
	dir := testRepo(t, map[string]string{"docs/adr/0001-a-decision.md": "# A decision\n"})
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	head := strings.TrimSpace(testGit(t, dir, "rev-parse", "HEAD"))

	t.Run("the committed contents come back", func(t *testing.T) {
		testWrite(t, dir, map[string]string{"docs/adr/0001-a-decision.md": "# Rewritten\n"})

		got, err := repo.File(head, "docs/adr/0001-a-decision.md")
		if err != nil {
			t.Fatalf("File: %v", err)
		}
		if string(got) != "# A decision\n" {
			t.Fatalf("File = %q, want the committed contents", got)
		}
	})

	t.Run("a path the revision does not hold is an error", func(t *testing.T) {
		if _, err := repo.File(head, "docs/adr/0009-absent.md"); err == nil {
			t.Fatal("File succeeded for an absent path, want an error")
		}
	})
}

func TestRepoFiles(t *testing.T) {
	dir := testRepo(t, map[string]string{
		"docs/adr/0001-a-decision.md":     "# A decision\n",
		"docs/adr/notes/0002-a-nested.md": "# Nested\n",
		"elsewhere/0003-another.md":       "# Elsewhere\n",
	})
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	head := strings.TrimSpace(testGit(t, dir, "rev-parse", "HEAD"))

	t.Run("the revision's own files come back", func(t *testing.T) {
		// The working tree gains a file the revision does not hold, which is
		// exactly what a comparison against a revision must not see.
		testWrite(t, dir, map[string]string{"docs/adr/0004-later.md": "# Later\n"})

		got, err := repo.Files(head, filepath.Join(dir, "docs", "adr"))
		if err != nil {
			t.Fatalf("Files: %v", err)
		}

		want := []string{"docs/adr/0001-a-decision.md", "docs/adr/notes/0002-a-nested.md"}
		if !slices.Equal(got, want) {
			t.Errorf("Files = %v, want %v", got, want)
		}
	})

	t.Run("a directory outside the working tree is an error", func(t *testing.T) {
		if _, err := repo.Files(head, filepath.Join(t.TempDir(), "elsewhere")); err == nil {
			t.Fatal("Files succeeded outside the working tree, want an error")
		}
	})
}

func TestRepoCommitterDate(t *testing.T) {
	dir := testRepo(t, map[string]string{"docs/adr/0001-a-decision.md": "# A decision\n"})
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	t.Run("the day a revision was committed on", func(t *testing.T) {
		got, err := repo.CommitterDate("HEAD")
		if err != nil {
			t.Fatalf("CommitterDate: %v", err)
		}

		want := strings.TrimSpace(testGit(t, dir, "log", "-1", "--format=%cs"))
		if got != want {
			t.Errorf("CommitterDate = %q, want %q", got, want)
		}
	})

	t.Run("a revision git cannot resolve is an error", func(t *testing.T) {
		if _, err := repo.CommitterDate("no-such-revision"); err == nil {
			t.Fatal("CommitterDate succeeded on a revision that does not exist, want an error")
		}
	})
}

func TestRepoCheckout(t *testing.T) {
	dir := testRepo(t, map[string]string{
		"spec/clauses/UZ-V-001.md": "# A clause\n",
		"spec/conform/uz-v-001.md": "# A test\n",
	})
	repo, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	head := strings.TrimSpace(testGit(t, dir, "rev-parse", "HEAD"))

	t.Run("every directory the caller named comes back, with the revision's files in it", func(t *testing.T) {
		// The working tree gains a document the revision does not hold: a
		// checkout is what the revision says, not what the disk says.
		testWrite(t, dir, map[string]string{"spec/clauses/UZ-V-002.md": "# A later clause\n"})

		tree, err := repo.Checkout(head, map[string]string{
			"clause":  filepath.Join(dir, "spec", "clauses"),
			"conform": filepath.Join(dir, "spec", "conform"),
			// A kind the revision predates: the directory is there and empty,
			// so the corpus of that revision simply holds none of them.
			"measure": filepath.Join(dir, "spec", "measures"),
		})
		if err != nil {
			t.Fatalf("Checkout: %v", err)
		}
		defer func() {
			if err := tree.Close(); err != nil {
				t.Errorf("Close: %v", err)
			}
		}()

		if got := slices.Sorted(maps.Keys(tree.Dirs())); !slices.Equal(got, []string{"clause", "conform", "measure"}) {
			t.Fatalf("Dirs = %v, want one entry per directory asked for", got)
		}
		for name, want := range map[string][]string{
			"clause":  {"UZ-V-001.md"},
			"conform": {"uz-v-001.md"},
			"measure": nil,
		} {
			entries, err := os.ReadDir(tree.Dirs()[name])
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			got := make([]string, 0, len(entries))
			for _, entry := range entries {
				got = append(got, entry.Name())
			}
			if !slices.Equal(got, want) {
				t.Errorf("%s holds %v, want %v", name, got, want)
			}
		}
		// The layout below the root is the repository's own, so a report about
		// the revision names paths a reader can find in it.
		if rel, err := filepath.Rel(tree.Root(), tree.Dirs()["clause"]); err != nil || filepath.ToSlash(rel) != "spec/clauses" {
			t.Errorf("clause dir = %q under %q, want the repository's own layout", tree.Dirs()["clause"], tree.Root())
		}
	})

	t.Run("closing removes the tree", func(t *testing.T) {
		tree, err := repo.Checkout(head, map[string]string{"": filepath.Join(dir, "spec", "clauses")})
		if err != nil {
			t.Fatalf("Checkout: %v", err)
		}
		root := tree.Root()

		if err := tree.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Errorf("stat %s = %v, want the tree removed", root, err)
		}
	})

	t.Run("a revision git cannot resolve is an error", func(t *testing.T) {
		if _, err := repo.Checkout("no-such-revision", map[string]string{"": filepath.Join(dir, "spec", "clauses")}); err == nil {
			t.Fatal("Checkout succeeded on a revision that does not exist, want an error")
		}
	})
}
