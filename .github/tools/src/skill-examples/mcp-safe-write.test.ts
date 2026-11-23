// Extracts the read and write helpers directly from forge's MCP security checklist so edits to the guide are tested.
import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

const checklistPath = fileURLToPath(
  new URL(
    "../../../../skills/forge/references/mcp/mcp-security-checklist.md",
    import.meta.url,
  ),
);

function fence(language: string, marker: string): string {
  const text = fs.readFileSync(checklistPath, "utf8");
  const blocks = [...text.matchAll(/^```(\w+)\n([\s\S]*?)^```$/gm)];
  const hit = blocks.find((m) => m[1] === language && m[2]!.includes(marker));
  assert.ok(hit, `the checklist has no ${language} block that holds ${marker}`);
  return hit[2]!;
}

function workspace(t: { after: (fn: () => void) => void }) {
  const base = fs.realpathSync(
    fs.mkdtempSync(path.join(os.tmpdir(), "mcp-safe-write-")),
  );
  t.after(() => fs.rmSync(base, { recursive: true, force: true }));
  const root = path.join(base, "root");
  const outside = path.join(base, "outside");
  fs.mkdirSync(path.join(root, "dir"), { recursive: true });
  fs.mkdirSync(outside);
  fs.writeFileSync(path.join(outside, "secret.txt"), "secret");
  return { root, outside };
}

// The swap an attacker makes: the directory the write is walking through becomes a link that leaves the root.
function swapDirForLink(root: string, outside: string): void {
  fs.renameSync(path.join(root, "dir"), path.join(root, "dir.old"));
  fs.symlinkSync(outside, path.join(root, "dir"));
}

function outsideFiles(outside: string): string[] {
  return fs.readdirSync(outside).sort();
}

const BLOCKED = /Path traversal blocked/;

function makeFifo(file: string): void {
  const made = spawnSync("mkfifo", [file]);
  assert.equal(made.status, 0, String(made.stderr));
}

// A reader that keeps a FIFO open, so that a write-only open of it succeeds instead of failing.
function holdReader(
  t: { after: (fn: () => void) => void },
  file: string,
): void {
  const handle = fs.openSync(
    file,
    fs.constants.O_RDONLY | fs.constants.O_NONBLOCK,
  );
  t.after(() => fs.closeSync(handle));
}

// --- TypeScript form (Linux only: it relies on /proc/self/fd) ---

type WriteInside = (root: string, input: string, data: string) => void;
type FsLike = typeof fs;

function load<T>(code: string, name: string, fsLike: FsLike): T {
  return new Function("fs", "path", `${code}\nreturn ${name};`)(fsLike, path);
}

// An fs whose openSync runs a callback after the Nth open, which is the moment between two steps of the helper.
function fsWithStepHook(afterOpen: (count: number) => void): FsLike {
  let count = 0;
  return {
    ...fs,
    openSync: (...args: Parameters<typeof fs.openSync>) => {
      const handle = fs.openSync(...args);
      count += 1;
      afterOpen(count);
      return handle;
    },
  } as FsLike;
}

const linuxOnly =
  process.platform !== "linux" && "the TypeScript form needs /proc/self/fd";
const writeInsideCode = () => fence("javascript", "function writeInside");

test(
  "check then write with safePath leaves the root when a directory is swapped for a link",
  { skip: linuxOnly },
  (t) => {
    const { root, outside } = workspace(t);
    const safePath = load<(input: string, root: string) => string>(
      fence("javascript", "function safePath"),
      "safePath",
      fs,
    );
    const checked = safePath("dir/new.txt", root);
    swapDirForLink(root, outside);
    fs.writeFileSync(checked, "x");
    assert.deepEqual(outsideFiles(outside), ["new.txt", "secret.txt"]);
  },
);

test(
  "writeInside writes a new file and replaces an existing one",
  { skip: linuxOnly },
  (t) => {
    const { root } = workspace(t);
    const write = load<WriteInside>(writeInsideCode(), "writeInside", fs);
    write(root, "dir/new.txt", "one");
    write(root, "dir/new.txt", "two");
    assert.equal(
      fs.readFileSync(path.join(root, "dir", "new.txt"), "utf8"),
      "two",
    );
  },
);

