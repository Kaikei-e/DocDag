// Package parse turns Markdown files with YAML frontmatter into documents: the
// frontmatter split, a strict YAML decode, body links and derived edges.
package parse

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
// FrontmatterLine, BodyLine and KeyLines are 1-based file lines, zero when
// unknown, so a finding can name the exact key or body line it is about.
//
// Kind is the kind whose directory the file was read from, empty on a
// single-kind corpus. Identity is the token the identifier was read from: the
// file name under the single-kind rules, and under a kind the frontmatter id
// where the document writes one and the name's stem where it does not. A
// finding quotes it when the token yields no identifier at all, which is the
// one case where a document exists with an empty ID.
type Document struct {
	Path            string
	Name            string
	ID              model.ID
	Kind            string
	Identity        string
	Frontmatter     map[string]any
	Body            string
	HasFrontmatter  bool
	MatchesPattern  bool
	FrontmatterLine int
	BodyLine        int
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

// embeddedPosition matches the position a decode failure writes into its own
// text, such as the earlier definition a duplicate key names.
var embeddedPosition = regexp.MustCompile(`\[(\d+):(\d+)\]`)

// offsetBy moves a block-relative failure onto the file. The message counts
// from the block's first line too, so a reader is not told two positions.
func (e *FrontmatterError) offsetBy(first int) *FrontmatterError {
	message := embeddedPosition.ReplaceAllStringFunc(e.Message, func(match string) string {
		parts := embeddedPosition.FindStringSubmatch(match)
		line, err := strconv.Atoi(parts[1])
		if err != nil {
			return match
		}
		return fmt.Sprintf("[%d:%s]", first+line, parts[2])
	})
	return &FrontmatterError{Message: message, Line: first + e.Line, Column: e.Column}
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

// File parses one Markdown file under the single-kind identity rules: the file
// name carries the identity. A frontmatter decode failure is recorded on the
// returned document rather than returned, so later checks still run.
func File(path string, cfg config.Config) (*Document, error) {
	doc, err := readDocument(path)
	if err != nil {
		return nil, err
	}
	norm := cfg.Normalizer()
	doc.MatchesPattern = norm.MatchesFilename(doc.Name)
	doc.Identity = doc.Name
	if id, ok := norm.Normalize(doc.Name); ok {
		doc.ID = id
	}
	return doc, nil
}

// KindFile parses one Markdown file as a document of the named kind: the
// frontmatter id key carries the identity where the document writes one, and
// the file name's stem otherwise. A file that yields neither is a document
// without an identity, which CheckDocuments reports rather than skips.
func KindFile(path string, cfg config.Config, kind string) (*Document, error) {
	doc, err := readDocument(path)
	if err != nil {
		return nil, err
	}
	doc.Kind = kind
	// A kind names a directory of its own, so membership of that directory is
	// what makes a file one of its documents. There is no file-name pattern to
	// fall short of, and a file that yields no identity is reported rather than
	// passed over in silence.
	doc.MatchesPattern = true
	norm := cfg.KindNormalizer(kind)
	doc.Identity = strings.TrimSuffix(doc.Name, markdownExt)
	if written, ok := Attr(doc.Frontmatter, config.KeyID); ok {
		doc.Identity = strings.TrimSpace(written)
	}
	if id, ok := norm.Normalize(doc.Identity); ok {
		doc.ID = id
	}
	return doc, nil
}

// readDocument reads one Markdown file into a document: the frontmatter split
// from the body, decoded, and located. It settles everything but identity,
// which is the one thing the kinds disagree about.
func readDocument(path string) (*Document, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read document %s: %w", path, err)
	}
	doc := &Document{Path: path, Name: filepath.Base(path)}

	frontmatter, body, ok := SplitFrontmatter(src)
	doc.Body = string(body)
	// The body is a suffix of the file, so what precedes it locates its first
	// line without re-walking the block.
	doc.BodyLine = 1 + bytes.Count(src[:len(src)-len(body)], []byte("\n"))
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
		doc.Err = fe.offsetBy(doc.FrontmatterLine)
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
		doc.Path = LocalPath(base, doc.Path)
	}
}

