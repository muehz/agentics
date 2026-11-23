#!/usr/bin/env bash
# flow.sh <command> [arguments]
#
#   log <phase> <event> <target> <status> <why> <evidence> <next> [--folder <dir>] [--expect-blob <hash>]
#   status [--folder <dir>] [--metrics]
#   where
#   bind [--folder <dir>]
#
# Appends rows to, and reads, a work item's append-only state.tsv. The record
# contract is skills/execute/references/flow-state.md. Exit codes: 0 success,
# 1 usage, 2 invalid cell, 3 no flow found, 4 more than one flow matched,
# 5 (where only) this branch is a small fix in progress, with no work folder.
#
# Written for bash 3.2 (the macOS default): indexed arrays and awk only.
set -euo pipefail

PHASES="intent spec decompose execute gate"
EVENTS="create challenge approve revoke diverge verify revert demote block unblock pivot waive branch release change skip na enter bind pause done review escape"
STATUSES="ok failed blocked pending declined cancelled"
REVIEW_KINDS="plan packet quality integration"
# Value forms of review and escape evidence. They live in variables because
# bash 3.2 matches a quoted [[ =~ ]] pattern as a literal string.
RE_PAIR='^[a-z-]+=[^ ]+$'
RE_TS='^20[0-9]{2}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$'
RE_ROUND='^[1-9][0-9]*$'
RE_BLOCKED='^(0|[1-9][0-9]*)$'
VERSION_LINE="# flow-state 1"
TAB=$'\t'
HEADER="ts${TAB}session${TAB}phase${TAB}event${TAB}target${TAB}status${TAB}why${TAB}evidence${TAB}next"
# The skill prefixes a call with FLOW_WORK_ROOT=<dir> when AGENTS.md or
# CLAUDE.md names another work root. The value is relative to the repository.
ROOT="${FLOW_WORK_ROOT:-docs/specs}"
top=""
RESOLVED_FOLDER=""

usage() {
  cat <<'USAGE'
Usage: flow.sh log <phase> <event> <target> <status> <why> <evidence> <next> [--folder <dir>] [--expect-blob <hash>]
       flow.sh status [--folder <dir>] [--metrics]
       flow.sh where
       flow.sh bind [--folder <dir>]

log     appends one row to <dir>/state.tsv (the bound flow when --folder is omitted).
        evidence "auto" stores blob=<hash of the approved content> on an approve row
        and commit=<HEAD> patch-id=<id> on a verify, revert, or diverge row.
        "auto:<sha>" names a commit other than HEAD.
        --expect-blob <hash> applies to an approve row with evidence "auto" only; it
        writes the row only if the artifact's current blob hash equals <hash>.
status  prints the flow summary; --metrics adds seven measures.
where   prints the bound flow's folder.
bind    appends a bind row naming the current branch, once; a second call writes
        nothing. Never on the default branch, a detached HEAD, or a small fix.

Closed sets:
  phase   intent spec decompose execute gate
  event   create challenge approve revoke diverge verify revert demote block unblock
          pivot waive branch release change skip na enter bind pause done
          review escape
  status  ok failed blocked pending declined cancelled
  next    first word is a phase name or done

review evidence  kind=<plan|packet|quality|integration> dispatched=<ts> round=<n>
                 blocked=<seconds>; status ok or failed
escape evidence  found-by=<gate|ci|copilot|person> severity=<critical|important>
                 review=<kind>; status failed

Environment: FLOW_WORK_ROOT (default docs/specs), FLOW_SESSION (default
CLAUDE_CODE_SESSION_ID, else -).
Exit codes: 0 ok, 1 usage, 2 invalid cell, 3 no flow found, 4 more than one
flow, 5 small fix in progress (where only).
USAGE
}

in_set() {
  local needle="$1" list="$2" w
  for w in $list; do [ "$w" = "$needle" ] && return 0; done
  return 1
}

strip_cell() { printf '%s' "$1" | tr '\t\n\r' '   '; }

trim() {
  local v="$1"
  v="${v#"${v%%[![:space:]]*}"}"
  v="${v%"${v##*[![:space:]]}"}"
  printf '%s' "$v"
}

# clean strips tab/CR/LF, trims, then applies the formula-injection guard: a
# cell starting with =, +, -, @, or a single quote gets one leading quote. The
# trim comes first so the character the guard tests is the character a
# spreadsheet will see.
clean() {
  local v
  v="$(trim "$(strip_cell "$1")")"
  case "$v" in
    =* | +* | -* | @* | "'"*) printf "'%s" "$v" ;;
    *) printf '%s' "$v" ;;
  esac
}

fail() {
  echo "flow.sh: invalid $1: '$2' -- $3" >&2
  exit 2
}