test(
  "writeInside rejects absolute, empty, dot and parent segments before any file system call",
  { skip: linuxOnly },
  (t) => {
    const { root, outside } = workspace(t);
    const write = load<WriteInside>(
      writeInsideCode(),
      "writeInside",
      fsWithStepHook(() => assert.fail("opened a path")),
    );
    for (const input of [
      "/etc/x",
      "../x",
      "dir/../../x",
      "dir//x",
      "./x",
      "",
    ]) {
      assert.throws(() => write(root, input, "x"), BLOCKED, input);
    }
    assert.deepEqual(outsideFiles(outside), ["secret.txt"]);
  },
);

test(
  "writeInside refuses a directory that is already a link out of the root",
  { skip: linuxOnly },
  (t) => {
    const { root, outside } = workspace(t);
    fs.symlinkSync(outside, path.join(root, "linked"));
    const write = load<WriteInside>(writeInsideCode(), "writeInside", fs);
    assert.throws(() => write(root, "linked/new.txt", "x"), BLOCKED);
    assert.deepEqual(outsideFiles(outside), ["secret.txt"]);
  },
);

test(
  "writeInside refuses a final name that is a link to an existing file or to a missing one",
  { skip: linuxOnly },
  (t) => {
    const { root, outside } = workspace(t);
    fs.symlinkSync(
      path.join(outside, "secret.txt"),
      path.join(root, "dir", "existing.txt"),
    );
    fs.symlinkSync(
      path.join(outside, "new.txt"),
      path.join(root, "dir", "dangling.txt"),
    );
    const write = load<WriteInside>(writeInsideCode(), "writeInside", fs);
    assert.throws(() => write(root, "dir/existing.txt", "x"), BLOCKED);
    assert.throws(() => write(root, "dir/dangling.txt", "x"), BLOCKED);
    assert.equal(
      fs.readFileSync(path.join(outside, "secret.txt"), "utf8"),
      "secret",
    );
    assert.deepEqual(outsideFiles(outside), ["secret.txt"]);
  },
);

test(
  "writeInside refuses a hard link to a file outside the root and leaves that file alone",
  { skip: linuxOnly },
  (t) => {
    const { root, outside } = workspace(t);
    fs.linkSync(
      path.join(outside, "secret.txt"),
      path.join(root, "dir", "hard.txt"),
    );
    const write = load<WriteInside>(writeInsideCode(), "writeInside", fs);
    assert.throws(() => write(root, "dir/hard.txt", "x"), BLOCKED);
    assert.equal(
      fs.readFileSync(path.join(outside, "secret.txt"), "utf8"),
      "secret",
    );
  },
);

test(
  "writeInside replaces a longer existing file with the new data and nothing else",
  { skip: linuxOnly },
  (t) => {
    const { root } = workspace(t);
    const write = load<WriteInside>(writeInsideCode(), "writeInside", fs);
    write(root, "dir/new.txt", "a longer first version");
    write(root, "dir/new.txt", "x");
    assert.equal(
      fs.readFileSync(path.join(root, "dir", "new.txt"), "utf8"),
      "x",
    );
  },
);

test(
  "the hard link test fails when the link count check is removed from writeInside",
  { skip: linuxOnly },
  (t) => {
    const { root, outside } = workspace(t);
    const original = writeInsideCode();
    const mutant = original.replace(" || info.nlink > 1", "");
    assert.notEqual(
      mutant,
      original,
      "the helper no longer checks the link count",
    );
    fs.linkSync(
      path.join(outside, "secret.txt"),
      path.join(root, "dir", "hard.txt"),
    );
    const write = load<WriteInside>(mutant, "writeInside", fs);
    write(root, "dir/hard.txt", "x");
    assert.equal(
      fs.readFileSync(path.join(outside, "secret.txt"), "utf8"),
      "x",
    );
  },
);

// An open of a FIFO that has no reader blocks until one appears. This fs refuses to block: it fails the open of the
// FIFO unless the helper asked for O_NONBLOCK, which stands in for the hang that a bare open would be.
function fsThatHangsOnABlockingFifoOpen(): FsLike {
  return {
    ...fs,
    openSync: (...args: Parameters<typeof fs.openSync>) => {
      const [target, flags] = args;
      if (
        String(target).endsWith("/fifo") &&
        !(Number(flags) & fs.constants.O_NONBLOCK)
      ) {
        throw new Error("would block on a FIFO open");
      }
      return fs.openSync(...args);
    },
  } as FsLike;
}

