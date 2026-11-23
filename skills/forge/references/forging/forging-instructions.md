# Instructions creator

This guide covers two kinds of instruction file. [`AGENTS.md` and `CLAUDE.md`](#agentsmd-and-claudemd) are plain
Markdown that a host loads for the whole repository or a directory. The rest of the guide covers the Copilot files:
`.github/copilot-instructions.md` and `.instructions.md` files.

Instruction files (`.instructions.md`) are scoped coding standards that Copilot loads automatically when it edits files
that match their `applyTo` glob pattern. They encode ADR decisions, team conventions, and enforcement rules into context
that applies at edit time.

## AGENTS.md and CLAUDE.md

`AGENTS.md` and `CLAUDE.md` hold what an agent needs in every session in a repository: how to build and test, where
non-obvious things are, and facts the code does not show. A host loads the file into the context of every session, so
the file is plain Markdown. It has no front matter and no `applyTo`; those belong to the Copilot files. Match the style
of the repository's existing file before you add a line to it.

### Which hosts read which file

| Host                       | Reads                                                                                                                             |
| -------------------------- | --------------------------------------------------------------------------------------------------------------------------------- |
| Claude Code                | `CLAUDE.md`, `.claude/CLAUDE.md`, and `CLAUDE.local.md`. From v2.1.277 it reads `AGENTS.md` only when none of those three exists. |
| Copilot CLI                | `AGENTS.md` from the Git root and the current working directory                                                                   |
| Copilot cloud agent        | Every `AGENTS.md` in the repository (`**/AGENTS.md`)                                                                              |
| Codex                      | `AGENTS.md`                                                                                                                       |
| Claude Code managed review | `CLAUDE.md` and `REVIEW.md`. Its documentation does not name `AGENTS.md`.                                                         |
| VS Code                    | Depends on the harness of the session; check the host's documentation                                                             |

These rows come from the hosts' published documentation, read in 2026-10. A host changes them, so recheck the host's
page before a rule depends on one: [Claude Code memory](https://code.claude.com/docs/en/memory),
[Copilot CLI commands](https://docs.github.com/en/copilot/reference/copilot-cli-reference/cli-command-reference),
[Copilot cloud agent](https://docs.github.com/en/copilot/tutorials/cloud-agent/get-the-best-results), and
[Codex `AGENTS.md`](https://learn.chatgpt.com/docs/agent-configuration/agents-md).

Three consequences follow:

- **One file serves every host when it is `AGENTS.md` and no `CLAUDE.md` exists.** Add a `CLAUDE.md` and Claude Code
  stops reading `AGENTS.md` and reads only the `CLAUDE.md`. A repository that needs both keeps one source of truth: the
  `CLAUDE.md` points at `AGENTS.md` or imports it, as Claude Code's memory documentation describes, and holds only what
  is specific to Claude Code. Never copy a rule into both files, because the copies drift.
- **A nested file does not behave the same everywhere.** Copilot cloud agent reads every `AGENTS.md`, and Copilot CLI
  reads the root and the working directory. The documentation of the other hosts does not promise that a nested file
  overrides its parent. Put a rule that every session needs in the root file, and put a directory's own commands and
  conventions in that directory's file. A nested file never contradicts the root file.
- **Older Claude Code versions read `CLAUDE.md` only.** A repository with a team on older versions needs the `CLAUDE.md`
  pointer too.

### Combining with Copilot instruction files

A Copilot session can load `AGENTS.md`, `.github/copilot-instructions.md`, and every `.instructions.md` whose `applyTo`
matches. Do not rely on one file overriding another, so write each rule once:

- Rules for every host and every file go in `AGENTS.md`.
- A rule that applies only to some files goes in a Copilot `.instructions.md` file, with its `applyTo` glob.
- A Claude Code rule goes in `CLAUDE.md`, only when it is not true for the other hosts.

### What belongs in the file

Use the file for a rule or a fact that an agent gets wrong without it and cannot learn from the repository:

- the exact commands to build, test, and format, with the flags that matter;
- where something non-obvious lives, such as a generated directory or the one place a concept is implemented;
- a convention the code does not show, and the reason for it when the reason is not obvious;
- a trap the repository has, such as a command that is slow or a test that needs a service.

Leave out what has a better home:

| Content                                                   | Put it in                                                      |
| --------------------------------------------------------- | -------------------------------------------------------------- |
| A multi-step procedure the agent runs on request          | A skill ([forging-skills.md](forging-skills.md))               |
| A rule that must hold whatever the model decides          | A hook ([forging-hooks.md](forging-hooks.md)) or a check in CI |
| A rule a linter, formatter, or type checker can enforce   | That tool's configuration                                      |
| What the code, its names, or its comments already say     | Nowhere                                                        |
| History, a changelog, or a status that goes stale         | The commit log or the issue                                    |
| Contributor guidance for people, such as how to open a PR | `CONTRIBUTING.md` (`scaffold init` writes it)                  |

### Keep it short enough to stay always-on

Every line costs attention in every session, and a long file dilutes the lines that matter. Treat each line as a claim
the agent needs: when you cannot name a mistake that the line prevents, remove it. Remove a line that agents ignore, as
well as one they already follow; an instruction nobody follows changes nothing. When a section grows steps or examples,
move it to a skill and leave one pointer line. Point to a file by its path and not by a copy of its content, so the file
cannot go stale in two places.

Test the file the way you test any instruction: run a task in a fresh session and check that the agent finds the command
or the path the file names ([instruction-eval-criteria.md](../evals/instruction-eval-criteria.md)).

### Instructions vs skills

| Criterion               | Use Instructions                                                      | Use Skills                                                                      |
| ----------------------- | --------------------------------------------------------------------- | ------------------------------------------------------------------------------- |
| **Activation**          | Must always apply when context matches                                | Should only apply for specific requests                                         |
| **Scope signal**        | Deterministic — file type or directory path via `applyTo` glob        | Semantic — request content and intent via description trigger phrases           |
| **Context window cost** | Fixed — always present for matched files                              | Variable — zero until invoked, then loaded on demand                            |
| **Content type**        | Standards, conventions, constraints                                   | Domain expertise, procedural knowledge                                          |
| **Typical authorship**  | Standards, platform, or architecture teams                            | Domain or feature teams                                                         |
| **Right fit example**   | "All Markdown files must use heading hierarchy"                       | "When asked to write a system prompt, follow these tone and structure rules"    |
| **Wrong fit example**   | "Instructions for writing order queries" (too specific — use a skill) | "Always use 4-space indentation in Python files" (always-on — use instructions) |

### When both apply

Use an instruction to enforce the standard (always-on). Use a skill to provide the domain expertise needed to apply it
well (on-demand).

- **Docs example.** An instruction enforces documentation heading hierarchy via `applyTo: **/*.md`. A `docs` skill
  provides the Diátaxis framework knowledge to write docs.
- **Review example.** An instruction enforces code review priorities via `applyTo: **`. A `review` skill provides the
  detailed review methodology.

### Instruction types

Choose the instruction type that matches your enforcement scope.

| Type                          | Location                                        | Scope                                   | Content                                                             |
| ----------------------------- | ----------------------------------------------- | --------------------------------------- | ------------------------------------------------------------------- |
| **Repository Context**        | `.github/copilot-instructions.md`               | Global (no `applyTo`)                   | Repo overview, build commands, architecture summary, terminology    |
| **Language Standards**        | `.github/instructions/{lang}.instructions.md`   | File-type (`applyTo: **/*.py`)          | Import ordering, class patterns, error handling, coding conventions |
| **Documentation Standards**   | `.github/instructions/{docs}.instructions.md`   | File-type (`applyTo: **/*.md`)          | Heading hierarchy, navigation, formatting rules, writing style      |
| **Domain-Specific Standards** | `.github/instructions/{domain}.instructions.md` | Directory (`applyTo: infra/**/*.bicep`) | Module structure, naming conventions, security properties           |

> Types adapted from ADR-0085: Agent Instructions Pattern

## When to use

- To encode ADR decisions as enforceable coding standards
- To add team conventions scoped to specific file types
- To provide context Copilot should always have when it edits certain files
- To consolidate scattered documentation into a single authoritative source

**Don't create an instruction** in any of these cases:

- The guidance is task-oriented (use a skill).
- The guidance applies to all file types with no scope benefit (use `copilot-instructions.md`).
- The guidance is a one-time procedure (use a prompt or runbook).

## File conventions

- **Extension**: `.instructions.md`
- **Location**: Usually `.github/instructions/` in the target repository.
- **Naming**: `{domain}.instructions.md` (e.g., `security-and-owasp.instructions.md`)
- **Registration**: plugin manifests list skills and agents and have no `instructions` key. They do **not** register
  instruction directories. Keep `.instructions.md` files in the target repository's instruction location instead.

## YAML front matter

```yaml
---
applyTo: "**/*.py"
description: "Python coding standards for our team"
---
```

| Field         | Required | Purpose                                       |
| ------------- | -------- | --------------------------------------------- |
| `applyTo`     | Yes      | Glob pattern that scopes when this file loads |
| `description` | Yes      | Brief purpose statement for the instruction   |

### Common applyTo patterns

| Pattern                                         | Scope                       |
| ----------------------------------------------- | --------------------------- |
| `*`                                             | All files (universal rules) |
| `**/*.md`                                       | All markdown files          |
| `**/*.sh`                                       | Shell scripts               |
| `**/Dockerfile,**/*.dockerfile,**/compose*.yml` | Container files             |
| `**/.github/workflows/*.yml`                    | GitHub Actions workflows    |

Multiple patterns are comma-separated.

### Instruction stacking and precedence

Multiple instruction files can match simultaneously. When a user edits a Python file, they may receive:

1. Global instructions from `copilot-instructions.md` (repo context, build commands)
2. Universal instructions (e.g. an accessibility standard, `applyTo: **`)
3. Language-specific instructions from `python.instructions.md` (Python conventions, `applyTo: **/*.py`)

Copilot concatenates instructions in this order. When rules conflict, **more specific scopes take precedence**:

- Directory-scoped (`applyTo: infra/**/*.bicep`) overrides file-type-scoped (`applyTo: **/*.bicep`)
- File-type-scoped (`applyTo: **/*.py`) overrides universal (`applyTo: **`)
- Universal overrides global (no `applyTo`)

## ADR-based instruction structure

For instructions that encode Architecture Decision Records:

```markdown
# {Organization} {Domain Title}

**Source Authority:** {repository path} **Last Synchronized:** {YYYY-MM-DD} **ADRs Included:** ADR-001, ADR-015

---

## Loading Triggers

### Conditions

- User asks about {domain topic}
- User modifies {relevant file types}
- User mentions {key concepts}

### Keywords

`keyword1, keyword2, keyword3, keyword4, keyword5`

---

## ADR-001: {Title}

### Decision

{What was decided and why — 1-3 paragraphs}

### Requirements

**MUST:**

- **Requirement Label**: Specific, actionable requirement with a verb

**SHOULD:**

- **Recommendation Label**: Best practice guidance

**MUST NOT:**

- **Prohibition Label**: Explicitly forbidden actions

### Exceptions

{Documented exceptions, or "None documented."}

### Enforcement

- **How**: Automation, review gates, policies
- **When**: CI, PR review, deployment
- **Who**: Team, pipeline, policy engine

---

## Cross-References

- `{related-domain}.instructions.md` in the same instructions directory

## Revision History

| Date       | Change           | ADRs Affected |
| ---------- | ---------------- | ------------- |
| YYYY-MM-DD | Initial creation | ADR-001       |
```

## Quick reference: requirements classification

| Level        | Meaning                           | Example                                           |
| ------------ | --------------------------------- | ------------------------------------------------- |
| **MUST**     | Mandatory, enforced by automation | "Use OAuth 2.0 / OIDC for authentication"         |
| **SHOULD**   | Recommended, enforced by review   | "Prefer managed identities over service accounts" |
| **MAY**      | Optional, team discretion         | "May use connection pooling for high-throughput"  |
| **MUST NOT** | Prohibited, blocked by policy     | "Do not hardcode secrets in source files"         |

Each requirement should use an actionable verb: _configure_, _implement_, _validate_, _use_, _enforce_. Do not use vague
phrases like "follow best practices."

## Instruction evaluation workflow

After you draft or refactor an instruction file, run an evaluation pass before you treat it as complete. Revise the
wording until the behavior is clear across platforms and scopes.

### Recommended eval sequence

1. Define scenarios that cover normal use, edge cases, conflicting scopes, and platform-specific execution.
   [Eval mode](../../SKILL.md#eval-mode) sets the scenario count.
2. Evaluate the instruction file against those scenarios with a structured subagent prompt
3. Score the results with a shared rubric that covers clarity, completeness, platform fit, and conflicts
4. Check the instruction against other `.instructions.md` files in scope for contradictions or overlap
5. Verify that a command that differs between Bash and PowerShell has a block for each, and that a command that is the
   same in both is written once
6. Revise the instruction file to target the weakest criteria first
7. Re-run the eval until the scores meet the pass bar in [Eval mode](../../SKILL.md#eval-mode)

Load these references:

- [../evals/instruction-eval-criteria.md](../evals/instruction-eval-criteria.md) for the scoring rubric (clarity,
  completeness, compatibility, conflict resistance)
- [../evals/instruction-eval-runner-prompt.md](../evals/instruction-eval-runner-prompt.md) for the subagent prompt
  template
- [../evals/instruction-improvement-loop.md](../evals/instruction-improvement-loop.md) for the full revision cycle with
  conflict and platform checks

## Common mistakes

| Mistake                                | Fix                                                                                                                                                            |
| -------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Missing enforcement section            | Document WHO enforces, HOW, and WHEN                                                                                                                           |
| Too few trigger keywords               | Include 5+ keywords that cover the full domain                                                                                                                 |
| Stale sync date                        | Update `Last Synchronized` when you edit the file                                                                                                              |
| Wrong applyTo scope                    | Test glob pattern against actual repo file paths                                                                                                               |
| ADR sections without all 4 parts       | Every ADR needs Decision, Requirements, Exceptions, Enforcement                                                                                                |
| Instruction body over 3,000 words      | Long instructions degrade model attention on distant sections. Split the body into multiple scoped instructions, or move detailed content to a skill reference |
| Procedural how-to in instructions      | Instructions are standards, not tutorials. Move how-to content to a skill. If needed, link from the instruction to the skill                                   |
| Dynamic values embedded as static text | Dates, user IDs, and environment-specific values become stale. Resolve dynamic values at runtime. Instructions should contain stable rules only                |

**RELATED:** Use `/agentics:forge skill` when the guidance you write is task-oriented rather than file-scoped.
