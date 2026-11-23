# Session storage and plan surfaces

Where a skill puts a file, and which native surface it publishes a plan to. Every harness ships its own tools for both.
Agentics uses those tools first. It adds its own only for what those tools cannot hold. It feature-detects each tool. It
does not guess the tool from the product name.

## Choose the location by lifetime

Use the active host/tool's storage facilities and permission context before you choose a path. A path that the shell can
write to does not establish that an image viewer, document tool, or another agent can access it.

| The file must live             | Put it                                                                                                                                                      |
| ------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Until the turn or session ends | The host/tool's designated scratch location, or its runtime temporary-file facility within permitted roots                                                  |
| Past the session               | An ordinary writable workspace path that follows the project's artifact convention                                                                          |
| In the repository              | The work root that [repo-conventions.md](repo-conventions.md#work-root) resolves, stored as the [storage setting](repo-conventions.md#storage-setting) says |

Honor a user-supplied destination within the active permissions. For disposable script files, use the runtime's unique
temporary-file API. Clean up files the script owns.

Do not do any of these:

- Hardcode an OS temp path.
- Invent a shared scratch environment variable.
- Assume the host deletes arbitrary files at session end.

If no usable temporary location is available, do these steps:

- Use a permitted workspace location.
- Clean up disposable files after use.

Keep plans, handoffs, review reports, and evidence needed by later sessions out of temporary storage. Prefer an existing
project artifact directory. Otherwise, the owning skill supplies a workspace-relative default. Report the resolved path.
Workspace artifacts are visible to version control. Do not stage them unless the workflow or user requests it. An
explicitly ephemeral plan remains disposable and unstaged, even when its permitted location is inside the workspace.

Do not use git metadata or harness installation/state directories as generic artifact stores. They may be read-only,
outside the writable worktree, or managed by the host. Use a native persistence tool only when its contract supports the
artifact's lifetime and the user has authorized it.

The ban on git metadata has two named exceptions: the approval ledger, and the clone-local store that holds the work
folder under the `local` storage setting. Both live under the Git common directory and are shared by worktrees. Git's
own mechanisms used for their purpose, such as the exclude file and worktree administration, are not artifact stores. A
file a skill invents inside the git directory is an artifact store, so a recorded branch or base name goes to session
storage.

## How to read the evidence column

Provider paths drift between versions. A directory found on one machine may have been created by Agentics rather than by
the harness. Every row below carries how the claim was established. Trust the rows in this order. Prefer the path the
host names in the live session over any row here.

| Evidence     | Meaning                                                                   |
| ------------ | ------------------------------------------------------------------------- |
| `documented` | The provider's own reference page states it.                              |
| `observed`   | Seen on a real machine, with the harness confirmed as the author.         |
| `unverified` | Not established. Feature-detect at run time; never write it into a skill. |

## Claude Code

Source: [Explore the .claude directory](https://code.claude.com/docs/en/claude-directory), read 2026-08-22.

| Item               | Path                                                  | Evidence     |
| ------------------ | ----------------------------------------------------- | ------------ |
| Native plan store  | `~/.claude/plans/`                                    | `documented` |
| Native task list   | `~/.claude/tasks/`, `TaskCreate` / `TaskUpdate` tools | `documented` |
| Session scratch    | the path the host names in its context                | `observed`   |
| Plan approval tool | `ExitPlanMode`                                        | `observed`   |

`~/.claude/plans/` holds "Plan files written during plan mode". It sits in the home directory, not in any repository.
Claude Code deletes files there once they pass `cleanupPeriodDays` (default 30). `~/.claude/tasks/` holds "per-session
task lists written by the task tools" under the same cleanup.

The task tools are present by default only on Opus 4.7, Sonnet 4.6, Haiku 4.5, and earlier models, and elsewhere on
opt-in (`CLAUDE_CODE_ENABLE_TODO_TOOLS=1`, `--allowedTools`, or `--tools`). `TodoWrite` replaces them under
`CLAUDE_CODE_ENABLE_TASKS=0` ([tools reference](https://code.claude.com/docs/en/tools-reference), read 2026-11-19).
Feature-detect the tool; with none present, there is no native progress slot.

`ExitPlanMode` takes no plan argument and reads the plan file written in plan mode. Outside plan mode it returns "You
are not in plan mode" and shows no dialog (observed 2026-10-23). Plan mode also blocks repository writes, so Agentics
does not use it to approve a decomposed plan.

**A project-level `.claude/plans/` is not a Claude Code concept.** The documented project `.claude/` tree holds
`settings.json`, `rules/`, `skills/`, `commands/`, `output-styles/`, `agents/`, `workflows/`, and `agent-memory/`. It
has no `plans/` entry. Agentics itself created every project-level `.claude/plans/` found on the development machine.
Each one holds Agentics-shaped files. The six files in `~/.claude/plans/` carry harness-generated slug names, and none
uses the Agentics plan template. The slug names and the missing Agentics template tell the two sets apart.

## Copilot CLI

Source:
[GitHub Copilot CLI configuration directory](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-config-dir-reference)
and [About GitHub Copilot CLI session data](https://docs.github.com/en/copilot/concepts/agents/copilot-cli/chronicle),
read 2026-08-22.

| Item              | Path                                                                                                                                                                                                                          | Evidence     |
| ----------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------ |
| Session directory | `~/.copilot/session-state/<session-id>/`                                                                                                                                                                                      | `documented` |
| Native plan file  | `<session-dir>/plan.md`                                                                                                                                                                                                       | `observed`   |
| Session scratch   | `<session-dir>/files/`                                                                                                                                                                                                        | `observed`   |
| Native task list  | `update_todo` tool (source-observed in the Copilot CLI 1.0.109 build, 2026-11-19; absent from a 1.0.109 `copilot -p` session, which had the `todos` table, 2026-11-19), else the `todos` table through the session `sql` tool | `observed`   |
| Question tool     | `ask_user`                                                                                                                                                                                                                    | `observed`   |

The reference page says each session directory "stores an event log (`events.jsonl`) and workspace artifacts (plans,
checkpoints, tracked files)". It names no individual artifact file. A census of 45 session directories on the
development machine found `files/`, `checkpoints/`, `research/`, and `workspace.yaml` in every one, and `plan.md` in
one. That `plan.md` carries no Agentics plan-template marker, so Copilot wrote it.

A `<session_context>` block that carries a `Plan file:` path names the plan file for the live session. Prefer that path
over the table.

## Codex

| Item              | Path or tool         | Evidence     |
| ----------------- | -------------------- | ------------ |
| Session records   | `~/.codex/sessions/` | `documented` |
| Session scratch   | none found           | `unverified` |
| Native task list  | `update_plan` tool   | `unverified` |
| Native plan store | none found           | `unverified` |

The [Codex permissions reference](https://learn.chatgpt.com/docs/permissions), checked 2026-09-06, documents `:tmpdir`
(the `$TMPDIR` location) and `:slash_tmp` (`/tmp`) as writable by default in the `:workspace` profile. Profiles can deny
either. These are permission defaults, not a universal Codex scratch-directory contract. The same profile protects
`.git/` and `.codex/` inside workspace roots. Use the live session's permissions and the selected tool's output
contract. Do not infer writable storage from a directory name.

Session records and internal executable/cache directories are host state, not artifact destinations. Feature-detect
native plan/task tools in the active session.

## VS Code

Nothing verified. Feature-detect everything.

## Do not write plans into harness state directories

`.claude/`, `.codex/`, and `.github/copilot/` inside a repository hold harness configuration. A plan the user wants
committed is a review artifact, not harness state. Put a committed plan under the work root.

Never create a `.claude/plans`, `.codex/plans`, or `.github/copilot/plans` directory. A work folder goes under the work
root. Read the work root from [repo-conventions.md](repo-conventions.md#work-root). Read from
[repo-conventions.md](repo-conventions.md#storage-setting) whether the work folder is committed there or kept in the
clone-local store. No skill changes where the trail is stored without telling the user.

## Worktrees and ignore rules

The skill that creates a worktree removes it. `execute` removes an isolated-packet or arena worktree once its result is
integrated or dropped. `gate branch` removes the feature worktree on a local merge or a discard.

A feature worktree that outlives its run is removed by `git worktrees prune`. This is the normal case after a draft pull
request. A skill that finds a feature worktree whose branch has merged offers the prune.

A removal is forced only for a worktree the user discarded. A failed removal is reported and never retried with
`--force`.

A path that exists only for Agentics tooling is ignored through the repository's local exclude file. A worktree parent,
a work folder's `temp/`, and a decision log are such paths. No skill appends to a tracked `.gitignore` on its own or
commits one. `scaffold` edits `.gitignore` with its preview and confirmation; otherwise only the user's explicit request
allows the edit. When the exclude file cannot be written, the skill reports it and leaves the path untracked.

### Handoffs and resume capsules

Leave each handoff document or resume capsule unstaged. When `handoff` creates the repository's default `docs/handoffs/`
directory, it adds `/docs/handoffs/` to that repository's local exclude file once. Resolve the exclude file from the
repository root with `git rev-parse --path-format=absolute --git-path info/exclude`. Preserve existing entries. An
existing `docs/handoffs/` directory, a project-configured handoff directory, and a user-supplied project destination
retain their existing ignore policy. Leave tracked ignore files and existing index entries intact.

If the exclude file cannot be resolved or written, report that the artifact remains untracked and that this run could
not set up the local exclusion. Outside a Git repository, write to the permitted workspace and report that no Git
exclude rule applies.

## Publish the plan to the native surface

A plan document is the payload, not the display or the approval prompt. Of these three duties, only the payload needs a
file.

| Duty              | Native slot                                                                         | What Agentics adds                                                                         |
| ----------------- | ----------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------ |
| Show progress     | `TaskCreate` / `TaskUpdate` (or `TodoWrite`), `update_todo`, `update_plan`, `todos` | nothing — call the tool                                                                    |
| Get approval      | Copilot's plan-approval tool or `ask_user`; a chat question elsewhere               | the wording and answer handling, in [native-plan-projection.md](native-plan-projection.md) |
| Carry the payload | none is rich enough                                                                 | the plan document: dependencies, parallel groups, criteria, tier                           |

`execute` reads dependencies, parallel groups, acceptance criteria, and model tier from the document. No harness plan
surface holds those four, so the document stays.

Feature-detect each tool. Do not infer availability from the product name. Decompose writes its canonical document
first. Then it loads the plan and asks for approval as [native-plan-projection.md](native-plan-projection.md) defines.

If a tool is absent or fails, or the run produces multiple plans, do these steps:

- Retain the documents.
- Report the skipped or failed projection.

Do not combine independent plans in one task list with colliding IDs. An unattended run must not fire an approval prompt
that nobody can answer.

The document remains required when work must outlive the session. A display projection may still accompany it. The named
tools are not proof that every host implements them. Use only tools available in the current session.
