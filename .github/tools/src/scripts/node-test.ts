#!/usr/bin/env node

// AGX_TEST_CONCURRENCY overrides the number of concurrent test files.
// By default, node:test runs at least four files at once. These suites spend much of
// their time waiting for child processes, even on a two-CPU CI runner.
import { availableParallelism } from "node:os";
import { spawnSync } from "node:child_process";

const MINIMUM = 4;

export function concurrency(
  cpus = availableParallelism(),
  override?: string,
): number {
  if (override !== undefined) {
    if (!/^[1-9]\d*$/.test(override) || !Number.isSafeInteger(Number(override)))
      throw new Error("AGX_TEST_CONCURRENCY must be a positive safe integer");
    return Number(override);
  }
  return Math.max(MINIMUM, cpus - 1);
}

function main(patterns: string[]): number {
  let limit;
  try {
    limit = concurrency(
      availableParallelism(),
      process.env.AGX_TEST_CONCURRENCY,
    );
  } catch (error) {
    console.error(String(error));
    return 2;
  }
  const result = spawnSync(
    process.execPath,
    ["--test", `--test-concurrency=${limit}`, ...patterns],
    { stdio: "inherit" },
  );
  if (result.error) {
    console.error(result.error.message);
    return 1;
  }
  return result.status ?? 1;
}

if (import.meta.main) process.exit(main(process.argv.slice(2)));
