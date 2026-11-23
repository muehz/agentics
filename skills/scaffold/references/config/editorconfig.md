# Scaffold reference: .editorconfig

## Questions to ask

None. Scan the project for file types. Generate the file without asking.

## Generation process

1. Scan the project tree for the file extensions present
2. Set the style conventions from the project signals and the decision table below
3. Apply per-language rules only to the file types present in the repo
4. Validate the generated config against existing formatting tools (Prettier, ESLint, Black)
5. Flag any conflicts

## Style decision points

| Decision            | Default                                          | Override when                                                     |
| ------------------- | ------------------------------------------------ | ----------------------------------------------------------------- |
| Tabs vs spaces      | Spaces for most languages; tabs for Go, Makefile | Team preference or existing codebase convention                   |
| Indent size         | 4 for `[*]`; 2 for JS/TS/YAML/JSON/Bicep         | Linter config (`.eslintrc`, `pyproject.toml`) says otherwise      |
| Line endings        | LF (Unix)                                        | CRLF only for `.bat`/`.cmd` files                                 |
| Charset             | UTF-8                                            | Latin1 for legacy codebases                                       |
| Trailing whitespace | Trim                                             | Preserve for Markdown (line-break semantics) and CSV (field data) |
| Final newline       | Insert                                           | A named consumer tool of that file type rejects a final newline   |

## Base config (always include)

```ini
# EditorConfig: https://EditorConfig.org
root = true

[*]
charset = utf-8
end_of_line = lf
insert_final_newline = true
trim_trailing_whitespace = true
indent_style = space
indent_size = 4
```

## Per-language overrides

| File pattern        | indent_size | Notes                              |
| ------------------- | ----------- | ---------------------------------- |
| `*.md`              | (inherit 4) | `trim_trailing_whitespace = false` |
| `*.{yml,yaml}`      | 2           |                                    |
| `*.json`            | 2           |                                    |
| `*.{ts,js,tsx,jsx}` | 2           |                                    |
| `*.{cs,csx}`        | 4           |                                    |
| `*.go`              | 4           | `indent_style = tab`               |
| `*.py`              | 4           |                                    |
| `*.bicep`           | 2           |                                    |
| `*.ps1`             | 4           |                                    |
| `*.sh`              | 4           |                                    |
| `Makefile`          | (inherit)   | `indent_style = tab`               |
| `*.csv`             | Inherit     | `trim_trailing_whitespace = false` |

## Windows note

PowerShell Core (pwsh 6.0+) handles LF correctly. Do NOT set `.ps1` to CRLF, because CRLF causes diff noise. Only
`.bat`/`.cmd` files need CRLF. If those files are present, add a `[*.{bat,cmd}]` section with `end_of_line = crlf`.

## Conformance check (audit mode)

✅ Conformant if the file contains `root = true` and at least one `[*]` section with both `indent_style` and
`indent_size`.
