// Package newdoc creates the next document from a template and keeps the
// documents it supersedes consistent.
package newdoc

import (
	"time"

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
	return "", model.ErrNotImplemented
}

// Kebab converts a title into a lowercase, hyphen-separated slug.
func Kebab(title string) string { return "" }

// Filename builds the `<id>-<kebab-title>.md` file name.
func Filename(id model.ID, title string) string { return "" }

// LoadTemplate reads the configured template file, falling back to
// DefaultTemplate when none is configured.
func LoadTemplate(cfg config.Config) (string, error) { return "", model.ErrNotImplemented }

// EdgeBlock renders the requested edge keys as a YAML frontmatter fragment.
func EdgeBlock(cfg config.Config, req Request) (string, error) {
	return "", model.ErrNotImplemented
}

// Render applies the template to the document data.
func Render(tmpl string, data TemplateData) ([]byte, error) { return nil, model.ErrNotImplemented }

// RewriteStatus replaces only the status value in a document's frontmatter and
// leaves every other byte, body included, untouched.
func RewriteStatus(src []byte, field, status string) ([]byte, error) {
	return nil, model.ErrNotImplemented
}

// RewriteStatusFile applies RewriteStatus to a file in place.
func RewriteStatusFile(path, field, status string) error { return model.ErrNotImplemented }

// Create writes the new document and rewrites the status of every document it
// supersedes. It returns the path of the created file.
func Create(g *model.Graph, cfg config.Config, req Request) (string, error) {
	return "", model.ErrNotImplemented
}
