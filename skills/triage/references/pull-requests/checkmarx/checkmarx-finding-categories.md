# Finding Categories

Known Checkmarx One finding categories, each with its meaning, common false positive patterns, and assessment guidance.
In this file, **mask or redact** means `core.setSecret()` in GitHub Actions code and a language-appropriate redaction
helper applied at construction time in general code. The remediation-patterns files show each mechanism.

The headings group categories by the label Checkmarx usually gives them. A finding's severity comes from its own row's
label, mapped to the shared four as [checkmarx-triage.md](checkmarx-triage.md#track-findings-in-working-memory) says, so
a CRITICAL row takes the same category guidance as a row of any other label, whatever heading its category sits under.

## HIGH severity

### Passwords And Secrets - Generic Password

**What it means:** Checkmarx detected what appears to be a hardcoded password, API key, token, or other secret in the
scanned file. The category covers any source or config file in any language.

**Common contexts:**

- Source files (Python, Go, Java, JavaScript, Rust, C#, etc.) with string literals that look like credentials
- Configuration files (YAML, JSON, TOML, properties, env-style) with `password:`, `token:`, `apiKey:`, or `secret:` keys
- Test fixtures with placeholder credentials
- Infrastructure-as-code (Terraform, Pulumi, Helm, Kustomize) with embedded values
- Workflow YAML and CI configuration with literal credential values (a sub-case of the above)

**False positive patterns:**

- **References, not values.** Anything that resolves at runtime is a reference. Examples are environment variables
  (`$PASSWORD`, `${PASSWORD}`, `os.environ["PASSWORD"]`, `os.Getenv("PASSWORD")`), config-provider lookups, and
  secret-manager fetches. The Actions-JS sub-case is `${{ secrets.PASSWORD }}` and `${{ vars.PASSWORD }}`. The runner
  resolves these template references.
- **Input/parameter declarations.** A definition that describes the shape of an input is not a credential. Examples are
  a function parameter named `password`, workflow `inputs:` with name `password`, and struct field `Password string`.
  Assess the value, not the key name.
- **Placeholder/dummy values.** Values like `test-password`, `placeholder`, `changeme`, and `xxxxx` in test files are
  not real credentials. You may still replace them with clearly-fake values that signal intent, for example
  `test-password-placeholder`.
- **Public, non-sensitive constants flagged by entropy.** Long random-looking strings that are public identifiers
  (UUIDs, deterministic test seeds, well-known public keys) are not secrets.

**Assessment guidance:**

- **True Positive (High confidence):** An actual credential value (API key, connection string with embedded password,
  PEM private key, signing token) appears in source or config.
- **Likely True Positive (Medium confidence):** A value looks like a credential, but you can't fully determine if it's
  real. An example is a long random-looking string in a YAML file with no clear public-identifier purpose.
- **False Positive (High confidence):** The value is a runtime reference (env var, secret-manager lookup,
  `${{ secrets.* }}` for Actions), a clearly-fake placeholder, or a public non-sensitive constant.

**Remediation:** A real credential is never an automatic fix. It is already in the PR's history, so removing it from the
next commit does not contain the leak, and only someone with access to the credential can rotate it. Classify it
`true_positive` with no commit, and give it to the developer: rotate it first, then replace the literal with a read from
the project's existing secrets-management mechanism (env var, secret manager, config provider). Never copy the value
into a rationale, a section, or a reply.

**Safe remediation:** A clearly fake value in a test fixture that the scanner flags can be replaced with a
clearly-labeled non-credential value. Masking or redacting a secret before it reaches a log is also safe.

---

## MEDIUM severity

### Missing_HSTS_Header

**What it means:** Checkmarx detected an HTTP client or server that does not set the HTTP Strict Transport Security
(HSTS) header. Without it, connections may be open to downgrade attacks.

**Common contexts:**

- HTTP client code that makes API calls (for example `fetch()`, `axios`, custom HTTP clients)
- Server-side code handling HTTP responses

**False positive patterns:**

- **API clients that only consume endpoints.** If the code is an HTTP client that calls an external API and does not
  serve HTTP, HSTS is not applicable.
- **Internal/private API calls.** These are calls to internal services over HTTPS where the infrastructure layer handles
  HSTS.

**Assessment guidance:**

- **Not Applicable (High confidence):** The flagged code is an HTTP client library that makes outbound API calls. HSTS
  is a server-side response header, and clients cannot set it.
- **True Positive (Medium confidence):** The flagged code is an HTTP server or response handler that should set HSTS
  headers.

**Remediation:** For client code, classify as Not Applicable. For server code, the finding goes to the developer with
the header as the suggested approach, never as a commit: HSTS tells every browser to refuse plain HTTP for the site
(and, with `includeSubDomains`, for every subdomain) for the whole `max-age`, which changes behavior well beyond this
PR, and a proxy or load balancer may already set it.

---

## LOW severity

### Privacy_Violation_in_Error_Messages

**What it means:** An error class or error-handling path puts possibly sensitive data (PII, credentials, internal URLs)
in its error message. Logs or user-facing output could then expose that data.

**Common contexts:**

- Custom error classes that include request parameters, URLs, or user identifiers in their message
- API client code that passes response details into error constructors
- Error aggregation that collects and re-throws multiple error details

**False positive patterns:**

- Error messages that include only non-sensitive context, such as operation name, request type, or status code. These
  are safe to log.
- Errors where the code masked or redacted the sensitive input before it reached the error path.

**Assessment guidance:**

- **True Positive (Medium confidence):** Error messages include URL parameters, request bodies, authentication tokens,
  or user-provided credential values that could contain secrets.
- **Likely True Positive (Low confidence):** Error messages include contextual data that might contain sensitive
  information. Whether it does depends on the runtime inputs. An example is a URL that could contain embedded
  credentials.
- **False Positive (High confidence):** Error messages include only operation names, status codes, or non-sensitive
  identifiers.

**Safe remediation:**

- Sanitize error messages to exclude sensitive parameters
- Mask or redact secrets before any code path that might log them.
- Replace raw URL/parameter inclusion with redacted versions

---

### Privacy_Violation_in_Files

**What it means:** Code writes possibly sensitive data (from user input, API responses, or configuration) to files. The
sensitive information can then persist on disk.

**Common contexts:**

- Code that writes configuration files that include credential-derived values
- Code that logs to files with request/response details
- Temporary file creation with sensitive content

**Assessment guidance:**

- **True Positive (Medium confidence):** File writes include values derived from credential inputs without sanitization.
- **Not Applicable (High confidence):** File writes contain only non-sensitive configuration data or the written values
  have already been sanitized.

---

### Secret_Leak_in_Error_Messages

**What it means:** An error-handling path includes secret values (API keys, passwords, tokens) in error messages that
could be logged or displayed.

**Common contexts:**

- API client code that passes authentication credentials through error chains
- Error classes that capture full request context including Authorization headers
- Login/authentication methods that include credential values in failure messages

**False positive patterns:**

- **Secrets masked or redacted before error paths.** If the code masked the secret at input time, it will not appear in
  raw form in error messages or logs.
- **Error messages that reference the secret input name but not the value.** For example, "Failed to authenticate with
  the provided api-key input" names the input, not its value.

**Assessment guidance:**

- **True Positive (High confidence):** A secret value flows directly into an error message, and nothing masks or redacts
  it first.
- **Likely True Positive (Medium confidence):** A secret value could flow into an error message through some error
  paths, but the primary path masks it.
- **False Positive (Medium confidence):** The error message references input names or types, not values, OR the secret
  is reliably masked/redacted before any error path.

**Safe remediation:**

- Mask or redact secret inputs at the earliest point in the program, which is input parsing/validation.
- Sanitize error messages to exclude raw credential values
- Use generic error messages for auth failures: "Authentication failed" instead of "Authentication failed with password:
  {value}"

---

### Use_of_Deprecated_or_Obsolete_Functions

**What it means:** Code calls an API, function, or method that its maintainer deprecated. The maintainer may remove it
in a future version.

**Common contexts:**

- Calls to deprecated standard-library APIs (Node.js, Python, Go, Java, .NET)
- Use of deprecated third-party library methods
- Calls to deprecated internal APIs within workspace/monorepo packages

**False positive patterns:**

- **The "deprecated" API is the only available option.** If no replacement exists yet, the use of the deprecated API is
  intentional.
- **The deprecation is internal.** It is within this monorepo's own code, and nobody has built the replacement yet.

**Assessment guidance:**

- **True Positive (Medium confidence):** The deprecated function has a documented replacement that is available and
  compatible.
- **Not Applicable (High confidence):** The function is not actually deprecated, or the scanner misidentified a naming
  pattern as a deprecation indicator.
- **True Positive, not remediable (Medium confidence):** The function is deprecated but the replacement requires
  architectural changes that are out of scope.

**Safe remediation:** If a drop-in replacement exists, swap the function call. If the replacement requires API changes,
architectural changes, or new dependencies, the finding is still `true_positive`: leave it without a commit and give it
an `intervention_reason`. The rollup counts it under Needs you. `likely_positive` is for a finding you could not
confirm, not for one you confirmed and cannot fix.
