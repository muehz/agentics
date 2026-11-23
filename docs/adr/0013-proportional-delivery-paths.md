# Each delivery path owes work in proportion to the change

This record covers the paths a change can take between a direct edit and the full chain of intent, spec, decompose,
execute and gate. It decides who finishes a fix that `diagnose` has proven, how the small-fix test is measured, what the
`execute` entries accept, how many reviews and suite runs each path owes, and what a path does when a repository has no
tests or a check cannot run in the session. The findings come from the skill audit in
`docs/designs/skill-audit-2026-11/consolidated-issues.md`, theme T1 and item C10.3.

The owner has settled the one question this record put to them: a path exists between the small fix and the full chain.
A one-task change that fails only the size signal takes the middle path, and the other rules say where they use it.

## Rules

### A one-task change that fails only the size signal takes a middle path between the small fix and the full chain

- **Answers:** C1.1
- **Rule:** A change takes the middle path when it fails only the size signal of the small-fix test, fires no
  elevated-tier signal (no public interface or contract, no auth, trust boundary or data handling, no migration), and
  fits one task. It gets one short brief that holds the problem, the behavior, the acceptance criteria and the single
  task; one independent review of that brief; one approval question; then the single-task entry of `execute`, and
  `gate`. `intent` writes the brief as `brief.md` in the work folder, the routing outcome is `brief`, and its approval
  is an `approve brief.md` row. On the middle path the brief replaces `intent.md` and its approval, so the whole path
  has one approval question. The brief is a chain artifact wherever 0016 and 0017 name an intent, a spec or a plan: it
  is hashed, guarded and committed with its approval row, and it stands in for the plan in the closure and slice checks.
  `execute` reads the brief's approval in place of the spec and plan approvals. For these changes the middle path
  supersedes the two sentences that forbade it: "Every intent-started work item gets an approved spec" in 0008 and "Size
  decides only the small-fix path" in 0009. A change that fires an elevated-tier signal, or that needs more than one
  task, takes the full chain with the intent approval, the independent spec review and the tiers of 0008. The small-fix
  path stays, measured as the rule on the small-fix test below says. A change that outgrows its path mid-run stops and
  enters the next heavier one; it is never finished on the lighter path, so a middle-path change that grows past one
  task stops and enters the chain, as a small fix does today. The middle path is a fourth artifact shape and a fourth
  routing outcome, and each gets eval scenarios. Today a request that fails any small-fix signal starts at `intent`, and
  size decides nothing outside those signals. (verified: skills/routing/references/chain-phases.md:112-113, 2026-11-11)
- **Default / Rejected:** add the middle path and route to it by the risk signals 0008 already uses for the tier,
  bounded by the one-task limit; rejected: confirming 0008 and 0009 unchanged in their routing, with the corrected
  small-fix test and the routine tier's combined approval of spec and plan as the only relief, because a change one line
  over a limit would still get an intent, a spec and a plan, with two approval questions and two independent reviews
  before any code. The audit's example, a fix with a proven cause that touches 4 files and 70 lines, would still need
  all three artifacts, and the audit ranks this as its first cross-cutting finding. The reason 0009 gives for the
  small-fix path is that the pull request reviewer is the second person and can read the change in one sitting, and that
  reason holds for a one-task change with no elevated signal as much as for a 50-line one. 0008's objection was to
  routing by size or by apparent concreteness, because the agent that plans the work also makes that call; the middle
  path routes by the same risk signals 0008 uses, so the objection is met rather than overridden.
- **Later work changes:** `skills/intent/SKILL.md`, `skills/routing/references/chain-phases.md`,
  `skills/execute/references/fix-entry.md`, `skills/decompose/references/chain-artifacts.md`, `hooks/go/internal/flow/`,
  `skills/execute/references/flow-state.md`, `skills/execute/SKILL.md`, `skills/routing/SKILL.md`

### One owner finishes a diagnosed fix

