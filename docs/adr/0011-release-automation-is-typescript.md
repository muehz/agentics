# Release automation is TypeScript; the install path is Bash

Agentics has two kinds of maintainer tooling that look alike and follow different rules. The release commands run on a
GitHub-hosted runner, inside the release workflow and nowhere else. The install scripts run on the machine of anyone who
installs the plugin from a checkout. This record decides the language of each: the release commands are TypeScript that
Node runs directly, and the install path is Bash.

## Release automation is TypeScript

The release commands live in `.github/tools/src/release/`. `cli.ts` has seven commands: `set-version`, `resume-check`,
`check-commit`, `bundle`, `context`, `verify` and `stage`. `plugin.ts` is the `semantic-release` plugin, and
`.github/release/.releaserc.json` lists it by path. The composite action `.github/actions/release` runs one command per
call, and `.github/workflows/release.md` is its only consumer.

Node runs these files as they are. There is no build step, no compiled output, and no dependency beyond Node's own
modules.

The reasons:

- **The release runs inside a Node program.** `semantic-release` computes the version and loads its plugins as Node
  modules. It loads `plugin.ts` with no loader flag. Commands in the same language as the tool they plug into keep the
  pipeline in one language and one toolchain.
- **Nothing is built before a command runs.** A job that runs a command needs Node and a checkout. The `level` and
  `release` jobs set up no Go.
- **The rest of the maintainer tooling is the same.** The conformance suite and the other tools under
  `.github/tools/src/` are TypeScript that Node runs directly, so the release commands share their tests, typecheck and
  formatting.

Go stays where Go is the point: the hook binaries under `hooks/go`. `bundle` builds the hook verifier and checks the
binaries with it, so the release action sets up Go for `bundle` only, and the `prepare` and `stage` jobs set up Go to
build the hook binaries.

## The install path is Bash

> **Superseded on 2026-11-21:** [ADR 0026](0026-the-install-path-is-typescript-on-node.md) moved the install path to
> TypeScript on Node. The rest of this record stands.

`.github/scripts/plugin.sh` and the libraries under `.github/scripts/plugin/` install the plugin from a checkout. They
are Bash, and installing needs neither Go nor Node.

The reasons:

- **An install must work on a fresh clone.** A consumer or maintainer installs the plugin before any toolchain is set
  up. A script in Go or Node would make that toolchain a hard requirement for every install.
- **The alternative to a hard requirement is worse.** Each CLI installs a local marketplace by copying the source
  directory, ignored and untracked files included. `plugin/stage.sh` builds a clean copy outside the repository for the
  CLIs to read. If staging needed a toolchain and fell back to the checkout when it was missing, every install would
  copy the whole working tree: 318 MB measured for 17 MB of plugin.
- **The one check that uses Node is optional.** The install path checks the manifests on Node 22.18 or later. When Node
  is missing or too old, it warns and continues.

Only `plugin.sh --validate` has hard requirements: Node for the conformance suite and the tools tests, and Go to build
the hook verifier.

This rule covers the install path and nothing else. No consumer runs a release command, so the rule gives no reason to
write the release commands in shell.

## Rules the release commands keep

- **The process that holds the App token stays minimal.** The plugin does not import the manifest writer. Its `prepare`
  step runs `cli.ts set-version` in a child Node process, so the writer never shares a process with the token.
- **The child environment is scrubbed.** That child receives only `PATH`, `HOME`, `USERPROFILE`, `SYSTEMROOT`, `TMPDIR`,
  `TEMP` and `TMP`. The App token and every `GITHUB_*` and `AGX_*` variable stay out of it.
- **Actions are pinned to a commit SHA.** The release action sets up Node and Go through actions pinned by SHA, and the
  workflow tests reject an action reference that is not a 40-hex commit.
- **There are no third-party runtime dependencies.** A production file in `.github/tools/src/release/` imports only
  `node:*` modules and relative paths, and `zero-deps.test.ts` fails on any other import. `semantic-release` and its
  plugins are a pinned install under `.github/release`, as [ADR 0007](0007-release-version-is-computed-from-subjects.md)
  set up. They are the tool the plugin runs inside, not something the release commands import.
- **A redirect carries the token only within the API origin.** `downloadReleaseAsset` follows a 301 with the token only
  when the target has the same scheme, host and port as the API. Comparing the host alone would follow a redirect from
  https to http on the same host and send the token in cleartext. A 302 is fetched by a second request that carries no
  credential, and any other redirect is an error.

## What this costs

**The runners need a pinned Node.** The action installs Node 26.7.0, the version `release.md` pins, so `resume-check`
works before a job's own Node step, and `prepare` and `stage` do not depend on the runner image's Node. The first call
in a job records the version in an environment variable, and a later call in the same job skips `setup-node`. The
`level` and `release` jobs also keep their own `Set up Node` step, which runs before `npm ci`. A runner that cannot run
`setup-node` cannot run a command.

**Type stripping limits the syntax.** Node strips types and does not compile them, so the source uses erasable syntax
only: no enums, namespaces or parameter properties. `.github/tools/tsconfig.json` enforces this with
`erasableSyntaxOnly`, and `tsc --noEmit` checks the types.

**Some tests need more than the tools install.** The action, bundle and hook-build tests need a POSIX host, and the
plugin end-to-end test needs `.github/release/node_modules`. Under `CI=true` a missing install fails the test. Elsewhere
it is reported as skipped with its reason, never as passed. A test on a platform it does not support is skipped the same
way, under `CI=true` as well.

## Considered and declined

**Release commands in Go.** Declined: the `semantic-release` plugin has to be Node code either way, so the pipeline
would span two languages and two toolchains. The plugin would build and spawn a binary, and the `level` and `release`
jobs would set up Go only for that.

**Release commands in shell.** Declined: the install-path rule does not reach them, and release logic that checks
commits, assets and API responses needs tests that run in-process.

**Install scripts in Go or Node.** Declined for the reasons under "The install path is Bash": a toolchain would become a
requirement for every install, or a fallback would copy the whole working tree.

**Parse the Go hook binaries in TypeScript.** Declined: `bundle` checks that the hook binaries were built with the
toolchain `hooks/go/go.mod` pins. Node has no equivalent of the `debug/buildinfo` package, and a parser would have to
read three container formats. `bundle` runs `go version <binary>`, which reads the same data, and the job that runs
`bundle` always has Go.

**A native Node action in place of the composite action.** Declined: a native action runs a committed `dist/` bundle.
The pinning check skips a local `./` action, so that bundle would sit in the release path pinned by nothing and compared
to nothing. A composite action runs the source files of the checked-out commit.

**Import the manifest writer into the plugin.** Declined: it would put the App token in the process that writes the
manifests.
