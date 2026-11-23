# Flow state: evidence forms and skill rows

Split from [flow-state.md](flow-state.md). Defines `state.tsv` evidence forms, `verify` evidence validation, the rows
each skill writes, and pre-v2 migration notes.

## Evidence forms

| Row                                                                                                      | `evidence`                                                                                                                                                      |
| -------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `approve <artifact>` with status `ok`                                                                    | `text=<hash>`, written by `auto` with `--expect-blob`; a row written before version 2 holds `blob=<hash>`                                                       |
| `approve <artifact>` with status `declined`                                                              | `none`                                                                                                                                                          |
| `verify <task>`, `verify red-<task>`, `verify red-fix`, `verify fix` for a commit                        | `commit=<sha> patch-id=<id>`; `commit=<sha>` alone for a merge commit                                                                                           |
| `revert`, `diverge` for a commit                                                                         | `commit=<sha> patch-id=<id>`; `commit=<sha>` alone for a merge commit; `flow` checks no form for these two                                                      |
| `verify red-<task>`, `verify red-fix` for a failing run with no commit                                   | `no-commit exit=<n> cmd=<command> out=<excerpt>`                                                                                                                |
| `verify <task>`, `verify fix` with status `failed`, for a failure with no commit to name                 | `cause=<word>` alone, the failure form                                                                                                                          |
| `verify fix` with status `failed`, for a Stage 2 review that rejected the staged diff (no commit exists) | `branch=<branch> patch-id=<id of the rejected diff>`                                                                                                            |
| `verify <task>` from an isolated packet                                                                  | `base=<sha> head=<sha> integrated=<sha> patch-id=<id>`                                                                                                          |
| `verify wave-<n>`                                                                                        | `commit=<integrated sha>`, optionally with `patch-id=<id>`                                                                                                      |
| `verify group-<name>`, `verify final`                                                                    | `commit=<integrated sha>`, optionally with `patch-id=<id>`, written by hand                                                                                     |
| `diverge` with `--entry`                                                                                 | the row's evidence, then `entry=<hash of the divergence entry>`                                                                                                 |
| `order <n>`                                                                                              | `entry=<hash of the standing order>`, written by `auto`                                                                                                         |
| `commit <kind>`                                                                                          | `commit=<sha> patch-id=<id>`, written by `auto:<sha>`                                                                                                           |
| `drop <task>`                                                                                            | `none`, or a pointer                                                                                                                                            |
| `note <target>`                                                                                          | by target, in the `note` table under [Events](flow-state.md#events)                                                                                             |
| `challenge` with status `ok` and nothing to challenge                                                    | `scan=clear`                                                                                                                                                    |
| `review`                                                                                                 | `kind=<plan\|packet\|quality\|integration\|spec> dispatched=<ts> round=<n> blocked=<seconds>`, then optional pairs                                              |
| `escape`                                                                                                 | `found-by=<gate\|ci\|copilot\|person> severity=<critical\|important> review=<plan\|packet\|quality\|integration>`, then optional `word=<the finder's own word>` |
| `enter`                                                                                                  | `mode=<mode> branch=<branch>`                                                                                                                                   |
| `bind`                                                                                                   | `none`, or `slice=<ids>` when `flow bind --slice <ids>` writes it                                                                                               |
| `skip chain <branch>`, `pivot fix <branch>`                                                              | `base=<merge-base sha>`                                                                                                                                         |
| `waive test-first` in the unassigned fix record                                                          | `branch=<branch>`, as on `enter`, so the row is attributed to its branch                                                                                        |
| `branch`                                                                                                 | the PR number; `slice=S<n>` names the slice the verdict covers                                                                                                  |
| `release`, `change`                                                                                      | the tag or the change-record path                                                                                                                               |

The optional pairs of a `review` row are `rev=<n>`, the revision of the artifact the run reviewed;
`by=<independent|self>`; and `commit=<sha>`, the commit the review covered. The four other pairs stay required. A
`review` row's `commit=` names what the review covered, so the
[commit check](flow-approval-and-audit.md#the-commit-check) skips it.

In the no-commit form, `<n>` is a non-zero exit status. The command is the text between `cmd=` and the first ` out=`,
and the excerpt is the rest, which may be empty. A no-commit cell longer than 500 characters is refused with exit 2. The
commit check skips this form.

On a branch bound with a slice, the helper appends the branch's `slice=` pair to the evidence of every `branch` row and
of every row that carries `commit=`. See [Slice scope](flow-commands.md#slice-scope).

Pass `auto` as the evidence. `flow` computes it as:

- `text=<hash>` on an `approve` row, which requires `--expect-blob`; see
  [the guard](flow-approval-and-audit.md#the-approval-guard)
- `entry=<hash>` on an `order <n>` row, the hash of entry `<n>` of `## Standing orders` in `plan.md`
- `commit=<HEAD> patch-id=<id>` on a `verify`, `revert`, or `diverge` row

`<id>` is the first field of `git show <sha> | git patch-id --stable`. `auto:<sha>` names a commit other than `HEAD`. A
`commit` row takes only `auto:<sha>`, never a bare `auto`. `auto` names a commit with these limits:

- A merge commit has no single diff, so the row stores `commit=<sha>` alone and is checked by ancestry.
- A commit that is not a merge and changes nothing has no patch-id, so the row is refused with exit 2. No row is written
  with an empty `patch-id=`.
- A bare `auto` on a `verify`, `revert`, or `diverge` row is refused with exit 2 when `HEAD` changes only the flow's
  work folder, because the row would name a commit that holds no work. The message says
  `auto names HEAD, which changes only the work folder; pass auto:<sha> for the task commit`.
- Any other non-merge commit whose patch-id Git cannot compute is refused with exit 2. The message says
  `git computed no patch-id for commit <sha>`. A red, fix, or task row for a non-merge commit without its patch-id fits
  no [`verify` form](#verify-evidence), so the helper never stores one.

`entry=` is written only by the helper. A typed `entry=` pair is refused with exit 2, so a row cannot record an entry
the helper did not read.

A `review` row's own `ts` is the return time, so latency is `ts` minus `dispatched`. Its `status` is `ok` when the
reviewer cleared the unit and `failed` when it reported findings. An `escape` row has status `failed` and copies the
`next` of the row before it, so it never changes the phase a reader selects. It may be appended after a `branch` row.
`triage` writes `escape` rows with phase `gate` and no `bind` row. Skills append nothing after a `done` row; a row
appended anyway reopens the flow ([Closing a flow](flow-commands.md#closing-a-flow)).

### Verify evidence

A `verify` target falls in one class, the first that matches:

| Class      | Targets                              | Evidence that fits                                                                               |
| ---------- | ------------------------------------ | ------------------------------------------------------------------------------------------------ |
| integrated | `wave-<id>`, `group-<name>`, `final` | `commit=<sha>`, optionally with `patch-id=<id>`                                                  |
| red        | `red-<task>`, `red-fix`              | the commit form, or the no-commit form                                                           |
| fix        | `fix`                                | the commit form; with status `failed`, also the failure form, or `branch=<branch> patch-id=<id>` |
| task       | any other target                     | the commit form, or the isolated form; with status `failed`, also the failure form               |

- The **commit form** is `commit=<sha> patch-id=<id>`, or `commit=<sha>` alone when that commit is a merge commit.
- The **isolated form** is `base=<sha> head=<sha> integrated=<sha> patch-id=<id>`.
- The **failure form** is `cause=<word>` alone. It records a failure with no commit to name, such as `cause=repair`,
  `cause=context`, or `cause=red` ([two-strike-stop.md](two-strike-stop.md)).
- `<sha>` is a commit id of 7 to 64 lowercase hex characters. `<id>` is a patch-id of exactly 40 or exactly 64 lowercase
  hex characters. `<word>` is one or more of `a-z`, `0-9`, and `-`. `<branch>` is any value without a space.
- Each key appears at most once. Besides a form's own keys, a row may carry `cause=<word>`, and `slice=<ids>`, which the
  helper appends on a bound slice. No other key fits. A token that is not a `key=value` pair, and an empty cell, fit no
  form.

So a file path, `none`, prose such as `tests pass`, `uncommitted: tests pass in the worktree`, and a bare SHA each fit
no form. On a red, fix, or task row, `commit=<sha>` alone fits none when the commit is not a merge; on an integrated row
it fits.

`flow log` checks a hand-typed `verify` cell against these forms, then against the repository:

- each `commit=`, `integrated=`, `base=`, and `head=` value must name a commit (`<value>^{commit}` resolves)
- in the commit form, and on an integrated row that carries one, `patch-id=` must equal the patch-id that `auto:<sha>`
  would store for the commit
- a merge commit takes no `patch-id=`, because a merge has none
- the isolated form's `patch-id=` is checked for shape only, because it describes the packet diff, which no single
  commit holds

So a working-tree snapshot hash written as `commit=<hash> patch-id=<id>` has the right shape and is still refused,
because it names no commit. A cell that fails writes no row, prints this, and exits 2:

```text
flow: invalid evidence: '<cell>' -- <reason>
```

`<reason>` names what the cell breaks: a token that is not a pair, a key that repeats or belongs to no form of the
class, a value of the wrong shape, an empty cell, a value that does not resolve, a patch-id that does not match, or a
merge commit with a patch-id or a non-merge commit without one. For a set of keys that fits no form, it reads
`<class> <status> evidence is one of: <forms>`.

`auto` and `auto:<sha>` are resolved into the commit form before the row is stored. A cell that holds the literal word
`auto` in `state.tsv`, written there by hand, fits no form.

## Rows each skill writes

| Skill                            | Rows                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| -------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `intent`                         | `create intent.md`; `challenge intent.md`; `approve intent.md` on the user's yes (evidence `auto`); on the middle path `create brief.md` and `approve brief.md` in their place; `next` reads `spec`, or `execute` after `approve brief.md`; `waive "diagnose <branch>"` for a user's change after a declined diagnose step; `waive chain`, `waive direction-blockers`, and `waive brief-questions` for a declined process gate                                                                                                                                                                        |
| `tdd`                            | none of its own; `waive test-first`, `waive delete-untested`, and `waive refactor` are recorded by the delivery skill                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `spec`                           | `create spec.md`; `challenge spec.md`; `approve spec.md` on the user's yes; `pause spec.md` when a routine hand-off leaves the approval to `decompose`; `revoke spec.md` when `spec align` changes it                                                                                                                                                                                                                                                                                                                                                                                                 |
| `decompose`                      | `create plan.md`; `challenge plan.md`; `approve spec.md` and `approve plan.md` (one routine-tier answer may write both); `order <n>` for each standing order right after `approve plan.md`; `review` for the plan review (target `plan`)                                                                                                                                                                                                                                                                                                                                                              |
| `execute`                        | `enter run` (mode); `approve plan.md` when the invocation approves; `order <n>` at intake for an unrecorded standing order the user confirms, and for each order appended mid-run; `diverge`; `commit`; `drop`; `verify`; `revert`; `demote`; `block`; `unblock`; `pivot`; `waive`; `review` (one per packet, group quality, integration, and final reviewer return; a `quality` row for a small fix); `escape`; `pause`; for a small fix `skip chain <branch>`, `verify red-fix`, `verify fix`, `pivot fix <branch>`; `waive "diagnose <branch>"` for a user's change after a declined diagnose step |
| `gate`                           | `branch`, `release`, or `change`; `escape` for a defect it finds that a review passed; `approve` for an artifact it reconciled, only on the user's yes in Step 5 and with `--expect-blob`; `commit housekeeping` for a change it authored; `commit user` for an unaccounted commit the user accepts; `skip` and `na` per checklist item; `done` when its destination ends the work                                                                                                                                                                                                                    |
| every flow skill except `triage` | `bind` (through `flow bind`) after resolving a flow it did not create                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `triage`                         | `escape` only, with phase `gate` and no `bind` row, for a Critical or Important finding it fixes on the pull request of a work item that has a flow                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `routing`                        | `done` for a local-store flow whose work is complete, which closes it                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `recall`                         | none                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |

An `approve spec.md` or `approve plan.md` row's `why` names the tier and the signals that fired, as
`<tier>: <signals>; <basis>`. An `approve intent.md` row names no tier.

## Files written before version 2

- An existing `state.tsv` keeps its `# flow-state 1` line and is never rewritten. New rows are appended to it. A new
  file has no version line and starts with the header row. A helper older than this format skips a file's first two
  lines unread, so it drops the first row of a file created without the line.
- A `blob=` approval is checked by the first rule for as long as it exists. When it goes stale it is approved again, and
  the new row carries `text=`.
- After the upgrade the ledger is empty, so every existing approval whose hash matches reads `unconfirmed here`. The
  phase that needs it presents the artifact, asks once, and writes a new row on yes.
- A standing order or an open divergence written before the upgrade reads `unrecorded` until its row is written again
  with `auto` or `--entry`.
- A flow that relied on being the only unbound flow gets exit 3 until `flow bind` binds it.
- A lane branch bound before the upgrade has no slice. Run `flow bind --slice S<n>` on it before its `branch` verdict,
  or the verdict covers the whole plan.

No `verify` row is read as legacy. The patch-id requirement shipped in the commit that created `state.tsv`, so no row of
this format predates it, and the cutover for the [`verify` forms](#verify-evidence) is the start of the format. A bare
SHA is invalid wherever it appears, and `commit=<sha>` alone on a red, fix, or task row is invalid when the commit is
not a merge. A committed row that fits no form, `verify`, `waive`, or `demote`, stays as written and reads as an
[`invalid:` line](flow-commands.md#invalid-rows).
