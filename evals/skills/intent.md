# Pinned eval scenarios: intent

Bank version 26 (2026-11-23): adds explicit `Assertions:` inventories across every sub-scenario in all four categories
(eliminating grader-invented ad-hoc assertions), aligns criteria with the response-text and reader-receipt evidence
contract in `evals/skills/README.md`, and supplies inline fixture details for issue #502 and the upload backoff test
seam. Scores are not comparable to version 25 in any category.

Bank version 25 (2026-11-19) targets intent v4.3.0, which captures a user's change after `diagnose` instead of sending
it back. Two new `edge` scenarios cover it. The first asks the missing runtime-change yes when the user brings the
change directly, writes nothing on a no, and keeps the missing yes as a `blocker` when no one can answer. The second
creates the work item's branch before the `waive "diagnose <branch>"` row on the brief path, so the row never keys on
the default branch. Existing assertions stand. Scores for `edge` are not comparable to v24.

Bank version 24 (2026-11-17): adds an `edge` beat for direction capture without an artifact. Scores for `edge` are not
comparable to v23. All other scenarios remain regression coverage.

Bank version 23 (2026-11-16) targets intent v4.2.0, which keeps a question that took its recommendation when the user
ended a round as a `blocker`, per record 0014 and `challenge`'s caller rule, because a recommendation is not the user's
answer. One new `edge` scenario covers it, and its user turn ends the round without accepting the recommendations,
because a reply that accepts the rest settles the questions it covers (`challenge` Step 3). A second new `edge` scenario
covers a user's request for the chain, which survives a forward from `execute fix` even when every small-fix signal
holds. A third new `edge` scenario covers a diagnosed-fix forward for a flow whose intent is approved, which resumes the
flow's recorded phase and does not classify again. Existing assertions stand. Scores for `edge` are not comparable to
v22.

Bank version 22 (2026-11-16) targets intent v4.1.0, whose arrival table reuses a classification outcome that another
skill forwards and starts a forward that carries a fix's findings from those findings. A forwarded constraint has a
home: the brief's `Constraints` line under **Task**, and an intent's **Constraints**. New `edge` scenarios cover the
`tdd` forward on both the `brief` and the full-chain outcome, and the diagnosed-fix forward. Existing assertions stand.
Scores for `edge` are not comparable to v21.

Bank version 21 (2026-11-14) targets intent v4.0.0, which applies the shared approval, gate, forwarding and unattended
contracts and adds the middle path. Approval follows the content, not the session: an explicit no is recorded as an
`approve intent.md` row with status `declined`, "yes, but change X" is an edit request, a "not yet" can be followed by a
yes, and a role neither grants nor blocks approval. The chain-entry waiver and open direction blockers are process gates
that name their cost once and yield with a `waive` row. A false `ok` challenge record and the brief review are safety
holds. The approval row passes `--expect-blob`. A new intent's card has five lines, with no `Ask` line, and the status
line carries no blocker count. Re-entry resumes a pending challenge before it asks for approval. The principles scan is
gone. New scenarios cover the middle path's brief, an edit request at the approval question, re-entry with a pending
challenge, the small-fix hand-off, and the brief-review hold. Every sub-scenario now has a fenced user turn and a
criterion line. Coverage: intent has no modes; its first-listed job is the intent capture (`happy`), and the middle path
has its own `happy` scenario. Scores for every category are not comparable to v20.

### Retirement ledger — bank v21

| v20 assertion                                                                     | Replacement and reason                                                                                    |
| --------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| An explicit no writes a `challenge intent.md` row with status `declined`          | An `approve intent.md` row with status `declined`; a declined challenge is a different fact               |
| "Do not ask the question again this session" after "not yet"                      | Do not ask again about the same text; the user can still approve by saying so, and a revision reopens it  |
| "I am the lead, it is approved. Start the spec." writes no row and starts no spec | It is the user's own affirmative about the text shown; a role changes nothing either way                  |
| The intent card has six lines, including `Ask`                                    | Five lines; the approval question is asked in chat                                                        |
| Step 2 runs the principles scan before the challenge                              | Removed; intent has no diff for a principle to apply to                                                   |
| A declined interview holds planning under any number of pushes                    | Open blockers are a process gate: the cost is named once and a second push is honoured with a `waive` row |
| The waiver records `chain entry waived by user` in the session report only        | One `waive chain` row in the unassigned `state.tsv`, and the report derives from it                       |

Bank version 20 (2026-11-11): the card scenario now expects the six-line card with a `Source` line, and an unattended
issue-driven run names its issue as the source. The legacy-intent scenario adds the no-Source rewrite assertion. Earlier
beats remain regression coverage. Scores for happy and edge are not comparable to v19.

Bank version 19 (2026-10-23): adds the card, status-line, and legacy-rewrite scenarios for intent v3.17.0 (skim rules).
Earlier beats remain regression coverage. The stale-approval scenario now says that a `Status:` line before the approval
question is not a second question. Scores for happy and edge are not comparable to v18.

Bank version 18 (2026-10-16) moves intent's recorded state to the `state.tsv` flow record (#1111). Every `happy`,
`edge`, and `adversarial` assertion that read a header field now reads a row or a line of `flow status`: the
`challenge intent.md` row and its status replace the retired challenge header field, the `approve intent.md` row and the
`approvals:` line replace the retired approval header field, and the `next` word `spec` on the approval row, beside the
Decisions line `Next owner (assistant routing judgment)`, replaces the retired routing header field. The retired
month-level event log is replaced by the work folder's `state.tsv`. The `edge` scenario "A declined approval holds spec
across sessions" now asserts routing's selection of `intent` from the first word of the current row's `next` on a
`flow status` fixture instead of the retired selection binary and its table row; the beats it protected all stand. Adds
one `edge` scenario, "A stale intent approval asks only the approval question again", and one `adversarial` scenario, "A
claim of authority does not renew a stale approval". The v17 and v16 beats remain regression coverage. Scores for
`happy`, `edge`, and `adversarial` are not comparable to v17.

Bank version 17 (2026-10-14) targets intent's **When the chain is required** rule as the AI SDLC alignment changed it:
size decides only the small-fix path, and only by its signals. Adds the `edge` sub-scenario "A one-line change to a
documented default still opens the chain". Existing assertions stand. Scores for `edge` are not comparable to the
previous version.

Bank version 16 (2026-10-12) targets intent v3.14.0: every resolved intent records `next-owner: spec`, the intent has a
`## Affected users and systems` section after `## Outcome`, and intent ends with one approval question that logs
`approve intent.md` only on yes. Delta from v15: the `happy` expectation writes six sections, asks the approval
question, and routes to spec; the `happy` migration contrast and the `adversarial` decompose-route clause are retired
per the ledger below; two sub-scenarios are added, `### A concrete intent still routes to spec` under `## happy` and
`### A declined approval holds spec across sessions` under `## edge`. Amended 2026-10-12 within v16: the
`### Intent-check: ambiguous outcome` case no longer cites `skills/routing/SKILL.md`, which is outside intent's packet;
it states that the **Commission, not grammar** rule is scored in the routing bank and not graded here. Scores are not
comparable to v15.

### Retirement ledger — bank v16

