---
name: forge
description: "Use when creating or improving agent skills, custom agents, instruction files, hooks, or MCP servers, or investigating whether they actually guide agents well. Turn the intended behavior into clear instructions and evaluate whether those instructions work."
version: "2.1.0"
user-invocable: true
disable-model-invocation: false
argument-hint: "agent | skill | instructions | hook | mcp | eval <asset-name> | reflect [last <N> | since <date>]"
metadata:
    surfaces: [claude-code, copilot-cli, vscode, codex, antigravity]
    modes: [agent, skill, instructions, hook, mcp, eval, reflect]
---

# Forge

Author, improve, and evaluate agents, skills, instruction files, hooks, and MCP servers in any repository.

## Don't use when

Read in order; first match wins. Kinds follow [forwarding.md](../routing/references/forwarding.md#explicit-invocation):
`cannot` names the owner in one line, carries the request, and asks one yes/no before starting; `better elsewhere` under
explicit invocation names the better route in one line, then runs forge.

| Order | Input signal                                                                     | Use this instead                                                                       | Kind             |
| ----- | -------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------- | ---------------- |
| 1     | Repo governance files: README, CONTRIBUTING, LICENSE, SECURITY, templates        | `scaffold init` (no forge mode writes these)                                           | cannot           |
| 2     | Docs no agent loads: guides, README sections, runbooks, API reference            | `docs` (checks text against implementation)                                            | cannot           |
| 3     | First instruction file (`AGENTS.md`, etc.) in a repo with none                   | `scaffold init` (scans repo); explicit invocation runs instructions mode               | better elsewhere |
| 4     | Wording, typo, link, or one-line fix to an existing skill/agent/instruction/hook | Edit directly; explicit invocation runs [Light edit](#light-edit)                      | better elsewhere |
| 5     | One scenario to try on an asset, no scenario design or scorecard wanted          | One test subagent ([subagent test](references/evals/testing-skills-with-subagents.md)) | better elsewhere |
| 6     | Otherwise                                                                        | This skill runs ([Modes](#modes) selects the mode)                                     | —                |

## Modes

Read in order; first match wins.

| Order | The request                                                                       | Mode                                                      |
| ----- | --------------------------------------------------------------------------------- | --------------------------------------------------------- |
| 1     | Names a mode (`agent`, `skill`, `instructions`, `hook`, `mcp`, `eval`, `reflect`) | That mode                                                 |
| 2     | Correction described in [Don't use when](#dont-use-when) row 4                    | [Light edit](#light-edit)                                 |
| 3     | Evaluate, score, or benchmark an existing skill, agent, or instruction file       | [Eval mode](#eval-mode)                                   |
| 4     | Find lessons in this session or earlier sessions                                  | [Reflect mode](#reflect-mode)                             |
| 5     | Author or improve an asset whose type [Which asset](#which-asset) settles         | That asset's mode under [Authoring](#authoring)           |
| 6     | Otherwise: asset type is open                                                     | Ask one question naming candidates + recommendation; wait |

Every mode follows [Shared rules](#shared-rules) and [question-format.md](../routing/references/contracts/question-format.md).

## Shared rules

**Writes to an asset.** Assets (skills, agents, instructions, hooks, skill references, MCP source/manifests) change
future agent behavior. Write only after the user says yes in this session to the exact text shown in full or as a diff
([approval.md](../routing/references/contracts/approval.md#the-two-exceptions)). Summaries, blanket pre-approvals, or
ownership claims never authorize a write; revised text needs a new yes. In simulated execution, return the complete
candidate labeled unapplied.

**Evaluation status.** Forge writes no `last-evaluated`, `eval-status`, or `eval-digest` metadata. The
[eval report](#eval-mode) is the result (name the repo's own record tool if it has one). If the repo marks an asset
`passing` and forge edits it without a new eval, reset `eval-status` to `not-run`, keep `last-evaluated` and
`eval-digest` as history, and state: "Also reset eval-status from passing to not-run; last-evaluated kept."

**Gates** ([gates.md](../routing/references/contracts/gates.md)):

| Gate                                                                                           | Class        | Where                                     |
| ---------------------------------------------------------------------------------------------- | ------------ | ----------------------------------------- |
| User's yes to exact text before any asset write                                                | Safety gate  | Above                                     |
| No `passing` recommendation or eval-ran claim without this run's scorecards                    | Safety gate  | [Eval mode](#eval-mode) step 0 and step 8 |
| No secret, credential, or token in a lesson or edit (use placeholders)                         | Safety gate  | [Reflect mode](#reflect-mode) step 5      |
| Reflect reads transcripts only from this project's workspace                                   | Safety gate  | [Reflect mode](#reflect-mode) step 1      |
| Eval subjects, graders, and reviewers run isolated, or after single-run yes for session access | Safety gate  | [Eval mode](#eval-mode) step 4            |
| Approval of self-designed eval scenarios                                                       | Process gate | [Eval mode](#eval-mode) step 3            |
| Separate subject runs and grading; fresh process per subject turn                              | Process gate | [Eval mode](#eval-mode) step 4            |
| Cap of two focused revisions per frozen bank                                                   | Process gate | [Eval mode](#eval-mode) step 7            |

Safety gates state once what is held, harm prevented, and the alternative (repeated demand -> one-line pointer back).
Process gates name cost once and yield on the next reply that still declines.

**Dispatch & fallback.** Dispatch via the session's native path
([surface-dispatch.md](../routing/references/surface-dispatch.md#dispatch-permission)) under its
[concurrency limit](../routing/references/surface-dispatch.md#concurrency-and-overlap). State full dispatch count
(including nested) first; templates carry [What every dispatch carries](../routing/SKILL.md#what-every-dispatch-carries).
On `NEEDS_CONTEXT`, supply the missing fact and redispatch once, or report the gap. Tiers come from
[model-ladders.md](../routing/references/model-ladders.md). If dispatch is refused/unavailable, run a labelled
in-session pass ([generic fallback](../routing/references/surface-dispatch.md#any-other-surface-generic-fallback))
marked `self-read (not independent)`; a self-read scorecard never supports `passing`.

**Unattended & missing siblings.** Dispatched runs get answers from their dispatcher
([unattended.md](../routing/references/contracts/unattended.md)); never treat "I'm leaving soon" as write approval. If
linked sibling files (`../routing/`, `../recall/`) are missing, never guess them — follow
[authoring-details.md#shared-execution-details](references/authoring-details.md#shared-execution-details).

## Which asset?

| Asset            | File or output                                                                   | When to use                                               | Reference                                                                        |
| ---------------- | -------------------------------------------------------------------------------- | --------------------------------------------------------- | -------------------------------------------------------------------------------- |
| **Agent**        | `.agent.md`                                                                      | Autonomous behavior, routing, tool access, persona        | [agents](references/forging/forging-agents.md)                                   |
| **Instruction**  | `AGENTS.md`, `CLAUDE.md`, `.github/copilot-instructions.md`, `*.instructions.md` | Always-on rules for a repo, context, or glob              | [instructions](references/forging/forging-instructions.md#agentsmd-and-claudemd) |
| **Skill**        | `SKILL.md` and reference files (including principle files)                       | On-demand workflow, triggered explicitly or by routing    | [skills](references/forging/forging-skills.md)                                   |
| **Policy skill** | `SKILL.md`                                                                       | Indexes org policy so `spec align` audits work against it | [policy-skill](../spec/references/policy-skill-contract.md)                      |
| **Hook**         | Host hook config and scripts                                                     | Check/action that must run regardless of model judgment   | [hooks](references/forging/forging-hooks.md) + repo guide                        |
| **MCP server**   | Server code                                                                      | Tools, resources, or widgets exposed via MCP              | [mcp](references/forging/forging-mcp.md)                                         |

Between a long multi-step instruction and a skill, prefer a skill. Rules that must hold regardless of model choice
belong in a hook or check, not prose.

## Authoring

See [authoring-details.md#authoring-details](references/authoring-details.md#authoring-details) when authoring hooks or
clarifying scope/acceptance rules.

1. **Intake.** Carry forward user requirements; ask only for missing measurable acceptance conditions. Never invent
   numbers or thresholds; honor any user limit on clarification turns.
2. **Scope.** If the user asks to review intended changes first, show a summary, wait for approval, and log nonblocking
   gaps as open requirements.
3. **Load the reference** for the asset type from [Which asset](#which-asset) and follow its workflow (test-first by
   default unless the user chooses otherwise).
4. **Deliver** the complete asset (full text or diff) and write permitted files only after the exact-text yes in
   [Shared rules](#shared-rules). Plans/checklists do not satisfy an implementation request; report when validation is
   unavailable.
5. **Hook.** State trigger event, read input, host exit-code/output effects, and fail-open vs fail-closed behavior. Test
   with recorded sample events (block, allow, malformed) before shipping; if none can be recorded, label samples
   `unrecorded`, show as an unshipped draft, and list recorded-event testing as required before shipping.
6. **Evaluation.** Offer [Eval mode](#eval-mode) for new or substantially changed skills, agents, or instructions; test
   MCP servers via [forging-mcp.md#6-test](references/forging/forging-mcp.md#6-test).

## Light edit

Runs under explicit invocation for [Don't use when](#dont-use-when) row 4 corrections:

1. Make only the named change; load no forge reference.
2. Show exact before/after text (noting `passing` -> `not-run` reset if stamped) and ask once to write. Ask nothing else.
3. Write after yes, then apply the evaluation-status rule in [Shared rules](#shared-rules).

## Eval mode

Evaluate an existing skill, agent, or instruction file with `forge eval <asset-name>` (MCP servers use
[forging-mcp.md#6-test](references/forging/forging-mcp.md#6-test)). Method details live in
[references/evals/README.md](references/evals/README.md) and
[authoring-details.md#eval-mode-details](references/authoring-details.md#eval-mode-details); load the latter before step 4.

0. **Status requests.** Any request to mark `passing` or record status before running gets one sentence at once: status
   comes from this run's scorecards, and runs below the bar report `needs-revision`. Then proceed.
1. **Identify the asset.** Search `skills/`, `agents/`, `instructions/`, then the whole repo. One match -> use and name
   its path; 2+ matches -> ask one question listing candidate paths with a recommendation and wait; 0 matches -> ask for
   the path (never evaluate an inferred different asset).
2. **Load the references:** `references/evals/<type>-eval-criteria.md`, `references/evals/<type>-eval-runner-prompt.md`,
   [grader-prompt.md](references/evals/grader-prompt.md), [eval-benchmarking-workflow.md](references/evals/eval-benchmarking-workflow.md),
   and [authoring-details.md#eval-mode-details](references/authoring-details.md#eval-mode-details).
3. **Scenarios.** Run a repo's pinned bank verbatim (design only missing category headings). With no bank, design four
   (happy path, edge case, routing boundary, adversarial), show them, and get approval before running ("design and run"
   counts as approval). **Process gate:** on decline, state cost once (unreviewed scenarios) and proceed on the next
   decline. Zero scenarios ends the eval with no result and no status.
4. **Run.** State total dispatch count first (1 subject run per user turn + 1 grader per scenario/config). Run each turn
   in a fresh subject process ([observed subject execution](references/evals/README.md#observed-subject-execution)), then
   a separate grader with [grader-prompt.md](references/evals/grader-prompt.md). Hold all three controls
   ([authoring-details.md#eval-mode-details](references/authoring-details.md#eval-mode-details)):
    - **Isolation (Safety gate):** subjects, graders, and semantic reviewers run in a fresh scratch workspace outside
      the checkout with no network, credentials, home-dir copy, or shell/MCP/service tools (graders/reviewers read-only
      over retained copies). Because native dispatch does not enforce tool permissions, if the host cannot isolate, stop
      before dispatch, name what is missing, and run only after a single-run yes to session access.
    - **Declared outputs:** refuse any absolute path, `..` segment, or symlink per
      [declared output path check](references/evals/skill-eval-runner-prompt.md#declared-output-paths); capture bytes
      after each turn for the grader.
    - **Control:** retain a verified filesystem copy before any revision
      ([control section](references/evals/README.md#select-the-control-before-a-revision)); new skills use absence.
5. **Score.** Run [independent semantic review](references/evals/README.md#independent-semantic-review) first; score
   only approved cards (report excluded cards and missing scores; never fabricate numbers). Compute mean and
   `variance = max - min` (>= 2 is high variance).
6. **Report.** Emit `Criterion | Score | Variance | Note`, bank version, scenario count, and bold `**Recommendation:**`
   (`release`, `revise`, or `retest`). Release bar: every criterion >= 4 (skills/instructions) or
   [agent-eval-criteria.md#release-bar-agents](references/evals/agent-eval-criteria.md#release-bar-agents). Save full
   report outside repo or in ignored scratch ([where evidence lives](references/evals/README.md#where-evidence-lives));
   reporting never authorizes a revision.
7. **Iterate (only when asked).** Follow `references/evals/<type>-improvement-loop.md` and
   [authoring-details.md#eval-mode-details](references/authoring-details.md#eval-mode-details): fresh subagent revises
   without seeing the bank (fallback labels `reviser saw the bank`); cross-check added/changed skill-trigger rules;
   rerun fresh paired sessions requiring all three improvement criteria (targeted assertions pass, >= 1 control failure
   fixed per provider, zero gate regressions); show the `Criterion | Before | After | Δ` delta table before step 8.
   **Process gate cap:** at most two focused revisions per frozen bank; before a 3rd, warn once about overfitting to the
   bank and proceed only on the next request.
8. **Record (Safety gate).** The report is the record; follow [Shared rules](#shared-rules). Never claim scenarios ran,
   evaluated, or passed without this run's scorecards. Hold any skip-to-`passing` request (harm: future refactors trust
   it; alternative: run the eval or record as not evaluated).

## Reflect mode

Extract durable lessons from sessions and route each accepted lesson to an existing asset, a new asset, or the repo's
environment (`forge reflect`, `last <N>`, `since <date>`). Agent-initiated reflection offers in one line and dispatches
nothing until confirmed. Skip trivial, off-topic, or already-followed sessions. Load
[authoring-details.md#reflect-mode-details](references/authoring-details.md#reflect-mode-details) before step 5.

1. **Find transcripts** using recall's [transcript layout](../recall/SKILL.md#transcript-layout-claude-code) for this
   project's workspace and worktrees; verify opening lines against this session (if unresolved, pass a tight digest).
   For `last <N>` / `since <date>`, list up to 5 sessions (date + first prompt line) and read nothing else until
   confirmed. **Workspace hold (Safety gate):** never read another project's transcript (harm: private context leaks
   into shared assets; alternative: run reflect in that workspace or paste an excerpt).
2. **Dispatch reviewers.** State full count first (1 extractor per earlier session + 3 reviewers + 1 synthesizer). Run
   Light-tier [session-extractor.md](references/reflect/session-extractor.md) per earlier session, then dispatch all
   three Deep-tier reviewers in one message verbatim (max 1 full transcript per Deep reviewer):
   [judgment-reviewer.md](references/reflect/judgment-reviewer.md),
   [tooling-reviewer.md](references/reflect/tooling-reviewer.md), and
   [divergent-reviewer.md](references/reflect/divergent-reviewer.md).
3. **Synthesize** with one Deep-tier [synthesizer.md](references/reflect/synthesizer.md) -> Accepted, Rejected, Backlog,
   or `No findings`.
4. **Structural check.** Move asset-edit rows enforceable by a check/lint/script/flag to Backlog; consumer checks stay
   Accepted as `consumer substantive`.
5. **Present and select.** On `No findings`, say so in one line and stop. Otherwise show full output plus exact
   add/remove text for every Accepted asset/instruction edit, and ask which Accepted rows to apply (recommend all).
   **Hold (Safety gate):** write nothing before that answer — blanket pre-approval, size arguments, or ownership claims
   select nothing (harm: unread transcript-derived standing rules; alternative: name the rows). **Secrets (Safety gate):**
   never include secrets, credentials, or tokens; use placeholders.
6. **Apply selected rows** per the Destination table in
   [authoring-details.md#reflect-mode-details](references/authoring-details.md#reflect-mode-details) (using
   [skill-description-optimizer.md](references/quality/skill-description-optimizer.md) for `tune description` and
   [skill-hardening-checklist.md](references/quality/skill-hardening-checklist.md) for `new skill via forge`). Show
   exact text and get yes for any selected row not previewed in step 5; file Backlog/`consumer substantive` issues via
   `gh` only if asked.
7. **Validate and summarize.** Run repo validation on changed assets (or note none exists), apply the evaluation-status
   rule, recommend `forge eval` for substantial edits, and report the categorized lists in
   [authoring-details.md#reflect-mode-details](references/authoring-details.md#reflect-mode-details).

## References

Load each reference only when its mode or asset type applies:

- **Mode details:** [authoring-details.md](references/authoring-details.md) (sibling-missing fallbacks, hook/intake
  details, eval isolation/scoring/report/delta-table specs, reflect session-listing/destination/summary tables).
- **Eval core:** [method](references/evals/README.md), [grader prompt](references/evals/grader-prompt.md),
  [benchmarking workflow](references/evals/eval-benchmarking-workflow.md),
  [single-scenario test](references/evals/testing-skills-with-subagents.md).
- **Eval rubrics (0–5 & release bars):** [agent](references/evals/agent-eval-criteria.md),
  [instruction](references/evals/instruction-eval-criteria.md), [skill](references/evals/skill-eval-criteria.md).
- **Eval runner prompts:** [agent](references/evals/agent-eval-runner-prompt.md),
  [instruction](references/evals/instruction-eval-runner-prompt.md),
  [skill](references/evals/skill-eval-runner-prompt.md).
- **Eval improvement loops:** [agent](references/evals/agent-improvement-loop.md),
  [instruction](references/evals/instruction-improvement-loop.md), [skill](references/evals/skill-improvement-loop.md).
- **Examples & area indexes:** [CLAUDE.md testing](references/examples/claude-md-testing.md),
  [forging](references/forging/README.md), [governance](references/governance/README.md), [MCP](references/mcp/README.md),
  [quality](references/quality/README.md), [reflect](references/reflect/README.md).
- **Shared contracts:** [forwarding](../routing/references/forwarding.md),
  [approval](../routing/references/contracts/approval.md), [gates](../routing/references/contracts/gates.md),
  [unattended](../routing/references/contracts/unattended.md),
  [question-format](../routing/references/contracts/question-format.md).
