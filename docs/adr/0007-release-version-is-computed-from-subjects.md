# The release version is computed from commit subjects

When authors choose a release number in their pull requests, concurrent changes can pick the same version. Computing the
version from merged subjects gives the release workflow one place to choose the version and tag.

The maintainer dispatches the release from `main`. Semantic-release reads the Conventional Commit subjects since the
last tag and computes the next version. `feat` is a minor, `fix` and `perf` are a patch, and `!` or a `BREAKING CHANGE:`
footer is a major. Every other type releases nothing. A job holding a GitHub App token that bypasses the `main` ruleset
writes the five manifest version fields, commits them as `chore(release): vX.Y.Z [skip ci]`, and tags `vX.Y.Z`. The
release workflow independently rebuilds and attests the assets from that commit before publishing the requested stable
release or prerelease.

## How the pipeline is split

`.github/workflows/release.md` runs on manual dispatch. Set `prerelease` to publish a non-latest prerelease.

- `level` holds a read-only token. It runs `release resume-check`, and otherwise runs semantic-release with `--dry-run`
  to compute the level. It points `--repository-url` at the local checkout, because semantic-release verifies push
  access even in a dry run and this job has no credential that could push.
- `approve-major` runs only for a major, under the `github-release` environment and its required reviewer.
- `release` runs under the `release-commit` environment, which is restricted to `main`. It runs
  `npm ci --ignore-scripts` first, mints the App token after the install, and passes the token only to the
  semantic-release step and the step that records the result. semantic-release commits, tags, and pushes.
- `prepare` holds no token. `release check-commit` verifies the commit's shape, its parent, the author and committer
  name, and the tag order before `bundle` runs. It is built from the pushed head, a reviewed commit, and never from the
  release commit it verifies, so a release job that edits the verifier cannot pass its own check. On a resumed run the
  head is the release commit itself, so the verifier comes from that commit's first parent.
- `stage` rebuilds and attests the assets, then publishes from the baseline notes. `agent` writes the highlights.

The tooling lives in `.github/release/`: semantic-release 25.0.9 pinned exactly, with four plugins in a fixed order.
They are `commit-analyzer`, `release-notes-generator`, an in-tree plugin, and `@semantic-release/git`. The `stage` job
publishes to GitHub. [ADR 0011](0011-release-automation-is-typescript.md) describes the TypeScript commands for updating
the manifest versions and checking the release commit in `.github/tools/src/release/`.

## What this supersedes

[ADR 0011](0011-release-automation-is-typescript.md) holds that the release commands take no third-party runtime
dependencies. Version resolution sits outside that rule: the version is computed by a pinned Node package under
`.github/release/`. ADR 0011's install-path rule stands: no consumer runs this tooling.

## What this costs, stated plainly

**A dependency tree shares a job with a token that bypasses review.** This is the residual risk accepted in the
decision: one `release` job in which semantic-release commits, tags, and pushes, rather than a token-free planning job
and a separate Go job that holds the token. A compromised dependency in that job could push to `main` with the bypass.
The controls that remain are exact pins, `--ignore-scripts`, the four-entry plugin list under a config test, the token
minted after the install and passed to two steps (semantic-release, and the step that records the result and pushes the
resumed tag), the environment restricted to `main`, the commit-shape check in `prepare`, a ruleset on `v*` tags, and the
reviewer on a major. `prepare` rejects any release commit that changes more than the five version values, which limits
what a compromised job can land under a release to a wrong version number. The author and committer name comparison is a
sanity check, not authentication.

**A `docs` change that should ship must be labeled `feat` or `fix`.** The type is the only input, so a shipped-text
change titled `docs:` releases nothing.

**A revert releases nothing, in any shape.** An explicit release rule in the analyzer configuration says so, including
for a GitHub-shaped revert whose body reads `This reverts commit <sha>.`. The cost is that reverting a shipped `feat` or
`fix` does not ship the revert on its own; it needs a separate `fix` or `feat` change, or a manual re-title.

**A major is a larger number than before.** Nothing pins an Agentics version, so this costs a digit.

**Between a merge and its release commit, `main` carries the previous version.** The release commit follows within one
run, and the session-start check and the hook fetch URL stay correct in the meantime.

## Consequences

The workflow depends on settings no file in this repository can hold: the App and its two environments, the ruleset
bypass, the `v*` tag ruleset, squash-only merges, and the secrets. The runbook in
[`docs/maintainers/release-runbook.md`](../maintainers/release-runbook.md) lists the setup and recovery steps.

`plugin.sh --bump`, `cmd/bump`, and `release version` are retired (see `docs/RENAMES.md`), and so are the part-based
bump logic in `agenticsversion`, the six-manifest and `(<version>)` rules, and the `target_sha` and `request_id`
dispatch inputs. Reverting this decision restores the hand-chosen version and requires the bump command to return.
