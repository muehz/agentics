# Scaffold reference: .gitignore

## Questions to ask

If `.gitignore` is missing or empty, ask as a ❓ block:

> ❓ **Q1** - **Gitignore platform**: What language/platform should I use for the .gitignore template? Common options:
> Node, VisualStudio (.NET/C#), Python, Go, Java, Terraform.
>
> ➡️ The template(s) matching the stacks detected in the file tree, combined when several are present; otherwise the
> tech stack named in the project identity step.

## Source

Fetch the matching template from the [github/gitignore](https://github.com/github/gitignore) repository. Each platform
maps to one template file:

| Platform             | Template file            |
| -------------------- | ------------------------ |
| Node.js / TypeScript | `Node.gitignore`         |
| .NET / C#            | `VisualStudio.gitignore` |
| Python               | `Python.gitignore`       |
| Go                   | `Go.gitignore`           |
| Java                 | `Java.gitignore`         |
| Terraform            | `Terraform.gitignore`    |

If the github/gitignore API is unavailable, use a sensible inline template for the detected stack.

> **Empty repo:** If the repo has no files yet (nothing to scan), **always ask the platform question**. Do not skip it.
> Do not emit an empty file.

## Local additions

Always append this section at the bottom of the generated file:

```gitignore
# Local additions
.DS_Store
*.local
```

## Conformance check (audit mode)

✅ Conformant if the file exists and is non-empty.
