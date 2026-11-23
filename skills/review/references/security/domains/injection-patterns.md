# Injection patterns

Reference for audit Subagent #1 alongside [owasp-top10-checklist.md](owasp-top10-checklist.md). Trace source-to-sink
before you flag.

## SQL injection (CWE-89)

**Dangerous shape.** Any string-built SQL with user input inside the keyword region.

```javascript
// Vulnerable
const rows = await db.query(`SELECT * FROM users WHERE email = '${req.body.email}'`);
```

```python
# Vulnerable
cursor.execute("SELECT * FROM users WHERE id = " + request.args["id"])
```

**Safe alternative.** Parameterize via the driver's binding API.

```javascript
// `?` is the MySQL placeholder; node-postgres writes `$1`
const rows = await db.query("SELECT * FROM users WHERE email = ?", [req.body.email]);
```

```python
cursor.execute("SELECT * FROM users WHERE id = %s", (request.args["id"],))
```

```csharp
using var cmd = new SqlCommand("SELECT * FROM Users WHERE Id = @id", conn);
cmd.Parameters.AddWithValue("@id", id);
```

**ORM exceptions.** Most ORMs parameterize automatically, as in `users.findOne({ id })`. Watch for raw escape hatches:
``db.raw(`...${x}...`)``, `Sequelize.literal`, `EF.FromSqlRaw` with an interpolated string, and SQLAlchemy `text()` with
f-strings.

## NoSQL injection (CWE-943)

**MongoDB.** Operator injection via raw request body:

```javascript
// Vulnerable: attacker sends { "username": { "$ne": null }, "password": { "$ne": null } }
db.users.findOne({ username: req.body.username, password: req.body.password });
```

Validate input shape (string, not object) at the boundary, or whitelist allowed operators. Use a validator like Zod,
Joi, or class-validator before the value reaches the query.

## Command injection (CWE-78)

**Dangerous shape.** Shell invocation with concatenated arguments.

```javascript
// Vulnerable
require("child_process").exec(`convert ${req.body.path} out.png`);
```

```python
# Vulnerable
os.system(f"git clone {url}")
subprocess.run(f"tar -xzf {archive}", shell=True)
```

**Safe alternative.** Pass arguments as an array. Do not invoke a shell.

```javascript
require("child_process").execFile("convert", [req.body.path, "out.png"]);
```

```python
subprocess.run(["git", "clone", "--", url], check=True)
```

```csharp
var psi = new ProcessStartInfo("git") { UseShellExecute = false };
psi.ArgumentList.Add("clone");
psi.ArgumentList.Add("--");
psi.ArgumentList.Add(url);
Process.Start(psi);
```

An argument list stops shell injection, not argument injection: a value that starts with `-` still reaches the program
as an option (`git clone --upload-pack=...` reads it as one). Put `--` before the untrusted value when the program
accepts it, as the Python and C# examples do, or reject values that start with `-`. The JavaScript example needs one of
these too.

When the shell is required, validate the input against a strict allowlist before you compose the command.

## LDAP injection (CWE-90)

