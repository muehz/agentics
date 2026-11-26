# Repository readiness audit

Report what prevents reliable work in this repository now. This is inspection, not initialization. Do not write consumer
files, create directories, install dependencies, repair configuration, or start a release. Tests can emit their normal
temporary output only when that execution is authorized. Do not choose commands that regenerate tracked files or mutate
the application merely to complete an audit.

## Discover evidence

1. Establish the repository root and current working state. Make no modification. Inspect hidden configuration as well
   as visible files. A directory without Git still supports a readiness report. Mark unavailable Git/release evidence
   explicitly. Do not initialize the directory.
2. Read the applicable `AGENTS.md` files and nearest README. Then follow their setup and verification references.
   Inspect actual package scripts, build files, test configuration, CI commands, language/toolchain versions, and
   existing verification skills. Resolve relative paths from the working directory the documentation specifies.
3. Identify applicable build, test, lint, typecheck, and real-app feedback loops. Record each command, its source path,
   prerequisites, and working directory. Prefer the project's documented commands. Inspect their current targets before
   execution. Do not invent a generic command merely because a language is detected.

    For each loop, also trace its wiring from the repository's real configuration:
    - every CI job step that invokes the command (file and line)
    - every hook configuration that invokes the command
    - every package or build script that invokes the command

    A configuration file that merely exists is not an invocation. Record the selection each invocation uses: the glob,
    include and exclude lists, test filter, ignore patterns, and working directory.

4. Run applicable authorized local checks whose prerequisites are available and whose effects are appropriate for an
   audit. Read-only permission does not grant paid dispatch, credential access, dependency installation, network
   mutation, deployment, or data-changing app probes. For each run record the exact command, directory, observed exit
   result, and concise output evidence. If permission or safe prerequisites are absent, do not execute the check. Report
   that limitation and the next action needed.

## Status contract

| Status         | Required evidence                                                                                                            |
| -------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| `pass`         | The applicable requirement was inspected and met.                                                                            |
| `missing`      | An applicable required file, configured loop, or command is absent after a check of its real discovery locations.            |
| `failed`       | An inspected contract is violated or an executed check fails. Name the distinctive defect or observed result.                |
| `blocked`      | A known prerequisite prevents inspection or execution, such as a missing tool, credentials, permission, or safe environment. |
| `unverified`   | Evidence remains insufficient or the check was not run. Give the reason and what would establish it.                         |
| `inapplicable` | The item does not apply to this repository or is optional and intentionally absent. State why.                               |

`pass` also needs this evidence:

- For a runnable check, an actual successful run in this audit.
- For a feedback loop, a `ran` or `protective` Wiring value.

A missing configured loop is different from a configured loop blocked by a missing executable. An absent typecheck in a
project without a typecheck toolchain may be inapplicable. An absent required typecheck in a TypeScript project is
missing. A command found in README or a green historical CI badge is unverified until run here. Blocked and unverified
rows never count as `pass`, and no overall readiness claim may conceal them.

### Wiring column

Feedback-loop rows carry a **Wiring** value beside their status. It records how far the check reaches. The status still
uses only the six values above.

| Wiring             | Meaning                                                                      | Row status                                                 |
| ------------------ | ---------------------------------------------------------------------------- | ---------------------------------------------------------- |
| `unwired`          | runs locally, but no CI job, hook, or script invokes it                      | `failed` when the repository has CI or hooks that skip it  |
| `excludes <paths>` | an invocation exists, but its selection misses in-scope files of its module  | `failed`, with the paths named                             |
| `empty run`        | the run executed zero tests, or skipped all of them                          | `failed` when matching tests exist, otherwise `unverified` |
| `ran`              | invoked, the selection reaches every in-scope file, tests or checks executed | `pass`                                                     |
| `protective`       | `ran`, and an approved probe showed the check fails on planted input         | `pass`, with the probe evidence                            |

**In-scope files.** For each row, list the tracked files that match the runner's own naming convention within the row's
module. A module is the directory tree under a manifest that the runner reads, such as `go.mod`, `package.json`, or
`pyproject.toml`, minus any nested module that has its own row. When the runner reads no manifest, the repository root
is the module. A repository with several modules gets one row per module and runner. A file in another module is not in
scope for this row, because that module's row answers for it. Examples of a convention are `*.test.cjs` for
`node --test`, `*_test.go` for `go test`, and `test_*.py` or `*_test.py` for pytest. Compare them with what each
invocation's selection and working directory reach. A file the convention matches inside the module that a row's own
selection misses is excluded for that row. Name it, even when another invocation of the same module covers it. When the
runner's convention cannot be determined, the comparison is `unverified` with the reason, never `excludes`. For a
linter, typechecker, or build, the convention is the tool's own file patterns and project configuration. Examples are
`**/*.ts` under `tsconfig.json` `include`, or the extensions an ESLint config targets. An `empty run` for these tools
means zero files were checked.

**Observed.** Take executed, skipped, and failed counts from the run's own output. Report a disabled lint rule or a skip
marker that silences the check for in-scope files on the row. Include its configuration path.

A repository with no CI and no hooks at all gets one row, `missing: no guardrail`, instead of an `unwired` row per
command. When CI configuration is readable but its runs cannot be observed here, do these steps:

1. Record the invocation from configuration.
2. Keep the observed part `unverified`.

A local run of the same command and selection may supply the observed counts for `ran`. Say which run the counts came
from. Recommend that an existing command be wired before a new one is added.

## Inspect in priority order

### 1. Runnable feedback loops

