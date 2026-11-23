# Skills resolve the environment from one conventions file, and a deliberate hook rule outranks skill text

The skills assume a repository on github.com with a `main` branch, a test suite, a current Bash or PowerShell 7, a
person at the keyboard and a short history. Nothing reconciles a repository or host that differs, and in several places
the plugin's own hooks deny a command its skills require. This record decides how a skill learns the environment, which
hosts and shells are supported and what happens on the others, what "unattended" means, which side changes when a hook
and a skill disagree, and how steps that grow with repository size are bounded. The findings come from
`docs/designs/skill-audit-2026-11/consolidated-issues.md`, theme T10.

## Rules

### The conventions file resolves every environment fact a skill acts on

- **Answers:** C10.1
- **Rule:** `skills/routing/references/repo-conventions.md` gains seven conventions beside the six it has: forge host,
  canonical remote, base branch, merge strategy, verification command, commit sign-off and signing, and work root. Each
  resolves by the file's existing order of pointer, discovery, default, and each gets a discovery list (for the base
  branch, the order [0016](0016-git-write-ownership.md) decides: a base the user named or one already recorded for the
  work item, then the base of the branch's open pull request, then the default branch of the base remote; for the host:
  the canonical remote's URL). A skill that needs one of these facts reads it there and never hard-codes `main`,
  `origin` or `github.com`. When discovery finds nothing and the default would select a write target, the skill asks
  once and the answer holds for the session. A contributor who cannot edit the instruction file is never asked to add a
  pointer; the session answer is enough, and a value the user states is a convention value, not a divergence. The
  work-root declaration that `skills/execute/references/flow-state.md` reads from `AGENTS.md` moves into the same
  pointer section. The verification line may say that the repository has no command; what a delivery path does then is
  decided in [0013](0013-proportional-delivery-paths.md). (verified: skills/routing/references/repo-conventions.md:3,
  2026-11-11)
- **Default / Rejected:** one lookup file with one precedence order; rejected: a lookup in each skill that needs the
  fact, because that is the present state, in which gate tries `main` then `master`
  (`skills/gate/references/branch/branch-workflow.md:66`) and each skill guesses differently.
- **Later work changes:** `skills/routing/references/repo-conventions.md`,
  `skills/routing/references/user-authorized-divergence.md`, `skills/gate/references/branch/branch-workflow.md`,
  `skills/execute/references/flow-state.md`, `skills/scaffold/`

### github.com and GitHub Enterprise Server are supported, and a skill checks the forge host before any GitHub step

- **Answers:** C10.2
- **Rule:** github.com and GitHub Enterprise Server are supported; every other forge is out of scope. Skills build links
  and `gh` host arguments from the resolved forge host, never from the literal `github.com`. A skill that needs GitHub
  checks the resolved forge host first. On a host outside the supported set it says `host not supported: <host>` once,
  finishes its local work, and runs no `gh` command. On a supported host or plan that has no draft pull requests, the
  skill pushes the branch, says that drafts are unavailable, and hands the user the create command; it opens a ready
  pull request itself only where the draft rule, which [0014](0014-approval-and-pushback.md) makes a repository policy,
  has been switched off in the hook settings. (verified: skills/gh/references/posting-envelope.md:76, 2026-11-11)
- **Default / Rejected:** github.com and GitHub Enterprise Server supported, every other forge out of scope; rejected:
  supporting only github.com and declaring Enterprise Server out of scope in `README.md` and `docs/surfaces/README.md`,
  because the difference between the two is replacing a literal host with a resolved one, which the conventions rule
  above already requires, and an enterprise installation would get no pull request, triage or posting step from
  Agentics, which works against the enterprise direction `README.md` states.
- **Later work changes:** `skills/gh/references/posting-envelope.md`, `skills/gh/SKILL.md`,
  `skills/gh/references/projects.md`, `skills/gate/references/change/adapters/github-issue.md`,
  `skills/triage/SKILL.md`, `README.md`, `docs/surfaces/README.md`

### PowerShell 7 and Git Bash are the supported Windows shells, and Windows PowerShell 5.1 is out of scope

- **Answers:** C10.4
- **Rule:** PowerShell 7 and Git Bash are the supported Windows shells; Windows PowerShell 5.1 is out of scope. A skill
  that detects 5.1 says so once and names the two supported shells, and the supported-shell statement is added to
  `README.md` and the surface documents. A mandatory step has a Bash form and a PowerShell form, or its skill names the
  one shell it needs and stops with that line on a host without it. Bash snippets run on bash 3.2, or the skill states
  the version it needs (associative arrays need 4.0). Every path is quoted, and worktree and file lists are read as
  whole lines, never as whitespace-separated fields. No skill text depends on a file's line endings, and approval
  hashing normalizes line endings as [0017](0017-flow-state-v2.md) decides; the CR-preserving check of
  [0012](0012-flow-state-is-a-verb-on-the-hook-binary.md) remains only for approvals written under it. (verified:
  skills/git/references/worktrees.md:404, 2026-11-11)
