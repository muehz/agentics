import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test, { type TestContext } from "node:test";
import {
  findInterpreter,
  requireInterpreter,
} from "../entrypoint/interpreters.ts";

const source = path.resolve(import.meta.dirname, "../../../..");
const skipBash = requireInterpreter("bash");
const bash = findInterpreter("bash") ?? "bash";
const posixPaths = new Map<string, string>();
function posix(value: string): string {
  if (process.platform !== "win32") return value;
  const cached = posixPaths.get(value);
  if (cached) return cached;
  // Git Bash mounts the Windows temporary directory at /tmp; use its mapping
  // rather than assuming that /c exposes the same sandbox path.
  const result = spawnSync(
    bash,
    [
      "--noprofile",
      "--norc",
      "-c",
      'PATH=/usr/bin:/bin cygpath -u "$1"',
      "fixture-path",
      value,
    ],
    { encoding: "utf8", timeout: 10_000 },
  );
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr);
  const converted = result.stdout.trim();
  assert.ok(converted);
  posixPaths.set(value, converted);
  return converted;
}
const quote = (value: string): string => `'${value.replaceAll("'", "'\\''")}'`;
const coreCommands = [
  "awk",
  "sed",
  "head",
  "dirname",
  "basename",
  "mkdir",
  "mktemp",
  "rm",
  "mv",
  "cp",
  "chmod",
  "cat",
];
let corePaths: Map<string, string> | undefined;

function write(root: string, name: string, body: string): void {
  const file = path.join(root, name);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, body, { mode: 0o755 });
}

function commands(): Map<string, string> {
  if (corePaths) return corePaths;
  corePaths = new Map();
  for (const name of coreCommands) {
    const result = spawnSync(
      bash,
      ["--noprofile", "--norc", "-c", `command -v ${name}`],
      {
        encoding: "utf8",
        timeout: 10_000,
        env: { ...process.env, PATH: "/usr/bin:/bin" },
      },
    );
    assert.ifError(result.error);
    assert.equal(result.status, 0, `${name}: ${result.stderr}`);
    corePaths.set(name, result.stdout.trim());
  }
  return corePaths;
}

