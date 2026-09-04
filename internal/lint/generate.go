package lint

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
	"github.com/Kaikei-e/DocDag/internal/newdoc"
)

// fixtureIDBase is where generated identifiers start on a corpus that counts
// them, high enough that a skeleton never collides with a document somebody
// wrote.
const fixtureIDBase = 9000

// Skeleton is the fixture `docdag new --fixture` writes: the documents of the
// corpus where the rule has to fire and of the one where it must not. Dir is
// the fixture's own directory, which the two corpora sit under.
type Skeleton struct {
	Rule  string
	Dir   string
	Files []SkeletonFile
}

// SkeletonFile is one generated document.
type SkeletonFile struct {
	Path    string
	Content []byte
}

// Apply writes the skeleton, leaving alone every file that already exists: a
// second run fills in what a first one could not, and never overwrites a
// fixture somebody has since written by hand. It reports the paths it wrote.
func (s Skeleton) Apply() ([]string, error) {
	written := make([]string, 0, len(s.Files))
	for _, file := range s.Files {
		if _, err := os.Stat(file.Path); err == nil {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(file.Path), 0o755); err != nil {
			return written, fmt.Errorf("create %s: %w", filepath.Dir(file.Path), err)
		}
		if err := os.WriteFile(file.Path, file.Content, 0o644); err != nil {
			return written, fmt.Errorf("write %s: %w", file.Path, err)
		}
		written = append(written, file.Path)
	}
	return written, nil
}

// Generate derives a rule's fixture from the rule itself: the documents that
// satisfy one alternative of its condition, and the same documents with one
// literal of that alternative broken. Identifiers, kinds and directories come
// from the configuration, so what it writes is a corpus the corpus's own rules
// would accept.
func Generate(cfg config.Config, name, root, fixtures string, today time.Time) (Skeleton, error) {
	a := analyzer{cfg: cfg}
	u, ok := named(a.units(), SectionRules, name)
	if !ok {
		return Skeleton{}, fmt.Errorf("no rule %q is configured: %w", name, model.ErrInvalidConfig)
	}
	chosen := conjunct{}
	found := false
	for _, c := range u.dnf {
		if _, unsat := a.unsatisfiable(c, u.scope); unsat {
			continue
		}
		chosen, found = c, true
		break
	}
	if !found {
		return Skeleton{}, fmt.Errorf("rule %q has no alternative a document could satisfy: %w", name, model.ErrInvalidConfig)
	}

	skeleton := Skeleton{Rule: name, Dir: filepath.Join(fixtures, name)}
	for _, side := range []string{FixtureFires, FixtureSilent} {
		b := &builder{cfg: cfg, analyzer: a, rule: name, today: today}
		if side == FixtureSilent {
			b.broken = breakable(chosen)
		}
		b.build(chosen, u)
		for _, doc := range b.docs {
			skeleton.Files = append(skeleton.Files, SkeletonFile{
				Path:    filepath.Join(skeleton.Dir, side, kindDir(cfg, root, doc.kind), doc.filename(cfg)),
				Content: doc.render(cfg, b.today),
			})
		}
	}
	// A generated fixture is only a fixture if it does what a fixture has to do,
	// and a condition resting on a threshold or a one-hop clause is satisfiable
	// in more ways than a generator should choose between. Where the skeleton
	// cannot be confirmed, it is written anyway and says what is left to do:
	// the layout is the tedious part, and the judgement is the author's.
	if err := verify(cfg, root, skeleton); err != nil {
		skeleton.annotate(err)
	}
	return skeleton, nil
}

// annotate adds a note to every generated document saying the skeleton does not
// yet fire the way a fixture must.
func (s *Skeleton) annotate(err error) {
	note := fmt.Sprintf("\n<!-- TODO: %v. Adjust these documents until docdag lint --fixtures passes. -->\n", err)
	for i := range s.Files {
		s.Files[i].Content = append(s.Files[i].Content, []byte(note)...)
	}
}

// kindDir names the directory inside a fixture that a document of one kind
// lives in: the kind's own path relative to the corpus root, and the fixture
// directory itself on a corpus that declares no kinds.
func kindDir(cfg config.Config, root, kind string) string {
	spec, declared := cfg.Kind(kind)
	if !declared {
		return ""
	}
	return kindRelative(root, spec.Dir)
}

