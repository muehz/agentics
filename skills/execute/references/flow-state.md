# Flow state: the `state.tsv` record

One append-only file per work item records where the work is, what is approved, and what comes next. Skills write it
through `flow`. A person, a maintainer, or an agent in a new session with no context reads it with `flow status`.

## Where it lives

A work folder is `<work-root>/YYYY-MM/<purpose>-<topic>/`. It holds the item's `intent.md`, `spec.md`, `plan.md` or
`brief.md`, and `state.tsv`. `<work-root>` is `docs/specs`. The [storage setting](#storage) decides whether the folder
is committed under the work root or kept in the clone's Git directory.

When `AGENTS.md` or `CLAUDE.md` names another directory, prefix every `flow` call with `FLOW_WORK_ROOT=<directory>` in
the same command. A shell call keeps no variables from the one before. The directory is relative to the repository root.
Use these forms:

- bash: `FLOW_WORK_ROOT=<directory> agx flow …`
- PowerShell: `$env:FLOW_WORK_ROOT = '<directory>'` before the call

Rows that belong to no flow (a small fix, a gate verdict on the default branch) go to
`<work-root>/unassigned/state.tsv`. To write them there, pass `--folder <work-root>/unassigned`. That file is never a
flow.

The helper keeps two files of its own in `<git-common-dir>/agentics/flow/`. `<git-common-dir>` is the directory that
`git rev-parse --git-common-dir` prints, which every worktree of the clone shares. Git never commits or lists it.

- `ledger.tsv` records the approvals, standing orders, rule divergences, `commit` rows and finding waivers written in
  this clone. See [The ledger](flow-approval-and-audit.md#the-ledger).
- `closed.tsv` remembers closed flows so that lookup does not read them again. See
  [Closed flows and lookup cost](flow-commands.md#closed-flows-and-lookup-cost).

## Storage

One setting decides where a new work folder lives:

- `committed` keeps it under the work root, in the repository's commits.
- `local` keeps it at `<git-common-dir>/agentics/work/<work-root>/YYYY-MM/<name>/`, with its planning files and
  `state.tsv`. Every worktree of the clone shares it, and nothing stages it. Under `local`, small-fix rows go to
  `<git-common-dir>/agentics/work/<work-root>/unassigned/state.tsv`.

The helper takes the first of these values:

1. The Git config value `agentics.flowStorage`, which holds the user's own choice.
2. The environment variable `FLOW_STORAGE`, in which a skill passes the setting named in the repository's instruction
   file.
3. `committed`.

Any value other than `committed` or `local` stops every command with exit 2 before it does any work:

```text
flow: storage setting "<value>" from Git config agentics.flowStorage is invalid; use committed or local
```

The message names `FLOW_STORAGE` when the value came from the environment. Discovery yields a proposal, never a value:
the user's yes becomes the Git config value.

**Logical folder.** A relative `--folder` names the logical folder, `<work-root>/YYYY-MM/<name>` or
`<work-root>/unassigned`, in both modes. Every row and every message names the logical folder too. A relative folder
resolves against the repository root, which is what `flow where` prints, and never against the current directory.

- An existing flow is found in whichever store holds it. The setting decides only where a new flow is created.
- When both stores hold the same monthly folder, the call exits 4 and names both directories.
- `unassigned` is the exception: a write goes to the store the setting names, and a read uses the rows of both files.
- An absolute `--folder`, or a relative one outside these two shapes, is used as given. The setting does not apply to
  it.
- A `--folder` inside the local store is refused with exit 2, and the message names the logical folder to pass instead.

**Finding the files.** `flow path [--folder <dir>]` prints the absolute directory that holds the work folder's files,
and creates nothing. For a flow in the committed store it is the folder itself, and for a flow in the local store it is
the folder's directory there, whatever the setting. For a folder not created yet, it prints where `flow log` would
create it. Use that directory only to read and write the folder's files. Pass the logical folder to every `flow` call,
and never run Git inside the local store.

## Layout

A new file starts with the header row, which names the columns, and carries no version line: each row is read by its own
content, so nothing depends on a version. A file written before this format starts with `# flow-state 1` and then the
header row. It is read the same way and takes every new row, and the helper never rewrites that line. Every later line
is one row of nine tab-separated cells. A reader skips empty lines, lines that start with `#`, header rows and lines
short of nine cells.

| Column     | Content                                                                                                         |
| ---------- | --------------------------------------------------------------------------------------------------------------- |
| `ts`       | ISO 8601 UTC, stamped by `flow`                                                                                 |
| `session`  | `FLOW_SESSION`, else `CLAUDE_CODE_SESSION_ID`, else `-`                                                         |
| `phase`    | `intent`, `spec`, `decompose`, `execute`, `gate`                                                                |
| `event`    | one of the events below                                                                                         |
| `target`   | the artifact, task, wave, branch, or item the event is about                                                    |
| `status`   | `ok`, `failed`, `blocked`, `pending`, `declined`, `cancelled`                                                   |
| `why`      | one plain line, the reason, held to the commit-message standard; a spec or plan approval's `why` names its tier |
| `evidence` | one or more `key=value` pointers, a PR number, a path, a tag, or `none`; never a paragraph                      |
| `next`     | required; the first word is a phase name or `done`, then one line that says what to do                          |

The `-` in `session` is a placeholder, not a formula, so it is the one cell value that starts with `-` and carries no
guard quote. `state-tsv-shape` accepts exactly `-` in that column and nowhere else.

## Events

| Event       | Used for                                                                                                                                                                     |
| ----------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `create`    | an artifact's first write, e.g. `create intent.md`                                                                                                                           |
| `challenge` | a challenge result for an artifact: `ok`, `declined`, or `pending`; `ok` with evidence `scan=clear` records a scan that found nothing to challenge                           |
| `approve`   | the user's answer to an approval question: `ok` approves the artifact's content, `declined` records a no                                                                     |
| `revoke`    | an approval withdrawn: `spec align` changed an approved spec, or the user withdrew it                                                                                        |
| `diverge`   | a divergence entry, e.g. `diverge task-2 files`, `diverge spec.md success-criteria`, `diverge intent.md constraints`, or a user-authorized divergence, `diverge task-2 rule` |
| `verify`    | a task's, red commit's, fix's, wave's, group's, or final verification, e.g. `verify task-3`, `verify red-fix`, `verify group-<name>`, `verify final`                         |
| `revert`    | work discarded, e.g. `revert task-3`                                                                                                                                         |
| `demote`    | a group split into one-task waves; `target` is `wave-<id>`, e.g. `demote wave-2`                                                                                             |
| `block`     | a blocker raised, e.g. `block task-4`                                                                                                                                        |
| `unblock`   | a blocker cleared                                                                                                                                                            |
| `pivot`     | a change of approach, e.g. `pivot wave-2`; `pivot fix <branch>` ends a small fix                                                                                             |
| `waive`     | a skipped gate or a user override; `target` is a head from [Operands](#operands) and its arguments, e.g. `waive aggregate-review`                                            |
| `branch`    | a `gate branch` verdict; `target` is the branch                                                                                                                              |
| `release`   | a `gate release` verdict; `target` is the tag                                                                                                                                |
| `change`    | a `gate change` verdict; `target` is the change id                                                                                                                           |
| `skip`      | a gate checklist item skipped, or a small fix that leaves the chain, e.g. `skip chain <branch>`                                                                              |
| `na`        | a gate checklist item that does not apply                                                                                                                                    |
| `enter`     | a run's start; `target` is `run` and `evidence` is `mode=<auto, pair, or arena> branch=<branch>`                                                                             |
| `bind`      | a branch bound to this flow; `target` is the branch                                                                                                                          |
| `review`    | a reviewer returned; `target` is `plan`, a task id, a group name, `final`, or `fix` for an `execute fix` review                                                              |
| `escape`    | a Critical or Important defect that passed a review and was found later; `target` is the task, group, or `final`                                                             |
| `pause`     | a turn that ends with flow work unfinished; `next` says where to pick up                                                                                                     |
| `done`      | the work is complete: gate's destination was a local merge or a discard, or `routing` closed a local-store flow whose work is complete                                       |
| `order`     | a standing order recorded; `target` is its position `<n>` in `## Standing orders` of `plan.md`; status `ok`                                                                  |
| `commit`    | a commit that is not a task, red, or artifact-only commit; `target` is `fix`, `triage`, `user`, or `housekeeping`; status `ok`                                               |
| `drop`      | a task dropped from the run; `target` is the task id; status `cancelled`                                                                                                     |
| `note`      | a fact the workflow used to write into approved text; `target` is `slices`, `slice-S<n>`, `lane-S<n>`, `coverage`, or `policy`; status `ok` or `failed`                      |

The `review` row also accepts the kind `spec` and three optional pairs, described under
[Evidence forms](flow-evidence.md#evidence-forms). The `escape` row accepts an optional fourth pair, `word=`. Its
`severity` stays `critical` or `important`.

`verify <task>` with status `pending` records a task commit that awaits its group verdict. A task is complete when its
latest valid `verify` row is `ok`; see [Invalid rows](flow-commands.md#invalid-rows).

A `note` row's evidence depends on its target:

| `note` target | Records                                    | Evidence                         |
| ------------- | ------------------------------------------ | -------------------------------- |
| `slices`      | the plan's slice list and its final review | `slices=S1,S2,… final=<yes\|no>` |
| `slice-S<n>`  | a slice's issue link                       | `issue=<#N or owner/repo#N>`     |
| `lane-S<n>`   | a lane's merge                             | `merged=<sha>`                   |
| `coverage`    | a coverage re-check result                 | `none`, or a pointer             |
| `policy`      | a policy version stamp                     | `skill=<name> version=<version>` |

The skills' review and retry budgets in the table below are counted from these rows. The helper accepts the rows and
counts nothing; the skills count.

| Counter                                     | Rows it reads                                                                                   |
| ------------------------------------------- | ----------------------------------------------------------------------------------------------- |
| Document review runs per revision           | `review` rows of kind `plan` or `spec` with `rev=` and `by=`                                    |
| Stage 1 re-reviews of a packet              | `review <task>` rows of kind `packet`; the stop is `block <task>`                               |
| Failures against a task                     | `verify <task>` rows with status `failed`; the evidence may add `cause=<word>` to name the kind |
| A replaced packet, which resets the counter | `pivot <task>`                                                                                  |
| The user's grant of one more fresh dispatch | `unblock <task>`                                                                                |
| Fix attempts per defect                     | `verify <task>` rows with status `failed` and `cause=fix-attempt`                               |
| The arena re-run                            | `pivot arena`                                                                                   |
| Changes that gate authored                  | `commit housekeeping`                                                                           |

## Evidence forms

Moved to [flow-evidence.md § Evidence forms](flow-evidence.md#evidence-forms).

## Operands

A `waive` target is a head, then its arguments, separated by single spaces. The head is one of this closed list:

| Head                                                                              | Arguments                           | Emitted in                                                                                  |
| --------------------------------------------------------------------------------- | ----------------------------------- | ------------------------------------------------------------------------------------------- |
| `checklist`, `proportionality`, `coverage`                                        | none                                | `skills/decompose/SKILL.md`                                                                 |
| `no-code`                                                                         | 0 or more task ids                  | `skills/decompose/SKILL.md`                                                                 |
| `controller-rule`                                                                 | none                                | `skills/execute/SKILL.md`                                                                   |
| `independence`, `quality-review`                                                  | 1 group                             | `skills/execute/SKILL.md`                                                                   |
| `verification`                                                                    | 1 or more task ids                  | `skills/execute/SKILL.md`                                                                   |
| `deepen`                                                                          | 1 shape                             | `skills/execute/SKILL.md`                                                                   |
| `aggregate-review`                                                                | 0 or 1 group                        | `skills/execute/references/aggregate-wave-review.md`                                        |
| `final-review`                                                                    | none                                | `skills/execute/references/aggregate-wave-review.md`                                        |
| `union-rule`                                                                      | none                                | `skills/execute/references/parallel-lanes.md`                                               |
| `stage-1`                                                                         | none                                | `skills/execute/references/stage-1-dispatch-rule.md`                                        |
| `verification-preflight`                                                          | none                                | `skills/execute/references/verification-preflight.md`                                       |
| `wave-pauses`                                                                     | none                                | `skills/execute/references/wave-pauses.md`                                                  |
| `diagnose`                                                                        | 1 branch                            | `skills/execute/references/fix-entry.md`; also `skills/intent/SKILL.md`                     |
| `test-first`, `delete-untested`, `refactor`                                       | none                                | `skills/tdd/SKILL.md` (recorded by the delivery skill)                                      |
| `root-cause`                                                                      | none                                | `skills/diagnose/SKILL.md` (recorded by the delivery skill)                                 |
| `prototype-code`                                                                  | none                                | `skills/prototype/SKILL.md` (recorded by the delivery skill)                                |
| `chain`, `direction-blockers`                                                     | none                                | `skills/intent/SKILL.md`                                                                    |
| `brief-questions`                                                                 | none                                | `skills/intent/references/brief.md`                                                         |
| `direction-review`                                                                | none                                | `skills/spec/references/draft.md`                                                           |
| `spec-step`                                                                       | 1 step name                         | `skills/spec/references/draft.md`                                                           |
| `docs-refresh`, `review`, `converge`, `tests`, `work-folder`, `security-depth`    | none                                | `skills/gate/references/emission.md`                                                        |
| `finding`                                                                         | 2: lane, locator                    | `skills/gate/references/emission.md`                                                        |
| `artifact`, `migrations`, `flags`, `rollback`, `ci`, `on-call`, `comms`, `window` | none; the head may carry `:<check>` | `skills/gate/references/emission.md`                                                        |
| `rule`                                                                            | 1 rule name                         | `skills/routing/references/user-authorized-divergence.md`; also any gate outside this table |

- An argument is a run of characters other than a space. Arguments are checked for count only, never against `plan.md`,
  so a `waive` row stays valid when the plan it names changes. Two spaces in a row make an empty argument, which is
  refused.
- A group names a wave or a group, as in `waive aggregate-review wave-2` or `waive quality-review g1`. A branch is the
  current branch name, which may contain `/`, as in `waive diagnose chore/issue-1141`. A shape, a step name, and a rule
  name are each written as one hyphenated word by convention, as in `waive spec-step seams` or `waive rule docs-first`;
  the helper checks only the count.
- Only the eight release items, `artifact`, `migrations`, `flags`, `rollback`, `ci`, `on-call`, `comms`, and `window`,
  may carry `:<check>`. `<check>` is one or more of `a-z`, `0-9`, and `-`, as in `waive ci:required-checks`.
- A `demote` target is `wave-<id>`, where `<id>` is one or more of `A-Z`, `a-z`, `0-9`, and `-`.

The helper holds the same list, and a test fails when the two differ. `flow log` refuses a `waive` or `demote` row whose
target fits no operand, writes no row, prints this, and exits 2:

```text
flow: invalid target: '<target>' -- <reason>
```

The reasons are:

- `head <h> is not in the Operands list of flow-state.md`
- `<h> takes <n> arguments, got <m>`
- `an argument is empty`
- `:<check> is allowed only on a release item`
- `check is empty`
- `check <c> is not a-z 0-9 -`
- for a `demote` row, the message ends `-- demote names wave-<id>`

A `waive` target of `aggregate review wave-5` or `wave-3-aggregate` names no listed head, a `waive finding` target with
a lane and no locator has too few arguments, and a `demote` target of `2` names no wave. Each is refused.

An Agentics gate uses its listed head. Any other rule a user lets you skip uses `rule <name>`. A `waive` row that an
[`invalid:` line](flow-commands.md#invalid-rows) names waives nothing, so the gate it named is asked again.

## What a reader sees

A reader treats the file as a set: identical rows count once. Rows are ordered by `ts`, with file order as the
tie-break. The current state is the row with the greatest `ts`. File order carries no meaning. A rebase replay that
repeats a row, or a union merge that interleaves two branches, changes nothing a reader sees. A writer only appends.
Nothing rewrites or sorts the file, and nothing rewrites the version line of a file that has one.

## When to write a row

Write a row at one of these checkpoints:

- a phase's start
- an artifact's creation
- a challenge result
- an approval, a declined approval, or a revocation
- a standing order or a divergence
- a task, wave, group, or final verification
- a reviewer returning (a `review` row)
- a defect found after a review cleared it (an `escape` row)
- a commit that no task, red, or artifact row covers (a `commit` row)
- a dropped task
- a fact the workflow used to write into approved text (a `note` row)
- a block or unblock
- a change of approach
- a waiver
- a mode
- a bind
- a gate verdict
- a small fix's start

A turn that leaves flow work unfinished ends with a `pause` row. A correction is a new row. Never edit a row. Write
`create` only for an artifact's first write. In a committed work folder, stage `state.tsv` with its own `git add`, after
you confirm that the artifact it describes is in the index. A row never says `ok` for a file nobody staged. A work
folder in the local store is never staged.

## Approval and verification

Moved to [flow-approval-and-audit.md § Approval and verification](flow-approval-and-audit.md#approval-and-verification).

## The ledger

Moved to [flow-approval-and-audit.md § The ledger](flow-approval-and-audit.md#the-ledger).

## Commit accounting

Moved to [flow-approval-and-audit.md § Commit accounting](flow-approval-and-audit.md#commit-accounting).

## Auditing a branch

Moved to [flow-approval-and-audit.md § Auditing a branch](flow-approval-and-audit.md#auditing-a-branch).

## Slice scope

Moved to [flow-commands.md § Slice scope](flow-commands.md#slice-scope).

## Commands

Moved to [flow-commands.md § Commands](flow-commands.md#commands).

## Finding the flow

Moved to [flow-commands.md § Finding the flow](flow-commands.md#finding-the-flow).

## Reading the state

Moved to [flow-commands.md § Reading the state](flow-commands.md#reading-the-state).

## Selecting the phase

The phase to run is the first word of the current row's `next`. These take precedence over it:

- `flow where` exit 5 resumes the small fix. Continue with `execute fix`, or hand to `gate` when its `verify fix` row is
  `ok`.
- A `mismatch:` line selects `recall`.
- An approval the next phase needs that reads `stale` or `unconfirmed here` stops the run for a new approval.
    - The intent approval is included: `spec` does not start while `approvals:` lacks a current `intent.md`.
- Exit 3 gets the response its caller states under [Commands](flow-commands.md#commands).

A `next` whose first word is `gate` is offered, not run, because completion is an explicit request. Selection never
creates approval. An `approve` row with status `ok` exists only because the user said yes. Gate writes one for an
artifact it reconciled only on the user's yes in
[Step 5](../../gate/references/branch/branch-workflow.md#step-5-reconcile-divergences), and never carries an earlier
approval forward. Like every `approve` row with evidence `auto`, it passes
[the approval guard](flow-approval-and-audit.md#the-approval-guard): the call needs `--expect-blob` with the `text=`
hash that the `hashes:` line prints for the artifact after the edit. A declined approval writes an `approve` row with
status `declined` and evidence `none`, which approves nothing.

## Rows each skill writes

Moved to [flow-evidence.md § Rows each skill writes](flow-evidence.md#rows-each-skill-writes).

## Files written before version 2

Moved to [flow-evidence.md § Files written before version 2](flow-evidence.md#files-written-before-version-2).