test(
  "writeInside refuses a FIFO that has no reader instead of blocking on it",
  { skip: linuxOnly },
  (t) => {
    const { root } = workspace(t);
    makeFifo(path.join(root, "dir", "fifo"));
    const write = load<WriteInside>(
      writeInsideCode(),
      "writeInside",
      fsThatHangsOnABlockingFifoOpen(),
    );
    assert.throws(() => write(root, "dir/fifo", "x"), BLOCKED);
  },
);

test(
  "writeInside refuses a FIFO that a reader holds open",
  { skip: linuxOnly },
  (t) => {
    const { root } = workspace(t);
    const fifo = path.join(root, "dir", "fifo");
    makeFifo(fifo);
    holdReader(t, fifo);
    const write = load<WriteInside>(
      writeInsideCode(),
      "writeInside",
      fsThatHangsOnABlockingFifoOpen(),
    );
    assert.throws(() => write(root, "dir/fifo", "x"), BLOCKED);
  },
);

test(
  "the FIFO test fails when O_NONBLOCK is removed from writeInside",
  { skip: linuxOnly },
  (t) => {
    const { root } = workspace(t);
    makeFifo(path.join(root, "dir", "fifo"));
    const original = writeInsideCode();
    const mutant = original.replaceAll(" | O_NONBLOCK", "");
    assert.notEqual(
      mutant,
      original,
      "the helper no longer names O_NONBLOCK in a flag expression",
    );
    const write = load<WriteInside>(
      mutant,
      "writeInside",
      fsThatHangsOnABlockingFifoOpen(),
    );
    assert.throws(
      () => write(root, "dir/fifo", "x"),
      /would block on a FIFO open/,
    );
  },
);

test(
  "writeInside stays inside the root when a directory is swapped for a link between the steps",
  { skip: linuxOnly },
  (t) => {
    const { root, outside } = workspace(t);
    // Open 1 is the root and open 2 is `dir`; the swap lands before the final open.
    const write = load<WriteInside>(
      writeInsideCode(),
      "writeInside",
      fsWithStepHook((count) => count === 2 && swapDirForLink(root, outside)),
    );
    write(root, "dir/new.txt", "x");
    assert.deepEqual(outsideFiles(outside), ["secret.txt"]);
    assert.equal(
      fs.readFileSync(path.join(root, "dir.old", "new.txt"), "utf8"),
      "x",
    );
  },
);

test(
  "writeInside refuses a final name swapped for a link between the steps",
  { skip: linuxOnly },
  (t) => {
    const { root, outside } = workspace(t);
    const write = load<WriteInside>(
      writeInsideCode(),
      "writeInside",
      fsWithStepHook(
        (count) =>
          count === 2 &&
          fs.symlinkSync(
            path.join(outside, "new.txt"),
            path.join(root, "dir", "new.txt"),
          ),
      ),
    );
    assert.throws(() => write(root, "dir/new.txt", "x"), BLOCKED);
    assert.deepEqual(outsideFiles(outside), ["secret.txt"]);
  },
);

test(
  "the link tests fail when O_NOFOLLOW is removed from writeInside",
  { skip: linuxOnly },
  (t) => {
    const { root, outside } = workspace(t);
    const original = writeInsideCode();
    const mutant = original.replaceAll(" | O_NOFOLLOW", "");
    assert.notEqual(
      mutant,
      original,
      "the helper no longer names O_NOFOLLOW in a flag expression",
    );
    fs.symlinkSync(outside, path.join(root, "linked"));
    fs.symlinkSync(
      path.join(outside, "gone.txt"),
      path.join(root, "dir", "dangling.txt"),
    );
    const write = load<WriteInside>(mutant, "writeInside", fs);
    write(root, "linked/new.txt", "x");
    write(root, "dir/dangling.txt", "x");
    assert.deepEqual(outsideFiles(outside), [
      "gone.txt",
      "new.txt",
      "secret.txt",
    ]);
  },
);

// --- Python form (Linux and macOS) ---