// builder assembles one fixture corpus: the document the rule is about and
// whatever companions its edges need.
type builder struct {
	cfg      config.Config
	analyzer analyzer
	rule     string
	today    time.Time
	broken   *literal
	docs     []*fixtureDoc
	counter  int
	// peers indexes the companion documents by the clause that created them, so
	// a one-hop clause describes the neighbour a degree clause already made
	// rather than a second one beside it.
	peers map[string][]*fixtureDoc
	// conditions is the rule's own conditions, which is where a one-hop clause's
	// attributes are read from: a literal carries their canonical rendering, and
	// the clause itself carries the operands.
	conditions []config.Condition
}

// fixtureDoc is one generated document, as the frontmatter it will carry.
type fixtureDoc struct {
	kind  string
	id    model.ID
	title string
	attrs map[string]string
	lists map[string][]string
	todo  []string
}

// build writes the documents one alternative of a condition calls for. A broken
// literal is the one the `ok` corpus violates — the whole difference between
// the two sides of a fixture.
func (b *builder) build(c conjunct, u unit) {
	b.peers = map[string][]*fixtureDoc{}
	b.conditions = u.conditions
	subject := b.newDoc(b.subjectKind(c, u), "Fixture for "+b.rule)
	for _, l := range c.literals {
		b.apply(subject, l)
	}
	if b.broken != nil {
		b.violate(subject, *b.broken)
	}
	b.complete()
}

// subjectKind picks the kind the document a rule is about is written as. Where
// the condition pins one, that is the answer; where it does not, the generator
// guesses from what the condition reads — a field only one kind declares, a
// status only some kinds answer to — because a document has to live in some
// kind's directory before any rule can be run against it.
//
// The guess is the generator's alone. The analysis narrows kinds only where the
// configuration makes the narrowing certain, and a document of an open kind may
// write any key at all.
func (b *builder) subjectKind(c conjunct, u unit) string {
	if !b.cfg.Multikind() {
		return ""
	}
	possible, _, narrowed := b.analyzer.kinds(c, u.scope)
	if narrowed && len(possible) == 1 {
		return possible[0]
	}
	if len(possible) == 0 {
		possible = b.cfg.KindNames()
	}
	for _, l := range c.literals {
		if !isAttr(l.kind) || l.negate {
			continue
		}
		narrowedTo := intersect(possible, b.kindsReading(l))
		if len(narrowedTo) > 0 {
			possible = narrowedTo
		}
	}
	if len(possible) == 0 {
		return ""
	}
	return possible[0]
}

// kindsReading names the kinds whose documents could carry one attribute clause
// as it is written: the kinds that declare the field, and for the status field
// the kinds whose vocabulary holds the value.
func (b *builder) kindsReading(l literal) []string {
	names := b.cfg.KindNames()
	if l.key == b.cfg.EffectiveStatus() && l.kind == litEq {
		answering := []string{}
		for _, name := range names {
			if containsFold(b.cfg.KindStatusValues(name), l.value) {
				answering = append(answering, name)
			}
		}
		return answering
	}
	if _, top := b.cfg.Fields[l.key]; top {
		return names
	}
	declaring := []string{}
	for _, name := range names {
		if _, ok := b.cfg.Kinds[name].Fields[l.key]; ok {
			declaring = append(declaring, name)
		}
	}
	return declaring
}

// intersect returns the names both lists hold, in the first one's order.
func intersect(first, second []string) []string {
	both := make([]string, 0, len(first))
	for _, name := range first {
		if slices.Contains(second, name) {
			both = append(both, name)
		}
	}
	return both
}

// apply writes what one literal asks of the subject document, unless it is the
// literal this side of the fixture is built to violate.
func (b *builder) apply(doc *fixtureDoc, l literal) {
	if b.suppressed(l) {
		return
	}
	// A document's kind is the directory it lives in, so writing the key again
	// would be one declaration twice — and a duplicate key does not decode.
	if l.key == config.KeyKind && isAttr(l.kind) {
		return
	}
	switch l.kind {
	case litEq, litContains:
		b.write(doc, l.key, l.value)
	case litNot:
		if value, ok := b.otherValue(doc.kind, l.key, []string{l.value}); ok {
			b.write(doc, l.key, value)
		}
	case litDegree:
		b.edges(doc, l, max(l.min, 1))
	case litVia:
		b.neighbour(doc, l)
	case litAbsent:
		// Nothing to write: the document satisfies it by not declaring the edge.
	default:
		doc.todo = append(doc.todo, "satisfy "+l.String())
	}
}

