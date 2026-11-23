#!/usr/bin/env bash
# Stands in for the claude CLI. FAKE_CLAUDE_MODE picks the shape: good (default), truncated (review text with no
# terminator, the shape of a transcript that died mid-stream), payload (emit {{DIR}}/review-payload verbatim),
# bad-version (--version exits 7), slow (a valid review after a 3 second wait), and hang (records its pid in {{DIR}}/claude-leader.pid, then sleeps 30s).
mode="${FAKE_CLAUDE_MODE:-good}"
record() {
    printf 'ANTHROPIC_API_KEY=%s|OPENAI_API_KEY=%s|CODEX_HOME=%s|HOST_SESSION=%s|PLUGIN_ROOT=%s|CLAUDE_PLUGIN_ROOT=%s\n' \
        "${ANTHROPIC_API_KEY-<unset>}" "${OPENAI_API_KEY-<unset>}" "${CODEX_HOME-<unset>}" \
        "${CLAUDE_CODE_SESSION_ACCESS_TOKEN-<unset>}" "${PLUGIN_ROOT-<unset>}" "${CLAUDE_PLUGIN_ROOT-<unset>}" \
        >> "{{DIR}}/claude-environment"
}
if [ "${1:-}" = "--version" ]; then
    echo version >> "{{DIR}}/claude-invocations"
    record
    [ "$mode" = bad-version ] && exit 7
    echo fake 1.0
    exit 0
fi
echo dispatch >> "{{DIR}}/claude-invocations"
record
printf '%s\n' "$*" > "{{DIR}}/claude-argv"
cat >/dev/null
if [ "$mode" = slow ]; then sleep 3; fi
if [ "$mode" = hang ]; then
    echo "$$" > "{{DIR}}/claude-leader.pid"
    sleep 30
fi
if [ "$mode" = payload ]; then
    cat "{{DIR}}/review-payload"
    exit 0
fi
echo "Fake claude review. No findings."
if [ "$mode" != truncated ]; then echo "REVIEW-COMPLETE: no-findings"; fi
