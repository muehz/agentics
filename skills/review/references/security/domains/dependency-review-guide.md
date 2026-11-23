# Dependency review guide

Reference for audit Subagent #4. Review changes to package manifests and lockfiles for known vulnerabilities,
supply-chain risk, and license-driven security issues.

## Scope

Trigger when the diff touches any of:

| Ecosystem | Manifest                                           | Lockfile                                             |
| --------- | -------------------------------------------------- | ---------------------------------------------------- |
| npm       | `package.json`                                     | `package-lock.json`, `npm-shrinkwrap.json`           |
| pnpm      | `package.json`                                     | `pnpm-lock.yaml`                                     |
| yarn      | `package.json`                                     | `yarn.lock`                                          |
| Python    | `pyproject.toml`, `requirements*.txt`, `setup.cfg` | `poetry.lock`, `pdm.lock`, `Pipfile.lock`, `uv.lock` |
| .NET      | `*.csproj`, `Directory.Packages.props`             | `packages.lock.json`                                 |
| Go        | `go.mod`                                           | `go.sum`                                             |
| Rust      | `Cargo.toml`                                       | `Cargo.lock`                                         |
| Java      | `pom.xml`, `build.gradle*`                         | resolved tree from build tool                        |
| Ruby      | `Gemfile`                                          | `Gemfile.lock`                                       |

## What to look for

### 1. Known vulnerabilities (CVE / GHSA)

For each newly added or upgraded package:

- Cross-reference the version against GitHub Advisory Database, OSV.dev, or the ecosystem-specific advisory feed (npm
  advisories, PyPA, RustSec, etc.).
- A vulnerability is a finding only if (a) the version range matches the advisory and (b) the project actually invokes
  the vulnerable function. Pure transitive presence is informational.
- Cite the advisory ID (`GHSA-xxxx-xxxx-xxxx`, `CVE-YYYY-NNNNN`).

### 2. Typosquatting and lookalike packages

| Pattern                                                            | Why suspect                                 |
| ------------------------------------------------------------------ | ------------------------------------------- |
| Package name differs from a popular package by 1–2 chars           | `requets`, `urllib3-secure`, `lodash-utils` |
| Hyphen vs underscore variants of a known package                   | `python_dateutil` vs `python-dateutil`      |
| New publisher with sub-week-old account                            | Account-age check via registry metadata     |
| Package version 0.0.x or 1.0.0 with high install count discrepancy | Recently created, suspiciously popular      |
| Dependency added outside the user's named-import set               | Pulled in to land a payload                 |

When in doubt, check the package's source repository, recent release cadence, and download trend. Flag the package for
human review rather than block it automatically.

### 3. Suspicious install hooks

| Ecosystem | Hook                                                                                      |
| --------- | ----------------------------------------------------------------------------------------- |
| npm       | `preinstall`, `install`, `postinstall`, `prepublish`, `prepare` scripts in `package.json` |
| Python    | `setup.py` with arbitrary code (vs declarative `pyproject.toml`)                          |
| Ruby      | `extconf.rb` with shell-out                                                               |
| Cargo     | `build.rs` that does network or shell I/O                                                 |

A new dependency with a `postinstall` script that downloads a binary or runs a network call is a finding. Flag for
review even when the script looks legitimate.

### 4. Pinning and lockfile hygiene

| Check                                                                        |
| ---------------------------------------------------------------------------- |
| Dependencies pinned (no caret or tilde where org policy requires exact pins) |
| Lockfile present and committed                                               |
| Lockfile in sync with manifest (CI should fail otherwise)                    |
| `integrity` / `resolved` entries present in npm-style lockfiles              |
| `--frozen-lockfile` / `--locked` used in CI                                  |

A diff that adds a manifest entry but does not update the lockfile is suspicious. The dependency was not tested with
deterministic resolution.

### 5. Transitive risk

Most vulnerabilities live in transitive dependencies. Triage:

- Did the diff resolve a transitive dependency to a vulnerable version (lockfile pin)?
- Did a top-level upgrade pull in a new transitive dependency that introduces a CVE?
- Does the application actually call the vulnerable function? Pure presence is Informational.

### 6. License overlap with security

License is mostly a Legal concern, but two cases overlap with security:

- **License removed or changed** to a more permissive one in a fork. This may indicate a republished package.
- **Network-copyleft (AGPL) added** unexpectedly. This is likely a policy violation and may indicate a forked or
  repackaged dependency.

### 7. Removed pins / downgrades

A diff that **removes** a pin or **downgrades** a package version needs an explanation. Common malicious-looking
patterns:

- Downgrade to a version with a known CVE
- Replacement of a pinned version with a range that resolves to a vulnerable version
- Replacement of a registry source with a Git URL or tarball URL

### 8. SBOM discipline

When the project produces an SBOM (CycloneDX, SPDX):

- The diff updates the SBOM if the dependency tree changed
- The SBOM is committed or produced in CI as an artifact
- Vulnerability scanning consumes the SBOM rather than the manifest alone.

## How to triage without running scanners

No security path in this skill runs `npm audit`, `pip-audit`, `dotnet list package --vulnerable`, or an equivalent. That
makes this one of the two weakest domains, with secrets. A clean result here says the least. Instead:

1. Read the diff for added or upgraded packages.
2. For each one, search the GitHub Advisory Database for `GHSA` advisories that match the package and version. Use the
   host's GitHub MCP tool if one is exposed. Otherwise, use `gh api`. This skill declares `Bash(gh api:*)`, so the CLI
   path is available. **Fall back to model knowledge only when both fail. When you do, say so in the finding.** A
   training cutoff bounds recalled advisories, so a knowledge-only result is the weakest evidence this guide produces.
   Never report it as a clean dependency check.
3. If the user has authorized network use, check `osv.dev` for ecosystems with a public OSV feed:
    - Make the check through the GitHub API or web search.
4. When uncertain, propose that the user run the relevant scanner and re-invoke the skill.

| Ecosystem | Scanner the user can run                                |
| --------- | ------------------------------------------------------- |
| npm       | `npm audit --omit=dev`, `osv-scanner`                   |
| pnpm      | `pnpm audit`, `osv-scanner`                             |
| Python    | `pip-audit`, `safety check`, `osv-scanner`              |
| .NET      | `dotnet list package --vulnerable --include-transitive` |
| Go        | `govulncheck ./...`, `osv-scanner`                      |
| Rust      | `cargo audit`                                           |
| Ruby      | `bundle audit check --update`                           |
| Multi     | `osv-scanner`, `trivy fs .`, GitHub Dependabot          |

## Reporting format

```markdown
**[HIGH]** {package}@{version} has known vulnerability {GHSA-xxxx-xxxx-xxxx} (CVE-YYYY-NNNNN) Manifest: `package.json`
line 27 / Lockfile: `package-lock.json` line 1502 Severity: CVSS {score} ({vector}) Reachability: {function {name} is
invoked from {file}:{line} | not invoked, transitive only} Fix: upgrade to {fixed version}
(`npm install {package}@{fixed}` or pin in manifest).
```

## False positives to filter

- Advisories that affect a function the project does not import or call
- Dev-only dependencies in a context where dev deps do not ship to production
- Advisories already silenced via the project's allowlist (`.npmrc`, `.snyk`, `osv-scanner.toml`). Verify that the
  silence is intentional.
- Test-runner packages flagged for vulnerabilities in features the test runner does not use

## Composition

Under `--composed`, return advisories in the `advisories` array of the JSON envelope. The caller merges them with
broader review output. See [../security-principles.md](../security-principles.md).
