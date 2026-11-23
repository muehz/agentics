# Rollback readiness

Section 4 of the [pre-deploy checklist](pre-deploy-checklist.md#section-4-rollback-readiness), also reused by change
mode's rollback phase. It checks that a working recovery path exists and is staged before the deploy. Verifying it now
is cheap; finding it missing during an incident turns a quick recovery into a sustained outage.

Its questions join the checklist's one question round. Ask only what cannot be read or inferred.

## Step 1: Identify the rollback target

The default target is the last version that deployed successfully. Find it in, in order:

1. the deploy history of the CI or CD system,
2. the target environment's deploy log,
3. the previous release tag: `git tag --merged <artifact-ref> --sort=-creatordate`, filtered to the repository's release
   tag pattern, skipping the artifact's own tag.

Report the inferred target in the summary:

```text
Rollback target: <ref> (<short SHA>), last deployed to <env> on <date>
```

When none of the sources names a target, ask for it in the question round. Carry the user's ref as `rollback_ref` and
verify it with `git rev-parse --verify --end-of-options "${rollback_ref}^{commit}"`. Check the resolved target against
the artifact registry too.

No target, from the sources or the user, makes the item `fail: no rollback target`.

## Step 2: Check the target still deploys

A rollback target that no longer works is worse than none: the team believes it has a recovery path that does not exist.

1. **The artifact is still available:** the image is in the registry, the build is on the artifact server, the tag is
   neither deleted nor garbage-collected. Read the full image reference as `rollback_image`, the release tag as
   `rollback_tag`, and the GitHub repository as `target_repo` from the identified rollback artifact. Pass each value as
   a single argument.

    ```bash
    # Container image: exit 0 means the manifest exists
    docker manifest inspect -- "$rollback_image"

    # GitHub release: lists its assets, and fails when the release does not exist
    gh release view --repo "$target_repo" --json assets -- "$rollback_tag"
    ```

    Run gh's [host preflight](../../../gh/SKILL.md#part-1-github-mcp-server-primary) before the `gh` command. Off
    `github.com`, include the host in `target_repo`. On a host gh does not support, ask the user whether the release
    exists before judging its availability.

2. **The infrastructure is still compatible.** The target's database schema, environment variables and configuration
   must match what the deploy leaves behind. A migration in this deploy can leave a schema the target cannot run on,
   which is the most common rollback failure. When the deploy has a migration, ask in the question round:

    ```text
    ❓ **Q<n>** - **Schema compatibility**: The rollback target <ref> was built against schema <version>, and this deploy runs <N> migrations. Can <ref> run on the migrated schema? Answer yes, no (a rollback also needs a schema revert), or unsure.

    ➡️ <no, when any migration is irreversible; yes, when each migration is shown forward-compatible; otherwise unsure>
    ```

    With no migration, the check is `n/a: no migration`.

An unavailable artifact, `no`, or `unsure` makes the item `fail`, naming which.

## Step 3: Stage the rollback command

Write the rollback command out before the deploy, ready to copy, so nobody builds it under pressure. Write it once when
it is the same in Bash and PowerShell 7, and give a second block only where the syntax differs.

```text
Rollback command (staged, not run):

<rollback command, for example: kubectl rollout undo deployment/<name> -n <namespace> --to-revision=<rev>>
```

Usual rollbacks by deploy mechanism:

| Deploy mechanism      | Usual rollback                                                                         |
| --------------------- | -------------------------------------------------------------------------------------- |
| Kubernetes Deployment | `kubectl rollout undo deployment/<name> -n <ns> --to-revision=<rev>`                   |
| Helm release          | `helm rollback <release> <revision> -n <ns>`                                           |
| Terraform             | Revert the configuration to the previous commit, then `terraform plan` and `apply`     |
| A platform service    | `<platform> releases:rollback <release-id>`                                            |
| A database migration  | The migration's down step, only when it is reversible; otherwise a restore from backup |
| Anything else         | Ask the user for the team's rollback command; do not guess                             |

A mechanism that matches more than one row (a Helm release that runs a migration) lists the command for each.

## Step 4: Estimate the time to recover

From "decided to roll back" to "customers see recovery":

- **Command:** seconds to minutes.
- **Propagation:** cache, load-balancer convergence and replica restarts; often the longest part.
- **Verification:** until a synthetic check passes and the metrics return to baseline.

```text
Time to recover: about <total> (command <a>, propagation <b>, verification <c>)
```

When the team states an acceptable recovery time for this service, a longer estimate is `fail`, with a canary deploy as
the suggested fix. With no stated limit, report the estimate and do not judge it.

## Step 5: Irreversibility

When the deploy carries an irreversible migration that Section 2 confirmed, rolling back the application does not
restore the earlier state. Full recovery takes three steps:

1. Roll the application back to the target with the staged command.
2. Restore the database from a snapshot taken before the migration, or replay events to that point.
3. Reconcile the data written between the snapshot and the rollback.

Ask in the question round whether a current restore runbook exists, and where. A runbook that is missing or stale (one
`/agentics:ops audit` would flag) makes the item `fail: no current restore runbook`, with `/agentics:ops author` and
`/agentics:ops drill` as the fix.

## Summary line

The checklist summary carries one line for this section, and the monitor window reuses it:

```text
Rollback: <pass | fail: <reason>>: target <ref>, command staged, about <duration> to recover<; schema revert needs restore runbook <path>>
```

## When no clean rollback exists

An irreversible migration with no restore runbook, a new external dependency the target cannot reach, or data already
visible to users that a rollback would invalidate: each leaves no clean rollback, and the deploy itself may not be safe.
Report it as the item's failure and name the safer options: a canary deploy, a split into smaller deploys that each have
a rollback, or waiting until the cause is addressed. The user decides, as the workflow's no-go handling says.

## Composition with other skills

- **`/agentics:ops author`** and **`/agentics:ops drill`** write and test a restore procedure before an irreversible
  deploy relies on it.
- **`/agentics:ops response`** tracks an executed rollback as an incident; the hand-off context from the monitor window
  fills its first record.
