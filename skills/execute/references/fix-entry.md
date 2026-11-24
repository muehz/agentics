# The `fix` entry

This file holds the rules for [the `fix` entry](../SKILL.md#the-fix-entry) of `SKILL.md`.

Call the `flow` helper as bare `agx flow …`, as [flow-state.md](flow-commands.md#commands) shows. The plugin root is
needed only for that section's launcher-path fallback.

## Admission

`/agentics:execute fix` runs a small fix: a change that the
[small-fix classification](../../routing/references/chain-phases.md#small-fix-classification) admitted and announced.
The entry is reached from `routing`, from `diagnose` with a proven cause or with a [user's change](#a-users-change), or
from the user's `/agentics:execute fix <one sentence>`. The classification admits a small fix, not the word `fix`: a
request that has not passed it runs it first and announces the fix the same way, a direct invocation included.

Before the classification, run `agx flow where`. Read the result by the first row that matches:

| Order | Result                                   | Do                                                                                                                                                                                                      |
| ----- | ---------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1     | Exit 5: a small fix is in progress here  | Resume it from its last row                                                                                                                                                                             |
| 2     | A flow or chain artifact covers the item | The flow's phase; a fix for a task of a running plan is in **A diagnosed fix** of `SKILL.md`. A [user's change](#a-users-change) first takes the steps in its **On a branch a flow already covers**     |
| 3     | A small-fix signal fails                 | Write no row. Forward to `intent`, or to `diagnose` for a failure whose cause is not yet known. A [user's change](#a-users-change) goes to `intent` with the reason and the report, never to `diagnose` |
| 4     | Otherwise                                | Run the fix                                                                                                                                                                                             |

The announcement, unchallenged, is the go-ahead for local commits. It authorizes no push. In the announcement, say what
bookkeeping the pull request will carry: under the `committed` [storage setting](flow-state.md#storage), the
`<work-root>/unassigned/state.tsv` rows and their commits; under `local`, none. `arena` is not available on this entry:
an `arena` argument runs the fix and says so.

### A user's change

A user's change is a change the user directs without a proven cause, handed over as
[A user's change](../../diagnose/SKILL.md#a-users-change) of `diagnose` says. It comes with the diagnose report (the
six-part report, handed off in this session or pointed to by the user) and a statement of one of two reasons:

- `declined`: the user declined a diagnose step (the Iron Law or a phase) after `diagnose` named its cost, as a
  [process gate](../../routing/references/contracts/gates.md#process-gate) allows.
- `no root cause`: `diagnose` reached **When no root cause is found**, and the user wants a monitoring, reproduction, or
  handling change.

Only the report and that statement together make a user's change. A request to skip a diagnosis that never ran ("skip
the diagnosis, just add a retry") is not one: it goes to `diagnose`, which names the cost once and, on the next decline,
hands the change back as a user's change. Every other report of an unexplained failure still goes to `diagnose`. The
report travels with the change as context.

**The runtime-change yes.** A user's change that alters runtime behavior (a retry, a timeout, a changed error) needs the
user's yes. The yes exists when the user named this runtime change in their own words, or answered yes to the step-4
question of diagnose's **When no root cause is found** in this session. Otherwise ask one yes/no question in the
[question format](../../routing/references/contracts/question-format.md) before any edit or row: on row 2 before the
forward, on row 4 before step 1. A monitoring or reproduction change that leaves runtime behavior unchanged needs no
yes. On no, or no answer, write no row, change nothing, and report that the change was not made. Never send it back to
`diagnose`.

**The waiver row.** For `declined`, write one `waive "diagnose <branch>"` row, keyed by the current branch as
`skip chain <branch>` is. Its `why` names the declined step and the user's reason, or `none given`, and its evidence is
`none`. Before writing it, check the trail you write to and the fix record for this branch (the rows after the latest
`skip chain <branch>`) for an existing `waive "diagnose <branch>"` row, and write none when one exists. Read the fix
trail from the directory `agx flow path --folder <work-root>/unassigned` names, which holds it under either
[storage setting](flow-state.md#storage); under `local` it is not in the working tree. `no root cause` writes no `waive`
row, because nothing was waived.

**On a branch a flow already covers.** On Admission row 2, execute itself acts before it forwards to the flow's phase.
It asks for any missing runtime-change yes. For `declined`, it runs the existing-row check against both that flow's
`state.tsv` and the fix record for this branch and, when neither has one, writes the waiver row into that flow's
`state.tsv`, with `runtime change approved by the user` in its `why` when the yes applies. Use phase `execute`, that
flow's folder in `flow_folder`, and that flow's current `next` in `next`:

```bash
agx flow log execute waive "diagnose $branch" ok "$waive_why" none "$next" --folder "$flow_folder"
```

It writes no other row of its own. That rule does not stop the `bind` row that
[flow-state.md](flow-evidence.md#rows-each-skill-writes) requires after resolving a flow execute did not create. The
forward carries the reason, the report, and the yes, and the flow's phase records them by its own rules. For
`no root cause`, the yes travels only in that forward.

## Steps

1. **Branch.** Apply [The work-item branch](../../git/references/branch.md#the-work-item-branch): on a detached HEAD or
   a branch on the resolver's `protected` line, create the feature branch with no question. Build the name from the one
   sentence and announce it.
2. **Record the skip.** In one shell call, capture the current branch with `git branch --show-current` in `branch`. Stop
   if it fails or prints nothing. Read the other data from scratch files or pass it through structured tool arguments.
   Use `base_oid` for the validated merge-base id, `why` for the reason, `next` for the next step beginning with
   `execute`, and `mode` and `mode_reason` for the selected mode and its reason. Use `folder` for the logical
   `unassigned` directory under the work root. The work root is `docs/specs`, or the directory `AGENTS.md` or
   `CLAUDE.md` names ([flow-state.md](flow-state.md#where-it-lives)). The reason opens with `bug fix:` or `change:` and
   lists the five signals with their evidence. For a [user's change](#a-users-change), it opens
   `change: user's change after diagnose (<declined|no root cause>)`, then lists the five signals, and adds
   `runtime change approved by the user` when the yes applies. Then run both commands in that call:

    ```bash
    agx flow log execute skip "chain $branch" ok "$why" "base=$base_oid" "$next" --folder "$folder" &&
      agx flow log execute enter run ok "$mode chosen: $mode_reason" "mode=$mode branch=$branch" "$next" --folder "$folder"
    ```

    For `declined`, the same call writes the [waiver row](#a-users-change) between the two, with its reason in
    `waive_why`, unless the existing-row check finds one:

    ```bash
    agx flow log execute skip "chain $branch" ok "$why" "base=$base_oid" "$next" --folder "$folder" &&
      agx flow log execute waive "diagnose $branch" ok "$waive_why" none "$next" --folder "$folder" &&
      agx flow log execute enter run ok "$mode chosen: $mode_reason" "mode=$mode branch=$branch" "$next" --folder "$folder"
    ```

    Every later row of this fix passes the same `--folder`. No `plan.md` is written and no work folder is created.

3. **One-task packet.** Write down the task the fix is held to, with task ID `fix`: the goal (the one sentence), the
   file scope (the files from signals 3 and 5), the verification command (the signal 4 test or check), and the
   acceptance criteria (the behavior the sentence names, and a passing run of that command). Stage 1 does not run,
   because no plan exists to review the packet against. The standing-orders register is the user's stated constraints,
   or `None recorded.`
4. **A bug fix lands its failing test first.** When the `why` opens with `bug fix:`, the session writes the regression
   test per `tdd`, runs it, and confirms it fails for the cause `diagnose` found. Then it attempts the red commit of the
   test files alone, under the [commit contract](../../git/references/commit.md#the-commit-contract), and logs
   `verify red-fix` with status `ok` and evidence `auto`. When a pre-commit hook rejects that commit because the test
   fails, make no red commit: log `verify red-fix` with the no-commit evidence form
   ([flow-state.md](flow-evidence.md#evidence-forms)), and the test lands in the fix commit. Never bypass the hook. The
   fix never changes that test.

    **A declined test.** This applies to either kind of small fix, `bug fix:` or `change:`. When the user declines
    `tdd`'s test, as its [When the user declines a step](../../tdd/SKILL.md#when-the-user-declines-a-step) allows, the
    session writes no regression test. [Small-fix signal 4](../../routing/references/chain-phases.md#small-fix-signals)
    then rests on an existing covering test, named and run, or, when none covers the touched code, on the closest
    executable check: the session names it, runs it as
    [Running it](../../routing/references/repo-conventions.md#running-it) allows, and sees it fail before the change. An
    unauthorized run leaves signal 4 unmet, and step 6 runs. When no such check exists, write no waiver and take step 6.
    Once signal 4 holds, write one `waive test-first` row to `<work-root>/unassigned` as part of the branch's fix
    record: the rows after the latest `skip chain <branch>` row. Its `why` names the declined test, the user's reason or
    `none given`, and what signal 4 rests on: the covering test, or the closest check with the failing result it gave
    before the change. Its evidence is `branch=<branch>`, with `branch` captured in the same shell call as in step 2, so
    that `gate` attributes it to this branch in the shared file. Before writing it, check that fix record for an
    existing `waive test-first` row whose evidence is `branch=<branch>` for this branch, read as for the
    [waiver row](#a-users-change), and write none when one exists; another branch's row never counts:

    ```bash
    agx flow log execute waive test-first ok "$waive_why" "branch=$branch" "$next" --folder "$folder"
    ```

    The `test-first` operand takes no argument. This path writes no `verify red-fix` row, because no red test commit
    exists. Step 5 sees the check pass.

5. **Change, verify, review.** The session writes the change itself, in every mode; no implementer is dispatched for it
   until the failure ladder below calls for a fresh one. Then:
    - Run the verification command as [Running it](../../routing/references/repo-conventions.md#running-it) allows.
      After a declined test, this run sees the named check pass.
    - Check every signal again on the whole change since `base`, by the classification's commands. Leave
      `<work-root>/unassigned/state.tsv` out of the count.
    - Stage the change by name, and dispatch one quality reviewer on the staged diff with
      [code-quality-reviewer-prompt.md](code-quality-reviewer-prompt.md). A fix is one task, so this is its whole
      review.

    When the reviewer returns, log `review fix` with status `ok` on `CLEAR` or `failed` on `FINDINGS`, always carrying
    `kind=quality` in evidence: `kind=quality dispatched=<ts> round=<n> blocked=<seconds>`
    ([flow-state.md](flow-evidence.md#evidence-forms)). Then:
    - On `CLEAR`, commit the staged change and log `verify fix` with status `ok` and evidence `auto`.
    - On `FINDINGS`, no commit exists. Log `verify fix` with status `failed` and evidence
      `branch=<branch> patch-id=<id of the rejected staged diff>`. Fix the findings and continue the same reviewer.

    Each failed round counts on the fix's failure ladder ([two-strike-stop.md](two-strike-stop.md)). After two, dispatch
    one fresh implementer with a packet that carries the findings and the failed attempts. After two more, log
    `block fix`, stop, and ask the user. A blocked fix stays a small fix in progress: it has no `pivot fix <branch>`
    row, so routing resumes it.

6. **Stop and forward on a failed signal.** When a signal fails at any point (the diff passes a size limit, the change
   touches a contract, the test does not cover the touched code, no executable check exists after a declined test as
   [signal 4](../../routing/references/chain-phases.md#small-fix-signals) requires, the work splits into two concerns),
   do these steps in order:
    1. Unstage the change. Its content stays in the working tree. Nothing is committed, reverted, or deleted, and
       earlier commits stay.
    2. Use the same data inputs as for the skip row in step 2. Capture the current branch in this call, put **every**
       failed signal and its exact limit/count into `why` (for example, both the line/file size limit and the new-file
       limit when both fail), and read what the findings leave open into `next` with an `intent` prefix. Then run
       `agx flow log execute pivot "fix $branch" ok "$why" "base=$base_oid" "$next" --folder "$folder"`.
    3. Forward to `intent` with the findings, the signals, and the paths changed so far, and say that the change is
       unstaged and uncommitted. A change that failed only the size signal and fits one task can take the middle path
       there. For a [user's change](#a-users-change), the forward also carries the reason, the report, and whether the
       runtime-change yes and the waiver row are already recorded.

    A user request for the chain, at any point, ends the fix the same way.

7. **Commit the trail on its own.** In the `committed` setting, stage `<work-root>/unassigned/state.tsv` with its own
   `git add` and commit it in one artifact-only commit after the turn's last commit of work. Never put it inside the red
   commit or the fix commit, so each recorded patch-id covers only the change. Under `local`, nothing is staged.
8. **Hand off to `gate`.** Run the [standalone fix commit audit](#standalone-fix-commit-audit) and read its output as
   that section says. Ask the user no question about the commits it lists. Only a failed or misdirected audit holds the
   handoff. Otherwise make the completion handoff in [Gates](../SKILL.md#gates). The handoff carries the one sentence,
   the reason, the signals, for a bug fix the red commit's SHA or the recorded failing run, and the audit's finding
   lines. After a declined test (step 4), it carries instead of the red commit the covering test with its passing run,
   or the named check with its failing run (from the `waive test-first` row's `why`) and its passing run from step 5.
   Execute never pushes.

## Standalone fix commit audit

Run the audit from the repository with no `--folder`. With no bound flow and an open `skip chain <branch>` row for the
current branch, the case where `flow where` exits 5, the audit reads this branch's
[small-fix record](flow-approval-and-audit.md#the-small-fix-record).
[Auditing a branch](flow-approval-and-audit.md#auditing-a-branch) says what it checks and prints.

`<base>` is the `base=` value of the current branch's latest `skip chain <branch>` row. It is the `base_oid` that step 2
recorded. On a resumed fix, read that row from the trail in the directory `agx flow path --folder "$folder"` names. Load
the value into `base_oid` from a scratch file or a structured tool argument, never typed into the command:

```bash
agx flow audit --base "$base_oid"
```

When the work root is not `docs/specs`, prefix the call with `FLOW_WORK_ROOT=<directory>`, as
[flow-state.md](flow-state.md#where-it-lives) says. When `agx` is unavailable on `PATH`, use the
[launcher fallback](flow-commands.md#commands).

Read the result and emit the specific completion handoff or hold report for that outcome:

- Exit 0 with `audit: clean`: make the completion handoff carrying `audit: clean` and any `note:` lines.
- Exit 0 with `unaccounted:`, `mismatch:` or `invalid:` lines: make the completion handoff carrying each verbatim
  finding line and tell the user that `gate` raises each one as a finding. They do not hold the handoff, and no question
  is asked.
- `note:` lines go in the report.
- Exit 0 without the `note: scope not checked (small fix: the size signal applies)` line: a flow is bound to this
  branch, and the audit read that flow instead of this fix. Hold the handoff and report
  `not computed (a flow is bound to this branch)`.
- A non-zero exit: hold the handoff and report `not computed (<reason>)`, with the exact reason the helper printed. Exit
  1 is a usage error, an unexpected failure, or a Git read the audit could not make. Exit 2 is a missing or bad `--base`
  or an invalid storage setting, exit 3 no small fix open on this branch, and exit 4 more than one match. Never run an
  audit by hand in its place.
