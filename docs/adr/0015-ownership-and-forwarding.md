# Every job has one owner, and a skill reaches another skill one way

This record decides who owns each job in the skill suite, how a skill passes work it does not own to the skill that
does, and what it means when the user names a skill themselves. The audit in
`docs/designs/skill-audit-2026-11/consolidated-issues.md` (theme T4, plus C1.6 from T1) found jobs with no owner,
hand-offs to targets that do not exist or that refuse the hand-off, seven pairs of skills that send the same input back
and forth, four different route-away formats, and skills that refuse the request their own description advertises. Each
rule below is decided once here, before any skill text changes.

Three neighbouring questions are decided elsewhere and are only referred to here: how heavy the delivery path for a
mutating change is (0013), who pushes, force-pushes and merges (0016), and how a fix made after a gate verdict is
recorded (0017).

## Rules

### An explicit invocation runs the skill, and one protocol carries everything else

- **Answers:** C4.3
- **Rule:** `skills/routing/references/forwarding.md` is the only route-away protocol. A skill that does not own its
  input carries the input to the owner by that file's target classification: it announces and runs a local-only target,
  asks one yes/no question before a target that writes, pushes or posts, and states the boundary when no skill owns the
  job. It forwards at most once, never tells the user to retype the request, and never asks a starter question on the
  target's behalf; the target asks its own questions. The three-part **Mismatch / One question to start / Handoff**
  block (in `intent`, `challenge`, `discern` and `execute`) and the stop-and-name form (in `principles`, `handoff`,
  `ops`, `deepen`, `git`, `docs` and `forge`) are removed. An explicit invocation is the user naming the skill for this
  request, as a slash command or in plain words. Under an explicit invocation, each `## Don't use when` row is one of
  two kinds. A **cannot** row names a deliverable no mode of the skill produces (`/git` asked to open a pull request);
  it forwards by the protocol. A **better elsewhere** row names work the skill can do but another skill does better (a
  one-line edit under `/forge`, a diff review while a bot comment is open under `/review`); the skill says so in one
  line, names the other skill, and runs. When the model loads a skill on its own, from a description match or a forward,
  every row still applies. Each row is marked with its kind. A stop that protects safety rather than ownership (an
  active incident, for example) is not a route-away and follows the gate tiers in 0014. The audit's statement that only
  `gh`, `review` and `recall` cite the protocol is no longer exact: `deepen`, `tdd` and `triage` now cite it too, and
  the other skills still do not. (verified: skills/routing/references/forwarding.md:3, 2026-11-11;
  skills/intent/SKILL.md:115, 2026-11-11; skills/challenge/SKILL.md:110, 2026-11-11; skills/handoff/SKILL.md:65,
  2026-11-11)
- **Default / Rejected:** the user's explicit choice of skill wins over a better-elsewhere row; rejected: keeping every
  row binding under an explicit invocation, because it makes the user who typed the skill's name argue with it, and
  rejected: dropping the rows entirely for explicit invocations, because `/git` cannot open a pull request however it is
  asked.
- **Later work changes:** `skills/routing/references/forwarding.md`, `skills/intent/SKILL.md`,
  `skills/challenge/SKILL.md`, `skills/challenge/references/discern.md`, `skills/execute/SKILL.md`,
  `skills/principles/SKILL.md`, `skills/ops/SKILL.md`, `skills/deepen/SKILL.md`, `skills/git/SKILL.md`,
  `skills/docs/SKILL.md`, `skills/forge/SKILL.md`, `skills/review/SKILL.md`, `skills/handoff/SKILL.md`,
  `skills/gh/SKILL.md`, `skills/triage/SKILL.md`

### A skill runs every request its description advertises

