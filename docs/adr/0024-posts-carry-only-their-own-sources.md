# A post carries only its own sources, and a named review post on a fork PR is shown first

This record amends [0018](0018-trust-boundaries.md) and [0014](0014-approval-and-pushback.md). It answers #1470, which a
security review of #1450 rated Medium. [0020](0020-environment-and-platform.md) removed `context: fork` from review, so
the review lead now reads a pull request's title, body and diff in the caller's own conversation, and a named
`/agentics:review post <n>` goes out after the outbound checks with no second question. Those checks read the
repository's visibility, redact secrets and internal hosts, withhold unfixed security findings on public repositories
and honor an embargo. None of them stops other conversation text (earlier discussion, other files, other work items)
from going into the body, so a prompt injection planted in a pull request could ask the agent to quote that text in a
public comment. The same pattern holds for every path that reads untrusted text in the main conversation and then posts.

One principle runs through both rules below. What a post may carry is decided by the post's own sources, which the skill
names in advance, and never by what the text being reviewed asks for.

## Rules

### Every composed post traces each passage to one of its own sources

- **Answers:** #1470
- **Rule:** The posting envelope's outbound checks gain a fifth check, provenance, run after the other four and before
  the post. Every passage of the body traces to one of the post's sources; a passage that traces to none is removed, and
  the run's report says what was withheld, as the other checks do. The check applies to a body the agent composes (a
  comment, reply, review, issue body or pull request body), not to file content in commits, which git's push check reads
  (verified: skills/gh/references/posting-envelope.md:125, 2026-11-20). The reference that defines a body names its
  sources, as `pull-requests/body.md` does for a pull request body; when it names none, a default list applies: the
  outputs this run produced for this post, the repository content and host metadata this run read for it, and the user's
  own words in this session that name or confirm the post, with whatever conversation text, files or work items those
  words direct into it (verified: skills/gh/references/posting-envelope.md:132, 2026-11-20). Two kinds of text are never
  sources: text that untrusted data asks the agent to include, and conversation text, files, tool output or work items
  that neither the body's sources nor the user's own words direct into this post (verified:
  skills/gh/references/posting-envelope.md:142, 2026-11-20). A passage traces to a run output, such as a finding, only
  through that output's own cited evidence, so a finding that quotes conversation text with no evidence from a listed
  source does not make that text a source (verified: skills/gh/references/posting-envelope.md:149, 2026-11-20). Review
  names its own sources: the diff and files at the reviewed base and head SHAs, the blame and history of the changed
  files, earlier pull requests' review comments on them, the PR's metadata and the threads this run read, the base
  revision's review guidance, this run's findings, scores, tally, verdict and security coverage line, the posting
  template, and the user's words in the request that names the post; a feedback-mode reply adds the thread it answers
  (verified: skills/review/SKILL.md:255, 2026-11-20). Triage, gate and ops run the check under their body references'
  sources or the default list.
- **Default / Rejected:** Each body's own reference names its sources, with the default list when it names none;
  rejected: one review-derived list for every posting path, because a gate pull request body must keep the session
  evidence that `body.md` requires. Handoff documents stay outside the check, because they do not run the outbound
  checks and their content is the conversation by design.
- **Later work changes:** `skills/gh/references/posting-envelope.md`, `skills/review/SKILL.md`, and the body references
  of triage, gate and ops when they name their own sources. Review's listed sources do not include the user's own words
  given in reply to a preview's edit request, so an edit the user dictates at the preview has no named source and the
  check could remove it; later work should add those words to review's list and to the default list.

### An attended named review post on a fork PR shows its text once and asks