- **Answers:** C1.2
- **Rule:** A fix whose cause `diagnose` has proven is finished by `execute`, entered from `diagnose` in the same
  session with the root cause, the evidence, the failing reproduction and the test seam. `diagnose` does not hand the
  fix to `routing` or `intent` to be classified again. Which entry it takes is fixed by where the bug sits. When the
  small-fix signals hold and no flow covers the branch, it is `execute fix`, and the reproduction becomes the red test.
  When the bug blocks a task of a plan that is running, the fix is added to that plan as one task under a recorded
  divergence, after one question to the user, and `execute fix` never opens on a branch that a flow covers. In every
  other case the fix takes the middle path when it qualifies for it under the rule above, and `intent` when it does not,
  and that path starts from the findings instead of deriving them again. The eval banks for `diagnose`, `routing`,
  `decompose` and `gate` are rewritten to this one answer. The audit's three claims hold today: Phase 4 runs only inside
  an approved plan or `execute fix`, and everything else stops after Phase 3 and goes to `intent`; an open small fix on
  a plan branch with no `bind` row makes `flow where` exit 5, so `gate` reads the whole branch as a small fix; and the
  four banks give four different owners. (verified: skills/diagnose/SKILL.md:207-214,
  hooks/go/internal/flow/lookup.go:135, hooks/go/internal/flow/where.go:13-15, 2026-11-11)
- **Default / Rejected:** `execute` is the single owner and `diagnose` enters it directly; rejected: letting `diagnose`
  write the fix in its own Phase 4 outside `execute`, because that makes a second place where code changes without the
  red commit, the review and the trail rows `execute` owns, which is the duplication 0009 declined for `routing`.
- **Later work changes:** `skills/diagnose/SKILL.md`, `skills/execute/SKILL.md`,
  `skills/execute/references/fix-entry.md`, `skills/execute/references/plan-divergence-and-trail.md`,
  `evals/skills/diagnose.md`, `evals/skills/routing.md`, `evals/skills/decompose.md`, `evals/skills/gate.md`,
  `docs/guide/02-routing.md`

### The small-fix test counts only what the fix changes, and counts it once

- **Answers:** C1.3
- **Rule:** The five signals of 0009 stay, and signal 5 is measured as follows. A file's changed lines are the larger of
  its added and removed counts in `git diff --numstat`, so a modified line counts once, in both the 50-line limit and
  the half-file limit. The half-file limit applies only to a non-test file with more than 10 lines at the base; a
  smaller file is bounded by the 50-line limit alone. Lockfiles and generated files are left out of the file count and
  the line count; a generated file is one git marks `linguist-generated` or one a named command rewrites, where the
  command is named by the repository's instruction file (`AGENTS.md` or its conventions section) or by a generated-file
  header in the file itself. A file with neither mark counts as hand-written. Untracked files count only when the fix
  created them: files that were untracked before the classification, ignored files, and files under the work root are
  left out. Signal 4 also holds in a repository with no test suite when the fix names the closest executable check, and
  that check is seen to fail before the change and pass after it. A new non-test file that the fix itself creates still
  fails signal 5. The audit's claims hold today: changed lines are "added plus removed", the text has no exclusion for
  lockfiles or generated files, the untracked list is read whole, and signal 4 accepts only an existing covering test or
  a new regression test. (verified: skills/routing/references/chain-phases.md:122-138, 2026-11-11)
- **Default / Rejected:** correct the measurement and keep the limits of 3 files and 50 lines; rejected: raising the
  limits, because the audit's failures (a one-line `VERSION` bump, 3 lines of a 10-line config, a lockfile refresh) come
  from how lines are counted, and a larger number would admit real rewrites without fixing them. Also rejected: dropping
  the half-file limit, which the user added to 0009 to catch a file that is wholly rewritten inside the line budget.
- **Later work changes:** `skills/routing/references/chain-phases.md`, `skills/execute/references/fix-entry.md`,
  `skills/gate/references/branch/branch-workflow.md`, `evals/skills/routing.md`, `evals/skills/intent.md`

### The fix path is one announcement, one change and one review, and execute accepts a single task

- **Answers:** C1.4
- **Rule:** Three things hold. First, on the `fix` entry the session that announced the fix may write the change itself.
  The entry owes one announcement, which is the go-ahead; a feature branch created and announced under 0016, with no
  question, when the session is on the default branch or a detached HEAD; the red commit for a bug fix, or the recorded
  failing run 0016 allows in its place; one fix commit; one dispatched quality review; and the offer to `gate`. No
  separate implementer dispatch is required for the red leg or the fix leg. Second, `execute` accepts one
  already-decomposed task (a file scope, a verifiable acceptance criterion and a verification command) as an input, so
  the floor of two tasks on the inline list is removed. A task that qualifies for the middle path needs no approved
  spec, because its reviewed and approved brief stands in for the spec and the plan; any other task needs the approved
  spec first, exactly as the inline list does. A one-task plan is a valid plan, and checklist minimums such as the three
  risk rows scale down to what the plan contains. Third, `arena` enters through that single-task input, or through one
  named task of an approved plan, and stays unavailable on the `fix` entry. The audit's claims hold today: the fix entry
  dispatches an implementer and the controller writes no code; the inputs are an approved plan, an inline list of "2+"
  tasks, or a small fix; and `arena` takes "one task", which none of those inputs supplies. (verified:
  skills/execute/references/fix-entry.md:44-57, skills/execute/SKILL.md:46, skills/execute/SKILL.md:119,
  skills/execute/SKILL.md:185-187, skills/decompose/SKILL.md:731, 2026-11-11)