User input concatenated into an LDAP filter (`(uid=...)`) lets attackers manipulate matches. Escape per RFC 4515 (`*`,
`(`, `)`, `\`, NUL) or use a parameterized LDAP API. Most modern LDAP libraries provide a builder. Use it.

## XPath injection (CWE-643)

Same shape as SQL injection, with user input concatenated into an XPath expression. Use parameterized XPath APIs (for
example, Java's `XPathVariableResolver` with `$name` variables in the expression) or escape quotes deterministically.

## Cross-site scripting (XSS), CWE-79

### Reflected XSS

User input echoed into the response.

```javascript
// Vulnerable
res.send(`<h1>Hello ${req.query.name}</h1>`);
```

Use a templating engine that escapes by default (Handlebars `{{ }}`, JSX, Razor, or Jinja2 with `autoescape` on). Never
bypass with `{{{ }}}`, `dangerouslySetInnerHTML`, or `@Html.Raw` on user input.

### Stored XSS

The app persists user input, then renders it to other users. Use the same defense. For rich-text fields, also consider a
vetted library (DOMPurify, Bleach) that sanitizes on write.

### DOM XSS

```javascript
// Vulnerable
document.querySelector("#out").innerHTML = location.hash.slice(1);
```

Treat any browser source (`location.*`, `document.referrer`, `window.name`, `postMessage` data) as untrusted. Write to
`textContent`, not `innerHTML`. Avoid `eval`, `Function(...)`, `setTimeout(string)`.

## Server-side template injection (SSTI), CWE-1336

User input concatenated into a template string grants RCE in many engines, such as Jinja2, Twig, ERB, Handlebars (when
it compiles user-supplied text), and Velocity.

```python
# Vulnerable
template = Environment().from_string("Hello " + name)
```

Templates must be constants in the source. User input is a value passed to the template, not part of the template
source.

## Header / response splitting (CWE-113)

User input in `Set-Cookie`, `Location`, or custom headers enables splitting and cache poisoning when the code does not
strip CR/LF. Modern frameworks reject `\r\n` in header values. Verify that the code uses the header API and not raw
socket writes.

## Log injection (CWE-117)

User input written into log lines unescaped lets attackers forge log entries.

```javascript
// Vulnerable
logger.info(`login failed for ${req.body.username}`);
```

Use structured logging (`logger.info({ event: "login_failed", username })`). Strip CR/LF when you log free text. Never
log secrets, tokens, or full request bodies.

## Path traversal (CWE-22)

```python
# Vulnerable
open(os.path.join(BASE_DIR, request.args["file"]))
```

```javascript
// Vulnerable
fs.readFile(path.join(uploadDir, req.params.name));
```

**Safe pattern.** Resolve to an absolute path. Verify that it stays inside the intended root.

```python
import os
def safe_join(base, name):
    full = os.path.realpath(os.path.join(base, name))
    if not full.startswith(os.path.realpath(base) + os.sep):
        raise ValueError("path traversal")
    return full
```

```javascript
const path = require("path");
function safeJoin(base, name) {
    const full = path.resolve(base, name);
    const root = path.resolve(base);
    if (!full.startsWith(root + path.sep)) throw new Error("traversal");
    return full;
}
```

The JavaScript form checks the path text only, and the Python form also follows symlinks. When the root can hold
symlinks, resolve the JavaScript result with `fs.realpathSync` before the check. Reject names that contain `..`, NUL, or
absolute prefixes before you join them. Use a whitelist of allowed names where possible.

## Open redirect (CWE-601)

```javascript
// Vulnerable
res.redirect(req.query.next);
```

Allowlist hosts, or restrict to relative paths that start with `/` and not `//` or `/\`. Browsers read both as a
protocol-relative URL and redirect off-domain.

## SSRF (CWE-918)

See A01 (Broken Access Control, which includes SSRF as of the 2025 edition) in
[owasp-top10-checklist.md](owasp-top10-checklist.md). Validate scheme, host, and resolved IP class. Either disable
redirects or re-validate after each hop.

## Detection heuristics

| Pattern                                                         | Likelihood it is exploitable                                   |
| --------------------------------------------------------------- | -------------------------------------------------------------- |
| String concat or template inside a SQL keyword block            | High. Assume vulnerable until ORM or parameterization is shown |
| `shell: true` or `shell=True` with concatenated args            | High                                                           |
| `innerHTML` / `dangerouslySetInnerHTML` with non-constant value | High                                                           |
| `pickle.loads`, `yaml.load`, `BinaryFormatter` on request data  | Critical, RCE class                                            |
| `path.join(base, userInput)` without root containment check     | Medium-High                                                    |
| `redirect(userInput)` without allowlist                         | Medium                                                         |
| `Math.random()` in a token-shaped variable                      | Medium                                                         |
