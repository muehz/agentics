---
name: ops
description: "Use when an outage or security incident needs coordinated response: contain the impact, restore service, and preserve evidence. Also use for postmortems, incident drills, and runbooks. Ordinary development failures belong to diagnose."
version: "2.2.0"
user-invocable: true
disable-model-invocation: false
argument-hint: "response | postmortem [incident-id] | drill [scenario | runbook-path] | author [from-postmortem <id> | <name>] | audit [path]; aliases: live/now/active→response, retro/pm→postmortem, tabletop/gameday/exercise/test→drill, create/new→author, review/check→audit"
metadata:
    surfaces: [claude-code, copilot-cli, vscode, codex, antigravity]
    modes: [response, postmortem, drill, author, audit]
    mode-aliases:
        live: response
        now: response
        active: response
        retro: postmortem
        pm: postmortem
        tabletop: drill
        gameday: drill
        exercise: drill
        test: drill
        create: author
        new: author
        review: audit
        check: audit
---

# Ops

This skill responds to incidents, learns from them, rehearses for them, and keeps the runbooks current. Reliability incidents (outages, performance regressions, data integrity) and security incidents (suspected breach, exploited vulnerability, leaked credential) branch at declaration time.

Skill commands here use the `/agentics:<skill>` form that Claude Code, Copilot CLI, and VS Code Chat accept. On Codex,
type `$agentics:<skill>` instead.

## Don't use when