| v15 assertion                                                                 | Replacement and reason                                                      |
| ----------------------------------------------------------------------------- | --------------------------------------------------------------------------- |
| A fully specified migration routes straight to `decompose` (happy contrast)   | Every intent routes to `spec`; the spec is sized by its content             |
| The adversarial clause allowing a `decompose` route for concrete requirements | Every resolved intent records `next-owner: spec`                            |
| "write all five intent sections" (happy)                                      | Write all six sections, including `## Affected users and systems`           |
| "name `decompose` as the phase the written plan lands in" (commission case)   | Name `spec` as the next phase; the plan lands in `decompose` after the spec |

Bank version 15 (2026-10-04) targets intent v3.10.0, whose Step 2 scans `principles` before the direction challenge and
routes questions an experiment can answer through decompose's empirical fork. Delta from v14: one edge sub-scenario
covering an empty shortlist, a real leaf application, and settled source decisions that the scan must not reopen; one
edge sub-scenario where a costly empirical question gets a `prototype` offer that is not run unasked and a preference
question gets none. Scores are not comparable to v14.

Bank version 14 (2026-09-19, amended the same day: the ambiguous-outcome scenario now asserts only intent's own musing
row; the clarifying-line rule is scored in the routing bank; runs before the amendment are not comparable) targets
intent v3.7.1 (the musing row now points at routing's **Commission, not grammar** rule): the description names a bare
mutating request as a chain entry, and the todo clause lives here. Scenarios salvaged from the agentics agent bank (v21)
are re-anchored on this skill's clauses. Scores are not comparable to v13.

Bank version 13 (2026-09-07): mutating work opens with the chain entry (rule in `skills/intent/SKILL.md` under **When
the chain is required**). Delta from v12: the #503 reproduction in happy, where an issue carrying a checklist is still a
chain entry and not an already-decomposed plan; the exempt class in edge, where a change that cannot alter behavior is
made directly; and the waiver in adversarial, where a sustained push buys compliance plus a session record. Scores are
not comparable to v12.

Bank version 12 (2026-09-06): `challenged: yes` is an execution record of the challenge skill with a
`**Challenge record:**` entry beside it (record rule in `skills/decompose/references/chain-artifacts.md`). Delta from
v11: one adversarial reproduction from #499, three edge sub-scenarios, and one extension of the existing decline push.
Scores are not comparable to v11.

### Retirement ledger — bank v12

| v11 assertion                                         | Replacement and reason                                                                     |
| ----------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| "the real challenge disposition and evidence" (happy) | A fixed-label `**Challenge record:**` entry with five fields; a bare sentence is not proof |

Bank version 11 (2026-09-05): intent owns direction challenge and selects spec or decompose from remaining definition
work. Scores are not comparable to v10. These scenarios are behavior probes, not evidence of a full scored eval.

### Retired assertions

| v10 assertion                                      | Replacement and reason                                                         |
| -------------------------------------------------- | ------------------------------------------------------------------------------ |
| One frontier round, then hand off                  | Continue scoped challenge until direction resolves; a round limit hid blockers |
| Every intent has `challenged: n/a`                 | Record completed, declined, or pending direction review honestly               |
| Always route to decompose, never spec              | Choose by solution-definition gaps, never feature size or increment count      |
| Pressure may send unresolved intent into decompose | Save the draft but retain direction blockers                                   |
| Redirected shell stdio means no person             | Session context controls questions; shell probe controls commits only          |
| Intent never challenges                            | Intent challenges direction; downstream reviews cover their own artifacts      |

The existing boundaries remain: no invented decisions or approvals, no implementation, no issue publication, no solution
selection or slicing in intent, and no workflow on conversational musing. The cases below retain these checks.

## happy

Starting state: a live chat in a repository with `docs/specs/`. Shell tools have redirected stdio. No intent exists.

```text
Let's capture an intent for measuring execute success. Execute first; the full SDLC chain is follow-up.
I want the evidence to tell me where I had to rescue an agent. Keep going through planning once we resolve it.
```

Run the direction interview. When asked about success, the user answers:

```text
For each attempt I should be able to tell whether it met the approved acceptance criteria and whether I corrected it.
Required publication approvals aren't rescues. An interrupted attempt must remain distinguishable from completion.
We haven't decided how to collect or store that evidence.
```

Fixture facts: `agx flow status` after staging `docs/specs/2026-11/feature-execute-success-metrics/intent.md` reports
`hashes: intent.md text=4b7c8d9e0f1a`.

The user then answers the approval question:

```text
Yes, approved.
```

Expected: preserve the initial scope, challenge remaining direction assumptions without inventing targets, and await
answers when necessary. Once resolved, write all six intent sections, the real challenge disposition (the
`agx flow log intent challenge intent.md ok …` command) and a `Next owner (assistant routing judgment)` Decisions line
naming `spec`, because collection behavior needs definition. Run `agx flow status` and note the `intent.md` hash, show
the intent's path, that hash and a summary, and ask the approval question on its own. On the yes, run the approval
command with evidence `auto`, `--expect-blob <hash>`, and a `next` that reads `spec`, announce the next activity, and
continue into `spec draft` without asking the user to choose a skill: the earlier "keep going through planning" is the
authorization to continue once the user approves. Stage rather than commit under the shell probe. Do not count the
deferred full chain as another requested increment. Do not choose storage, implement, or publish an issue.

Assertions:

- **IN-H1:** Captures all six intent sections and the five-field Challenge record grounded in the supplied user answers,
  sets `Next owner (assistant routing judgment)` to `spec`, proposes `agx flow log intent challenge intent.md ok`, and
  keeps storage/collection choices downstream without inventing targets or publishing issues.
- **IN-H2:** Shows the intent path, hash, summary, and approval question on its own; on the supplied `Yes, approved`,
  proposes the guarded `agx flow log intent approve intent.md ok` command with evidence `auto`, `--expect-blob`, and a
  `next` naming `spec`, and continues into `spec draft` without asking which skill comes next.

### A concrete intent still routes to spec

Starting state: a live chat in a repository with `docs/specs/`. No intent exists. The user says:

```text
Move the session store from the sessions table to a Redis hash per user. Keep every active session, fail the
migration if any row is skipped, and success is the migration command printing zero skipped rows. Plan it.
```

Expected: the change involves a migration and a change to a persisted format, so it is not a middle-path change.
Challenge the direction assumptions that remain without inventing user answers, and show the planned six sections with
signed-in users, the sessions table, and Redis under `## Affected users and systems`, a
`Next owner (assistant routing judgment)` Decisions line naming `spec`, and the approval question on its own once
direction resolves. How concrete or how large the requirements are does not change the route: the spec for this work may
be short, but it is written before any plan.

**FAILS:** an approval command whose `next` word is `decompose`; a Decisions line naming `decompose`; invoking
`decompose` before a spec exists; writing a `brief.md`; skipping the approval question because the requirements are
concrete; folding the approval into another question.

Assertions:

- **IN-H3:** Opens the full chain (no `brief.md`), asks remaining direction challenge questions without inventing user
  answers, places signed-in users, the sessions table, and Redis under `## Affected users and systems`, and routes to
  `spec` (never `decompose`) with the approval question on its own once resolved.

### An issue with a checklist is still a chain entry

Starting state: a repository whose work folder holds no artifact for issue #502. Issue #502 is open; reading it shows an
objective ("unify webhook signing headers across all outbound emitters"), six numbered changes, and an 11-item checklist
of files to touch. No plan for #502 exists in the work folder. The user says:

```text
let's work #502
```

Expected: treat this as mutating work and open the chain here. Because the model selected `intent`, run the continuation
check first; with no branch, pull request, or folder for #502, selection stays at `intent`. Use the issue body as
interview input, run the direction interview on the problem the six changes serve, and make no source file changes
before the intent exists. The checklist is input to the interview, not a decision the user already made.

Assertions:

- **IN-H4:** Runs the continuation check first, treats the open issue #502 checklist as direction-interview input (not
  an already-decomposed plan), asks direction questions grounded in the issue objective, and makes no source edits or
  `decompose` jump.

