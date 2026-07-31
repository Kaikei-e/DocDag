// Package newdoc creates the next document from a template and keeps the
// documents it supersedes consistent.
package newdoc

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"time"
	"unicode"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
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

const frontmatterDelimiter = "---\n"

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
			return "", fmt.Errorf("identifier %q is not a number: %w", id, model.ErrInvalidConfig)
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

// Filename builds the `<id>-<kebab-title>.md` file name.
func Filename(id model.ID, title string) string {
	slug := Kebab(title)
	if slug == "" {
		return id.String() + ".md"
	}
	return id.String() + "-" + slug + ".md"
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
// leaves every other byte, body included, untouched.
func RewriteStatus(src []byte, field, status string) ([]byte, error) {
	text := string(src)
	if !strings.HasPrefix(text, frontmatterDelimiter) {
		return nil, fmt.Errorf("document does not open with a frontmatter block: %w", model.ErrInvalidConfig)
	}
	rest := text[len(frontmatterDelimiter):]
	end := strings.Index(rest, "\n"+frontmatterDelimiter)
	if end < 0 {
		return nil, fmt.Errorf("frontmatter block is not terminated: %w", model.ErrInvalidConfig)
	}

	var block []string
	if end > 0 {
		block = strings.Split(rest[:end], "\n")
	}
	entry := field + ": " + status
	rewritten := false
	for i, line := range block {
		if strings.HasPrefix(line, field+":") {
			block[i] = entry
			rewritten = true
			break
		}
	}
	if !rewritten {
		block = append(block, entry)
	}
	return []byte(frontmatterDelimiter + strings.Join(block, "\n") + rest[end:]), nil
}

// RewriteStatusFile applies RewriteStatus to a file in place.
func RewriteStatusFile(path, field, status string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", path, err)
	}
	src, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	out, err := RewriteStatus(src, field, status)
	if err != nil {
		return fmt.Errorf("rewrite %s: %w", path, err)
	}
	if err := os.WriteFile(path, out, info.Mode().Perm()); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Create writes the new document and rewrites the status of every document it
// supersedes. It returns the path of the created file.
func Create(g *model.Graph, cfg config.Config, req Request) (string, error) {
	superseded, err := documents(g, cfg, req.Supersedes)
	if err != nil {
		return "", err
	}
	if _, err := documents(g, cfg, req.DependsOn); err != nil {
		return "", err
	}
	id, err := NextID(g, cfg)
	if err != nil {
		return "", err
	}
	edges, err := EdgeBlock(cfg, req)
	if err != nil {
		return "", err
	}
	tmpl, err := LoadTemplate(cfg)
	if err != nil {
		return "", err
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
		return "", err
	}

	path := filepath.Join(cfg.Dir, Filename(id, req.Title))
	if err := writeNew(path, doc); err != nil {
		return "", err
	}
	for _, n := range superseded {
		if err := RewriteStatusFile(documentPath(cfg, n), cfg.StatusField, config.StatusSuperseded); err != nil {
			return "", err
		}
	}
	return path, nil
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
