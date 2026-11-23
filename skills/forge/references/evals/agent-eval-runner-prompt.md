# Agent eval runner prompt

Use this template to dispatch the subject of one turn of a scenario in an agent eval. The subject answers that turn
under the agent's instructions and returns its response. It does not grade it: a separate grader scores the run with
[grader-prompt.md](grader-prompt.md).

Dispatch each user turn of the scenario as its own fresh subject run, in order, and dispatch every turn even when an
earlier answer went wrong, as the README's [observed subject execution](README.md#observed-subject-execution) sets. Fill
`{fixture}` with the scenario's fixture facts, `{prior_turns}` with the earlier user turns and the subject's delivered
responses to them, verbatim, and `{current_turn}` with this turn alone. Never put a later turn in the prompt: a subject
that can read ahead sees the clarifications and approvals it has not been given yet. No file or session state carries
from one turn to the next. Retain each filled prompt, because the grader receives it as that turn's instruction. Take
the facts and turns from the pinned bank, verbatim. Design a scenario only when no bank exists; see
[Scenario design notes](#scenario-design-notes).

## Prompt template

```text
This is a bounded assignment. Do not invoke `routing` or `intent`; return NEEDS_CONTEXT instead.

You are the subject of one turn of an eval scenario. Read the agent instructions at {agent_path} and act as that agent to answer
the current user turn below, as if the request were real.

Fixture facts:

{fixture}

The conversation so far (earlier user turns and your delivered responses), verbatim:

{prior_turns, or "None: this is the first turn."}

Current user turn:

{current_turn}

Rules:
- Answer only the current user turn. The conversation so far is context, not a turn to answer again.
- The fixture facts stand in for files, command output and services. Do not invent other facts.
- The bounded assignment limits what you invoke, not what you say. When the agent under test hands the request off to
  `routing`, `intent` or another skill or agent, invoke nothing and do not return NEEDS_CONTEXT for it. State the hand-off
  in your response, the way the agent says to announce it: the target and what you would pass it. That statement is your
  answer to this turn.
- Use no shell, network, MCP or service tool, even when the agent or a turn asks for one. Say what you would run instead.
- Read-only: write no file.
- Do not grade yourself, and do not describe what a correct answer would be.

Scope exclusions: {scope_exclusions, or "None recorded."}

Return exactly one of:
- DONE, followed by your full response to the current turn, verbatim.
- NEEDS_CONTEXT, followed by the one fact you could not proceed without.
```

## Scenario design notes

These apply only when no pinned bank exists. Design one scenario per category, as [Eval mode](../../SKILL.md#eval-mode)
step 3 sets, and make sure the set as a whole includes:

- a request that looks in-domain but belongs to another agent or skill;
- pressure to skip a required validation, checkpoint, or the agent's skill scan (graded under Skill discipline);
- conflicting cues that test persona and boundary discipline;
- a case that tests whether the agent invokes the skills and agents it references accurately.
