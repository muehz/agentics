# Releases are pre-releases, promoted by hand

> **Superseded by [ADR 0010](0010-releases-are-dispatched-and-published-as-real-releases.md) (2026-10-26):** a dispatch
> now publishes a real, latest release by default. The pre-release path below stays available through the `prerelease`
> dispatch input.

The release workflow used to assemble an attested draft and stop; the maintainer published the draft from the
repository's Releases page (ADR 0005). That draft carried GitHub's `untagged-<hash>` placeholder hazard: a draft could
come to report a placeholder `tag_name` even after `stage` had created the real tag first, and publishing under that
placeholder left a stray `untagged-*` tag that failed every later run's `release context` until it was deleted by hand.
That happened twice on 2026-09-23: v1.118.0's run failed on `untagged-47c53fdb02c6142f25e6`, and the v1.119.0 draft read
`untagged-8ba0ad8d74e148293aee` and had to be re-bound by hand before publishing.

The workflow now publishes. `release stage` still builds a draft — immutable releases lock assets at publication, so the
nine assets must land before the release is published, and building a draft first is how
`gh release create --prerelease` with files does the same thing internally — but it now ends by publishing that draft as
a non-latest pre-release, in the same PATCH that pins `tag_name` and `target_commitish`, so a placeholder cannot survive
to a publish form. This decision supersedes ADR 0005's "the workflow stops at an attested draft," but keeps its
principle: the final step is a human act. What changes is which act that is. ADR 0005 called that act clicking "Publish
release" on a draft. This record calls it promotion: the maintainer clears "Set as a pre-release" and sets "Set as the
latest release" on a release the workflow already published.

## Why

This is github/gh-aw's own pattern (`release.md` there runs
`gh release create --generate-notes --prerelease --latest=false`), and it removes the placeholder hazard by
construction: nothing waits on a draft, so nothing can be published under a tag name GitHub swapped out from under it
while it waited. The maintainer wanted this pattern instead of a narrower fix that kept drafts and only tightened the
tag binding — that fix was weighed twice on 2026-09-23 and set aside both times.

## What this costs, stated plainly

**Immutability locks at creation, inside the `stage` job, not at a later human click.** A bad build is a published
pre-release the moment `stage` finishes; nothing after that job can change its assets or its tag. Recovery is the next
patch version, not a corrected republish.

**A failed enrichment, or a failure after the publishing PATCH, is not retried by a rerun.** The `published` stage
output still means "already published when this run found it," so a rerun that finds the pre-release skips enrichment
the same way it always skipped a published release. If the notes agent failed, or `stage`'s own post-publication reread
and verification failed, the pre-release is already live with only its baseline notes. The maintainer edits the notes by
hand; nothing in the repository re-attempts them.

**The smoke proof leaves a public pre-release.** `release smoke` used to leave a draft nobody but the maintainer could
see; it now leaves a real, publicly visible pre-release for the SHA it targeted, with nothing to delete afterward.
_(Retired on 2026-10-11, see [ADR 0007](0007-release-version-is-computed-from-subjects.md) and `docs/RENAMES.md`: a
dispatch now creates a real release commit and tag, so the command has no safe target.)_

**Every version bump is public before a human looks at it.** The workflow used to hold a draft, visible only to
collaborators, until someone chose to publish it. Now the pre-release, its assets, and its notes are live to anyone the
moment the run finishes; the maintainer's review happens after the fact, at promotion time, not before publication.

## Considered and declined

**Keep drafts and fix only the tag binding.** Declined twice on 2026-09-23. It would have closed the immediate hazard
without adopting the gh-aw pattern the maintainer wanted, and a fresh placeholder edge case could still surface later.

**A promote workflow, like gh-aw's `promote-release.yml`.** Declined. It would restore an automated publication path —
the same shape of decision ADR 0005 removed — and the point of this change is that the manual act is real, not that it
is merely renamed.

**Cumulative notes since the last full (non-pre-release) release.** Declined in favor of per-version notes, matching
gh-aw: each pre-release's notes run from the immediately previous release, promoted or not.

**A `promoted` stage output that re-runs enrichment on a rerun after promotion.** Declined for the churn it would add to
`stage`, `context`, and the notes adapter for a case the maintainer accepted living with (plan challenge Q1): a failed
enrichment is fixed by hand.

## Consequences

The workflow now publishes. `stage` holds the one publishing PATCH — `draft:false`, `prerelease:true`,
`make_latest:"false"`, with `tag_name` and `target_commitish` set — sent only after every asset is uploaded and
verified. Promotion is UI-only: no promote workflow, no promote subcommand, nothing in this repository changes a
release's pre-release or latest state after `stage` publishes it. `release context` counts published pre-releases in the
notes range, whether or not they were ever promoted. The notes adapter enriches only a published pre-release's body and
leaves a promoted release alone.

Reverting this decision restores the draft pattern of ADR 0005: `stage` would need to stop again before the publishing
PATCH, and the placeholder-tag mitigation ADR 0005 already carries would need to stay in force. Reverting does not
change any pre-release this pattern already published; those stay exactly as they are, promoted or not, since nothing in
the repository cleans them up (intent Q5).
