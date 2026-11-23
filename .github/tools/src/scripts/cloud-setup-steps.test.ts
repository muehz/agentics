import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import test from "node:test";
import {
  npmFloor,
  runCloudSetup,
  systemDeps,
  type CloudSetupDeps,
  type RunResult,
} from "./cloud-setup-steps.ts";

interface Options {
  npmVersion?: string;
  npmAfterInstall?: string;
  present?: string[];
  uid?: number;
  failures?: string[];
  env?: NodeJS.ProcessEnv;
  packageJson?: string;
}

function fixture(options: Options = {}) {
  const root = fs.mkdtempSync(path.join(os.tmpdir(), "cloud-setup-steps-"));
  fs.writeFileSync(
    path.join(root, "package.json"),
    options.packageJson ?? '{"engines":{"npm":">=12.2.1"}}',
  );
  const events: string[] = [];
  const links: string[] = [];
  let stdout = "";
  let stderr = "";
  let installed = false;
  const present = new Set(
    options.present ?? ["npm", "node", "go", "make", "shellcheck", "jq", "gh"],
  );
  const failures = new Set(options.failures ?? []);
  const env: NodeJS.ProcessEnv = {
    PATH: "/bin",
    AGX_TOOLS_DIR: "/tools",
    AGX_BIN_DIR: "/bin-dir",
    AGX_ORIGINAL_PATH: "/usr/bin:/bin-dir",
    ...options.env,
  };
  const deps: CloudSetupDeps = {
    env,
    repoRoot: root,
    run: (command, args): RunResult => {
      const line = [command, ...args].join(" ");
      if (command === "npm" && args[0] === "--version")
        return {
          status: 0,
          stdout:
            (installed ? options.npmAfterInstall : undefined) ??
            options.npmVersion ??
            "12.2.1",
        };
      if (command === "node" && args[0] === "--version")
        return { status: 0, stdout: "v26.12.1\n" };
      if (command === "go" && args[0] === "version")
        return { status: 0, stdout: "go version go1.27.4 linux/amd64\n" };
      events.push(line);
      if (command === "npm" && args[0] === "install") installed = true;
      if (command === "apt-get" && args[0] === "install") {
        const tool = args[args.length - 1] ?? "";
        if (!failures.has(line)) present.add(tool);
      }
      return { status: failures.has(line) ? 1 : 0, stdout: "" };
    },
    which: (name) => (present.has(name) ? `/bin/${name}` : undefined),
    link: (target, linkPath) => links.push(`${target} -> ${linkPath}`),
    uid: () => options.uid ?? 1000,
    out: (text) => {
      stdout += text;
    },
    err: (text) => {
      stderr += text;
    },
  };
  return {
    deps,
    events,
    links,
    stdout: () => stdout,
    stderr: () => stderr,
    cleanup: () => fs.rmSync(root, { recursive: true, force: true }),
  };
}

const SETUP = "node .github/tools/src/scripts/setup-worktree.ts";
const GO_TEST = "go test -C hooks/go ./...";

test("runs setup then the go test and prints the ready summary", (t) => {
  const f = fixture();
  t.after(f.cleanup);
  assert.equal(runCloudSetup([], f.deps), 0);
  assert.deepEqual(f.events, [SETUP, GO_TEST]);
  assert.equal(
    f.stdout(),
    [
      "==> Checking npm",
      "npm 12.2.1 satisfies engines.npm (>=12.2.1).",
      "==> Running setup-worktree.ts (npm ci, Go modules, Git hooks)",
      "==> Smoke test: go test -C hooks/go ./...",
      "==> Ready",
      "go:        go version go1.27.4 linux/amd64",
      "node:      v26.12.1",
      "npm:       12.2.1",
      "tool dir:  /bin-dir",
      "",
    ].join("\n"),
  );
  assert.equal(f.stderr(), "");
});

test("--skip-tests drops the go test and says so", (t) => {
  const f = fixture();
  t.after(f.cleanup);
  assert.equal(runCloudSetup(["--skip-tests"], f.deps), 0);
  assert.deepEqual(f.events, [SETUP]);
  assert.match(f.stdout(), /^Skipped the smoke test \(--skip-tests\)\.$/m);
  assert.doesNotMatch(f.stdout(), /Smoke test: go test/);
});