# validate_evidence checks the status and evidence of a review or escape row
# and exits 2 on any invalid form; other events return at once. The evidence is
# exactly the event's keys, each once, as key=value pairs split by one space.
validate_evidence() {
  local event="$1" status="$2" evidence="$3" keys pair key value seen=" " want=0
  local -a pairs
  case "$event" in
    review)
      in_set "$status" "ok failed" || fail status "$status" "a review row is ok or failed"
      keys="kind dispatched round blocked"
      want=4
      ;;
    escape)
      [ "$status" = "failed" ] || fail status "$status" "an escape row is failed"
      keys="found-by severity review"
      want=3
      ;;
    *) return 0 ;;
  esac
  case "$evidence" in
    *"  "*) fail evidence "$evidence" "an empty pair; split key=value pairs with one space" ;;
  esac
  IFS=' ' read -r -a pairs <<< "$evidence"
  [ "${#pairs[@]}" -eq "$want" ] || fail evidence "$evidence" "$event evidence is exactly: $keys"
  for pair in "${pairs[@]}"; do
    [[ "$pair" =~ $RE_PAIR ]] || fail evidence "$evidence" "'$pair' is not a key=value pair"
    key="${pair%%=*}"
    value="${pair#*=}"
    in_set "$key" "$keys" || fail evidence "$evidence" "unknown key $key; expected: $keys"
    if in_set "$key" "$seen"; then fail evidence "$evidence" "key $key repeats"; fi
    seen="$seen$key "
    case "$key" in
      kind | review) in_set "$value" "$REVIEW_KINDS" || fail evidence "$evidence" "$key must be one of: $REVIEW_KINDS" ;;
      dispatched) [[ "$value" =~ $RE_TS ]] || fail evidence "$evidence" "dispatched must be an ISO 8601 UTC ts like 2026-10-30T10:00:00Z" ;;
      round) [[ "$value" =~ $RE_ROUND ]] || fail evidence "$evidence" "round must be a positive integer" ;;
      blocked) [[ "$value" =~ $RE_BLOCKED ]] || fail evidence "$evidence" "blocked must be a whole number of seconds" ;;
      found-by) in_set "$value" "gate ci copilot person" || fail evidence "$evidence" "found-by must be one of: gate ci copilot person" ;;
      severity) in_set "$value" "critical important" || fail evidence "$evidence" "severity must be one of: critical important" ;;
    esac
  done
}

first_token() {
  local t=""
  read -r t _ <<< "$1" || true
  printf '%s' "$t"
}

# ---------------------------------------------------------------------------
# Reading: a file is a set of rows ordered by ts (file order breaks ties).
# ---------------------------------------------------------------------------

# rows_of prints a state.tsv's rows as a reader sees them: the version line and
# header row dropped (also when repeated by a union merge), identical rows once,
# ordered by ts. A rebase replay or a union merge changes nothing it prints.
rows_of() {
  tail -n +3 "$1" | awk -F'\t' 'NF >= 9 && $1 != "ts"' |
    LC_ALL=C sort -s -t "$TAB" -k1,1 | awk '!seen[$0]++'
}

last_event() { rows_of "$1" | awk -F'\t' '{ e = $4 } END { print e }'; }

is_active() {
  local e
  [ -f "$1" ] || return 1
  e="$(last_event "$1")"
  [ -n "$e" ] && [ "$e" != "done" ]
}

# bind_ts prints the ts of the latest bind row naming the branch, or nothing.
# A bind row stores the branch as clean wrote it, formula guard included, so
# the lookup compares against that same encoding.
bind_ts() {
  local stored
  stored="$(clean "$2")"
  rows_of "$1" | awk -F'\t' -v b="$stored" '$4 == "bind" && $5 == b { t = $1 } END { print t }'
}

has_bind() { rows_of "$1" | awk -F'\t' '$4 == "bind" { f = 1 } END { exit f ? 0 : 1 }'; }

repo_top() { top="$(git rev-parse --show-toplevel 2> /dev/null || true)"; }

# all_flows prints the absolute folder of every <root>/<month>/<name>/state.tsv.
# <root>/unassigned/state.tsv sits one level up and is never a flow.
all_flows() {
  local f
  for f in "$top/$ROOT"/*/*/state.tsv; do
    if [ -f "$f" ]; then dirname "$f"; fi
  done
  return 0
}

current_branch() { git -C "$top" symbolic-ref --quiet --short HEAD 2> /dev/null || true; }

# default_ref prints the ref this checkout compares against: origin/HEAD's
# target (a remote tip, not a possibly stale local branch), else main, else
# master.
default_ref() {
  local ref b
  if ref="$(git -C "$top" symbolic-ref --quiet --short refs/remotes/origin/HEAD 2> /dev/null)"; then
    printf '%s' "$ref"
    return 0
  fi
  for b in main master; do
    if git -C "$top" show-ref --verify --quiet "refs/heads/$b"; then
      printf '%s' "$b"
      return 0
    fi
  done
  return 0
}

# on_default succeeds on the default branch and on a detached HEAD.
on_default() {
  local branch ref
  branch="$(current_branch)"
  [ -n "$branch" ] || return 0
  ref="$(default_ref)"
  [ "$branch" = "${ref#origin/}" ]
}

