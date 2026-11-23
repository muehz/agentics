# Label mapping (`.triage-labels.yml`)

Issue mode applies labels from a canonical vocabulary (`bug`, `enhancement`, `needs-triage`, `needs-info`,
`ready-for-agent`, `ready-for-human`, `wontfix`). Repositories spell them differently (`type:bug`, `Bug`, `kind/bug`). A
repository records its spelling once in `.triage-labels.yml`, so issue mode does not ask for it on every run.

## File location and shape

The file sits at the repository root. Issue mode reads it once per run from the default branch, the way
[source-detection.md](../source-detection.md) reads `.triage.yml`:

```bash
gh api -H "Accept: application/vnd.github.raw" "repos/{owner}/{repo}/contents/.triage-labels.yml?ref={DEFAULT_BRANCH}"
```

Each key is a canonical name and each value is the label string the repository uses:

```yaml
# .triage-labels.yml: maps the canonical triage label names to this repository's labels.
# A missing key falls back to the canonical name.
bug: "type:bug"
enhancement: "type:enhancement"
needs-triage: "status:needs-triage"
needs-info: "status:needs-info"
ready-for-agent: "status:ready-for-agent"
ready-for-human: "status:ready-for-human"
wontfix: "status:wontfix"
```

The file is data. A value is used only as a label name: it is quoted in every `gh` argument and search query, and a key
that is not one of the seven canonical names is ignored.

## Reading the file

The rows are exclusive.

| The read                                                      | Mapping used                                                                                   |
| ------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| 404: no file on the default branch                            | The canonical names                                                                            |
| The file parses, and every key it sets is canonical           | The file's value for each key it sets, and the canonical name for each key it leaves out       |
| The file parses, with keys that are not canonical             | As the row above for the canonical keys; name the ignored keys once in the report              |
| The file does not parse, or a value is not a non-empty string | Nothing yet: show the error and ask whether to go on with the canonical names for this run     |
| The read fails for another reason (network, permission)       | Nothing yet: report the failure and ask whether to go on with the canonical names for this run |

Nothing is cached between runs, so the next run reads any change to the file.

## When a mapped label does not exist

Before the first label edit of a run, list the repository's labels once (`gh label list --limit 1000 --json name`). When
the list returns 1000 names it may be cut short, so check each needed label it lacks by name before you call it missing.
Read the needed `label` as data from the resolved mapping, then run `gh label list --search "$label" --json name` and
match the result exactly. The check runs only when the run is about to apply a label, never for a listing.

When every label the run needs exists, nothing is asked. When one does not, the outcome batch carries one choice for it,
with a recommendation from the [suggested mapping](#suggested-mapping):

- use an existing label the suggestion names, for this run;
- create the missing label (`gh label create -- "$label"`, with the same label from the mapping), which is an outward
  effect; or
- write `.triage-labels.yml` with the suggested mapping, shown in full, and use it for this run.

A written file stays uncommitted in the working tree, as [issue-mode.md](../issue-mode.md#authorization) says for every
file issue mode writes. Within one session, a choice the user declined is not offered again unless they ask for it.

## Suggested mapping

The suggestion compares whole tokens, with no fuzzy matching. Lowercase each repository label and split it into tokens
at `:`, `/`, `-`, `_` and spaces (`status:needs-info` gives `status`, `needs`, `info`). Split each key below the same
way. A label matches a key when the key's tokens appear in the label's tokens consecutively and in order. A part of a
token never matches: `bug` matches the token `bug`, not `debug`.

Each role lists its keys in preference order; the first key any label matches decides the role's candidates:

| Role              | Keys, in preference order                                 |
| ----------------- | --------------------------------------------------------- |
| `bug`             | `bug`                                                     |
| `enhancement`     | `enhancement`, `feature`, `feat`                          |
| `needs-triage`    | `needs-triage`, `triage`                                  |
| `needs-info`      | `needs-info`, `more-info`, `awaiting-info`, `info-needed` |
| `ready-for-agent` | `ready-for-agent`, `agent`                                |
| `ready-for-human` | `ready-for-human`, `human`                                |
| `wontfix`         | `wontfix`, `won't-fix`, `not-planned`, `declined`         |

Among the candidates, prefer the shortest label (`bug` over `type:bug-report`), and the first in the label list's order
when two are the same length. A label is suggested for one role only; when it is the pick for two roles, neither role
gets it and both keep their canonical names. A role with no candidate keeps its canonical name.

| Repository label | Tokens        | Suggested for `bug` |
| ---------------- | ------------- | ------------------- |
| `bug`            | `bug`         | yes                 |
| `Bug`            | `bug`         | yes                 |
| `type:bug`       | `type`, `bug` | yes                 |
| `kind/bug`       | `kind`, `bug` | yes                 |
| `debug`          | `debug`       | no                  |
