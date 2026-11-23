# Orchestration Lifecycle: Phase 7 and Follow-Ups

Split from [orchestration-lifecycle.md](orchestration-lifecycle.md). Covers posted-formatting verification (Phase 7) and
follow-ups within an execution.

## Phase 7 — Verify Posted Formatting

Before you declare the run complete, fetch every posted reply and rollup comment back from GitHub. This phase catches
comment-body corruption and common markdown hazards in the final posted bodies.

### 7.1 — Build the verification set

Create one verification record for every response actually posted in Phase 6:

| Field            | Description                                                                                                  |
| ---------------- | ------------------------------------------------------------------------------------------------------------ |
| `source`         | Source adapter name (`copilot`, `code-security`, `code-quality`, `bot-review`, `human-feedback`) or `rollup` |
| `target_type`    | `thread_reply`, or `issue_comment` for the rollup                                                            |
| `target_id`      | Root review comment ID for replies, PR number or comment ID for comments                                     |
| `expected_body`  | Final body after Phase 6 SHA substitution and approval-path edits                                            |
| `marker`         | Idempotency marker expected in the posted body                                                               |
| `posted_comment` | GitHub comment/reply ID returned by the create or update operation                                           |

Do not include skipped drafts, withheld replies, or push-blocked or fork-skipped per-source replies. Those were not
posted. A response whose post failed after the fallback retry is not a record either, because there is no posted body to
fetch. Report it as a verification warning per 7.5, with the target and the posting error, and confirm that the rollup
details line for its source records the failure.

Rollup sections are checked as part of the rollup body, not as records of their own.

The rollup's record carries the comment id that 6.4 step 8 recorded as its `posted_comment`. A follow-up's edit (see
"Follow-ups within an execution") reuses that record with the edited body as `expected_body`.

### 7.2 — Fetch posted bodies back

Prefer GitHub MCP read tools when available. Paginate with `perPage: 100` until every page is exhausted. Fall back to
`gh api`:

```bash
# Top-level PR comments / rollup comments
gh api repos/{owner}/{repo}/issues/{PR_NUM}/comments --paginate

# Review-thread replies and review comments
gh api repos/{owner}/{repo}/pulls/{PR_NUM}/comments --paginate
```

Match fetched comments by `posted_comment`. When Phase 6 captured no ID, match a thread reply by its expected
idempotency marker and the author `{LOGIN}`, and match the rollup by its execution key and the author `{LOGIN}`. Never
match the rollup by the rollup marker, which earlier rollups also carry. If neither finds a body, report a verification
warning for that response and continue.

### 7.3 — Compare fetched body to finalized draft

Compare each fetched body against the corresponding final body after Phase 6 SHA substitution. For approval-path
comments, compare against the accepted or edited final body, not the original subagent draft. For the unified rollup,
compare against the final body that Phase 6.4 constructs.

Normalize only GitHub API transport details that do not change rendering, such as trailing newlines and CRLF-to-LF line
endings. Do not normalize escaped HTML comment markers, missing blank lines, malformed tables, or changed link text.
This phase exists to find them.

### 7.4 — Run markdown/rendering checks

For each fetched body, check these hazards:

| Check                                                   | Failure signal                                                                                                                                                                                                                |
| ------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Idempotency markers are intact and parseable            | The marker is missing, visible-escaped as `<\!--`, duplicated unexpectedly, or does not match the source or the target                                                                                                        |
| HTML comments are closed                                | The body contains a `<!--` sequence without a later matching `-->`. The sequence can hide the rest of the rendered comment                                                                                                    |
| Tables have separator rows and consistent column counts | A table-like block has no separator row, or any row has a different pipe-column count once escaped literal pipes (`\|`) are ignored                                                                                           |
| Blockquotes are preceded by a blank line                | The first `>` line in a blockquote block follows normal non-blockquote text with no blank line between, so sections can merge unexpectedly                                                                                    |
| Code fences are balanced                                | The body has an odd number of triple-backtick fences for a given fence style                                                                                                                                                  |
| Links are well-formed                                   | Markdown links have empty labels, empty URLs, unbalanced brackets, or a missing URL group                                                                                                                                     |
| Post cap holds                                          | A run or follow-up created a second top-level PR comment for its execution, counting only comments by `{LOGIN}` (a replacement for a deleted rollup is allowed), or edited a comment that carries a retired per-source marker |
| Section anchors are paired                              | A `<!-- agentics:triage:section:{key} -->` in the rollup has no matching `<!-- agentics:triage:section-end:{key} -->`                                                                                                         |
| Rollup shape holds                                      | The rollup fails a rule in `rollup-template.md`, "Validate before posting"                                                                                                                                                    |

### 7.5 — Report warnings without blocking

Formatting verification is non-blocking. If any check fails, report a concise warning block with:

- The source and target
- The posted GitHub comment or reply URL when available
- The failed check name
- The smallest useful excerpt or marker needed to edit the comment in place

Do not do any of these from Phase 7:

- Roll back posted comments.
- Delete comments.
- Fail the whole triage run.
- Re-run the posts automatically.

The operator can edit the affected comment in place or rerun triage if the issue is systemic.

## Follow-ups within an execution

A follow-up is work the developer asks for in the conversation that ran the command, on the same PR, with no
`/agentics:triage` typed. Examples are a further fix, a re-check of one source, and a drafted reply. A re-typed
`/agentics:triage pr` is a new command and so a new execution, not a follow-up (`rollup-template.md`, "Execution").

The work follows whatever rules govern it, for example the `review` skill, the thread-reply rules in 6.2, and the git
skill's [push workflow](../../git/references/push.md) for any push it makes. Replies a follow-up posts carry their own
thread markers. Triage adds one duty. When the work changes what the rollup reports, edit the execution's rollup once,
in place. Do these steps:

1. Start from the rollup body that the execution holds. If the body is lost, fetch the comment by its id. If the id is
   lost too, list the PR's top-level comments with `perPage: 100` until every page is exhausted, and find the one that
   carries this execution's key and whose author is `{LOGIN}`. Never edit a comment that carries the key under any other
   author, and name it in the session report. A follow-up never makes up a key; without the execution's own key it edits
   nothing and says so.
2. Change only the rows and Details blocks of the sources that the follow-up touched. Recompute the Total row, the
   header verdict, and the needs-you list from the resulting rows. A re-check that detects nothing sets the source's row
   to `No new findings`. Carry every other row and block forward byte for byte, including the link line.
3. Check the edited body against `rollup-template.md`, "Validate before posting", and the outbound checks. Edit the
   comment, fetch it back as Phase 7 does, and tell the developer in one line that the rollup was updated.

A follow-up that changes nothing in the rollup makes no edit. A follow-up re-detects no source it was not asked about,
runs no base sync, and creates no worktree for another source. It never creates a top-level comment, except the
replacement below.

**Edit call.** Use the issue-comment update tool of the GitHub MCP server when the session has one. Otherwise write the
edited body to a file in session storage with the file-writing tool, and send it with `gh api`:

```bash
gh api --method PATCH repos/{owner}/{repo}/issues/comments/{comment_id} -F body=@"{BODY_FILE}"
```

**When the rollup is gone.** When the edit returns not found, the rollup comment no longer exists. Create a replacement
top-level comment under the same execution key, with the edited body, and record its id. When that create fails or
returns no answer, follow the search-before-retry rule of 6.4 step 8. Say so to the developer in one line. The
replacement carries no link to the deleted comment. It keeps the link line that the original had. The execution then
still has at most one live top-level comment, and a later search for the key finds one comment.
