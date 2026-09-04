// Package brief assembles the reading a caller needs about one document: where
// it resolves to, its typed-edge neighbourhood, and one section of each of
// those documents, inside a token budget.
package brief

import (
	"fmt"
	"os"
	"slices"
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/internal/graph"
	"github.com/Kaikei-e/DocDag/internal/parse"
	"github.com/Kaikei-e/DocDag/model"
)

// SchemaVersion is the version of the JSON brief. Version 2 heads it with the
// preset revision, as the validation report does.
const SchemaVersion = 2

// Defaults a caller applies when the user asks for nothing in particular.
const (
	DefaultDepth   = 1
	DefaultBudget  = 2000
	DefaultSection = "Decision"
)

// charsPerToken is the tokens-per-character approximation the budget counts in.
// It is deliberately crude: the budget bounds how much prose a reader is handed,
// not what a particular tokenizer would charge for it.
const charsPerToken = 4

// Options parameterise a brief. A Budget of zero or less is unbounded; a Depth
// of zero reports the reference and its resolution alone.
//
// AsOf is the day the brief is about: what is binding, what a reference
// resolves to and which conflicts an exception defeats are all answers about a
// moment wherever a kind declares a period. The zero time means today. At names
// the revision the documents were read from, empty for the working tree; the
// brief carries it so a reader knows which vault they are looking at.
type Options struct {
	Depth   int
	Types   []model.EdgeType
	Budget  int
	Section string
	All     bool
	AsOf    time.Time
	At      string
}

// Entry is one document in a brief. Excerpt is the first paragraph of the
// requested section, empty when the document has no such section or when the
// budget left no room for it. Relation names why a document is in the brief and
// is written only in the related group, where the walk that found it is not the
// answer: a clause and its exception are one hop apart the same way a clause
// and its subject are, and which of the two a reader is looking at matters.
type Entry struct {
	ID       model.ID `json:"id"`
	Title    string   `json:"title"`
	Status   string   `json:"status"`
	Path     string   `json:"path"`
	Relation string   `json:"relation,omitempty"`
	Excerpt  string   `json:"excerpt,omitempty"`
}

// Line is the one-line form of an entry: what a reader is left with when the
// budget has no room for prose.
func (e Entry) Line() string {
	line := fmt.Sprintf("%s  %s  [%s]  %s", e.ID, e.Title, e.Status, e.Path)
	if e.Relation != "" {
		line += "  (" + e.Relation + ")"
	}
	return line
}

// Budget records what a brief was allowed to cost and what it did cost.
type Budget struct {
	Limit    int `json:"limit"`
	Used     int `json:"used"`
	Degraded int `json:"degraded"`
}

// Brief is the assembled context of one document. PresetVersion is the revision
// of the preset the corpus is written against, left out where the configuration
// names none.
// Related and Suppressed are the normative neighbourhood, and are left out
// entirely for a configuration that declares none of what they read.
type Brief struct {
	SchemaVersion int `json:"schema_version"`
	PresetVersion int `json:"preset_version,omitempty"`
	// AsOf is the day the brief was assembled for and At the revision it was
	// read from, so a brief says which corpus at which moment it describes.
	AsOf        string  `json:"as_of,omitempty"`
	At          string  `json:"at,omitempty"`
	Ref         Entry   `json:"ref"`
	ResolvesTo  []Entry `json:"resolves_to"`
	Related     []Entry `json:"related,omitempty"`
	Ancestors   []Entry `json:"ancestors"`
	Descendants []Entry `json:"descendants"`
	// Suppressed is one line per conflict about this document that a recorded
	// exception defeats — the reading that says why a permission and a
	// prohibition are allowed to stand side by side.
	Suppressed []string `json:"suppressed,omitempty"`
	Budget     Budget   `json:"budget"`
}

// entries flattens a brief into the order it is read and the budget is spent in.
func (b *Brief) entries() []*Entry {
	out := []*Entry{&b.Ref}
	for i := range b.ResolvesTo {
		out = append(out, &b.ResolvesTo[i])
	}
	for i := range b.Related {
		out = append(out, &b.Related[i])
	}
	for i := range b.Ancestors {
		out = append(out, &b.Ancestors[i])
	}
	for i := range b.Descendants {
		out = append(out, &b.Descendants[i])
	}
	return out
}

