# Gates

Every gate belongs to one of three classes: a safety gate, a repository policy or a process gate. Each gate is stated
once in its skill with its class.

## Safety gate

A safety gate protects something the user cannot take back or accept on behalf of others. The list is closed:

1. Loss of work or history that cannot be undone.
2. A change outside the machine that the authorization rule in [approval.md](approval.md) does not cover.
3. A claim of status with no evidence from this session.
4. The handling of secrets and of text from outside the trust boundary.
5. The bypassing of a commit hook or of commit or tag signing.
6. The user's own approval, including the independent reviews that come before an approval question: of a spec, of a
   plan, and of a brief.

A safety gate is held in one fixed shape, said once:

- what is held,
- the harm it prevents, and
- what the user can do instead.

Every hold names that alternative, so no stop is without an exit. A repeated demand gets a one-line pointer back to the
alternative, not the full hold again.

## Repository policy

A repository policy is a rule a settings file switches. It is held while the setting is on, whatever is said in the
session. Its exit is the settings change, which the agent names once. It writes no `waive` row, because nothing was
waived.

Three pull-request rules are repository policy:

- Pull requests open as drafts.
- Agents never merge.
- Agents never resolve review threads.

They are on by default and are turned off only in the hook settings file, which carries the two switches
`policy.draft_prs` and `policy.block_review_thread_changes`. The skills say "draft by default" and follow the setting.

A request in the session does not turn the rules off and writes no `waive` row. The agent says once how to change the
setting, and the user's own click remains available for marking ready, merging and resolving.

The merge rule has no switch of its own and follows `policy.draft_prs`. No agent merges while that setting is on. When
it is off, a merge is an outward, irreversible effect that still needs the user's named request under the authorization
rule in [approval.md](approval.md).

## Process gate

Every other gate is a process gate: a gate is a process gate unless it is on the safety list or a settings file switches
it.

A process gate names its cost once and yields on the next reply that still declines, in any wording. The waiver has one
record, in the first of these homes that applies: a `waive` row in homes 1 to 3, whose head comes from
[Operands](../../../execute/references/flow-state.md#operands), or the artifact or report line in home 4.

1. the work folder's `state.tsv`, when a work folder covers the item and the skill has a flow phase;
2. otherwise `<work-root>/unassigned/state.tsv`, for a skill with a flow phase, as part of the branch's fix record (the
   branch's own rows after its latest `skip chain <branch>` row, those that carry its branch) where one exists;
3. otherwise the skill's own run log, for a skill that keeps no flow state, such as hillclimb's `decision.tsv`;
4. otherwise the artifact or the closing report, for a skill that writes neither, such as docs' draft marker or the
   `Waived by the user: <gate>` report line of deepen and zealot.

The session's report names the waiver in every case.