- **Default / Rejected:** PowerShell 7 as the minimum, beside Git Bash; rejected: supporting Windows PowerShell 5.1 by
  limiting every PowerShell snippet to 5.1 syntax (no `??`, `&&`, `||` or ternary) under a conformance check, because no
  test runs on 5.1 today and the flow helper no longer needs `pwsh -File`, so the remaining 5.1 gap is snippet syntax
  that would have to be policed from then on: every existing snippet audited, some rewritten, a check that parses
  PowerShell fences, and a 5.1 runner before the claim is true.
- **Later work changes:** `skills/git/references/worktrees.md`, `skills/triage/references/orchestration-lifecycle.md`,
  `skills/triage/references/subagent-contract.md`, `README.md`, `docs/surfaces/`

### A deliberate hook rule outranks skill text; a heuristic false positive is a hook defect

- **Answers:** C10.5
- **Rule:** when a hook denies a command a skill prescribes, the side that changes depends on why the hook denies it.
  Where `hooks/README.md` states the denial as a rule, the skill changes, and it never prescribes a spelling whose
  effect is to avoid inspection. Under this, the git skill stops offering `git reset --hard <ref>` and offers
  `git restore` or `git stash`, or hands the command to the user; gate keeps its saved branch name outside the Git
  directory, so its cleanup can be spelled; triage's harness call stays inspected and its text says a harness must not
  contain denied commands; triage's posting fallback and recovery list, review's dependency guide and diagnose's
  environment probe are respelled without a trailing `rm`, `xargs -r`, a credential-file read or `env | grep`. Where a
  heuristic denies a form the skills recommend as the safe one, the hook is corrected as an ordinary change under 0004;
  this covers the secrets write-scan on an environment-variable read and on the `${user_config.KEY}` placeholder. A hook
  denial of a flow call is reported as `flow unavailable: denied by a hook`, with the write rule unchanged. The
  change-record skill tells the user to set `AGX_RELEASE_RECORD` before a deploy. The draft-only pull request rule and
  the review-thread rule the same settings table governs are repository policy, as [0014](0014-approval-and-pushback.md)
  decides: a skill follows each while its setting is on and names the setting once as the exit. The alert-dismissal rule
  is a repository policy in the same class, because the hook setting `policy.block_security_alert_dismissal` switches it
  (`hooks/README.md:1899`): a skill prescribes no dismissal while the setting is on and names the setting once as the
  exit. A conformance check runs each command a skill prescribes through the committed hook binary under default policy
  and fails on a denial. A command a skill prescribes only for a repository policy that has been switched off is checked
  under that setting, and under default policy the check expects its denial. (verified: hooks/README.md:239, 2026-11-11)
- **Default / Rejected:** the hook is the reference and skill text follows it; rejected: carving a hook exception for
  each skill command, because the hook sees only command text and an exception for a skill is an exception for any agent
  that types the same text.
- **Later work changes:** `skills/git/SKILL.md`, `skills/git/references/guide.md`,
  `skills/gate/references/branch/branch-workflow.md`, `skills/gate/references/change/`,
  `skills/triage/references/subagent-contract.md`, `skills/triage/references/orchestration-lifecycle.md`,
  `skills/triage/references/recovery.md`, `skills/review/references/security/domains/dependency-review-guide.md`,
  `skills/diagnose/SKILL.md`, `skills/routing/references/chain-phases.md`, `hooks/go/internal/secretsguard/`,
  `hooks/README.md`, `.github/tools/src/conformance/domains/skills/`

### A skill branches only on a condition it can read

- **Answers:** C10.6
- **Rule:** a run is unattended when one of three things is true, and attended otherwise: the dispatching prompt says no
  user will answer (a dispatched subagent always qualifies, and its dispatcher answers its questions); the invocation is
  non-interactive and the prompt or host context says so (a print-mode, CI or scheduled run); or the user said in this
  conversation that they will not answer. Redirected standard input or output never makes a run unattended, so the
  terminal probe in `skills/decompose/SKILL.md` stops deciding anything about presence. The definition lives in a new
  file, `skills/routing/references/unattended.md`, and every skill that says "unattended", "headless" or "AFK" links to
  it. No skill branches on the user being away: it asks, and where its text says not to block, it states its assumption
  and continues. No skill promises an action at a later time unless it names the host tool that will run it; otherwise
  it tells the user what to check and when, and acts on the next message. A condition described as invocation state is
  restated as text the skill can read: the dispatching prompt, a file or a command's output; which of those is trusted
  is decided in [0018](0018-trust-boundaries.md). `context: fork` is removed from review and threat-model, because a
  forked skill starts without the conversation and both skills read it and wait for an answer. A skill's `allowed-tools`
  lists every tool its mandatory steps use. A path rule that differs by operating system gives each form or finds the
  path by listing. (verified: skills/decompose/SKILL.md:126, 2026-11-11)
