# 02. Routing

Your session's own assistant is the entrypoint. The host surfaces every installed skill's description and your assistant
loads the one that owns the request, and the [`routing`](../../skills/routing/SKILL.md) skill carries the decision tree
for lifecycle phases and multi-skill requests. Your job is to give your session a routable request. This page shows how.

Skill commands here use the `/agentics:<skill>` form that Claude Code, Copilot CLI, and VS Code Chat accept. On Codex,
type `$agentics:<skill>` instead.

## Show where the work stands

Include the issue or work folder and say what you want to do next. Your session reads the work folder's `state.tsv`
record, following `routing`, before choosing the lifecycle owner. The `flow` helper reads that record, and the
[routing reference](../../skills/routing/references/chain-phases.md) says how. The phase to run is the first word of the
current row's `next`, so an intent awaiting your approval stays in intent, an approved intent goes to `spec`, and a plan
still awaiting approval goes back to `decompose`. An approval whose artifact changed since you approved it reads as
stale and stops the run until you approve again. This includes the intent's approval. When the helper does not run, the
session says so, reads the work folder by hand, and applies the same rule.

[`intent`](../../skills/intent/SKILL.md) captures and challenges the problem, outcome, and scope, then asks you to
approve it. [`spec`](../../skills/spec/SKILL.md) defines the solution, sized to what the work needs. Every spec gets an
independent review before you are asked to approve it. [`decompose`](../../skills/decompose/SKILL.md) turns the spec
into ordered, testable tasks and obtains execution approval. On routine work one question approves the spec and the plan
together. [`execute`](../../skills/execute/SKILL.md) implements an approved plan and verifies each task. After
implementation is complete, [`gate`](../../skills/gate/SKILL.md) verifies closure and records the result. The
[Agentics AI SDLC](../AISDLC.md) describes the risk tiers that decide how many approval questions you see.

You do not need a separate command to enter the chain. Ask to start or continue work ("let's work on #123", "implement
this issue") and [`routing`](../../skills/routing/SKILL.md) reads the issue, checks for an existing branch, pull
request, or work folder, and enters the phase that owns the work now. An approved plan runs in `execute` without a
second approval question; push, pull request creation, and merge still ask. To choose a phase yourself, invoke it
directly, for example `/agentics:intent` or `/agentics:spec`.

Use these prompts for five common starting points:

```text
Implement the issue displayed above. It adds a CSV export endpoint, and no intent or approved plan covers it yet.
```

Routing's continuation check finds no existing work, so the chain starts at `intent`. The issue describes a requested
behavior change; displaying it does not establish that planning has happened. The
[chain-entry rule](../../skills/intent/SKILL.md#when-the-chain-is-required) owns when that process applies.

```text
Write the spec for the intent I approved in this work folder.
```

This goes to `spec` after the session reads the intent and finds your current approval of it in the work folder's
`state.tsv`. Every intent leads to a spec, so a concrete intent gets a short one rather than none. The spec's
independent review runs before you are asked to approve it. Writing the spec does not approve it.

```text
Page 2 of the export list repeats the last row of page 1. Fix it.
```

A failure report is not a chain entry, so this goes to `diagnose` first, which finds the root cause before anything
changes. Only once the cause is known does the fix take the small-fix path, and only when every small-fix signal holds:
the diff fits in one sentence, it addresses one concern, it changes no public interface, contract, trust boundary, or
data handling, a test covers the touched code, and the diff stays within the size limits. Otherwise the fix enters the
chain. The session announces the classification and each signal before any change. `execute fix` commits the failing
test that reproduces the bug on its own before the fix. `execute` implements it with no work folder, and gate checks it
before the pull request. Ask for the full chain at any time to move the work into `intent`.

```text
Continue implementing the approved plan in this work folder. Read its task list and resume the remaining work.
```

This goes to `execute` after the session reads the plan and confirms its approval and ordered tasks. Existing approval
carries forward for that scope. A file named `plan.md` alone does not establish approval or completed implementation.

```text
Explain the issue displayed above: what problem does it describe, and what would change for users?
```

This asks for an explanation, so the session answers through the relevant domain route without starting the
implementation lifecycle. Naming an issue does not turn a question into a request to build it.

## Describe the outcome, not the tool

```text
Checkout requests started timing out after yesterday's deploy. Find the root cause before proposing any fix.
```

Why this phrasing works: the symptom ("timing out after yesterday's deploy") and the demanded outcome ("root cause
before any fix") are the routing signals. They match the [`diagnose`](../../skills/diagnose/SKILL.md) skill's trigger
exactly, so no tool name is needed. Routing goes by the deliverable phrase, the thing you ask to be produced, not by
incidental nouns in the sentence. Put the outcome in the main clause.

## Name a skill when you already know the workflow

```text
Review this change interactively. I want the checklist and a review verdict.
```

```text
Use TDD to add rate limiting to the login endpoint.
```

Why this phrasing works: skills own the bounded, well-defined workflows: review, commits, GitHub operations. Naming one
makes the route explicit. The second example names `tdd`, but the change is not a task in an approved plan, so routing
starts at `intent`; once the plan is approved, `execute` applies `tdd` to each task. Slash syntax and plain phrasing are
equivalent for model-invocable skills (see the [guide conventions](README.md)).

## Compound requests are fine, if you say the dependency

```text
Add the export endpoint, and once it exists, wire the dashboard button to it.
```

Why this phrasing works: "once it exists" tells routing the second half consumes the first half's output, so the work
runs sequentially instead of as two independent streams. Independent halves ("also, separately, ...") run in parallel.
You never manage the dispatch. You only make the dependency explicit.

**Pitfall:** a skill-sounding noun in your sentence is not always the route. "The deploy runbook says the flag exists,
but staging still fails; find the root cause" routes to `diagnose` because the requested output is a root cause. "Author
a deploy runbook" routes to the `ops` skill because the runbook is the output. If Agentics routes somewhere you did not
expect, state the outcome more precisely.

If the work touches code you don't know yet, start with the [Understand](side-understand.md) side page. You can return
to it from any phase.

Next: [03. Plan](03-plan.md)
