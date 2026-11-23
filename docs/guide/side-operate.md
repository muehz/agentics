# Operate

Run the software after it ships: incidents, the runbooks that prevent their repeats, and sustained improvement of the
metrics that matter. This is a side page that picks up where [06. Ship](06-ship.md) ends.

## When production is down, say so plainly

```text
Checkout is down in production, customers are affected. Run incident response.
```

Why this phrasing works: "down", "production", "customers affected". The impact statement is what gives
[`ops`](../../skills/ops/SKILL.md) response mode precedence over everything else. The session announces the route and
runs it; an active outage never waits on a consent question. GitHub artifacts (tracking issue, status updates) are
drafted, and nothing posts without your consent.

## Close the loop afterward

```text
Write the blameless postmortem for yesterday's checkout incident.
```

```text
Author a runbook from that postmortem's action items.
```

Why this phrasing works: the `ops` skill's `postmortem` mode and its `author from-postmortem` mode are a designed chain.
The postmortem's action items become the skeleton of the runbook that makes the next occurrence boring. "Blameless"
isn't decoration; it's the postmortem contract. Runbooks then stay honest through "audit the deploy runbook against the
current infra" and stay rehearsed through "drill me on it."

## Improve a metric until it hits the target

```text
Make cold start 50% faster. Build a measurement harness first, then iterate: one hypothesis at a time, keep or revert by the numbers.
```

Why this phrasing works: a target plus "iterate by the numbers" is [`hillclimb`](../../skills/hillclimb/SKILL.md)'s
trigger. It freezes a measurement harness, then loops one mechanism-named hypothesis per iteration, accepting only wins
that beat noise. This is the loop for sustained improvement; a single perf bug is `diagnose`. And the target is the
target: the skill refuses to relax the predicate to declare victory.

**Pitfall:** during an active outage, don't dress the report in technical vocabulary and split it yourself. "Fix the
500s on /api/checkout now, we'll do process later" invites routing to treat it as a backend bug. State the impact
("production, customers affected") and let the incident workflow own the whole report. Root-causing happens inside it,
with the comms and severity framing an outage actually needs.

Back to the main path: [06. Ship](06-ship.md), or the [index](README.md).
