# Interface design

Use this parallel sub-agent pattern when the design-twice floor in the skill body fires. That floor covers any
non-trivial interface. Use it also when the user asks to explore alternatives for a chosen deepening candidate or a
write-time module. This pattern is based on "Design It Twice" (Ousterhout). Your first idea is unlikely to be the best,
and at design time it's still free to throw away. Vocabulary per [language.md](language.md): **module**, **interface**,
**seam**, **adapter**, and **leverage**.

## Process

### 1. Frame the problem space

Before you spawn sub-agents, write a user-facing explanation of the module's problem space:

- The constraints any interface would need to satisfy: what callers must be able to do, what invariants must hold, and
  what error modes exist
- The dependencies it would rely on, and which category they fall into (see [deepening.md](deepening.md))
- A rough code sketch that makes the constraints concrete, not a proposal

Show this to the user. Then immediately go on to Step 2. The user reads and thinks while the sub-agents work in
parallel.

### 2. Spawn sub-agents

Spawn two to four sub-agents in parallel with this session's native dispatch path (see
[surface dispatch](../../routing/references/surface-dispatch.md)), one for each constraint below. Agents 1 and 2 always
run, and two meet the design-it-twice floor. Add Agent 3 when the module has several callers with different needs, and
Agent 4 when a dependency crosses a seam. Each must produce a **radically different** interface for the module. A
sub-agent starts no sub-agents of its own. Where the session has no dispatch path, draft one design per constraint
yourself, one at a time.

Prompt each sub-agent with a separate technical brief. For an existing candidate, include file paths and coupling
details. For a module not yet written, include where it will live and the callers it serves. For both, include the
dependency category from [deepening.md](deepening.md) and what sits behind the seam. The brief is independent of Step
1's user-facing explanation. Give each agent a different design constraint:

- Agent 1: "Minimize the interface. Aim for 1–3 entry points max. Maximise leverage per entry point."
- Agent 2: "Maximise flexibility. Support many use cases and extension."
- Agent 3: "Optimise for the most common caller. Make the default case trivial."
- Agent 4 (if applicable): "Design around ports & adapters for cross-seam dependencies."

Include both [language.md](language.md) vocabulary and the resolved glossary's vocabulary in the brief so each sub-agent
names things consistently with both.

Each sub-agent outputs:

1. Interface (types, methods, params, plus invariants, ordering, and error modes)
2. Usage example that shows how callers use it
3. What the implementation hides behind the seam
4. Dependency strategy and adapters (see [deepening.md](deepening.md))
5. Trade-offs: where leverage is high and where it's thin

### 3. Present and compare

Before you present the designs, screen each returned design against [design-red-flags.md](design-red-flags.md). Revise a
flagged design, or present it and name the flag. Never present it as clean.

Present designs one at a time so the user can absorb each one. Then compare them in prose. Contrast by **depth**
(leverage at the interface), **locality** (where change concentrates), and **seam placement**.

Then recommend. Name the strongest design. Say why. If elements from different designs would combine well, propose a
hybrid. Be opinionated: the user wants a clear recommendation, not a menu.
