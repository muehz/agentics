# Scaffold reference: .github/dependabot.yml

## Questions to ask

Ask what's open as one ❓ round. Give each question a ➡️ recommendation drawn from the repo's manifest files. Skip a
topic the request already settles.

- **Package ecosystems in use** (select all that apply). Each name below is the exact `package-ecosystem` value that
  Dependabot accepts:
    - `npm` (Node.js/JS/TS)
    - `nuget` (.NET/C#)
    - `pip` (Python)
    - `gomod` (Go modules)
    - `terraform` (Terraform modules)
    - `github-actions` (GitHub Actions workflows, almost always included)
    - `docker` (Dockerfile base images)
    - other: use the value that the
      [Dependabot options reference](https://docs.github.com/en/code-security/dependabot/working-with-dependabot/dependabot-options-reference)
      lists for `package-ecosystem`. A name that is not on that list makes Dependabot reject the whole file.
- **Manifest directory per ecosystem** (default: `/`).
- **Update day** (default: Monday).

## Output pattern

Use this as the canonical pattern:

```yaml
version: 2
updates:
    - package-ecosystem: "github-actions"
      directory: "/"
      schedule:
          interval: "weekly"
          day: "monday"
      open-pull-requests-limit: 5
      commit-message:
          prefix: "ci"
      groups:
          actions-major:
              update-types:
                  - major
          actions-minor-patch:
              update-types:
                  - minor
                  - patch

    - package-ecosystem: "npm"
      directory: "/"
      schedule:
          interval: "weekly"
          day: "monday"
      open-pull-requests-limit: 5
      commit-message:
          prefix: "deps"
      groups:
          npm-major:
              update-types:
                  - major
          npm-minor-patch:
              update-types:
                  - minor
                  - patch
```

Repeat the same block structure for each additional ecosystem. Set `package-ecosystem` to the exact value from the
question list, such as `gomod` for Go. Set `commit-message.prefix` to match the ecosystem (`nuget`, `pip`, `go`,
`terraform`, `docker`). Name the groups after the ecosystem, such as `gomod-major` and `gomod-minor-patch`.

## Rules

- Always include `github-actions` unless the user explicitly opts out
- Group major updates apart from minor+patch updates to keep PRs manageable
- `open-pull-requests-limit: 5` is a recommended default
- Write no `labels` key unless the user names labels. Dependabot labels each PR `dependencies` and adds an ecosystem
  label by default, and it creates those labels as needed. A custom label that does not exist in the repository is
  ignored, and a custom list replaces the defaults.

## Conformance check (audit mode)

✅ Conformant if the file is valid YAML, `version: 2` is present, and at least one `package-ecosystem` entry exists.
