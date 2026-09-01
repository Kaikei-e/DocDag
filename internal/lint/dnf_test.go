package lint

import (
	"slices"
	"strings"
	"testing"

	"github.com/Kaikei-e/DocDag/internal/config"
)

// expand is called through this helper so a test never repeats the depth.
func testExpand(cfg config.Config, cond config.Condition) ([]conjunct, bool) {
	return analyzer{cfg: cfg}.expand(cond, false, 0)
}

func testHasLiteral(c conjunct, want literal) bool { return slices.Contains(c.literals, want) }

func TestExpandConjunction(t *testing.T) {
	cfg := testConfig()
	cond := config.Condition{
		Inbound: config.EdgeCondition{Edge: "supersedes"},
		Attr:    map[string]config.AttrCondition{config.DefaultStatusField: testEq(config.StatusAccepted)},
	}

	dnf, complete := testExpand(cfg, cond)

	if !complete {
		t.Error("complete = false, want a condition this small to expand whole")
	}
	if len(dnf) != 1 {
		t.Fatalf("conjunctions = %d, want 1: %v", len(dnf), dnf)
	}
	if len(dnf[0].literals) != 2 {
		t.Fatalf("literals = %v, want the edge clause and the attribute clause", dnf[0])
	}
	if !testHasLiteral(dnf[0], literal{kind: litDegree, key: "supersedes", inbound: true, min: 1}) {
		t.Errorf("literals = %v, want the inbound supersedes clause", dnf[0])
	}
	if !testHasLiteral(dnf[0], literal{kind: litEq, key: config.DefaultStatusField, value: config.StatusAccepted}) {
		t.Errorf("literals = %v, want the status clause", dnf[0])
	}
}

func TestExpandAnyOf(t *testing.T) {
	cfg := testConfig()
	cond := config.Condition{
		NotInbound: "supersedes",
		AnyOf: []config.Condition{
			testAttr(config.DefaultStatusField, testEq(config.StatusAccepted)),
			testAttr(config.DefaultStatusField, testEq(config.StatusProposed)),
		},
	}

	dnf, complete := testExpand(cfg, cond)

	if !complete || len(dnf) != 2 {
		t.Fatalf("dnf = %v (complete %v), want one conjunction per alternative", dnf, complete)
	}
	for _, c := range dnf {
		if !testHasLiteral(c, literal{kind: litAbsent, key: "supersedes", inbound: true}) {
			t.Errorf("conjunction %v, want the surrounding not_inbound clause distributed into it", c)
		}
	}
}

func TestExpandNegation(t *testing.T) {
	cfg := testConfig()
	tests := []struct {
		name  string
		not   config.Condition
		want  []literal
		count int
	}{
		{
			name:  "an equality becomes an inequality",
			not:   testAttr(config.DefaultStatusField, testEq(config.StatusAccepted)),
			want:  []literal{{kind: litNot, key: config.DefaultStatusField, value: config.StatusAccepted}},
			count: 1,
		},
		{
			name:  "an existence clause becomes an absence",
			not:   config.Condition{Inbound: config.EdgeCondition{Edge: "supersedes"}},
			want:  []literal{{kind: litAbsent, key: "supersedes", inbound: true}},
			count: 1,
		},
		{
			name:  "an absence becomes an existence",
			not:   config.Condition{NotInbound: "supersedes"},
			want:  []literal{{kind: litDegree, key: "supersedes", inbound: true, min: 1}},
			count: 1,
		},
		{
			name: "a threshold keeps its own negation, having no word of its own",
			not:  config.Condition{Inbound: config.EdgeCondition{Edge: "supersedes", Min: testInt(3)}},
			want: []literal{{kind: litDegree, key: "supersedes", inbound: true, min: 3, negate: true}},

			count: 1,
		},
		{
			name: "a conjunction becomes a disjunction of negations",
			not: config.Condition{
				Inbound: config.EdgeCondition{Edge: "supersedes"},
				Attr:    map[string]config.AttrCondition{config.DefaultStatusField: testEq(config.StatusAccepted)},
			},
			count: 2,
		},
		{
			name:  "a disjunction becomes a conjunction of negations",
			not:   config.Condition{AnyOf: []config.Condition{{NotInbound: "supersedes"}, {NotOutbound: "supersedes"}}},
			count: 1,
			want: []literal{
				{kind: litDegree, key: "supersedes", inbound: true, min: 1},
				{kind: litDegree, key: "supersedes", min: 1},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dnf, complete := testExpand(cfg, config.Condition{Not: &tt.not})

			if !complete {
				t.Fatal("complete = false, want the negation to expand whole")
			}
			if len(dnf) != tt.count {
				t.Fatalf("conjunctions = %d, want %d: %v", len(dnf), tt.count, dnf)
			}
			for _, want := range tt.want {
				if !testHasLiteral(dnf[0], want) {
					t.Errorf("conjunction %v, want the literal %v", dnf[0], want)
				}
			}
		})
	}
}

