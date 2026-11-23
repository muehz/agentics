# Publication is a manual act; the workflow stops at an attested draft

> **Superseded in part by [ADR 0006](0006-releases-are-pre-releases-promoted-by-hand.md) (2026-09-23):** the workflow
> now publishes a pre-release; the manual act is promotion.
>
> **Amended by [ADR 0007](0007-release-version-is-computed-from-subjects.md) (2026-10-11):** the workflow depends on the
> `github-release` environment again, for one purpose: a computed major release waits there for a reviewer before the
> release commit is pushed. Publication stays a manual act.

> **Superseded in part by [ADR 0010](0010-releases-are-dispatched-and-published-as-real-releases.md) (2026-10-26):** the
> workflow publishes a real release when a maintainer dispatches it. The costs below still apply.

The agentic release workflow used to end in a `publish` job behind the `github-release` deployment environment, which
required one named reviewer who could not be bypassed by administrators. That job promoted the draft with
`release publish`, a Go subcommand that re-verified the tag, the release identity, the full seven-asset roster, and
every asset's bytes both before and after the one PATCH it sent, and computed `make_latest` from version order.

That gate is now removed. The workflow assembles a verified draft, attests it, enriches its notes, and stops. The
maintainer publishes the draft from the repository's Releases page, and that click is the sign-off.

## Why

The environment reviewer added nothing the draft did not already offer. The reviewer's whole job was to open the draft,
read its notes, check its assets, and approve. Publishing that same draft by hand performs the same inspection and the
same decision, in one action instead of two, and in the interface where the artifact actually lives. Keeping both meant
the maintainer approved a deployment in order to authorize a publication they could have performed directly.

## What this costs, stated plainly

Three protections went with the command, and none has a technical successor. They are listed here rather than in a
commit message because a later reader deciding whether to restore automated publication needs them in one place.

**Publication authorization widened.** It was "the one named reviewer, administrators cannot bypass." It is now "any
principal holding `contents: write`." The manual click is a real human act, but it is not an enforced one: nothing stops
a different token from publishing the draft.

**Nothing verifies the published release.** `verifyAfterPromotion` re-read the release and the tag after the PATCH and
repeated every check against the published result. The smoke proof now verifies the draft a run leaves and never sees a
published release. Residual detection is a consumer running `gh attestation verify`, which is opt-in, and
`conflictPlaceholderTag` on a later run.

**The window between attestation and publication is unbounded.** Promotion used to happen seconds after attesting,
inside one run. The draft now waits for a human indefinitely. A `contents: write` actor who replaces a draft asset in
that window ships bytes the attestation does not cover, because release immutability locks the assets only at
publication. This is why the attestation moved into `stage` ahead of the first upload rather than staying where the
promotion was: the attestation must exist however long the draft waits, so a consumer can check the bytes they actually
downloaded.

**A manual publish can mint the wrong tag.** GitHub reports a draft's `tag_name` as `untagged-<hash>` once the create
response goes stale, and `release publish` sent `tag_name` explicitly so the promotion could not create a tag named
after the placeholder. That guarantee is now a maintainer reading the tag field before clicking, per the instruction in
`CONTRIBUTING.md`. The failure mode is a tag-pinned consumer resolving nothing.

**Amended 2026-09-22 (#620).** The tag-field guard failed for v1.110.0, which published under a placeholder. `stage` now
creates `refs/tags/vX.Y.Z` at the target before it creates the draft, so GitHub binds the draft to the real tag and the
publish form cannot offer a placeholder. `release context` fails a run while any `untagged-*` tag or placeholder-tagged
published release exists, and prints the recovery. The residual risk this paragraph recorded is closed; the manual click
remains the sign-off.

## Considered and declined

**Keep `release publish` as a maintainer command, alongside `smoke`.** This would have preserved the verified promotion
path for anyone who wanted it, at the cost of two publication routes with different guarantees and a command with no
caller in CI. The user was offered this explicitly and accepted the trade-off, on the grounds that the point of the
change is that publication is the human's action.

**Keep the environment and drop only the `publish` job.** Declined as the worst of both: the environment would remain in
repository settings, gating nothing, and a future workflow edit could silently start depending on it again.

**Attest in a job after `stage`.** Declined. The draft would exist unattested for one job's duration, and a maintainer
watching the Releases page could publish inside that window.

## Consequences

`release` has five subcommands: `version`, `bundle`, `context`, `stage`, and `smoke`. The composite action runs the
first four. No subcommand publishes.

The `stage` job now carries `contents: write`, `id-token: write`, and `attestations: write` together, where the write
and the attestation permissions used to sit in separate jobs and the attestation half sat behind the reviewer. A
compromised step in `stage` can therefore write releases and mint attestations in the same job, with no reviewer in
front of it. That consolidation is the direct price of attesting before the draft exists, and it is the first thing to
re-examine if this decision is revisited.

The `github-release` environment itself is repository configuration, not code. Nothing in this repository depends on it
after this change, and deleting it is a separate maintainer action.

Restoring automated publication means re-authoring `publish.go` and its test suite, both of which are recoverable from
history at the commit that removed them, and restoring the environment and its reviewer in repository settings.
