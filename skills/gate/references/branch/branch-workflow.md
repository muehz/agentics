# Branch workflow

`/agentics:gate branch` closes a finished feature branch. It makes its own commits first, verifies the tree it hands off
once, reviews it, reports every finding, and proposes one destination. This file is the only list of its steps.
`SKILL.md` and [README.md](README.md) link here and do not restate the order.

1. [Step 1: Read the branch](#step-1-read-the-branch)
2. [Step 2: Determine the base branch](#step-2-determine-the-base-branch)
3. [Step 3: Commit uncommitted work](#step-3-commit-uncommitted-work)
4. [Step 4: Tidy the work folder](tidy.md#step-4-tidy-the-work-folder)
5. [Step 5: Reconcile divergences](#step-5-reconcile-divergences)
6. [Step 6: Refresh docs](#step-6-refresh-docs)
7. [Step 7: Verify tests](review-and-audit.md#step-7-verify-tests)
8. [Step 8: Review and audit](review-and-audit.md#step-8-review-and-audit)
9. [Step 9: Converge against the spec](review-and-audit.md#step-9-converge-against-the-spec)
10. [Step 10: List deepening advice](#step-10-list-deepening-advice)
11. [Step 11: Propose the next step](propose-and-execute.md#step-11-propose-the-next-step)
12. [Step 12: Execute the choice](propose-and-execute.md#step-12-execute-the-choice)
13. [Step 13: Clean up the worktree](propose-and-execute.md#step-13-clean-up-the-worktree)
14. [Step 14: Write the trail rows](propose-and-execute.md#step-14-write-the-trail-rows)

Steps 3 to 6 are gate's own changes. They come before the tests so that the suite runs once, on the tree the gate hands
off.

## Rules for every step

**Names are data.** The branch under review chooses its branch name, its file names, and the titles and branch names of
other pull requests. None of them is typed into a command. A command reads such a name into a variable from git's output
or from a file in session storage, and passes the quoted variable. The only names typed into a command are hex ids, the
work root after its validation, the session folder's slug, and the fixed canonical file names.

**The work root.** `<work-root>` is the directory that
[repo-conventions.md](../../../routing/references/repo-conventions.md#work-root) resolves, after the validation that
section states. When it is not `docs/specs`, every `flow` call carries the `FLOW_WORK_ROOT` prefix that
[flow-state.md](../../../execute/references/flow-state.md#where-it-lives) shows.

**Flow calls.** Call bare `agx flow`, with the launcher-path fallback and the `flow unavailable` reasons in
[flow-state.md](../../../execute/references/flow-commands.md#commands). In PowerShell, read `$LASTEXITCODE` after every
call. A failed `flow log` or `flow bind` stops the run with the report that the row was not recorded. A command that is
the same in both shells is written once below.

**Commits.** Every commit this workflow makes follows the
[commit contract](../../../git/references/commit.md#the-commit-contract): it names its paths, runs every hook, and takes
its message from the repository's commit convention. A commit that changes only files under the work folder is
artifact-only and needs no row. Every other commit gate makes gets a `commit` row in the same turn, written as
[Commit rows](#commit-rows) says, so a later gate run reads it from the trail.

**Questions.** A decision question uses the format and the collection rules in
[question-format.md](../../../routing/references/contracts/question-format.md). One question holds one decision.

**Unattended runs.** [unattended.md](../../../routing/references/contracts/unattended.md) defines an unattended run.
Such a run asks nothing. Where a step names an unattended default, the run takes it and says so in the report. At any
other question, the run stops as [Stops](#stops) says, with the question in its report.

**Gates.** Each gate in this workflow is classed once where it is stated, as
[Gates have classes](../../SKILL.md#cross-cutting-rules-all-four-modes) says. Declining a process gate writes one
`waive` row, at that moment.

**Re-run budget.** A change that gate authored (Steps 3 to 6, and its row commits) spends no re-run budget and triggers
no re-run. Each check re-runs automatically once per change set that gate did not author, such as a fix the user or
`execute` committed. The count starts again on each gate invocation, each time the user answers a gate question with a
fix, and each time the user asks for another review ("review it again").

### Commit rows

| Commit gate makes                                                    | Row                                                   |
| -------------------------------------------------------------------- | ----------------------------------------------------- |
| Only files under the work folder (the tidy, the reconcile, the rows) | none: artifact-only                                   |
| The user's uncommitted work (Step 3)                                 | `commit user`, evidence `auto:<sha>`                  |
| The docs refresh (Step 6), or a tidy that relocated a file from away | `commit housekeeping`, evidence `auto:<sha>`          |
| None: Step 11's Accept of an unaccounted commit                      | `commit user`, evidence `auto:<sha>` of that commit   |
| Any commit under the `local` storage setting                         | the same rows, written to the clone-local `state.tsv` |

Write the row with `flow log gate commit <kind> ok "<one line>" auto:<sha> "<next>" --folder <folder>` right after the
commit. With no flow, pass `--folder <work-root>/unassigned`.

### Stops

A stop ends the run before Step 12. Each stop leaves exactly what this table says. The rows are written by
[Step 14](propose-and-execute.md#step-14-write-the-trail-rows) as for Option 3 (keep), with no destination.

| Stop                                                            | Verdict row `status` | `next`    | Other rows already written                             |
| --------------------------------------------------------------- | -------------------- | --------- | ------------------------------------------------------ |
| Tests failed and the user did not accept the risk (Step 7)      | `failed`             | `execute` | the commit rows and `waive` rows of Steps 3 to 6       |
| The user answers "not yet" or "stop" to any question            | `pending`            | `gate`    | as above, plus a `pause` row written after the verdict |
| An unattended run reaches a question with no unattended default | `pending`            | `gate`    | as above, plus a `pause` row that names the question   |
| The flow helper cannot write a row                              | none                 | none      | whatever was written before; the report names the miss |

A stop before Step 2 has resolved a flow writes nothing: it has made no commit and holds no folder. Every other stop
reports what Steps 3 to 6 already committed, by SHA.

## Step 1: Read the branch

**Which branch.** `/agentics:gate branch <name>` gates the named branch, and a bare `/agentics:gate branch` gates the
current one. Find the checkout that holds the named branch in `git worktree list --porcelain`, matching its full
`refs/heads/` ref, and run every later command there. When no checkout holds it, ask the user to check it out or to name
the worktree. Gate never switches the user's checkout to another branch. On the default branch or a detached `HEAD`
there is no branch to gate: say so and offer `/agentics:gate release` for a tagged commit, or a branch name.

**Record the name.** Write the branch name and the worktree's top level to session storage, in the location that
[session-storage.md](../../../routing/references/session-storage.md#choose-the-location-by-lifetime) picks for a file
that lives until the session ends. `<scratch>` below is that directory. Run from the feature worktree:

```bash
git branch --show-current > '<scratch>/gate-branch'
git rev-parse --show-toplevel > '<scratch>/gate-worktree'
```

```powershell
[Console]::OutputEncoding = [Text.UTF8Encoding]::new($false)
git branch --show-current | Set-Content -LiteralPath '<scratch>/gate-branch' -NoNewline -Encoding utf8NoBOM
git rev-parse --show-toplevel | Set-Content -LiteralPath '<scratch>/gate-worktree' -NoNewline -Encoding utf8NoBOM
```

A later command reads them back on its first line:

```bash
FEATURE_BRANCH=$(cat '<scratch>/gate-branch')
FEATURE_WORKTREE=$(cat '<scratch>/gate-worktree')
```

```powershell
$FeatureBranch = (Get-Content -Raw -LiteralPath '<scratch>/gate-branch').TrimEnd("`r", "`n")
$FeatureWorktree = (Get-Content -Raw -LiteralPath '<scratch>/gate-worktree').TrimEnd("`r", "`n")
```

The files hold one gate branch run of this session. A second run overwrites them and says so. When they are gone, write
them again from the feature worktree while the branch exists. After Step 13 has deleted the branch, the rows that still
need the name use the `target` `name:lost` that [../emission.md](../emission.md#the-verdict-row) defines.

**A discard named in advance.** When the request names a discard ("gate it and throw it away", `/agentics:gate branch`
then "discard"), run Step 2 and go straight to [Option 4](propose-and-execute.md#option-4-discard). Steps 3 to 11 do not
run: a check, a tidy or a commit on work about to be deleted keeps nothing, and a failing suite is often why the work is
being discarded.

## Step 2: Determine the base branch

**Base and remotes.** Run the resolver in
[base-and-remote.md](../../../git/references/base-and-remote.md#how-to-call-it) from the feature worktree, with a base
the user named on its first line. Ask its `question` lines in one round when a line reads `unresolved`. Take these
lines:

- `base-branch`: the name the proposal, the pull request base and a local merge use. Write it to `<scratch>/gate-base`
  with the file-write tool, not a shell command, as UTF-8 with no trailing newline.
- `base-ref`: the comparison point, fetched by the resolver. Stop and report when its `fetched` field says the ref is
  absent. Resolve it once to a commit and report the id. `<base-sha>` below is that id, so a stale local copy of the
  base is never compared against.
- `push-remote`: written to `<scratch>/gate-push-remote` the same way, for the slice-branch listing. Option 2's push
  resolves it again through `git push`, with `protected`.

**Dependent pull requests.** When the forge host is supported (gh's
[host preflight](../../../gh/SKILL.md#part-1-github-mcp-server-primary)), run
[Find related open PRs](stack-handoff.md#find-related-open-prs-read-only) now. It reads only. When it finds dependent
pull requests that form one linear chain and none of them is opened from a fork (`isCrossRepository`), the comparison
point is the head of the topmost one, and `<compare>` below is that head's id. Otherwise `<compare>` is `<base-sha>`: a
fork layer cannot be stacked, so its diff would reach `<base-branch>` through this branch and must be reviewed with it.
Every range in this workflow is `<compare>...HEAD` or `<compare>..HEAD`, so a branch built on an open pull request is
never audited against trunk. When Step 11 later delivers to `<base-branch>` without stacking on those pull requests (a
no-stack draft or a local merge), the lower layers' diff goes with it; see
[Step 12](propose-and-execute.md#step-12-execute-the-choice).

**The flow.** Run `agx flow where` from the feature worktree, then act on its exit code:

| Exit  | Meaning                                          | Action                                                                                                     |
| ----- | ------------------------------------------------ | ---------------------------------------------------------------------------------------------------------- |
| 0     | one flow                                         | The printed folder is the session folder. Run `agx flow bind --folder <folder>`.                           |
| 3     | no flow is bound to this branch or changed by it | Ask which work folder this branch belongs to, or none. A folder named: bind it. None: a no-flow run.       |
| 4     | several flows matched                            | Ask which one, listing the printed folders. Bind the answer.                                               |
| 5     | a small fix in progress                          | A [small-fix hand-off](#small-fix-hand-off). Run no `bind`.                                                |
| other | the helper failed                                | Report `flow unavailable: <reason>` once, read the work folder by hand for reads, and stop before a write. |

A discard named in advance binds nothing: a `bind` row on the feature branch is deleted with it. An unattended run takes
"none" at exit 3 and stops at exit 4. A no-flow run writes its rows to `<work-root>/unassigned/state.tsv` and skips
Steps 4, 5 and 9. Gate never creates a work folder.

Then read `flow status --folder <folder>`. Its `storage:` line names where the folder lives. Under `local`, nothing in
the folder is staged or committed, and Step 4 does not run.

**Slice scope.** The `slice:` line of `flow status` decides the scope, in the forms
[Slice scope](../../../execute/references/flow-commands.md#slice-scope) gives. When it reads `slice: <ids> (<branch>)`,
the run is scoped to those slices: Step 8 requires rows only for their tasks, and Step 9 judges only the criteria those
tasks trace to. Every other task and criterion reads `n/a: outside selected slice`. When the plan has `## S<n>` slice
sections and the line reads `all` or `not bound`, ask which slice the branch carries, or the whole plan, and run
`agx flow bind --folder <folder> --slice <ids>` on a slice answer.

## Step 3: Commit uncommitted work

Show `git status --porcelain`, `git diff --stat` and `git diff --cached --stat`. Sort every changed path:

- **In scope:** a path in the `**Files:**` list of an in-scope plan task, a path under the work folder, or, with no
  plan, a path the branch's commits already changed.
- **Out of scope:** every other path.

Commit the in-scope paths with no question: completing the gated work authorizes a scoped local commit. Write the
`commit user` row unless the commit is artifact-only. Ask one question about the out-of-scope paths, listing them, with
the recommendation to leave them out when nothing on the branch touches them. On "commit them", commit them in the same
way. On "leave them", they stay uncommitted, the report names them, and a removal in Step 13 that would lose them stops
for the user. The unattended default is "leave them".

## Step 4: Tidy the work folder

Read [tidy.md § Step 4: Tidy the work folder](tidy.md#step-4-tidy-the-work-folder) before doing this step.

## Step 5: Reconcile divergences

This step folds the open divergence entries into the approved artifact bodies, so the review sees the reconciled text.
It edits `intent.md`, `spec.md`, `plan.md` and `brief.md`, and never code. It is the only place a divergence reaches an
approved body; every earlier phase appends an entry to `## Divergences` and leaves the body alone.

**When it does not run.** A no-flow run, a small-fix hand-off, a discard named in advance, and a folder with no open
entry. An entry is open when it has no `Reconciled:` ending
([chain-artifacts.md](../../../decompose/references/chain-artifacts.md#the-divergences-section)). A user-authorized
divergence is written already reconciled, so this step never lists it.

**1. Read the entries.** Read `flow status --folder <folder>` and each artifact's `## Divergences` section. An entry
that the status lists as `unrecorded` has no force: report it, and apply nothing from it. For every other open entry,
work out the minimal body edit that makes the artifact match what shipped, and sort it:

| Entry                                                                                                                                                                            | What gate does                                                                                                   |
| -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| The edit is ambiguous, or contradicts another section                                                                                                                            | Report-only. Note why. Guess no edit.                                                                            |
| The edit changes reviewed scope: a success criterion, a user story, an interface, a task's acceptance, or which files a task touches (a rename of a listed file is the next row) | Report-only. Name `spec` (for `spec.md`) or `decompose` (for `plan.md`), which review and re-approve a revision. |
| Any other edit, such as a renamed file or a step done another way under the same criterion                                                                                       | Apply.                                                                                                           |

The first row wins when an entry matches two.

**2. Apply and show.** The artifacts live in `<dir>`, the directory that `flow path --folder <folder>` prints; under the
`local` storage setting Git does not hold it. Before the first edit to an artifact, copy `<dir>/<artifact>` to
`<scratch>/reconcile-<artifact>` with the file-copy tool. Make each applicable edit, append ` Reconciled: <YYYY-MM-DD>.`
to its entry, and show the diff of each edited artifact against that copy
(`git diff --no-index -- <scratch>/reconcile-<artifact> <dir>/<artifact>`). An unattended run applies nothing and
reports the entries as open, because an edit no one can approve would leave the artifact's approval stale and stop every
later phase.

**3. Ask once.** The edits are changes to approved text, and only the user approves text. After the diffs, read
`flow status --folder <folder>` and note each edited artifact's hash from its `hashes: <artifact> text=<hash>` line.
Then ask one question that offers an answer for each edited artifact: approve the reconciled text, or keep the approved
text.

- **Approve.** Write the row with that hash, which
  [the approval guard](../../../execute/references/flow-approval-and-audit.md#the-approval-guard) requires:

    ```bash
    agx flow log gate approve <artifact> ok "<why>" auto "<next>" --folder <folder> --expect-blob '<hash>'
    ```

    The `why` is `<tier>: <signals>; reconciled <N> divergences`, with the tier and signals copied from the artifact's
    latest earlier `approve` row; for `intent.md` and `brief.md` it is `reconciled <N> divergences`. When the guard
    refuses, the artifact changed after the `flow status` read: stop and report it, and never write the row with other
    evidence.

- **Keep.** Copy `<scratch>/reconcile-<artifact>` back over `<dir>/<artifact>`, which removes gate's own edit and its
  `Reconciled:` endings under either storage setting, and write
  `agx flow log gate approve <artifact> declined "<why>" none "<next>" --folder <folder>`. Its entries stay open.

**4. Commit.** Commit the edited artifacts and `state.tsv` by name, as the gated work. The commit is artifact-only.
Under the `local` storage setting nothing is committed.

**5. Report.** One line, `Reconcile: <A> applied, <U> left open`. A `spec.md` entry left open is counted again in Step
9, where the Spec lane reports it as a gap.

## Step 6: Refresh docs

The refresh is a process gate. `--no-docs`, or a plain-language skip said at any point before or during the step,
declines it. Say once what goes unverified ("the refresh will not check this diff against the docs"), write
`waive docs-refresh` with the user's reason or `none given`, and never raise it again in the run. A decline mid-step
discards the refresh's uncommitted edits and keeps a refresh commit that already landed. For each tracked file the
refresh edited, take its repository-relative name from the refresh's edit list as `refresh_path` and run
`git restore --staged --worktree -- ":(top,literal)$refresh_path"`. Remove each file the refresh created by passing its
exact path as data to the file tool.

Run `docs` maintain mode with the plan's `## Documentation impact` section as the work list, or a scan of the diff for
doc-relevant changes when there is no such section. A section that reads "none" runs nothing. A file that `docs` routes
to another owner (a runbook to `ops`, for example) is listed as not refreshed with that owner. A file that Step 3 left
uncommitted is not edited, so a decline can never discard the user's own changes; it is listed as not refreshed with the
owner `uncommitted changes`. Commit the refresh's in-scope edits with no question and ask about an out-of-scope edit, as
Step 3 does. The commit gets a `commit housekeeping` row.

The line reads `Docs: refreshed (<N> files)`, `Docs: no impact declared`, or `Docs: waived`, with
`; not refreshed: <path> (<owner>)` added for each routed file.

## Step 7: Verify tests

Read [review-and-audit.md § Step 7: Verify tests](review-and-audit.md#step-7-verify-tests) before doing this step.

## Step 8: Review and audit

Read [review-and-audit.md § Step 8: Review and audit](review-and-audit.md#step-8-review-and-audit) before doing this
step.

## Step 9: Converge against the spec

Read [review-and-audit.md § Step 9: Converge against the spec](review-and-audit.md#step-9-converge-against-the-spec)
before doing this step.

## Step 10: List deepening advice

This step runs when Step 8 produced a review result. Inspect the modules the branch touched with
[deepen](../../../deepen/SKILL.md)'s lens: shallow modules, tight coupling, and seams that make the code hard to test or
extend. Report a short list with each module or path, its evidence (a `path:line` or a concrete observation from the
diff), and an optional next action, such as "run `deepen` on `<path>`". An empty list reads
`Deepening advice: none found`.

```text
Deepening advice (does not affect the verdict):
| Module/path | Evidence | Optional next action |
| ----------- | -------- | -------------------- |
```

The advice never changes the verdict, never withholds or recommends a destination, never enters the pull request body,
and never authorizes a refactor.

## Small-fix hand-off

A branch is a small-fix hand-off when `flow where` exits 5: `<work-root>/unassigned/state.tsv` holds a
`skip chain <branch>` row for it with no later `pivot fix <branch>` row. The record is the set of rows
[The small-fix record](../../../execute/references/flow-approval-and-audit.md#the-small-fix-record) defines, read with
`<compare>` as the base. Gate reads one more row with it: a `waive test-first` row after the skip row whose evidence is
`branch=<branch>`. It accounts for no commit and stands in for the red commit below; another branch's row never counts.
A `review fix` row carries no branch, so the record can hold another branch's. Gate takes as this branch's review only a
`review fix` row it can tie to this branch, such as one whose `commit=` pair names a commit on the branch, never another
branch's row. A row that an `invalid:` line names, matched as
[Invalid rows](../../../execute/references/flow-commands.md#invalid-rows) says, can stay in the record, and the audit's
`invalid:` lines report it, but the reads below skip it: it closes nothing, names no commit, and is no red evidence.

- **Verification.** The latest `verify fix` row closes the record when its status is `ok`. Otherwise the trail check
  reports the fix as in progress.
- **Red commit.** When the `skip chain` row's `why` opens with `bug fix:`, the record needs a `verify red-fix` row, and
  the red-commit check runs on it. A `waive test-first` row in the record takes its place: the user declined the test,
  and the row's `why` names the covering test, or the check that failed before the change, so no red-commit check runs.
  A bug fix with neither row is an Important finding.
- **The description.** The pull request description carries these lines, which Option 2 passes to `gh`:

```text
Small fix: <the one sentence>
Why: <the reason from the skip chain row>
Signals: one sentence; one concern; no interface, contract, trust-boundary, or data-handling change; covered by <test>; <F> non-test files, <L> changed lines, largest <P>% of a file
Red commit: <sha> | n/a: not a bug fix | n/a: test declined; <check> failed before the change | n/a: test declined; covered by <test>
```

A small fix has no work folder and no `spec.md`, so Steps 4, 5 and 9 do not run, and its rows go to
`<work-root>/unassigned/state.tsv`.

## Step 11: Propose the next step

Read [propose-and-execute.md § Step 11: Propose the next step](propose-and-execute.md#step-11-propose-the-next-step)
before doing this step.

## Step 12: Execute the choice

Read [propose-and-execute.md § Step 12: Execute the choice](propose-and-execute.md#step-12-execute-the-choice) before
doing this step.

## Step 13: Clean up the worktree

Read [propose-and-execute.md § Step 13: Clean up the worktree](propose-and-execute.md#step-13-clean-up-the-worktree)
before doing this step.

## Step 14: Write the trail rows

Read [propose-and-execute.md § Step 14: Write the trail rows](propose-and-execute.md#step-14-write-the-trail-rows)
before doing this step.
