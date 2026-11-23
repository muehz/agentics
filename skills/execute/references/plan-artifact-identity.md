# Plan artifact identity

This file holds the rule that the **Plan artifact identity** paragraph of
[`SKILL.md`](../SKILL.md#step-1-load-review-and-compute-waves) points to.

**Plan artifact identity.** Derive a non-repository plan's destination from its accepted content and the current branch.
Never use a provider name, source filename, or session path for it.

**Seed.** If HEAD is on a non-default branch, the seed is the branch name with everything through its last `/` removed.
The default branch is the first of these that resolves:

- the branch `origin/HEAD` names;
- a branch literally named `main`, when one exists;
- a branch literally named `master`, when one exists.

Use the content seed in any of these cases:

- HEAD is on the default branch.
- HEAD is detached.
- None of those three checks resolves a default branch.
- The branch seed slugifies to nothing.

The content seed is the first of these that exists:

- the first non-placeholder `**Goal:**` value;
- the first task heading with its `Task N:` label removed;
- `execution`.

Never guess an unresolved default branch. Fall through to the content seed. A wrong guess can seed the real default
branch's own work from its name.

**Purpose.** The purpose words of decompose are `feature`, `refactor`, `upgrade`, `infrastructure`, `architecture`,
`data`, `process`, and `design`. If the content seed begins with one of them, do these steps:

1. Use that word as the prefix.
2. Remove it from the content seed.

Otherwise, use `feature`. The purpose always comes from the content seed, never from the branch.

**Slug.** Make the slug from the seed in these steps:

1. Lowercase the seed.
2. Replace every maximal run outside ASCII `a-z` and `0-9` with one hyphen.
3. Trim leading and trailing hyphens.
4. If the result is longer than 40 characters, cut it at the last hyphen at or before the 40th character. If there is no
   hyphen, cut it at the 40th character. Then trim trailing hyphens again.
5. Use `execution` if nothing remains.

The resulting `<topic>` is `<purpose>-<slug>`. On branch `feat/417-spec-policy-conflicts` the folder is
`feature-417-spec-policy-conflicts`. A harness-generated branch `worktree-bridge-cse_01V4p7u34RpkNgnL9GsaFbqB` slugifies
to 44 characters and is cut to `worktree-bridge-cse`. (`chain-artifacts.md` writes the folder as
`YYYY-MM/<purpose>-<topic>`, where its `<topic>` is this rule's `<slug>`). Apply this rule by hand in every case.

Serialize the persisted copy with LF line endings and exactly one trailing newline. The destination is a work folder per
[../decompose/references/chain-artifacts.md](../../decompose/references/chain-artifacts.md). Its logical folder is
`<plan-parent>/<YYYY-MM>/<purpose>-<slug>`, and the [storage setting](flow-state.md#storage) decides which directory
holds its files. First search both stores, `<plan-parent>/*/<purpose>-<slug>/plan.md` and
`<git-common-dir>/agentics/work/<plan-parent>/*/<purpose>-<slug>/plan.md`, for a file whose body below the header is
identical. If a file matches, do these steps:

1. Reuse the first match. Its logical folder is the one its month and name give.
2. In a committed work folder, stage it.

Otherwise the logical folder is `<plan-parent>/<YYYY-MM of today>/<purpose>-<slug>`. Run
`agx flow path --folder <logical folder>`, as [flow-state.md](flow-commands.md#commands) shows. It prints the absolute
directory that holds the folder's files, or the one `flow log` would create, and creates nothing. The destination is
`plan.md` in that directory. Pass the logical folder, never that directory, to every later `flow` call. When the
destination exists with a different body, do these steps:

1. Stop before the first dispatch.
2. Report both paths.

Never overwrite a different plan. Never add a suffix to the folder name. A folder that `intent.md` or `spec.md` already
created is this plan's home, not a collision. Execute leaves those siblings untouched.

This applies only to a plan with no owner-written repository file. Such a plan is an inline task list, a session plan
file, or a paste. When such a plan does not already open with the provenance header from that reference, prepend it:
`derived-from: none`. The go-ahead Step 0 requires is the approval. The `approve plan.md` row records it per **Plan
approval record**. No header field carries it. A plan that already carries a header keeps that header exactly as found.
Compare candidates on the body below the header. A header difference does not turn a reused plan into a collision.