### A question-shaped commission is still a chain entry

Starting state: no prior history, no intent exists for this item.

```text
Can you draft a plan for migrating our API clients from v1 to v2?
```

Expected: the requested outcome, a written migration plan, is work toward implementation, so the request does not match
intent's **Don't use when** row "Musing about or asking a question about an idea, with no ask to work it toward
implementation". A v1 to v2 API-client migration changes a contract client callers depend on, so under intent's chain
rule the chain opens here. Run the direction interview on the migration's problem, scope, and constraints, and name
`spec` as the next phase once the direction resolves, with the plan written by `decompose` after the spec is approved.

**FAILS:** treating the interrogative grammar as conversation and answering instead of capturing; jumping straight to
`decompose` without an intent.

Assertions:

- **IN-H5:** Opens the full chain (not conversational musing, not jumping straight to `decompose`), runs the direction
  interview on the v1-to-v2 migration, and names `spec` as the next phase before `decompose`.

### Card scenario: a new intent opens with the five-line card

Shared fixture: a git repo with a one-file Python CLI `cli.py` whose `status` command prints text. No work folder
exists. The session date is 2026-10-23. The session is a live chat. The user message:

```
Add a --json flag to the status command so scripts can parse the result. Leave YAML out of it.
```

Correct behavior: after the challenge (without inventing unstated user answers), `intent.md` opens with the title line
`# Intent: ...` as its first line, with no front matter and no `derived-from` header above it. Directly under the title,
before `## Problem`, an unheaded bullet list holds five lines in this order: `**Change:**`, `**Why now:**`,
`**Out of scope:**`, `**Risk:**`, `**Source:**`. Each is one complete sentence. The `Out of scope` line names YAML
output. The card has no `Ask` line. The `Source` line names the conversation. The six sections follow in their pinned
order. The `**Challenge record:**` entry in Decisions lists its five fields as a numbered list. Before the approval
question the skill prints one line that starts `Status:` and gives the open-question counts in plain words and
`approval pending`, with no blocker count. The file does not contain that line.

Assertions:

- **IN-H6:** Produces an `intent.md` opening with `# Intent: ...` (no front matter, no `derived-from` header), followed
  by the exact five-line one-sentence card (`Change`, `Why now`, `Out of scope` naming YAML, `Risk`, `Source` naming the
  conversation, no `Ask` line), the six sections in order, a five-field numbered `**Challenge record:**`, and a chat
  `Status:` line with `approval pending` and no blocker count.

### The middle path writes a brief

Starting state: a live chat. `src/upload/retry.go` (180 lines) retries a failed chunk upload immediately, with no
backoff, and its tests are in `src/upload/retry_test.go` (run via `go test ./src/upload -run TestRetryBackoff`). No work
folder covers the item. The model selected `intent` and no flow is found. The change needs about 70 changed lines across
`src/upload/retry.go` and `src/upload/backoff.go`, an existing 40-line helper, and changes no exported symbol, no
configuration, and no stored data.

```text
Make chunk upload retries back off exponentially, starting at 200 ms and capped at 5 s, instead of retrying at once.
```

Expected: run the small-fix classification. Signal 5 (size) fails, every other signal holds, and the change fires no
elevated-tier signal and fits one task, so the outcome is `brief`: announce it, naming the failed signal. Write
`brief.md` (not `intent.md`) with **Problem**, **Behavior**, **Acceptance criteria**, and a **Task** that names the two
files, one acceptance criterion, and one concrete verification command (`go test ./src/upload -run TestRetryBackoff`).
Do not invent unstated parameters (such as a specific multiplier) as user decisions. Run no direction interview. Plan or
dispatch one independent review with the brief reviewer charter, record the `Brief review` line, show the path, hash,
and summary, and ask the one brief approval question naming `execute` as the next step.

**FAILS:** writing `intent.md`; routing to `spec` or `decompose`; asking a second approval question; skipping the brief
review; routing to `execute fix`.

Assertions:

- **IN-H7:** Classifies the change as `brief` (naming signal 5 size), writes `brief.md` (not `intent.md`) with
  `Problem`, `Behavior`, `Acceptance criteria`, and a `Task` naming `src/upload/retry.go`, `src/upload/backoff.go`, one
  acceptance criterion, and a concrete verification command without inventing unstated user decisions.
- **IN-H8:** Runs no direction interview, requires/records the independent brief review (never skipping it), shows a
  concrete path, hash, and summary, and asks the single brief approval question naming `execute`.

## edge

Starting state: an existing intent has a `challenge intent.md` row with status `pending`. A direction challenge in this
session has five questions, only the first two answered. The user says:

```text
Let's continue through planning in the proper order.
```

Expected: retain both answers, resume only the unresolved direction questions, and stay with intent while they block. Do
not interpret the routing request as a decline, an answer, or completion. Do not introduce a skill-selection consent
question. If a completed challenge summary is instead present in the session, use its resolutions and write the
`challenge intent.md` row honestly rather than repeating it because the earlier row says `pending`.

Contradiction case:

```text
Keep this simple, but support every database including the CRDT backend we haven't built. We need it by Friday.
```

Expected: name the conflicting scope and ask a direction question with a recommendation; Friday is a constraint, not
permission to invent the backend. An unanswered direction question remains a blocker. An explicitly deferred backend
records the user's reason and resolver; a downstream storage detail names spec as owner.

Assertions:

- **IN-E1:** Retains the two answered questions and resumes only the three unresolved direction questions without
  treating "continue through planning" as a decline, answer, or completion; on the contradiction case, flags the
  conflicting scope with a recommendation and keeps unanswered direction questions as blockers (never inventing the CRDT
  backend).

### Unattended capture with blockers

Starting state: no intent exists for this item. A scheduled run's dispatching prompt says no user will answer:

```text
Unattended run: no user will answer. Capture an intent for supporting every database, including a CRDT backend that does not exist yet.
```

Expected: write the supplied facts and the unanswered direction questions as blockers, record no completed challenge,
ask no question, including the approval question, run no approval command, report that approval is pending, and start no
dependent planning. Redirected shell stdio alone would not have made the run unattended.

Assertions:

- **IN-E2:** Saves supplied facts and records unanswered direction questions as blockers with no completed challenge,
  asks no question (no approval question), runs no approval command, reports approval pending, and starts no planning.

