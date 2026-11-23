# Scaffold reference: CONTRIBUTING.md

## Questions to ask

Ask what is open as one ❓ round. Give each question a ➡️ recommendation drawn from the request and the repo. Skip a
topic the request already settles.

- **Repository scope:** what lives here and what does not.
- **Development prerequisites:** Node.js version, tools, CLIs.
- **Local setup commands:** `git clone`, `npm install`, etc.
- **Pre-PR validation commands:** exact `lint`, `build`, `test` commands.
- **Commit convention:** Conventional Commits / release-please, or a different convention.
- **Deployment or release workflow notes,** if any.

## Structure

Follow this structure:

1. **Scope:** what the repo owns (source code, infra, CI/CD, tests, docs)
2. **Prerequisites:** required tools and versions
3. **Local setup:** clone and install commands
4. **Validate before opening a PR:** exact lint/build/test commands; infra validation if applicable
5. **Branching and pull requests:** branch from `main`, focused PRs, PR description guidelines
6. **Recommended PR checklist:** markdown task list of gates
7. **Commit message convention:** Conventional Commits or project-specific
8. **Deployment workflow notes:** CI/CD trigger behavior (optional)

## Notes

- CONTRIBUTING.md is valid in `.github/`, the repo root, or `docs/`.
    - GitHub checks those locations in that order and uses the first file it finds.
    - The recommended location is the **root**.
- GitHub automatically shows a link to CONTRIBUTING.md on the "New issue" page when the file is in any valid location

## Conformance check (audit mode)

✅ Conformant if the file contains sections that cover prerequisites, local setup, the PR process, and commit message
conventions.
