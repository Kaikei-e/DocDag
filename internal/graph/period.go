package graph

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Kaikei-e/DocDag/config"
	"github.com/Kaikei-e/DocDag/model"
)

// periodLineage is the edge type an end date is derived over: what ends a
// document is the successor that replaced it. It is the lineage resolve walks
// and binding reads, and the one edge type the out-of-force weight rule below
// exempts — a successor that is not in force yet is exactly the fact
// pending_successor reports, and an index that dropped its edge would silence
// it.
const periodLineage = config.EdgeSupersedes

// period is one document's interval of force, held as the days a frontmatter
// key writes: an empty From has always begun and an empty Until never ends.
// Days are compared as text because ISO 8601 dates sort chronologically —
// there is no clock arithmetic to get wrong, and no timezone can carry a corpus
// past a day its reader has not reached.
type period struct {
	// declared marks a document whose kind declares a period at all. One whose
	// kind declares none is in force whatever day is asked about, which is what
	// keeps a configuration without periods answering as it always did.
	declared bool
	from     string
	// explicit is the end the document wrote down and derived the end its
	// accepted successors imply; until is the one that counts, which is the
	// explicit one wherever there is one.
	explicit string
	derived  string
	until    string
	// sources are the accepted successors the derived end was read from,
	// sorted, and earliest the ones whose day it is.
	sources  []model.ID
	earliest []model.ID
	problems []periodProblem
}

// periodProblem is one thing wrong with the days a document wrote: a value that
// is not a date, an interval that ends before it begins, or an end that
// disagrees with the successors. It carries the key it is about, so a finding
// points at the line a reader has to change.
type periodProblem struct {
	rule   string
	key    string
	detail string
}

// Periods is what a corpus's periods say on one day: which documents are in
// force, and what is wrong with the days they wrote. It is computed before the
// projections, because a projection may read in_force and nothing the periods
// read is derived.
type Periods struct {
	day     string
	periods map[model.ID]period
}

// EvalPeriods computes the interval every document is in force for and answers
// it against one day. asOf is the day the corpus is being asked about, the zero
// time meaning today.
func EvalPeriods(g *model.Graph, cfg config.Config, asOf time.Time) Periods {
	p := Periods{day: asOfDay(asOf), periods: make(map[model.ID]period, len(g.Nodes))}
	if !cfg.Periods() {
		// Nothing declares a period, so nothing has one: the map stays empty and
		// every document answers as always in force.
		return p
	}
	successors := acceptedSuccessors(g, cfg)
	for _, id := range g.NodeIDs() {
		n := g.Nodes[id]
		spec, declared := cfg.KindPeriod(n.Kind)
		if !declared {
			continue
		}
		p.periods[id] = documentPeriod(n, spec, successors[id])
	}
	return p
}

// documentPeriod reads one document's own days and the end its successors
// derive, and settles which of the two counts.
func documentPeriod(n *model.Node, spec config.PeriodSpec, successors []successor) period {
	d := period{declared: true}
	from, ok := periodDay(n, spec.FromField())
	if !ok {
		d.problems = append(d.problems, periodProblem{
			rule: model.RulePeriodInvalid, key: spec.FromField(),
			detail: fmt.Sprintf("%s %q is not a date as YYYY-MM-DD", spec.FromField(), from),
		})
		from = ""
	}
	d.from = from

	if spec.Until != "" {
		until, ok := periodDay(n, spec.Until)
		switch {
		case !ok:
			d.problems = append(d.problems, periodProblem{
				rule: model.RulePeriodInvalid, key: spec.Until,
				detail: fmt.Sprintf("%s %q is not a date as YYYY-MM-DD", spec.Until, until),
			})
		case until == "":
			// The document writes no end of its own, which is the open period
			// every document has until a successor derives one for it.
		case d.from != "" && until < d.from:
			d.problems = append(d.problems, periodProblem{
				rule: model.RulePeriodInvalid, key: spec.Until,
				detail: fmt.Sprintf("%s %s is before %s %s", spec.Until, until, spec.FromField(), d.from),
			})
			d.explicit = until
		default:
			d.explicit = until
		}
	}

	// The derived end is the day the first accepted successor begins on: from
	// then, what binds is the successor. It is never written back to the
	// document — a derived value the corpus stores is a value that goes stale.
	for _, s := range successors {
		d.sources = append(d.sources, s.id)
		if d.derived == "" || s.from < d.derived {
			d.derived = s.from
		}
	}
	for _, s := range successors {
		if s.from == d.derived {
			d.earliest = append(d.earliest, s.id)
		}
	}

	d.until = d.explicit
	if d.until == "" {
		d.until = d.derived
	}
	if d.explicit != "" && d.derived != "" && d.explicit != d.derived {
		// Which of the two is right is not a question the graph answers, so the
		// finding carries no fix, exactly as derived_conflict does not.
		d.problems = append(d.problems, periodProblem{
			rule: model.RulePeriodConflict, key: spec.Until,
			detail: fmt.Sprintf("%s %s disagrees with the %s its accepted successor %s begins on",
				spec.Until, d.explicit, d.derived, joinIDs(d.earliest, ", ")),
		})
	}
	return d
}

// successor is one accepted document that supersedes another, and the day it
// begins on.
type successor struct {
	id   model.ID
	from string
}

