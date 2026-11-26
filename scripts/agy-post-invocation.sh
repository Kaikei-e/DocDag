#!/bin/sh
# Antigravity PostInvocation hook: atomically drain any pending validation
# reports or corpus hints queued by agy-post-tool.sh and inject them as a
# systemMessage. Always outputs valid JSON on stdout and exits 0.
set -u

payload=$(cat)

if ! command -v jq >/dev/null 2>&1; then
    echo '{}'
    exit 0
fi

raw_conv_id=$(printf '%s' "$payload" | jq -r '.conversationId // empty' 2>/dev/null)
conv_id=$(printf '%s' "$raw_conv_id" | tr -cd 'A-Za-z0-9_-')
[ -n "$conv_id" ] || { echo '{}'; exit 0; }

state_dir="${PLUGIN_DATA:-${TMPDIR:-/tmp}/docdag-agy-hooks}"
pending_file="$state_dir/${conv_id}.pending"

if [ ! -s "$pending_file" ]; then
    echo '{}'
    exit 0
fi

claimed="$pending_file.$$"
mv "$pending_file" "$claimed" 2>/dev/null || { echo '{}'; exit 0; }

msg=$(cat "$claimed" 2>/dev/null)
rm -f "$claimed"

if [ -z "$msg" ]; then
    echo '{}'
    exit 0
fi

jq -n --arg msg "$msg" '{injectSteps:[{systemMessage:{systemMessage:$msg}}]}'
exit 0