- **Answers:** C1.6
- **Rule:** A description is a promise: every request it names is one the body runs. Where the two disagree, the body
  changes to run the request, unless the request belongs to another owner under the ownership rule below, in which case
  the description drops it. The four named cases are decided as follows. `tdd` runs a request to implement a change
  test-first or to add tests; the delivery path that 0013 assigns to the change attaches to the work, and `tdd` does not
  refuse for lack of an approved plan. `/gate` with a readiness question runs `gate check` against the current head and
  answers from fresh evidence; it runs no disposition, pushes nothing and writes no verdict row, and the read-only
  refresh of recorded evidence stays as the behaviour when another skill forwards the question. `forge` makes a wording,
  typo, link or one-line correction to an existing skill, agent or instruction file itself, under its own quality rules
  and without the full authoring workflow. `scaffold` treats a request worded as creation ("set up the standard files",
  "create", "add", "init") as init; init still starts from the scan and previews before it writes, and bare `scaffold`
  stays audit. (verified: skills/tdd/SKILL.md:26, 2026-11-11; skills/gate/SKILL.md:108, 2026-11-11;
  skills/forge/SKILL.md:103, 2026-11-11; skills/scaffold/SKILL.md:156, 2026-11-11)
- **Default / Rejected:** change the body to keep the promise; rejected: narrowing each description to what the body
  accepts today, because the advertised requests are the ones users bring and narrowing leaves them with no owner.
- **Later work changes:** `skills/tdd/SKILL.md`, `skills/gate/SKILL.md`, `skills/forge/SKILL.md`,
  `skills/scaffold/SKILL.md`, `skills/routing/SKILL.md`, `evals/skills/tdd.trigger.json`

### Each job has one owner, and a hand-off names only a target that accepts it

- **Answers:** C4.2
- **Rule:** Every job a skill names has exactly one owner, and a hand-off or an offer names only a target whose entry
  rule accepts the input as it stands. A row whose target is "No skill" is a boundary statement and never a stop: the
  skill says it has no workflow for the job, and the session may do the work directly. The owners of the jobs the audit
  found unowned are:
    - **UX deliverables** (flows, wireframes, information architecture, mockups): no Agentics owner. The rows in
      `intent`, `spec` and `prototype` say Agentics has no workflow for this and stop naming "an outside UI/UX skill" as
      a hand-off target. Frontend implementation is an ordinary mutating change.
    - **REST and OpenAPI reference documentation**: `docs`. It writes and maintains endpoint reference from the route
      files, handlers and specification; running a generator stays with the project's tooling.
    - **Deleting dead code, unused files or feature flags**: an ordinary mutating change, started through `routing` on
      the delivery path 0013 assigns. `deepen` owns only the proposal of a structural refactor.
    - **Hook authoring** (hook configuration and hook scripts): `forge`, as an asset type beside agents, skills,
      instruction files and MCP servers.
    - **`AGENTS.md` and other instruction-file authoring and edits**: `forge`, in its instructions mode. `scaffold`
      creates only the starter file when none exists. A lesson appended to an instruction file by `gate` or by
      `forge reflect` follows `forge`'s placement and format rules; how many approvals that write needs is decided
      in 0018.
    - **Principle files**: `forge`, which no longer sends an edit to an Agentics asset out as a documentation edit.
    - **Purging a leaked secret**: `ops` owns the response and its order (rotate the credential first). `git` owns the
      local history rewrite. The push of rewritten history follows 0016.
    - **Fixing a finding that `gate` raised**: `execute`, as a fix task under the same plan, or `execute fix` for a
      small fix. `gate` never fixes. The path's weight is 0013's and the record of the fix is 0017's.
    - **`LICENSE`, `SECURITY.md`, `CODE_OF_CONDUCT.md`, `FUNDING.yml`**: `scaffold init`, once the user states the
      choice (which licence, which contact). The user speaks for the organization unless they say otherwise. `scaffold`
      asks once and never picks a licence itself.
    - **Planning offered from `explain` or `docs`**: the offer names `routing`, which selects the phase the work can
      enter. It does not name `decompose`, which refuses input with no intent or spec.
    - **Approval or header repair on an existing spec**: `spec draft`, resumed at its approval step.
    - **A yes to the threat-model offer in `spec draft`**: runs `threat-model` and then returns to the spec. The
      forwarding table classifies `/threat-model` as an offer instead of "no forward". (verified:
      skills/spec/SKILL.md:24, 2026-11-11; skills/docs/SKILL.md:26, 2026-11-11; skills/zealot/SKILL.md:23, 2026-11-11;
      skills/scaffold/SKILL.md:36, 2026-11-11; skills/forge/SKILL.md:7, 2026-11-11; skills/principles/SKILL.md:24,
      2026-11-11; skills/explain/SKILL.md:217, 2026-11-11; skills/decompose/SKILL.md:35, 2026-11-11;
      skills/routing/references/forwarding.md:67, 2026-11-11)
