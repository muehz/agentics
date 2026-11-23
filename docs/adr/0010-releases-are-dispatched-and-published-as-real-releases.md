# Releases are dispatched, and publish as real releases

> **Supersedes [ADR 0006](0006-releases-are-pre-releases-promoted-by-hand.md) (2026-10-26):** the workflow no longer
> publishes every release as a pre-release for the maintainer to promote. ADR 0005's list of costs still holds.

The release workflow used to run on every push to `main`, publish each release as a non-latest pre-release, and wait for
the maintainer to promote it from the Releases page (ADR 0006). Eight releases, v1.210.1 through v1.217.0, were
published in three days, and none was promoted, so no release was `latest`.

The workflow now runs only when a maintainer dispatches it on `main`, and a dispatch publishes a real release that is
marked `latest`. The dispatch is the human act that ADR 0005 and ADR 0006 kept: nothing publishes until a maintainer
asks. One dispatch after several merges releases every commit since the last tag as a single version, at the highest
level among them.

A boolean `prerelease` dispatch input, which defaults to `false`, keeps the previous behavior available. With it set to
`true` the workflow publishes the non-latest pre-release that ADR 0006 described, and the maintainer promotes it by
hand.

## Why

The promotion step was a gate in name only. It was not happening, and the cost of leaving it undone was a Releases page
with no current release. Moving the human act to the start of the run puts it where it can be done: the maintainer
decides when a release is wanted, instead of reviewing one per merge after it is public.

## What this costs, stated plainly

**No person looks between publication and `latest`.** A real release is `latest` the moment `stage` publishes it, and
release immutability locks its assets and tag at that moment. A bad build is corrected only by the next version. The
attestation, the rebuild comparison and the conformance run still gate it, and the `prerelease` input is the cautious
path.

**Authorization is unchanged.** GitHub requires
[repository write access to run a workflow manually](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/manually-run-a-workflow).
The workflow's job and token permissions are separate controls and do not change here. The `release-commit` environment
restricts the run to `main` and has no reviewers; a major release still waits for the `github-release` reviewer (ADR
0007).

**The notes are edited after publication.** The AI highlights replace the baseline notes on a release that is already
public. A failed enrichment is not retried, so a failure leaves the baseline notes on a `latest` release, and the
maintainer edits them by hand.

**A late `latest` fails a good release.** After a real publish `stage` reads `releases/latest` until it names the
release, with the four-read budget the other rereads use. If it never does, the run fails after publication, and a rerun
verifies the release without enriching it. Whether four reads cover GitHub's lag is unobserved; the first live dispatch
is the only evidence, and the budget widens in one constant.

**Unreleased work accumulates.** A merge no longer starts a release, so `main` runs ahead of the last tag until someone
dispatches.

## Considered and declined

**Keep the push trigger and add a dispatch.** Declined: every merge would still release, which is the cadence being
changed.

**Make a dispatch publish only a pre-release.** Declined: that keeps the promotion step that was not happening.

**Convert a published pre-release on a later dispatch.** Declined. A rerun never changes a published release, in either
direction; `stage` verifies it and says when the input did not apply. Promotion of an existing pre-release stays a
manual act.

**A separate promote workflow.** Declined, as in ADR 0006.

## Consequences

`release stage` takes `--prerelease true|false` and rejects any other value. Its one publishing PATCH sends
`prerelease: false` and `make_latest: "true"` by default, and `prerelease: true` and `make_latest: "false"` with the
flag. The notes adapter enriches a release only while its `prerelease` equals the state the workflow recorded for the
run. Existing pre-releases are left as they are. Reverting this decision restores the pre-release default of ADR 0006.
