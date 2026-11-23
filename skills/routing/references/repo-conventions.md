# Repository conventions

Where a consumer repository keeps its glossary, context map, ADR directory, standards documents, pull request template,
and commit convention. Every Agentics skill and agent that reads or writes one of these resolves it here. Such a skill
or agent does not restate the lookup. Plan-parent resolution is separate: it lives in the
[decompose skill](../../decompose/SKILL.md), **Resolve the plan parent**.

This file also resolves eight facts about the repository that a skill acts on: the forge host, the canonical remote, the
base branch, the merge strategy, the verification command, commit sign-off and signing, the work root, and the storage
setting. A skill that needs one of these facts reads it here and never hard-codes `main`, `origin` or `github.com`.

## Precedence

The same three steps resolve every convention:

1. **Pointer.** A path or value named for that convention in the repository's `## Repository conventions` section, in
   `AGENTS.md`, `CLAUDE.md`, or `.claude/CLAUDE.md`, wins.
2. **Discovery.** Otherwise, what the convention's discovery list below finds.
3. **Default.** Otherwise, the default the list names.

Two conventions put steps in front of these three. [Base branch](#base-branch) and [Storage setting](#storage-setting)
each state their own order.

The forge host, the canonical remote, the base branch and the merge strategy have no default, because a default would
select a write target. When discovery finds nothing for one of them, the skill asks once and the answer holds for the
session. A contributor who cannot edit the instruction file is never asked to add a pointer. The session answer is
enough. A value the user states is a convention value, not a divergence.

## Conflicts

These are conflicts, not choices:

- Two of `AGENTS.md`, `CLAUDE.md`, and `.claude/CLAUDE.md` name different paths or values for the same convention.
- A pointer names a path that does not exist.
- A pointer names an absolute path, or one whose normalized form leaves the repository through `..`.
- Discovery finds two candidates at the same rank.

The missing-path conflict applies to the glossary, the context map, the ADR directory, the standards documents, the pull
request template and the commit convention, whose pointers name an existing file or directory. It does not apply to the
work root, which is created at the first save, or to a pointer whose value is not a path. A work-root declaration that
fails the validation under [Work root](#work-root) is not a conflict either: it is reported and `docs/specs` is used.

On a conflict, do these steps:

- Name every source with its path.
- Ask the user once which one is canonical, as a ❓ question with a ➡️ recommendation.
- Read or write nothing for that convention until the user answers.

On a conflict, never do any of these:

- Pick one silently.
- Fall through to the next step.
- Create a further file to sidestep the conflict.

An answer applies to the rest of the session. `scaffold`'s pointer offer records it permanently. A read-only audit, such
as `scaffold audit`, asks nothing. It reports the conflict as `failed` and names every source.

## Pointer section

An optional section in `AGENTS.md` (or `CLAUDE.md` or `.claude/CLAUDE.md`). Any line may be absent:

    ## Repository conventions

    - Glossary: <path>
    - Context map: <path>
    - ADRs: <directory>
    - Standards: <path>[, <path>]
    - PR template: <path>
    - Commit convention: <path>
    - Forge host: <host>
    - Canonical remote: <repository URL>
    - Base branch: <branch name>
    - Merge strategy: <merge method>
    - Verification command: <command>, or "none"
    - Commit sign-off and signing: <requirement>
    - Work root: <directory>
    - Storage setting: committed or local

Paths are repository-relative. Nothing is read or written through a pointer that is absolute or leaves the repository.
The planning-parent declaration keeps its own form.

## Discovery

### Glossary and context map

1. A root `GLOSSARY-MAP.md` or `CONTEXT-MAP.md` means **multiple contexts**. Route the topic to its context through the
   map, per [context-map.md](../../deepen/references/context-map.md).
2. Otherwise a root `GLOSSARY.md` or `CONTEXT.md` is the **single context** glossary.
3. Otherwise there is no glossary yet. Default: `CONTEXT.md` at the root, created lazily by `deepen` only, when the
   first term is resolved.

A `Context map:` pointer means multiple contexts wherever its map lives. Both naming schemes have equal standing. Both
map files, or both glossary files at the same level, are a conflict.

### ADR directory

Candidates, in order: `docs/adr`, `docs/decisions`, `docs/architecture/decisions`, `adr`, `doc/adr`. The one directory
that holds ADRs is used. Two or more that hold ADRs are a conflict. An empty directory holds no decisions, so it never
competes with one that does. When none holds ADRs, the first that exists is used. Default: `docs/adr`, created lazily by
`docs` with the first ADR.

In a multi-context repository, the same candidates and rules find a context's own ADR directory. They apply relative to
the directory that holds that context's glossary (for example `src/billing/docs/adr`). Its default is `docs/adr` there,
created lazily the same way. System-wide decisions stay in the root directory resolved above. It is the only directory
that the `ADRs:` pointer names.

### Standards documents

Read every one of these that exists, together and with no ranking:

- `CONTRIBUTING.md` (in `.github/`, the root, then `docs/`, GitHub's order)
- `STYLE.md`, `STANDARDS.md`, `STYLEGUIDE.md`, and `CODING_STANDARDS.md` at the root or under `docs/`

A contradiction between two standards documents is a review finding, not a resolver conflict. No default.

### Pull request template

`pull_request_template.md`, matched case-insensitively, in `.github/`, the root, or `docs/`, in that order. When no such
file exists, use a `PULL_REQUEST_TEMPLATE/` directory in the same locations and order. Use its only template if it holds
one. If it holds several, ask the user once, and use the one the user picks. Default: the calling skill's own shape.

### Commit convention

The first of these that exists is the convention:

1. A commitlint configuration at the root (`commitlint.config.*`, `.commitlintrc`, `.commitlintrc.*`, or a `commitlint`
   key in `package.json`). Its rules are the convention.
2. A commit message template: the file `git config --get commit.template` names, else a root `.gitmessage`.
3. Recent history: when at least 15 of the last 20 subjects (`git log -20 --no-merges --format=%s`) share one shape,
   that shape is the convention. Examples of a shape are Conventional Commits types, a ticket prefix like `ABC-123:`, or
   a bracketed component.

A pointer names the document that states the convention. Default: Conventional Commits, as
[commit.md](../../git/references/commit.md#fallback-conventional-commits-format) describes. Two commitlint
configurations at the root are a conflict. History that disagrees with a configuration is not a conflict, and the
configuration wins.

### Forge host

The host in the canonical remote's URL. Default: none. A skill that needs the host asks once.

### Canonical remote

The canonical remote is the base remote: the repository the work is delivered to, which in a fork clone is the parent
and not the fork. The first of these that exists is the canonical remote:

1. The remote whose repository is the base of the branch's open pull request.
2. The remote that the host reports as the parent of a fork.
3. The only remote, when there is one and the host does not report its repository as a fork.

A pointer wins over this list, as [Precedence](#precedence) states. It names the repository; the canonical remote is the
remote that points at it. The pointer is read from a trusted revision, never from the branch being resolved, and it
replaces the base of an open pull request only with the user's confirmation, as
[base-and-remote.md](../../git/references/base-and-remote.md#how-to-call-it) says.

When the host reports a parent that no remote points at, the canonical remote is the parent's URL, and fetches use that
URL. It never selects the fork.

Default: none. With several remotes and nothing found, the skill asks once. The push remote is a separate fact that
[git](../../git/SKILL.md) resolves.

[base-and-remote.md](../../git/references/base-and-remote.md) is the discovery procedure for the canonical remote and
the base branch.

### Base branch

The first of these that exists is the base branch:

1. A base the user named, or one already recorded for the work item.
2. The base of the branch's open pull request.
3. The default branch of the canonical remote, read from that remote's `HEAD` ref.

A pointer gives the repository's usual base branch and stands in for the third step. The first two steps belong to one
work item and still come first. Default: none. When no step finds a base, the skill asks one question and does not
guess.

### Merge strategy

The first of these that exists is the merge strategy:

1. The one merge method the host allows for the repository, when it allows exactly one.
2. A merge method stated in a standards document.

A pointer names the merge method and wins over this list, as [Precedence](#precedence) states.

Default: none. A skill about to merge asks once.

### Verification command

The first of these that exists is the verification command:

1. A test or verification command stated in the instruction file or a standards document.
2. The test script of the package manifest or build file at the repository root.

A pointer may state that the repository has no command. Default: none, which is the same statement. The delivery path
then verifies by the closest executable check.

#### Running it

The command runs the branch's own code. Every skill that runs it, or a repository test, decides first whose branch it is
working on and who must ask for the run:

- **Fork code.** This rule applies the cases of git's [Fork code](../../git/references/worktrees.md#fork-code) rule to
  the branch a skill works on, including a branch already checked out or reused. git's sentence that a request with no
  start point is not fork code speaks only to creating a worktree. When the invocation names a pull request, that pull
  request is the one checked, not the current branch's own, and with the host unreadable a named pull request is fork
  code. The branch is fork code when its pull request comes from another repository, when its upstream merge ref is
  `refs/pull/<number>/head`, or when its upstream is a remote or URL that git's rule would call fork code, such as a
  branch `gh pr checkout` made from a fork's pull request. Fork code runs nothing unless the user, in their own turn,
  names that pull request or ref and asks for it to run.
- **Another person's branch.** A same-repository branch is another person's when its open pull request has an author
  other than the authenticated host account, or the host reports no author for it. Compare the pull request's author
  login with the authenticated login, for example from `gh pr view --json author` and `gh api user`. A failed or
  unavailable read falls under the host clause below. Git author and committer names never decide ownership
  ([untrusted-data.md](contracts/untrusted-data.md)). Show the exact command first, and run it only when the user
  approves it, names the command, or asked for the build or tests to run. In a run whose dispatcher answers for the
  user, run it only when the dispatching prompt authorized it. An approval covers that command on that branch for the
  rest of the session, and a dispatching prompt carries it to the workers it dispatches.
- **Every other branch** is the user's, including a branch from an earlier session, a resumed branch and the default
  branch. Run the command without asking.
- **The host cannot be read** (`gh` is missing, is not signed in, or a lookup fails). The default branch and a branch
  with no upstream are the user's. Any other branch is another person's. The fork-code clause still decides first.
- **A named verification is the request.** When the user, in their own turn, names a branch or pull request to a skill
  whose workflow runs the command on it, that is the request for the tests to run. gate's readiness check, triage on a
  same-repository pull request, and git's worktree baseline on a start point the user named are such workflows. The
  skill shows the command in its report. The fork-code clause still decides first: naming fork code to such a skill is
  not asking for it to run. A caller's argument is not the user's naming, as in git's Fork code rule. Invoking a skill
  whose method includes running tests (tdd, diagnose, hillclimb, scaffold, execute, zealot, handoff or prototype) is not
  that request.
- **Not authorized.** Run nothing, and report the checks unverified with the command that would have run.

### Commit sign-off and signing

- **Signing.** `git config --get commit.gpgsign` and `git config --get user.signingkey`.
- **Sign-off.** A sign-off requirement stated in a standards document.

Default: sign as git is configured and add no sign-off line. Signing is never bypassed.

### Work root

The directory that holds the work folders. No discovery: only a pointer changes it. Default: `docs/specs`.

A declared work root is validated before it is typed into a command or used to create a directory:

- It is repository-relative.
- Its normalized form stays inside the repository.
- It contains only letters, digits, `.`, `_`, `-` and `/`.

A declaration that fails is reported, and the default `docs/specs` is used.

### Storage setting

One setting with two values decides where a work folder lives:

- `committed` commits the planning files and `state.tsv` under the work root.
- `local` keeps the work folder, with its planning files and `state.tsv`, in a clone-local store under the Git common
  directory. Worktrees share the store, and nothing in it is staged.

The setting resolves in this order:

1. The user's stated or configured value. It holds for their clone, because a contributor cannot edit the instruction
   file.
2. The pointer.
3. Discovery.
4. The default, `committed`.

Discovery looks for two cases:

- A work root that is ignored or banned for committed planning files.
- A canonical remote the user has no push permission on, which marks a repository the user does not own.

A discovered case leads to one proposal of `local` when the run is attended. An unattended run (defined in
[unattended.md](contracts/unattended.md#when-a-run-is-unattended)) stops unless a value is configured. Discovery never
switches the setting on its own, and no skill changes where the trail is stored without telling the user.

## Writers

- `deepen` writes terms at the resolved glossary. It decides when a decision merits an ADR. It creates the default
  glossary only when the contract resolves none.
- `docs` writes an ADR into the resolved ADR directory. It creates the default directory only when the contract resolves
  none.
- `scaffold` proposes `## Repository conventions` lines through individual previews
  ([agents-md.md](../../scaffold/references/community/agents-md.md)). Its explicit `init` creates standards documents
  and the pull request template through the same previews. `docs` edits standards documents on request.
- `git` words every commit message in the resolved convention. Other skills state what a commit is for and hand the
  wording to `git`.
- No other skill or agent writes the glossary, the context map, or ADRs.
