// Package newdoc creates the next document from a template and keeps the
// documents it supersedes consistent.
package newdoc

import (
	"bytes"
	"fmt"
	"io/fs"
	"maps"
	"os"
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

// DateLayout is the frontmatter date format.
const DateLayout = "2006-01-02"

// Request describes the document to create. A zero Date means today.
type Request struct {
	Title      string
	Supersedes []string
	DependsOn  []string
	Date       time.Time
}

// TemplateData is the value applied to the document template. EdgeBlock is a
// preformatted YAML fragment so templates never have to build lists.
type TemplateData struct {
	ID        model.ID
	Title     string
	Status    string
	Date      string
	EdgeBlock string
}

// NextID returns the first free identifier after the highest one in the corpus.
func NextID(g *model.Graph, cfg config.Config) (model.ID, error) {
	highest := 0
	for _, id := range slices.Sorted(maps.Keys(g.Nodes)) {
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

// EdgeBlock renders the requested edge keys as a YAML frontmatter fragment.
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
	var b strings.Builder
	for _, spec := range cfg.Edges {
		refs := requested[model.EdgeType(spec.Name)]
		if len(refs) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s:\n", spec.Key)
		for _, ref := range refs {
			id, ok := normalizer.Normalize(ref)
			if !ok {
				return "", fmt.Errorf("unrecognized reference %q: %w", ref, model.ErrUnknownID)
			}
			fmt.Fprintf(&b, "  - %q\n", id.String())
		}
	}
	return b.String(), nil
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
// apply, all computed without touching the disk.
type Plan struct {
	ID       model.ID
	Path     string
	Content  []byte
	Rewrites []Rewrite
}

// NewPlan computes what creating the requested document takes. Every rewrite
// is computed before anything is written: creating the new document and then
// failing half way through the old ones would leave a corpus nobody asked for.
func NewPlan(g *model.Graph, cfg config.Config, req Request) (Plan, error) {
	superseded, err := documents(g, cfg, req.Supersedes)
	if err != nil {
		return Plan{}, err
	}
	if _, err := documents(g, cfg, req.DependsOn); err != nil {
		return Plan{}, err
	}
	id, err := NextID(g, cfg)
	if err != nil {
		return Plan{}, err
	}
	edges, err := EdgeBlock(cfg, req)
	if err != nil {
		return Plan{}, err
	}
	tmpl, err := LoadTemplate(cfg)
	if err != nil {
		return Plan{}, err
	}
	date := req.Date
	if date.IsZero() {
		date = time.Now()
	}
	doc, err := Render(tmpl, TemplateData{
		ID:        id,
		Title:     req.Title,
		Status:    config.StatusProposed,
		Date:      date.Format(DateLayout),
		EdgeBlock: edges,
	})
	if err != nil {
		return Plan{}, err
	}

	plan := Plan{
		ID:       id,
		Path:     filepath.Join(cfg.Dir, Filename(cfg, id, req.Title)),
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

// Apply writes the planned document and then the planned rewrites, returning
// the path of the created file.
func (p Plan) Apply() (string, error) {
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

// Create writes the new document and rewrites the status of every document it
// supersedes. It returns the path of the created file.
func Create(g *model.Graph, cfg config.Config, req Request) (string, error) {
	plan, err := NewPlan(g, cfg, req)
	if err != nil {
		return "", err
	}
	return plan.Apply()
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
// anything carrying a directory component already locates itself.
func documentPath(cfg config.Config, n *model.Node) string {
	if filepath.IsAbs(n.Path) || filepath.Dir(n.Path) != "." {
		return n.Path
	}
	return filepath.Join(cfg.Dir, n.Path)
}
