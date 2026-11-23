import { mkdirSync, writeFileSync } from "node:fs";
import path from "node:path";

// The `skills` family runs the eval engine's `check --repo <repo> --json` through the Node script named in
// `AGX_CONFORMANCE_EVALS_CLI` and judges only the stream it prints. A fixture holds no eval engine, so a test points
// the variable at the stub written here and chooses the stream it prints.

/** The closing line `evals check --json` prints on a clean tree, with its newline. */
export const DEFAULT_STREAM = '{"done":true,"failures":0}\n';

/** What `writeEvalsStub` wrote. */
export type EvalsStub = {
  /** The Node script to put in `AGX_CONFORMANCE_EVALS_CLI`. */
  nodeScript: string;
  /** The file the stub prints. Rewrite it to change what the next run prints; no source changes. */
  streamFile: string;
};

const USAGE = "usage: evals check --repo <path> --json";

function nodeMain(streamFile: string): string {
  return `import { readFileSync } from "node:fs";

const streamFile = ${JSON.stringify(streamFile)};
const args = process.argv.slice(2);

function fail(message) {
  process.stderr.write(message + "\\n");
  process.exitCode = 2;
}

if (
  args.length !== 4 ||
  args[0] !== "check" ||
  args[1] !== "--repo" ||
  args[3] !== "--json"
) {
  fail(${JSON.stringify(USAGE)});
} else {
  let data;
  try {
    data = readFileSync(streamFile);
  } catch (error) {
    fail("evals stub: cannot read the stream file " + streamFile + ": " + error.message);
  }
  if (data !== undefined) process.stdout.write(data);
}
`;
}

/**
 * Writes the Node stub and its stream file under `dir`. The stub exits 2 for bad arguments or a missing stream file.
 */
export function writeEvalsStub(
  dir: string,
  stream: string = DEFAULT_STREAM,
): EvalsStub {
  const root = path.resolve(dir);
  const nodeScript = path.join(root, "evals-stub.mjs");
  const streamFile = path.join(root, "stream.jsonl");
  mkdirSync(root, { recursive: true });
  writeFileSync(nodeScript, nodeMain(streamFile));
  writeFileSync(streamFile, stream);
  return { nodeScript, streamFile };
}
