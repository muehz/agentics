# Secret pattern maintenance

Agentics uses one shared detector for the preventive write checks in `secrets-guardian`. The adapted provider rules
reference Gitleaks v8.28.0 at commit `4fb43823ef3d152d239e92d7d5cb04783b548062`.

The source is
[Gitleaks's pinned configuration](https://github.com/gitleaks/gitleaks/blob/4fb43823ef3d152d239e92d7d5cb04783b548062/config/gitleaks.toml).
Attribution and the retained license are recorded in THIRD_PARTY_NOTICES.md and
hooks/go/internal/secretpatterns/LICENSE.gitleaks.

The adapted rule IDs are aws-access-token, openai-api-key, anthropic-api-key, anthropic-admin-api-key, gitlab-pat,
gitlab-pat-routable, hashicorp-tf-api-token, and age-secret-key. Existing Agentics rules remain where their behavior is
intentionally retained. This is not full Gitleaks compatibility.

To update a rule, review the upstream diff at an immutable commit, preserve its capture and rule-local filtering
semantics, and document deliberate differences. Add a synthetic positive and negative fixture before changing the rule.
Do not copy the upstream repository-wide exclusions into Agentics silently. Never use a live credential as a fixture.

After reviewing the immutable upstream diff, adapt only the approved rule rows and add synthetic positive and negative
coverage. From the repository root, run the shared detector, its caller, and the provider pipeline:

```bash
go test -C hooks/go ./internal/secretpatterns ./internal/secretsguard ./internal/provider ./cmd/hook -count=1
```

Rebuild and verify the six runtime outputs with the pinned toolchain:

```bash
make -C hooks/go build-all
make -C hooks/go verify
go test -C hooks/go ./cmd/hook -run '^$' -bench BenchmarkPackagedHook -benchtime=3x -count=3
```

Commit the updated `hooks/SHA256SUMS`. Generated executables stay in the local checkout.

Use the Go version declared by `hooks/go/go.mod` and a writable Go cache. Compare matching benchmark-case medians,
investigating any increase above twice the retained baseline. The benchmark measures the local native build. Require all
six verification results to be `IDENTICAL`.

Review the hook inventory when a provider adds an execution or read tool, changes an MCP schema, changes its adapter
normalization, or introduces a deployment CLI usage shape. Add or update the exact roster and policy coverage only after
a synthetic test proves the behavior.

For a separate repository/history scan, first install and verify Gitleaks v8.28.0 through your normal tool-management
process. Check `gitleaks version`, then run `gitleaks git --redact --no-banner .` from the repository root. This command
is separate from the hooks; it does not enable GitHub settings or verify credentials against providers. Exit 1 indicates
findings, not a clean scan. Keep generated reports local unless their destination is explicitly approved.

GitHub's supported-pattern catalog is a coverage reference. TruffleHog provides another detector and verification
implementation. Neither replaces path-based protection before reads, and no after-the-fact scan can retract an already
disclosed value.