// Build assembles the brief of one document. It reports model.ErrUnknownID for
// a reference the corpus does not hold.
func Build(g *model.Graph, cfg config.Config, id model.ID, opts Options) (*Brief, error) {
	if _, ok := g.Node(id); !ok {
		return nil, fmt.Errorf("context of %s: %w", id, model.ErrUnknownID)
	}
	if opts.Section == "" {
		opts.Section = DefaultSection
	}

	b := &Brief{
		SchemaVersion: SchemaVersion,
		PresetVersion: cfg.PresetVersion,
		AsOf:          graph.AsOfDay(opts.AsOf),
		At:            opts.At,
		ResolvesTo:    []Entry{},
		Ancestors:     []Entry{},
		Descendants:   []Entry{},
	}
	ref, err := entry(g, opts.Section, id)
	if err != nil {
		return nil, err
	}
	b.Ref = ref

	// What is binding is a projection over the whole graph, so it is evaluated
	// once here rather than per candidate document.
	binding := make(map[model.ID]bool)
	for _, current := range graph.BindingSet(g, cfg, opts.AsOf) {
		binding[current] = true
	}

	taken := map[model.ID]bool{id: true}
	if b.ResolvesTo, err = entries(g, opts, taken, binding, resolution(g, cfg, id, opts.AsOf), true); err != nil {
		return nil, err
	}
	// The normative neighbourhood is claimed before the walks, so a document
	// that stands in one of these relations is reported as that rather than as
	// whichever direction happened to reach it first.
	if b.Related, err = relatedEntries(g, cfg, opts, taken, binding, id); err != nil {
		return nil, err
	}
	b.Suppressed = suppressed(g, cfg, id, opts.AsOf)
	ancestors := within(g, graph.Reverse(g, opts.Types...), id, opts.Depth)
	if b.Ancestors, err = entries(g, opts, taken, binding, ancestors, false); err != nil {
		return nil, err
	}
	descendants := within(g, graph.Adjacency(g, opts.Types...), id, opts.Depth)
	if b.Descendants, err = entries(g, opts, taken, binding, descendants, false); err != nil {
		return nil, err
	}

	applyBudget(b, opts.Budget)
	return b, nil
}

// resolution names the documents that currently stand in for a superseded
// reference.
func resolution(g *model.Graph, cfg config.Config, id model.ID, asOf time.Time) []model.ID {
	if _, ok := cfg.Edge(config.EdgeSupersedes); !ok {
		return nil
	}
	resolved, err := graph.ResolveAt(g, cfg, id, config.EdgeSupersedes, asOf)
	if err != nil {
		// A supersedes cycle is a validation finding; a brief still reports the
		// document the caller asked about.
		return nil
	}
	return resolved
}

// within returns the documents reachable over adj in at most depth hops, sorted
// and excluding the starting document.
func within(g *model.Graph, adj map[model.ID][]model.ID, id model.ID, depth int) []model.ID {
	seen := map[model.ID]bool{id: true}
	found := []model.ID{}
	frontier := []model.ID{id}
	for hop := 0; hop < depth && len(frontier) > 0; hop++ {
		next := []model.ID{}
		for _, current := range frontier {
			for _, neighbor := range adj[current] {
				if seen[neighbor] {
					continue
				}
				seen[neighbor] = true
				if _, known := g.Node(neighbor); !known {
					continue
				}
				next = append(next, neighbor)
				found = append(found, neighbor)
			}
		}
		frontier = next
	}
	slices.Sort(found)
	return found
}

// entries builds the entries of one group, skipping the documents an earlier
// group already reported. Binding documents alone are kept unless the caller
// asked for all of them; the resolution is always kept.
func entries(g *model.Graph, opts Options, taken, binding map[model.ID]bool, ids []model.ID, always bool) ([]Entry, error) {
	out := []Entry{}
	for _, id := range ids {
		if taken[id] {
			continue
		}
		if !always && !opts.All && !binding[id] {
			continue
		}
		taken[id] = true
		e, err := entry(g, opts.Section, id)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}

func entry(g *model.Graph, section string, id model.ID) (Entry, error) {
	n := g.Nodes[id]
	src, err := os.ReadFile(n.Path)
	if err != nil {
		return Entry{}, fmt.Errorf("read document %s: %w", n.Path, err)
	}
	_, body, _ := parse.SplitFrontmatter(src)
	return Entry{ID: n.ID, Title: n.Title, Status: n.Status, Path: n.Path, Excerpt: Section(string(body), section)}, nil
}

// applyBudget charges every one-line entry, then buys excerpts in reading order
// while they fit. An excerpt is dropped whole: a half sentence is worse than a
// one-line entry, and once one entry degrades every later one does too.
func applyBudget(b *Brief, limit int) {
	all := b.entries()
	used := 0
	for _, e := range all {
		used += tokens(e.Line())
	}
	degraded := 0
	spent := false
	for _, e := range all {
		if e.Excerpt == "" {
			continue
		}
		cost := tokens(e.Excerpt)
		if limit > 0 && (spent || used+cost > limit) {
			spent = true
			e.Excerpt = ""
			degraded++
			continue
		}
		used += cost
	}
	b.Budget = Budget{Limit: limit, Used: used, Degraded: degraded}
}

func tokens(s string) int {
	return (len(s) + charsPerToken - 1) / charsPerToken
}
