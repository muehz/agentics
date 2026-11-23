# Logic prototype

A logic prototype is a tiny interactive terminal app in which the user drives a state model by hand. Use it when the
question is about **business logic, state transitions, or data shape**. These models can look reasonable on paper and
still fail when you push them through real cases.

## When this is the right shape

- "I'm not sure if this state machine handles the edge case where X then Y."
- "Does this data model actually let me represent the case where..."
- "I want to feel out what the API should look like before writing it."
- Anything where the user wants to **press buttons and watch state change**.

If the question is "what should this look like," use [ui.md](ui.md) instead. If the answer is a number or an output one
run can print, use [probe.md](probe.md).

## Process

### 1. State the question

Before you write code, write down the state model and the question in one paragraph. Put the paragraph in the
prototype's README or in a top-of-file comment, so the verdict can be checked against it later.

### 2. Pick the language

Use the language that the host project uses. If the project has no obvious runtime, such as a docs repo, ask for the
runtime in the [question format](../../routing/references/contracts/question-format.md), in the same round as any other
open question. Recommend the runtime the repo suggests; when it suggests none, a runtime that is already installed; when
none is installed, Node.js with TypeScript, which the user must approve installing. In an
[unattended](../../routing/references/contracts/unattended.md) run, use the recommended runtime only when it is already
installed, and state the assumption.

Match the project's existing conventions for tooling. Do not add a new package manager or runtime just for the
prototype.

### 3. Isolate the logic in a portable module

Put the logic that answers the question behind a small, pure interface. Design the interface so you could lift it out
and drop it into the real codebase later. The TUI is throwaway.

The right shape depends on the question:

- **A pure reducer**, `(state, action) => state`. Good when actions are discrete events and state is a single value.
- **A state machine** with explicit states and transitions. Good when "which actions are even legal right now" is part
  of the question.
- **A small set of pure functions** over a plain data type. Good when there is no implicit current state, just
  transformations.
- **A class or module with a clear set of methods** when the logic genuinely owns ongoing internal state.

Pick the shape that best fits the question, _not_ the shape that is easiest to wire to a TUI. Keep it pure: no I/O, no
terminal code, no `console.log` for control flow. The TUI imports it and calls into it, and nothing flows the other
direction.

This module is the part that real implementation may start from, as a draft, through
[the throwaway line](../SKILL.md#the-throwaway-line). The TUI shell goes with the rest of the prototype.

### 4. Build the smallest TUI that exposes the state

Build it as a **lightweight TUI**. On every tick, clear the screen (`console.clear()` / `print("\033[2J\033[H")` /
equivalent). Then re-render the whole frame. The user should always see one stable view, not a scrollback that grows
without end.

Each frame has two parts, in this order:

1. **Current state**, pretty-printed and diff-friendly (one field per line, or formatted JSON). Use **bold** for field
   names or section headers. Use **dim** for less important context (timestamps, IDs, derived values). Native ANSI
   escape codes are fine: `\x1b[1m` bold, `\x1b[2m` dim, `\x1b[0m` reset. You need no styling library unless the project
   already has one.
2. **Keyboard shortcuts**. List them at the bottom: `[a] add user  [d] delete user  [t] tick clock  [q] quit`. Make the
   key bold and the description dim, or the reverse, so the shortcuts are easy to scan.

Behaviour:

1. **Initialise state** with a single in-memory object or struct. Render the first frame on start.
2. **Read one keystroke (or one line)** at a time. Dispatch it to a handler that mutates state.
3. **Re-render** the full frame after every action. Do not append to the existing frame. Replace it.
4. **Loop until quit.**

The whole frame should fit on one screen.

### 5. Make it runnable in one command

Put the one command at the top of the prototype's README or entry file, such as
`npx --no-install tsx prototype-checkout-fsm/tui.ts`. Use a runner the project already has installed, never one that a
command fetches on first use, and ask before installing a missing one, as [rule 2](../SKILL.md#rules-for-every-mode)
says. Do not add a script to the project's task-runner configuration.

### 6. Hand it over

Give the user the run command, because they will drive it themselves. The verdict waits on their run, so do not report
one yet. Pay attention when they say "wait, that shouldn't be possible" or "huh, I assumed X would be different." Those
reactions expose bugs in the _idea_. When they ask for new actions, add them.

In an unattended run nobody can drive the TUI. Write the cases the question names as a fixed list of actions, run them
through the pure module in one script, print the state after each, and report that output as the evidence.

### 7. Capture the answer

When the prototype has done its job, the answer to the question is the only thing to keep. Ask the user what it taught
them, or state the verdict from the output you saw. Then record it and close as [When done](../SKILL.md#when-done) says:
remove the throwaway only after the confirmation that section asks for, and in an unattended run leave it and report its
paths.

## Anti-patterns

- **Do not add tests.** A prototype that needs tests is no longer a prototype.
- **Do not wire it to the real database.** If the question is not specifically about persistence, use an in-memory
  store.
- **Do not generalise.** Do not design for "what if we wanted to support X later." The prototype answers one question.
- **Do not blur the logic and the TUI together.** If the reducer or state machine references `console.log`, prompts, or
  terminal escape codes, it is no longer portable. Keep the TUI as a thin shell over a pure module.
- **Do not ship the TUI shell into production.** The shell is optimised for hand use from a terminal. The logic module
  behind it is the part real implementation may start from.
