package graph

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/vcs"
)

const (
	testAcceptedDoc = "---\ntitle: Serve images from the application\nstatus: accepted\ndate: 2025-01-01\n---\n\n" +
		"# Serve images from the application\n\n## Decision Outcome\n\nThe application resizes and serves images itself.\n"
	testProposedDoc = "---\ntitle: Serve images from a CDN\nstatus: proposed\ndate: 2025-02-01\n---\n\n" +
		"# Serve images from a CDN\n\n## Decision Outcome\n\nA CDN fronts the image endpoint.\n"
	testBodylessDoc = "---\ntitle: Serve images from the application\nstatus: accepted\ndate: 2025-01-01\n---\n"
)

// testImmutableRepo commits an ADR corpus and returns the repository directory
// and an open handle on it.
func testImmutableRepo(t *testing.T, files map[string]string) (string, *vcs.Repo) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	dir := t.TempDir()
	testGit(t, dir, "init", "--quiet")
	testWriteFiles(t, dir, files)
	testGit(t, dir, "add", "-A")
	testGit(t, dir,
		"-c", "user.name=DocDag Test",
		"-c", "user.email=test@example.test",
		"-c", "commit.gpgsign=false",
		"commit", "--quiet", "-m", "the first revision")
	repo, err := vcs.Open(dir)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	return dir, repo
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

