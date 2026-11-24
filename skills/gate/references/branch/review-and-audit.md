# Branch mode: Steps 7–9 — Verify, review, audit, and converge

Split from [branch-workflow.md](branch-workflow.md). Covers Step 7 (test verification), Step 8 (multi-axis review and
controller audit), and Step 9 (spec convergence).

## Step 7: Verify tests

Run the verification command once, on the tree that Steps 3 to 6 left. When Step 3 left paths uncommitted, that tree is
not what Step 8 reviews or Option 2 pushes. Allocate a fresh absolute worktree path under `<scratch>` and retain that
path as `test_worktree` until cleanup. Create the detached worktree with
`git worktree add --detach "$test_worktree" HEAD`, run verification there, then run
`git worktree remove "$test_worktree"` from the feature worktree. Say where verification ran on the `Tests:` line. The
user's uncommitted edits stay where they are. The command is the one that
[repo-conventions.md](../../../routing/references/repo-conventions.md#verification-command) resolves. When none
resolves, discover the test entry points (workspace and package scripts included), map the changed paths to every
relevant command, and run each; when the mapping is uncertain, run the broader suite.

[Running it](../../../routing/references/repo-conventions.md#running-it) decides whether the command runs on this
branch. When the user invoked gate on the branch in their own turn, that invocation is the named-verification request
the rule describes. When gate is reached from another skill's hand-off, the rule's other clauses decide. A run the rule
does not authorize is reported `not run` with the command that would have run, as its **Not authorized** clause says.

| Outcome  | When                                                                                                                          | `Tests:` line                                                                                           | Then                                                              |
| -------- | ----------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------- |
| passed   | every command ran and exited 0                                                                                                | `passed (<command>: <summary>)`                                                                         | continue                                                          |
| failed   | a command ran and failed                                                                                                      | `failed (<N> failures in <command>)`                                                                    | stop, unless the user accepts the risk                            |
| not run  | a command exists and cannot run here: a missing toolchain, credential, service or device; or Running it does not authorize it | `not run (<command> cannot run here: <cause>)`, or `not run (<command>: not authorized on this branch)` | continue; the gate stays BLOCKED until a pointer or an acceptance |
| deferred | a plan task's verification names an owner other than this session (the user, or CI)                                           | `not run for <task> (owner: <owner>, <command>)`                                                        | the same as not run                                               |
| none     | the conventions say the repository has no command, and discovery finds none                                                   | `none in this repository`                                                                               | continue; a recorded substitute check is the evidence             |

When the outcome is passed for some commands and not run for others, print one line per command. The first matching row
of the table decides each command.

**A failure.** Show the failures and stop as [Stops](branch-workflow.md#stops) says. The review is not spent on a
failing branch. When the user accepts the risk in writing and directs the work on, write `waive tests` with their reason
or `none given`, and continue to Step 8. The tests stay `failed` in the report, the verdict row and the pull request
body: an acceptance moves the work on and never turns a failure into a pass.

**Not run and deferred.** A pointer to a run of the same command on the same head, such as a CI run link, clears the
line as passed or failed by what that run shows. Otherwise the gate stays BLOCKED until the user accepts the risk in
writing, which writes `waive tests`. Steps 8 to 10 run meanwhile and report their own results.

**No tests.** The change is verified by the closest executable check, seen to fail before and pass after, which a
`verify red-<task>` or `verify red-fix` row in the no-commit form records. Gate reads that row as evidence, not as a
finding. A report that says `none in this repository` never reads as a pass.

**A substitute where tests exist.** A task's `verify red-<task>` row in the no-commit form, in a repository whose
command ran, records that `tdd` found a failing test impractical for that task and ran the closest executable check
instead. Gate reads it as that task's red evidence, not a finding, and adds `; substitute for <task>: <the row's check>`
to the `Tests:` line. The suite's result never covers that task's behavior, so the pull request body lists the
substitute as an exception (Option 2). A task's latest `verify red-<task>` row whose `why` reads
`impractical test, unconfirmed` records a substitute no one confirmed, and Step 8 item 2 raises it as a finding. A later
red row that records a real failing test replaces the substitute: the older row stays in the trail, and it is no finding
and no exception. A task with neither a red row nor a test that covers it is reported as the trail check says.

**Re-runs.** The command re-runs automatically once after a change set gate did not author, under the re-run budget in
[Rules for every step](branch-workflow.md#rules-for-every-step).

## Step 8: Review and audit

This step runs once Step 7 passed, found no tests, could not run, or the user accepted its failure. Nothing here asks
the user to opt in, and nothing posts.

**1. Review.** The review is a process gate. `--no-review`, or a plain-language skip, declines it: say once what goes
unseen ("Standards, Spec and Security will not see this diff"), write `waive review`, and record `Review: waived`. On a
security-sensitive diff (auth, crypto, input handling, IaC, or a dependency manifest), a declined review still runs the
fast Security lane alone, composed from
[security-fast-scan.md](../../../review/references/security/security-fast-scan.md) over the same target, and its line
says "Standards and Spec will not see this diff; the fast Security lane still runs".

Pin the target first. Both must hold: `git -C "$FEATURE_WORKTREE" rev-parse HEAD` equals
`git rev-parse "refs/heads/$FEATURE_BRANCH"`, and `git -C "$FEATURE_WORKTREE" diff --name-only <compare>...HEAD` lists
at least one file. Then run `/agentics:review axes <compare> --security --fast` with the feature worktree as the working
directory, and name the Spec lane's source in the same invocation: the session folder's `spec.md`, else its `brief.md`,
else its `intent.md`, else its `plan.md`. Deep security runs only when the user asks for it. After the review returns,
read `HEAD` again. A result counts only when `HEAD` still equals the pinned SHA; otherwise discard it and run the review
again from the feature worktree.

Read the closing `Lanes:` line. When Standards, Spec with a source, or Security shows `skipped` or `failed`, or the line
is absent, record `Review: incomplete (<lane> not run)`, with the lane names from that line, or `Lanes line absent`.

A small-fix hand-off whose `review fix` row names the current `HEAD` in its `commit=` pair runs only the fast Security
lane, because the fix's own review already covered Standards. When the diff changed after that review, the full review
runs.

**An incomplete Security lane.** The fast Security lane marks its tally
`incomplete: <U> unsettled, settle with full depth` when it could not settle a candidate, so zeros from it are no clean
result. The mark counts on every fast Security result gate reads: the full review's, the lane run alone after a declined
review, and a small-fix hand-off's. Holding it is a process gate. Its cost: a path from source to sink that is split
across helper files can ship with no finding. The ways forward are full depth, which is
`/agentics:review axes <compare> --security` on the same pinned target and runs only on the user's yes, and its result
replaces the fast one; or a decline. Until then the lane is an open item. The gate status reads
`NOT READY: Security lane incomplete (<U> unsettled)`, and, as for a converge gap, Step 11 offers no merge and no pull
request. A decline in any wording writes `waive security-depth` with the user's reason or `none given`. The `Security:`
line then keeps the tally with its mark and adds `; full depth waived`, and Step 11 proposes as usual.

**2. Decision trail.** Read `flow status --folder <folder>` and the rows as
[What a reader sees](../../../execute/references/flow-state.md#what-a-reader-sees) defines them. Each failure is an
Important finding:

- **The plan approval.** The `approvals:` line must list `plan.md` (or `brief.md`) as `current`. A `stale` or
  `unconfirmed here` approval is the user's own approval, a safety gate: gate never writes it for them. Present the
  artifact by its path and the hash from its `hashes:` line, and ask once whether the user approves the current text. A
  yes writes the `approve` row through the guard, as Step 5 shows. A no writes `approve <artifact> declined`, and the
  gate holds: the finding takes no accept or defer, and Step 11 proposes keeping the branch while the user revises the
  artifact through `decompose` (for `plan.md`) or `intent` (for `brief.md`) and approves it again. The unattended
  default is to ask nothing and hold the same way.
- **The tasks.** Each in-scope task needs a latest `verify <task>` row with status `ok`, or a `drop <task>` row, which
  closes it. Skip a `verify` row that an `invalid:` line of `flow status` names, matched as
  [Invalid rows](../../../execute/references/flow-commands.md#invalid-rows) says: the latest row is the latest one no
  such line names, and an invalid row completes nothing. A task whose latest row is `pending` or `failed`, or that has
  no row, is a finding that names the task. A task whose verification was deferred to an owner is reported by Step 7,
  not here.
- **A substitute no one confirmed.** A task's latest `verify red-<task>` row, when its `why` reads
  `impractical test, unconfirmed`, is a finding of lane `trail` on `task:<task>`: the task's red evidence is a
  substitute check that `tdd` ran with no one to confirm it. The trail only grows, so an earlier row of this kind stays
  after the user chose the fix and `execute` recorded a real failing test in a later `verify red-<task>` row for the
  same task. That later row is the task's red evidence now: the older row is no finding, and the substitute is no
  exception. Step 11 asks it as it asks any finding. Fixing it is a real failing test through `execute`; accepting it
  says the substitute stands in for a test; an unattended run stops at the question, as
  [Unattended runs](branch-workflow.md#rules-for-every-step) say, so the substitute always reaches the user. A
  substitute the row does not mark unconfirmed was confirmed when the task ran, and is no finding.

On a [small-fix hand-off](branch-workflow.md#small-fix-hand-off), the trail is that record instead. A no-flow run has no
plan and no tasks, so this check reports nothing.

**3. Controller audit.** It runs when the review is waived too, because it reads git and the trail and does not judge
the diff, and it runs in every execution mode, `pair` included. The helper computes it, as
[Auditing a branch](../../../execute/references/flow-approval-and-audit.md#auditing-a-branch) defines, and gate never
audits by hand. With no flow and no small fix, gate does not call it and prints no `Controller audit:` line. On a flow
run, call `agx flow audit --base <compare> --folder <folder>`. On a
[small-fix hand-off](branch-workflow.md#small-fix-hand-off), call `agx flow audit --base <compare>` with no `--folder`,
so the helper reads this branch's small-fix record.

A Step 2 `flow unavailable`, a failed `flow where` or a helper that cannot run, is no no-flow run. Gate reports
`Controller audit: not computed (flow unavailable: <reason>)` and the hold below applies, so gate never reaches `READY`
without an audit line.

The audit's last line sets the `Controller audit:` line:

| Audit output                                            | `Controller audit:` line                                             |
| ------------------------------------------------------- | -------------------------------------------------------------------- |
| `audit: not run (this branch is not bound to the flow)` | `Controller audit: not run (branch not bound)`, held as not computed |
| `audit: clean`                                          | `Controller audit: clean`                                            |
| `audit: <n> findings`                                   | `Controller audit: <n> findings`                                     |
| any non-zero exit, or the helper is unavailable         | `Controller audit: not computed (<reason>)`                          |

- A `note: scope not checked (<reason>)` line adds `; scope not checked (<reason>)` to the line. It is no finding.
- Each `note: exempt` and `note: merge commit` line is listed in the report. The audit does not see an edit made inside
  a merge commit, and the report says so.
- Each `unaccounted:`, `mismatch:`, `out-of-scope:` and `invalid:` line is an Important finding of lane `audit`.
  Reproduce the audit line verbatim in the finding rather than paraphrasing it. Its locator is `commit:<sha>` for an
  `unaccounted:` or `mismatch:` line, with the sha the line names, `<path>` for an `out-of-scope:` line, and
  `task:<target>` with the invalid row's target for an `invalid:` line, as in `task:task-3` for an invalid
  `verify task-3` row, or `task:fix` and `task:red-fix` on a small fix. Step 11 asks each one as a finding. Gate asks
  nothing else about them and writes no row here.

`flow status` prints an `unaccounted:` line too: a count and short shas, measured from the default branch. The audit
prints one `unaccounted:` line per commit, with 12 characters of its sha and its subject, measured from `<compare>`.
Gate uses only the audit's lines. A line that ends `(row not attested here)` names a commit that a hand-written row, or
a row from another clone, records. This clone has no proof of that row, so the line stays a finding.

A `mismatch:` line's repair is a new row for the same event and target, with `auto`, or `auto:<sha>` for the commit that
now holds the change. Its author writes it, and gate writes none. When this clone lacks the row's commit, the mismatch
may come from a lower layer that was rewritten after the row was written: fetch that layer, or rebase onto it, and run
gate again before anyone writes a repair row.

On a slice-bound branch, `out-of-scope:` lines for paths that a lower slice's tasks list usually mean the branch is not
stacked on, or not rebased onto, that lower slice. They are no scope breach: rebase or stack the branch onto the lower
slice and run gate again.

An `invalid:` line of the audit names a `verify` row that fits no form. Its repair is a new `verify` row for the same
target with `auto` or `auto:<sha>`, and gate writes none. The `invalid:` lines of `flow status` also name `waive` and
`demote` rows, which the audit does not report. They are no finding. The report lists each `waive` line as a waiver that
was not recorded, and the gate it named is treated as not waived.

On a small-fix hand-off the audit prints `note: scope not checked (small fix: the size signal applies)`. Re-run
[small-fix signal 5](../../../routing/references/chain-phases.md#small-fix-signals) on
`git diff --numstat <compare>...HEAD`, without the flow artifacts, and a file past a limit is a finding.

A `commit fix` or `commit triage` row with no `review` row that names its commit is covered by this run's review, and
the report says so.

**An audit not computed.** Gate calls the audit only when Step 2 found a flow or a small fix, so any non-zero exit is a
failed audit, exit 3 with `--folder` included. `<reason>` is the helper's `flow: ` message, or the `flow unavailable`
reason from [Commands](../../../execute/references/flow-commands.md#commands). Gate runs no audit by hand in its place.
Holding the branch is a safety gate under verify before claim: with no audit output, nothing can be claimed about the
branch's records. The gate status reads `NOT READY: controller audit not computed`, and Step 11 offers no merge and no
pull request. The ways forward are to repair the helper and run gate again, or to keep the branch. No `waive` row exists
for this hold.

`Controller audit: not run (branch not bound)` on a flow run is the same hold: the audit checked nothing, so it reads as
not computed. The gate status reads `NOT READY: controller audit not run (branch not bound)`, and the ways forward are
the same, with this repair: bind the branch as Step 2 does, with `--slice <ids>` when the plan has slices, or, on a
detached `HEAD`, check out the feature branch first. Then run gate again.

**4. Red-commit check.** A bug fix commits its failing test before the fix. Find each red commit: every
`verify red-<task>` row of the flow, every `verify red-fix` row of the
[small-fix record](../../../execute/references/flow-approval-and-audit.md#the-small-fix-record), and a red SHA the pull
request description names. A row in the no-commit form has no commit to check and is skipped. A row that an `invalid:`
line names is skipped too, matched as [Invalid rows](../../../execute/references/flow-commands.md#invalid-rows) says: it
names no red commit, and the audit's `invalid:` lines report it. So is a row whose task is outside the selected slice,
and a row whose `commit=` is an ancestor of `<compare>` or whose `patch-id=` matches a commit in
`<base-sha>..<compare>`: that red commit belongs to a lower pull request. For each other one:

1. Resolve it: the row's `commit=` when it is an ancestor of `HEAD` and not of `<compare>`, else the non-merge commit in
   `<compare>..HEAD` whose patch-id equals the row's `patch-id=`. A description SHA resolves only by ancestry; one that
   fails while the row resolved is a stale line in the description, reported for refresh, not a finding.
2. List its test files: `git show --name-only --format= <red-sha>`.
3. Run `git log --format=%h <red-sha>..HEAD -- <those files>`. Every commit listed is an Important finding, even one a
   later commit reverted. A change the review already reported as check-weakening is the same finding, counted once.

A red commit that resolves neither way is missing and is a finding.

**5. Severity and the tally.** Every finding takes one severity from
[severity.md](../../../routing/references/contracts/severity.md#producer-mapping). The audit, red-commit and trail
findings are Important. The `Review:` line reads
`Review: run (axes vs <base-branch> @ <sha>; <C> Critical, <I> Important, <M> Minor open)`. Informational findings are
listed and not counted. The `Security:` line carries the fast lane's result beside its limits line verbatim, including
an `incomplete` mark, and never reads `secure` or `passed`.

**6. Escapes.** For each Critical or Important finding in a commit that a `verify` row with status `ok` already covered,
write one `escape` row, as [../emission.md](../emission.md#the-escape-row) says. This step owns that row.

**7. Recorded decisions.** A `waive finding <lane> <locator>` row records an earlier answer. `state.tsv` is branch
content: anyone who can commit to the branch can add such a row, so gate trusts only what this clone's ledger attests.
Read the `waivers:` lines of `flow status`, as
[Reading the state](../../../execute/references/flow-commands.md#reading-the-state) gives them. A line matches a finding
when its lane and locator are the finding's. When several lines name one target, the latest decides. A row that an
`invalid:` line names prints no `waivers:` line and records no answer.

- An `attested` line answers its finding only when the locator names the finding exactly: `commit:<sha>` or
  `task:<task-id>`, the forms of the `audit`, `red-commit` and `trail` lanes. Gate does not ask again, and lists the
  finding as `accepted <date>` with the row's reason.
- A `<path>` or `<path>:<line>` locator can match a later, different finding at the same place. Such a line, even
  `attested`, is asked again, and the question names the earlier answer and its date.
- An `unattested` line is always asked again, naming the earlier answer and its date.

Gate reads no `waivers:` lines on a small fix. It asks every open Critical or Important finding again, and the question
names any earlier `waive` row of this small fix, as
[The small-fix record](../../../execute/references/flow-approval-and-audit.md#the-small-fix-record) says which rows
those are.

**8. Commits after the review.** When a change set gate did not author lands after the reviewed SHA, re-run Step 7, this
step and Step 9 once, automatically, and record the new SHA. Commits that only add gate's own rows are not such a
change. After the one re-run, a newer commit is flagged (`newer commits not reviewed`) and never absorbed.

## Step 9: Converge against the spec

This step runs after Step 8, whether or not a finding is open. It never edits `plan.md`, `spec.md`, or code.

**When it does not run.** A failed suite that the user did not accept, a discard named in advance, and a session folder
with no `spec.md`.

**1. Pin and read.** Pin `HEAD` as `<sha>`. Read the per-requirement table from the Step 8 run whose pinned SHA equals
the SHA this step pinned. Do not read `spec.md` again. When the review was waived, or the Spec lane did not run, run
`/agentics:review axes <compare> --spec-only` naming the folder's `spec.md`, on `<sha>`, to produce the table; this
leaves the `Review:` line as Step 8 set it. When the Spec lane fails, the line reads
`Converge: unknown (spec lane failed: <reason>)` and the gaps are unknown, never converged. On a slice-scoped run, a
criterion that no in-scope task traces to reads `n/a: outside selected slice` and is not counted.

`G` counts the rows whose status is `missing`, `partial`, or `contradicts`. `unrequested` rows are listed and never
counted.

**2. Recorded divergences.** A gap is covered when a recorded `rule` divergence's scope holds it (the task its target
names, or the whole run for `run`) and the rule the record names is the rule the gap is about. A covered gap leaves `G`
and is listed as covered by that record. A `Divergence:` line found only in a commit body or the pull request body
covers nothing until the user confirms it in this session.

**3. Report.** One line: `Converge: converged (<N> requirements)` or `Converge: <G> gaps`, with
`; <K> covered by recorded divergences` when `K > 0`. When `G > 0`, render the gap table:

```text
| Requirement | Location in spec.md | Status | Evidence or gap | Severity |
| ----------- | ------------------- | ------ | --------------- | -------- |
```

**4. Withheld destinations.** While `G > 0`, the proposal offers no merge and no pull request. This is a process gate:
the cost is that the branch ships without what the spec asks. It names the gap count and the ways forward: route the
gaps to `decompose` to add tasks for them, keep the branch, or decline the hold. A decline in any wording writes
`waive converge` with the user's reason or `none given`, records `Converge: <G> gaps (waived)`, and Step 11 proposes as
usual. A gap closes when a converge pass that saw its fix reports it satisfied.

**5. Re-runs.** Converge re-runs under the same rule as the review: once automatically after a fix that touches a gap,
and whenever the user says "review it again". While a gap is withheld, gate may offer a converge re-run.