function fixture(t: TestContext, version: string | undefined) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "agentics-cloud-node-"));
  t.after(() => fs.rmSync(root, { recursive: true, force: true }));
  for (const name of [
    ".github/scripts/cloud-setup.sh",
    ".github/tools/src/scripts/node-range.ts",
  ]) {
    const target = path.join(root, name);
    fs.mkdirSync(path.dirname(target), { recursive: true });
    fs.copyFileSync(path.join(source, name), target);
  }
  write(
    root,
    "package.json",
    JSON.stringify(
      {
        engines: { node: "^22.23.5 || ^24.22.3 || >=26.12.1", npm: ">=12.2.1" },
      },
      null,
      2,
    ),
  );
  write(root, ".github/tools/package.json", '{"type":"module"}');
  write(root, "hooks/go/go.mod", "module fixture\n\ngo 1.27.4\n");
  write(root, "node-version", version ?? "");
  write(root, "events", "");
  for (const [name, executable] of commands())
    write(root, `bin/${name}`, `#!/bin/bash\nexec ${quote(executable)} "$@"\n`);
  const node = `#!/bin/bash
set -eu
version="$(cat "$CASE_ROOT/node-version")"
if [ "\${1:-}" = --version ]; then printf 'v%s\\n' "$version"; exit 0; fi
case "\${1:-}" in
  */node-range.ts)
    printf 'range:%s\\n' "$version" >> "$CASE_ROOT/events"
    case "$version" in 2[01].*|22.[0-9].*|22.1[0-7].*|23.*|24.[01].*|not-a-version|'') printf 'UNSAFE\\n' >> "$CASE_ROOT/events"; exit 0;; esac
    exec ${quote(posix(process.execPath))} "$@"
    ;;
  .github/tools/src/scripts/cloud-setup.ts)
    shift
    printf 'handoff\\n' >> "$CASE_ROOT/events"
    {
      printf 'args:%s\\n' "$*"
      printf 'PATH:%s\\n' "$PATH"
      printf 'TOOLS:%s\\n' "$AGX_TOOLS_DIR"
      printf 'BIN:%s\\n' "$AGX_BIN_DIR"
      printf 'ORIGINAL:%s\\n' "$AGX_ORIGINAL_PATH"
    } > "$CASE_ROOT/handoff"
    if [ -f "$CASE_ROOT/handoff-status" ]; then
      printf 'child output\\n' >&2
      exit "$(cat "$CASE_ROOT/handoff-status")"
    fi
    exit 0;;
  *) printf 'unexpected node command: %s\\n' "$*" >&2; exit 91;;
esac
`;
  write(root, "fake-node", node);
  if (version !== undefined) write(root, "bin/node", node);
  write(root, "fake-npm", "#!/bin/bash\nprintf '12.2.1\\n'\n");
  write(root, "bin/npm", "#!/bin/bash\nprintf '12.2.1\\n'\n");
  write(root, "bin/git", "#!/bin/bash\nprintf 'true\\n'\n");
  write(
    root,
    "bin/go",
    "#!/bin/bash\nif [ \"\${1:-}\" = env ]; then printf 'go1.27.4\\n'; else printf 'go version go1.27.4 linux/amd64\\n'; fi\n",
  );
  write(
    root,
    "bin/uname",
    '#!/bin/bash\nif [ "$1" = -s ]; then echo Linux; else echo x86_64; fi\n',
  );
  write(
    root,
    "bin/sha256sum",
    "#!/bin/bash\nprintf '6170b2d474e1c626f43cff3adc118df9fe3b573258a4fc3d850c8dbc6b079bbd  %s\\n' \"$1\"\n",
  );
  write(
    root,
    "bin/curl",
    `#!/bin/bash
set -eu
printf 'download\\n' >> "$CASE_ROOT/events"
while [ "$#" -gt 0 ]; do
  if [ "$1" = --output ]; then shift; : > "$1"; exit 0; fi
  shift
done
exit 92
`,
  );
  write(
    root,
    "bin/tar",
    `#!/bin/bash
set -eu
while [ "$#" -gt 0 ]; do
  if [ "$1" = -C ]; then shift; destination="$1"; break; fi
  shift
done
target="$destination/node-v26.12.1-linux-x64/bin"
mkdir -p "$target"
cp "$CASE_ROOT/fake-node" "$target/node"
cp "$CASE_ROOT/fake-npm" "$target/npm"
cp "$CASE_ROOT/fake-npm" "$target/npx"
chmod +x "$target/node" "$target/npm" "$target/npx"
printf '26.12.1\\n' >"$CASE_ROOT/node-version"
printf 'install\\n' >> "$CASE_ROOT/events"
`,
  );
  write(root, "bin/ln", '#!/bin/bash\ncp "$2" "$3"\n');
  const env: NodeJS.ProcessEnv = {
    ...process.env,
    CASE_ROOT: posix(root),
    AGX_TOOLS_DIR: posix(path.join(root, "tools-installed")),
    AGX_BIN_DIR: posix(path.join(root, "bin")),
    PATH: posix(path.join(root, "bin")),
    AGX_ORIGINAL_PATH: "sentinel-original-path",
  };
  return { root, env };
}

type Fixture = ReturnType<typeof fixture>;

function runBootstrap(f: Fixture, args: string[]) {
  const result = spawnSync(
    bash,
    [
      "--noprofile",
      "--norc",
      posix(path.join(f.root, ".github/scripts/cloud-setup.sh")),
      ...args,
    ],
    { cwd: f.root, env: f.env, encoding: "utf8", timeout: 20_000 },
  );
  assert.ifError(result.error);
  return result;
}

function readEvents(f: Fixture): string[] {
  return fs
    .readFileSync(path.join(f.root, "events"), "utf8")
    .split("\n")
    .filter((line) => line !== "");
}

function readHandoff(f: Fixture): Map<string, string> {
  const recorded = new Map<string, string>();
  const text = fs.readFileSync(path.join(f.root, "handoff"), "utf8");
  for (const line of text.split("\n")) {
    const at = line.indexOf(":");
    if (at > 0) recorded.set(line.slice(0, at), line.slice(at + 1));
  }
  return recorded;
}