test("an old npm is installed into the tool directory and linked into the bin directory", (t) => {
  const f = fixture({ npmVersion: "11.1.0", npmAfterInstall: "12.2.1" });
  t.after(f.cleanup);
  assert.equal(runCloudSetup(["--skip-tests"], f.deps), 0);
  assert.deepEqual(f.events, [
    "npm install --prefix /tools/npm --ignore-scripts --no-audit --no-fund npm@12.2.1",
    SETUP,
  ]);
  assert.deepEqual(f.links, [
    "/tools/npm/node_modules/.bin/npm -> /bin-dir/npm",
    "/tools/npm/node_modules/.bin/npx -> /bin-dir/npx",
  ]);
  assert.match(
    f.stderr(),
    /^cloud-setup: WARNING: npm 11\.1\.0 is older than the required 12\.2\.1\.$/m,
  );
  assert.match(f.stdout(), /^==> Installing npm 12\.2\.1$/m);
});

test("a failed npm install stops before setup", (t) => {
  const f = fixture({
    npmVersion: "11.1.0",
    failures: [
      "npm install --prefix /tools/npm --ignore-scripts --no-audit --no-fund npm@12.2.1",
    ],
  });
  t.after(f.cleanup);
  assert.equal(runCloudSetup([], f.deps), 1);
  assert.match(
    f.stderr(),
    /cloud-setup: ERROR: cannot install npm 12\.2\.1\. Check access to registry\.npmjs\.org\.\n$/,
  );
  assert.equal(f.events.includes(SETUP), false);
  assert.deepEqual(f.links, []);
});

test("an npm still too old after the install fails and names the bin directory", (t) => {
  const f = fixture({ npmVersion: "11.1.0", npmAfterInstall: "11.1.0" });
  t.after(f.cleanup);
  assert.equal(runCloudSetup([], f.deps), 1);
  assert.match(
    f.stderr(),
    /cloud-setup: ERROR: npm 11\.1\.0 is on PATH but package\.json requires >=12\.2\.1\. Put \/bin-dir first on PATH\.\n$/,
  );
  assert.equal(f.events.includes(SETUP), false);
});

test("a missing npm after the install attempt is reported as missing", (t) => {
  const f = fixture({
    present: ["node", "go", "make", "shellcheck", "jq", "gh"],
  });
  t.after(f.cleanup);
  assert.equal(runCloudSetup([], f.deps), 1);
  assert.match(f.stderr(), /ERROR: npm is missing is on PATH but/);
});

test("optional tools are installed with apt-get as root", (t) => {
  const f = fixture({
    uid: 0,
    present: ["npm", "node", "go", "apt-get", "jq"],
  });
  t.after(f.cleanup);
  assert.equal(runCloudSetup(["--skip-tests"], f.deps), 0);
  assert.deepEqual(f.events, [
    "apt-get update -qq",
    "apt-get install -y -qq make",
    "apt-get install -y -qq shellcheck",
    "apt-get install -y -qq gh",
    SETUP,
  ]);
  assert.match(
    f.stdout(),
    /^==> Installing optional tools with apt-get: make shellcheck gh$/m,
  );
  assert.equal(f.stderr(), "");
});

test("failed apt-get installs warn once per tool and setup continues", (t) => {
  const f = fixture({
    uid: 0,
    present: ["npm", "node", "go", "apt-get", "make", "shellcheck", "jq"],
    failures: ["apt-get update -qq", "apt-get install -y -qq gh"],
  });
  t.after(f.cleanup);
  assert.equal(runCloudSetup(["--skip-tests"], f.deps), 0);
  assert.match(f.stderr(), /^cloud-setup: WARNING: apt-get update failed\.$/m);
  assert.match(
    f.stderr(),
    /^cloud-setup: WARNING: could not install gh with apt-get\.$/m,
  );
  assert.match(
    f.stderr(),
    /^cloud-setup: WARNING: gh is not installed\. Some tasks need it: /m,
  );
  assert.equal(f.events.at(-1), SETUP);
});

