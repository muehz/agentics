# Tools

Maintainer-only dev tools. Nothing here ships in the plugin.

## Maintainer scripts

Maintainer scripts and their tests live in `.github/tools/src/scripts/`. Use erasable TypeScript and `node:test`. Follow
the repository's [supported Node and npm versions](../../CONTRIBUTING.md#prerequisites). Node runs the files directly
without a build step. Update script callers and fixtures when changing them. The `node-test.ts` launcher runs the tools
workspace tests with at least four test files at once.

Run `npm run test:tools` and `npm run test:typecheck` for changes. The tools suite needs the Go version pinned in
`hooks/go/go.mod` for script test fixtures. Go mutation baselines set `GOTOOLCHAIN=local`, so put the pinned Go
executable first on `PATH`. CI also runs `node --test ".github/tools/src/scripts/*.test.ts"` in a separate job on every
event. `setup-worktree.ts` prepares a new worktree; cloud setup provisions a suitable Node runtime before running its
typed version check and worktree setup.

Scripts under `skills/*/scripts/` run on consumers' machines and must not depend on repository tooling. The root
`stryker.config.mjs` remains Stryker's external entrypoint and delegates to the typed configuration.

## plugin (maintainer CLI)

`.github/tools/src/plugin/cli.ts` is the maintainer CLI behind setup, teardown, status, `--validate` and the VS Code
instructions. Node runs it directly, and every mode needs Node in the `engines.node` range. Its modules sit beside it:

- `args.ts` parses the argument list into an action, a validate mode and a VS Code variant.
- `common.ts` holds the checkout root, the marketplace name, the cache paths and CLI detection.
- `floor.ts` checks the running Node against `engines.node` and prints one line when it fails.
- `validate.ts` runs the `--validate` stages (hook verifier, conformance, typecheck, tools tests, boundaries).
- `stage.ts` builds, verifies and removes the clean marketplace stage that each CLI installs from.
- `marketplace.ts` registers the stage on each detected CLI, swaps back on teardown and lists status.
- `vscode.ts` prints the VS Code settings to write by hand.

`.github/tools/src/entrypoint/` holds the CLI's end-to-end tests. `testing/harness.ts` builds the fixtures and fake
CLIs, and `testing/shell-support.ts` holds the test PATH and POSIX quoting helpers.

