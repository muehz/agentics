# Save and stage the plan

Step 6 of `skills/decompose/SKILL.md` applies this procedure.

Build `PLAN_ARTIFACTS` as an explicit list of repository paths that this invocation wrote: the plan and each adopted
source. **`state.tsv` is deliberately not in that list.** `git add` validates every pathspec before it stages anything.
A pathspec for a file that does not exist yet fails the whole invocation with exit 128 and leaves `plan.md` unstaged
too. A work folder entered directly, with no `intent.md` or `spec.md` before it, has no `state.tsv` yet.

Use the resolved work folder, never a hardcoded parent. Include those provenance siblings before you stage. Do not stage
unrelated files. Do not leave a plan staged with its new source absent from the index.

This procedure saves and stages. It never commits. A plan is committed only with its approval row, after the user's yes,
as the **commit** paragraph of Step 6 in `skills/decompose/SKILL.md` sets, so a plan with no approval stays staged and
uncommitted. No terminal probe decides whether to commit: the user's approval does.

Keep the order below:

1. Stage the plan.
2. **Confirm the plan is in the index.**
3. Only then log the rows that assert the plan is in the index.
4. Stage that log, then stop.

`create` means an artifact's **first** write, not every write, per the event table in
[flow-state.md](../../execute/references/flow-state.md). This workflow preserves and updates an existing plan, so the
creation row is conditional. Capture whether `<work-folder>/plan.md` existed **before** this invocation wrote it. Emit
the row only when it did not.

A second `create plan.md` would make the artifact look created twice. The log is append-only, so nothing could ever
correct the count.

One `review plan` row follows for each plan-document reviewer return from Step 5, after the `create` row and before the
`challenge` row. An in-session self-read writes it too. A re-run (round 2) writes its own row. A run that ran no plan
review writes none.

The `challenge plan.md` row follows only a write that ran or declined the challenge pass. Otherwise, a slicing-only
request or a standalone coverage re-check would record a result that never happened.

