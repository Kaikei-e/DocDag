// Package newdoc creates the next document from a template and keeps the
// documents it supersedes consistent.
package newdoc

import (
	"bytes"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

// DefaultTemplate is the built-in minimal MADR document template.
const DefaultTemplate = `---
title: {{ .Title }}
status: {{ .Status }}
date: {{ .Date }}
{{ .EdgeBlock }}---

# {{ .Title }}

## Context and Problem Statement

## Decision Drivers

## Considered Options

## Decision Outcome
`

// KindTemplate is the built-in template for a document of a declared kind.
// What differs from kind to kind is the frontmatter the kind's own
// declarations produce — the identifier where the kind declares a pattern, the
// first word of its status vocabulary, the edges it may declare — so one
// template renders all of them rather than seven written into the binary. The
// body is a heading and nothing else: what belongs under it is the corpus's
// business, and `template:` replaces the whole file for a corpus with an
// opinion about it.
const KindTemplate = `---
{{ .IdentityBlock }}kind: {{ .Kind }}
title: {{ .Title }}
{{ .StatusBlock }}date: {{ .Date }}
{{ .FieldBlock }}{{ .EdgeBlock }}---

# {{ .Title }}
`

// DateLayout is the frontmatter date format.
const DateLayout = "2006-01-02"

// markdownExt is the extension a document file carries.
const markdownExt = ".md"

// Request describes the document to create. A zero Date means today, and an
// empty ID takes the next free identifier. Kind names the document kind to
// create, and is empty on a single-kind corpus — which is every corpus that
// declares no kinds at all.
type Request struct {
	ID         string
	Kind       string
	Title      string
	Supersedes []string
	DependsOn  []string
	Date       time.Time
}

// TemplateData is the value applied to the document template. Every field
// named Block is a preformatted YAML fragment, possibly empty, so templates
// never have to build lists or decide whether a key belongs in the block at
// all; the rest are plain values.
type TemplateData struct {
	ID            model.ID
	Kind          string
	Title         string
	Status        string
	Date          string
	IdentityBlock string
	StatusBlock   string
	FieldBlock    string
	EdgeBlock     string
}

// NextID returns the first free identifier after the highest one in the corpus.
func NextID(g *model.Graph, cfg config.Config) (model.ID, error) {
	return nextID(g, cfg, "")
}

// NextKindID returns the first free identifier after the highest one the named
// kind holds. Only that kind's documents are counted: another kind's
// identifiers are not numbers at all, and where they are they are a sequence
// of their own.
func NextKindID(g *model.Graph, cfg config.Config, kind string) (model.ID, error) {
	return nextID(g, cfg, kind)
}

// nextID counts up from the highest identifier held, over the whole corpus
// when kind is empty — which is what a single-kind corpus holds anyway.
func nextID(g *model.Graph, cfg config.Config, kind string) (model.ID, error) {
	highest := 0
	for _, id := range slices.Sorted(maps.Keys(g.Nodes)) {
		if kind != "" && g.Nodes[id].Kind != kind {
			continue
		}
		n, err := strconv.Atoi(id.String())
		if err != nil {
			return "", fmt.Errorf("identifier %q is not a number: %w", id, model.ErrInvalidDocument)
		}
		highest = max(highest, n)
	}
	return model.ID(fmt.Sprintf("%0*d", cfg.IDWidth, highest+1)), nil
}

// Kebab converts a title into a lowercase, hyphen-separated slug.
func Kebab(title string) string {
	var b strings.Builder
	separated := false
	for _, r := range strings.ToLower(title) {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			separated = true
			continue
		}
		if separated && b.Len() > 0 {
			b.WriteByte('-')
		}
		separated = false
		b.WriteRune(r)
	}
	return b.String()
}

// separatedSlug matches the slug placeholder together with the separator that
// joins it to the identifier, so a title yielding no slug leaves no orphan
// separator behind.
var separatedSlug = regexp.MustCompile(`[-_]?\{slug\}[-_]?`)

