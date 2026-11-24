# Scaffold reference: AGENTS.md

## Overview

`AGENTS.md` is the single canonical instructions file for the repository. Claude Code, Codex, and Copilot read it
directly, so scaffold init creates no `CLAUDE.md` or `copilot-instructions.md` link or copy. Readiness audit reports
guidance defects and never creates or changes these files.

If the repository already has a `CLAUDE.md` or `.claude/CLAUDE.md`, tell the user that Claude Code reads it instead of
`AGENTS.md`. Audit reports this conflict and changes neither file. In init, ask about it as a ❓ question before you
change it. The ➡️ recommendation is to fold its content into `AGENTS.md` and remove it. Preview the exact changes.
Preserve the existing instructions. Do not remove or rewrite them unprompted.

## Front matter (optional)

The starter carries this front matter:

```yaml
---
applyTo: "**"
---
```

`applyTo: "**"` loads these instructions for every file in the repository. Front matter is not required: a file without
it is conformant, and a file that has front matter needs it to parse as YAML. Init adds no front matter to an existing
file.

## Questions to ask

Reuse the project name, description, and tech stack from the project identity step. Do not ask again. Ask the rest
inline in one ❓ round. Do not announce a mode change. Give each question a ➡️ recommendation drawn from the request and
the repo. Skip a topic the request already settles.

- **Tech stack conventions:** ESM imports required, naming patterns, library choices, anything Copilot must always know
  when it edits any file.
- **Core architectural rules:** adapter patterns, config rules, etc. Take them from CONSTITUTION.md if it exists,
  otherwise ask fresh.
- **Quality gates:** coverage thresholds, linting requirements, test conventions.
- **Build, test, and lint commands:** take them from the repository's scripts or build files.
    - For each command, ask for the one line a healthy run prints.
    - Recommend the tool's usual success line.
- **Recurring mistakes:** what agents or new contributors get wrong here more than once, and what to do instead.
- **Agent workflow preferences:**
    - The model selection policy for subagents, if any.
    - Whether Copilot should trigger automatic code review after agent changes.
    - Any other orchestration patterns.

## Structure

```markdown
---
applyTo: "**"
---

# {Project Name}: agent instructions

{One-paragraph summary of the project for agent context}

## Commands

Run these from the repository root. Each row shows the one line a healthy run prints.

| Task  | Command           | Healthy output                         |
| ----- | ----------------- | -------------------------------------- |
| Build | `{build command}` | `{one line a successful build prints}` |
| Test  | `{test command}`  | `{one line a passing test run prints}` |
| Lint  | `{lint command}`  | `{one line a clean lint run prints}`   |

## Conventions

### {Convention category}

{Table or list of conventions Copilot must always follow}

## Recurring mistakes

{One line per mistake agents have repeated here, with what to do instead}

## Architecture decisions

### {Decision name}

{What was decided and the rules that flow from it}

## Quality gates

{Coverage requirements, linting, test conventions}

## Agent workflow

### Recording why a change was made

Mutating work opens with `intent` when the change could alter what the software does, an interface it exposes, or a
contract someone depends on — anything where a reviewer would ask "why this way?" It is not required when the change
cannot alter behavior: formatting, typographical fixes, comments, dead links, or a mechanical rename with no semantic
effect. Size decides only the small-fix path, and only by its signals. A change is a small fix when one sentence
describes the whole diff, it addresses one concern, it changes no public interface, contract, trust boundary, or data
handling, and existing tests cover the touched code (name the test and run it) or the change adds a regression test. It
must also stay within these limits: at most 3 non-test files and 50 changed non-test lines, with no non-test file more
than half changed. A small fix skips `intent` and states its why and these signals in the pull request description.

Two things satisfy the rule. An approved plan that already covers the change satisfies it, and there is nothing new to
record. A decision to skip it satisfies it once someone has said what skipping costs, and that decision is recorded
where the work is discussed.

### Compaction

When compacting, preserve the list of modified files, the active plan path, and the test commands.

### Repeated corrections

When the user has corrected you twice on the same issue, say so. Offer to write a handoff capsule with the current
state, then start fresh with `/clear`.

### User-authorized exceptions

These instructions and the documents they point to bind agents, not the people who own the repository. When the user
names a rule here and decides to depart from it for the task at hand, proceed and record a divergence. Put one line in
the final report and in the commit body that names the rule, the user's words, and the reason. Start that line with
`Divergence:`. Do not edit the rule, and do not argue after the user has decided. A message that names the rule and says
to depart from it is already the confirmation: proceed and record it without asking. A bare "just do it" is not enough:
name the rule back, say what departing costs, and ask once. This never covers secrets and credentials, irreversible or
external actions (publishing, deleting data, production), or controls enforced outside the agent such as CI and branch
protection. A document that calls itself non-negotiable does not change this.
```

