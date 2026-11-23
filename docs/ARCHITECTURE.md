# Architecture

> This document is for **contributors** who want to understand, extend, or modify the Agentics marketplace. If you're a
> user looking to install and use Agentics, start with the top-level [README](../README.md).

Agentics is a single-plugin marketplace. Skills, hooks, and surface adapters ship as part of the `agentics` plugin.
Agentics ships no MCP server configuration. Connecting MCP servers is the consumer's decision, made in their own
host-level or project-level config.

## Repository Layout

```text
.
├── .antigravity-plugin/
│   └── plugin.json                 # Antigravity plugin manifest
├── .claude-plugin/
│   ├── marketplace.json            # Claude Code and Copilot CLI marketplace registry
│   └── plugin.json                 # Claude agentics plugin manifest
├── .agents/plugins/
│   └── marketplace.json            # Codex marketplace registry
├── .codex-plugin/
│   └── plugin.json                 # Codex plugin manifest (root install, marketplace source.path "./")
├── evals/                         # Eval data: scenario banks (skills/), the eval record (skills.yaml), staged artifact
│                                  # sources (fixtures/), and engine-commit.txt, the muehz/evals commit Agentics was last
│                                  # validated against; the engine runs from a sibling clone
├── .github/
│   ├── PULL_REQUEST_TEMPLATE.md
│   ├── actions/conformance/        # Composite action: runs the TypeScript conformance suite in CI
│   ├── actions/release/            # Composite action: installs the pinned Node and runs one release subcommand in CI
│   ├── release/                    # Pinned semantic-release tooling: package.json, lockfile, .releaserc.json
│   ├── mutation/                   # Ledger of equivalent mutants (equivalent.json) that the mutation report drops
│   ├── plugin/
│   │   └── plugin.json             # Copilot CLI canonical component manifest
│   ├── scripts/
│   │   └── cloud-setup.sh          # Bootstrap: checks Go and Node, then hands off to src/scripts/cloud-setup.ts
│   ├── tools/                      # Maintainer-only TypeScript dev tools (the tools area), not shipped;
│   │                               # src/plugin/ is the maintainer CLI (setup, validate, teardown, status); src/scripts/ holds maintainer scripts and tests; src/peer/ is the cross-model review launcher; src/conformance/ is the
│   │                               # TypeScript conformance suite, five domains plus the opt-in probes; src/release/ is the release
│   │                               # command (cli.ts, eight subcommands) and the semantic-release plugin (plugin.ts),
│   │                               # which the release workflow runs on Node 26.12.1; src/coverage/ converts lcov and Go
│   │                               # coverage profiles to Cobertura; each has node:test suites
│   └── workflows/
│       ├── backlog-grooming.md     # gh-aw source for daily backlog grooming into Feature issues
│       ├── reminders.yml           # Opens the weekly upstream-review and monthly reference-repo review issues
│       ├── release.md              # gh-aw source for release publication
│       ├── validate.yml            # Validation entry point: PR metadata, checks, tests, the summary gate
│       ├── validate-coverage.yml   # Called by validate.yml: Go and TypeScript coverage, never on a pull request
│       ├── validate-manual.yml     # Called by validate.yml on manual dispatch: macOS and Windows hook checks
│       └── validate-mutation.yml   # Called by validate.yml on manual dispatch: report-only mutation testing
├── bin/                            # The agx launcher; selects hooks/bin/agx-<os>-<arch> under the plugin root
│   ├── agx                         # POSIX sh launcher (Bash and Git Bash)
│   └── agx.exe                     # Native Windows launcher, built from hooks/go/cmd/launcher
├── skills/                         # Plugin skills (workflow, planning, delivery, creation, operations)
├── hooks/                          # Packaged hooks and their source module
│   ├── README.md                   # Documents every shipped hook, one section per guardian, and indexes the roster
│   ├── hooks.claude.json           # Claude Code PreToolUse declaration
│   ├── hooks.copilot.json          # Copilot CLI hook config (native schema)
│   ├── hooks.codex.json            # Codex hook config (native schema)
│   ├── run-hook.cmd                # Cross-platform polyglot dispatcher — resolves the platform binary
│   ├── fetch-hook.ps1              # Windows half of the dispatcher's fetch path for an absent binary
│   ├── SHA256SUMS                  # Verification anchor for downloads, rebuilds, and releases — written by make build-all
│   ├── bin/                        # Committed platform binaries (agx-<os>-<arch>),
│                                   # one per supported platform; each serves all three hooks and the flow CLI
│   └── go/                         # Hook and flow module: builds hooks/bin/
│       ├── cmd/hook/               # Hook and flow binary entry point (Go)
│       ├── cmd/hooktool/           # Hook build and audit command
│       ├── cmd/launcher/           # bin/agx.exe source (standard library only)
│       ├── cmd/sourcedigest/       # Prints the source digest and its file set for the Makefile
│       ├── internal/               # Hook cores, shared engine, flow/ CLI and hook conformance checks
│       ├── surfaces/<surface>/     # Payload fixtures and render/normalize goldens
│       └── Makefile, go.mod, bench-baseline.txt
├── docs/
│   ├── README.md                   # Documentation index
│   ├── ARCHITECTURE.md             # This file
│   ├── CHARTER.md               # Editorial charter
│   ├── RENAMES.md                  # Renames and retirements, one row each
│   ├── adr/                        # Architecture decision records
│   ├── guide/                      # Numbered usage guide, one page per lifecycle phase
│   ├── maintainers/                # Maintainer procedures: secret-pattern updates and the release runbook
│   ├── specs/                      # Chain artifacts: docs/specs/YYYY-MM/<purpose>-<topic>/ (intent, spec, plan, state.tsv);
│   │                               # scratch and evidence go in each folder's Git-ignored temp/
│   └── surfaces/                   # Per-surface compatibility docs and the upstream review ledger
├── hooks.json                      # Antigravity PreToolUse hook config (native schema)
├── stryker.config.mjs              # Stryker mutation-testing config for the TypeScript areas, run from the root
├── AGENTS.md                       # Canonical AI-assistant instructions
├── CODEOWNERS                      # Review routing for protected paths
├── CONTRIBUTING.md                 # Contribution guide (people)
├── README.md                       # Marketplace overview
├── THIRD_PARTY_NOTICES.md          # Upstream attributions
└── LICENSE
```

