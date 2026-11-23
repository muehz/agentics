# MCP Security Checklist

MCP servers — especially MCPB bundles — run with the user's privileges. There is no permissions model. Security is the
developer's responsibility.

## Path Validation

Every tool that accepts a path must validate it.

| Check                            | What to do                                                                                                                             |
| -------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Resolve symlinks                 | Use `realpath` for reporting or the immutable-root read fallback. Use the handle-relative helpers for file operations where supported. |
| Reject parent-directory segments | After resolution, verify that the path is still under the allowed root                                                                 |
| Confine to allowed roots         | Maintain an explicit allowlist (e.g., user_config workspace path)                                                                      |
| Reject device files              | On Unix, reject `/dev/*`, `/proc/*`, `/sys/*`                                                                                          |
| Reject Windows reserved names    | `CON`, `PRN`, `AUX`, `NUL`, `COM1-9`, `LPT1-9`                                                                                         |

```javascript
function safePath(input, allowedRoot) {
    const realRoot = fs.realpathSync(allowedRoot);
    const resolved = path.resolve(allowedRoot, input);
    let realPath;
    try {
        // lstat sees a dangling symlink as existing, so realpath fails on it
        // instead of treating it as a new file.
        if (fs.lstatSync(resolved, { throwIfNoEntry: false })) {
            realPath = fs.realpathSync(resolved);
        } else {
            // A file that does not exist yet has no real path, so resolve its
            // parent directory instead. Only the file itself may be missing: a
            // missing directory could later be created as a link that leads
            // outside the root.
            realPath = path.join(fs.realpathSync(path.dirname(resolved)), path.basename(resolved));
        }
    } catch {
        // A path that cannot be resolved (dangling link, link loop, missing
        // parent directory or drive) cannot be confined, so reject it.
        throw new Error("Path traversal blocked");
    }
    if (!realPath.startsWith(realRoot + path.sep) && realPath !== realRoot) {
        throw new Error("Path traversal blocked");
    }
    return realPath;
}
```

`safePath` answers only whether the path was inside the root when the check ran. Use the handle-based operations below
to open files, subject to the read fallback rules.

## Safe reads inside a root

On POSIX with Python and on Linux with Node, use a handle-relative open for every read, regardless of the start-up
check. Ownership and mode bits cannot rule out a writer running as the server's user, such as a sync client or build
tool. Use a trusted, configured root. Open the directories relative to their parent handles, and open the final name
with `O_RDONLY | O_NOFOLLOW | O_NONBLOCK`. Check that the open descriptor refers to a regular file with exactly one hard
link. Read only from that descriptor.

Python uses `dir_fd` for relative opens:

```python
import errno
import os
import stat


def read_inside(allowed_root, user_path):
    parts = user_path.replace("\\", "/").split("/")
    if os.path.isabs(user_path) or any(p in ("", ".", "..") for p in parts):
        raise PermissionError("Path traversal blocked")
    handles = [os.open(allowed_root, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW)]
    try:
        for name in parts[:-1]:
            handles.append(os.open(name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=handles[-1]))
        handles.append(os.open(parts[-1], os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=handles[-1]))
        info = os.fstat(handles[-1])
        if not stat.S_ISREG(info.st_mode) or info.st_nlink != 1:
            raise PermissionError("Path traversal blocked: not a regular file with one link")
        with os.fdopen(handles[-1], "rb", closefd=False) as stream:
            return stream.read()
    except OSError as error:
        if error.errno in (errno.ELOOP, errno.ENOTDIR, errno.ENXIO):
            raise PermissionError("Path traversal blocked") from error
        raise
    finally:
        for handle in reversed(handles):
            os.close(handle)
```

On Linux, Node uses `/proc/self/fd` to resolve a name relative to an open directory handle. The example assumes `fs` and
`path` are the Node modules in scope.

