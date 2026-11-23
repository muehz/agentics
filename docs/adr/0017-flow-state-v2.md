# Flow state records the work item, and a review budget resets when the reviewed content changes

This record decides the second version of the flow-state model: what identifies a flow, how it opens and closes, what an
approval binds, how every commit and every state gets a row, how rows are scoped to a slice or lane, and what each
review budget counts and when it resets. It also decides how a repository keeps planning files out of its commits: one
storage setting, committed by default or local to the clone. The problems it answers are listed in
`docs/designs/skill-audit-2026-11/consolidated-issues.md`. The audit read the deleted `flow.sh` and `flow.ps1`; every
claim used here was checked again against `hooks/go/internal/flow/` and `skills/execute/references/flow-state.md`, where
the logic now lives ([0012](0012-flow-state-is-a-verb-on-the-hook-binary.md)).

Three boundaries hold throughout. What counts as an approval, and which words give or withdraw one, is decided in
record 0014. Who may make a commit or create a branch is decided in record 0016. How far text from another source is
trusted is decided in record 0018. This record decides only what the trail records and what a reader of it reports.

## Rules

### A flow is its work folder; it opens with its first row and closes on every destination

- **Answers:** C6.1
- **Rule:** A flow is identified by its work folder, never by a branch.
    1. **Lookup.** `flow where` matches a branch only through a `bind` row that names it or a `state.tsv` the branch
       changed. On the default branch it also matches the one open flow whose `state.tsv` has uncommitted changes. The
       fallback that adopts "the only active flow that has no `bind` row" is removed, so a stray unbound flow is never
       picked up by another request. The fallback removed is the one for committed work folders. Under the `local`
       storage setting decided below, one pre-bind match is the stated exception, with three limits: it applies only on
       the default branch, only before the flow's first approval, and only when exactly one open local-store flow has no
       `bind` row. With more than one such flow, lookup exits 3 and the user names the folder.
    2. **Open.** A flow opens with its first row, written with `--folder`. The default branch stays unbindable
       (`hooks/go/internal/flow/bind.go:32`). No chain commit is made on the default branch: a flow has a `bind` row for
       another branch before its first commit, and a phase that would commit on the default branch first creates and
       binds a feature branch under record 0016, and stops only when that fails.
    3. **Close.** A flow is closed by a `done` row, or by merge: the default branch's tip holds its `state.tsv`, and the
       latest `branch` verdict in it is `ok` and covers the whole plan (see the slice rule below). The draft pull
       request path therefore closes when the pull request merges, with no further row
       (`skills/gate/references/emission.md:80` writes none today). Under the `local` storage setting decided below, the
       default branch never holds the `state.tsv`, so the merge is recorded as a `done` row in the clone-local store,
       which needs no commit. `routing`'s continuation check, the first to run in that clone after the merge, writes the
       row through the flow helper, and no other skill does, because `recall` writes no rows; it writes the row when the
       latest `branch` verdict is `ok` for the whole plan and either the host reports the bound branch's pull request
       merged or the bound tip is reachable from the fetched base. With no host and a squash merge neither can be read,
       and the skill asks the user once. Lookup never matches a closed flow, so a reused branch name cannot resolve to
       finished work.
    4. **Exit codes.** The numbers 0 to 5 keep their meanings. Exit 3 means only "no flow is bound to this branch or
       changed by it". `flow-state.md` states one response per caller: `routing` runs the small-fix classification and
       otherwise starts at `intent`; a skill invoked for a named phase asks the user for the folder and passes
       `--folder`. No skill reads exit 3 as permission to pick a remaining flow.

    (verified: hooks/go/internal/flow/lookup.go:170, 2026-11-11)

- **Default / Rejected:** identity by work folder, explicit binding, and closure derived from the merged verdict;
  rejected: keeping the unbound-flow fallback, because unbound flows are normal and rarely closed, so a second request
  on a branch inherits the first item's phase; rejected: in `committed` mode, a `done` row written after the pull
  request merges, because no skill runs when someone merges on the host and the row would need its own commit on the
  default branch, whereas `local` mode writes it at the next lookup with no commit; rejected: letting the default branch
  bind, because every later flow would then match it.
