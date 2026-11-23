# Source playbooks

Why mode spawns one investigator for each available evidence category. Each investigator reads one playbook below. A
playbook is an example for a common MCP. Adapt it for another MCP in the same category. Two categories have a CLI
degrade path and never depend on an MCP. Source control always runs on git and the `gh` CLI. The issue tracker runs on
`gh issue` / `gh api` when the project tracks issues on GitHub (see `linear.md`).

| Category                     | Playbook                                               | Example MCP it documents                                                     |
| ---------------------------- | ------------------------------------------------------ | ---------------------------------------------------------------------------- |
| Source control history       | [`code-archaeology.md`](./sources/code-archaeology.md) | git, `gh` (no MCP needed)                                                    |
| Issue / ticket tracker       | [`linear.md`](./sources/linear.md)                     | Linear (adapt for Jira, Plane, Shortcut; GitHub Issues degrades to `gh` CLI) |
| Long-form documents          | [`notion.md`](./sources/notion.md)                     | Notion (adapt for Confluence, Google Docs, Coda)                             |
| Real-time team chat          | [`slack.md`](./sources/slack.md)                       | Slack (adapt for Discord, Microsoft Teams, Mattermost)                       |
| Infrastructure observability | [`datadog.md`](./sources/datadog.md)                   | Datadog (adapt for New Relic, Honeycomb, Grafana, Splunk)                    |
| Error / exception tracking   | [`sentry.md`](./sources/sentry.md)                     | Sentry (adapt for Rollbar, Bugsnag, Airbrake)                                |
| Product analytics warehouse  | [`databricks.md`](./sources/databricks.md)             | Databricks SQL (adapt for Snowflake, BigQuery, ClickHouse, dbt)              |

Cross-cutting:

- [`incident-postmortem.md`](./sources/incident-postmortem.md). If the target code looks defensive (null checks, retry,
  timeout, rate limit, feature flag, egress guard, OOM handler), add it.

## Evidence categories

The question alone cannot show which system holds the answer, so a full search queries every available category.
Enumerate what MCP tools the host exposes in this session (on Claude Code, tools named `mcp__<server>__<tool>`),
classify each server into one category by its name, tool names, and instructions, then apply the last column. Never add
server configuration to the repository.

| #   | Category                     | Example systems                 | Uniquely surfaces                                              | No MCP configured                                                                                                                                                                   |
| --- | ---------------------------- | ------------------------------- | -------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1   | Source control history       | git, `gh` pull requests         | Rationale captured during review                               | Always available through git and `gh`                                                                                                                                               |
| 2   | Issue / ticket tracker       | Linear, Jira, GitHub Issues     | The product or business forcing function                       | A bare `#NNNN` that fits this repository's numbering is a GitHub reference: use `gh issue`. A letter-prefixed or external ID (`PROJ-412`) is unreachable: record it as a named gap. |
| 3   | Long-form documents          | Notion, Confluence, Google Docs | PRDs, decision records, alternatives written before code       | A gap                                                                                                                                                                               |
| 4   | Real-time team chat          | Slack, Teams, Discord           | Deliberation that never reached a document                     | A gap                                                                                                                                                                               |
| 5   | Infrastructure observability | Datadog, Grafana, Honeycomb     | Runtime reality: monitors, thresholds, incident timelines      | A gap                                                                                                                                                                               |
| 6   | Error / exception tracking   | Sentry, Rollbar, Bugsnag        | The exceptions that motivated defensive code                   | A gap                                                                                                                                                                               |
| 7   | Product analytics warehouse  | Databricks, Snowflake, BigQuery | User behavior, experiments, where a threshold number came from | A gap                                                                                                                                                                               |

## Skipped categories

A category is left out only for one of three reasons, each written into **Sources Consulted**:

- **No MCP and no CLI path.** A gap, not a choice: "Team chat: skipped, no matching MCP; the conversational record was
  not searchable."
- **Provably irrelevant**, not probably: "Error tracking: skipped, the target is a build-time script with no runtime
  path." "It's feature code, error tracking won't have anything" does not qualify. Run the search and let the null
  result speak.
- **An unreachable ticket ID**, per row 2: "Ticket ENG-1201: not searched, no tracker MCP; the external ticket was not
  reachable from here."

## Why-mode report and failure modes

### Output section criteria

- **The Question.** One or two sentences.
- **The Code in Question.** Paths, line ranges, and key symbols.
- **What We Found.** `[Direct]` and `[Supported]` claims, each with a citation.
- **What We Can Reasonably Infer.** `[Inferred]` claims with the inference chain visible and hedged wording.
- **Competing Hypotheses.** Only when the evidence fits several stories: each hypothesis, the evidence for it, and the
  evidence against it.
- **What We Don't Know.** Specific gaps: what was searched, with which queries, and what was not searchable. On thin git
  history, name each empty search, pickaxe queries (`git log -S`, `git log -G`) included. When the record is silent,
  name a plausible next contact, such as the author of the last relevant commit or the pull request's reviewer.
- **Sources Consulted.** The coverage map, one line per category:
  `- <Source>: <what was searched>. <found | no relevant results | skipped: reason | skipped at user direction | not searched: the anchor answered the question>.`
- **Confidence Summary.** One or two sentences.

### Failure modes to avoid

- **Confident storytelling.** An uncited bullet belongs in **What We Can Reasonably Infer** or **Competing Hypotheses**,
  never in **What We Found**.
- **Recency bias.** The current shape built up over many decisions. Trace back past the last commit.
- **A report with no gaps.** It is suspicious, not strong.