# pick_one reads a newline-separated list of absolute folders. One entry: print
# it repo-relative and return 0. Several: list them on stderr and exit 4. None:
# return 1.
pick_one() {
  local list="$1" n d
  n="$(printf '%s' "$list" | grep -c . || true)"
  if [ "$n" -eq 1 ]; then
    d="$(printf '%s' "$list" | grep .)"
    printf '%s\n' "${d#"$top"/}"
    return 0
  fi
  if [ "$n" -gt 1 ]; then
    echo "flow.sh: more than one flow matches; ask which one:" >&2
    printf '%s' "$list" | grep . | while IFS= read -r d; do echo "  ${d#"$top"/}" >&2; done
    exit 4
  fi
  return 1
}

# small_fix_open succeeds when <root>/unassigned/state.tsv holds a skip row for
# "chain <branch>" with no later pivot row for "fix <branch>".
small_fix_open() {
  local f="$top/$ROOT/unassigned/state.tsv"
  [ -f "$f" ] || return 1
  rows_of "$f" | awk -F'\t' -v t1="chain $1" -v t2="fix $1" '
    $4 == "skip" && $5 == t1 { open = 1 }
    $4 == "pivot" && $5 == t2 { open = 0 }
    END { exit open ? 0 : 1 }'
}

# resolve_flow prints the bound flow's repo-relative folder. Return 1: none.
resolve_flow() {
  local d f branch tip mb matches="" cand="" best t pathspec changed list_tip list_fork untracked flows_list
  [ -n "$top" ] || return 1
  branch="$(current_branch)"
  pathspec="$ROOT/*/*/state.tsv"

  # 1. The active flow with a bind row naming this branch. A flow may name
  #    several branches (lanes). When two flows name it, the later bind wins.
  if [ -n "$branch" ]; then
    flows_list="$(all_flows)"
    while IFS= read -r d; do
      [ -n "$d" ] || continue
      if is_active "$d/state.tsv"; then
        t="$(bind_ts "$d/state.tsv" "$branch")"
        if [ -n "$t" ]; then cand="$cand$t$TAB$d"$'\n'; fi
      fi
    done <<EOF
$flows_list
EOF
    if [ -n "$cand" ]; then
      best="$(printf '%s' "$cand" | awk -F'\t' '$1 > m { m = $1 } END { print m }')"
      matches="$(printf '%s' "$cand" | awk -F'\t' -v b="$best" '$1 == b { print $2 }')"
      if pick_one "$matches"; then return 0; fi
    fi
  fi

  # A small fix in progress on this branch has no work folder, and it wins
  # over steps 2 and 3: otherwise a fix branch would resolve to an unrelated
  # unbound flow. The callers then resolve it to <root>/unassigned.
  if [ -n "$branch" ] && small_fix_open "$branch"; then return 1; fi

  # On the default branch or a detached HEAD only a bind can match: a gate
  # verdict there never lands in another item's flow.
  if on_default; then return 1; fi

  # 2. The one active flow whose state.tsv this branch changed. A path counts
  #    when it differs from the default branch's tip (drops a flow a squash
  #    merge already landed) and from the fork point (drops a flow only the
  #    default branch changed). Untracked files count. Paths under temp/ do not.
  tip="$(default_ref)"
  if [ -n "$tip" ]; then
    mb="$(git -C "$top" merge-base "$tip" HEAD 2> /dev/null || true)"
    [ -n "$mb" ] || mb="$tip"
    untracked="$(git -C "$top" ls-files --others --exclude-standard -- "$pathspec" 2> /dev/null || true)"
    list_tip="$({ git -C "$top" diff --name-only --diff-filter=AM "$tip" -- "$pathspec" 2> /dev/null || true; printf '%s\n' "$untracked"; } | sed '/^$/d' | LC_ALL=C sort -u)"
    list_fork="$({ git -C "$top" diff --name-only --diff-filter=AM "$mb" -- "$pathspec" 2> /dev/null || true; printf '%s\n' "$untracked"; } | sed '/^$/d' | LC_ALL=C sort -u)"
    changed="$(comm -12 <(printf '%s\n' "$list_tip") <(printf '%s\n' "$list_fork") | { grep -v '/temp/' || true; })"
    matches=""
    while IFS= read -r f; do
      [ -n "$f" ] || continue
      d="$top/$(dirname "$f")"
      if is_active "$d/state.tsv"; then matches="$matches$d"$'\n'; fi
    done <<EOF
$changed
EOF
    if pick_one "$matches"; then return 0; fi
  fi

  # 3. The only active flow that no branch is bound to.
  matches=""
  flows_list="$(all_flows)"
  while IFS= read -r d; do
    [ -n "$d" ] || continue
    if is_active "$d/state.tsv" && ! has_bind "$d/state.tsv"; then matches="$matches$d"$'\n'; fi
  done <<EOF
$flows_list
EOF
  if pick_one "$matches"; then return 0; fi
  return 1
}

