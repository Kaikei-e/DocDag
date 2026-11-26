#!/bin/sh
# SessionStart hook: orient the agent with docdag prime at session start, or
# fall back to a compact validation summary on older docdag binaries.
# Silent when the project has no corpus, and always exits 0.
set -u

root=${CLAUDE_PROJECT_DIR:-$PWD}
cd "$root" 2>/dev/null || exit 0

has_corpus=0
if [ -f docdag.yaml ]; then
    has_corpus=1
else
    for dir in docs/adr doc/adr docs/decisions docs/ADR adr; do
        if [ -d "$dir" ]; then
            has_corpus=1
            break
        fi
    done
fi
[ "$has_corpus" -eq 1 ] || exit 0

if ! command -v docdag >/dev/null 2>&1; then
    echo "docdag: not on PATH, enforcement is OFF"
    exit 0
fi

err_file=$(mktemp 2>/dev/null || echo "${TMPDIR:-/tmp}/docdag-session-start-$$.err")
trap 'rm -f "$err_file"' EXIT INT HUP TERM

print_exit_err() {
    first_err=""
    if [ -s "$2" ]; then
        first_err=$(head -n 1 "$2")
    fi
    if [ -n "$first_err" ]; then
        printf 'docdag validate: exited %d: %s\n' "$1" "$first_err"
    else
        printf 'docdag validate: exited %d\n' "$1"
    fi
}

report=$(docdag prime 2>"$err_file")
status=$?

case $status in
    0)
        if [ -n "$report" ]; then
            printf '%s\n' "$report"
        fi
        exit 0
        ;;
    2)
        if ! grep -q 'unknown command "prime"' "$err_file" 2>/dev/null; then
            print_exit_err "$status" "$err_file"
            exit 0
        fi
        # Older docdag binaries lack prime; fall back to validate summary.
        ;;
    *)
        print_exit_err "$status" "$err_file"
        exit 0
        ;;
esac

report=$(docdag validate 2>"$err_file")
status=$?

case $status in
    0)
        summary=$(printf '%s\n' "$report" | awk 'NF { line = $0 } END { print line }')
        if [ -n "$summary" ]; then
            printf 'docdag validate: %s\n' "$summary"
        else
            printf 'docdag validate: OK\n'
        fi
        ;;
    1)
        # On failure (summary.Errors > 0), docdag validate omits the OK summary
        # line (emitting only findings, fixes, and optionally "as of <day>").
        # Summarize the reported error and warning counts onto a single line.
        summary=$(printf '%s\n' "$report" | awk '
            /(^|: )ERROR / { errs++ }
            /(^|: )WARN / { warns++ }
            /^as of / { asof = $0 }
            END {
                if (errs == 0 && warns == 0) {
                    print "FAILED"
                } else {
                    msg = sprintf("FAILED: %d %s", errs, errs == 1 ? "error" : "errors")
                    if (warns > 0) {
                        msg = msg sprintf(", %d %s", warns, warns == 1 ? "warning" : "warnings")
                    }
                    if (asof != "") {
                        msg = msg ", " asof
                    }
                    print msg
                }
            }
        ')
        printf 'docdag validate: %s\n' "$summary"
        printf 'run docdag validate to see the findings\n'
        ;;
    *)
        print_exit_err "$status" "$err_file"
        ;;
esac

exit 0
