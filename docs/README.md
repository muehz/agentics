# Agentics documentation

This directory holds the longer explanations and reference material that would get in the way of the root README.

If you're using Agentics, start with the [consumer guide](guide/README.md). It walks through setup and day-to-day use
with prompts you can copy and adapt.

If you're changing Agentics, read [CONTRIBUTING.md](../CONTRIBUTING.md) first. [ARCHITECTURE.md](ARCHITECTURE.md)
explains how the repository fits together, and [CHARTER.md](CHARTER.md) records the opinions behind what belongs in the
plugin.

[AISDLC.md](AISDLC.md) describes the Agentics AI SDLC flow, from a request to a merged pull request, and records where
it follows or departs from Anthropic's published guidance.

Provider behavior changes often and doesn't belong in a generic overview. The [surface docs](surfaces/README.md) keep
those requirements, observations, and open gaps separate.

[RENAMES.md](RENAMES.md) records skill, agent, hook, and surface changes made in this repository.

Work folders under `docs/specs/` keep each work item’s intent, specification, implementation plan, and `state.tsv`
record together. Skills write the record through the `flow` helper. A small fix gets no work folder and logs to
`unassigned/state.tsv` under the work root.

The [decision records](adr/) explain choices that aren't obvious from the current code.

[THIRD_PARTY_NOTICES.md](../THIRD_PARTY_NOTICES.md) indexes third-party sources, adaptations, and license details. Some
vendored material keeps its license beside the files it covers. The
[third-party review reminder](../.github/workflows/reminders.yml) opens one combined issue on the first of each month at
09:00 UTC, with a checklist generated from the notices' source repositories and nested repository attributions.
Reviewers record upstream revisions and adoption recommendations, then link separate follow-up issues for worthwhile
improvements. Manual runs reuse the current month's issue, including closed issues, without overwriting its checklist or
findings.

[Secret pattern maintenance](maintainers/secret-pattern-maintenance.md) records the pinned detector sources, update
procedure, and separate repository/history scan.

[Model identifier pins](maintainers/model-ids.md) records the identifiers pinned off the model ladder and how to rename
a model.

The [release runbook](maintainers/release-runbook.md) lists the settings the release workflow depends on and the steps
for a failed run, a stranded tag, a rotated App key, a reinstalled App, and the admin-only manual path.

Repo-wide documents live at the root of this directory. More focused material lives beside the subject it describes or
in a named subdirectory.
