# Workflow steps

Use the current workflow's mode or entry path to determine which steps and checklists to track and when they enter the
session's todo list. Add each named step and checklist item verbatim, before that workflow's task-specific items. Add
each entry once per run. A re-entry updates its existing entries, and each later phase appends its own entries while
preserving the existing list and statuses. A phase adds only its own entries; the next phase adds its entries when it
starts. When the host tool replaces the whole list, send every current entry with its text unchanged, including any
`skip:` or `blocked:` note, along with the new ones.

Keep a step the workflow decides not to perform visible as `skip: <one-line reason>`. Keep an item waiting for an answer
as `blocked: <gap> — <the question>`. A blocked item still needs a decision. The mode owns conditional skips, checklist
outcomes, no-tool fallbacks, and any path that does not use a todo list.

Use [native plan projection](../native-plan-projection.md) when loading plan tasks into a host's task tool.
