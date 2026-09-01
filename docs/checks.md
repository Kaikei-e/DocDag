# Checks

Every finding `docdag validate` can report. See [commands.md](commands.md) for the output formats,
[configuration.md](configuration.md) for the keys named here, and [ci.md](ci.md) for
`--immutable-since`.

A finding reads:

```
<path>:<line>: <SEVERITY> <rule> <id>: <detail>
```

## Three kinds of check

| Kind | Severity comes from | Configurable |
| --- | --- | --- |
| **Structural** | a built-in default | `structural:` may raise one to `error`; it can never be lowered or disabled |
| **Rule** | the rule's own `severity:` | `rules:` defines them; the preset ships two, and a config that writes `rules:` replaces the list |
| **Reference and history** | `references.dangling`, or fixed | `dangling_reference` is off by default; `immutable_violation` needs `--immutable-since` |

`structural:` accepts exactly these twenty-nine names: `cycle`, `dangling_ref`, `id_collision`,
`invalid_frontmatter`, `missing_frontmatter`, `unknown_status`, `derived_conflict`,
`unstructured_supersedes`, `invalid_ref`, `empty_edge`, `inverse_mismatch`, `cardinality`,
`edge_attr_unknown`, `edge_attr_missing`, `edge_attr_invalid`, `id_mismatch`, `kind_mismatch`,
`unknown_field`, `unknown_field_value`, `missing_field`, `edge_kind_mismatch`, `deprecated_field`,
`stale_target`, `path_mismatch`, `modality_conflict`, `excepts_strict`, `period_invalid`,
`period_conflict`, `expired_deviation`. Naming anything else — `status_drift` and
`superseded_orphan` included, since they are rules — is a configuration error and exits 3.

## Status is a projection

Status is a projection of the graph, not an independent fact. With the ADR preset, an inbound
`supersedes` edge and a status other than `superseded` is an error (`status_drift`); `superseded`
with nothing superseding it is a warning (`superseded_orphan`). A document is *binding* when its
status is `accepted` and no document supersedes it.

Both are ordinary rules, so a configuration that writes its own `rules:` list replaces them. The
`fix:` suggestions are keyed on these two names: a rule renamed keeps its check but loses its
suggestion.

## Periods and the day a run is about

