# Approval, authorization and pushback each have one definition

The skills answer the same user sentence differently depending on which skill hears it. "Just do it" is a yes in one
skill, a question to repeat in another and a refusal in a third. A push named in the request counts in one skill and not
in the next. About twenty skills carry their own script for holding a line under pressure, and most of the lines they
hold protect a process step, not a safety property. This record gives one definition each to approval, an edit request,
asking again, authorization given up front and confirmation words. It sorts every gate into one of three classes, safety
gate, repository policy or process gate, gives each a defined exit, and names the one form in which an override is
recorded.

The problems come from the skill audit in `docs/designs/skill-audit-2026-11/consolidated-issues.md` (themes T2, T3 and
T9). The owner decided two of them: draft-only pull requests are a repository policy that a settings file switches, and
triage pushes and posts without asking when its invocation or the request named those steps.

Three neighbouring records decide the other side of an overlap. How an approval row proves where it came from is decided
in 0017. Which third-party text a skill may act on is decided in 0018. How a question is formatted is decided in 0019.

## Rules

### An approval is the user's affirmative reply about a named artifact, and a revision reopens the question

- **Answers:** C2.1
- **Rule:** An artifact is approved when the user, in this session, answers an approval question about it affirmatively
  or tells the agent in their own words to proceed on it. The wording, the language and the form do not matter:
  `/execute <plan>`, "looks good, build it" and "just do it" in reply to the question all approve, whichever skill or
  route is running. A claim that someone else approved is not the user's approval, and the agent asks the user once
  whether they approve. "Yes, but change X" is an edit request: the agent makes the change, shows it, and asks again,
  and the approval binds to the revised text. The limit on asking is per version of the content, not per session: after
  a no or a "not yet" the agent does not ask again about the same text, any revision reopens the question, and the user
  can approve later at any time by saying so. "Just do it" has one meaning for each situation: in reply to an approval
  question it is a yes, in reply to a process gate it declines that gate, and with nothing pending it tells the agent to
  continue on its stated defaults and approves nothing. The current text still says "Do not ask again this session" ten
  lines above a re-entry step that asks, and execute still sends a plain-words approval back to `decompose`
  (`skills/execute/SKILL.md:203`). (verified: skills/intent/SKILL.md:278, 2026-11-11)
- **Default / Rejected:** Any clear affirmative approves, and the ask-once limit follows the content; rejected: a fixed
  approval phrase or the slash form only, because the user's meaning is the same in both forms and the fixed forms are
  what produced three protocols for one sentence; rejected: ask-once per session, because the "no" branch of spec and
  decompose schedules a revision in the same session and then may not ask about it.
- **Later work changes:** `skills/intent/SKILL.md`, `skills/spec/references/draft.md`, `skills/decompose/SKILL.md`,
  `skills/execute/SKILL.md`, `skills/routing/references/user-authorized-divergence.md`, `evals/skills/intent.md`,
  `evals/skills/execute.md`, `evals/skills/routing.md`

### Only the user's reply causes an approval row

- **Answers:** C2.2
- **Rule:** No skill writes an `approve` row with status `ok` unless the user gave the approval defined above for that
  artifact in the session that writes it. A no is recorded as the `approve` row with status `declined` that 0017
  defines, which approves nothing. Three current paths stop. Gate's reconcile step edits approved artifacts and then
  writes `approve` rows with no question; it now leaves the edited artifact stale, shows the diff, and asks one question
  that covers every artifact it edited, and an unattended run reports the stale approval and asks nothing. When the edit
  changed reviewed scope, gate asks that question only after the review that 0017 requires for the new revision has run;
  an edit to `## Divergences` or to other unhashed text leaves the approval current and needs neither the review nor the
  question. Execute writes the plan's `approve` row before it has examined the plan (`skills/execute/SKILL.md:257`); it
  now examines the plan first, and a run that stops there writes no row. On the routine tier a `/execute <plan>`
  approves the plan while nothing approves the spec; execute now reads the spec's approval, and when it is missing it
  asks the combined question that 0008 defines before it starts. This record decides who may cause the row. The fields
  that let a later reader check that are decided in 0017. (verified:
  skills/gate/references/branch/branch-workflow.md:650, 2026-11-11)
- **Default / Rejected:** The user is asked; rejected: letting gate carry an earlier approval forward, because the edit
  it approves is a change to a success criterion the user has not seen, and `docs/AISDLC.md` already states that the
  agent never approves its own work.
