#!/bin/sh
# Antigravity SessionStart hook: inject corpus prime output (or a compact
# validate summary on older docdag binaries) when a corpus is present.
# Always outputs valid JSON on stdout and exits 0.
set -u

payload=$(cat)

has_jq=0
if command -v jq >/dev/null 2>&1; then
    has_jq=1
    root=$(printf '%s' "$payload" | jq -r '.cwd // .workspacePaths[0] // empty' 2>/dev/null)
else
    root=$PWD
fi

[ -n "$root" ] || { echo '{}'; exit 0; }
cd "$root" 2>/dev/null || { echo '{}'; exit 0; }

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
[ "$has_corpus" -eq 1 ] || { echo '{}'; exit 0; }

emit_message() {
    msg=$1
    if [ "$has_jq" -eq 1 ]; then
        jq -n --arg msg "$msg" '{injectSteps:[{systemMessage:{systemMessage:$msg}}]}'
    else
        escaped=$(printf '%s' "$msg" | awk '
            BEGIN { ORS = "" }
            {
                gsub(/\\/, "\\\\")
                gsub(/"/, "\\\"")
                gsub(/\t/, "\\t")
                gsub(/\r/, "\\r")
                if (NR > 1) printf "\\n"
                printf "%s", $0
            }
        ')
        printf '{"injectSteps":[{"systemMessage":{"systemMessage":"%s"}}]}\n' "$escaped"
    fi
}

if ! command -v docdag >/dev/null 2>&1; then
    emit_message "docdag: not on PATH, enforcement is OFF"
    exit 0
fi

err_file=$(mktemp 2>/dev/null || echo "${TMPDIR:-/tmp}/docdag-agy-session-start-$$.err")
trap 'rm -f "$err_file"' EXIT INT HUP TERM

report=$(docdag prime 2>"$err_file")
status=$?

if [ "$status" -eq 0 ]; then
    if [ -n "$report" ]; then
        emit_message "$report"
    else
        echo '{}'
    fi
    exit 0
fi

if [ "$status" -eq 2 ] && grep -q 'unknown command "prime"' "$err_file" 2>/dev/null; then
    report=$(docdag validate 2>"$err_file")
    vstatus=$?
    case $vstatus in
        0)
            summary=$(printf '%s\n' "$report" | awk 'NF { line = $0 } END { print line }')
            if [ -n "$summary" ]; then
                emit_message "docdag validate: $summary"
            else
                emit_message "docdag validate: OK"
            fi
            ;;
        1)
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
            emit_message "$(printf 'docdag validate: %s\nrun docdag validate to see the findings' "$summary")"
            ;;
        *)
            first_err=""
            if [ -s "$err_file" ]; then
                first_err=$(head -n 1 "$err_file")
            fi
            if [ -n "$first_err" ]; then
                emit_message "docdag validate: exited $vstatus: $first_err"
            else
                emit_message "docdag validate: exited $vstatus"
            fi
            ;;
    esac
    exit 0
fi

first_err=""
if [ -s "$err_file" ]; then
    first_err=$(head -n 1 "$err_file")
fi
if [ -n "$first_err" ]; then
    emit_message "docdag validate: exited $status: $first_err"
else
    emit_message "docdag validate: exited $status"
fi
exit 0