Check build, tests, lint, typecheck, and real-app commands where applicable. Confirm that referenced scripts and config
exist, package script names still match, and the toolchain supports the documented options. For real-app verification,
inspect the launch, drive, assertion, cleanup, and evidence instructions in a repository verification skill or runbook.
Run it only when locally authorized and safe. A document that promises runtime proof is not itself runtime proof. If no
real-app probe applies to a library, explain that. If a product requires one but has none, report missing.

Every row in this category carries its Wiring value and the evidence behind it. The evidence is the invocation source,
the selection, the HEAD it ran against, and the observed counts. Availability is not verification. Keep a command that
resolves separate from a run that executed tests or checks that reached the in-scope files. `protective` comes only from
an approved probe (`scaffold audit probe`). The audit itself never plants an input.

### 2. Standard protections

Load [protections.md](protections.md). Inspect local and CI secret scans, effective default-branch rules, release/deploy
environments, protected tags, and automation/bypass behavior. Report separate rows for each scope and consumer-owned
settings option. A configured remote rule cannot hide a missing local check or an unbound release writer when each scope
has its own row. Preserve existing suitable tools and hook managers. Configuration inspection and observed current-head
enforcement are distinct evidence. When access or safe proof is unavailable, retain blocked or unverified results. This
category never installs, repairs, plants fixtures, or changes local or remote protections during an audit.

### 3. AGENTS.md

Locate applicable guidance. Do not assume a root file exists. Use scaffold's retained conformance criteria for
structure. Then check commands and references against present paths and configuration. A recent timestamp cannot rescue
a removed script or renamed package command. An old file whose paths and commands still work need not be stale. Report
malformed instructions, broken paths, or command/config disagreement as failed with evidence. Keep a command's execution
status separate from the guidance's structural status. Preserve existing author content. Name init's addition as the
row's recommended action for a missing addition. The audit makes no edit.

Resolve the glossary, context map, ADR directory, standards documents, and PR template through
[repo-conventions.md](../../routing/references/repo-conventions.md). Report each resolved source as evidence. A conflict
or a pointer to a missing path is `failed`. Name every source in the row. A convention that discovery resolves, or that
the repository does not use, passes or is inapplicable with no recommendation. Name the resolved source, or the default
the contract names for that convention when nothing resolves. Name init's `## Repository conventions` line as the
recommended action only when the contract reports one of these:

- an ambiguity
- a conflict
- a custom location discovery cannot find

### 4. Agents, skills, and MCP configuration

Inspect the repository's actual provider configuration, referenced agents/skills, and relative resources. Validate
readability, declared file paths, configuration syntax, and referenced launch commands where evidence is accessible. Do
not impose Agentics's metadata or heading conventions on ordinary consumer assets. Follow the optional code-review skill
discovery order from scaffold: `.github/skills`, `.claude/skills`, then `.agents/skills`. An unreadable or invalid first
match is a defect, not permission to fall through to another match. Keep valid freeform author content intact. Optional
absence is inapplicable with `Optional — not configured` evidence, not a compliance failure.

Inspect MCP configuration. Do not disclose secret values or contact privileged services. Configuration existence does
not prove connectivity. Missing tools or credentials block the relevant verification. Untested connectivity is
unverified. Recommend the appropriate configuration owner or `forge` for asset authoring. Do not ship an MCP server or
change provider configuration from this audit.

### 5. Governance/community files

Use scaffold's file catalog and per-file conformance criteria. Before a discoverable governance file is declared
missing, check accepted root, `.github/`, and `docs/` locations. Report every absent applicable catalog file (including
`CODEOWNERS` on GitHub/GitLab) as `missing` with an actionable next step. Report detected paths and the particular
violated criterion. Apply the repository's requirements. Optional templates and irrelevant documents are not automatic
failures. Retain catalog coverage. Do not require a changelog where repository policy prohibits one. Inspect
legal/security files only as evidence of the project's stated requirements. Decisions about their contents remain with
the organization.

### 6. Release state

Safely read current version declarations, release documentation, existing pipeline configuration, and locally available
tags or release artifacts. Compare claims to these sources. Do not infer a published release from a version string. Do
not infer remote status from local tags. Use remote read evidence only when available and authorized. Otherwise mark
that claim unverified or blocked. A repository with no distribution obligation can mark release checks inapplicable.
Report missing release guidance or observed version/config disagreement. Do not author pipelines, publish, tag, or
deploy. Route release implementation to the repository's release owner.

## Report and stop

Use `Category | Status | Path / evidence | Recommended action`, plus a **Wiring** column on feedback-loop rows. Keep the
six categories above in order: they encode the agreed priority. Within a category, order the rows like this:

1. Failed and missing findings.
2. Blocked and unverified rows.
3. Pass and inapplicable rows.

Retain more than one row when a file's structural pass and command failure differ. Every row needs a path or concrete
evidence, a reason, and an actionable next step (or `None` for a satisfied requirement).

Summarize the highest-priority impediments and what remains unrun. Name the appropriate asset or release owner in the
rows for gaps at their boundaries. When the audit found gaps that scaffold covers, end with the one yes/no offer that
[SKILL.md](../SKILL.md#readiness-audit) describes:

- init, for starter-file and protection gaps
- feedback-loops, for a missing or stale verification skill

Offer whichever covers the higher-ranked gap. A yes starts that mode on the audit's gaps without a new scan. With no
such gaps, end with the report.

The audit shows no file-selection menu or preview and writes nothing. Init's menu starts only after the yes.