// Filename builds a document name from the configured name template.
func Filename(cfg config.Config, id model.ID, title string) string {
	name := cfg.FilenameTemplate()
	slug := Kebab(title)
	if slug == "" {
		name = separatedSlug.ReplaceAllString(name, "")
	}
	name = strings.ReplaceAll(name, "{slug}", slug)
	return strings.ReplaceAll(name, "{id}", id.String())
}

// KindFilename names the file a new document is written to. Where its kind
// declares an identifier pattern the identifier names the file: a declared
// pattern is the canonical spelling, and a file name whose stem the pattern
// accepts is what the reader turns back into identity, so a slug beside it
// would leave the name and the identity disagreeing. An identifier carrying a
// slash — `conform/uz-v-001` — is one no file name can hold, so its last
// segment names the file and the frontmatter `id:` carries the whole of it. A
// kind that declares no pattern keeps the digit-run identity, and with it the
// configured `filename:` template, which is also what an empty kind — a
// single-kind corpus — gets.
func KindFilename(cfg config.Config, kind string, id model.ID, title string) string {
	spec, declared := cfg.Kind(kind)
	if !declared || spec.ID == "" {
		return Filename(cfg, id, title)
	}
	return path.Base(id.String()) + markdownExt
}

// LoadTemplate reads the configured template file, falling back to
// DefaultTemplate when none is configured.
func LoadTemplate(cfg config.Config) (string, error) {
	if cfg.Template == "" {
		return DefaultTemplate, nil
	}
	src, err := os.ReadFile(cfg.Template)
	if err != nil {
		return "", fmt.Errorf("read template %s: %w", cfg.Template, err)
	}
	return string(src), nil
}

// LoadKindTemplate reads the configured template file, falling back to the
// built-in template a document of this kind is written from — and, where the
// request names no kind, to the single-kind one every corpus had before kinds
// existed. A configured `template:` wins for both: a corpus that writes its
// own template has said what a new document looks like.
func LoadKindTemplate(cfg config.Config, kind string) (string, error) {
	if cfg.Template != "" || kind == "" {
		return LoadTemplate(cfg)
	}
	return KindTemplate, nil
}

// identityBlock writes the identifier into the frontmatter of a document whose
// kind declares an identifier pattern. It is written whatever the file name
// ends up being: the identifier is what people and agents quote, and for a
// kind whose pattern no file name can hold it is the only place identity can
// live. A digit-run kind writes none, exactly as a single-kind corpus does —
// there the file name is the identifier.
func identityBlock(cfg config.Config, kind string, id model.ID) string {
	if spec, declared := cfg.Kind(kind); !declared || spec.ID == "" {
		return ""
	}
	return fmt.Sprintf("%s: %s\n", config.KeyID, id)
}

// statusBlock writes the status a new document opens at: the first word of the
// vocabulary its kind answers to. A kind that answers to none — a
// machine-written conformance test or measure — gets no status key rather than
// an invented value, which is what "this kind has no status" looks like in a
// document.
func statusBlock(cfg config.Config, kind, status string) string {
	if status == "" {
		return ""
	}
	return fmt.Sprintf("%s: %s\n", cfg.EffectiveStatus(), status)
}

// FieldBlock renders a commented stub for every field the kind declares that a
// document has to answer for: one it must write, or one whose value comes from
// a closed vocabulary. The stub names the vocabulary, so an author writing the
// key by hand does not have to go and read the configuration for the words.
//
// Like an edge stub it is a comment rather than a key: a required key present
// and holding a placeholder would be an unknown_field_value, which is a worse
// first validation than the missing_field the author is about to answer. A
// field the corpus is retiring gets no stub — nothing new should write it.
func FieldBlock(cfg config.Config, kind string) string {
	var b strings.Builder
	fields := cfg.FieldSpecs(kind)
	for _, name := range slices.Sorted(maps.Keys(fields)) {
		spec := fields[name]
		if spec.Deprecated || (!spec.Required && len(spec.OneOf) == 0) {
			continue
		}
		placeholder := "<value>"
		if len(spec.OneOf) > 0 {
			placeholder = "<" + strings.Join(spec.OneOf, "|") + ">"
		}
		fmt.Fprintf(&b, "# %s: %s\n", name, placeholder)
	}
	return b.String()
}

