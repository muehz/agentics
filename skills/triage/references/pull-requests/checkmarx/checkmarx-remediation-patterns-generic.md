# Checkmarx Remediation Patterns — Generic (Language-Agnostic)

Safe, behavior-preserving fix patterns for common Checkmarx findings in general-purpose application and service code.
Examples use Python, Go, Java, server-side Node.js, and Rust where they apply. Load this file during Phase 3 of the
Checkmarx triage workflow, before you apply any remediations.

**See also:** [`checkmarx-remediation-patterns-actions-js.md`](checkmarx-remediation-patterns-actions-js.md) has the
same patterns scoped to GitHub Actions JS/TS code (`@actions/core` toolkit, workflow YAML, `${{ secrets.* }}`
references). The heuristic in `checkmarx-triage.md` picks the file. For mixed repos, load both.

## Guardrails

### DO

- Redact credentials, tokens, and secret-derived values before the code writes them to logs, error messages, or files
- Sanitize error messages to remove parameter values, internal paths, and stack traces from anything user-facing
- Replace a clearly fake credential in a test fixture with a clearly-fake placeholder
- Give the developer a real credential found in source, as a finding to rotate and then move to the project's existing
  secrets-management mechanism (env vars, secret manager, config provider). Never commit that change yourself.
- Suggest HSTS and other HTTP security response headers, never commit them, and only for server-side code that emits
  HTTP responses

### DON'T

These changes look like safe fixes but break behavior:

- Introduce new configuration knobs, environment variables, or secret keys to "fix" a hardcoded value. This changes the
  public configuration contract. Use what the project already has.
- Change error class constructors, exception types, or error-handling flow. Call sites may match on type or message
  text. Additional try/catch around existing error handlers changes propagation flow and may swallow legitimate errors.
- Modify function signatures, return types, or public method contracts. Callers depend on the current signature.
- Alter authentication logic, request signing, or external API call patterns
- Remove or rename existing public interfaces, fields, or configuration keys
- Change test behavior, test assertions, or observable side effects. Assertions that you alter to match sanitized output
  hide real test failures and mask regressions.
- Redact globally via a log filter without field-level helpers. Redaction filters are easy to misconfigure.
  Per-call-site redaction at construction time is easier to verify.

## Patterns

### Secret masking / log redaction

**Principle.** Credentials, tokens, signed URLs, and any value derived from them must never appear in logs. The rule
covers the library logs that the application forwards. Redact where the code logs the value, or where the code builds a
value that always carries a secret.

```python
# Python — Before
logger.info("Fetching %s", url)  # url contains an embedded basic-auth credential

# After
from urllib.parse import urlsplit, urlunsplit

def redact_url(u: str) -> str:
    """Drop the userinfo and the query string."""
    try:
        parts = urlsplit(u)
    except ValueError:
        return "<unparsable URL>"
    host = parts.netloc.rpartition("@")[2]
    return urlunsplit(parts._replace(netloc=host, query=""))

logger.info("Fetching %s", redact_url(url))
```

```go
// Go — Before
log.Printf("Fetching %s", url)

// After
log.Printf("Fetching %s", redactURL(url))

// redactURL drops the userinfo and the query string (net/url).
func redactURL(raw string) string {
    u, err := url.Parse(raw)
    if err != nil {
        return "<unparsable URL>"
    }
    u.User = nil
    u.RawQuery = ""
    return u.String()
}
```

```java
// Java — Before
log.info("Fetching {}", url);

// After
log.info("Fetching {}", Redact.url(url));
// Redact.url drops the userinfo and the query string, for example through java.net.URI
```

```javascript
// Node.js (server-side) — Before
logger.info(`Fetching ${url}`);

// After
function sanitizeUrl(u) {
    let parsed;
    try {
        parsed = new URL(u);
    } catch {
        return "<unparsable URL>";
    }
    parsed.username = "";
    parsed.password = "";
    parsed.search = ""; // drop the query string
    return parsed.href;
}

logger.info(`Fetching ${sanitizeUrl(url)}`);
```

```rust
// Rust — Before
tracing::info!(%url, "fetching");

// After
tracing::info!(url = %redact_url(&url), "fetching");
// redact_url drops the userinfo and the query string, for example with the url crate
```

**DO NOT:**

- Delete the log statement to "fix" the finding. You lose operational visibility, and redaction keeps it.
- Lower the log level in place of redaction. The secret still reaches log sinks.
- Hand-roll a per-call-site redact regex five different ways. Define one helper. Reuse it.

### Error message sanitization

