// Package vcs asks git about a working tree by running it. The append-only
// history check needs the committed shape of a file and nothing more, and the
// field report needs the day each file last changed, so there is no repository
// library here, only the handful of questions those two ask.
package vcs

import (
	"bytes"
	"fmt"
	"os"
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
	scope, err := r.Rel(dir)
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
	scope, err := r.Rel(dir)
	if err != nil {
		return nil, err
	}
	out, err := run(r.root, "ls-files", "--others", "--exclude-standard", "-z", "--", scope)
	if err != nil {
		return nil, fmt.Errorf("list untracked files: %w", err)
	}
	return records(out), nil
}

// Files lists the repository-relative paths under dir that rev holds. It is the
// listing File reads one entry of, so a caller can rebuild what a directory
// looked like at a revision without walking the working tree, which is a
// different set of files.
func (r *Repo) Files(rev, dir string) ([]string, error) {
	scope, err := r.Rel(dir)
	if err != nil {
		return nil, err
	}
	out, err := run(r.root, "ls-tree", "-r", "-z", "--name-only", rev, "--", scope)
	if err != nil {
		return nil, fmt.Errorf("list %s at %s: %w", scope, rev, err)
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

// CommitterDate reports the day a revision was committed on, written as
// YYYY-MM-DD. It is what a check pins its as-of date to: the same commit
// answers the same way however long afterwards it is validated, which a clock
// cannot promise.
func (r *Repo) CommitterDate(rev string) (string, error) {
	out, err := run(r.root, "log", "-1", "--format=%cs", rev)
	if err != nil {
		return "", fmt.Errorf("read the committer date of %s: %w", rev, err)
	}
	day := strings.TrimSpace(string(out))
	if day == "" {
		return "", fmt.Errorf("read the committer date of %s: the log says nothing", rev)
	}
	return day, nil
}

// Tree is the corpus of one revision, written into a temporary directory so the
// documents can be read by the same parser the working tree is read by. The
// caller removes it with Close when it is done reading — a brief quotes
// document bodies from disk after the graph is built, so the files have to
// outlive the parse.
type Tree struct {
	root string
	dirs map[string]string
}

// Checkout materializes the documents under each of dirs as of rev. The keys
// are the caller's names for the directories — a kind each, or the one empty
// key a single-kind corpus has — and the result maps the same keys onto where
// the revision's copies landed, so one configuration can be rerooted onto them.
//
// The layout below the temporary root is the repository's own, so a document
// keeps the path it was committed under and a report about the revision names
// files a reader can find in it.
func (r *Repo) Checkout(rev string, dirs map[string]string) (*Tree, error) {
	root, err := os.MkdirTemp("", "docdag-at-")
	if err != nil {
		return nil, fmt.Errorf("create a working directory for %s: %w", rev, err)
	}
	tree := &Tree{root: root, dirs: make(map[string]string, len(dirs))}
	for name, dir := range dirs {
		rel, err := r.Rel(dir)
		if err != nil {
			_ = tree.Close()
			return nil, err
		}
		files, err := r.Files(rev, dir)
		if err != nil {
			_ = tree.Close()
			return nil, err
		}
		for _, path := range files {
			content, err := r.File(rev, path)
			if err != nil {
				// A path the listing named and the read cannot answer is a file
				// the revision does not hold as a blob — a submodule, say. The
				// corpus is what the revision holds, so it is left out.
				continue
			}
			target := filepath.Join(root, filepath.FromSlash(path))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				_ = tree.Close()
				return nil, fmt.Errorf("write the corpus at %s: %w", rev, err)
			}
			if err := os.WriteFile(target, content, 0o644); err != nil {
				_ = tree.Close()
				return nil, fmt.Errorf("write the corpus at %s: %w", rev, err)
			}
		}
		// A directory the revision holds nothing under is created empty rather
		// than left out: the corpus of that revision has no documents of that
		// kind, which is a different fact from a directory nobody can read.
		dir := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			_ = tree.Close()
			return nil, fmt.Errorf("write the corpus at %s: %w", rev, err)
		}
		tree.dirs[name] = dir
	}
	return tree, nil
}

