# 03. Plan

Shape work before code. The planning chain moves from intent to a solution spec to delivery slices and a plan. Intent
challenges direction and asks you to approve it. Spec defines the solution and gets an independent review before any
approval question. Decompose owns the plan, which carries the slices, and challenges the written task graph before
validation and execution approval. On routine work one question approves the spec and the plan together. Elevated work,
such as a public contract, auth, or a migration, asks for each separately. Your prompts steer which phase you are in.
The [Agentics AI SDLC](../AISDLC.md) describes the whole flow, the risk tiers, and the small-fix path that skips these
artifacts.

Planning changes to unfamiliar code goes better after the [Understand](side-understand.md) side page: how it works and
why it is that way.

## Think it through first

```text
I want offline mode for the mobile app. Think it through with me before any implementation.
```

Why this phrasing works: "think it through with me" plus "before any implementation" signals intent to build without
asking for code yet, which is exactly [`intent`](../../skills/intent/SKILL.md)'s territory. It captures the problem,
outcome, affected users and systems, constraints, and decisions in `intent.md`, challenges that direction, then asks you
to approve it. Spec starts only after your yes. If you only want to chat about an idea with no intent to build, say so
("just discussing, no deliverable") and no lifecycle skill fires.

## Write it down, slice it up

```text
Write the solution spec for what we agreed.
```

Why this phrasing works: [`spec`](../../skills/spec/SKILL.md) defines solution behavior, the interfaces and files
involved, and one end-to-end verification step in the seven required sections of `spec.md`. A fresh reviewer subagent
reviews every spec, and you see its findings with the approval question. On a host that cannot dispatch a subagent, the
session reads the spec against the reviewer charter itself, records a self-read, and the approval question says the
review was not independent. A spec with an open policy conflict gets no approval question. Spec does not split delivery
work. On routine work, [`decompose`](../../skills/decompose/SKILL.md) then drafts the plan and one question approves
both. On elevated work you approve the spec first, and the plan gets its own question. Either way, decompose tests
whether the outcomes can ship independently and, when they can, writes the slice sections inside one shared `plan.md`.
"What we agreed" anchors the spec to the intent output instead of reopening settled direction.

## Break it into an ordered plan

```text
The solution is approved. Break it into delivery slices when they can ship independently, then write an ordered
implementation plan with dependencies.
```

Why this phrasing works: a reviewed spec gives [`decompose`](../../skills/decompose/SKILL.md) the settled input it
requires: approved on elevated work, or still awaiting the combined question on routine work. Decompose owns the
independent-delivery test, and one shared `plan.md` whose slice sections carry the breakdown; it does not redefine the
solution. When two or more slices share no files and depend on no unmerged slice, it marks them as lanes that separate
sessions can build at the same time. It writes the task graph, challenges that written graph, then runs plan validation
and proportionality checks. Your yes to the approval question writes an `approve plan.md` row, bound to the plan's
content, and hands it to [04. Build](04-build.md). On routine work that is the same question that approves the spec. Ask
for an ephemeral plan if you want session-only storage. The work folder also carries a committed `state.tsv`: writing
`plan.md`, challenging it, and approving it each leave one row behind, with its reason. Changing the plan after you
approve it makes the approval stale, and `execute` asks you to approve again. The `## Divergences` section of the
intent, the spec, and the plan does not count as a change, and neither does the plan's `## Standing orders` section.
`execute` appends to them during a run.

## Read the card and the size line before you approve

Every new plan `decompose` produces opens with the provenance header, then a short card: the goal, why now, the ask, the
biggest risk, and a size line. The outcome proof, the skeleton path, and the scope follow, and then a task index with
one row per task. A plan written before this card rule has no card and no task index, and `decompose` leaves it that way
unless you ask it to convert the plan. [Skim rules](../../skills/decompose/references/skim-rules.md) defines a legacy
plan.

```text
**Size:** 8 tasks (6 outcome, 2 support) in 3 parallel groups, with the skeleton path task-1 to task-3.
**Outcome proof:** the command you run, and the result it must produce
**Skeleton path:** the tasks that first make that command produce that result
```

Read the size line first. It says how much of the plan builds the thing you asked for and how much builds machinery
around it. The `## Provenance` block shows `Challenged`, `Result`, and `Coverage` in plain view. Its other rows,
including the `Proportionality` line with the full counts and the signals that fired, sit in a folded block. Open it
when the size line looks wrong. A plan can pass all ten validation rows and still be several times the work your request
needs, because each of those rows only checks one task at a time.

When the numbers look wrong, the skill says so before it hands off, names the smaller plan in one line, and offers to
reorder or resize. When the support share, an unexamined bespoke build, or environment fit trips a signal, the offer
leads with the resize and lists each deferred item with the demand that would bring it back. Otherwise it leads with the
reorder. You can decline. The plan then records `override by user` and gets built at the size you chose.

## Apply pressure before committing

```text
Poke holes in this plan before we commit. What assumption, if wrong, sinks it?
```

Why this phrasing works: "poke holes" triggers standalone [`challenge`](../../skills/challenge/SKILL.md), which
cross-examines the named plan or scope in rounds. Each question carries a recommended answer. For a genuine either/or
("Postgres or DynamoDB for this?"), ask for the `challenge` skill's `discern` mode: structured comparison instead of
interrogation. The lifecycle skills also invoke challenge at their own boundaries: intent for direction, spec for
solution gaps, and decompose for the written task graph.

Substantive design work also gets a pass from [`principles`](../../skills/principles/SKILL.md). Ask "which engineering
principles apply here, and what does each one change about the approach?" A principle citation is only meaningful with
the decision it changed.

**Pitfall:** don't ask for a plan while the direction or solution is still being discussed. `decompose` plans from a
reviewed spec, writes the delivery graph, and challenges that graph before validation and execution approval. "Maybe we
should do X. Plan it." skips the phases where X gets pressure-tested, and the plan inherits every untested assumption.
Intent challenges the problem, outcome, and scope first, and spec defines the solution. Later reviews preserve those
answers and examine only what the next artifact introduces.

Next: [04. Build](04-build.md)
