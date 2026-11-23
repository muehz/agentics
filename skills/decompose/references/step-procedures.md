# Decompose Step Procedures

Detailed rules and edge cases referenced from [SKILL.md](../SKILL.md). Read the relevant section when executing that
step.

## Decomposition pattern details

### Vertical slice example

```text
Slice S1: Create a user through the real API, with persistence and verification.
Slice S2: Let that user sign in, with authentication and verification.
Each slice contains bounded tasks with explicit prerequisites.
```

Do not demand that every task ship alone. Necessary shared work belongs on the first outcome's path; do not front-load
all models or infrastructure for every future slice. Sibling groups do not imply independence. Dependency edges answer
one question: "If this task fails, which other tasks are blocked?"

### Parallel lanes lifecycle

Mark qualifying slices `**Lane:** yes` after the **Dependencies** line of each qualifying slice, by the lane rule under
**Lanes** in [chain-artifacts.md](chain-artifacts.md#slices-inside-planmd). Mark lanes against the merge state when the
plan is written. After the plan is approved, a lane's merge is recorded as a `note lane-S<n>` row, never as an edit to
the plan, because an edit makes the approval stale. Re-mark lanes only on a plan update that already needs a new
approval. A lane adds no approval question and no file; `execute` writes the launch packets.

### Bespoke workflow design

For a large or cross-cutting effort that no existing skill workflow sequences (a migration across many call sites, an
ambitious multi-part change, or work the user will step away from and trust later, such as a hand-off from `routing`'s
no-workflow-fits rule), the plan's first deliverable is the **designed workflow itself**:

- Resolve Step 0 input gaps before writing the phase list, so the list the user reviews sits on a defined solution.
- Write the phase list explicitly: what each phase does, what it verifies, and what fans out to whom. Present it before
  decomposing phases into tasks. Treat pushback on it as Step 1 input.
- Sequence the **riskiest unknown first**, so option value stays high while change is still cheap.
- Put the verification harness before the work it verifies. Capture the baseline from the pre-change state so every
  later check reads as "old value vs new value".
- Match rigor to the stakes. One-way doors and high blast radius get more phases and more verification. Rigor means
  checks and artifacts, not "try harder".
- Name the audit trail: state in the plan which decisions a reviewer should expect to find in the work folder's
  `state.tsv` ([flow-state.md](../../execute/references/flow-state.md)).

### Common mistakes

- **A skipped plan.** 3+ tasks with dependencies executed without a plan lead to rework. Write the plan first.
- **Over-planning.** A plan covers one feature, one refactor, or one milestone: the next shippable increment, not six
  months of work. Step 5's proportionality check measures this with counts.
- **The support system first.** Reliability, evidence capture, and isolation built ahead of a working end-to-end path
  are built on a guess. Reorder them behind the skeleton path per Step 5.
- **Code in the plan.** The plan describes behavior and verification, and `execute` writes the code.

---

## Step 0: Preflight edge cases

- **Forwarding and insistence.** Forward once by [forwarding.md](../../routing/references/forwarding.md) and name the
  open question the target needs, for example: "This is still an idea with no stated requirements, so I'm handing it to
  `intent` to shape the problem." When the user insists on a plan anyway, ask once for the requirements the plan would
  rest on. Requirements the user states are concrete inline requirements, and row 12 accepts them. When the user answers
  that ask with assumptions instead ("assume reasonable values"), write the plan: record each assumed value in the plan
  context as a user-approved assumption, not as a requirement, and ask nothing further.
- **Input readiness.** A source artifact is an intent, spec, or design in the repository. An approved spec carries real
  solution behavior, acceptance criteria, and scope; approval does not fill placeholders. Inline requirements and a
  legacy design need enough definition to write testable tasks. When a source leaves solution behavior, an interface, or
  a meaningful design choice undefined, run `spec draft` with the existing decisions and the precise gap, as row 11
  does. Do not infer missing requirements. Do not count explicitly deferred follow-ups as requested increments. No
  internal handoff authorizes implementation or external publication.
- **The routine hand-off.** `spec` hands over a reviewed spec without asking its approval when its tier is routine.
  Decompose accepts it when its `**Spec review:**` line is recorded under **Scope Decisions**, its Policy Conflicts has
  no `Resolution: open` entry, its **Further Notes** record no unresolved solution blocker, and its `challenge spec.md`
  row is not `pending`. Step 6 then asks the spec's approval with the plan's.
- **Re-entry.** Two requests need no new plan. A request to approve or execute an existing plan that has no current
  approval runs Step 6's approval question on that plan as it stands, after the offer gate. "Re-check coverage" runs the
  [standalone re-run](coverage-check.md#standalone-re-run).

---

## Step 1: Source handling and slices

- **Legacy design.** Read a legacy `design.md` in place wherever it lives, record its path in the plan's context, and
  write `derived-from: none`.
- **A named input that does not exist.** When the design file is missing, the slice issue resolves to nothing, or the
  "approved spec" was never written down, say so in one line and ask where it lives. Never reconstruct it from the
  request line or from memory: every task would inherit the invention.
- **Read the source's decisions.** Preserve resolved direction and solution decisions, and do not re-interview them.
  Record source review evidence as context only, and never copy its `challenge` result into the plan. When an in-flight
  five-section intent has a pending direction challenge, carry its open questions into the Step 4b round with the
  intent's settled answers beside them. The plan's challenge record holds the answers, and `intent.md` is not rewritten.
  An answer that changes a settled requirement goes to `intent`.
- **The empirical fork.** Record every observed answer in the plan as a lookup and identify any task held by missing
  evidence.
- **A source outside the work folder.** A spec or intent outside a work folder has no approval that the flow can read,
  so it goes to its owner (`spec` or `intent`) to be placed and approved.
- **Delivery slicing.** The breakdown lives inside `plan.md` as `## S<n>` sections. A single useful outcome needs no
  artificial breakdown, and a slicing-only request gets only that explanation. When the user asked only for slicing,
  follow [slicing.md](slicing.md)'s slicing-only steps, then save the plan as Step 6 does for that case.
- **Existing or selected slices.** Read the plan's slice sections and the source they cite. Fold any legacy `slices.md`
  beside the plan into those sections per [chain-artifacts.md](chain-artifacts.md) and remove it. Preserve stable IDs
  and external issue links. Task completion lives in `verify` rows, never in the plan. An explicit request for one slice
  limits the update to that group: preserve sibling groups, the `diverge` rows, and the Divergences history, and record
  the selected IDs in Scope. Treat unplanned prerequisites as blockers. Never create a per-slice plan. Surface a slice
  issue whose text conflicts with the plan; do not replace canonical scope silently. A legacy plan (no `## Task index`)
  keeps its layout on update (no card, Task index, fold, or Provenance split) unless the user asks to convert it.

---

## Step 4a: Task fields and folder hygiene

For each task, write the fields [plan-template.md](plan-template.md) names: a kebab-case ID, explicit dependencies by
task ID, a parallel group, exact repository-relative file paths with no line ranges, the source items it covers,
specific acceptance criteria, and bite-sized steps naming behaviors, test cases, and commands with expected output, with
pseudocode where logic is non-obvious.

- **`**Tier:**`** is `Deep`, `Standard`, or `Light` per **Model Selection for Subagents**.
- **`**Packet review:**`** is `yes — <one-line reason>` when the task's contract is unresolved, or when it touches a
  security boundary, a trust boundary, a state-binding seam, or a cross-platform seam, and `no` otherwise. `execute`
  skips its Stage 1 review only for a task marked `no` on a plan whose plan review was independent.
- **`**Review group:**`** is optional. A review group is the set of tasks one quality reviewer judges together. By
  default each delivery slice is one group named by its slice id (`S1`), and in an unsliced plan each wave is one group
  named `wave-<N>`. Tasks that share a name form one group. The graph of groups (with an edge when a task in one group
  depends on a task in another) must be acyclic. Unrelated to **Parallel group**.
- **`**Role:**`** is `outcome` or `support`, by the removal test in
  [proportionality-check.md](proportionality-check.md#task-roles). A task's own tests carry its role, so this field
  never asks for fewer tests.
- **`**Reproduction:**`** names the failing command or broken behavior to restore on a task that fixes a bug. `execute`
  splits such a task into a red leg and a fix leg.

**Formatting and hygiene.** Write every table padded, because Prettier rewrites an unpadded table and the rewrite stales
an approval. Write no file beside `intent.md`, `spec.md`, and `plan.md` except `state.tsv`. Put scratch and evidence in
the ignored `temp/`.

---

## Step 4b: Challenge resolutions, blockers, and unattended

- **Reuse.** A prior review of the same unchanged plan can be reused only with its scope and resolutions available.
- **Resolutions.** Rebuild gaps the source already answers or the user answers now in place. Never silently invent an
  answer.
- **An unanswered gap is a pending blocker.** An "undecided" answer counts as unanswered. Save the draft with a
  `pending` `challenge plan.md` row (Step 6), and offer no execution. Name each open question as a blocker in the report
  and list it under remaining questions in `**Challenge record:**`. Independent, fully defined planning may continue
  while the blocker waits. Once every gap is answered, rebuild affected tasks, then run Step 5.
- **Unattended runs.** When the run is unattended ([unattended.md](../../routing/references/contracts/unattended.md)),
  record `pending`, save the draft, and offer no execution. Still run Step 5 and record the real results; only the
  challenge result stays `pending`.
- **Challenge record.** Record an `ok` `challenge plan.md` row only after the challenge completes and findings are
  resolved or explicitly accepted. Record the five fields of the record rule in `## Plan challenge` under
  `**Challenge record:**` ([chain-artifacts.md](chain-artifacts.md)): target and scope, execution reference (session
  date plus rounds and question numbers asked), resolutions, remaining questions, and concerns set aside. A decline
  before any round reads `declined <YYYY-MM-DD>, no round asked` and names what went unexamined. Question
  recommendations alone are not resolutions.

---

## Step 5: Plan review and budget

- **Dispatch inputs and timing.** Pass the plan path, source paths, and any recorded `waive no-code` scope to the
  plan-review subagent. On an update of a pre-4.22 plan, list the task IDs the update adds or changes. Read current UTC
  time before dispatch (`date -u +%Y-%m-%dT%H:%M:%SZ`, or in PowerShell
  `(Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')`) and again on return. Step 6 writes one `review plan`
  row per return from these values.
- **Self-read fallback.** When the host cannot dispatch a subagent, read the finished plan against the reviewer prompt's
  criteria yourself in one pass, label the result `self-read (not independent)` in `**Plan review:**` and in the
  approval question, and set every task's `**Packet review:**` to `yes — plan review was a self-read`, so `execute`
  reviews every task at Stage 1. A self-read spends no independent run.
- **Review budget.** Each revision allows two independent runs. Count them from `review plan` rows whose evidence names
  the current `rev=` and `by=independent` (never from a header line or session memory). A `by=self` row is not counted.
    - Run 2 happens only when fixes for run 1's findings changed reviewed scope (a task, file, edge, or acceptance
      criterion). A wording fix causes no run.
    - A new revision with a fresh budget starts when reviewed scope changes for any reason other than fixing that
      revision's findings: a user-requested change, an edit request on the approval question, a coverage fix after
      review, or tasks added after gate's converge pass. Scope that no review covered is never approved or stamped as
      reviewed.
    - When the last run of a revision still reports a Critical or Important finding, present the findings and ask
      whether to revise the plan or stop. Ask no approval question; a finding closes only by a fix, and a user-ordered
      revision starts the next budget.
    - A resumed session reads the rows and may dispatch a reviewer while the revision has a run left.
- **Minor and Informational findings.** Present remaining Minor and Informational findings with the execution question,
  each recorded in Provenance as `**Plan review finding (presented):** <finding> (<location>)`. They never block.

---

## Step 6: Storage, tier, and hand-off

- **Failed-plan override.** A `failed` plan is not offered. After failing rows are reported, a user who tells you to
  proceed anyway in their own words approves the plan as it stands: show its path and hash, write the `approve plan.md`
  row with the failing rows in its `why`, and route to `execute`. A direction given before the rows were reported ("then
  hand it to execute") is not that approval.
- **Repository storage and folder identity.** Save the plan as `<plan-parent>/YYYY-MM/<purpose>-<topic>/plan.md` per
  [chain-artifacts.md](chain-artifacts.md). `<purpose>` is one of `feature`, `refactor`, `upgrade`, `infrastructure`,
  `architecture`, `data`, `process`, or `design`. Reuse an existing work folder when a sibling `intent.md` or `spec.md`
  already lives there. Otherwise derive the folder name as the **Plan artifact identity** rule in
  `skills/execute/references/plan-artifact-identity.md` would: the slug comes from the current non-default branch name
  when there is one, else from the content seed (the source card's `**Change:**` line, else the plan's own `**Goal:**`
  line; never a source filename), max 40 characters. Set `derived-from:` to `[spec.md]` for work started at intent,
  `[intent.md]` only for an in-flight five-section intent, and `none` for inline requirements or a legacy design.
- **Stage and log values.** Follow [save-and-stage.md](save-and-stage.md) exactly (commits nothing until approval):
    - `PLAN_EXISTED` is the value Step 4a recorded before the first write.
    - Each `review plan` row's evidence is
      `kind=plan dispatched=<UTC ts> round=<k> blocked=<seconds> rev=<n> by=<independent|self>`. `round` is the run's
      number within revision `rev`; `blocked` is the whole latency. Status is `ok` for `READY` and `failed` for
      `REVISE`.
    - A slicing-only run sets `CREATE_NEXT` to `decompose write the tasks` and writes no `challenge` or `review` row.
    - When the run ends without approval, end with a `pause plan.md` row whose `next` names what remains.
- **The tier.** Check the tier once against the final plan, starting from the spec's tier. Raise it to elevated when the
  plan has a `Deep` task, three or more file groups (distinct immediate parent directories of repository-relative paths
  in tasks' `**Files:**`, `.` for root files), an ownership boundary to another team, or any file in a task's Modify
  list with no test or verification command exercising it. The user can raise the tier and never lower it. A plan with
  no spec has no tier (`n/a: no spec; <basis>`). Show the plain meaning from
  [vocabulary.md](../../routing/references/contracts/vocabulary.md) beside a tier.
- **Commit and local storage.** A plan without approval stays staged and uncommitted. Under the `local` storage setting
  nothing is staged or committed.
- **Readiness claims.** Never call a plan "validated" unless every checklist row ran this session and passed. "Ready to
  execute" also requires the offer gate. "Reviewed" also requires an `ok` `challenge plan.md` row (a validated plan with
  a declined challenge is "validated; plan challenge declined"). If asked to call an unvalidated plan validated, say it
  has not been validated and name the outstanding checks.
- **Route.** On approval, route to `execute` (which verifies parallel groups, presents a dispatch preview, and starts in
  `auto` unless the user selected `pair` or `arena`).

---

## Checklist edge cases

- **Row 4 (Task index).** The Task index clause applies only when the plan has a `## Task index` section.
- **Row 8 (No code).** On a pre-4.22 plan, the no-code clause covers only tasks the update adds or changes; the
  specific-steps clause covers every task. Under a `waive no-code` row, row 8 reads
  `pass (no-code waived by user: <task ids>)`.
- **Row 9 (Commands).** Never fails solely because this session cannot reach a service: reachability is `execute`'s
  verification preflight, and a task naming its runner (user or CI) is valid.