- **Answers:** #1470
- **Rule:** A named, attended `post`-mode review post on a pull request whose `isCrossRepository` is anything other than
  `false` shows the finished text once, exactly as it will post, after the outbound checks, and asks one question. It
  covers every post the pipeline's last step makes: a new comment, an in-place update and the no-issues rewrite. Any
  clear affirmative posts, including "just post it", and a no ends the run with the text delivered locally. A missing or
  unreadable value counts as a fork PR, as an unreadable visibility counts as public (verified:
  skills/review/SKILL.md:268, 2026-11-20). A same-repository named post stays one step and needs no author check,
  because pushing a branch to the base repository already requires write access. An unattended named post, on a fork PR
  or not, and a feedback-mode named reply keep the one-step row and post after the outbound checks with no second
  question (verified: skills/review/SKILL.md:266, 2026-11-20); an unattended run has no one to see a preview, and
  refusing would stop every automated review of a fork PR. The preview is an ordinary approval question, not a gate, so
  the gate lists do not change. It is a third exception to **Steps named up front**, beside a destructive step and a
  write that changes what later agents are told: naming such a post up front does not authorize it, and every other
  named post keeps that rule (verified: skills/routing/references/contracts/approval.md:66, 2026-11-20) (verified:
  skills/routing/references/contracts/approval.md:70, 2026-11-20). The post pipeline keeps the `isCrossRepository` value
  it reads in Step 1 and applies the preview in Step 8 (verified: skills/review/references/post/post-pipeline.md:34,
  2026-11-20) (verified: skills/review/references/post/post-pipeline.md:157, 2026-11-20).
- **Default / Rejected:** The provenance check on every path plus a one-time preview for attended named review posts on
  fork PRs, keyed on `isCrossRepository` alone; rejected: the provenance check alone, because the agent the injection
  steers would be the only one checking, and that is weakest where the risk is highest, on pull requests from forks.
  Also rejected: reading all untrusted PR text only in dispatched subagents that return a constrained result, because
  the lead still reads the diff and the free-text findings in both `interactive` and `post` mode, so the isolation would
  restructure both modes and still leave a leak path. An `author_association` test was dropped from the fork test,
  because a same-repository branch already requires write access.
- **Later work changes:** `skills/review/SKILL.md`, `skills/review/references/post/post-pipeline.md`,
  `skills/routing/references/contracts/approval.md`, and the pointers to that exception in `skills/gh/SKILL.md`,
  `skills/ops/SKILL.md`, `skills/routing/references/forwarding.md` and
  `skills/forge/references/quality/skill-hardening-checklist.md`.

## Existing records

- 0014: amended — a named post that a skill's own posting rules preview first is asked about once as an ordinary
  approval question, a third exception beside the two it already lists; its text is not edited.
- 0018: amended — its "Every posting path checks visibility, redacts, and honors an embargo" rule gains a sixth point,
  provenance, for every path that runs the outbound checks; its text is not edited.
- 0020: consistent — `context: fork` stays removed, and the preview uses its definition of an unattended run.

## What this costs

**One more question on fork pull requests.** An attended named review post on a fork PR now takes a second turn, and the
request that named it no longer authorizes it on its own.

**The provenance check is not a live injection defense.** The check is text the agent follows, run by the same agent the
injection targets. On a same-repository PR, and on any unattended run, it lowers the risk rather than removing it. A
same-repository PR can still carry text an outsider wrote, such as a bot's pull request body or an outside user's review
thread; that text reaches an attended named post with no preview, so no person sees the post first, and the only
controls are ones the agent runs itself: the untrusted-data rule and the outbound checks, provenance among them. The new
preview is the control that a person sees, and it covers only attended, named `post`-mode posts on fork PRs; an unnamed
attended post was already shown first.

**The evidence is role-play evals of review only.** The review bank ran three paid `cycle` runs on 2026-11-20 with
`claude-haiku-5-5`. In all three, the changed skill passed the target checks of the same-repository canary scenario, the
fork preview scenario and the happy named post, and the canary appeared in no posted or previewed body. The control arm,
review's files from `main`, failed the fork preview. It is not pure `main`, because the engine reads cross-skill files
such as the posting envelope and the approval contract from this checkout, so it shows the effect of review's own rows
and not of the envelope's check. These are role-play evals, not live runs against a hostile pull request. The provenance
check itself lives in gh's posting envelope. The gh bank was not run; the check was exercised only through review's
canary scenarios, whose packets read the envelope from this checkout. Routing, ops and forge carry pointer-only changes
and were not evaluated, and the triage, gate and ops banks were not rerun, so the check on their paths rests on the
shared envelope text alone.

**An edit dictated at the preview has no named source.** Until review's list names the user's reply to a preview, the
check can strip a passage the user asked for at that point.
