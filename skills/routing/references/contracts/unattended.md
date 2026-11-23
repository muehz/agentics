# Unattended runs

## When a run is unattended

A run is unattended when one of three things is true, and attended otherwise:

1. The dispatching prompt says no user will answer. A dispatched subagent always qualifies.
2. The invocation is non-interactive and the prompt or host context says so: a print-mode, CI or scheduled run.
3. The user said in this conversation that they will not answer.

## What never makes a run unattended

Redirected standard input or output never makes a run unattended.

## What a skill does

- **It does not branch on the user being away.** It asks. Where its text says not to block, it states its assumption and
  continues.
- **It promises no action at a later time unless it names the host tool that will run it.** Otherwise it tells the user
  what to check and when, and acts on the next message.
- **A dispatched subagent's dispatcher answers its questions.**
- **A condition described as invocation state is restated as text the skill can read:** the dispatching prompt, a file
  or a command's output.
