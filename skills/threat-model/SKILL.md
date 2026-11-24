---
name: threat-model
description: "Use when designing a feature or system that needs its security risks understood before implementation. Identify trust boundaries, plausible abuse paths, and mitigations that can be built into the design."
version: "5.2.0"
user-invocable: true
disable-model-invocation: false
argument-hint: "[design doc, RFC, ADR, diagram, or a prose description of the system] [--composed]"
metadata:
    surfaces: [claude-code, copilot-cli, vscode, codex, antigravity]
    flags: [--composed]
allowed-tools:
    - Read
    - Grep
    - Glob
    - Write
    - Bash(gh repo view:*)
    - Bash(git rev-parse:*)
    - Bash(realpath:*)
    - Bash(ls -ld:*)
---

# Threat model

This skill builds a STRIDE threat model of a design: a design doc, RFC, ADR, diagram, API specification, or a prose
description of a system. It reads what describes the system, never the source code that implements it, so it can say
what could go wrong with the design and never whether the code is safe.

Skill commands here use the `/agentics:<skill>` form that Claude Code, Copilot CLI, and VS Code Chat accept. On Codex,
type `$agentics:<skill>` instead.

## Don't use when

Read the rows in order; the first match wins. Each row is marked with its kind, as
[forwarding.md](../routing/references/forwarding.md#explicit-invocation) defines it. A `cannot` row forwards by that
protocol, which decides whether the forward is announced or asked first — class A targets (such as
`/agentics:review axes <ref> --security` and `routing`) are announced and forwarded without asking confirmation, while
class B targets (such as `/agentics:triage pr <N>`) follow `forwarding.md`'s ask-first rule. On pushback against a
code-reading forward, say in one line that this skill reads no code so a model here would not examine the branch, offer
to model the design instead, and do not repeat the forward. A `safety stop` row is not a forward:
[forwarding.md](../routing/references/forwarding.md#explicit-invocation) does not carry it, so the skill hands off at
once and asks nothing. No partial threat model goes with a forward.

| Order | Input signal                                                                                                                                            | Use this instead                                                                               | Kind        |
| ----- | ------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- | ----------- |
| 1     | An attack or incident with current user impact                                                                                                          | `/agentics:ops response`, at once. Live impact needs containment, not a design model.          | safety stop |
| 2     | A security look at code: a diff, a pull request, a branch, a source file, or "scan this change"                                                         | `/agentics:review axes <ref> --security`. This skill has no code-reading mode.                 | cannot      |
| 3     | Findings that bots or external scanners (Checkmarx, CodeQL) posted on a pull request                                                                    | `/agentics:triage pr <N>`, which owns posted findings and their remediation.                   | cannot      |
| 4     | A general code review: correctness, style, architecture of existing code                                                                                | `review`.                                                                                      | cannot      |
| 5     | Building the mitigations a model recommends                                                                                                             | `routing`, which selects the phase the work enters. The mitigations become requirements there. | cannot      |
| 6     | Otherwise: a design, an existing system described in words, or a path to a document that describes a system (Markdown, a diagram, an API specification) | This skill runs. [Entry](#entry) picks the input.                                              | —           |

A path decides by what the file is: a document that describes a system takes row 6, and a source file takes row 2.

## Entry

The skill has one workflow and one flag. `--composed` returns the model to a calling skill instead of writing a file;
[Composed mode](#composed-mode) holds its contract. Read the rows in order; the first match wins.

| Order | Input                                                 | Action                                                                                                                                                         |
| ----- | ----------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1     | `--composed` is present                               | Model the text the caller passed and any document it names. Run [Composed mode](#composed-mode).                                                               |
| 2     | A path to a document, or a description that names one | Model that document, with the [workflow](references/threat-model-workflow.md).                                                                                 |
| 3     | A prose description with no document named            | Run the document search in Phase 1 of the [workflow](references/threat-model-workflow.md#phase-1-gather-scope), then model the result.                         |
| 4     | Otherwise: a bare invocation                          | When the conversation holds a design under discussion, say in one line that it is the target and model it. When it holds none, ask what to model, in one line. |

Then load [references/threat-model-workflow.md](references/threat-model-workflow.md) and follow it. Load the shared
parts of [review's security principles](../review/references/security/security-principles.md) that
[What this skill takes from review](#what-this-skill-takes-from-review) names.

## What this skill takes from review

Review's [security principles](../review/references/security/security-principles.md) are written for code. This skill
uses five of their sections and replaces the rest, because a design has no file, line or data flow to cite:

| Section of security-principles       | Here                                                                                                                                       |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------ |
| Priority hierarchy                   | Used to order threats of the same rating.                                                                                                  |
| Severity taxonomy                    | Used for the meaning of each rating and for the acceptance annotation. The rating itself comes from the workflow's matrix.                 |
| What a clean result means            | Used. No result of this skill says `Pass`, `Clean` or `Secure`.                                                                            |
| Composition contract (JSON envelope) | Used, with the threat-model fields that [Composed mode](#composed-mode) adds.                                                              |
| Scope discipline                     | Replaced: the scope is the system boundary that Phase 1 records.                                                                           |
| False-positive suppression           | Replaced: a threat is recorded only when the model can name its boundary, asset, actor and impact in plain words.                          |
| Evidence standard                    | Replaced: a threat cites the design section, diagram edge, or user statement it rests on. A threat that rests on none is an open question. |
| Reviewer conduct                     | Used.                                                                                                                                      |

A threat's rating maps to the shared severities by the Security lane row of
[severity.md](../routing/references/contracts/severity.md#producer-mapping): Critical stays Critical, High is
Important, and Medium and Low are Minor.

## Gates

Each gate below is stated once, with its class from [gates.md](../routing/references/contracts/gates.md). A safety gate
is held in that file's fixed shape, said once: what is held, the harm it prevents, and what the user can do instead. A
repeated demand gets a one-line pointer back to that alternative. A process gate names its cost once and yields on the
next reply that still declines.

- **No verdict on the system (safety gate: a claim of status with no evidence).** The model says what could go wrong
  with the design as described. It reads no code, so it cannot say whether the implementation matches the design or
  whether a listed mitigation was built. The document says so in its Scope. No result reads `Pass`, `Clean` or
  `Secure`, and no sentence can be quoted as "security reviewed this and it was fine". The harm is a reader who skips
  the code review because the design was modeled. Instead: once code exists, `/agentics:review axes <ref> --security` checks the
  code against the plan.
- **Every identified threat appears (safety gate: a claim of status with no evidence).** A model that drops a threat it
  found still reads as covering that boundary, so the next reviewer does not look there. Instead: the user can ask for
  a short model, in which threats rated Medium and Low take one line each in a summary list, or can put a boundary out
  of scope, which the Scope section then names.
- **Ratings come from the matrix (safety gate: a claim of status with no evidence).** A request to lower a rating, with
  no fact behind it, does not move it. A fact the user states about their own system does: "that endpoint is reachable
  only over the VPN" changes the likelihood, and the threat records the control as stated by the user, not as found in
  the design. Instead of a lower rating, the user can accept the risk under their own name.
- **An acceptance names who accepted it and where it is written (safety gate: the user's own approval).** The user's
  own acceptance in the session is recorded with the user as the accepting party and `stated by the user, <date>` as
  its source. A claim that someone else accepted is not the user's acceptance: it is recorded as
  `Unverified — no artifact cited` unless the user names the ticket, ADR or decision record, and the skill asks the
  user once whether they accept the risk themselves. The threat and its rating stay unchanged either way. The harm is a
  bare claim that renders exactly like a traced approval.
- **`Status: Approved` needs its two conditions (safety gate: a claim of status with no evidence).** The skill writes
  `Draft`. It writes `Reviewed` or `Approved` only when the user tells it to, and `Approved` only when every Critical
  and High threat has a named owner and every accepted risk cites a source other than `Unverified`. Otherwise it keeps
  the lower status and names what is missing.
- **Owners for Critical and High threats (process gate).** Phase 5 asks once for the owners it cannot read from the
  input. A user who declines is not asked again: those threats read `unassigned`, the status stays `Draft`, and the
  report names them. The cost, said once, is that a High threat with no owner is how a known risk ships.
- **The skill never posts, commits or pushes the model.** The file stays in the working tree for the user to place. The
  workflow's [Output](references/threat-model-workflow.md#output) section says what the report adds on a public
  repository or under an embargo.

## Composed mode

`spec` calls `/agentics:threat-model --composed` on a solution it is drafting, reads the envelope, shows the user each proposed
mitigation, and keeps only the ones the user accepts. Composed mode serves that caller:

- It writes no file and asks no question. The caller owns the conversation, so a scope item the input does not settle
  becomes an assumption in `open_questions`, and no owner question is asked.
- Every disposition and every mitigation is a proposal. Nothing is recorded as accepted for the user, and the model's
  status is `Draft`.
- It returns one envelope, defined in the workflow's
  [Composition](references/threat-model-workflow.md#composition) section, and stops. The caller surfaces it. Embed the
  complete escaped Markdown threat model (with Markdown Asset and Actor tables, trust-boundary diagram, STRIDE
  findings, and mitigations) directly inside the JSON `model` string — never write the Markdown outside the JSON and
  substitute a placeholder string or abbreviated summary inside `model`.

## References

- [references/threat-model-workflow.md](references/threat-model-workflow.md): the five phases from scope to mitigations,
  the output path, and the composed envelope.
- [references/threat-model-template.md](references/threat-model-template.md): the Markdown template with the diagram,
  the asset and actor tables, the risk matrix and the mitigation plan.
- [security-principles.md](../review/references/security/security-principles.md): the shared security foundation, used
  as [What this skill takes from review](#what-this-skill-takes-from-review) says.
- [gates.md](../routing/references/contracts/gates.md), [approval.md](../routing/references/contracts/approval.md),
  [unattended.md](../routing/references/contracts/unattended.md),
  [untrusted-data.md](../routing/references/contracts/untrusted-data.md) and
  [question-format.md](../routing/references/contracts/question-format.md): the shared rules this skill applies.
