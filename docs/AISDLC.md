# The Agentics AI SDLC

This document describes how Agentics carries a change from a request to a merged pull request, and how that flow lines
up with Anthropic's published guidance for AI-assisted development. The [consumer guide](guide/README.md) shows how to
ask for each step. The skills own the rules. This document summarizes them and links to their owners.

## The flow

### Stages and artifacts

| Stage           | Owner                                                                                             | Artifact                                                                                                                                                                            | Approval                                                                                      |
| --------------- | ------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| Plan            | [`intent`](../skills/intent/SKILL.md)                                                             | `intent.md`: Problem, Outcome, Affected users and systems, Constraints, Decisions, Open questions                                                                                   | One question once the challenge resolves the direction. A yes logs an `approve intent.md` row |
| Design          | [`spec`](../skills/spec/SKILL.md)                                                                 | `spec.md`: Problem Statement, Solution, Success Criteria, Verification, Scope Decisions, Policy Conflicts, Out of Scope, plus User Stories and Further Notes when they have content | After the independent spec review, and never while a Policy Conflicts entry reads `open`      |
| Build           | [`decompose`](../skills/decompose/SKILL.md), then [`execute`](../skills/execute/SKILL.md)         | `plan.md`: tasks, slices, and lanes                                                                                                                                                 | With the spec on the routine tier, or on its own on the elevated tier                         |
| Test            | `execute`, applying [`tdd`](../skills/tdd/SKILL.md) and [`diagnose`](../skills/diagnose/SKILL.md) | Task commits, plus a red commit for a bug fix                                                                                                                                       | None. Each task's verification is recorded                                                    |
| Review and ship | [`review`](../skills/review/SKILL.md) and [`gate`](../skills/gate/SKILL.md)                       | The gate record and the pull request                                                                                                                                                | The user decides each open Important finding that is not record drift                         |

Each work item keeps these artifacts in one work folder, with a `state.tsv` record that skills write through the `flow`
helper. The record holds every approval, verification, waiver, and skip with its reason, and `flow status` reads the
phase and the next step from it. Every intent leads to a spec, sized to its content, and
[ADR 0008](adr/0008-every-intent-gets-an-approved-spec.md) records why. Intent publishes nothing but its own
`intent.md`. A work item merges through one pull request, or one per lane or slice branch when the plan is sliced.

### Approvals and risk tiers

Risk sets how much ceremony runs, never who approves. The user approves every artifact, and the agent never approves its
own work.

A work item is elevated when any of these signals fires, and routine otherwise:

- The spec or the intent names a change to a public interface or contract.
- It touches auth, a trust boundary, or data handling. The intent's `## Affected users and systems` section is read for
  this signal.
- It involves a migration or a persisted-format change.
- The plan has a Deep task, three or more file groups, or an ownership boundary.
- A file in a task's Modify list has no test or verification command in the plan that exercises it.

The user can raise the tier at any point, never lower it. The tier and the signals that fired go in the `why` cell of
the spec and plan `approve` rows.

Every spec gets an independent review before the user is asked to approve it. The reviewer is a fresh general-purpose
subagent at the Deep model tier, carrying the spec-reviewer charter from `spec`'s references. It runs at Max only when
the user explicitly selects Max for the review. On Claude Code, Codex, and Cursor the reviewer sits one position above
its author, as `execute`'s reviewers do; at the top of a ladder the ladder's top-clamp handling applies instead (raise
effort where supported, otherwise keep the separate reviewer and disclose the shared binding). On Copilot CLI and VS
Code it follows review class (see **Reviewer selection** in `skills/routing/references/model-ladders.md`). It reports
findings only. The spec author resolves them or presents them with the approval question. On a host without subagent
dispatch, the spec author reads the spec against the charter in one pass, records a self-read review, and the approval
question states that the review was not independent.

- **Routine:** after the review, `decompose` drafts the plan against the still-unapproved spec, and one question
  approves both. `decompose` checks the tier before and after drafting. If a signal fires, it stops and asks for the
  spec's approval on its own first, and the plan gets its own question. With intent approval, a routine item asks two
  approval questions.
- **Elevated:** the spec is approved on its own. Then `decompose` writes the plan, runs its plan-document review (which
  every plan gets), and asks for the plan's approval. With intent approval, an elevated item asks three approval
  questions.

