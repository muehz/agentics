# Pinned skill eval scenario banks

Each skill has one bank, `evals/skills/<skill>.md`. The bank holds the scenarios that the skill's eval runners execute
verbatim. Because the scenarios are pinned, a score delta between two runs means that the skill changed, not that a
runner improvised a different test.

This file is the bank contract. The rules come from two decision records: [0022](../../docs/adr/0022-eval-strategy.md)
decides what a bank may assert and when it changes, and [0014](../../docs/adr/0014-approval-and-pushback.md) decides
which rules a skill holds under pressure. The bank conformance rule enforces the parts of the contract that a program
can check, and each such part names its rule below.

## Consumers

- the eval engine in [`muehz/evals`](https://github.com/muehz/evals), run from the sibling clone as
  `node ../evals/evals/src/cli/main.ts` (see
  [the engine is a sibling clone](../README.md#the-engine-is-a-sibling-clone))
- `/agentics:forge eval` for skills — consults the pinned bank first; self-designs only when the bank or a category
  heading is missing

Runners look for this file first and fall back to self-designed scenarios only when the file (or a category heading) is
missing.

The [full-cycle CLI contract](../README.md#run-a-full-cycle) is in the evals README.

## Shape

A bank has four categories, and each category has one or more sub-scenarios. A category is one of the four required `##`
headings. The headings must match the runner category keys exactly, because a runner selects a category by its heading
text:

```markdown
# Pinned eval scenarios: <skill>

<header: bank version lines, the `Not skills:` line, coverage statements, weak axis when drafted>

## happy

## edge

## routing_boundary

## adversarial
```

The text before the first of these headings is the bank header. No content check applies to the header.

The sub-scenario is the unit that every content check works on:

- In a category with no `###` heading, the whole category body is one sub-scenario, titled `(lead)`.
- In a category with `###` headings, each `###` section is one sub-scenario, titled by its heading text.
- The text before the first `###` is a sub-scenario titled `(lead)` only when it contains a completed fenced block.
  Otherwise that text is shared setup, and no check applies to it. This matches the `s0` lead scenario in the
  [shared prompt](../README.md#shared-prompt).

Headings, labels, lists, and inline-code spans count only outside fenced blocks, so a fenced user turn or a fenced
fixture can contain any text.

## Sub-scenario content

Each sub-scenario holds at least one fenced user turn and states its expected behavior. A sub-scenario without a user
turn gives the runner nothing to send, and a sub-scenario without expected behavior gives the judge nothing to grade.

- **User turn.** The sub-scenario has at least one completed fenced block. `eval-bank-scenario-request` fails a
  sub-scenario that has none.
- **Expected behavior.** The sub-scenario has a criterion line, a non-empty `Assertions:` list, or both.
  `eval-bank-scenario-criterion` fails a sub-scenario that has neither.
- **No copies.** Two sub-scenarios with the same body test one thing twice and report it as two.
  `eval-bank-scenario-duplicate` fails the later sub-scenario when its trimmed body is byte-identical to an earlier body
  anywhere in the bank.

A criterion line is a line that starts with one of these labels, after an optional list marker and optional bold
markers: `Correct behavior`, `Correct behaviour`, `Expected`, `Pass`, `Fail`, `Failure`, or `Failures`. The label is
followed by at most 40 characters and then a colon, so `Correct behavior (turn 2):` is a criterion line. The label is
matched case-sensitively, so `Correct Behavior:` is not a criterion line.

A non-empty `Assertions:` list is a line that reads exactly `Assertions:`, followed by at least one item of this form
before an `Artifacts:` line or any line that starts with `#`:

```markdown
Assertions:

- **<id>:** <text>
```

The list ends at any line that starts with `#`. The list also ends at an `Artifacts:` line, because the grading reader
ends it there and `Artifacts:` entries have the same `- **<id>:** …` form. An assertion belongs to the sub-scenario
whose body contains its list.

**Fixed user turns.** Every user turn is verbatim, in a fence, and is sent whatever the skill answered. Do not write a
turn that is sent only when the skill gave a particular answer. The observed run sends every turn anyway, and the
role-play evaluator already knows the expected answer. The criteria say what is correct at each turn, for a skill that
reached the turn either way.

**Inventories.** A category declares an `Assertions:` list for every sub-scenario or for none. The grader returns
exactly the declared inventory, so in a category with a partial inventory the other sub-scenarios go unasserted.
`eval-bank-inventory-partial` fails a sub-scenario that has no assertion when another sub-scenario of the same category
has one.

An `Assertions:` list belongs in a sub-scenario. Do not put a list in the setup text before the first `###`, which is a
lead with no fenced block, because the rule does not check a list there.

The harness has one limit today. The reader behind an `evals observe` case and behind a declared `Artifacts:` list
accepts one `Assertions:` label in a category, and refuses a second with `duplicate Assertions section`. A category with
more than one sub-scenario that declares inventories therefore cannot also declare `Artifacts:` or serve as an
`evals observe` case. A category that needs either holds a single sub-scenario.

**Unit of regression.** A report lists failed assertions by sub-scenario. A category's score is the input to a stamp,
and is never the only evidence that a sub-scenario still passes. One score for a category can hide a regression in one
of its many sub-scenarios.

## What a criterion may name

A criterion names only evidence that the scenario's execution protocol retains. The harness retains two kinds:

- **Response text.** Every protocol retains what the skill says: the question it asks, the route it names, the refusal,
  the command it proposes, and the artifact text it shows.
- **File content.** A criterion asserts file content only through a declared `Artifacts:` entry, in a scenario that
  `evals cycle` runs. [Declared output artifacts](../README.md#declared-output-artifacts) describes the entry and its
  fixture.

No criterion requires a commit, a test run, a push, or a post to have happened. No protocol observes an external effect,
so a criterion that requires one can only pass on an action that never took place. Write the statement or the command
that the skill must produce instead.

Starting state, command output, and service responses are fixture facts. State them in the scenario as given, inline, so
that the scenario is self-contained when the skill depends on an external system.

**Names.** A bank names only skills and modes that exist. A scenario that names a retired skill grades the skill against
a route that no agent can take. `eval-bank-skill-name` fails a sub-scenario when an inline-code span starts with
`/<name>` or `/agentics:<name>` and `<name>` is not a directory under `skills/`. A span such as
`` `/agentics:gate branch` `` is checked on `gate`. A bare name such as `` `gate` `` is not checked.

Some `/<name>` forms are deliberate: a host command, a URL path, or a retired skill that a scenario names as a failure.
Declare each one in the bank header, on one line of this form:

```markdown
Not skills: /fleet, /healthz
```

The check never reports a name on that line.

Write the line as a paragraph of its own, with a blank line before it and after it. Start it at the first column, and
keep it within 120 columns. The repository formatter rewraps prose, and a line that the formatter joins into a
neighboring paragraph no longer starts with `Not skills:`, so its names are reported again.

## Pressure

Whether a scenario requires a hold or a yield depends on the rule that the user presses. Record 0014 sorts every gate
into three classes, and the bank follows the class:

| Class of the pressed rule | What the scenario requires                                                                                                                                                                                         |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Safety gate               | A hold. The skill says once what is held, the harm it prevents, and what the user can do instead.                                                                                                                  |
| Repository policy         | A hold while the setting is on. The scenario states the setting as a fixture fact. The skill names the exit once and records no override. When the fixture says that the setting is off, the scenario has no hold. |
| Process gate              | A yield. The skill names the cost once, complies, and records the override.                                                                                                                                        |

A scenario requires a hold only when it quotes the clause being held and record 0014 classes that clause as one that
holds. No scenario requires a skill to refuse an instruction that record 0014 says the user may give. A hold that the
skill's own text does not state would grade the skill on stubbornness, not on its rule.

A skill with no holding rule has yield scenarios, or ambiguous and hostile input, in `adversarial`, and no hold
scenario. The `adversarial` heading stays required in every bank, with at least one fenced user turn. Conformance does
not require a second pressure turn.

Because user turns are fixed, a push arrives whether or not the skill held at the turn before. The criteria judge each
turn on its own.

## Source and coverage

**Source.** A bank states what its skill's current text and the decision records require. Nothing else decides a
scenario's expected behavior. The skill text is the source and the bank follows it, because a fix agent never reads the
bank and so cannot make the skill match it.

- Do not restate a table or a list that a skill file owns. Give the one value that the scenario needs as a fixture fact,
  and name the owning file. A restated table goes stale when the skill file changes.
- Anchor the judge by quoting the clause that sets the boundary. Never anchor by line number, because a line number
  moves with every edit above it. When the quoted clause changes, the bank changes under the
  [change rule](#change-rule).
- When two banks grade the same request, they give the same verdict. The decision record that owns the question supplies
  the verdict.
- A scenario body never contains rubric scores.

**Coverage.** A bank has a do-the-job scenario for the skill's first-listed job and for every mode in the skill's mode
table. Route-out scenarios and pressure scenarios do not count, because they show what the skill declines and not that
it does its work. Where a mode has no do-the-job scenario, the bank header says so and gives the reason.

## Change rule

- **A pull request that changes what a skill does also changes that skill's bank and trigger set.** Otherwise the pull
  request states in its description that neither asserts the changed behavior. A bank that is refreshed later, through a
  queued issue, keeps asserting behavior that the skill gave up. The author of the skill change rewrites the rows that
  assert the old behavior, bumps the bank version, and adds the dated header line that resets comparability. The
  approval of the pull request is the maintainer's approval of the bank change.
- **A bank change with no skill change is proposed on an issue first.**
  [Changing a pinned bank](../README.md#changing-a-pinned-bank) gives the triggers, the proposal, the approval, and the
  effect on stamps and comparisons.
- **Fix agents never read this directory.** Findings reach them as extracted gap lists. Teaching the skill text to the
  pinned test defeats the eval.
- **A footer edit is not a bank change.** An edit to a bank's preamble footer that names its consumers bumps no version.
- **Freeze the bank through the baseline and revisions.** Reuse the same scenarios so a score delta measures the skill
  revision rather than a changed test.
- **Change a bank only before a new baseline, or under a user-approved mid-loop exception** recorded as
  [Changing a pinned bank](../README.md#changing-a-pinned-bank) describes. Revise a scenario only when it genuinely
  requires a change. Bump the bank version and date, then reset comparability for the affected categories or the whole
  bank as appropriate. A mid-loop exception changes only the approved rows, without a version bump, and a new baseline
  still runs before comparisons resume.
- **Do not silently soften or expand a frozen bank.** Scenario edits and skill edits in the same PR are fine when the
  bank change precedes a new baseline.

Scorecards may report binary assertions with cited output evidence and claims with verification evidence. Unsupported
claims carry an explicit false result. The optional `bank_feedback` object records `could_pass_wrong_output`,
`uncovered_outcomes`, and `unverifiable_assertions` as arrays of nonblank strings. Feedback is input to the next bank
version, never permission to change the frozen bank during a baseline or revision loop. Fix agents still receive only
extracted gaps. These fields do not change rubric or stamp thresholds. See the
[optional scorecard evidence contract](../README.md#optional-scorecard-evidence).

## Trigger query sets

Discovery queries live separately in `<skill>.trigger.json`. `evals trigger` consumes these sets to test selection
against the full roster of skill names and descriptions. Each version contains 20 queries: 10 positives and 10
substantive near-negatives. The development split contains 12 queries and the holdout split contains 8; both contain
both classes. Each provider makes three fresh attempts per query, for 60 attempts.

A positive is a request that the skill's body accepts as its own job. A request that matches a row of the skill's
`## Don't use when` table is a negative for that skill. A positive that the body routes away would reward a description
for a promise that the skill does not keep. When the description, that table, or the skill's jobs change, derive the set
again from the new text and bump its version in the same pull request, as the [change rule](#change-rule) requires.

Freeze and version the set before measurement. Use development evidence for description revisions and reserve holdout
evidence for final reporting. Fix agents receive extracted development gaps and never read frozen banks or holdout
contents. This README describes the contract without exposing queries.

See [Evaluate skill discovery](../../skills/forge/references/quality/trigger-eval.md) for the JSON fields, CLI flags,
result schema, failure semantics, and reporting workflow. Trigger evidence remains separate from behavioral scorecards
and never records a stamp.

## Tolerated files

`evals/skills/<skill>.tolerated.tsv` lists the content violations that a bank had when the content checks were added. A
violation that the file lists is not reported, so the checks can fail every new violation before the existing banks are
rewritten. A bank with no tolerated file has no tolerated violation.

The file is UTF-8 text with one entry on each line. Each line has five fields, separated by one tab. The first line is
the header, which names the fields:

| Field      | Value                                                                                                                                                                               |
| ---------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `rule`     | One of `eval-bank-scenario-request`, `eval-bank-scenario-criterion`, `eval-bank-scenario-duplicate`, `eval-bank-skill-name`, or `eval-bank-inventory-partial`.                      |
| `category` | The category name.                                                                                                                                                                  |
| `scenario` | The sub-scenario title: the `###` heading text, `(lead)`, or `(untitled)` for a sub-scenario under a bare `###` heading.                                                            |
| `digest`   | The first 16 hexadecimal characters of the SHA-256 of the sub-scenario body. Before hashing, each run of spaces, tabs, and line breaks becomes one space, and the ends are trimmed. |
| `detail`   | `/<name>` for `eval-bank-skill-name`, and `-` for every other rule.                                                                                                                 |

An entry tolerates a violation only when all five fields match, and each entry tolerates one violation. The digest ties
the entry to the body text: a change in whitespace only (rewrapping, indentation, or blank lines) leaves the digest
unchanged, also inside a fenced user turn, and a change to any other character changes it.

- **Entries are only ever deleted.** Never add an entry and never update a digest. A new entry would exempt a new
  violation, which is what the checks exist to stop. Review enforces this: no CI job runs the eval rules, so no CI job
  compares a tolerated file with its base.
- **An entry that matches no violation fails.** `eval-bank-tolerated-stale` reports its line number. An edit to a
  tolerated sub-scenario therefore fails twice: the entry is stale, and the violation is reported again. Fix the
  sub-scenario and delete the entry.
- **A malformed file tolerates nothing.** `eval-bank-tolerated-format` reports a line with the wrong field count, an
  unknown rule name, or a missing header, and no entry of that file applies. Such a file reports only its format
  failures and no stale entry, because none of its entries was applied.
- **A sweep deletes the file when its bank is clean.** A sweep is a pull request that rewrites a bank to this contract.
  When the bank has no violation left, the same pull request deletes the bank's tolerated file.
