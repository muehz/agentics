# The install path is TypeScript on Node

> **Supersedes the install-path rule of [ADR 0011](0011-release-automation-is-typescript.md) (2026-11-21):** the
> maintainer entrypoints that install, validate and set up Agentics from a checkout are written in TypeScript and run on
> Node, not in Bash. The rest of that record stands.

ADR 0011 kept `plugin.sh` and `plugin/*.sh` in Bash so that no install needed a toolchain. That rule protected a
consumer from downloading a toolchain. The scripts it covers run only on a maintainer's machine, and a consumer who
installs through a CLI marketplace runs none of them.

## Decision

Installing from a checkout requires Node `^22.23.5 || ^24.22.3 || >=26.12.1`. The floor is the latest release of the 22
and 24 lines on 2026-11-20, and on the 26 line the release winget offers. Node 26.12.1 is the only tested route: the
validation workflows and the cloud bootstrap install that version and no other. The gh-aw detection jobs in
`backlog-grooming.lock.yml` and `release.lock.yml` set Node 24, because gh-aw v1.0.1 has no setting for it. The 22 and
24 floors get no CI run.

The `plugin.sh` modes (setup, teardown, status, validate, validate fast, the VS Code instructions) run from a TypeScript
entrypoint with unit tests, and the Bash files of that family are deleted. `cloud-setup.sh` shrinks to a bootstrap that
fetches and checksum-verifies Node and Go, then hands every other step to TypeScript, because only the Node fetch must
run before Node exists.

## Why

- **One language and testability.** The setup and validation logic sat in Bash that the TypeScript test suite could not
  reach, beside the TypeScript tooling that does the rest of the work.
- **No new burden on consumers.** The install-path rule never protected anyone the checkout path reaches.

## What stays

- **Shipped launchers stay as they are.** `hooks/run-hook.cmd`, `hooks/fetch-hook.ps1`, `bin/agx` and the `powershell`
  entry in `hooks/hooks.copilot.json` run on user machines before any toolchain is guaranteed.
- **Consumer-run helpers need no runtime beyond the operating system's shell.** `find-polluter.sh` stays shell and
  gained a PowerShell twin, and the `planning-protection` Node helper was replaced by a `.sh` and a `.ps1` with the same
  behavior. Consumer PowerShell runs on Windows PowerShell 5.1.
- **The gh-aw `.lock.yml` files stay compiled output**, never edited by hand.
- **Go stays where Go is the point:** the hook binaries and the source digest, which the Go implementation now computes.

## Consequences

- A maintainer does not need Bash to install, validate or set up a worktree on Windows.
- A maintainer without Node in the declared range cannot install a local copy from a checkout.
- A break on Node 22 or 24 surfaces only in use, and Node 26.12.1 is one release behind the 26.13.1 security release of
  2026-11-10, because winget offers no later version.
- Each consumer helper has two implementations to keep in step.
- Reverting this decision means restoring the Bash entrypoints from history.