- **Later work changes:** `hooks/go/internal/flow/`, `skills/execute/references/flow-state.md`,
  `skills/routing/references/chain-phases.md`, `skills/routing/SKILL.md`, `skills/gate/references/emission.md`,
  `hooks/README.md`

### An approval binds normalized text, and no required step writes inside it

- **Answers:** C6.2
- **Rule:** The approval hash covers the artifact's text outside `## Divergences` (`intent.md`, `spec.md`, `plan.md` and
  the middle path's `brief.md` of [0013](0013-proportional-delivery-paths.md)) and `## Standing orders` (`plan.md`),
  after normalization, and nothing the workflow requires after an approval changes that text.
    1. **Normalization.** Line endings become LF, trailing whitespace on a line is dropped, and trailing blank lines of
       the kept text are dropped in every artifact. Today they are dropped for `intent.md` and `spec.md` only, so the
       first `## Standing orders` or `## Divergences` heading appended to an approved plan makes it stale. Known limit,
       accepted 2026-11-14: blanks after a trailing backslash change a shell command's meaning (an escaped space against
       a line continuation), and the normalized hash does not see that edit; keeping trailing blanks would make routine
       editor trimming stale an approval, which this rule exists to prevent.
    2. **Required writes become rows.** A slice's issue link, a lane's merged state, a coverage re-check result, and a
       policy version stamp that changes no other text are recorded as `state.tsv` rows. None of them edits hashed text.
       A policy change that does alter the spec's text is a real change and still revokes.
    3. **Formatters.** The skill that asks for an approval first formats the artifact with the repository's own
       formatter, when one covers the path, so a commit-time formatter finds nothing to change. No skill's automatic
       formatting fix writes to an artifact that holds a current approval; it reports the file instead.
    4. **Old approvals.** The hash rule is named by the evidence key. A `blob=` row is checked by the first rule, CR
       bytes and Git attributes included, exactly as it was written. New approvals carry a new key and the rule above.
       Nothing is rebound. A stale first-rule approval is approved again under the new one.

    (verified: hooks/go/internal/flow/hash.go:63, 2026-11-11)

- **Default / Rejected:** normalize what carries no meaning and move required post-approval facts into rows; rejected: a
  hash with all whitespace collapsed, because a change to an indentation-significant example would then pass unapproved;
  rejected: more unhashed sections for the workflow's own notes, because every unhashed section is text that acts
  without approval (see the next rule).
- **Later work changes:** `hooks/go/internal/flow/`, `skills/execute/references/flow-state.md`,
  `skills/decompose/references/slicing.md`, `skills/decompose/references/coverage-check.md`,
  `skills/spec/references/align.md`, `skills/execute/references/parallel-lanes.md`,
  `skills/execute/references/plan-divergence-and-trail.md`, `skills/triage/references/checks/checks-tier-policy.md`

### An approval counts where it was given, for the content shown, until it is withdrawn

- **Answers:** C2.3
- **Rule:** An `approve` row is evidence of consent only in the clone where the user gave it, only for the content the
  user was shown, and only until the user withdraws it.
    1. **Where it was given.** On the user's yes, the helper writes the `approve` row and records it in a clone-local
       ledger under the Git common directory, shared by worktrees and never committed. That location is a named
       exception to the rule in `skills/routing/references/session-storage.md:37` that bans Git metadata as a generic
       artifact store: the ledger is the helper's own file, holds no artifact, and is the only store this rule allows
       there; the storage rule below names the one other exception, the local store for a work folder. `flow status`
       reports an `ok` approval that is missing from the ledger as `unconfirmed here`. That row arrived by merge, pull
       or copy. It is not a settled route: the phase that needs it presents the artifact and asks once, and a yes writes
       a new row. An unattended run cannot ask, so it stops before the phase that needs the approval, writes a `pause`
       row, and reports the artifact as unconfirmed. It never confirms on the user's behalf.
    2. **The content shown.** Every `approve` row with `auto` evidence requires `--expect-blob`, first approvals
       included. `flow status` prints each artifact's current hash so the skill can pass the one it read when the user
       approved. An artifact the user approves before it was presented, which record 0014 allows, is shown first, by its
       path and that hash at least, and only then is the row written.
    3. **Unhashed sections.** `## Standing orders` and `## Divergences` stay outside the approval hash, so an appended
       entry never makes an approval stale. Each entry has force only when a row records it with the entry's own hash:
       an `order` row for a standing order, a `diverge` row for a divergence. An entry with no such row, or a standing
       order or user-authorized `rule` divergence whose row is missing from the ledger, reads `unrecorded`. It is not
       sent to a worker as an order and it suppresses no review finding.
    4. **Withdrawal.** A user's later no to an approved artifact writes `revoke <artifact>`, from the flow skill in
       whose turn the user says it. `flow status` already drops an approval on a `revoke` row; the row has had no writer
       for this case.

    (verified: hooks/go/internal/flow/log.go:58, 2026-11-11)