- **Default / Rejected:** give each orphan job to the existing skill closest to it, and state a boundary where none is
  close; rejected: a new skill per orphan job, because `docs/CHARTER.md` sets the same bar for adding as for keeping and
  none of these jobs has shown the use that would meet it.
- **Later work changes:** `skills/intent/SKILL.md`, `skills/spec/SKILL.md`, `skills/spec/references/draft.md`,
  `skills/prototype/SKILL.md`, `skills/docs/SKILL.md`, `skills/zealot/SKILL.md`, `skills/forge/SKILL.md`,
  `skills/scaffold/SKILL.md`, `skills/principles/SKILL.md`, `skills/ops/SKILL.md`, `skills/git/SKILL.md`,
  `skills/review/SKILL.md`, `skills/gate/SKILL.md`, `skills/execute/SKILL.md`, `skills/explain/SKILL.md`,
  `skills/decompose/SKILL.md`, `skills/routing/references/forwarding.md`

### Each circular hand-off is broken at one named side

- **Answers:** C4.1
- **Rule:** A skill invoked by another skill does not route the input back to its caller, and the forward-once guard in
  the protocol applies to every skill. Each loop the audit named is broken as follows.
    - **`intent` and `challenge`.** When `intent` invokes `challenge`, the scope `intent` passes is the scope. The
      bare-goal row in `challenge` does not fire for a caller-supplied scope; `challenge` questions the direction it was
      given and returns.
    - **`routing` and `recall`.** The continuation check selects `recall` at most once per request. Only open pull
      requests and unmerged branches count as related work; a closed or merged pull request is history that the brief
      may mention. After the brief, `routing` selects the next phase from state with `recall` excluded. A `mismatch:`
      line has an owner for its repair: `execute` writes the repair row on the user's yes, and `recall` names that step
      instead of asking the user to write a row by hand. `flow` still reports a mismatch and never repairs it.
    - **`ops` and `diagnose`.** `ops` decides when root-cause work starts. `diagnose` starts on a hand-off from `ops`
      and does not ask for containment a second time. When containment is not available (no rollback target, no switch),
      `ops` records that and enters its resolve phase.
    - **`gh` and `git`.** `git resolve` owns a conflict on a pull request branch. Given the pull request's head and
      base, it starts the merge or rebase itself; an in-progress operation is no longer a precondition for a hand-off
      from `gh`, and `git` does not send the question back. Who pushes the result is 0016's.
    - **`review` and `triage`.** A `/review feedback` invocation that names one thread, or that carries a draft `triage`
      produced, is scoped to that thread, and the rows that send a multi-signal pull request to `triage` do not apply to
      it. For a plain diff review those rows are better-elsewhere rows: a one-line note, never a stop.
    - **`routing` Step 3.** The ladder has a terminal state. When `/challenge discern` and the fallback delegate find no
      owner, `routing` reports that no Agentics skill owns the request and the session works on it directly or asks one
      question. `discern` returns "no owner" as its result and no longer escalates to the step that called it.
    - **`docs` and `deepen` on decision records.** `docs` owns writing a decision record and applies the three-part test
      itself, reading it from `skills/deepen/references/adr-format.md`. An explicit request for a record is written; the
      test's result is stated in one line and does not block. `deepen` hands a qualifying decision to `docs` and is
      never a precondition for it. (verified: skills/challenge/SKILL.md:40, 2026-11-11;
      skills/routing/references/chain-phases.md:81, 2026-11-11; skills/recall/SKILL.md:220, 2026-11-11;
      skills/execute/references/flow-state.md:225, 2026-11-11; skills/diagnose/SKILL.md:22, 2026-11-11;
      skills/git/references/merge-conflicts.md:7, 2026-11-11; skills/review/SKILL.md:143, 2026-11-11;
      skills/routing/SKILL.md:235, 2026-11-11; skills/challenge/references/discern.md:23, 2026-11-11;
      skills/docs/SKILL.md:72, 2026-11-11)