The four manifest sets, `.antigravity-plugin/`, `.claude-plugin/`, `.github/plugin/`, and `.codex-plugin/`, are the
per-surface entry points. There is deliberately no `.plugin/` or root `plugin.json`. Both outrank `.github/plugin/` in
Copilot CLI's resolution order, so their absence makes `.github/plugin/plugin.json` the manifest Copilot resolves.

`AGENTS.md` is the canonical AI-assistant guidance file, and every supported surface reads it directly. The repository
keeps no `CLAUDE.md`, because Claude Code reads one instead of `AGENTS.md` when it exists. Current surface guides live
in `docs/surfaces/`.

## Area boundaries

Maintainer commands use the [supported Node and npm versions](../CONTRIBUTING.md#prerequisites).

The repository has four areas: `hooks`, `evals`, `tools`, and `skills`. Each area is the top-level directory of the same
name, except `tools`, which lives at `.github/tools` with the rest of the maintainer infrastructure. Each area depends
only on what the table allows, and a check fails when a dependency or import breaks a rule.

| Area     | Go                                  | TypeScript (`<area>/src`)                               |
| -------- | ----------------------------------- | ------------------------------------------------------- |
| `hooks`  | `hooks/go` requires no module, ever | none                                                    |
| `evals`  | none                                | none; eval data only, the engine is in `muehz/evals`    |
| `tools`  | none                                | depends on no area                                      |
| `skills` | none                                | none; skill scripts depend on nothing in the repository |

Six rules enforce the table:

- `area-go-require`: `hooks/go/go.mod` has no `require` and no `replace`.
- `area-go-module-set`: `hooks/go` is the only Go module root. Apart from `hooks/go/go.mod`, no `go.mod` or `go.work`
  sits at the repository root, under `hooks`, `evals`, `skills`, or `.github/tools`, or under a top-level `tools`
  directory. A `go.mod` or `go.work` under `docs/`, or under `.github/` outside `.github/tools`, is not checked.
- `area-ts-import`: no file in `tools` imports a repository file outside `tools` (another area, a root file, or anything
  else under `.github/`). Packages under `node_modules` are not counted. The check counts the import edges that the
  TypeScript compiler reports with `--explainFiles`.
- `area-ts-dependency`: the root `package.json` `workspaces` list is exactly `.github/tools`, and
  `.github/tools/package.json` names no local directory (`file:`, `link:`, or a `workspace:` path) as a dependency.
- `area-skill-import`: a script under `skills/<name>/` imports nothing outside its own skill directory, apart from Node
  built-ins.
- `area-codeowners`: `CODEOWNERS` has an entry for each area's directory (`/.github/tools/` for `tools`).

The Go rules run in the TypeScript conformance `repo` domain. The other four run through `npm run test:boundaries`,
which `npm run plugin:validate` also runs.

The path filter in `.github/tools/src/scripts/ci-areas.ts` decides whether a pull request or a push to `main` runs the
`go-suite` job and its race and fuzz legs, and which workspaces of the `ts-suite` job it runs. The macOS and Windows
jobs run only on manual dispatch. Each runs when any of its input areas changed:

| Job or shard                    | Runs when any of these areas changed |
| ------------------------------- | ------------------------------------ |
| `go-suite` shard `hooks`        | `hooks`, `skills`                    |
| `ts-suite` workspace `ts-tools` | `tools`, `hooks`, `evals`            |
| `go-suite` race and fuzz legs   | `hooks`                              |

The cheap jobs, including the `conformance` rule legs, run on every pull request; the boundary checks run in `ts-suite`.
A pull request that changes a path outside the four areas, other than Markdown under `docs/`, a root prose file, or a
plugin manifest, runs every shard. Changes under `.github/tools/src/scripts/` also run every shard. See
[Area boundaries in `CONTRIBUTING.md`](../CONTRIBUTING.md#area-boundaries) for the commands.

The `hooks` shard still depends on `skills`: command-guardian tests read `skills/diagnose/references/find-polluter.sh`
and `skills/scaffold/scripts/planning-protection.sh` as scanner input, and the flow helper's operands parity test reads
the Operands table in `skills/execute/references/flow-state.md`.

## What `agentics` Ships

The marketplace declares one plugin, `agentics`. It provides the routing skill, the hooks, and the shared utility
skills. Host loading-precedence rules only come into play once a consumer installs plugins of their own alongside it.
See [Cross-Platform Compatibility](#cross-platform-compatibility) below.

### Skills

Skills are organized by purpose:

- **Workflow:** `challenge` (incl. `discern` mode), `diagnose`, `hillclimb`, `principles`, `routing`, `execute`, `tdd`,
  `explain` (incl. `how`, `why`, and `teach` modes)
- **Planning:** `decompose`, `intent`, `spec`
- **Delivery:** `review` (incl. `impact` mode), `triage`, `zealot`, `docs`, `gate` (incl. `check` mode), `gh`, `git`
- **Creation:** `forge`, `scaffold`, `prototype`, `deepen`
- **Operations:** `ops`, `threat-model`, `handoff`, `recall`

### Flow state

Each work item keeps one append-only `state.tsv` in its work folder. It records the phase, the approvals, and what comes
next. Skills call bare `agx flow <command>` to write or read it: the launcher on `PATH`, `bin/agx`, or `bin/agx.exe`
from PowerShell on Windows. Where bare `agx` is not found, they run the launcher by its path under the plugin root. The
commands live in `hooks/go/internal/flow`, use the Go standard library and Git, and need no Bash or PowerShell runtime.
[Flow state](../skills/execute/references/flow-state.md) defines the columns, events, lookup rules, and command forms.
[ADR 0012](adr/0012-flow-state-is-a-verb-on-the-hook-binary.md) records the dependency on the binary shipped with the
plugin and the differences resolved during the port.

The `flow` verb runs before the hook pipeline, reads no stdin, and resolves no provider. Skills call bare `agx`, or the
launcher by path where the host does not put `bin/` on `PATH`, and the launcher selects one of the six platform
binaries; neither goes through `run-hook.cmd`, and flow performs no fetch. If the binary cannot run, the skill reports
`flow unavailable: <reason>`. Reads (`where`, `status`) use the work folder by hand. Writes (`log`, `bind`) stop and
report that the row was not recorded. The contract's
[Commands section](../skills/execute/references/flow-commands.md#commands) defines the reasons and recovery rules.

A reader treats the file as a set of rows ordered by `ts`. A rebase that replays a row or a union merge that interleaves
two branches changes nothing a reader sees. In-process Go tests use real Git fixture repositories and run through the
`hooks` shard on every supported CI OS. Existing state bytes and approval bindings are preserved. An earlier version
recorded state in `events.tsv` files, artifact headers, and a hook verb. Old work folders keep those files as history,
and nothing reads them.

### Hooks

Three hooks ship with the plugin; all run before a tool runs. Each provider config invokes one PreToolUse callback, for
6 configured command variants overall: 1 for Claude Code, 1 for Antigravity (in repository-root `hooks.json`), and 2
each for Codex and Copilot CLI. That callback runs two checks. `commands` applies the `command-guardian` rules and then
the `release-guardian` rules as one check, and `secrets-guardian` runs the credential checks. Each hook also stays
available as its own single-check command.

| Hook               | Event      | Purpose                                                                                           |
| ------------------ | ---------- | ------------------------------------------------------------------------------------------------- |
| `command-guardian` | PreToolUse | Severity-aware threat gate; blocks destructive ops, surfaces info-tier suggestions                |
| `release-guardian` | PreToolUse | Blocks production-shaped deploy commands without an existing gate change record                   |
| `secrets-guardian` | PreToolUse | Blocks a credential read into the transcript, or a secret written to disk or into a shell command |

The hooks do not gate merging a pull request, a forced push to `main` or `master`, or publishing a GitHub release.
Control of merging and forced pushes is left to the repository's own branch protection and review rules, which the hooks
do not replace; where a repository has none, or they exempt the caller (an org admin, for example), the hooks do not
gate the merge or the forced push. Publishing a GitHub release is left to the repository's permissions and any tag or
release rulesets.

Hook behavior lives in one Go binary per platform, built from `hooks/go/cmd/hook`, the three cores under
`hooks/go/internal/` (`commandguard`, `releaseguard`, and `secretsguard`), and the shared internal packages. Builds of
`hooks/go` use `CGO_ENABLED=0` and only the standard library. That module declares no third-party requirement. Flow
commands use the consumer's Git executable. `hooks/SHA256SUMS` is the verification anchor for downloads, rebuilds, and
releases; `make -C hooks/go build-all` writes it alongside the binaries it checksums. The shared `hooks/run-hook.cmd`
dispatcher resolves and execs the platform binary. When the binary is absent, the dispatcher downloads it from the
GitHub Release for the installed version and keeps it only if it matches `hooks/SHA256SUMS`; that path needs `curl` and
a SHA-256 tool on Unix, or Windows PowerShell on Windows. When no binary results, `pre-tool` and the three single-check
guard commands fail closed, and any other name exits 0. One binary serves all three hooks, and the release offers six
platform assets. `hooks/README.md` documents every hook in one section per guardian and indexes that roster.

Workflow policy rules read optional `.agentics/hooks.json` settings files, one in the repository and one in the user's
home directory, through the `settings` package. A guard resolves them only after a policy detection, and a broken file
leaves every default on. Safety rules have no key. See [hooks/README.md](../hooks/README.md#settings).

Per-surface hook configs live alongside the canonical config under `hooks/` (`hooks/hooks.copilot.json` and
`hooks/hooks.codex.json`) and at the repository root (`hooks.json` for Antigravity). Every surface config must call the
shared hook binary through `hooks/run-hook.cmd` instead of copying hook logic. Copilot CLI, Codex, and Antigravity use
native hook configs because their schemas and discovery paths differ from Claude Code.

Codex installs from the root of the verified plugin package or the separately built development stage. Its marketplace
entry (`.agents/plugins/marketplace.json`) sets `source.path` to `"./"`, the `./`-prefixed form the current official
documentation requires, and the plugin manifest lives at `.codex-plugin/plugin.json`. Repository history attributes
root-layout support to Codex CLI 0.140.0 and recommends 0.142.2 or newer. The current official-source review did not
establish those exact boundaries. Codex discovers the canonical `skills/` directory directly (it already contains
exactly the registered roster) and reads the Codex hook config via the manifest's `hooks` field
(`./hooks/hooks.codex.json`); the canonical hook binary runs unchanged through `hooks/run-hook.cmd`. The release and
development stages preserve the root layout and share the canonical skills and hook configs. The earlier committed
`.codex/` subtree and `scripts/sync-codex-plugin.sh` have been removed.

Codex exposes all Agentics-originated skills in the canonical `skills/` tree. No adapter skill or parity check is
needed.

### MCP Servers

Agentics ships no MCP server configuration and no Agentics manifest declares `mcpServers`. `gh`, the one skill that can
use an MCP server, detects whatever the host already exposes and falls back to a CLI when nothing is configured. Adding
a server is the consumer's decision, made in their own host-level or project-level MCP config.

## Cross-Platform Compatibility

Agentics has one product identity, `agentics`, but each host can have a native surface adapter. Shared hook logic lives
under `hooks/`, with per-surface hook configs under `hooks/` and at repository-root `hooks.json` (Antigravity);
per-surface compatibility docs live under `docs/surfaces/`. Antigravity, Claude Code, Copilot CLI, and Codex have
repository-local marketplace or plugin manifests. VS Code registers the verified package through its
`chat.pluginLocations` setting. See the [VS Code install guide](surfaces/vscode.md#configuration).

Claude Code and Copilot CLI read `.claude-plugin/marketplace.json` directly. VS Code Copilot Chat takes the package root
as a local plugin from `chat.pluginLocations`. It auto-detects the plugin format from the manifests at the root and at
format-specific paths (Agent Plugins 1.0, Copilot `plugin.json`, Claude `.claude-plugin/plugin.json`); Agentics has not
verified which of its manifests VS Code selects.

Copilot CLI and Codex use these repository adapter paths:

| Platform           | Repository adapter paths                                                                                          |
| ------------------ | ----------------------------------------------------------------------------------------------------------------- |
| GitHub Copilot CLI | `.claude-plugin/marketplace.json` (source "./"); its repository-root source resolves `.github/plugin/plugin.json` |
| Codex              | `.agents/plugins/marketplace.json` (source.path "./") and the repo-root `.codex-plugin/plugin.json`               |

Copilot CLI selects the first plugin manifest it finds in this exact order:

1. `.plugin/plugin.json`
2. `plugin.json`
3. `.github/plugin/plugin.json`
4. `.claude-plugin/plugin.json`

Agentics therefore keeps one Copilot component source of truth at `.github/plugin/plugin.json`. The two
higher-precedence candidates are absent, so Copilot resolves it before the lower-precedence Claude manifest.
`.github/plugin/` contains the Copilot component manifest. The `.claude-plugin/plugin.json` manifest registers
components for Claude-compatible hosts, but first-found-wins means Copilot never reads it.

**Loading order for skills differs by platform:**

- Claude Code: plugin skills are namespaced as `/plugin:skill`, so a same-named skill elsewhere loads alongside them
  rather than overriding them
- Copilot CLI: first-found-wins (earlier plugins take precedence; project-level overrides plugins)
- VS Code: undocumented. The Agent plugins page states no precedence rule for duplicate skill names (checked
  2026-09-24).

The Claude-compatible manifest references the canonical 26-skill roster.

The Codex manifest references the canonical 26-skill tree. Copilot CLI ships all 26 skills in its manifest (see
[docs/surfaces/copilot-cli.md](surfaces/copilot-cli.md)). Codex hooks remain staged. Only hooks verified against Codex
lifecycle behavior are enabled in `hooks/hooks.codex.json`.

## Local Validation

```bash
npm run plugin:validate
node .github/tools/src/conformance/cli.ts manifests --repo .
node .github/tools/src/conformance/cli.ts probes --repo . --allow-missing
npm run test:tools
npm run test:typecheck
npm run test:hooks
```

Run `npm ci --ignore-scripts` before the TypeScript commands. The conformance CLI's `--repo` defaults to the enclosing
checkout. For `hooks` or `all`, build `hooks/go/cmd/hooktool` from a trusted checkout and set `AGX_HOOK_VERIFIER` to its
absolute path; the audited root must also be absolute.

The `skills` domain also enforces artifact ownership: `skill-artifact-declared`, `skill-artifact-expected`, and
`skill-artifact-unique` check that each chain skill (`intent`, `spec`, `decompose`) declares its owned artifacts; each
artifact has exactly one owner. `spec` owns `spec.md`; `decompose` owns `plan.md`, which carries the delivery breakdown
when slicing applies. `retired-skill-reference` rejects retired names in active skill paths and references while
preserving historical records. `ai-attribution-trailer` requires the `git` skill to state its AI attribution trailer
prohibition verbatim and rejects any AI trailer instruction elsewhere in skills, eval banks, and the published docs
(`docs/guide`, `docs/surfaces`); a `Co-authored-by:` with no AI identity beside it stays allowed. `state-tsv-shape`
checks the version line, the header row, the closed sets, and the `ts` format in every committed `state.tsv` under
`docs/specs`, and accepts a repeated header and interleaved rows. `flow-emission-declared` requires each flow skill
(`intent`, `spec`, `decompose`, `execute`, `gate`, and `triage`) to name `agx flow` and link `flow-state.md`.
`skill-legacy-names` rejects the names of the retired state mechanisms (`chain state`, `internal/chain`, `events.tsv`,
and `log-row`) in any file under `skills/` except `skills/README.md`. `work-folder-pin` requires `chain-artifacts.md`
and the gate's `branch-workflow.md` to keep the sentences that define and enforce the work-folder layout.
`skill-repo-independence` rejects a skill file that names a hook, an Agentics path outside `skills/`, a plugin manifest,
or the Agentics repository. No path exception remains: any `hooks/` path in a skill fails, including a path to a
platform binary under `hooks/bin/`. Skills call the flow helper as `agx flow`, and no token in the rule matches bare
`agx` or `<plugin-root>/bin/agx`. The old hook binary name is banned everywhere. `run-hook`, hook config names, and
guardian names remain forbidden. The rule skips only the skills index, `skills/README.md`. The TypeScript suite enforces
this rule.

No conformance rule reads the untracked contents of a work folder's `temp/` directory, which holds scratch with no
content rules. A file there that Git tracks is still checked, except by `state-tsv-shape`, which skips every `state.tsv`
under `temp/`, tracked or not.

`npm run plugin:validate` runs the TypeScript conformance suite in two stages:
`node .github/tools/src/conformance/cli.ts hooks --repo <root>` and then `all --repo <root> --exclude hooks`, so the
hooks audit runs once. It needs a Go toolchain only to build the trusted hook verifier `hooktool` from `hooks/go`, which
it exports as `AGX_HOOK_VERIFIER`. It then runs `npm run test:typecheck` over the one npm workspace (`.github/tools`)
and the `node:test` suite of the TypeScript dev tools in `.github/tools/`, which need Node in the `engines.node` range
(as does the TypeScript conformance suite). The typecheck runs only when TypeScript is installed, which
`npm ci --ignore-scripts` does; otherwise the step reports it as skipped. The eval engine lives in `muehz/evals`, and
Agentics runs none of its tests. The conformance `skills` domain runs `evals check` through a sibling clone at
`../evals` (or `AGX_EVALS_ROOT`) and prints `skipped: no sibling evals clone` when there is none, which is always the
case in CI. Each run ends with a `=== Timing summary ===` of every stage. `npm run plugin:validate:fast` is the
edit-loop mode: it passes `--fast` to the `hooks` run (the domain then calls `hooktool audit --skip-tests`) and sets
`AGX_CONFORMANCE_SHORT=1` for the `all --exclude hooks` run, and it skips the tools test suite. It still needs Go (for
the hook verifier) and Node in the `engines.node` range. A pull request needs the full `--validate`. Installing the
plugin does not need a Go toolchain: the install paths run `cli.ts manifests`, the fast `manifests` domain only, and the
CLI setup path stops with one line naming the range when Node is outside it.

`.github/workflows/validate.yml` defines the validation jobs. PRs run the cheap checks and the conformance rule legs
every time. The `scope` job reads the immutable Git base, head and merge trees: only a complete, allowlisted asset-only
change can skip the test families entirely. A change whose other paths all sit inside the four areas runs only the
`ts-suite` workspaces and the `go-suite` legs that the [area table](#area-boundaries) selects. Paths outside the areas,
mode changes and uncertain history select full coverage. Each test suite runs once on a pull request, on Ubuntu and
without coverage reports. A skills-only change runs `go-suite`, whose hook tests read skills, but neither `ts-suite` nor
the race and fuzz legs: the tools tests that read the real skills tree repeat the conformance rules, which always run.
Only a manual dispatch runs the macOS and Windows jobs: it widens `go-suite` and `ts-suite` to every OS, runs
`validate-windows-dispatch` and `native-hook-smoke`, and selects full coverage without a diff. Pull requests, pushes and
the weekly schedule skip those jobs, which cost several times an Ubuntu minute. A push to `main` is classified the same
way as a pull request, from the last commit whose push run succeeded to the pushed commit, so it runs the shards that
read the changed areas, with coverage. The `scope` job finds that commit through the Actions API; with no such commit, a
failed lookup, or one that is not an ancestor of the push, the push selects full coverage. A new revision cancels an
older run of the same PR, and a newer push cancels an older push run; the next push's diff still starts at the last
validated commit, so it covers what the cancelled run left untested. Schedule and manual runs have separate concurrency
groups. The Go coverage report on `main` updates only on a push that runs every shard.

- `scope`: conservative change classification, with fixed full-coverage fallback outputs on uncertainty.

- `validate-claude-native`: Claude Code's own strict plugin validation.
- `validate-antigravity-native`: Antigravity's native plugin validation (`agy plugin validate`).
- `conformance`: Agentics's repository rules (`.github/tools/src/conformance`), one leg per domain: `Check / Skills`,
  `Manifests`, `Docs`, `Repo` and `Hooks`. The hooks leg passes `--fast` through the action's `skip-module-tests` input,
  so the hook auditor checks gofmt, vet, the schema, freshness, the byte-for-byte rebuild of the six hook binaries and
  the launcher, and the dispatch smoke, and leaves the module tests to `go-suite`. The skills leg also tests the code
  examples in forge's MCP security checklist, including on skill-only changes. Maintainers still run full validation
  locally.
- `go-suite` (`Test / Go`): every Go test of the `hooks/go` module, as matrix legs. `tests` runs the full non-short
  tests, enforces the 90% statement-coverage floor and records the coverage profile on Ubuntu, and rebuilds the six hook
  binaries and launcher and checks the resulting diff; `race` reruns them under `-race`; `fuzz` runs each fuzz target
  and uploads any crasher as the `fuzz-crashers` artifact. It runs on pull requests too. Manual dispatch adds macOS and
  Windows `tests` legs.
- `ts-suite`: the scoped, uninstrumented tools suite on Ubuntu, with the typecheck of every npm workspace and the area
  boundaries. Manual dispatch also runs macOS and Windows. Windows uses three tools shards. Each Windows shard runs two
  test files at once by default. A manual dispatch can select four for a timing comparison. The pinned Go toolchain
  builds the tools fixtures. No job runs the eval engine or its tests.
- `coverage-report`: converts the hooks coverage profile to Cobertura with
  `.github/tools/src/coverage/cli.ts goprofile`. It holds a read-only token, fails on a missing profile, reports
  coverage and gates nothing.
- `coverage-upload`: uploads that report as code coverage for same-repository pull requests and default-branch runs.
  Fork pull requests skip the job. Manual dispatches on other refs retain the report artifact and skip the provider
  upload. It gates nothing.
- The **ts-coverage** jobs run one Ubuntu tools shard under instrumentation. Its report requires every shard to succeed
  and produce LCOV before it combines the coverage and writes the workspace summary. The upload uses a separate label
  and retains the same manual-ref restriction as Go coverage. They run on pushes to main that touch their paths, on the
  weekly schedule, and on manual dispatch, never on a pull request. The ts-suite job runs the regular tests
  independently of them.
- `format`: a check-only Prettier run.
- `shellcheck`: the pinned shell-script linter.
- `gh-aw-lock-drift`: the gh-aw source-to-lock consistency check for `release.md` and `backlog-grooming.md`.
- `validate-windows-dispatch`: the real `cmd.exe` hook dispatch path. It rebuilds all six hook binaries and the
  `bin/agx.exe` launcher and fails on `git diff --exit-code -- hooks/ bin/`. It also verifies the Windows host binary's
  recorded checksum, and runs the find-polluter and planning-protection test tables under Windows PowerShell 5.1. The
  job runs on manual dispatch only. The `peer` dev tool in `.github/tools/src/peer` needs no job of its own: its
  `node:test` suite runs in the `ts-suite` job's `ts-tools` leg.
- `native-hook-smoke`: on manual dispatch only, on `macos-15-intel` and `windows-11-arm`, builds and checksum-verifies
  the host binary, confirms the runner's architecture, and smokes the native binary and `run-hook.cmd` dispatch.
- `pr-metadata` and `pr-version-fields`: pull-request-only checks of the title, the description, and the manifest
  version fields. `pr-metadata` passes the release level it previews to `validation-summary`.
- `coverage`, `manual`, and `mutation`: call jobs for `validate-coverage.yml`, `validate-manual.yml`, and
  `validate-mutation.yml`. Each groups jobs that most events skip, so a skipped group shows as one entry. `coverage`
  holds the Go and TypeScript coverage jobs and never runs on a pull request; `manual` holds `validate-windows-dispatch`
  and `native-hook-smoke`; `mutation` holds the report-only mutation jobs. The last two run on manual dispatch only.
- `validation-summary`: compares actual outcomes with the selected job inventory; missing or unexpected skipped work
  fails the summary. The PR-only jobs are expected to skip on backstops. `go-suite` is expected to skip on pull
  requests. `manual` is expected to skip on every event but a manual dispatch, where it succeeds. It does not wait on
  `coverage` or `mutation`, which gate nothing. Its run page summary leads with the verdict, shows a pull request's
  release level, and lists each check under its display name.

Every display name starts with its group (`Setup`, `PR`, `Check`, `Test`, `Coverage`, `Manual`, `Mutation`, `Summary`)
and a `/`, the separator GitHub already puts between a call job and the jobs it calls. A matrix job's name carries no
matrix expression, so GitHub appends the matrix values when it runs and shows the plain name when it is skipped.

PRs receive read-only cache access; push, scheduled, and manually dispatched runs on `main` may save caches. A manual
cold run disables caching. Cache identity includes the relevant Go module files, with OS, architecture and toolchain
separation supplied by the pinned setup action. Selected Go tests use `-count=1` so compilation reuse does not replace
test execution.

### Release boundary

The `release-candidate` artifact is retained for thirty days. This does not change the downstream asset verification,
rebuild or attestation requirements described below; a missing candidate cannot bypass them.

The maintainer release workflow computes the plugin version, assembles a verified, attested release, and publishes it
when a maintainer dispatches it on `main` (ADR 0010). By default that is a real, latest release; a `prerelease` dispatch
input publishes a non-latest pre-release that the maintainer promotes by hand. The version is a function of the squash
subjects since the last tag ([ADR 0007](adr/0007-release-version-is-computed-from-subjects.md)). The `level` job reads
the head and computes the level, a computed major waits at `approve-major`, and the `release` job runs the pinned
semantic-release from `.github/release/`. That run writes the five manifest version fields, commits them as
`chore(release): vX.Y.Z [skip ci]`, tags the commit, and pushes with a GitHub App token that bypasses the `main`
ruleset. The token is minted after `npm ci --ignore-scripts` and reaches only the semantic-release step and the step
that records the result.

The workflow runs `.github/tools/src/release/cli.ts`, whose subcommands are `set-version`, `resume-check`,
`check-commit`, `bundle`, `asset-roster`, `context`, `verify`, and `stage`. Node runs the TypeScript directly, with
imports limited to `node:*` modules and relative files. The release action installs Node 26.12.1 (SHA-pinned
`setup-node`, skipped when an earlier call in the same job already installed it). The semantic-release plugin
`.github/tools/src/release/plugin.ts` calls `set-version` in a child process whose environment holds only `PATH`,
`HOME`, `USERPROFILE`, `SYSTEMROOT`, `TMPDIR`, `TEMP`, and `TMP`, so the App token never reaches the writer. The
workflow uses the local composite action `.github/actions/release` to run release steps. That action runs the subcommand
its `command` input names. Only `bundle` sets up Go, to build the hook verifier. The action follows
`.github/actions/conformance`.

`release set-version` writes the computed version into the five manifests; `bundle` verifies the tag against them.
`release resume-check` reports whether a run analyzes the subjects, resumes an unreleased release commit at the head, or
has nothing to do, and it warns on an older tag with no published release. `release check-commit` runs in `prepare`,
which holds no token, and verifies the release commit's single parent, its author and committer name, that it changes
only the version values of the five manifests, and the tag order. It runs from the pushed head, never from the release
commit it verifies, and from that commit's first parent on a resumed run. `release bundle` builds the exact Agentics
asset set, including the six hook binaries, built at the target commit and never read from git; it refuses unless that
build reproduces `hooks/SHA256SUMS` at that commit. `release context` reads the baseline notes that semantic-release
wrote, or the squash subject list when a resumed run has none, then fetches the PR descriptions the squash subjects in
that range name. The range runs from the highest-version published release of either kind that is an ancestor of the
verified target, whether or not a pre-release was ever promoted. `context` fails when any published release still
carries GitHub's `untagged-` placeholder tag, or when any `untagged-*` tag exists on the remote, and prints the recovery
steps. `release stage` creates the `vX.Y.Z` tag if it is absent, creates or adopts the matching draft, uploads missing
assets, verifies every one, then publishes the release in one PATCH that pins `tag_name` and `target_commitish` so a
placeholder cannot reach the publish. By default the PATCH clears the pre-release flag and sets `make_latest`, and
`stage` then reads `releases/latest` until it names the release, failing the run if it never does; with
`--prerelease true` it publishes a non-latest pre-release and never reads `latest`. A release already published, in
either state, is verified and left unchanged. No subcommand promotes.

Before `stage` runs, the `stage` job uses `release asset-roster` from the target checkout to name the nine derivable
assets. It independently rebuilds the six hook binaries and the launcher at that commit, verifies the runtime manifest,
and compares each re-derived asset with the candidate from `prepare`. It reconciles the completed comparisons against
the target's asset registry, then verifies the full ten-file roster and its `SHA256SUMS`. Attestation with Sigstore
comes before the first upload because immutable releases lock the assets and tag at publication. A same-SHA rerun finds
the attestation by digest and does not create a second.

The notes agent submits optional highlights through the `update_release_notes` safe output. After threat detection
succeeds, that job copies the approved agent envelope and the workflow's staged flag into the same-run
`release-notes-handoff` artifact. The upload fails if the name is already occupied. The job has no repository write
permission.

The `write-notes` job requires successful staging, detection, and handoff. It downloads the release identity by the
stage upload's artifact ID from the current run. Before checkout it checks the file's SHA-256 digest and compares its
commit SHA with the verified target, both supplied directly by `stage`. It checks out that target and passes the
identity file and checked SHA to `.github/tools/src/scripts/release-notes.ts`. The adapter validates the record before
any staged or no-item exit. For an update, it reads the recorded release ID in the workflow repository, checks the
remote identity, and changes only the body. It leaves the release alone if its pre-release state has changed, as when
someone promoted it. It composes that body in four parts: a highlights heading the adapter owns, the agent's categorized
sections beneath it, a provenance line naming the run, engine, and model that drafted the notes, and the baseline notes
last, collapsed in a "Full changelog" block. The adapter cannot select a release or change its publication state. It
recognizes a body it composed by shape rather than by which run wrote it or how its agent phrased the highlights, so a
re-dispatch — even one whose agent worded things differently — is a no-op, and only a body it does not recognize, such
as a hand edit, is left alone rather than overwritten. Unlike gh-aw's built-in `update-release`, this adapter reads the
release by the ID the `stage` job published, so it cannot select a different release that carries the same tag.

The pattern that recognizes the provenance line is defined once, in `.github/tools/src/release/notes-contract.ts`, which
no subcommand reads, and the adapter imports it. The heading is written twice, once in the adapter that composes it and
once in `notes-contract.ts`, and the tests of each pin it to the same text. The engine and model are written twice in
the workflow, in its engine block and in the `NOTES_ENGINE` and `NOTES_MODEL` values the adapter reads. The release
engine-model test checks that they agree.

The conclusion reports the handoff and writer results separately, including a writer failure after a successful handoff.
A failed or absent notes update leaves the baseline available. The dispatch is the sign-off (ADR 0010). In pre-release
mode the maintainer promotes the release from the Releases page, clearing "Set as a pre-release" and setting "Set as the
latest release". Release immutability locks the assets and tag at publication, inside the `stage` job, so it must remain
enabled in the repository settings; nothing after `stage` publishes can change the assets or the tag. A rerun that finds
an already-published release, in either state, skips enrichment. Workflow concurrency serializes each run.

Agent-written highlights reach a published release before any person reads them. That is accepted: threat detection must
succeed and the 12000-byte cap applies before the adapter writes, and a wrong body is edited by hand afterward. In
pre-release mode the release is never marked latest, and the maintainer reads the notes when promoting (#698).

The `level` job, preparation, and the agent have read-only repository permissions. The `release` job's own token is
read-only as well; it writes the release commit and tag only through the App token. The `stage` job, which publishes the
release, holds `contents: write` and the attestation permissions. The notes writer holds `contents: write` and sets up
the pinned Node runtime for the dependency-free notes adapter.

## References

- [AGENTS.md](../AGENTS.md) - Canonical AI-assistant instructions
- [README.md](../README.md) - Marketplace overview
- [docs/README.md](README.md) - Documentation index
- [CHARTER.md](CHARTER.md) - Editorial charter
- [THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md) - Third-party attributions