- **Default / Rejected:** a local ledger, a guard on every approval, a row per unhashed entry, and a written revocation;
  rejected: trusting the `session` cell, because a merged row carries a session id as easily as any other cell;
  rejected: hashing the two sections into the approval, because every recorded divergence or new standing order would
  then force a reapproval of the whole plan; rejected: leaving first approvals unguarded, because the file can change
  between the moment it is shown and the moment the row is logged.
- **Later work changes:** `hooks/go/internal/flow/`, `skills/execute/references/flow-state.md`,
  `skills/execute/references/standing-orders.md`, `skills/execute/references/plan-divergence-and-trail.md`,
  `skills/routing/references/chain-phases.md`, `skills/routing/references/forwarding.md`,
  `skills/routing/references/session-storage.md`, `skills/intent/SKILL.md`, `skills/spec/references/draft.md`,
  `skills/decompose/SKILL.md`, `skills/execute/SKILL.md`, `skills/review/SKILL.md`

### Every commit on a flow branch has a row, written by whoever made it

- **Answers:** C6.3
- **Rule:** Every non-merge commit between the base and `HEAD` of a flow branch is accounted for by a row that names it,
  and the set of kinds is closed. Task, red and artifact-only commits keep their present forms; a red leg whose commit a
  pre-commit hook rejected has no commit to account for, and its `verify red-<task-id>` or `verify red-fix` row takes
  the no-commit evidence form that record 0016 decides. Every other commit gets a `commit` row with evidence
  `auto:<sha>` and one of four kinds: `fix` (a fix for a gate, review or converge finding), `triage` (a fix triage made
  on the pull request), `user` (a commit the user made or directed), and `housekeeping` (gate's tidy, reconcile,
  docs-refresh and record-drift commits, a formatter commit). An ignore-rule commit is not housekeeping: record 0016
  lets no skill make one on its own, and one the user asked for is kind `user`.
    1. The skill that makes the commit writes the row in the same turn. Gate writes rows for its own commits, so a later
       gate run reads them from the trail. The exemption for "this gate run's own commits" is replaced by those rows.
    2. A commit made by the user, or by a skill that writes no flow rows, is recorded by gate's audit. The audit lists
       the unaccounted commits once and asks the user whether they are intended. On yes it writes their `commit user`
       rows. A recorded commit is not a finding on any later run.
    3. A `commit` row accounts for a commit; it does not clear it. A `fix` or `triage` commit is still reviewed, and its
       `review` row names the same commit.

    (verified: skills/gate/references/branch/branch-workflow.md:1196, 2026-11-11)

- **Default / Rejected:** one closed set of commit rows, each written at commit time; rejected: widening gate's
  exemption list, because an exemption that lives in one run's memory is lost by the next run; rejected: letting gate
  write `verify` rows for commits it did not dispatch, because a `verify` row claims a task verification that never
  happened.
- **Later work changes:** `hooks/go/internal/flow/`, `skills/execute/references/flow-state.md`,
  `skills/gate/references/branch/branch-workflow.md`, `skills/gate/references/emission.md`, `skills/triage/SKILL.md`,
  `skills/execute/references/fix-entry.md`

### Every state the chain can be in has a row

