# Skill-to-skill forwarding

This file is the only route-away protocol. A skill follows it when its preflight, route-out table, or
`## Don't use when` row names another skill as the owner of the input. A skill's route-away cites this file and does not
restate it. The skill boundary stays in place.

A skill that does not own its input carries the input to the owner. It does one of three things, as the
[target classification](#target-classification) assigns:

- It announces and runs a local-only target.
- It asks one yes/no question before a target that writes, pushes or posts.
- It states the boundary when no skill owns the job.

The skill forwards at most once. It never tells the user to retype the request. It never asks a starter question on the
target's behalf. The target asks its own questions.

Read [Explicit invocation](#explicit-invocation) first. It decides whether a row routes away at all. Then honor the
explicit implementation exception below. Otherwise, use the behavior that the
[target classification](#target-classification) table assigns.

## Explicit invocation

An explicit invocation is the user naming the skill for this request, as a slash command or in plain words. Under an
explicit invocation, each `## Don't use when` row of the named skill is one of two kinds. Each row is marked with its
kind.

- A **cannot** row names a deliverable that no mode of the skill produces, such as `/agentics:git` asked to open a pull
  request. The skill forwards by this protocol.
- A **better elsewhere** row names work the skill can do but another skill does better, such as a one-line edit under
  `/agentics:forge`. The skill says so in one line, names the other skill, and runs.

When the model loads a skill on its own, from a description match or a forward, every row applies.

A stop that protects safety rather than ownership, such as an active incident, is not a route-away. This protocol does
not carry it anywhere.

## Explicit implementation request

When a user explicitly asks to start, continue, implement, or work an identified issue, that request authorizes three
things:

- the [continuation check](chain-phases.md#continuation-check);
- phase selection;
- the writing of the local lifecycle artifacts that the work needs.

For such a request, do these steps:

1. Announce the handoff.
2. Apply the selected phase's own entry rule, exemptions, and existing approval evidence.
3. Do not ask for a second handoff confirmation.

A read-only issue question does not qualify. This exception authorizes neither remote writes nor a bypass of the
target's own approval gates.

## The route is already settled

A route is already settled when one of these holds:

- the user named the phase skill themselves (`/agentics:gate branch`, or the same request in words);
- an approved plan records the execution approval (`flow status` lists a current `plan.md` approval);
- the user named the step in the request, its turn has come, and it touches what the user named.

That is the answer the offer would ask for. When a route is settled, announce it in one line and run. Do not ask the
yes/no question. Forwarding does not weaken consent. The approval already happened, and a repeated question would only
relitigate a decided answer.

A step the user named in the request is a commit, a push, a draft pull request, starting execution after planning, or
posting a named comment or issue. It is settled in every skill, with no second question. It covers only what the user
named. A named post that a skill previews first is the exception, as
[approval.md](contracts/approval.md#a-named-post-a-skill-previews-first) says.

The target's consent gates for anything the user did not name still fire, such as gate's push boundary and execute's own
gates. Gate's Step 3 proposes a draft PR against the base. It proposes a `gh stack` layer instead when the branch builds
on one open PR or a linear chain of them. An open spec gap withholds both. A draft PR or push named earlier answers only
the plain draft proposal. It never authorizes a stack. A stack needs its own yes
([gate's Answers](../../gate/references/branch/propose-and-execute.md#answers)).

Execution that the user named up front ("plan this, then hand it to execute") is settled when planning ends. The plan is
shown, by its path and hash at least, before the approval row is written. An approved plan whose `**Coverage:**` line
lists an unwaived blocking item is not settled. Such a plan goes to `/agentics:decompose`, with the blocking items
named. `execute` itself refuses such a plan, so a forward there would stop at its Step 1.

An approval that `flow status` reports as `unconfirmed here` is not a settled route. The phase that needs the approval
presents the artifact and asks once, and a yes writes a new row. An unattended run cannot ask, so it stops before that
phase and reports the artifact as unconfirmed.

Two kinds of step are never settled by an earlier statement.

- A destructive step: deleting a branch, discarding work, a force push, a history rewrite or an irreversible migration.
  It is confirmed once at the point of action, after the agent has shown the resolved target and what will be lost. The
  reply must name the action or its target in the user's own words; a bare "yes" does not confirm it.
- A write that changes what later agents are told: an instruction file, a skill, a generated verification skill or its
  helper scripts. It needs the user's yes in the session, after the exact text to be written is shown.

The same holds for a phase that the current row's `next` selected. A selected `execute` on a plan with a current
`plan.md` approval for this item is settled, whichever way the user arrived
([chain-phases.md](chain-phases.md#what-selection-does-not-decide)).

## Target classification

The one list of how a route-out reaches its target. A caller passes the input, the authorization the user already gave,
and the requested scope. It never reclassifies the target.

Read the rows in order: the first match wins. Check [the settled route](#the-route-is-already-settled) before the table:
a settled route removes an offer's question and changes no other row.

| Target                                                                                                                                                      | Behavior                 | Why                                                                                                                                              |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| `/agentics:review feedback`, `/agentics:review interactive`, `/agentics:review axes`, `/agentics:challenge`, `/agentics:diagnose`, `/agentics:zealot tests` | A. auto-forward          | Local only. See the note on `/agentics:zealot tests` below the table.                                                                            |
| `/agentics:gate` for a readiness question ("Is this ready to merge?")                                                                                       | A. auto-forward          | Its Step 0 refreshes the evidence read-only, answers, and runs nothing                                                                           |
| `routing`, `/agentics:intent`, `/agentics:spec`, `/agentics:decompose`                                                                                      | A. auto-forward          | They select a phase or write local lifecycle artifacts. Their own approval questions still fire.                                                 |
| `/agentics:execute` on an approved plan whose `**Coverage:**` line lists an unwaived blocking item                                                          | to `/agentics:decompose` | Name the blocking items. `execute` refuses such a plan.                                                                                          |
| `/agentics:execute` on a plan whose approval reads `unconfirmed here`                                                                                       | ask first                | The phase that needs the approval presents the plan and asks once                                                                                |
| `/agentics:execute` on a plan with a current approval                                                                                                       | settled route            | The approval already happened                                                                                                                    |
| `/agentics:execute` on a plan with no current approval                                                                                                      | to `/agentics:decompose` | [Approval](contracts/approval.md) is the user's yes in any wording, the execution question, or execution named up front. A forward is never one. |
| `/agentics:gate`, `/agentics:triage pr`, `/agentics:gh` writes, `/agentics:git`, `/agentics:git resolve`, `/agentics:scaffold`, `/agentics:threat-model`    | B. offer                 | They write code, push, post, or change repository files other than lifecycle artifacts                                                           |
| "No skill", normal implementation                                                                                                                           | boundary statement       | Say that this skill has no workflow for the job. The session may do the work directly. Never a stop.                                             |
| `_forbidden_`, CI                                                                                                                                           | no forward               | State the boundary and stop. See the guard rails.                                                                                                |
| Otherwise: any target that no row above names                                                                                                               | B. offer                 | An unnamed target may write, push, or post                                                                                                       |

`/agentics:zealot tests` writes only disposable mutant worktrees and snapshot files under the host temporary directory,
plus the worktree metadata Git keeps for them. It removes all of it before it reports.

## Job owners

Every job a skill names has exactly one owner. A skill that does not own a job looks up the owner here and reaches it by
the [target classification](#target-classification) above.

| Job                                                               | Owner                                     |
| ----------------------------------------------------------------- | ----------------------------------------- |
| UX deliverables                                                   | No skill                                  |
| REST and OpenAPI reference documentation                          | `docs`                                    |
| Deleting dead code, unused files or feature flags                 | `routing`, as an ordinary mutating change |
| Hook authoring                                                    | `forge`                                   |
| Instruction-file authoring and edits                              | `forge`                                   |
| The starter instruction file, when none exists                    | `scaffold`                                |
| Principle files                                                   | `forge`                                   |
| Purging a leaked secret: the response and its order               | `ops`                                     |
| Purging a leaked secret: the local history rewrite                | `git`                                     |
| Fixing a finding `gate` raised                                    | `execute`                                 |
| `LICENSE`, `SECURITY.md`, `CODE_OF_CONDUCT.md`, `FUNDING.yml`     | `scaffold init`                           |
| Planning offered from `explain` or `docs`                         | `routing`                                 |
| Approval or header repair on an existing spec                     | `spec draft`                              |
| A yes to the threat-model offer in `spec draft`                   | `threat-model`, then back to the spec     |
| The scope of a challenge that `intent` invokes                    | `intent`                                  |
| The repair row for a `mismatch:` line                             | `execute`                                 |
| When root-cause work starts in an incident                        | `ops`                                     |
| A conflict on a pull request branch                               | `git resolve`                             |
| One named review thread, or a draft `triage` produced             | `review feedback`                         |
| A request no skill owns after the ambiguity ladder                | No skill                                  |
| Writing a decision record                                         | `docs`                                    |
| Implementing test-first, or adding tests                          | `tdd`                                     |
| A readiness question asked as `/agentics:gate`                    | `gate check`                              |
| A wording, typo, link or one-line correction to an Agentics asset | `forge`                                   |
| A request worded as creation of standard files                    | `scaffold init`                           |
| Push and force-push                                               | `git`                                     |
| Removing a worktree its run has finished with                     | The skill that created it                 |
| Removing a feature worktree that outlived its run                 | `git worktrees prune`                     |

A "No skill" owner is a boundary statement: the skill says it has no workflow for the job, and the session may do the
work directly.

A readiness question that a skill forwards goes to `/agentics:gate` by the target classification above, which answers
from its read-only refresh.

The last three rows are pointer rows, decided by the rules on who writes to Git: the [git skill](../../git/SKILL.md)
states the rule for a push and a force-push, and [session-storage.md](session-storage.md#worktrees-and-ignore-rules)
states the rule for removing a worktree.

## A. Auto-forward: announce, then run

Use this behavior when the [target classification](#target-classification) marks the target A. Such a target cannot post
or push. It leaves nothing behind beyond the work folder's lifecycle artifacts (`intent.md`, `brief.md`, `spec.md`,
`plan.md`, `state.tsv`). Scratch that it writes and removes before it reports, as `/agentics:zealot tests` does, does
not count.

- Announce in one line that names the situation and the target's safety scope. Then invoke the target with the input and
  any data already fetched:

    > This is one human thread. Running `/agentics:review feedback` drafts only, and nothing posts.

- Consent questions inside the target still fire. Auto-forward hands over the input. It never pre-answers the target's
  own gates.
- A forward to `/agentics:review` names one of the local modes above explicitly. Never use `post`, which posts a public
  PR comment.

## B. Offer-to-forward: one yes/no question

Use this behavior when the table marks the target as an offer. Examples are `/agentics:triage pr` (auto-posts
bot-finding replies and pushes once) and `/agentics:gate` (can merge locally or push a draft PR). `/agentics:execute` is
never an offer: the table gives each `/agentics:execute` case its own row. Ask one yes/no question that names the
target's side-effect scope. Forward only on yes:

> This PR has 3 unresolved threads plus bot findings. Settle them all in one coordinated pass (via
> `/agentics:triage pr`)? It integrates fixes, pushes once, and posts bot-finding replies (human replies stay drafts).

Name what runs and what does or does not post. A phase-boundary question reads as natural English. Describe the step in
plain English first. Put the invocation in parentheses at the end. The parenthetical carries the leading slash and names
the skill and mode only, never argument placeholders. Never lead with the command. For example: "Ready to close this out
(via `/agentics:gate branch`)?"

**On "no", stay where you are.** A declined forward does not become permission to do the target's work under another
heading. Stay inside the forwarding skill's own read-only scope. Report what it already fetched. Ask what the user wants
instead. Never carry out the deferred write. Never run a partial version of it as a compromise. The decline covered the
side effects that the question named. Those side effects, done without the label, do not answer the question.

## Guard rails for both behaviors

- **Forward at most once per invocation.** This guard applies to every skill. A skill invoked by another skill does not
  route the input back to its caller. If the target's own preflight bounces the input back anyway, stop and explain the
  bounce to the user. Never forward again or loop.
- **Outside the explicit implementation exception and a settled route, never auto-forward to a target the table marks as
  an offer.**
- **The forwarding skill does none of the deferred work itself.** It passes along what it already fetched and hands over
  cleanly. Do not add partial analysis under its own heading.
- A row whose target is "No skill" or normal implementation is a boundary statement and never a stop. The skill says it
  has no workflow for the job, and the session may do the work directly.
- A row whose target is `_forbidden_` or CI forwards nowhere. State the boundary and stop.