- **Later work changes:** `skills/gate/references/branch/branch-workflow.md`, `skills/execute/SKILL.md`,
  `skills/execute/references/flow-state.md`, `skills/decompose/SKILL.md`, `evals/skills/gate.md`

### A step the user names up front is authorized, and any clear affirmative confirms

- **Answers:** C2.4
- **Rule:** A step the user names in their request (a commit, a push, a draft pull request, starting execution after
  planning, posting a named comment or issue) is authorized when its turn comes, in every skill, with no second
  question, as long as what it touches is what the user named. At the start of the work the agent says in one line which
  named steps it took as authorized and which it will still ask about. Confirmation words are the same everywhere: any
  clear affirmative in any language confirms, and an affirmative that carries a change is an edit request. A destructive
  step is the first of two exceptions to both halves. Deleting a branch, discarding work, a force push, a history
  rewrite or an irreversible migration is confirmed once at the point of action, after the agent has shown the resolved
  target and what will be lost, whether or not it was named up front, and the reply must name the action or its target
  in the user's own words; a bare "yes" does not confirm it. The second exception is a write that changes what later
  agents are told (an instruction file, a skill, a generated verification skill or its helper scripts): naming it up
  front or approving it in advance does not authorize it, and 0018 requires the user's yes in the session after the
  exact text to be written has been shown. No step requires an exact English token. `discard` and `irreversible-ok`
  become examples of a reply that names a destructive action, not the only accepted ones. Posting is not destructive, so
  the literal `post` is dropped and any clear affirmative confirms a post. (verified:
  skills/gate/references/change/change-workflow.md:164, 2026-11-11)
- **Default / Rejected:** Named steps count for every skill, and a destructive step keeps one confirmation at the point
  of action; rejected: asking again at each outward step, because that is the behaviour that makes a named push count in
  gate and not in git; rejected: literal tokens, because they fail a user who writes in another language or paraphrases,
  while the property worth keeping (a reflex "yes" destroys nothing) survives without them.
- **Later work changes:** `skills/gate/SKILL.md`, `skills/git/SKILL.md`, `skills/gh/SKILL.md`,
  `skills/decompose/SKILL.md`, `skills/ops/SKILL.md`, `skills/gate/references/change/change-workflow.md`,
  `skills/gate/references/release/pre-deploy-checklist.md`, `skills/gate/references/branch/branch-workflow.md`

### A waiver needs no reason and has one record: a `waive` row, or an artifact or report line where no row fits

- **Answers:** C2.5
- **Rule:** A user who declines a process gate is never asked for a reason and is never refused for lacking one. A
  reason the user gives is recorded, and otherwise the record reads `none given`. Every override has one record, in the
  first home of `skills/routing/references/contracts/gates.md` that applies. For a skill with a flow phase (homes 1
  and 2) it is a `waive <what>` row written through `flow log` in the work item's `state.tsv`, or in the unassigned one
  when the work has no folder. The `waive` verb already exists in the flow contract and the hook binary
  (`hooks/go/internal/flow/cells.go:12`). A skill that keeps no flow state but writes its own run log (home 3), such as
  hillclimb's `decision.tsv`, records a `waive` row in that log. In homes 1 to 3, a line in a preview, a run report or a
  commit body is derived from that row and is never the only record. A skill that writes neither (home 4) records the
  override as its artifact or report line instead. A skill that resumes reads the `waive` rows and does not ask again,
  and gate reads them in place of reporting a missing review. Three execute references still record their override in
  the preview and the run report only: the Stage 1 skip, the waived wave pauses and the waived verification preflight.
  (verified: skills/execute/references/stage-1-dispatch-rule.md:33, 2026-11-11)
- **Default / Rejected:** No reason required, one row per override; rejected: requiring a reason, because the user owns
  the decision and a demanded reason is a second gate; rejected: forbidding a recorded reason, because a reason the user
  volunteers is the most useful part of the record; rejected: the chat report as the record, because a new session
  cannot read it.
- **Later work changes:** `skills/execute/references/stage-1-dispatch-rule.md`,
  `skills/execute/references/wave-pauses.md`, `skills/execute/references/verification-preflight.md`,
  `skills/execute/references/plan-divergence-and-trail.md`, `skills/routing/references/user-authorized-divergence.md`,
  `skills/decompose/SKILL.md`, `skills/gate/references/branch/branch-workflow.md`, `evals/skills/decompose.md`,
  `evals/skills/gate.md`

### What the user says about their own work is taken as true