// openingStatus is the status a new document is created with: the first word
// of its kind's vocabulary, and `proposed` on a single-kind corpus, which is
// what every document created before kinds existed opened at.
func openingStatus(cfg config.Config, kind string) string {
	if kind == "" {
		return config.StatusProposed
	}
	vocabulary := cfg.KindStatusValues(kind)
	if len(vocabulary) == 0 {
		return ""
	}
	return vocabulary[0]
}

// edgeEnds reports, for the document that writes an edge key down, the kinds
// its own end may have and the kinds the reference may name. A reverse edge's
// key names the edge's source, so for it the two swap.
func edgeEnds(spec config.EdgeSpec) (own, target []string) {
	if spec.Direction == config.DirectionReverse {
		return spec.To, spec.From
	}
	return spec.From, spec.To
}

// declarable reports whether a document of this kind may write an edge key at
// all: the edge constrains its writing end to a set of kinds this one is in,
// or it constrains it not at all. A request that names no kind declares
// nothing this way — a single-kind corpus writes exactly the edges it asked
// for, as it always has.
func declarable(spec config.EdgeSpec, kind string) bool {
	if kind == "" {
		return false
	}
	own, _ := edgeEnds(spec)
	return len(own) == 0 || slices.Contains(own, kind)
}

// EdgeBlock renders the requested edge keys as a YAML frontmatter fragment,
// followed — for a document of a declared kind — by a commented stub for every
// other edge that kind may declare. A stub is a comment rather than an empty
// key because a key present and naming nothing is the empty_edge finding: the
// point is to show an author what the configuration offers, not to hand them a
// document whose first validation fails.
func EdgeBlock(cfg config.Config, req Request) (string, error) {
	requested := map[model.EdgeType][]string{
		config.EdgeSupersedes: req.Supersedes,
		config.EdgeDependsOn:  req.DependsOn,
	}
	declared := make(map[model.EdgeType]bool, len(cfg.Edges))
	for _, spec := range cfg.Edges {
		declared[model.EdgeType(spec.Name)] = true
	}
	for _, name := range []model.EdgeType{config.EdgeSupersedes, config.EdgeDependsOn} {
		if len(requested[name]) > 0 && !declared[name] {
			return "", fmt.Errorf("edge %q is not declared by the configuration: %w", name, model.ErrInvalidConfig)
		}
	}

	normalizer := cfg.Normalizer()
	var refs, stubs strings.Builder
	for _, spec := range cfg.Edges {
		entries := requested[model.EdgeType(spec.Name)]
		if len(entries) == 0 {
			if declarable(spec, req.Kind) {
				writeStub(&stubs, spec)
			}
			continue
		}
		if err := writeRefs(&refs, spec, entries, normalizer); err != nil {
			return "", err
		}
	}
	return refs.String() + stubs.String(), nil
}

// writeRefs writes one edge key and the references requested under it.
func writeRefs(b *strings.Builder, spec config.EdgeSpec, refs []string, normalizer config.IDNormalizer) error {
	// An entry under an edge that requires an attribute is incomplete without
	// it, and a creation has nothing to put there: refusing says so, where
	// writing the entry anyway would hand back a document whose first
	// validation is an edge_attr_missing error.
	for _, name := range spec.AttrNames() {
		if spec.Attrs[name].Required {
			return fmt.Errorf("edge %q requires the attribute %q on every entry, which a created document has no value for: %w",
				spec.Name, name, model.ErrInvalidConfig)
		}
	}
	fmt.Fprintf(b, "%s:\n", spec.Key)
	for _, ref := range refs {
		id, ok := normalizer.Normalize(ref)
		if !ok {
			return fmt.Errorf("unrecognized reference %q: %w", ref, model.ErrUnknownID)
		}
		fmt.Fprintf(b, "  - %q\n", id.String())
	}
	return nil
}