- **Answers:** C6.4
- **Rule:** Each state below has exactly one row, and a reader takes the state from the rows, never from chat or from
  marks in the plan.
    1. **Dropped task.** `drop <task>` with status `cancelled` and the reason. Execute's completion check and gate's
       per-task check count a dropped task as closed, and each dependent gets its own `drop` or `pivot` row.
    2. **Resume.** A task commit writes `verify <task>` with status `pending` when it is made, and the group verdict
       writes the final `ok` or `failed`. Before any dispatch, execute reconciles the base-to-`HEAD` commits and the
       working tree against the rows: a `pending` row resumes at its review, and a commit with no row or a dirty tree
       stops for the user.
    3. **Completion.** A task is complete when its latest `verify` row is `ok`. Check marks in `plan.md` carry no state.
       Flow completion is the close rule above.
    4. **Pause.** Every flow skill, gate included, ends a turn that leaves flow work unfinished with a `pause` row. A
       "not yet" to gate's proposal is one.
    5. **Declined approval.** A no to an approval question writes `approve <artifact>` with status `declined`. Only
       status `ok` approves, as today. `challenge <artifact>` with status `declined` means only that the challenge was
       declined.
    6. **Clear scan.** An ambiguity scan that finds nothing to challenge writes `challenge <artifact>` with status `ok`
       and evidence `scan=clear`.
    7. **Escape severity.** An `escape` row keeps `severity=<critical|important>`, assigned by the mapping in record
       0021 from whatever scale the finder used, and gains one field that keeps the finder's own word. The helper
       accepts only `critical` and `important` today (`hooks/go/internal/flow/cells.go:159`), which gate, CI and people
       do not emit, so the skill that records the finding maps it first.
    8. **Format.** New files start with the header row and carry no version line, because the helper reads each row by
       its own content and the nine columns are unchanged; the header row is what names them. A file that starts with
       `# flow-state 1` keeps that line and stays readable. No existing file is rewritten, and the new rows are appended
       to files with or without the line. (Amended 2026-11-14: the first draft started new files with `# flow-state 2`.
       A helper older than this change skips a file's first two lines unread, so it drops the first row of a file
       created without the line; the owner accepted that cost.)

    (verified: skills/execute/references/flow-state.md:374, 2026-11-11)

- **Default / Rejected:** a row per state, added to the existing nine-cell format; rejected: recording a declined
  approval as a declined challenge, because one row then has two meanings and the metrics count a refusal as a challenge
  outcome; rejected: plan check marks as the completion record, because they are edits inside approved text; rejected:
  rewriting existing files to the new version, because the trail is append-only and record 0012 keeps it so.
- **Later work changes:** `hooks/go/internal/flow/`, `skills/execute/references/flow-state.md`,
  `skills/execute/SKILL.md`, `skills/execute/references/two-strike-stop.md`, `skills/gate/references/emission.md`,
  `skills/intent/SKILL.md`, `skills/spec/SKILL.md`, `skills/decompose/SKILL.md`,
  `skills/decompose/references/chain-artifacts.md`

### Rows, verdicts and modes are scoped to a slice or lane

- **Answers:** C6.5
- **Rule:** A slice or lane branch is judged on its own slice, and nothing one lane records changes what another lane
  reads.
    1. **Scope.** The `bind` row of a slice or lane branch records `slice=S<n>`. Gate on that branch requires `verify`
       rows only for that slice's tasks and judges only the spec criteria those tasks trace to. Every other task and
       criterion reads `n/a: outside selected slice`.
    2. **Verdicts.** A `branch` verdict names its slice. It turns earlier commit evidence into history only for that
       slice, and only when its status is `ok`. Today any verdict row does, a failed one or another lane's included
       (`hooks/go/internal/flow/status.go:189`).
    3. **Closing.** A slice's merge writes no `done`. The flow closes when every slice has a merged `ok` verdict and,
       where the plan needs a final review, a `verify final` row with status `ok`. Under the `local` storage setting the
       same continuation check that closes a single-branch flow writes the `done` row, once it can read every slice's
       merge in that way.
    4. **The final review.** The final integration review of a laned plan belongs to the home session, the one that runs
       execute on the plan with no slice argument, on a branch that has merged the base after every lane merged. The
       audit's claim that the review has no owner no longer holds as written:
       `skills/execute/references/aggregate-wave-review.md:81` names "the lane that merges last". That owner cannot act,
       because a lane stops at its draft pull request and cannot know it is last, so the text is replaced.
    5. **Mode.** Gate performs the controller audit in every mode, including `pair`. An unrecorded commit is an
       Important finding in the `audit` lane. Accepting it writes an attested `commit user` row.
    6. **Merged lanes.** Two lanes that each record divergences merge without changing hashed text, under the approval
       rule above, so the merged `plan.md` keeps its approval.

    (verified: skills/gate/references/branch/branch-workflow.md:733, 2026-11-11)

