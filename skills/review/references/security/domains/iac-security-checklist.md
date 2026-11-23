# Infrastructure-as-code security checklist

Reference for audit Subagent #5. Use it to catch misconfigurations. It covers container images, Terraform, Bicep and
ARM, Kubernetes manifests, GitHub Actions, and CI configs.

## Docker / container images

| Check                                                                          | Why it matters                                                                |
| ------------------------------------------------------------------------------ | ----------------------------------------------------------------------------- |
| `USER` directive sets a non-root user                                          | Containers default to root; root in container = root on host if escape exists |
| Base image pinned by digest (`@sha256:...`) or specific version, not `:latest` | Reproducibility; protects against tag re-points                               |
| Multi-stage build leaves only runtime artifacts in the final image             | Reduces attack surface; compilers and toolchain do not ship                   |
| No secrets baked into layers (build args, COPY of credential files)            | Layers are visible in registry; secrets live forever                          |
| `HEALTHCHECK` defined                                                          | Misconfigured containers fail loudly                                          |
| Minimal base (distroless, alpine, slim) when feasible                          | Fewer packages, fewer CVEs                                                    |
| `apt`/`apk` clean-up in the same RUN layer                                     | Prevents stale package indices in image                                       |
| No remote scripts piped into a shell at build time                             | Unsigned, untrusted execution at build time                                   |

```dockerfile
# Vulnerable patterns
FROM node:latest
COPY config/secrets.yaml /app/secrets.yaml
RUN apt-get update && apt-get install -y curl
USER root

# Safer
FROM node:20.11.0-alpine@sha256:abc123
WORKDIR /app
COPY --chown=node:node package*.json ./
RUN npm ci --omit=dev
COPY --chown=node:node . .
USER node
HEALTHCHECK --interval=30s CMD node healthcheck.js
CMD ["node", "server.js"]
```

## Terraform

| Resource                             | Common mistakes                                                                                    |
| ------------------------------------ | -------------------------------------------------------------------------------------------------- |
| `aws_s3_bucket`                      | Public ACLs, missing `aws_s3_bucket_public_access_block`, no encryption, no versioning, no logging |
| `aws_security_group`                 | `0.0.0.0/0` ingress on 22, 3306, 3389, 5432, 6379                                                  |
| `aws_iam_policy` / `policy_document` | `Action: "*"` on `Resource: "*"`; wildcard cross-account trust                                     |
| `aws_db_instance`                    | `publicly_accessible = true`, missing `storage_encrypted`, no automated backups                    |
| `aws_kms_key`                        | No rotation, missing key policy with explicit principals                                           |
| `aws_lambda_function`                | Permissive resource-based policy, env vars holding secrets in plaintext                            |
| `azurerm_storage_account`            | `public_network_access_enabled = true`, `allow_nested_items_to_be_public = true`                   |
| `azurerm_key_vault`                  | `public_network_access_enabled = true`, no purge protection, no soft delete                        |
| `google_storage_bucket`              | `uniform_bucket_level_access = false`, no `public_access_prevention`                               |

| Cross-cutting check                                                       |
| ------------------------------------------------------------------------- |
| Encryption at rest enabled (storage, DB, queue)                           |
| Encryption in transit enforced (TLS, no plaintext SQL or Redis listeners) |
| Logging enabled (S3 access logs, CloudTrail, Activity Log, Audit Logs)    |
| Network exposure minimized (private endpoints, VPC-only, no public IPs)   |
| IAM follows least privilege; wildcards justified per resource             |
| Tags include `owner`, `environment`, `data-classification`                |

## Bicep / ARM

Bicep and ARM have the same risk classes as Terraform. Common Bicep-specific checks:

| Pattern                                                                        | Risk                                                     |
| ------------------------------------------------------------------------------ | -------------------------------------------------------- |
| `properties.publicNetworkAccess: 'Enabled'`                                    | Public surface where private endpoint is appropriate     |
| Storage `allowBlobPublicAccess: true`                                          | Anonymous blob access                                    |
| `properties.minimumTlsVersion: 'TLS1_0'` or `'TLS1_1'`                         | Deprecated TLS                                           |
| Role assignment via `Microsoft.Authorization/roleAssignments` with broad scope | Over-privilege at subscription or management group level |
| Key Vault with `properties.enableSoftDelete: false`                            | Destructive deletion possible                            |
| `outputs:` exposing secret values                                              | Secret leaks into deployment history                     |

## Kubernetes