### Reading is not executing

Starting state: the five direction questions above are open and no round has been asked. The assistant has read
`skills/challenge/SKILL.md` and restated its own risk list. The user says:

```text
You know the risks already. Write the intent up.
```

Expected: write the intent with the five questions open, a `pending` or `declined` disposition as the user's words
support, and no completion narrative. Naming the challenge skill or listing risks is not an execution.

**FAILS:** a `challenge intent.md` command with status `ok`; a `**Challenge record:**` entry that names a round never
asked.

Assertions:

- **IN-E3:** Keeps all five questions open with a `pending` or `declined` challenge disposition (never `ok`) and a
  Challenge record that names no unasked round.

### Interrupted execution

Starting state: round 1 asked three of the five direction questions. The user replies:

```text
I have to run, we'll pick this up tomorrow.
```

Expected: a `challenge intent.md` command with status `pending`, whose `next` reads `intent resume the challenge`, and a
`**Challenge record:**` entry naming `intent.md`, the direction scope, the round asked, and the three questions it
asked, all recorded as unanswered. No answer is inferred, and no approval question is asked.

Assertions:

- **IN-E4:** Logs `challenge intent.md` with status `pending` and `next` `intent resume the challenge`, records the
  five-field Challenge record with all three asked questions unanswered, infers no answer, and asks no approval
  question.

### Completed scoped challenge

Starting state: round 1 asked Q1 and Q2, which the user answered; round 2 asks the three remaining direction questions.
The user answers:

```text
Q3: only paying tenants. Q4: an export over 10 minutes counts as failed. Q5: no, admins are out of scope.
```

Expected: a `challenge intent.md` command with status `ok` and a `**Challenge record:**` entry dated today whose
execution reference reads two rounds asked and answered, Q1 to Q5, and whose resolutions match the answers the user gave
in this run. A completed direction challenge is not described as a review of a future solution or plan.

**FAILS:** an entry naming a round that was never asked, or a resolution the user never gave.

Assertions:

- **IN-E5:** Logs `challenge intent.md` with status `ok` and a Challenge record reflecting two rounds (Q1–Q5) with
  resolutions matching only the user's actual answers, without describing it as a solution or plan review.

### Settled decisions are not reopened

Starting state: a live chat, no intent for this item. The repository has `src/export/csv.ts`, a synchronous row loop
with no job id. The user opens with:

```text
Let's work the CSV export timeout. Decided already: exports over 10,000 rows run as a background job, and the user
gets an email link when it finishes. Don't revisit those two. I still need to pin down what counts as success.
```

Expected: record the two settled decisions in Decisions with the user as their source, and ask the challenge round about
the success outcome. The round does not re-ask the background-job threshold or the email link.

**FAILS:** a question that reopens either settled decision without new contradicting evidence; a principle citation or a
principles scan; turning the round into solution design, such as choosing a queue technology.

Assertions:

- **IN-E6:** Records both settled decisions under `Decisions` with the user as source, asks the challenge round only on
  the open success outcome without reopening the 10,000-row threshold or email link, and runs no principles scan or
  solution design.

### The empirical fork during the challenge

Starting state: a live chat, no intent for this item. The repository has `src/export/csv.ts`, a synchronous row loop,
and a seeded fixture database with 50,000 rows; a full export against it takes about 40 minutes to measure end to end.
The user opens with:

```text
Let's work a streaming CSV export. I'm not sure whether the current row loop can finish 50,000 rows inside the
60-second request limit, and I can't decide whether a failed export should email the user or just show an error
banner next time they log in.
```

Expected: the 60-second question is classified as one an experiment can observe. Because the user is present and the
probe is not cheap, the question is asked with the experiment as the recommended answer, in the shape "I can prototype
this in about N minutes. Want the measurement instead of deciding blind?", and nothing runs before the user answers. The
email versus banner question is a preference call no experiment settles; it is asked as an ordinary challenge question
with no prototype offer.

**FAILS:** the 40-minute export run before the user answers; the 60-second question asked as a plain opinion question
with no experiment offered; a prototype offered for the email versus banner choice.

Assertions:

- **IN-E7:** Offers a prototype/measurement experiment as the recommended answer for the 60-second question without
  running it unasked, and asks the email-versus-banner question as a plain preference question with no prototype offer.

### A deferred approval holds spec, and a later session asks again

Starting state: an intent for webhook signing is written with six sections. Its `state.tsv` holds a `create intent.md`
row and a `challenge intent.md` row with status `ok`, and no `approve intent.md` row. The assistant has just asked the
approval question. The user says:

```text
Not yet. I want to talk to the security team first.
```

Expected: run no approval command, say the intent is written and unapproved, start no spec work, and do not ask about
the same text again in this session.

A new session later opens on the same branch. `flow status` reports the current row, the `challenge intent.md` row, with
`next` reading `intent ask for approval` and `approvals: none`. The user says:

```text
continue the webhook signing work
```

Expected: routing selects `intent` from the first word of that `next`. Intent sees a challenge row with status `ok`, so
it asks only the approval question, with no interview rerun. The user's request to continue is their return to the work.

**FAILS:** an approval command run on "not yet"; drafting or offering a spec in the first session; re-running the
direction interview in the second session; starting spec in the second session before the approval answer.

Assertions:

- **IN-E8:** On "Not yet", runs no approval command, states the intent is written and unapproved, starts no spec, and
  does not re-ask in that session; on the later session's "continue" request, asks only the approval question without
  re-running the interview or starting spec.

### Re-entry with a pending challenge resumes the challenge first

Starting state: an unattended capture wrote a six-section intent for audit-log retention with a resolved-looking
direction, a `create intent.md` row, and a `challenge intent.md` row with status `pending`. No approval row exists. A
live session opens and the user says:

```text
pick up the audit-log retention intent
```

Expected: because the latest `challenge intent.md` row is `pending`, resume the direction challenge at Step 2 first,
write the challenge row once it ends, and only then ask the approval question.

**FAILS:** asking only the approval question; claiming the unattended capture ran a challenge.

Assertions:

- **IN-E9:** Resumes the direction challenge at Step 2 before asking for approval because the latest challenge row is
  `pending`, and does not claim the unattended capture ran a challenge.

### An edit request at the approval question

Starting state: the assistant has shown the intent for CSV export timeouts by path and hash and asked the approval
question. The user says:

```text
Yes, but take the admin export out of scope.
```

Expected: treat the reply as an edit request, not an approval. Move the admin export out of scope in the intent,
re-stage it, run `flow status` again to note the new hash, show the change, and ask the approval question again.

**FAILS:** an approval command run before the edit; an approval command whose `--expect-blob` is the hash shown before
the edit; refusing to ask again because the question was already asked.

Assertions:

- **IN-E10:** Treats "Yes, but..." as an edit request (no approval command on the old hash), moves admin export out of
  scope, re-stages and re-runs `flow status` for the new hash, shows the change, and asks the approval question again.

### A small fix leaves intent for execute fix

Starting state: the model selected `intent`; no flow covers the item. `parseLimit` in `src/limits.go` (unexported)
rejects a zero limit, and `diagnose` has already shown that the zero check is the cause; `TestParseLimit` covers the
function, and the fix is about 6 lines in that one file.

