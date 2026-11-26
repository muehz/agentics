# Staged secret scans

Use these examples only for a missing protection selected through [init](../init.md). Preserve a suitable existing
scanner. Preview the consumer's actual hook path, formatter command, restaging behavior, scanner version, policy path
and installation-document path before you write. Reuse the consumer's hook manager. Do not install a second manager or
replace existing checks. The examples assume a POSIX shell (Git Bash on Windows) and npm only where the existing
formatter uses npm. Verify those prerequisites on the consumer's supported hosts.

The selected starter uses gitleaks 8.30.1 and scans added lines in the staged Git diff. That includes an unborn commit.
It does not scan every indexed file body or unstaged or untracked content. Install the verified platform binary on PATH
from the [official release](https://github.com/gitleaks/gitleaks/releases/tag/v8.30.1). Document the checksum and the
installation command chosen for that host. Retain an existing suitable policy. If the consumer selects a new policy,
this complete `.gitleaks.toml` extends the built-in rules:

```toml
[extend]
useDefault = true
```

Review inert fixture exceptions individually. Real findings stop adoption. Do not silently baseline them or exclude
whole fixture directories. The version check and nonzero exits below also stop a missing, unsupported or failed scan.
Adapt both the command's policy path and diagnostic's documentation path together in the preview.

## Husky

Append the scanner after the existing formatter and check flow in the consumer's existing Husky pre-commit file. This
complete npm example retains lint-staged's own staging behavior. Preserve any other existing checks in their current
order. Propagate the failures of those checks. Do not replace the manager's generated launcher or change shared Git
config.

```sh
npx --no-install lint-staged || exit $?

if ! command -v gitleaks >/dev/null 2>&1; then
	echo 'Secret scan stopped: install gitleaks 8.30.1 on PATH. See CONTRIBUTING.md, Secret scanning.' >&2
	exit 1
fi

scanner_version=$(gitleaks version) || exit $?
case "$scanner_version" in
	8.30.1|v8.30.1) ;;
	*)
		echo 'Secret scan stopped: install supported gitleaks 8.30.1 on PATH. See CONTRIBUTING.md, Secret scanning.' >&2
		exit 1
		;;
esac

gitleaks git --pre-commit --staged --redact --config .gitleaks.toml
```

## Staged scanner script

For the following managers, preview this complete scan-only script at the consumer-selected path
`scripts/scan-staged-secrets.sh`. The formatter stays in the manager configuration and runs once. The scanner reads the
index, redacts findings, returns its actual exit status and performs no restaging.

```sh
if ! command -v gitleaks >/dev/null 2>&1; then
	echo 'Secret scan stopped: install gitleaks 8.30.1 on PATH. See CONTRIBUTING.md, Secret scanning.' >&2
	exit 1
fi

scanner_version=$(gitleaks version) || exit $?
case "$scanner_version" in
	8.30.1|v8.30.1) ;;
	*)
		echo 'Secret scan stopped: install supported gitleaks 8.30.1 on PATH. See CONTRIBUTING.md, Secret scanning.' >&2
		exit 1
		;;
esac

gitleaks git --pre-commit --staged --redact --config .gitleaks.toml
```

## Lefthook

Merge these entries into the existing `pre-commit` configuration. This complete npm example runs the formatter before
the scanner and stops on failure. Keep the consumer's actual formatter command and its existing `stage_fixed` behavior,
because lint-staged already handles its own index updates. Never set `stage_fixed` on the scanner. Preserve other
commands. Review their priorities so the scan runs after all intended formatting and restaging.

```yaml
pre-commit:
    piped: true
    commands:
        format:
            priority: 1
            run: npx --no-install lint-staged
        secrets:
            priority: 2
            run: sh scripts/scan-staged-secrets.sh
```

The [priority documentation](https://lefthook.dev/configuration/priority/) specifies sequential ordering with
`piped: true`. The [piped](https://github.com/evilmartians/lefthook/blob/master/docs/configuration/piped.md) option
stops after a failure and conflicts with `parallel: true`. Preview any change to an existing parallel execution policy.
The [stage_fixed documentation](https://lefthook.dev/configuration/stage_fixed/) explains index updates. Before consumer
execution, do these steps:

1. Verify that the installed manager supports these options.
2. Prove native ordering and failure propagation in a disposable repository.

A parsed example alone is not enforcement evidence.

## pre-commit (local)

For a consumer that already uses pre-commit, append the local hook after its existing formatters and checks. This
complete configuration uses the scan-only script and a system gitleaks 8.30.1 binary on PATH. Only add global
`fail_fast: true` when an existing formatter must stop subsequent hooks (and preview its effect); do **not** add global
`fail_fast: true` when preserving existing non-formatter checks so global hook behavior stays unchanged. Preserve
existing entries and formatter modification and retry behavior. Do not add this manager to a Husky repository.

```yaml
repos:
    - repo: local
      hooks:
          - id: secret-scan
            name: Scan staged secrets
            entry: sh scripts/scan-staged-secrets.sh
            language: system
            pass_filenames: false
            always_run: true
```

`pass_filenames: false` ensures that the scanner does not misread filename arguments as options. `always_run: true`
avoids filename-filter skips. The scanner selects the staged diff itself.
[pre-commit](https://pre-commit.com/#creating-new-hooks) documents these behaviors and `language: system`.

A consumer may instead select the pinned upstream hook, after its existing formatter and check entries:

```yaml
repos:
    - repo: https://github.com/gitleaks/gitleaks
      rev: v8.30.1
      hooks:
          - id: gitleaks
            args: [--config, .gitleaks.toml]
            always_run: true
```

The [v8.30.1 upstream manifest](https://github.com/gitleaks/gitleaks/blob/v8.30.1/.pre-commit-hooks.yaml) selects a
staged Git scan with redaction and passes no filenames. Its `golang` environment builds and installs the scanner. The
local `system` example requires a separately installed binary. Verify the resulting version, policy and native hook
execution before adoption. Do not substitute the upstream system variant unless you have checked its argument behavior.

## Existing Checkmarx consumers

When the repository already uses Checkmarx, do these steps:

- Offer Checkmarx 2ms.
- Retain a suitable existing scan alongside it.

As of 2026-10-14, the [official 2ms documentation](https://github.com/Checkmarx/2ms#scan-targets) documents filesystem
scanning with `--path` and Git-history scanning. It describes no native staged switch. Do not invent one or pass staged
filenames to a command that expects a directory.

If selected, a consumer-owned, separately tested adapter can materialize a snapshot of the index and scan that directory
with `2ms filesystem --path`. That scans full indexed file contents, a different scope from staged added lines. Require
proof for each of these:

- Partial staging.
- Spaces, renames, and deletions.
- Symlinks.
- Snapshot containment and cleanup.
- Redacted reporting.
- Nonzero exit propagation for findings and setup errors.

Verify the selected 2ms version and its reporting capabilities before you emit a runnable hook. Keep failure exits
enabled. Do not use `--ignore-on-exit` to suppress findings or errors. Do not enable live credential validation for a
local fixture test. This reference supplies guidance, not an index adapter or an unverified claim of staged parity.

## Verification boundary

Test the selected copied configuration through actual Git commits in a disposable repository. Cover these cases:

- A clean commit.
- A synthetic staged finding with redacted output.
- A formatter failure.
- A missing or unsupported scanner.
- Partial staging.
- Paths with spaces.

Confirm these results:

- The formatter runs before the scanner.
- Failures preserve HEAD.
- Setup errors are distinguishable from real findings.

Never use a real credential or bypass a user's hooks as a probe. An unavailable native manager is blocked or unverified,
not a passing protection. Local proof establishes neither CI delivery nor provider evaluation.

## CI workflow

Use this workflow only when the selected scaffold preview identifies missing CI secret scanning. Inspect the consumer's
actual workflow. Preserve any suitable existing scanner and checks. If the consumer's established pattern is to merge
scans into the existing workflow, merge the scan into it. During preview, do these steps:

- Replace the example's `main` push branch with the consumer's discovered default branch.
- Cover every selected event.

Confirm the consumer's policy path, verified scanner release and archive hash, and job budget before you emit the
preview.

This complete GitHub Actions example uses the same verified Gitleaks release and policy as the local examples. Its
five-minute job budget includes checkout and installation. The scan itself has a 240-second timeout. At use time, do
these steps:

- Verify the pinned `actions/checkout` revision and SHA against its
  [official releases](https://github.com/actions/checkout/releases).
- Verify the Gitleaks release and Linux archive checksum against its
  [official release](https://github.com/gitleaks/gitleaks/releases/tag/v8.30.1).

The explicit event SHA pairs and shallow history checks fail closed when the range is invalid or unavailable. Findings
and incomplete scans return nonzero with redacted diagnostics. `--diff-merges=first-parent` controls merge diff
comparison. It does not restrict Git traversal to first-parent history.

CI materializes `.gitleaks.toml` from the validated event base into a temporary directory, so a proposed allowlist
cannot hide findings in its own change. It rejects checkout `.gitleaksignore` files and ignores inline `gitleaks:allow`
comments. Individually reviewed exceptions belong in the trusted policy. Gitleaks 8.30.1 loads the checkout ignore file
even with an explicit ignore path, so an empty override alone is insufficient. The presence check stops before the scan
and leaves the checkout unchanged. Explicit `--config` takes precedence over Gitleaks's configuration environment
variables. Preview the entire policy dependency closure. Verify it too. This standalone example supports built-in
extensions, not `extend.path` files from the untrusted checkout. Adapt such dependencies from the same trusted source
before you offer this workflow for a consumer that needs them.

Disclose the checkout-ignore restriction in the init preview. Do not delete an existing consumer ignore file. Do one of
these instead:

- Offer a normal reviewed policy change that migrates each accepted exception into the trusted base policy.
- Retain a suitable existing scanner with its own verified trust model.

The pinned [Gitleaks detector setup](https://github.com/gitleaks/gitleaks/blob/v8.30.1/cmd/root.go#L279-L299) documents
that Gitleaks loads ignore files additively, which requires this refusal.

A missing base policy stops adoption. That includes a first push whose before SHA is all zero. The pull request that
adds this workflow and `.gitleaks.toml` has no policy at its base, so this job fails on that pull request. It fails
again on the push that merges it, because that push's before SHA is a commit that also lacks the policy. Name both
failures in the init preview. Offer policy adoption separately. Do these steps in order:

1. Preview an administrator-selected immutable policy commit and exact SHA256, gated to an explicit initial base.
2. Prove absent-ref, digest-mismatch, and other-base failures.

Never do any of these:

- Infer that bootstrap from the PR head.
- Silently fall back to built-in defaults.
- Baseline real findings.

This consumer template contains no Agentics-specific bootstrap. Remove a temporary bridge after the policy is present in
the trusted base.

A PR can still edit the workflow definition itself. Audit protection and required review for the workflow, policy and
any policy dependencies. Prepare the missing remote protection proposal separately. Trusted configuration selection
alone does not establish an administrator-controlled check or required merge protection. Local staged hooks use the
consumer's local policy and have a different trust boundary. Before you claim CI enforcement, test these cases:

- Clean and secret-bearing event ranges.
- A head-only policy exception.
- Automatic ignore and inline suppression.
- Policy removal.
- Missing trusted policy.

```yaml
name: Secret scan
on:
    pull_request:
    push:
        branches: [main]
    merge_group:
permissions:
    contents: read
jobs:
    gitleaks:
        name: gitleaks
        runs-on: ubuntu-latest
        timeout-minutes: 5
        steps:
            - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
              with:
                  fetch-depth: 0
                  persist-credentials: false
            - name: Install verified gitleaks
              shell: bash
              run: |
                  set -euo pipefail
                  scanner_dir="$RUNNER_TEMP/gitleaks"
                  mkdir -p "$scanner_dir"
                  curl --fail --silent --show-error --location \
                    https://github.com/gitleaks/gitleaks/releases/download/v8.30.1/gitleaks_8.30.1_linux_x64.tar.gz \
                    --output "$scanner_dir/archive.tar.gz"
                  printf '%s  %s\n' \
                    551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb \
                    "$scanner_dir/archive.tar.gz" | sha256sum --check --strict
                  tar -xzf "$scanner_dir/archive.tar.gz" -C "$scanner_dir" gitleaks
                  test "$("$scanner_dir/gitleaks" version)" = 8.30.1
                  printf '%s\n' "$scanner_dir" >> "$GITHUB_PATH"
            - name: Scan introduced commits
              shell: bash
              env:
                  SCAN_EVENT: ${{ github.event_name }}
                  PR_BASE_SHA: ${{ github.event.pull_request.base.sha }}
                  PR_HEAD_SHA: ${{ github.event.pull_request.head.sha }}
                  PUSH_BEFORE_SHA: ${{ github.event.before }}
                  PUSH_AFTER_SHA: ${{ github.event.after }}
                  GROUP_BASE_SHA: ${{ github.event.merge_group.base_sha }}
                  GROUP_HEAD_SHA: ${{ github.event.merge_group.head_sha }}
              run: |
                  set -euo pipefail
                  case "$SCAN_EVENT" in
                    pull_request) before="$PR_BASE_SHA"; after="$PR_HEAD_SHA" ;;
                    push) before="$PUSH_BEFORE_SHA"; after="$PUSH_AFTER_SHA" ;;
                    merge_group) before="$GROUP_BASE_SHA"; after="$GROUP_HEAD_SHA" ;;
                    *) echo 'Secret scan incomplete: unsupported event.' >&2; exit 1 ;;
                  esac
                  zero=0000000000000000000000000000000000000000
                  if [[ ! "$before" =~ ^[0-9a-f]{40}$ || ! "$after" =~ ^[0-9a-f]{40}$ || "$after" == "$zero" ]]; then
                    echo 'Secret scan incomplete: invalid commit range.' >&2
                    exit 1
                  fi
                  if [[ "$(git rev-parse --is-shallow-repository)" != false ]]; then
                    echo 'Secret scan incomplete: shallow history.' >&2
                    exit 1
                  fi
                  git cat-file -e "$after^{commit}" || {
                    echo 'Secret scan incomplete: head commit unavailable.' >&2; exit 1;
                  }
                  if [[ "$before" == "$zero" ]]; then
                    if [[ "$SCAN_EVENT" != push ]]; then
                      echo 'Secret scan incomplete: zero base for non-push event.' >&2; exit 1
                    fi
                    range="$after"
                  else
                    git cat-file -e "$before^{commit}" || {
                      echo 'Secret scan incomplete: base commit unavailable.' >&2; exit 1;
                    }
                    range="$before..$after"
                  fi
                  if [[ -e .gitleaksignore || -L .gitleaksignore ]]; then
                    echo 'Secret scan incomplete: checkout .gitleaksignore is unsupported; migrate individually reviewed exceptions to the trusted policy.' >&2
                    exit 1
                  fi
                  policy_dir=$(mktemp -d "$RUNNER_TEMP/gitleaks-policy.XXXXXX")
                  trap 'rm -rf -- "$policy_dir"' EXIT
                  policy="$policy_dir/.gitleaks.toml"
                  if [[ "$before" == "$zero" ]] || ! git cat-file -e "$before:.gitleaks.toml" 2>/dev/null; then
                    echo 'Secret scan incomplete: trusted policy unavailable at event base; approve an immutable bootstrap policy before adoption.' >&2
                    exit 1
                  fi
                  git show "$before:.gitleaks.toml" > "$policy"
                  : > "$policy_dir/.gitleaksignore"
                  if gitleaks git --redact --config "$policy" --gitleaks-ignore-path "$policy_dir/.gitleaksignore" --ignore-gitleaks-allow --log-opts="$range --diff-merges=first-parent" --timeout=240; then
                    exit 0
                  else
                    status=$?
                    echo 'Secret scan blocked: findings or incomplete scan; inspect the redacted diagnostic.' >&2
                    exit "$status"
                  fi
```

This event-range scan checks the commits that the selected pull request, default-branch push, or merge queue group
introduces. It includes removed findings in those commits, side-branch commits, and merge-resolution changes. A separate
full-history audit can find pre-existing or removed secrets and material on branches outside the event range. Treat real
findings as an adoption blocker. Review inert fixture exceptions individually. Do not hide findings with a broad or
automatic baseline. This template is guidance for a preview, not an active repository workflow. The addition of a remote
required check is a separate administrator decision and needs actual delivery evidence.