- `contract.test.ts` covers the CLI's modes, messages and exit codes.
- `stage.test.ts` covers the stage code against real directories.
- `harness.test.ts` covers the fixtures themselves.
- `interpreters.test.ts` covers probing for `sh`, `bash`, `pwsh` and `powershell`.
- `conformance-action.test.ts` covers the conformance action's steps and cache contract.
- `polluter.test.ts` is a case table for `skills/diagnose/references/find-polluter.sh` and its twin `find-polluter.ps1`.
  It runs every row against the `.sh` under `sh` and against the `.ps1` under PowerShell, so it runs on Windows too. See
  [the twin tables](#twin-script-tables) for how each leg picks its interpreter.

## conformance (TypeScript)

`.github/tools/src/conformance` is the TypeScript conformance suite. Its five domains are `docs`, `hooks`, `manifests`,
`repo` and `skills`. `probes` installs the plugin into a provider CLI, so it runs only when you name it and `all` leaves
it out. Node runs the files directly (Node in the `engines.node` range of `package.json`, no build step).
`npm run plugin:validate` and the conformance action run this suite. Validation builds the trusted hook verifier, runs
`hooks`, then `all --exclude hooks` so the hook audit runs once. The action uses its `auditor` input to select the
trusted checkout supplying the CLI and verifier.

    node .github/tools/src/conformance/cli.ts docs --repo .
    node .github/tools/src/conformance/cli.ts skills --repo .
    node .github/tools/src/conformance/cli.ts repo --repo . --short

No rule reads the untracked contents of a work folder's `temp/` directory (`<YYYY-MM>/<purpose>-<topic>/temp`).
`.github/tools/src/conformance/work-folder-temp.ts` is the one definition of that directory, and every walk that starts
at the repository root uses it. A walk still visits a file there that Git tracks, except that `state-tsv-shape` skips
every `state.tsv` under `temp/`, tracked or not. The `repo` domain lists tracked files on every run; the `skills` and
`manifests` walks list them only when they first reach such a directory.

The CLI keeps exit codes 0, 1 and 3. `--short` selects short mode, as does `AGX_CONFORMANCE_SHORT=1`. `--fast` implies
short mode and skips the hooks module tests. `all` accepts repeatable `--exclude <domain>` flags and otherwise runs all
five domains in name order. The CLI exits 1 for rule failures, audit errors and invalid arguments.

`hooks` runs the trusted hook verifier that `AGX_HOOK_VERIFIER` names; the verifier and the `--repo` path must be
absolute, or the domain ends in an audit error. `probes` installs the checkout into the Copilot CLI under temporary
`COPILOT_HOME` and cache directories and checks the installed plugin and skills. It needs `copilot` on `PATH`; with
`--allow-missing` a missing CLI is a skip instead of an audit error.

`--stale-review-exit-code` applies to `docs` only. It exits `3` when `docs` failed and every failure is a stale
`review-age`, and `1` for any other failure, an audit error, or a mix. `AGX_LEDGER_MAX_AGE_DAYS` arms the ledger's age
check: unset or empty is off, a negative value is off, and a value that is not an integer is an audit error.

The `eval-check` family of `skills` runs `evals check` as a child process from the `muehz/evals` clone beside the
suite's own checkout: `../evals`, found from this module's location and never from `--repo`, or the directory
`AGX_EVALS_ROOT` names. The checkout being audited therefore cannot supply the script. When that directory does not
exist the family prints `skipped: no sibling evals clone` to stderr and reports nothing, so CI, which has no clone,
never runs the eval rules. A clone that exists but lacks `evals/src/cli/main.ts` or its `node_modules` is an audit
error: run `npm ci --ignore-scripts` in the clone. `evals/engine-commit.txt` records the engine commit Agentics was last
validated against. `AGX_CONFORMANCE_EVALS_CLI` replaces the script with the path it names, resolved against the working
directory. It is a test seam; the audited checkout cannot set it and CI does not.

### Skill standards baseline

The `skill-standards` family reads a baseline of accepted findings from
`.github/tools/src/conformance/skill-baseline/<skill>.json`, one file per skill that has findings. Each entry names a
rule, a repo-relative path under that skill and a key. One entry covers every finding with the same rule, path and key
in that file, so a reference repeated on several lines needs one entry. A finding that was fixed but whose entry stays
in the file fails as `skill-baseline-stale`, at the baseline file's path; a baseline may never name that rule. A sweep
that changes a mode, alias or flag updates the skill's `metadata.modes`, `metadata.mode-aliases` or `metadata.flags`,
and a sweep edits only its own skill's baseline file. The step labels a reference may name come from the headings and
bold lead-ins of any Markdown file of the target skill, not only its `SKILL.md`.

### Shell data in skill templates

The skills audit reports named data placeholders embedded in shell examples and inline Git, GitHub CLI, and `agx`
commands. Pass values from the user, filesystem, or another tool as separate arguments, through a quoted expansion, or
through a data file. Keep the command text fixed.

`src/conformance/skill-shell-reviewed.json` records safe exceptions with the path, heading, command line, occurrence
count, and a reason. Use it when the workflow already constrains the value to a safe form. When the occurrence count
changes, the audit reports a stale entry so the changed example can be reviewed again.

### Skill file size

The skills audit caps every git-tracked file under `skills/` at 40,000 bytes, so that a whole file read by Codex is
never cut. The cap runs outside the standards family, so no `skill-baseline/` entry can accept a finding.

`src/conformance/skill-size-allowlist.json` lists the files that were already over the cap, each with its path and a
ceiling equal to its size when it was listed. A listed file may shrink but not grow past its ceiling. Entries only
shrink: lower a ceiling when the file shrinks, and remove the entry once the file is within the cap. An entry whose file
is no longer tracked, or is within the cap, fails the audit as stale.

### Shell allowlist

The `repo` domain's `shell-allowlist` rule fails every tracked shell or PowerShell file that
`src/conformance/shell-allowlist.json` does not list. A file counts by its extension (`.sh`, `.bash`, `.ps1`, `.psm1`,
`.cmd` or `.bat`), or, when it has none, by a first line that is a `#!` naming `sh`, `bash`, `zsh`, `pwsh` or
`powershell`, directly or through `env`.

The allowlist is a JSON array of `{ "path", "reason" }` objects. `path` is the file as Git spells it and `reason` is one
line saying why the file is not TypeScript. An entry that no tracked file matches fails as `shell-allowlist-stale`:
remove it when its file is deleted or moved. A missing allowlist reads as empty, so deleting the file silences nothing,
and a malformed one is an audit error.

When you add a shell or PowerShell file, port it to TypeScript, or add an entry with the reason it must stay.

## peer

`peer` asks a model from another family to review a committed git range. It is a TypeScript tool that Node runs directly
(Node in the `engines.node` range of `package.json`, no build step). It runs on Linux and macOS only. On Windows it
exits 1 with a message, because it needs POSIX process groups and `ps`.

    node .github/tools/src/peer/cli.ts --target codex --dry-run --summary "smoke test"
    node .github/tools/src/peer/cli.ts --target codex --base "$(git merge-base main HEAD)" --head HEAD --summary "review this branch"

| Flag                  | Meaning                                                                                        |
| --------------------- | ---------------------------------------------------------------------------------------------- |
| `--target <t>`        | `auto` (default), `all`, `claude`, or `codex`. `all` is Claude and Codex.                      |
| `--base`, `--head`    | The range. Defaults `HEAD~1` and `HEAD`. There is no three-dot mode: pass a merge base.        |
| `--plan`, `--summary` | A plan file (its first 240 lines travel in the prompt) or a one-line intent.                   |
| `--timeout <s>`       | Seconds per review. Default 300. A positive integer.                                           |
| `--output <path>`     | A findings file (one target) or a directory (several). Default is under `docs/reviews/peer/`.  |
| `--exclude <glob>`    | Add a diff exclusion. Repeatable. Lockfiles and generated directories are excluded by default. |
| `--force`             | Dispatch even when the post-exclusion diff is over `PEER_MAX_DIFF_BYTES` (256000).             |
| `--dry-run`           | Print what would be dispatched and the prompt, and run nothing.                                |

`auto` picks the other family from the host's marker variable: Claude Code (`CLAUDECODE=1`) dispatches to Codex, Codex
(`CODEX_CI=1`) to Claude, and Antigravity (`ANTIGRAVITY_AGENT=1`) to both. With none of these set, `auto` exits 1 and
asks for an explicit `--target`. The review ends with exactly one `REVIEW-COMPLETE:` line, and the tool rejects output
without it.

Environment: `PEER_MAX_DIFF_BYTES`, `PEER_PREFLIGHT_TIMEOUT` (15), `PEER_HEARTBEAT_INTERVAL` (30, `0` turns the ticker
off), `PEER_CLAUDE_MODEL`, `PEER_CLAUDE_FALLBACK_MODEL`, `PEER_CODEX_MODEL`, and the launcher overrides
`PEER_REVIEW_LAUNCHER` (Claude) and `PEER_REVIEW_CODEX_LAUNCHER` (Codex). An empty override disables that target.

Exit codes: `0` success, `1` any other failure (a reviewer that failed or cannot start, a missing platform, a git
preflight refusal such as not a repository, a bad revision, or a diff over the size limit, an output path or manifest
that cannot be written, and the Windows refusal), `2` usage error, `3` nothing to review, `4` output validation failed,
`124` timed out. A run of one target exits with that target's code. A run of several targets exits `1` when any target
failed, and `manifest.json` records each target's code. When the launcher itself receives `SIGINT`, `SIGTERM`, or
`SIGHUP`, it stops every reviewer process group it still owns (TERM, then KILL after five seconds) and exits `128` plus
the signal number (`130`, `143`, `129`); it starts no further reviewer. A `SIGKILL` cannot be caught, so it still leaves
the reviewers running.

Output goes to `docs/reviews/peer/<utc-timestamp>-<pid>-<range>/<target>.md` with a `manifest.json` beside it, where the
timestamp is compact UTC such as `20261021T055326Z`. The one exception is a single target with `--output <file>`: that
writes exactly that file and no manifest. Those reports are unstaged workspace artifacts and are ignored by Git.

Each reviewer runs in its own process group under a scrubbed environment. The tool passes a target only its own vendor's
credentials, so the host session token never reaches another vendor.

### Edge cases

- `--timeout` takes a positive integer only; `0` is a usage error, not a way to switch the limit off.
  `PEER_PREFLIGHT_TIMEOUT` follows the same rule: `0` or a non-integer warns and uses 15.
- A non-integer `PEER_MAX_DIFF_BYTES` or `PEER_HEARTBEAT_INTERVAL` warns and uses the default.
- A single `--output <file>` in a directory that does not exist creates the directory.
- A relative `--output` is made absolute everywhere it appears: the manifest, the summary, the path handed to Codex's
  `--output-last-message`, and the validation messages.
- A launcher that cannot start prints `ERROR: cannot run <path>`. The version probe's other diagnostics are not shown.
- If the Claude review file cannot be written, the run exits 1 before validation (also after a timeout).
- An `--output` path that is not a regular file (a directory, for one) for a single target fails validation (exit 4 for
  Codex, exit 1 for Claude) without a stack trace.
- A manifest that cannot be written is one `ERROR: cannot write` line and exit 1, and no reviewer starts when the first
  write fails.

### Why there is no hook-trust bypass

Codex has a flag, `--dangerously-bypass-hook-trust`, that runs hooks without the persisted-trust check. A headless run
needs no trust prompt answered, so this tool does not pass it (issue #1090). On 2026-10-20, with Codex 0.163.0 on
Windows, a throwaway `CODEX_HOME` held one untrusted user-level `PreToolUse` hook that appends to a file outside the
workspace. Three runs of `codex exec -m gpt-6.1-sol --sandbox read-only` each finished in 7 to 8 seconds with exit 0.
One run had no hook installed and was the control:

- without the flag, the untrusted hook did not run, and Codex neither prompted nor hung;
- with the flag, the hook ran and wrote its file outside the workspace, so the read-only sandbox does not contain hook
  commands.

So the flag was never needed to avoid a prompt, and it only widens what a "read-only" review can run. The probe covered
one operating system, one Codex version, and a user-level hook; it did not cover a macOS keychain, a persisted-trust
prompt, or a hook installed by a plugin. The tool never passes `--dangerously-bypass-hook-trust`, and `source.test.ts`
fails if that flag or `--dangerously-skip-permissions` appears in a tool source.

### Tests

    npm run test:tools
    npm run test:typecheck

The suite runs the real CLI against fake `claude` and `codex` launchers, so it spends no model tokens. The tests that
start the CLI or a process group need Linux or macOS and skip on Windows. The unit tests of the configuration, the
environment allowlist, the review check, the manifest text, and the source scan, and the Windows refusal test, run
everywhere. `npm run plugin:validate` runs `npm run test:tools` and `npm run test:typecheck`, and ends with a timing
summary of each stage. `npm run plugin:validate:fast` skips `npm run test:tools` but still runs the typecheck, which
covers the one npm workspace (`.github/tools`). The typecheck runs only when TypeScript is installed, so run
`npm ci --ignore-scripts` once at the repository root to enable it.

## release

`release` holds the commands the release workflow runs, in TypeScript that Node runs directly (Node in the
`engines.node` range of `package.json`, no build step; the workflow pins 26.12.1). The workflow and `semantic-release`
configuration run `cli.ts` and `plugin.ts` ([ADR 0011](../../docs/adr/0011-release-automation-is-typescript.md)).

    node .github/tools/src/release/cli.ts <command> --flag value ...

- `set-version --repo ROOT --version MAJOR.MINOR.PATCH` writes one version into the four manifests by textual splice,
  and refuses a version that is not greater than the current one.
- `resume-check --checkout DIR --sha SHA --published FILE` decides whether a head is `analyze`, `resume` or `released`,
  and writes `mode` and `resume_tag`.
- `check-commit --checkout DIR --sha SHA --tag TAG --author NAME [--parent SHA]` verifies the shape of the release
  commit.
- `verify --assets DIR --tag TAG` checks the complete ten-file release roster and its `SHA256SUMS`.
- `bundle --repo DIR --sha SHA --main-ref REF --tag TAG --out DIR` writes the ten release files from a commit into
  `--out`, which must exist and be empty. It needs `go` and `AGX_HOOK_VERIFIER`, the absolute path of the trusted hook
  verifier.
- `context --repo OWNER/REPO --checkout DIR --sha SHA --tag TAG --baseline FILE --out DIR` writes the release-notes
  context from the baseline file and GitHub.
- `stage --repo OWNER/REPO --sha SHA --tag TAG --assets DIR --notes FILE [--prerelease true|false]` creates or recovers
  the draft, fills and verifies it, and publishes it. `--prerelease true` publishes a non-latest pre-release. The
  default, `false`, publishes a real release that is marked latest. A release that is already published is verified and
  left unchanged.

Flags take `--name value` or `--name=value`. A single-dash flag, `-h`, a positional argument, an unknown flag, and a
missing or empty required flag exit 2. Exit 2 is also the code for a value a command rejects before any work: `--sha`
(resume-check, check-commit, context, stage), `--tag` (verify, context, stage), `--repo` (context, stage), `--parent`
(check-commit), `--prerelease` (stage), and an unreadable `--notes` file (stage). Any other failure exits 1. With no
command, or an unknown one, the usage text goes to stderr and the exit code is 2. A failure prints
`release: <command>: <message>` to stderr, except that the version check `bundle` runs first prints
`release bundle: <message>`.

`context` and `stage` call GitHub with the first non-empty of `GH_TOKEN` and `GITHUB_TOKEN`. `GITHUB_API_URL` replaces
the API and upload hosts unless it is empty or `https://api.github.com`. `resume-check` and `stage` write their step
outputs to the file `GITHUB_OUTPUT` names, or to stdout as `key=value` lines when it is unset or empty. The
`semantic-release` plugin (`plugin.ts`) reads `AGX_RELEASE_OUT`, the absolute directory it writes `plan.json` and
`baseline.md` into, and `AGX_MAJOR_APPROVED`, which must be `true` for a major release outside a dry run.

The commands live in `.github/tools/src/release/`: `cli.ts` dispatches, `commands/<name>.ts` holds one command each, and
`io.ts`, `args.ts`, `version.ts`, `files.ts`, `github.ts`, `assets.ts`, `identity.ts`, `tar.ts`, `hooks-build.ts`,
`notes-contract.ts`, `engine-model.ts` and `plugin.ts` hold the shared parts and the `semantic-release` plugin.
`testing/` holds the test helpers. A production file imports only `node:*` modules and relative imports, and a test
fails on anything else.

    npm run test:tools

A few tests need something the plain tools install lacks: bash (the action tests), a POSIX host (the symlink, bundle and
hook-build tests), `go` (the hook build), and `npm ci --ignore-scripts --prefix .github/release` (the plugin end-to-end
test). Without one of these the test is reported as skipped with its reason, never as passed. Under `CI=true` a missing
prerequisite fails the test instead. The exception is a test that needs a POSIX host: it skips on Windows even under
`CI=true`, because Windows cannot provide one.

Tests that find their interpreter through `entrypoint/interpreters.ts` are the other exception, and `CI` does not decide
them. The Go tests do the same through `hookutiltest.RequireInterpreter`. Either helper probes `sh`, `bash`, `pwsh` or
`powershell` with a trivial command and runs the one that passed. A missing or broken interpreter skips the test, even
under `CI=true`, unless the job's `AGX_REQUIRED_INTERPRETERS` lists it. That variable holds a comma-separated list of
those four names, and the workflows set it in their job `env` blocks. A listed interpreter that is missing or fails its
probe fails the test, and a name outside the four is an error.

### Twin script tables

`entrypoint/polluter.test.ts` and `scripts/scaffold-protection.test.ts` are case tables. Each row runs against a script
and its twin: `find-polluter.sh` and `find-polluter.ps1` under `skills/diagnose/references/`, and
`planning-protection.sh` and `planning-protection.ps1` under `skills/scaffold/scripts/`. The `.sh` leg runs under `sh`
and the `.ps1` leg under the PowerShell that `twinPowerShell()` in `entrypoint/interpreters.ts` picks:
`AGX_TWIN_POWERSHELL` (`pwsh` or `powershell`) when set, otherwise `pwsh` when it works, else `powershell`. The tables
do not skip on Windows by platform. A missing interpreter skips its leg, or fails it when the job lists the interpreter
in `AGX_REQUIRED_INTERPRETERS`. The `validate-windows-dispatch` job in `.github/workflows/validate-manual.yml` sets
`AGX_TWIN_POWERSHELL: powershell` and runs both tables under Windows PowerShell 5.1; other runs use `pwsh` where it is
installed.

    node --test .github/tools/src/entrypoint/polluter.test.ts .github/tools/src/scripts/scaffold-protection.test.ts

## coverage

`coverage` turns test coverage into Cobertura XML for the CI coverage upload. It needs no Go toolchain. Each workspace
writes its own lcov report with `npm --prefix <workspace> run test:coverage`, which runs the same tests as `test` under
Node's coverage and writes `<workspace>/coverage/lcov.info`; test files and `testing/` helpers are excluded.

    npm run test:tools:coverage
    node .github/tools/src/coverage/cli.ts lcov --root . --input .github/tools:.github/tools/coverage/lcov.info --out cobertura-typescript.xml
    node .github/tools/src/coverage/cli.ts summary --root . --input .github/tools:.github/tools/coverage/lcov.info
    node .github/tools/src/coverage/cli.ts goprofile --source-root . --module hooks/go --profile coverage-hooks.out --out cobertura.xml

- `lcov` merges the workspace reports, given as `<workspace>:<lcov file>`, into one Cobertura document.
- `summary` prints a Markdown table of line and branch coverage per workspace and in total, then lists the source files
  that no test loaded. CI appends it to the job summary. An unloaded file is listed, never a failure.
- `goprofile` converts one Go module's `go test -coverprofile` output directly into Cobertura. It reads the Go sources
  under `--source-root` and writes filenames relative to that checkout root, using forward slashes. The source root
  defaults to the current directory.

## boundaries

`boundaries` checks the area dependency rules that the conformance domains do not cover: workspace `package.json`
dependencies, TypeScript imports between areas (read from `tsc --explainFiles`), imports from skill scripts, and the
`CODEOWNERS` entries. It is a TypeScript tool that Node runs directly. It prints `FAIL <rule>: <path>: <message>` lines
and exits `0` (no failures), `1` (failures), or `2` (`tsc` could not run). `npm run plugin:validate` runs it. The rules
are in [`docs/ARCHITECTURE.md`](../../docs/ARCHITECTURE.md#area-boundaries).

    npm run test:boundaries