// suppressed reports whether a literal is left out of this side of the fixture:
// the one being broken, and every other clause over the same edge as a broken
// edge clause. A rule asking for an edge and for something about what it
// reaches is broken by leaving the edge out, and a one-hop clause that quietly
// put it back would make the `ok` corpus fire.
func (b *builder) suppressed(l literal) bool {
	if b.broken == nil {
		return false
	}
	if *b.broken == l {
		return true
	}
	if b.broken.kind != litDegree && b.broken.kind != litVia {
		return false
	}
	return (l.kind == litDegree || l.kind == litVia) && l.key == b.broken.key && l.inbound == b.broken.inbound
}

// violate breaks one literal, which is what makes the `ok` corpus the true
// negative it has to be. An attribute is broken by a value from its own
// vocabulary that no alternative of the rule names, an edge by leaving it out,
// and an absent edge by declaring one.
func (b *builder) violate(doc *fixtureDoc, l literal) {
	switch l.kind {
	case litEq, litContains:
		if value, ok := b.otherValue(doc.kind, l.key, b.named(l.key)); ok {
			b.write(doc, l.key, value)
			return
		}
		doc.todo = append(doc.todo, "make sure "+l.String()+" does not hold")
	case litNot:
		b.write(doc, l.key, l.value)
	case litAbsent:
		b.edges(doc, literal{kind: litDegree, key: l.key, inbound: l.inbound, min: 1}, 1)
	case litDegree, litVia:
		// Left out of the document entirely, which is what apply already did.
	default:
		doc.todo = append(doc.todo, "make sure "+l.String()+" does not hold")
	}
}

// breakable names the literal the `ok` corpus violates: the first one, in the
// conjunction's own order, that a generated document can be made to fail. A
// conjunction of nothing but thresholds and one-hop clauses has none, and the
// skeleton says so rather than writing a document that quietly fires.
func breakable(c conjunct) *literal {
	for _, l := range c.literals {
		switch l.kind {
		case litEq, litContains, litNot, litDegree, litAbsent:
			return &l
		}
	}
	if len(c.literals) == 0 {
		return nil
	}
	return &c.literals[0]
}

// named lists every value the rule's alternatives pin one key to, so breaking a
// literal does not accidentally satisfy another alternative — a rule written as
// "MUST or MUST_NOT" is not broken by writing MUST_NOT.
func (b *builder) named(key string) []string {
	values := []string{}
	u, ok := named(b.analyzer.units(), SectionRules, b.rule)
	if !ok {
		return values
	}
	for _, c := range u.dnf {
		for _, l := range c.literals {
			if l.key == key && (l.kind == litEq || l.kind == litContains) {
				values = append(values, l.value)
			}
		}
	}
	return values
}

// otherValue returns a value of a key's vocabulary that is none of the excluded
// ones, and reports whether the vocabulary offers one.
func (b *builder) otherValue(kind, key string, excluded []string) (string, bool) {
	values, closed := b.analyzer.domain(key, kind)
	if !closed {
		return "", false
	}
	for _, value := range values {
		if !containsFold(excluded, value) {
			return value, true
		}
	}
	return "", false
}

// write records one scalar frontmatter value. An edge key is not written this
// way: its entries are references, which link records.
func (b *builder) write(doc *fixtureDoc, key, value string) {
	doc.attrs[key] = value
}

// edges gives a document the edges one degree clause asks for, writing each of
// them into whichever end declares it and creating the companion documents they
// point at.
func (b *builder) edges(doc *fixtureDoc, l literal, count int) {
	spec, declared := b.cfg.Edge(model.EdgeType(l.key))
	if !declared {
		doc.todo = append(doc.todo, "declare edge "+l.key)
		return
	}
	// The companion is at the far end of the clause: the tail of an inbound
	// edge, the head of an outbound one.
	far := spec.To
	if l.inbound {
		far = spec.From
	}
	kind := ""
	if len(far) > 0 {
		kind = far[0]
	}
	for i := 0; i < count; i++ {
		peer := b.newDoc(kind, fmt.Sprintf("Fixture %s for %s", l.key, b.rule))
		from, to := doc, peer
		if l.inbound {
			from, to = peer, doc
		}
		b.link(spec, from, to)
		b.peers[peerKey(l)] = append(b.peers[peerKey(l)], peer)
	}
}