test("optional tools are skipped with a warning when not root", (t) => {
  const f = fixture({ present: ["npm", "node", "go", "apt-get"] });
  t.after(f.cleanup);
  assert.equal(runCloudSetup(["--skip-tests"], f.deps), 0);
  assert.deepEqual(f.events, [SETUP]);
  for (const [tool, use] of [
    ["make", "make (hook rebuild)"],
    ["shellcheck", "shellcheck (CI repro)"],
    ["jq", "jq (evals workflow)"],
    ["gh", "gh (pull requests)"],
  ]) {
    assert.ok(
      f
        .stderr()
        .includes(
          `cloud-setup: WARNING: ${tool} is not installed. Some tasks need it: make (hook rebuild), shellcheck (CI repro), jq (evals workflow), gh (pull requests).\n`,
        ),
      `${tool} ${use}`,
    );
  }
});

test("root without apt-get warns and does not install", (t) => {
  const f = fixture({
    uid: 0,
    present: ["npm", "node", "go", "make", "shellcheck", "jq"],
  });
  t.after(f.cleanup);
  assert.equal(runCloudSetup(["--skip-tests"], f.deps), 0);
  assert.deepEqual(f.events, [SETUP]);
  assert.match(f.stderr(), /WARNING: gh is not installed/);
});

test("a failing setup-worktree stops before the go test", (t) => {
  const f = fixture({ failures: [SETUP] });
  t.after(f.cleanup);
  assert.equal(runCloudSetup([], f.deps), 1);
  assert.deepEqual(f.events, [SETUP]);
  assert.equal(
    f.stderr(),
    "cloud-setup: ERROR: setup-worktree.ts failed. Read its message above.\n",
  );
  assert.doesNotMatch(f.stdout(), /Ready/);
});

test("a failing go test fails with the smoke test message", (t) => {
  const f = fixture({ failures: [GO_TEST] });
  t.after(f.cleanup);
  assert.equal(runCloudSetup([], f.deps), 1);
  assert.equal(
    f.stderr(),
    "cloud-setup: ERROR: hooks/go tests failed. Read the output above.\n",
  );
  assert.doesNotMatch(f.stdout(), /Ready/);
});

test("the ready summary warns when the bin directory was not on the original PATH", (t) => {
  const f = fixture({ env: { AGX_ORIGINAL_PATH: "/usr/bin:/bin" } });
  t.after(f.cleanup);
  assert.equal(runCloudSetup(["--skip-tests"], f.deps), 0);
  assert.equal(
    f.stderr(),
    "cloud-setup: WARNING: /bin-dir was not on PATH when this script started. Add it to PATH in each new shell.\n",
  );
});

test("a bin directory that only prefixes a PATH entry still warns", (t) => {
  const f = fixture({ env: { AGX_ORIGINAL_PATH: "/bin-dir2:/usr/bin" } });
  t.after(f.cleanup);
  assert.equal(runCloudSetup(["--skip-tests"], f.deps), 0);
  assert.match(f.stderr(), /\/bin-dir was not on PATH/);
});

test("an unknown argument fails before any step", (t) => {
  const f = fixture();
  t.after(f.cleanup);
  assert.equal(runCloudSetup(["--bogus"], f.deps), 1);
  assert.equal(f.stderr(), "cloud-setup: ERROR: unknown option: --bogus\n");
  assert.equal(f.stdout(), "");
  assert.deepEqual(f.events, []);
});

for (const name of ["AGX_TOOLS_DIR", "AGX_BIN_DIR", "AGX_ORIGINAL_PATH"]) {
  test(`${name} unset or empty fails before any step`, (t) => {
    for (const value of [undefined, ""]) {
      const f = fixture({ env: { [name]: value } });
      t.after(f.cleanup);
      assert.equal(runCloudSetup([], f.deps), 1);
      assert.equal(
        f.stderr(),
        `cloud-setup: ERROR: ${name} is not set. Run .github/scripts/cloud-setup.sh.\n`,
      );
      assert.equal(f.stdout(), "");
      assert.deepEqual(f.events, []);
    }
  });
}