- **Answers:** C3.2
- **Rule:** A plain statement from the user about their own organisation, ownership, intent, plans or history is
  accepted and acted on, and where an artifact records sources it is recorded as stated by the user. "The frontend team
  owns this" changes the route without proof. "It's just an outage" selects the reliability branch. "I'll link it later"
  opens the pull request with no second ask. "Create an issue for X" authorizes creating that issue under the
  authorization rule above. A team's own drill date and timings are recorded as given by the team, not marked
  unverified; the rule against unverified claims covers what the agent asserts, not what the user tells it. When
  evidence in the session contradicts a statement, the agent says once what it observed and then follows the user. That
  covers the user's own work and never a safety gate: a statement does not override a fact a posting path reads for
  itself, such as a repository's visibility under [0018](0018-trust-boundaries.md). A cheap check the agent can run
  itself needs no permission and no proof from the user. A role or title changes nothing in either direction: a safety
  gate holds for everyone and a process gate yields to everyone, so a skill has no reason to weigh who is asking.
  Routing still tells the agent to ask for the review, the record or the person behind such a claim before it may change
  a route. (verified: skills/routing/SKILL.md:243, 2026-11-11)
- **Default / Rejected:** Believe the user and say once what contradicts them; rejected: asking for proof before a
  statement may change a route or a branch, because the agent cannot inspect the proof either and the cost of a wrong
  statement falls on the person who made it.
- **Later work changes:** `skills/routing/SKILL.md`, `skills/ops/references/response/response-workflow.md`,
  `skills/ops/SKILL.md`, `skills/gh/SKILL.md`, `skills/gh/references/pull-requests/linkage.md`, `skills/tdd/SKILL.md`,
  `evals/skills/routing.md`, `evals/skills/ops.md`, `evals/skills/gh.md`

### One confirmation for each outward or irreversible effect, one question for each decision

- **Answers:** C9.2
- **Rule:** A workflow asks for confirmation once for each effect that leaves the machine or cannot be undone, and asks
  a question only when the answer changes what it produces. Everything else proceeds on a stated default. Writes of the
  same kind are previewed and confirmed as one batch, and a batch that was confirmed is not previewed again. The drafts
  that 0018 requires the user to accept one by one, those built from review threads of bot accounts the routing table
  does not name, are the exception: no batch confirmation and no single question covers them, and the named triage run
  that the side-effect rule below lets push and post does not cover them either. A checklist is reported in one pass and
  confirmed once at its end; it stops in the middle only for a failed item or a destructive step. Decisions that have a
  recommended answer go out in one round, and the user may end a round at any point: the remaining questions take their
  recommendation, and the record says so. A remaining question whose recommended answer would publish, post or halt the
  work is left out of that default, as 0019 decides for a bulk accept: it stays open until the user answers it by name.
  Something the user named up front or confirmed earlier in the run is not asked again. This replaces a confirmation
  after each of the eight release sections and after each change phase, scaffold init's repeated preview and confirm,
  the ops declare sequence that precedes any containment guidance, the three asks on a pull request creation, the
  per-section question counts in docs, and challenge's "Never trim the round". The layout of a question is decided
  in 0019. (verified: skills/gate/references/release/pre-deploy-checklist.md:4, 2026-11-11)
- **Default / Rejected:** Confirm effects, not steps; rejected: a numeric cap on questions for each skill, because the
  right number depends on the work and a cap invites a count to be gamed; rejected: keeping a confirmation for each
  section as an audit trail, because the report already lists every item with its result.
- **Later work changes:** `skills/gate/references/release/pre-deploy-checklist.md`,
  `skills/gate/references/change/change-workflow.md`, `skills/scaffold/references/init.md`,
  `skills/ops/references/response/response-workflow.md`, `skills/gh/SKILL.md`, `skills/docs/SKILL.md`,
  `skills/challenge/SKILL.md`, `skills/triage/references/issue-mode.md`

### Every gate is a safety gate, a repository policy or a process gate, and the draft-only pull request rules are repository policy