- **Default / Rejected:** the session may write a small fix and the dispatched review stays as the independent check;
  rejected: keeping the controller rule on the fix path, because a second dispatch for a change of at most 50 lines
  costs more than the edit, and the independence it buys is already supplied by the review and by the pull request
  reviewer 0009 relies on. Also rejected: dropping the review instead, which would leave the fix with no check inside
  the session.
- **Later work changes:** `skills/execute/SKILL.md`, `skills/execute/references/fix-entry.md`,
  `skills/execute/references/arena-mode.md`, `skills/decompose/SKILL.md`, `evals/skills/execute.md`,
  `evals/skills/routing.md`

### Each path owes a fixed number of reviews and suite runs, and none is repeated on unchanged content unless the user asks

- **Answers:** C1.5
- **Rule:** No step repeats an independent review or a full-suite run on content that has not changed since the last
  one, unless the user asks for it. The counts per path are these. A direct edit owes neither. A small fix owes one
  quality review and one full-suite run; at `gate`, the review covers only the lanes the fix's own review did not
  (security), unless the diff changed after that review. The middle path owes one review budget per revision of the
  brief, counted and reset as 0017 counts a document review, one quality review of the diff and one full-suite run; a
  brief review that was a self-read brings the packet review, as 0021 says for a plan. The full chain owes one review
  budget per revision of the spec and one per revision of the plan, as 0017 counts it (up to two independent runs per
  revision), one quality review per review group in `execute`, a packet review only for tasks the plan marks, and for
  every task when the plan review was a self-read (0021), and one branch review at `gate`. The challenge rounds in
  `intent` and `decompose` are conversations with the user and are not counted. The tier is evaluated twice at most: by
  `spec` from the spec and the intent, and by `decompose` once, against the final plan, for plan-level signals only.
  `decompose` takes the spec's result as given and does not check before drafting and again before the question. For
  suites, `tdd` runs the tests that cover the touched code in each cycle and after each refactoring, and the full suite
  once, when the task's last cycle is green. In an `execute` wave that shares a working tree, the controller makes that
  full run on the committed snapshot. `gate` makes its own commits first and runs the full suite once on the tree it
  hands off, plus one run for each fix that follows a failure. The audit's claims hold today: the spec and plan reviews
  are each non-waivable and `gate` runs `/review axes` on every run; the tier is evaluated by `spec`, then by
  `decompose` before the draft, after it, and again before the combined question; `tdd` runs the full suite in Green and
  after every refactoring; and `gate` re-runs Step 1 after each of its own commits. (verified:
  skills/spec/references/draft.md:342, skills/spec/references/draft.md:358, skills/decompose/SKILL.md:297,
  skills/decompose/SKILL.md:524, skills/decompose/SKILL.md:678, skills/gate/SKILL.md:349, skills/tdd/SKILL.md:41-42,
  skills/tdd/SKILL.md:214, skills/gate/references/branch/branch-workflow.md:608,
  skills/gate/references/branch/branch-workflow.md:687, skills/gate/references/branch/branch-workflow.md:1072,
  2026-11-11)
- **Default / Rejected:** fixed counts per path with reuse of a review or run whose content is unchanged; rejected:
  making the spec or plan review waivable, which 0008 declined because a waivable review becomes the default under time
  pressure. Also rejected: keeping a full-suite run in every `tdd` cycle, because it multiplies the slowest step by the
  number of cycles and, in a shared working tree, tests a sibling's half-written edit.
- **Later work changes:** `skills/spec/references/draft.md`, `skills/decompose/SKILL.md`, `skills/tdd/SKILL.md`,
  `skills/gate/SKILL.md`, `skills/gate/references/branch/branch-workflow.md`, `skills/execute/SKILL.md`,
  `skills/execute/references/fix-entry.md`

### A path can finish without a test suite, and a check that cannot run here does not void the others

