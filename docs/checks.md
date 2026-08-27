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

`structural:` accepts exactly these twelve names: `cycle`, `dangling_ref`, `id_collision`,
`invalid_frontmatter`, `missing_frontmatter`, `unknown_status`, `derived_conflict`,
`unstructured_supersedes`, `invalid_ref`, `empty_edge`, `inverse_mismatch`, `cardinality`. Naming
anything else — `status_drift` and `superseded_orphan` included, since they are rules — is a
configuration error and exits 3.

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
skipped in silence. Fix: `add a YAML frontmatter block with title and status`.

Raise it with `structural: {missing_frontmatter: error}` once every document is expected to carry
one.

### `id_collision` — error, structural

Two or more files normalize to the same identifier: `shares its identifier with <paths>`. The
finding is filed against the first path and names the rest under `related`. Identity is the file's
digit run, so `0004-a.md` and `0004-b.md` collide. No fix suggestion; `docdag new` refuses to run at
all while a corpus carries one.

### `unknown_status` — error, structural

`status_values` is configured, the document's status is not blank, and the value does not fold onto
the vocabulary: `status %q is outside the vocabulary <values>`. Comparison is case-insensitive, and
a value like `superseded by 0003` is accepted when a `derived_edges:` pattern claims it. Fix: `use
one of: <values>`.

`withdrawn` is in the preset vocabulary for a proposal that was dropped rather than replaced: it
binds nothing, and because nothing supersedes it, it raises no `superseded_orphan` warning either.

Unrecognized frontmatter keys are ignored entirely, so another tool's fields raise nothing.

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
`any-of`, `list-attrs`, `fan-in`, `depends-impact`.