An update that also answers the execution question yes still writes its `approve plan.md` row. That row records the
approval transition, not the write. Write it with evidence `auto` and `--expect-blob <hash>`. `<hash>` is the hash that
`flow status` prints on its `hashes: plan.md text=<hash>` line for the content the user was shown. The
[approval guard](../../execute/references/flow-approval-and-audit.md#the-approval-guard) writes the row only when the
plan still has that hash.

The user's yes to the plan also confirms its standing orders. Right after the `approve plan.md` row, write one
`order <n>` row for each top-level item of the plan's `## Standing orders` section, with `<n>` its position counted from
1 and evidence `auto`. A plan with no such section writes none:

    agx flow log decompose order <n> ok "confirmed with the plan approval" auto "execute run the plan" --folder <work-folder>

`flow` stores `entry=<hash of the order>` and records it in this clone's ledger, so `execute` sends the order to workers
as an order ([standing-orders.md](../../execute/references/standing-orders.md)). In a committed work folder, stage
`state.tsv` again after these rows, then commit the plan and `state.tsv` together under the
[commit contract](../../git/references/commit.md#the-commit-contract). That commit is the only one this skill makes for
the plan.

**Where the folder lives.** The [storage setting](../../execute/references/flow-state.md#storage) decides whether
anything is staged. Each snippet reads the `storage:` line of `flow status`. When it prints `storage: local`, the work
folder is in the clone's Git directory: the snippet still writes the rows, and it skips every `git add` and the commit.
A folder with no `state.tsv` yet has no `storage:` line, and `flow status` exits 3 for it. Its plan was saved in the
plan directory, the directory that `flow path --folder <work-folder>` prints. Under `committed` that is the working-tree
folder itself, and under `local` it is `<git-common-dir>/agentics/work/<work-folder>`. Each snippet captures the plan
directory once, as `PLAN_DIR` in bash and `$PlanDir` in PowerShell. On exit 3 it reads the store from whether the plan
directory is the working-tree folder: `committed` when it is, `local` when it is not.

**The check before the first save.** Under `committed`, run `git check-ignore -q <work-folder>/plan.md` before the first
save into the folder. When it exits 0, say that the work root is ignored by Git and that planning files cannot be
committed there, then stop and report, with no rows written. The same holds when `AGENTS.md` or `CLAUDE.md` says
planning files are not committed: say so, and quote that sentence as the reason. Both snippets repeat the check before
any `git add`, and stop with exit 1 before any row when it matches.

Both snippets call bare `agx flow …`, as [flow-state.md](../../execute/references/flow-commands.md#commands) shows. When
bare `agx` is not found (exit 127 in Bash, the `catch` in PowerShell), the snippet stops; rerun it with that section's
launcher-path fallback in place of `agx`. Only the fallback needs the plugin root, which the
[plugin-root order](../../routing/references/chain-phases.md#resolve-the-plugin-root) resolves. In PowerShell, read
`$LASTEXITCODE` after each call.

The work root is `docs/specs`. When `AGENTS.md` or `CLAUDE.md` names another directory, set `WORK_ROOT` (`$WorkRoot` in
PowerShell) to it, as the **Resolve the plan parent** step in `SKILL.md` says. Each snippet passes it to `flow` on the
same command line, because a shell call keeps no variables from the one before.

Set the values below before the snippet runs:

- `PLAN_EXISTED` is 1 when `plan.md` was already in the plan directory before this invocation wrote it. Read it before
  the write, from `<plan directory>/plan.md`, with the plan directory that `flow path --folder <work-folder>` prints;
  never from `<work-folder>/plan.md`. Under `local` the plan is not saved there, so that test would read 0 on every
  update and log another `create plan.md` row.
- `CHALLENGE_RAN` is 1 when this invocation ran or declined the Step 4b pass, so it has a result to record. A
  slicing-only request and a standalone coverage re-check set it to 0 and write no `challenge` row.
- `CREATE_NEXT` is `decompose challenge the plan` when a challenge row follows. For a slicing-only plan it is
  `decompose write the tasks`, because `execute` refuses a plan with no tasks.
- `REVIEW_STATUS`, `REVIEW_WHY`, and `REVIEW_EVIDENCE` (`$PlanReviews` in PowerShell) hold one entry per plan reviewer
  return, in order. Each status is `ok` when the verdict is Ready and `failed` when it asked for revision. Each `why` is
  one line on what the review found. Each evidence is
  `kind=plan dispatched=<UTC ts, form 2026-10-30T10:00:00Z> round=<1 or 2> blocked=<latency seconds> rev=<n> by=<independent|self>`.
  `round` is the run's number within revision `rev`, and `by` is `self` for a self-read. Step 5's review budget counts
  only the rows that name the current `rev=` with `by=independent`, so a row without both pairs is invisible to it. The
  latency is the whole blocked time, because `decompose` runs no implementer. The entry list is empty when no review
  ran. The rows carry the same `next` as the `create` row, `CREATE_NEXT`.

```bash
WORK_ROOT="docs/specs" # or the directory AGENTS.md / CLAUDE.md names

# The plan directory: the working-tree folder under committed, the local store's directory under local.
PLAN_DIR=$(FLOW_WORK_ROOT="$WORK_ROOT" agx flow path --folder "<work-folder>") \
  || { rc=$?; if [ "$rc" -eq 127 ]; then echo 'agx not found: rerun with the launcher-path fallback in flow-state.md#commands' >&2; else echo 'flow path failed; not staging or logging a row' >&2; fi; exit 1; }

# A folder in the local store is never staged. Exit 3 means no state.tsv yet, so no storage: line.
STATUS_OUT=$(FLOW_WORK_ROOT="$WORK_ROOT" agx flow status --folder "<work-folder>")
STATUS_CODE=$?
if [ "$STATUS_CODE" = "127" ]; then
  echo 'agx not found: rerun with the launcher-path fallback in flow-state.md#commands' >&2; exit 1
elif [ "$STATUS_CODE" = "0" ]; then
  if printf '%s\n' "$STATUS_OUT" | grep -qFx 'storage: local'; then STORAGE=local; else STORAGE=committed; fi
elif [ "$STATUS_CODE" = "3" ]; then
  # -ef compares the directories themselves, so path spelling and separators do not matter.
  if [ "$PLAN_DIR" -ef "<work-folder>" ]; then STORAGE=committed; else STORAGE=local; fi
else
  echo 'flow status failed; not staging or logging a row' >&2; exit 1
fi

if [ "$STORAGE" = "committed" ]; then
  if git check-ignore -q "<work-folder>/plan.md"; then
    echo 'the work root is ignored by Git, so planning files cannot be committed there; stop and report' >&2; exit 1
  fi
  # PLAN_ARTIFACTS is the explicit nonempty array collected above. state.tsv is not in it.
  git add -- "${PLAN_ARTIFACTS[@]}" || { echo 'staging failed; not logging a row' >&2; exit 1; }
  git diff --cached --name-only
  # Confirm the plan is in the index before any row claims it is. A missing adopted
  # source fails the add above and would otherwise leave the row asserting `ok`
  # about a staging that never happened. -F so a dot in the path is a literal dot.
  git diff --cached --name-only | grep -qFx "<work-folder>/plan.md" || { echo 'plan not staged; stop and report' >&2; exit 1; }
fi

# Observed, not predicted: under committed the add above is confirmed, and under
# local the rows describe the write alone, so the rows may say `ok`.
ROW_WRITTEN=0
# `create` is the first write only, so an update logs no row.
if [ "$PLAN_EXISTED" != "1" ]; then
  FLOW_WORK_ROOT="$WORK_ROOT" agx flow log decompose create plan.md ok "<one line: what the plan delivers>" plan.md "$CREATE_NEXT" --folder "<work-folder>" \
    || { rc=$?; if [ "$rc" -eq 127 ]; then echo 'create row not written; agx not found: rerun with the launcher-path fallback in flow-state.md#commands' >&2; else echo 'create row not written; stop and report' >&2; fi; exit 1; }
  ROW_WRITTEN=1
fi
# One `review plan` row per Step 5 plan reviewer return, between `create` and `challenge`.
for i in "${!REVIEW_STATUS[@]}"; do
  FLOW_WORK_ROOT="$WORK_ROOT" agx flow log decompose review plan "${REVIEW_STATUS[$i]}" "${REVIEW_WHY[$i]}" "${REVIEW_EVIDENCE[$i]}" "$CREATE_NEXT" --folder "<work-folder>" \
    || { rc=$?; if [ "$rc" -eq 127 ]; then echo 'review row not written; agx not found: rerun with the launcher-path fallback in flow-state.md#commands' >&2; else echo 'review row not written; stop and report' >&2; fi; exit 1; }
  ROW_WRITTEN=1
done
# CHALLENGE_RESULT is the Step 4b result for this plan: ok, declined, or pending. CHALLENGE_NEXT is
# "decompose answer the open challenge questions" for pending, else "decompose ask for approval".
if [ "$CHALLENGE_RAN" = "1" ]; then
  FLOW_WORK_ROOT="$WORK_ROOT" agx flow log decompose challenge plan.md "$CHALLENGE_RESULT" "<one line: what the challenge settled>" plan.md "$CHALLENGE_NEXT" --folder "<work-folder>" \
    || { rc=$?; if [ "$rc" -eq 127 ]; then echo 'challenge row not written; agx not found: rerun with the launcher-path fallback in flow-state.md#commands' >&2; else echo 'challenge row not written; stop and report' >&2; fi; exit 1; }
  ROW_WRITTEN=1
fi
if [ "$STORAGE" = "committed" ] && [ "$ROW_WRITTEN" = "1" ]; then
  git add -- "<work-folder>/state.tsv" || { echo 'row written but not staged; report it' >&2; exit 1; }
fi
```

```powershell
# PowerShell
$WorkRoot = 'docs/specs' # or the directory AGENTS.md / CLAUDE.md names
$env:FLOW_WORK_ROOT = $WorkRoot

# The plan directory: the working-tree folder under committed, the local store's directory under local.
try {
    $PlanDir = agx flow path --folder '<work-folder>'
} catch { Write-Error 'agx not found: rerun with the launcher-path fallback in flow-state.md#commands'; exit 1 }
if ($LASTEXITCODE -ne 0) { Write-Error 'flow path failed; not staging or logging a row'; exit 1 }

# A folder in the local store is never staged. Exit 3 means no state.tsv yet, so no storage: line.
try {
    $StatusOut = agx flow status --folder '<work-folder>'
} catch { Write-Error 'agx not found: rerun with the launcher-path fallback in flow-state.md#commands'; exit 1 }
if ($LASTEXITCODE -eq 0) {
    $Storage = if (@($StatusOut) -contains 'storage: local') { 'local' } else { 'committed' }
} elseif ($LASTEXITCODE -eq 3) {
    # Full paths compare equal whatever the separators, and need nothing on disk.
    $WorkTreeDir = [System.IO.Path]::GetFullPath((Join-Path (Get-Location).ProviderPath '<work-folder>')).TrimEnd('\', '/')
    $Storage = if ([System.IO.Path]::GetFullPath($PlanDir).TrimEnd('\', '/') -eq $WorkTreeDir) { 'committed' } else { 'local' }
} else { Write-Error 'flow status failed; not staging or logging a row'; exit 1 }

if ($Storage -eq 'committed') {
    git check-ignore -q '<work-folder>/plan.md'
    if ($LASTEXITCODE -eq 0) {
        Write-Error 'the work root is ignored by Git, so planning files cannot be committed there; stop and report'; exit 1
    }
    # $PlanArtifacts is the explicit nonempty array collected above. state.tsv is not in it.
    git add -- $PlanArtifacts
    if ($LASTEXITCODE -ne 0) { Write-Error 'staging failed; not logging a row'; exit 1 }
    git diff --cached --name-only
    if (-not (git diff --cached --name-only | Select-String -SimpleMatch -Quiet '<work-folder>/plan.md')) {
        Write-Error 'plan not staged; stop and report'; exit 1
    }
}

# `create` is the first write only, so an update logs no row.
$RowWritten = $false
if (-not $PlanExisted) {
    try {
        agx flow log decompose create plan.md ok '<one line: what the plan delivers>' plan.md $CreateNext --folder '<work-folder>'
    } catch { Write-Error 'create row not written; agx not found: rerun with the launcher-path fallback in flow-state.md#commands'; exit 1 }
    if ($LASTEXITCODE -ne 0) { Write-Error 'create row not written; stop and report'; exit 1 }
    $RowWritten = $true
}
# One `review plan` row per Step 5 plan reviewer return, between `create` and `challenge`. Each $PlanReviews entry
# is a hashtable with Status, Why, and Evidence.
foreach ($r in $PlanReviews) {
    try {
        agx flow log decompose review plan $r.Status $r.Why $r.Evidence $CreateNext --folder '<work-folder>'
    } catch { Write-Error 'review row not written; agx not found: rerun with the launcher-path fallback in flow-state.md#commands'; exit 1 }
    if ($LASTEXITCODE -ne 0) { Write-Error 'review row not written; stop and report'; exit 1 }
    $RowWritten = $true
}
# $ChallengeResult is the Step 4b result for this plan: ok, declined, or pending. $ChallengeNext is
# 'decompose answer the open challenge questions' for pending, else 'decompose ask for approval'.
if ($ChallengeRan) {
    try {
        agx flow log decompose challenge plan.md $ChallengeResult '<one line: what the challenge settled>' plan.md $ChallengeNext --folder '<work-folder>'
    } catch { Write-Error 'challenge row not written; agx not found: rerun with the launcher-path fallback in flow-state.md#commands'; exit 1 }
    if ($LASTEXITCODE -ne 0) { Write-Error 'challenge row not written; stop and report'; exit 1 }
    $RowWritten = $true
}
if ($Storage -eq 'committed' -and $RowWritten) {
    git add -- '<work-folder>/state.tsv'
    if ($LASTEXITCODE -ne 0) { Write-Error 'row written but not staged; report it'; exit 1 }
}
```

The rows' fields follow the event table and evidence forms in [flow-state.md](../../execute/references/flow-state.md).

Three properties hold together only in that order:

- The rows say `ok` because the add is confirmed in the index first, so the claim is observed, not predicted.
- The commit waits for the approval, so one commit carries the plan, the rows, and the `approve plan.md` row that names
  its hash. A plan is never committed ahead of the approval that makes it committable, and the rows are never left
  staged behind a commit that already finished.
- `state.tsv` never appears in a pathspec before it exists, so the atomic `git add` cannot fail on it.

Under `local` nothing is staged or committed, so only the first property applies, with the write in place of the add.

A row that asserts `ok` for a write before the write happened is the unverified claim that Step 6 forbids. This log is
append-only, so a later row can only supersede a wrong row, never correct it.