func TestExpandDoubleNegation(t *testing.T) {
	cfg := testConfig()
	inner := testAttr(config.DefaultStatusField, testEq(config.StatusAccepted))

	dnf, _ := testExpand(cfg, config.Condition{Not: &config.Condition{Not: &inner}})

	if len(dnf) != 1 || !testHasLiteral(dnf[0], literal{kind: litEq, key: config.DefaultStatusField, value: config.StatusAccepted}) {
		t.Fatalf("dnf = %v, want the original claim back", dnf)
	}
}

func TestExpandEmptyCondition(t *testing.T) {
	cfg := testConfig()

	held, _ := testExpand(cfg, config.Condition{})
	if len(held) != 1 || len(held[0].literals) != 0 {
		t.Errorf("dnf = %v, want one empty conjunction, which is the condition that holds everywhere", held)
	}

	empty := config.Condition{}
	denied, _ := analyzer{cfg: cfg}.expand(empty, true, 0)
	if len(denied) != 0 {
		t.Errorf("negated dnf = %v, want no conjunctions, which is the condition that holds nowhere", denied)
	}
}

// testWide is a condition of nested alternatives, each level pinning a key of
// its own so no two of the alternatives it expands to are the same clause.
func testWide(depth, breadth int) config.Condition {
	cond := config.Condition{Attr: map[string]config.AttrCondition{}}
	if depth == 0 {
		return cond
	}
	key := "k" + string(rune('a'+depth))
	for i := range breadth {
		alternative := testWide(depth-1, breadth)
		alternative.Attr[key] = testEq(string(rune('a' + i)))
		cond.AnyOf = append(cond.AnyOf, alternative)
	}
	return cond
}

// TestExpandTooWide drives the expansion past its own bound: three nested
// alternatives of five each are 125 conjunctions, and the walk stops at 64.
func TestExpandTooWide(t *testing.T) {
	cfg := testConfig()

	dnf, complete := testExpand(cfg, testWide(3, 5))

	if complete {
		t.Error("complete = true, want the expansion to report that it stopped short")
	}
	if len(dnf) > maxConjuncts {
		t.Errorf("conjunctions = %d, want at most %d", len(dnf), maxConjuncts)
	}
	if narrow, _ := testExpand(cfg, testWide(2, 5)); len(narrow) != 25 {
		t.Errorf("conjunctions = %d, want 25 when the expansion stays inside the bound", len(narrow))
	}
}

func TestConjunctCovers(t *testing.T) {
	accepted := literal{kind: litEq, key: config.DefaultStatusField, value: config.StatusAccepted}
	superseded := literal{kind: litAbsent, key: "supersedes", inbound: true}
	narrow := newConjunct([]literal{accepted, superseded})
	wide := newConjunct([]literal{accepted})

	if !narrow.covers(wide) {
		t.Error("covers = false, want the conjunction claiming more to cover the one claiming less")
	}
	if wide.covers(narrow) {
		t.Error("covers = true, want the weaker conjunction not to cover the stronger")
	}
}

func TestLiteralString(t *testing.T) {
	tests := []struct {
		name string
		l    literal
		want string
	}{
		{name: "a bare edge clause names no window", l: literal{kind: litDegree, key: "supersedes", inbound: true, min: 1}, want: "inbound supersedes"},
		{name: "a threshold names its window", l: literal{kind: litDegree, key: "x", min: 5}, want: "outbound x (min 5)"},
		{name: "an absence is the word the vocabulary spells it with", l: literal{kind: litAbsent, key: "x"}, want: "not_outbound x"},
		{name: "an attribute names its key and operand", l: literal{kind: litEq, key: "status", value: "accepted"}, want: "attr status: eq accepted"},
		{name: "a one-hop clause names what it wants of the neighbour", l: literal{kind: litVia, key: "premise", value: "status eq retired"}, want: "via premise {status eq retired}"},
		{name: "a negated literal says so", l: literal{kind: litVia, key: "premise", negate: true}, want: "not via premise {}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.l.String(); got != tt.want {
				t.Errorf("String = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRenderAttrsIsCanonical(t *testing.T) {
	first := renderAttrs(map[string]config.AttrCondition{"a": testEq("1"), "b": testNot("2")})
	second := renderAttrs(map[string]config.AttrCondition{"b": testNot("2"), "a": testEq("1")})

	if first != second {
		t.Errorf("renderAttrs = %q and %q, want one canonical rendering", first, second)
	}
	if !strings.Contains(first, "a eq 1") {
		t.Errorf("renderAttrs = %q, want it to name the operands", first)
	}
}
