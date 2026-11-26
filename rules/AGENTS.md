# DocDag — Decision & Specification Graph

When the workspace contains `docdag.yaml` or a decision/spec directory (`docs/adr`, `docs/ADR`, `docs/decisions`, `spec/`) and `docdag` is on `PATH`:

## Query before reading files

- Do NOT scan, glob, or read whole ADR or spec directories to discover active decisions or follow links — ask `docdag` first.
- `docdag prime` — inspect corpus layout, declared fields, required sections, and validation health.
- `docdag context <ref>` — read a document, its resolved successor, typed-edge neighbors, and excerpt in one call (accepts ID or managed file path).
- `docdag query --binding --fields id,title,status` — list all active/binding records with titles (add `--where <key>=<op>:<val>` for declared fields).
- `docdag resolve <ref> --fields id,title,status` — find the current binding successor of a superseded record.
- `docdag query <ref> --ancestors|--descendants --fields id,title,status` — impact and dependency analysis.

## Creating and editing records

- `docdag new "<title>" [--supersedes <ref>]` (or `--kind <kind> --id <id>`) — create or supersede a record without renumbering.
- After editing a managed Markdown file, run `docdag validate --touching <path>`.
- After editing `docdag.yaml`, run `docdag lint`.
- Never edit document IDs or renumber existing records.
