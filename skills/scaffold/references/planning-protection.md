# Optional consumer planning protection

Planning protection is init's fourth menu section ([init.md](init.md#phase-1-scan-and-select)); the readiness audit only
reports existing configuration and its limits. It has three parts the user selects independently: recording the parent
in `AGENTS.md`, the root ignore block, and native validation. Each selected part is shown before it is written, and the
selected parts share init's previews: the `AGENTS.md` record is an agent-instruction write and is shown alone, while the
ignore block and the validator files join the shared preview. An absent block or validator is not a compliance failure,
and a declined part leaves its files unchanged.

## Resolve the parent

The parent is the work root. [repo-conventions.md](../../routing/references/repo-conventions.md#work-root) resolves and
validates it, and [Resolve the plan parent](../../decompose/SKILL.md#resolve-the-plan-parent) is how planning uses it.
Scaffold does not implement a second resolver. The contract decides three cases:

| Order | What the contract finds                                       | What scaffold does                                                                                                                                                                                                       |
| ----- | ------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 1     | Instruction files that name different work roots              | A conflict: name every source with its path and ask once which is canonical, as the contract's [Conflicts](../../routing/references/repo-conventions.md#conflicts) section says. Protect nothing until the user answers. |
| 2     | A declared work root that fails the contract's validation     | Report the declaration and the reason. The contract then resolves `docs/specs`; offer protection for `docs/specs` and say so.                                                                                            |
| 3     | Otherwise: a valid declared work root, or none (`docs/specs`) | Use it.                                                                                                                                                                                                                  |

Existing work stays in its established work folder. A changed parent selects the home for new work only. Agree a parent,
not individual artifact paths.

Inspect the parent, with hidden content and tracked files, before you offer the ignore block. If it holds unrelated
documents, offer a dedicated subtree as the parent instead, or leave the block out. Do not impose the block on a shared
directory unasked. When the user asks for it there, name the cost once (unrelated documents become ignored); on a
repeated request, write it. Never move the directory's content. Report tracked extras by path and leave them tracked:
untracking a file is the user's decision. Recording the parent in `AGENTS.md`, when selected, preserves the file's
structure and content, updates an existing declaration rather than add a duplicate, and writes only the agreed line. A
declined record leaves `AGENTS.md` byte-identical.

## Preview the root ignore merge

The [canonical hierarchy template](planning-protection.gitignore) owns all ignore patterns. The preview helper, in two
forms ([planning-protection.sh](../scripts/planning-protection.sh) and
[planning-protection.ps1](../scripts/planning-protection.ps1)), takes the resolved parent and never resolves one itself.
It prints the full proposed root `.gitignore` to stdout and writes nothing. Show that complete preview before the user's
yes. Preserve unrelated rules, comments, encoding, and line endings, including CRLF. Apply the approved bytes with a
file-writing mechanism that preserves them. Never redirect stdout into the input file: the shell can truncate it before
the helper reads it.

Find the installed Agentics root from the active surface's plugin location or install inventory, and verify that the
helper and template exist there. Do not assume a Claude cache root on Codex, Copilot, or another surface. Run from the
consumer repository root. Replace the two placeholders with the verified install root and the resolved parent, keeping
the double quotes so a path with spaces stays one argument. On macOS and Linux, run the `sh` helper:

```text
sh "<agentics-root>/skills/scaffold/scripts/planning-protection.sh" --parent "<parent>" --gitignore .gitignore
sh "<agentics-root>/skills/scaffold/scripts/planning-protection.sh" --parent "<parent>"
```

On Windows, run the PowerShell helper:

```text
powershell -NoProfile -ExecutionPolicy Bypass -File "<agentics-root>\skills\scaffold\scripts\planning-protection.ps1" --parent "<parent>" --gitignore .gitignore
powershell -NoProfile -ExecutionPolicy Bypass -File "<agentics-root>\skills\scaffold\scripts\planning-protection.ps1" --parent "<parent>"
```

Read the exit code right after each run: `$?` in Bash, `$LASTEXITCODE` in PowerShell.

The first form previews a merge into an existing root `.gitignore`; the second, with no root ignore file, previews a new
one. Neither writes.

One clearly marked block is added or replaced. A rerun must not duplicate it. Malformed or duplicate marker blocks,
unreadable input, and rejected parent values stop generation rather than authorize a hand merge that guesses intent. On
a parent change, do these steps:

1. Preview the full replacement.
2. Explain both parents' resulting discovery behavior.

Existing work is not moved, deleted, or untracked.

The template allows these paths:

- Parent-level `history.md`
- Valid monthly directories
- Canonical work-item files, including `state.tsv`
- Only `state.tsv` in the reserved `<work-root>/unassigned/` directory, directly under the parent

There is no month-level `unassigned/` directory. The template rejects extras and deeper nesting, such as directories
disguised as canonical files. Ignores constrain discovery. They cannot untrack existing files or stop `git add -f`.
Exact local work-item naming and tracked/force-added enforcement require separately agreed native validation.

## Verify the applied rules

After an authorized write, run actual Git checks in the consumer, not just string comparisons. Use hypothetical
untracked probe paths under the actual agreed parent with `--no-index`, so tracked status cannot mask the rule. Check
every allowed canonical file, parent history, and the reserved unassigned log. Also check rejected extras at each depth,
invalid months, and directory paths disguised as files. No probe file creation is necessary. For example:

```text
git check-ignore --no-index -q -- "<parent>/2026-10/feature-example/plan.md"
git check-ignore --no-index -q -- "<parent>/unassigned/state.tsv"
git check-ignore --no-index -q -- "<parent>/2026-10/feature-example/extra.md"
git check-ignore --no-index -q -- "<parent>/2026-13/feature-example/plan.md"
```

The first two are allowed paths and must exit 1; the last two are rejected paths and must exit 0. Substitute the agreed
local work-item name. Read the exit code right after each command: `$?` in Bash, `$LASTEXITCODE` in PowerShell.

Inspect exit status and errors. An error is not a passing rejection check. When you diagnose a failure, use a separate
`git check-ignore --no-index -v -- "$probe_path"` invocation with the same probe path passed as one argument to identify
the effective rule. Verbose negation output is not proof that a path is ignored. An unrelated ignored ancestor can stop
Git before it descends into the parent, so allowed-file negations cannot rescue those paths. If positive probes remain
ignored, do these steps:

1. Report the effective ancestor rule.
2. Block any claim that protection works.

Never silently unignore an unrelated ancestor. A proposed adjustment is a further edit to `.gitignore`, shown before it
is written like the block itself. Report observed results and tracked extras separately. A matching generated preview
does not prove effective consumer rules or Agentics live alignment.

## Optional repository-native validation

Offer native validation independently of the ignore block. Before you author it, confirm these items for the consumer:

- Actual language
- Available tooling
- Local work-item naming
- Monthly layout
- Reserved fallback
- Existing validation entry points

Agree which tracked and force-added paths it must reject. Agree how it will run. Follow the canonical layout contract.
Do not ship a generic validator or install Agentics conformance in a consumer repository.

Show every proposed validator file and its exact verification command in init's shared preview, as their own entries.
Write them only on the user's yes. Then run the agreed local command. Report the actual result. A CI change needs the
user to ask for it and its own preview; a yes to the validator does not cover CI. A declined validator leaves existing
files unchanged and preserves the ignore option's independent decision. With no validator, describe tracked-file and
force-add enforcement as absent, not passing. Only an observed run in the consumer repository shows that the protection
works; template fixtures and this workflow's authoring do not.
