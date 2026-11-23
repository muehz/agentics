# Scaffold reference: README.md

## Questions to ask

Ask what's open as one ❓ round, each question with a ➡️ recommendation drawn from the request and the repo. Skip a
topic the request already settles.

- **Key features or capabilities** (bullet list).
- **GitHub Actions badges** to include, if any (workflow names).
- **Prerequisites** to run or develop this project.
- **Primary setup steps** (high-level).
- **Deployment target or runtime environment.**
- **Architecture diagram or reference document** to link to, if any.

## Structure

Follow this order:

1. Project name / logo (use repo name if no logo)
2. Brief one-sentence description
3. Status badges (CI, deploy, or analysis; optional)
4. Overview paragraph
5. Key features (bullet list)
6. Prerequisites
7. Getting started (setup commands)
8. Usage examples
9. Configuration
10. Architecture overview (link to `docs/ARCHITECTURE.md` if present)
11. Links to further documentation

## Rules

- Do NOT include LICENSE, CONTRIBUTING, or CHANGELOG sections. Those have dedicated files.
- Do NOT overuse emojis
- Use GFM (GitHub Flavored Markdown) and GitHub admonition syntax where appropriate
- Keep it short and useful to a developer who just cloned the repo

## Badge pattern

```markdown
[![{Workflow name}](https://github.com/{owner}/{repo}/actions/workflows/{workflow-file}/badge.svg)](https://github.com/{owner}/{repo}/actions/workflows/{workflow-file})
```

Take `{owner}` and `{repo}` from the repository's `origin` remote. Take `{workflow-file}` from a file that exists in
`.github/workflows/`. Write no badge for a workflow the repository does not have.

## Notes

- README.md is valid in `.github/`, the repo root, or `docs/`, in the order GitHub checks them. Audit mode accepts all
  three. Unless there is a specific reason not to, keep it at the **root**. Developers who clone the repo find it there
  most easily.

## Conformance check (audit mode)

✅ Conformant if the file contains a project title, a description, and at least 3 of these items:

- Badges.
- Prerequisites.
- Setup instructions.
- Usage examples.
- An architecture reference.
- A contributing link.
