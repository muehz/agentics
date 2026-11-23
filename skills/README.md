# Agentics skills

Each directory here contains a skill. The `SKILL.md` inside it owns the workflow, including when it should run, what it
should avoid, and which references it loads.

Agentics keeps one editable skill tree. The provider manifests decide how that tree is packaged for each coding tool, so
no single manifest tells the whole story.

## Why the skills are this detailed

Skills here are intentionally explicit rather than trusting the agent to improvise. Every skill is evaluated against
smaller, lower-cost models — the GPT Luna and Claude Haiku lines (see the [eval lanes](../evals/README.md#eval-lanes)) —
so that a weaker model has enough structure to stay on track and deliver work that meets the bar. The goal is for a
model like Luna to handle difficult implementation work on its own and escalate to stronger, more expensive models
mainly to review its work (see [model ladders](routing/references/model-ladders.md)). If Luna and Haiku can follow the
rules and stay on track, stronger models should be able to as well.

## Using a skill

You can ask for a skill in plain language. Some coding tools also expose slash commands or other shortcuts. Start with
the consumer guide's [setup](../docs/guide/01-setup.md) and [routing](../docs/guide/02-routing.md) pages for the path
that fits your tool.

When a README or guide disagrees with a skill, the skill's `SKILL.md` wins.

## Changing a skill

[CONTRIBUTING.md](../CONTRIBUTING.md) covers the development workflow. The repository instructions in
[AGENTS.md](../AGENTS.md) point at where skill changes are documented, starting with [`forge/SKILL.md`](forge/SKILL.md).

A pull request that adds a skill or changes how one behaves must include evaluation results. Run the skill's pinned
scenario bank through the eval engine in [`muehz/evals`](https://github.com/muehz/evals), use a coding tool's own eval
tooling, or do both. Nothing runs these evals automatically.
[Evaluate a proposed skill change](../CONTRIBUTING.md#evaluate-a-proposed-skill-change) says what to run and how to
report it.