// LocalPath rewrites one path the way a caller standing in base would type it.
func LocalPath(base, path string) string {
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
	entries, err := markdownEntries(dir)
	if err != nil {
		return nil, err
	}
	docs := make([]*Document, 0, len(entries))
	for _, name := range entries {
		doc, err := File(filepath.Join(dir, name), cfg)
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

// KindDir parses every Markdown file directly in one kind's directory. Unlike
// Dir it skips nothing: the directory is what declares a file a document of
// this kind, so a file that yields no identity is a finding rather than another
// tool's file.
func KindDir(dir string, cfg config.Config, kind string) ([]*Document, error) {
	entries, err := markdownEntries(dir)
	if err != nil {
		return nil, err
	}
	docs := make([]*Document, 0, len(entries))
	for _, name := range entries {
		doc, err := KindFile(filepath.Join(dir, name), cfg, kind)
		if err != nil {
			return nil, err
		}
		docs = append(docs, doc)
	}
	return docs, nil
}

// Kinds parses every declared kind's directory. The kinds are read in sorted
// name order and each directory in file-name order, so the corpus is assembled
// the same way on every run and an identifier collision names the same first
// document each time.
func Kinds(cfg config.Config) ([]*Document, error) {
	docs := []*Document{}
	for _, name := range cfg.KindNames() {
		kindDocs, err := KindDir(cfg.Kinds[name].Dir, cfg, name)
		if err != nil {
			return nil, err
		}
		docs = append(docs, kindDocs...)
	}
	return docs, nil
}

// Documents parses the corpus a configuration describes: every kind's
// directory, or the single documents directory of a corpus that declares no
// kinds.
func Documents(cfg config.Config) ([]*Document, error) {
	if cfg.Multikind() {
		return Kinds(cfg)
	}
	return Dir(cfg.Dir, cfg)
}

// markdownEntries lists the Markdown files directly in dir, in the order
// os.ReadDir returns them, which is by file name.
func markdownEntries(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read documents directory %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != markdownExt {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

// Attr reads a scalar frontmatter value as a string.
func Attr(fm map[string]any, key string) (string, bool) {
	value, ok := fm[key]
	if !ok {
		return "", false
	}
	return Scalar(value)
}

// Refs reads a list-valued frontmatter key as raw, un-normalized references. A
// scalar value is accepted as a single-element list. invalid holds the entries
// that are not scalars at all, rendered as written: an unquoted wikilink
// decodes as a nested sequence, and dropping it silently would hide the very
// link the tool exists to find.
func Refs(fm map[string]any, key string) (refs, invalid []string) {
	items, ok := listItems(fm, key)
	if !ok {
		return nil, nil
	}
	refs = make([]string, 0, len(items))
	for _, item := range items {
		ref, ok := Scalar(item)
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

// RefEntry is one entry under an edge key: the raw, un-normalized reference it
// names and the attributes it was written with. Attrs holds the values as YAML
// decoded them, so the caller can report a value that is not a scalar rather
// than lose it; a plain reference carries none.
type RefEntry struct {
	Ref   string
	Attrs map[string]any
}

// RefEntries reads a list-valued frontmatter key as references that may carry
// attributes: a scalar item is a plain reference, and a mapping item naming a
// ref key is an attributed one, the remaining keys being its attributes. As in
// Refs, invalid holds the entries that are neither, rendered as written — a
// mapping without a ref names no document, and a caller must not drop it in
// silence. Only an edge whose spec declares attributes reads its key this way;
// every other edge keeps taking plain references alone.
func RefEntries(fm map[string]any, key string) (entries []RefEntry, invalid []string) {
	items, ok := listItems(fm, key)
	if !ok {
		return nil, nil
	}
	entries = make([]RefEntry, 0, len(items))
	for _, item := range items {
		if ref, isScalar := Scalar(item); isScalar {
			if ref = strings.TrimSpace(ref); ref == "" {
				continue
			}
			entries = append(entries, RefEntry{Ref: ref})
			continue
		}
		entry, ok := attributedEntry(item)
		if !ok {
			invalid = append(invalid, fmt.Sprint(item))
			continue
		}
		entries = append(entries, entry)
	}
	return entries, invalid
}

// attributedEntry reads one mapping item as an attributed reference. A mapping
// whose ref is missing, is not a scalar or is blank names nothing, so it is not
// an entry at all and the caller reports it the way it reports any other item
// that is not a reference.
func attributedEntry(item any) (RefEntry, bool) {
	mapping, isMapping := item.(map[string]any)
	if !isMapping {
		return RefEntry{}, false
	}
	ref, isScalar := Scalar(mapping[config.EdgeRefKey])
	if !isScalar {
		return RefEntry{}, false
	}
	if ref = strings.TrimSpace(ref); ref == "" {
		return RefEntry{}, false
	}
	attrs := make(map[string]any, len(mapping)-1)
	for name, value := range mapping {
		if name != config.EdgeRefKey {
			attrs[name] = value
		}
	}
	return RefEntry{Ref: ref, Attrs: attrs}, true
}

// listItems renders a frontmatter value as the list it stands for: a scalar is
// a one-element list, and a key that is absent or empty is no list at all.
func listItems(fm map[string]any, key string) ([]any, bool) {
	value, ok := fm[key]
	if !ok || value == nil {
		return nil, false
	}
	items, isList := value.([]any)
	if !isList {
		return []any{value}, true
	}
	return items, true
}

// Scalar renders a decoded YAML scalar as the string it was written as, and
// reports whether the value is a scalar at all. A zero-padded reference decodes
// as a number, so numbers must stringify.
func Scalar(value any) (string, bool) {
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