# resolve_into sets the global RESOLVED_FOLDER to the bound flow's folder;
# callers read it straight after the call. A small fix in progress resolves to
# <root>/unassigned. Exits 3 (none) or 4.
resolve_into() {
  local out="" rc=0 branch
  repo_top
  out="$(resolve_flow)" || rc=$?
  if [ "$rc" -eq 4 ]; then exit 4; fi
  if [ "$rc" -ne 0 ] || [ -z "$out" ]; then
    branch="$(current_branch)"
    if [ -n "$top" ] && [ -n "$branch" ] && small_fix_open "$branch"; then
      RESOLVED_FOLDER="$top/$ROOT/unassigned"
      return 0
    fi
    echo "flow.sh: no flow found; ask which flow, or pass --folder" >&2
    exit 3
  fi
  RESOLVED_FOLDER="$top/$out"
}

where_cmd() {
  local out="" rc=0 branch
  repo_top
  out="$(resolve_flow)" || rc=$?
  if [ "$rc" -eq 4 ]; then exit 4; fi
  if [ "$rc" -ne 0 ] || [ -z "$out" ]; then
    branch="$(current_branch)"
    if [ -n "$top" ] && [ -n "$branch" ] && small_fix_open "$branch"; then
      echo "flow.sh: small fix in progress on $branch; no work folder" >&2
      exit 5
    fi
    echo "flow.sh: no flow found" >&2
    exit 3
  fi
  printf '%s\n' "$out"
  exit 0
}

# ---------------------------------------------------------------------------
# Approved content and commit evidence
# ---------------------------------------------------------------------------

# content_hash prints the blob hash an approval binds to. For plan.md the two
# sections execute appends during a run are left out, because adding to them
# asks no approval. intent.md and spec.md leave out only `## Divergences`,
# because gate and execute append there after approval. The blank lines at the
# end of the kept text of intent.md and spec.md are dropped too, whether or not
# the section is present: appending the first `## Divergences` section adds a
# blank separator line before the heading and must not change the hash, even
# when the approved file already ended in blank lines. Other files hash as they
# are. Line endings are not normalized: the kept lines hash with their bytes, a
# CR included, and only a heading match ignores a trailing CR. flow.ps1 follows
# the same rule, on every OS.
content_hash() {
  local f="$1/$2" h="" orders=0
  [ -f "$f" ] || return 1
  case "$2" in
    plan.md) orders=1 ;;
    intent.md | spec.md) orders=0 ;;
    *)
      h="$(git hash-object -- "$f" 2> /dev/null || true)"
      [ -n "$h" ] || return 1
      printf '%s' "$h"
      return 0
      ;;
  esac
  # The filtered text is piped to git hash-object on stdin, so no temp file is
  # needed; --path keeps the same attribute lookup as hashing the file itself.
  h="$(approved_text "$orders" < "$f" | git hash-object --stdin --path="$f" 2> /dev/null || true)"
  [ -n "$h" ] || return 1
  printf '%s' "$h"
}

# approved_text reads an artifact on stdin and prints the text its approval
# hash covers; orders is 1 for plan.md and 0 for intent.md and spec.md (see
# content_hash). The history search runs the same filter, so a revision hashes
# exactly as the file did when it was approved.
approved_text() {
  # BINMODE=3 makes gawk on Git for Windows read and write bytes as they are.
  # Its default text mode drops the CR of every CRLF before the program sees the
  # line, which would hash a CRLF file as LF. Other awks ignore the variable.
  awk -v BINMODE=3 -v orders="$1" '
    { l = $0; sub(/\r$/, "", l) }
    l ~ /^## / {
      skip = (l == "## Divergences" || (orders == 1 && l == "## Standing orders"))
    }
    !skip { n++; kept[n] = $0; blank[n] = (l == "") }
    END {
      if (orders == 0) while (n > 0 && blank[n]) n--
      for (i = 1; i <= n; i++) print kept[i]
    }'
}

# approved_pointer prints where a stale approval's content can be compared: the
# newest of the last 20 commits touching the file whose revision hashes, as
# content_hash would, to the approved blob. The approved hash covers filtered
# text, so it is rarely a stored object and cannot be diffed directly. It only
# reads history. Any Git failure, a revision the commit lacks (one that deleted
# the file), or no match gives the not-found text.
approved_pointer() {
  local folder="$1" artifact="$2" want="$3" orders="" prefix rel shas sha h short shown
  local notfound="approved content not found in the last 20 commits touching the file"
  case "$artifact" in
    plan.md) orders=1 ;;
    intent.md | spec.md) orders=0 ;;
  esac
  if [ -z "$want" ] || ! prefix="$(git -C "$folder" rev-parse --show-prefix 2> /dev/null)"; then
    printf '%s' "$notfound"
    return 0
  fi
  # rev-parse and <rev>:<path> name paths from the repository root; :(top)
  # makes the log pathspec root-relative too, whatever directory -C chose.
  rel="$prefix$artifact"
  shas="$(git -C "$folder" log -n 20 --format=%H -- ":(top)$rel" 2> /dev/null)" || shas=""
  for sha in $shas; do
    # pipefail (set at the top) fails the substitution when git show fails.
    if [ -n "$orders" ]; then
      h="$(git -C "$folder" show "$sha:$rel" 2> /dev/null | approved_text "$orders" | git hash-object --stdin --path="$folder/$artifact" 2> /dev/null)" || continue
    else
      h="$(git -C "$folder" show "$sha:$rel" 2> /dev/null | git hash-object --stdin --path="$folder/$artifact" 2> /dev/null)" || continue
    fi
    if [ -n "$h" ] && [ "$h" = "$want" ]; then
      short="$(git -C "$folder" rev-parse --short "$sha" 2> /dev/null)" || break
      # The printed command is meant to be copied, so quote a path a shell
      # would split or expand.
      shown="$rel"
      case "$rel" in
        *[!A-Za-z0-9._/@%+=:,-]*) shown="'$(printf '%s' "$rel" | sed "s/'/'\\\\''/g")'" ;;
      esac
      printf 'compare with commit %s (from the repository root): git diff %s -- %s' "$short" "$short" "$shown"
      return 0
    fi
  done
  printf '%s' "$notfound"
}

