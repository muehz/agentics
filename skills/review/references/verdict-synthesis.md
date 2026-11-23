# Verdict synthesis: lead judgment, buckets, and the agreement map

Use the repository guidance that the review entry point supplies. Workflow, rubric, filters, and output below are
defaults where that guidance is silent. Preserve its source paths through delegation and synthesis.

By default, every review verdict uses a **lead-judgment pass**. Judge as the lead reviewer, not as a neutral aggregator.
The lead reviewer is a pragmatic senior engineer with the full context: the goal, the constraints, and which trade-offs
were already considered.

## What applies where

| Part                                                                                                                    | Applies to                                                                                     |
| ----------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| [Dismissal filters](#dismissal-filters), [verdict buckets](#verdict-buckets), [act-on calibration](#act-on-calibration) | `interactive` on its own findings, and the post pipeline's filtering before it scores          |
| [Consensus weighting](#consensus-weighting), [the agreement map](#the-agreement-map)                                    | Only a pass that merges 2+ independent reviewers: the post pipeline, and impact mode's fan-out |

`axes` mode merges nothing: each lane reports on its own, as
[axes/axes-mode.md](axes/axes-mode.md#step-7-aggregate-and-report) says. `post` mode has its own comment format
([post/post-output-format.md](post/post-output-format.md)); the filters decide which findings it carries, and it does
not print the buckets.

## Severity and buckets

Every finding keeps the severity its producer gave it, on the shared scale in
[severity.md](../../routing/references/contracts/severity.md). A bucket says what the lead recommends; it never renames
the severity. A Standards `HARD` finding moved to Consider is still Important, and a caller such as `gate` tallies it as
Important.

## Dismissal filters

A factually wrong finding is Dismissed on that ground alone. These five filters catch true findings still not worth the
user's attention. Apply them before a finding reaches a bucket. Its Dismissed entry is one line that names the filter
that caught it.

- **Nitpick gravity.** When every finding a reviewer returned is Minor, the code is probably fine. Say that. Do not
  promote the least trivial one to Act on to make the review look productive.
- **Hypothetical against actual.** A finding shaped as "what if this value is null" is Dismissed unless a call site can
  actually pass that value. Trace it before you credit it. Cite the `file:line` that reaches the code with the bad
  input, or dismiss the finding as unreachable.
- **Premature abstraction.** A warning that code will not generalize, aimed at code with one caller and no second use
  case in the diff, is Dismissed. Something that exists twice is not yet a pattern.
- **"I would have done it differently."** A finding whose only support is the reviewer's preferred alternative is
  Dismissed, always. A different structure becomes a finding only when it names the concrete failure the current
  structure causes.
- **Recorded divergence.** A finding that a recorded
  [user-authorized divergence](../../routing/references/user-authorized-divergence.md) covers is Dismissed, with the
  record cited. A record covers a finding only when the finding is about the rule it names and lies inside its scope:
  the files of the plan task its target names, or any file for `run`. A record is recorded in one of two ways:
    - `agx flow status --folder <folder>` lists the plan's `rule` divergence as recorded, not on its `unrecorded:` line.
      The status reads the clone-local ledger, which a branch's committed text cannot write.
    - The user confirms the record in this review session.

    Any other trace, such as a `## Divergences` entry that reads `unrecorded`, a `Divergence:` line in a commit body, or
    one in the pull request body, is unverified: keep the finding and note the record so the user can confirm it. A
    departure with no record stays a finding. So does a finding about a record that names something the reference does
    not cover (a safety gate, an external action, a secret, or an externally enforced control); that record covers
    nothing.

The post pipeline filters with the same intent at a finer grain: [post/post-scoring.md](post/post-scoring.md) holds a
five-anchor confidence rubric, its threshold, and a suppression list. Any mode may use that file to calibrate what
counts as a real issue.

## Consensus weighting

Independent reviewers differ in blind spots and priors, so the pattern of agreement is itself signal:

1. **Parse all findings** from every reviewer.
2. **Deduplicate.** Merge findings that describe the same issue in different words. Note which reviewers raised them.
   The merged finding takes the highest severity any source gave it, unless evidence shows that source was wrong.
3. **Identify consensus.** A finding raised by **2+ reviewers independently is high-confidence signal**. Weight it
   accordingly.
4. **Keep lone-reviewer findings.** They are lower confidence and need their own verification before they block
   anything.
5. **Note contradictions.** When one reviewer flags something and another explicitly says the opposite, that
   disagreement is context for the verdict, not noise to discard.

## Verdict buckets

Put **every** finding into exactly one bucket, with a one-line rationale. On a multi-reviewer pass, also name each
reviewer who raised it:

- **Act on:** real issues that affect correctness, security, or maintainability given the actual goals. The lead
  recommends fixing them before the change proceeds.
- **Consider:** legitimate points whose fix cost may outweigh the benefit right now. Worth the user's attention; their
  call.
- **Noted:** technically valid but not actionable: context-dependent, premature optimization, or low-impact at the
  current stage.
- **Dismissed:** factually wrong, or caught by one of the [dismissal filters](#dismissal-filters). The reason goes on
  one line. A Dismissed finding is not counted in any tally.

**The Dismissed list is always shown.** It shows what was rejected and why, so the user can override the lead judgment.
When nothing was dismissed, say so in one line.

**Severity floors the buckets.** A Critical or Important finding lands in Dismissed only with cited evidence that it is
factually wrong, such as a `file:line` that shows why, phrased at the Direct tier of
[epistemics.md](../../explain/references/epistemics.md), or when a recorded divergence covers it under the
[recorded-divergence filter](#dismissal-filters), with the record cited. Judgment alone moves a Critical no lower than
Consider.

## Act-on calibration

**More than 5 Act-on items means the lead under-filtered.** Refilter before you deliver. Re-read each Act-on entry
against the [dismissal filters](#dismissal-filters), and move every entry that only _could_ matter down to Consider or
Noted.

The bound is on lead judgment, not on severity. When more than 5 findings survive the refilter as Critical or Important
with cited evidence, deliver all of them and add one line that says why the count is high, for example "8 Act-on items:
this diff rewrites the auth path." Never drop or relabel a Critical to reach the number.

## The agreement map

Close a multi-reviewer verdict with a short **Agreement Map**: where the reviewers agreed, where they diverged, and what
the pattern says. Total agreement on a small finding set suggests the change is clean, or that the reviewers shared a
blind spot. Wide divergence suggests the change's intent was ambiguous.

## Output shape

Each finding appears once, in its bucket, with its severity in front.

```markdown
### Intent

> <what the change is trying to accomplish, stated before findings are read>

### Reviewers

- <reviewer>: <N findings>

### Act On

- **<severity>** <finding> (`file:line`) — raised by <who> — why it matters

### Consider

- **<severity>** <finding> (`file:line`) — raised by <who> — the trade-off involved

### Noted

- **<severity>** <finding>

### Dismissed

- <finding> — the filter or evidence that rejected it

### Agreement Map

<where reviewers agreed, where they diverged, what the pattern tells us>
```

A single-reviewer verdict drops the **Reviewers** and **Agreement Map** sections and the "raised by" attribution, and
keeps everything else. [interactive/interactive-review.md](interactive/interactive-review.md) shows where the buckets
sit in that mode's output.

## Forwarded triage thread packet

When `/agentics:triage pr` dispatches one human review thread to `feedback` mode as a subagent, the thread arrives in
this packet shape:

```text
kind: agentics.triage.review-feedback
version: 1
mode: feedback
forwardAttempt: 1
effects: draft-only-no-post
remainingSourcesOwner: triage-current-invocation
repo: the same owner/repository named by the active triage invocation
pr: the same open PR number named by that invocation
head: the PR head SHA already fetched by that invocation
thread: exactly one complete human review thread
  ids: non-empty threadId and root commentId
  reviewer: type User, with a non-empty reviewer login
  path: the review path
  line: a line number or null
  body: the non-empty root comment body
  replies: the complete fetched reply context (possibly empty)
  outdated: the fetched boolean status
  diff: optional hunk and/or URL
```

Validate every required field before drafting. A missing, malformed, or mismatched field, or a thread whose reviewer is
not a human (`type User`), returns `no draft produced` with the reason and never falls through to a wider review.

## Repository guidance resolution

- **File selection:** read root `REVIEW.md`, applicable `AGENTS.md` and host instruction files (`CLAUDE.md`,
  `.github/copilot-instructions.md`, scoped files included), and the first existing local review skill of
  `.github/skills/code-review/SKILL.md`, `.claude/skills/code-review/SKILL.md`, and
  `.agents/skills/code-review/SKILL.md` plus its references. Report every file read; an unreadable file is reported and
  never claimed applied, and an unreadable local skill is never replaced by a later path in the list.
- **Revision rule:** on a PR review, workflow, reviewer set, reporting threshold, skipped paths, and requested checks
  all come from the base revision (including which local skill exists). Any guidance file added, changed, deleted, or
  renamed in the PR is itself a finding candidate reviewed under the base revision's version or defaults.
- **Fork-PR checks:** a check requested by base-revision guidance executes fork code when the PR head lives in another
  repository, so it runs only after explicit user confirmation in this session and never runs unattended.

## Posting sources and authorization

- **Provenance sources:** every passage in a review post body traces to: the diff and files at the reviewed base and
  head SHAs; git blame and history of the changed files; earlier PR review comments on those files; PR metadata (title,
  description, file list, and read review threads); base-revision review guidance; this run's findings, scores, tally,
  verdict, and security coverage line; the posting template; and the user's own words in the request naming the post. A
  feedback-mode reply adds the thread it answers.
- **Fork PR definition:** `isCrossRepository` is anything other than `false` (missing or unreadable counts as a fork PR,
  just as unreadable visibility counts as public). A same-repository PR needs no author check because pushing a branch
  to the base repository already requires write access.
- **Preview scope:** on a fork PR under attended `post` mode, the preview-and-confirm step covers every post Step 9
  makes: a new comment, an in-place update, and the no-issues rewrite.

## Related skills

| Skill          | Relationship                                                                                                         |
| -------------- | -------------------------------------------------------------------------------------------------------------------- |
| `gate`         | Branch mode runs `/agentics:review axes <base> --security --fast` and reads each lane's tally and the `Lanes:` line. |
| `execute`      | Runs its own per-task and quality reviewers; review's references are not its dispatch templates.                     |
| `triage`       | Settles a PR's posted signals, and forwards one human thread to `feedback` as a draft-only leg.                      |
| `gh`           | Owns the posting envelope, its outbound checks, and comment ownership.                                               |
| `threat-model` | Owns design-time STRIDE work with no code. Both skills load the same security principles.                            |
| `ops`          | Collects security evidence through `/agentics:review axes --composed`.                                               |