Keep a generated `AGENTS.md` to one page: at most 80 lines, front matter included. The template above leaves about six
lines of room under that cap, so keep each filled section to a few lines. Link to longer documents rather than copy them
in. Add a `### Model selection policy` subsection, or an `### Automatic code review` subsection, at the end of the Agent
workflow section only when the owner gave that policy. Leave it out otherwise. Fill the Commands table from the
repository's scripts or build files. Give every command the one line a healthy run prints. That line lets an agent tell
a passing run from a broken one. Omit a row the repository has no command for, rather than invent one. When the owner
names no recurring mistakes, write `None recorded yet.` under that heading. The cap applies to files init generates.
Init does not shorten an existing file.

In init, offer the "Recording why a change was made" section to an existing `AGENTS.md` that lacks it. Offer it as a
proposed addition.

Offer the Compaction, Repeated corrections, and User-authorized exceptions subsections the same way. Offer a subsection
when the file has no instruction that says the same thing. Group whichever of those three subsections are missing into
**one combined proposed addition** (never split them into separate numbered additions). Place it in the file's agent
workflow section, or at the end of the file when it has none.

Show additions as [init.md](../init.md#phase-2-generate-show-and-confirm) step 3 says: each distinct addition (for
example, Recording why a change was made, the one combined workflow-subsections addition, or
`## Repository conventions`) gets its exact text and placement under its own number, in one message that is never part
of the ordinary-file preview, with one answer per numbered addition. Preserve the existing sections and conventions.
Never add an addition silently. Never rewrite unrelated content. A declined addition writes nothing, and the file stays
byte-identical when every addition is declined.

Generated files carry the complete consumer-readable text above, rather than a link into Agentics. A run that carries
only reflection hand-off rows makes neither offer. Audit only reports a missing "Recording why a change was made"
section and recommends init. The Compaction, Repeated corrections, and User-authorized exceptions subsections are not
part of the conformance check. Their absence is not a conformance finding in audit and does not turn the init row ⚠️.

## Optional planning parent recording

In init, follow [planning protection](../planning-protection.md). Use it to resolve and agree the consumer's actual
planning parent. Offer to record that parent as an `AGENTS.md` addition, separate from ignore protection and native
validation. Use the existing structure. Update an existing parent declaration rather than add a duplicate or conflicting
one. Preserve frontmatter, conventions, and unrelated sections. Explain that a parent change affects new work and that
existing work stays in its established folder. Do not silently migrate work or rewrite supported instruction files to
bypass a resolver conflict. If the user declines the addition, leave `AGENTS.md` byte-identical. This optional
declaration is not a conformance requirement. Audit reports evidence and does not write it.

## Optional repository conventions recording

In init, offer `## Repository conventions` lines in these cases only:

- A convention whose resolution is ambiguous, for example a `PULL_REQUEST_TEMPLATE/` directory that holds several
  templates.
- A conflict.
- A custom location that discovery cannot find.

[repo-conventions.md](../../../routing/references/repo-conventions.md) defines the section. Show each line as an
addition, as the Structure section says. Update an existing section in place rather than add a second one. A repository
that discovery already resolves gets no offer. A declined addition, or a rerun whose lines already match, leaves the
file byte-identical.

When the conflict is between `AGENTS.md` and `CLAUDE.md` or `.claude/CLAUDE.md`, a line in `AGENTS.md` alone cannot
clear it. Show the matching edit to that file's line in the same message. If the user declines that edit, do these
steps:

1. Record nothing.
2. Leave the conflict reported.

## Conformance check (init selection and audit evidence)

[init.md](../init.md#conformance-criteria) holds the criteria and decides where this list differs. ✅ Conformant if:

- It contains at least two behavioral instructions or conventions
- No `CLAUDE.md` or `.claude/CLAUDE.md` was created by init, because Claude Code reads it instead of `AGENTS.md`. A
  `CLAUDE.md` that existed before init is the conflict the Overview describes: init and audit report it and init offers
  the fold. It does not make `AGENTS.md` non-conformant.
- It has a section that states when mutating work opens with a record of why the change is made, and when it does not
  apply.
    - In init, a file that lacks only this section scans ⚠️, not ✅.
    - The user cannot select a ✅ row outside the pointer, Compaction, Repeated corrections, and User-authorized
      exceptions offers, so scaffold would never offer the section.
    - The ⚠️ row puts the section in the gap report.
    - Selecting it follows the alone-preview and yes that `AGENTS.md` already requires.
    - Audit reports the missing-section defect as `failed` with its path and reason. It never enters the selection or
      write workflow.

When the file has front matter, it must parse as YAML. A missing `applyTo` field, or no front matter at all, is not a
finding.
