# Output discipline

Shared rules for what every skill prints, drafts, and claims. Skills link here instead of repeating this boilerplate.

## Verbatim drafts and zero placeholders

Whenever a workflow previews, drafts, or delivers an artifact, diff, comment, rollup, brief, or command, render the
complete verbatim text using concrete computed or fixture-backed values:

- No elision markers (`...`, `(unchanged)`, comment-only stubs) and no narrative summaries in place of required content.
- No unresolved template placeholders (`<trunk>`, `<summary>`, `<test command>`, `<login>`, `[annotation ...]`,
  `<all task ids>`, `<real counts>`). Resolve every value from inspected repo state, tool output, or scenario fixtures;
  if a value is unavailable, state that explicitly rather than printing a placeholder.

## Executed vs. simulated status honesty

Report a write, commit, push, check, review, post, URL, SHA, or read-back value as completed or passing only after the
proving command actually ran and succeeded in this session.

- In simulated execution or unapplied previews, deliver the complete candidate text labeled unapplied/proposed and state
  explicitly which checks or actions did not run.
- Never claim a check passed, a comment posted, or a branch pushed from memory, from an earlier turn, or from intent
  alone.

## Question formatting

Decision questions follow [question-format.md](question-format.md): column-0 `❓ **Q1** - **<title>**: ...` and
`➡️ <recommendation>` inside one visible blockquote posted before any ask-tool call. Fixed yes/no confirmations and
approval questions keep their fixed wording.

## Skill command syntax

Skill commands use `/agentics:<skill>` on Claude Code, Copilot CLI, and VS Code Chat, and `$agentics:<skill>` on Codex.

## Gate response shape

Every gate follows [gates.md](gates.md):

- **Safety gate:** state once what is held, the harm prevented, and the user's alternative; repeated demands get a
  one-line pointer back.
- **Repository policy:** hold while enabled, name the governing setting once, and write no `waive` row.
- **Process gate:** name the cost once and yield on the next declining reply with the required waiver record.
