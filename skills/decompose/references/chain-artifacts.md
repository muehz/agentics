# Chain artifacts: the work folder and the provenance header

The canonical definition of where chain artifacts live and what they carry. `intent`, `spec`, `decompose`, `execute`,
and `gate` cite this file and never restate it.

## The work folder

Each work item gets one folder. It sits inside a monthly folder under the plan parent, which the **Resolve the plan
parent** step in `skills/decompose/SKILL.md` resolves:

    <plan-parent>/YYYY-MM/<purpose>-<topic>/
    ├── intent.md       # required at the hand-off when a present plan.md or spec.md names it
    ├── brief.md        # the middle path's brief, in place of intent.md
    ├── plan.md         # required at the hand-off
    ├── spec.md         # optional
    ├── state.tsv       # optional; the flow record, rules below
    └── temp/           # optional, ignored by Git
        └── **/*        # unrestricted contents

The work root is `docs/specs`. If `AGENTS.md` or `CLAUDE.md` names another directory for plans or work folders, do these
steps:

- Use that directory instead.
- Prefix every `flow` call with `FLOW_WORK_ROOT=<directory>`. The directory is relative to the repository root.

A shell call keeps no variables from the one before, so put the prefix on the same command line as `agx flow …`, as
[flow-state.md](../../execute/references/flow-commands.md#commands) shows. Create the root when it does not exist. Never
create a work root in `.claude/`, `.codex/`, or `.github/copilot/`. They hold harness configuration. This file calls the
root `<plan-parent>` or `<work-root>`.

`YYYY-MM` is the month in which the work item's first artifact was written. A legacy source outside the work folder is
not one of the item's artifacts. A legacy source that is read in place never becomes one. A copy made by decompose's
**Legacy source placement** counts as written on the day it is copied. A later artifact of the same work item joins the
folder its source lives in, so a month rollover never splits a work item.

`<purpose>` is one of `feature`, `refactor`, `upgrade`, `infrastructure`, `architecture`, `data`, `process`, `design`.
The rule under **What the folder holds** decides which files must be present, at the pull-request hand-off.

Every writer derives `<purpose>-<topic>` by the **Plan artifact identity** rule in
`skills/execute/references/plan-artifact-identity.md`. On a branch other than the repository's default branch, the rule
seeds `<topic>` from the branch name with everything through its last `/` removed. As a result, a work item's folder
carries the name a person chose for its branch. On any other branch, each writer applies the rule's slug step to its own
content seed.

- `decompose` seeds from the source card's `**Change:**` line when the source has a card, else from the plan's own
  `**Goal:**` line.
- `execute`, which materializes only a plan with no repository source, seeds from the plan's `**Goal:**` line.
- `spec` seeds from the feature's name as the user stated it.
- `intent` seeds from the problem as the user stated it: the first clause of the invocation, before any "so that".

`<purpose>` is the content seed's leading purpose word, else `feature`, whichever seed named the topic. `<topic>` is the
seed after these steps, in this order:

1. Lowercase the seed.
2. Collapse every run outside `a-z` and `0-9` to one hyphen.
3. Trim leading and trailing hyphens.
4. If the seed is longer than 40 characters, cut it at the last hyphen at or before its 40th character.

`<topic>` is the rule's `<slug>`, so `decompose` and `execute` never disagree about a folder name. Nothing else reaches
the folder name: no path, filename, punctuation, date, or numeric collision suffix. A folder that `intent.md` or
`spec.md` already created stays the work item's home. No writer renames an existing folder when the rule changes.

When `execute` materializes a plan that arrived with no repository file, it first searches
`<plan-parent>/*/<purpose>-<slug>/plan.md` across months. If it finds a body identical below the header, it reuses that
folder. Otherwise it writes to the current month. When that file already exists with a different body, `execute` stops
before dispatch and reports both paths. A folder that `intent.md` or `spec.md` already created is the plan's home, not a
collision.

`state.tsv` is the one log in a work folder. This file grants it once, not per work item, because emission is automatic.
The standard workflow never depends on a file the user had to ask for.
[flow-state.md](../../execute/references/flow-state.md) defines it, and skills write it through `flow`. It is a log, not
a chain artifact, and carries no provenance header. The header schema applies to `spec.md` and `plan.md` alone.
`decompose` reads it and `gate` re-checks it. Approving this file approves no other root file. Any other `*.tsv` file
directly in a work folder is history from before `state.tsv`. Leave it in place. Never rewrite it.

A writer that resolves no work folder writes to `<plan-parent>/unassigned/state.tsv`. The writer passes
`--folder <plan-parent>/unassigned`. In practice, such a writer is `gate` on the default branch or `execute fix` on a
small fix. That file is never a flow. The log follows the work root: a directory named in `AGENTS.md` or `CLAUDE.md`
moves the monthly folders and the log with them.

**Storage.** One setting decides where a new work folder lives. It has two values:

- `committed` keeps the folder under the work root, in the repository's commits.
- `local` keeps the folder, with its planning files and `state.tsv`, in the clone's Git directory. Every worktree of the
  clone shares it, and nothing stages it. The `unassigned` log moves there too.

In both modes, `--folder` and every row name the logical folder, `<plan-parent>/YYYY-MM/<purpose>-<topic>` or
`<plan-parent>/unassigned`. [flow-state.md](../../execute/references/flow-state.md#storage) defines how the setting
resolves, where each store keeps the folder, and how `flow path` finds its files. The setting moves the files and never
turns the log off: every writer writes its rows in either mode, because a log a consumer can silently turn off is a log
no reviewer can rely on.

Each artifact has exactly one owning skill: `intent` owns `intent.md`, `spec` owns `spec.md`, and `decompose` owns
`plan.md`. `intent` also writes the middle path's `brief.md`, which takes the place of `intent.md`. Skill frontmatter
declares `metadata.artifact` as a single filename. Artifact-less skills omit the field.

Readers (`decompose`, `execute`, `spec`, and `gh` linkage) accept a plan at a legacy path when the user names it.

## What the folder holds

The folder's root holds `intent.md`, `brief.md`, `plan.md`, `spec.md`, `state.tsv`, and the `temp/` directory, and
nothing else. Names are exact and lowercase. A `*.tsv` history file from before `state.tsv` is the one exception. It
stays in place.

**The required pair, at the hand-off.** `intent.md` and `plan.md` are the required pair when `/agentics:gate branch`
hands a branch off to a pull request. Before that moment, a folder holds only the artifacts that its chain has produced
so far. A folder with only `intent.md`, or with `intent.md` and `spec.md`, is a valid mid-chain folder. So is the pull
request that a headless `intent` writer opens with `intent.md` alone (see **Write, stage, commit**).

`plan.md` is always required at the hand-off. `intent.md` is required only when a present `plan.md` or `spec.md` names
`[intent.md]` in its `derived-from` field. A source named there but absent from the folder is genuinely missing. A
folder whose `plan.md` and `spec.md` name no `[intent.md]` is complete without one. That covers these cases:

- Concrete inline requirements, a plan that `execute` materialized, and a waived intent. Their plans carry
  `derived-from: none`.
- A spec-first entry. Its spec carries `none`.

No header field records a waiver. `intent` records it in the session report. At the hand-off, gate searches elsewhere
for a required file (a wrong-case name, `temp/`, a legacy plan path). Before it calls the file missing, gate moves a
file that it finds into place. Only a genuinely missing required file stops the gate. Nothing fabricates a placeholder
for it.

**The small-fix exception.** A small fix that `execute fix` ran has no work folder and needs neither file at the
hand-off. `/agentics:gate branch` accepts that hand-off only when `<work-root>/unassigned/state.tsv` holds a
`skip chain <branch>` row with no later `pivot fix <branch>` row. It then checks the fix's rows, its red commit for a
bug fix, and the pull request description in place of a folder.

**Optional files.** `spec.md` is present when `spec` ran. `state.tsv` is present once a writer has written a row. Its
rules are above.

**`temp/`.** The folder's scratch and evidence directory: optional, ignored by Git, with unrestricted contents,
including nested folders. It has no content rules. No skill or check inspects what it holds. One exception: at the
hand-off, gate may recover a misplaced `intent.md` or `plan.md` from it. Put scratch and evidence in the ignored
`temp/`, which needs no approval. Nothing there is committed, and the standard workflow never depends on a file in it.
`execute`'s lane launch packets live in `temp/lanes/`: each one copies committed plan content, so a lost packet is
written again from the plan. What a reviewer needs goes into the canonical artifact's existing sections
(`## Plan challenge`, `## Provenance`, `## Divergences`) or the pull request instead.