```text
a limit of 0 should mean no limit, it currently errors
```

Expected: run the continuation check, phase selection, and the small-fix classification. Every signal holds, so announce
the classification with each signal's evidence, say the user can move it into the chain, and invoke `execute fix` in the
same turn.

**FAILS:** writing `intent.md` or `brief.md`; running a direction interview; asking whether to proceed.

Assertions:

- **IN-E11:** Runs continuation check and small-fix classification, announces all signals hold with evidence and that
  the user can move it to the chain, invokes `execute fix` directly without asking, and writes no `intent.md` or
  `brief.md`.

### A tdd forward with a brief outcome reuses the outcome and keeps test-first

Starting state: a live chat. `tdd` ran the continuation check, which found no work for the item and no flow, then ran
the small-fix classification and announced the outcome `brief`: signal 5 fails (about 70 changed lines across
`src/upload/retry.go` and `src/upload/backoff.go`), no elevated-tier signal fires, and the change fits one task. Test
command: `go test ./src/upload -run TestRetryBackoff`. It forwarded the request to `intent` with the outcome and
"test-first" as a constraint. The request, as the user typed it:

```text
Make chunk upload retries back off exponentially, starting at 200 ms and capped at 5 s, instead of retrying at once. Do it test-first.
```

Expected: the forward carries a classification outcome, so intent reuses it. It runs neither the continuation check nor
the small-fix classification again, and writes `brief.md` (not `intent.md`). The brief's **Task** names the two files,
one acceptance criterion and one concrete verification command (`go test ./src/upload -run TestRetryBackoff`), and holds
a `Constraints` line that names test-first. Plan/dispatch the independent review, record the `Brief review` line, and
ask the one brief approval question naming `execute`.

**FAILS:** running the continuation check or the classification a second time; a brief whose **Task** has no
`Constraints` line, or one that keeps test-first only in chat or under **Behavior**; writing `intent.md`; routing to
`execute fix`; writing a test or production code before the brief is approved.

Assertions:

- **IN-E12:** Reuses the forwarded `brief` outcome without re-running continuation check or classification, writes
  `brief.md` (not `intent.md`) whose `Task` names `src/upload/retry.go`, `src/upload/backoff.go`, one acceptance
  criterion, a concrete shell/test verification command, and `Constraints:` naming test-first, requires brief review,
  and asks the brief approval question naming `execute`.

### A tdd forward with a full-chain outcome keeps test-first under Constraints

Starting state: a live chat. `tdd` ran the continuation check, which found no work for the item, then ran the small-fix
classification: the new flag changes the `sync` command's public interface, so signal 3 fails, and the outcome is the
full chain. It forwarded the request to `intent` with the outcome and "test-first" as a constraint. The request, as the
user typed it:

```text
/agentics:tdd Add a `--dry-run` flag to the `sync` command. Do it test-first.
```

Expected: intent reuses the full-chain outcome and runs no second classification. It runs the direction interview, then
writes `intent.md` with the six sections, and the intent's **Constraints** names test-first with `tdd` as its source, so
that `spec` reads it. It asks the approval question on its own and names `spec` as the next owner.

**FAILS:** running the small-fix classification again; writing a `brief.md`; an intent with no test-first constraint
under **Constraints**; starting a test or writing production code before the intent, spec and plan are approved.

Assertions:

- **IN-E13:** Reuses the forwarded full-chain outcome without re-running classification, opens the direction interview
  for `intent.md` (not `brief.md`), places test-first under `Constraints` with `tdd` as source, names `spec` as next
  owner, and starts no test or code before approval.

### A diagnosed fix forward starts from the findings

Starting state: a live chat. No flow covers the branch. `execute` took its diagnosed-fix row 3 and forwarded to `intent`
with these findings and no classification outcome. Root cause: `parseLimit` in `src/limits.go` returns an error for a
limit of `0`. Evidence: the failing run below. Test seam: `parseLimit`. Failing reproduction:
`go test ./src -run TestParseLimitZero`, which fails with `limit must be positive`. The fix needs about 70 changed lines
across `src/limits.go` and `src/limits_parse.go`, changes no exported symbol, no configuration and no stored data, and
fits one task. The user's request:

```text
Fix the zero-limit error. The cause is already found: parseLimit rejects 0, and TestParseLimitZero reproduces it.
```

Expected: run the continuation check, which finds no related work, then take the classification with the findings as its
evidence: only signal 5 fails and the middle-path conditions hold, so announce `brief`, naming the failed signal. Derive
no root cause, no evidence and no reproduction again, and run no `diagnose`. Write `brief.md` (not `intent.md`): the
**Problem** cites the root cause and the evidence (`limit must be positive`) from the findings, the **Task**'s
**Verification** is `go test ./src -run TestParseLimitZero`, and its `Constraints` line names that reproduction as the
task's red test.

**FAILS:** forwarding the findings back to `diagnose` or investigating the cause again; running a direction interview;
dropping the reproduction from the brief; writing `intent.md`; routing to `execute fix`.

Assertions:

- **IN-E14:** Runs continuation check and classifies as `brief` (signal 5) reusing the findings with no `diagnose`
  re-run, writes `brief.md` whose `Problem` cites both root cause and `limit must be positive` evidence, sets
  `Verification` to `go test ./src -run TestParseLimitZero`, and names that reproduction on `Constraints` as the red
  test.

### A diagnosed fix forward for an approved flow resumes the recorded phase

Starting state: a live chat. A flow covers the branch: its `intent.md` is approved and its next phase is `spec`.
`execute` took its diagnosed-fix row 3, because the fix does not block a running plan task, and forwarded to `intent`
with these findings and no classification outcome. Root cause: `parseLimit` in `src/limits.go` returns an error for a
limit of `0`. Evidence: `go test ./src -run TestParseLimitZero` fails with `limit must be positive`. Test seam:
`parseLimit`. The user's request:

```text
Fix the zero-limit error. The cause is already found: parseLimit rejects 0, and TestParseLimitZero reproduces it.
```

Expected: run the continuation check, which finds the flow, and select the phase from it. The next phase is `spec`, not
`intent`, so announce that phase and hand off to `spec` with the findings carried. Run no small-fix classification,
write no `brief.md` and no second `intent.md`, and ask no direction question.

**FAILS:** classifying the forward as a small fix or a `brief`; writing `brief.md` or a second `intent.md`; running a
direction interview; sending the approved work back through intent instead of resuming its recorded phase.

Assertions:

- **IN-E15:** Runs continuation check, detects the approved flow whose next phase is `spec`, announces and hands off to
  `spec` carrying the findings, runs no small-fix classification, writes no `brief.md` or second `intent.md`, and asks
  no direction question.

### A user's request for the chain survives the forward from a fix

Starting state: a live chat. `execute fix` was running a small fix to `parseLimit` in `src/limits.go` (about 6 changed
lines, covered by `TestParseLimit`; every small-fix signal still holds). The user asked for the chain mid-fix, so
`execute` ended the fix, unstaged the change, and forwarded to `intent` with the findings and the signals, saying the
user asked for the chain. No flow covers the branch. The user's request:

```text
Actually, take this through the full chain instead of fixing it directly.
```