# patch_id_of prints the first field of git patch-id --stable over a commit.
patch_id_of() {
  git show "$1" 2> /dev/null | git patch-id --stable 2> /dev/null | awk '{ print $1; exit }'
}

# ---------------------------------------------------------------------------
# log
# ---------------------------------------------------------------------------

log_cmd() {
  local folder="" expect="" has_expect=0 count=0 phase event target status why evidence next tok h session sha id
  local p0="" p1="" p2="" p3="" p4="" p5="" p6=""
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --folder)
        [ "$#" -ge 2 ] || { usage >&2; exit 1; }
        folder="$2"
        shift 2
        ;;
      --expect-blob)
        [ "$#" -ge 2 ] || { usage >&2; exit 1; }
        expect="$2"
        has_expect=1
        shift 2
        ;;
      *)
        # An eighth positional argument matches no branch: count reaches 8 and
        # the check below rejects it.
        case "$count" in
          0) p0="$1" ;;
          1) p1="$1" ;;
          2) p2="$1" ;;
          3) p3="$1" ;;
          4) p4="$1" ;;
          5) p5="$1" ;;
          6) p6="$1" ;;
        esac
        count=$((count + 1))
        shift
        ;;
    esac
  done
  [ "$count" -eq 7 ] || { usage >&2; exit 1; }
  if [ "$has_expect" -eq 1 ]; then
    [ "$(trim "$(strip_cell "$p1")")" = "approve" ] && [ "$(trim "$(strip_cell "$p5")")" = "auto" ] || { usage >&2; exit 1; }
  fi

  # Validate the stripped, trimmed, pre-guard values so a guard quote never
  # masks a closed-set mismatch.
  phase="$(trim "$(strip_cell "$p0")")"
  event="$(trim "$(strip_cell "$p1")")"
  target="$(trim "$(strip_cell "$p2")")"
  status="$(trim "$(strip_cell "$p3")")"
  why="$(trim "$(strip_cell "$p4")")"
  evidence="$(trim "$(strip_cell "$p5")")"
  next="$(trim "$(strip_cell "$p6")")"

  in_set "$phase" "$PHASES" || fail phase "$phase" "expected one of: $PHASES"
  in_set "$event" "$EVENTS" || fail event "$event" "expected one of: $EVENTS"
  [ -n "$target" ] || fail target "$target" "a target is required"
  in_set "$status" "$STATUSES" || fail status "$status" "expected one of: $STATUSES"
  [ -n "$why" ] || fail why "$why" "a one-line reason is required"
  [ -n "$evidence" ] || fail evidence "$evidence" "evidence is required; write none when there is nothing to point at"
  [ -n "$next" ] || fail next "$next" "next is required on every row"
  tok="$(first_token "$next")"
  in_set "$tok" "$PHASES done" || fail next "$next" "the first word must be a phase name or done"
  validate_evidence "$event" "$status" "$evidence"

  if [ -z "$folder" ]; then
    resolve_into
    folder="$RESOLVED_FOLDER"
  fi

  case "$evidence" in
    auto | auto:*)
      case "$event" in
        approve)
          [ "$evidence" = "auto" ] || fail evidence "$evidence" "auto:<sha> applies to commit rows only"
          h="$(content_hash "$folder" "$target" || true)"
          # The row stores the hash that was compared, so a later edit reads stale instead of recording unseen bytes.
          if [ "$has_expect" -eq 1 ] && { [ -z "$h" ] || [ "$h" != "$expect" ]; }; then
            fail evidence "$expect" "$folder/$target current blob=${h:-unavailable} does not match --expect-blob"
          fi
          [ -n "$h" ] || fail evidence auto "cannot hash $folder/$target"
          evidence="blob=$h"
          ;;
        verify | revert | diverge)
          if [ "$evidence" = "auto" ]; then
            sha="$(git rev-parse --verify 'HEAD^{commit}' 2> /dev/null || true)"
          else
            sha="$(git rev-parse --verify "${evidence#auto:}^{commit}" 2> /dev/null || true)"
          fi
          [ -n "$sha" ] || fail evidence "$evidence" "no such commit to verify against"
          id="$(patch_id_of "$sha")"
          evidence="commit=$sha patch-id=$id"
          ;;
        *) fail evidence "$evidence" "auto applies to approve, verify, revert, and diverge rows only" ;;
      esac
      ;;
  esac

  session="${FLOW_SESSION:-${CLAUDE_CODE_SESSION_ID:-}}"
  if [ -n "$session" ]; then session="$(clean "$session")"; else session="-"; fi
  [ -n "$session" ] || session="-"

  mkdir -p "$folder"
  local file="$folder/state.tsv"
  # Create-or-nothing, not test-then-write: noclobber makes a lost race fail
  # instead of truncating another writer's rows. One printf is one write, so
  # the version line and header land together. The append below is O_APPEND.
  if [ ! -f "$file" ]; then
    (
      set -o noclobber
      printf '%s\n%s\n' "$VERSION_LINE" "$HEADER" > "$file"
    ) 2> /dev/null || true
  fi
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    "$session" "$phase" "$event" "$(clean "$target")" "$status" "$(clean "$why")" \
    "$(clean "$evidence")" "$(clean "$next")" >> "$file"
  exit 0
}

