# Execute mode and step procedures

Detailed mode procedures, preflight tables, wave-placement mechanics, and mid-run rules for `execute`. Load the matching
section from [SKILL.md](../SKILL.md) when a run enters that mode or step.

## Modes

### `auto pause`

Subagents implement and the user reviews at each wave's end. The argument word `pause`, or a plain request to stop at
each wave, selects it. It runs `auto` with the wave pauses of [wave-pauses.md](wave-pauses.md), and its `enter run`
row's `why` says `auto with wave pauses`.

### `pair`

The session implements each task here with the user and dispatches no implementer. Stage 1, the quality reviews, and the
aggregate review still run through reviewer subagents, except on a host without dispatch
([parallel-lanes.md](parallel-lanes.md#native-dispatch-by-surface)).

- The run pauses at each planned wave's end, never after each task.
- `pair` runs one task at a time, so it applies no independence check and demotes nothing.
- It records commits as `auto` does: the session commits each task's files under **Commit evidence** and writes the same
  `verify` rows, with `commit=<sha> patch-id=<id>`, so that `gate`'s controller audit can check a pair run.
- The `enter run` row's `mode=pair` records the mode, and execute writes no waiver for it.
- Only the user selects `pair`: an explicit argument, a plain request, or a stored preference. A small plan, a long
  plan, or a user who seems to want involvement is not a request for `pair`.
- When the user has corrected this session twice on the same issue, say so and offer a `handoff` capsule plus `/clear`.

### `arena`

The bakeoff races N attempts at **one** task, cross-judges them, picks a base, and grafts the best parts. It enters on
an explicit `arena` argument or an unmistakable bakeoff ask ("race three approaches"), with a single-task input or one
named task of an approved plan. It is not available on the `fix` entry. [arena-mode.md](arena-mode.md) holds its phases,
cost preview, and its one re-run.

### Mode switching after a redirect

Show other modes only after the user stops or redirects a running run and asks for a different style: one question in
the [question format](../../routing/references/contracts/question-format.md), a recommendation from why they redirected,
and a resume from the last verified boundary.

## Step 0: Preflight

### Input classification

Read the rows in order; the first match wins.

| Order | Input                                                                                                    | Run                                               |
| ----- | -------------------------------------------------------------------------------------------------------- | ------------------------------------------------- |
| 1     | `fix`, a small fix `routing` admitted, or a [user's change](fix-entry.md#a-users-change) from `diagnose` | [The `fix` entry](../SKILL.md#the-fix-entry)      |
| 2     | A hand-off from `diagnose`                                                                               | [A diagnosed fix](../SKILL.md#a-diagnosed-fix)    |
| 3     | A work folder whose `brief.md` holds the single task                                                     | A one-task run from the brief's `## Task`         |
| 4     | A plan with at least one `### Task` (a file path, pasted text, or a plan produced this session)          | The plan                                          |
| 5     | One or more already-decomposed tasks, inline                                                             | A one-wave plan, materialized in Step 1           |
| 6     | Otherwise                                                                                                | The matching **Don't use when** row in `SKILL.md` |

- A brief's `## Task` section names the task's **Files**, **Acceptance criterion**, and **Verification**; its packet is
  that section with the brief's **Behavior** and **Acceptance criteria**. Any **Open questions** item the user waived
  under **Decisions** travels in the packet as a decision left to the implementer.
- **Already decomposed** means each task names its file scope, a verifiable acceptance criterion, and a verification
  command. One such task is enough.
- When the work folder of a one-task or inline input holds a spec, that spec's approval applies as **The spec on the
  routine tier** says; with no spec, the approval row's `why` reads `n/a: no spec`.
- An input that is not yet in the repository, such as inline tasks or pasted text, has no folder until Step 1 sub-step 2
  persists it: Step 0 makes no `flow status` call for it, reads it as a flow with no approval, and takes its approval
  from rows 4 to 6 of the approval table below.

### Approval decision table

Read `agx flow status --folder <plan folder>` and its `approvals:` and `hashes:` lines. The artifact is `brief.md` for a
brief and `plan.md` otherwise. Read the rows in order; the first match wins.

| Order | State                                                                              | Do                                                                                                                                                                        |
| ----- | ---------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1     | The latest `review plan` row reports open blocking findings with no later `ok` run | Hold: the plan review is a safety gate. Say what is held and why, and offer `decompose` to revise it.                                                                     |
| 2     | `current`                                                                          | Approved. Run.                                                                                                                                                            |
| 3     | `stale` or `unconfirmed here`                                                      | Present the artifact by path and hash, ask one approval question, and follow [Stale approvals and reapproval](flow-approval-and-audit.md#stale-approvals-and-reapproval). |
| 4     | No approval, and the user told execute in their own words to run this plan or task | That is the approval. Show the path and hash before Step 1 sub-step 6 writes the row.                                                                                     |
| 5     | No approval, and execute was selected by `routing` or forwarded by another skill   | Forward to `decompose`, whose approval question this is ([forwarding.md](../../routing/references/forwarding.md#target-classification)).                                  |
| 6     | Otherwise (for example, only a claim that someone else approved)                   | Ask the user once whether they approve.                                                                                                                                   |

- The words and form never matter: `/agentics:execute <plan>`, "looks good, build it", and "just do it" in reply to the
  question all approve ([approval.md](../../routing/references/contracts/approval.md)).
- "Yes, but change X" is an edit request: send the change to `decompose`, which revises the plan and asks again; the
  approval binds the revised text.
- A no writes `approve <artifact>` with status `declined` and evidence `none`, and the same text is not asked about
  again.
- An unattended run ([unattended.md](../../routing/references/contracts/unattended.md)) cannot ask: it writes a `pause`
  row and reports the artifact as unapproved.

**The spec on the routine tier.** When the work folder holds a `spec.md` and `approvals:` lists no current `spec.md`,
the plan is not approved alone. On the routine tier ([vocabulary.md](../../routing/references/contracts/vocabulary.md))
ask the one combined question that approves the spec and the plan together, and on yes write both rows, each with its
own `--expect-blob`. On the elevated tier, forward to `spec`, resumed at its approval step. A brief stands in for the
spec and the plan, so a brief run reads only `approve brief.md`.

### Resume reconciliation

Before any dispatch, reconcile the branch with the trail. Read `unaccounted:` and `mismatch:` from `flow status`, and
run `git status --porcelain`. Read the rows in order; the first match wins:

1. A `mismatch:` line: show it, and on the user's yes write the repair row it needs (`auto:<sha>` for the commit), as
   [The commit check](flow-approval-and-audit.md#the-commit-check) says.
2. An unaccounted commit that the stack filter of **The boundary check** keeps, or a dirty working tree outside the work
   folder: stop and ask the user what it is. A commit the user made or directed gets its `commit user` row.
3. A `verify <task>` row with status `pending`: that task resumes at its review group's quality review.
4. Otherwise: resume at the first task whose latest `verify` row is not `ok` and that has no `drop` row.

## A diagnosed fix

`diagnose` enters execute in the same session with the root cause, the evidence, the failing reproduction, and the test
seam. Read the rows in order; the first match wins:

1. **The bug blocks a task of a plan running on this branch.** Ask one question: add the fix to this plan as one task?
   On yes, append a `## Divergences` entry to `plan.md` that adds the task with its packet, write its `diverge` row with
   `--entry last`, and run it through Step 1 sub-steps 7 to 10 and Step 2 like any task, with the reproduction as its
   red test. On no, the blocked task stays `blocked:`.
2. **Every small-fix signal holds and no flow covers the branch.** Run [The `fix` entry](../SKILL.md#the-fix-entry); the
   reproduction is its red test.
3. **Otherwise.** Forward to `intent` with the findings, which writes a brief when the middle path applies and starts
   the chain when it does not. Pass the findings so nothing is derived again.

Diagnose's three-attempt limit holds across implementers: see [two-strike-stop.md](two-strike-stop.md).

## Step 1: Load, review, and compute waves

1. **Read and check coverage.** Read the plan, the brief, or the inline tasks. Open the standing-orders register
   ([standing-orders.md](standing-orders.md)). Check coverage: the latest `note coverage` row in `agx flow status` holds
   the result (a coverage re-check on an approved plan writes that row and leaves the plan unedited). Status `failed`
   names the blocking items in its `why`; `ok` means none block. With no such row, read the `**Coverage:**` line in
   `## Provenance`: its `blocking:` field names the items, and `blocking: none`, `n/a — …`, or no line means none block.
   Blocking items stop the run before anything else: name them and go to `decompose`. An item under the line's `waived:`
   field, or named in a `waive coverage` row later than the result, never stops the run.
2. **Persist.** Keep a plan that is already in the repository where it is. Otherwise find `<plan-parent>` with
   decompose's [Resolve the plan parent](../../decompose/SKILL.md#resolve-the-plan-parent) scan and the destination with
   [plan-artifact-identity.md](plan-artifact-identity.md) (never use a provider name, source filename, or session path).
   [chain-artifacts.md](../../decompose/references/chain-artifacts.md) defines the folder layout. A native plan file, a
   scratch file, pasted text, session output, and an inline list all become the same repository artifact; an inline list
   carries its `## Standing orders` register. Change nothing in a plan's approved text. Add no header to a plan that has
   none.
3. **Review and verification preflight.** Read the plan critically. Load
   [verification-preflight.md](verification-preflight.md) before any probe and run its preflight: probe each task's
   verification entry point and pick any authorized bootstrap. Missing verification narrows eligible work; it never
   blocks its own bootstrap.
4. **Concerns.** Pause the affected tasks and report: **Gap:** `<what the plan leaves unspecified>` / **Blocked
   task(s):** `<which>` / **May proceed:** `<independent, fully specified tasks>` / **Question:** `<what unblocks>` /
   **Resolve:** `<answer here, or decompose>`. Hold the affected tasks and their dependents. Only **May proceed**
   continues, as a reduced plan.
5. **Waves and groups.** When slices carry `**Lane:** yes`, apply [parallel-lanes.md](parallel-lanes.md) first and build
   waves only over what it leaves to this session, or over one lane for an `S<n>` argument. Then:
    1. **Build waves** from the dependency graph and declared parallel groups. Tasks that write the same files never
       share a wave. A task the plan lists in a later wave stays in that wave even when it has no dependency; the graph
       never moves a task earlier than the plan put it. After a wave's integration verification and plan divergence are
       done, release each later-wave task that has no edge into that wave while the wave's group review runs.
    2. **Assign review groups.** A slice or a `**Review group:**` override names a group. Any other task takes the
       default `wave-<N>` group of its wave.
    3. **Check the group graph for cycles.** An edge runs from a group to another group that one of its tasks depends
       on. A cycle stops the run.
    4. **Place each task after the named groups it needs.** A task that depends on a task of another slice or
       `**Review group:**` group goes no earlier than the wave after that group's last wave. Move dependents later as
       needed; repeat until nothing moves. Then rename default groups task by task to the `wave-<N>` of each task's
       final wave.
    5. **Check the group graph again.** A cycle stops the run.

    On a cycle, stop before any dispatch, name the groups and the edges that close it, and route the plan to
    `decompose`. Then, in `auto` and `arena`, run the
    [Independence Assessment Checklist](execution-rubrics.md#independence-assessment-checklist) on every
    claimed-parallel group. A group that fails is demoted in place to consecutive one-task waves in dependency order,
    and every later wave shifts after them. The placement loses no parallelism, because a task that depends on another
    group waits for that whole group to clear anyway.

6. **Record the run.** Everything that could refuse the plan has now passed. In order:
    1. Apply [The work-item branch](../../git/references/branch.md#the-work-item-branch): on a detached HEAD or a branch
       on the resolver's `protected` line, create the feature branch with no question and announce its name.
    2. When the invocation approved the artifact, or Step 0 asked and the user said yes, write the approval through
       **Plan approval record** ([standing-orders.md](standing-orders.md)) with the hash the user was shown. For a
       brief, the same steps write `approve brief.md`.
    3. Confirm unrecorded standing orders as [standing-orders.md](standing-orders.md) says. A brief has none to confirm:
       its orders are inside its approved text.
    4. Load [plan-divergence-and-trail.md](plan-divergence-and-trail.md), which holds the row forms. Write the run's
       `enter run` row, then run `agx flow bind --folder <plan folder>`. It binds this branch once and writes nothing
       when the branch is already bound.
    5. In a committed work folder, commit the plan and `state.tsv` in one artifact-only commit when either has an
       uncommitted change. A local work folder is never staged.

    When a write or the commit fails, stop before the first dispatch and report it.

7. **Tiers.** Assign each task a model tier ([execution-rubrics.md](execution-rubrics.md#model-tiers)) and apply the
   first clear model, effort, or tier instruction within its scope.
8. **Stage 1.** Apply [stage-1-dispatch-rule.md](stage-1-dispatch-rule.md) to every task. For an inline task, a tier
   this run assigned at sub-step 7 counts as its stated **Tier**; a task of an untrusted plan still takes Stage 1. Run
   the self-containment check for each skip. Dispatch every required Stage 1 reviewer now, for every wave, in batches no
   larger than the surface's limit
   ([Concurrency and overlap](../../routing/references/surface-dispatch.md#concurrency-and-overlap)). A packet is
   reviewed against the plan, not a diff, so later waves do not wait.
9. **Preview.** Print the [dispatch preview](execution-rubrics.md#dispatch-preview). Do not wait.
10. **Native tasks and todo list.** Load the tasks into the host's task tool as
    [native-plan-projection.md](../../routing/references/native-plan-projection.md) defines. A task whose latest
    `verify` row is `ok` is seeded as done; one with a `drop` row is seeded as dropped. Track
    [workflow steps](../../routing/references/contracts/workflow-steps.md) for Steps 0 to 2 of the selected path and the
    Independence Assessment Checklist in `auto` and `arena`, followed by the plan's tasks (one record per task with its
    text). When an upstream skill already started the list, keep its earlier statuses and skipped steps exactly as
    received and add only this run's records.

## Step 2: Execute waves

### When the user changes the work mid-run

Read the rows in order; the first match wins:

1. A new constraint on how the work is done, in a run with a `plan.md`: append it to the register with its `order` row.
   A run with no `plan.md` (a brief, an inline list, or one task) has no register, and the helper refuses its `order`
   row, so the constraint changes the approved text and takes row 3; a brief's revision goes to `intent`, which reviews
   it and asks for its approval again.
2. An exception to a repository rule for this work: a
   [user-authorized divergence](plan-divergence-and-trail.md#user-authorized-divergence).
3. A change to what a task builds (its scope, criteria, or the task list): let running tasks finish, hold the rest, and
   send the revision to `decompose`. The revised plan needs a new approval, and the run resumes at Step 0.
4. Otherwise: ask one question.

### Task verification, commit evidence, and commit accounting

- **Verify each task as it returns.** On `DONE` or `DONE_WITH_CONCERNS`, make the task commit and run its verification
  command yourself, as the packet's Running-it outcome allows
  ([Running it](../../routing/references/repo-conventions.md#running-it)); under `not authorized: <command>`, leave
  `verify <task-id>` pending and report the command unverified. Keep the output for the reviewer, with secrets redacted
  and the command, exit status, and task label intact. A commit that fails your verification goes back to its
  implementer.
- **Commit evidence.** Stage exactly the files the implementer changed, then commit only those files with an explicit
  pathspec under the [commit contract](../../git/references/commit.md#the-commit-contract), changing nothing in between.
  That commit is the task commit. Write `verify <task-id>` with status `pending` and evidence `auto:<sha>` at once. At
  the group verdict write the final row with `auto:<sha>`: `ok` for a commit the reviewer cleared, `failed` for one it
  rejected. A commit that fails your own verification gets its `failed` row at once. Every fix commit gets its own rows
  the same way; one that answers an integration finding takes its verdict from the continued aggregate reviewer. A
  discarded attempt is undone with `git revert --no-edit <sha>` and its `revert <task-id>` row. Flow artifacts go in
  artifact-only commits. An isolated packet records its own form ([isolated-packets.md](isolated-packets.md)).
- **Commit cadence.** A plan run makes one task commit per task, on the feature branch, and pushes none of them. Every
  other commit of the run (a red, fix, revert, artifact-only or merge commit, or one that **Every commit has a row**
  records) comes from the rule that requires it, never from a choice of cadence. The cadence is settled for every plan:
  no skill asks the user how often to commit. Push, pull request creation and merge keep their own go-ahead.
- **Every commit has a row.** Write the row when the commit is made, naming it as `auto:<sha>`: a bare `auto` reads
  `HEAD`, which a trail commit may have moved. A task's rejected round keeps its commit and gets its `failed` row, and
  the correction gets its own commit and rows, never an amend. A commit of the run that no task, red, or artifact-only
  row covers, such as a follow-up the user directed, a controller-rule waiver's change, or a closing documentation
  commit, gets `commit <fix|triage|user|housekeeping>` with evidence `auto:<sha>`, as
  [flow-state.md](flow-approval-and-audit.md#commit-accounting) says.

### The boundary check

For a plan run, at each review-group boundary (once the group's quality review and any integration review it triggered
have cleared) and again before the completion handoff:

1. Run `agx flow status --folder <plan folder>` and read `unaccounted:`.
2. When this branch's pull request is based on another open pull request's branch, the helper still measures from the
   default branch. Use [the stack-base resolver](plan-divergence-and-trail.md#the-stack-base-resolver) to obtain
   `base_oid`. Keep only the listed commits that `git rev-list HEAD --not "$base_oid"` holds. A commit outside that
   range belongs to the lower layer. If the resolver fails, stop the filter and report `not computed`.
3. When the count is larger than the ids listed, list the rest of that range yourself.
4. Nothing releases a dependent group, and the handoff waits, while a kept commit remains. A commit this run made gets
   the row it lacks now, and the report names it. Any other commit stops the run for one question: what is it? A commit
   the user made or directed gets its `commit user` row. A line that reads `not computed` is reported as such, never
   read as `none`.
5. At the same boundary compare the changed paths of every task and fix commit of the group with its packet's
   `**Files:**`. A path the packet did not name writes the `diverge` row
   ([plan-divergence-and-trail.md](plan-divergence-and-trail.md)) before any dependent is released.

For a standalone fix, run the [standalone fix commit audit](fix-entry.md#standalone-fix-commit-audit) before handoff. It
is `agx flow audit` on the current branch's small-fix record, measured from the `base=` of its `skip chain <branch>`
row. The artifact-only trail commit needs no row. Its finding lines travel with the handoff, and `gate` raises them as
findings; no question is asked. Only a failed or misdirected audit holds the handoff: report `not computed (<reason>)`
and hold it until the audit runs on this fix's record.

### Review groups and snapshot rules

**Reviews judge a snapshot, never the live tree.** Siblings share one working tree. Hand each reviewer the list of
commits it judges, each labelled with its task, never a range and never the working tree. Prefer a verification command
scoped to the task. A repository-wide command runs after every implementer in the wave has returned, or its output is
reported as taken with sibling writes in flight. Never present output taken over a changing tree as a task's stable
result.

**Review group rules:**

- A group is a slice of a sliced plan, a wave (`wave-<N>`) of an unsliced plan, or the tasks that share a
  `**Review group:**` name. A demoted wave stays one group.
- When every task of the group that is not stopped has a task commit that passed your verification, or a task commit and
  a `waive` row that waives or defers its verification, dispatch one fresh quality reviewer with the commit list, the
  verification output, those `waive` rows, and the packets. A stopped task does not hold the group: the review covers
  the tasks that committed and names the stopped task, whose dependents stay held.
- Findings name a task. Send each to that task's implementer, and continue the same reviewer with the fix commits. A fix
  round counts once on the ladder of the task it names.
- A group whose quality review carries its `waive "quality-review <group>"` row counts as cleared at that row, here and
  in **Dispatch per mode**. Its aggregate review still runs, with the waiver in place of the quality verdicts.
- A task in another group that depends on this group starts once the group has cleared its quality review and any
  integration review it triggered, and its **boundary check** found no unaccounted commit and no unrecorded scope
  change. A task whose prerequisites are all in its own group starts once each has a task commit that passed your
  verification, or a task commit and a `waive` row that waives or defers its verification; the dependent's packet names
  each such unverified prerequisite. Independent tasks never wait.
- Every reviewer return writes one `review` row with `kind`, `dispatched`, `round`, and `blocked`. Take both times from
  the clock: run `date -u +%Y-%m-%dT%H:%M:%SZ` (PowerShell:
  `(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')`) at dispatch and at return. `blocked` is the seconds
  of that interval in which no implementer ran. The target is the task id for a packet review, the group name for a
  quality or integration review, or `final`.
- When `gate`, `ci`, `copilot`, or a person reports a defect that a review cleared during the run, map its severity with
  [severity.md](../../routing/references/contracts/severity.md). A Critical or Important one writes an `escape` row with
  phase `execute`, status `failed`, evidence
  `found-by=<gate|ci|copilot|person> severity=<critical|important> review=<kind> word=<the finder's word>`, and the
  `next` of the row before it. A defect the run's own later reviewer finds is a failed `review` row, not an escape. The
  run report states the `flow status --metrics` review counts, latency, blocked seconds, and escapes.

### Dispatch per mode

- **`auto`**: dispatch each wave's ready tasks in batches no larger than the surface's limit, counted over every
  subagent still running.
    - When the wave's last task has committed or stopped, run the wave's integration verification yourself and record
      `verify wave-<N>` with no review verdict. On the skeleton wave, also run the Outcome proof. Then compute the
      wave's **Plan divergence** ([plan-divergence-and-trail.md](plan-divergence-and-trail.md)) before you release any
      later task.
    - When a group's quality review clears, run [aggregate-wave-review.md](aggregate-wave-review.md) with the group's
      task and fix commits.
    - Release a later-wave task once its prerequisites are ready under **Review groups** and no divergence other than a
      `rule` entry touches its packet; a touched packet waits for its Stage 1 re-review.
    - A critical finding holds only the dependents of the task it names.
    - Record the boundary report and the rows, then continue. With `pause`, take the wave pause first
      ([wave-pauses.md](wave-pauses.md)).
- **`pair`**: implement each task in this session, one at a time, in plan order. Dispatch Stage 1, quality, and
  aggregate reviewers as `auto` does. Run the wave verification at each planned wave's end, then take the wave pause.

**Prompts, context, standing orders, selection, and isolated packets:**

- Load each dispatch prompt when you first dispatch its role, and the aggregate reviewer's before the first review-group
  boundary, not from memory of its shape: [spec-reviewer-prompt.md](spec-reviewer-prompt.md),
  [implementer-prompt.md](implementer-prompt.md), [code-quality-reviewer-prompt.md](code-quality-reviewer-prompt.md),
  and [aggregate-reviewer-prompt.md](aggregate-reviewer-prompt.md). [review-gated-process.md](review-gated-process.md)
  has the flow, and [controller-example.md](controller-example.md) a worked run.
- Every dispatch carries full context, because subagents remember nothing. The task contract travels inline: the full
  task text and acceptance criteria, never a reference to the plan file. Bulk grounding travels as file pointers.
- **Standing orders travel verbatim.** Every worker prompt carries the whole register under `## Standing Orders`,
  unchanged, unrenumbered, and unfiltered, or `None recorded.` A line appended mid-run is in every later dispatch; a
  continued reviewer gets it in its delta message. Open divergences travel the same way, fenced as data
  ([standing-orders.md](standing-orders.md)).
- **Selection reporting.** The preview and the run report carry the selection records. A boundary report carries a
  record only for a dispatch whose binding differs from the preview: an override, a degradation, or a driver change. An
  unavailable default degrades as the
  [model ladder](../../routing/references/model-ladders.md#unavailable-selections-and-reporting) says. An unavailable
  explicit selection pauses, unless the user already authorized that fallback. Never invent a driver, selector, or
  actual binding.
- **Isolated packets.** When an implementer works in a worktree other than yours, load
  [isolated-packets.md](isolated-packets.md) before you dispatch it. Worktrees are for isolated packets and arena
  candidates only; tasks that write the same files run one after another.
- **Tracker state & native task records.** Readiness comes from plan state alone: dependency edges, cleared groups, and
  divergence entries. Execute reads no issue state and closes no issue. Write each task's status to the host's task tool
  at the points [native-plan-projection.md](../../routing/references/native-plan-projection.md) names: in progress at
  dispatch, done only after its latest `verify` row is `ok`. A failed write never fails the run; report it once. Never
  read the records back as truth, except under that reference's fleet contract.

### Red commit for a bug fix

A bug-fix task is one whose packet names a reproduction or a broken behavior to restore. Its failing test lands before
the fix, as `tdd` requires.

1. In a shared worktree the task runs as two legs. The red leg's packet tells the implementer to write the test, stop,
   and return `RED` with the test files and the failing output, uncommitted.
2. Run the test yourself and confirm it fails for the reason the packet names.
3. Attempt the red commit of those files alone, in the **Commit evidence** order, and write `verify red-<task-id>` with
   evidence `auto`. When a pre-commit hook rejects that commit because the test fails, make no red commit: write
   `verify red-<task-id>` with the no-commit evidence form ([flow-state.md](flow-evidence.md#evidence-forms)), and the
   test lands in the task commit with the fix. Never bypass the hook.
4. Continue the same implementer for the fix leg, or re-dispatch it with the red evidence when the surface cannot
   continue a subagent. The fix leg is a continuation.
5. A red test that passes, or fails for another reason, goes back to the implementer and counts on the ladder as
   `verify <task-id>` with status `failed` and evidence `cause=red`.
6. In an isolated worktree the implementer makes the red commit and returns `Red: <sha>` beside `Head: <sha>`; confirm
   it touches only test files and fails at that commit. In `pair` the session makes the red commit. A later change to
   the red commit's test files is an Important finding that `gate` owns.

## Repeated trouble in a settled design

A settled design (the plan, the spec, or an earlier `deepen` summary) can be the cause of trouble that one more fix does
not clear. Each time you write a `diverge`, `pivot`, or `block` row, or a reviewer finding or an implementer concern
arrives, compare it with the run's earlier ones. Two of the same shape across different tasks make a pattern:

- the same workaround in unrelated callers;
- an escape hatch to compile (`any`, a cast, an optional field that is always set);
- callers that must know a module's internal rules;
- special-case branches for unrelated edge cases.

One task failing again and again is [two-strike-stop.md](two-strike-stop.md), not a pattern. When a pattern forms, hold
the tasks that touch the design, let independent tasks continue, and ask one question in the
[question format](../../routing/references/contracts/question-format.md): send the evidence to `deepen`, whose scrap
trigger decides if the design is the problem? Recommend yes. The evidence is each entry or row, its task, its files, and
the shared shape.

- **Yes.** Forward to `deepen` under the
  [forwarding protocol](../../routing/references/forwarding.md#target-classification) with that evidence, and write
  `pause run`. A redesign returns as a revised plan through `decompose`, and the run resumes at Step 0.
- **No.** Release the held tasks and write one `waive "deepen <shape>"` row, where `<shape>` is one hyphenated word. Do
  not ask about that shape again.

An [unattended](../../routing/references/contracts/unattended.md) run cannot ask: it writes the `pause` row with the
evidence and reports.
