# Wiring probe

`scaffold audit probe` runs the readiness audit, then proves that selected checks can fail. It is opt-in: a bare audit
never probes.

## Preview

Before any probe runs, pick a `<run-id>` once for this run. It is unique on the host, so concurrent runs never share a
path. An example is `probe-` plus a UTC timestamp and a random suffix.

List every probe in one preview. Give these parts for each probe:

- The check's command.
- The control run: that same command, run unmodified in the disposable worktree before anything is planted.
- The planted failing input:
    - For a test runner, a failing test file in a covered path.
    - For a linter, a single violation of an enabled rule.
- The disposable worktree path: `<temp>/scaffold-probes/<run-id>/<probe>`. `<probe>` is a short name that is unique
  within the run, such as `01-test`.

Probe only rows that the audit graded `ran` and whose check is a test runner or a linter. These are the two kinds with a
defined planted input. A build, typecheck, or real-app row stays `ran` with the reason "no planted input for <kind>".
The preview never invents a mutation for such a row.

Ask once for approval of the whole list, as a ❓ question with a ➡️ recommendation. The preview is a process gate
([gates.md](../../routing/references/contracts/gates.md#process-gate)). When the user declines the list ("skip the
list"), name its cost once: the planted inputs run unseen. On the next reply that still declines, run the probes without
it. A no to the probes themselves runs nothing, and the report keeps those rows at `ran`. The probes never touch the
user's tree, whatever the user says.

## Run

A probe tests HEAD, so it needs a clean tree. When `git status --porcelain` lists anything, run no probe:

- Every selected row stays `ran` with the reason "uncommitted changes; the probe would test HEAD, not the audited tree".
- The report asks the user to commit or set the changes aside and rerun.

Before you create any worktree, record these in the user's tree:

- `git status --porcelain`
- `git diff HEAD`
- `git worktree list --porcelain`

For each approved probe, set `<temp-path>` to `<temp>/scaffold-probes/<run-id>/<probe>`, where `<temp>` is the operating
system's temporary directory. Refuse a path inside the repository or its git directory. Refuse a path that already
exists, because this run did not create it. Then create a detached worktree at HEAD with
`git worktree add --detach "<temp-path>" HEAD`. Quote the path, because a temporary directory can contain spaces. Git
creates the missing parent directories. Record each `<temp-path>` that this command creates.

A fresh worktree holds only tracked files. Ignored files, such as installed dependencies and build output, are absent.
Both runs execute the branch's own code, so they run only as
[Running it](../../routing/references/repo-conventions.md#running-it) allows. Run the control first, and plant nothing
until it has passed:

1. **Control run.** In the probe worktree, run the check's exact command from the directory that corresponds to its
   recorded working directory. The control passes when it exits zero and its own output shows that tests or checks
   executed. A control that fails, errors, times out, or executes nothing proves nothing about the check. Do not plant
   an input for that probe. The row stays `ran` with the reason "control run failed: <exit result and one line of
   output>". Do not install dependencies to make the control pass on your own. When the control failed only because
   ignored dependencies are missing and the repository has a lockfile, ask one yes/no question that names the exact
   install command for the probe worktree, with install scripts off (for example `npm ci --ignore-scripts`). On a yes,
   run it in the probe worktree only and run the control again. On a no, the row stays `ran`.
2. **Planted run.** Plant the input only in the probe worktree. Run the same command from the same directory. Record the
   exit result and output evidence.

Never plant, edit, stage, or commit anything in the user's working tree or on any branch.

The temp root, snapshot, and scoped removal mirror zealot tests mode
([workflow.md](../../zealot/references/tests/workflow.md), Steps 2 and 6). Unlike zealot, a probe never carries
uncommitted changes, so it requires a clean tree.

## Clean up

Remove exactly the worktrees that this run recorded, one at a time, with `git worktree remove --force "<temp-path>"`.
Remove each one after its run. Remove it also when the check errors or times out. Before each removal, confirm that the
path is one of this run's recorded `<temp-path>` values. A path that matches only the `<temp>/scaffold-probes/` prefix
is not enough. Never remove any other path, including another run's directory under that prefix. Report a recorded path
that cannot be removed.

Then delete this run's `<temp>/scaffold-probes/<run-id>/` directory if it is empty. Leave `<temp>/scaffold-probes/`
itself in place.

Run the three recorded commands again in the user's tree. Confirm that the output matches the records taken before the
probe.

## Report

Grade a probe by comparing the control run with the planted run:

| Control run | Planted run                                            | Row                                                                             |
| ----------- | ------------------------------------------------------ | ------------------------------------------------------------------------------- |
| passed      | failed, and the output names the planted input         | `protective`, with the control and planted evidence                             |
| passed      | passed                                                 | `failed` with the defect "check cannot fail on <input>"                         |
| passed      | failed, and the output does not name the planted input | stays `ran`, with the reason "failed for a reason other than the planted input" |
| failed      | not run                                                | stays `ran`, with the control failure as the reason                             |

A check that fails on the planted input is not `protective` unless the control passed in the same worktree. Without the
control, a missing dependency or an unrelated failure looks the same as a check that works.

A probe that could not run leaves the row at `ran` with the reason. The cases are:

- Worktree creation refused.
- A prerequisite missing.
- A dirty tree.
- An unsupported check kind.
