package vcs

import (
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
