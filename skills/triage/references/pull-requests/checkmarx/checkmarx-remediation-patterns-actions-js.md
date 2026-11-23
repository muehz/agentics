# Checkmarx Remediation Patterns — GitHub Actions (JS/TS)

Safe, behavior-preserving fix patterns for common Checkmarx findings in **GitHub Actions JavaScript/TypeScript** code
(custom actions, workflow scripts, `@actions/*` toolkit usage). Load it during Phase 3 of the Checkmarx triage workflow,
before you apply any remediations.

**See also:** [`checkmarx-remediation-patterns-generic.md`](checkmarx-remediation-patterns-generic.md) holds the
language-agnostic patterns (Python, Go, Java, server-side Node.js, Rust). The heuristic in `checkmarx-triage.md` picks
the file. For mixed repos, load both.

## Guardrails

### DO

- Add `core.setSecret()` calls before the code uses values in logs
- Sanitize error messages to remove sensitive parameter values
- Replace hardcoded test credentials with clearly-fake placeholder values (for example `test-password-placeholder`)
- Add log redaction for URLs that contain credentials

### DON'T

These changes look like safe fixes but break behavior:

- Introduce new action inputs or secrets references (`${{ secrets.X }}`), for example to replace hardcoded values. A new
  input changes the action's public API contract.
- Change error class constructors or error handling flow. Catch blocks may match on message text. A try/catch around
  existing error handlers changes the error propagation flow and may swallow legitimate errors.
- Modify function signatures or return types. Callers depend on the current signature.
- Alter authentication logic or API call patterns
- Remove or rename existing inputs or outputs
- Change test behavior or test assertions. Assertions that you alter to match sanitized output hide real test failures
  and mask regressions.

## Patterns

### Secrets masking (`core.setSecret`)

```typescript
// Before
const token = core.getInput("api-token");
await deploy(token);

// After
const token = core.getInput("api-token");
core.setSecret(token);
await deploy(token);
```

### Error message sanitization

```typescript
// Before
throw new Error(`Auth failed for ${url} with token ${token}`);

// After
throw new Error(`Auth failed for ${sanitizeUrl(url)}`);
```

### Placeholder credential replacement

```yaml
# Before (test fixture)
password: "myRealPassword123"

# After
password: "test-password-placeholder"
```

### Log redaction

```typescript
// Before
console.log(`Fetching ${url}`);

// After
function sanitizeUrl(u: string): string {
    let parsed: URL;
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

console.log(`Fetching ${sanitizeUrl(url)}`);
```

### HSTS header addition (server code only)

Adding HSTS changes how every browser treats the site for the header's whole `max-age`, so this is the suggested
approach for the developer, not a change to commit (see
[checkmarx-finding-categories.md](checkmarx-finding-categories.md#missing_hsts_header)).

```typescript
// Before
app.use(helmet());

// After
app.use(helmet({ hsts: { maxAge: 31536000, includeSubDomains: true } }));
```

This pattern does not apply to HTTP client code, because HSTS is a server-side response header. If the flagged code is a
client, classify the finding as `not_applicable`.

### Deprecated function replacement

```typescript
// Before
const result = oldApi.deprecatedMethod(data);

// After
const result = newApi.replacementMethod(data);
```

Use this pattern only if a drop-in replacement exists. If the replacement requires API changes or new dependencies, the
finding stays `true_positive` with no commit and an `intervention_reason`. The rollup counts it under Needs you.

**Where Actions code lives.** Workflow files and everything under `.github/workflows/` and `.github/actions/` are paths
triage never pushes ([subagent-contract.md](../../subagent-contract.md#paths-triage-never-pushes)). A finding there is
assessed like any other, and its fix goes to the developer as a suggested approach, never as a commit. These patterns
apply as commits only to Actions code outside those paths, such as a JavaScript action published from its own directory.
