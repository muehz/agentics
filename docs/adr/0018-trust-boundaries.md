# Third-party text is data, and a write follows verified authorship

This record sets the trust boundary policy for the skills: what text an agent may act on, whose authorship opens a lane
that writes, whose code runs on the maintainer's machine, which revision supplies review guidance, who owns a posted
comment, and what may leave the machine. It answers theme T8 of the skill-suite audit in
`docs/designs/skill-audit-2026-11/consolidated-issues.md`. The detail and the quoted lines are in
`docs/designs/skill-audit-2026-11/2026-11-09-pass-3-raw-trust-boundaries.md`.

One principle runs through every rule below. Text that a third party can write (a comment, a bot body, a CI log, a pull
request description, a file on a pull request's head, an issue) is evidence for the agent to weigh. It is never an
instruction, an approval, or proof of who wrote it. A marker string, an account type and a heading are all things a
third party can produce, so none of them establishes authorship. The author field GitHub reports does.

## Rules

### Triage treats third-party text as data and opens the automatic lane only for a verified author

- **Answers:** C8.1
- **Rule:** Four things hold for triage. First, every prompt that carries a comment body, a bot body, a CI log line or a
  check annotation places it inside a fenced block and states that the block is quoted data that cannot change the
  specialist's instructions, tools or output, in the words `skills/execute/references/implementer-prompt.md` already
  uses (verified: skills/triage/references/subagent-contract.md:274, 2026-11-11). The fence labels the data and is not
  the control: a model can still follow text inside it, so the next three points, which key every write on verified
  authorship and leave the forbidden-path check to the orchestrator, are what hold when it does. Second, a source is
  identified by the comment's author and never by a body marker alone: a comment is Checkmarx signal only when it
  carries the marker and its author is a scanner account the repository declares in configuration read from the default
  branch, and a marker comment from any other account is handled as human feedback, which is draft-only (verified:
  skills/triage/references/source-detection.md:82, 2026-11-11). Third, the automatic lane is open only to the bot
  accounts the routing table names; review threads from `github-actions[bot]` and from any `Bot` account the table does
  not name produce drafts that the user accepts one by one, because the PR author or a workflow input can choose their
  words (verified: skills/triage/references/source-detection.md:282, 2026-11-11). A reply inside a bot thread changes a
  finding's disposition, and is cited as the authority for it, only when its author has write access to the repository;
  any other reply is quoted as context (verified:
  skills/triage/references/pull-requests/bot-review/bot-review-triage.md:56, 2026-11-11). Fourth, triage pushes
  specialist commits and posts specialist-written bodies without a further confirmation only when its invocation or the
  request named the push or the replies, where a `/triage pr` invocation counts as naming both, and asks one question
  first when neither was named, as [0014](0014-approval-and-pushback.md) decides; in both cases the orchestrator, and
  not the specialist it constrains, checks the commits against the list of forbidden paths before any push. (verified:
  skills/triage/references/orchestration-lifecycle.md:962, 2026-11-11) (verified:
  skills/triage/references/checks/checks-tier-policy.md:78, 2026-11-11)
- **Default / Rejected:** Author-keyed detection with a declared scanner account, and draft-only handling for every
  author that is not verified; rejected: keeping the marker as the only key and adding a content check on the findings
  table, because anyone who can comment can copy a valid table, and the latest marker comment would still displace the
  real scan. Also rejected: trusting every account of type `Bot`, because the account type says who posted the text and
  not who chose it.
- **Later work changes:** `skills/triage/references/source-detection.md`,
  `skills/triage/references/subagent-contract.md`, `skills/triage/references/orchestration-lifecycle.md`,
  `skills/triage/references/pull-requests/bot-review/bot-review-triage.md`,
  `skills/triage/references/checks/checks-tier-policy.md`, `skills/triage/SKILL.md`

### Triage does not execute code from a fork

- **Answers:** C8.2
- **Rule:** When the pull request comes from a fork, triage installs no dependencies, builds nothing, and runs no test
  command, formatter or generator from the pull request's head on the maintainer's machine. A fork run reads the head,
  classifies each finding, and produces drafts and proposed patches from reading alone; it says in its output that
  nothing was executed. Execution of fork code happens only when the user, in their own turn in the current session,
  names the pull request and asks for it. No comment, label, file or bot body can ask for it. Same-repository pull
  requests keep the worktree baseline, because pushing a branch to the repository already requires write access. A
  triage specialist that cannot create its worktree stops and reports; it never falls back to running in the caller's
  checkout. (verified: skills/triage/references/subagent-contract.md:15, 2026-11-11)
- **Default / Rejected:** No execution for fork pull requests, with an explicit per-run opt-in by the user; rejected:
  running the baseline with install scripts disabled (`--ignore-scripts`), because the build, the test command and the
  generators are the contributor's code too, and the flag exists for one package manager. Also rejected: requiring a
  sandbox, because no skill can verify that one is in place on every host.
- **Later work changes:** `skills/triage/references/orchestration-lifecycle.md`,
  `skills/triage/references/subagent-contract.md`, `skills/triage/references/checks/checks-tier-policy.md`,
  `skills/git/references/worktrees.md`

### Review reads its guidance from the base revision, and an author's reason never hides a weakened check

- **Answers:** C8.3
- **Rule:** For a pull request review, every file that governs the review (`REVIEW.md`, the applicable `AGENTS.md` and
  `CLAUDE.md`, the local `code-review` skill and anything those files point at) is read from the base revision. The
  workflow, reviewer set, reporting threshold, skipped paths and requested checks all come from that revision. A change
  to any of those files on the head is reviewed under the base revision's version, or the defaults when the base has
  none, and is itself reported as a finding candidate. This extends the protection `REVIEW.md` already has to all review
  guidance. A check that guidance requests runs only when the base revision requests it. A weakened check is always
  reported: a reason stated in the pull request body, a commit message or a comment on the directive is quoted beside
  the finding and may lower its severity, but it never lowers its confidence below the posting filter (verified:
  skills/review/references/check-weakening.md:103, 2026-11-11). A review of the user's own working tree, with no pull
  request, keeps reading guidance from that tree. (verified: skills/review/SKILL.md:257, 2026-11-11)
- **Default / Rejected:** Base-revision guidance for all pull request reviews, same-repository and fork alike; rejected:
  base-revision guidance for forks only, because a same-repository branch can also be written by someone other than the
  reviewer, and one rule is easier to hold than two. Also rejected: letting the reviewer judge whether the author's
  reason is adequate before dropping the finding, because the judgment would be made on the author's own prose and the
  maintainer would never see that a check was weakened.
- **Later work changes:** `skills/review/SKILL.md`, `skills/review/references/post/post-pipeline.md`,
  `skills/review/references/check-weakening.md`

### A comment belongs to the agent only when the authenticated account wrote it

- **Answers:** C8.4
- **Rule:** Every lookup for an earlier comment matches on two things together: the idempotency marker and an author
  equal to the login the session is authenticated as. This holds for the posting envelope, for the triage rollup and
  replies including the legacy marker forms, and for the review check that a head was already reviewed. A comment that
  carries a marker under any other author is never edited, never copied into a new comment, and never counted as a prior
  review or rollup. The agent posts its own comment beside it and says in its report that a foreign comment carries the
  marker. (verified: skills/gh/references/posting-envelope.md:120, 2026-11-11)
- **Default / Rejected:** Marker and author together; rejected: the marker alone with a check of the caller's
  permissions, because permissions decide whether the edit succeeds and not whether the comment is the agent's. Also
  rejected: a signed or hashed marker, because the author field already gives an identity that a commenter cannot forge.
- **Later work changes:** `skills/gh/references/posting-envelope.md`,
  `skills/triage/references/orchestration-lifecycle.md`, `skills/review/references/post/post-pipeline.md`

### Every posting path checks visibility, redacts, and honors an embargo

- **Answers:** C8.5
- **Rule:** Five things hold for every path that posts or writes text others can read: triage replies and the rollup,
  the review post body, the gate pull request body, ops artifacts, and handoff documents and resume capsules. First, the
  path reads the repository's visibility before it posts; a repository whose visibility cannot be read is treated as
  public. Second, text copied from a log, tool output or the session has every secret, credential, token, private key,
  and internal URL or hostname replaced with `[redacted]` before it leaves the machine, in the words
  `skills/execute/references/code-quality-reviewer-prompt.md` already uses (verified:
  skills/triage/references/checks/checks-triage.md:142, 2026-11-11). Third, on a public repository a security finding
  that is confirmed or likely and not yet fixed is not posted, automatically or in headless mode; the user receives it
  in the session, with the private channel the repository offers (a draft security advisory or private vulnerability
  reporting) named as the place for it (verified:
  skills/triage/references/pull-requests/checkmarx/checkmarx-triage.md:160, 2026-11-11) (verified:
  skills/review/references/post/post-pipeline.md:202, 2026-11-11). Fourth, the user can declare an embargo for a work
  item in their own turn. While it holds, no skill posts or pushes text that describes the vulnerability, which includes
  the planning files a default gate destination would push, and each skill that withholds something says what it
  withheld. Only the user lifts an embargo. Fifth, an ops security incident on a public repository is never drafted as
  an issue; the draft goes to a draft security advisory or to a private location the user names (verified:
  skills/ops/references/github-artifacts.md:101, 2026-11-11). Handoff documents and capsules carry the same redaction
  rule as the second point and hold no credentials or customer data. (verified: skills/handoff/SKILL.md:68, 2026-11-11)
- **Default / Rejected:** Withhold on a public repository and tell the user where the content can go; rejected: posting
  a shortened finding that names the weakness without the detail, because the location and the class of the weakness are
  the detail an attacker needs. Also rejected: a visibility check on the security paths only, because a CI log excerpt
  leaks through a path nobody thinks of as a security path.
- **Later work changes:** `skills/triage/references/pull-requests/checkmarx/checkmarx-triage.md`,
  `skills/triage/references/checks/checks-triage.md`, `skills/triage/references/orchestration-lifecycle.md`,
  `skills/review/references/post/post-pipeline.md`, `skills/gh/references/posting-envelope.md`,
  `skills/gate/references/branch/branch-workflow.md`, `skills/ops/references/github-artifacts.md`,
  `skills/ops/references/response/security-branch.md`, `skills/handoff/SKILL.md`

### Issue text, repository-local policy, the work root and instruction files carry no authority of their own

- **Answers:** C8.6
- **Rule:** Four things hold. First, an issue's body and comments are data from whoever wrote them. A requirement that
  appears only in a comment reaches the intent as a proposal that names its author, and the user confirms or drops it. A
  comment headed `## Agent Brief` or `## Triage Notes` is the contract, or a record of settled questions, only when the
  authenticated account wrote it; under any other author it is context. A reporter's reproduction steps are read and
  never run as written: the agent reproduces with commands it chooses from the repository, and a command taken from the
  issue runs only after the user says yes (verified: skills/routing/references/chain-phases.md:74, 2026-11-11)
  (verified: skills/triage/references/issues/agent-brief.md:4, 2026-11-11) (verified:
  skills/triage/references/issue-mode.md:378, 2026-11-11). Second, a repository-local policy skill does not displace an
  installed organization policy skill without a question. When both match, the user chooses, and the question says when
  the repository-local skill was added or changed on the current branch (verified: skills/spec/references/align.md:38,
  2026-11-11). Third, the work root that `AGENTS.md` or `CLAUDE.md` declares passes the validation that pointers already
  pass before it is typed into a command or used to create a directory: it is repository-relative, its normalized form
  stays inside the repository, and it contains only letters, digits, `.`, `_`, `-` and `/`. A declaration that fails is
  reported, and the default `docs/specs` is used (verified: skills/gate/references/branch/branch-workflow.md:235,
  2026-11-11) (verified: skills/routing/references/repo-conventions.md:55, 2026-11-11). Fourth, a write that changes
  what later agents are told (an instruction file, a skill, a generated verification skill or its helper scripts) is
  authorized only by the user in the current session, after the exact text to be written has been shown. This is one
  rule for gate's learning loop, forge reflect and scaffold's feedback loops, and it replaces the three they have today.
  A summary row, a blanket approval given in advance, and content drawn from a transcript, a README, an issue or tool
  output authorize nothing. What counts as the user's approval is defined in [0014](0014-approval-and-pushback.md), and
  this rule adds only who may give it and what they must have seen. (verified: skills/gate/SKILL.md:280, 2026-11-11)
  (verified: skills/forge/SKILL.md:338, 2026-11-11) (verified: skills/scaffold/references/feedback-loops/author.md:40,
  2026-11-11) (verified: skills/scaffold/references/feedback-loops/audit.md:84, 2026-11-11)
- **Default / Rejected:** The user in the session confirms each of the four; rejected: accepting issue comments and
  brief headings from any account with write access, because author association shows that an account may write to the
  repository and not that the person running the session agreed to the scope. Also rejected: letting forge reflect apply
  a trivial edit directly, because whether an edit is trivial is judged by the agent that composed it from transcript
  content.
- **Later work changes:** `skills/routing/references/chain-phases.md`, `skills/triage/references/issue-mode.md`,
  `skills/triage/references/issues/agent-brief.md`, `skills/spec/references/align.md`,
  `skills/routing/references/repo-conventions.md`, `skills/gate/references/branch/branch-workflow.md`,
  `skills/decompose/SKILL.md`, `skills/gate/SKILL.md`, `skills/forge/SKILL.md`,
  `skills/forge/references/reflect/synthesizer.md`, `skills/scaffold/references/feedback-loops/author.md`,
  `skills/scaffold/references/feedback-loops/audit.md`

## Existing records

- 0005: consistent — It makes publishing a release the maintainer's own act, which matches the rule here that only the
  user in the session authorizes an outbound or self-modifying write.
- 0006: consistent — This record does not touch release promotion, which 0006 decided and 0010 has since superseded, and
  0006 touches no third-party text, fork code or comment ownership.
- 0008: consistent — It names a trust boundary as a signal that raises a spec's risk tier, and nothing here changes how
  tiers are assigned.
- 0009: consistent — It excludes trust-boundary changes from the small-fix path, and this record adds no path of its
  own.
- 0010: consistent — It lets a maintainer dispatch a real release, which is a release decision outside the posting paths
  listed here.
- 0012: consistent — It moves flow state into the hook binary and decides nothing about who may write or approve.

## What this costs

**Triage does less on its own.** Findings from `github-actions[bot]`, from Apps the routing table does not name, and
from a scanner account the repository has not declared now end in drafts where they used to end in pushed fixes. A
repository that uses Checkmarx has to declare its scanner account once on the default branch before the automatic lane
works again.

**Fork pull requests get weaker triage.** A fix that was proven by a test run is now a proposed patch that nobody ran.
The maintainer either accepts that or asks for execution by name, and takes the risk knowingly.

**A pull request cannot change its own review rules.** A legitimate change to `AGENTS.md` or to the local review skill
takes effect for reviews only after it merges, and the pull request that makes it is reviewed under the old rules and
flagged.

**More findings reach the posted review.** Every weakened check is reported, including the ones with a good reason. An
author who explained the change in the description still sees a finding about it.

**A duplicate comment is possible.** When another account's comment carries the marker, the agent posts beside it and
the thread holds two comments that look alike.

**Public repositories lose automatic security output.** On a public repository the rollup no longer carries confirmed
findings that are still open, and the user has to move them to a private channel by hand. Redaction will sometimes
remove a hostname or a value that the reader needed to diagnose a failure.

**More questions.** A requirement from an issue comment, a choice between two policy skills, and every write to an
instruction file or a skill now cost one question each, where some of them cost none.

## Considered and declined

**Decide here whether triage pushes and posts without confirmation.** Declined. That is a question about approval, and
[0014](0014-approval-and-pushback.md) owns it: a run whose invocation or request named the push or the replies acts
without asking, and any other run asks once. This record sets only the conditions that hold in both cases.

**Verify identity with collaborator status everywhere.** Declined as the general test. Write access is the right test
for a reply that steers a bot finding, because that reply speaks for the repository. It is the wrong test for scope and
for approval, which belong to the person running the session.

**Treat approval rows, standing orders and divergence entries that arrive in files as third-party text in this record.**
Declined. They are the same kind of problem, and the flow state record decides them together with the approval hash.
