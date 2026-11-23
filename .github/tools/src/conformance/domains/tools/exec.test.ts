import assert from "node:assert/strict";
import {
  chmodSync,
  mkdirSync,
  mkdtempSync,
  renameSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import {
  describeChildFailure,
  findOnPath,
  realExec,
  splitPathList,
  tail,
} from "./exec.ts";

test("describeChildFailure renders a nonzero exit", () => {
  assert.equal(
    describeChildFailure("git", { code: 128 }),
    "git exited with code 128",
  );
});

test("describeChildFailure renders a signal", () => {
  assert.equal(
    describeChildFailure("hook audit", { signal: "SIGKILL" }),
    "hook audit was killed by SIGKILL",
  );
});

test("describeChildFailure renders a spawn failure", () => {
  assert.equal(
    describeChildFailure("evals check", { startError: "spawn node ENOENT" }),
    "evals check could not start: spawn node ENOENT",
  );
});

test("realExec returns the exit code and the merged output", async () => {
  const result = await realExec({
    dir: process.cwd(),
    bin: process.execPath,
    args: ["-e", "console.log('out'); console.error('err'); process.exit(3)"],
    env: process.env,
  });
  assert.equal(result.code, 3);
  assert.match(result.output, /out/);
  assert.match(result.output, /err/);
  assert.equal(result.spawnError, undefined);
});

test("realExec keeps the order in which stdout and stderr were written", async () => {
  const script = [
    "const fs = require('node:fs');",
    "for (let i = 0; i < 50; i++) {",
    "  fs.writeSync(1, 'o' + i + '\\n');",
    "  fs.writeSync(2, 'e' + i + '\\n');",
    "}",
  ].join("\n");
  const result = await realExec({
    dir: process.cwd(),
    bin: process.execPath,
    args: ["-e", script],
    env: process.env,
  });
  const expected = Array.from({ length: 50 }, (_, i) => `o${i}\ne${i}\n`).join(
    "",
  );
  assert.equal(result.code, 0);
  assert.equal(result.output, expected);
});

test("realExec keeps a multibyte character intact", async () => {
  const script =
    "require('node:fs').writeSync(1, Buffer.from('é€😀'.repeat(20000)))";
  const result = await realExec({
    dir: process.cwd(),
    bin: process.execPath,
    args: ["-e", script],
    env: process.env,
  });
  assert.equal(result.output, "é€😀".repeat(20000));
});

test("realExec reads back output larger than one read chunk intact", async () => {
  const script =
    "const fs = require('node:fs'); const line = 'x'.repeat(99) + '\\n'; for (let i = 0; i < 5000; i++) fs.writeSync(1, line)";
  const result = await realExec({
    dir: process.cwd(),
    bin: process.execPath,
    args: ["-e", script],
    env: process.env,
  });
  assert.equal(result.code, 0);
  assert.equal(result.output, `${"x".repeat(99)}\n`.repeat(5000));
});

test("realExec returns empty output when the child writes nothing", async () => {
  const result = await realExec({
    dir: process.cwd(),
    bin: process.execPath,
    args: ["-e", "process.exit(0)"],
    env: process.env,
  });
  assert.equal(result.code, 0);
  assert.equal(result.output, "");
});

test("realExec reports a binary that cannot start", async () => {
  const result = await realExec({
    dir: process.cwd(),
    bin: "agentics-no-such-binary-xyz",
    args: [],
    env: process.env,
  });
  assert.equal(result.code, null);
  assert.ok(result.spawnError !== undefined && result.spawnError.length > 0);
});

test("realExec resolves instead of rejecting when spawn throws synchronously", async () => {
  const result = await realExec({
    dir: process.cwd(),
    bin: process.execPath,
    args: ["bad\0arg"],
    env: process.env,
  });
  assert.equal(result.code, null);
  assert.ok(result.spawnError !== undefined && result.spawnError.length > 0);
  assert.equal(result.output, "");
});

test(
  "realExec reports a process killed by a signal",
  {
    skip:
      process.platform === "win32"
        ? "Windows does not report termination signals"
        : false,
  },
  async () => {
    const result = await realExec({
      dir: process.cwd(),
      bin: process.execPath,
      args: ["-e", "process.kill(process.pid, 'SIGKILL')"],
      env: process.env,
    });
    assert.equal(result.code, null);
    assert.equal(result.spawnError, "killed by SIGKILL");
  },
);

test("tail keeps the last n lines and trims trailing newlines", () => {
  assert.equal(tail("a\nb\nc\nd\n\n", 2), "c\nd");
  assert.equal(tail("a\nb", 5), "a\nb");
  assert.equal(tail("", 3), "");
});

test("findOnPath finds a binary on PATH and returns undefined on an empty PATH", () => {
  const nodeDir = path.dirname(process.execPath);
  const found = findOnPath("node", { PATH: nodeDir });
  assert.ok(found !== undefined && found.startsWith(nodeDir));
  const empty = mkdtempSync(path.join(tmpdir(), "agentics-emptypath-"));
  try {
    assert.equal(findOnPath("node", { PATH: empty }), undefined);
  } finally {
    rmSync(empty, { recursive: true, force: true });
  }
});

test("findOnPath returns undefined when PATH is unset or empty", () => {
  assert.equal(findOnPath("node", {}), undefined);
  assert.equal(findOnPath("node", { PATH: "" }), undefined);
});

function withTools(
  body: (made: { first: string; second: string; name: string }) => void,
): void {
  const root = mkdtempSync(path.join(process.cwd(), ".findonpath-"));
  const name = "agentics-fake-tool";
  const file = process.platform === "win32" ? `${name}.exe` : name;
  try {
    const first = path.join(root, "first");
    const second = path.join(root, "second");
    for (const dir of [first, second]) {
      mkdirSync(dir);
      writeFileSync(path.join(dir, file), "", { mode: 0o755 });
    }
    body({ first, second, name });
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

test("findOnPath resolves an absolute PATH entry", () => {
  withTools(({ first, second, name }) => {
    const found = findOnPath(name, {
      PATH: [first, second].join(path.delimiter),
    });
    assert.ok(found !== undefined && path.isAbsolute(found));
    assert.equal(path.dirname(found), first);
  });
});

test("findOnPath rejects a relative PATH entry that holds the tool", () => {
  withTools(({ first, name }) => {
    const relative = path.relative(process.cwd(), first);
    assert.ok(!path.isAbsolute(relative));
    assert.equal(findOnPath(name, { PATH: relative }), undefined);
    assert.equal(
      findOnPath(name, { PATH: `.${path.sep}${relative}` }),
      undefined,
    );
  });
});

test("findOnPath does not fall through a relative match to a later absolute entry", () => {
  withTools(({ first, second, name }) => {
    const relative = path.relative(process.cwd(), first);
    const env = { PATH: [relative, second].join(path.delimiter) };
    assert.equal(findOnPath(name, env), undefined);
  });
});

test("findOnPath fails for an empty PATH entry that resolves to the working directory", () => {
  withTools(({ first, second, name }) => {
    const previous = process.cwd();
    process.chdir(first);
    try {
      const env = { PATH: [path.delimiter, second].join("") };
      assert.equal(findOnPath(name, env), undefined);
    } finally {
      process.chdir(previous);
    }
  });
});

test("findOnPath skips a relative entry that holds nothing and uses a later absolute entry", () => {
  withTools(({ first, name }) => {
    const empty = path.relative(process.cwd(), process.cwd()) || ".";
    const found = findOnPath(name, {
      PATH: [`${empty}${path.sep}no-such-dir`, first].join(path.delimiter),
    });
    assert.ok(found !== undefined);
    assert.equal(path.dirname(found), first);
  });
});

test("splitPathList removes every quote from a Windows entry", () => {
  assert.deepEqual(
    splitPathList('"C:\\Program Files\\Tools\\bin";C:\\Windows', true),
    ["C:\\Program Files\\Tools\\bin", "C:\\Windows"],
  );
  assert.deepEqual(splitPathList('C:\\a"b"c;D:\\d', true), [
    "C:\\abc",
    "D:\\d",
  ]);
});

test("splitPathList keeps a semicolon inside quotes, and an unbalanced quote runs to the end", () => {
  assert.deepEqual(splitPathList('"C:\\a;b";C:\\c', true), [
    "C:\\a;b",
    "C:\\c",
  ]);
  assert.deepEqual(splitPathList('"C:\\a;C:\\b', true), ["C:\\a;C:\\b"]);
});

test("splitPathList keeps empty Windows entries but returns no entry for an empty value", () => {
  assert.deepEqual(splitPathList("", true), []);
  assert.deepEqual(splitPathList(";C:\\a;;", true), ["", "C:\\a", "", ""]);
});

test("splitPathList treats a quote as an ordinary character on POSIX", () => {
  assert.deepEqual(splitPathList('"/a:b":/c', false), ['"/a', 'b"', "/c"]);
  assert.deepEqual(splitPathList("", false), [""]);
});

test(
  "findOnPath resolves a quoted Windows PATH entry",
  { skip: process.platform === "win32" ? false : "Windows PATH syntax" },
  () => {
    withTools(({ first, second, name }) => {
      const found = findOnPath(name, { PATH: `"${first}";"${second}"` });
      assert.ok(found !== undefined && path.isAbsolute(found));
      assert.equal(path.dirname(found), first);
    });
  },
);

test(
  "findOnPath resolves a quoted Windows PATH entry that contains a semicolon",
  { skip: process.platform === "win32" ? false : "Windows PATH syntax" },
  () => {
    withTools(({ first, name }) => {
      const renamed = `${first};semi`;
      renameSync(first, renamed);
      const found = findOnPath(name, { PATH: `"${renamed}"` });
      assert.ok(found !== undefined);
      assert.equal(path.dirname(found), renamed);
    });
  },
);

test("findOnPath honors PATHEXT on Windows and the executable bit on POSIX", () => {
  withTools(({ first, name }) => {
    if (process.platform === "win32") {
      assert.equal(
        findOnPath(name, { PATH: first, PATHEXT: ".cmd" }),
        undefined,
      );
      assert.ok(
        findOnPath(name, { PATH: first, PATHEXT: ".EXE" }) !== undefined,
      );
    } else {
      chmodSync(path.join(first, name), 0o644);
      assert.equal(findOnPath(name, { PATH: first }), undefined);
    }
  });
});
