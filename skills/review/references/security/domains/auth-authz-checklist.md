# Authentication and authorization checklist

Reference for audit Subagent #2. Run these checks on any change that touches identity, sessions, tokens, or access
control.

## Sessions

| Check                                                       | What to look for                                       |
| ----------------------------------------------------------- | ------------------------------------------------------ |
| Session ID rotates on login                                 | Old session not destroyed → session fixation (CWE-384) |
| Session bound to IP or device fingerprint where appropriate | Stolen cookie reusable from anywhere                   |
| Idle and absolute timeouts enforced server-side             | Client-side expiry only is bypassable                  |
| Logout invalidates the server-side record                   | Logout that only deletes the cookie is not logout      |
| Concurrent session policy is explicit                       | Silent allow-all may be intended; if so, document it   |

## Cookies

Required flags for session and auth cookies:

| Flag       | Setting                                                     |
| ---------- | ----------------------------------------------------------- |
| `Secure`   | Always (HTTPS-only)                                         |
| `HttpOnly` | Always (JS cannot read)                                     |
| `SameSite` | `Lax` minimum; `Strict` for high-sensitivity flows          |
| `Path`     | Covers every route the cookie protects (`/` is normal)      |
| `Domain`   | Omit it (host-only) unless subdomains must share the cookie |
| Expiration | Short for session cookies; explicit for persistent cookies  |

```javascript
res.cookie("sid", token, {
    httpOnly: true,
    secure: true,
    sameSite: "lax",
    path: "/",
    maxAge: 1000 * 60 * 60 * 8,
});
```

## JWT

| Check                                                                   | Failure mode                                      |
| ----------------------------------------------------------------------- | ------------------------------------------------- |
| Algorithm pinned by the verifier (do not accept `alg` from the token)   | `alg: none` / algorithm confusion (RS256 ↔ HS256) |
| Signature verified before any claim is read                             | Trusting unverified claims                        |
| `exp`, `nbf` validated                                                  | Token replay long after issuance                  |
| `aud` validated against this service                                    | Cross-audience replay                             |
| `iss` validated against expected issuer(s)                              | Token from another tenant accepted                |
| Key rotation via JWKS, not hardcoded keys                               | Key compromise = forever compromise               |
| Short lifetime (minutes), refresh tokens for long-lived sessions        | Stolen access tokens valid for hours              |
| Revocation strategy in place (denylist, short TTL, or token versioning) | Cannot kick a compromised user                    |

```javascript
// Safe verification (jose: pinned algorithm, keys from the issuer's JWKS, audience and issuer checked)
import { createRemoteJWKSet, jwtVerify } from "jose";

const jwks = createRemoteJWKSet(new URL("https://auth.example.com/.well-known/jwks.json"));

const { payload } = await jwtVerify(token, jwks, {
    algorithms: ["RS256"],
    audience: "api.example.com",
    issuer: "https://auth.example.com",
});
```

`jwtVerify` rejects an expired token and one whose `nbf` has not been reached. With `jsonwebtoken`, a key-lookup
function is only valid in the asynchronous form, `jwt.verify(token, getKey, options, callback)`. The synchronous call
throws.

Never store JWTs in `localStorage` for high-value sessions. XSS can exfiltrate them. Cookies with `HttpOnly` plus CSRF
defense are safer.

## Password storage (CWE-256, CWE-916)

| Algorithm               | Acceptable use                                 | Notes                             |
| ----------------------- | ---------------------------------------------- | --------------------------------- |
| argon2id                | Preferred for new code                         | Tune memory, time, parallelism    |
| scrypt                  | Acceptable                                     | Tune `N`, `r`, `p`                |
| bcrypt                  | Acceptable; cost 12 minimum                    | Watch the 72-byte input cap       |
| PBKDF2                  | Acceptable when above unavailable; ≥600k iters | SHA-256 minimum                   |
| MD5, SHA-1              | Never                                          | Cracked in seconds                |
| Plain SHA-256 / SHA-512 | Never (without KDF)                            | Fast hashes are not for passwords |