// peerKey names the companions one edge clause reaches, so a one-hop clause
// over the same edge finds the neighbour a degree clause already created.
func peerKey(l literal) string {
	if l.inbound {
		return "^" + l.key
	}
	return l.key
}

// neighbour makes one one-hop clause hold: the neighbour it needs is the one an
// edge clause already created where there is one, and a new companion where
// there is not, and it is written with the attributes the clause asks of it.
func (b *builder) neighbour(doc *fixtureDoc, l literal) {
	existing := b.peers[peerKey(l)]
	if len(existing) == 0 {
		b.edges(doc, literal{kind: litDegree, key: l.key, inbound: l.inbound, min: 1}, 1)
		existing = b.peers[peerKey(l)]
	}
	if len(existing) == 0 {
		doc.todo = append(doc.todo, "give the "+l.key+" neighbour "+l.value)
		return
	}
	peer := existing[0]
	for key, want := range b.viaAttrs(l) {
		if key == config.KeyKind {
			continue
		}
		switch {
		case want.Eq != nil:
			b.write(peer, key, *want.Eq)
		case want.Contains != nil:
			b.write(peer, key, *want.Contains)
		case want.Not != nil:
			if value, ok := b.otherValue(peer.kind, key, []string{*want.Not}); ok {
				b.write(peer, key, value)
				continue
			}
			peer.todo = append(peer.todo, fmt.Sprintf("write a %s that is not %s", key, *want.Not))
		default:
			peer.todo = append(peer.todo, "answer the "+l.key+" clause: "+l.value)
		}
	}
}

// viaAttrs reads back the attribute conditions one one-hop literal stands for.
// The literal carries their canonical rendering, which is what identifies the
// clause among the rule's own conditions.
func (b *builder) viaAttrs(l literal) map[string]config.AttrCondition {
	for _, cond := range b.conditions {
		for _, clause := range cond.ViaClauses() {
			if clause.Edge == l.key && clause.Inbound == l.inbound && renderAttrs(clause.Attr) == l.value {
				return clause.Attr
			}
		}
	}
	return nil
}

// link writes one edge into the document that declares it, which is its source
// unless the edge reads its key in reverse.
func (b *builder) link(spec config.EdgeSpec, from, to *fixtureDoc) {
	holder, named := from, to
	if spec.Direction == config.DirectionReverse {
		holder, named = to, from
	}
	holder.lists[spec.Key] = append(holder.lists[spec.Key], b.entry(spec, named.id))
}

// entry renders one edge entry: the bare reference, or the mapping form where
// the edge requires attributes, with a value each attribute accepts.
func (b *builder) entry(spec config.EdgeSpec, target model.ID) string {
	pairs := []string{}
	for _, name := range spec.AttrNames() {
		attr := spec.Attrs[name]
		if !attr.Required {
			continue
		}
		pairs = append(pairs, fmt.Sprintf("%s: %s", name, attrValue(attr, b.today)))
	}
	if len(pairs) == 0 {
		return target.String()
	}
	return fmt.Sprintf("{%s: %s, %s}", config.EdgeRefKey, target, strings.Join(pairs, ", "))
}

// attrValue is a value one edge attribute accepts: the first word of its
// vocabulary, a number, or today.
func attrValue(attr config.EdgeAttrSpec, today time.Time) string {
	if len(attr.OneOf) > 0 {
		return attr.OneOf[0]
	}
	switch attr.ValueType() {
	case config.AttrTypeNumber:
		return "1"
	case config.AttrTypeDate:
		return today.Format(newdoc.DateLayout)
	}
	return "fixture"
}

// complete fills in what every document of its kind has to carry whatever the
// rule asked for: a status, and the fields the kind requires.
func (b *builder) complete() {
	for _, generated := range b.docs {
		field := b.cfg.EffectiveStatus()
		if _, written := generated.attrs[field]; !written {
			if vocabulary := b.cfg.KindStatusValues(generated.kind); len(vocabulary) > 0 {
				generated.attrs[field] = vocabulary[0]
			}
		}
		for name, spec := range b.cfg.FieldSpecs(generated.kind) {
			if _, written := generated.attrs[name]; written || !spec.Required || spec.Deprecated {
				continue
			}
			if len(spec.OneOf) > 0 {
				generated.attrs[name] = spec.OneOf[0]
				continue
			}
			generated.attrs[name] = "fixture"
		}
	}
}

