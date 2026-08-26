// Package parse turns Markdown files with YAML frontmatter into documents: the
// frontmatter split, a strict YAML decode, body links and derived edges.
package parse

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
)

// Delimiter opens and closes a frontmatter block.
const Delimiter = "---"

const markdownExt = ".md"

// Document is one Markdown file after parsing and before graph construction.
// FrontmatterLine and KeyLines are 1-based file lines, zero when unknown, so a
// finding can name the exact key it is about.
type Document struct {
	Path            string
	Name            string
	ID              model.ID
	Frontmatter     map[string]any
	Body            string
	HasFrontmatter  bool
	MatchesPattern  bool
	FrontmatterLine int
	KeyLines        map[string]int
	Err             error
}

// FrontmatterError is a frontmatter decode failure with the position of the
// offending token. UnmarshalFrontmatter reports it relative to the first line
// of the block; File offsets it onto the file.
type FrontmatterError struct {
	Message string
	Line    int
	Column  int
}

func (e *FrontmatterError) Error() string {
	return fmt.Sprintf("decode frontmatter at line %d, column %d: %s", e.Line, e.Column, e.Message)
}

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
// Unknown keys stay allowed: other repositories carry extra frontmatter fields.
// The parser already rejects duplicate keys, so strictness needs no option.
func UnmarshalFrontmatter(src []byte) (map[string]any, error) {
	if len(bytes.TrimSpace(src)) == 0 {
		return nil, nil
	}
	var fm map[string]any
	if err := yaml.Unmarshal(src, &fm); err != nil {
		return nil, frontmatterError(err)
	}
	return fm, nil
}

// frontmatterError lifts goccy's position and message out of a decode failure,
// leaving the multi-line source excerpt behind: a finding occupies one line and
// carries its position in fields.
func frontmatterError(err error) *FrontmatterError {
	var located yaml.Error
	if !errors.As(err, &located) {
		return &FrontmatterError{Message: err.Error()}
	}
	fe := &FrontmatterError{Message: located.GetMessage()}
	if tk := located.GetToken(); tk != nil && tk.Position != nil {
		fe.Line, fe.Column = tk.Position.Line, tk.Position.Column
	}
	return fe
}

// KeyLines reports the line of every top-level key in a frontmatter block,
// 1-based and relative to the first line of the block. A block that does not
// parse has no keys; the decode failure is reported separately.
func KeyLines(src []byte) map[string]int {
	file, err := parser.ParseBytes(src, 0)
	if err != nil {
		return nil
	}
	lines := make(map[string]int)
	for _, doc := range file.Docs {
		switch body := doc.Body.(type) {
		case *ast.MappingNode:
			for _, value := range body.Values {
				recordKeyLine(lines, value)
			}
		case *ast.MappingValueNode:
			recordKeyLine(lines, body)
		}
	}
	return lines
}

func recordKeyLine(lines map[string]int, value *ast.MappingValueNode) {
	if value == nil || value.Key == nil {
		return
	}
	tk := value.Key.GetToken()
	if tk == nil || tk.Position == nil {
		return
	}
	if _, seen := lines[tk.Value]; !seen {
		lines[tk.Value] = tk.Position.Line
	}
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
	// The block always opens the file, so its first line is the line after the
	// delimiter and every position inside it offsets by the delimiter line.
	doc.FrontmatterLine = 1
	fm, err := UnmarshalFrontmatter(frontmatter)
	if err != nil {
		var fe *FrontmatterError
		errors.As(err, &fe)
		doc.Err = &FrontmatterError{Message: fe.Message, Line: doc.FrontmatterLine + fe.Line, Column: fe.Column}
		return doc, nil
	}
	doc.Frontmatter = fm
	doc.KeyLines = make(map[string]int, len(fm))
	for key, line := range KeyLines(frontmatter) {
		doc.KeyLines[key] = doc.FrontmatterLine + line
	}
	return doc, nil
}

// Localize rewrites document paths the way a caller would type them: forward
// slashes, and relative to base when the document lives under it.
func Localize(docs []*Document, base string) {
	for _, doc := range docs {
		doc.Path = localPath(base, doc.Path)
	}
}

func localPath(base, path string) string {
	if !filepath.IsAbs(path) {
		return filepath.ToSlash(path)
	}
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
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
