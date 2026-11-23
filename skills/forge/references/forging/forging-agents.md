# Agents creator

Agents are `.agent.md` files with YAML front matter and a markdown system prompt. Two patterns exist: **lean
orchestrators** (routing only) and **domain agents** (phased workflows with skill integration). Most new agents should
be lean.

## When to use

- To create a new agent for a repository
- To refactor a legacy agent
- To choose between lean vs domain pattern
- To add skill integration or checkpoint gates

**Don't create an agent** in any of these cases:

- A skill covers the use case.
- An existing agent already routes to the domain.
- The workflow doesn't need delegation.

## Agent file conventions

- Extension: `kebab-case.agent.md` (e.g., `backend.agent.md`); the `name` field matches the filename without the
  extension.
- Registration: follow the repository's own convention for how agents are discovered or listed.

## Required frontmatter

```yaml
---
name: my-agent
description: "Use for... [clear trigger phrases and domain scope]"
color: blue
metadata:
    surfaces: [claude-code, copilot-cli, vscode]
---
```

The evaluation record is kept apart from the agent file, as [Agent evaluation workflow](#agent-evaluation-workflow)
describes. No eval key sits in frontmatter.

| Field         | Required | Constraints                                                                         |
| ------------- | -------- | ----------------------------------------------------------------------------------- |
| `name`        | Yes      | Lowercase with hyphens, max 64 characters, unique across all plugins                |
| `description` | Yes      | Starts with "Use for..." or "Use when...", includes trigger phrases, max 1024 chars |
| `color`       | No       | Visual identifier in team mode. It must be unique across agents                     |
| `model`       | No       | Override model (e.g., `fable` for complex reasoning)                                |
| `version`     | No       | Quoted semantic version, such as `"1.0.0"`. Shipped agents carry it                 |

Do **not** put platform-specific fields like `tools:` or `agents:` in frontmatter. The agent's "What You Don't Do"
constraints and the platform's permission and sandbox model enforce tool access. This keeps agents cross-platform across
Claude Code, Copilot CLI, and VS Code.

### Required sections

1. **Persona.** One paragraph that sets identity, expertise, and work style.
2. **How You Work.** 3–5 numbered principles that guide behavior. The first principle is a skill-scan rule. Base it on
   this text. "Scan before you answer. Before any response or action, scan the available skills. When one owns the
   current phase, announce and invoke it before analysis. When the user asks to skip the scan, say once what that costs:
   a skill that owns the phase goes unused and the work is improvised from memory. When the next reply still declines,
   follow the user and record that the scan did not run. Do not imitate a skill from memory, and do not narrate an empty
   scan when no skill applies. A dispatched bounded assignment is the exception: continue its assigned workflow and do
   not invoke `routing` or `intent`." Put every skill name in the charter in backticks, such as `threat-model`. In
   backticks, a skill name is never mistaken for ordinary prose, and a scan can resolve each skill unambiguously.
3. **Areas You Own.** Concrete responsibilities.
4. **Tools and evidence.** The domain's concrete tools, commands, data sources, and artifacts that the agent reads or
   runs. Say what the agent does when a source is unavailable. Generic text such as "use available read-only tools" does
   not meet this requirement. Use the heading `## Tools and evidence` exactly. Two bullets from a security charter show
   the level of detail:
    - "**Vulnerability evidence.** The advisory record (CVE, GHSA), the dependency graph path that proves whether the
      package ships in a production artifact, and the reachability of the vulnerable call."
    - "**No ref for a code review.** If no diff, branch, or pull request is supplied, ask for the ref."
5. **Escalation.** A routing table to other agents for out-of-scope work. Never let an agent silently ignore work
   outside its domain. When the repository has an agent that owns architecture, cross-boundary architecture decisions
   escalate to it.
6. **What You Don't Do.** Explicit constraints and boundaries.

Anti-patterns:

- Ownership overlap without disambiguation keywords
- Descriptions without trigger phrases
- A mix of advisor and builder roles
- Escalation to agents that don't exist
- Platform-specific tool lists in frontmatter

## Pattern 1: Lean orchestrator

When the agent routes requests to skills and agents, use this pattern. It runs ~30-40 lines.

```markdown
---
name: my-agent
description: "Purpose statement"
version: "1.0.0"
---

## My Agent

One-line role description. Routes to specialized skills.

## Capabilities

- **Domain A**: Brief description
- **Domain B**: Brief description

## Routing

| Domain   | Skills           |
| -------- | ---------------- |
| Domain A | skill-a, skill-b |
| Domain B | skill-c, skill-d |
```

## Pattern 2: Domain agent

When the agent owns a complex multi-phase workflow with validation gates, use this pattern. It runs ~200-400 lines.

### 5-phase structure

1. **PHASE 1: Parameter Validation.** Extract and validate inputs.
2. **PHASE 2: Context Analysis.** IF/THEN/ELSE decision routing.
3. **PHASE 3: Execution.** Invoke skills per decision path.
4. **PHASE 4: Validation.** Checkpoint gates with pass/fail criteria.
5. **PHASE 5: Error Handling.** Categorized error routing.

### Key elements

- **Decision logic**: Use IF/THEN/ELSE, not narrative prose
- **Skill invocation**: Put `**INVOKE** skill-name` in bold on its own line.
- **Checkpoints**: `✅ CHECKPOINT N` markers with pass/fail checklists
- **Skills section**: List required skills at the top of the agent body

```markdown
## PHASE 2: Context Analysis

IF request_type == "architecture_review": THEN path = well-architected

- Query IaC files
- Score WAF pillars ELSE IF request_type == "cost_optimization": THEN path = cost-analysis
- Query Azure cost data

### ✅ CHECKPOINT 2

- [ ] Query strategy determined
- [ ] Required tools identified

**IF checkpoint fails → revise strategy or request clarification**
```

## Skill integration

| Pattern | How to Reference Skills                           |
| ------- | ------------------------------------------------- |
| Lean    | List skill names in the routing table             |
| Domain  | `**INVOKE** skill-name` on its own line per phase |
| Domain  | Add a "REQUIRED SKILLS" section at the agent top  |

Don't duplicate skill content in the agent. Reference the skill by name.

**RELATED:** Use `/agentics:forge skill` when you create the skills your agent will reference.

## Runtime governance

Agents that call tools at runtime need policy enforcement, threat detection, and audit trails. Six runtime patterns
serve them:

| Pattern                   | Purpose                                                                                          |
| ------------------------- | ------------------------------------------------------------------------------------------------ |
| **Governance Policy**     | Composable, serializable allow/deny + rate-limit + content-filter object                         |
| **Intent Classification** | Pattern-based threat detection (exfiltration, prompt injection, privilege escalation) pre-flight |
| **Tool Decorator**        | `@govern(policy)` wrapper that enforces allowlist, rate limit, content filter, and logs          |
| **Trust Scoring**         | Decay-based reliability tracking for multi-agent delegation                                      |
| **Audit Trail**           | Append-only JSONL log of every governance decision                                               |
| **Framework Integration** | Wiring patterns for PydanticAI, CrewAI, OpenAI Agents SDK                                        |

Match strictness to risk:

| Level        | Controls                                        | Use Case                     |
| ------------ | ----------------------------------------------- | ---------------------------- |
| **Open**     | Audit only, no restrictions                     | Internal dev/testing         |
| **Standard** | Tool allowlist + content filters                | General production agents    |
| **Strict**   | All controls + human approval for sensitive ops | Financial, healthcare, legal |
| **Locked**   | Allowlist only, no dynamic tools, full audit    | Compliance-critical systems  |

Load [../governance/runtime-governance-patterns.md](../governance/runtime-governance-patterns.md) for full Python
implementations of each pattern, framework-specific examples, and the governance implementation checklist.

### Compliance-bearing skill routes

Validate agents that orchestrate skills with requirement traceability for compliance too. When an agent references
compliance-related skills, or its changes affect compliance-bearing skill routes:

1. Verify the agent's skill references resolve to actual skill directories on disk
2. Ensure no phantom routes exist (skills referenced in routing but not on disk)
3. Run `forge` guidance (including
   [`../governance/compliance-audit-procedures.md`](../governance/compliance-audit-procedures.md)) after a significant
   restructure of the agent

## Agent evaluation workflow

After you draft or refactor an agent, run an explicit evaluation pass before you consider the work complete. Use the
same test-first sequence as `forge` until the agent consistently routes and responds as intended. When the user defers
evaluation, for example "do not evaluate it yet", skip this pass. When the user defers evaluation, report the agent as
`not-run` (not evaluated). The user's deferral overrides this pass.

### Recommended eval sequence

1. Define happy-path, edge-case, routing, and out-of-scope scenarios for the agent.
   [Eval mode](../../SKILL.md#eval-mode) sets the scenario count.
2. Dispatch an eval runner subagent to role-play those scenarios against the agent instructions
3. Score the results with a shared rubric instead of intuition
4. Revise the agent instructions to target the weakest criteria first
5. Re-run the eval until the scores meet the agent pass bars in [Eval mode](../../SKILL.md#eval-mode)
6. Record the run apart from the agent file; the report is the record. Report a pass only from an actual run. An edit
   without a new run is never reported as passing. Where the repository defines an evaluation record, see
   [Evaluation record](forging-skills.md#3-evaluation-record) for where it is kept.

Load these references:

- [../evals/agent-eval-criteria.md](../evals/agent-eval-criteria.md) for the scoring rubric (routing, persona
  consistency, tool use, boundary quality)
- [../evals/agent-eval-runner-prompt.md](../evals/agent-eval-runner-prompt.md) for the adversarial subagent prompt
  template
- [../evals/agent-improvement-loop.md](../evals/agent-improvement-loop.md) for the full eval-to-rewrite loop

## Common mistakes

| Mistake                              | Fix                                           |
| ------------------------------------ | --------------------------------------------- |
| Agent executes instead of routing    | Move workflow logic to skills                 |
| Narrative prose instead of decisions | Convert to IF/THEN/ELSE                       |
| References to non-existent skills    | Verify skills exist before you reference them |
| Too many phases for simple routing   | Use lean pattern instead                      |
| Over-engineered front matter         | Start with required fields only               |
