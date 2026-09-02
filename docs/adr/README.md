# Architecture decision records

Five records stand behind the design DocDag ships. They are the reasoning;
[configuration.md](../configuration.md) and [checks.md](../checks.md) are what the binary does, so a
record is read for *why* a key exists and the reference pages for what it accepts today. The records
are written in Japanese, the language they were argued in.

Read **0001 first**: it establishes the vocabulary — kinds, edge attributes, projections, the `spec`
preset — that the other four extend. The remaining four are independent of one another and can be
read in any order, or singly, when a particular check needs an explanation.

Every record is **Accepted**, and the four that were implemented after acceptance carry their
departures inline rather than in a superseding record: 0002 under 実装時の注記 and 0005 under 実装時の逸脱. A
decision the implementation took differently is recorded where the decision is, because a reader who
has found the record has found the only place the difference matters.

## [0001 — the `spec` preset, without an expression language](0001-extend-docdag-with-spec-preset-without-expression-language.md)

The record that opens the series. It decides that a corpus may hold several `kinds:` of document,
each in a directory of its own and with an identity of its own; that an edge may carry attributes;
that `projections:` derive named boolean attributes and `binding:` names the one that defines what
is in force; that `fields:` declares the lifecycle of a frontmatter key against a `preset_version:`;
and that all of it ships as a second preset, `spec`, describing a normative standard as a graph.
What it declines is the obvious way to get there: no expression language, no embedded CUE or Nickel
schema, no Datalog engine — the rule vocabulary stays fixed and finite, which is what every later
record is able to reason about mechanically.

## [0002 — target conditions and path constraints](0002-target-conditions-and-path-constraints.md)

An invariant like "a conformance test enforces the current clause, not the one its successor
replaced" is about the document at the *far* end of an edge, which a one-hop rule cannot say. The
record decides two answers: `target:` on an edge spec, holding the same `when` vocabulary against
the document the edge points at and reported as the fixed error `stale_target`; and
`path_constraints:`, comparing what two composed edges reach. It declines everything that would make
those constraints a query language — regular path expressions, wildcards, paths of three steps or
more, deciding implication between constraints, and nesting `via` inside a target — because the
decidable fragment is the whole point. Its note records that the staged rollout it prescribed for
`path_constraints:` was skipped: the series shipped at once, so the demand it wanted to wait for was
never observed.

## [0003 — modality, strong permission and conflict detection](0003-modality-strong-permission-and-conflict-detection.md)

A clause that says `MAY` is indistinguishable, in a graph, from a clause the standard never wrote —
so a later `SHOULD NOT` silently overwrites the permission. The record decides that a clause holds
`modality` in the five BCP 14 keywords rather than a strength and a polarity; that a subject is a
`topic` document and `about` an edge, so two clauses can be known to speak about one thing; that a
permission standing against a prohibition is the structural finding `modality_conflict`; that an
exception is an `excepts` edge carrying a written `scope`, which suppresses the weak conflicts it
answers and is refused against a strict rule; and that RFC 2119 §5's interoperability obligation is
an `interop` edge. It declines free-choice permission semantics, any machine reading of the `scope`
prose, and guessing which of two colliding clauses is the wrong one.

## [0004 — `docdag lint`: vacuity, contradiction and silence](0004-preset-lint-vacuity-and-conflict-detection.md)

A check that never fires is either a healthy corpus or a rule that cannot fire, and nothing in the
output tells the two apart. The record decides a command that asks about the configuration in three
layers: the rules alone, expanded to disjunctive normal form and judged by set operations over the
declared finite vocabularies; the rules against the current vault; and each rule against its own
`ruleid/` and `ok/` fixtures, the pair that proves a silent rule *can* speak. It declines a general
SAT or SMT solver — the judgement closes over finite domains without one — reading any prose,
letting a lint result change what `validate` decides, and walking the whole history to find when a
rule last fired.

## [0005 — in-force periods and the as-of projection](0005-in-force-periods-and-as-of-projection.md)

"B replaces A, but B is still in trial, so A is what binds" had no vocabulary: the moment B declared
`supersedes: [A]`, A stopped binding and its status became an error. The record decides that a kind
declares a `period:` naming the two frontmatter keys its documents write their days in; that the
engine computes one virtual attribute, `in_force`, from the period and the day the run is about, so
the rule vocabulary gains no date literal and no arithmetic; that an unwritten end is derived from
the day an accepted successor begins; that `status_drift` becomes time-dependent and gains
`pending_successor` and `premature_superseded` beside it; and that `--as-of` and `--at` are
independent axes, valid time and transaction time. It declines annulment, retroactivity, any
granularity finer than a day, timezones, an Allen-relation vocabulary for comparing intervals, and
scanning history for the moment a document stopped binding. Its deviation section records five
places the implementation differs, the largest being that only the JSON reports always carry `as_of`
— a text report carries the day only where some kind declares a period, because changing the first
line of every text report would break every golden file and problem matcher that reads it.