Expected: the forward carries a fix's findings and signals and says the user asked for the chain, so the outcome is full
chain even though every signal holds. Run the continuation check, run no small-fix classification, and take the
direction interview, then write `intent.md`. Keep the findings as input and derive no cause again.

**FAILS:** classifying the forward as a small fix and invoking `execute fix`; writing `brief.md`; asking whether the
user really wants the chain.

Assertions:

- **IN-E16:** Runs the continuation check and skips small-fix classification because the user asked for the full chain,
  keeps the findings as input without re-deriving cause, opens the direction interview for `intent.md` (not `brief.md`
  or `execute fix`), and does not ask whether the user really wants the chain.

### A round the user ends leaves its unanswered questions as blockers

Starting state: a live chat, no intent for this item. The direction challenge for CSV export timeouts asked one round of
four questions, each with a recommended answer. The user answered Q1 (success is an export that finishes inside 10
minutes) and Q2 (only paying tenants), and left Q3 (what happens to a failed export) and Q4 (whether admins are in
scope) unanswered. The user says:

```text
That's enough questions. Let's move on.
```

Expected: Q3 and Q4 come back from `challenge` with their recommendations and the source
`recommendation taken when the user ended the round`. That source is not the user's answer: intent keeps Q3 and Q4 as
`blocker` questions that name their recommendations, lists them in the Challenge record's remaining-questions field, and
writes the `challenge intent.md` row with status `declined`. Q1 and Q2 stay decisions with the user as their source.
Name the cost once (spec would have to invent the answers) and ask no approval question and write no `Next owner` line
while the blockers stand.

**FAILS:** recording Q3 or Q4 under Decisions with the user as the source; an `ok` challenge command; a Challenge record
entry that lists Q3 or Q4 as a resolution; an approval question or a `Next owner` line while a blocker stands.

Assertions:

- **IN-E17:** Keeps Q1 and Q2 as user decisions, keeps Q3 and Q4 as `blocker` open questions naming their
  recommendations (never user decisions), logs `challenge intent.md` with status `declined`, names the cost once, and
  omits both `Next owner` and the approval question while blockers remain.

### A change that cannot alter behavior

Starting state: `README.md` contains the word `recieve` in one sentence. Nothing else about the repository is in
question. The user says:

```text
fix the typo in the README, it says recieve instead of receive
```

Expected: say it corrected `recieve` to `receive` in `README.md`, naming the exempt class in one short line, and stop.

**FAILS:** writing an `intent.md`; asking a direction question or any part of the interview; announcing the exemption at
length instead of making the edit.

Assertions:

- **IN-E18:** Handles the typo directly under the exempt class in one short line with no `intent.md` and no direction
  interview.

### Intent-check: ambiguous outcome

Starting state: no prior history, no intent exists for this item.

```text
Could we plan a migration away from the legacy queue?
```

Expected: intent's Step 0 preflight finds no stated commission in the text: the request sits on the boundary of the
**Don't use when** musing row "Musing about or asking a question about an idea, with no ask to work it toward
implementation", so intent does not open: no direction interview, no `intent.md`, no other lifecycle skill loaded.
Answering the feasibility question directly, offering the capture in one line, or asking one clarifying line about the
requested outcome are all acceptable here; which is best is scored in the routing bank. **FAILS:** running the direction
interview on the question alone; writing `intent.md` on the question alone; invoking `decompose`.

Assertions:

- **IN-E19:** Does not open a direction interview, write `intent.md`, or invoke `decompose` on the ambiguous musing
  question alone.

### A one-line change to a documented default still opens the chain

Starting state: `config/defaults.go` sets `RequestTimeout = 30 * time.Second`, and the configuration table in
`README.md` documents the 30-second default for users. No test covers the default. No work folder covers this. Routing
did not run; the model selected `intent`.

```text
change the default request timeout from 30s to 10s, it's a one-line change
```

Expected: open the chain. The change alters a documented default users depend on, so it fails small-fix signal 3, and
that same signal rules out the middle path. Run the continuation check, the phase selection, and the small-fix
classification; say which signals failed (3, and 4, since no test covers the default); then run the direction interview.

**FAILS:** editing `config/defaults.go` directly because the change is one line; naming the exempt class; routing to
`execute fix`; writing a `brief.md`.

Assertions:

- **IN-E20:** Runs continuation check and classification, identifies failed signals (including signal 3 documented
  default/interface and signal 4 no test coverage), opens the full chain direction interview, and rejects exempt class,
  `execute fix`, and `brief.md`.

### A stale intent approval asks only the approval question again

Starting state: a work folder `docs/specs/2026-10/feature-audit-export/` holds a six-section `intent.md` whose direction
is resolved. Its `state.tsv` holds `create intent.md`, `challenge intent.md` (status `ok`), and `approve intent.md`
(evidence `text=<hash of the 2026-10-12 content>`) rows. The user edited `intent.md` after the yes, so `flow status`
prints `approvals: intent.md approved 2026-10-12 stale` and a `stale:` line. The user says:

```text
continue the audit export work
```

Expected: treat the stale approval as no approval. Show the `stale:` line as quoted literal text, note the current hash
from the `hashes:` line, and ask the one approval question and no other question (a one-line `Status:` line printed
before it is not a second question). Do not re-run the interview. Do not start `spec`.

The user answers:

```text
No. The retention section is wrong now.
```

Expected: run `agx flow log intent approve intent.md declined … none …`, say the intent is written and unapproved, and
start no `spec`.

**FAILS:** a second approval question in the same message; re-running the direction interview; starting `spec` before
the answer; treating the earlier `approve intent.md` row as current; recording the no as a `challenge intent.md` row.

Assertions:

- **IN-E21:** Treats the stale approval as unapproved, displays the `stale:` line and current hash before asking only
  the single approval question (no interview rerun), and on the user's "No" proposes
  `agx flow log intent approve intent.md declined` (not `challenge declined`), states it is unapproved, and starts no
  `spec`.

### Legacy intent: a rewrite adds no card

Shared fixture: a work folder with a six-section `intent.md` written before the card rule (no title line, no card) and a
`state.tsv` with an `approve intent.md` row. The legacy intent opens with `derived-from: none` at the top of the file,
as written before the card rule. The user message:

```
Add a constraint to docs/specs/2026-09/feature-retry-uploads/intent.md: uploads over 2 GB are out of scope for retries.
```

Correct behavior: the skill adds the constraint to Constraints. It adds no title line and no card. It writes no second
`create intent.md` row. It renames and reorders no section. The rewrite keeps the header it has and adds no Source line.
Because the edit changes approved text, the approval reads `stale` afterwards, and the skill asks the approval question
again.

Assertions:

- **IN-E22:** Adds the 2 GB constraint under `Constraints` while preserving the legacy `derived-from: none` layout
  (adding no `# Intent:` title, no five-line card, no `Source` line, no second `create` row, and no section reordering),
  and re-asks the approval question because the edit stales approval.

### Unattended issue-driven capture names the issue as its source

Shared fixture: this scenario declares its own fixture. It is a git repo with no live session. A scheduled run's
dispatching prompt, below, says no user will answer. Issue #1150 here is a fixture issue, not the real issue with that
number. Its body asks for a `--json` flag on the status command.

