# Scaffold reference: .markdownlint.json

## Questions to ask

None. Generate the standard config.

## Standard config

```json
{
    "MD004": false,
    "MD007": false,
    "MD012": false,
    "MD013": false,
    "MD024": false,
    "MD025": false,
    "MD029": false,
    "MD033": false,
    "MD036": false,
    "MD041": false
}
```

## Why these rules are disabled

| Rule  | What it checks                         | Why disabled                                             |
| ----- | -------------------------------------- | -------------------------------------------------------- |
| MD004 | Unordered list style                   | Mixed `-` and `*` valid in docs                          |
| MD007 | Unordered list indentation             | Conflicts with some Markdown renderers                   |
| MD012 | Multiple blank lines                   | Common in long docs                                      |
| MD013 | Line length                            | Prose and tables routinely exceed 80 chars               |
| MD024 | Duplicate headings                     | Common in multi-section docs (e.g., repeated "Overview") |
| MD025 | Multiple top-level headings            | Some templates use multiple H1s                          |
| MD029 | Ordered list item prefix               | `1.` vs sequential numbering both valid                  |
| MD033 | Inline HTML                            | Docs use HTML such as `<details>` or `<br>` in tables    |
| MD036 | Emphasis used as heading               | Common in informal docs                                  |
| MD041 | First line should be top-level heading | Files with front matter violate this                     |

## Conformance check (audit mode)

✅ Conformant if the file is valid JSON and contains at least one rule key.
