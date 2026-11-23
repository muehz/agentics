# Native plan projection

How `decompose` loads a plan into the host's own plan and task tools and asks for approval through them, and how
`execute` keeps those task records current. The repository `plan.md` and the `state.tsv` rows stay canonical
([chain-artifacts.md](../../decompose/references/chain-artifacts.md#harness-surfaces-are-projections)). A host store is
written from them. Agentics never reads it back as truth. The one exception is the Copilot fleet contract in
[surface-dispatch.md](surface-dispatch.md).

Feature-detect each tool at run time. Do not choose a tool by product name.

## Task records

A single plan has one record per `### Task` entry.

| Field        | Value                                                                              |
| ------------ | ---------------------------------------------------------------------------------- |
| Key          | the task's `**ID:**` value, for example `task-1` or `auth-middleware`              |
| Title        | `Task <N>: <task title>`                                                           |
| Text         | the marker, then the task's Dependencies, Files, Acceptance Criteria, and Steps    |
| Plan ID      | the plan's repository path, or `session` for a session-only plan                   |
| Dependencies | the IDs in the task's `**Dependencies:**` field                                    |
| First status | `pending`, or `done` (`completed`) when plan state already shows the task complete |

Every record carries a marker. The plan ID is the plan's canonical repository path, for example
`docs/specs/2026-10/feature-x/plan.md`. For an explicit session-only plan that has no repository path, the plan ID is
the word `session`. On Copilot, the marker is the first description line `agentics-plan-task <plan-id>`. On other hosts,
the marker is a `[<task-id>]` prefix on the item's text. A reload finds its own records by the marker. A marked record
of a different plan is not this plan's record: a reload keeps its status only for the same plan. A skill's own process
rows carry no marker, and a reload never changes or deletes them. Process rows are decompose's Steps 0 to 6 and
checklist rows, and execute's Steps 0 to 2 and the Independence checklist.

A multi-plan run loads nothing. It reports the projection as skipped for the set and never merges task IDs across plans.

## Load

The load is an upsert by key. It also removes marked records whose key is no longer in the plan. It never resets the
status of a record of the same plan. Running it twice, or after a crash between two records, leaves the same end state.
A new record starts as `pending`. When `execute` seeds a record for a task that plan state already shows complete, such
as on a resumed run in a new session, the record starts as `done` (`completed`). Plan state decides this. The store
never does.

`decompose` runs the load whenever it has written the canonical `plan.md` for a single plan, including when it withholds
the approval question. Only the question depends on the plan's readiness.

| Host                | Tool                                                       | How to load                                                                      |
| ------------------- | ---------------------------------------------------------- | -------------------------------------------------------------------------------- |
| Copilot CLI and app | the session `sql` tool: the `todos` and `todo_deps` tables | The SQL in the next block, kept because the projection reads `todos.status` back |
| Claude Code         | `TodoWrite`, or `TaskCreate` and `TaskUpdate` when present | Send the whole list: the process rows unchanged, plus one marked item per task   |
| Codex               | `update_plan`, when the session exposes it                 | Same as Claude Code, with one marked step per task                               |
| Any other host      | none                                                       | Skip the load and report it as unavailable                                       |

On Copilot, run these statements in order. Double any single quote in a value. `todos.id` is the primary key, so the
upsert converges. `<id-list>` is the quoted list of the plan's task IDs.

    SELECT count(*) FROM todos WHERE id IN (<id-list>) AND IFNULL(description, '') NOT LIKE 'agentics-plan-task%';
    INSERT INTO todos (id, title, description) VALUES ('<task-id>', 'Task <N>: <title>', 'agentics-plan-task <plan-id>' || char(10) || '<text>')
      ON CONFLICT(id) DO UPDATE SET title = excluded.title, description = excluded.description,
        status = CASE WHEN substr(todos.description, 1, length('agentics-plan-task <plan-id>') + 1) = 'agentics-plan-task <plan-id>' || char(10)
          THEN todos.status ELSE 'pending' END
      WHERE todos.description LIKE 'agentics-plan-task%';
    DELETE FROM todo_deps WHERE todo_id IN (SELECT id FROM todos WHERE description LIKE 'agentics-plan-task%')
      OR depends_on IN (SELECT id FROM todos WHERE description LIKE 'agentics-plan-task%' AND id NOT IN (<id-list>));
    DELETE FROM todos WHERE description LIKE 'agentics-plan-task%' AND id NOT IN (<id-list>);
    INSERT INTO todo_deps (todo_id, depends_on) VALUES ('<task-id>', '<dependency-id>');
    UPDATE todos SET status = 'done' WHERE id = '<task-id>' AND status <> 'done' AND description LIKE 'agentics-plan-task%';

Run the first statement once, before any write. It must return 0. A larger count means an ID collision with a row
Agentics did not write: treat the load as failed before any write, report the projection as unavailable, and ask the
approval question in chat. Run the `INSERT INTO todos` statement once per task and the `INSERT INTO todo_deps` statement
once per dependency edge. The `CASE` on the upsert keeps the status only when the existing row carries this plan's
marker line. A row marked for another plan resets to `pending`, so a completed task of plan A never reads `done` for
plan B. The stale-row delete already removes the rows of other plans whose IDs are not in the new plan. The
`[<task-id>]` prefix form on other hosts has no cross-plan store and needs no plan ID. The `WHERE` on the upsert is
defense in depth. It leaves a row that Agentics did not write alone. The dependency delete removes every edge on a
marked task and every edge toward a removed task, so it cannot block the delete of that task. Edges that other agents
wrote toward a kept Agentics task are left alone. Run the last statement only for a task that plan state already shows
complete, and only when `execute` seeds the load. It never moves a record away from `done`. It also corrects a record
left at `in_progress` or `blocked` because a status write failed.

`TodoWrite` and `update_plan` replace the whole list and cannot be read back. A skill that loads them sends its own
current list, with the statuses it already holds in its context, plus the marked items. A resumed run in a new session
builds that list from plan state.

On Copilot, also write the full `plan.md` body to the `Plan file:` path in the `<session_context>` block. Make the first
line name the canonical repository path. Overwrite the file on every load and never edit it by hand. When the host
exposes no plan file, such as Claude Code outside plan mode, load task records only.

For an explicit session-only plan, no work folder exists. Leave out the line that names a repository path.

## Approval

`decompose` asks its approval question after the load succeeded. It asks it once, through exactly one mechanism. The
wording and the answer handling are the same for every mechanism.

| Host                | Mechanism                                                                                 |
| ------------------- | ----------------------------------------------------------------------------------------- |
| Copilot CLI and app | The session's plan-approval tool when it exposes one, otherwise the `ask_user` tool       |
| Claude Code         | A chat question. `ExitPlanMode` needs plan mode, which blocks the writes that come first. |
| Codex, any other    | A chat question                                                                           |

Ask in chat when the load failed, the tool is absent, or the tool returns an error. An unattended run asks nothing. On
the combined routine-tier question, show the spec review's findings and the tier line in a chat message before the
prompt, and make the first lines of the Copilot plan file state that a yes also approves `spec.md` at its path.

## Status

`execute` writes a task's status at these points. Copilot's `todos.status` accepts exactly `pending`, `in_progress`,
`done`, and `blocked`. `TodoWrite` and `update_plan` accept `pending`, `in_progress`, and `completed`.

| Point                                                                                    | Copilot `todos.status`                  | `TodoWrite` and `update_plan`                          |
| ---------------------------------------------------------------------------------------- | --------------------------------------- | ------------------------------------------------------ |
| The task's implementer is dispatched                                                     | `in_progress`                           | `in_progress`                                          |
| The task's review-group quality review cleared its commit and its commit evidence exists | `done`                                  | `completed`                                            |
| The task is paused as `blocked: <gap> — <question>`                                      | `blocked`                               | `in_progress`, with the text prefixed `blocked: <gap>` |
| The task is skipped as `skip: <reason>`                                                  | `done`, with a `skip:` description line | `completed`, with the text prefixed `skip: <reason>`   |

An implementer's `DONE` status alone never sets `done`. A task sent back by review returns to `in_progress`.

On Copilot, the reason for a skipped task goes on a last description line `skip: <reason>`. The first line stays the
marker. A reload overwrites the description, so when `execute` seeds a task that plan state shows as skipped, it writes
the `skip:` line again.

On `TodoWrite` and `update_plan`, the marker stays first and the note follows it: `[<task-id>] blocked: <gap> ...` and
`[<task-id>] skip: <reason> ...`.

## Failure

A failed load, approval prompt, or status write never changes a run's outcome. Keep the canonical files. Report the
failure once, as unavailable, and never as succeeded. After a failed load, write no status for that run, and report it
once.

## The fleet exception

The Copilot fleet contract in [surface-dispatch.md](surface-dispatch.md) has subagents update their own rows and treats
the SQL state as the board of record. That path is unchanged. Under it, `execute` checks a subagent's `done` claim
against the Stage 2 evidence before it keeps the row `done`.