- **Default / Rejected:** slice scope carried in the rows; rejected: one flow per slice, because the slices share one
  approved plan and one spec, and separate flows would need the same approval several times; rejected: gating only the
  whole plan, because lanes exist so that each slice can merge alone.
- **Later work changes:** `hooks/go/internal/flow/`, `skills/execute/references/flow-state.md`,
  `skills/gate/references/branch/branch-workflow.md`, `skills/gate/references/emission.md`,
  `skills/execute/references/parallel-lanes.md`, `skills/execute/references/aggregate-wave-review.md`

### A storage setting keeps planning files and `state.tsv` committed by default, or local to the clone

- **Answers:** C6.7
- **Rule:** One storage setting with two values decides where a work folder lives. `committed` is the default and is
  today's behavior: planning files and `state.tsv` are committed under the work root, and a small fix commits its rows
  to `unassigned/state.tsv`. `local` keeps the work folder, with its planning files and `state.tsv`, in a clone-local
  store under the Git common directory: shared by worktrees, never staged, read and written by the same `flow` commands
  through the work root. This is a second named exception to the ban on Git metadata as a generic artifact store in
  `skills/routing/references/session-storage.md`, which is amended to allow exactly this store and the approval ledger.
  The setting resolves in the order record 0020 gives every convention, extended by one step in front for the user's own
  value: the user's stated or configured value comes first and holds for their clone, because a contributor cannot edit
  the instruction file; then the pointer in the repository's instruction file; then discovery; then the default
  `committed`. Discovery looks for a work root that is ignored or banned for committed planning files, and for a base
  remote the user has no push permission on, which marks a repository the user does not own; under `local` no Agentics
  file, commit or pull-request line reaches that project. A discovered case leads to one proposal of `local` when the
  run is attended, and an unattended run stops unless a value is configured. Under `local`, the flow creates and binds
  its feature branch at the point where `committed` mode would make its first artifact commit, which is the first
  approval; before that, the default-branch match is the one open local-store flow with no `bind` row. The case is
  detected and stated, never discovered by a failing command. Before the first save, the saving skill checks whether the
  work root is ignored or banned for committed planning files, and says so with the reason instead of failing at
  `git add` and writing no row. Before a small fix starts in a repository the user does not own, the skill says what
  bookkeeping the pull request will carry, or that it will carry none. No skill changes where the trail is stored
  without telling the user, and every gate report states the mode. A pull-request body carries no `Signals:` lines in
  either mode, as record 0019 decides: a small fix's signals are recorded in its trail rows, so what a pull request
  carries is the `state.tsv` and its commits in `committed` mode and nothing in `local` mode. Today the text says there
  is no opt-out switch and that a repository that does not want the file does not install the skills. (verified:
  skills/decompose/references/chain-artifacts.md:77, skills/routing/references/session-storage.md:37, 2026-11-11)
- **Default / Rejected:** one storage setting, `committed` by default and `local` where the repository or the
  contributor needs it; rejected: no opt-out, with planning files and `state.tsv` always committed under the work root,
  because the chain then cannot be used at all in a repository that ignores or bans those paths, and a contributor's
  pull request to someone else's project still carries a `state.tsv` and its commits that the owners never asked for,
  which is the finding itself. What that choice offered in exchange, that every reviewer can rely on the trail being in
  the diff, is kept as the default wherever the owner wants it, and the mode is always stated, so no reviewer is told a
  trail exists when it does not.
- **Later work changes:** `skills/decompose/references/save-and-stage.md`,
  `skills/decompose/references/chain-artifacts.md`, `skills/execute/references/fix-entry.md`,
  `skills/execute/references/flow-state.md`, `skills/routing/references/session-storage.md`, `hooks/go/internal/flow/`,
  `skills/routing/references/repo-conventions.md`

### A document review budget counts runs against one revision, and a changed scope starts a new one

