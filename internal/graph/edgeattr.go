package graph

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
	"github.com/Kaikei-e/DocDag/internal/parse"
)

// edgeEntries reads the references under an edge key. An edge that declares no
// attributes reads plain references alone, exactly as it did before attributes
// existed: a mapping entry under such a key names no document and stays what it
// has always been, an entry the builder cannot resolve. Only a spec with an
// attrs block accepts `{ref: 0001, reason: conflict}`.
func edgeEntries(doc *parse.Document, spec config.EdgeSpec) (entries []parse.RefEntry, invalid []string) {
	if len(spec.Attrs) > 0 {
		return parse.RefEntries(doc.Frontmatter, spec.Key)
	}
	refs, invalid := parse.Refs(doc.Frontmatter, spec.Key)
	entries = make([]parse.RefEntry, 0, len(refs))
	for _, ref := range refs {
		entries = append(entries, parse.RefEntry{Ref: ref})
	}
	return entries, invalid
}

// edgeAttrs validates one entry against the attributes its edge declares and
// returns the attributes to record on the edge. Only a value the declaration
// accepts is recorded, so every attribute an edge carries satisfies its spec
// and a reader of the graph never sees a value the vocabulary rejected; the
// value that was written down is in the finding instead.
func edgeAttrs(cfg config.Config, doc *parse.Document, spec config.EdgeSpec, entry parse.RefEntry) (map[string]string, []model.Finding) {
	findings := []model.Finding{}
	if len(spec.Attrs) == 0 {
		return nil, findings
	}
	attrs := make(map[string]string, len(entry.Attrs))
	for _, name := range slices.Sorted(maps.Keys(entry.Attrs)) {
		declared, ok := spec.Attr(name)
		if !ok {
			findings = append(findings, edgeAttrFinding(cfg, doc, spec, model.RuleEdgeAttrUnknown,
				fmt.Sprintf("%s reference %q carries unknown attribute %q, declared: %s",
					spec.Name, entry.Ref, name, strings.Join(spec.AttrNames(), ", "))))
			continue
		}
		// A value that is not a scalar has no string form to check, so it is
		// rendered as written, the way an entry that is not a reference is.
		value, isScalar := parse.Scalar(entry.Attrs[name])
		if !isScalar {
			value = fmt.Sprint(entry.Attrs[name])
		}
		if !isScalar || !declared.Accepts(value) {
			findings = append(findings, edgeAttrFinding(cfg, doc, spec, model.RuleEdgeAttrInvalid,
				fmt.Sprintf("%s reference %q attribute %q is %q, want %s",
					spec.Name, entry.Ref, name, value, declared.Wants())))
			continue
		}
		attrs[name] = value
	}
	// A plain reference carries no attributes at all, so it is missing every
	// required one: an attribute a scalar entry could opt out of would not be
	// required, and the vocabulary a preset closes would have a hole in it.
	for _, name := range spec.AttrNames() {
		if _, written := entry.Attrs[name]; written || !spec.Attrs[name].Required {
			continue
		}
		findings = append(findings, edgeAttrFinding(cfg, doc, spec, model.RuleEdgeAttrMissing,
			fmt.Sprintf("%s reference %q is missing required attribute %q", spec.Name, entry.Ref, name)))
	}
	if len(attrs) == 0 {
		return nil, findings
	}
	return attrs, findings
}

// edgeAttrFinding files one attribute finding on the line the edge key was
// written on, which is where the entry it is about lives.
func edgeAttrFinding(cfg config.Config, doc *parse.Document, spec config.EdgeSpec, rule, detail string) model.Finding {
	return model.Finding{
		Severity: cfg.Severity(rule),
		Rule:     rule,
		ID:       doc.ID,
		Detail:   detail,
		Location: model.Locate(doc.Path, doc.FrontmatterLine, doc.KeyLines, spec.Key),
	}
}