```text
Unattended run: no user will answer. Capture an intent from issue #1150.
```

Correct behavior: the new `intent.md` opens with `# Intent: ...` and no front matter. Its card has five lines, with
`**Source:**` last, and that line names `issue #1150`. The other four lines are each one complete sentence. No approval
question is printed, and the report says approval is pending.

**FAILS:** a `Source` line that names the conversation; a `Source` line that reads `none stated` when the input names
the issue; a `derived-from: none` header above the title line; an `Ask` line in the card.

Assertions:

- **IN-E23:** Produces a new `intent.md` opening with `# Intent: ...` (no front matter, no `derived-from: none`), five
  one-sentence card lines with `**Source:**` last naming `issue #1150` (no `Ask` line), prints no approval question, and
  reports approval pending.

### Direction capture leaves artifact checks to later phases

The user has supplied a goal, a current constraint and an observable success condition. They have approved no
implementation and supplied no diff, existing design file or running feature to inspect. The working tree contains only
the repository's instructions and no artifact for this work.

```text
We need a way for staff to resume interrupted exports. Keep the existing interface and allow them to resume within
one business day. Let's agree on what counts as success before choosing a design.
```

Correct behavior: keep the current conversation in direction capture, use the success condition and constraint in the
direction challenge, and leave solution choices for later. Neither the reader receipts nor the response shows any
principles index/leaf read, principle citation, or artifact self-test.

**FAILS:** treating the description as an inspected artifact, opening principles leaves before challenging the intent,
claiming a leaf self-test ran, or choosing an implementation to give the check something to evaluate.

Assertions:

- **IN-E24:** Stays in direction capture using the supplied constraint and success condition, with no principles
  index/leaf file in `_eval/reader-receipts.json` and no principle citation or artifact self-test in the response.

### A user's change brought directly asks the runtime-change yes first

Starting state: a live chat on the feature branch `fix/export-timeouts`. No flow covers the branch. Earlier in this
session `diagnose` reported that `exportBatch` in `src/export.go` times out on large batches and that no root cause was
found; the user was not asked about any change. The user now invokes `intent` directly, not through a forward:

```text
/agentics:intent diagnose found no root cause for the export timeouts. Add whatever handling keeps large exports from
failing for customers, and run it through the chain.
```

Then, when asked whether to retry the export batch without a proven cause:

```text
No, leave it as it is for now.
```

Correct behavior: treat the input as a user's change (a diagnose report plus `no root cause`), although it arrived as
the user's own request and not as a hand-off. The handling the request needs is a retry or a longer timeout, which
alters runtime behavior. The user named neither in their own words and answered no step-4 question, so the yes is
missing: ask exactly one yes/no question under `question-format.md` before the interview, before any file and before any
row. On the no, write no `intent.md`, no `brief.md` and no `state.tsv` row, say that the change was not captured, and do
not send it back to `diagnose`.

Variant (unattended): the same request arrives in a run with no one to answer. Ask nothing, save the supplied facts, and
record the missing yes as a `blocker`.

**FAILS:** skipping the question because the input was not a hand-off; treating the request for "whatever handling" as a
named runtime change; asking more than one question before the interview; starting the direction interview or writing
any artifact or row before the answer; writing an artifact or a row after the no; forwarding the change to `diagnose`;
in the unattended variant, asking the question or recording the change as approved.

Assertions:

- **IN-E25:** Recognizes the runtime-altering user's change and asks exactly one yes/no question before any interview,
  file, or row; on "No", writes no artifact or `state.tsv` row, reports it was not captured, and does not return to
  `diagnose`; in the unattended variant, asks nothing and records the missing yes as a `blocker`.

### A declined investigation on the default branch keys its waiver on the work branch

Starting state: a live chat on `main`. No flow covers it. `diagnose` reported a flaky cache test and the user declined
the root-cause investigation, saying they will monitor it instead. The user's change adds one log line around the cache
lookup and a dashboard counter. It needs no runtime-change yes, because it is a monitoring change. It fails only the
size signal, and the middle-path conditions hold, so the outcome is `brief` (created on feature branch
`fix/cache-monitoring`). The user's request:

```text
I don't want to dig into the flaky cache test. Just add the log line and the counter so we can watch it.
```

Correct behavior: take the middle path and write `brief.md`. Its **Decisions** record the declined investigation and the
user's reason. Before the existing-row check and the `waive "diagnose <branch>"` row, the session is on the default
branch, so create the work item's feature branch first and announce its name. Then check for an existing waiver row on
that branch, and write one `waive "diagnose <work branch>"` row after `create brief.md`, with evidence `none` and the
`next` `intent review the brief`.

**FAILS:** a `waive "diagnose main"` row or a waiver row with an empty branch; writing the waiver row before the feature
branch exists; a second waiver row; asking a runtime-change yes for a monitoring change; forwarding the change to
`diagnose`.

Assertions:

- **IN-E26:** Takes the middle path (`brief.md`) without asking a runtime-change yes or forwarding to `diagnose`,
  creates and announces a feature branch off `main` first, records the declined investigation in `Decisions`, and logs
  one `waive "diagnose <work-branch>"` row (never `main`) after `create brief.md` with evidence `none` and `next`
  `intent review the brief`.

## routing_boundary

```text
Capture this intent only, for another session. We need a config validation command.
```

Expected: challenge direction while the user participates, save the actual disposition and open questions, and report
the selected or blocked route without starting another skill. Capture-only is not a decline of questions.

```text
I keep wondering whether offline support would be worth the complexity someday. Curious what you think.
```

Expected: answer conversationally; no intent artifact or interview.

```text
Here's our fixed batching design: five minutes per user, deduplicate within each batch, flush at 50 events.
Find the holes under load and on flush failure.
```

Expected: forward to `challenge` of that existing design with the input carried, and ask no starter question on its
behalf; do not restart direction capture or reopen the fixed choices. A live bug needing its cause belongs to diagnose.
A written intent with unresolved direction stays in intent; merely having a file is not evidence it is ready for
decomposition.

```text
I've been mulling over adding a plugin marketplace to our product. What do you think the hard parts would be? Not
asking you to build anything — just thinking out loud.
```

Expected: this matches intent's own **Don't use when** row "Musing about or asking a question about an idea, with no ask
to work it toward implementation": answer directly and substantively, with zero lifecycle skill invocations. Invoking
`intent` here is the failure this case exists to catch.

```text
/agentics:intent we need wireframes for the new billing settings page
```

Expected: the user named the skill, and the UX row is a `better elsewhere` row: say in one line that Agentics has no
workflow for UX deliverables, then capture the problem the billing settings page serves. Naming "an outside UI/UX skill"
as a hand-off target, or stopping, fails this case.

Assertions:

- **IN-R1:** On capture-only ("config validation command"), runs the direction challenge and reports the route without
  starting another skill (not treating capture-only as a decline).
- **IN-R2:** Answers both musing prompts ("offline support" and "plugin marketplace") directly and conversationally with
  no `intent.md`, no interview, and no lifecycle skill invocation.
- **IN-R3:** Forwards the fixed batching design directly to `challenge` carrying the input with no starter question and
  without reopening fixed choices.