- **Answers:** C7.1
- **Rule:** The plan review and the spec review each allow two independent runs per revision of the artifact, and the
  count is read from the `review` rows, never from the session or a header line.
    1. **A revision.** Run 1, then run 2 only when the fixes for run 1's findings changed reviewed scope. A wording fix
       causes no run, as today.
    2. **Reset.** A new revision, with a fresh budget, starts when reviewed scope changes for any reason other than
       fixing that revision's findings: a change the user asks for, a `spec align` write that changes behavior, a
       proportionality resize, a coverage fix, or tasks added after gate's converge pass. Scope that no review covered
       is never approved or stamped as reviewed.
    3. **Order.** Decompose runs its proportionality and coverage checks before the plan review, so its own mandatory
       steps do not change the plan after the last run.
    4. **A spent budget.** When the last run of a revision still reports a blocking finding, the skill presents the
       findings and asks the user to revise or stop. The approval question is not asked, and the finding stays
       non-waivable. A revision the user orders starts the next budget.
    5. **Self-read.** A self-read on a host that cannot dispatch is recorded as a self-read and spends no independent
       run.
    6. **Resume.** A resumed session reads the rows and may dispatch a reviewer when the revision has a run left.

    (verified: skills/decompose/SKILL.md:533, 2026-11-11)

- **Default / Rejected:** a budget per revision, stored in rows; rejected: a budget per file for its lifetime, because
  required changes arrive after it is spent and are then approved unreviewed or never approved; rejected: a budget per
  session, because a new session would reset it with no change to the content; rejected: no cap, because a reviewer that
  always finds something never lets the plan reach its approval question. How many reviews a delivery path requires is
  decided in record 0013; this rule decides only what a run is counted against.
- **Later work changes:** `skills/decompose/SKILL.md`, `skills/spec/references/draft.md`,
  `skills/spec/references/align.md`, `skills/execute/references/flow-state.md`, `hooks/go/internal/flow/`

### Execute keeps one counter per task, and every budget ends at a question

- **Answers:** C7.2
- **Rule:** Each execute budget has one counter, one scope and a stated reset. The count is derived from the task's
  rows, so it survives a session, and a spent budget always ends at a user question.
    1. **Stage 1 re-review.** A packet that has not been dispatched is reviewed again each time a divergence other than
       a `rule` entry touches it, up to two re-reviews. A third touch writes `block <task>` and asks the user. No packet
       waits for a review its budget forbids.
    2. **One failure ladder.** A failed correction, a failed continuation of an isolated packet, a REVISE return, a
       second NEEDS_CONTEXT return on the same packet, and a red leg that fails for the wrong reason each count once
       against the task being worked, never against another task whose files the fix touched. Two failures lead to one
       fresh dispatch. Two more stop the task. The user hears after at most four failures and two workers, and the
       isolated-packet repair stop is this same stop.
    3. **Reset.** Only a replaced packet resets the counter. The user's "continue" grants one more fresh dispatch with
       two attempts, not an unbounded run.
    4. **Diagnose.** Diagnose's three-attempt limit counts fix attempts per defect across implementers, so a fresh
       implementer does not reset it. When it is reached inside execute, the task stops with the same question.
    5. **Arena.** An arena run has one re-run in total, whichever phase asks for it. A second need stops and asks,
       although arena has no other user stop.
    6. **Unattended modes.** In `auto`, a stop blocks that task only, and independent tasks continue.

    (verified: skills/execute/references/stage-1-dispatch-rule.md:48, 2026-11-11)

- **Default / Rejected:** one ladder per task, counted from rows; rejected: keeping the two-strike ladder and the
  isolated-packet ladder apart, because mixed counts are undefined and the worst case is eight failures and three
  workers before the user hears; rejected: a counter that restarts with each fresh implementer for diagnose's limit,
  because it lets execute attempt the fix that diagnose forbids; rejected: letting "continue" lift the bound, because
  one word would then remove every later stop for the task.
- **Later work changes:** `skills/execute/SKILL.md`, `skills/execute/references/stage-1-dispatch-rule.md`,
  `skills/execute/references/two-strike-stop.md`, `skills/execute/references/isolated-packets.md`,
  `skills/execute/references/arena-mode.md`, `skills/diagnose/SKILL.md`, `skills/execute/references/flow-state.md`

### Gate's own commits spend no re-run budget, and converge re-runs like the review