const HARNESS = `
import json, os, sys
root, outside, scenario, snippet = sys.argv[1:5]
real_open = os.open
opened = []

def swap_dir():
    os.rename(os.path.join(root, "dir"), os.path.join(root, "dir.old"))
    os.symlink(outside, os.path.join(root, "dir"))

def hook(*args, **kwargs):
    if args[0] == "fifo" and not args[1] & os.O_NONBLOCK:
        # A bare open of a FIFO with no reader blocks forever; stand in for that hang.
        raise RuntimeError("would block on a FIFO open")
    handle = real_open(*args, **kwargs)
    opened.append(args[0])
    if len(opened) == 2 and scenario == "swap-dir":
        swap_dir()
    if len(opened) == 2 and scenario == "swap-name":
        os.symlink(os.path.join(outside, "new.txt"), os.path.join(root, "dir", "new.txt"))
    return handle

os.open = hook
namespace = {}
exec(open(snippet).read(), namespace)
error = None
data = b"abcdef" if scenario == "short-write" else b"x"
if scenario == "short-write":
    # A write call that accepts one byte at a time, as a full disk or a pipe can.
    real_write = os.write
    os.write = lambda handle, chunk: real_write(handle, bytes(chunk)[:1])
try:
    namespace["write_inside"](root, sys.argv[5], data)
except (PermissionError, RuntimeError) as caught:
    error = str(caught)
print(json.dumps({"error": error, "opened": len(opened)}))
`;

const hasPython = spawnSync("python3", ["--version"]).status === 0;
const pythonSkip =
  process.platform === "win32"
    ? "the Python form needs O_NOFOLLOW and dir_fd"
    : !hasPython && !process.env["CI"] && "python3 is not installed";

type Workspace = { root: string; outside: string };

function runPython(
  t: { after: (fn: () => void) => void },
  scenario: string,
  input: string,
  code: string,
  setup?: (w: Workspace) => void,
) {
  const w = workspace(t);
  setup?.(w);
  const snippet = path.join(path.dirname(w.root), "snippet.py");
  const harness = path.join(path.dirname(w.root), "harness.py");
  fs.writeFileSync(snippet, code);
  fs.writeFileSync(harness, HARNESS);
  const run = spawnSync(
    "python3",
    ["-I", harness, w.root, w.outside, scenario, snippet, input],
    { encoding: "utf8" },
  );
  assert.equal(run.status, 0, run.stderr);
  return {
    ...w,
    result: JSON.parse(run.stdout) as { error: string | null; opened: number },
  };
}

const pythonCode = () => fence("python", "def write_inside");

function readFixtures(root: string): void {
  const dir = path.join(root, "dir");
  fs.writeFileSync(path.join(dir, "plain"), "fixture");
  fs.symlinkSync("plain", path.join(dir, "symbolic"));
  fs.symlinkSync("dir", path.join(root, "alias"));
  fs.writeFileSync(path.join(dir, "original"), "fixture");
  fs.linkSync(path.join(dir, "original"), path.join(dir, "hard"));
}

const disallowedReads = ["dir/symbolic", "alias/plain", "dir/hard", "dir"];

test(
  "Linux Node guide reads only a regular file with one link",
  { skip: linuxOnly },
  (t) => {
    const { root } = workspace(t);
    readFixtures(root);
    const opens: { name: string; flags: number; fd: number }[] = [];
    const reads: unknown[] = [];
    const observed = {
      ...fs,
      openSync(name: string, flags: number) {
        const fd = fs.openSync(name, flags);
        opens.push({ name, flags, fd });
        return fd;
      },
      readFileSync(fd: number) {
        reads.push(fd);
        return fs.readFileSync(fd);
      },
    } as typeof fs;
    const read = load<(root: string, input: string) => Buffer>(
      fence("javascript", "function readInside"),
      "readInside",
      observed,
    );
    assert.equal(read(root, "dir/plain").toString(), "fixture");
    assert.equal(opens.length, 3);
    const [rootOpen, directoryOpen, leafOpen] = opens;
    assert.equal(rootOpen!.name, root);
    assert.equal(directoryOpen!.name, `/proc/self/fd/${rootOpen!.fd}/dir`);
    assert.equal(leafOpen!.name, `/proc/self/fd/${directoryOpen!.fd}/plain`);
    for (const { flags } of opens) assert.ok(flags & fs.constants.O_NOFOLLOW);
    assert.deepEqual(reads, [leafOpen!.fd]);
    for (const input of disallowedReads) {
      assert.throws(() => read(root, input), BLOCKED, input);
    }
  },
);

