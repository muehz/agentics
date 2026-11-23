# Guard the Context Window

_Apply when context fills up: large outputs, long files, repeated reads, fan-out planning._

The context window is finite and non-renewable within a session. Every token that enters should earn its place.

Context overflow degrades reasoning quality, creates compression artifacts, and halts progress. Unlike compute or time,
context spent inside a session cannot be reclaimed.

Use these practices:

- **Isolate large payloads.** Route verbose outputs, screenshots, and large documents to subagents. The main context
  gets summaries, not raw data.
- **Do not read what you will not use.** Read selectively based on relevance. If the current task does not need a file,
  skip it.
- **Keep frequently used content inline.** Templates and references that every invocation uses belong in the skill file,
  not in separate files that cost a read each time.
- **Size phases and cap scope.** Limit the files read and the tool calls made in each phase. Account for mechanism
  costs.

A file that a skill, a charter, or the user tells you to read in full is read in full, whatever its length. The
instruction wins over the size rule below. This covers a `SKILL.md`, a leaf, a brief, or a spec. The instruction binds
whoever it addresses, so a subagent told to read a file in full reads it, and the main thread does not read it as well.

This principle is the reason this library is an index plus leaf files. The index costs little on every task, and a full
rule loads only when its principle is in play.

## Self-test

- Is this a file, command output, or search result that no instruction tells me to read in full, and does it run past
  200 lines? Check with a line count or a bounded run first. If yes, send it to a subagent or read a narrower slice.
  Keep the summary.
- Will I use every part of what I am about to read? If no, read a narrower slice.
- Is this content needed on every invocation? If yes, keep it inline. If no, move it behind a separate file.
- Does this phase have a stated cap on the number of files read? If no, set one before you start. Count the files read
  so far. At the cap, stop reading and summarize.