**Principle.** Errors travel further than logs. They reach exception handlers, monitoring tools, sometimes user-facing
output, and often go back into logs. Strip parameter values, full URLs with query strings, stack traces, and absolute
internal paths from error messages. Keep the operation name, error category, and any non-sensitive identifiers, because
an operator needs these to triage.

```python
# Python — Before
raise AuthError(f"Auth failed for {url} with token {token}")

# After
raise AuthError(f"Auth failed for {redact_url(url)}")
```

```go
// Go — Before
return fmt.Errorf("auth failed for %s with token %s", url, token)

// After
return fmt.Errorf("auth failed for %s", redactURL(url))
```

```java
// Java — Before
throw new AuthException("Auth failed for " + url + " with token " + token);

// After
throw new AuthException("Auth failed for " + Redact.url(url));
```

```javascript
// Node.js — Before
throw new Error(`Auth failed for ${url} with token ${token}`);

// After
throw new Error(`Auth failed for ${sanitizeUrl(url)}`);
```

```rust
// Rust — Before
return Err(AuthError::new(format!("auth failed for {url} with token {token}")));

// After
return Err(AuthError::new(format!("auth failed for {}", redact_url(&url))));
```

**DO NOT:**

- Catch and re-raise with a sanitized message but lose the original cause. Keep the cause via `from` / `Throwable.cause`
  / `errors.Wrap` / `Error.cause`. Sanitize only the _outer_ message.
- Change the exception or error class to a "safer" one. Call sites may match on type.
- Reword the error in a way that breaks log-based alerting that greps for specific text.

### Hardcoded credential replacement

**Principle.** A real credential in source is a leak the moment it lands in version control history. The fix has two
parts, and both belong to the developer: rotate the credential, because a new commit does not remove it from history,
and then read it from the project's existing secrets mechanism instead. The triage specialist cannot rotate anything, so
it never commits this fix. It classifies the finding `true_positive` with no commit and gives the "After (real code)"
shape below as the suggested approach. For a test fixture holding a clearly fake value, the specialist may replace it
with a clearly-fake placeholder. A reviewer who skims the diff must see at once that it is not a real secret.

```python
# Python — Before
API_KEY = "ak_live_91f...REAL_KEY..."

# After (real code)
API_KEY = load_api_key()  # placeholder: call the project's existing secrets accessor

# After (test fixture)
API_KEY = "test-api-key-placeholder"
```

```go
// Go — Before
const apiKey = "ak_live_91f...REAL_KEY..."

// After (real code)
apiKey := loadAPIKey() // placeholder: call the project's existing secrets accessor

// After (test fixture)
const apiKey = "test-api-key-placeholder"
```

```java
// Java — Before
private static final String API_KEY = "ak_live_91f...REAL_KEY...";

// After (real code) — read through the project's existing secrets accessor
private final String apiKey = loadApiKey(); // placeholder call

// After (test fixture)
private static final String API_KEY = "test-api-key-placeholder";
```

```javascript
// Node.js — Before
const apiKey = "ak_live_91f...REAL_KEY...";

// After (real code)
const apiKey = loadApiKey(); // placeholder: call the project's existing secrets accessor

// After (test fixture)
const apiKey = "test-api-key-placeholder";
```

```rust
// Rust — Before
const API_KEY: &str = "ak_live_91f...REAL_KEY...";

// After (real code)
let api_key = load_api_key()?; // placeholder: call the project's existing secrets accessor

// After (test fixture)
const API_KEY: &str = "test-api-key-placeholder";
```

Follow the project's existing secrets-management documentation for the actual indirection (Vault, AWS Secrets Manager,
sealed config, dotfiles, CI secret stores). This patterns file does not prescribe a mechanism.

**DO NOT:**

- Add a new environment variable or config key unless you first check what the project already uses. Fragmentation is
  its own security problem.
- Commit a change to a real credential. The developer rotates it first and then changes the code.
- Replace a real credential with a placeholder in a non-test file. That is a behavior change that looks like a doc
  change.
- Copy the credential's value into a rationale, a section, or a reply.

### HSTS and HTTP security headers

**Principle.** HSTS, `Content-Security-Policy`, `X-Content-Type-Options`, and similar headers are **server-side
response** concerns. Code that handles inbound HTTP requests and writes responses emits them. They are **not**
applicable to:

- HTTP clients (`requests`, `net/http` client, `HttpClient`, `fetch`, `reqwest`). Clients consume responses. They do not
  set response headers.
- gRPC, message-bus, or non-HTTP service code
- CLI tools, batch jobs, and library code that does not bind to a port

