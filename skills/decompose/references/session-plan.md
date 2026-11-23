# Session plan

This runs only when the user explicitly asks for an ephemeral or session-only plan, under **Plan Storage** in
`skills/decompose/SKILL.md`. Nothing here is staged or committed, and no `state.tsv` row is written, because no work
folder exists.

**Copilot CLI.** Write the plan with the `edit` or `create` tools to the `Plan file:` path in the `<session_context>`
block. Do not create a file under `<plan-parent>/`. Then load the tasks into the session `todos` table as
[native-plan-projection.md](../../routing/references/native-plan-projection.md) defines, without the repository-path
line.

**Every other host.** Resolve the host's designated scratch location or permitted temporary-file facility with
[session-storage.md](../../routing/references/session-storage.md). Then create a unique plan folder there. Do not invent
a common scratch environment variable or hardcode a temporary path. If temporary storage is unavailable, create a unique
`agentics-plan-<unique-id>/` directory directly under the writable workspace root, and keep it unstaged.

Report the plan's absolute path, and use it as `<plan-path>` in Step 6's question. A yes to that question writes no row
here: `execute` materializes the plan into a work folder and records the approval there. Remove the folder only after
the plan is executed or transferred to a durable artifact and no longer needed. If the session ends first, report that
the file remains. Do not promise automatic cleanup.