```javascript
function readInside(allowedRoot, input) {
    const parts = input.split(/[\\/]/);
    if (path.isAbsolute(input) || parts.some((p) => p === "" || p === "." || p === "..")) {
        throw new Error("Path traversal blocked");
    }
    const { O_RDONLY, O_DIRECTORY, O_NOFOLLOW, O_NONBLOCK } = fs.constants;
    const handles = [fs.openSync(allowedRoot, O_RDONLY | O_DIRECTORY | O_NOFOLLOW)];
    try {
        for (const name of parts.slice(0, -1)) {
            handles.push(fs.openSync(`/proc/self/fd/${handles.at(-1)}/${name}`, O_RDONLY | O_DIRECTORY | O_NOFOLLOW));
        }
        const target = `/proc/self/fd/${handles.at(-1)}/${parts.at(-1)}`;
        handles.push(fs.openSync(target, O_RDONLY | O_NOFOLLOW | O_NONBLOCK));
        const info = fs.fstatSync(handles.at(-1));
        if (!info.isFile() || info.nlink !== 1) {
            throw new Error("Path traversal blocked: not a regular file with one link");
        }
        return fs.readFileSync(handles.at(-1));
    } catch (error) {
        if (error.code === "ELOOP" || error.code === "ENOTDIR" || error.code === "ENXIO") {
            throw new Error("Path traversal blocked");
        }
        throw error;
    } finally {
        handles.reverse().forEach((handle) => fs.closeSync(handle));
    }
}
```