// writeStub writes one edge the kind may declare, commented out, with a
// placeholder entry naming what the edge reaches and the attributes it
// requires.
func writeStub(b *strings.Builder, spec config.EdgeSpec) {
	fmt.Fprintf(b, "# %s:\n#   - %s\n", spec.Key, stubEntry(spec))
}

// stubEntry describes one entry of an edge: the kinds it may name, and, where
// the edge requires attributes, the mapping form that carries them.
func stubEntry(spec config.EdgeSpec) string {
	_, target := edgeEnds(spec)
	placeholder := "<ref>"
	if len(target) > 0 {
		placeholder = "<" + strings.Join(target, "|") + ">"
	}
	pairs := []string{config.EdgeRefKey + ": " + placeholder}
	for _, name := range spec.AttrNames() {
		attr := spec.Attrs[name]
		if attr.Required {
			pairs = append(pairs, name+": <"+attrPlaceholder(attr)+">")
		}
	}
	if len(pairs) == 1 {
		return placeholder
	}
	return "{" + strings.Join(pairs, ", ") + "}"
}

// attrPlaceholder describes what one required attribute takes, as the word a
// stub stands in place of a value with.
func attrPlaceholder(attr config.EdgeAttrSpec) string {
	if len(attr.OneOf) > 0 {
		return strings.Join(attr.OneOf, "|")
	}
	switch attr.ValueType() {
	case config.AttrTypeNumber:
		return "number"
	case config.AttrTypeDate:
		return "YYYY-MM-DD"
	}
	return "string"
}

// Render applies the template to the document data.
func Render(tmpl string, data TemplateData) ([]byte, error) {
	parsed, err := template.New("document").Parse(tmpl)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := parsed.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("apply template: %w", err)
	}
	return buf.Bytes(), nil
}

// RewriteStatus replaces only the status value in a document's frontmatter and
// leaves every other byte, body included, untouched. It reads the block with
// the engine's own parser, so what `new` rewrites is exactly what `validate`
// manages, line endings included.
func RewriteStatus(src []byte, field, status string) ([]byte, error) {
	start, end, ok := parse.FrontmatterSpan(src)
	if !ok {
		return nil, fmt.Errorf("document carries no terminated frontmatter block: %w", model.ErrInvalidDocument)
	}

	entry := []byte(field + ": " + status)
	out := make([]byte, 0, len(src)+len(entry)+2)
	out = append(out, src[:start]...)
	block := src[start:end]
	rewritten := false
	for offset := 0; offset < len(block); {
		next := len(block)
		if cut := bytes.IndexByte(block[offset:], '\n'); cut >= 0 {
			next = offset + cut + 1
		}
		content, ending := splitLineEnding(block[offset:next])
		if !rewritten && bytes.HasPrefix(content, []byte(field+":")) {
			out = append(out, entry...)
			out = append(out, ending...)
			rewritten = true
		} else {
			out = append(out, block[offset:next]...)
		}
		offset = next
	}
	if !rewritten {
		out = append(out, entry...)
		out = append(out, blockLineEnding(src[:start], block)...)
	}
	return append(out, src[end:]...), nil
}

// splitLineEnding separates a line from the ending it was written with, so a
// rewritten line keeps the file's own line ending.
func splitLineEnding(line []byte) (content, ending []byte) {
	switch {
	case bytes.HasSuffix(line, []byte("\r\n")):
		return line[:len(line)-2], line[len(line)-2:]
	case bytes.HasSuffix(line, []byte("\n")):
		return line[:len(line)-1], line[len(line)-1:]
	}
	return line, nil
}

// blockLineEnding reports the line ending an appended entry should carry: the
// block's own, or the opening delimiter's when the block is empty.
func blockLineEnding(opening, block []byte) []byte {
	source := block
	if len(source) == 0 {
		source = opening
	}
	if bytes.HasSuffix(source, []byte("\r\n")) {
		return []byte("\r\n")
	}
	return []byte("\n")
}

