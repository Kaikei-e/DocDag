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

`structural:` accepts exactly these twenty names: `cycle`, `dangling_ref`, `id_collision`,
`invalid_frontmatter`, `missing_frontmatter`, `unknown_status`, `derived_conflict`,
`unstructured_supersedes`, `invalid_ref`, `empty_edge`, `inverse_mismatch`, `cardinality`,
`edge_attr_unknown`, `edge_attr_missing`, `edge_attr_invalid`, `id_mismatch`, `kind_mismatch`,
`unknown_field`, `edge_kind_mismatch`, `deprecated_field`. Naming anything else — `status_drift`
and `superseded_orphan` included, since they are rules — is a configuration error and exits 3.

## Status is a projection

Status is a projection of the graph, not an independent fact. With the ADR preset, an inbound
`supersedes` edge and a status other than `superseded` is an error (`status_drift`); `superseded`
with nothing superseding it is a warning (`superseded_orphan`). A document is *binding* when its
status is `accepted` and no document supersedes it.

Both are ordinary rules, so a configuration that writes its own `rules:` list replaces them. The
`fix:` suggestions are keyed on these two names: a rule renamed keeps its check but loses its
suggestion.

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

## Field lifecycle

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
measures reference "0001" attribute "expires" is "soon", want a date as YYYY-MM-DD
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

### `status_drift` — error, preset rule

The document has an inbound `supersedes` edge and a status other than `superseded`: `has inbound
supersedes but status is not superseded`. An absent status counts. Fix: `set status: superseded in
<path>`.

### `superseded_orphan` — warn, preset rule

The document's status is `superseded` and nothing supersedes it: `status is superseded but no
document supersedes it`. Fix: `declare supersedes: 0002 in the replacing document, or set status:
withdrawn`.

### The `spec` preset's rules

`preset: spec` replaces the two rules above with five of its own, written in
[configuration.md](configuration.md#the-spec-preset) and carrying no fix suggestion — each names a
decision rather than an edit:

- `orphan_must` — error. An accepted `level: MUST` clause that no conformance test enforces: `is
  MUST and accepted but nothing enforces it`. Either write the test or drop the clause to `SHOULD`.
- `orphan_test` — error. A `conform` document that declares no `enforces:`: `enforces no clause`.
- `stale_premise` — error. An accepted document one hop from a `retired` premise: `is accepted but a
  premise is retired`.
- `deviation_pressure` — warn. Five or more deviations recorded against one clause: `has 5+
  deviations; reconsider the clause`.
- `no_counterexample` — warn. An accepted clause with no `counterexample:`: `is accepted without a
  counterexample`.

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
`any-of`, `list-attrs`, `fan-in`, `depends-impact`, `projections`, `edge-attrs`, `kinds`,
`spec-vault`. The last four carry a `docdag.yaml` of their own, so run them with
`--config <dir>/docdag.yaml`.

`kinds` is the multi-kind corpus: clauses, conformance tests and deviations in three directories,
carrying one of each kind finding. Its documents live under the kinds rather than in the fixture
directory itself, so it is run by `--config` alone:

```console
$ docdag validate --config testdata/fixtures/kinds/docdag.yaml
```

`spec-vault` is a small corpus under the `spec` preset — thirteen documents across the seven kinds,
configured by `preset: spec` and nothing else. Nothing structural is wrong with it; every finding it
carries comes from a preset rule:

```console
$ docdag validate --config testdata/fixtures/spec-vault/docdag.yaml
```