// acceptedSuccessors names, per document, the accepted successors that carry a
// beginning, sorted by identifier. Only an accepted successor derives an end:
// a proposal that is withdrawn stops deriving one by ceasing to be accepted,
// which is how an abrogation is taken back without rewriting anything.
//
// A successor whose own kind declares no period still has a beginning — the
// date field, which is what a kind that names no from: reads — because what
// derives the end is the day the replacement started, not whether the
// replacement has an interval of its own.
func acceptedSuccessors(g *model.Graph, cfg config.Config) map[model.ID][]successor {
	out := make(map[model.ID][]successor, len(g.Nodes))
	for _, e := range g.EdgesOfType(periodLineage) {
		s, known := g.Node(e.From)
		if !known {
			continue
		}
		if status, ok := canonicalKindStatus(cfg, s.Kind, s.Status); !ok || !strings.EqualFold(status, config.StatusAccepted) {
			continue
		}
		day, valid := periodDay(s, successorFromField(cfg, s.Kind))
		if !valid || day == "" {
			continue
		}
		out[e.To] = append(out[e.To], successor{id: s.ID, from: day})
	}
	for id := range out {
		slices.SortFunc(out[id], func(a, b successor) int { return strings.Compare(a.id.String(), b.id.String()) })
	}
	return out
}

// successorFromField names the key a successor's beginning is read from: its
// own kind's, or the date field for a kind that declares no period at all.
func successorFromField(cfg config.Config, kind string) string {
	if spec, ok := cfg.KindPeriod(kind); ok {
		return spec.FromField()
	}
	return config.KeyDate
}

// periodDay reads one date-valued frontmatter key. It answers with the value as
// written and whether it is a date, so a caller can tell a key nobody wrote
// (empty, valid) from one written as prose (non-empty, invalid).
func periodDay(n *model.Node, key string) (string, bool) {
	raw, written := n.Attr(key)
	value := strings.TrimSpace(raw)
	if !written || value == "" {
		return "", true
	}
	if _, err := time.Parse(config.AttrDateLayout, value); err != nil {
		return value, false
	}
	return value, true
}

// Day is the day the periods were evaluated for, written as a frontmatter
// writes one.
func (p Periods) Day() string { return p.day }

// Declared reports whether a document's kind declares a period, which is what
// makes its force a question about a day rather than a constant.
func (p Periods) Declared(id model.ID) bool { return p.periods[id].declared }

// InForce reports whether a document is in force on the day the periods were
// evaluated for: its period has begun and has not ended, over the closed-open
// interval [from, until). A document whose kind declares no period is always in
// force.
func (p Periods) InForce(id model.ID) bool {
	d, ok := p.periods[id]
	if !ok || !d.declared {
		return true
	}
	if d.from != "" && p.day < d.from {
		return false
	}
	return d.until == "" || p.day < d.until
}

// Ended reports the day a document's own frontmatter says it ends on, and
// whether it says one at all. Only the day the document wrote counts: an end
// derived from a successor is what supersession already reports.
func (p Periods) Ended(id model.ID) (string, bool) {
	d := p.periods[id]
	return d.explicit, d.declared && d.explicit != ""
}

// CheckPeriods reports what is wrong with the days a corpus wrote: a value that
// is not a date, an interval that ends before it begins, an end that disagrees
// with the successors, and a record whose end has passed while its status still
// says it is in effect. A corpus whose kinds declare no period sees none of
// them.
func CheckPeriods(g *model.Graph, cfg config.Config, asOf time.Time) []model.Finding {
	findings := []model.Finding{}
	periods := EvalPeriods(g, cfg, asOf)
	for _, id := range g.NodeIDs() {
		n := g.Nodes[id]
		spec, declared := cfg.KindPeriod(n.Kind)
		if !declared {
			continue
		}
		d := periods.periods[id]
		for _, problem := range d.problems {
			findings = append(findings, periodFinding(g, cfg, n, d, problem))
		}
		if f, expired := expiredDeviation(cfg, n, periods, spec); expired {
			findings = append(findings, f)
		}
	}
	SortFindings(findings)
	return findings
}

// periodFinding files one problem on the line the day was written on, which is
// the line a reader has to change. A conflict relates the successors it
// disagrees with: they are the other half of the disagreement, and what a
// reader has to read before deciding which day is wrong.
func periodFinding(g *model.Graph, cfg config.Config, n *model.Node, d period, problem periodProblem) model.Finding {
	f := model.Finding{
		Severity: cfg.Severity(problem.rule),
		Rule:     problem.rule,
		ID:       n.ID,
		Detail:   problem.detail,
		Location: n.Location(problem.key, statusField(cfg)),
	}
	if problem.rule != model.RulePeriodConflict {
		return f
	}
	related := make([]model.Location, 0, len(d.sources))
	for _, id := range d.sources {
		source, ok := g.Node(id)
		if !ok {
			continue
		}
		related = append(related, source.Location(successorFromField(cfg, source.Kind), statusField(cfg)))
	}
	f.Related = related
	return f
}

// expiredDeviation reports a document whose own end has passed while its status
// still says it is in effect: a departure recorded until a day that has come
// and gone is the case the name is for, and the check reads any kind whose
// period names an end.
//
// A document the successors ended is not reported here — that is supersession,
// and status_drift and premature_superseded are what say it — and one whose
// status has moved on has already answered.
func expiredDeviation(cfg config.Config, n *model.Node, periods Periods, spec config.PeriodSpec) (model.Finding, bool) {
	ended, written := periods.Ended(n.ID)
	if !written || periods.Day() < ended {
		return model.Finding{}, false
	}
	status, known := canonicalKindStatus(cfg, n.Kind, n.Status)
	if !known || !strings.EqualFold(status, config.StatusAccepted) {
		return model.Finding{}, false
	}
	return model.Finding{
		Severity: cfg.Severity(model.RuleExpiredDeviation),
		Rule:     model.RuleExpiredDeviation,
		ID:       n.ID,
		Detail:   fmt.Sprintf("%s %s has passed and the status is still %s", spec.Until, ended, status),
		Location: n.Location(spec.Until, statusField(cfg)),
	}, true
}