If Checkmarx flags an HSTS-class finding on client or non-HTTP code, classify as **Not Applicable**. For server code,
the finding is real but not an automatic fix: the header changes how every browser treats the site for its whole
`max-age`, so the developer decides. The examples below are the suggested approach to give them, at the framework's
middleware layer rather than per-route. They are not changes to commit.

```python
# Python (Flask) — server-side response handler
@app.after_request
def set_hsts(response):
    response.headers["Strict-Transport-Security"] = "max-age=31536000; includeSubDomains"
    return response
```

```go
// Go (net/http) — server middleware
func hsts(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        next.ServeHTTP(w, r)
    })
}
```

```java
// Java (Spring Security) — server config
http.headers(h -> h.httpStrictTransportSecurity(hsts ->
    hsts.maxAgeInSeconds(31536000).includeSubDomains(true)));
```

```javascript
// Node.js (Express) — server middleware
app.use(helmet.hsts({ maxAge: 31536000, includeSubDomains: true }));
```

```rust
// Rust (axum) — server middleware
use tower_http::set_header::SetResponseHeaderLayer;
use axum::http::header::{HeaderValue, STRICT_TRANSPORT_SECURITY};

let layer = SetResponseHeaderLayer::if_not_present(
    STRICT_TRANSPORT_SECURITY,
    HeaderValue::from_static("max-age=31536000; includeSubDomains"),
);
```

Where the code already calls `helmet()`, the
[Actions-JS file's HSTS section](checkmarx-remediation-patterns-actions-js.md#hsts-header-addition-server-code-only)
shows how to pass the same HSTS options to that call.

**DO NOT:**

- Add code that sets HSTS to an HTTP **client** to "address" the finding. The header has no effect there, so the change
  adds dead code.
- Set HSTS in application code if the load balancer or reverse proxy already sets it. Duplicate headers cause confusion
  and may conflict.
- Pick a `max-age` of zero or a few minutes "to be safe". That disables HSTS for clients that already cached the policy.

### Unsafe deserialization

**Principle.** Several languages ship serialization formats that execute attacker-controlled code on deserialize. The
bug is to treat them as "just data formats". Use a data-only format (JSON, msgpack, protobuf) for any input that crosses
a trust boundary. Keep the unsafe formats for internal-only, fully-trusted producers, for example a single-process
cache.

**Python `pickle` from untrusted source.** `pickle.load` / `pickle.loads` invokes arbitrary `__reduce__` methods and can
run any code the attacker chooses. If the source is neither your own process nor a fully-trusted file under your
exclusive control, switch to:

- `json.loads`, for plain data
- A schema-validated format (`pydantic` over JSON, msgpack with a strict schema)

Never unpickle data from network requests, user uploads, or shared filesystems.

**Java Jackson polymorphic types.** Default typing (`ObjectMapper.enableDefaultTyping()`) or
`@JsonTypeInfo(use = Id.CLASS)` with untrusted input lets an attacker instantiate gadget classes on the classpath and
trigger RCE. To fix it, choose from these options:

- Disable default typing.
- Use `@JsonTypeInfo(use = Id.NAME)` with an explicit `@JsonSubTypes` allowlist.
- Migrate to a polymorphic-free DTO.

For raw input from network calls, prefer concrete typed `readValue(json, Foo.class)` over generic `Object`
deserialization.

**Node.js `eval` / `vm` from untrusted strings.** `eval(userInput)`, `new Function(userInput)`, and
`vm.runInNewContext(userInput)` all execute attacker-supplied JavaScript. There is no safe way to "sandbox" this in
`vm`, because `vm` has known escapes. Replace it with one of these:

- `JSON.parse`, for data
- A real expression evaluator (`expr-eval`, `jsep` + manual walk), for math
- A worker process with strict ipc, for actual code execution

**Go `gob` from untrusted source.** `encoding/gob.Decode` registers types that the stream references, and crafted input
can force it into massive memory allocations or panics. `gob` has no schema and no allocation cap. For data that crosses
trust boundaries, use `encoding/json`, `protobuf`, or `msgpack` with a known schema. Reserve `gob` for in-process or
fully-trusted-channel use.

**DO NOT:**

- "Validate" untrusted input and then deserialize it with `pickle` / `gob` / default-typing Jackson. For these formats,
  validation runs _after_ deserialization, and the RCE has already happened.
- Wrap unsafe deserialization in try/catch on the assumption that crashed gadget chains mean safety. An attacker only
  needs one to succeed.
- Switch from `pickle` to `dill` or `cloudpickle` to "fix" the finding. They have the same RCE properties.
