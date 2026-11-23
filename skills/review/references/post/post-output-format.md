# Automated review output format

The post pipeline writes its PR comment in this format, inside the shared
[posting envelope](../../../gh/references/posting-envelope.md): the idempotency marker on the first line, the
head-metadata line under it, the body, and the envelope's footer as the last line. When the base revision's guidance
supplies its own report structure, use it, and keep the source citations, the security coverage statement, the marker,
the head line, and the footer in it.

Write every statement about coverage from what this run did. A comment makes no claim about tools that did not run and
carries no internal counts, such as reviewer hits or dropped candidates; those go in the run's local report.

## Comment format (issues found)

```markdown
<!-- agentics:review:post -->
<!-- agentics:head:{full_head_sha} -->

### Code review

Found {N} issues:

1. **{severity}** {brief description of issue} ({source, e.g. AGENTS.md says "...", or a bug in {file and code
   snippet}})

{permalink to file and line range}

2. **{severity}** {brief description of issue} ({source})

{permalink to file and line range}

---

Security coverage: read by a model against {the domains read}{; not read: the domains skipped, and why}. This lists what
one reading found. It is not a scan, and it does not establish that the diff is secure.

> _Written by an AI agent_
```

`{severity}` is the shared severity from [severity.md](../../../routing/references/contracts/severity.md): Critical,
Important, or Minor. Informational context does not get a numbered item.

## Security findings

Security findings from reviewer #8 go in the same numbered list as every other finding.
[../security/security-findings-format.md](../security/security-findings-format.md) defines their shape: severity, OWASP
category, CWE, traced evidence, a concrete fix, and the permalink. On a public repository, a confirmed or likely
security issue that is not fixed stays out of the comment, as
[post-pipeline.md](post-pipeline.md#step-9-post-the-comment) says.

**The security coverage line does not depend on whether a security finding exists.** It goes on every posted comment.
Otherwise a reader who sees no security items reads the review as "a reviewer looked and the code is fine". Build it
from what reviewer #8 returned: the domains it read, the domains it skipped, and the gaps it could not check. When the
base revision's guidance requested a security check that ran, name it and its result in that line.

Never write "no security issues found". Never render a clean security result as `Pass`, `Clean`, or `Secure`.

## Comment format (no issues)

```markdown
<!-- agentics:review:post -->
<!-- agentics:head:{full_head_sha} -->

### Code review

No findings cleared the confidence bar at this head. Read for: {the review passes performed, including the applicable
repository guidance}.

Security coverage: read by a model against {the domains read}{; not read: the domains skipped, and why}. A clean review
is not a finding that the diff is secure.

> _Written by an AI agent_
```

The pipeline uses this format when it updates its own earlier comment for a head where nothing cleared the bar, and when
the base revision's guidance asks for an empty-result report. Otherwise, with nothing to report and no earlier comment,
nothing posts. State a clean result as an outcome, never as a verdict.

## Idempotent posting

Before you post, find the agent's own comment: a top-level PR comment that carries the `<!-- agentics:review:post -->`
marker and whose author is the login the session is authenticated as. Update that comment in place and post no new one,
so a rerun produces one review comment, not a growing list. A comment with the marker under another author is never
edited or copied; post beside it and say so in the run's report.
[skills/gh/references/posting-envelope.md](../../../gh/references/posting-envelope.md#posting-mechanics) gives the
find-and-update commands.

The `<!-- agentics:head:{full_head_sha} -->` line holds the full 40-character head SHA recorded in pipeline Step 1. The
next run's Step 1 compares it to the PR's current head to decide whether this head is already reviewed.

## Permalink requirements

Every issue links the relevant file and line range in this format:

```
https://{host}/{owner}/{repo}/blob/{full_sha}/{file_path}#L{start}-L{end}
```

**Rules:**

- **Host and repository.** `{host}` is the forge host the repository resolves to, and `{owner}/{repo}` is the repository
  under review.
- **Full SHA.** Use the complete 40-character SHA recorded in pipeline Step 1. Never abbreviate it, and never write
  `$(git rev-parse HEAD)` or similar: the comment renders as Markdown, so a shell command appears literally.
- **Which SHA.** Read it off the finding's `exists_at_head` field, which the reviewer that read the hunk set:
    - `yes` links the PR's head SHA.
    - `no` links the base SHA, which shows the code before the deletion. The lines no longer exist at head.
    - `never` links the head SHA where the missing code belongs, and the finding text says the link marks a gap.

    Do not re-derive it from the diff at post time; a second guess is how two answers come to disagree.

- **Line range.** Put `#` right after the file path, use `L{start}-L{end}` (e.g., `L10-L15`), and include at least one
  line of context before and after: for lines 5-6, link `L4-L7`.

**Example:**

```
https://github.com/anthropics/claude-code/blob/1d54823877c4de72b2316a64032a54afc404e619/README.md#L13-L17
```

## Comment guidelines

- Keep output brief, with no emojis and no filler.
- Each issue cites its source (an `AGENTS.md` instruction, a code comment, git history, and so on).
- Write for a reader who has never seen Agentics. A recommendation stays a recommendation unless the user made the
  decision in this session.
- Post with `gh pr comment`, not web fetch, with the body from a file, never inline.
