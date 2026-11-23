# Release runbook

The plugin version is computed from squash subjects and released by `.github/workflows/release.md`
([ADR 0007](../adr/0007-release-version-is-computed-from-subjects.md)). This page holds the settings the workflow
depends on and the steps to take when a run needs a person.

## Who appears as the author

Two identities appear on a release, and the split is deliberate. The release commit and the `vX.Y.Z` tag are made by the
release App, so they show as `<app-slug>[bot]`. The GitHub release itself is published by the `stage` job with the
workflow's own token, so the Releases page shows `github-actions[bot]`.

The App token carries the ruleset bypass on `main` and `v*`. The `release` job, under the `release-commit` environment,
is the only job that mints it. The `stage` job builds, attests, and publishes without it. Publishing as the App would
put that token in a second job, which ADR 0007 avoids.

## Check the distribution

The release publishes ten files, including `agentics-plugin-<tag>.tar.gz`, the source archive, six separate native
runtimes, third-party notices, and `SHA256SUMS`. The plugin archive carries the Windows launcher and the runtime
manifest. The runtime executables are separate assets resolved as needed.

Use `node .github/tools/src/release/cli.ts verify --assets <release-dir> --tag <tag>` for a directory with the full
roster. For a consumer install, use the [setup guide](../guide/01-setup.md) with the verified package and a fresh
provider profile. For an offline smoke check, verify the matching runtime against the package's manifest and place it in
the package's `hooks/bin/` before registration. Confirm that the installed CLI prints help with `AGX_NO_FETCH=offline`.

## Prerequisites

These are repository and organization settings. No file in this repository can hold them, and the release job fails
without them.

- [ ] The release GitHub App is installed on this repository only, with Contents write and Metadata read.
- [ ] The `release-commit` environment exists and is restricted to `main`, with no required reviewer.
- [ ] The `github-release` environment exists with a required reviewer. A computed major waits there.
- [ ] `AGX_RELEASE_CLIENT_ID` and `AGX_RELEASE_PRIVATE_KEY` are secrets of the `release-commit` environment, and the
      repository-level copies, including any `AGX_RELEASE_APP_ID`, are deleted.
- [ ] The App is a bypass actor on the `main` ruleset in `always` mode.
- [ ] A ruleset on `v*` blocks tag create, update, and delete for everyone except the App and organization admins.
- [ ] Merge commits and rebase merges are disabled in repository settings and in the `main` ruleset's allowed merge
      methods, so the squash subject is the only input.

## Re-dispatch a failed run

A failed run is safe to run again. The run resumes when the head of `main` is an unreleased release commit, meaning a
`chore(release): vX.Y.Z [skip ci]` commit whose tag is absent or has no published release. It creates the tag at that
commit if it is missing and goes on to `stage`.

1. Open Actions, select **Agentics Release**, and choose **Run workflow** on `main`.
2. Or, from a terminal: `gh workflow run release.lock.yml --ref main`.
3. Read which steps ran. The `level` job's `Decide resume or analyze` step writes the mode as a job output, not to the
   log. When the `level` job's `Compute the level` step runs, the mode is `analyze`: the head is not an unreleased
   release commit, and the run computes a level from the subjects since the last tag. When that step is skipped and the
   `release` job runs, the mode is `resume`, and the run stages the head's tag.

A run at a head that is already released changes nothing.

## A tag with no release

`resume-check` warns `vX.Y.Z has no published release; settle it by hand` when a `v*` tag reachable from `main` is older
than the head and has no published release. The run proceeds without it.

1. Find the tag's commit: `git rev-parse vX.Y.Z^{commit}`.
2. Open an issue that names the tag and why its release did not publish. The next release covers its changes, so nothing
   else is required.
3. Delete the tag only when no release was ever published for it:

    ```bash
    git push origin :refs/tags/vX.Y.Z
    ```

    The ruleset on `v*` blocks the deletion for everyone except the App and organization admins, so an admin does this.
    A published release's tag is never reused.

## Rotate the App key

1. In the App's settings, generate a new private key.
2. In this repository's `release-commit` environment, replace the `AGX_RELEASE_PRIVATE_KEY` secret with the new key's
   contents.
3. Re-dispatch the release workflow, because a merge no longer starts one, and confirm the `release` job mints its
   token.
4. Delete the old key in the App's settings.

The client ID in `AGX_RELEASE_CLIENT_ID` does not change when a key rotates.

## Reinstall the App

1. Install the App on this repository only, with Contents write and Metadata read. Grant nothing else.
2. Put the new `AGX_RELEASE_CLIENT_ID` and `AGX_RELEASE_PRIVATE_KEY` in the `release-commit` environment if the App or
   its key changed.
3. Add the App to the `main` ruleset's bypass list in `always` mode, and to the `v*` ruleset's allowed actors. A new
   installation has a new actor ID, so the old entries do not carry over.
4. Re-dispatch the release workflow and confirm the `release` job's `Mint the release App token` step passes.