for (const version of [
  undefined,
  "not-a-version",
  "20.19.0",
  "22.17.0",
  "22.18.0",
  "22.23.5",
  "23.5.0",
  "24.0.0",
  "24.1.0",
  "24.2.0",
  "24.22.0",
  "24.22.3",
  "25.0.0",
  "26.7.0",
  "26.12.1",
]) {
  test(
    `cloud bootstrap safely handles Node ${version ?? "missing"}`,
    { skip: skipBash },
    (t) => {
      const f = fixture(t, version);
      const result = runBootstrap(f, ["--skip-tests"]);
      assert.equal(result.status, 0, result.stdout + result.stderr);
      const events = readEvents(f);
      assert.ok(
        !events.includes("UNSAFE"),
        `TypeScript helper executed on an unsupported runtime: ${events.join(",")}`,
      );
      const accepted = ["22.23.5", "24.22.3", "26.12.1"].includes(
        version ?? "",
      );
      assert.equal(events.includes("install"), !accepted, events.join(","));
      assert.equal(events.at(-1), "handoff");
      if (
        version === undefined ||
        version === "not-a-version" ||
        /^(?:2[01]|22\.(?:\d|1[0-7])|23|24\.[01])\./.test(version)
      ) {
        assert.ok(
          events.indexOf("install") <
            events.findIndex((event) => event.startsWith("range:")),
          events.join(","),
        );
      } else {
        // A runtime that can load the helper is asked before any install.
        assert.equal(events[0], `range:${version}`, events.join(","));
      }
      assert.equal(fs.existsSync(path.join(f.root, "node_modules")), false);
    },
  );
}

for (const flag of ["--help", "-h"]) {
  test(
    `${flag} prints usage and exits 0 before anything happens`,
    { skip: skipBash },
    (t) => {
      const f = fixture(t, undefined);
      const result = runBootstrap(f, [flag]);
      assert.equal(result.status, 0, result.stderr);
      assert.ok(result.stdout.startsWith("Usage:"), result.stdout);
      assert.deepEqual(readEvents(f), []);
      assert.equal(fs.existsSync(path.join(f.root, "tools-installed")), false);
    },
  );
}

test(
  "an unknown option prints usage then the error and exits 1 before anything happens",
  { skip: skipBash },
  (t) => {
    const f = fixture(t, undefined);
    const result = runBootstrap(f, ["--bogus"]);
    assert.equal(result.status, 1, result.stdout);
    assert.match(
      result.stderr,
      /^Usage:[\s\S]*\ncloud-setup: ERROR: unknown option: --bogus\n$/,
    );
    assert.deepEqual(readEvents(f), []);
    assert.equal(fs.existsSync(path.join(f.root, "tools-installed")), false);
  },
);

test(
  "the hand-off passes --skip-tests and the three variables",
  { skip: skipBash },
  (t) => {
    const f = fixture(t, "26.12.1");
    const result = runBootstrap(f, ["--skip-tests"]);
    assert.equal(result.status, 0, result.stdout + result.stderr);
    const recorded = readHandoff(f);
    assert.equal(recorded.get("args"), "--skip-tests");
    assert.equal(recorded.get("ORIGINAL"), f.env.PATH);
    assert.notEqual(recorded.get("ORIGINAL"), "sentinel-original-path");
    const bin = recorded.get("BIN") ?? "";
    const tools = recorded.get("TOOLS") ?? "";
    assert.ok(bin.startsWith("/") && bin.endsWith("/bin"), bin);
    assert.ok(
      tools.startsWith("/") && tools.endsWith("/tools-installed"),
      tools,
    );
    assert.equal(bin, f.env.AGX_BIN_DIR);
    assert.equal(tools, f.env.AGX_TOOLS_DIR);
    assert.ok((recorded.get("PATH") ?? "").startsWith(`${bin}:`));
  },
);

test(
  "the hand-off passes no arguments without --skip-tests",
  { skip: skipBash },
  (t) => {
    const f = fixture(t, "26.12.1");
    const result = runBootstrap(f, []);
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.equal(readHandoff(f).get("args"), "");
  },
);

test(
  "a failing hand-off exits with the child status and adds no error line",
  { skip: skipBash },
  (t) => {
    const f = fixture(t, "26.12.1");
    fs.writeFileSync(path.join(f.root, "handoff-status"), "7");
    const result = runBootstrap(f, []);
    assert.equal(result.status, 7);
    assert.ok(result.stderr.endsWith("child output\n"), result.stderr);
    assert.doesNotMatch(result.stderr, /cloud-setup: ERROR/);
  },
);

test(
  "the work directory is removed after the hand-off",
  { skip: skipBash },
  (t) => {
    const f = fixture(t, "26.12.1");
    const tmp = path.join(f.root, "tmp");
    fs.mkdirSync(tmp);
    f.env.TMPDIR = posix(tmp);
    const result = runBootstrap(f, ["--skip-tests"]);
    assert.equal(result.status, 0, result.stdout + result.stderr);
    assert.equal(readEvents(f).at(-1), "handoff");
    assert.deepEqual(
      fs
        .readdirSync(tmp)
        .filter((name) => name.startsWith("agentics-cloud-setup.")),
      [],
    );
  },
);
