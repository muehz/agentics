# Comment Parsing

How to locate and parse the Checkmarx One PR comment and extract its security findings.

## Comment structure

Checkmarx posts a top-level PR issue comment, not a review thread. The comment has this general structure:

```markdown
![Logo](https://cdn.ast.checkmarx.net/integrations/logo/Checkmarx.png) **Checkmarx One – Scan Summary & Details** –
[scan-id](scan-url)

<!--new issues section-->

---

<details><summary><b>New Issues (N)</b></summary>
Checkmarx found the following issues in this Pull Request

| #   | Severity                                                              | Issue      | Source File / Package  | Checkmarx Insight                                              |
| --- | --------------------------------------------------------------------- | ---------- | ---------------------- | -------------------------------------------------------------- |
| 1   | ![HIGH](https://cdn.ast.checkmarx.net/integrations/severity/High.png) | Issue Name | /path/to/file.ts: [42] | <details><summary>details</summary>Description text.</details> |

...

</details>

<!--new issues section-->
```

Key structural markers:

- `<!--new issues section-->` marks the start and the end of the new issues block. It appears twice, once before the
  block and once after it.
- `<details><summary><b>New Issues (N)</b></summary>` opens the collapsible block that holds the findings.
- The markdown table uses `|` delimiters with a header row and a separator row.

## Which comment is parsed

The orchestrator chooses the comment during detection and passes its full body; the specialist parses only that body. A
comment counts only when both hold ([source-detection.md](../../source-detection-automated.md#checkmarx-one)):

1. Its body contains `<!--new issues section-->`.
2. Its author is one of the scanner accounts the repository declares in `checkmarx_accounts` in `.triage.yml`, read from
   the default branch. No login is built in, because each deployment chooses the account that posts (a GitHub App, a
   machine user, or a PAT-backed account), and `checkmarx-one[bot]` is only a common default.

The marker alone is never enough: anyone who can comment on the PR can paste a findings table under it. Of the comments
that pass both tests, the latest by `created_at` is used (scan reruns create new comments).

## Extracting the scan URL

The scan URL is in the first line, after the logo:

```markdown
**Checkmarx One – Scan Summary & Details** – [scan-id](scan-url)
```

Extract with:

```regex
/\*\*Checkmarx One.*?\*\*\s*–\s*\[.*?\]\((https:\/\/[^)]+)\)/
```

## Extracting the new issues count

```regex
/<summary><b>New Issues \((\d+)\)<\/b><\/summary>/
```

If the count is absent or 0, do these steps:

1. Report "No new issues".
2. Stop.

For a positive count, compare the reported count with the number of parsed rows. Any mismatch, including zero parsed
rows or a shortfall, is a Checkmarx parsing failure for human intervention. Retain any rows that did parse for
assessment. Never treat a nonzero count with a mismatched parsed count as clean.

For example, `<summary><b>New Issues (0)</b></summary>` with an empty table is a clean result.
`<summary><b>New Issues (2)</b></summary>` with an empty table is a parse failure, as is the same summary with one
parseable row. The rollup's needs-you list must show both failures. The one parsed row in the second case remains
available for assessment. A comment with no new-issues block is also clean because there is no reported count to
contradict it.

## Parsing the findings table

The table is between the `<summary>` line and the closing `</details>` in the new issues section.

### Table columns

| Column                | Content             | Notes                                                                                             |
| --------------------- | ------------------- | ------------------------------------------------------------------------------------------------- |
| `#`                   | Row number          | Sequential integer                                                                                |
| Severity              | Image with alt text | `![CRITICAL](url)`, `![HIGH](url)`, `![MEDIUM](url)`, or `![LOW](url)`                            |
| Issue                 | Issue type name     | May be plain text or a markdown link `[Name](url)`                                                |
| Source File / Package | File path + line    | Format: `/path/to/file.ext: [line]` or `/path/to/file.ext: [line](url)`                           |
| Checkmarx Insight     | Info block          | `<details><summary>details</summary>text</details>` optionally followed by `[Attack Vector](url)` |

### Row parsing regex

Process each non-header, non-separator line whose first cell is a number:

```regex
/^\|?\s*(\d+)\s*\|\s*!\[(CRITICAL|HIGH|MEDIUM|LOW)\].*?\|(.+?)\|(.+?)\|(.+)$/
```

Groups:

1. Row number
2. Severity (from image alt text)
3. Issue type (may contain a markdown link; strip `[` and `](url)`)
4. Source file + line
5. Info text

### Extracting file path and line number

The source column uses this format, with backslash escapes that GitHub markdown adds:

```text
/path/to/file\.ext: [42](optional-url)
```

Steps:

1. Remove all backslash escapes: replace `\.` with `.`, `\-` with `-`
2. Extract file path: everything before `: [`
3. Extract line number: the integer inside `[N]`
4. Remove the leading `/` from the file path, because the path is repo-relative and not absolute

```regex
/^\/?(.*?):\s*\[(\d+)\]/
```

### Extracting issue type

The issue column may be:

- Plain text: `Passwords And Secrets - Generic Password`
- Markdown link: `[Missing_HSTS_Header](url)`

If it is a link, extract only the text between `[` and `]`.

### Extracting info text

The info column contains a nested `<details>` element:

<!-- prettier-ignore -->
```html
<details><summary>details</summary>Description text.</details>
```

Extract the text between `</summary>` and `</details>`:

```regex
/<\/summary>(.*?)<\/details>/
```

The text may end in `...` because Checkmarx truncated it. Use it as assessment context. You read the actual source file
for the full picture.

## Edge cases

### Backslash escaping

GitHub's markdown rendering adds backslash escapes to special characters in the bot comment (periods, hyphens,
underscores). Always unescape before you use paths or text.

### Multiple new issues sections

Some Checkmarx configurations post separate sections, for example SAST, SCA, and IaC. Each has its own
`<!--new issues section-->` block. Parse every new issues block, including those after the first.

### Line number links

Some line numbers are links, such as `[42](https://...)`. Extract only the number. Ignore the URL.

### Missing line numbers

Rarely, a finding has no line number, for example a package-level SCA finding. For such a finding, do these steps:

1. Set `line` to `null`.
2. Assess the finding at the file or package level.

A package-level finding's fix is a dependency change, which lives in a manifest or lockfile. Those are paths triage
never changes, so such a finding is never remediated by the specialist.