- **Answers:** C3.1
- **Rule:** Every gate belongs to one of three classes, and each gate is stated once in its skill with its class. A
  **safety gate** protects something the user cannot take back or accept on behalf of others. The list is closed: loss
  of work or history that cannot be undone; a change outside the machine that the authorization rule above does not
  cover; a claim of status with no evidence from this session; the handling of secrets and of text from outside the
  trust boundary; the bypassing of a commit hook or of commit or tag signing, which 0016 forbids; and the user's own
  approval, including the independent reviews that come before an approval question: the spec review that 0008 requires,
  the plan review that 0013 and 0017 keep non-waivable, and the review of the brief on the middle path of 0013. A safety
  gate is held in one fixed shape, said once: what is held, the harm it prevents, and what the user can do instead.
  Every hold names that alternative, so no stop is without an exit, and a repeated demand gets a one-line pointer back
  to it, not the script again. A **repository policy** is a rule a settings file switches. It is held while the setting
  is on, whatever is said in the session; its exit is the settings change, which the agent names once; and it writes no
  `waive` row, because nothing was waived. Three pull-request rules are repository policy: pull requests open as drafts,
  agents never merge, and agents never resolve review threads. They are on by default and are turned off only in the
  hook settings file that already carries the switches `policy.draft_prs` and `policy.block_review_thread_changes`. The
  skills say "draft by default" and follow the setting. A request in the session does not turn the rules off and writes
  no `waive` row: the agent says once how to change the setting, and the user's own click remains available for marking
  ready, merging and resolving. The merge rule has no switch of its own and follows `policy.draft_prs`: no agent merges
  while that setting is on, and when it is off a merge is an outward, irreversible effect that still needs the user's
  named request under the authorization rule above. Every other gate is a **process gate**. It names its cost once and
  yields on the next reply that still declines, in any wording, with the `waive` row defined above. Deepen's candidate
  pick, recall's scope recap, handoff's no-duplication rule, hillclimb's attempt floor and forge's scenario approval are
  process gates. The five pushback scripts become the safety hold and the process yield, and forge's hardening checklist
  and compression playbook require this contract in place of a clause for each skill. (verified: hooks/README.md:1895,
  hooks/README.md:1898, hooks/README.md:217, 2026-11-11)
- **Default / Rejected:** a gate is a process gate unless it is on the safety list or a settings file switches it, and
  the three pull-request rules are repository policy, on by default; rejected: putting the three pull-request rules on
  the safety list, removing the two switches from the hook settings and adding a hook block on merging, because the hook
  is what enforces these rules and it already ships the switch, so a skill that calls them non-waivable claims more than
  the plugin does. That choice would also cost a hook change with a rebuild of all six binaries, leave hosts that have
  no draft pull requests at a dead end, deny an owner ready pull requests in their own repository, and stretch the
  safety list past harms that cannot be undone: a ready pull request can be returned to draft and a resolved thread
  reopened. Rejected: letting each skill declare its own tier and script, because that is how five scripts and a dozen
  "safety" gates that guard no safety property came about; rejected: one tier in which everything yields, because a
  deleted branch or a false "verified" cannot be waived back.
- **Later work changes:** `skills/routing/SKILL.md`, `skills/execute/SKILL.md`, `skills/gate/SKILL.md`,
  `skills/review/SKILL.md`, `skills/decompose/SKILL.md`, `skills/hillclimb/SKILL.md`, `skills/challenge/SKILL.md`,
  `skills/explain/SKILL.md`, `skills/tdd/SKILL.md`, `skills/deepen/SKILL.md`, `skills/forge/SKILL.md`,
  `skills/threat-model/SKILL.md`, `skills/prototype/SKILL.md`, `skills/intent/SKILL.md`, `skills/handoff/SKILL.md`,
  `skills/recall/SKILL.md`, `skills/gh/SKILL.md`, `skills/triage/SKILL.md`,
  `skills/principles/references/never-block-on-the-human.md`,
  `skills/forge/references/quality/skill-hardening-checklist.md`,
  `skills/forge/references/quality/skill-compression-playbook.md`

### A side effect nobody asked for needs a named request or one confirmation, and a named triage run pushes and posts without asking

- **Answers:** C9.3
- **Rule:** A side effect the user did not ask for needs either the user's named request, under the authorization rule
  above, or one confirmation. The effects are a commit, a push, a public post, deleting code outside the request,
  starting another skill that writes, pushes or posts, as the forwarding protocol in 0015 classifies it, and writing a
  file nobody asked for. Triage stays automatic when it was named. A `/triage pr` invocation, or a request in words that
  names the push or the replies, is the named request for that run's base merge, push, bot-thread replies and rollup,
  and the opening line says the run will push and post. A request that names neither ("can you get it cleaned up?") gets
  one question first, and a scheduled run acts only when its invocation named these steps. What a named run may act on
  is limited by every rule of 0018: fork pull requests, draft-only sources, withheld security findings and an embargo.
  Zealot's direct fixes stay inside the accepted list it has shown the user and are followed by a build. Deepen asks
  before it starts `decompose` and before it writes `CONTEXT.md`. The worktree workflow's bare `.gitignore` commit is
  not confirmed here: the ignore-rule decision in [0016](0016-git-write-ownership.md) removes it outright. Hillclimb
  commits a win only on a branch it named at the start, never on the default branch. Today a request in words about a
  pull request's bot findings starts the run with no question, whether or not it named the push, and the bot-thread
  replies are mandatory. (verified: skills/triage/SKILL.md:192, skills/triage/SKILL.md:224, 2026-11-11)