Always use a per-user salt. The KDF generates it. Never log or echo the hash. A pepper is optional. If you use one,
store it outside the DB, in a KMS or secret manager.

## OAuth 2.0 / OIDC

| Check                                                                |
| -------------------------------------------------------------------- |
| Authorization Code flow with PKCE for public clients                 |
| `state` parameter validated to prevent CSRF on the callback          |
| `nonce` validated for OIDC flows                                     |
| Redirect URIs strictly allowlisted (no wildcards, no open redirects) |
| Tokens never logged, never appear in URLs after the callback         |
| Implicit flow not used                                               |
| Resource Owner Password Credentials flow not used (deprecated)       |
| ID token signature, audience, issuer, expiration verified            |

## API keys / service credentials

| Check                                                                       |
| --------------------------------------------------------------------------- |
| Keys scoped to the minimum permissions required                             |
| Rotation procedure documented; keys do not live forever                     |
| Stored in a secret manager, not committed to source                         |
| Logged usage tied to key ID, not key value                                  |
| Rate limiting per key                                                       |
| Key prefix or ID identifiable (for revocation) without revealing the secret |

## Authorization (authz)

| Pattern        | When to use                                            | Pitfalls                                    |
| -------------- | ------------------------------------------------------ | ------------------------------------------- |
| RBAC           | Static, role-driven access                             | Role explosion; combinations not modeled    |
| ABAC           | Attribute-driven (resource owner, tenant, time-of-day) | Policy complexity; needs central evaluation |
| ReBAC          | Relationship-driven (user → group → resource)          | Needs a graph store / dedicated service     |
| Scopes (OAuth) | Third-party access delegation                          | Scopes are not authorization on their own   |

Required checks at every protected endpoint:

1. **AuthN.** Is the caller who they say they are? (Verified token / session)
2. **AuthZ.** Does the caller have permission for _this resource_ and _this operation_?
3. **Tenancy.** For multi-tenant systems, is the resource in the caller's tenant?
4. **Object-level.** Does the caller own or have access to the specific record?

A handler that does (1) but skips (2)–(4) is the most common authorization bug.

```javascript
// Vulnerable: auth without object check
app.delete("/posts/:id", requireAuth, async (req, res) => {
    await db.posts.delete(req.params.id);
    res.sendStatus(204);
});

// Safe
app.delete("/posts/:id", requireAuth, async (req, res) => {
    const post = await db.posts.findOne({ id: req.params.id });
    if (!post) return res.sendStatus(404);
    if (post.authorId !== req.user.id && !req.user.isAdmin) return res.sendStatus(403);
    await db.posts.delete(req.params.id);
    res.sendStatus(204);
});
```

## CORS

| Check                                                                            |
| -------------------------------------------------------------------------------- |
| `Access-Control-Allow-Origin` is explicit, not `*`, when credentials are allowed |
| `Access-Control-Allow-Credentials: true` only on origins that need it            |
| Preflight responses cache duration is bounded                                    |
| Allowed methods and headers are minimal                                          |
| Origin allowlist does not include `null`                                         |

## CSRF

| Check                                                                                 |
| ------------------------------------------------------------------------------------- |
| State-changing routes require a CSRF token, double-submit cookie, or SameSite cookie  |
| GETs are safe (no side effects). Otherwise CSRF defense applies to GETs too           |
| Bearer-token APIs (no cookies) are not CSRF-vulnerable; do not invent token machinery |

## Account lifecycle

| Surface          | Check                                                                                     |
| ---------------- | ----------------------------------------------------------------------------------------- |
| Registration     | Email verification required for trust-building features                                   |
| Password reset   | Cryptographically random tokens, single-use, short TTL, bound to user                     |
| Account lockout  | After N failed attempts, with backoff; do not trivially expose lockout state to attackers |
| MFA enrollment   | Required at least optionally; recovery codes generated and shown once                     |
| Account deletion | Documented; respects data-retention policy                                                |

## What to flag

A handler that adds a new authenticated route without an object-level authorization check is High severity. A handler
that handles JWT by hand instead of with a vetted library is at least Medium. It likely needs a full check against the
JWT table above.
