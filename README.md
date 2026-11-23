# Agentics

Agentics is a plugin for AI coding assistants that brings together workflows and hooks to help keep them on track. It
gives an agent a process for working through an idea, planning the changes, writing the code, and checking what it
actually did.

> **Heads up: this is my playground.** I use it to try ideas, break things, and rework them, so stability is not
> guaranteed. Skills, hooks, and file layouts change often and sometimes without notice. If you pin a version and skim
> the release notes before you upgrade, you'll have a better time.

I started putting this together because the inconsistency from one session to the next drove me insane. I've spent
countless hours researching, watching YouTube videos, borrowing good ideas, and trying things in my own projects. This
repo is where that work landed.

As agents have gone from completing the occasional line of code to doing most of the coding, I've wanted to give them
more room to work. I'm a lot more comfortable doing that when there are clear rules, checks, and places to stop and
involve me.

## What's in here

Agentics installs as one plugin, `agentics@muehz`. Most of what you'll interact with lives in two places:

- [Skills](skills/README.md) describe how to do the work. There are workflows for planning, debugging, testing,
  reviewing changes, writing docs, and handing work off between sessions.
- [Hooks](hooks/README.md) add checks before tool calls, including risky commands, secrets, and production deployments.

The repo also contains the [Go code](hooks/go/) behind the hooks, the [maintainer tools](.github/tools/README.md), and
[evaluations](evals/README.md) for checking how the skills behave. The [architecture guide](docs/ARCHITECTURE.md)
explains how it all fits together.

There are adapters for Google Antigravity, Claude Code, Codex, GitHub Copilot CLI, and VS Code Copilot Chat. What loads
and what has been verified differs between them, so the [surface docs](docs/surfaces/README.md) keep track of those
details.

## Why the skills are this detailed

If you open one of the skills, it can look heavy compared to a loose prompt that lets the agent figure things out on its
own. That detail is deliberate. Agentics skills are evaluated against smaller, lower-cost models — currently the GPT
Luna and Claude Haiku lines (see the [eval lanes](evals/README.md#eval-lanes)) — so they give a weaker model enough
structure to stay on track and produce work that meets a reliable baseline.

The goal is for a cheaper model like Luna to carry a difficult task on its own and reach out to stronger, more expensive
models primarily to review its work (see the [model ladders](skills/routing/references/model-ladders.md)). If Luna and
Haiku can follow a skill's rules and stay on the rails, stronger models can too.

## Give it a try

Use the [setup guide](docs/guide/01-setup.md) to verify a versioned release package, install it in your coding tool, and
check that Agentics loaded.

Once it's installed, you can ask for a workflow in plain language:

```text
Use Agentics scaffold to get this repo ready for coding agents.
```

The [consumer guide](docs/guide/README.md) walks through using Agentics day to day, with prompts you can adapt to your
own work. If you want to change the plugin itself, [CONTRIBUTING.md](CONTRIBUTING.md) covers local setup and validation.

## Still a work in progress

I don't claim to be an expert in this realm. This is the setup I find useful, and I keep changing it as I learn what
works. Some parts have been tested pretty hard. Other parts are still experiments, especially when the coding tools
change underneath them. Because that happens every. single. day.

Treat this as a playground, not a product. Expect renamed skills, rewritten hooks, and the occasional idea that gets
ripped out because it didn't pan out. There's no compatibility promise, and I can't promise a fix on any timeline.
You're welcome to borrow, fork, and poke at it, but don't build anything you can't afford to rework on top of it.

A lot of the good ideas aren't mine. [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) records what came from where, and
the [charter](docs/CHARTER.md) explains what I think belongs in Agentics. The [documentation index](docs/README.md) is
there when you want to dig further.

## Where this is going

The end goal is a governance baseline a team could adopt: a shared set of workflows and guardrails for coding agents
that an organization can install, configure, and audit. It isn't there yet. Today it's my testing ground for that idea,
and the release machinery is further along than the evidence that the workflows help.