- **Answers:** C10.3
- **Rule:** Three cases are told apart. When the repository has no test command at all, `gate` Step 1 has a third
  outcome beside pass and fail: it reports that no tests exist, continues to the review and the audit, and never reports
  tests as passed. On every path the change is then verified by the closest executable check, seen to fail before and
  pass after, which is the substitute `tdd` already allows, and `gate` reads a recorded substitute as evidence, not as a
  finding. When a test command exists but cannot run in the session (missing credentials, hardware, a toolchain or a
  service), tests stay `NOT RUN` with the cause named and the gate stays `BLOCKED`, as now, but the review, the audit
  and the other steps still run and report their own results instead of `n/a`. When a plan task can be verified only
  with credentials or hardware the session lacks, the plan is valid if the task names the command, the expected result
  and who or what runs it (the user, or CI). `execute` records that task as verification deferred and `gate` lists it as
  `NOT RUN` with that owner. A deferred verification blocks the gate in the same way as a test command that cannot run:
  the gate stays `BLOCKED` until the named owner's run exists with a pointer to its result, or the user accepts the risk
  in writing, and the review, the audit and the other steps run meanwhile. The audit's claims hold today: Step 1 has
  only a pass branch and a fail branch; an accepted test risk leaves the review, security, audit and converge lines
  `n/a`; the small-fix path needs a covering or new regression test; and the worked example fails a plan whose task
  verifies only against a sandbox the session cannot reach. (verified:
  skills/gate/references/branch/branch-workflow.md:36-60, skills/gate/references/branch/branch-workflow.md:863-865,
  skills/routing/references/chain-phases.md:122, skills/decompose/references/worked-examples.md:155-157,
  skills/tdd/SKILL.md:255-256, 2026-11-11)
- **Default / Rejected:** name the three cases and let every other check run; rejected: treating a repository without
  tests as an accepted risk on every gate run, because the user would accept the same risk each time and lose the review
  and the audit with it. Also rejected: counting a check that could not run as passed, which would hide the one fact the
  pull request reviewer needs.
- **Later work changes:** `skills/gate/references/branch/branch-workflow.md`, `skills/decompose/SKILL.md`,
  `skills/decompose/references/worked-examples.md`, `skills/tdd/SKILL.md`, `skills/routing/references/chain-phases.md`,
  `skills/execute/SKILL.md`

## Existing records

- 0008: amended — its sentence "Every intent-started work item gets an approved spec" gives way for middle-path changes,
  which get a reviewed and approved brief in place of the intent, the spec and the plan; its intent approval, spec
  review and tiers stay for every other change.
- 0009: amended — its sentence "Size decides only the small-fix path" gives way, because a one-task change that fails
  only the size signal now takes the middle path; the rules above also change how its fifth signal is measured and who
  may write the fix.
- 0012: consistent — these rules use the existing `flow` verbs and the exit 5 lookup as that record describes them.

## What this costs

**The small-fix test is longer to state.** The size signal now has a counting rule, a floor for small files and three
kinds of excluded file. The 10-line floor is a chosen number, not a measured one.

**A small fix can be written by the session that reviews its result.** The dispatched review and the pull request are
the only independent readers. A session that writes a poor fix and a reviewer that misses it leave nothing else in the
way.

**Fewer suite runs means a failure can surface later.** A regression that a per-cycle full run would have caught is now
found at the end of the task or at the gate, after more code sits on top of it.

**Reusing a review depends on knowing what it covered.** `gate` has to tell from the record whether a diff changed since
its review. Where the record cannot show that, the review runs again.

**A repository without tests gets a gate that is not blocked.** The report says no tests exist, and a reader who skims
it may take the missing failure for a pass.

**The planning agent again judges which path a change takes.** 0008 declined routing by how concrete or how large the
work looks, because the agent that plans the work also makes that call. The middle path brings that call back for one
class of change, bounded by the tier signals and the one-task limit. It also adds a fourth artifact shape, a fourth
routing outcome, and eval scenarios for both.

**The routine tier still stacks two reviews for a change of more than one task.** The middle path takes only a one-task
change with no elevated signal out of the full chain. For everything else these rules remove repeats. They do not remove
the spec review or the plan review from a routine change that goes through the full chain.

## Considered and declined

**Let the user waive the chain for one change, in place of a middle path.** Declined. A waiver exists already, and the
audit's finding is that the default route is out of proportion, not that the user lacks an override. A path that must be
asked for each time is paid for in questions.

**Raise the small-fix limits until the audit's examples pass.** Declined. The 4-file, 70-line fix would pass at 5 files
and 100 lines, and the next example would sit just above that. The limit stays a size test, and what lies above it for a
one-task change is the middle path.
