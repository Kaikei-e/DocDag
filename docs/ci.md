# Continuous integration

`docdag validate` exits 1 on any error, so one step gates a repository. See
[commands.md](commands.md) for the output formats and [checks.md](checks.md) for what each finding
means.

## The composite action

The composite action downloads the release binary for the runner, verifies it against the release's
`checksums.txt` before running it, and needs no Go toolchain:

```yaml
name: decisions
on: [push, pull_request]

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: Kaikei-e/DocDag@v0.3.0
        with:
          version: v0.3.0                  # default: latest
          args: validate --format github   # this is the default
          working-directory: .             # this is the default
```

| Input | Default | Meaning |
| --- | --- | --- |
| `version` | `latest` | the release tag to install; `latest` resolves to the newest release |
| `args` | `validate --format github` | the arguments passed to `docdag` |
| `working-directory` | `.` | the directory `docdag` runs in |

The action runs on Linux and macOS runners, amd64 and arm64. On a Windows runner it fails with a
diagnostic rather than passing silently; install with `go install` there instead.

### Pinning

A tag can be moved, so pin the action by commit SHA when the supply chain matters —
`uses: Kaikei-e/DocDag@<40-char-sha> # v0.3.0`. This repository pins the actions it uses that way.

### Annotations

`--format github` writes one workflow command per finding. A workflow step renders at most ten
annotations, so a corpus with more findings than that needs a second `--format text` run written
into `$GITHUB_STEP_SUMMARY` to show the rest:

```yaml
      - uses: Kaikei-e/DocDag@v0.3.0        # annotations, and the gate
      - if: always()
        shell: bash
        run: docdag validate --format text >> "$GITHUB_STEP_SUMMARY"
```

## Append-only history

`docdag validate --immutable-since <rev>` treats a decision that was `accepted`, `superseded` or
`withdrawn` at `<rev>` as a record rather than a draft. It compares the working tree against
`git merge-base <rev> HEAD` and allows exactly three kinds of change to such a document:

- the value of the `status_field`,
- entries **added** under a configured `inverse:` key, so a later decision can record that it
  replaced this one,
- lines appended to the end of the body.

Anything else — another frontmatter key changed, added or removed; an inverse entry removed; a line
of the existing body rewritten; the file deleted — is an `immutable_violation` error naming what
changed. Renames are not followed, so moving a closed document reads as the deletion it is:

```console
$ docdag validate --immutable-since origin/main
docs/adr/0001-serve-images-from-the-application.md:11: ERROR immutable_violation 0001: the body changed at line 11, which append-only history forbids
```

A document that `<rev>` did not hold is new and is always allowed. The check runs `git` from `PATH`
and is off unless the flag is given; a corpus outside a git repository, or a machine without `git`,
exits 3.

### Running it in CI

The check needs enough history for `git merge-base <rev> HEAD` to resolve, which a shallow clone
does not have. Ask for the full history and name the branch the change is proposed against:

```yaml
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: Kaikei-e/DocDag@v0.3.0
        with:
          args: validate --immutable-since origin/${{ github.base_ref || github.event.repository.default_branch }}
```

`github.base_ref` is the branch a pull request targets and is empty on a push, where the default
branch is the useful comparison.

## Periodic runs

`validate` and `lint --corpus` answer for the day HEAD was committed on, so one commit gates the
same way however long afterwards the job runs — a corpus that passed yesterday cannot fail today
because a date rolled over. That is what a gate needs, and it is exactly why an expiry the corpus
declared is not noticed until the next commit touches the repository.