**The ignore rule.** `temp/` must be ignored by Git. Set the shell tool's working directory to the work folder and run
`git check-ignore -v --no-index -- temp/agentics-ignore-probe`. The probe path can be absent. The command names the
covering rule for tracked and untracked paths. When no rule covers it, the default rule in the repository root
`.gitignore` is `/<plan-parent>/**/temp/`. For the default `docs/specs` parent, that rule is `/docs/specs/**/temp/`. One
rule covers every month and work folder. An ignore rule never untracks a path already in the index. A tracked or
force-added path under `temp/` must therefore leave the index while its local contents stay.

**Extras.** Any other root entry, file or directory, tracked or untracked, is an extra. No skill writes one, and a skill
default or agent preference never creates one. A leftover legacy `slices.md` in an active folder is an extra too. A
legacy `design.md` is not an extra while a reader still accepts it: `spec draft` and `decompose` read one as input, and
an existing artifact's `derived-from` may name it (see **Legacy `design.md`** below). At the hand-off, gate does these
things:

- It moves the session folder's extras into `temp/` with their nested structure.
- It keeps both copies under distinct names when a name collides.
- It rewrites a `derived-from` that names a `design.md` that is now in `temp/` to `none`.

Cleanup never deletes content. It never touches a folder outside the session's work.

