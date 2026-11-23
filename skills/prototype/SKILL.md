---
name: prototype
description: "Use when a design question cannot be settled confidently through discussion alone. Build a small, runnable experiment to test the uncertain behavior, then record what the result supports and what remains unknown."
version: "2.1.0"
user-invocable: true
disable-model-invocation: false
argument-hint: "[logic | ui | probe]"
metadata:
    surfaces: [claude-code, copilot-cli, vscode, codex, antigravity]
    modes: [logic, ui, probe]
---

# Prototype

A prototype is **throwaway code that answers one question**. The question decides the shape, and the verdict is the only part that outlives it.

## Don't use when

Read the rows in order; the first match wins. The **Kind** column says what an explicit invocation does with the row, as [forwarding.md](../routing/references/forwarding.md#explicit-invocation) defines it: a `cannot` row forwards by that protocol, and a `better elsewhere` row is named in one line and the run continues at Step 0. When the model loaded this skill on its own, every row forwards.

| Order | Input signal                                                                                           | Use this instead                                                                                                                                                                                                                                                                                                                                           | Kind             |
| ----- | ------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------- |
| 1     | Code that must ship, be maintained or have tests, including moving a prototype's code into the product | `routing`, as an ordinary mutating change. It assigns the delivery path, with its tests and review.                                                                                                                                                                                                                                                        | cannot           |
| 2     | A UX deliverable: flows, wireframes, information architecture or mockups as documents                  | No skill: Agentics has no workflow for UX deliverables ([job owners](../routing/references/forwarding.md#job-owners)), and the session may do the work directly. Building the frontend is row 1. A layout question that runnable variants can answer is this skill's `ui` mode: offer it as one yes/no question and build nothing until the user says yes. | cannot           |
| 3     | A decision record, a written comparison or requirements                                                | `docs` for a decision record or comparison, `spec` for requirements.                                                                                                                                                                                                                                                                                       | cannot           |
| 4     | A request to pressure-test or poke holes in a plan or design                                           | `challenge`. It questions a plan; this skill builds something runnable.                                                                                                                                                                                                                                                                                    | cannot           |
| 5     | The design of code meant to survive: module boundaries, interfaces, seams                              | `deepen`, which designs for code that stays. This skill can still build competing throwaway sketches of the interface.                                                                                                                                                                                                                                     | better elsewhere |
| 6     | Otherwise                                                                                              | This skill runs, from [Step 0](#step-0-name-the-question).                                                                                                                                                                                                                                                                                                 | —                |

## Step 0: name the question

State in one sentence the question this prototype answers and the observation that would answer it. If the request leaves inputs open, such as policy parameters or sample data, state the values you assume and invite the user to change them. Then take the first row that matches:

| Order | The request                                                                                                                                                       | Action                                                                                                                                                                                                                                                                                 |
| ----- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1     | Cites a source that already answers the question: a design-system rule, a decision record, a measured result                                                      | Answer from the source: give the verdict, cite the source, and say nothing was built. The run is finished, with no throwaway to remove. A request to build it anyway is a declined step ([Gates](#gates)).                                                                             |
| 2     | Names no question and no purpose, only a thing to build ("prototype the new search")                                                                              | Ask one line: "What do you want to learn from it?" In an [unattended](../routing/references/contracts/unattended.md) run, take the question the request implies, state it as an assumption at the top of the prototype and in the report, and continue at [Pick a mode](#pick-a-mode). |
| 3     | Otherwise: an open question that a run can answer, or a build with a stated purpose ("mock the login so we can test the redirect"), whose purpose is the question | Continue at [Pick a mode](#pick-a-mode).                                                                                                                                                                                                                                               |

## Pick a mode

Take the first row that matches. Rows 2 to 4 each match only when the request fits that row and neither of the other two. Ask decision questions in the [question format](../routing/references/contracts/question-format.md).

| Order | Signal                                                                                                                                   | Mode                                                                                                                                                                                                                                                                                                                             |
| ----- | ---------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1     | The argument names a mode                                                                                                                | That mode.                                                                                                                                                                                                                                                                                                                       |
| 2     | The answer is an observation one run can print: a timing, an output shape, whether a library or approach works at all                    | `probe`: [probe.md](references/probe.md).                                                                                                                                                                                                                                                                                        |
| 3     | The question is only about how something looks or is laid out: a page, a screen, a layout, a component                                   | `ui`: [ui.md](references/ui.md).                                                                                                                                                                                                                                                                                                 |
| 4     | The question is only about rules, states, transitions or data shape that a person wants to push through cases by hand                    | `logic`: [logic.md](references/logic.md).                                                                                                                                                                                                                                                                                        |
| 5     | Otherwise: the request fits two of rows 2 to 4 or none, such as "try out the new approval flow", which can mean its rules or its screens | Ask one question. Recommend the mode that matches the surrounding code (a backend module means `logic`, a page or component means `ui`); with no code, recommend the mode the wording leans toward. In an unattended run, take the recommended mode and state it as an assumption at the top of the prototype and in the report. |

When the mode is `logic` or `probe` and the repository has no obvious runtime, ask the runtime in the same round, as [logic.md](references/logic.md#2-pick-the-language) says.

## Rules for every mode

1. **Throwaway from the start, clearly marked.** Put it next to the module or page it prototypes, in a `prototype-<name>` file or directory (for example `prototype-checkout-fsm.ts`) or a `scratch/` subdirectory. Follow the project's routing convention for a throwaway UI route, and add no new top-level structure. Before you write, say in one line which files you will create and which existing files you will edit.
2. **One command to run.** Put the command at the top of the prototype file or its README, such as `npx --no-install tsx <path>`, `python <path>` or `bun <path>`. Use only a runner that is already installed. Never use a command that fetches a missing package and runs it, such as plain `npx`, `npx -y`, `bunx` or `pnpm dlx`: it runs whatever the registry in the project's configuration serves. When no installed runner fits, ask before installing one; in an unattended run, install nothing and report the command and the missing runner. Do not edit the project's task-runner configuration for it.
3. **No persistence by default.** State lives in memory. When the question is about persistence, use a scratch database or a local file whose name says "PROTOTYPE - wipe me".
4. **Fake every identity and credential.** A prototype that touches auth uses invented users, hard-coded fake tokens, and a local stub for the identity provider. It never uses a real account, a real secret, or a live identity provider or tenant, even a test one, and never opens, reads, quotes or copies a value from an environment file or credential store. This is a safety gate ([Gates](#gates)).
5. **Skip the polish.** Add no tests and no abstractions, and no error handling beyond what makes it run.
6. **Surface the state.** After every action (`logic`), variant switch (`ui`) or run (`probe`), print or render the full relevant state.

## The throwaway line

Rules 3 to 5 shape the prototype's code: no tests, no error handling, faked identities. Moved into the product as it stands, it ships all three. What this skill hands on is the verdict. Building the answer for real is [Don't use when](#dont-use-when) row 1, and that work may start from the prototype's pure logic module or winning variant as a draft, which then gets the tests and review its delivery path requires.

Keeping prototype code out of the product is a process gate. A request to promote the throwaway as it stands, in any wording ("add tests to it", "wire it to the real database", "move it into `src/`", "ship this variant"), gets its cost named once, in one sentence about this prototype. For example: "This mock accepts its hard-coded tokens and calls a local stub instead of your identity provider." On the next reply that still asks, comply: forward the move to `routing` as an ordinary change, carrying the request and the declined gate, so the skill that carries the delivery path records one `waive prototype-code` row ([flow-state.md](../execute/references/flow-state.md#events)). Do not harden the throwaway in place under this skill.

When the request names the promotion up front ("if it looks okay, commit it as the real implementation"), name the cost in the opening line and build. When the user then confirms, forward without naming the cost again.

## Gates

Two gates in this skill are safety gates ([gates.md](../routing/references/contracts/gates.md#safety-gate)). Each holds in that file's shape: what is held, the harm, and what to do instead. A repeated demand gets a one-line pointer back to the alternative.

- **Credentials (rule 4).** Held: a real account, secret, identity provider or tenant in the prototype. Harm: unreviewed code with no error handling can log or commit the credential, and deleting the file does not un-leak it. Instead: a local stub with the same shape, or the user runs the real check outside the prototype.
- **Status claims.** Held: calling the prototype tested, validated, production-ready or done when [When done](#when-done) is not met, or a verdict that neither a run in this session nor a cited source shows. Harm: the next person builds on a result nobody saw. Instead: report what ran and what it showed, and name what is still missing.

Every other stop in this skill is a process gate ([gates.md](../routing/references/contracts/gates.md#process-gate)): the Step 0 question, the mode question and the throwaway line. When the user declines one, name its cost once in one sentence and do what they asked on the next reply that still declines. For example, "build it anyway" against Step 0 row 1 costs a prototype that can only restate its source.

## When done

A prototype is done when all three hold:

1. **The question is answered.** The user or the run gives the verdict, not just "it runs". A `ui` verdict is the user's pick.
2. **The verdict is recorded where it survives the prototype's removal.** Take the first row that matches:

    | Order | Situation                                      | Where the verdict goes                                                                                                                                                                                                                                                   |
    | ----- | ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
    | 1     | Another skill or a dispatcher invoked this run | Return the report to the caller. The [empirical questions contract](../routing/references/contracts/empirical-questions.md#record-the-result) governs how they record the result.                                                                                        |
    | 2     | The user named a place                         | That place. A comment on an issue or pull request the user named is authorized ([approval.md](../routing/references/contracts/approval.md#steps-named-up-front)), and it carries only the question, the verdict and the evidence.                                        |
    | 3     | Otherwise                                      | Ask one question. Recommend the issue or pull request the work belongs to when one exists; otherwise a decision record written by `docs` when the verdict settles a design decision; otherwise the message of the commit that removes the prototype, made through `git`. |

3. **The throwaway is gone.** Removing it discards files and edits that cannot be recovered, so list every file you created and every existing file you edited, and ask once; the reply must name the removal ([approval.md](../routing/references/contracts/approval.md#the-two-exceptions)). When the user keeps it, or in an unattended run, leave it and report its paths. The prototype is then not done.

Close with one line each:

- The question.
- The verdict in one sentence, with what remains unknown, or "open" and what would settle it.
- The evidence: what ran and what it showed.
- Where the verdict is recorded.
- The throwaway: removed, or its paths and that it remains.

## References

- [logic.md](references/logic.md): the `logic` mode, a terminal app that drives a state model by hand.
- [ui.md](references/ui.md): the `ui` mode, structurally different variants on one route behind a switcher.
- [probe.md](references/probe.md): the `probe` mode, one script that runs once and prints the observation.
- [forwarding.md](../routing/references/forwarding.md): the route-away protocol and explicit invocation.
- [gates.md](../routing/references/contracts/gates.md), [approval.md](../routing/references/contracts/approval.md), [unattended.md](../routing/references/contracts/unattended.md), [question-format.md](../routing/references/contracts/question-format.md): the shared contracts this skill applies.
