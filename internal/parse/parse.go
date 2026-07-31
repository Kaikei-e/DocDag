// Package parse turns Markdown files with YAML frontmatter into documents: the
// frontmatter split, a strict YAML decode, body links and derived edges.
package parse

import (
	"github.com/goccy/go-yaml"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// Delimiter opens and closes a frontmatter block.
const Delimiter = "---"

// Document is one Markdown file after parsing and before graph construction.
type Document struct {
	Path           string
	Name           string
	ID             model.ID
	Frontmatter    map[string]any
	Body           string
	HasFrontmatter bool
	MatchesPattern bool
	Err            error
}

// DecodeOptions returns the goccy/go-yaml options used for frontmatter blocks.
// Unknown keys stay allowed: other repositories carry extra frontmatter fields.
func DecodeOptions() []yaml.DecodeOption { return nil }

// SplitFrontmatter separates a leading delimited YAML block from the body. ok
// is false when the file does not open with a frontmatter delimiter.
func SplitFrontmatter(src []byte) (frontmatter, body []byte, ok bool) { return nil, nil, false }

// UnmarshalFrontmatter decodes a frontmatter block with a strict YAML parser.
func UnmarshalFrontmatter(src []byte) (map[string]any, error) {
	return nil, model.ErrNotImplemented
}

// File parses one Markdown file. A frontmatter decode failure is recorded on
// the returned document rather than returned, so later checks still run.
func File(path string, cfg config.Config) (*Document, error) {
	return nil, model.ErrNotImplemented
}

// Dir parses the Markdown files directly in dir that carry frontmatter or are
// named like a managed document. A file whose name holds no identifier is not a
// managed document, whatever its frontmatter says.
func Dir(dir string, cfg config.Config) ([]*Document, error) {
	return nil, model.ErrNotImplemented
}

// Attr reads a scalar frontmatter value as a string.
func Attr(fm map[string]any, key string) (string, bool) { return "", false }

// Refs reads a list-valued frontmatter key as raw, un-normalized references. A
// scalar value is accepted as a single-element list.
func Refs(fm map[string]any, key string) []string { return nil }
