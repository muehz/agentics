# OWASP Top 10 checklist (2025)

> Targets OWASP Top 10:2025 (the released edition at https://owasp.org/Top10/2025/). Migrated from the 2021 edition. The
> category IDs, names, scope, and CWE mappings below were verified against the published per-category pages at
> https://owasp.org/Top10/2025/A0{1-9}_2025-*/ and .../A10_2025-Mishandling_of_Exceptional_Conditions/.

Reference for audit Subagent #1. Pair with [injection-patterns.md](injection-patterns.md) for deeper injection coverage.

## A01:2025 — Broken Access Control

Unchanged rank (#1). The 2025 edition merges Server-Side Request Forgery (SSRF), CWE-918, into this category. The SSRF
rows and code sample below moved from the retired 2021 A10 slot.

| Look for                                                                                        | Why it matters                                       |
| ----------------------------------------------------------------------------------------------- | ---------------------------------------------------- |
| Route handlers added without an authorization check                                             | Anyone can hit them                                  |
| `req.params.id` used to fetch a record without ownership check                                  | Insecure Direct Object Reference (IDOR), CWE-639     |
| Role check on the front-end only                                                                | Attacker bypasses by hitting the API directly        |
| Wildcard CORS with credentials                                                                  | Cross-origin reads of authenticated responses        |
| Force browse: sequential IDs, predictable URLs                                                  | Mass enumeration                                     |
| Outbound HTTP using a URL or hostname derived from user input                                   | SSRF, CWE-918                                        |
| URL parsing that does not block `localhost`, `127.0.0.0/8`, `169.254.169.254`, `[::1]`, RFC1918 | Cloud metadata or internal-network exposure via SSRF |
| Server-side image fetch, webhook, or proxy features                                             | Common SSRF entry points                             |
| Imported URL libraries that follow redirects to internal hosts                                  | Redirect-based SSRF bypass                           |

```javascript
// Vulnerable
app.get("/api/orders/:id", async (req, res) => {
    const order = await db.orders.find(req.params.id);
    res.json(order);
});

// Safe
app.get("/api/orders/:id", requireAuth, async (req, res) => {
    const order = await db.orders.findOne({
        id: req.params.id,
        userId: req.user.id,
    });
    if (!order) return res.sendStatus(404);
    res.json(order);
});
```

```python
# Vulnerable
import requests
def fetch(url):
    return requests.get(url).text

# Safer (allowlist + scheme + IP class checks)
from urllib.parse import urlparse
ALLOWED_HOSTS = {"images.example.com", "cdn.example.com"}
def fetch(url):
    p = urlparse(url)
    if p.scheme not in {"https"} or p.hostname not in ALLOWED_HOSTS:
        raise ValueError("disallowed url")
    return requests.get(url, allow_redirects=False, timeout=5).text
```

## A02:2025 — Security Misconfiguration

Moved up from #5 to #2. Same scope as the 2021 category (hardening across the application stack).

| Look for                                                          |
| ----------------------------------------------------------------- |
| Debug mode enabled in production config                           |
| Default credentials, default admin accounts                       |
| Missing security headers (HSTS, CSP, X-Content-Type-Options)      |
| Open management interfaces (`/actuator/*`, `/admin`) without auth |
| `secret_key_base = "changeme"` style placeholders shipped         |

OWASP now tracks verbose stack traces and framework banners returned to clients under
[A10:2025, Mishandling of Exceptional Conditions](#a102025--mishandling-of-exceptional-conditions). The 2025 mapping
puts CWE-209 and CWE-756 there, not here.

## A03:2025 — Software Supply Chain Failures

New name and broadened scope for A06:2021 "Vulnerable and Outdated Components." It covers the full supply chain:
dependency and component tracking, CI/CD and build-pipeline integrity, change management (SBOM, IaC), and artifact
provenance. [dependency-review-guide.md](dependency-review-guide.md) covers it. At triage time, look for:

- `package.json` / `requirements.txt` / `*.csproj` changes that pin a version with a known CVE
- Removal of a `pin` or upgrade-blocker without an explanation
- New transitive dep introduced via a low-popularity package
- `npm install` from an arbitrary registry, `pip install` from a URL in CI
- GitHub Actions referenced by mutable tag (`@v3`) instead of full SHA. Whoever controls the upstream repo can repoint
  the tag.
- Container images pulled by `:latest` rather than digest

## A04:2025 — Cryptographic Failures

Moved from #2 to #4. Same core scope as the 2021 category.

| Look for                                                 | Why it matters                         |
| -------------------------------------------------------- | -------------------------------------- |
| Plaintext passwords or tokens at rest                    | Database compromise = total compromise |
| MD5 or SHA-1 for password hashing                        | Trivially crackable                    |
| `Math.random()` or `random.random()` for security tokens | Predictable                            |
| Static IVs, ECB mode, hardcoded keys                     | Pattern leakage, key compromise        |
| TLS 1.0 / 1.1 / `verify=False` in clients                | MITM exposure                          |

Use platform crypto APIs (`crypto.randomBytes`, `secrets.token_*`, `RandomNumberGenerator.GetBytes`) and
password-specific KDFs (argon2id, scrypt, or correctly-tuned bcrypt).

OWASP now tracks sensitive data placed in URLs (query strings, referrer leakage) under
[A06:2025, Insecure Design](#a062025--insecure-design) (CWE-598). It tracks sensitive data written to logs under
[A09:2025, Security Logging and Alerting Failures](#a092025--security-logging-and-alerting-failures) (CWE-532).

## A05:2025 — Injection

Dropped from #3 to #5. It remains one of the most-tested categories: OWASP data shows 100% of applications were tested
for some form of injection. That is a coverage figure, not a prevalence claim.
[injection-patterns.md](injection-patterns.md) covers it in detail. At triage time:

- SQL: any keyword-bearing string built with concatenation or templating from user input
- NoSQL: query objects assembled from raw request bodies (`{ $ne: ... }` injection)
- Command: shell invocations with concatenated arguments
- LDAP, XPath, header, and log injection have the same shape: untrusted input enters a structured-syntax string.
- LLM or agent prompt injection:
    - It treats untrusted content (tool output, retrieved documents, or chat history) as trusted instructions.
    - The A05:2025 page flags this class but covers it separately in the OWASP LLM Top 10 (LLM01:2025 Prompt Injection).
    - Cite LLM01 for these findings.

## A06:2025 — Insecure Design

Renumbered from A04:2021. This category remains architectural rather than pattern-based. Watch for:

- Workflow that sends sensitive operations as fire-and-forget without confirmation
- Reset flows with predictable tokens
- Trust placed in a header that the edge does not strip (for example, `X-Forwarded-For` for authorization), CWE-807.
- Multi-step processes where state is held client-side and resubmitted (privilege escalation), CWE-642.
- Sensitive data placed in URLs or query strings (browser history, proxy logs, referrer leakage), CWE-598.
- Credentials or secrets held in memory or storage without a defined protection boundary, CWE-256 and CWE-501.

When a code audit shows a design-time concern, escalate to the `threat-model` skill rather than flag it inline.

## A07:2025 — Authentication Failures

Renamed from "Identification and Authentication Failures" (2021). Same rank (#7) and scope.
[auth-authz-checklist.md](auth-authz-checklist.md) covers it. Quick triage:

- Hardcoded JWT secret
- `algorithm: "none"` accepted
- Missing `aud` / `iss` / `exp` validation
- Session fixation (no rotation on login)
- Predictable reset tokens
- Unprotected brute-force, credential-stuffing, or password-spray surfaces (no rate limiting, no lockout)

## A08:2025 — Software or Data Integrity Failures

Renamed from "Software and Data Integrity Failures" (2021). The scope narrowed slightly now that dependency and
build-pipeline sourcing moved to [A03:2025](#a032025--software-supply-chain-failures). This category covers how you
verify the integrity of what you already trust as a source. It does not cover the sourcing itself.

| Look for                                                                              |
| ------------------------------------------------------------------------------------- |
| Unsigned auto-update flows                                                            |
| Deserialization of untrusted input (`pickle.loads`, `yaml.load`, `BinaryFormatter`)   |
| CI workflow that executes a downloaded artifact without checksum verification         |
| Plugin, library, or CDN asset loaded without subresource integrity or signature check |

## A09:2025 — Security Logging and Alerting Failures

Renamed from "Security Logging and Monitoring Failures" (2021). Same rank (#9) and scope.

| Look for                                                      |
| ------------------------------------------------------------- |
| Authentication events not logged (success or failure)         |
| Authorization denial silently dropped                         |
| Empty catch blocks around security-sensitive operations       |
| Logs that capture full request bodies including tokens or PII |
| No alerting wired to security events                          |

A diff that adds a security-sensitive code path without a corresponding log line is a finding.

## A10:2025 — Mishandling of Exceptional Conditions

New in 2025. It covers improper error handling and logic errors that come from abnormal conditions. These occur in
applications that fail to prevent, detect, or gracefully respond to exceptional states.

| Look for                                                                                              | Why it matters                                             |
| ----------------------------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| Verbose stack traces or database error details returned to clients                                    | Sensitive data exposure, CWE-209                           |
| Security-relevant check (auth, authorization, quota) that fails open inside a catch or except block   | Attacker forces the exception to bypass the check, CWE-636 |
| Missing or incomplete input validation before use                                                     | Reaches a downstream sink in an unvalidated state          |
| Error handling deferred to a single high-level catch instead of at the point of failure               | Loses context needed to fail safely                        |
| Resource cleanup skipped when an exception is thrown (unclosed file handles, sockets, upload streams) | Resource exhaustion or DoS                                 |
| Partial or inconsistent rollback on a multi-step transaction failure                                  | State corruption (e.g., financial transactions)            |
| NULL or undefined dereference reachable from attacker-controlled input                                | Crash or undefined behavior, CWE-476                       |
| No custom error page, with the framework default error page returned                                  | Information disclosure, CWE-756                            |

```python
# Vulnerable: fails open on error
def check_permission(user, resource):
    try:
        return acl.lookup(user, resource)
    except ACLServiceError:
        return True  # allow on error

# Safer: fails closed, logs, and stays fail-closed
def check_permission(user, resource):
    try:
        return acl.lookup(user, resource)
    except ACLServiceError:
        logger.error("acl lookup failed for user=%s resource=%s; denying by default", user, resource)
        return False
```

## Cross-reference

Representative CWEs per category, verified against each category's published 2025 mapping. For the full lists, see the
category pages linked at the top of this file.

| Category | CWE                                         |
| -------- | ------------------------------------------- |
| A01      | CWE-22, CWE-352, CWE-639, CWE-862, CWE-918  |
| A02      | CWE-16, CWE-489, CWE-547, CWE-611           |
| A03      | CWE-1035, CWE-1104, CWE-1357, CWE-1395      |
| A04      | CWE-296, CWE-321, CWE-327, CWE-330          |
| A05      | CWE-77, CWE-78, CWE-79, CWE-89, CWE-94      |
| A06      | CWE-256, CWE-501, CWE-598, CWE-807          |
| A07      | CWE-287, CWE-384, CWE-521, CWE-798          |
| A08      | CWE-345, CWE-502, CWE-829                   |
| A09      | CWE-117, CWE-223, CWE-532, CWE-778          |
| A10      | CWE-209, CWE-274, CWE-476, CWE-636, CWE-756 |