| Manifest field                                                                                                        | Concern                                               |
| --------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| `securityContext.privileged: true`                                                                                    | Container has full host access                        |
| `hostNetwork: true`, `hostPID: true`, `hostIPC: true`                                                                 | Breaks pod isolation                                  |
| `securityContext.runAsUser: 0`, or neither a nonzero `runAsUser` nor `runAsNonRoot: true` in effect for the container | Runs as root inside the pod                           |
| `securityContext.allowPrivilegeEscalation: true`                                                                      | setuid binaries can elevate                           |
| `securityContext.capabilities.add` includes `SYS_ADMIN`, `NET_ADMIN`, `ALL`                                           | Effectively privileged                                |
| `automountServiceAccountToken: true` on pods that do not call the API                                                 | Unnecessary credential exposure                       |
| Image referenced by a mutable tag (`:latest`, `:v1`) instead of a digest                                              | Image substitution risk                               |
| Missing `resources.limits` (CPU, memory)                                                                              | DoS. A single pod can starve the node.                |
| Missing `NetworkPolicy`                                                                                               | Default-allow east-west traffic                       |
| Secrets mounted as env vars                                                                                           | Leaked via crash dumps, child processes; prefer files |
| `RoleBinding` / `ClusterRoleBinding` to `cluster-admin`                                                               | Almost always over-privileged                         |

```yaml
# Hardened pod baseline
spec:
    automountServiceAccountToken: false
    securityContext:
        runAsNonRoot: true
        runAsUser: 10001
        seccompProfile:
            type: RuntimeDefault
    containers:
        - name: app
          image: registry.example.com/app@sha256:abc123
          imagePullPolicy: IfNotPresent
          securityContext:
              allowPrivilegeEscalation: false
              readOnlyRootFilesystem: true
              capabilities:
                  drop: ["ALL"]
          resources:
              limits:
                  cpu: "500m"
                  memory: "512Mi"
              requests:
                  cpu: "100m"
                  memory: "128Mi"
```

## GitHub Actions

| Pattern                                                                             | Risk                                                             |
| ----------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| `${{ github.event.pull_request.title }}` interpolated into `run:` script            | Template injection. The attacker controls the PR title (CWE-94). |
| Same for `head_ref`, `body`, comment bodies, issue titles                           | Template injection                                               |
| `permissions:` not declared                                                         | Default permissive token                                         |
| `permissions: write-all` or `contents: write` when not needed                       | Over-privileged token                                            |
| Third-party actions referenced by mutable tag (`@v3`)                               | Tag re-point lets the action publisher change behavior           |
| `pull_request_target` workflow that checks out PR head and runs untrusted code      | RCE on a workflow with secrets access                            |
| Secrets passed via `env:` into untrusted code paths                                 | Secret exfiltration                                              |
| `actions/checkout` with `persist-credentials: true` in jobs that run untrusted code | Credentials accessible to attacker code                          |

```yaml
# Vulnerable: PR title goes straight into a shell command
- run: echo "Title is ${{ github.event.pull_request.title }}"

# Safer: pass it through an environment variable and quote the expansion
- run: echo "Title is $TITLE"
  env:
      TITLE: ${{ github.event.pull_request.title }}
```

Pin third-party actions by SHA:

```yaml
- uses: actions/checkout@b4ffde65f46336ab88eb53be808477a3936bae11 # v4.1.1
```

Set `permissions:` at the workflow or job level explicitly:

```yaml
permissions:
    contents: read
    pull-requests: write # only if posting comments
```

## CI / other

| Pattern                                                  | Risk                                      |
| -------------------------------------------------------- | ----------------------------------------- |
| Remote install script piped into a shell during CI       | Untrusted execution                       |
| Secrets echoed (e.g., `set -x`, debug logs)              | Tokens in CI logs                         |
| Long-lived deploy credentials instead of OIDC federation | Standing credentials are a target         |
| Branch protection bypass for bots                        | Unsigned changes reach protected branches |
| Self-hosted runners without ephemeral isolation          | Cross-job contamination                   |

## What to flag

- Public network exposure on data stores → High or Critical
- IAM wildcards on managed resources → High
- Privileged containers, host namespaces, root containers → High
- GitHub Actions template injection on `pull_request` trigger → Critical
- Mutable image or action references → Medium
- Missing encryption-at-rest or TLS minimums → Medium
- Missing logs or audit trail → Low to Medium, based on data sensitivity

## Composition

This subagent returns findings in the same JSON envelope as code findings. Always cite the specific resource type and
the property at fault, for example `aws_s3_bucket.public_access_block missing`. With these citations, skills that call
this subagent can group misconfigurations alongside code-level findings.