- **Default / Rejected:** ask once, or act on a named request, and triage is no exception in either direction: the user
  who types `/triage pr 92` has named the push, and the user who asks vaguely has not; rejected: having triage always
  show the fixes and the reply drafts and ask one question before the push and the posts, however it was invoked, with a
  scheduled or unattended run stopping at local commits and drafts, because it costs one stop in every run, leaves an
  unattended run with nothing visible on the pull request, and does not reduce the risk that remains. That risk is about
  whose text triage obeys, which is 0018's decision, and a confirmation the user answers without reading each fix does
  not lower it. Rejected: announcing the effect and proceeding when nothing named it, because an announcement the user
  cannot answer before the commit or the deletion lands is not consent; rejected: a confirmation for every write inside
  the requested scope, because that is the stacking the confirmation rule above removes.
- **Later work changes:** `skills/zealot/references/comments/workflow.md`, `skills/deepen/SKILL.md`,
  `skills/hillclimb/SKILL.md`, `skills/triage/SKILL.md`

## Existing records

- 0005: consistent — a release is still published by a person, and no rule here lets a named step publish one.
- 0008: consistent — only the user's yes approves an intent, a spec or a plan, the routine tier keeps its combined
  question, and the independent spec review stays a gate the user cannot waive because it sits on the safety list.
- 0009: consistent — a small fix still leaves the chain through its `skip chain` row and needs no approval row.
- 0012: consistent — overrides and approvals are rows written through `flow log`, and the `waive` verb it ported is the
  one this record uses.

## What this costs

**A looser yes can approve something the user meant to keep discussing.** Accepting any clear affirmative trades the
occasional misread for the end of three protocols. The approval binds to a content hash, so a misread is corrected by
saying no and is visible as a row.

**Gate asks one more question.** Reconciling divergences used to finish without the user. It now ends with a question
whenever it edited an approved artifact, and an unattended gate run ends with a stale approval that someone must clear.

**A named push goes out without a second look.** The user who writes "commit, push and open a draft" gets exactly that.
Whatever the checks before the push did not catch is now on the remote.

**A named triage run pushes and posts with no person reading first.** A push and public replies can go out unread, so
everything rests on what 0018 lets triage treat as an instruction.

**A settings file can make an agent open ready pull requests.** The draft, merge and review-thread rules hold only while
their settings are on, the skills must read the setting, and merging stays unenforced by any hook.

**Process gates yield after one sentence.** A user in a hurry can skip the plan challenge, a wave pause or the test that
would have gone red first, and the only trace is a `waive` row. Reviews and the pull request are what catch the result.

**The agent will sometimes act on a false statement.** Believing "the frontend team owns this" costs a wrong route when
it is wrong. The alternative costs every truthful user a demand for proof the agent cannot check.

**Every skill's pressure text is rewritten.** About twenty skills, the forge hardening checklist and most eval banks
carry the old scripts, and the banks that grade a refusal of a process waiver stop being valid until 0022's pressure
rule is applied to them.

**Fewer confirmations leave less record of consent for each step.** A release checklist confirmed once at its end shows
that the user saw the report, not that they read each section.

## Considered and declined

**Move the independent spec review to the process tier.** The contract's test would put it there, since skipping a
review destroys nothing. Declined for this record: 0008 considered a waivable review and rejected it because the routine
tier's combined approval relies on the review having run. The review therefore stays on the safety list as part of what
makes an approval mean something, and changing that is a decision about 0008, not about pushback.

**Yield a process gate on the first decline, with no cost sentence.** Declined. The one sentence is what makes the
waiver informed, and it is the difference between a recorded override and a silent skip.

**Keep "sustained push" as the trigger for yielding.** Declined. Whether a push is sustained is a judgment two runs make
differently; "the next reply that still declines" can be checked.
