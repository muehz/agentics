# Contributing to Agentics

This guide is for **people** contributing to this repository. It covers what lives here, how to set up locally, what to
run before opening a pull request, and how changes are versioned and described.

[`AGENTS.md`](AGENTS.md) is a different document with a different audience. It orients **AI coding assistants** working
in this checkout: the commands, where each kind of change is documented, and the facts they cannot find by reading the
code. It is not a contribution guide. The rules it points at live in this file and in the Agentics skills, and this file
does not repeat what a skill already owns.

## Scope

This repository is the `agentics` plugin: a single-plugin marketplace packaging curated skills and hooks for AI coding
assistants. What lives here:

| In scope                                     | Where                                                                                              |
| -------------------------------------------- | -------------------------------------------------------------------------------------------------- |
| Skills                                       | `skills/<name>/SKILL.md` (+ `references/`)                                                         |
| Hooks a consumer installs                    | `hooks/` — the README (one section per guardian), the surface configs, `run-hook.cmd`, `bin/`      |
| Hook source                                  | `hooks/go/` (Go module that builds the platform runtimes and Windows launcher)                     |
| Maintainer tooling and the conformance suite | `.github/tools/src/` (TypeScript)                                                                  |
| Provider manifests and marketplaces          | `.antigravity-plugin/`, `.claude-plugin/`, `.github/plugin/`, `.codex-plugin/`, `.agents/plugins/` |
| Repository documentation                     | `docs/` (see [`docs/README.md`](docs/README.md)) and the root documents                            |
| Eval scenario banks and the eval record      | `evals/` (the engine is in [`muehz/evals`](https://github.com/muehz/evals))                        |

What does **not** live here:

- **MCP server configuration.** Agentics ships none on any surface. Installing and configuring MCP servers is the
  consumer's decision. Do not add an `mcpServers` block to a manifest to make a skill work.
- **History narrated inside skill bodies.** Those describe current state only. History goes in `git log`, and every
  rename or retirement gets a row in [`docs/RENAMES.md`](docs/RENAMES.md).

Before proposing a new skill, read [`docs/CHARTER.md`](docs/CHARTER.md). It is the editorial charter for what earns a
place in the suite. A change that does not clear it will not be merged however well it is implemented.

## Directory names

Use lowercase kebab-case for documentation, skill, and work-item directories. Go package and command directories use
short lowercase names without separators, such as `secretpatterns` and `windowsraceverify`. Keep filenames required by
providers and tools, including `SKILL.md` and `AGENTS.md`, in their required form.

Shared evaluation data lives in root `evals/`; the eval engine is a sibling clone of `muehz/evals`. Maintainer
procedures live in `docs/maintainers/`, and work items follow `docs/specs/YYYY-MM/<purpose>-<topic>/`.

## Maintainer scripts

Maintainer scripts and their tests live under `.github/tools/src/scripts/`. The maintainer CLI that installs, validates,
tears down and reports on the local plugin is `.github/tools/src/plugin/cli.ts`; the install path is TypeScript on Node
([ADR 0026](docs/adr/0026-the-install-path-is-typescript-on-node.md)). Use erasable TypeScript and `node:test`, and
update callers when changing a script. See [Maintainer scripts](.github/tools/README.md#maintainer-scripts) for commands
and bootstrap details.

Scripts under `skills/*/scripts/` run on consumers' machines and must not depend on repository tooling.

## Prerequisites

| Tool                                     | Needed for                                                                        | Notes                                                                                                                                                       |
| ---------------------------------------- | --------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `git`                                    | Everything                                                                        | The maintainer CLI is Node and needs no POSIX shell, so it runs on Windows; `cloud-setup.sh` is the one maintainer shell script and runs on Linux and macOS |
| At least one host CLI                    | Trying your change in a real session                                              | `claude`, `copilot`, `codex`, or `agy`; the maintainer CLI detects whichever are installed                                                                  |
| Go toolchain (see `go.mod` files)        | Hook checks and TypeScript test fixtures                                          | `hooks/go` is the only Go module and takes no external module; the tools script tests need its pinned Go version on `PATH`                                  |
| Node.js ^22.23.5, ^24.22.3 or >= 26.12.1 | The maintainer CLI, `npm install`, prettier, release, tools and eval engine tests | Node 25 is excluded: npm 12 does not support it. Exact tool versions are pinned by `package.json` + `package-lock.json`                                     |
| npm >= 12.2.2                            | `npm install`, `npm ci`                                                           | Node 22 bundles npm 10 and Node 24 and 26 bundle npm 11, so on every line run `npm install --global npm@12.2.2` after installing Node                       |
| Installed packages                       | `--validate` and `--validate --fast`                                              | Run `npm ci --ignore-scripts` or `npm install` at the repository root; the conformance `skills` domain needs the `yaml` package                             |

The maintainer CLI runs its TypeScript entrypoint directly, so a Node older than 22.18 cannot load it: Node's own load
error appears instead of the range message. Use a Node in the range above.

The one dependency install step is `npm install`. It installs the pinned formatting toolchain (`husky`, `lint-staged`,
`prettier`), the Stryker mutation-testing toolchain, and the packages both validate modes need (`typescript` and
`yaml`), and it activates the pre-commit hook that formats staged files. Skip it and your commits go out unformatted;
the CI `format` check then fails on your PR. Both `--validate` modes also fail without the packages: the conformance
`skills` domain reports
`ERROR skills: audit could not run: the yaml package is not installed; run npm ci --ignore-scripts at the repository root`.
`npm ci --ignore-scripts` installs the same packages without activating the hook. Installing the plugin needs no
packages. The release tooling in `.github/release/` is a separate pinned package; it needs
`npm ci --ignore-scripts --prefix .github/release` before its tests run (see
[Validate Before Opening a PR](#validate-before-opening-a-pr)).

Each new `git worktree` needs its own setup. Git shares `core.hooksPath=.github/husky/_` across worktrees, but
`.github/husky/_/` is generated per checkout and ignored. A new worktree has no hooks, and Git skips them without a
warning. Run `npm run worktree:setup` from the new worktree. It downloads the Go modules, installs pinned development
tools, and generates only the `pre-commit` Husky launcher, removing launchers for other hooks. An existing worktree
drops its old launchers the next time `npm run prepare` runs there. Setup reuses the active hook path without writing
shared Git configuration, so an initialized repository can prepare new worktrees inside a restricted sandbox.

If the hook path needs activation and the sandbox blocks the write, run `npm run prepare` in the main checkout from a
terminal with write access to its `.git` directory. Then retry worktree setup. Setup fails when activation is denied or
`HUSKY=0` disables hooks, even when old hook files exist. It installs dependencies with `--ignore-scripts` and runs hook
installation explicitly, so npm's `ignore-scripts` setting cannot silently skip worktree hooks.

## Secret scanning

GitHub push protection is the check that stops a detected secret from reaching the remote. It runs on GitHub when you
push, for every machine and tool, and needs no install. It blocks the secret types GitHub detects and any custom
patterns that have push protection turned on. Agentics runs no local or CI scanner of its own. Organization
administrators can approve a push that push protection blocks. Custom patterns can be added in the organization or
repository security settings.

Read the current settings and custom patterns with these commands. Both usually need administrator access; other users
may see `null` or an error:

```bash
gh api repos/muehz/agentics --jq '.security_and_analysis'
gh api orgs/muehz/secret-scanning/custom-patterns
```

## Local Setup

Text files check out with LF even when `core.autocrlf=true`. Ordinary `.bat` and `.cmd` files use CRLF;
`hooks/run-hook.cmd` stays LF because it also runs under Bash. Binary files are excluded from normalization.

An existing checkout can retain its old line endings after `.gitattributes` changes. Save any local work first, then use
a fresh clone or worktree to pick up the defaults. To refresh the current checkout instead, confirm `git status --short`
is empty, then run `git ls-files -z | git checkout-index --force --stdin -z` in Bash. This rewrites tracked working-tree
files from the index; do not run it with uncommitted changes.

```bash
git clone https://github.com/muehz/agentics.git
cd agentics
npm install                        # pinned formatting toolchain + activates the pre-commit hook (.github/husky/)
npm run plugin:setup               # builds and stages local native files, then registers and installs the plugin
```

| Action                          | Command                                                  |
| ------------------------------- | -------------------------------------------------------- |
| Setup local dev (CLI)           | `npm run plugin:setup`                                   |
| Print VS Code settings          | `npm run plugin:vscode`                                  |
| Print VS Code Insiders settings | `node .github/tools/src/plugin/cli.ts --vscode-insiders` |
| Run the conformance suite       | `npm run plugin:validate`                                |
| Run it without the tests        | `npm run plugin:validate:fast`                           |
| Show installed plugins          | `npm run plugin:status`                                  |
| Swap CLI back to remote         | `npm run plugin:teardown`                                |

`--vscode` prints the three settings to add, check, and remove. Agentics never edits your `settings.json`, and
`--vscode` ignores `--teardown` and `--status`. The maintainer CLI does not hand your checkout to the CLIs. It copies
the checkout's tracked and untracked-but-not-ignored files into
`${XDG_CACHE_HOME:-$HOME/.cache}/agentics-local-marketplace` and registers that instead. A CLI installs a local
marketplace by copying the source directory, and in directory mode it copies everything — so pointing one at the
checkout puts `.git/`, `node_modules/`, and `.claude/worktrees/` in its install cache, 318 MB of it for 17 MB of plugin.

Your uncommitted edits still reach the install: the stage is built from the working tree, not from `HEAD`, and it is
rebuilt on every run. `--status` shows its path and size, and `--teardown` deletes it.

**Manual interrupt check (Windows).** Press Ctrl+C during a setup (`npm run plugin:setup`). Then run the setup again: it
should run, and no `.agentics-local-marketplace.lock` directory should be left in the cache root: `XDG_CACHE_HOME` if
set, else `$HOME/.cache`, and on Windows in PowerShell or cmd, where `HOME` is usually unset, `%USERPROFILE%\.cache`.
The check does not promise that an interrupt at any moment leaves either the old stage or nothing: an interrupt while
the old stage is being removed leaves it partly removed, with the lock released, and the next setup replaces it. Closing
the console window during a setup sends a hangup, which starts the same cleanup that Windows may cut short a few seconds
later, so the same check applies to it.

For a single Claude Code session, use `claude --plugin-dir <stage-path>` after setup has generated the stage. Use
`npm run plugin:status` to find the stage path. End users can install a
[verified release package](docs/guide/01-setup.md).

After installing on Codex, restart and run `/hooks` to review and trust the Agentics hooks before expecting them to run.

### Cloud environments

A fresh Linux checkout, such as a Claude Code cloud environment, has no Go and no hooks. Run
`./.github/scripts/cloud-setup.sh` as the environment setup script. The script is a bootstrap, because nothing else can
run before Node exists. It checks git, curl, tar, awk and a SHA-256 tool, then the Go version in `hooks/go/go.mod` and
the Node version in `package.json` `engines.node`, and it installs a pinned release with a checksum check when the
installed one is missing or older. It then exports `AGX_TOOLS_DIR`, `AGX_BIN_DIR` and `AGX_ORIGINAL_PATH` and hands off
to `node .github/tools/src/scripts/cloud-setup.ts`, which runs the remaining steps in TypeScript: the `engines.npm`
version when npm is older (npm comes from the registry into the tool directory, because no Node release bundles npm 12),
the optional tools, `setup-worktree.ts`, `go test -C hooks/go ./...`, and the ready summary. It stops with a message at
the first wrong tool or version. Pass `--skip-tests` to skip the tests. The checksums in the script must change when a
pinned version changes.

The environment needs network access to the Go module proxy, the npm registry, `github.com`, `go.dev`, and `nodejs.org`.
A cloud session does not have the installed plugin, your global instructions, or sibling repositories. This repository
does not need them.

### Checking the installed plugin

Each CLI keeps its own install snapshot, so one CLI can be current while another is stale. A merge leaves every CLI on
the older version until it reinstalls, and a stale install also hides new skills.

1. Read the `version` in `.claude-plugin/plugin.json`.
2. Run `npm run plugin:status`. It lists the installed `agentics@muehz` version for every CLI it detects. The install
   caches below give the same evidence without the script.
3. If any CLI differs, run `npm run plugin:setup`. Then run `/reload-plugins` in Claude Code and start a new session in
   Codex and Copilot.
4. Run `npm run plugin:status` again. The script reports a failed install as a warning and continues, so this second
   check is the proof. If a CLI still shows the old version, remove its plugin and rerun the script:
   `claude plugin uninstall agentics@muehz`, `copilot plugin uninstall agentics@muehz`, or
   `codex plugin remove agentics@muehz`.

| CLI         | Version evidence                                                                              |
| ----------- | --------------------------------------------------------------------------------------------- |
| Antigravity | `agy plugin list` (or `agy plugin list --json`), or `~/.gemini/config/plugins/agentics/`      |
| Claude Code | `claude plugin list`, or the newest directory under `~/.claude/plugins/cache/muehz/agentics/` |
| Codex       | `codex plugin list`, or the newest directory under `~/.codex/plugins/cache/muehz/agentics/`   |
| Copilot CLI | `copilot plugin list` only; Copilot keeps no versioned cache directory                        |

## Validate Before Opening a PR

Run what applies to what you touched. Every one of these has a CI counterpart, so a failure here is a failure there.

**Always:**

```bash
npm run plugin:validate      # TypeScript conformance (hooks, then the rest), then the tools tests
npm run format               # formatting (the pre-commit hook already covers normal commits)
```

The conformance suite is the repo's main gate. It runs one domain per rule area — `manifests`, `hooks`, `skills`,
`docs`, `repo` — and `--validate` runs them all with the TypeScript CLI:
`node .github/tools/src/conformance/cli.ts hooks` first, then `all --exclude hooks`, so the hooks audit runs once. The
`skills` domain also checks references between skills, skill frontmatter and repeated blocks against a per-skill
baseline of accepted findings. A change that renames a mode, flag, step or heading updates the skill's declarations and
its own baseline file; the rules are under [Skill standards baseline](.github/tools/README.md#skill-standards-baseline).
While you edit, `npm run plugin:validate:fast` is quicker: it skips the hooks module tests and coverage floor, the
`tools` test suite, and still runs the other conformance gates, the typecheck, and the area boundaries. It is for the
edit loop only and ends by saying the test suites were not run. A pull request needs the full `npm run plugin:validate`.
Both modes end with a `=== Timing summary ===` that lists each stage's result and seconds. To iterate on a single domain
without the rest:

```bash
node .github/tools/src/conformance/cli.ts <domain> --repo .
```

On a busy machine, set `AGX_TEST_CONCURRENCY=2` before running validation or `npm run test:tools` to limit the
TypeScript test suite to two test files at once. In PowerShell, use `$env:AGX_TEST_CONCURRENCY = '2'`; in Bash, use
`AGX_TEST_CONCURRENCY=2 npm run plugin:validate`. The value must be a positive integer; `1` runs files serially. Without
the override, the test runner uses at least four files, or one fewer than the available CPUs on larger machines. This
setting controls test-file concurrency, not subprocesses within a test or Go tests.

The CLI runs five domains. `all` takes repeatable `--exclude <domain>` flags. The `hooks` domain needs
`AGX_HOOK_VERIFIER`, the absolute path of a trusted verifier built from `hooks/go/cmd/hooktool`; see
[`.github/tools/README.md`](.github/tools/README.md#conformance-typescript). The `probes` selection is opt-in and
exercises installed provider CLIs.

**If you changed anything under `hooks/go/`:**

```bash
npm run test:hooks
```

**The eval engine is not changed here.** The engine lives in [`muehz/evals`](https://github.com/muehz/evals); this
repository keeps only the eval data under `evals/`. Fix an engine bug there, and file an issue in that repository for
one you cannot fix. Agentics runs the engine from a sibling clone
([`evals/README.md`](evals/README.md#the-engine-is-a-sibling-clone)).

**If you changed the release tooling (`.github/tools/src/release/`, `.github/release/`) or the PR metadata jobs or
release workflow rules:**

```bash
npm ci --ignore-scripts --prefix .github/release
node --test ".github/tools/src/scripts/*.test.ts"
npm run test:tools
```

The release proof test (`plugin.e2e.test.ts`) runs `semantic-release` against fixture repositories and needs the install
above. It needs no Go toolchain, because the plugin runs the TypeScript writer. Under `CI=true`, a test whose
prerequisite is missing fails instead of skipping.

**If you changed hook build inputs (`hooks/go/`):** rebuild with the pinned toolchain and commit the updated
`hooks/SHA256SUMS`. Git tracks the manifest and the text launcher. Generated native executables are local build outputs.

On native Windows, the conformance suite omits the `hooks/go/cmd/hook` test package because Defender can quarantine its
test executable before execution. Other hook packages and native binary checks still run. The suite explicitly reports
the omitted tests and module-wide coverage as skipped; run the complete hooks suite on Linux or macOS.

On Linux and macOS, the conformance suite's module gates enforce a 90% statement-coverage floor over the whole module.
In CI, the `Test / Go` job's `tests` leg enforces the same floor from the same profile. Reproduce the enforced aggregate
`total:` value from the same all-package profile with:

```bash
go test -C hooks/go ./... -coverpkg=./... -coverprofile=coverage.out
go tool -C hooks/go cover -func=coverage.out
rm hooks/go/coverage.out
```

Build and verify the current candidate during review.

```bash
make -C hooks/go build-all
make -C hooks/go verify
```

`build-all` writes six runtimes to `hooks/bin/`, the native Windows launcher to `bin/agx.exe`, and the runtime manifest
to `hooks/SHA256SUMS`. The Windows launcher ships in the verified release archive. The runtime manifest certifies the
separate platform assets. The default hook audit rejects tracked native executables and rebuilds generated outputs from
source. `Test / Go` rebuilds with `make -B -C hooks/go build-all` and checks for a change in the tracked manifest.

"Source" here means the source the binaries are built from. The digest skips `_test.go` files and the directories named
in `hooks/go/digest-exclude` (`hooktool`, the conformance suite, and other code no hook binary imports), so an edit
confined to those needs no rebuild. An edit to any other non-test `.go` file under `hooks/go`, to `go.mod`, or to
`digest-exclude` itself does. `go run -C hooks/go ./cmd/sourcedigest --list` prints the files the digest covers.

The freshness check also reads the Go version the linker recorded in the rebuilt binaries and requires it to satisfy the
`go` directive in `hooks/go/go.mod`. Build with a toolchain of that version, or the rebuilt binaries fail
`freshness-toolchain-drift`.

**If you added a fixture, testdata directory, or golden file:** add it to `.prettierignore` in the same commit. That
file matters because the tests compare those bytes exactly, several fixtures are malformed JSON on purpose, and prettier
errors rather than warns on them.

**If you added or changed a skill:** evaluate it and report the results in the pull request, as
[Evaluate a proposed skill change](#evaluate-a-proposed-skill-change) describes. `--validate` does not run evals. A new
or substantially changed skill also clears the introduction gate in
[`skills/forge/references/quality/skill-hardening-checklist.md`](skills/forge/references/quality/skill-hardening-checklist.md)
before stamping `eval-status: passing`. This requires a token-efficiency pass on the body, a disposition check over any
`references/`, and a pinned scenario bank in `evals/` with a real baseline eval.

`evals/skills.yaml` holds every skill's eval result: `last-evaluated` (`never` or `YYYY-MM-DD`), `eval-status`
(`passing`, `needs-revision`, or `not-run`), and, for a `passing` skill, `eval-digest`. Each directory under `skills/`
has exactly one entry, and no `SKILL.md` frontmatter holds these fields. A new skill starts at `never` and `not-run`.
The file also holds `model-ladders-reviewed` and the Copilot `settings`. Conformance rules `eval-record-valid`,
`eval-record-entry`, `eval-frontmatter-clean`, and `eval-digest` enforce this.

**Indent rules live in `.editorconfig`; `.prettierrc` carries only what `.editorconfig` cannot express.** The two merge
rather than compete — prettier reads both, and `.prettierrc` wins only for the keys it sets. Today that is one thing:
`proseWrap: "always"` at `printWidth: 120`, so prose wraps at one width instead of at whatever each author typed. Keep
indent widths in `.editorconfig`.

**The prose-wrap override skips every file with YAML frontmatter, and that is load-bearing.** `proseWrap: "always"`
folds a long `description:` into a multi-line YAML block, and the conformance suite reads that field with a same-line
extractor, so a folded value reads as missing — and hosts route on it. The override excludes `SKILL.md` and the gh-aw
source `.github/workflows/release.md`; `.prettierignore` excludes all work artifacts under `docs/specs/`. Add other new
frontmatter-bearing Markdown files to `excludeFiles` in the same commit. `prettier --check` will not catch a missing
override exclusion.

One glob trap, because it bites twice: a pattern with no `/` matches the **basename in any directory**. `*.md` therefore
matches every markdown file in the repo, and `skills/*.md` matches nothing useful. Write `**/SKILL.md`, and verify a
scoping change by counting how many frontmatter files prettier rewrites rather than by reading the glob.

### Negative tests must fail when the check is removed

A test asserting that something is rejected proves nothing until you delete the check it guards and watch the test fail.
Three shapes pass either way: `err != nil` where a later stage would also error, `Contains(err, "<literal>")` where
another failure path carries that same literal, and an assertion against a helper rather than against the code path that
calls it. Assert the distinctive message, then mutate the check and confirm the test dies with it.

### Mutation testing

The mutation jobs in [`validate-mutation.yml`](.github/workflows/validate-mutation.yml) change small parts of the code
(mutants) and runs the tests. A mutant that every test survives shows a test that does not check that behavior. The jobs
are report-only: it never fails a pull request because a mutant survived. A job fails only when a tool fails, for
example a Stryker error or a Go package that fails the gremlins baseline.

- Manual runs only: the full scope, with one merged summary. A full run takes many runner hours, so nothing starts the
  workflow on a pull request or on a schedule; dispatch it from the Actions tab when you want the report. Each Stryker
  leg reuses the saved result of a mutant whose file and covering test files are unchanged. A manual run with `fresh`
  set tests every mutant again.
- Locally: `npm run test:mutation -- <file>...` for TypeScript, and `make -C hooks/go mutation PKG=./internal/<name>`
  for Go. This is the quick check for the files a change touches.

A full Stryker run over a large area can take many hours. These options bound one run:

- `--max-minutes <n>` stops Stryker after n minutes. A stopped run has no result, so pair it with a sample.
- `--sample <n> --seed <s>` mutates about n mutants drawn from the seed. Use a new seed each run, such as the week, so
  repeated runs cover different mutants. The draw is written to `reports/mutation/sample.json`.
- `--max-test-seconds <s>` times each test file first and leaves out the files slower than s seconds, listed in
  `reports/mutation/test-cost.json`. Mutants that only those files catch report as surviving or not covered.
  `--test-times <file>` reuses that report instead of timing again.
- `--ignore-static` skips mutants that run only while a module loads. Stryker warns when they take a large share of the
  run, since each one reruns every test file.

A survivor that cannot change behavior (an equivalent mutant) goes in
[`.github/mutation/equivalent.json`](.github/mutation/equivalent.json) with `tool`, `file`, `mutator`, the trimmed
`source` line, and a `reason`. The report matches an entry on `tool`, `file`, `mutator`, and the trimmed source line. An
edit to that line stops the match. A full run lists each entry that matched no survivor under "Stale ledger entries", so
update or remove it.

Know these limits when you read a report:

- A Stryker run that you stop with Ctrl-C leaves `.stryker-tmp/` behind. Delete it before `npm run plugin:validate`,
  which reads every `SKILL.md` and finds each copy there a duplicate.
- gremlins mutates every line of a changed file, not only the changed lines. Its "mutated" cell shows `—`.
- Stryker skips the test files in `EXCLUDED_TEST_FILES` in [`stryker.config.mjs`](stryker.config.mjs). A mutant that
  only those tests would kill shows as a survivor or as not covered. The report lists the excluded files.
- The workflow installs gremlins at the version in `GREMLINS_VERSION`. The same pin is in
  [`hooks/go/Makefile`](hooks/go/Makefile). Dependabot does not update it, so a maintainer changes both by hand.

### Known host traps

A whole-repo command can fail here for a reason unrelated to the change. Scope it rather than treating the check as
unreachable:

| Command                   | What can go wrong here                                     | Scope it to                                                                                                                                                                         |
| ------------------------- | ---------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `npm run plugin:validate` | Requires the hook toolchain and installed npm dependencies | Run one domain with `node .github/tools/src/conformance/cli.ts <domain> --repo .`; `hooks` requires a trusted verifier. Fast mode skips tests and does not replace full validation. |
| `npm run format:check`    | Nothing known; an offender is a real finding               | `npx prettier --check <paths>` for the paths the change touched                                                                                                                     |

The hooks module tests run on Linux for selected pull requests, pushes and weekly runs, and on all supported operating
systems on manual dispatch. A failure there is a real finding. `rg` skips dot-directories without `--hidden`, so a bare
sweep cannot see `.claude/`.

### Healthy output

Each entry below is the output of a passing run. If a run does not show it, the check did not pass. A check can still
skip work without installed dependencies: the boundary check prints `SKIP <rule>: TypeScript is not installed` above
`boundaries: 0 failures`, and a row of the `=== Timing summary ===` reads `skipped`.

| Command                        | Healthy output                                                                                                                                                   |
| ------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `npm run test:hooks`           | `ok` for each tested package (`?` for a package with no test files), and no `FAIL` line                                                                          |
| `npm run test:tools`           | `ℹ fail 0` and `ℹ cancelled 0`                                                                                                                                   |
| `npm run test:typecheck`       | nothing after each `tsc --noEmit -p tsconfig.json` line                                                                                                          |
| `npm run test:boundaries`      | `boundaries: 0 failures`                                                                                                                                         |
| `npm run format:check`         | `All matched files use Prettier code style!`                                                                                                                     |
| `npm run plugin:validate`      | every stage row of `=== Timing summary ===` reads `passed`                                                                                                       |
| `npm run plugin:validate:fast` | `✅ Fast validation passed. The Go and TypeScript test suites were not run; ...`, and no timing summary row reads `skipped` (`tools tests  not run` is expected) |

## Area boundaries

The four areas (`hooks`, `evals`, `tools`, `skills`) may depend on each other only as
[`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md#area-boundaries) allows. To check a change, run the boundary check from
the repository root. `npm run plugin:validate` runs it too.

    npm run test:boundaries

Each failure prints as `FAIL <rule>: <path>: <message>`, and the last line is `boundaries: <N> failures`. Exit code `0`
means no failures, `1` means failures, and `2` means `tsc` could not run (an `ERROR` line says why, and it outranks any
failure). `SKIP <rule>: TypeScript is not installed` means `node_modules/typescript/bin/tsc` is missing; run
`npm ci --ignore-scripts` once to enable `area-ts-import` and `area-skill-import`.

| Rule                 | Fails when                                                                                                         |
| -------------------- | ------------------------------------------------------------------------------------------------------------------ |
| `area-go-require`    | `hooks/go/go.mod` has any `require` or `replace`                                                                   |
| `area-go-module-set` | a `go.mod` other than `hooks/go/go.mod`, or any `go.work`, exists at the root or inside the four areas             |
| `area-ts-import`     | `tools` imports a repository file outside `tools`                                                                  |
| `area-ts-dependency` | the workspace list is not exactly `tools`, or `.github/tools/package.json` names a local directory as a dependency |
| `area-skill-import`  | a skill script imports a file outside its own skill directory or a non-built-in package                            |
| `area-codeowners`    | `CODEOWNERS` lacks an entry for an area                                                                            |

The two Go rules run in the conformance `repo` domain; the other four are the TypeScript check. `hooks/go` is the only
Go module root the Go rules recognize, and for them the `tools` area is both `.github/tools` and a top-level `tools`
directory.

CI runs the cheap jobs, including the `Check` legs, on every pull request. `Test / Go` and the `ts-suite` workspaces
follow the changed areas: a pull request that touches one area runs the shards that read it, `skills` feeds the `hooks`
shard, and a path outside the four areas that is not documentation Markdown, root prose, or a plugin manifest runs
everything. Changes under `.github/tools/src/scripts/` also run everything. The table is in
`.github/tools/src/scripts/ci-areas.ts`. `CODEOWNERS` keeps its `*` line and one entry per area.

## Branching and Pull Requests

- Branch from `main`. Pull requests target `main`.
- Keep pull requests focused. A pull request should contain one coherent change. A refactor bundled with a behavior
  change is two pull requests.
- Describe **what changed and why** in the summary, and include the verification steps you actually ran along with their
  results. If a check was unavailable to you, say so rather than omitting it.
- Fill in [the pull request template](.github/PULL_REQUEST_TEMPLATE.md). Its four sections **are** this repository's
  changelog entry — the description becomes the commit body on squash. See
  [Versioning and the change record](#versioning-and-the-change-record).
- `@zacharymuehlich` owns every path via [`CODEOWNERS`](CODEOWNERS) and is requested automatically.
- Formatting is fixed for you at commit time: the pre-commit hook (`.github/husky/pre-commit`, activated by
  `npm install`) runs prettier over your staged files. CI's `format` job is check-only and never pushes. If it fails,
  your clone missed `npm install` or a commit used `--no-verify`; run `npm run format` and commit the result.

### Flow-state merges

`.gitattributes` marks `docs/specs/**/state.tsv` as `merge=union`. When two branches both append rows to the same
`state.tsv`, a local `git merge`, `git rebase`, or `git cherry-pick` keeps both sides' rows with no conflict. When both
branches created the file, Git keeps one version line and one header row. A conflict resolved by hand can repeat them,
and the conformance rule `state-tsv-shape` accepts that.

GitHub did not apply the driver when tested: on 2026-09-29 the REST merges API (`POST /repos/{owner}/{repo}/merges`)
returned `409 Merge conflict` for a pair of branches that a local `git merge` merged cleanly. Expect the merge button
and the merge queue to behave the same way; only the API was tested. When a pull request reports a conflict only in a
`state.tsv`, merge `origin/main` into the branch locally, where the driver applies, and push. Git reads `.gitattributes`
from the checked-out branch, so a branch cut before the `merge=union` line existed still conflicts on that first merge:
keep both sides' rows, remove the markers, and commit. A row edited by hand survives a union merge in both versions, so
write a correction as a new row and never edit one.

A merge never needs a re-sort. A union merge can put one side's rows ahead of the other's, and a rebase can replay a row
a second time. The `flow` helper reads the file as a set: identical rows count once, rows are ordered by `ts` with file
order as the tie-break, and the current state is the row with the greatest `ts`. File order carries no meaning, so a
replayed row or an interleaved row changes nothing a reader sees. Nothing rewrites or sorts the file, and no conformance
rule checks row order. The contract is in [`flow-state.md`](skills/execute/references/flow-state.md#what-a-reader-sees).

### Recommended PR checklist

- [ ] `npm run plugin:validate` passes (the full run; `plugin:validate:fast` does not count)
- [ ] `npm run test:hooks` passes if you touched `hooks/go`
- [ ] Any added or changed skill was evaluated and the results are in `## Verification`
      ([Evaluate a proposed skill change](#evaluate-a-proposed-skill-change))
- [ ] Hook binaries rebuilt (`make -C hooks/go build-all`) after the final rebase if hook source changed
- [ ] Formatting clean (the pre-commit hook covers this; `npm run format:check` to confirm), with any new fixture added
      to `.prettierignore`
- [ ] PR title is `<type>(<scope>)<!>: <summary>` with no version suffix; its type decides the release
- [ ] PR description's `## What changed` written
- [ ] `docs/RENAMES.md` gained a row, if anything was renamed or retired
- [ ] `README.md`, `AGENTS.md`, and `docs/README.md` updated where they now say something untrue
- [ ] `## Verification` filled in with what you ran and what it showed

## Versioning and the change record

No one picks the plugin version. A squash merge to `main` is released by semantic-release from the PR title, which
becomes the commit subject:

| Title                                                                 | Release |
| --------------------------------------------------------------------- | ------- |
| `feat(...)`                                                           | minor   |
| `fix(...)`, `perf(...)`                                               | patch   |
| any type with `!`, or a body `BREAKING CHANGE:` footer                | major   |
| `docs`, `refactor`, `test`, `chore`, `ci`, `style`, `build`, `revert` | none    |

A `revert:` title without `!` releases nothing, including one whose body carries `This reverts commit <sha>.` (what
`git revert` and GitHub's revert button write into the revert commit), because the release configuration says so. A
`revert!:` title is a major like any other type with `!`. Reverting a shipped `feat` or `fix` therefore does not ship
the revert; a separate `fix` or `feat` change does. Retitle a pull request opened from the GitHub button to
`revert: <summary>`; a title such as `Revert "feat: x"` fails the title rule.

A change to skill or hook text that should ship is a `feat` or a `fix`, never a `docs`. A retirement that does not break
an install is a `feat` without `!`. The PR metadata jobs reject a title ending in a version, a `BREAKING CHANGE:` body
line without `!` in the title, and any change to a plugin version field in the five manifests. It prints the level the
merge adds to the next release as `Release level: <none|patch|minor|major>`. The preview reads only the title and the
breaking-change footer, and the release level follows from those alone.

A major waits for the `github-release` reviewer before the release commit is pushed. The release commit changes only the
five manifest version fields, is authored by the release App, and is tagged `v<version>`. The `prepare` job verifies
that shape before anything is built from it. A maintainer dispatches the workflow when a release is wanted, as [CI](#ci)
describes.

### The pull request description is the changelog

This repository keeps no `CHANGELOG.md`. Pull requests squash into `main` with the PR title as the commit subject and
the PR description as the commit body, so `git log` is the change record. Write the entry once, in the description, and
describe the change there.

Two repository settings make this work, and both are load-bearing: `squash_merge_commit_title` is `PR_TITLE`,
`squash_merge_commit_message` is `PR_BODY`, and squash is the only merge method allowed. A merge commit or a rebase
merge would drop the description on the floor.

| Section               | Holds                                                                    |
| --------------------- | ------------------------------------------------------------------------ |
| `## What changed`     | What changed and why. **Required.**                                      |
| `## Breaking changes` | What a revert undoes and does not, what could break, and migration steps |
| `## Verification`     | What ran, what it showed, and any check that could not run               |
| `## Issue linkage`    | `Closes #N`, `Part of #N`, or `Issue linkage: none — <reason>`           |

`## What changed` has no word target: keep it as short as possible without losing granularity, and let a small visual
carry structure that prose would narrate
([`skills/gh/references/pull-requests/body.md`](skills/gh/references/pull-requests/body.md)). The
`PR / Title and description` job fails on a missing or empty section. It is the only backstop: with no changelog file, a
description nobody wrote is a change nobody recorded.

The template carries no checkboxes. The whole description becomes the commit body, so scaffolding would travel into
`git log` with it. Your pre-merge checklist is the [Recommended PR checklist](#recommended-pr-checklist) above.

Renames and retirements need one more step: add a row to [`docs/RENAMES.md`](docs/RENAMES.md) in the same pull request.
`git log` cannot answer "what happened to `wrap`?" without the commit that did it.

Changes made before 2026-08-21 were recorded in `CHANGELOG.md`. That file is readable at commit `8fc040a8`, the last
commit that contained it.

## Commit Message Convention

Agentics uses [Conventional Commits](https://www.conventionalcommits.org/) with an optional scope. The title carries no
version; the type decides the release (see [Versioning](#versioning-and-the-change-record)):

```text
<type>(<scope>)<!>: <summary>
```

Allowed types: `feat`, `fix`, `refactor`, `docs`, `test`, `chore`, `style`, `ci`, `perf`, `build`, `revert`. The
artifact areas include `hooks`, `skills`, `plugins`, `tools`, `docs`, `evals`, or a single skill name. A `!` marks a
change that breaks existing behavior for consumers.

Commits carry no AI attribution trailer. The `git` skill forbids one even when a host supplies it, and the conformance
rule `ai-attribution-trailer` rejects any text in skills, eval banks, or the published docs (`docs/guide`,
`docs/surfaces`) that asks for one.

```text
feat(hooks): secrets-guardian blocks credential reads before they happen
refactor(hooks)!: rename tool-guardian to command-guardian; block on Codex
docs: correct the registered skill count in the copilot-cli probe block
fix(skills): handoff questions read as natural English with the invocation in parentheses
```

Write the summary as a statement of what the change does, starting in lower case, with no trailing period. Proper names
and acronyms within the summary keep their usual case. The PR title becomes the squash commit subject; CI checks its
type, optional scope and breaking-change marker, and summary on every push to the pull request.

## CI

Pull requests to `main`, pushes to `main`, a weekly schedule, and manual dispatch run
[`validate.yml`](.github/workflows/validate.yml). Pull requests run only the Ubuntu jobs, and run each test suite once,
without coverage reports. A push to `main` and the weekly schedule add the coverage reports. The macOS and Windows legs
of the Go and TypeScript suites, and the `Manual` jobs, run only on manual dispatch, which keeps macOS and Windows
runner minutes, several times the price of an Ubuntu minute, off everything else. A push to `main` runs the jobs that
read the areas changed since the last push run that succeeded, and a newer push cancels an older push run. The weekly
and manual runs select every test area.

Each job name starts with its group: `Setup`, `PR`, `Check`, `Test`, `Coverage`, `Manual`, `Mutation`, or `Summary`. The
`Coverage`, `Manual`, and `Mutation` jobs live in the reusable workflows `validate-coverage.yml`, `validate-manual.yml`,
and `validate-mutation.yml`, which only `validate.yml` calls. Most events skip those groups, and a skipped group shows
as one entry instead of one per job. `Summary` is the single gate: it fails when any job's result differs from what the
event and the changed areas call for, shows the release level a pull request would ship, and lists every check on the
run page.

| Job                                          | What it does                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| PR / Title and description                   | The PR title against the commit convention, the release markers, and the `## What changed` section; reports the release level to `Summary`; pull requests only                                                                                                                                                                                                                                                                                                                                                      |
| PR / Version fields                          | Fails when a pull request changes the plugin version in any of the five manifests; pull requests only                                                                                                                                                                                                                                                                                                                                                                                                               |
| Check / Claude plugin (strict)               | The provider-native Claude Code manifest validator, pinned                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| Check / Antigravity plugin                   | The provider-native Antigravity manifest validator (`agy plugin validate`)                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| Check / Skills, Manifests, Docs, Repo, Hooks | Agentics's own repository rules (`.github/tools/src/conformance`), one leg per area. Hooks runs the Go hook auditor without the module tests: gofmt, vet, the schema, source-only executable enforcement and verification of rebuilt runtimes and the launcher, and the dispatch smoke. Skills also runs the forge MCP safe-write examples                                                                                                                                                                          |
| Check / Format                               | Prettier `--check`. It never pushes; the pre-commit hook does the fixing                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| Check / Shellcheck                           | `shellcheck -S style` over every tracked `*.sh`, except the frozen corpus copies of `plugin.sh` and `verify-stage.sh` under `hooks/go/cmd/hook/testdata/corpus/scripts/`                                                                                                                                                                                                                                                                                                                                            |
| Check / gh-aw lock drift                     | Recompiles the `release` and `backlog-grooming` gh-aw locks with pinned gh-aw and fails on any change, including an uncommitted lock                                                                                                                                                                                                                                                                                                                                                                                |
| Test / Go (os, check)                        | Every Go test, when a change reaches the hooks module (hooks or skills), pull requests included. `tests` runs `go test ./...` over `hooks/go` (`internal/flow`, the agx launcher and the hook binary included), enforces the 90% statement-coverage floor, and checks that rebuilt runtimes reproduce the tracked checksum manifest; `race` reruns the tests under `-race`; `fuzz` runs each fuzz target for 30s. Race and fuzz run only when the hooks change. Manual dispatch adds macOS and Windows `tests` legs |
| Test / TypeScript (os, workspace, shard)     | The tools npm tests, the typecheck of every npm workspace, and the area boundaries, only when a change touches their areas (`.github/tools/src/scripts/ci-areas.ts`): on Ubuntu when selected, and the tests on macOS and Windows on manual dispatch. Installs pinned Go for the tests that compile Go fixtures. No CI job runs the eval engine or its tests                                                                                                                                                        |
| Coverage / TypeScript report                 | Converts the Ubuntu lcov reports into one Cobertura report with `.github/tools/src/coverage/cli.ts lcov`, writes line and branch coverage per workspace, and the source files no test loaded, to the job summary; a second job uploads the report as code coverage; never runs on a pull request; runs on pushes to main that touch its paths, the weekly schedule and manual dispatch                                                                                                                              |
| Manual / Windows hook dispatch               | Builds the Windows `.exe` from source, then runs `hooks/run-hook.cmd` and that `.exe` through real `cmd.exe`; also covers the Windows fetch path through `fetch-hook.ps1`. Runs on manual dispatch only                                                                                                                                                                                                                                                                                                             |
| Manual / Native hook smoke (os)              | On manual dispatch only, on `macos-15-intel` and `windows-11-arm`, rebuilds the runner's own binary and checks it against `hooks/SHA256SUMS`, confirms each runner is really the labeled architecture, runs the native binary directly, then a command-guardian deny/benign dispatch smoke through `run-hook.cmd` with fetching off and the other architecture's binary removed                                                                                                                                     |
| Coverage / Go report and upload              | Converts the Ubuntu Go profile to Cobertura and uploads it as code coverage; never on a pull request                                                                                                                                                                                                                                                                                                                                                                                                                |
| Mutation / Mutate (tool, target)             | Report-only mutation testing with Stryker and gremlins, one leg per area, and `Mutation / Summary` merges the survivors; manual dispatch only                                                                                                                                                                                                                                                                                                                                                                       |
| Summary                                      | Compares each job's result with what the event and the changed areas call for, and writes the run page summary                                                                                                                                                                                                                                                                                                                                                                                                      |

When a fuzz step fails, Go writes the crashing input under that package's `testdata/fuzz/` directory on the runner, and
the job uploads it as the `fuzz-crashers` artifact. That artifact is the only copy that outlives the run — the log gives
you the path and hash but not the bytes. Download it, restore the file at the same path in your checkout, and
`go test -C hooks/go ./internal/shellwords -run 'FuzzParse/<hash>'` replays exactly that input. Commit the file
alongside the fix: seed corpus entries run on every ordinary `go test`, so the crash stays pinned as a regression.

The mutation jobs run on manual dispatch only. They are report-only, and `Summary` does not wait on them. See
[Mutation testing](#mutation-testing).

On pull requests, drafts included, the `PR / Title and description` and `PR / Version fields` jobs check the PR title
against the commit convention, the `## What changed` section, and the manifest version fields. They run on the same
events as the rest of `validate.yml`, so an edit to the title or description alone does not re-run them: push again
after editing.

Tool versions are pinned inline on purpose so an upstream release cannot turn a passing pull request red on its own.
Roll them forward deliberately, in a requirements review.

Node 26.12.1 bundles npm 11, and `engines.npm` requires npm 12. Every job that runs `npm` or `npx` therefore installs
the pinned npm 12 straight after `setup-node` (`npm install --global --ignore-scripts npm@12.2.2`).
`ci-workflow.test.ts` fails when a job runs npm without that step, or when the pinned version differs from
`engines.npm`. A change to the version updates `package.json`, `.github/release/package.json`, the workflows, and the
recompiled `release.lock.yml` together.

The [agentic GitHub Release workflow](.github/workflows/release.md) runs only when a maintainer dispatches it on `main`
(Actions, Agentics Release, Run workflow); a push to `main` starts no release, so merges accumulate until you dispatch,
and one dispatch releases every commit since the last tag as a single version at the highest level among them. It
versions the repository only when a commit since the last tag releases. Provider installs resolve `agentics@muehz` from
the user's verified release package. The workflow builds and verifies the ten release files, attests their provenance,
publishes the release, and enriches its notes with AI highlights (ADR 0010). By default the release is a real release,
marked latest, and the dispatch is the sign-off: release immutability locks the assets and tag at publication, and
nobody looks between publication and `latest`. Set the `prerelease` input to `true` to publish a non-latest pre-release
instead. Only a major release waits on a reviewer, before the release commit is pushed; no job waits on one after that.
You promote a pre-release yourself from the repository's Releases page once the run has finished — untick "Set as a
pre-release" and tick "Set as the latest release" (unless you are promoting a back-port behind a newer release) — and
that promotion is the sign-off. Before promoting, check the pre-release's assets and notes. An enriched release opens
with a highlights heading, carries the agent's highlights grouped into categories, then a provenance line naming the
run, engine, and model that drafted them, and ends with the baseline notes generated from the squash commit subjects,
collapsed in a "Full changelog" block. AI failure leaves the baseline notes in place. You can edit the notes at any
time; a release the workflow no longer recognizes as its own composition is left untouched on a rerun rather than
overwritten. A stray `untagged-*` tag can still remain from a release made under the old draft pattern;
`release context` fails on one, and the recovery is `git push origin :refs/tags/<tag>` to delete it. A bad release is
not corrected in place — release immutability locks its assets and tag the moment `stage` publishes it — it is replaced
by the next patch version. If a run fails after the release commit and tag are pushed, dispatch the workflow on `main`
again: it stages the head's tag. Staging reconciles a matching draft without overwriting assets or notes; an
already-published matching release, in either state, is verified and left unchanged, and the log says when the
`prerelease` input did not apply. The `stage` job log says which it did: `adopted draft <id>`, `created draft <id>`,
`published release <id>`, `published pre-release <id>`, `found published release <id>`, or
`found published pre-release <id>`. [`docs/maintainers/release-runbook.md`](docs/maintainers/release-runbook.md) covers
key rotation and the manual path.

`release.md` is a [gh-aw](https://github.com/githubnext/gh-aw) source file. GitHub runs the compiled `release.lock.yml`
beside it, never the Markdown, so a change to the source takes effect only once the lock file is recompiled and
committed in the same change.

The workflow itself calls one command through the local composite action `.github/actions/release`. That command is
`.github/tools/src/release/cli.ts`, TypeScript that Node runs directly, and it has seven subcommands: `set-version`,
`resume-check`, `check-commit`, `bundle`, `context`, `verify`, and `stage`. The action installs the pinned Node (the
version `release.md` pins) and runs all but `set-version`, which semantic-release's release plugin
(`.github/tools/src/release/plugin.ts`) runs in a child process when it prepares the release commit. Only `bundle` sets
up Go, to build the hook verifier.

A second gh-aw workflow, [`backlog-grooming.md`](.github/workflows/backlog-grooming.md), runs daily at 13:10
America/Chicago and on manual dispatch. It groups open issues into Feature issues labeled `agentics: feature`, links the
grouped issues as sub-issues, moves children between its own Features or closes them when a grouping changes, and
rewrites the one issue labeled `agentics: backlog-index`, creating and pinning it on the first run. Scheduled runs
apply. A manual dispatch defaults to `mode: plan`, which writes only the proposal into the index and changes no other
issue. The agent only proposes; `.github/tools/src/scripts/backlog-grooming.ts` applies the plan within per-run caps and
only edits Feature issues that carry the `agentics: feature` label. Its lock follows the same recompile rule as
`release.md`: edit the source, recompile, and commit the lock in the same change.

## Evaluate a proposed skill change

Every pull request that adds a skill or changes how one behaves must carry evaluation evidence. No automatic check runs
evals for you, so the evidence is the author's responsibility, and a reviewer may hold the merge until it is there. A
change to wording, links, or formatting that cannot change behavior needs no evaluation. When unsure, evaluate.

Evaluate the skill with one or both of these:

- **The Agentics eval engine**, which lives in [`muehz/evals`](https://github.com/muehz/evals) and runs from a sibling
  clone ([`evals/README.md`](evals/README.md#the-engine-is-a-sibling-clone)). Run the skill's pinned scenario bank
  (`evals/skills/<name>.md`) against the changed skill and against `main` with `evals cycle`; if the description
  changed, also run its trigger bank with `evals trigger` ([`evals/README.md`](evals/README.md#cli-contract)). Name the
  engine commit you ran in the evidence.
- **A coding tool's own eval tooling**, such as `claude plugin eval` for Claude Code, for behavior the engine cannot
  observe, such as routing or hook interaction inside a real host.

The evaluation must be large enough to show a regression. Run every category of the bank and repeat runs whose results
vary. A single passing scenario is not enough. A change to the bank goes in the same pull request as the skill change
([ADR 0022](docs/adr/0022-eval-strategy.md)). In the pull request's `## Verification`, name the tool, model, and
revision you ran, give the results for the candidate and the base, and explain any criterion that fell below the bar. A
drop below the bar does not block the merge by itself. The reviewer reads the evidence and decides, and a maintainer
stamps `evals/skills.yaml` only after reading it.

No workflow in this repository runs evals or writes eval-status stamps. A maintainer reviews the evidence before
stamping. There is no schedule, automatic PR evaluation, or required eval gate. This is deliberate: provider runs cost
more than PR CI's budget allows, so the evaluation [expected of every skill change](#evaluate-a-proposed-skill-change)
is run by its author.

### Capture and compare a chain baseline

`evals baseline` runs four chain skills through Copilot CLI. They are `intent`, `spec`, `decompose`, and `execute`, in
that order. `--capture` writes the verdicts and scorecards into one directory. `--compare` re-runs the four skills and
reports any criterion that fell below the bar:

```bash
node ../evals/evals/src/cli/main.ts baseline --capture evals/runs/<date>
node ../evals/evals/src/cli/main.ts baseline --compare evals/runs/<date>
```

The directory comes first and the flags come after it. Run the commands from the repository root. A relative path
resolves against the repository root, not the working directory. Pass an absolute path if that is easier.
`--model MODEL` selects the Copilot model, and the default is `EVAL_MODEL`, then `settings.copilot-model` in
`evals/skills.yaml`. `--rebanked "NAMES"` names the skills whose bank was re-authored after the capture, so compare
measures them against the absolute bar instead of the old verdict. Its default is `decompose spec`.

Raw local captures belong in ignored `evals/runs/`; do not commit them. The retired historical captures from 2026-08-30
through 2026-09-06 remain recoverable through Git history at revision `7eb59f532e0cf7dc0fa91950bf02aaf9abd9df40`, under
`.claude/evals/baselines/`.

Use `--model` and `--rebanked`, not the retired baseline script's `COPILOT_MODEL=` and `REBANKED=` variables. The
baseline command does not interpret those variables as options.

A bad argument exits 2, where the shell exited 1. A failed run or a regression exits 1 and keeps its run directory,
whose path the command prints.

### Prove a re-eval sweep

`evals proof` checks that every stamp agrees with its evidence and with the issue tracker:

```bash
GH_TOKEN=$(gh auth token) node ../evals/evals/src/cli/main.ts proof --since 2026-09-25
```

It checks four things for each skill.

- The skill's entry in `evals/skills.yaml` has an `eval-status` of `passing` or `needs-revision`. With `--since`, which
  is optional, `last-evaluated` is a real `YYYY-MM-DD` date on or after that date.
- A skill's `eval-digest` equals `evals digest`.
- Each `needs-revision` skill has an open `skill-quality` issue titled `skill-reeval: <name> stamped needs-revision`.
- No `passing` skill has such an issue open.

It prints each mismatch, then one summary line. It exits 1 on any mismatch or when it cannot list the issues or read a
skill, and 2 on a bad argument. A `--repo` with no skills is a bad argument, rejected before the issue listing. It reads
issues through the GitHub API with `GH_TOKEN` or `GITHUB_TOKEN`. `--github-repo` names the repository, and its default
is `muehz/agentics`.

## Live host checks

CI can't prove that an external coding tool loaded Agentics or honoured a hook decision. If a change affects
installation, hook dispatch, or skill routing, run the **Live check** in the affected
[surface document](docs/surfaces/README.md) and report the result in the pull request.

## Decisions and Architecture

- Read [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the marketplace design, repository layout, and loading order.
- Read [`docs/surfaces/`](docs/surfaces/) for one document per supported surface and the upstream review ledger
  recording when each provider's requirements were last checked.
- Read [`docs/adr/`](docs/adr/) for the architecture decision records.

Open an ADR when a decision is hard to reverse, surprising without context, and the result of a real trade-off. Not
every choice needs one.

## Third-Party Content

Preserve upstream licenses and attribution when changing third-party content. Keep unrelated vendored prose unchanged.

Attribution lives in [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md). Record new vendored content and substantive
adaptations there in the same pull request.

## Questions

Open an issue. For where a kind of change is documented in this checkout, [`AGENTS.md`](AGENTS.md) has the map.

## License

By contributing, you agree that your contributions are licensed under the repository's [LICENSE](LICENSE).
