# Agentics Charter

This is the editorial charter for the Agentics plugin. It answers one question. What does it mean for a skill,
instruction, hook, or MCP server to earn its place here? These principles are opinions, not a checklist. Use them to
decide what to build, what to change, and what to cut.

---

## 1. Each asset type has exactly one job

Skills are workflows. Instructions are persistent rules. Hooks are guardrails. MCP servers are integrations. When an
asset is doing the job of a different asset type, fix the boundary. Don't stretch the wrong one.

## 2. Trigger boundaries must be explicit; ambiguity must be handled

Skill triggers and descriptions should make ownership clear. Some requests legitimately cross boundaries; when two
assets match, apply a documented precedence or disambiguation path. If overlap adds no value, narrow or merge the
assets.

## 3. Routing beats execution

A skill that hands off to three specialists is worth more than a 2000-line monolith. The suite composes; it doesn't
accumulate. If you feel the urge to add another branch to an existing skill, consider whether you're building a new one
instead.

## 4. Skills own _how_

A skill is a recipe: the steps, checks, and stopping points for a kind of work. It does not define a persona; the agent
that loads it keeps its own judgment and voice. When a skill starts to cover a second kind of work, create a distinct
skill. Don't make the existing one larger.

## 5. Hook failure posture matches consequence; required features fail loud

Hooks may automate protection, checks, or formatting, but their failure posture is deliberate per hook. A guard whose
absence would silently remove protection fails closed; best-effort formatting and reporting hooks may fail open. A hook
must not be the sole home of a required user-facing capability. If removing it breaks the core workflow, that capability
belongs in a skill.

## 6. Reference content loads on demand

`SKILL.md` is the entry point. Long reference material lives in `references/` and gets pulled in only when needed.
Always-loaded context has a real cost. The context budget is a shared resource across the whole suite.

## 7. Instructions are rules, not playbooks

"Always do X" is an instruction. "When asked to Y, follow these steps" is a skill. Instructions don't have numbered
steps; if yours does, it's the wrong asset type and the steps belong in a `SKILL.md`.

## 8. A hard rule may carry the failure it prevents

A rule holds under pressure when the reader can see what breaking it costs. State the rule, then give at most one line
for what breaks without it, drawn from the domain the skill works in. `/agentics:git` already does this: "NEVER force
push to `main` or `master` — it rewrites shared history and can drop teammates' commits." The rationale is optional, and
it is one line. A rule that needs a paragraph of justification is a rule the reader skips; put the long version in
`references/`.

This is not the history narration that skill bodies ban. Rationale names a failure mode in the world. History narrates
the asset's own past, and that belongs in `git log` (with a row in `docs/RENAMES.md` for a rename or a retirement).

## 9. Surface support is declared in the contract that owns it

Availability, packaging, and verified behavior are separate claims. Skill availability is recorded in
`metadata.surfaces`; provider manifests define what each adapter packages; surface documents and the review ledger
record what has actually been observed. Cross-platform is the default, anything narrower is explicitly scoped, and
silent platform-specific behavior is a bug.

## 10. MCP servers are infrastructure, not products

Agentics ships none of them. A server is plumbing the consumer runs in their own environment; a skill that can use one
detects it and degrades to a CLI when it's absent. If a skill's value proposition is "use this MCP server," the skill is
the wrapper that gives the server a job. It still has to work without it.

## 11. The suite gets pruned, not just appended to

Assets earn their place. Low-invocation assets that aren't guardrails are deletion candidates, not permanent fixtures.
Curated is better than collected. The bar for adding is the same as the bar for keeping.

## 12. No backwards-compatibility shims

When an asset changes shape, the callers change with it. We don't ship v1-compatible wrappers alongside a v2. The suite
is small enough to refactor in place. Lean on that fact.

## 13. Workflow intent is language-agnostic; commands follow the project

Workflow skills describe intent without requiring an unrelated ecosystem. When commands are needed, detect and use the
project's existing tooling; ecosystem names are acceptable as detected commands or clearly illustrative examples. Skills
whose subject is a specific language or framework may prescribe that ecosystem.

## 14. Use the harness, don't fight it

Every harness ships native machinery for plans, task lists, approval prompts, and temporary files. A skill uses that
machinery first, and adds its own only for what the machinery cannot hold. A skill that writes a file where the harness
offers a tool leaves the host display empty and the approval prompt silent, and the user then approves the same work
twice. Feature-detect the tool; never guess it from the product name. When no native slot exists, degrade to a file, and
choose a permitted location that matches its required lifetime. `skills/routing/references/session-storage.md` carries
the verified paths and the lifetime rule that picks between them.

## 15. Interactive sessions are the product boundary

Agentics supports interactive work between a person and an agent, and extensions of that work that preserve its context
and human direction. It does not provide consumer CI/CD automation or build alternatives to native pipeline
capabilities. A useful technique does not belong in the plugin merely because an agent can perform it.

Agentics may use CI to test, evaluate, and release itself; that infrastructure supports the plugin rather than expanding
its product scope.

## 16. Built for a second team, proven on the first

Agentics is developed in one maintainer's repositories but designed for adoption elsewhere. A choice that only suits
this repository is a default a consumer can change, not a rule.

## 17. Write for the floor model, escalate for review

Skills are detailed on purpose, not because frontier models need every step spelled out. Each skill is evaluated against
the weakest models worth running — currently the GPT Luna and Claude Haiku lines — so a lower-cost model can follow the
workflow, stay on track, and implement difficult work to a reliable baseline on its own, calling in stronger models
primarily to review its work. If Luna and Haiku can hold the line, stronger models can too. Do not strip explicit steps
or stopping points just because a frontier model would usually infer them; move detail into `references/` when the
context budget calls for it (§6).