- **Default / Rejected:** break each loop at the side that has the information to act; rejected: relying on the
  forward-once guard alone, because it ends a loop by stopping and leaves the user with no owner for the work.
- **Later work changes:** `skills/challenge/SKILL.md`, `skills/challenge/references/discern.md`,
  `skills/intent/SKILL.md`, `skills/routing/SKILL.md`, `skills/routing/references/chain-phases.md`,
  `skills/recall/SKILL.md`, `skills/execute/references/flow-state.md`,
  `skills/ops/references/response/response-workflow.md`, `skills/diagnose/SKILL.md`, `skills/gh/SKILL.md`,
  `skills/git/SKILL.md`, `skills/git/references/merge-conflicts.md`, `skills/review/SKILL.md`, `skills/triage/SKILL.md`,
  `skills/docs/SKILL.md`, `skills/deepen/SKILL.md`

### Every advertised operation has a mode, and every decision table has exactly one matching row

- **Answers:** C4.4
- **Rule:** Mode selection and decision tables are total and exclusive. An operation the description or the
  `argument-hint` advertises has a mode that performs it, or it is removed. A table states its precedence (first match
  wins), ends with an "otherwise" row, uses half-open numeric bands so a boundary value matches one band, and tests only
  conditions the agent can observe; a condition it cannot observe becomes one question. A threshold or term has one
  definition in one file, and other skills link to it. A skill infers the mode from the invocation and the conversation,
  announces it in one line, and asks only when two modes remain plausible; a bare invocation asks a fixed question only
  when the conversation holds no answer. When two modes match and one's output contains the other's, the containing mode
  runs. Choosing a mode inside the invoked skill is never a route-away. The named cases are decided as follows. `git`
  performs rebase, stash, cherry-pick and bisect itself under its safety protocol. `execute auto` accepts a wave-pause
  option, so subagents implement and the user reviews at each wave's end. `explain` runs critique for "explain X and
  tell me what's wrong". In `ops`, an active-incident signal anywhere in the invocation selects `response` before any
  first-word alias is read. `/challenge` with competing options runs discern. `docs` builds a README that does not exist
  and maintains one that does. An approved plan with an unwaived blocking coverage item goes to `decompose`. Anything
  with current user impact goes to `ops`, which classifies its severity, and `diagnose` takes a failure with no live
  impact. The cases with no matching row (gate auto-detect on a tagged non-default branch, `/triage <N>` with no `gh`, a
  spec criterion not checkable from a diff, a minor degradation reaching a few customers, a lone fifth proportionality
  signal, an interface-only fix in interactive review) each take the "otherwise" row their table gains. Table defects
  whose subject another record owns stay there: the small-fix size arithmetic (0013), two bind rows in `flow where`
  (0017) and severity mappings (0021). (verified: skills/git/SKILL.md:70, 2026-11-11; skills/execute/SKILL.md:7,
  2026-11-11; skills/ops/SKILL.md:108, 2026-11-11; skills/triage/SKILL.md:288, 2026-11-11;
  skills/routing/references/forwarding.md:46, 2026-11-11)
- **Default / Rejected:** first match wins with a stated order and a final "otherwise" row; rejected: keeping unordered
  tables and resolving a double match by judgment, because the audit found the same input taking different rows in
  different sessions.
- **Later work changes:** `skills/git/SKILL.md`, `skills/execute/SKILL.md`, `skills/explain/SKILL.md`,
  `skills/ops/SKILL.md`, `skills/ops/references/audit/audit-workflow.md`, `skills/challenge/SKILL.md`,
  `skills/docs/SKILL.md`, `skills/triage/SKILL.md`, `skills/scaffold/SKILL.md`, `skills/gate/SKILL.md`,
  `skills/decompose/SKILL.md`, `skills/routing/SKILL.md`, `skills/routing/references/chain-phases.md`,
  `skills/routing/references/forwarding.md`, `skills/threat-model/references/threat-model-workflow.md`