test(
  "POSIX Python guide reads only a regular file with one link",
  { skip: pythonSkip },
  (t) => {
    const { root } = workspace(t);
    readFixtures(root);
    const snippet = fence("python", "def read_inside");
    const harness = `
import sys
root, *disallowed = sys.argv[1:]
opened = []
real_open = os.open
def observe(name, flags, *args, **kwargs):
    fd = real_open(name, flags, *args, **kwargs)
    opened.append((name, flags, kwargs.get("dir_fd"), fd))
    return fd
os.open = observe
try:
    assert read_inside(root, "dir/plain") == b"fixture"
finally:
    os.open = real_open
assert [entry[0] for entry in opened] == [root, "dir", "plain"], opened
assert [entry[2] for entry in opened] == [None, opened[0][3], opened[1][3]], opened
assert all(entry[1] & os.O_NOFOLLOW for entry in opened), opened
for name in disallowed:
    try:
        read_inside(root, name)
    except PermissionError as error:
        assert "Path traversal blocked" in str(error), (name, str(error))
    else:
        raise AssertionError(f"expected refusal for {name}")
`;
    const run = spawnSync(
      "python3",
      ["-I", "-c", `${snippet}\n${harness}`, root, ...disallowedReads],
      {
        encoding: "utf8",
      },
    );
    assert.equal(run.status, 0, run.stderr);
  },
);

test(
  "write_inside writes a new file inside the root",
  { skip: pythonSkip },
  (t) => {
    const { root, result } = runPython(t, "plain", "dir/new.txt", pythonCode());
    assert.equal(result.error, null);
    assert.equal(
      fs.readFileSync(path.join(root, "dir", "new.txt"), "utf8"),
      "x",
    );
  },
);

test(
  "write_inside rejects parent segments and absolute paths before it opens anything",
  { skip: pythonSkip },
  (t) => {
    for (const input of ["../x", "/etc/x", "dir//x", "./x"]) {
      const { result } = runPython(t, "plain", input, pythonCode());
      assert.match(result.error ?? "", BLOCKED, input);
      assert.equal(result.opened, 0, input);
    }
  },
);

test(
  "write_inside refuses a directory link, a file link and a dangling link",
  { skip: pythonSkip },
  (t) => {
    const cases: [string, (w: Workspace) => void][] = [
      [
        "linked/new.txt",
        ({ root, outside }) =>
          fs.symlinkSync(outside, path.join(root, "linked")),
      ],
      [
        "dir/existing.txt",
        ({ root, outside }) =>
          fs.symlinkSync(
            path.join(outside, "secret.txt"),
            path.join(root, "dir", "existing.txt"),
          ),
      ],
      [
        "dir/dangling.txt",
        ({ root, outside }) =>
          fs.symlinkSync(
            path.join(outside, "gone.txt"),
            path.join(root, "dir", "dangling.txt"),
          ),
      ],
    ];
    for (const [input, setup] of cases) {
      const { outside, result } = runPython(
        t,
        "plain",
        input,
        pythonCode(),
        setup,
      );
      assert.match(result.error ?? "", BLOCKED, input);
      assert.deepEqual(outsideFiles(outside), ["secret.txt"], input);
    }
  },
);

test(
  "write_inside refuses a hard link to a file outside the root and leaves that file alone",
  { skip: pythonSkip },
  (t) => {
    const { outside, result } = runPython(
      t,
      "plain",
      "dir/hard.txt",
      pythonCode(),
      ({ root, outside }) =>
        fs.linkSync(
          path.join(outside, "secret.txt"),
          path.join(root, "dir", "hard.txt"),
        ),
    );
    assert.match(result.error ?? "", BLOCKED);
    assert.equal(
      fs.readFileSync(path.join(outside, "secret.txt"), "utf8"),
      "secret",
    );
  },
);

test(
  "write_inside replaces a longer existing file with the new data and nothing else",
  { skip: pythonSkip },
  (t) => {
    const { root, result } = runPython(
      t,
      "plain",
      "dir/new.txt",
      pythonCode(),
      ({ root }) =>
        fs.writeFileSync(
          path.join(root, "dir", "new.txt"),
          "a longer first version",
        ),
    );
    assert.equal(result.error, null);
    assert.equal(
      fs.readFileSync(path.join(root, "dir", "new.txt"), "utf8"),
      "x",
    );
  },
);