- **Answers:** C7.3
- **Rule:** A gate re-run budget is charged only by a change that gate did not author.
    1. **Gate's commits.** Those of gate's tidy, reconcile, docs-refresh and record-drift changes that only record
       process make at most one bookkeeping commit per gate run, under record 0016. Gate makes its commits before the
       first run of the checks they would invalidate, wherever the step order allows. One made later is checked by a
       pass over that commit alone. It triggers no full re-run of the suite, the checklist, the review or converge, and
       it is not counted.
    2. **The budget.** Each check re-runs automatically once per change set that gate did not author, such as a user fix
       or a dispatched fix. The count starts again on each gate invocation and each time the user answers a gate
       question with a fix or asks for another review.
    3. **Converge.** Converge re-runs under the same rule as the review: once automatically after a fix that touches a
       gap, and whenever the user says "review it again". While a gap is withheld, gate may offer a converge re-run.

    (verified: skills/gate/references/branch/branch-workflow.md:1476, 2026-11-11)

- **Default / Rejected:** charge budgets to outside changes only; rejected: today's single re-run counted from any
  commit, because gate's own record-drift commit spends it before the user is asked anything and a later fix can then
  never be seen by converge; rejected: unlimited automatic re-runs, because each is a full suite and review run. How
  many suite runs a delivery path gets is decided in record 0013; this rule decides only what spends one.
- **Later work changes:** `skills/gate/references/branch/branch-workflow.md`, `skills/gate/SKILL.md`,
  `skills/gate/references/emission.md`

## Existing records

- 0008: consistent — an approval is still a row written on the user's yes and every spec still gets its independent
  review; this record adds what the row binds and what the review budget counts.
- 0009: amended — a small fix's rows go to the clone-local store under the `local` storage setting, which a repository
  the user does not own runs; under `committed`, the default, they stay in the committed `unassigned/state.tsv`.
- 0012: amended — the `flow` verb on the hook binary, the exit codes, the append-only file and the rule that no approval
  is rebound all stay, while "the flow contract stays" gives way to the lookup, hash, row kinds and scoping decided
  here.

## What this costs

**Every rule here that touches the helper is a rebuild of six binaries.** Lookup, the hash, the guard, the ledger, the
new events and the slice scope all live in `hooks/go/internal/flow/`, and record 0012 already requires all six assets to
be rebuilt for any production Go change.

**Two hash rules are checked for as long as old approvals exist.** A first-rule approval keeps its bytes-exact check, so
a plan approved before this change still goes stale on its first standing order and is approved again once.

**A fresh clone asks again.** The ledger is local, so an approval made on another machine reads `unconfirmed here` and
costs one question per artifact. That is the price of not treating a merged row as the user's consent.

**More rows, and more questions in unattended runs.** Each commit kind, drop, pending verification and declined approval
is a row, and budgets that used to end silently now end at a question. An `auto` run stops the affected task and reports
it, where today it either loops or continues on unreviewed work.

**Closure by merge needs a read of the default branch.** Deciding whether a flow is closed reads its `state.tsv` at the
default branch's tip. On a stale local default branch a merged flow stays open until the next fetch.

**In `local` mode the trail stays in one clone.** The reviewer of the pull request sees no trail, the trail is lost with
the clone and does not travel to another machine or lane host, and gate's audit evidence exists only locally. A `local`
flow whose pull request merges stays open until `routing`'s next continuation check in that clone writes its `done` row,
because closure by merge has no `state.tsv` on the default branch to read. Both modes must be tested.

**The lookup fallback is gone.** A branch that relied on being the only unbound flow now gets exit 3 and a question
until it is bound.

## Considered and declined

**Keying a flow on an issue or pull request number.** Declined. Many flows have neither when they open, the chain must
work with no host at all, and the work folder already exists from the first row.

**Recording provenance in the row alone.** Declined. Every cell of a committed row can be written by whoever authored
the commit. Only something that never leaves the clone can say "this was approved here", which is why the ledger is
local and why the storage rule that bans Git metadata as a generic store needs a named exception for it.

**Format-before-hash inside the helper.** Declined. The helper would have to run the repository's formatter, which is a
tool call with its own configuration, version and failure modes. The skill that presents the artifact formats it, and
the helper normalizes only what needs no tool.

**A `local` storage mode for small fixes only.** Declined. It would settle the pull-request half of the storage question
and leave a repository that bans committed planning files with no path, so the one setting covers both halves.
