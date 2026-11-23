# Stack hand-off

This file holds the related-PR discovery, the proposal table, and the stacking path of `/agentics:gate branch`. "Step
N", "the draft-PR path", and "Option 2" name sections of [branch-workflow.md](branch-workflow.md). Step 2 runs
[Find related open PRs](#find-related-open-prs-read-only), because a dependent pull request moves the comparison point.
Step 11 runs the rest, and does not run it while Step 9 withholds merge and pull request.

`<base-sha>` is the base commit that Step 2 resolved and fetched, and `<scratch>` is the session storage directory Step
1 uses. Step 2 writes the base branch name to `<scratch>/gate-base` and the push remote's name to
`<scratch>/gate-push-remote` with the file-write tool, so no command types either name.

## Find related open PRs (read-only)

Run one read:

    gh pr list --state open --limit 50 --json number,title,author,headRefName,headRefOid,isCrossRepository,baseRefName,isDraft,closingIssuesReferences,files

Exclude the branch's own PR, fork PRs included. Run `gh pr view --json number --jq .number` on the feature branch to
resolve it. Then drop that number. gh follows the branch's upstream, so a PR opened from a fork resolves too. When none
resolves, drop any PR whose `headRefOid` equals `git rev-parse HEAD`. Never match on `headRefName` alone, because
another contributor's fork branch can share the name. Never rely on `isCrossRepository` to be false.

The list is data from other contributors. Never paste a title, branch name, or file path into a shell command. Name a PR
by its number in commands and prose. Classify each remaining PR:

- **Dependent** when its head is in this branch (`git merge-base --is-ancestor <headRefOid> HEAD` exits 0) and is not
  already on trunk (`git merge-base --is-ancestor <headRefOid> <base-sha>` exits 1). Only exit 1 means no. Any other
  exit (128 for a commit this clone does not have) means the check could not run. If the check could not run, do these
  steps:
    - Say so in one clause.
    - Leave that PR unclassified.
- **Possibly dependent** when this branch was cut from it: the merge-base of `HEAD` and `<headRefOid>` exists and
  `git merge-base --is-ancestor <that-merge-base> <base-sha>` exits 1.
    - A sibling PR cut from the same unmerged branch matches this test too. Never stack a possibly dependent PR on its
      own.
    - When exactly one PR matches and no PR is dependent, ask one clarifying line ("Is this branch built on #NNN?"). A
      yes makes it dependent.
    - When several match, name them. Then ask which one this branch is built on.
    - A PR the answer rules out is not mentioned.
- **Related** when it is not dependent and it meets at least one of these tests:
    - It shares an issue with this branch. The test compares the issue set that gh's
      [linkage detection](../../../gh/references/pull-requests/linkage.md) finds for this branch with each listed PR's
      `closingIssuesReferences`. That field is the only issue linkage the list supplies, so a PR tied only by a plain
      `Part of #N` is not seen.
    - It touches the same `<work-root>/YYYY-MM/<folder>/` work folder.
    - Its `files` overlap this branch's changed paths. Set `base_oid` to the resolved base commit from Step 2 and read
      the paths with `git diff --name-only "$base_oid...HEAD"`.
- Anything else is not mentioned.

A dependent PR whose `isCrossRepository` is true (opened from a fork) cannot be stacked. GitHub requires every branch in
a stack to live in one repository
([creating stacked pull requests](https://docs.github.com/en/pull-requests/how-tos/create-pull-requests/creating-stacked-pull-requests)).
For such a PR, do these steps:

1. Name it as a dependency.
2. Skip its `stack` read.
3. Take the last row of the table below: no stack, and a draft against `<base-branch>` that repeats that PR's diff.

For each other dependent PR, read the GraphQL `stack` field with gh's
[definition-of-done query](../../../gh/SKILL.md#definition-of-done-github-confirms-the-stack). The list read itself can
fail: a very large PR can make the `files` field fail. When `gh`, authentication, or a remote is missing, or the list
read itself fails, do these steps:

1. Skip the search.
2. Say in one clause that related PRs could not be checked.

When the list returned 50 PRs, say in one clause that only the 50 most recent open PRs were checked. `gh` stops there,
so an older lower PR would be missed.

## Find sibling slice branches (read-only)

This section runs in Step 11, after the open-PR read of Step 2. It does not run while Step 9 withholds merge and pull
request. When that read was skipped or failed, the PR-backed tests here cannot run. The slice chain is not checked, and
the plain draft proposal against `<base-branch>` stands.

This section applies only to a sliced plan. Find that plan from the branch's flow. Run `flow where` from the feature
worktree ([../emission.md](../emission.md), "Where it goes"). It prints the work folder the branch is bound to. A slice
branch such as `feat/export-s2` finds the same folder through its `bind` row, so no branch-name match is needed.

- When it exits 3 or 5, skip this section and say so in one clause.
- When it exits 4, ask which flow the branch belongs to. Then continue with that folder.
- When the folder's `plan.md` has no `## S<n>` slice sections, skip this section and say so in one clause.

From that `plan.md`, read each active slice's ID and **Dependencies**. Only entries that are slice IDs are edges. A
named external decision or `none` adds none. A slice marked `Retired:` is not in the graph.

**Recorded names.** The base branch and push remote names are data like any branch name. Commands read them back from
`<scratch>/gate-base` and `<scratch>/gate-push-remote`, as the first lines of each recipe below do.

**Candidates.** List the local and push-remote branch heads that are in this branch and not already on trunk, lowest
first. Each line is the commit count above trunk, the head, the full ref, and the remote value. For a local branch, the
remote value is how the push remote's ref of the same name stands, whether or not the branch tracks it. The remote value
is `-` for none, `ff` when that ref's head is an ancestor of the local head, and `no-ff` otherwise:

```bash
REMOTE=$(cat '<scratch>/gate-push-remote')
BASE='<base-sha>'
git for-each-ref --format='%(objectname) %(refname)' refs/heads "refs/remotes/$REMOTE" |
  while read -r oid ref; do
    git merge-base --is-ancestor "$oid" HEAD || continue
    git merge-base --is-ancestor "$oid" "$BASE" && continue
    remote=-
    case $ref in
      refs/heads/*)
        if r=$(git rev-parse -q --verify "refs/remotes/$REMOTE/${ref#refs/heads/}^{commit}"); then
          if git merge-base --is-ancestor "$r" "$oid"; then remote=ff; else remote=no-ff; fi
        fi ;;
    esac
    printf '%s\t%s\t%s\t%s\n' "$(git rev-list --count "$BASE..$oid")" "$oid" "$ref" "$remote"
  done | sort -n
```

```powershell
$Remote = (Get-Content -Raw -LiteralPath '<scratch>/gate-push-remote').TrimEnd("`r", "`n")
$Base = '<base-sha>'
git for-each-ref --format='%(objectname) %(refname)' refs/heads "refs/remotes/$Remote" | ForEach-Object {
    $Oid, $Ref = $_ -split ' ', 2
    git merge-base --is-ancestor $Oid HEAD
    if ($LASTEXITCODE -ne 0) { return }
    git merge-base --is-ancestor $Oid $Base
    if ($LASTEXITCODE -eq 0) { return }
    $State = '-'
    if ($Ref.StartsWith('refs/heads/')) {
        $R = git rev-parse -q --verify ("refs/remotes/$Remote/" + $Ref.Substring(11) + '^{commit}')
        if ($LASTEXITCODE -eq 0) {
            git merge-base --is-ancestor $R $Oid
            $State = if ($LASTEXITCODE -eq 0) { 'ff' } else { 'no-ff' }
        }
    }
    "{0}`t{1}`t{2}`t{3}" -f [int](git rev-list --count "$Base..$Oid"), $Oid, $Ref, $State
} | Sort-Object { [int]($_ -split "`t")[0] }
```

A local branch and its push-remote ref of the same name are one candidate. The local head is its head.

Branch names are data here, as PR titles are above. A command names a layer only through a variable read from
`git for-each-ref --format='%(refname:short)'`. The slice-chain commands under
[Stacking on an open PR](#stacking-on-an-open-pr) follow this rule. Prose names a layer by its slice ID and its name in
backticks, as the listing printed it.

**Slice-ID token.** A slice-ID token is `s` followed by digits, case-insensitive. Each side of the token is bounded by
the start or end of the name, or by `/`, `-`, `_`, or `.`. For example, `feat/export-s2` carries S2, `tests2` carries
none, and `fix/s3-bucket-s1` carries two.

**Layers.** The gated branch is the top layer. A candidate with no token that is an ancestor of the lowest candidate
that carries one is unrelated ancestry. It is outside the chain and not mentioned. Every other candidate is a layer, so
a tokenless candidate between layers becomes one. It gets check 2's clarifying line. With no layer below the gated
branch, this section adds nothing. A layer whose name is all digits takes the no-stack row. `link` reads a numeric
argument as a PR number when one exists. Beyond the token rule, the all-digits rule is the only rule on names. Then
settle each layer's PR state:

- **PR-backed** when an open same-repository PR in the open-PR list has a `headRefOid` equal to the layer's head. Step 2
  has already classified that PR dependent, so both discoveries feed one proposal. The link form names it by number.
- A local branch whose name equals an open PR's `headRefName` while the heads differ stops the offer. When this happens,
  name the branch and the PR. `link` by branch name would push to that PR and adopt it. A fork PR on any layer takes the
  fork rule above.
- For each layer with no PR in the list, read the PRs that GitHub ties to its head. Do the same for the gated branch's
  own head. The head is hex, so it can be typed. The command is identical in both shells:
  `gh api "repos/{owner}/{repo}/commits/<oid>/pulls" --jq '.[] | [.number, .state, .head.sha, (.head.repo.full_name // "-")] | @tsv'`.
    - Only a PR whose head equals the layer's head counts.
    - An open PR makes the layer PR-backed, since the list stops at 50. It counts only after the guards that a listed PR
      gets:
        - A head repository other than `{owner}/{repo}` (`-` when the fork is gone) takes the fork rule above.
        - Its `stack` field is read as a dependent PR's is.
        - Step 11's readiness check covers it as a lower PR.
    - A merged or closed PR makes a layer below the gated branch a **merged slice**. That branch is never re-offered and
      leaves the layers.
    - The gated branch is never a merged slice. It always stays the top layer.
    - When the gated branch's own head has a merged or closed PR, say so in one clause and offer no slice chain, so the
      plain draft proposal stands.
    - An error that says GitHub has no such commit means the layer was never pushed and has no PR.
    - Any other failure means the chain could not be checked. When that happens, say so in one clause and offer no slice
      chain.
- The list can also miss a same-name PR on another head.
    - Read the open PRs whose head branch has the layer's name. Read the name into `Bk`, as the slice-chain commands
      read it.
    - This read covers each local layer below the gated branch that still has no PR. The gated branch's own PR is Step
      11's.
    - GitHub applies the `owner:branch` filter, so 50 is no limit here. `-F` (not `-f`) is what expands `{owner}` in a
      field:
      `gh api -X GET "repos/{owner}/{repo}/pulls" -f state=open -F head="{owner}:$Bk" --jq '.[] | [.number, .head.sha] | @tsv'`
      (PowerShell: `-F "head={owner}:$Bk"`).
    - A PR on a different head stops the offer, as the same-name rule above does.
- A layer that exists only as a remote-tracking ref and is not PR-backed takes the no-stack row. `link` pushes branch
  arguments, and such a layer has no local branch to push.
- A local layer whose same-name push-remote ref reads `no-ff` takes the no-stack row too, whether or not the branch
  tracks that ref. `no-ff` means the remote holds commits the local branch lacks, so `link`'s push would not
  fast-forward.

When no layer is left below the gated branch, this section adds nothing. This holds when none was found and when each
one left as a merged slice. `link` needs at least two PRs, so Step 11's PR classification alone decides the proposal.
The proposal is a plain draft against `<base-branch>` when nothing is dependent.

**Checks.** The layers are a **slice chain** only when all of these hold. Name each failure except the association's in
one clause. Each of those failures takes the no-stack row: no stack, and a draft against `<base-branch>`, worded as the
table's last row words it.

1. **Ancestry.** Each layer's head is an ancestor of the next one up, and the chain ends at `HEAD`. Each head differs
   from the layer below it (for the bottom layer, from `<base-sha>`).
2. **Association.** Every layer carries exactly one token that names an active slice. No two layers share a slice or a
   head.
    - Otherwise, ask one clarifying line, as the possibly-dependent rule does. In the line, name the layers bottom to
      top and the slices they would carry. For example: "Is this branch chain S1, S2, S3?"
    - A yes supplies the association, and the checks go on.
    - Any other answer drops the slice chain. Step 11's PR classification alone then decides the proposal.
3. **Order.** Every dependency of a chain slice on another chain slice points at a lower layer. Name an edge that points
   upward.
4. **Linearity.** Each layer's slice depends, directly or transitively, on the slice of the layer below it. For example,
   S3 on S1 and S2 with S2 on S1 is a line.
    - Two chain slices can have no dependency path between them, such as S2 on S1 and S3 on S1.
    - When that happens, nothing says which builds on the other. Name the pair.
5. **Completeness.** Every active slice that a chain slice depends on, directly or transitively, has a layer or is a
   merged slice below the bottom layer.
    - Slices outside that dependency closure are ignored. A slice not built yet above the gated one, or an independent
      slice, never blocks the offer.
    - A missing slice whose branch left the listing because its head reached trunk still counts as merged. That holds
      when a branch that carries its token has a head whose merged or closed PR the read above finds. Rerun the listing
      without its two ancestry filters to find such a branch.
    - When no branch carries its token any more, read the merged PRs once:
      `gh pr list --state merged --limit 100 --json number,headRefName,headRefOid,isCrossRepository`. This happens when
      GitHub deleted the merged branch and the local one was pruned. The list is as untrusted as the open-PR list.
    - The slice counts as merged when a same-repository PR there meets both of these tests:
        - Its `headRefName` carries exactly the slice's token.
        - Its `headRefOid` is in this branch (`git merge-base --is-ancestor <headRefOid> HEAD` exits 0).
    - A slice that merged after the layers above were cut from it is found by its PR, not its branch.
    - When the read returns 100 PRs and none matches, say in one clause that only the 100 most recent merged PRs were
      checked.
    - A chain slice that depends on an active slice that is neither a layer nor merged takes the no-stack row. Name that
      slice.
    - That row's plain draft against `<base-branch>` stays sound when the slice did merge unseen, because its changes
      are already on trunk.

**Stack-N form.** When the lowest PR-backed layer is already in stack N (its `stack` read above):

- Stack N's open entries must be the chain's lowest layers in chain order.
- Otherwise, the no-stack row names the mismatch.
- The link form is `gh stack link N` followed by the layers above it, not `--base`.

When the slice chain holds but Step 11's dependent PRs alone would not form one chain, the slice chain decides the
proposal. The proposal says so.

## Choose the proposal

| Discovery result                                                            | Proposal                                                                                                                                        |
| --------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Nothing dependent                                                           | A draft PR against `<base-branch>`. The first note below the table adds detail.                                                                 |
| Dependent PRs that form one linear chain (each head contains the one below) | Stack on the topmost. Name its stack number if present. Otherwise give the base and lower PRs in bottom-to-top order. Use the link forms below. |
| A slice chain of two or more sibling slice branches                         | One stack of every layer, bottom to top, each named with its slice ID. The second note below the table adds detail.                             |
| Dependent PRs that do not form one chain                                    | Name every dependent PR. Recommend no stack. Propose a draft against `<base-branch>`, and note that it repeats those PRs' diffs.                |

- **Nothing dependent.** Name each related PR with its overlap. Flag a likely conflict for path overlap. Never recommend
  stacking a related PR.
    - A release or other long-lived `<base-branch>` is the same case only when no open PR has it as its head. Without
      such a PR there is no stack. Otherwise, [item 7](../../../gh/SKILL.md#pull-request-creation-workflow) decides.
- **A slice chain of two or more sibling slice branches.** Say that `link` pushes every branch. Say that it creates a
  draft PR for each of the K layers with none. State K. The link form is one of these:
    - The base form in [Stacking on an open PR](#stacking-on-an-open-pr). List the layers bottom to top, with a PR
      number for a PR-backed layer and a branch variable for one with none.
    - The stack-N form from [Find sibling slice branches](#find-sibling-slice-branches-read-only).

Before you offer a stack, run gh's readiness check on each lower PR. The check covers state, mergeable, review decision,
checks, and unresolved threads ([stack.md](../../../gh/references/stack.md#rebase-ripple), "Check before stacking on
existing work"). Put anything outstanding, plus the cascade cost, in the same sentence as the stack offer. Name each
lower PR by number and author. Say that `link` pushes this branch and also retargets and enrolls those PRs in the stack.
Say that once GitHub confirms the stack, each lower PR's description is rewritten to carry its layer position. That
rewrite touches PRs another author owns. When `gh extension list` shows no `github/gh-stack`, the offer carries
`gh extension install github/gh-stack` as its own clause. The clause says it installs third-party code that runs with
the user's GitHub credentials. The same yes authorizes it.

A slice chain's offer applies all of that to its PR-backed layers. The unpublished layers have no PR to check. The
offer:

- Names each layer bottom to top with its slice ID.
- Says how many draft PRs `link` will create.
- Says that `link` pushes every branch in the chain.
- Says when the slice chain, not Step 11's dependent PRs, is why a stack is offered.
- Names each merged slice that left the layers with its PR number.

When a merged slice's PR closed unmerged, its commits are still in this branch and not on trunk. The offer says that its
changes ride in the layer above it.

Write one or two sentences. For example:

- "Tests pass and no finding is open. The next step is a draft PR against main. Want me to push and open it?"
- "Tests pass and no finding is open. #NNN is still open and this branch is built on it. Want me to stack this on #NNN
  with `gh stack` so it reviews as its own layer? Any later change to #NNN cascades a merge into this layer."
- "Tests pass and no finding is open. This branch is S3 on top of the slice branches `feat/export-s1` (S1) and
  `feat/export-s2` (S2), none of them published. Want me to link all three as one stack with `gh stack`? `link` pushes
  the three branches and creates three draft PRs; the unpublished layers have no PR to check, and any later change to a
  lower layer cascades a merge into every layer above it."

## Stacking on an open PR

A yes to Step 11's stack proposal runs this path instead of the draft-PR path's push and its PR creation. `link` does
the push. Load `gh`'s stack mode in this session ([Part 3](../../../gh/SKILL.md#part-3-stacked-prs-stack-mode)), the way
the draft-PR path loads its PR creation workflow. Follow the PR creation workflow's non-trunk base rule (item 7). A
slice chain takes the same path. Each item says where its slice branches differ:

0. **Preflight.** Do these steps:
    - When `gh extension list` shows no `github/gh-stack` and Step 11's yes included the install, run
      `gh extension install github/gh-stack` first.
    - For a slice chain, rerun the sibling slice branch listing before the readiness re-check.
        - If a layer's head moved, a layer appeared or left, or a layer gained a PR, do these steps:
            - Stop.
            - Push nothing.
            - Step 11's offer runs again.
    - Re-run Step 11's full readiness check on each lower PR (open, mergeable, review decision, checks, unresolved
      threads).
    - If a lower PR merged or closed since, or a signal changed from what the offer disclosed, do these steps:
        - Stop.
        - Report it.
        - Push nothing.
        - Step 11's offer runs again with the new state.
    - Resolve issue linkage now, before `link` creates the PR, by
      [linkage.md](../../../gh/references/pull-requests/linkage.md). Scan from the topmost lower PR's head rather than
      `<base-branch>`, so the lower layers' issues stay out of candidate detection.
    - For a slice chain, the readiness re-check covers only the PR-backed layers, because an unpublished layer has no PR
      to check.
    - For a slice chain, resolve linkage for every layer that `link` will create a PR for. Use the slice's **Issue**
      field when it is set. Otherwise, follow linkage.md and scan from the head of the layer below (from `<base-branch>`
      for the bottom layer).
1. Run the `link` form that Step 11 proposed.
    - Read the branch from the recorded name, as the draft-PR path reads it. Pass it as `"$FEATURE_BRANCH"`
      (`$FeatureBranch` in PowerShell). Never type it.
    - Use `gh stack link N "$FEATURE_BRANCH"` when the lower PR is in stack N, otherwise
      `gh stack link --base "$BASE_BRANCH" <lower PRs, bottom to top> "$FEATURE_BRANCH"`.
    - Read `BASE_BRANCH` (`$BaseBranch`) from the [recorded base](#find-sibling-slice-branches-read-only). This item
      writes the recorded base first when slice discovery did not.
    - `link` pushes the branch and creates its PR as a draft. The help text does not document the default, so confirm it
      with `gh pr view <n> --json isDraft`. Never pass `--open`.
    - Name lower PRs by number, never by a title or branch name from the discovery list.
    - When `link` fails outright, stop. Then report its output.
    - A slice chain runs the form its offer proposed through the slice-chain commands below this list. Then it reads
      every layer's PR with `gh pr view "$Bk" --json number,isDraft,headRefOid` (`"$FEATURE_BRANCH"` for the top). It
      confirms that each PR `link` created is a draft on that layer's head.
2. Run gh's definition-of-done check on the new PR. The `stack` field must list the lower PRs and the new one in order.
    - If the check exits 9 or returns a null `stack`, follow item 7:
        - Report "base-chained, not a stack: stacked PRs aren't enabled here".
        - Report the web UI finish (**Create stack** or **Add to stack**).
        - Never call it a stack.
    - The branch is pushed and the draft exists either way, so the gate still goes on to item 3 and Step 14. The gate's
      report says which of the two it is.
    - For a slice chain, the `stack` field must list every layer bottom to top, above any entries stack N already held.
    - For a slice chain, a null `stack` report names every draft PR `link` created.
3. Write the new PR's body through `gh` with the draft-PR path's evidence and item 0's linkage.
    - When item 2 confirmed the stack, do these steps:
        - Place the evidence and the linkage per
          [body.md](../../../gh/references/pull-requests/body.md#native-stack-layers).
        - Write every adopted lower PR's body bottom to top by the same section. gh's adoption workflow requires these
          edits, and the stack offer disclosed them. The stack yes covers those edits.
    - When item 2 did not confirm the stack, do these steps:
        - Write an ordinary draft PR body.
        - Leave the lower PRs' bodies alone, since no PR is a stack layer.
    - For a slice chain, a confirmed stack gets every layer's body, created or adopted, bottom to top by that section.
    - For a slice chain, a null `stack` gets an ordinary draft PR body on every PR `link` created. It leaves adopted PRs
      alone.
    - For a slice chain, each created layer carries its own linkage from item 0.
    - For a slice chain, each lower layer's body says it was tested only as part of the top layer. Step 7 tested the top
      layer.

**Slice-chain commands.** Each `Bk` is read from the head that the listing printed. The head is hex, so it can be typed.
There is one `Bk` per layer with no PR below the top. A PR-backed layer takes its PR number in that place. A chain
already in a stack takes the stack-N form from [Find sibling slice branches](#find-sibling-slice-branches-read-only).
Each variable must hold exactly one name. When a variable is empty, the branch moved since the listing, so stop. When a
variable holds more than one name, two branches share that head, so name them. Then stop. For three layers with no PR:

```bash
FEATURE_BRANCH=$(cat '<scratch>/gate-branch')
BASE_BRANCH=$(cat '<scratch>/gate-base')
B1=$(git for-each-ref --points-at <layer-1-head> --format='%(refname:short)' refs/heads)
B2=$(git for-each-ref --points-at <layer-2-head> --format='%(refname:short)' refs/heads)
[ -n "$B1" ] && [ "$(printf '%s\n' "$B1" | wc -l)" -eq 1 ] &&
  [ -n "$B2" ] && [ "$(printf '%s\n' "$B2" | wc -l)" -eq 1 ] &&
  gh stack link --base "$BASE_BRANCH" "$B1" "$B2" "$FEATURE_BRANCH"
# item 1's check, after re-reading and re-checking the variables the same way, once per layer:
gh pr view "$B1" --json number,isDraft,headRefOid
```

```powershell
$FeatureBranch = (Get-Content -Raw -LiteralPath '<scratch>/gate-branch').TrimEnd("`r", "`n")
$BaseBranch = (Get-Content -Raw -LiteralPath '<scratch>/gate-base').TrimEnd("`r", "`n")
$B1 = git for-each-ref --points-at <layer-1-head> --format='%(refname:short)' refs/heads
$B2 = git for-each-ref --points-at <layer-2-head> --format='%(refname:short)' refs/heads
if (@($B1).Count -eq 1 -and @($B2).Count -eq 1) { gh stack link --base $BaseBranch $B1 $B2 $FeatureBranch }
# item 1's check, after re-reading and re-checking the variables the same way, once per layer:
gh pr view $B1 --json number,isDraft,headRefOid
```

Then write the trail rows (Step 14). Arm the check-back as the draft-PR path does. [watch.md](watch.md) also reads the
stack's lower layers. Step 14's push of the row commit goes through git's
[push workflow](../../../git/references/push.md#before-the-push), inside the same yes.
