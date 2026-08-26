package graph

import (
	"maps"
	"os"
	"path/filepath"
	"strings"

	"github.com/Kaikei-e/DocDag/internal/model"
)

// Touching keeps the findings a set of files is about: filed against one of
// them, related to one of them, or filed against a document one of them borders
// over a typed edge. A path naming a directory stands for everything under it.
func Touching(findings []model.Finding, g *model.Graph, paths []string) []model.Finding {
	touched := newPathSet(paths)
	near := neighbourhood(g, touched)
	out := make([]model.Finding, 0, len(findings))
	for _, f := range findings {
		if touches(f, touched, near) {
			out = append(out, f)
		}
	}
	return out
}

func touches(f model.Finding, touched pathSet, near map[model.ID]bool) bool {
	if near[f.ID] || touched.has(f.Location.Path) {
		return true
	}
	for _, related := range f.Related {
		if touched.has(related.Path) {
			return true
		}
	}
	return false
}

// neighbourhood is the named documents plus everything one typed edge away:
// changing a document is what puts its neighbours out of step, so their
// findings belong to the same question.
func neighbourhood(g *model.Graph, touched pathSet) map[model.ID]bool {
	named := make(map[model.ID]bool)
	for id, n := range g.Nodes {
		if touched.has(n.Path) {
			named[id] = true
		}
	}
	near := maps.Clone(named)
	for _, e := range g.Edges {
		if named[e.From] {
			near[e.To] = true
		}
		if named[e.To] {
			near[e.From] = true
		}
	}
	return near
}

// pathSet answers "is this file one of the ones the caller named", comparing
// absolute paths so a report and a command line can spell the same file
// differently.
type pathSet struct {
	files map[string]bool
	dirs  []string
}

func newPathSet(paths []string) pathSet {
	set := pathSet{files: make(map[string]bool, len(paths))}
	for _, path := range paths {
		abs, _ := filepath.Abs(path)
		if info, err := os.Stat(abs); err == nil && info.IsDir() {
			set.dirs = append(set.dirs, abs+string(filepath.Separator))
			continue
		}
		set.files[abs] = true
	}
	return set
}

func (s pathSet) has(path string) bool {
	if path == "" {
		return false
	}
	abs, _ := filepath.Abs(path)
	if s.files[abs] {
		return true
	}
	for _, dir := range s.dirs {
		if strings.HasPrefix(abs, dir) {
			return true
		}
	}
	return false
}
