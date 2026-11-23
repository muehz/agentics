# Evaluate skill discovery

Measure whether a router selects a skill from the full roster of skill names and descriptions. Keep discovery evidence
separate from the behavioral scorecards that `forge eval` produces.

To measure discovery, dispatch a fresh subagent per query. Give it the roster of skill names and descriptions and one
query. Accept only a roster name or `NONE`. When the host allows, run three fresh attempts per query. Report the hit
rate separately for positives and near-negatives.

## Freeze the query set

Before measurement, have an independent author create the trigger query set. Keep it with the skill's pinned scenario
bank, where [a trigger set lives](../evals/README.md#where-a-bank-and-a-trigger-set-live). Use 20 realistic queries: 10
positives and 10 substantive near-negatives that belong to adjacent work. Avoid trivial unrelated requests that make the
negative class easy to reject.

Assign 12 queries to `development` and 8 to `holdout`, a 60/40 split. Include both positive and negative queries in each
split. Before the baseline, freeze the text, labels, split assignments, and positive integer `version`. Before a changed
set starts a new baseline, increment the version. Do not compare revisions measured against different sets as though
only the description changed.

Before you freeze the set or run the baseline, have a separate reviewer review it. Have the reviewer work in a clean
context and make no edits. Check labels, realistic overlap with adjacent skills, uniqueness, class balance, and split
assignments. Give the skill owner only extracted gaps. Keep holdout contents with the independent evaluator.

Use a JSON object with `version` and `queries`. Each query requires a unique `id`, a nonempty `query`, an explicit
boolean `should_trigger`, and a `split` of `development` or `holdout`. IDs start with a letter or digit and contain only
letters, digits, periods, underscores, and hyphens. Hold exactly 10 positives across the whole set. An equal class
balance within each split is not required.

Keep fix agents away from frozen banks and holdout contents, including raw responses that repeat queries. Give those
agents extracted behavioral gaps from development evidence only. The independent evaluator owns the query set and
retains holdout results for the final report.

## Classify the results

Interpret each response against the target skill:

- `hit`: a positive query selected the target.
- `miss`: a positive query selected another skill or `NONE`.
- `false_positive`: a negative query selected the target.
- `correct_negative`: a negative query selected another skill or `NONE`.
- `failed`: the attempt failed to execute or return a valid selection.

Check failures separately from coverage. A miss or false positive is a measured routing result, not an execution
failure.

## Revise from development evidence

Compare development results by provider across description revisions. Use the extracted gaps to tighten the
[description](skill-description-optimizer.md). Then measure the revision with the same frozen set and fresh attempts.
Retain the new attempts separately from earlier runs. Keep the roster stable where possible. Record any roster changes
that could affect routing.

Do not use holdout evidence to choose description wording. After you choose the description from development evidence,
report holdout results separately for each provider. In that report, include failures, coverage, the query-set version,
and the evaluated revision. Do not count failed attempts as correct selections or hide them in aggregate rates.

Trigger results measure discovery only. They never support a `passing` status: that needs the separate behavioral
evaluation, which supplies the required measured evidence.