// newDoc adds one document to the corpus under construction, under an
// identifier its kind accepts.
func (b *builder) newDoc(kind, title string) *fixtureDoc {
	doc := &fixtureDoc{
		kind:  kind,
		id:    b.identifier(kind, b.counter),
		title: title,
		attrs: map[string]string{},
		lists: map[string][]string{},
	}
	b.counter++
	b.docs = append(b.docs, doc)
	return doc
}

// identifier invents an identifier the kind's rules accept: a number a
// digit-run corpus counts in, and a sample of the pattern a kind declares one.
func (b *builder) identifier(kind string, index int) model.ID {
	spec, declared := b.cfg.Kind(kind)
	if !declared || spec.ID == "" {
		width := max(b.cfg.IDWidth, 1)
		return model.ID(fmt.Sprintf("%0*d", width, fixtureIDBase+index))
	}
	if sample, ok := sampleID(spec.ID, index); ok {
		return model.ID(sample)
	}
	return model.ID(fmt.Sprintf("TODO-%d", index))
}

// filename names the file a generated document is written to, under the same
// rules `docdag new` writes one under.
func (d fixtureDoc) filename(cfg config.Config) string {
	return newdoc.KindFilename(cfg, d.kind, d.id, d.title)
}

// render writes the document: the identity its kind calls for, the frontmatter
// the literals produced, and a body saying what the fixture is for and what an
// author still has to do.
func (d fixtureDoc) render(cfg config.Config, today time.Time) []byte {
	var b strings.Builder
	b.WriteString("---\n")
	if spec, declared := cfg.Kind(d.kind); declared && spec.ID != "" {
		fmt.Fprintf(&b, "%s: %s\n", config.KeyID, d.id)
	}
	if d.kind != "" {
		fmt.Fprintf(&b, "%s: %s\n", config.KeyKind, d.kind)
	}
	fmt.Fprintf(&b, "%s: %s\n", config.KeyTitle, d.title)
	for _, key := range slices.Sorted(maps.Keys(d.attrs)) {
		fmt.Fprintf(&b, "%s: %s\n", key, d.attrs[key])
	}
	fmt.Fprintf(&b, "%s: %s\n", config.KeyDate, today.Format(newdoc.DateLayout))
	for _, key := range slices.Sorted(maps.Keys(d.lists)) {
		fmt.Fprintf(&b, "%s:\n", key)
		for _, entry := range d.lists[key] {
			fmt.Fprintf(&b, "  - %s\n", entry)
		}
	}
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s\n", d.title)
	for _, todo := range d.todo {
		fmt.Fprintf(&b, "\n<!-- TODO: %s -->\n", todo)
	}
	return []byte(b.String())
}

// verify reports whether a generated skeleton does what a fixture has to do:
// fire in one corpus and stay silent in the other. It reads the files back the
// way lint reads them, so what it answers is what `docdag lint --fixtures` will.
func verify(cfg config.Config, root string, skeleton Skeleton) error {
	sandbox, err := os.MkdirTemp("", "docdag-fixture-")
	if err != nil {
		return fmt.Errorf("verify the fixture: %w", err)
	}
	defer func() { _ = os.RemoveAll(sandbox) }()
	base := skeleton.Dir
	for _, file := range skeleton.Files {
		rel, err := filepath.Rel(base, file.Path)
		if err != nil {
			return fmt.Errorf("verify the fixture: %w", err)
		}
		target := filepath.Join(sandbox, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("verify the fixture: %w", err)
		}
		if err := os.WriteFile(target, file.Content, 0o644); err != nil {
			return fmt.Errorf("verify the fixture: %w", err)
		}
	}
	for _, side := range []string{FixtureFires, FixtureSilent} {
		fired, err := firings(Options{Config: cfg, Root: root}, filepath.Join(sandbox, side), skeleton.Rule)
		if err != nil {
			return err
		}
		if side == FixtureFires && len(fired) == 0 {
			return fmt.Errorf("the generated %s corpus does not fire %s", side, skeleton.Rule)
		}
		if side == FixtureSilent && len(fired) > 0 {
			return fmt.Errorf("the generated %s corpus fires %s", side, skeleton.Rule)
		}
	}
	return nil
}
