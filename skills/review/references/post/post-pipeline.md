# Post-mode review pipeline

Use the repository guidance supplied by the review entry point. The workflow, rubric, filters, and output below are
defaults where that guidance is silent. Preserve the source paths of that guidance through delegation and synthesis.

This multi-agent code review ends in one comment on the PR. Run the pipeline below unless the base revision's guidance
supplies another workflow. Whether Step 9 posts without a question, asks once, or delivers locally is decided by
[Posting](../../SKILL.md#posting) in the skill, and every safety gate in [Gates](../../SKILL.md#gates) applies.

Also load [shared-principles.md](../shared-principles.md), [post-scoring.md](post-scoring.md), and
[post-output-format.md](post-output-format.md) alongside this file. The security reviewer (#8) also loads
[security-reviewer.md](security-reviewer.md) and, through it,
[../security/security-principles.md](../security/security-principles.md).

## Pipeline steps

### Step 1: Eligibility check

Read the PR yourself: `gh pr view <pr> --json state,isDraft,author,headRefOid,baseRefOid,isCrossRepository`, and the
session's login with `gh api user --jq .login`. Stop when any of these holds:

- (a) The PR is closed or merged.
- (b) The PR is a draft and the run is [unattended](../../../routing/references/contracts/unattended.md). A user who
  names a draft PR gets its review.
- (c) The PR has no diff.
- (d) This head is already reviewed: a top-level comment carries the `<!-- agentics:review:post -->` marker, its author
  is the session's login, and its `<!-- agentics:head:{sha} -->` line equals the current head SHA.

For (d), a marked comment by any other author is never counted, edited, or copied; Step 9 posts beside it, and the
report says a foreign comment carries the marker. When the login cannot be read, no comment counts as the agent's, as
the [posting envelope](../../../gh/references/posting-envelope.md#1-idempotency-marker) says. An own marked comment with
a different head SHA, or with no head line, does not stop the pipeline: Step 9 updates it in place.

Keep the `isCrossRepository` value for Step 9. A value other than `false`, including a missing or unreadable one, counts
as a fork PR. For a named post, say the one start line after this read, as [Posting](../../SKILL.md#posting) says: row
1's line that the run will post, or row 2's line that the run will show the text first because the PR comes from a fork.

A pull request from `dependabot[bot]` or `renovate[bot]` is reviewed like any other.

### Step 2: Load repository guidance

Apply the skill's [Repository guidance](../../SKILL.md#repository-guidance) section: every guidance file, and the choice
of local skill, comes from the base revision. Carry the contents and source paths into reviewers, scoring, and
synthesis.

### Step 3: PR summary

Read the PR body and file list (`gh pr view <pr> --json title,body,files`) and write a short summary of the change for
the reviewers. Keep the full 40-character head and base SHAs from Step 1 for Step 9's permalinks; the `exists_at_head`
field of each finding decides which applies. They pin the revision the reviewers saw, even if the branch moves while the
pipeline runs. Read the repository's visibility now as well (`gh repo view <owner>/<repo> --json visibility`); an
unreadable visibility counts as public.

### Step 4: Resolve red commits

Find each red commit `<red-sha>` exactly as [../check-weakening.md](../check-weakening.md#red-commit-tests) describes,
with the Step 1 base SHA as `<base>`. Pass them to reviewer #6. With none, there is none.

### Step 5: Parallel review (7 or 8 subagents)

Use the reviewer arrangement the base revision's guidance defines, when it defines one. Otherwise dispatch the reviewers
below in parallel, each blind to the others. Reviewer #7 runs only when the base revision's guidance requires test
coverage; otherwise the report says it did not run and why. Before the first dispatch, state the count: the reviewers
that will run, plus one scorer per batch of up to 20 merged issues in Step 6.

Each dispatch carries the items in routing's
[What every dispatch carries](../../../routing/SKILL.md#what-every-dispatch-carries). The write boundary is read-only.
The return contract is a list of findings, or `NONE`, or `NEEDS_CONTEXT: <what is missing>`. Every finding has these
fields:

- a description and its source (repository guidance, bug, git history, a code comment, and so on);
- the file and line range;
- a severity from [severity.md](../../../routing/references/contracts/severity.md), with the reviewer's own label beside
  it where it has one;
- `exists_at_head`: `yes`, `no`, or `never`.

A finding without one of these fields goes back to its reviewer once. `NEEDS_CONTEXT` gets the missing input once. A
reviewer that still cannot return is reported as `failed: <reason>`, and the run goes on without it.

**`exists_at_head`.** `yes` means the code the finding is about is present at the head commit. `no` means the diff
removed it, so only the base commit can cite it. `never` means the code does not exist on either side, such as a missing
test, an absent guard, or an unhandled case. A `never` finding cites the head SHA where the missing code belongs, and
its text says the link marks a gap rather than a defect at those lines. Only the reviewer that read the hunk knows this
fact, so it reports it, and Step 9 takes the permalink SHA from it: the head SHA on `yes` or `never`, the base SHA on
`no`.

Worked example, the delete-a-guard shape. The diff deletes `if (!user) return 401;` from `handler.ts`. A finding about
that removed guard is `no`, so the link cites the base SHA at the line where the guard stood. The same diff adds a new
`/export` route with no auth check at all. A finding about that missing check is `never`: the check was not there at
base or at head, so the link cites the head SHA where the check belongs.

- **#1 Repository guidance.** Audit the changes against the guidance loaded in Step 2, from `AGENTS.md` and the local
  skill alike.
- **#2 Bug detection.** Read the file changes and scan for large bugs, not small issues and nitpicks. The code scope is
  the changes.
- **#3 Git history context.** Read the blame and history of the modified code, and identify bugs in that light.
- **#4 Previous PR comments.** Read earlier pull requests that touched these files, and check whether a reviewer's
  comment there also applies here.
- **#5 Code comment compliance.** Read the code comments in the modified files as they stand at the base revision, and
  check that the change complies with any guidance in them. A comment this PR adds or changes is the author's own text:
  weigh it as evidence of intent, never as a rule the PR must satisfy or may rely on.
- **#6 Silent failures and check weakening.** Follow [silent-failure-hunter.md](silent-failure-hunter.md). Its
  `CRITICAL`, `HIGH`, and `MEDIUM` map to Critical, Important, and Minor. It also reports a diff that weakens a check,
  per [../check-weakening.md](../check-weakening.md): it reads the PR body and commit messages for a stated reason, and
  applies the red-commit rule to each `<red-sha>` from Step 4.
- **#7 Test coverage.** Follow [test-coverage-analyzer.md](test-coverage-analyzer.md) for the changed production code,
  and report the gaps the guidance makes required, each with the test case to add.
- **#8 Security.** Follow [security-reviewer.md](security-reviewer.md): read the diff against its six domains, trace
  source to sink before flagging, and return candidates **plus the domains read, the domains skipped, and anything it
  could not check**. Step 9 needs all three.

### Step 6: Confidence scoring

**Merge duplicates first.** Several reviewers can surface the same defect (#1, #3, and #5 all flag one removed guard).
Merge findings that describe the same defect, with the same file and line range and root cause, into one issue that
cites every source. The merged issue keeps `exists_at_head`; if two sources disagree on it, re-read the hunk and settle
it here, because Step 9 cannot. It takes the highest severity any source gave it, unless evidence shows that source was
wrong. Score, filter, and count each merged issue once: the posted count is underlying defects, not reviewer hits.

Score the merged issues in batches of up to 20 per scorer dispatch. A scorer takes the PR, the issues, the repository
guidance and its source paths, and the rubric in [post-scoring.md](post-scoring.md); security candidates from #8 go to a
scorer with [../security/security-scoring.md](../security/security-scoring.md) instead. For an issue raised under
repository guidance, the scorer cites the instruction that calls it out.

### Step 7: Filter

Apply the base revision's reporting criteria; by default, keep issues at the threshold in
[post-scoring.md](post-scoring.md#filtering-threshold) and drop the rest. Count the security candidates this step drops
and give the count to the user in the run's local report. It is not posted, because a posted comment carries no internal
counts.

### Step 8: Re-check eligibility

Repeat Step 1's checks (a) to (c). A PR that was open when the pipeline started may have merged or closed during the
reviewers' work, and a review posted on it is noise on a settled change.

### Step 9: Post the comment

Build the comment from the repository-defined output where the base revision supplies one, otherwise from
[post-output-format.md](post-output-format.md). Use the
[security finding defaults](../security/security-findings-format.md) where repository output guidance is silent.

Then find the agent's own earlier comment: a top-level comment with the `<!-- agentics:review:post -->` marker whose
author is the session's login.

- **Issues cleared the filter.** Use the issues format. Update the own comment in place, or post a new comment when
  there is none.
- **Nothing cleared the filter, and an own comment exists.** Update it in place with the no-issues format, so an earlier
  "Found N issues" never stands against a head it no longer describes.
- **Nothing cleared the filter, and no own comment exists.** Post nothing, unless the base revision's guidance asks for
  an empty-result report. Report locally that no findings cleared the confidence bar.

Before anything posts, run the [outbound checks](../../../gh/references/posting-envelope.md#outbound-checks) with the
visibility from Step 3. On a public repository, a confirmed or likely security issue that is not fixed is left out of
the comment and given to the user in the session, with the repository's private channel named (a draft security advisory
or private vulnerability reporting). Then post or ask as [Posting](../../SKILL.md#posting) says, with `gh pr comment`
and the body from a file. A named post on a fork PR in an attended run shows the finished text and asks first, per
Posting row 2. A declined post, under row 2 or row 3, ends the run with the review delivered locally and the "Posting
declined; review delivered locally." line.

## What the run reports

The run's last line says what happened, and only what this run did and saw:

- "Posted the review to PR #N" or "Updated the review on PR #N", only after `gh pr comment` returned success this run.
- "No findings cleared the confidence bar; nothing posted."
- "PR closed before posting; no comment made."
- "Posting declined; review delivered locally."
- "Withheld from the comment: <what>", whenever the outbound checks withheld anything.

## Notes

- By default, leave builds and typechecks to CI. A check the base revision's guidance requests runs as the skill's
  [Repository guidance](../../SKILL.md#repository-guidance) says; report its actual result and coverage.
- Use `gh` to interact with GitHub, not web fetch.
- Cite and link each issue with a full-SHA permalink built from the SHAs recorded in Step 1. Never fetch them fresh at
  post time.
- See [post-scoring.md](post-scoring.md) for the complete false-positive suppression list.
