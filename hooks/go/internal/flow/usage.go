package flow

const usage = `Usage: flow log <phase> <event> <target> <status> <why> <evidence> <next> [--folder <dir>] [--expect-blob <hash>] [--entry <n|last>]
       flow status [--folder <dir>] [--metrics]
       flow where
       flow bind [--folder <dir>] [--slice <ids>]
       flow path [--folder <dir>]
       flow audit [--folder <dir>] --base <sha>

log     appends one row to <dir>/state.tsv (the bound flow when --folder is omitted).
        evidence "auto" stores text=<hash of the approved text> on an approve row
        and commit=<HEAD> patch-id=<id> on a verify, revert, or diverge row.
        "auto:<sha>" names a commit other than HEAD; a commit row takes only
        this form.
        --expect-blob <hash> is required on an approve row with evidence "auto" and
        applies to no other row; it writes the row only if the artifact's current
        text hash, the one flow status prints, equals <hash>.
        --entry <n|last> applies to a diverge row only; it appends entry=<hash>
        of entry <n>, or the last one, of ## Divergences in the artifact the
        target names: intent.md, spec.md, plan.md or brief.md as its first word,
        otherwise plan.md, or brief.md when the folder has no plan. A rule row,
        target <task> rule or run rule, with --entry is also kept in the ledger.
status  prints the flow summary; --metrics adds seven measures.
where   prints the bound flow's folder.
bind    appends a bind row naming the current branch, once; a second call writes
        nothing. Never on the default branch, a detached HEAD, a small fix, or a
        flow whose verdict is on the default branch.
        --slice <ids> records slice=<ids>: one slice id S<n>, or several joined
        by commas. Then every branch row, and every row whose evidence holds
        commit=, gets the slice= pair of the branch. Ids on a branch bound with
        none write a new bind row; ids other than the bound ones are refused.
path    prints the absolute directory that holds the folder's files (the bound
        flow's when --folder is omitted): the folder itself under the work root,
        or its local-store directory. A folder not created yet prints where log
        would create it. Creates nothing.
audit   prints one line per finding for the commits of <sha>..HEAD, then
        audit: clean or audit: <n> findings: unaccounted: commits no record
        names, mismatch: recorded commits that no longer match, out-of-scope:
        changed paths no task of the plan lists, and invalid: verify rows.
        note: lines name exempt commits, merge commits, and, when the scope
        cannot be checked, note: scope not checked (<reason>). The bound
        flow when --folder is omitted; a small fix in progress (lookup exit
        5) audits the small-fix record instead, and its scope is not
        checked. A branch not bound to the flow prints audit: not run. Every
        mode, pair included, is audited. Reads only, and exits 0 whatever it
        finds; a missing --base, or a <sha> that names no commit, exits 2.

Closed sets:
  phase   intent spec decompose execute gate
  event   create challenge approve revoke diverge verify revert demote block unblock
          pivot waive branch release change skip na enter bind pause done
          review escape order commit drop note
  status  ok failed blocked pending declined cancelled
  next    first word is a phase name or done

review evidence  kind=<plan|packet|quality|integration|spec> dispatched=<ts>
                 round=<n> blocked=<seconds>; status ok or failed; optional
                 pairs rev=<n> by=<independent|self> commit=<sha>
escape evidence  found-by=<gate|ci|copilot|person> severity=<critical|important>
                 review=<plan|packet|quality|integration>; status failed;
                 optional pair word=<the finder's own word>
approve          status declined records a no; its evidence is none
order            target the standing order's position <n> in ## Standing orders
                 of plan.md; status ok; evidence auto, stored as entry=<hash>;
                 refused in a folder with no plan.md
verify           target red-fix or red-<task> also takes the no-commit form
                 no-commit exit=<n> cmd=<command> out=<excerpt>, with a
                 non-zero <n> and at most 500 characters
commit           target fix, triage, user or housekeeping; status ok; evidence
                 auto:<sha>
drop             target the task id; status cancelled
note             status ok or failed; evidence by target:
                 slices      slices=S1,S2,... final=<yes|no>
                 slice-S<n>  issue=<#N or owner/repo#N>
                 lane-S<n>   merged=<sha>
                 coverage    none, or a pointer
                 policy      skill=<name> version=<version>

Environment: FLOW_WORK_ROOT (default docs/specs), FLOW_SESSION (default
CLAUDE_CODE_SESSION_ID, else -), FLOW_STORAGE (committed, the default, or local).
The Git config agentics.flowStorage wins over FLOW_STORAGE.
Exit codes:
  0  ok
  1  usage or an unexpected failure
  2  invalid cell, which includes a refused call form, an invalid storage setting,
     a --folder inside the local store, a refused bind, and an audit --base
     that is missing or names no commit
  3  no flow found
  4  more than one flow matched, or both stores hold the folder
  5  small fix in progress (where only)
`
