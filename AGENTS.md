# AGENTS.md

`AGENTS.md` is canonical. Claude Code, Codex, and Copilot read it directly. Do not add a `CLAUDE.md` or
`.claude/CLAUDE.md`: Claude Code reads that file instead of `AGENTS.md` when one exists. The `no-claude-md` conformance
rule enforces this.

This file orients an agent in this checkout: the commands, where each kind of change is documented, and facts the code
does not show. It does not say which workflow to run. The Agentics skills own that, and
[`CONTRIBUTING.md`](CONTRIBUTING.md) owns the rules for contributing here.

## Installed plugin

Each CLI keeps its own install snapshot of the Agentics plugin, so a CLI can run an older version than this checkout,
and a stale install hides new skills. [`CONTRIBUTING.md`](CONTRIBUTING.md#checking-the-installed-plugin) says how to
compare the installed versions with `.claude-plugin/plugin.json`.

## Quick start

Run commands from the repository root.

| Task                                   | Command                                                                                                          |
| -------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| Install dev dependencies               | `npm install`                                                                                                    |
| Prepare a new worktree                 | `npm run worktree:setup`                                                                                         |
| Prepare a cloud environment            | `./.github/scripts/cloud-setup.sh`                                                                               |
| List local development tasks           | `node .github/tools/src/plugin/cli.ts --help`                                                                    |
| Install local checkout into CLIs       | `npm run plugin:setup`                                                                                           |
| Show installed plugins & marketplaces  | `npm run plugin:status`                                                                                          |
| Remove local plugins, restore remote   | `npm run plugin:teardown`                                                                                        |
| Print VS Code local plugin settings    | `npm run plugin:vscode`                                                                                          |
| Test hook code                         | `npm run test:hooks`                                                                                             |
| Run the eval engine (sibling clone)    | `node ../evals/evals/src/cli/main.ts <verb>`                                                                     |
| Test TypeScript dev tools              | `npm run test:tools`                                                                                             |
| Write TypeScript coverage as lcov      | `npm run test:tools:coverage`                                                                                    |
| Summarize TypeScript coverage          | `node .github/tools/src/coverage/cli.ts summary --root . --input .github/tools:.github/tools/coverage/lcov.info` |
| Typecheck TypeScript workspaces        | `npm run test:typecheck`                                                                                         |
| Check area boundaries                  | `npm run test:boundaries`                                                                                        |
| Run full conformance                   | `npm run plugin:validate`                                                                                        |
| Run fast conformance (edit loop)       | `npm run plugin:validate:fast`                                                                                   |
| Format the repository                  | `npm run format`                                                                                                 |
| Check formatting                       | `npm run format:check`                                                                                           |
| Build native outputs and the manifest  | `make -C hooks/go build-all`                                                                                     |
| Mutation-test TypeScript files         | `npm run test:mutation -- <file>...`                                                                             |
| Mutation-test one hooks package        | `make -C hooks/go mutation PKG=./internal/<name>`                                                                |
| Compare hook latency with the baseline | `make -C hooks/go bench`                                                                                         |

`--validate` is the full validation and what a pull request needs. `--validate --fast` skips the Go and TypeScript test
suites, so it is for the edit loop only. Both modes end with a per-stage timing summary. Every mode needs Node in the
`engines.node` range of `package.json` (`^22.23.5 || ^24.22.3 || >=26.12.1`, with 26.12.1 tested). `--validate` also
needs Go, and it runs the TypeScript conformance suite, which needs the installed packages: run
`npm ci --ignore-scripts` once. The conformance `skills` domain reads frontmatter with the `yaml` package and reports an
audit error without it, and the typecheck needs TypeScript. Go 1.27.4 builds the trusted hook verifier `hooktool` from
`hooks/go`. The hook checks use that verifier. The eval engine lives in [`muehz/evals`](https://github.com/muehz/evals)
and runs from a clone at `../evals` beside this checkout, or wherever `AGX_EVALS_ROOT` points; it needs Node 26 or
later. Both `--validate` modes run the eval rules through that clone and print `skipped: no sibling evals clone` without
one. CI has no clone, so it never runs them. [`evals/README.md`](evals/README.md#the-engine-is-a-sibling-clone) says how
to set the clone up. The repository requires npm 12.2.1 or later (`engines.npm`); Node 26 bundles npm 11, so run
`npm install --global npm@12.2.1` first. `--fast` needs the same Go, Node in the range and installed packages: it still
builds the hook verifier, runs the TypeScript conformance and typecheck, and checks boundaries. Developer setup from a
source-only checkout needs the pinned Go toolchain and Node in range. Users can install the
[verified release package](docs/guide/01-setup.md) without a Go toolchain.

How these commands fail on a given host, and how to scope around the failure, is in
[`CONTRIBUTING.md`](CONTRIBUTING.md#known-host-traps). The output each check prints when it passes is in
[`CONTRIBUTING.md`](CONTRIBUTING.md#healthy-output).

## Read before changing

Start with the nearest `README.md`, then load the owner for the change:

| Change area                                              | Read first                                                                                                                                                   |
| -------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| General architecture and packaging                       | [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)                                                                                                               |
| Adding a dependency or import between areas              | [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md#area-boundaries) and [`.github/tools/src/boundaries/`](.github/tools/src/boundaries/)                          |
| What belongs in Agentics                                 | [`docs/CHARTER.md`](docs/CHARTER.md)                                                                                                                         |
| Provider-specific packaging or claims                    | [`docs/surfaces/README.md`](docs/surfaces/README.md) and the provider's surface document                                                                     |
| Hook implementation and packaged binaries                | [`hooks/README.md`](hooks/README.md) and the guardian section for the hook                                                                                   |
| Skills, instructions, or MCP servers                     | [`skills/forge/SKILL.md`](skills/forge/SKILL.md) and its relevant authoring or quality reference                                                             |
| Vendored or adapted upstream content                     | [`THIRD_PARTY_NOTICES.md`](THIRD_PARTY_NOTICES.md) and the nearest attribution or license file                                                               |
| A question about how a hook behaves                      | the hook's section in [`hooks/README.md`](hooks/README.md), the surface config beside it, and its Go core in [`hooks/go/`](hooks/go/), read before answering |
| Writing or changing a test                               | [`CONTRIBUTING.md`](CONTRIBUTING.md#negative-tests-must-fail-when-the-check-is-removed)                                                                      |
| Opening a pull request, versioning, or the change record | [`CONTRIBUTING.md`](CONTRIBUTING.md#branching-and-pull-requests) and [Versioning](CONTRIBUTING.md#versioning-and-the-change-record)                          |

Use [`docs/README.md`](docs/README.md) to find the rest of the documentation.

## Skill changes need evaluation

`--validate` and the pull-request checks never run skill evals, and no workflow in this repository does. A pull request
that adds a skill or changes how one behaves needs evaluation results from the eval engine in
[`muehz/evals`](https://github.com/muehz/evals), run from the sibling clone, from a coding tool's own eval tooling, or
from both. Report them in the pull request's `## Verification`.
[Evaluate a proposed skill change](CONTRIBUTING.md#evaluate-a-proposed-skill-change) defines what counts. Passing
conformance is not evidence that a skill still behaves.

## Go comments

Package and exported-identifier documentation is the floor. Other comments explain constraints and reasons rather than
narrating what the code does. Never cite plan, spec, or task identifiers (`S1`, `spec Q8`, `wave 4`) in code comments;
they point nowhere once the work folder is archived. State the reason itself.

After moving an exported Go identifier, update its leading comment to use the destination identifier's name and state
its contract. Audit separately declared constants individually.
