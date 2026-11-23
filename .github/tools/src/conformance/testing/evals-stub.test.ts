import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import {
  mkdirSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test } from "node:test";
import { DEFAULT_STREAM, writeEvalsStub } from "./evals-stub.ts";
import { removeTree } from "./host.ts";

const TWO_FAILURES_AND_CLOSE =
  [
    '{"rule":"eval-record","path":"evals/skills.yaml","message":"first"}',
    '{"rule":"eval-digest","path":"evals/skills.yaml","message":"second\\nline"}',
    '{"done":true,"failures":2}',
  ].join("\n") + "\n";

/** Runs `body` with a fresh scratch directory (named with a space when `spaced`) and removes it afterwards. */
async function withScratch<T>(
  body: (dir: string) => Promise<T> | T,
  spaced = false,
): Promise<T> {
  const base = mkdtempSync(path.join(tmpdir(), "agentics-evalsstub-"));
  const dir = spaced ? path.join(base, "with space") : base;
  mkdirSync(dir, { recursive: true });
  try {
    return await body(dir);
  } finally {
    removeTree(base);
  }
}

type Ran = { status: number | null; stdout: string; stderr: string };

function runNode(script: string, args: string[]): Ran {
  const ran = spawnSync(process.execPath, [script, ...args], {
    encoding: "utf8",
  });
  return { status: ran.status, stdout: ran.stdout, stderr: ran.stderr };
}

/** Every file and every empty directory under `dir`, as `f:<rel>` and `d:<rel>`, sorted, with `/` separators. */
function listing(dir: string): string[] {
  const out: string[] = [];
  const walk = (rel: string): void => {
    const here = path.join(dir, ...rel.split("/").filter(Boolean));
    const entries = readdirSync(here, { withFileTypes: true });
    if (entries.length === 0 && rel !== "") out.push(`d:${rel}`);
    for (const entry of entries) {
      const child = rel === "" ? entry.name : `${rel}/${entry.name}`;
      if (entry.isDirectory()) walk(child);
      else out.push(`f:${child}`);
    }
  };
  walk("");
  return out.sort();
}

const GOOD_ARGS = ["check", "--repo", "anywhere", "--json"];
const BAD_ARGS: string[][] = [
  ["check", "--repo", "x"],
  [],
  ["verify", "--repo", "x", "--json"],
  ["check", "--repo", "x", "--json", "extra"],
];

test("the helper writes only the Node stub and its stream file", () =>
  withScratch((dir) => {
    const stub = writeEvalsStub(dir);
    assert.deepEqual(listing(dir), ["f:evals-stub.mjs", "f:stream.jsonl"]);
    assert.equal(stub.nodeScript, path.join(dir, "evals-stub.mjs"));
    assert.equal(stub.streamFile, path.join(dir, "stream.jsonl"));
    assert.ok(path.isAbsolute(stub.nodeScript));
    assert.equal(readFileSync(stub.streamFile, "utf8"), DEFAULT_STREAM);
  }));

test("the default stream is the clean closing line and a newline", () => {
  assert.equal(DEFAULT_STREAM, '{"done":true,"failures":0}\n');
});

test("the Node stub prints the default stream and exits 0 with nothing on stderr", () =>
  withScratch((dir) => {
    const stub = writeEvalsStub(dir);
    assert.deepEqual(runNode(stub.nodeScript, GOOD_ARGS), {
      status: 0,
      stdout: DEFAULT_STREAM,
      stderr: "",
    });
  }));

test("the Node stub prints a custom stream byte for byte", () =>
  withScratch((dir) => {
    const stub = writeEvalsStub(dir, TWO_FAILURES_AND_CLOSE);
    assert.deepEqual(runNode(stub.nodeScript, GOOD_ARGS), {
      status: 0,
      stdout: TWO_FAILURES_AND_CLOSE,
      stderr: "",
    });
  }));

test("a stream with no trailing newline and an empty stream are printed as they are", () =>
  withScratch((dir) => {
    const stub = writeEvalsStub(dir, "no newline");
    assert.equal(runNode(stub.nodeScript, GOOD_ARGS).stdout, "no newline");
    writeFileSync(stub.streamFile, "");
    assert.deepEqual(runNode(stub.nodeScript, GOOD_ARGS), {
      status: 0,
      stdout: "",
      stderr: "",
    });
  }));

test("rewriting the stream file changes the next Node run", () =>
  withScratch((dir) => {
    const stub = writeEvalsStub(dir);
    assert.equal(runNode(stub.nodeScript, GOOD_ARGS).stdout, DEFAULT_STREAM);
    writeFileSync(stub.streamFile, TWO_FAILURES_AND_CLOSE);
    assert.equal(
      runNode(stub.nodeScript, GOOD_ARGS).stdout,
      TWO_FAILURES_AND_CLOSE,
    );
  }));

for (const args of BAD_ARGS) {
  test(`the Node stub exits 2 with a usage line on stderr for [${args.join(" ")}]`, () =>
    withScratch((dir) => {
      const stub = writeEvalsStub(dir);
      const ran = runNode(stub.nodeScript, args);
      assert.equal(ran.status, 2);
      assert.equal(ran.stdout, "");
      assert.match(ran.stderr, /usage: evals check --repo <path> --json/);
    }));
}

test("the Node stub exits 2 naming the stream file when it is missing", () =>
  withScratch((dir) => {
    const stub = writeEvalsStub(dir);
    rmSync(stub.streamFile);
    const ran = runNode(stub.nodeScript, GOOD_ARGS);
    assert.equal(ran.status, 2);
    assert.equal(ran.stdout, "");
    assert.ok(ran.stderr.includes(stub.streamFile), ran.stderr);
  }));

test("the Node stub works from a scratch directory whose name holds a space", () =>
  withScratch((dir) => {
    const stub = writeEvalsStub(dir);
    assert.ok(stub.nodeScript.includes(" "));
    assert.deepEqual(runNode(stub.nodeScript, GOOD_ARGS), {
      status: 0,
      stdout: DEFAULT_STREAM,
      stderr: "",
    });
  }, true));