On macOS with Node, use `safePath` for reads only if the [start-up check](#when-to-refuse-writes) passes and the
documented deployment rules out every other writer. If either condition fails, leave the read tools unregistered and
report that reads are off because the root cannot be guaranteed immutable. Windows cannot pass the POSIX ownership and
mode check, so the server registers no read tools there. Report that the platform lacks the required checks.

## Safe writes inside a root

`safePath` returns a string, and the caller writes to that string later. Between the check and the write, a process that
can change the workspace can replace the final name, or a directory on the way to it, with a symbolic link. The write
then lands outside the root. The gap applies to a new file such as `dir/new.txt` and to an existing file alike.
Resolving the path again does not close it, because the same gap sits between that check and the next call.

Make the check and the write one operation on a handle:

- Reject an absolute path, an empty segment, `.`, and `..` before any file system call.
- Open the root directory once. Open each directory on the way with `O_DIRECTORY | O_NOFOLLOW`, relative to the handle
  of the one before it (`openat`). A link at any step fails the open and is not followed.
- Open the final name relative to the last directory handle with `O_NOFOLLOW` and `O_NONBLOCK`, and without `O_TRUNC`. A
  tool that must not replace an existing file adds `O_EXCL`. Without `O_NONBLOCK`, opening a FIFO that has no reader
  blocks the server before `fstat` can refuse it. A regular file ignores the flag.
- Read the descriptor with `fstat` before it is changed. Refuse anything that is not a regular file, and refuse a file
  with more than one link. Only then truncate the file and write to it.
- Write to the descriptor that the open returned, until every byte is written. A write call can accept fewer bytes than
  it was given, so loop on the count it returns. Never reopen the path.
- Close every handle in a `finally` block.

A FIFO placed inside the root fails the open with `ENXIO` when nothing reads it, and passes the open when something
does. Treat `ENXIO` as a refusal, and let the `fstat` check refuse the second case.

A hard link that someone placed inside the root still reaches a file outside it, and `O_NOFOLLOW` does not stop that.
`O_EXCL` stops it for a new file. For an existing file, the link count refuses it, and `O_TRUNC` in the open would
destroy the outside file before any check could run.

Python has `openat` through `dir_fd`. This form works on Linux and macOS:

```python
import errno
import os
import stat


def write_inside(allowed_root, user_path, data):
    parts = user_path.replace("\\", "/").split("/")
    if os.path.isabs(user_path) or any(p in ("", ".", "..") for p in parts):
        raise PermissionError("Path traversal blocked")
    handles = [os.open(allowed_root, os.O_RDONLY | os.O_DIRECTORY)]
    try:
        for name in parts[:-1]:
            handles.append(os.open(name, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=handles[-1]))
        handles.append(os.open(parts[-1], os.O_WRONLY | os.O_CREAT | os.O_NOFOLLOW | os.O_NONBLOCK, 0o600, dir_fd=handles[-1]))
        info = os.fstat(handles[-1])
        if not stat.S_ISREG(info.st_mode) or info.st_nlink > 1:
            raise PermissionError("Path traversal blocked: not a regular file with one link")
        os.ftruncate(handles[-1], 0)
        view = memoryview(data)
        while view:
            view = view[os.write(handles[-1], view) :]
    except OSError as error:
        if error.errno in (errno.ELOOP, errno.ENOTDIR, errno.ENXIO):
            raise PermissionError("Path traversal blocked") from error
        raise
    finally:
        for handle in handles:
            os.close(handle)
```

Node has no `openat`. On Linux, `/proc/self/fd/<n>` names an open handle, so a path built on it resolves from the handle
and not from the root's name. This TypeScript form works on Linux only:

```javascript
function writeInside(allowedRoot, input, data) {
    const parts = input.split(/[\\/]/);
    if (path.isAbsolute(input) || parts.some((p) => p === "" || p === "." || p === "..")) {
        throw new Error("Path traversal blocked");
    }
    const { O_RDONLY, O_DIRECTORY, O_NOFOLLOW, O_NONBLOCK, O_WRONLY, O_CREAT } = fs.constants;
    const handles = [fs.openSync(allowedRoot, O_RDONLY | O_DIRECTORY)];
    try {
        for (const name of parts.slice(0, -1)) {
            handles.push(fs.openSync(`/proc/self/fd/${handles.at(-1)}/${name}`, O_RDONLY | O_DIRECTORY | O_NOFOLLOW));
        }
        const target = `/proc/self/fd/${handles.at(-1)}/${parts.at(-1)}`;
        handles.push(fs.openSync(target, O_WRONLY | O_CREAT | O_NOFOLLOW | O_NONBLOCK, 0o600));
        const info = fs.fstatSync(handles.at(-1));
        if (!info.isFile() || info.nlink > 1) {
            throw new Error("Path traversal blocked: not a regular file with one link");
        }
        fs.ftruncateSync(handles.at(-1), 0);
        fs.writeFileSync(handles.at(-1), data);
    } catch (error) {
        if (error.code === "ELOOP" || error.code === "ENOTDIR" || error.code === "ENXIO") {
            throw new Error("Path traversal blocked");
        }
        throw error;
    } finally {
        handles.forEach((handle) => fs.closeSync(handle));
    }
}
```

### When to refuse writes

Refuse writes when the server cannot use one of the forms above, or when another process can change the workspace.
Refuse in these cases:

- The server runs on macOS in Node. Node has no race-free way to open a path inside a root there. Use the Python form
  for writes. In Node, register read tools only under the immutable-root conditions in
  [Safe reads inside a root](#safe-reads-inside-a-root).
- The server runs on Windows. Neither form works there: the Python form needs `O_DIRECTORY`, `O_NOFOLLOW` and
  descriptor-relative opens, and Node has no race-free open. Writes require a Windows-specific safe-write
  implementation. Leave read tools unregistered because Windows cannot pass the POSIX check.
- Another user, a sync client, a build tool, or an extracted archive can write inside the root.
- The root is on a network share, where link and rename behavior is not the local file system's.

Check this once at start-up: the root is owned by the server's user, no group or other write bit is set, and no
directory on the way is writable by anyone else. When the check fails, refuse writes and say why in the tool's error:
"Writes are off: the workspace can be changed by another process." Follow
[Safe reads inside a root](#safe-reads-inside-a-root) to decide which reads are available.

## Command Injection

If the server spawns processes, never interpolate user input into shell strings.

| Avoid                                 | Use                                                     |
| ------------------------------------- | ------------------------------------------------------- |
| `exec(\`grep \${pattern} \${file}\`)` | `spawn('grep', [pattern, file])` (array form, no shell) |
| `child_process.exec(...)`             | `child_process.spawn(...)` or `execFile(...)`           |
| `os.system(...)` (Python)             | `subprocess.run([...], shell=False)`                    |

For a tool that must accept command-like input (e.g., a SQL tool), parameterize the query rather than concatenate it.

## Input Sanitization

Validate every parameter against its schema before use.

- Strings: length limits, character class allowlists, regex patterns
- Integers: min/max bounds, reject NaN/Infinity
- File uploads: size limits, MIME type checks, scan for embedded executables
- URLs: scheme allowlist (https only?), host allowlist if the server makes outbound requests

## Secrets Handling

- Read API keys, tokens, and credentials from env vars or user_config (with `sensitive: true`)
- Never log secrets. Sanitize log output before you write it.
- Never include secrets in tool error messages
- Never write secrets to disk except where the user explicitly configured (e.g., a credentials file path)

```javascript
// Sanitize before logging
function sanitize(obj) {
    const SECRET_KEYS = /api_key|token|password|secret/i;
    return JSON.stringify(obj, (k, v) => (SECRET_KEYS.test(k) ? "[REDACTED]" : v));
}
```

## Least Privilege

- Declare only the file system access the server actually needs
- If the server only touches one subdirectory, don't request `${HOME}` access
- For network tools, use specific upstream URLs in user_config, not arbitrary URL parameters
- Where the OS supports it, run subprocesses with reduced privileges

## Outbound Network

- When the server wraps a known API, allowlist outbound hosts
- For user-supplied URLs, reject private IP ranges (`10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `127.0.0.0/8`,
  `169.254.0.0/16`) to prevent SSRF
- Validate TLS certificates. Never disable cert validation.
- Set sensible timeouts on every outbound request

## Logging

- Log enough to debug: tool name, anonymous parameters, error category, duration
- Never log full parameter contents (they may contain secrets or user data)
- Never log response bodies (they may contain user data from the upstream system)
- Rotate the log file, or cap its size

## MCPB-Specific Hardening

- Pin every dependency to an exact version in `package.json` / `requirements.txt`
- Audit dependencies before bundling (`npm audit`, `pip-audit`)
- Verify checksums of any downloaded runtime components
- Sign the bundle for public distribution
- Document the threat model in the README: what the server can and cannot do

## Test the Hardening

For each item in this checklist, write at least one test that asserts the safe behavior:

- Path traversal test: send a path with parent-directory segments. Verify the rejection.
- Symbolic-link swap test: replace a directory on the path, then the final name, with a link to a directory outside the
  root, once before the write and once between the check and the write. Verify that nothing appears outside the root.
  Run the same test against a copy of the helper with `O_NOFOLLOW` removed, and verify that it fails: a swap test that
  passes without the guard proves nothing.
- Confined-read test: verify that a regular file can be read and that symbolic-link substitutions and files with
  multiple hard links are refused before any bytes are read.
- Command injection test: send shell metacharacters as input. Verify that they never reach a shell.
- Secret leak test: trigger an error. Verify that secrets don't appear in the message.
- Allowlist bypass test (if SSRF protection is in place): try to fetch from `127.0.0.1`. Verify the rejection.

## Anti-Patterns

- "We'll add validation later": it never happens, and the server ships with the holes
- Trust in input from the LLM because "the LLM is on our side": the LLM faithfully passes user input. User input is
  untrusted.
- A single try/catch around the whole tool handler → silent failures and lost errors. Catch specific exceptions per
  call.
- A catch of `Error` that returns a generic "something failed" message → masks security-relevant failures from logging