func testWriteFiles(t *testing.T, dir string, files map[string]string) {
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

func testImmutableConfig(dir string) config.Config {
	cfg := config.ADRPreset()
	cfg.Dir = filepath.Join(dir, "docs", "adr")
	cfg.Edges[0].Inverse = testInverseKey
	return cfg
}

func testCheckImmutable(t *testing.T, dir string, repo *vcs.Repo) []model.Finding {
	t.Helper()
	findings, err := CheckImmutable(repo, testImmutableConfig(dir), "HEAD", dir)
	if err != nil {
		t.Fatalf("CheckImmutable: %v", err)
	}
	return findings
}

func TestCheckImmutable(t *testing.T) {
	corpus := map[string]string{
		"docs/adr/0001-serve-images-from-the-application.md": testAcceptedDoc,
		"docs/adr/0002-serve-images-from-a-cdn.md":           testProposedDoc,
	}

	t.Run("an untouched corpus reports nothing", func(t *testing.T) {
		dir, repo := testImmutableRepo(t, corpus)

		if got := testCheckImmutable(t, dir, repo); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("a new document is always allowed", func(t *testing.T) {
		dir, repo := testImmutableRepo(t, corpus)
		testWriteFiles(t, dir, map[string]string{"docs/adr/0003-new.md": testProposedDoc})

		if got := testCheckImmutable(t, dir, repo); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("a document that was not accepted may change freely", func(t *testing.T) {
		dir, repo := testImmutableRepo(t, corpus)
		testWriteFiles(t, dir, map[string]string{
			"docs/adr/0002-serve-images-from-a-cdn.md": strings.Replace(testProposedDoc, "A CDN fronts", "Nothing fronts", 1),
		})

		if got := testCheckImmutable(t, dir, repo); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("appending to the body is allowed", func(t *testing.T) {
		dir, repo := testImmutableRepo(t, corpus)
		testWriteFiles(t, dir, map[string]string{
			"docs/adr/0001-serve-images-from-the-application.md": testAcceptedDoc + "\n## Consequences\n\nImage traffic shares the request pool.\n",
		})

		if got := testCheckImmutable(t, dir, repo); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("appending to a document that had no body is allowed", func(t *testing.T) {
		name := "docs/adr/0001-serve-images-from-the-application.md"
		dir, repo := testImmutableRepo(t, map[string]string{name: testBodylessDoc})
		testWriteFiles(t, dir, map[string]string{name: testBodylessDoc + "# Serve images from the application\n"})

		if got := testCheckImmutable(t, dir, repo); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("changing the status value is allowed", func(t *testing.T) {
		dir, repo := testImmutableRepo(t, corpus)
		testWriteFiles(t, dir, map[string]string{
			"docs/adr/0001-serve-images-from-the-application.md": strings.Replace(testAcceptedDoc, "status: accepted", "status: superseded", 1),
		})

		if got := testCheckImmutable(t, dir, repo); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("adding an entry under an inverse key is allowed", func(t *testing.T) {
		dir, repo := testImmutableRepo(t, corpus)
		testWriteFiles(t, dir, map[string]string{
			"docs/adr/0001-serve-images-from-the-application.md": strings.Replace(
				testAcceptedDoc, "status: accepted", "status: accepted\n"+testInverseKey+":\n  - 0002", 1),
		})

		if got := testCheckImmutable(t, dir, repo); len(got) != 0 {
			t.Fatalf("findings = %+v, want none", got)
		}
	})

	t.Run("rewriting the body is a violation", func(t *testing.T) {
		dir, repo := testImmutableRepo(t, corpus)
		testWriteFiles(t, dir, map[string]string{
			"docs/adr/0001-serve-images-from-the-application.md": strings.Replace(
				testAcceptedDoc, "The application resizes", "A CDN resizes", 1),
		})

		f := testAssertSingleFinding(t, testCheckImmutable(t, dir, repo),
			model.RuleImmutableViolation, model.SeverityError, "0001")
		if !strings.Contains(f.Detail, "body") {
			t.Errorf("detail = %q, want it to name the body", f.Detail)
		}
		if f.Location.Line != 11 {
			t.Errorf("location = %+v, want the first line the body diverges at", f.Location)
		}
	})

	t.Run("changing another frontmatter key is a violation", func(t *testing.T) {
		dir, repo := testImmutableRepo(t, corpus)
		testWriteFiles(t, dir, map[string]string{
			"docs/adr/0001-serve-images-from-the-application.md": strings.Replace(
				testAcceptedDoc, "title: Serve images from the application", "title: Serve images ourselves", 1),
		})

		f := testAssertSingleFinding(t, testCheckImmutable(t, dir, repo),
			model.RuleImmutableViolation, model.SeverityError, "0001")
		if !strings.Contains(f.Detail, "title") {
			t.Errorf("detail = %q, want it to name the key", f.Detail)
		}
		if f.Location.Line != 2 {
			t.Errorf("location = %+v, want the line of the key that changed", f.Location)
		}
	})

	t.Run("removing an entry under an inverse key is a violation", func(t *testing.T) {
		withInverse := map[string]string{
			"docs/adr/0001-serve-images-from-the-application.md": strings.Replace(
				testAcceptedDoc, "status: accepted", "status: accepted\n"+testInverseKey+":\n  - 0002", 1),
		}
		dir, repo := testImmutableRepo(t, withInverse)
		testWriteFiles(t, dir, map[string]string{
			"docs/adr/0001-serve-images-from-the-application.md": testAcceptedDoc,
		})

		f := testAssertSingleFinding(t, testCheckImmutable(t, dir, repo),
			model.RuleImmutableViolation, model.SeverityError, "0001")
		if !strings.Contains(f.Detail, testInverseKey) {
			t.Errorf("detail = %q, want it to name the inverse key", f.Detail)
		}
	})

	t.Run("deleting an accepted document is a violation", func(t *testing.T) {
		dir, repo := testImmutableRepo(t, corpus)
		if err := os.Remove(filepath.Join(dir, "docs", "adr", "0001-serve-images-from-the-application.md")); err != nil {
			t.Fatalf("remove: %v", err)
		}

		f := testAssertSingleFinding(t, testCheckImmutable(t, dir, repo),
			model.RuleImmutableViolation, model.SeverityError, "0001")
		if !strings.Contains(f.Detail, "deleted") {
			t.Errorf("detail = %q, want it to say the document was deleted", f.Detail)
		}
	})

	t.Run("findings name the document the way a caller would type it", func(t *testing.T) {
		dir, repo := testImmutableRepo(t, corpus)
		if err := os.Remove(filepath.Join(dir, "docs", "adr", "0001-serve-images-from-the-application.md")); err != nil {
			t.Fatalf("remove: %v", err)
		}

		got := testCheckImmutable(t, dir, repo)
		want := "docs/adr/0001-serve-images-from-the-application.md"
		if len(got) != 1 || got[0].Location.Path != want {
			t.Fatalf("findings = %+v, want the path %q", got, want)
		}
	})
}