### Execution

- `auto` is the default. Dispatched subagents implement each task. One model quality review runs per review group, and
  an integration review runs at group boundaries when its triggers fire. No user stop between waves. When tasks prove
  not to be independent, `auto` runs one task per wave.
- `pair` is the in-session mode, where the user and the agent work in the same session. The same model reviews run
  through reviewer subagents, and the run pauses for the user's review at each wave's end. Only the user selects `pair`.
  A stored `pair` preference counts as the user's choice, and the run names it at the start.
- `arena` runs competing candidates for one task. It is chosen explicitly and is never the default.

On a host without subagent dispatch, `execute` stops instead of running `auto` or `arena`. There, `pair` is the only
mode that runs, and only the user selects it. Its reviews then run in the session as separate passes.

In `auto` and `arena`, the session driving the run never changes code. It writes flow artifacts, stages, and commits. It
makes exactly two kinds of code-touching commit and writes the content of neither: the `git revert` that discards an
attempt, and the red commit of the test files an implementer returns. A review finding goes back to the implementer or
to a fresh one. After two failed corrections on one task, the session re-dispatches it once to a fresh subagent with a
rewritten packet. If that attempt also fails twice, the task stops and the user chooses continue, drop, or replace.

Each task's `verify` row records its commit and the `git patch-id --stable` of the implementer's diff. Flow artifacts go
in artifact-only commits. `gate branch` checks that every commit on the branch is a recorded task commit, a recorded red
commit, or an artifact-only commit, that each recorded patch-id still matches, and that every changed file falls within
a task's file scope or a recorded plan divergence. `gate branch` requires the same commit evidence in every mode. An
unrecorded commit is an Important finding in the `audit` lane. Accepting it writes an attested `commit user` row. A
known gap remains: an edit made inside a merge commit escapes the audit's commit checks, and only the file-scope check
can see it, and only when its path is out of scope.

`decompose` marks a slice as a lane (`**Lane:** yes`) when its dependencies are `none` or merged slices, its tasks
depend on no unmerged slice, its files overlap no other open lane, and at least two slices qualify. `execute` writes one
launch packet per lane under the work folder's `temp/lanes/`, and the user starts each lane in its own session on their
host. The home session runs `gate branch` on each lane branch. Agentics does not dispatch lanes itself.

### Tests and review

- A bug fix starts with `diagnose`. Its failing test is committed on its own before the fix. Review and gate report any
  later change to that commit's test files as an Important finding. Outside a plan task or a small fix, `diagnose` stops
  at the root cause and hands the fix to `intent` or to the small-fix path.
- `tdd` is a method `execute` applies to a task in an approved plan, or to a small fix. It is never a path from intent
  to code.
- Gate shows the review tally on its `Review:` line (`<N> Important, <M> Nit open`) and the open findings in its report.
  The tally never enters the pull request body, which carries only a failed or unrun check and each finding a person
  accepted or deferred. For each open Important finding that is not record drift it asks one question: fix, accept with
  a reason, or defer with an issue link. It resolves record-drift findings itself and reports what it did. Record drift
  is limited to Markdown and `state.tsv` paths (the gate edits only paths the branch changed), never a file documenting
  a hook's safety rules, exceptions, or limits, a workflow source, an agent definition, or a fixture, and never a
  Security or red-commit finding. Nothing halts on a finding alone.
- A repository's `REVIEW.md`, when present, defines the review passes, the severities, and what to skip, and it wins
  where it disagrees with the review skill. `scaffold` offers a starter.
- Every postmortem has an action item for a regression test or eval, or the line `n/a: <reason>`.

### The small-fix path

Routing classifies a change as a small fix only when no work folder covers it and every signal holds:

1. The diff can be described in one sentence. That sentence goes in the announcement and the pull request.
2. It addresses one concern.
3. It changes no public interface, contract, trust boundary, or data handling.
4. Existing tests cover the touched code and the covering test is named and run, or the fix adds a regression test.
5. It changes at most 3 non-test files and at most 50 changed lines across them, and no non-test file has more than half
   of its lines changed.