**Legacy `design.md`.** A `design.md` from before increment C is a legacy source. `spec draft` and `decompose` still
read one as input, so a `design.md` in a work folder is a source, not an extra. Readers still accept `[design.md]` in an
existing artifact's `derived-from`. It carries no provenance header and gets none. No writer creates, copies, or names a
`design.md` in a work folder. A writer reads a legacy design in place. It writes its own artifact with
`derived-from: none`. It records the design's original path in that artifact's context.

## The provenance header

Every `spec.md` and `plan.md` opens with YAML front matter. The front matter is the one field below between two `---`
lines, in the same shape as a `SKILL.md` frontmatter block. A blank line and the Markdown body follow. A reader that
needs "the body below the header" splits at the second `---`. The schema:

```yaml
derived-from: [intent.md] | [spec.md] | [design.md] | none
```

`derived-from` is relative because git versions the folder as a unit. No sha pin is needed. A named source must be a
regular sibling file in the same folder.

A new `intent.md` carries no header. It opens with its `# Intent: <topic>` title line. An existing intent's
`derived-from: none` header is accepted and ignored, and a reader takes a file that does not open with `---` whole.

`spec.md` uses `none` when no input artifact exists, or derives from `[intent.md]`. A plan names exactly one source:

- `[spec.md]` for any work that started at intent.
- `[intent.md]` only for an in-flight five-section intent.
- `none` for concrete inline requirements recorded in its body, a waived intent, a plan `execute` materializes, or a
  legacy design read in place.

`[design.md]` is a legacy-only value. Readers accept it on an existing artifact, and no writer sets it. A list with two
entries, a reference to a missing file, or a malformed reference is an invalid header. `decompose` returns an artifact
with an invalid header to its owner, and `gate` reports it.

