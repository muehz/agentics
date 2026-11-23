# Automated Source Detection Reference

Split from [source-detection.md](source-detection.md). Defines detection rules for Copilot Review, Checkmarx One, GitHub
Code Security, GitHub Code Quality, Checks, and Bot Review.

## Copilot Review

### Identity

- **Review author:** Use `pull_request_read` method `get_reviews` first to identify Copilot-authored reviews. The author
  passes the [bot gate](source-detection.md#bot-gate-layer-1-structural) and its login is exactly one of Copilot's two
  forms: `copilot-pull-request-reviewer[bot]` (GraphQL, and the CLI fallback below) or `Copilot` (REST). No other login
  is Copilot, however much of the word it contains.
- **Review type:** Pull request review with inline thread comments, plus findings Copilot **suppressed**. Copilot keeps
  a suppressed finding out of the threads. It lists the finding only inside the review body, under
  `<details><summary>Review details</summary>` as `### Suppressed comments (N)`. Each suppressed finding has a
  `**path:line**` heading, one bullet of finding text, and a code fence.
- **Detection method:** Call `pull_request_read` method `get_reviews`. Identify the Copilot review IDs. Then call
  `get_review_comments`, filtered to those reviews.

### Detection steps

1. Fetch all reviews with `pull_request_read` (method: `get_reviews`).
2. Identify reviews that Copilot authored: an author that passes the bot gate with one of the two logins above.
3. Fetch all review threads with `pull_request_read` (method: `get_review_comments`, paginate with `perPage: 100`).
4. Filter the threads to only those that belong to Copilot-authored reviews.
5. Filter for threads where `isResolved` is `false`.
6. Confirm at least one unresolved Copilot thread exists.
7. Read each Copilot review's **body**. Locate the `Review details` block and, inside it, `### Suppressed comments (N)`.
   Parse every `**path:line**` heading and the bullet beneath it into a finding with `threadId: none`,
   `commentId: none`, and `suppressed: true`.
8. A suppressed finding has no thread to resolve, so it is never "already handled". Include every one from the most
   recent Copilot review. If a later review round posted a suppressed finding as a real thread, drop the suppressed
   copy. The thread is the finding.

> **Why filter by review author?** Human reviewers and other bots may also have unresolved threads. It would be wrong to
> triage them here. **Reopened threads.** A thread that was resolved and later reopened has `isResolved == false`. The
> filter catches it, so triage treats it as a regular unresolved thread. The orchestrator does not tell "never resolved"
> apart from "was resolved, then reopened". Replies are drafted fresh against the current thread state: the current
> comment, not the original. GitHub's review-comment API does not expose resolution history without a separate
> timeline-events query. That query is out of scope on purpose.
>
> **Human replies.** A human reply inside a Copilot thread leaves the thread with Copilot Review, because the root
> comment's author decides the source. The subagent reads the reply as context. Only a reply whose author has write
> access to the repository can change the disposition ([subagent-contract.md](subagent-contract.md#quoted-source-data)).
> This holds for every thread source below.

### Edge cases

| Scenario                     | Handling                                                                                                   |
| ---------------------------- | ---------------------------------------------------------------------------------------------------------- |
| All threads already resolved | Report "No unresolved Copilot review threads". Skip this source.                                           |
| `isOutdated` = true          | Include in triage. The code may have moved, but the finding may still apply.                               |
| No reviews at all            | Skip this source                                                                                           |
| Suppressed comments only     | Triage them, because this is still a Copilot source. The detection line reads `(0 threads, N suppressed)`. |
| Multiple review rounds       | Include all unresolved threads regardless of which review round posted them                                |

### Data to pass to subagent

Pass the complete unresolved thread data with these fields: `threadId`, root `commentId`, `path`, `line`, the comment
body, every reply with its author and `author_can_write`, and the `isOutdated` flag. Pass suppressed findings in the
same list, with `threadId: none`, `commentId: none`, and `suppressed: true`. The subagent validates them exactly as it
validates threads.

## Checkmarx One

### Identity

- **Detection key:** the `<!--new issues section-->` body marker **and** an author the repository declares as its
  Checkmarx scanner account. Checkmarx One's PR-decoration identity can be a GitHub App, a machine user, or a PAT-backed
  account that whoever set up the integration chose, so no login is built in: `checkmarx-one[bot]` is a common default,
  not a contract. The repository declares its accounts in the `checkmarx_accounts` list of `.triage.yml`, read from the
  default branch (see "Repository configuration comes from the default branch" above):

    ```yaml
    checkmarx_accounts: ["checkmarx-one[bot]"]
    ```

- **Comment type:** Top-level PR issue comment (not a review thread)
- **Detection method:** `pull_request_read` (method: `get_comments`) or `issue_read` (method: `get_comments`)

### Detection steps

1. Fetch all PR comments (paginate until exhausted).
2. Filter to comments whose **body contains the `<!--new issues section-->` HTML marker**.
3. Keep only comments whose `user.login` is in `checkmarx_accounts`. Each marker comment from any other author is an
   `unverified` result in the Phase 1 detection-state record: it is not Checkmarx signal, triage acts on none of it, and
   the Detection notes name its author and say that a scanner account can be declared in `.triage.yml` on the default
   branch. With no `checkmarx_accounts` key, every marker comment is `unverified`.
4. If multiple comments remain, select the latest by `created_at` (scan reruns create new comments).
5. Locate the `<details><summary><b>New Issues (N)</b></summary>` block.
6. If N is 0 or the table is empty, do these steps:
    - Report "No new Checkmarx findings".
    - Skip this source.

> **Checkmarx is keyed on its marker and a declared author.** Every other adapter keys on a built-in author identity
> plus thread mechanics. Checkmarx has no built-in identity, so the repository supplies it. The marker alone is never
> enough, because anyone who can comment can copy a valid findings table, and the latest marker comment would displace
> the real scan. User framing cannot reclassify a declared scanner comment, just as it cannot override a bot login. A
> marker comment from an undeclared author is a top-level comment, which neither Checkmarx nor the Human Feedback
> adapter (line-anchored threads only) acts on; the Detection note is all it produces.

**Comment signature:**

```html
<!--new issues section-->
<details>
    <summary><b>New Issues (N)</b></summary>
    <!-- markdown table with findings -->
</details>
```

### Edge cases

| Scenario                                           | Handling                                                                                                                                                          |
| -------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Multiple declared-author comments carry the marker | Use the latest by `created_at` timestamp                                                                                                                          |
| New Issues (0)                                     | Skip this source, because there are no actionable findings                                                                                                        |
| No comment carries the marker                      | The source is not present. Skip it silently, with no warning.                                                                                                     |
| A marker comment from an undeclared author         | `unverified`: a Detection note naming the author, and nothing else. It is never Checkmarx signal and never displaces a declared scanner's comment.                |
| A comment mentions Checkmarx but has no marker     | Skip with warning: possible format change or a different scan type. This heuristic exists **only** to raise the warning. It never promotes a comment into triage. |
| Comment is very long (>65k chars)                  | GitHub truncates long comments. Paginate, or use the API to get the full body.                                                                                    |
| Comment was edited                                 | Use current body (latest state). An edit keeps the comment's author, so a declared scanner's edited comment stays signal.                                         |

### Data to pass to subagent

Pass the full body of the latest declared-author comment that carries the `<!--new issues section-->` marker.

## GitHub Code Security

### Identity

- **Bot username:** `github-advanced-security[bot]`
- **Comment type:** PR review thread (line-anchored code scanning alert annotations, posted as a review)
- **Detection method:** Call `pull_request_read` method `get_reviews`. Identify the reviews that
  `github-advanced-security[bot]` authored. Then call `get_review_comments`, filtered to those reviews.

### Detection steps

1. Fetch all reviews with `pull_request_read` (method: `get_reviews`).
2. Identify reviews that `github-advanced-security[bot]` authored.
3. Fetch all review threads with `pull_request_read` (method: `get_review_comments`, paginate with `perPage: 100`).
4. Filter threads to those that belong to the bot's reviews, or whose root comment author is the bot.
5. Filter for threads where `isResolved` is `false`.
6. Confirm at least one unresolved thread exists.
7. _(Cross-check, optional)_ When thread data looks incomplete or ambiguous, consult the authoritative alert list for
   the PR: `gh api -X GET repos/{owner}/{repo}/code-scanning/alerts -f pr=<N>`. That list carries the rule ID, severity,
   and alert state that comment bodies may omit.

> **Detection keys on `user.login` plus unresolved-thread mechanics, never on comment-body markers.** Body formats for
> GitHub-native scanners are not a stable contract, so the specialist parses bodies leniently.

### Edge cases

| Scenario                                                      | Handling                                                                                    |
| ------------------------------------------------------------- | ------------------------------------------------------------------------------------------- |
| All threads already resolved                                  | Report "No unresolved GitHub Code Security threads". Skip this source.                      |
| `isOutdated` = true                                           | Include in triage. The code may have moved, but the alert may still apply.                  |
| Alert dismissed in the code-scanning UI but thread unresolved | Include it. The specialist notes the dismissal state from the alerts API in its assessment. |
| Alert also surfaces as a failed check run                     | This adapter owns it. See the dedup rule in the Checks section below.                       |
| No reviews from the bot at all                                | Skip this source                                                                            |

### Data to pass to subagent

Pass the complete unresolved thread data with these fields: `threadId`, root `commentId`, `path`, `line`, the comment
body, every reply with its author and `author_can_write`, and the `isOutdated` flag. When the body or the alerts API
cross-check carries them, also pass the alert number and rule ID.

## GitHub Code Quality

### Identity

- **Bot username:** `github-code-quality[bot]`
- **Comment type:** PR review thread (line-anchored quality findings, usually carrying a suggested autofix)
- **Detection method:** Same shape as GitHub Code Security. Call `pull_request_read` method `get_reviews`. Identify the
  reviews that `github-code-quality[bot]` authored. Then call `get_review_comments`, filtered to those reviews.

### Detection steps

1. Fetch all reviews with `pull_request_read` (method: `get_reviews`).
2. Identify reviews that `github-code-quality[bot]` authored.
3. Fetch all review threads with `pull_request_read` (method: `get_review_comments`, paginate with `perPage: 100`).
4. Filter threads to those that belong to the bot's reviews, or whose root comment author is the bot.
5. Filter for threads where `isResolved` is `false`.
6. Confirm at least one unresolved thread exists.

> **Comment-format tolerance.** GitHub Code Quality became generally available on 2026-07-20, but its comment bodies may
> change shape. Detection never depends on body format. When a body does not parse, the specialist triages from thread
> context (path, line, diff) and flags the parse gap in its output notes.

### Edge cases

| Scenario                                             | Handling                                                                                              |
| ---------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| All threads already resolved                         | Report "No unresolved GitHub Code Quality threads". Skip this source.                                 |
| Coverage-summary comment (aggregate, no line anchor) | Not a finding. Skip it. Only line-anchored review threads are triaged.                                |
| Suggested autofix present in the body                | Treat it as a proposal. The specialist verifies it independently (build and tests) before it commits. |
| Unrecognized body format                             | Triage from thread context (path, line, diff). Warn in the rollup notes.                              |
| `isOutdated` = true                                  | Include in triage. The code may have moved, but the finding may still apply.                          |
| Quality gate also surfaces as a failed check run     | This adapter owns it. See the dedup rule in the Checks section below.                                 |

### Data to pass to subagent

Pass the complete unresolved thread data with these fields: `threadId`, root `commentId`, `path`, `line`, the comment
body, every reply with its author and `author_can_write`, and the `isOutdated` flag. When the suggested-autofix block is
present, also pass it.

## Checks (GitHub Actions and other Checks-API providers)

### Identity

- **API endpoint:** `pull_request_read` method `get_check_runs` (or `get_status_checks` for the rolled-up state). No
  single "bot username" exists, because check runs come from many providers (`github-actions`, `circleci`, `buildkite`,
  `azure-pipelines`, custom apps).
- **Comment type:** None. Check runs arrive through the Checks API, not as PR comments.
- **Detection method:** Check-runs API filtered to `status == "completed"` AND (`conclusion == "failure"` OR
  `conclusion == "timed_out"`).

### Detection steps

1. Resolve the SHA CI ran on: `PR_REMOTE_SHA`, the head GitHub holds, recorded in orchestrator Phase 0.4. After a base
   sync, `PR_HEAD_SHA` is the local merge commit, which has no check runs yet.
2. Fetch all check runs for that SHA: `pull_request_read` method `get_check_runs` (paginate with `perPage: 100`).
3. Filter to runs where `status == "completed"` AND (`conclusion == "failure"` OR `conclusion == "timed_out"`). Skip
   `cancelled`, `neutral`, `skipped`, `success`. Treat `timed_out` as actionable. Tier it as `intervention`.
4. Group the surviving runs by `app.slug`, so the subagent can apply provider-specific log retrieval (see
   [checks-log-parsing.md](checks/checks-log-parsing.md)).
5. If the resulting list is empty, do these steps:
    - Report "No failed check runs on this PR".
    - Skip this source.

> **Why filter by `head_sha == PR_REMOTE_SHA`?** The API also returns older runs from earlier PR commits. Only runs on
> the head GitHub holds reflect the code under review. The Checks specialist still verifies its fixes at the synced
> head, where the base merge may already have fixed a failure.

### Edge cases

| Scenario                                                                                                                          | Handling                                                                                                                                                                                                                                                                                                                                                                               |
| --------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| All checks succeeded                                                                                                              | Skip this source                                                                                                                                                                                                                                                                                                                                                                       |
| Some checks still `in_progress`                                                                                                   | Triage what failed. Do not block on in-progress checks. The user can re-run triage when they finish.                                                                                                                                                                                                                                                                                   |
| `conclusion == "neutral"` or `"skipped"`                                                                                          | Not a failure. Skip it.                                                                                                                                                                                                                                                                                                                                                                |
| `conclusion == "timed_out"`                                                                                                       | Include it. Tier it as `intervention`, because the root cause of a timeout requires environment context.                                                                                                                                                                                                                                                                               |
| Re-run created multiple check runs for the same name                                                                              | Use the latest by `completed_at`                                                                                                                                                                                                                                                                                                                                                       |
| Workflow has matrix jobs (e.g., `test (node 18)`, `test (node 20)`)                                                               | Each is a separate check run. Triage each independently.                                                                                                                                                                                                                                                                                                                               |
| Provider is not GitHub Actions and full logs are unavailable                                                                      | Use annotations only. Tier on annotations alone. If the annotations are insufficient, demote the tier to `intervention`.                                                                                                                                                                                                                                                               |
| Deploy-stage check failure                                                                                                        | Always tier as `intervention` per [checks-tier-policy.md](checks/checks-tier-policy.md)                                                                                                                                                                                                                                                                                                |
| Check run originates from code scanning / code quality (`app.slug` or name contains `code-scanning`, `CodeQL`, or `code-quality`) | If the corresponding dedicated adapter detected signal on this PR, skip the check run. The dedicated adapters are GitHub Code Security and GitHub Code Quality. The dedicated adapter owns the finding, so a second triage of the check run would double-triage it. Keep the check run only when the dedicated adapter found nothing (threshold-only failure with no posted comments). |

### Data to pass to subagent

Pass an array of failed check-run objects. Each object has these fields:

- `check_run_id`
- `name`
- `app.slug` (provider identity)
- `head_sha` (must equal `PR_REMOTE_SHA`)
- `details_url`
- `conclusion`
- `started_at` / `completed_at`

The subagent fetches annotations and logs itself via the references in `checks/`.

## Bot Review

The generic source for review bots no specialised adapter claims. Copilot Review, GitHub Code Security, and GitHub Code
Quality keep their own adapters. Bot Review takes every other bot's line-anchored review threads.

### Identity

- **Author:** the root comment's author passes the bot gate (see "Bot gate" under Human Reviewer Comments). The author's
  login matches no routing-table row other than a Bot Review row. `github-actions[bot]` routes its check runs to Checks
  and its line-anchored review threads here.
- **Lane:** a login the routing table names for Bot Review on the automatic lane gets automatic replies and fixes. Every
  other login, `github-actions[bot]` included, is on the draft lane: its threads become drafts that the developer
  accepts one by one, and a run with no one present does not dispatch them (`Pending Review`). A workflow's input or the
  PR author can choose the words a `github-actions[bot]` comment carries, and an account type says who posted text, not
  who chose it.
- **Comment type:** PR review thread (line-anchored). A bot's top-level comments and review bodies are not this source.
- **Detection method:** Reuse paginated `get_review_comments` for threads and paginated `get_reviews` for review bodies.
  Filter both post-fetch.

### Detection steps

1. Reuse the thread list that Human Reviewer Comments detection fetched (step 1 there). Do not fetch it twice. Also
   reuse the complete, paginated `get_reviews` list from specialised-source detection. If it is not already available,
   fetch it. Inspect nonempty review bodies whose authors pass the bot gate. Apply the routing table first. Discard
   Out-of-scope authors. Leave specialised authors with their adapters. Record each unmatched bot with no line-anchored
   threads as `unhandled` in the Phase 1 detection-state record. For each unhandled bot, include its login, review ID,
   body, and URL as evidence for the unhandled-bot note. Include the evidence even when no Bot Review source is
   dispatched.
2. Keep threads with `isResolved == false` whose root comment's author passes the bot gate.
3. Drop threads whose root author matches a routing row that sends line-anchored threads somewhere other than Bot
   Review. Keep every author the table routes to Bot Review, `chatgpt-codex-connector[bot]` and `github-actions[bot]`
   included. The dropped rows are specialised adapters and `Out of scope` rows (`dependabot[bot]`, `renovate[bot]`, and
   any org opt-out row). Discard routing-table Out-of-scope authors before you construct the detection-state record. Do
   not record their signals as `skipped` or `unhandled`. They produce no reply, rollup row, Detection note, or
   unhandled-bot note.
4. Group the remaining threads by root author login. Each login becomes one rollup row, `Bot Review ({login})`, and
   carries its lane from the routing table.
5. If no thread remains, do these steps:
    - Report "No unresolved threads from unlisted review bots".
    - Skip this source.

The root comment's author decides the source. A human reply inside a bot's thread does not move the thread to Human
Feedback. The subagent reads the reply as context.

### Edge cases

| Scenario                                                          | Handling                                                                                                  |
| ----------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| Threads from two or more unlisted bots                            | One Bot Review source and subagent. One rollup row per login, in detection order.                         |
| A human replied inside a bot's thread                             | The thread is still Bot Review. Only a reply from an author with write access can change the disposition. |
| Body carries a severity badge (`P0`–`P3`, `[nit]`, …)             | The subagent parses it into `severity`. It reports it in the rollup only, never in the reply.             |
| Unrecognized body format                                          | Triage from thread context (path, line, diff). Note the parse gap in the rollup.                          |
| The same defect raised by Copilot, Code Quality, or Code Security | The specialised source owns it. Bot Review returns `duplicate_of` (see the subagent contract).            |
| `isOutdated` = true                                               | Include it. Validate it against the current file.                                                         |
| A bot that posts review bodies but no line-anchored thread        | Not this source. It stays an unhandled bot signal note.                                                   |

### Data to pass to subagent

Pass every kept thread with `threadId`, root `commentId`, `user.login` (the bot), `path`, `line`, `start_line`,
`original_line`, `original_start_line`, `side`, `start_side`, the root comment body, every reply in the thread with its
author and `author_can_write`, and `isOutdated`. Also pass the unresolved threads of Copilot Review, GitHub Code
Quality, and GitHub Code Security as `peers`, read-only. Each peer carries `source`, `threadId`, `path`, `line`,
`start_line`, `original_line`, `original_start_line`, `side`, `start_side`, and its body. Peers let the subagent
recognise duplicates. Preserve null range fields for single-line comments. `line` and `original_line` are the respective
range ends.
