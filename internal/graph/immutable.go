package graph

import (
	"fmt"
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
	"github.com/Kaikei-e/DocDag/internal/vcs"
)

// immutableStatuses are the statuses that close a document to editing: what a
// team has agreed to is a record, and a record is appended to, not rewritten.
var immutableStatuses = []string{config.StatusAccepted, config.StatusSuperseded, config.StatusWithdrawn}

// CheckImmutable reports documents that were closed at rev and have since
// changed in a way the append-only policy forbids. Paths are reported relative
// to root. A new document is always allowed.
func CheckImmutable(repo *vcs.Repo, cfg config.Config, rev, root string) ([]model.Finding, error) {
	base := repo.MergeBase(rev)
	changes, err := repo.Changes(base, cfg.Dir)
	if err != nil {
		return nil, err
	}
	untracked, err := repo.Untracked(cfg.Dir)
	if err != nil {
		return nil, err
	}
	normalizer := cfg.Normalizer()

	findings := []model.Finding{}
	for _, change := range changes {
		name := filepath.Base(change.Path)
		if change.Status == 'A' || slices.Contains(untracked, change.Path) || !normalizer.MatchesFilename(name) {
			continue
		}
		id, ok := normalizer.Normalize(name)
		if !ok {
			continue
		}
		was, err := repo.File(base, change.Path)
		if err != nil {
			continue
		}
		committed, ok := closedDocument(cfg, was)
		if !ok {
			continue
		}
		findings = append(findings, compareToCommitted(cfg, repo, change, id, committed, root)...)
	}
	SortFindings(findings)
	return findings, nil
}

// closedDocument decodes a committed file and reports it when its status closed
// it to editing.
func closedDocument(cfg config.Config, src []byte) (*parse.Document, bool) {
	frontmatter, body, ok := parse.SplitFrontmatter(src)
	if !ok {
		return nil, false
	}
	fm, err := parse.UnmarshalFrontmatter(frontmatter)
	if err != nil {
		return nil, false
	}
	raw, ok := parse.Attr(fm, statusField(cfg))
	if !ok {
		return nil, false
	}
	status, _ := canonicalStatus(cfg, raw)
	if !slices.ContainsFunc(immutableStatuses, func(closed string) bool { return strings.EqualFold(status, closed) }) {
		return nil, false
	}
	return &parse.Document{Frontmatter: fm, Body: string(body)}, true
}

func compareToCommitted(cfg config.Config, repo *vcs.Repo, change vcs.Change, id model.ID, was *parse.Document, root string) []model.Finding {
	path := reportedPath(root, repo.Root(), change.Path)
	if change.Status == 'D' {
		return []model.Finding{immutableViolation(id, path, firstFileLine, "the document was deleted")}
	}
	now, err := parse.File(filepath.Join(repo.Root(), filepath.FromSlash(change.Path)), cfg)
	if err != nil || now.Err != nil {
		return []model.Finding{immutableViolation(id, path, firstFileLine, "the document no longer parses")}
	}

	findings := []model.Finding{}
	for _, detail := range frontmatterChanges(cfg, was.Frontmatter, now.Frontmatter) {
		findings = append(findings, immutableViolation(id, path,
			model.Locate(path, now.FrontmatterLine, now.KeyLines, detail.key).Line, detail.text))
	}
	if line, changed := bodyDivergence(was.Body, now.Body); changed {
		findings = append(findings, immutableViolation(id, path, now.BodyLine+line,
			fmt.Sprintf("the body changed at line %d, which append-only history forbids", now.BodyLine+line)))
	}
	return findings
}

// change is one forbidden frontmatter difference and the key it belongs to.
type change struct {
	key  string
	text string
}

// frontmatterChanges lists the frontmatter differences the policy forbids. The
// status value may move freely, and an inverse key may gain entries: a later
// decision has to be able to record that it replaced this one.
func frontmatterChanges(cfg config.Config, was, now map[string]any) []change {
	inverse := inverseKeys(cfg)
	keys := make(map[string]bool, len(was)+len(now))
	for key := range was {
		keys[key] = true
	}
	for key := range now {
		keys[key] = true
	}

	changes := []change{}
	for _, key := range slices.Sorted(maps.Keys(keys)) {
		if key == statusField(cfg) {
			continue
		}
		before, hadBefore := was[key]
		after, hasAfter := now[key]
		if inverse[key] {
			if missing := droppedRefs(was, now, key); missing != "" {
				changes = append(changes, change{key: key, text: fmt.Sprintf("%s no longer lists %s", key, missing)})
			}
			continue
		}
		switch {
		case hadBefore && !hasAfter:
			changes = append(changes, change{key: key, text: fmt.Sprintf("frontmatter key %q was removed", key)})
		case !hadBefore && hasAfter:
			changes = append(changes, change{key: key, text: fmt.Sprintf("frontmatter key %q was added", key)})
		case !reflect.DeepEqual(before, after):
			changes = append(changes, change{key: key, text: fmt.Sprintf("frontmatter key %q changed", key)})
		}
	}
	return changes
}

func inverseKeys(cfg config.Config) map[string]bool {
	keys := make(map[string]bool, len(cfg.Edges))
	for _, spec := range cfg.Edges {
		if spec.Inverse != "" {
			keys[spec.Inverse] = true
		}
	}
	return keys
}

// droppedRefs names the entries a key held before and does not hold now.
func droppedRefs(was, now map[string]any, key string) string {
	before, _ := parse.Refs(was, key)
	after, _ := parse.Refs(now, key)
	missing := make([]string, 0, len(before))
	for _, ref := range before {
		if !slices.Contains(after, ref) {
			missing = append(missing, ref)
		}
	}
	return strings.Join(missing, ", ")
}

// bodyDivergence reports the zero-based body line where the new body stops
// continuing the committed one. Trailing whitespace is not content, so a body
// that only gained lines at the end continues it.
func bodyDivergence(was, now string) (int, bool) {
	trimmed := strings.TrimRight(was, " \t\r\n")
	// A document that carried no body is continued by every body, and splitting
	// the empty string would claim one empty line of content it never had.
	if trimmed == "" {
		return 0, false
	}
	before := strings.Split(trimmed, "\n")
	after := strings.Split(now, "\n")
	for i, line := range before {
		if i >= len(after) {
			return len(after) - 1, true
		}
		if strings.TrimRight(line, "\r") != strings.TrimRight(after[i], "\r") {
			return i, true
		}
	}
	return 0, false
}

func immutableViolation(id model.ID, path string, line int, detail string) model.Finding {
	return model.Finding{
		Severity: model.SeverityError,
		Rule:     model.RuleImmutableViolation,
		ID:       id,
		Detail:   detail,
		Location: model.Location{Path: path, Line: line},
	}
}

// reportedPath names a repository-relative path the way a caller standing in
// root would type it.
func reportedPath(root, repoRoot, path string) string {
	// git resolves symlinks and short names in the root it reports; the
	// caller's directory must be resolved the same way before they compare.
	abs := filepath.Join(resolved(repoRoot), filepath.FromSlash(path))
	rel, err := filepath.Rel(resolved(root), abs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(abs)
	}
	return filepath.ToSlash(rel)
}

func resolved(dir string) string {
	if r, err := filepath.EvalSymlinks(dir); err == nil {
		return r
	}
	return dir
}
