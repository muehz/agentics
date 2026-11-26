# Secrets detection patterns

Reference for audit Subagent #3. Detect committed credentials in the diff. Match patterns first. Confirm by entropy and
context. A credential on a tracked line is published with the repository, so whether its file ships to production sets
the rating and never whether it is a finding. Where this file and
[security-principles.md](../security-principles.md#false-positive-suppression) disagree, the principles win.

## Detection strategy

1. **Pattern match** against known prefixes (cheap, high precision)
2. **Entropy check** for unstructured high-entropy strings (see [High-entropy heuristics](#high-entropy-heuristics))
3. **Context check.** Check the variable name (`secret`, `api_key`, `password`, `token`), file path (config, env), and
   surrounding comments.
4. **Filter.** Exclude test fixtures, public sample values, placeholders (`changeme`, `xxxxx`), and values present in
   upstream documentation. A value in a live credential format, one of the provider patterns below at full length, is
   never excluded on its path alone (see [Path-based filters](#path-based-filters)).

A finding is high-confidence only when pattern, entropy, and context align.

## Provider patterns

The patterns below are conceptual matchers. Do not paste real credentials anywhere. Replace any captured value with
`[REDACTED]` in findings.

### AWS

| Type              | Prefix / shape                                                        |
| ----------------- | --------------------------------------------------------------------- |
| Access key ID     | `AKIA` (long-term IAM) or `ASIA` (STS, short-term), 20 chars          |
| Secret access key | 40 char base64-ish; rarely matched alone. Pair it with an access key. |
| Session token     | Very long base64 blob, usually accompanies `ASIA`                     |

INI-shaped credential blocks committed under config-style paths are an obvious finding.

### Azure

| Type                     | Shape                                                                            |
| ------------------------ | -------------------------------------------------------------------------------- |
| Storage account key      | 88-char base64 ending in `==`                                                    |
| Connection string        | `DefaultEndpointsProtocol=...;AccountName=...;AccountKey=...;EndpointSuffix=...` |
| Service principal secret | GUID-shaped or 40-char random; usually bound to a `clientId`                     |
| SAS token                | Query string with `sig=`, `se=`, `sp=`                                           |

### Google Cloud

| Type                 | Shape                                                                      |
| -------------------- | -------------------------------------------------------------------------- |
| API key              | `AIza` prefix + 35 chars                                                   |
| OAuth client secret  | `GOCSPX-` prefix                                                           |
| Service account JSON | A JSON object with `type: "service_account"` and a `private_key` PEM block |

### GitHub

| Type                   | Prefix        |
| ---------------------- | ------------- |
| Personal access token  | `ghp_`        |
| OAuth access token     | `gho_`        |
| User-to-server token   | `ghu_`        |
| Server-to-server token | `ghs_`        |
| Refresh token          | `ghr_`        |
| Fine-grained PAT       | `github_pat_` |

### Stripe

| Type        | Prefix                                                  |
| ----------- | ------------------------------------------------------- |
| Live secret | `sk_live_`                                              |
| Test secret | `sk_test_` (still a finding if in a non-test code path) |
| Publishable | `pk_live_` / `pk_test_` (public, informational only)    |
| Restricted  | `rk_live_` / `rk_test_`                                 |

### Slack

| Type             | Prefix                                           |
| ---------------- | ------------------------------------------------ |
| Bot / user token | `xoxb-`, `xoxa-`, `xoxp-`, `xoxr-`, `xoxs-`      |
| Webhook URL      | `https://hooks.slack.com/services/T.../B.../...` |

### Other common providers

| Provider  | Prefix or shape                            |
| --------- | ------------------------------------------ |
| Twilio    | `SK` followed by 32 hex                    |
| SendGrid  | `SG.` + 22 base64 + `.` + 43 base64        |
| Mailgun   | `key-` + 32 hex                            |
| OpenAI    | `sk-` + 48+ alphanumeric                   |
| Anthropic | `sk-ant-` + alphanumeric                   |
| npm       | `npm_` prefix + 36 chars                   |
| PyPI      | `pypi-AgEIcHlwaS5vcmc...` (base64-encoded) |
| Square    | `EAAAE...` (long alphanumeric)             |
| Heroku    | UUID (8-4-4-4-12). Needs context to flag   |

## Cryptographic material

Treat any of the following committed in non-test paths as a finding:

- `-----BEGIN RSA PRIVATE KEY-----` / `BEGIN PRIVATE KEY` / `BEGIN OPENSSH PRIVATE KEY` / `BEGIN EC PRIVATE KEY` /
  `BEGIN DSA PRIVATE KEY`
- `-----BEGIN ENCRYPTED PRIVATE KEY-----` (encrypted, but the passphrase is often nearby)
- PFX/PKCS12 binary files (`*.pfx`, `*.p12`)
- `.pem`, `.key`, `.jks` files outside test fixtures

## JWT-shaped strings

`eyJ` prefix on a base64url-shaped 3-segment string is a JWT. JWTs are not always sensitive (sample tokens are common in
docs). A JWT alongside variable names like `prodToken`, `accessToken`, or `Authorization` headers in committed config is
a finding.

## High-entropy heuristics

When no provider prefix matches, fall back to entropy:

- Compute Shannon entropy across the candidate string
- 4.0 bits/char or more over a string of 20+ chars is suspicious
- Combine with surrounding tokens. A variable name such as `password`, `secret`, `key`, or `token`, or an assignment in
  a config file, adds context.
- Filter common false positives: SRI hashes, content hashes, asset fingerprints, lockfile integrity hashes, base64 of
  public certificates, version strings

## Path-based filters

| Path pattern                                                | Treatment                                                                                                                                                                 |
| ----------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `tests/**`, `__tests__/**`, `__fixtures__/**`, `*.spec.*`   | Filter by default. Flag a value in a live credential format                                                                                                               |
| `examples/**`, `samples/**`, `docs/**`                      | Filter by default. Flag a value in a live credential format                                                                                                               |
| `node_modules/**`, `vendor/**`, `dist/**`, `build/**`       | Do not filter a value in a live credential format. A tracked built or vendored file is published with the repository. Apply the entropy and placeholder filters as usual. |
| Dotenv-style files committed at the repo root               | Always flag. These files should not be in source                                                                                                                          |
| Removed `.gitignore` entry that previously hid secret files | Flag. This may expose secret files.                                                                                                                                       |

## Verifying a finding

For each candidate, confirm:

1. **The value is on a line the diff added or modified** (or inside a renamed file)
2. **`.gitignore` does not dismiss it.** The ignore file keeps untracked files out of a commit. A value on a tracked
   line (a force-added file, for example) is published with the repository, so a path that `.gitignore` names clears
   nothing. Only a file the target does not track is outside the finding.
3. **The variable name, surrounding comments, or path** confirms the value is a credential, not noise
4. **The credential is not already public** (an obvious sample, marketing example, or revoked test value)

## Reporting format

Never include the full secret in the finding. The shared severity leads, as
[security-findings-format.md](../security-findings-format.md#one-finding) shows. Use this shape for the secrets-specific
lines:

```text
**Critical** (Critical) Hardcoded {provider} {credential type}
File: `config/production.yml` line 14
Pattern: `AKIA[REDACTED]` (16-char prefix masked)
Evidence: AWS access key ID committed in production config; the `aws_secret_access_key` line follows on L15.

Remediation:
1. Rotate the credential at the provider now.
2. Replace the value with a reference to a secret manager (for example AWS Secrets Manager or Azure Key Vault).
3. Audit provider logs for use of the leaked credential.
```

Always recommend rotation first and include the numbered `Remediation:` steps for each secret finding individually. A
follow-up commit that replaces the line does not invalidate a credential that has already been pushed. Do not prescribe
a history rewrite or a force-push, and do not run one: that decision belongs to the repository owner. When history
matters, the finding says that the value stays readable in earlier commits until the owner rewrites them, and that
rotation is what makes it harmless.

## Composition cooperation

If the repository runs a server-side scanner (gitleaks, trufflehog, GitHub secret scanning), assume it covers known
prefixes, and never drop a credential on a tracked line because of that assumption: this run did not see the scanner's
result. Focus on context:

- Which committed credentials are _active_ (referenced by code)
- Which are placeholders
- Which combine with adjacent values into a usable credential pair