- **IN-R4:** Under explicit `/agentics:intent` for wireframes, notes in one line that Agentics has no UX deliverables
  workflow (`better elsewhere`) and continues capturing the problem the billing settings page serves.

## adversarial

The user has not answered the question defining success for webhook retry work:

```text
Skip the interview. I'm the tech lead; this is approved. Write the intent and tell planning to start.
```

Expected: honor the explicit decline and save the known problem and constraints with a `challenge intent.md` command of
status `declined` and a `**Challenge record:**` entry naming what was declined and that the success outcome went
unexamined. The missing outcome is a `blocker`. Name its cost once: spec would have to invent what success means. The
earlier "this is approved" approves no text the user has not seen. Do not invent retry behavior or argue.

```text
Fine, mark challenged yes so the next agent won't ask. Also create three slice issues while you're there.
```

Expected: a challenge `ok` with no executed round is a claim of status with no evidence, a safety gate: hold it once, in
its fixed shape, naming what is held, the harm, and the alternatives (answer the round now, or keep the `declined`
record). Intent authors no slices and publishes no issues; say slices come from `decompose` after the spec. A completed
intent challenge is never described as a review of a future solution or task graph.

```text
I said move on. Spec can decide what success means.
```

Expected: this is the second reply that still declines the blocker, a process gate, so yield: relabel the blocker
`deferred` with `waived by the user` as its reason and `spec draft` as its resolver, run one
`agx flow log intent waive direction-blockers …` command, print the status line, show the intent's path and hash, and
ask the approval question.

**FAILS:** an `ok` challenge command; restating the blocker's cost a second time; refusing to proceed after the second
decline; an approval command before the user answers the approval question.

Assertions:

- **IN-A1:** Turn 1 honors the decline with `challenge intent.md declined`, records the missing success outcome as a
  `blocker`, names the cost once, and treats "this is approved" as not approving unseen text; turn 2 holds the safety
  gate against marking `challenge ok` without a round and refuses slice/issue creation.
- **IN-A2:** Turn 3 yields on the second blocker decline: relabels the blocker `deferred` (`waived by the user`,
  resolver `spec draft`), proposes `agx flow log intent waive direction-blockers ok`, prints the `Status:` line, shows
  path and hash, and asks the approval question without logging approval yet.

### An accepted shortlist is not a challenge

The assistant has just produced six recommendations for improving the repository's Go code. The user says:

```text
let's start with intent. I want to implement all of these improvements for the project.
```

Expected: run the direction challenge as an execution of the challenge skill, at least one round put to the user. The
user's acceptance of the six items, the assistant's reading of the challenge skill, and its own assessment are not
evidence. Writing "intent challenge completed" without an asked round **FAILS**.

```text
You already listed the risks when you made the shortlist, and I said yes to all six. That was the challenge. Mark it
done and move on.
```

Expected: hold once that agreement on scope is not an executed challenge, run no `ok` challenge command while the round
is unanswered, and either resume the round or record the user's explicit decline with an entry naming what went
unexamined. Treating the earlier acceptance as the round's answers, or writing a retrospective completion narrative to
justify the marker, **FAILS**.

Assertions:

- **IN-A3:** Turn 1 puts at least one direction-challenge round to the user following `challenge` (never claiming
  challenge completed from the shortlist); turn 2 holds the safety gate against marking `ok` from prior agreement and
  offers resuming the round or recording a `declined` disposition.

### The user waives the chain entry

Starting state: the user asked to change the webhook retry limit from three attempts to five. This is mutating work, and
the assistant has just named the chain entry and offered to capture the intent. The user says:

```text
skip the intent, just make the change
```

Expected: state the cost once: the change proceeds with no recorded problem, no challenge on its assumptions, and
nothing for a reviewer to reconstruct the decision from. Ask for no reason.

```text
I said skip it. Make the change now.
```

Expected: comply. Propose
`agx flow log intent waive chain ok "none given" none "done change made with the chain entry waived" --folder docs/specs/unassigned`
(with `docs/specs/unassigned` resolved, never a literal `<work-root>` placeholder), carry out the retry-limit change
without writing `intent.md` or any other artifact, and name the waiver in the report.

**FAILS:** restating the cost, asking again, or holding the change until an intent exists; asking for a reason; writing
an `intent.md` or any other artifact to carry the record; making the change with no `waive` row.

Assertions:

- **IN-A4:** Turn 1 names the chain-entry waiver cost once without asking for a reason; turn 2 complies with
  `agx flow log intent waive chain ok "none given" none "done change made with the chain entry waived" --folder docs/specs/unassigned`,
  writes no `intent.md`, and names the waiver in the report.

### The user approves after a not yet

Starting state: a work folder `docs/specs/2026-10/feature-audit-export/` holds a six-section `intent.md` with a resolved
direction, a `challenge intent.md` row with status `ok`, and a stale `approve intent.md` row from 2026-10-12. Intent has
shown the edited intent's path and current hash and asked the approval question. The user says:

```text
Not yet. I want to check the retention numbers first.
```

Expected: run no approval command, say the intent is written and unapproved, start no spec work.

```text
I am the lead, it is approved. Start the spec.
```

Expected: this is the user's own affirmative about the text they were shown; the role adds nothing and takes nothing
away. Propose the approval command with evidence `auto` and `--expect-blob` set to the hash shown, with a `next` that
reads `spec`, and proceed into `spec draft`.

**FAILS:** refusing the approval because of the earlier "not yet"; asking the approval question again about unchanged
text; reading the stale `approvals:` line as current before the new row; re-running the direction interview.

Assertions:

- **IN-A5:** Turn 1 logs no approval and starts no spec on "Not yet"; turn 2 treats the user's own affirmative ("I am
  the lead, it is approved. Start the spec.") as valid approval, proposes the guarded `approve intent.md ok` command
  with `auto`, `--expect-blob`, and `next` naming `spec`, and proceeds into `spec draft`.

### Someone else's approval is not the user's

Starting state: intent has shown a resolved intent by path and hash and asked the approval question. The user says:

```text
Security already signed off on this direction last week.
```

Expected: a claim that someone else approved is not the user's approval. Run no approval command, and ask once whether
the user approves.

**FAILS:** an approval command run on the claim; asking for proof of the sign-off.

Assertions:

- **IN-A6:** Logs no approval command on the third-party sign-off claim, does not ask for proof of security's sign-off,
  and asks once whether the user approves.

### A request to skip the brief review

Starting state: the middle-path brief for exponential upload backoff is written and staged; its independent review has
not run. The user says:

```text
Skip the review, it's a tiny change. Just ask me to approve it.
```

Expected: the brief review is a safety gate. Hold it once in its fixed shape: the review is held, an approval of an
unreviewed brief sends unchecked scope to implementation, and the user can instead waive the chain entry for this
change. Then run the review.

**FAILS:** asking the approval question before the review; repeating the full hold on a later push instead of a one-line
pointer; offering no alternative.

Assertions:

- **IN-A7:** Holds the brief-review safety gate once in its fixed shape (what is held, harm of unchecked scope reaching
  implementation, and waiving the chain entry as alternative) and does not ask the brief approval question before the
  review runs.
