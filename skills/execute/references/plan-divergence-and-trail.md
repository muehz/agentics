# Plan divergence and decision trail

This file holds the rules that the `Plan divergence` and `Decision trail` sections of [`SKILL.md`](../SKILL.md) point
to.

## Plan divergence

A committed artifact that silently disagrees with the diff is a stale record, which is worse than no record. At each
wave boundary report, compare what shipped with the approved text. Three kinds of difference count:

- **Plan fields.** The packet's file scope (`**Files:**`), acceptance criteria (`**Acceptance Criteria:**`), or
  dependency edges (the packet's `**Dependencies:**` value and the plan's `## Dependency Graph`, not the
  `**Dependencies:**` line under `## Constraints`).
- **Spec.** Shipped behavior, an interface, or a success criterion that differs from `spec.md`.
- **Intent.** An outcome, constraint, or decision in `intent.md` that no longer holds.

Wording, names, and formatting are not divergences. You decide whether a difference counts. Ask the user nothing. A
user-authorized divergence is a separate kind that the user states, so nothing compares it; see
[User-authorized divergence](#user-authorized-divergence).

For the plan fields, a divergence is any of these:

- a file created or modified that `**Files:**` did not name;
- a named file the task did not need;
- a criterion whose test command, result, or condition changed;
- an edge added or removed.

These are not divergences:

- a different variable name, comment, or formatting choice;
- a criterion reworded to verify the same thing.

A fix round, a continued reviewer's correction, and any later commit of the group can widen the file scope after the
wave's own comparison ran. At each review-group boundary, the **boundary check** in `SKILL.md` compares the paths of
every task and fix commit of the group with its packet's `**Files:**`, and a path that no packet or entry names is a
divergence like any other. Append its entry and write its row before any dependent of the group is released, and name
the commit it came from.

The persisted plan path itself is never a file-scope divergence for whichever commit carries it. It is execution-owned
and not part of any task packet's `**Files:**` list.

Record a divergence: **append** one entry to the `## Divergences` section of the artifact it amends, in the directory
that `flow path --folder <plan folder>` prints. Use the entry format in
[chain-artifacts.md](../../decompose/references/chain-artifacts.md#the-divergences-section). **Never edit the body of an
approved artifact to record a divergence.** A body edit makes the approval `stale`, and a stale plan approval stops the
run. The approval hash leaves the section out, so an appended entry leaves the approval `current`. A file with no such
heading gets one at its end with its first entry. In a committed work folder, stage the artifact path again after the
write, as Step 1 sub-step 2 does. Execute commits it in the wave's artifact commit, under the run's commit
authorization, with no approval question. Nothing here pushes.

**Self-check.** Before you write to an approved artifact, run `flow status` and note whether its approval reads
`current`. After the write, run it again. When the approval read `current` before and reads `stale` after, the write
changed the body. Rebuild the file from its last staged or committed content plus the new entries only, stage it, and
run `flow status` once more. Continue the run and ask for no new approval. When the approval was already `stale` before
the write, a hand edit or an earlier body edit caused it. Rebuild nothing, because that would undo a user's edit. Append
the entry. For `plan.md`, stop the run for a new approval, as Step 0 requires. For `spec.md` and `intent.md`, continue;
the approval stays `stale`.

**Stops.** Divergences do not stop the run. Stop only for something outside the authorization: scope expansion past the
intent, a contradicted user decision, a destructive step, or an external publication.

    ## Divergences

    - **2026-08-30, execute wave 2, task-7 files** — added `src/adapters/legacy.ts`, which the packet did not name. Reason: the interface it implements lives there, not in `src/core/`.

The comparison runs while you assemble the boundary report. That report is the **Wave pauses** step that shows what
shipped. The comparison runs in every mode that emits that report: `pair` before "Ready for feedback.", and `auto` and
waived pauses in the informational report. `auto` computes earlier than it reports. The wave's entries are appended as
soon as the controller's integration verification finishes, and **before** any next-wave task is released into the
overlap. A packet released first could start on a Stage 1 verdict that the wave's own diff has already invalidated. The
report lists each entry it appended. Each entry is also one **Decision trail** row: `diverge` with `target`
`<task-id> <field>` for a plan entry, or `<artifact> <field>` for a spec or intent entry, and status `ok`. When the
entry names a commit, `evidence` is `auto` (or `auto:<sha>` for another commit). Otherwise it is the amended artifact's
path. Write the row right after you append its entry, and pass `--entry last`, so `flow` adds `entry=<hash>` of the
entry it just read. The target is one cell, so quote it as one argument:

    agx flow log execute diverge "<task-id> <field>" ok "<why>" <evidence> "execute <next step>" --folder <plan folder> --entry last

A spec or intent entry quotes its target the same way, as `"spec.md <field>"` or `"intent.md <field>"`. Unquoted, the
target's words become separate arguments, and the call prints the usage and exits 1 with no row written.

An open entry with no row that holds its hash reads `unrecorded` in `flow status`, and so does an entry whose text
changed after its row ([flow-state.md](flow-approval-and-audit.md#standing-orders-and-divergences)). A divergence found
mid-wave waits for that report. A run that never reaches a boundary has nothing verified to record. An entry other than
a `rule` entry that touches a not-yet-implemented packet sends that packet back through Stage 1 per the **Stage 1
dispatch rule**. The report names each packet it re-reviews.

### User-authorized divergence

A [user-authorized divergence](../../routing/references/user-authorized-divergence.md) is a departure from a repository
rule that the user decided on. It amends no approved text, so nothing compares it and `gate` has nothing to reconcile.
When the run has a plan, the controller appends one entry for it to the plan's `## Divergences`, already reconciled:

    - **<date>, execute wave <n>, <task-id> rule** — <rule> (<document>). User: "<their words>". Reason: <reason>. Reconciled: <YYYY-MM-DD>.

Use `run` in place of the task id when the divergence belongs to no single task. The entry follows the timing and
evidence rules above: the controller appends it at the wave boundary, and the row's evidence is `auto` when the entry
names a commit and the plan path otherwise. The entry is also one **Decision trail** row, `diverge <task-id> rule` or
`diverge run rule`, written with `--entry last`. Its target is one quoted argument, `"<task-id> rule"` or `"run rule"`:

    agx flow log execute diverge "<task-id> rule" ok "<why>" <evidence> "execute <next step>" --folder <plan folder> --entry last

`flow` records a `rule` row in this clone's ledger, and a `rule` row the ledger lacks, such as one copied from another
clone, reads `unrecorded` whether or not its entry is open. The `Reconciled:` ending keeps it out of the open
divergences that travel to workers; the register line carries it to them instead
([standing-orders.md](standing-orders.md)).

The task commit, or for an isolated packet its integration merge commit, carries the record in its body. When a
`Divergence (...)` line covers the task and its work departs from the rule, put that line in `Divergence:` form as the
second paragraph of the message file. Write the message file as data. In the shell call that reads its path into
`message_file`, also read the selected task paths as a nonempty, NUL-delimited list and turn each into one
`:(top,literal)` entry in `task_pathspecs`. Run `git commit -F "$message_file" -- "${task_pathspecs[@]}"`. A `run` scope
requires the line only on tasks that depart. For an isolated packet, the controller carries it on the integration merge
commit. Load the validated worker head SHA into `head_oid` in the same call as the message file path, then run
`git merge --no-ff -F "$message_file" -- "$head_oid"` ([isolated-packets.md](isolated-packets.md)). Read the commit's
body with `git log --format=%B "$base_oid..HEAD" --`, using the validated base id.

A `rule` entry never sends a packet back through Stage 1 on its own. When the departure also changes a task's file
scope, acceptance criteria, or edges, the comparison records that plan divergence as its own entry, as it does today,
and that entry alone triggers the re-review. The two entries are separate.

When the authorized departure also makes the work differ from `spec.md` or `intent.md`, the wave-boundary comparison
would append an ordinary open entry for it, and gate Step 5 would fold that departure into the approved text only after
the user approves the reconciled text. The controller instead appends the matching entry to that artifact's
`## Divergences`, already ending `Reconciled: <YYYY-MM-DD>.`, with a `diverge <artifact> <field>` row written with
`--entry last`. Because the entry is already reconciled, Step 5 never lists it. The `rule` entry names the user's
decision it rests on.

### The stack-base resolver

Resolve the base remote with the [base and remote resolver](../../git/references/base-and-remote.md). Write its named
remote as one line in a work-item scratch file through a file-writing tool. Pass the file's path in
`AGX_STACK_REMOTE_FILE` when running the block, using a structured environment argument or a quoted variable in the same
call. A URL, an unresolved remote, or a ref missing from local tracking state cannot supply this comparison.

```bash
(
  if [ -n "${AGX_STACK_REMOTE_FILE:-}" ] &&
    remote=$(cat -- "$AGX_STACK_REMOTE_FILE" 2>/dev/null) && [ -n "$remote" ] &&
    base=$(GH_PROMPT_DISABLED=1 gh pr view --json baseRefName --jq .baseRefName 2>/dev/null) && [ -n "$base" ] &&
    git check-ref-format "refs/remotes/$remote/$base" >/dev/null 2>&1 &&
    oid=$(git rev-parse --verify --end-of-options "refs/remotes/$remote/$base^{commit}" 2>/dev/null); then
    printf '%s\n' "$oid"
  else
    printf '%s\n' 'not computed' >&2
    exit 1
  fi
)
```

On success, carry the full object id as `base_oid`. A failure leaves standard output empty and stops the filter.

## Decision trail

Every run, in every mode, records its decisions in the `state.tsv` of the work folder that holds the plan. It writes
them with bare `agx flow log`, as [flow-state.md](flow-commands.md#commands) shows. The plugin root, resolved by the
[plugin-root order](../../routing/references/chain-phases.md#resolve-the-plugin-root), is needed only for that section's
launcher-path fallback. The rows follow the contract in [flow-state.md](flow-state.md). That reference is the canonical
state format for the whole suite. Other skills route their rows there. They do not invent a format of their own. Only
the controller writes rows. Implementer and reviewer subagents never touch `state.tsv`. A `fix` run has no work folder.
It writes `<work-root>/unassigned/state.tsv` through `--folder <work-root>/unassigned`, as **The `fix` entry** says.

**Reading and binding.** `<plan folder>` is the folder that holds the plan. Pass `--folder <plan folder>` on every
`agx flow status` and `agx flow log` call of a run. Without it, the call finds the flow only through a `bind` row for
the current branch. On the default branch, that is often none, and the call exits 3. A `agx flow log` call also exits 3
until a `state.tsv` exists. So a run that starts the flow passes the folder from its first row. A run starts the flow
when it materialized the plan's work folder, and when it accepted a repository plan whose folder has no `state.tsv` yet.
A plan that is not yet in the repository has no folder to pass before Step 1 persists it. Step 0 treats such a plan as a
plan that has no flow. `agx flow where` takes no `--folder` and names the flow the current branch finds. Exit 3 means no
flow exists. Exit 4 means more than one matched, so ask the user which. Exit 5 means a small fix is in progress on this
branch. After this skill reads a flow it did not create, run `agx flow bind --folder <plan folder>`. It appends the
branch's `bind` row once. A second call writes nothing. `agx flow bind` refuses the default branch and a detached
`HEAD`, so skip it there.

**The run's first row.** Once the plan is persisted (Step 1 sub-step 2), read `mode` (`auto`, `pair`, or `arena`),
`reason`, and the logical plan `folder` from scratch files or structured tool input. In that same shell call, capture
`branch` with `git branch --show-current` and stop if it fails or prints nothing. Run
`agx flow log execute enter run ok "$mode chosen: $reason" "mode=$mode branch=$branch" "execute run the plan" --folder "$folder"`.
This row records the mode; gate's controller audit runs the same way in every mode. A lane run writes its own
`enter run` row (see **Running one lane** in [parallel-lanes.md](parallel-lanes.md)). A fix has no plan to persist, so
its `enter run` row comes right after its `skip chain` row (**The `fix` entry**).

Write one row, at the moment it happens, for each of these, with `phase` `execute`. In the first column, the first word
is the `event` and the rest is the `target`:

| Event                                                                                                                                                                                                                                                                                                                                                 | `event` and `target`                                        | `status`         |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------- | ---------------- |
| a task commit's verdict, written when the group reviewer renders it (one row per implementer commit, evidence `auto:<sha>`); a commit that fails the controller's own verification gets its `failed` row at once, and a fix commit that answers an integration-review finding gets its row from the continued aggregate reviewer's verdict            | `verify <task-id>`                                          | `ok` or `failed` |
| a bug-fix task's red commit, its test confirmed failing                                                                                                                                                                                                                                                                                               | `verify red-<task-id>`                                      | `ok`             |
| a small fix's red commit, its test confirmed failing                                                                                                                                                                                                                                                                                                  | `verify red-fix`                                            | `ok`             |
| an integration review that returned CLEAR, after any fix rounds on the continued reviewer (FINDINGS is a `failed` `review` row, not a verify row; a group's recorded skip writes `verify group-<name>` with its `waive aggregate-review` row, and `verify final` is written when the final review returns CLEAR or with its `waive final-review` row) | `verify group-<name>` or `verify final`                     | `ok`             |
| the controller's wave integration verification                                                                                                                                                                                                                                                                                                        | `verify wave-<N>`                                           | `ok` or `failed` |
| a reviewer returned                                                                                                                                                                                                                                                                                                                                   | `review <task-id, group-name, or final>`                    | `ok` or `failed` |
| a Critical or Important defect that passed a review and was found later                                                                                                                                                                                                                                                                               | `escape <task-id, group-name, or final>`                    | `failed`         |
| work discarded or reverted                                                                                                                                                                                                                                                                                                                            | `revert <task-id>`                                          | `ok`             |
| a divergence entry, written with `--entry last` right after the entry is appended                                                                                                                                                                                                                                                                     | `diverge <task-id> <field>` or `diverge <artifact> <field>` | `ok`             |
| a user-authorized divergence, written with `--entry last` right after the entry is appended                                                                                                                                                                                                                                                           | `diverge <task-id> rule` or `diverge run rule`              | `ok`             |
| a standing order appended to the register (evidence `auto`)                                                                                                                                                                                                                                                                                           | `order <n>`                                                 | `ok`             |
| a commit on the branch that no task, red, or artifact-only row covers, such as a follow-up the user directed or a closing documentation commit, written when it is made (evidence `auto:<sha>`)                                                                                                                                                       | `commit <fix, triage, user, or housekeeping>`               | `ok`             |
| a task dropped from the run                                                                                                                                                                                                                                                                                                                           | `drop <task-id>`                                            | `cancelled`      |
| a fact the workflow used to write into approved text                                                                                                                                                                                                                                                                                                  | `note <slices, slice-S<n>, lane-S<n>, coverage, or policy>` | `ok` or `failed` |
| a group split into one-task waves                                                                                                                                                                                                                                                                                                                     | `demote wave-<N>`                                           | `ok`             |
| a blocker raised                                                                                                                                                                                                                                                                                                                                      | `block <task-id>`                                           | `blocked`        |
| a blocker cleared                                                                                                                                                                                                                                                                                                                                     | `unblock <task-id>`                                         | `ok`             |
| a change of approach                                                                                                                                                                                                                                                                                                                                  | `pivot <task-id or wave-N>`                                 | `ok`             |
| `aggregate review skipped: <evidence>` or a user override                                                                                                                                                                                                                                                                                             | `waive <what>`                                              | `ok`             |
| a small fix starting                                                                                                                                                                                                                                                                                                                                  | `skip chain <branch>`                                       | `ok`             |
| a small fix's Stage 2 verdict                                                                                                                                                                                                                                                                                                                         | `verify fix`                                                | `ok` or `failed` |
| a small fix stopped and routed into the chain                                                                                                                                                                                                                                                                                                         | `pivot fix <branch>`                                        | `ok`             |
| a turn that ends with the run unfinished                                                                                                                                                                                                                                                                                                              | `pause run`                                                 | `pending`        |

The `waive <what>` target names a head from [Operands](flow-state.md#operands).

`why` is one plain-language line that says what drove the decision, e.g.
`subagent edited the wrong helper; Stage 2 caught it`. `evidence` is `auto` for these rows, as the **Commit evidence**
form says:

- a task's `verify` row;
- a fix's `verify` row;
- a `revert` row;
- a `diverge` row that names a commit;
- an `order` row, for which `flow` stores `entry=<hash of the standing order>`.

A `commit` row takes `auto:<sha>`, never a bare `auto`. A `drop` row's evidence is `none` or a pointer, and a `note`
row's depends on its target, as the `note` table in [flow-state.md](flow-state.md#events) lists. A `verify wave-<N>` row
records the controller's wave integration verification and carries no aggregate verdict. A `verify wave-<N>`,
`verify group-<name>`, or `verify final` row writes `commit=<integrated sha>` by hand, with no `auto` and no patch-id. A
`review` row writes `kind=<plan|packet|quality|integration> dispatched=<ts> round=<n> blocked=<seconds>`, and an
`escape` row writes `found-by=<gate|ci|copilot|person> severity=<critical|important> review=<kind>`, both as
[flow-state.md](flow-evidence.md#evidence-forms) defines them. An isolated packet's `verify` row writes
`base=<sha> head=<sha> integrated=<sha> patch-id=<id>` itself. Other rows use the pointer
[flow-state.md](flow-evidence.md#evidence-forms) lists, or `none`. `next` opens with `execute` while tasks remain, and
with `gate` on the row that closes the last wave. On a fix, `next` opens as follows:

- The `skip chain` and `enter run` rows open it with `execute`.
- A `verify fix` row with status `ok` opens it with `gate`.
- The `pivot fix <branch>` row opens it with `intent`.

In a committed work folder, stage `state.tsv` with its own `git add` after each row, once the artifact the row describes
is staged. Commit it in the wave's or the run's artifact commit, which touches only flow artifacts. A work folder in the
local store is never staged. Reviewer dispatch timing is logged in `review` rows. The trail also carries the end-of-run
self-audit against the transcript. When this run wrote 8 or more rows, the trail also carries a fresh-eyes trail audit.
That audit's named auditor opens the report's **Attention** section.
