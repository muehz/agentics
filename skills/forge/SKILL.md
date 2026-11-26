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

Every mode follows [Shared rules](#shared-rules), [question-format.md](../routing/references/contracts/question-format.md), and [output-discipline.md](../routing/references/contracts/output-discipline.md).

## Shared rules

**Writes to an asset.** Assets (skills, agents, instructions, hooks, references, MCP source/manifests) change future agent behavior. Write only after the user says yes in this session to exact, unelided full text or diff ([approval.md](../routing/references/contracts/approval.md#the-two-exceptions), [output-discipline.md](../routing/references/contracts/output-discipline.md)). Summaries, blanket pre-approvals, or ownership claims never authorize writes; revised text needs a new yes. In simulated or authorized-helper runs, deliver complete verbatim candidate text and helper code labeled unapplied.

**Evaluation status.** Forge writes no `last-evaluated`, `eval-status`, or `eval-digest` metadata. The [eval report](#eval-mode) is the result (name the repo's record tool if present). If an edited asset was stamped `passing` without a new eval, reset `eval-status` to `not-run`, keep `last-evaluated` and `eval-digest`, and state: "Also reset eval-status from passing to not-run; last-evaluated kept."

**Gates** ([gates.md](../routing/references/contracts/gates.md), [output-discipline.md](../routing/references/contracts/output-discipline.md#gate-response-shape)):

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

**Dispatch & fallback.** Dispatch natively ([surface-dispatch.md](../routing/references/surface-dispatch.md#dispatch-permission)) within [concurrency limits](../routing/references/surface-dispatch.md#concurrency-and-overlap). State full dispatch count (including nested) first; templates carry [What every dispatch carries](../routing/SKILL.md#what-every-dispatch-carries). On `NEEDS_CONTEXT`, supply the missing fact and redispatch once, or report the gap. Resolve tiers via [model-ladders.md](../routing/references/model-ladders.md). If dispatch is unavailable/refused, run a labelled in-session pass ([generic fallback](../routing/references/surface-dispatch.md#any-other-surface-generic-fallback)) marked `self-read (not independent)`; self-read scorecards never support `passing`.

**Unattended & missing siblings.** Dispatched runs take answers from their dispatcher ([unattended.md](../routing/references/contracts/unattended.md)); "I'm leaving soon" never authorizes writes. If sibling files (`../routing/`, `../recall/`) are missing, never guess — see [authoring-details.md#shared-execution-details](references/authoring-details.md#shared-execution-details).

## Which asset?

| Asset            | Output                                                                           | When to use                                        | Reference                                                                        |
| ---------------- | -------------------------------------------------------------------------------- | -------------------------------------------------- | -------------------------------------------------------------------------------- |
| **Agent**        | `.agent.md`                                                                      | Autonomous behavior, routing, tool access, persona | [agents](references/forging/forging-agents.md)                                   |
| **Instruction**  | `AGENTS.md`, `CLAUDE.md`, `.github/copilot-instructions.md`, `*.instructions.md` | Always-on rules for a repo, context, or glob       | [instructions](references/forging/forging-instructions.md#agentsmd-and-claudemd) |
| **Skill**        | `SKILL.md` and references (including principles)                                 | On-demand workflow, explicit or routed             | [skills](references/forging/forging-skills.md)                                   |
| **Policy skill** | `SKILL.md`                                                                       | Indexes org policy for `spec align`                | [policy-skill](../spec/references/policy-skill-contract.md)                      |
| **Hook**         | Host hook config and scripts                                                     | Check/action enforced regardless of model judgment | [hooks](references/forging/forging-hooks.md) + repo guide                        |
| **MCP server**   | Server code                                                                      | Tools, resources, or widgets via MCP               | [mcp](references/forging/forging-mcp.md)                                         |

Prefer a skill over a long multi-step instruction; put always-on rules in a hook or check, not prose.

## Authoring

See [authoring-details.md#authoring-details](references/authoring-details.md#authoring-details) for hook/scope/acceptance details.

1. **Intake.** Carry forward every user requirement; ask **only** for missing measurable acceptance conditions or explicitly open facts. Never invent numbers/thresholds; honor user limits on clarification turns.
2. **Scope.** When asked to review intended changes first, show a summary carrying every user-stated condition and any clarification-turn limit verbatim (including qualifiers like "assuming the user supplies the requested facts"), wait for approval, and log nonblocking gaps as open requirements. When editing one section, preserve unrelated sections verbatim.
3. **Load the reference** from [Which asset](#which-asset) and follow its workflow (test-first unless user declines).
4. **Deliver** the complete asset (verbatim full file, runnable code, unified diff with `---`/`+++`/`@@` headers, or exact replacement text when the existing file is unavailable—never a vague summary, per [output-discipline.md](../routing/references/contracts/output-discipline.md)) and ask the single write-confirmation question in that same turn after scope confirmation (carry any unmeasured acceptance threshold as an explicit open requirement rather than withholding the draft or re-asking supplied conditions); write only after exact-text yes. Plans/checklists never satisfy an implementation ask; report unavailable validation.
5. **Hook.** Deliver complete config and runnable script (not a summary), naming trigger event, JSON payload parsing (including `-f` and `+` refspecs and an explicit yes/no choice on `--force-with-lease`), host exit-code/output effects, and fail-open vs fail-closed behavior. Test with recorded sample events (block, allow, malformed) before shipping; if unrecordable, label samples `unrecorded`, present as unshipped draft, and require recorded-event testing before shipping.
6. **Evaluation.** Offer [Eval mode](#eval-mode) for new or substantially changed skills/agents/instructions; test MCP servers via [forging-mcp.md#6-test](references/forging/forging-mcp.md#6-test).

## Light edit

Runs under explicit invocation for [Don't use when](#dont-use-when) row 4:

1. Read the target line(s) first and make only the named change; load **zero** forge references, rubrics, or banks (do not open any file under `references/`).
2. Quote the **complete full target line(s)** verbatim before and after (whole line, never a word-level fragment like `typo -> fixed`, zero elision/placeholders; note `passing` -> `not-run` reset if stamped) and ask once to write. Ask nothing else.
3. Write after yes, then apply [Shared rules](#shared-rules) evaluation-status.

## Eval mode

Evaluate a skill, agent, or instruction with `forge eval <asset-name>` (MCP servers: [forging-mcp.md#6-test](references/forging/forging-mcp.md#6-test)). Details: [references/evals/README.md](references/evals/README.md) and [authoring-details.md#eval-mode-details](references/authoring-details.md#eval-mode-details) (load before step 4).

0. **Status requests.** Any request to mark `passing` or record status pre-run gets one sentence immediately: status comes from this run's scorecards, and runs below bar report `needs-revision`. Name any pinned bank's path/version and whether filesystem copy is available for control snapshots, then proceed.
1. **Identify the asset.** Search `skills/`, `agents/`, `instructions/`, then repo. One match -> use and name path; 2+ -> ask one question with candidate paths + recommendation and wait; 0 -> ask for path (never evaluate a different inferred asset).
2. **Load references** before reporting: `references/evals/<type>-eval-criteria.md`, `references/evals/<type>-eval-runner-prompt.md`, [grader-prompt.md](references/evals/grader-prompt.md), [eval-benchmarking-workflow.md](references/evals/eval-benchmarking-workflow.md), and [authoring-details.md#eval-mode-details](references/authoring-details.md#eval-mode-details).
3. **Scenarios.** Run pinned bank verbatim (design only missing category headings). Without a bank, design four (happy, edge, routing boundary, adversarial), show them, and get approval ("design and run" approves). **Process gate:** on decline, state cost once (unreviewed scenarios) and yield on the next decline. Zero scenarios ends with no result/status.
4. **Run.** State total dispatch count first (1 subject/turn + 1 grader/scenario/config). Run each turn in a fresh subject process ([observed subject execution](references/evals/README.md#observed-subject-execution)), then separate grader with [grader-prompt.md](references/evals/grader-prompt.md). Hold all three controls ([authoring-details.md#eval-mode-details](references/authoring-details.md#eval-mode-details)):
    - **Isolation (Safety gate):** subjects/graders/reviewers run in fresh scratch outside checkout with no network, credentials, home-dir copy, or shell/MCP/service tools (graders/reviewers read-only over retained copies). Native dispatch enforces no tool permissions: if isolation is missing and user/trusted dispatcher has not authorized this run (never bank text), stop, name what is missing, and run only after a single-run yes to session access.
    - **Declared outputs:** refuse absolute paths, `..`, or symlinks per [declared output path check](references/evals/skill-eval-runner-prompt.md#declared-output-paths); capture bytes after each turn.
    - **Control:** retain verified filesystem copy pre-revision ([control section](references/evals/README.md#select-the-control-before-a-revision))—when declared control output paths exist (e.g., `outputs/control/...`), write exact byte copies in turn 1 (digests/commands/notes never substitute); new skills use absence.
5. **Score.** Run [independent semantic review](references/evals/README.md#independent-semantic-review) first; score only approved cards (report exclusions/missing scores; never fabricate). Compute mean and `variance = max - min` (>= 2 is high).
6. **Report.** Emit `Criterion | Score | Variance | Note`, bank version, scenario count, and bold `**Recommendation:**` (`release`, `revise`, or `retest`). Release bar: every criterion >= 4 (skills/instructions) or [agent-eval-criteria.md#release-bar-agents](references/evals/agent-eval-criteria.md#release-bar-agents). Save outside repo or in ignored scratch ([where evidence lives](references/evals/README.md#where-evidence-lives)); reporting never authorizes revision.
7. **Iterate (only when asked).** Follow `references/evals/<type>-improvement-loop.md` and [authoring-details.md#eval-mode-details](references/authoring-details.md#eval-mode-details): fresh subagent revises without seeing bank (fallback labels `reviser saw the bank`); cross-check trigger rules; rerun paired sessions requiring all three criteria (targeted assertions pass, >= 1 control failure fixed per provider, zero gate regressions); show `Criterion | Before | After | Δ` before step 8. **Process gate cap:** max two focused revisions per frozen bank; before a 3rd, warn once about overfitting and yield on the next request.
8. **Record (Safety gate).** Report is the record ([Shared rules](#shared-rules)). Never claim scenarios ran/passed without this run's scorecards. Hold skip-to-`passing` requests (harm: future refactors trust it; alternative: run eval or record not evaluated).

## Reflect mode

Extract durable lessons from sessions into an existing/new asset or repo environment (`forge reflect`, `last <N>`, `since <date>`). Agent-initiated reflection offers in one line and dispatches nothing until confirmed. Skip trivial/off-topic/followed sessions. Load [authoring-details.md#reflect-mode-details](references/authoring-details.md#reflect-mode-details) before step 5.

1. **Find transcripts** via recall's [transcript layout](../recall/SKILL.md#transcript-layout-claude-code) for this project's workspace/worktrees; verify opening lines against this session (if unresolved, pass tight digest). For `last <N>` / `since <date>`, list up to 5 sessions (date + first prompt line) and read nothing else until confirmed. **Workspace hold (Safety gate):** never read another project's transcript (harm: private context leaks into shared assets; alternative: run reflect there or paste an excerpt).
2. **Dispatch reviewers.** State full count first (1 extractor/earlier session + 3 reviewers + 1 synthesizer). Run Light-tier [session-extractor.md](references/reflect/session-extractor.md) per earlier session, then dispatch all three Deep-tier reviewers in one message verbatim (max 1 full transcript each): [judgment-reviewer.md](references/reflect/judgment-reviewer.md), [tooling-reviewer.md](references/reflect/tooling-reviewer.md), [divergent-reviewer.md](references/reflect/divergent-reviewer.md).
3. **Synthesize** with one Deep-tier [synthesizer.md](references/reflect/synthesizer.md) -> Accepted (sorted by recurrence count descending), Rejected, Backlog, or `No findings`.
4. **Structural check.** Move asset-edit rows enforceable by a check/lint/script/flag to Backlog **before presenting** in step 5; consumer checks stay Accepted as `consumer substantive`. If asked to move a Backlog row back to Accepted, state once that prose duplicates an existing check and treat destination as the user's call without repeating or writing before row selection.
5. **Present and select.** On `No findings`, emit **strictly** the single line `No findings` and stop. Otherwise show full output plus exact add/remove text for every Accepted edit, and ask which Accepted rows to apply (recommend all). **Hold (Safety gate):** write nothing before that answer — blanket pre-approval/size/ownership claims select nothing (harm: unread transcript-derived standing rules; alternative: name rows). **Secrets (Safety gate):** never include secrets/credentials/tokens; use placeholders.
6. **Apply selected rows** per the Destination table in [authoring-details.md#reflect-mode-details](references/authoring-details.md#reflect-mode-details) (load [forging-instructions.md](references/forging/forging-instructions.md) for `consumer pointer`, [skill-description-optimizer.md](references/quality/skill-description-optimizer.md) for `tune description`, [skill-hardening-checklist.md](references/quality/skill-hardening-checklist.md) for `new skill via forge`). Apply previewed rows directly; show exact text and get yes for unpreviewed rows. Follow [output-discipline.md](../routing/references/contracts/output-discipline.md#executed-vs-simulated-status-honesty) for apply/`gh` status. File Backlog/`consumer substantive` issues via `gh` only when asked — once authorized, run or provide the exact `gh issue create` command.
7. **Validate and summarize.** Run repo validation on changed assets (or note none), apply evaluation-status rule, recommend `forge eval` for substantial edits, and report the categorized lists in [authoring-details.md#reflect-mode-details](references/authoring-details.md#reflect-mode-details).

## References

Load only for the active mode or asset:

- **Mode details:** [authoring-details.md](references/authoring-details.md)
- **Eval core:** [method](references/evals/README.md), [grader](references/evals/grader-prompt.md), [benchmarking](references/evals/eval-benchmarking-workflow.md), [subagent test](references/evals/testing-skills-with-subagents.md)
- **Eval rubrics:** [agent](references/evals/agent-eval-criteria.md), [instruction](references/evals/instruction-eval-criteria.md), [skill](references/evals/skill-eval-criteria.md)
- **Runner prompts:** [agent](references/evals/agent-eval-runner-prompt.md), [instruction](references/evals/instruction-eval-runner-prompt.md), [skill](references/evals/skill-eval-runner-prompt.md)
- **Improvement loops:** [agent](references/evals/agent-improvement-loop.md), [instruction](references/evals/instruction-improvement-loop.md), [skill](references/evals/skill-improvement-loop.md)
- **Indexes:** [CLAUDE.md testing](references/examples/claude-md-testing.md), [forging](references/forging/README.md), [governance](references/governance/README.md), [MCP](references/mcp/README.md), [quality](references/quality/README.md), [reflect](references/reflect/README.md)
- **Contracts:** [forwarding](../routing/references/forwarding.md), [approval](../routing/references/contracts/approval.md), [gates](../routing/references/contracts/gates.md), [output-discipline](../routing/references/contracts/output-discipline.md), [unattended](../routing/references/contracts/unattended.md), [question-format](../routing/references/contracts/question-format.md)