# ---------------------------------------------------------------------------
# bind
# ---------------------------------------------------------------------------

# bind_cmd appends one bind row naming the current branch to the resolved (or
# named) flow, unless a bind row for that branch already exists. A lane's
# session calls it on its own branch. It is idempotent, so a skill can call it
# whenever it resolved the flow without knowing which lookup step matched.
bind_cmd() {
  local folder="" branch last ph nx
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --folder)
        [ "$#" -ge 2 ] || { usage >&2; exit 1; }
        folder="$2"
        shift 2
        ;;
      *) usage >&2; exit 1 ;;
    esac
  done
  repo_top
  branch="$(current_branch)"
  [ -n "$branch" ] || fail branch "" "a detached HEAD has no branch to bind"
  if on_default; then fail branch "$branch" "the default branch is never bound to a flow"; fi
  if [ -z "$folder" ]; then
    resolve_into
    folder="$RESOLVED_FOLDER"
  fi
  if [ "$(basename "$folder")" = "unassigned" ]; then
    fail folder "$folder" "a small fix has no flow to bind"
  fi
  [ -f "$folder/state.tsv" ] || fail folder "$folder" "no state.tsv to bind to"
  if [ -n "$(bind_ts "$folder/state.tsv" "$branch")" ]; then exit 0; fi
  last="$(rows_of "$folder/state.tsv" | tail -n 1)"
  ph="$(printf '%s' "$last" | awk -F'\t' '{ print $3 }')"
  nx="$(printf '%s' "$last" | awk -F'\t' '{ print $9 }')"
  [ -n "$ph" ] || ph="intent"
  [ -n "$nx" ] || nx="intent resume"
  log_cmd "$ph" bind "$branch" ok "bound $branch to this flow" none "$nx" --folder "$folder"
}

# ---------------------------------------------------------------------------
# status
# ---------------------------------------------------------------------------