// Rewrite is a status change that has been computed but not yet written.
type Rewrite struct {
	Path    string
	Status  string
	Content []byte
	Mode    fs.FileMode
}

// planRewrite reads a document and computes its rewritten form without
// touching the file.
func planRewrite(path, field, status string) (Rewrite, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Rewrite{}, fmt.Errorf("stat %s: %w", path, err)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return Rewrite{}, fmt.Errorf("read %s: %w", path, err)
	}
	content, err := RewriteStatus(src, field, status)
	if err != nil {
		return Rewrite{}, fmt.Errorf("rewrite %s: %w", path, err)
	}
	return Rewrite{Path: path, Status: status, Content: content, Mode: info.Mode().Perm()}, nil
}

func (r Rewrite) apply() error {
	if err := os.WriteFile(r.Path, r.Content, r.Mode); err != nil {
		return fmt.Errorf("write %s: %w", r.Path, err)
	}
	return nil
}

// RewriteStatusFile applies RewriteStatus to a file in place.
func RewriteStatusFile(path, field, status string) error {
	planned, err := planRewrite(path, field, status)
	if err != nil {
		return err
	}
	return planned.apply()
}

// Plan is the document Create would write and the status rewrites it would
// apply, all computed without touching the disk. An Exists plan names a
// document the corpus already holds under the requested identifier and title:
// there is nothing left to write.
type Plan struct {
	ID       model.ID
	Path     string
	Content  []byte
	Rewrites []Rewrite
	Exists   bool
}

// NewPlan computes what creating the requested document takes. Every rewrite
// is computed before anything is written: creating the new document and then
// failing half way through the old ones would leave a corpus nobody asked for.
func NewPlan(g *model.Graph, cfg config.Config, req Request) (Plan, error) {
	if err := noCollision(g); err != nil {
		return Plan{}, err
	}
	superseded, err := documents(g, cfg, req.Supersedes)
	if err != nil {
		return Plan{}, err
	}
	if _, err := documents(g, cfg, req.DependsOn); err != nil {
		return Plan{}, err
	}
	id, err := identifier(g, cfg, req)
	if err != nil {
		return Plan{}, err
	}
	if n, ok := g.Node(id); ok {
		if !sameTitle(n.Title, req.Title) {
			return Plan{}, fmt.Errorf("document %s is titled %q, not %q: %w", id, n.Title, req.Title, model.ErrIDConflict)
		}
		return Plan{ID: id, Path: documentPath(cfg, n), Exists: true}, nil
	}
	edges, err := EdgeBlock(cfg, req)
	if err != nil {
		return Plan{}, err
	}
	tmpl, err := LoadKindTemplate(cfg, req.Kind)
	if err != nil {
		return Plan{}, err
	}
	date := req.Date
	if date.IsZero() {
		date = time.Now()
	}
	status := openingStatus(cfg, req.Kind)
	doc, err := Render(tmpl, TemplateData{
		ID:            id,
		Kind:          req.Kind,
		Title:         req.Title,
		Status:        status,
		Date:          date.Format(DateLayout),
		IdentityBlock: identityBlock(cfg, req.Kind, id),
		StatusBlock:   statusBlock(cfg, req.Kind, status),
		FieldBlock:    FieldBlock(cfg, req.Kind),
		EdgeBlock:     edges,
	})
	if err != nil {
		return Plan{}, err
	}

	plan := Plan{
		ID:       id,
		Path:     filepath.Join(documentsDir(cfg, req.Kind), KindFilename(cfg, req.Kind, id, req.Title)),
		Content:  doc,
		Rewrites: make([]Rewrite, 0, len(superseded)),
	}
	for _, n := range superseded {
		planned, err := planRewrite(documentPath(cfg, n), cfg.StatusField, config.StatusSuperseded)
		if err != nil {
			return Plan{}, err
		}
		plan.Rewrites = append(plan.Rewrites, planned)
	}
	return plan, nil
}

