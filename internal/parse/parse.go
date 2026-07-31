// Package parse turns Markdown files with YAML frontmatter into documents: the
// frontmatter split, a strict YAML decode, body links and derived edges.
package parse

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// Delimiter opens and closes a frontmatter block.
const Delimiter = "---"

const markdownExt = ".md"

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
// The parser already rejects duplicate keys, so strictness needs no option.
func DecodeOptions() []yaml.DecodeOption { return nil }

// byteOrderMark is what a Windows editor may write in front of the opening
// delimiter. It belongs to neither the frontmatter block nor the body.
var byteOrderMark = []byte{0xEF, 0xBB, 0xBF}

// FrontmatterSpan reports the byte range of the frontmatter block inside src:
// start is the first byte after the opening delimiter line, end the first byte
// of the closing delimiter line. ok is false when src carries no terminated
// block. Both CRLF and LF line endings delimit a block, so a document authored
// on Windows is managed like any other.
func FrontmatterSpan(src []byte) (start, end int, ok bool) {
	offset := 0
	if bytes.HasPrefix(src, byteOrderMark) {
		offset = len(byteOrderMark)
	}
	opening, next := readLine(src[offset:])
	if !isDelimiter(opening) {
		return 0, 0, false
	}
	start = offset + next
	for pos := start; pos < len(src); {
		line, advance := readLine(src[pos:])
		if isDelimiter(line) {
			return start, pos, true
		}
		pos += advance
	}
	return 0, 0, false
}

// SplitFrontmatter separates a leading delimited YAML block from the body. ok
// is false when the file does not open with a frontmatter delimiter.
func SplitFrontmatter(src []byte) (frontmatter, body []byte, ok bool) {
	start, end, ok := FrontmatterSpan(src)
	if !ok {
		return nil, src, false
	}
	_, next := readLine(src[end:])
	return src[start:end], src[end+next:], true
}

// readLine splits the first line off src, returning it without its line ending
// and the offset the next line starts at.
func readLine(src []byte) (line []byte, next int) {
	if end := bytes.IndexByte(src, '\n'); end >= 0 {
		return bytes.TrimSuffix(src[:end], []byte("\r")), end + 1
	}
	return src, len(src)
}

func isDelimiter(line []byte) bool { return string(line) == Delimiter }

// UnmarshalFrontmatter decodes a frontmatter block with a strict YAML parser.
func UnmarshalFrontmatter(src []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(src)) == 0 {
		return nil, nil
	}
	var fm map[string]any
	if err := yaml.UnmarshalWithOptions(src, &fm, DecodeOptions()...); err != nil {
		return nil, fmt.Errorf("decode frontmatter: %w", err)
	}
	return fm, nil
}

// File parses one Markdown file. A frontmatter decode failure is recorded on
// the returned document rather than returned, so later checks still run.
func File(path string, cfg config.Config) (*Document, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read document %s: %w", path, err)
	}
	norm := cfg.Normalizer()
	name := filepath.Base(path)
	doc := &Document{Path: path, Name: name, MatchesPattern: norm.MatchesFilename(name)}
	if id, ok := norm.Normalize(name); ok {
		doc.ID = id
	}

	frontmatter, body, ok := SplitFrontmatter(src)
	doc.Body = string(body)
	if !ok {
		return doc, nil
	}
	doc.HasFrontmatter = true
	fm, err := UnmarshalFrontmatter(frontmatter)
	if err != nil {
		doc.Err = err
		return doc, nil
	}
	doc.Frontmatter = fm
	return doc, nil
}

// Dir parses the Markdown files directly in dir whose name matches the preset
// filename pattern. The name carries the identity, so a file named anything
// else is not a managed document, whatever its frontmatter says.
func Dir(dir string, cfg config.Config) ([]*Document, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read documents directory %s: %w", dir, err)
	}
	docs := make([]*Document, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != markdownExt {
			continue
		}
		doc, err := File(filepath.Join(dir, entry.Name()), cfg)
		if err != nil {
			return nil, err
		}
		if !doc.MatchesPattern || doc.ID == "" {
			continue
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// Attr reads a scalar frontmatter value as a string.
func Attr(fm map[string]any, key string) (string, bool) {
	value, ok := fm[key]
	if !ok {
		return "", false
	}
	return scalar(value)
}

// Refs reads a list-valued frontmatter key as raw, un-normalized references. A
// scalar value is accepted as a single-element list. invalid holds the entries
// that are not scalars at all, rendered as written: an unquoted wikilink
// decodes as a nested sequence, and dropping it silently would hide the very
// link the tool exists to find.
func Refs(fm map[string]any, key string) (refs, invalid []string) {
	value, ok := fm[key]
	if !ok || value == nil {
		return nil, nil
	}
	items, isList := value.([]any)
	if !isList {
		items = []any{value}
	}
	refs = make([]string, 0, len(items))
	for _, item := range items {
		ref, ok := scalar(item)
		if !ok {
			invalid = append(invalid, fmt.Sprint(item))
			continue
		}
		if ref = strings.TrimSpace(ref); ref == "" {
			continue
		}
		refs = append(refs, ref)
	}
	return refs, invalid
}

// scalar renders a decoded YAML scalar as the string it was written as. A
// zero-padded reference decodes as a number, so numbers must stringify.
func scalar(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case bool:
		return strconv.FormatBool(v), true
	case int:
		return strconv.Itoa(v), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case uint64:
		return strconv.FormatUint(v, 10), true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	default:
		return "", false
	}
}
