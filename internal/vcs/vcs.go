// Package vcs asks git about a working tree by running it. The append-only
// history check needs the committed shape of a file and nothing more, so there
// is no repository library here, only the four questions that check asks.
package vcs

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo is one git working tree.
type Repo struct {
	root string
}

// Open reports the working tree dir belongs to. A directory outside a
// repository, or a machine without git, is an error: the caller asked for a
// history check that cannot be answered.
func Open(dir string) (*Repo, error) {
	out, err := run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("%s is not inside a git repository: %w", dir, err)
	}
	return &Repo{root: strings.TrimSpace(string(out))}, nil
}

// Root is the working tree's top-level directory.
func (r *Repo) Root() string { return r.root }

// MergeBase returns the commit where rev and HEAD diverged, or rev unchanged
// when git cannot resolve one: a caller naming a bare commit still gets the
// comparison it asked for.
func (r *Repo) MergeBase(rev string) string {
	out, err := run(r.root, "merge-base", rev, "HEAD")
	if err != nil {
		return rev
	}
	return strings.TrimSpace(string(out))
}

// Change is one path the working tree changed relative to a commit. Status is
// git's letter: M modified, D deleted, A added.
type Change struct {
	Status byte
	Path   string
}

// Changes lists the working-tree changes under dir relative to base. Renames
// are not detected: a moved document is a deletion and an addition, and the
// deletion is what a caller asking about a closed record needs to hear.
func (r *Repo) Changes(base, dir string) ([]Change, error) {
	scope, err := r.scope(dir)
	if err != nil {
		return nil, err
	}
	out, err := run(r.root, "diff", "--name-status", "--no-renames", "-z", base, "--", scope)
	if err != nil {
		return nil, fmt.Errorf("diff %s: %w", base, err)
	}
	return parseNameStatus(out), nil
}

// Untracked lists the files under dir that git does not track.
func (r *Repo) Untracked(dir string) ([]string, error) {
	scope, err := r.scope(dir)
	if err != nil {
		return nil, err
	}
	out, err := run(r.root, "ls-files", "--others", "--exclude-standard", "-z", "--", scope)
	if err != nil {
		return nil, fmt.Errorf("list untracked files: %w", err)
	}
	return records(out), nil
}

// File returns the contents of a repository-relative path as of rev.
func (r *Repo) File(rev, path string) ([]byte, error) {
	out, err := run(r.root, "show", rev+":"+path)
	if err != nil {
		return nil, fmt.Errorf("read %s at %s: %w", path, rev, err)
	}
	return out, nil
}

// scope turns a filesystem directory into the repository-relative pathspec git
// wants.
func (r *Repo) scope(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", dir, err)
	}
	root, err := filepath.EvalSymlinks(r.root)
	if err != nil {
		root = r.root
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return "", fmt.Errorf("%s is outside %s: %w", dir, r.root, err)
	}
	return filepath.ToSlash(rel), nil
}

// parseNameStatus reads git's NUL-separated name-status stream, in which each
// change is a status letter followed by the path it applies to.
func parseNameStatus(out []byte) []Change {
	fields := records(out)
	changes := make([]Change, 0, len(fields)/2)
	for i := 0; i+1 < len(fields); i += 2 {
		changes = append(changes, Change{Status: fields[i][0], Path: fields[i+1]})
	}
	return changes
}

func records(out []byte) []string {
	fields := make([]string, 0, 8)
	for _, field := range bytes.Split(out, []byte{0}) {
		if len(field) > 0 {
			fields = append(fields, string(field))
		}
	}
	return fields
}

func run(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if message := strings.TrimSpace(stderr.String()); message != "" {
			return nil, fmt.Errorf("%s: %v", message, err)
		}
		return nil, err
	}
	return out, nil
}