A kind that declares `period:` says which frontmatter keys its documents write the days they are in
force between; [configuration.md](configuration.md#periods-and-as-of) has the declaration and the
`--as-of` defaults. From it the engine computes one virtual attribute:

```
in_force(d) := from(d) ≤ as-of < until(d)
```

`in_force` reads exactly as a projection does — the string `"true"` or `"false"` — so a rule, a
projection or a `target:` asks for it with `attr: {in_force: {eq: "true"}}`. **It is the one
attribute the engine computes rather than reads**, and the only exception to "the rule vocabulary is
fixed and complete": the vocabulary still has no expression language, no arithmetic and no date
literal, because the comparison happens in the engine and only its answer reaches a condition. A
projection named `in_force` is a configuration error (exit 3) — it would shadow the attribute it was
written to read. A document whose kind declares no period is **always in force**, so a corpus that
declares none answers exactly as it did before periods existed.

`until` is derived where a document writes none: it is the earliest day an **accepted** successor
begins on, and the derived day is never written back into the frontmatter. A successor that is
withdrawn stops deriving one, and the predecessor's period opens up again — an abrogation taken
back without rewriting anything.

**An out-of-force document's statements lose their weight.** Once a document has left force, the
edges it declares stop counting: for the degree thresholds (`inbound: {edge: deviates-from, min:
5}`), for the one-hop clauses (`via`, `via_inbound`), and for the recorded exceptions that suppress
a `modality_conflict`. An expired departure is a record of something that was, not a claim about
what is, and a standard that kept counting them would report pressure nobody is applying. Two things
are deliberately exempt: the `supersedes` lineage, which is what the ends are *derived from* — a
successor that is not in force yet is exactly what `pending_successor` reports — and
`path_constraints`, which are statements about the shape of the corpus rather than about what holds
today. A corpus without periods loses nothing, because there every document is in force.

### `period_invalid` — error, structural

A period key holds a value that is not a date, or an interval ends before it begins:

```
in_force_from "next quarter" is not a date as YYYY-MM-DD
in_force_until 2026-01-01 is before in_force_from 2026-06-01
```

On the line the day is written on. A day nobody can read constrains nothing, so the document is
neither begun nor ended by it. No fix suggestion: only the author knows which day was meant.

### `period_conflict` — error, structural

A document writes an end that disagrees with the day its accepted successor begins on:
`in_force_until 2026-12-31 disagrees with the 2026-06-01 its accepted successor UZ-V-002 begins
on`. The successors are listed under `related`, and the day the document wrote is the one that
decides what is in force — the finding says the two disagree, not which is right. No fix
suggestion, for the reason `derived_conflict` carries none: DocDag does not guess.

### `expired_deviation` — warn, structural

A document's own end has passed while its status still says it is in effect: `expires 2026-08-01 has
passed and the status is still accepted`. On the line the day is written on. It is named after the
case it is for — a departure recorded until a day that has come and gone — and it reads any kind
whose `period:` names an `until` key, so a corpus that dates its decisions gets the same reading of
them. A document the successors ended is not reported here: that is supersession, and
`status_drift` and `premature_superseded` are what say it.

## Document structure

### `invalid_frontmatter` — error, structural

The file opens a `---` block that does not parse as YAML, duplicate keys included. The detail is the
YAML parser's own message, positioned on the file's own line and column. No fix suggestion.

### `missing_frontmatter` — warn, structural

A file whose name matches the managed-document pattern carries no terminated `---` block: `no
frontmatter block`. A file that does *not* match the pattern is simply not a managed document and is
skipped in silence — on a corpus that declares `kinds:`, where the directory is what declares a file
a document, it is an `id_mismatch` instead. Fix: `add a YAML frontmatter block with title and
status`.

Raise it with `structural: {missing_frontmatter: error}` once every document is expected to carry
one.

### `id_collision` — error, structural

Two or more files normalize to the same identifier: `shares its identifier with <paths>`. The
finding is filed against the first path and names the rest under `related`. Identity is the file's
digit run, so `0004-a.md` and `0004-b.md` collide; on a corpus declaring `kinds:` it is whatever the
kinds' patterns say, and two documents of *different* kinds that normalize to one identifier
collide like any other pair. No fix suggestion; `docdag new` refuses to run at all while a corpus
carries one.

### `unknown_status` — error, structural

`status_values` is configured, the document's status is not blank, and the value does not fold onto
the vocabulary — the vocabulary of the document's own kind, where kinds declare their own:
`status %q is outside the vocabulary <values>`. Comparison is case-insensitive, and a value like
`superseded by 0003` is accepted when a `derived_edges:` pattern claims it. Fix: `use one of:
<values>`.

`withdrawn` is in the preset vocabulary for a proposal that was dropped rather than replaced: it
binds nothing, and because nothing supersedes it, it raises no `superseded_orphan` warning either.

Unrecognized frontmatter keys are ignored entirely, so another tool's fields raise nothing — unless
they are in a kind declared `closed: true`, which is `unknown_field` below, or declared under `fields:` and retired, which is
`deprecated_field`.

### `empty_edge` — error, structural

An edge key — or a configured `inverse:` key — is present but names no document: `<key> is present
but names no document`. `supersedes:` with nothing under it, `[]`, `""`, or a list of blank items
all qualify, because each reads as a declared relation and builds none. No fix suggestion.

### `invalid_ref` — error, structural

An entry under an edge key or an inverse key is not identifier-shaped: `<type> reference %q is not
an identifier`. `see 0042`, a sentence and a slug are all invalid refs. An Obsidian wikilink is
unwrapped first, so `supersedes: ["[[0042]]"]` and `[[0042|alias]]` name document 0042 like everyone
else. No fix suggestion — the entry has to be rewritten by hand.

### `dangling_ref` — error, structural

A typed edge, or an inverse-key entry, names an identifier the corpus does not hold: `<type>
reference %q does not name a document`. This is the identifier-shaped case; a reference that is not
identifier-shaped is `invalid_ref` instead. Fix: `did you mean 0002, 0003 or 0042?`, naming up to
three nearest identifiers, and omitted when there is no plausible candidate.

## Kinds

The four checks below exist only for a corpus that declares `kinds:`; see
[configuration.md](configuration.md). A corpus that declares none is single-kind, and none of them
can fire.

### `id_mismatch` — error, structural

A file in a kind's directory yields no identifier of that kind: `"README" is not an identifier of
kind "clause", which reads ^UZ-[A-Z]-\d{3}$`, the pattern named only where the kind declares one.
The identifier is the frontmatter `id:` where the document writes one and the file name's stem
otherwise, so this is the finding for a stray file, for a misspelled identifier, and for a document
of a kind whose pattern carries a slash — which no file name can hold — that writes no `id:` at all.
The finding names no document, because there is none to name: it is filed on the `id:` key, or on
the file's first line. The file is left out of the graph, so nothing resolves to it. No fix
suggestion.

The single-kind reader skips a file whose name it does not recognize, in silence. A kind's directory
is a declaration instead, so what it holds is reported rather than skipped.

### `kind_mismatch` — error, structural

A document writes a `kind:` its directory disagrees with: `frontmatter kind "conform" disagrees with
directory kind "clause"`. Filed on the `kind` key. The directory's answer is the one that stands —
it chose the identity rules the document was read under — so the document is still a clause to every
rule and every edge constraint. No fix suggestion: which of the two is wrong is not knowable.

### `unknown_field` — error, structural

A kind declared `closed: true` carries a frontmatter key the configuration does not know:
`frontmatter key "owner" is not declared by the closed kind "clause", declared: date, enforces, id,
kind, status, supersedes, title`. One finding per unknown key, each on its own key's line, and the
known keys are listed in alphabetical order. They are `title`, `date`, `id`, `kind`, the status
field, every edge `key:` and `inverse:`, every derived-edge `field:`, and every name declared under
`fields:` — the corpus's own and the kind's. A kind that is not closed ignores unknown keys, which
is what every corpus does by default. No fix suggestion.

### `edge_kind_mismatch` — error, structural

An edge declaring `from:` or `to:` has an endpoint of another kind:

```
enforces source UZ-V-006 is kind "clause", want one of: conform
enforces target dev-0001 is kind "deviation", want one of: clause
```

Filed against the document that declared the edge, on the key it declared it under, with the
endpoint of the wrong kind under `related`. Only an endpoint the corpus holds is checked: a
reference naming no document has no kind to be wrong about, and is a `dangling_ref` instead. A
reference that names a document of the wrong kind still resolves, so this finding says what is
actually wrong rather than reporting the reference as naming nothing. No fix suggestion.

## Declared fields

The three checks below are what a `fields:` declaration turns on: the value a document writes under
a declared key, whether it writes the key at all, and whether the key is one the corpus is retiring.
A key nobody declares is not unknown, only undeclared, so a corpus that declares nothing sees none
of them.

### `deprecated_field` — warn, structural, error past its sunset

A document writes a frontmatter key the configuration retired under `fields:`; see
[configuration.md](configuration.md). One finding per retired key per document, each on the line
the key is written on. The detail names what the declaration says and nothing it does not:

```
frontmatter key "owner" is deprecated
frontmatter key "owner" is deprecated since preset version 2
frontmatter key "owner" is deprecated since preset version 2, sunset 2027-01-01
frontmatter key "owner" is deprecated since preset version 2, past its sunset 2027-01-01
```

The last form is an **error**; the first three are warnings. The comparison is by calendar day
against the day the command runs, and the sunset day itself is the last day the field is tolerated,
so a `sunset: 2027-01-01` first errors on 2027-01-02. Because the day is the corpus's own deadline,
the escalation happens whatever `structural:` says; what `structural: {deprecated_field: error}`
raises is the pre-sunset form, which is how a corpus finishes a migration ahead of its own date.

Fix: `migrate owner to owned-by`, where the declaration names a `migrate_to`. A field being removed
rather than moved carries no suggestion — only the author knows what the value was for.

A kind that declares the key itself reads its own declaration, so a field the corpus retired is not
retired for a kind that re-declares it. And a declared field — retired or not — is a *known* key, so
a `closed: true` kind accepts it instead of reporting `unknown_field`: a migration in progress is
not a mistake.

### `unknown_field_value` — error, structural

A document writes a declared field whose value is outside the `one_of` the configuration gives it:
`modality "SHALL" is outside the vocabulary MUST, MUST_NOT, SHOULD, SHOULD_NOT, MAY`. One finding
per key per document, on the line the key is written on, and the vocabulary is listed in the order
the configuration declares it.

The comparison is **exact**, case included, for the reason an edge attribute's `one_of` is: a
declared vocabulary is a closed machine vocabulary a preset revision renames wholesale, where a
`status` is prose a person writes by hand. A value that is not a scalar at all — a list, a mapping —
is not a value under that key, and is reported as `missing_field` where the field is required. No
fix suggestion: the detail already names the whole vocabulary.

### `missing_field` — error, structural

A document of a kind whose `fields:` declares `required: true` does not write that key:
`frontmatter key "modality" is required, one of: MUST, MUST_NOT, SHOULD, SHOULD_NOT, MAY`. The
vocabulary is named where the declaration has one. There is no line of its own to point at, so the
finding lands on the status field and then on the frontmatter block itself: the reader is being sent
to a document, not to a mistake in it. No fix suggestion.

Both read the declarations a document of its own **kind** sees — the corpus's, with the kind's
winning by name — and both answer for open kinds as well as closed ones: `closed: true` is about
which keys may appear, and these two are about what the declared ones say. `docdag new --kind`
offers a required or closed-vocabulary field as a commented stub naming the vocabulary, so the
finding lands on a line the author is already looking at.

`required` and `one_of` describe a field the corpus keeps, so writing either beside
`deprecated: true` is a configuration error (exit 3), as is an empty or repeated `one_of` value.

## Edge attributes

An edge that declares `attrs:` reads `{ref: 0001, reason: conflict}` as well as a plain `0001`; see
[configuration.md](configuration.md). The three checks below are about those attributes. Each is
filed on the line the edge key is written on, and each runs whatever the reference beside it
resolves to: an entry can name no document *and* leave out a required attribute, and both are worth
saying. An edge that declares no attributes reads plain references alone, so a mapping under it is
an entry that names no document, exactly as it was before attributes existed.

### `edge_attr_unknown` — error, structural

An entry carries a key the edge does not declare: `supersedes reference "0001" carries unknown
attribute "note", declared: reason`. The declared names are listed in alphabetical order. `ref` is
the reference itself rather than an attribute, and an edge that declares an attribute of that name
is a configuration error. No fix suggestion.

### `edge_attr_missing` — error, structural

An entry does not carry an attribute the edge declares `required: true`: `supersedes reference
"0001" is missing required attribute "reason"`. A plain reference carries no attributes at all, so
on an edge with a required attribute it reports the same finding: a requirement a scalar entry could
opt out of would not be one. No fix suggestion.

### `edge_attr_invalid` — error, structural

An attribute value is not one the declaration accepts. One finding per rejected value:

```
supersedes reference "0001" attribute "reason" is "rewrite", want one of: recurrence, premise-collapse, conflict, vocabulary
measures reference "0001" attribute "agreement" is "high", want a number
measures reference "0001" attribute "taken_on" is "soon", want a date as YYYY-MM-DD
measures reference "0001" attribute "model" is "[haiku sonnet]", want a string
```

`one_of` compares exactly, case included, because an edge attribute is a closed vocabulary a preset
revision renames wholesale rather than prose a person writes by hand — this is where it parts
company with `status_values`. A `number` is anything that parses as one, `0.90` included, which is
recorded as `0.9`; a `date` is `YYYY-MM-DD` and nothing else; and a value that is not a scalar at
all — a list, a mapping — satisfies nothing and is reported as it was written. A rejected value is
not recorded on the edge, so every attribute the graph carries is one its declaration accepts. No
fix suggestion.

## The graph

### `cycle` — error, structural

A closed path over the edges of a type declared `acyclic: true`: `<type> cycle: 0001 -> 0002 ->
0001`. One finding per cycle, filed against its smallest member with the rest under `related`. With
`acyclic_union: true` and more than one acyclic type, cycles that only the union of those types
closes are reported too, as `cycle over supersedes, depends-on: …`. Fix: `remove one of the listed
edges`.

### `cardinality` — error, structural

An edge spec's `max_inbound`, `max_outbound` or `min_outbound` is exceeded or unmet. One finding per
violated bound:

```
3 inbound amends edges exceed max_inbound 1
2 outbound amends edges exceed max_outbound 0
0 outbound amends edges fall short of min_outbound 1
```

A bound of `0` is unbounded for the two maxima, and `min_outbound: 0` can never trip. No fix
suggestion.

Where an edge names its endpoint kinds, its bounds are read over those kinds alone: the outbound
bounds over `from:`, the inbound ones over `to:`. That is what makes `min_outbound: 1` sayable — a
lower bound is the one bound a document with **no such key at all** can violate, so without the
scoping it would report every document of every other kind, the edge's own targets included. A
document of another kind that does hold such an edge is an `edge_kind_mismatch`, which says the
actual mistake. An edge that names no kinds, and every corpus without `kinds:`, is bounded over the
whole corpus exactly as before.

### `inverse_mismatch` — error, structural

An edge spec declares `inverse:` and the two sides disagree — either the target does not list the
source under the inverse key (`<inverse> does not list 0001, which declares supersedes`), or the
inverse key lists a document that declares no such edge back (`<inverse> lists 0001, which declares
no supersedes edge to this document`). Filed on the inverse key's owner, with the peer under
`related`. No fix suggestion.

### `derived_conflict` — error, structural

An edge derived from a field value runs against a structured edge of the same type: `derived
supersedes edge 0003 -> 0002 contradicts the structured edge 0002 -> 0003`. One of the two is wrong;
DocDag does not guess which. No fix suggestion.

### `unstructured_supersedes` — warn, structural

An edge came from a `derived_edges:` pattern rather than an edge key: `supersedes edge 0003 -> 0002
comes from a field value; declare it in frontmatter`. This is what a conventional MADR `status:
superseded by 0003` raises — a suggestion, not a failure. The graph is the same either way, and
moving the string to a `supersedes:` key clears the warning. Fix: `declare supersedes: 0002 in
0003`.

### `stale_target` — error, structural

An edge whose spec declares `target:` points at a document that does not satisfy it; see
[configuration.md](configuration.md#target-conditions). An edge that declares no `target:` can
never raise it, so a corpus enables the check by configuring it and hears nothing otherwise. The
severity is fixed at error, like `cardinality`.

```
enforces targets UZ-V-001, which UZ-V-001a supersedes
depends-on targets 0001, which 0002 supersedes
amends targets 0006, which does not satisfy the edge's target condition
```

The first two are a `leaf_of:` failure: it names every document that replaced the target, and the
edge name reads as the configuration declared it rather than inflected for one document or several.
Every other condition reports the third form — what a target has to look like is whatever the corpus
declared, and there is no shorter way to say it. The target's own location is under `related`, next
to the replacements where there are any.

The finding is filed against the document that *declared* the edge, on the key it declared it under
— the derived field's line where the edge came from a `derived_edges:` pattern, since a target
condition reaches derived edges too. A reference naming no document raises `dangling_ref` alone:
there is no target to hold a condition against.

Fix: `did you mean UZ-V-004?`, and `did you mean one of: 0004, 0005?` where the lineage branched.
Only a `leaf_of:` target carries one — it says the target was replaced, and `resolve` walks the
lineage to the replacement. **The check is local and only the suggestion is transitive**: a lineage
that loops has no leaf to name, so the finding stands with no fix beside it. Any other condition is
a statement about the target document, and which document satisfies it instead is not a question
the graph answers.

A condition written on an edge spec sees the target's own local condition and nothing further:
nesting `via` or another `target` inside it is a configuration error, so the depth stays fixed at
two — one edge, then a condition about what is at the end of it. That is the bar every future
addition to the vocabulary is reviewed against as well: a word is added only if conditions stay
inside the bisimulation-invariant fragment, which is what keeps a check a question about a
document's neighbourhood rather than a query language with an evaluator to reason about.

### `path_mismatch` — error, structural

A document from which the `path:` of a `path_constraints:` entry reaches something its `equals:` or
`subset_of:` does not; see [configuration.md](configuration.md#path-constraints). Like
`stale_target`, it fires only where a configuration declares one, and its severity is fixed:

```
amend_targets_current: amends -> ^supersedes reaches 0002, want none
amend_scope_consistent: amends -> depends-on reaches 0006, which depends-on does not
```

The detail names the constraint, the path as it was written — `^` and all — and the documents in
P(d) − Q(d); those documents are also under `related`. The finding sits on the key declaring the
path's **first** step, since that is the only step the document itself wrote down; where it declares
none — which a reversed first step never does — it falls back on the status field and then on the
frontmatter's opening line.

No fix suggestion. Two paths disagree and DocDag does not guess which of them is the wrong one —
the same policy as `derived_conflict`.

### `modality_conflict` — error, structural

Two clauses that are both **binding**, both `about:` one topic, and whose modalities cannot both
hold. ADR-0003's table, with the rows reading "A is", the columns "B is":

| A \ B | MUST | MUST_NOT | SHOULD | SHOULD_NOT | MAY |
| --- | --- | --- | --- | --- | --- |
| **MUST** | — | **strong** | — | weak | — |
| **MUST_NOT** | **strong** | — | weak | — | weak |
| **SHOULD** | — | weak | — | weak | — |
| **SHOULD_NOT** | weak | — | weak | — | weak |
| **MAY** | — | weak | — | weak | — |

Two modalities collide exactly when one of them forbids and the other does not; the collision is
**strong** where both are strict rules, which is the `MUST` against the `MUST_NOT`. Every other cell
is silence — two prohibitions do not disagree, and neither do a requirement and a permission.

The finding is filed against the clause with the smaller identifier, on the line it states its
modality, with the other clause and every shared topic under `related`:

```
UZ-V-009.md:4: ERROR modality_conflict UZ-V-009: is MUST and UZ-V-010 is MUST_NOT about topic/seed-recording
  fix: revise one modality: a strict rule cannot be defeated
```

A pair sharing two topics disagrees once, over both: the topics are listed together rather than
reported twice. Binding is whatever the configuration's `binding:` projection names, which under the
`spec` preset is `effective` — every accepted, unsuperseded clause, at whatever strength it is in
force at. A clause that does not bind states nothing to collide with, so a superseded or unaccepted
clause is never half of a pair.

**Suppression.** A **weak** conflict with an `excepts` edge between the two clauses, in either
direction, is suppressed: it is left out of the report, left out of the summary counts and therefore
out of the exit code, and shown by `validate --show-suppressed` with the edge that answers it named
on the same line:

```
UZ-V-006.md:4: ERROR modality_conflict UZ-V-006: is MAY and UZ-V-008 is SHOULD_NOT about topic/inferential-grader, suppressed by excepts UZ-V-006 -> UZ-V-008 (scope: only where the run also records a calibration measure)
```

A suppressed finding carries no `fix:` — the remedy for an unanswered weak conflict is `declare
excepts: <B> in <A> with scope:, or revise one modality`, and this one already has the edge that
line asks for.

The JSON report marks such a finding `"suppressed": true`, and carries it only under the flag — the
default report is byte-identical to what it was without one. `context <ref>` shows the same line for
the clauses it is about, whatever the flag says: the exception is the reading that makes a
permission standing beside a prohibition make sense.

A **strong** conflict is never suppressed, and its fix says so rather than offering the exception:
in defeasible deontic logic a strict rule's consequence follows without exception, so no defeater
can be recorded against it. DocDag does not guess which of the two clauses is the wrong one, exactly
as `derived_conflict` does not.

The check is a pair check per topic, quadratic in the clauses that share one. Topics are meant to be
cut at the granularity a paragraph defines and two to five clauses hang off, so it is linear in
practice; `stats` reports the per-topic counts to watch that with.

### `excepts_strict` — error, structural

An `excepts` edge points at a clause whose modality is `MUST` or `MUST_NOT`: `excepts targets
UZ-V-002, which is MUST and cannot be defeated`. Filed against the document that declares the
exception, on its `excepts:` line, with the target's modality line under `related`. A defeater does
not draw a conclusion of its own — it stops a defeasible one from being drawn — and a strict rule
has nothing for it to stop. No fix suggestion: either the target's modality is wrong or the
exception is.

### `status_drift` — error, preset rule

The document has an inbound `supersedes` edge and a status other than `superseded`: `has inbound
supersedes but status is not superseded`. An absent status counts. Fix: `set status: superseded in
<path>`.

**The two presets read this name differently, on purpose.** The `adr` preset's rule is
time-independent: the moment a successor exists, the predecessor has to say `superseded`. The
`spec` preset's rule is about the day — `an in-force successor supersedes it but status is not
superseded` — so a successor that is `proposed`, in `trial`, or accepted but dated next quarter
leaves its predecessor alone until it takes over. An `adr` corpus that declares a `period:` gets
the time-independent reading until it replaces the rule; the three rules to replace it with are in
[configuration.md](configuration.md#time-dependent-status-checks).

### `superseded_orphan` — warn, preset rule

The document's status is `superseded` and nothing supersedes it: `status is superseded but no
document supersedes it`. Fix: `declare supersedes: 0002 in the replacing document, or set status:
withdrawn`.

### The `spec` preset's rules

`preset: spec` replaces the two rules above with ten of its own, written in
[configuration.md](configuration.md#the-spec-preset). Only its `status_drift` carries a fix
suggestion; the rest name a decision rather than an edit:

- `orphan_must` — error. An accepted `modality: MUST` or `modality: MUST_NOT` clause that no
  conformance test enforces: `is MUST or MUST_NOT and accepted but nothing enforces it`. Either
  write the test or drop the clause to `SHOULD` or `SHOULD_NOT`, which is what an unenforced strict
  clause carries the force of anyway.
- `orphan_test` — error. A `conform` document that declares no `enforces:`: `enforces no clause`.
- `stale_premise` — error. An accepted document one hop from a premise that has left force: `is
  accepted but a premise is no longer in force`. It reads the day the premise names under
  `retired_on`, not the word `retired` — a premise retired next month holds its clause up until next
  month. `retired` stays in the premise vocabulary for the person writing the document.
- `deviation_pressure` — warn. Five or more deviations recorded against one clause: `has 5+
  deviations; reconsider the clause`.
- `no_counterexample` — warn. An accepted clause with no `counterexample:`: `is accepted without a
  counterexample`.
- `may_without_interop` — warn. An accepted `modality: MAY` clause with no `interop:`: `is MAY but
  names no MUST clause that guarantees interoperation without it`. RFC 2119 §5 makes an option's
  interoperability a MUST-level obligation; a warning rather than an error, because for most options
  it is obvious and §6 warns against making the keywords a ritual.
- `interop_not_must` — error. Some clause an `interop:` names is not a `MUST`: `interop must point
  at a MUST clause`. The obligation the option leans on is a MUST or it is not an obligation.
- `status_drift` — error. The time-dependent reading described above: `an in-force successor
  supersedes it but status is not superseded`. Fix: `set status: superseded in <path>`.
- `pending_successor` — warn. An accepted clause with a declared successor that has not taken over —
  nobody has accepted it, or its period has not begun: `a successor is declared but not yet in
  force; this clause remains binding until then`. Nothing is wrong; the revision is in flight, and
  the clause is still what binds.
- `premature_superseded` — error. The mirror image: a clause marked `superseded` while no successor
  is in force, `status is superseded but no successor is in force yet`. Nobody is bound by either
  clause, which is a gap in the standard rather than a change in flight.

## The reference layer

The reference layer is the untyped links found in document bodies: `[[wikilink]]`,
`[[wikilink|alias]]` and relative Markdown links to other managed documents, plus the wikilinks in
frontmatter values when `references.scan` asks for them. A link joins the layer only when its whole
target is identifier-shaped under `references.pattern`, or, for a Markdown link, names a managed
file. A link inside a fenced code block or an inline code span is an example and is skipped, and
`[[upstream]]` and `[[3days-recap]]` are not references at all — neither target is wholly a digit
run.

The layer is surfaced by `--include-refs` and `stats`, is never part of a constraint, and carries no
invariants. Prose therefore cannot fail a build unless `references.dangling` opts in.

### `dangling_reference` — off by default

Severity comes from `references.dangling`: `off` (the default), `warn` or `error`. With it set, a
reference-layer link whose target is identifier-shaped but names no document reports `wikilink
reference %q does not name a document`, or `markdown` for a Markdown link. Duplicate identical links
on one line are reported once. Fix: the same `did you mean …?` as `dangling_ref`.

## History

### `immutable_violation` — error, only under `--immutable-since`

A document that was `accepted`, `superseded` or `withdrawn` at the base revision changed in a way
append-only history forbids. Its severity is fixed and it is not listed under `structural:`. The
distinct reports are:

```
the document was deleted
the document no longer parses
frontmatter key "date" was removed
frontmatter key "owner" was added
frontmatter key "title" changed
amended_by no longer lists 0004
the body changed at line 11, which append-only history forbids
```

No fix suggestion. [ci.md](ci.md) covers what the check allows and how to wire it into a workflow.

## Lint findings

`docdag lint` reports on the configuration rather than on the corpus: a rule no document could ever
fire, a rule every document fires, a rule that says what another rule already says. **None of them
is structural.** `structural:` cannot raise or lower one, their severities are fixed, and `validate`
never reports them — the health of a `docdag.yaml` and the state of a vault have different
lifecycles. See [commands.md](commands.md#lint) for the flags and the exit codes.

A lint finding is located in the configuration file, on the line the rule, projection, edge or path
constraint was written on, and at the virtual path `<preset:adr>` or `<preset:spec>` for a rule the
preset ships:

```
docdag.yaml:41: WARN never_fired deviation_pressure: fired on 0 of 128 clause documents
  fix: keep it only if a fixture under lint/ shows it can fire (docdag lint --fixtures)
```

| Finding | Severity | Layer | Trigger |
| --- | --- | --- | --- |
| `unsatisfiable_condition` | error | 1 | one alternative of a condition contradicts itself |
| `unfirable_rule` | error | 1 | every alternative of a rule contradicts itself |
| `unsatisfiable_projection` | error | 1 | every alternative of a projection contradicts itself |
| `ambivalent_fix` | error | 1 | two rules whose remedies write two values into one key can both fire |
| `tautological_rule` | warn | 1 | a rule that constrains nothing, or whose alternatives cover a whole vocabulary |
| `tautological_projection` | warn | 1 | the same, for a projection |
| `subsumed_rule` | warn | 1 | a rule that fires only where a same-or-weaker rule already fires |
| `shadowed_rule` | warn | 1 | the same, where the subsumed rule is the stronger of the two |
| `unused_edge` | warn | 1 | a declared edge type nothing in the configuration reads |
| `unused_status` | warn | 1 | a declared status vocabulary no condition reads |
| `prefer_target` | warn | 1 | a path constraint an edge's own `target:` would say one hop shorter |
| `condition_too_wide` | warn | 1 | a condition that expands past 64 alternatives, so the analysis stops |
| `never_fired` | warn, info | 2 | a rule the corpus never fires — info where a fixture shows it can |
| `always_fired` | warn | 2 | a rule every document it could apply to fires |
| `never_true` | warn, error | 2 | a projection that holds nowhere — error where `binding:` names it |
| `always_true` | warn | 2 | a projection that holds for every document it could apply to |
| `unused_edge_in_corpus` | info | 2 | a declared edge type the corpus holds no edge of |
| `newly_fired` / `stopped_firing` | info | 2 | a rule whose silence began or ended since `--since <rev>` |
| `fixture_mismatch` | error | 3 | a rule that did not fire in `ruleid/`, or fired in `ok/` |
| `missing_fixture` | warn | 3 | a rule with no `ruleid/` or no `ok/` directory |

`info` is a severity of lint's own: it never affects an exit code, `--strict` does not raise it, and
the text report prints it as `INFO`, the GitHub format as a `notice` and rdjson as `INFO`.

### Layer 1: what the configuration says about itself

Every condition is expanded into disjunctive normal form — `any_of` is the disjunction, everything
else the conjunction, and `not` is pushed down to the literals — and each conjunction is checked for
a contradiction. The judgement is a set operation over finite domains: the declared vocabularies
(`status_values`, a field's `one_of`, the kind names, a projection's `true`/`false`), the integer
window a degree clause asks for, and the kinds an edge joins. There is no solver and no search,
which is the direct return of a fixed vocabulary with no expression language in it.

### `unsatisfiable_condition` — error, lint

One alternative of a condition holds a contradiction, so no document can satisfy it: `one
alternative contradicts itself: attr status cannot be both "accepted" and "proposed"`. The reported
contradictions are

- one attribute pinned to two values, or pinned to a value the same conjunction denies,
- a value outside the vocabulary the attribute answers to — `status_values`, the kind's own where
  kinds declare one, a field's `one_of`, the kind names, `true`/`false` for a projection,
- an edge required and forbidden at once (`inbound: X` beside `not_inbound: X`),
- two degree windows that do not meet, or a threshold above the edge's own `max_inbound` or
  `max_outbound`,
- a `kind` no clause of the conjunction leaves: an `attr: {kind: ...}` against the `from:`/`to:`
  kinds of an edge the same conjunction names,
- a one-hop clause no neighbour could answer, because the edge joins it to kinds whose vocabulary
  does not hold the value it asks for,
- the same contradictions inside an edge's `target:` condition, where `leaf_of:` reads as the
  `not_inbound:` it is sugar for.

Fix: `drop the alternative or the clause that contradicts it`.

### `unfirable_rule` — error, lint

Every alternative of a rule contradicts itself, so the rule can never report anything: `every
alternative contradicts itself: <the first reason>`. It is the same analysis as above, reported
once for the whole rule rather than once per alternative. `unsatisfiable_projection` is the same
finding for a projection, and an edge target that no target could satisfy is reported as
`unsatisfiable_condition` against the edge.

### `tautological_rule` — warn, lint

A rule that fires on every document says nothing about the one it is filed against: `constrains
nothing, so it holds for every document`, or `the alternatives cover the whole vocabulary of status
(proposed, accepted, …), so it holds for every document that writes it`. `tautological_projection`
is the same for a projection. This is the vacuity model checkers look for: an antecedent that is
always true.

### `subsumed_rule` and `shadowed_rule` — warn, lint

Rule A is subsumed by rule B when every alternative of A claims everything some alternative of B
claims: A fires only where B fires, so one of the two reports nothing new. Two rules that subsume
each other are one rule written twice, reported once against the one written later. Where A is the
*stronger* of the two it is `shadowed_rule` instead: it fires only where the build has already
failed, so nobody ever reads it. A tautological or unfirable rule is left out of the comparison —
everything is subsumed by a rule that fires everywhere — because both are already reported as what
they are.

### `ambivalent_fix` — error, lint

Two rules whose `fix:` writes a value into the same frontmatter key demand two different values, and
their conditions can hold for one document at once. Only the remedies DocDag itself generates are
compared — the `set <field>: <value>` shapes of `status_drift` and `superseded_orphan` — because a
rule's own `message:` is prose, and lint does not read prose. The preset's own pair cannot both
fire: one needs an inbound `supersedes` and the other needs none.

### `unused_edge` — warn, lint

A declared edge type nothing reads. An edge is *read* when a rule, a projection, an edge `target:`
or a path constraint names it, when a `derived_edges:` entry produces it, when another edge's
`leaf_of:` walks it, when it declares an `inverse:`, endpoint kinds, a degree bound or attributes —
each of those is a structural check that reads the declaration — or when it is one of the four the
engine reads by name: `supersedes`, `depends-on`, `about` and `excepts`. What is left is an edge
that is parsed and drawn and never reasoned about, which is usually a rule that was renamed.

### `unused_status` — warn, lint

A declared status vocabulary that no rule, projection or target condition reads — for a corpus with
kinds, one kind's vocabulary at a time. The finding is about the vocabulary rather than about a
single value on purpose: a value a rule does not name still changes what the rule answers, because
`status: {eq: accepted}` answers differently for `accepted` than for every other word of the
vocabulary. What is genuinely unused is a vocabulary nothing asks about, where `unknown_status` is
the only check its words ever reach.

### `prefer_target` — warn, lint

A `path_constraints:` entry that an edge's own `target:` would say one hop shorter: a two-step
`path:` with `equals: none` says that whatever the first step reaches has no second step to take,
which is exactly `target: {not_outbound: <second>}` on the first edge — or `not_inbound:` where the
second step is walked backwards. Only that shape is reported. A one-step path is about the documents
an edge leaves rather than the one it reaches, a path whose first step is `^` reversed starts at a
document no target condition can constrain, and a `subset_of` comparison is not the empty set.

### `condition_too_wide` — warn, lint

A condition whose expansion passes 64 alternatives, or nests deeper than 16. The analysis stops
there and the rest of that rule is not judged, so a configuration cannot make lint take exponential
time. Fix: split the `any_of` nesting into separate rules.

### Layer 2: what the corpus says about the configuration

`--corpus` builds the graph exactly as `validate` does and evaluates every rule and projection over
it. `never_fired` and `always_fired` count against the documents a rule *could* apply to: the count
is narrowed to one kind wherever the condition pins one, by an `attr: {kind: ...}` clause or by
naming an edge only one kind can be at the near end of, so `fired on 0 of 128 clause documents` is
not counted against a vault of ten thousand documents of other kinds. A rule with nothing in scope
at all is reported at `info`.

`never_true` on the projection `binding:` names is an error rather than a warning: the set of
documents in force is then empty, and every command that reads it answers about nothing.

`--since <rev>` builds the corpus a second time from the files that revision holds and reports
`newly_fired` and `stopped_firing` as facts, at `info`. Both are read through `git`, so a corpus
outside a repository cannot answer them.

### Layer 3: the fixtures a rule is tested by

`--fixtures <dir>` runs each rule against two miniature corpora of its own:

```
lint/
  orphan_must/
    ruleid/       # here the rule must fire
    ok/           # here it must not
```

The names are Semgrep's. Each directory is an independent corpus read under the same `docdag.yaml`
as the vault, with the documents directories rerooted onto it: for a corpus that declares no kinds
the fixture directory *is* the documents directory, and for one that declares `kinds:` the fixture
holds the same kind-relative layout — `lint/orphan_must/ruleid/spec/clauses/UZ-V-900.md`. A kind the
fixture does not hold is read as empty rather than as an error, so a fixture holds only the kinds
its rule is about.

A fixture may be written for any rule, any projection and any structural check: what "fires" means
is a finding of that name, or a projection that holds somewhere. A suppressed finding has not fired,
which is what lets an `ok/` fixture record an exception and show the suppression working.

`fixture_mismatch` is filed on the fixture — the document that fired, or the directory the rule was
silent in — and relates the rule's own line in the configuration. `missing_fixture` asks for the two
directories, and exempts the rules a preset ships, because DocDag ships their fixtures with them.

Together the two layers answer the question a silent check leaves open. A fixture proves the rule
*can* fire; the corpus shows it is *not* firing now. A `never_fired` rule whose `ruleid/` fixture
passes falls to `info` for exactly that reason.

## The fixture corpora

This repository ships one corpus per failure mode under `testdata/fixtures`, next to the clean
`ok-madr` and `ok-basic` used by the README's quickstart. From a checkout:

```console
$ docdag validate --dir testdata/fixtures/status-drift
testdata/fixtures/status-drift/0001-serve-images-from-the-application-server.md:3: ERROR status_drift 0001: has inbound supersedes but status is not superseded
  fix: set status: superseded in testdata/fixtures/status-drift/0001-serve-images-from-the-application-server.md
```

That run exits 1. The other directories are named for the finding or the behaviour they exercise —
`cycle`, `union-cycle`, `union-cycle-shadowed`, `superseded-orphan`, `id-collision`, `dangling`,
`dangling-reference`, `empty-edge`, `invalid-yaml`, `inverse-mismatch`, `cardinality`, `withdrawn`,
`any-of`, `list-attrs`, `fan-in`, `depends-impact`, `projections`, `edge-attrs`, `target`,
`path-constraints`, `kinds`, `spec-vault`. The last six carry a `docdag.yaml` of their own, so run
them with `--config <dir>/docdag.yaml`.

`target` is the corpus whose edges declare what they may point at: a `depends-on` left on a
replaced decision and an `amends` on a deprecated one, one `stale_target` each. `path-constraints`
states the same sort of invariant over two composed edges instead, and carries one violation of
each comparison:

```console
$ docdag validate --dir testdata/fixtures/path-constraints --config testdata/fixtures/path-constraints/docdag.yaml
```

`kinds` is the multi-kind corpus: clauses, conformance tests and deviations in three directories,
carrying one of each kind finding. Its documents live under the kinds rather than in the fixture
directory itself, so it is run by `--config` alone:

```console
$ docdag validate --config testdata/fixtures/kinds/docdag.yaml
```

`spec-vault` is a small corpus under the `spec` preset — twenty-six documents across the eight
kinds, configured by `preset: spec` and nothing else. Five of its eight findings come from a preset
rule; the other three are structural: a conformance test still enforcing the clause its successor
replaced, which is the `stale_target` the preset's `target:` conditions are there for, a `MUST`
standing against a `MUST_NOT` about one subject, which is the strong `modality_conflict`, and a
departure recorded until a day that has passed, which is `expired_deviation`:

```console
$ docdag validate --config testdata/fixtures/spec-vault/docdag.yaml
```

It carries one more conflict that is not in that report: `UZ-V-006` permits what `UZ-V-008` says not
to do, and the vault records the exception, so the finding is suppressed. `--show-suppressed` is
what reads it, and `docdag context UZ-V-006` is where the whole neighbourhood — the subject, the
exception, the interoperation requirement — is written out.

It carries the day, too: `UZ-V-011` is the revision of `UZ-V-003` and is still in `trial`, so
nothing has taken over and `UZ-V-003` keeps binding — which is the `pending_successor` warning, a
change in flight rather than a fault. `premise/hand-written-notes-are-enough` names the day it
stopped holding under `retired_on:`, which is what `stale_premise` reads, and `dev-0001` names an
`expires:` that has passed. `docdag query --binding --as-of <day>` walks the same corpus at another
day.

### The lint fixtures

`testdata/lint/adr` and `testdata/lint/spec` are the per-rule fixtures of the two presets, one
directory per rule and per structural check, each holding a `ruleid/` corpus and an `ok/` one. They
are run by DocDag's own test suite, so a preset edit that stops a rule from firing — or starts it
firing where it should not — fails here rather than in somebody's repository:

```console
$ docdag lint --config testdata/lint/spec/docdag.yaml --fixtures testdata/lint/spec
OK: no lint findings
```

They double as documentation: `testdata/lint/spec/orphan_must/ruleid` is the smallest corpus that
`orphan_must` reports, and its `ok/` is the same corpus with the conformance test that answers it.
