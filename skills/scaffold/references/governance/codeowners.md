# Scaffold reference: CODEOWNERS

## Questions to ask

Ask what's open as one ❓ round. Give each question a ➡️ recommendation drawn from an existing `CODEOWNERS` file or
owners the request names. Never recommend a team name guessed from elsewhere. With no such source, recommend the
single-owner default output. Leave the owner for the user to name. Skip a topic the request already settles.

- **Repository owner(s):** the GitHub team(s) or individuals, as `@OrgName/team-name` or `@username`.
- **Directory-specific owners,** if any: e.g. `infra/` owned by one team and `src/` owned by another.

## Default output (single owner)

```text
# CODEOWNERS file for {repo-name}

## All changes require approval from the owning team

* {owner}
```

Write each `{owner}` as the handle the user named, such as `@org/team-name` or `@username`. Write no owner that the user
did not name.

## Multi-owner output

```text
# CODEOWNERS file for {repo-name}

## Default owner for all files
* {default-owner}

## Infrastructure owned by the infrastructure owner
/infra/ {infra-owner}

## Source code owned by the application owner
/src/ {app-owner}
```

## Notes

- CODEOWNERS is valid in `.github/`, the repo root, or `docs/`, checked in that order. `.github/CODEOWNERS` is fully
  valid, idiomatic, and checked **first**.
- CODEOWNERS only auto-requests reviews from the listed owners. It does not block a merge. To enforce approval, enable
  "Require review from Code Owners" in the repo's branch protection settings.
- Order matters. The **last matching pattern in the file wins**, not the most specific one. List broad patterns first,
  such as `*`. List specific patterns after them, so the specific rule is the last match for those paths.

## Conformance check (audit mode)

✅ Conformant if the file has at least one ownership rule (`*` or path pattern) and no syntax errors.