test("an engines.npm that is not a single floor fails", (t) => {
  const f = fixture({ packageJson: '{"engines":{"npm":"^12.0.0"}}' });
  t.after(f.cleanup);
  assert.equal(runCloudSetup([], f.deps), 1);
  assert.match(
    f.stderr(),
    /ERROR: package\.json engines\.npm is not in the form ">=X\.Y\.Z"\./,
  );
});

test("npmFloor reads the floor and rejects other forms", () => {
  assert.equal(npmFloor('{"engines":{"npm":">=12.2.1"}}'), "12.2.1");
  assert.throws(() => npmFloor("{}"), /not in the form/);
  assert.throws(() => npmFloor("not json"), /not in the form/);
  assert.throws(
    () => npmFloor('{"engines":{"npm":">=12.2"}}'),
    /not in the form/,
  );
});

test("an unexpected error prints one error line and exits 1", (t) => {
  const f = fixture();
  t.after(f.cleanup);
  f.deps.run = () => {
    throw new Error("spawn exploded");
  };
  assert.equal(runCloudSetup([], f.deps), 1);
  assert.equal(f.stderr(), "cloud-setup: ERROR: spawn exploded\n");
});

test("an npm version that is not a plain X.Y.Z counts as below the floor", (t) => {
  for (const npmVersion of ["12.2", "garbage", "12.2.1-pre"]) {
    const f = fixture({ npmVersion, npmAfterInstall: "12.2.1" });
    t.after(f.cleanup);
    assert.equal(runCloudSetup(["--skip-tests"], f.deps), 0);
    assert.match(f.events[0] ?? "", /^npm install /, npmVersion);
  }
});

test("npm 12.10.0 satisfies a 12.2.1 floor numerically", (t) => {
  const f = fixture({ npmVersion: "12.10.0" });
  t.after(f.cleanup);
  assert.equal(runCloudSetup(["--skip-tests"], f.deps), 0);
  assert.deepEqual(f.events, [SETUP]);
});

test(
  "the system lookup finds regular executable files only",
  {
    skip: process.platform === "win32" && "POSIX only: executable bits",
  },
  (t) => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "cloud-setup-which-"));
    t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
    fs.writeFileSync(path.join(dir, "tool"), "#!/bin/sh\n", { mode: 0o755 });
    fs.writeFileSync(path.join(dir, "plain"), "x", { mode: 0o644 });
    fs.mkdirSync(path.join(dir, "folder"), { mode: 0o755 });
    const env = { PATH: dir };
    const deps = systemDeps(env, dir);
    assert.equal(deps.which("tool", env), path.join(dir, "tool"));
    assert.equal(deps.which("plain", env), undefined);
    assert.equal(deps.which("folder", env), undefined);
    assert.equal(deps.which("missing", env), undefined);
  },
);

test("the entry file runs unconditionally and reports a missing hand-off variable", () => {
  const env = { ...process.env };
  for (const name of ["AGX_TOOLS_DIR", "AGX_BIN_DIR", "AGX_ORIGINAL_PATH"])
    delete env[name];
  const result = spawnSync(
    process.execPath,
    [path.join(import.meta.dirname, "cloud-setup.ts")],
    { env, encoding: "utf8" },
  );
  assert.equal(result.status, 1);
  assert.equal(
    result.stderr,
    "cloud-setup: ERROR: AGX_TOOLS_DIR is not set. Run .github/scripts/cloud-setup.sh.\n",
  );
});

test(
  "the system link replaces an existing link",
  { skip: process.platform === "win32" && "POSIX only: symbolic links" },
  (t) => {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), "cloud-setup-link-"));
    t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
    const deps = systemDeps({}, dir);
    deps.link("/first", path.join(dir, "bin", "npm"));
    deps.link("/second", path.join(dir, "bin", "npm"));
    assert.equal(fs.readlinkSync(path.join(dir, "bin", "npm")), "/second");
  },
);
