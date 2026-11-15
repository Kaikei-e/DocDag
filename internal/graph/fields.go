package graph

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

// CheckDeprecatedFields reports documents whose frontmatter still writes a key
// the configuration retired. asOf is the day the sunsets are compared against:
// it is handed in rather than read from the clock so a report is reproducible,
// and so the as-of projection a later ADR adds has one seam to arrive through.
// The zero time means today, which is what a caller with no opinion means.
func CheckDeprecatedFields(g *model.Graph, cfg config.Config, asOf time.Time) []model.Finding {
	findings := []model.Finding{}
	day := asOfDay(asOf)
	for _, id := range g.NodeIDs() {
		n := g.Nodes[id]
		fields := cfg.FieldSpecs(n.Kind)
		for _, name := range slices.Sorted(maps.Keys(fields)) {
			spec := fields[name]
			if _, written := n.Attrs[name]; !written || !spec.Deprecated {
				continue
			}
			findings = append(findings, deprecatedField(cfg, n, name, spec, day))
		}
	}
	SortFindings(findings)
	return findings
}

// CheckFieldValues reports what a field declaration says about the value a
// document writes under it: a value outside a declared vocabulary, and a
// required key nothing wrote at all. Both read the declarations a document of
// its kind sees, so a kind that declares a field describes it for its own
// documents and nobody else's, and both answer for open kinds as well as closed
// ones: a closed kind is about which keys may appear, and this is about what
// the declared ones say.
func CheckFieldValues(g *model.Graph, cfg config.Config) []model.Finding {
	findings := []model.Finding{}
	for _, id := range g.NodeIDs() {
		n := g.Nodes[id]
		fields := cfg.FieldSpecs(n.Kind)
		for _, name := range slices.Sorted(maps.Keys(fields)) {
			spec := fields[name]
			if spec.FieldType() == config.FieldTypeList {
				findings = append(findings, checkListField(cfg, n, name, spec)...)
				continue
			}
			findings = append(findings, checkScalarField(cfg, n, name, spec)...)
		}
	}
	SortFindings(findings)
	return findings
}

func checkScalarField(cfg config.Config, n *model.Node, name string, spec config.FieldSpec) []model.Finding {
	if value, written := n.Attr(name); written {
		if !spec.Accepts(value) {
			return []model.Finding{unknownFieldValue(cfg, n, name, value, spec)}
		}
		return nil
	}
	if !spec.Required {
		return nil
	}
	if raw, present := n.Attrs[name]; present && raw != nil {
		return []model.Finding{nonScalarField(cfg, n, name, raw)}
	}
	return []model.Finding{missingField(cfg, n, name, spec)}
}

func checkListField(cfg config.Config, n *model.Node, name string, spec config.FieldSpec) []model.Finding {
	raw, present := n.Attrs[name]
	if !present || raw == nil {
		if spec.Required {
			return []model.Finding{missingField(cfg, n, name, spec)}
		}
		return nil
	}
	items, ok := n.AttrList(name)
	if !ok {
		return []model.Finding{fieldShapeFinding(cfg, n, name,
			fmt.Sprintf("frontmatter key %q is declared as a list but holds a mapping", name))}
	}
	var findings []model.Finding
	if spec.MinItems > 0 && len(items) < spec.MinItems {
		unit := "items"
		if len(items) == 1 {
			unit = "item"
		}
		findings = append(findings, fieldShapeFinding(cfg, n, name,
			fmt.Sprintf("frontmatter key %q holds %d %s, want at least %d", name, len(items), unit, spec.MinItems)))
	}
	for _, item := range items {
		if !spec.Accepts(item) {
			findings = append(findings, unknownFieldValue(cfg, n, name, item, spec))
		}
	}
	return findings
}

func nonScalarField(cfg config.Config, n *model.Node, name string, raw any) model.Finding {
	if _, isList := raw.([]any); isList {
		return fieldShapeFinding(cfg, n, name,
			fmt.Sprintf("frontmatter key %q is declared as a scalar but holds a list; declare it with type: %s",
				name, config.FieldTypeList))
	}
	return fieldShapeFinding(cfg, n, name,
		fmt.Sprintf("frontmatter key %q is declared as a scalar but holds a mapping", name))
}