- **Default / Rejected:** attended unless one of the three conditions is read; rejected: inferring absence from a
  terminal probe or from silence, because the probe reads the tool's pipes, not the person, and silence within a turn is
  not observable.
- **Later work changes:** `skills/routing/references/unattended.md`, `skills/decompose/SKILL.md`, `skills/tdd/SKILL.md`,
  `skills/intent/SKILL.md`, `skills/triage/SKILL.md`, `skills/diagnose/SKILL.md`, `skills/review/SKILL.md`,
  `skills/threat-model/SKILL.md`, `skills/forge/SKILL.md`, `skills/gate/references/release/release-workflow.md`,
  `skills/ops/references/communication-templates.md`, `.github/tools/src/conformance/domains/skills/frontmatter.ts`

### A step whose cost grows with the repository states its bound

- **Answers:** C10.7
- **Rule:** every step whose cost grows with history, diff size or the length of a remote list names a bound in its
  skill text and says what it does at the bound; it never samples silently. The per-request continuation check and
  `flow where` must not cost more as finished work folders accumulate; how the lookup narrows is left to the work on
  `hooks/go/internal/flow/`; no decision record covers it. Review states a diff size above which it reports the size and
  asks whether to narrow the range or review in slices. Issue mode lists with an explicit limit and reports how many
  issues exist and how many it read. The bounds are numbers in the owning skill, set from a measured run. (verified:
  hooks/go/internal/flow/lookup.go:44, 2026-11-11)
- **Default / Rejected:** a stated bound and a stated action at the bound; rejected: leaving cost unbounded until a
  large repository reports it, because the continuation check runs on every request to start or continue work.
- **Later work changes:** `hooks/go/internal/flow/`, `skills/routing/references/chain-phases.md`,
  `skills/review/SKILL.md`, `skills/triage/references/issue-mode.md`

## Existing records

- 0001: consistent — it is already superseded by 0004, and nothing here restores a per-surface difference.
- 0011: consistent — the install path stays Bash; the shell rules here govern skill snippets, not install scripts.
- 0004: consistent — the Go hook engine stays the reference, and the hook corrections named here are ordinary changes
  under it.
- 0008: consistent — its "unattended run leaves the intent unapproved" rule stands and now has a definition to test
  against.
- 0012: amended — the `flow unavailable` reasons gain `denied by a hook`; the direct binary call it introduced already
  removed the Windows flow-helper collision.

## What this costs

**More questions on unusual repositories.** A repository whose base branch or host cannot be discovered now gets one
question per session where a skill used to guess. A contributor without write access to the instruction file answers it
again in each session.

**Some skill commands get weaker or move to the user.** The git skill loses `git reset --hard <ref>` as something it
runs. Where the replacement does not do the same job, the user types the command.

**A new conformance check that needs the hook binary.** Checking prescribed commands against the hook ties the skills
conformance domain to the committed binary and to a parser for command fences. A snippet with placeholders needs a rule
for what is substituted before the check can run it.

**Review and threat-model run in the main conversation.** Removing `context: fork` puts their reading and output into
the caller's context window.

**Bounds will sometimes stop work a user wanted.** A review that reports a size and asks is one more turn on a large
change.

**Enterprise Server support is claimed without a test host.** Every hard-coded host in the skills is rewritten, and
Enterprise Server behavior is claimed with no host to test it on until someone supplies one.

**A host restricted to Windows PowerShell 5.1 cannot run the PowerShell snippets.** It gets one line that names the two
supported shells, and `README.md` and the surface documents have to say so.

## Considered and declined

**A separate environment file per repository, written by Agentics.** Declined. It would be a planning file in a
repository the user may not own, and the instruction file already carries the pointer section.

**Telling skills to call an uninspected spelling.** Declined. `./harness.sh` passes where `bash harness.sh` is read, and
a skill that prefers the first is steering around its own plugin's guard.

**Detecting an unattended run from the environment.** Declined. No variable or probe is common to the three hosts, and
the one probe in use reports the tool's pipes.