An artifact's approval is an `approve` row in `state.tsv`. The row is bound to the artifact's content by its `text=`
hash ([flow-state.md](../../execute/references/flow-approval-and-audit.md#approval-and-verification)), and it is written
only through the [approval guard](../../execute/references/flow-approval-and-audit.md#the-approval-guard). The
`approve spec.md` row is what `skills/decompose/SKILL.md` Step 0 reads for an approved spec. An `approve plan.md` row
records the go-ahead to execute the plan. `decompose` writes it when the user answers yes to the execution question.
That yes also confirms the plan's standing orders, so right after it `decompose` writes one `order <n>` row with
evidence `auto` for each line of the plan's `## Standing orders` section ([save-and-stage.md](save-and-stage.md) gives
the call). `execute` writes the `approve plan.md` row the same way when it materializes a plan that arrived with no
repository file. `execute` also writes it the same way when a repository plan has no current `approve` row at invocation
and the user invoked execute on it. `execute` never writes an `approve plan.md` row for a plan with no `### Task` entry.
It refuses such a plan and routes to `decompose`. An intent's approval is the user's yes to the one approval question
`intent` asks. The yes is recorded as an `approve intent.md` row, like the others. No spec work starts without it. The
middle path's `brief.md` is a chain artifact too. It is hashed and approved like an intent: the user's yes to its one
approval question is an `approve brief.md` row. A rewrite of an approved artifact makes its approval read `stale`, and
the user approves again. An entry appended to its `## Divergences` section is not a rewrite.

Nothing the workflow requires after an approval edits the approved text. Each such fact is a `note` row in `state.tsv`,
never an edit to approved text:

- a slice's issue link, `note slice-S<n>`
- a lane's merge, `note lane-S<n>`
- a coverage re-check result, `note coverage`
- a policy version stamp, `note policy`

[flow-state.md](../../execute/references/flow-state.md#events) gives each row's evidence.

Each artifact's `challenge` row records review of that artifact's own scope, independently. Intent reviews direction,
spec reviews the solution, and plan reviews the written task graph.

**Execution rule.** A `challenge` row with status `ok` means the challenge skill ran to completion for this artifact and
this scope in an identifiable session. Its findings were resolved or explicitly accepted. None of these ever upgrades
the row:

- A read of the challenge skill.
- A mention of the challenge skill.
- Acceptance of a recommendation list.
- Approval of scope.
- The author's private risk assessment.

Real answers from an earlier round, or from an earlier session on the same artifact scope, carry forward. Never
fabricate execution to skip the skill. Status `declined` records an explicit decline of this artifact's challenge.
Status `pending` means the pass did not run, is pending, or was interrupted. Do not copy a prior artifact's `ok` or
`declined` result into a downstream artifact's record. Preserve settled decisions from the source. Challenge only
questions that the current artifact introduces.

**Record rule.** Each artifact with a challenge pass carries one entry with the fixed label `**Challenge record:**`.
Intent places it in `## Decisions`, spec in `## Scope Decisions`, and plan keeps its `## Plan challenge` section with
the same fields. The entry names five fields, in this order:

1. The target artifact and the scope examined.
2. The execution reference, in one of three forms:
    - The session date, the rounds asked and answered, and the question numbers.
    - `prior record dated YYYY-MM-DD`, for a pass carried forward on the same scope.
    - `declined YYYY-MM-DD, no round asked`, for a decline before any round was asked.
3. Each resolution with its answer and source.
4. Remaining questions with their disposition, or "none".
5. Concerns weighed and set aside, when the summary listed any.

In an intent and a spec, write the five fields as a numbered list nested under the `**Challenge record:**` label, in
this order. The plan's record already uses a numbered list.

**A recommendation taken when the user ended the round is not an answer.** When the user ends a round after answering
part of it, `challenge` gives each unanswered question of that round its recommended answer with the source
`recommendation taken when the user ended the round`. That source is not the user's answer. `challenge`'s summary lists
these questions under **Resolved decisions**. A question whose recommended answer would publish, post, or halt the work
stays open. Unasked questions are the open frontier. Both are already remaining questions in field (4). A recommendation
the user accepted, by name or by accepting every recommendation in the round, is an answer, with the source
`user accepted the recommendation`. A question that took its recommendation when the round ended never goes in field
(3). It goes in field (4), with its recommendation and the source `recommendation taken when the user ended the round`.
Field (3) is otherwise unchanged: resolutions from the user, a lookup, a dispatcher, or the source artifact stay there
with their sources. The owning skill's own rules settle that question's disposition and the challenge row's status, as
[`challenge`'s caller rule](../../challenge/SKILL.md#called-by-another-skill) says.

The execution reference is never a transcript or session identifier. The entry is present in these cases:

- Status `ok`.
- Status `declined`. The entry states what was declined and what went unexamined.
- A `pending` row after at least one round. The entry states the round asked and the unanswered questions.

Only a pass that was never offered has no entry. That is a `pending` row where the owning skill never invoked
`challenge` at all. Examples are an unattended run and a scan that came back Clear. A `challenge` row alone never proves
a review, resolves an open blocker, or authorizes implementation. An `ok` row with no entry is a defect.

**The execution reference has two sources.**

The challenge summary's **Scope examined** line supplies the target, the scope, the rounds asked and answered, and the
question numbers. The owning skill supplies the session date. A copy of the summary line alone therefore never completes
field (2). The writer adds the date to it.

**A decline before any round was asked has no summary**, because `challenge` writes one only for an interview that ran.
That entry cites no summary and invents no `Scope examined` line. Its field (2) is the fixed form
`declined YYYY-MM-DD, no round asked`. Field (1) names the artifact and what went unexamined in place of a scope the
interview never covered. Fields (3) and (5) read "none". A `declined` row still carries an entry here, because the pass
was offered and refused. The alternative is that a writer invents rounds that were never asked.

**A carried-forward pass has no current summary either.**

When a prior review on the same artifact scope stays valid, this session ran no interview, so no **Scope examined** line
exists to copy. Field (2) is the fixed form `prior record dated YYYY-MM-DD`. The date is that of the review carried
forward. Field (1) names the artifact and the same scope that prior review covered. Fields (3), (4), and (5) carry the
prior record's resolutions, remaining questions, and set-aside concerns forward unchanged. A resolution keeps its
original source. Never invent a round number, a question number, or this session's date for a pass that did not run
here. If the scope has moved, the prior record does not carry forward. Run the review on what changed.

Conformance never reads an artifact header and pins no sentence of this schema. This file is the definition. Conformance
is never evidence that a challenge ran. The record entry is that evidence.

## The Divergences section

`intent.md`, `spec.md`, `plan.md`, and `brief.md` may each carry a level-two heading `## Divergences`. It holds the
differences that a later phase found between the approved text and the work. `execute` appends to it at wave boundaries.
`gate` reconciles it. A file gets the heading, at its end, with its first entry. A plan keeps the heading where the plan
template puts it.

An entry is one list item:

    - **<date>, <phase and wave or step>, <target>** — <what differs from the approved text>. Reason: <why>.

`<target>` is the task and field for a plan (`task-7 files`), and the section or criterion for a spec or an intent. A
reconciled entry ends with `Reconciled: <YYYY-MM-DD>.` An entry with no `Reconciled:` ending is open. The open count is
the number of open entries. Each entry also has one `diverge` row in `state.tsv`.

**Never edit the body of an approved artifact to record a divergence.** A body edit makes the approval `stale`, and a
stale plan approval stops the run. The approval hash leaves the section out, so an appended entry leaves the approval
`current` ([flow-state.md](../../execute/references/flow-approval-and-audit.md#approval-and-verification)). Open entries
are amendments to the approved text. `execute` passes them to every worker. `gate` folds an entry into the body only
after the user approves the reconciled text in
[Step 5](../../gate/references/branch/branch-workflow.md#step-5-reconcile-divergences), and the user's yes writes the
new approval. An entry the user keeps stays open.

## Code in artifacts

Preparation artifacts (`intent.md`, `spec.md`, and `plan.md`) decide what to build and how to check it. `execute`,
applying `tdd`, writes the code and the tests. This section is the single definition of which code blocks an artifact
may carry.

The rule covers blocks of code, meaning text in a programming, query, or configuration language. A diagram (a `mermaid`
fence), a directory tree, a table, or a `text` fence that illustrates structure is not code. The rule does not cover it.
The paste test below is how to decide.

A code block, fenced or indented, is exactly one of these four kinds:

1. **Pseudocode.** A fenced block whose info string is `pseudocode`. It states logic that prose would blur, in no real
   language.
2. **Quote.** Existing code copied verbatim, with its `path:lines` citation in the sentence directly above the block.
3. **Command.** A shell command to run and its expected output.
4. **Contract.** Notation for an interface the spec defines: a signature, a schema, a state table, or an example
   payload. Allowed in a spec's `## Solution` only. An intent and a plan carry none. A plan that needs a contract cites
   the spec section, or quotes the existing definition under kind 2. An intent holds only the quote and command kinds
   (kinds 2 and 3).

Any other block is banned. The paste test: a block that a person could paste into a source file, and that would
implement or test the change when it runs, is implementation code. Function bodies and test code are banned. A signature
with no body is a contract. A literal value the code must contain (a config key, an error string) goes in a sentence and
not in a block.

The version gate applies to specs and plans, which normally carry a `**Written by:**` line. An intent carries none and
has no reviewer, so its rule is the skill sentence alone. A spec or plan whose line names an earlier version, or that
has no such line, is exempt from the new checks. `spec align` on an older spec keeps the code already in it. An update
to such a plan keeps the code already in its existing tasks. The update's new or changed tasks follow the rule, and the
update writes no `**Written by:**` line into the plan and does not change an existing one to claim the new version.

`intent`, `spec`, and `decompose` cite this section rather than restate it, except that a reviewer prompt run verbatim
by a subagent carries the allowed kinds by their markers, because that subagent cannot read this file.

## intent.md

`intent.md` is the capture artifact that `intent` owns. It records a problem as the user stated it, not a solution. That
lets `spec draft` define the solution and not ask again what was already said. It has six sections, in this order, plus
an optional `## Divergences` section at its end. Each of the six is present even when it reads "none stated":

1. `## Problem`: what is wrong or wanted, from the user's perspective, in the user's words where possible.
2. `## Outcome`: what success looks like, as a number, a comparison, or a test. "Faster" is not an outcome until it
   names one.
3. `## Affected users and systems`: who uses or depends on what this work changes, and the systems, interfaces, and data
   it touches.
    - `spec` reads this section to set the initial tier. See the shared definition of
      [data handling](../../routing/references/contracts/vocabulary.md#data-handling). `decompose` takes that tier as
      given and checks only the plan-level signals.
4. `## Constraints`: deadlines, systems that must not change, budgets, compatibility.
5. `## Decisions`: choices the user stated during capture.
    - `spec draft` treats each as a sourced requirement. `decompose` treats each as settled. Neither re-asks them.
6. `## Open questions`: everything capture could not settle, including every claim the user asserted as already handled
   but could not point to.
    - A non-interactive run lists here what an interactive one would have asked.

**The title and the card.** A new intent opens with a `# Intent: <topic>` title line. Directly under it, before
`## Problem`, an unheaded bullet list of five lines carries the labels `Change`, `Why now`, `Out of scope`, `Risk`, and
`Source`, with `Source` last. A new intent has no `Ask` line: it asks for approval in chat, not in the file. The card
adds no section, so the six sections and their order are unchanged. An intent written before the card rule, or whose
card has no `Source` line, stays valid, and a skill that rewrites it adds no card and no `Source` line. A card that
already holds an `Ask` line keeps it, because removing it would make an approval stale.

Open questions distinguish direction blockers, explicitly deferred questions with a resolver, and downstream solution or
implementation questions with their owner. Decisions also records the assistant-selected next owner and reason, clearly
distinguished from user decisions.

Once the direction is resolved, the `approve intent.md` row's `next` reads `spec`, which matches the Decisions line. A
new intent never routes to `decompose`. Every intent hands to `spec`. A blocked direction writes no `approve intent.md`
row. A headless writer fills the six sections from its input alone. It takes the pull-request route below.

**In-flight intents.** An intent with five sections, and no `## Affected users and systems`, was written before the
six-section rule. It keeps the route it recorded, so a concrete one still selects `decompose`. It needs no
`approve intent.md` row. An intent stays in flight only until `intent` next writes the file. That write converts it to
six sections, and its `approve intent.md` row's `next` reads `spec`. The `## Affected users and systems` section is the
marker of an intent written under the current rules.

## Slices inside plan.md

A sliced work item still has one `plan.md`. There is never a per-slice plan or folder. Unsliced work keeps the ordinary
`## Tasks` section. Sliced work replaces `## Tasks` with one `## S1`, `## S2`, ... section per slice. ID-only headings
keep issue anchors (`#s1`, `#s2`) stable when titles change. Each active section opens with:

- **Title:** a short descriptive name.
- **Outcome:** the independently verifiable result.
- **Scope:** what belongs in this increment and its boundary with siblings.
    - Cite the source artifact's numbered sections or the inline `## Requirements` section instead of a copy of its
      acceptance criteria.
- **Dependencies:** sibling slice IDs, named external decisions, or `none`.
- **Lane:** optional; `yes` only on a slice that qualifies as a lane (below). Absent otherwise.
- **Issue:** optional GitHub issue link, only when one was created or verified before the plan's approval. A link made
  after the approval is a `note slice-S<n>` row, not an edit to this field.

The section then holds its `### Task N` entries. Task IDs stay globally unique across the plan, and cross-slice
dependencies are task edges.

**Lanes.** A lane is a slice that a separate session can build on its own branch while its siblings are built elsewhere.
A slice qualifies when all three hold:

- Its **Dependencies** are `none` or name only slices already merged into the branch the plan targets (the pull request
  base).
- No task under it depends on a task in an unmerged slice.
- No file in its tasks' **Files** lists appears in another open lane. An open lane is a lane not yet merged.

When two qualifying slices share a file, the lower-numbered slice keeps the lane. `decompose` applies that tie-break
first. Then it writes `**Lane:** yes` on the remaining slices only when two or more remain. One lane gives no parallel
work. Every plan update re-marks the lanes and removes the field from a slice that no longer qualifies. A limited
one-slice update may re-mark the `**Lane:**` field on sibling slices. That is the only sibling change it makes. A lane
adds no approval and changes no dependency. `execute` writes one launch packet per lane in the work folder's
`temp/lanes/`.

A retired slice keeps its `## S<n>` heading with `Retired: <reason>` and no tasks, so its reservation and old links
survive. Never reuse an ID. Active dependencies never target a missing, retired, or cyclic ID. Issue state does not
change the repository's scope or dependencies.

Record concrete inline requirements once, in a `## Requirements` section placed directly after `## Provenance`. Slices
cite that section rather than repeat it.

An explicit request for only one slice limits the plan update to that slice. Scope lists the selected slice IDs. The
update preserves sibling sections (apart from their `**Lane:**` field), the `verify` rows that record their tasks'
completion, the `diverge` rows, and the Divergences history. Do not create a second plan or overwrite sibling work.

A slicing-only request writes the slice sections with no tasks. The plan gets no `approve plan.md` row.
`**Skeleton path:**` reads `none — slicing only`. Provenance records `**Result:** not run — slicing only, no tasks`.
`## Task index` reads `none: slicing only`. `execute` refuses such a plan and routes to `decompose`.

GitHub issue projection stays explicit and optional. An issue links `plan.md#s<n>`. A slice authorizes no
implementation, publication, or merge.

## Reconciling a legacy `slices.md`

When `decompose` reads a folder that holds a legacy `slices.md` beside `plan.md`, it does these steps:

- It folds each slice section into `plan.md` as defined above.
- It preserves IDs, retired entries with reasons, issue links, and task dependencies.
- It sets the plan's `derived-from` to the source alone.
- It removes the legacy file with `git rm`.

External `#s<n>` links to the legacy file resolve through history. Update issue bodies only on explicit request. A
leftover `slices.md` that no `decompose` run folded is an extra, and gate moves it to `temp/` at the hand-off.

`decompose` reports other extras in a legacy folder. It never silently deletes them and never treats them as
user-authorized. The user decides each one: relocate it or retire it, or leave it, and gate moves it to `temp/`.

## Migrating a dated layout

For a repository that still holds dated `YYYY-MM-DD-*` work folders or flat dated files:

1. For each legacy work item, derive the month from the date prefix. Strip the date from the name to get the work-item
   folder.
2. `git mv` each `plan.md` that carries a valid provenance header to `<parent>/YYYY-MM/<purpose>-<topic>/plan.md`.
3. `git rm` legacy files without a header and every extra file.
    - Their content stays reachable at `https://github.com/<owner>/<repo>/blob/<sha>/<old-path>`, where `<sha>` is a
      commit that still holds them.
    - This deletion is an explicit history operation that the user requested. Gate's cleanup never deletes. It moves
      extras to `temp/` instead.
4. Relocate an extra file that a live document depends on to a real home. Update the reference.
5. Record the old-path to new-path map in the migration commit or pull request description.
    - Where the repository keeps a rename record, record the map there too.
6. Leave migrated plan bodies unchanged. Old paths inside their step commands are a historical record.

Migration invents no provenance. A legacy file without a header gets none.

## Write, stage, commit

Always write. Always stage. Commit only with the artifact's approval. A work folder under `local` storage is the
exception: it is written and never staged or committed. In a session, the skill that wrote the artifact commits it
together with its `approve` row, `state.tsv` included, on the feature branch, under the
[commit contract](../../git/references/commit.md#the-commit-contract). An artifact with no approval stays staged and
uncommitted. No terminal probe decides whether a session commits, because a host's shell tool reports redirected stdio
whether or not a person is answering. A headless writer with no session at all (a scheduled drafting run) does these
steps:

- It commits on a working branch of its own.
- It pushes that branch.
- It opens a pull request for a person to approve.

That writer never commits to or pushes the default branch.

The shapes of a new intent, spec, and plan are in [skim-rules.md](skim-rules.md).

## Harness surfaces are projections

The file is canonical. Skills fill these surfaces **from** the file, after they write the file:

- Copilot CLI's session plan file and its `todos` table.
- Claude Code's `TodoWrite`.
- Codex's `update_plan`. Feature-detect it. `skills/routing/references/session-storage.md` marks it unverified.

`skills/routing/references/native-plan-projection.md` defines the load, the approval mechanism, and the status values.

Skills never fill them instead of the file. A projection can be regenerated. A commit trail cannot be reconstructed.