status_cmd() {
  local folder="" metrics=0 file agg kind a b c d branch
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --folder)
        [ "$#" -ge 2 ] || { usage >&2; exit 1; }
        folder="$2"
        shift 2
        ;;
      --metrics)
        metrics=1
        shift
        ;;
      *) usage >&2; exit 1 ;;
    esac
  done
  if [ -z "$folder" ]; then
    repo_top
    local out="" rc=0
    out="$(resolve_flow)" || rc=$?
    if [ "$rc" -eq 4 ]; then exit 4; fi
    if [ "$rc" -ne 0 ] || [ -z "$out" ]; then
      branch="$(current_branch)"
      if [ -n "$top" ] && [ -n "$branch" ] && small_fix_open "$branch"; then
        printf 'small fix: %s\n' "$branch"
        exit 0
      fi
      echo "flow.sh: no flow found; ask which flow, or pass --folder" >&2
      exit 3
    fi
    folder="$top/$out"
  else
    repo_top
  fi
  file="$folder/state.tsv"
  [ -f "$file" ] || { echo "flow.sh: no state.tsv in $folder" >&2; exit 3; }

  local ver
  ver="$(head -n 1 "$file")"
  agg="$(rows_of "$file" | awk -F'\t' '
    function epoch(t,   y, m, d, H, M, S, mm, doy, era, yoe, doe) {
      y = substr(t, 1, 4) + 0; m = substr(t, 6, 2) + 0; d = substr(t, 9, 2) + 0
      H = substr(t, 12, 2) + 0; M = substr(t, 15, 2) + 0; S = substr(t, 18, 2) + 0
      if (m <= 2) y -= 1
      era = int(y / 400); yoe = y - era * 400
      mm = (m > 2) ? m - 3 : m + 9
      doy = int((153 * mm + 2) / 5) + d - 1
      doe = yoe * 365 + int(yoe / 4) - int(yoe / 100) + doy
      return (era * 146097 + doe - 719468) * 86400 + H * 3600 + M * 60 + S
    }
    {
      n++; sess[$2] = 1
      lts = $1; lses = $2; lph = $3; lev = $4; lst = $6; lnx = $9
      # Only an approve row with status ok approves; a declined one records a refusal.
      ok = ($6 == "ok")
      if ($4 == "approve" && ok) { ats[$5] = $1; aev[$5] = $8 }
      if ($4 == "revoke")  { delete ats[$5]; delete aev[$5] }
      if ($4 == "block")   { bts[$5] = $1 }
      if ($4 == "unblock") { delete bts[$5] }
      if ($4 == "enter" && $5 == "run") { mode = $8 }
      if ($4 == "branch" || $4 == "release" || $4 == "change") { cut = $1; cutn = n }
      if ($4 == "create" && $5 == "intent.md" && ci == "") ci = $1
      if ($4 == "approve" && ok && $5 == "intent.md" && ai == "") ai = $1
      if ($4 == "approve" && ok && $5 == "spec.md" && as == "") as = $1
      if ($4 == "create" && $5 == "plan.md" && cp == "") cp = $1
      if ($4 == "approve" && ok && $5 == "plan.md" && ap == "") ap = $1
      if ($4 == "branch" && gb == "") gb = $1
      if ((($4 == "approve" && ok) || $4 == "revoke") && $5 == "spec.md") { rw++; rwts[rw] = $1 }
      key = $4 "\t" $5; ev[key] = $8; evts[key] = $1; evn[key] = n
      # match() and substr() keep this POSIX awk: no capture groups, no {n}.
      if ($4 == "review") {
        if (match($8, /kind=[a-z]+/)) rv[substr($8, RSTART + 5, RLENGTH - 5)]++
        if (match($8, /dispatched=[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]T[0-9][0-9]:[0-9][0-9]:[0-9][0-9]Z/)) {
          lat = epoch($1) - epoch(substr($8, RSTART + 11, 20))
          if (lat > 0) rlat += lat
        }
        if (match($8, /blocked=[0-9]+/)) rblk += substr($8, RSTART + 8, RLENGTH - 8) + 0
      }
      if ($4 == "escape") nesc++
    }
    END {
      if (n == 0) { print "E"; exit }
      c = 0; for (s in sess) c++
      print "P\t" lph "\t" lev "\t" lst "\t" lnx
      print "S\t" c "\t" lts "\t" lses
      if (mode != "") print "M\t" mode
      for (t in ats) print "A\t" t "\t" ats[t] "\t" aev[t]
      for (t in bts) print "B\t" t "\t" bts[t]
      for (k in ev) {
        # Newer than the verdict: a later ts, or the same ts and a later place
        # in the file, which is the tie-break the reader order uses.
        newer = (evts[k] > cut) || (evts[k] == cut && evn[k] > cutn)
        if (ev[k] ~ /commit=[^ ]+/ && ev[k] ~ /patch-id=[^ ]+/ && newer) print "C\t" k "\t" ev[k]
      }
      r = 0
      if (cp != "") { for (i = 1; i <= rw; i++) if (rwts[i] > cp) r++ }
      print "T1\t" ((ci != "" && ai != "") ? epoch(ai) - epoch(ci) : "n/a")
      print "T2\t" ((ai != "" && as != "") ? epoch(as) - epoch(ai) : "n/a")
      print "T3\t" ((cp != "") ? r : "n/a")
      print "T4\t" ((ap != "" && gb != "") ? epoch(gb) - epoch(ap) : "n/a")
      print "R\t" (rv["plan"] + 0) "\t" (rv["packet"] + 0) "\t" (rv["quality"] + 0) "\t" (rv["integration"] + 0)
      print "RT\t" (rlat + 0) "\t" (rblk + 0)
      print "X\t" (nesc + 0)
    }')"

  local ph="" ev="" st="" nx="" cnt="0" lts="" lses="" empty=0 mode=""
  local appr="" stale="" ablob pointer nextact blk="" mism="" cur state sha id t1="n/a" t2="n/a" t3="n/a" t4="n/a"
  local checked="" tip bids="" c2 ok
  local reviews="" rlat="0" rblk="0" escapes="0"
  while IFS=$'\t' read -r kind a b c d; do
    case "$kind" in
      E) empty=1 ;;
      P) ph="$a"; ev="$b"; st="$c"; nx="$d" ;;
      S) cnt="$a"; lts="$b"; lses="$c" ;;
      M) mode="$a" ;;
      A)
        cur="$(content_hash "$folder" "$a" || true)"
        if [ -n "$cur" ] && [ "blob=$cur" = "$c" ]; then state="current"; else state="stale"; fi
        appr="${appr}approvals: $a approved ${b%%T*} $state"$'\n'
        if [ "$state" = "stale" ]; then
          case "$c" in
            blob=?*) ablob="$c"; pointer="$(approved_pointer "$folder" "$a" "${c#blob=}")" ;;
            *) ablob="blob=unrecorded"; pointer="$(approved_pointer "$folder" "$a" "")" ;;
          esac
          if [ "$a" = "plan.md" ]; then
            nextact="Review it, then approve it again; execute will not start until you do."
          else
            nextact="Review it, then approve it again; a phase that needs this approval will not start until you do."
          fi
          stale="${stale}stale: $folder/$a differs from the content approved ${b%%T*} (approved $ablob, current blob=${cur:-unavailable}). $pointer. $nextact"$'\n'
        fi
        ;;
      B) blk="${blk}blocks: $a blocked since ${b%%T*}"$'\n' ;;
      C) checked="${checked}$a${TAB}$b${TAB}$c"$'\n' ;;
      T1) t1="$a" ;;
      T2) t2="$a" ;;
      T3) t3="$a" ;;
      T4) t4="$a" ;;
      R)
        # Kinds in the order of REVIEW_KINDS; a kind with no rows is left out.
        [ "$a" = "0" ] || reviews="$reviews plan=$a"
        [ "$b" = "0" ] || reviews="$reviews packet=$b"
        [ "$c" = "0" ] || reviews="$reviews quality=$c"
        [ "$d" = "0" ] || reviews="$reviews integration=$d"
        ;;
      RT) rlat="$a"; rblk="$b" ;;
      X) escapes="$a" ;;
    esac
  done <<EOF
