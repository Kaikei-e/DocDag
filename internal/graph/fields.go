package graph

import (
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/Kaikei-e/DocDag/internal/config"
	"github.com/Kaikei-e/DocDag/internal/model"
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
			value, written := n.Attr(name)
			switch {
			case written && !spec.Accepts(value):
				findings = append(findings, unknownFieldValue(cfg, n, name, value, spec))
			case !written && spec.Required:
				// A key written as a list or a mapping is not a scalar and
				// reads as absent, which is what a required scalar key
				// carrying one is: the value it was to hold is not there.
				findings = append(findings, missingField(cfg, n, name, spec))
			}
		}
	}
	SortFindings(findings)
	return findings
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

// asOfDay renders the comparison date as the calendar day a sunset is written
// as. Days are compared as text because ISO 8601 dates sort chronologically:
// there is no clock arithmetic to get wrong, and no timezone can carry a corpus
// past a deadline its reader has not reached.
func asOfDay(asOf time.Time) string {
	if asOf.IsZero() {
		asOf = time.Now()
	}
	return asOf.Format(config.AttrDateLayout)
}

// sunsetPassed reports whether the comparison day is past the day the field's
// sunset names. The sunset day itself still warns: it is the last day the
// field is tolerated, not the first day it is refused.
func sunsetPassed(spec config.FieldSpec, day string) bool {
	if _, ok := spec.SunsetDate(); !ok {
		return false
	}
	return day > spec.Sunset
}