func fieldShapeFinding(cfg config.Config, n *model.Node, name, detail string) model.Finding {
	return model.Finding{
		Severity: cfg.Severity(model.RuleMissingField),
		Rule:     model.RuleMissingField,
		ID:       n.ID,
		Detail:   detail,
		Location: n.Location(name, statusField(cfg)),
	}
}

// unknownFieldValue files a value outside the declared vocabulary on the line
// the key is written on, which is the line the value has to be rewritten on.
func unknownFieldValue(cfg config.Config, n *model.Node, name, value string, spec config.FieldSpec) model.Finding {
	return model.Finding{
		Severity: cfg.Severity(model.RuleUnknownFieldValue),
		Rule:     model.RuleUnknownFieldValue,
		ID:       n.ID,
		Detail:   fmt.Sprintf("%s %q is outside the vocabulary %s", name, value, spec.Wants()),
		Location: n.Location(name),
	}
}

// missingField files a required key nobody wrote. There is no line of its own
// to point at, so the finding lands on the status field and then on the
// frontmatter block itself: the reader is being sent to a document, not to a
// mistake in it.
func missingField(cfg config.Config, n *model.Node, name string, spec config.FieldSpec) model.Finding {
	detail := fmt.Sprintf("frontmatter key %q is required", name)
	if wants := spec.Wants(); wants != "" {
		detail += ", one of: " + wants
	}
	return model.Finding{
		Severity: cfg.Severity(model.RuleMissingField),
		Rule:     model.RuleMissingField,
		ID:       n.ID,
		Detail:   detail,
		Location: n.Location(name, statusField(cfg)),
	}
}

// deprecatedField files one deprecation on the line the key was written on,
// which is the line a reader has to change.
func deprecatedField(cfg config.Config, n *model.Node, name string, spec config.FieldSpec, day string) model.Finding {
	past := sunsetPassed(spec, day)
	// Past its sunset the finding is an error whatever structural: says: the
	// day is the corpus's own deadline, and a check that kept warning after it
	// would make the deadline a comment. What structural: can raise is the
	// deprecation before that day, which is the form a corpus still chooses.
	severity := cfg.Severity(model.RuleDeprecatedField)
	if past {
		severity = model.SeverityError
	}
	return model.Finding{
		Severity: severity,
		Rule:     model.RuleDeprecatedField,
		ID:       n.ID,
		Detail:   deprecatedDetail(name, spec, past),
		Location: n.Location(name),
	}
}

// deprecatedDetail says which key is retired, which preset revision retired it
// and what its sunset is, leaving out whatever the declaration does not say.
func deprecatedDetail(name string, spec config.FieldSpec, past bool) string {
	detail := fmt.Sprintf("frontmatter key %q is deprecated", name)
	if spec.Since > 0 {
		detail += fmt.Sprintf(" since preset version %d", spec.Since)
	}
	switch {
	case past:
		detail += fmt.Sprintf(", past its sunset %s", spec.Sunset)
	case spec.Sunset != "":
		detail += fmt.Sprintf(", sunset %s", spec.Sunset)
	}
	return detail
}

// AsOfDay renders the day a run is about the way frontmatter writes one. Days
// are compared as text because ISO 8601 dates sort chronologically: there is no
// clock arithmetic to get wrong, and no timezone can carry a corpus past a
// deadline its reader has not reached. The zero time means today, which is what
// a caller with no opinion about the date means.
//
// It is exported because every report that carries an as-of date has to write
// the same day the checks compared against, down to the spelling.
func AsOfDay(asOf time.Time) string {
	if asOf.IsZero() {
		asOf = time.Now()
	}
	return asOf.Format(config.AttrDateLayout)
}

// asOfDay is AsOfDay under the name the package's own callers read it by.
func asOfDay(asOf time.Time) string { return AsOfDay(asOf) }

// sunsetPassed reports whether the comparison day is past the day the field's
// sunset names. The sunset day itself still warns: it is the last day the
// field is tolerated, not the first day it is refused.
func sunsetPassed(spec config.FieldSpec, day string) bool {
	if _, ok := spec.SunsetDate(); !ok {
		return false
	}
	return day > spec.Sunset
}