$(printf '%s\n' "$agg" | LC_ALL=C sort)
EOF

  # Commit rows: the commit is an ancestor of HEAD, or its patch-id matches a
  # non-merge commit on this branch (a rebase moved it). Otherwise a mismatch.
  if [ -n "$checked" ]; then
    tip="$(default_ref)"
    if [ -n "$tip" ]; then
      bids="$(git -C "$folder" rev-list --no-merges "$tip..HEAD" 2> /dev/null | while IFS= read -r c2; do (cd "$folder" && patch_id_of "$c2"); done || true)"
    fi
    while IFS=$'\t' read -r a b c; do
      [ -n "$a" ] || continue
      sha="$(printf '%s' "$c" | sed -n 's/.*commit=\([^ ]*\).*/\1/p')"
      id="$(printf '%s' "$c" | sed -n 's/.*patch-id=\([^ ]*\).*/\1/p')"
      ok=0
      if git -C "$folder" merge-base --is-ancestor "$sha" HEAD 2> /dev/null; then ok=1; fi
      if [ "$ok" -eq 0 ] && [ -n "$id" ] && printf '%s\n' "$bids" | grep -qxF "$id"; then ok=1; fi
      if [ "$ok" -eq 0 ]; then
        mism="${mism}mismatch: $a $b commit $sha is not an ancestor of HEAD and patch-id $id matches no commit on this branch"$'\n'
      fi
    done <<EOF
$(printf '%s' "$checked" | LC_ALL=C sort)
EOF
  fi
  if [ "$ver" != "$VERSION_LINE" ]; then
    mism="${mism}mismatch: format version line is '$ver', expected '$VERSION_LINE'"$'\n'
  fi

  printf 'flow: %s\n' "$folder"
  if [ "$empty" -eq 1 ]; then
    printf 'phase: none (no rows)\nnext: none\n'
  else
    printf 'phase: %s (%s, %s)\nnext: %s\n' "$ph" "$ev" "$st" "$nx"
  fi
  if [ -n "$appr" ]; then printf '%s' "$appr"; else printf 'approvals: none\n'; fi
  if [ -n "$stale" ]; then printf '%s' "$stale"; fi
  if [ -n "$blk" ]; then printf '%s' "$blk"; else printf 'blocks: none\n'; fi
  if [ -n "$mode" ]; then
    local m br
    m="$(printf '%s' "$mode" | sed -n 's/.*mode=\([^ ]*\).*/\1/p')"
    br="$(printf '%s' "$mode" | sed -n 's/.*branch=\([^ ]*\).*/\1/p')"
    printf 'mode: %s (%s)\n' "${m:-unknown}" "${br:--}"
  fi
  printf 'sessions: %s\n' "$cnt"
  if [ "$empty" -eq 0 ]; then printf 'last: %s %s\n' "$lts" "$lses"; fi
  if [ "$metrics" -eq 1 ]; then
    local u1="$t1" u2="$t2" u4="$t4"
    [ "$u1" = "n/a" ] || u1="${u1}s"
    [ "$u2" = "n/a" ] || u2="${u2}s"
    [ "$u4" = "n/a" ] || u4="${u4}s"
    printf 'time-to-intent: %s\nintent-to-spec: %s\nspec-rework: %s\nplan-to-verdict: %s\n' "$u1" "$u2" "$t3" "$u4"
    printf 'reviews:%s\nreview-time: latency=%ss blocked=%ss\nescapes: %s\n' "${reviews:- none}" "$rlat" "$rblk" "$escapes"
  fi
  if [ -n "$mism" ]; then printf '%s' "$mism"; fi
  exit 0
}

cmd="${1:-}"
if [ -z "$cmd" ]; then usage >&2; exit 1; fi
shift
case "$cmd" in
  -h | --help) usage; exit 0 ;;
  log) log_cmd "$@" ;;
  status) status_cmd "$@" ;;
  where) where_cmd "$@" ;;
  bind) bind_cmd "$@" ;;
  *) usage >&2; exit 1 ;;
esac
