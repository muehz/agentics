# AGENTS.md

## Public repository

Agentics is a public repository. Git history and GitHub posts can be copied or indexed as soon as they are published.

- Keep secrets, personal data, internal URLs, private logs, and material from other private repositories out of commits
  and GitHub posts. Use redacted examples when reporting a problem.
- Before committing, review the staged diff and commit message. Before pushing, verify the destination and review every
  outgoing commit, including its message.
- Treat pull requests, issues, comments, and attachments as public. Review them before posting. Report vulnerabilities
  through [SECURITY.md](SECURITY.md).

## Read before changing

Start with the nearest `README.md`. Use the Agentics skills to choose a workflow and [CONTRIBUTING.md](CONTRIBUTING.md)
for repository rules.

| Change area                           | Read first                                                                                                                           |
| ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| Project scope                         | [Charter](docs/CHARTER.md)                                                                                                           |
| Architecture and packaging            | [Architecture](docs/ARCHITECTURE.md)                                                                                                 |
| Dependencies and imports across areas | [Area boundaries](docs/ARCHITECTURE.md#area-boundaries) and [boundary checks](.github/tools/src/boundaries/)                         |
| Provider packaging or behavior        | [Provider surfaces](docs/surfaces/README.md) and the relevant provider page                                                          |
| Hook behavior or implementation       | [Hook guide](hooks/README.md), the provider's hook config, and the [Go implementation](hooks/go/)                                    |
| Skills, instructions, or MCP servers  | [Forge skill](skills/forge/SKILL.md) and the relevant reference                                                                      |
| Vendored or adapted content           | [Third-party notices](THIRD_PARTY_NOTICES.md) and the nearest attribution or license                                                 |
| Tests                                 | [Negative test requirements](CONTRIBUTING.md#negative-tests-must-fail-when-the-check-is-removed)                                     |
| Pull requests and releases            | [Pull request rules](CONTRIBUTING.md#branching-and-pull-requests) and [versioning](CONTRIBUTING.md#versioning-and-the-change-record) |

Find the rest of the documentation in the [docs index](docs/README.md).

## Validation

Use the Node and npm versions in [package.json](package.json) and the Go version in [hooks/go/go.mod](hooks/go/go.mod).
Run `npm ci --ignore-scripts` from the repository root.

- Run `npm run plugin:validate` for a pull request.
- Run `npm run plugin:validate:fast` during development. It runs conformance, type checking, and area checks and skips
  the Go and TypeScript test suites.

Both commands print a timing summary. They run eval rules when an
[evals checkout](evals/README.md#the-engine-is-a-sibling-clone) is available at `../evals` or through `AGX_EVALS_ROOT`.
See [host-specific failures](CONTRIBUTING.md#known-host-traps) and [healthy output](CONTRIBUTING.md#healthy-output) for
troubleshooting.

### Skill changes need evaluation

Evaluate new skills and behavior changes with the [eval engine](https://github.com/muehz/evals), the coding tool's eval
runner, or both. Report the results in the pull request's `## Verification`.
[Evaluate a proposed skill change](CONTRIBUTING.md#evaluate-a-proposed-skill-change) defines what counts.

## Code comments

Document modules and public APIs according to the language's conventions. Use other comments to explain constraints,
invariants, and reasons that the code cannot convey on its own.

State the reason directly. Plan, spec, and task identifiers lose their context when work folders are archived. When a
public symbol moves or changes name, update its documentation to match its name and contract. Check separately declared
constants individually.

## Quick start

Run commands from the repository root.

| Task                                                | Command                                                                                                          |
| --------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| Install dev dependencies and set up local Git hooks | `npm install`                                                                                                    |
| Prepare a new worktree                              | `npm run worktree:setup`                                                                                         |
| Prepare a Linux or macOS cloud environment          | `./.github/scripts/cloud-setup.sh`                                                                               |
| List local development tasks                        | `node .github/tools/src/plugin/cli.ts --help`                                                                    |
| Install this checkout into local CLIs               | `npm run plugin:setup`                                                                                           |
| List installed plugins and marketplaces             | `npm run plugin:status`                                                                                          |
| Remove local plugins and restore remote ones        | `npm run plugin:teardown`                                                                                        |
| Print local plugin settings for VS Code             | `npm run plugin:vscode`                                                                                          |
| Run the eval engine from a sibling clone            | `node ../evals/evals/src/cli/main.ts <verb>`                                                                     |
| Test hooks                                          | `npm run test:hooks`                                                                                             |
| Test the TypeScript tools                           | `npm run test:tools`                                                                                             |
| Check TypeScript types                              | `npm run test:typecheck`                                                                                         |
| Check area boundaries                               | `npm run test:boundaries`                                                                                        |
| Format the repository                               | `npm run format`                                                                                                 |
| Check formatting                                    | `npm run format:check`                                                                                           |
| Build native outputs and the manifest               | `make -C hooks/go build-all`                                                                                     |
| Write TypeScript coverage as LCOV                   | `npm run test:tools:coverage`                                                                                    |
| Summarize TypeScript coverage                       | `node .github/tools/src/coverage/cli.ts summary --root . --input .github/tools:.github/tools/coverage/lcov.info` |
| Mutation-test TypeScript files                      | `npm run test:mutation -- <file>...`                                                                             |
| Mutation-test a hooks package                       | `make -C hooks/go mutation PKG=./internal/<name>`                                                                |
| Compare hook latency with the baseline              | `make -C hooks/go bench`                                                                                         |

## Installed plugin

Each CLI keeps its own plugin snapshot. If a skill is missing or behaves differently from this checkout,
[check the installed version](CONTRIBUTING.md#checking-the-installed-plugin) against `.claude-plugin/plugin.json`.

Users can install a [verified release package](docs/guide/01-setup.md) without a Go toolchain.