// Root is the temporary directory the revision's documents live under, which is
// what their paths are named relative to.
func (t *Tree) Root() string { return t.root }

// Dirs maps each directory the caller asked for onto its copy in the tree.
func (t *Tree) Dirs() map[string]string { return t.dirs }

// Close removes the temporary tree.
func (t *Tree) Close() error {
	if t == nil || t.root == "" {
		return nil
	}
	return os.RemoveAll(t.root)
}

// LastChanged reports the day each file under dirs last changed, keyed by the
// path git names it under and written as YYYY-MM-DD. Naming no directory asks
// about the whole working tree.
//
// It is one git invocation for the whole corpus rather than one per document: a
// log walk already knows the paths of every commit it reads, and a subprocess
// per document is a stall a report does not need.
func (r *Repo) LastChanged(dirs ...string) (map[string]string, error) {
	// The NUL before each commit's date is what separates one commit's file
	// list from the next: a path can be spelled like a date, and a bare line
	// cannot say which of the two it is.
	args := []string{"log", "--format=%x00%cs", "--name-only", "--"}
	for _, dir := range dirs {
		scope, err := r.Rel(dir)
		if err != nil {
			return nil, err
		}
		args = append(args, scope)
	}
	out, err := run(r.root, args...)
	if err != nil {
		return nil, fmt.Errorf("read the log: %w", err)
	}
	return parseLastChanged(out), nil
}

// parseLastChanged reads the log walk: one NUL-introduced chunk per commit, its
// date on the first line and the paths it touched on the rest. A merge carries
// no paths and contributes nothing. The largest day per path wins rather than
// the first, because a rebased history can hold a later commit date deeper in
// the walk than the order suggests.
func parseLastChanged(out []byte) map[string]string {
	changed := map[string]string{}
	for _, chunk := range bytes.Split(out, []byte{0}) {
		lines := strings.Split(string(chunk), "\n")
		day := strings.TrimSpace(lines[0])
		if day == "" {
			continue
		}
		for _, path := range lines[1:] {
			if path != "" && day > changed[path] {
				changed[path] = day
			}
		}
	}
	return changed
}

// Rel names a filesystem path the way git does: relative to the working tree
// root and slash-separated. It is the spelling a pathspec takes and the one
// every path git prints comes back in, so a caller matching its own files
// against git's output goes through here.
func (r *Repo) Rel(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", path, err)
	}
	// Both sides go through the same resolution, because a difference in
	// spelling between them is read as distance between them.
	rel, err := filepath.Rel(resolveExisting(r.root), resolveExisting(abs))
	if err != nil {
		return "", fmt.Errorf("%s is outside %s: %w", path, r.root, err)
	}
	return filepath.ToSlash(rel), nil
}

// resolveExisting spells path the way the filesystem does, which is the way git
// spells the working tree root it prints: symlinks followed, and on Windows 8.3
// short names like RUNNER~1 written out as runneradmin.
//
// It resolves the longest ancestor that exists and puts the rest back on,
// because filepath.EvalSymlinks answers nothing at all for a path that is not
// on disk, and a caller may well name one: a kind directory a revision predates
// is asked about before anything has been written under it. Falling back to the
// unresolved spelling there is what made a path inside the working tree — a
// temporary directory under macOS's /var, a symlink to /private/var — come out
// as a ..-escaping pathspec git rejects as outside the repository.
func resolveExisting(path string) string {
	dir, remainder := path, ""
	for {
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(resolved, remainder)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			// Not even the volume root answered, so there is nothing to resolve
			// against and the path stands as it was spelled.
			return path
		}
		remainder = filepath.Join(filepath.Base(dir), remainder)
		dir = parent
	}
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