Where a corpus declares a [`period:`](configuration.md#periods-and-as-of), pair the gate with a
scheduled run that says out loud that it is asking about today:

```yaml
name: expiries
on:
  schedule:
    - cron: "0 6 * * 1"          # Mondays, 06:00 UTC

jobs:
  as-of-today:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Say which day this run asks about
        shell: bash
        run: echo "DOCDAG_AS_OF=$(date -I)" >> "$GITHUB_ENV"
      - uses: Kaikei-e/DocDag@v0.3.0
        with:
          args: validate --format github
```

The day goes through the environment rather than through `args:`. A `with:` value is a string the
action hands to `docdag` as written — nothing evaluates a `$(…)` in it, so `--as-of $(date -I)`
would reach the flag parser as the four words it is and fail with `unknown shorthand flag: 'I'`.
`$DOCDAG_AS_OF` is read by every command that takes `--as-of`, so one `$GITHUB_ENV` line also names
the day for a whole pipeline where several commands run together, and the flag still wins wherever a
step writes one.

The scheduled run is what turns `expired_deviation`, a premise past its `retired_on` and a successor
that has come into force into findings on the day they happen, rather than on the day somebody
happens to commit.

Reproducing a past answer is the other direction: `--at <rev>` reads every managed document from a
revision, and the two flags compose — `docdag query --binding --at v1.2.0 --as-of 2026-06-01` is
what the vault at that release said was in force that day, which is the question an incident review
asks.

## Linting the configuration

`docdag lint` answers about `docdag.yaml` rather than about the documents, and `validate` never runs
it: a lint warning on every pull request is a warning nobody reads. Run it where the configuration
or the fixtures change, and on a schedule for the corpus layer:

```yaml
name: decisions
on: [push, pull_request]

jobs:
  changes:
    runs-on: ubuntu-latest
    outputs:
      config: ${{ steps.filter.outputs.config }}
    steps:
      - uses: actions/checkout@v4
      - uses: dorny/paths-filter@v3
        id: filter
        with:
          filters: |
            config:
              - docdag.yaml
              - lint/**

  validate:                                    # every pull request
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: Kaikei-e/DocDag@v0.3.0

  lint:                                        # only where the rules changed
    needs: changes
    if: needs.changes.outputs.config == 'true'
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: Kaikei-e/DocDag@v0.3.0
        with:
          args: lint --all --format github
```

`lint --all` runs every layer: the configuration alone, the rules against the current vault, and
each rule's own `ruleid/` and `ok/` fixtures under `lint/`. It exits 1 on any error, 2 on warnings
alone and 3 on a configuration that does not validate, so a repository that wants the warnings to
gate adds `--strict` and one that does not lets the 2 through:

```yaml
        with:
          args: lint --all --strict --format github
```

The corpus layer answers about a vault that changes under it — a rule that fires nowhere today may
be the one that catches tomorrow's mistake — so it is worth a scheduled run of its own:

```yaml
on:
  schedule:
    - cron: "0 6 * * 1"
jobs:
  lint:
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0
      - uses: Kaikei-e/DocDag@v0.3.0
        with:
          args: lint --corpus --since origin/main --format text
```

`--since` needs enough history for `git merge-base` to resolve, exactly as `--immutable-since` does.
[commands.md](commands.md#lint) covers the flags and [checks.md](checks.md#lint-findings) every
finding.

## The pre-commit hook

A [pre-commit](https://pre-commit.com) hook is also shipped:

```yaml
repos:
  - repo: https://github.com/Kaikei-e/DocDag
    rev: v0.3.0
    hooks:
      - id: docdag-validate      # any .md or docdag.yaml edit: the invariants
      - id: docdag-lint          # a docdag.yaml edit: the rules themselves
```

Two hooks, because a configuration change and a document change break different things. `docdag-lint`
runs on a `docdag.yaml` edit alone and answers about the rules — layer 1 only, which reads no
documents and costs milliseconds; the corpus and fixture layers belong in CI. Install one without the
other and the other question goes unasked.

Hooks are advisory: `git commit --no-verify` walks straight past them, and a contributor who never
ran `pre-commit install` never had them. CI is the gate; the hook only shortens the loop.

The plugin ships a `PostToolUse` hook for agents as well — see [agents.md](agents.md).