// noCollision refuses a corpus in which two files claim one identifier: the
// next identifier is not knowable, and neither is which file a reference names.
func noCollision(g *model.Graph) error {
	for _, f := range g.Findings {
		if f.Rule == model.RuleIDCollision {
			return fmt.Errorf("document %s %s: %w", f.ID, f.Detail, model.ErrIDConflict)
		}
	}
	return nil
}

// identifier resolves the identifier a request asks for, or the next free one
// when it asks for none and the identity rules can count one up.
func identifier(g *model.Graph, cfg config.Config, req Request) (model.ID, error) {
	if req.Kind == "" {
		if req.ID == "" {
			return NextID(g, cfg)
		}
		id, ok := cfg.Normalizer().Normalize(req.ID)
		if !ok {
			return "", fmt.Errorf("unrecognized reference %q: %w", req.ID, model.ErrUnknownID)
		}
		return id, nil
	}
	spec, declared := cfg.Kind(req.Kind)
	if !declared {
		return "", fmt.Errorf("no kind %q is declared: %w", req.Kind, model.ErrUnknownID)
	}
	if req.ID == "" {
		// Counting up is a property of digit-run identity alone. A declared
		// pattern is a spelling, not a sequence: what follows UZ-V-006 is a
		// decision, so the caller has to have made it.
		if spec.ID != "" {
			return "", fmt.Errorf("kind %q reads identifiers as %s, which nothing counts up: name the identifier: %w",
				req.Kind, spec.ID, model.ErrUnknownID)
		}
		return NextKindID(g, cfg, req.Kind)
	}
	id, ok := cfg.KindNormalizer(req.Kind).Normalize(req.ID)
	switch {
	case ok:
		return id, nil
	case spec.ID == "":
		return "", fmt.Errorf("unrecognized reference %q: %w", req.ID, model.ErrUnknownID)
	}
	return "", fmt.Errorf("%q is not an identifier of kind %q, which reads %s: %w",
		req.ID, req.Kind, spec.ID, model.ErrUnknownID)
}

func sameTitle(a, b string) bool { return strings.TrimSpace(a) == strings.TrimSpace(b) }

// Apply writes the planned document and then the planned rewrites, returning
// the path of the created file.
func (p Plan) Apply() (string, error) {
	if p.Exists {
		return p.Path, nil
	}
	if err := writeNew(p.Path, p.Content); err != nil {
		return "", err
	}
	for _, planned := range p.Rewrites {
		if err := planned.apply(); err != nil {
			return "", err
		}
	}
	return p.Path, nil
}

// writeNew refuses to touch an existing document: creating a decision must
// never overwrite one.
func writeNew(path string, content []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	return nil
}

func documents(g *model.Graph, cfg config.Config, refs []string) ([]*model.Node, error) {
	normalizer := cfg.Normalizer()
	out := make([]*model.Node, 0, len(refs))
	for _, ref := range refs {
		id, ok := normalizer.Normalize(ref)
		if !ok {
			return nil, fmt.Errorf("unrecognized reference %q: %w", ref, model.ErrUnknownID)
		}
		n, ok := g.Nodes[id]
		if !ok {
			return nil, fmt.Errorf("unknown document %s: %w", id, model.ErrUnknownID)
		}
		out = append(out, n)
	}
	return out, nil
}

// documentPath reads a bare file name as relative to the documents directory;
// anything carrying a directory component already locates itself, which every
// document of a multi-kind corpus does — it was read out of its kind's own
// directory.
func documentPath(cfg config.Config, n *model.Node) string {
	if filepath.IsAbs(n.Path) || filepath.Dir(n.Path) != "." {
		return n.Path
	}
	return filepath.Join(documentsDir(cfg, n.Kind), n.Path)
}

// documentsDir names the directory a document of one kind lives in: the kind's
// own, or the single documents directory of a corpus that declares no kinds.
func documentsDir(cfg config.Config, kind string) string {
	if spec, declared := cfg.Kind(kind); declared {
		return spec.Dir
	}
	return cfg.Dir
}
