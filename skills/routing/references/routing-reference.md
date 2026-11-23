# Routing reference

## Worked commission test

These rows tell a commission (a lifecycle row) from unclear phrasing and plain conversation, per **Commission, not
grammar** in SKILL.md.

| Request                                                 | Reading                                                | Route                                                                                                                                                                                                                                                          |
| ------------------------------------------------------- | ------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| "Can you draft a plan for moving off the legacy queue?" | commission: names a deliverable (a plan)               | the owning phase (`intent` when no intent exists)                                                                                                                                                                                                              |
| "Could we plan a migration away from the legacy queue?" | unclear: feasibility phrasing, no deliverable named    | one concise clarifying line; no phase                                                                                                                                                                                                                          |
| "What does the legacy queue do today?"                  | conversation: seeks explanation                        | direct answer; no phase                                                                                                                                                                                                                                        |
| "Is this ready to merge?"                               | commission (readiness): asks for a completion judgment | `gate`, forwarded: its Step 0 refreshes the recorded evidence read-only, answers, and runs nothing. Asked as `/agentics:gate`, the question runs `gate check`: against the current head, from fresh evidence, with no disposition, no push and no verdict row. |

---

## Worked small-fix test

These examples apply the [small-fix signals](chain-phases.md#small-fix-signals) to a change that no flow covers. The
counts come from `git diff --numstat` against the base. A file's changed lines are the larger of its added and removed
counts. Lockfiles and generated files are left out of both counts.

1. **A version bump.** The change replaces the only line of a one-line `VERSION` file. Numstat reports 1 added and 1
   removed, so the counts are 1 file and 1 line. The size signal decides, and it holds: 1 file is within the limit of 3
   non-test files and 1 line is within the limit of 50 changed lines. The half-file limit applies only to a file with
   more than 10 lines at the base, so it does not apply to this file. Every other signal holds. Outcome: a small fix,
   which goes to `execute fix`.
2. **A dependency bump.** The change replaces one line of a 20-line `package.json` and 60 lines of a 400-line
   `package-lock.json`. The lockfile is left out of the file count and the line count, so the counts are 1 file and 1
   line. The size signal decides, and it holds: the counts are within both limits, and 1 line of 20 is under half of the
   manifest. Every other signal holds. Outcome: a small fix, which goes to `execute fix`.
3. **A fix in a repository with no test suite.** The change makes `scripts/backup.sh` exit 1 when its target directory
   is missing. No test covers the script, and the repository has no suite to add a regression test to. The covered
   signal decides, and it holds through the closest executable check. The fix names that check:
   `sh scripts/backup.sh /no/such/dir` must exit 1. The check is seen to fail before the change, when the command exits
   0, and to pass after it. The fix changes 1 file and 2 lines, within the size limits, and the covered signal decides.
   Outcome: a small fix, which goes to `execute fix`.
4. **A one-task change over the size limits.** The change replaces 70 lines across four source files of 100 lines each,
   so the counts are 4 files and 70 lines. The change fits one task and fires no elevated-tier signal: it changes no
   public interface or contract, no auth, trust boundary or data handling, and it has no migration. The size signal
   decides, and it is the only signal that fails: 4 files is over the limit of 3 non-test files and 70 lines is over the
   limit of 50. Outcome: the change takes the [middle path](chain-phases.md#middle-path), not the small-fix path and not
   the full chain.

---

## Worked incident precedence (Step 1)

**Request:** "Checkout is failing in production right now. The payment queue is backing up, and orders are erroring for
every customer."

- **Condition:** a live failure materially affects users and revenue right now. Step 1's incident bullet matches,
  whatever wording describes the outage.
- **Route:** `ops` in response mode (`/agentics:ops response`), immediately. The domain-flavored words ("queue",
  "erroring", "payment") do not send it to `diagnose`.
- The hand-off carries the skill name and mode, a one-line situational brief, and the active-outage signal. Non-urgent
  asks in the same message defer until the incident resolves.

---

## Orchestration mode mapping

An agent has three execution paths: it calls a tool itself, it becomes a specialist inside the same session, or it hands
the work to a subagent. Each platform's orchestration modes map to those paths.

| Execution path                                                                                  | Claude Code mode(s)                                                   | Copilot CLI                                                                 |
| ----------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| **A direct tool call**: the agent runs the tool itself                                          | **Direct**                                                            | **Direct**                                                                  |
| **A specialist step in the same session**: the agent's tool set and prompt are swapped in place | **Team teammate** (`CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`)          | _(No direct analog)_. The closest option is Sequential with context handoff |
| **A hand-off to a subagent**: a separate agent does the work and returns the result             | **Sequential**, **Parallel**, **Escalated**, **Reasoning** (one-shot) | **Sequential**, **Parallel**, **Escalated**, **Reasoning**                  |

Each mode name has a plain meaning:

- **Direct**: one skill owns the request and runs it.
- **Team teammate**: a spawned agent that stays addressable across turns.
- **Sequential**: one skill's output is needed before the next starts.
- **Parallel**: independent skills run separately, and their results merge afterward.
- **Escalated**: an ambiguous or high-risk request gets tighter scoping and stricter sequencing.
- **Reasoning**: no owner is clear, so the request escalates instead of guessing one.

The specialist step swaps the tool set and prompt in place, mid-session. It maps to Claude Code's experimental team
mode. There, a spawned teammate stays addressable across turns. Copilot CLI has no equivalent _in-place
reconfiguration_. The specialist is a separate dispatched agent rather than the current agent with new tools. Copilot
subagents can still persist and communicate across parent, child, and sibling relationships, so follow-up work does not
require a fresh one-shot handoff. See [surface dispatch](surface-dispatch.md) for each surface's continuation mechanism
and its current limits.

---

## Authorization and inspection rules

- **One authorization rule.** A plan that `flow status` lists as a current `plan.md` approval is consent to execute that
  plan's scope, however the user arrived, so a selected `execute` is announced and run with no forwarding question.
  `execute`'s own preflight still runs. An approval that reads `unconfirmed here` is not consent. A plan whose
  `**Coverage:**` line names an unwaived blocking item goes to `decompose`. What counts as the user's approval, in any
  wording, and what "just do it" means in each situation, is in [approval.md](contracts/approval.md). Selection never
  creates approval.
- **Steps named up front.** A step the user names in the request (a commit, a push, a draft pull request, starting
  execution after planning) is authorized when its turn comes, under
  [Steps named up front](contracts/approval.md#steps-named-up-front). The handoff carries which named steps are
  authorized and which are still reserved.
- **Authorization carries forward, and only forward.** An authorized handoff does not re-ask a consent question the user
  already answered. A request to discuss or plan never becomes permission to implement or publish.
- **Inspect, do not accept.** A claimed state selects a phase only after the named artifact is read. Reading it is a
  check the session runs itself; it asks the user for no proof. The test for a claimed plan is **Already decomposed**
  below. A domain noun is not evidence that decomposition or execution is authorized.
- **Memory is a lead.** Remembered facts (an earlier session's branch heads, pull request states, CI results, model
  names) are where to look, not evidence. Refresh any changeable fact before it decides a route, an approval, or a
  readiness claim. Take `recall` when the state has to be reconstructed.

---

## Step 0 preflight details

- **Already decomposed.** Before a request reaches `execute` on a claimed plan, read the plan. A file with no `### Task`
  entry, or one that `flow status` does not list as a current `plan.md` approval, is not an approved plan, and it goes
  to `decompose`. When the user approves a named plan in their own words in this session, that is their approval under
  [approval.md](contracts/approval.md), and `execute` examines the plan before it records the approval.
- **A failure report is not a chain entry.** Root-cause a report of something already broken with `diagnose` before
  anything changes. Examples are an error, a failing check, wrong output, and a slowdown. Once `diagnose` proves the
  cause, it carries the fix forward itself with its findings, into the entry that the place of the bug decides; the fix
  is not classified again here. When a proven cause reaches routing anyway, with its root cause, evidence, and failing
  reproduction, take the entry `diagnose` would take: `execute fix` when every
  [small-fix signal](chain-phases.md#small-fix-signals) holds and no flow covers the branch, a new task in the running
  plan when the bug blocks one, and otherwise the [small-fix classification](chain-phases.md#small-fix-classification)
  outcome, which starts from the findings. A [user's change](../../execute/references/fix-entry.md#a-users-change) is
  not a failure report: it takes the start-or-continue path, and small-fix classification decides between `execute fix`
  and `intent`.

---

## Step 1 routing details

- **Row 1 precedence.** Symptom wording inside an active-incident report (error codes, failing endpoints, backend nouns)
  never sends it to `diagnose` and never splits it into a fix-now and postmortem-later composite. Defer other matches,
  record each one, and hand it back into the decision tree when the incident closes or carry it into the postmortem's
  action items. Deferral is a tracked hand-back, never a drop.
- **Product runbooks and documents are row 15 work.** A runbook or document _for_ the user's product, service, or
  on-call rotation goes to `docs`, or to `ops` (`/agentics:ops author`) for an operational runbook. Steady-state
  alerting and SLO design is product work too. Such a runbook sub-request takes row 15 even inside a mutating
  commission, before Step 0 row 9, and its dispatch waits for earlier sub-requests under Step 2's sequential rule.
- **Row 11's delivery path.** `tdd` runs the request. Run the
  [small-fix classification](chain-phases.md#small-fix-classification) first when the change is mutating and no flow
  covers it, and pass its outcome as the delivery path the work runs under; a task of an approved plan carries that plan
  instead.
- **Row 13 is a host limit.** The host blocks the session from loading a skill with `disable-model-invocation: true`.
  Name the match and the plugin-qualified command, and do no partial version of its work.
- **Rigid skills keep their discipline.** `tdd`, `diagnose`, and `/agentics:gate check` never adapt their steps to
  context. A user who declines a step meets the process-gate path in [gates.md](contracts/gates.md#process-gate): the
  cost named once, then a yield on the next decline, with one `waive` row. Safety holds inside them (such as status
  claims without evidence) never yield. Every other skill adapts its principles to context.
- **Step 1 invocation payload.** A Step 1 route is a step of this workflow, not a route-away. The target's own consent
  gates still fire. Carry the skill name and mode, a concise brief of what happened and what is asked, and every urgency
  or impact signal that selected the route.

---

## Step 2 and 3 details

- **Sequential cues.** The trigger for sequencing is the dependency, not conjunction words: "then" and "after" are
  common cues, and a bare "and" also sequences when one half builds on the other ("add X on top of it", "using the new
  schema").
- **User's word and rule divergence.** "The frontend team owns this" changes the route; "that's already been reviewed"
  drops the sub-request it names. When the user departs from a repository rule covering the work, load
  [user-authorized-divergence.md](user-authorized-divergence.md) and carry its Divergence record in the handoff. When
  the user names no rule, routing makes the one read-back that file requires before the handoff. With no flow for the
  run, the session that owns the run writes the `waive rule` row.

---

## Step 4 verification details

- **Handoff context.** The handoff names the later `gate` offer and carries known verification gaps and waivers from
  item 6 of **What a phase handoff carries** in SKILL.md.
- **Closure-owning workflows.** When the destination owns its own closure, as `ops` does through resolution and
  postmortem, the `gate` offer waits until that workflow completes.
- **When verification cannot run.** Missing verification infrastructure goes to `scaffold`, and an ordinary failing
  check goes to `diagnose`, each reached per [forwarding.md](forwarding.md#target-classification).

---

## Delegation details

- **Controller return contract.** The controller requires of a return only what the template asked for, and every word a
  template can return has a branch in the controller. A template never tells a subagent to invoke another skill's
  workflow for setup; the controller does that before dispatching.
- **Continuation.** Some surfaces support continuation or two-way messaging with a dispatched subagent; write the prompt
  so it works either way, and use continuation only after confirming this session has it.
- **Adapter rule.** Load the adapter rather than inventing an equivalent: [model-ladders.md](model-ladders.md) for
  tiers, [surface-dispatch.md](surface-dispatch.md) for dispatch tools and limits, and
  [copilot-tools.md](copilot-tools.md) to translate Claude Code tool names onto another surface (including for
  non-dispatch questions about tool names).
