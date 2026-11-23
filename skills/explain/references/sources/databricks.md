# Databricks Analytics & System Tables

## What this source contains

Databricks is the product-analytics, data-pipeline, and warehouse-telemetry layer. It complements Datadog: Datadog is
the _infra/runtime_ view. Databricks is the _product/data_ view (what users did, which experiments ran, how feature
usage evolved, where a threshold constant came from).

Table and column names depend on the warehouse. The Databricks system tables below exist in every workspace that has
them enabled. Everything else in this list is a category to look for. Find the real names with `SHOW TABLES` and
`DESCRIBE TABLE` before you query.

- **Product analytics events.** A raw event table, and often typed, deduplicated per-event models built over it (a `dbt`
  project is common). User behavior: feature invocations, clicks, accepts/rejects, submissions, client-reported errors.
- **Usage & billing events.** Tables that record what customers consumed. For cost- or volume-driven decisions.
- **Experiment / feature-flag data.** Exposure and outcome tables.
- **System tables.** `system.query.history`, `system.compute.warehouses`, `system.billing.usage`, `system.access.audit`.
  They answer "was this query expensive?", "how often did anyone run this?", "when did warehouse load spike?"
- **dbt lineage.** Models in the analytics schemas reveal what pipelines depend on a table or field. Upstream changes
  frequently motivate consumer-code changes.
- **Databricks notebooks.** Exploratory analyses engineers wrote before code changes. **Not queryable via the SQL MCP.**
  If you suspect the rationale lives in a notebook, name it as a gap.

## How to search it

Use the Databricks SQL MCP. Primary tool: `execute_sql_read_only`. If it returns a `statement_id`, poll with
`poll_sql_result` rather than run the query again.

**Orient before you query.** Probe before you trust a table or column name. The repo (a dbt project, schema docs, the
code that emits the event) often names the catalog and schema that hold the data. Then list and describe:

```sql
SHOW TABLES IN <catalog>.<schema> LIKE '*<keyword>*';
DESCRIBE TABLE <catalog>.<schema>.<table>;
```

**Time-bound every query.** These tables are huge and unconstrained scans time out. Filter on the table's event-time
column (`DESCRIBE TABLE` shows it) or, for `system.query.history`, on `start_time`. Use a window that brackets the ship
date, typically ~30 days before and after. Widen the window only for a strong reason.

**Prefer curated models over the raw table.** When the warehouse has typed, deduplicated models built over the raw event
table, query those. Raw event tables usually have duplicates and untyped JSON property columns. Find the model for an
event with `SHOW TABLES ... LIKE`. Drop to the raw table only in these cases:

- There is no curated model yet.
- You need events from inside the model's refresh lag. Deduplicate those rows on the event ID column.

`DESCRIBE TABLE` on a model usually shows event properties as typed columns and client context (client version,
platform, country) as separate columns. If a property you need is missing, check whether it exists only inside the raw
table's JSON column.

### Investigation patterns that tend to pay off

Pick the table + column combination that matches the target:

1. **Event usage trajectory.** Daily counts on the relevant event model across a ±30d window around the PR merge. A step
   function from zero to steady volume within a day or two of the merge is strong circumstantial evidence the PR
   launched the feature. A decay to zero suggests a deprecation or deletion.
2. **Guard-rail / defensive-check origin.** Distribution (median / p99 / max) of the relevant property column in the 14
   days _before_ the PR. A p99 that matches the target's threshold constant suggests the number was chosen from data.
3. **Experiment / feature-flag lookup.** Run `SHOW TABLES ... LIKE '*experiment*'` (or `'*flag*'`) to find the exposure
   table. Pull exposure counts by variant for the relevant flag key near the PR date.
4. **Query-history evidence for migrations, backfills, or perf rewrites.** `system.query.history`, filtered by
   `statement_text ILIKE '%<table_or_symbol>%'` with a tight `start_time` window, surfaces the expensive queries that
   likely motivated the change. Sort by `total_duration_ms` or aggregate `SUM(read_bytes)`, `COUNT(*)`.
5. **dbt lineage.** If the target reads from or writes into a model, the model's own git history (in this repo) often
   carries the rationale. Hand that lead back to the git investigator rather than chase it yourself.

## What good evidence looks like here

Beyond the pattern shapes above:

- An error-classifying event's count drops to near zero in the days after a defensive-code PR. The drop suggests the PR
  resolved that error class
- An exposure table row names the target's feature-flag key with a "shipped" / "concluded" decision around the PR ship
  date

## Common pitfalls

- **Instrumented ≠ caused.** An event's existence means someone cared enough to log it, not that the target code exists
  _because_ of it. Pair with a PR/commit citation from the git investigator before you claim causation.
- **Silent instrumentation changes.** A step function in event volume may mean a new event began to be recorded, not
  that user behavior changed. Check for instrumentation PRs in the same window before you read the ramp as a
  feature-launch signal.
- **Schema drift.** Event properties evolve. A column on a curated model today may not have existed when the target was
  written. Older data may carry the property only inside the raw table's JSON column.
- **Refresh lag.** Curated models are rebuilt on a schedule (often hourly or daily). For events from the last few hours,
  fall back to the raw table and deduplicate by the event ID column.
- **Warehouse-specific tables.** Experiment, feature-flag, billing, and usage tables vary between companies. A classic
  failure mode is to report a result from a table whose existence you never confirmed. Probe with `SHOW TABLES` /
  `DESCRIBE TABLE` first.
- **Retention cliff.** If the relevant window predates the table's retention or the model's creation date, that's a
  _gap_, not a null result. Name it explicitly so the synthesizer doesn't read "no results" as "no activity."
- **Notebooks aren't queryable.** The SQL MCP can't see Databricks notebooks. If you suspect the rationale lives in one,
  return a gap.

## What to return

For each relevant finding:

- Type (product event / experiment exposure / usage or billing event / system-table row / dbt model)
- Fully-qualified table name and the exact query you ran
- Time window queried
- Compact numeric summary (counts, percentiles, first/last-seen timestamps). **Don't dump raw rows.**
- Temporal correlation with the target's ship date (e.g., "first row 2024-08-15; PR #123 merged 2024-08-14")
- Relevance + strength: direct / circumstantial / weak