test(
  "write_inside writes every byte when the write call accepts only one at a time",
  { skip: pythonSkip },
  (t) => {
    const { root, result } = runPython(
      t,
      "short-write",
      "dir/new.txt",
      pythonCode(),
    );
    assert.equal(result.error, null);
    assert.equal(
      fs.readFileSync(path.join(root, "dir", "new.txt"), "utf8"),
      "abcdef",
    );
  },
);

test(
  "the hard link test fails when the link count check is removed from write_inside",
  { skip: pythonSkip },
  (t) => {
    const original = pythonCode();
    const mutant = original.replace(" or info.st_nlink > 1", "");
    assert.notEqual(
      mutant,
      original,
      "the helper no longer checks the link count",
    );
    const { outside, result } = runPython(
      t,
      "plain",
      "dir/hard.txt",
      mutant,
      ({ root, outside }) =>
        fs.linkSync(
          path.join(outside, "secret.txt"),
          path.join(root, "dir", "hard.txt"),
        ),
    );
    assert.equal(result.error, null);
    assert.equal(
      fs.readFileSync(path.join(outside, "secret.txt"), "utf8"),
      "x",
    );
  },
);

test(
  "write_inside refuses a FIFO that has no reader instead of blocking on it",
  { skip: pythonSkip },
  (t) => {
    const { result } = runPython(
      t,
      "plain",
      "dir/fifo",
      pythonCode(),
      ({ root }) => makeFifo(path.join(root, "dir", "fifo")),
    );
    assert.match(result.error ?? "", BLOCKED);
  },
);

test(
  "write_inside refuses a FIFO that a reader holds open",
  { skip: pythonSkip },
  (t) => {
    const { result } = runPython(
      t,
      "plain",
      "dir/fifo",
      pythonCode(),
      ({ root }) => {
        const fifo = path.join(root, "dir", "fifo");
        makeFifo(fifo);
        holdReader(t, fifo);
      },
    );
    assert.match(result.error ?? "", BLOCKED);
  },
);

test(
  "the FIFO test fails when O_NONBLOCK is removed from write_inside",
  { skip: pythonSkip },
  (t) => {
    const original = pythonCode();
    const mutant = original.replace(" | os.O_NONBLOCK", "");
    assert.notEqual(
      mutant,
      original,
      "the helper no longer names O_NONBLOCK in a flag expression",
    );
    const { result } = runPython(t, "plain", "dir/fifo", mutant, ({ root }) =>
      makeFifo(path.join(root, "dir", "fifo")),
    );
    assert.match(result.error ?? "", /would block on a FIFO open/);
  },
);

test(
  "write_inside stays inside the root when a directory is swapped for a link between the steps",
  { skip: pythonSkip },
  (t) => {
    const { root, outside, result } = runPython(
      t,
      "swap-dir",
      "dir/new.txt",
      pythonCode(),
    );
    assert.equal(result.error, null);
    assert.deepEqual(outsideFiles(outside), ["secret.txt"]);
    assert.equal(
      fs.readFileSync(path.join(root, "dir.old", "new.txt"), "utf8"),
      "x",
    );
  },
);

test(
  "write_inside refuses a final name swapped for a link between the steps",
  { skip: pythonSkip },
  (t) => {
    const { outside, result } = runPython(
      t,
      "swap-name",
      "dir/new.txt",
      pythonCode(),
    );
    assert.match(result.error ?? "", BLOCKED);
    assert.deepEqual(outsideFiles(outside), ["secret.txt"]);
  },
);

test(
  "the link tests fail when O_NOFOLLOW is removed from write_inside",
  { skip: pythonSkip },
  (t) => {
    const original = pythonCode();
    const mutant = original.replaceAll(" | os.O_NOFOLLOW", "");
    assert.notEqual(
      mutant,
      original,
      "the helper no longer names O_NOFOLLOW in a flag expression",
    );
    const { outside, result } = runPython(
      t,
      "plain",
      "linked/new.txt",
      mutant,
      ({ root, outside }) => fs.symlinkSync(outside, path.join(root, "linked")),
    );
    assert.equal(result.error, null);
    assert.deepEqual(outsideFiles(outside), ["new.txt", "secret.txt"]);
  },
);