A request that states current user impact, full or partial, is an incident: it selects `response` before this table is read, as [Which mode?](#which-mode) says. Otherwise read the rows from the top; the first match wins. The **Kind** column says what an explicit invocation does with the row, as [forwarding.md](../routing/references/forwarding.md#explicit-invocation) defines it: a `cannot` row forwards, and a `better elsewhere` row is named in one line and the run continues here. When the model loaded this skill on its own, every row forwards.

| Input signal                                                                                                          | Use this instead                         | Kind             | Why                                                                                                                    |
| --------------------------------------------------------------------------------------------------------------------- | ---------------------------------------- | ---------------- | ---------------------------------------------------------------------------------------------------------------------- |
| A bug, test failure, or regression to root-cause, with no current user impact                                         | `diagnose`                               | better elsewhere | Root-cause work with no live impact is debugging. Under `/agentics:ops response`, declare it at the severity it earns. |
| A proactive threat model of a design, with no active incident                                                         | `threat-model`                           | cannot           | This skill responds to incidents. It does not assess a design.                                                         |
| A proactive security review of code that names security, a vulnerability, or a class such as auth                     | `/agentics:review axes --security`       | cannot           | Code-scoped security review belongs to `review`.                                                                       |
| A pull request or diff to review for bugs or logic errors, with no active incident                                    | `review`                                 | cannot           | Code review verdicts belong to `review`.                                                                               |
| Pull request findings or check-run failures that have not reached users                                               | `triage`                                 | cannot           | A flagged pull request or a failing check is a signal, not an incident, until users are affected.                      |
| The deliverable is prose or reference: a README, a guide, an architecture explainer, or endpoint or OpenAPI reference | `docs`                                   | cannot           | A runbook is a procedure for an operator under pressure, not reference or training material.                           |
| A proposed system or process change that needs approval, impact analysis, or a change-board rollback                  | `/agentics:gate change`                  | cannot           | That is a change record, not a repeatable operational procedure.                                                       |
| Committing a runbook file or closing out its branch                                                                   | `/agentics:git commit`, `/agentics:gate` | cannot           | Those land the content. This skill produces it.                                                                        |

## Workflow

Load [references/shared-principles.md](references/shared-principles.md) once the mode is known. It holds the severity table, the branch decision, the roles, the blameless rule, evidence preservation, and the runbook rules every mode assumes. Then load the mode's workflow file and follow it. Announce the mode in one line, once.

Ask every decision question in the exact format and with the answer collection that [question-format.md](../routing/references/contracts/question-format.md) defines: `❓ **Q<n>** - **<title>**:` followed by a line starting with `➡️` giving the recommendation. One question holds one decision.

### Which mode?

Read the rows from the top; the first match wins.

| Order | The invocation or the conversation                                                                                                               | Mode                        |
| ----- | ------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------- |
| 1     | States that production or users are affected now ("down", "outage", "breach", "paged", "alert firing"), with no resolution language and no drill | `response`                  |
| 2     | Its first word is a mode or an alias from the frontmatter                                                                                        | that mode                   |
| 3     | Describes a resolved incident ("is over", "resolved", "restored") or asks for a retro or postmortem                                              | `postmortem`                |
| 4     | Asks to rehearse or exercise (drill, tabletop, game day)                                                                                         | `drill`                     |
| 5     | Asks to check, audit, or review an existing runbook                                                                                              | `audit`                     |
| 6     | Asks to write a runbook                                                                                                                          | `author`                    |
| 7     | Otherwise                                                                                                                                        | ask the mode question below |

A drill scenario's name ("region outage") and a past incident are not current impact, so row 1 does not take them. A request that matches row 1 never waits for a mode question: `/agentics:ops new outage in checkout` is `response`.

With row 7, call no tools, load no reference and draft nothing before the answer:

> ❓ **Q1** - **Mode**: Respond to an active incident (`/agentics:ops response`), write a postmortem (`/agentics:ops postmortem`), run a drill (`/agentics:ops drill`), write a runbook (`/agentics:ops author`), or audit one (`/agentics:ops audit`)?
>
> ➡️ The mode the conversation points at: postmortem when an incident was just resolved, audit when a runbook path is in context; `response` when nothing points, because a missed active incident costs more than a downgrade.

Anything a draft needs that the request does not give becomes a question or an `<angle-bracket>` placeholder, never an invented fact.

### /agentics:ops response

Coordinate an active incident: declare, assemble, contain, resolve, and hand off. Follow [references/response/response-workflow.md](references/response/response-workflow.md). Its first reply gives containment guidance before any question, completes the declaration checklist (severity, branch, incident record draft in a fenced block, channels, and first internal update), and asks only the decisions whose answer changes what it produces. On an early-resolution override (after naming the recovery-window cost once), record the early resolve in the incident record and output the resolved update in a fenced block labeled as a draft (never posted unless `gh` ran).

**Branch.** The declare phase asks whether the incident involves unauthorized access, credential compromise, malicious activity, or unexpected data exposure, unless the request already says. Yes or unsure loads [security-branch.md](references/response/security-branch.md); no loads [reliability-branch.md](references/response/reliability-branch.md); both runs security comms and forensics with reliability customer recovery. The user's own answer decides, including "it's just an outage". When evidence in the session later points the other way, say once what was observed and ask the question again.

**One responder is a valid incident.** At any severity, when nobody else is available, the user holds every role, the security lead's decisions included. Do not stall until a role is filled. Record `Single operator: one person holding every role` in the incident record.

**Root-cause work.** This skill decides when root-cause work starts, by the hand-off table in the workflow's Phase 4. `diagnose` starts on that hand-off and does not ask about containment again. When no containment exists, the hand-off happens with the impact still running.

### /agentics:ops postmortem

Run a blameless retrospective on a resolved incident. Follow [references/postmortem/postmortem-workflow.md](references/postmortem/postmortem-workflow.md); the artifact follows [postmortem-template.md](references/postmortem/postmortem-template.md). The argument is an optional incident reference, such as the tracking issue number. Without one, ask for the reference and any timeline notes. When the user describes the incident, draft from whatever they give, even when they mention notes they did not paste, with `<angle-bracket>` placeholders for the rest (including `Branch` unless the user explicitly supplies it), and ask for the reference and the notes after the draft. Action items always include a regression test or eval, or `n/a: <reason>`. Any promised action-item issue previews appear in fenced blocks in the same reply.

### /agentics:ops drill

Rehearse the response process with a scenario, or exercise one runbook. Follow [references/drill/drill-workflow.md](references/drill/drill-workflow.md). The argument picks the type:

| Order | Argument                                    | Drill type                                                                            |
| ----- | ------------------------------------------- | ------------------------------------------------------------------------------------- |
| 1     | A path that resolves to an existing runbook | runbook drill: the deliverable is a delta report against the runbook                  |
| 2     | A scenario name or a free-text scenario     | scenario drill: the deliverable is a debrief with action items                        |
| 3     | Otherwise (no argument)                     | ask one question: a scenario, or a runbook (recommend runbook when one is in context) |

Both types run as a tabletop (narrated) or a game day (real execution in a non-production environment). The starter scenarios are in [drill-scenarios.md](references/drill/drill-scenarios.md). For a security drill on a public repository, keep drill comms and the debrief record in a private drill channel, draft security advisory, or private location—never a public issue.

### /agentics:ops author

Create a runbook. `/agentics:ops author <name>` starts from the template; `/agentics:ops author from-postmortem <issue>` seeds it from a postmortem issue first. Follow [references/author/author-workflow.md](references/author/author-workflow.md), and for the seeded variant [from-postmortem.md](references/author/from-postmortem.md) before it. The runbook has the nine mandatory sections of [runbook-template.md](references/runbook-template.md).

### /agentics:ops audit

Check one runbook for staleness, command drift, dead links, structural gaps, ownership, and scope. Follow [references/audit/audit-workflow.md](references/audit/audit-workflow.md). The report gives each step a status and lists the findings by severity.

## Rules every mode follows

### Writes outside the machine

Every GitHub artifact this skill produces (a tracking issue, a status comment, a postmortem issue, an action-item issue, a drill debrief, a publishing pull request, or a draft security advisory) is drafted in full in a fenced block in chat first—never promise a draft without including its fenced block in the reply. [github-artifacts.md](references/github-artifacts.md) holds the templates and [github-publishing.md](references/github-publishing.md) the publishing paths. Comms to channels, status pages and customers are drafts for the user to send.

- **Authorization.** A post goes out on any clear affirmative to its draft, or without a second question when the user named that post up front, as [approval.md](../routing/references/contracts/approval.md) says. A named post that a skill previews first is the exception, as [approval.md](../routing/references/contracts/approval.md#a-named-post-a-skill-previews-first) says. Naming a kind of post counts only for that kind: "post the cadence updates on the tracking issue" authorizes each cadence update for this incident (shown in chat as it goes), and does **not** authorize creating the tracking issue itself—still show the tracking issue draft for a yes. A post of the same kind is confirmed with its batch: the action-item issues of one postmortem are previewed together and confirmed once. A blanket "post everything, don't ask" authorizes the kinds the agent lists back in one line at that point; the line names any post it will still show first.
- **Outbound checks.** Before any post, run the [outbound checks](../gh/references/posting-envelope.md#outbound-checks). The envelope's marker and footer rules apply to every comment and issue body.
- **A security incident on a public repository is never drafted as an issue or a comment there.** The tracking record and the postmortem go to a draft security advisory, or to a private location the user names, as [security-branch.md](references/response/security-branch.md#where-the-record-lives) says.
- **Status of a post.** Report a post as made or cite a URL **only** when this session actually ran the command and captured its URL; otherwise label it drafted/proposed (never claim a simulated post succeeded or invent a returned URL).

These are [safety gates](../routing/references/contracts/gates.md#safety-gate) for posts the user has not authorized and for content the outbound checks withhold: a request does not lift them. When one holds, say once what is held, the harm, and what the user can do instead (post it themselves, or use the private channel). A repeated request gets **exactly one sentence on one line** pointing back to that alternative (do not restate the harm or repeat the full hold).

The skill cannot act on a timer. It does not promise a cadence post or a drill inject at a later time; it says when the next one is due and drafts it on the user's next message. A run that is [unattended](../routing/references/contracts/unattended.md) posts only what was named up front and returns every other draft in its report.

### Status honesty

Never state as verified what this session did not produce and check. This is the status-claim [safety gate](../routing/references/contracts/gates.md#safety-gate):

- **`Last tested`** changes when a drill completed in this session, or when the user gives the date of their team's own drill, recorded as given by them. An audit, an edit, or a partial drill does not change it.
- **Audit and drill results** report only the checks that ran. A check that could not run is `uncertain` or `pending user decision`.
- **Posted, opened, merged** is claimed only after the command ran and succeeded in this session.
- **Timings and test claims** the user states about their own systems are recorded verbatim as stated by them (keeping every part of their statement, e.g., `"per the requester: tested in production; usually completes in under 30 seconds"`), and repeated in full if pressed without dropping the production-test clause or changing `Last tested`. A claim with no source is marked `estimated` and gets a drill action item to confirm it. Unsupplied postmortem fields stay `<angle-bracket>` placeholders with no source label.
- **A postmortem** never states a cause the session's evidence contradicts, and never drops a material contributing factor the user has stated; the blameless rule in [shared-principles.md](references/shared-principles.md#blameless-framing) reframes such a factor in terms of the system.

When it holds, say once what is held, the harm, and the alternative: the user can edit and publish their own text.

### Where runbooks live

Runbooks live at `runbooks/<name>.md` in the repository where the user invokes the skill, or in that repository's existing layout, such as `docs/runbooks/`. The user can name the path (`/agentics:ops author docs/runbooks/<name>`).

In the Agentics plugin repository itself, the default is to give the runbook in chat: a `runbooks/` directory committed there ships an operational file to every install. This is a [process gate](../routing/references/contracts/gates.md#process-gate). Name that cost once; if the user still asks for the file, write it where they say. The examples under `references/examples/` are illustrative and are never copied to the repository root.

## References

- [references/shared-principles.md](references/shared-principles.md) covers severity, the branch decision, roles, the blameless rule, evidence preservation, and the runbook rules, loaded once a mode is known.
- [references/response/README.md](references/response/README.md) covers the response-mode reference index.
- [references/response/response-workflow.md](references/response/response-workflow.md) covers the five-phase response workflow and the hand-off to `diagnose`.
- [references/response/reliability-branch.md](references/response/reliability-branch.md) covers the reliability branch.
- [references/response/security-branch.md](references/response/security-branch.md) covers the security branch, its restricted comms, and where its record lives.
- [references/postmortem/README.md](references/postmortem/README.md) covers the postmortem-mode reference index.
- [references/postmortem/postmortem-workflow.md](references/postmortem/postmortem-workflow.md) covers the five-phase postmortem workflow.
- [references/postmortem/postmortem-template.md](references/postmortem/postmortem-template.md) covers the structure of the postmortem artifact.
- [references/drill/README.md](references/drill/README.md) covers the drill-mode reference index.
- [references/drill/drill-workflow.md](references/drill/drill-workflow.md) covers the six-phase workflow for both drill types.
- [references/drill/drill-scenarios.md](references/drill/drill-scenarios.md) covers the starter scenarios, three reliability and three security.
- [references/author/README.md](references/author/README.md) covers the author-mode reference index.
- [references/author/author-workflow.md](references/author/author-workflow.md) covers the six-phase runbook authoring workflow.
- [references/author/from-postmortem.md](references/author/from-postmortem.md) covers seeding a runbook from a postmortem issue.
- [references/audit/README.md](references/audit/README.md) covers the audit-mode reference index.
- [references/audit/audit-workflow.md](references/audit/audit-workflow.md) covers the six-check audit and its report.
- [references/examples/README.md](references/examples/README.md) covers the illustrative runbook examples.
- [references/examples/example-database-failover-runbook.md](references/examples/example-database-failover-runbook.md) covers an example database failover runbook, deliberately stale for audit practice.
- [references/runbook-template.md](references/runbook-template.md) covers the runbook template that `/agentics:ops author` produces.
- [references/communication-templates.md](references/communication-templates.md) covers the comms templates, including the restricted security template.
- [references/github-artifacts.md](references/github-artifacts.md) covers the templates for the GitHub artifacts the skill drafts.
- [references/github-publishing.md](references/github-publishing.md) covers publishing a runbook beyond its home repository.
- Related skills: `diagnose` takes root-cause work on the hand-off from response; `triage` is a common upstream signal; `gate` closes out a remediation or runbook branch; `review` runs the security-branch evidence read (`/agentics:review axes --security --composed`); `docs` writes a long-form postmortem; `gh` performs the GitHub writes.