### Descriptions do not overlap without a stated precedence, and frontmatter matches the body

- **Answers:** C4.5
- **Rule:** When two descriptions match the same request, one stated precedence decides, and it lives in `routing` Step
  1 and nowhere else; a skill's description does not restate it. The precedences are: current user impact goes to `ops`
  before `diagnose`; a pull request with bot, check or several human signals goes to `triage` before `review`; remote
  GitHub state goes to `gh` and local repository state to `git`; a readiness question goes to `gate` before `review`; an
  existing plan or scope goes to `challenge` and a bare goal to `intent`; a document that feeds implementation goes to
  `spec` before `docs`. A skill's `argument-hint` lists exactly the modes, flags and aliases the body accepts, and
  `metadata.surfaces` lists only surfaces the body supports. Whether conformance enforces that match is decided in
  [0019](0019-output-and-skill-structure.md); today it checks only `name`, `description` and `version`. What a trigger
  bank must contain once a description changes is decided in [0022](0022-eval-strategy.md). What a `context: fork` skill
  can observe is decided in 0020. (verified: .github/tools/src/conformance/domains/skills/frontmatter.ts:163,
  2026-11-11; skills/explain/SKILL.md:7, 2026-11-11; skills/tdd/SKILL.md:3, 2026-11-11)
- **Default / Rejected:** one precedence list in `routing`; rejected: each skill listing its neighbours in its own
  description, because the copies drift and the description budget is spent on other skills' jobs.
- **Later work changes:** `skills/routing/SKILL.md`, `skills/README.md`, `skills/explain/SKILL.md`,
  `skills/zealot/SKILL.md`, `skills/gate/SKILL.md`, `skills/tdd/SKILL.md`

## Existing records

- 0008: consistent — this record leaves the route from intent to 0013; it changes only how a skill reaches another
  skill.
- 0009: consistent — the small-fix signals still decide which delivery path a change takes; `tdd` running an advertised
  request does not change the path that attaches to it.
- 0012: consistent — `flow` stays a verb on the hook binary that reports a mismatch and repairs nothing; this record
  only names `execute` as the writer of the repair row.

## What this costs

**A user who names the wrong skill gets that skill.** A better-elsewhere row now produces one line of advice and then
the work, so a review can run while a bot finding is open and `forge` can make an edit a plain editor would have made.
The result can be weaker than the other owner's.

**Four skills grow.** `forge` gains hook authoring, `AGENTS.md` and a light edit path; `docs` gains endpoint reference;
`git` gains four operations and starting a merge for a pull request conflict; `scaffold init` gains licence and policy
files. Each is more text in a suite the audit already found too long.

**Every skill is edited.** One protocol means removing the route-away block from each skill that has its own, marking
every `## Don't use when` row with its kind, and ordering every decision table. The trigger banks and the conformance
rule follow under 0022 and 0019.

**`ops` can hand a live incident to `diagnose` uncontained.** When no containment exists, root-cause work starts while
the damage continues. The alternative was no legal move at all.

**UX work stays without a workflow.** A request for flows or mockups is told that Agentics has no workflow for it and is
done in the session with no skill's checks.

**A decision record can be written for a decision that fails the test.** An explicit request wins, and the record says
in one line that the test did not pass.

## Considered and declined

**Add a UX skill.** Declined for now. No usage data shows how often a request reaches the three UX rows, and
`docs/CHARTER.md` asks an asset to earn its place. This is a product call the owner can reverse; the boundary statement
is the default because it costs nothing to replace with a real owner later, and a hand-off to a skill that does not
exist is the defect being fixed.

**Make `routing` the only skill allowed to forward.** Declined. Every route-away would then take two hops, and a skill
that already holds the fetched input would have to hand it to `routing` to hand on. The target classification already
gives each caller the same answer.

**Let a skill refuse and name the owner.** Declined. It is the stop-and-name form several skills use today. The user
then retypes the request to a second skill, and the context the first skill gathered is lost.