The agent announces the classification and every signal. The user can move the work into the full chain at any time. In
`auto`, `execute fix` has a dispatched subagent implement the change; in `pair` the session makes it. Either way it
saves no work folder and logs a `skip chain <branch>` row to `unassigned/state.tsv` under the work root. If a signal
fails during implementation, it stops and routes the work to `intent`. A branch with that row and no later
`pivot fix <branch>` row is a small fix in progress, and routing resumes it. Gate checks the verification, the review,
the controller audit, the red commit for a bug fix, and the pull request description, which states the why, the signals,
and the red commit's SHA for a bug fix. [ADR 0009](adr/0009-size-decides-only-the-small-fix-path.md) records why size
now decides this path.

## Alignment by stage

Each row is one recommendation from the [sources](#sources-and-re-review) that applies to what Agentics prescribes in a
session or what `scaffold` tells a repository to set up. The Status column reads:

- **Aligned:** Agentics already did this before the alignment work.
- **Closed here:** the alignment work made Agentics follow it.
- **Divergence:** Agentics departs on purpose, for the stated reason.
- **Gap:** Agentics does not follow it yet, and the named issue owns the work.

| Stage         | Recommendation                                                                                                                  | Source         | Status      | Agentics mechanism or reason                                                                                                                                                                                                                                                                                                                 |
| ------------- | ------------------------------------------------------------------------------------------------------------------------------- | -------------- | ----------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Scope         | Separate people hold the author, approver, and reviewer roles                                                                   | Playbook       | Divergence  | Agentics has one user per session, who holds every human role. The separation kept is that the agent never approves its own work                                                                                                                                                                                                             |
| Scope         | A Maintain stage runs headless metric bands, scheduled scans, and agent-triggered runbooks                                      | Playbook       | Divergence  | Interactive sessions are the product boundary ([charter §15](CHARTER.md#15-interactive-sessions-are-the-product-boundary)). Monthly `REVIEW.md` tuning falls here too                                                                                                                                                                        |
| Scope         | Agents run in CI                                                                                                                | Playbook       | Divergence  | Agentics ships no headless operation or consumer CI/CD automation ([charter §15](CHARTER.md#15-interactive-sessions-are-the-product-boundary))                                                                                                                                                                                               |
| Plan          | A human approves the intent before design starts                                                                                | Playbook       | Closed here | `intent` ends with one approval question, asked on its own. A yes logs an `approve intent.md` row, and spec does not start without it                                                                                                                                                                                                        |
| Plan          | The intent names the users and systems the change affects                                                                       | Playbook       | Closed here | `intent.md` has an `## Affected users and systems` section, which also feeds the risk-tier signals                                                                                                                                                                                                                                           |
| Plan          | Intent and spec are each reviewed in their own pull request                                                                     | Playbook       | Divergence  | With one user, a separate pull request adds no second reviewer. Approvals are recorded in the work folder's `state.tsv`, and the work item merges through its own pull request rather than one per artifact                                                                                                                                  |
| Plan          | Intents are kept in the repository as durable records                                                                           | Playbook       | Aligned     | Each work item has a work folder, grouped by month, with the intent beside its spec and plan so `derived-from` links stay relative                                                                                                                                                                                                           |
| Plan          | Past work is consolidated and reversed work is retired                                                                          | Playbook       | Gap         | [Work-folder retention](https://github.com/muehz/agentics/issues/11) tracks how to prune completed folders and preserve durable decisions                                                                                                                                                                                                    |
| Plan          | The flow is measured: time to committed intent, spec rework after planning, first-pass merge rate, plan approval to merge       | Playbook       | Divergence  | `flow status --metrics` prints time to intent, intent to spec, spec rework, and plan approval to the gate verdict, plus review counts by kind, summed review latency and blocked seconds, and the escape count. It has no first-pass merge rate, and the last time measure ends at the verdict, because gate writes its row before the merge |
| Design        | Every change that has an intent gets a written spec before planning                                                             | Playbook       | Closed here | Every intent leads to an approved `spec.md`, sized to its content. `decompose` refuses an intent-only source for work that started at intent                                                                                                                                                                                                 |
| Design        | The spec is self-contained: problem, binding constraints, interfaces and files, exclusions, and an end-to-end verification step | Playbook       | Closed here | The spec has seven required sections. Its Verification section holds the end-to-end step the plan's outcome proof implements                                                                                                                                                                                                                 |
| Design        | Every spec gets an independent review before approval                                                                           | Playbook       | Closed here | A fresh general-purpose subagent at the Deep tier (Max on explicit selection) reviews every spec with the spec-reviewer charter. Deep sits one position above the Standard author, as in `execute`. Its findings precede the approval question. On a host without dispatch, a disclosed self-read stands in                                  |
| Design        | Policy conflicts are resolved before the spec is approved                                                                       | Playbook       | Closed here | Each Policy Conflicts entry has a `Resolution:` line. Spec asks no approval question while any entry reads `open`                                                                                                                                                                                                                            |
| Design        | An artifact records the version of the tool that wrote it                                                                       | Playbook       | Closed here | The spec records `Written by: spec v<version>`, as the plan does                                                                                                                                                                                                                                                                             |
| Design        | The prompt that produced an artifact is logged with it                                                                          | Playbook       | Divergence  | The intent and its challenge record carry what drove the spec. Logging the prompt adds a record nobody reviews                                                                                                                                                                                                                               |
| Build         | No code is written before a plan is accepted                                                                                    | Playbook       | Closed here | `tdd` is a method `execute` applies to a task in an approved plan or to a small fix, never a path from intent to code                                                                                                                                                                                                                        |
| Build         | Risk decides how much ceremony runs and who reviews independently, never who approves                                           | Playbook       | Closed here | Routine and elevated tiers from objective signals. The user can raise a tier, never lower it. The tier and its signals are recorded in the spec and plan `approve` rows                                                                                                                                                                      |
| Build         | Independent work runs in parallel sessions                                                                                      | Playbook       | Closed here | `decompose` marks lanes and `execute` writes a launch packet per lane. The user starts each lane on their host, each lane's session writes a `bind` row for its branch so every lane branch finds the same flow, and the home session gates each lane branch                                                                                 |
| Build         | Parallel sessions are dispatched automatically                                                                                  | Playbook       | Gap         | The user starts each lane from the launch packet that `execute` writes                                                                                                                                                                                                                                                                       |
| Build         | The plan and the code stay in sync during the build                                                                             | Playbook       | Gap         | The workflow records divergences during execution. The automated reminder and opt-in Stop hook remain planned                                                                                                                                                                                                                                |
| Build         | Implementation starts in a fresh context after the spec                                                                         | Best practices | Aligned     | `execute` gives every implementer a fresh context                                                                                                                                                                                                                                                                                            |
| Build         | The driving session delegates code changes and does not patch them inline                                                       | Best practices | Closed here | In `auto` and `arena`, only dispatched subagents change code. Gate audits every commit and changed file against the dispatched tasks                                                                                                                                                                                                         |
| Build         | After two failed corrections, start over in a fresh context                                                                     | Best practices | Closed here | A task that fails two corrections is re-dispatched once to a fresh subagent, then stops for the user's decision. A session the user has corrected twice on one issue offers a `handoff` capsule and `/clear`, a rule in `execute`'s `pair` mode and the `scaffold` `AGENTS.md` template                                                      |
| Build         | Key context is preserved when the session compacts                                                                              | Best practices | Closed here | The `scaffold` `AGENTS.md` template has a compaction-preservation line. `scaffold` offers it and the two-corrections line to an existing `AGENTS.md`                                                                                                                                                                                         |
| Test          | A bug fix commits its failing test before the fix                                                                               | Playbook       | Closed here | For bug fixes, the red commit lands on its own, with its SHA and patch-id on a `verify red-<task-id>` or `verify red-fix` row. A small fix's pull request description also states the SHA                                                                                                                                                    |
| Test          | The implementation does not weaken its own tests                                                                                | Playbook       | Closed here | The playbook's review alternative, not a hook: gate and `review` report any change to the red commit's test files                                                                                                                                                                                                                            |
| Test          | A separate agent writes the tests                                                                                               | Playbook       | Divergence  | A separate test writer remains an open design question                                                                                                                                                                                                                                                                                       |
| Test          | Every incident leaves a regression test or eval behind                                                                          | Playbook       | Closed here | Every postmortem has a regression test or eval action item, or `n/a: <reason>`                                                                                                                                                                                                                                                               |
| Test          | Lessons are kept in a version-controlled lessons file                                                                           | Playbook       | Divergence  | Issues stay the system of record, as the playbook itself advises elsewhere                                                                                                                                                                                                                                                                   |
| Test          | The repository has an eval suite for its agent behavior                                                                         | Playbook       | Gap         | Agentics keeps skill eval banks here and runs them with the [sibling eval engine](../evals/README.md)                                                                                                                                                                                                                                        |
| Test          | Agentics gates its own changes on evals                                                                                         | Playbook       | Gap         | Skill evaluations run separately from the pull request checks. Report results for behavior changes in the pull request                                                                                                                                                                                                                       |
| Review        | Review shows a severity tally and the open findings, and a person decides                                                       | Playbook       | Closed here | Gate reports the tally in its report, never the pull request body, and asks fix, accept with a reason, or defer with an issue link for each finding that is not record drift, resolving record drift itself. Nothing halts on a finding alone                                                                                                |
| Review        | A `REVIEW.md` defines review passes, severities, and skips                                                                      | Playbook       | Closed here | `review` reads `REVIEW.md` first. `scaffold` offers an editable starter                                                                                                                                                                                                                                                                      |
| Small changes | A change small enough to describe in one sentence skips the planning artifacts                                                  | Best practices | Closed here | The small-fix path. See [Conflicts between the sources](#conflicts-between-the-sources)                                                                                                                                                                                                                                                      |
| Setup         | The instruction file is named `CLAUDE.md`                                                                                       | Both           | Divergence  | Agentics uses `AGENTS.md`, which Claude Code, Codex, and Copilot all read                                                                                                                                                                                                                                                                    |
| Setup         | The instruction file fits on one page and lists commands, conventions, and recurring mistakes                                   | Playbook       | Closed here | The `scaffold` `AGENTS.md` template has that content list and a one-page cap of 80 lines                                                                                                                                                                                                                                                     |
| Setup         | Each verification command shows its expected healthy output                                                                     | Playbook       | Closed here | The `scaffold` `AGENTS.md` template shows one line of healthy output per command. Agentics's own `AGENTS.md` is the gap below                                                                                                                                                                                                                |
| Setup         | Agentics's own `AGENTS.md` shows healthy output for its commands                                                                | Playbook       | Gap         | `AGENTS.md` links to [healthy command output](../CONTRIBUTING.md#healthy-output)                                                                                                                                                                                                                                                             |
| Setup         | Permission allowlists, secret-file deny rules, a sandbox, a post-edit formatter, and protected-path hooks                       | Best practices | Gap         | Repository owners configure tool permissions and sandboxes in their host. See [07. Hooks](guide/07-hooks.md) for managed settings                                                                                                                                                                                                            |

## Conflicts between the sources

The playbook covers the lifecycle: stages, artifacts, and gates. Best practices covers technique inside one session.
Where they disagree, the playbook wins on lifecycle structure and best practices wins on in-session technique. Each
conflict is recorded once, here.

- **Small changes.** The playbook carries a change through intent, spec, and plan. Best practices skips planning when
  the diff can be described in one sentence. Agentics keeps the playbook's structure and adds the small-fix path: a
  change that meets every small-fix signal, the first of which is the one-sentence diff, skips the three artifacts. It
  still gets verification, review, the gate, and a pull request whose description says why. The pull request reviewer is
  always a second person.
- **A separate test writer.** The playbook has a separate agent write the tests. Whether that extra role earns its cost
  remains an open design question.

## Sources and re-review

- "The AI-native SDLC playbook", Anthropic: <https://claude.com/blog/the-ai-native-sdlc-playbook>. Published Aug 21
  2026, read 2026-10-11.
- "Best practices for Claude Code", Anthropic: <https://code.claude.com/docs/en/best-practices>. Read 2026-10-11.

No automated check watches these pages. When Anthropic updates either one, re-review by hand:

1. Read the updated page in full and note the date.
2. Compare each recommendation with the [alignment table](#alignment-by-stage), row by row.
3. Add a row for each new recommendation, and change the status of any row the update affects.
4. File an issue for each new gap and name it in the row. Write an ADR when a change reverses an existing Agentics rule.
5. Update the read date above in the same pull request.
