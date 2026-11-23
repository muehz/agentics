# Agentics consumer guide

This guide is for using Agentics day to day. It leans on copyable prompts instead of assuming that every coding tool
exposes the same commands or plugin features.

Work moves through four phases: Plan, Build, Verify, and Ship. Start with [setup](01-setup.md) and
[routing](02-routing.md), then follow the link at the end of each page through the phases. Two side pages cover work
outside that line, and the reference pages go deeper. Once the flow makes sense, jump straight to the page that matches
the work in front of you.

The guide explains how to ask for the work. It doesn't copy the rules from the skills or hooks. Links point back to the
files that own those rules so the guide doesn't become another version of them. The [Agentics AI SDLC](../AISDLC.md)
summarizes the whole flow in one place: stages, approvals, risk tiers, execution modes, and the small-fix path. For why
the skills are written with this much structure, see
[Why the skills are this detailed](../../README.md#why-the-skills-are-this-detailed).

Slash commands are shortcuts where a coding tool supports them. Plain chat requests are the portable path.

When you invoke an installed Agentics skill explicitly, Claude Code, Copilot CLI, and VS Code Chat use
`/agentics:<skill>`. Codex uses `$agentics:<skill>`. The plugin-qualified forms prevent an Agentics skill from colliding
with a built-in or project skill of the same name. The skills and this guide print every command in the
`/agentics:<skill>` form; on Codex, type `$agentics:` in place of `/agentics:`.

Coding tools don't behave the same way. When that difference matters, the guide points to the
[surface docs](../surfaces/README.md), which separate published requirements from behavior Agentics has actually
observed.

## Main path

| Page                         | Use it when                                                                  |
| ---------------------------- | ---------------------------------------------------------------------------- |
| [01. Setup](01-setup.md)     | Installing Agentics and checking that it loaded                              |
| [02. Routing](02-routing.md) | Phrasing a request so it reaches the right skill                             |
| [03. Plan](03-plan.md)       | Shaping work before code: intent, spec, and an ordered plan                  |
| [04. Build](04-build.md)     | Running an approved plan, test-driving a change, and committing as you go    |
| [05. Verify](05-verify.md)   | Reviewing the diff, root-causing a failure, and running a security pass      |
| [06. Ship](06-ship.md)       | Running the completion gate, the GitHub mechanics, the docs, and the handoff |

## Side pages

| Page                             | Use it when                                                                      |
| -------------------------------- | -------------------------------------------------------------------------------- |
| [Understand](side-understand.md) | You need to know how code works, why it is that way, or where you left off       |
| [Operate](side-operate.md)       | The change is live: an incident, a postmortem or runbook, or a metric to improve |

## Reference

| Page                                                         | Use it when                                                                         |
| ------------------------------------------------------------ | ----------------------------------------------------------------------------------- |
| [07. Hooks](07-hooks.md)                                     | A command, a credential read, or a secret write was blocked, or a hook needs tuning |
| [08. Extend](08-extend.md)                                   | You want your own skills or agents, session lessons, or a verify loop               |
| [09. Recipes and pitfalls](09-recipes-and-pitfalls.md)       | You want an end-to-end prompt sequence or the pitfalls in one table                 |
| [10. Update and troubleshoot](10-update-and-troubleshoot.md) | Agentics needs refreshing, removal, or help with a missing workflow or hook         |

Next: [01. Setup](01-setup.md)
