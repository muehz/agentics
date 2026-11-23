#!/usr/bin/env node

import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { globSync, mkdirSync } from "node:fs";
import path from "node:path";

const plans = {
  "windows-tools": { workspace: ".github/tools", count: 3 },
  "coverage-tools": { workspace: ".github/tools", count: 1 },
} as const;

type Plan = keyof typeof plans;

function shardFor(plan: Plan, file: string): number {
  if (plan === "coverage-tools") return 1;
  const area = file.split("/")[1];
  if (area === "scripts") return 1;
  if (area === "conformance" || area === "boundaries") return 2;
  return 3;
}

export function partitionTests(plan: Plan, files: string[]): string[][] {
  const shards: string[][] = Array.from(
    { length: plans[plan].count },
    () => [],
  );
  for (const file of files.map((name) => name.replaceAll("\\", "/")).sort()) {
    shards[shardFor(plan, file) - 1]!.push(file);
  }
  assert(
    shards.every((shard) => shard.length > 0),
    `${plan}: empty test shard`,
  );
  assert.equal(
    new Set(shards.flat()).size,
    files.length,
    `${plan}: duplicate test file`,
  );
  return shards;
}

function main(args: string[]): number {
  const [plan, shard, workers] = args;
  assert(plan && Object.hasOwn(plans, plan), "unknown CI test plan");
  const selected = plan as Plan;
  const { workspace, count } = plans[selected];
  assert(
    shard && /^[1-4]$/.test(shard) && Number(shard) <= count,
    "invalid CI test shard",
  );
  const coverage = selected.startsWith("coverage-");
  assert(
    args.length === 3 &&
      (coverage ? workers === "1" : workers === "2" || workers === "4"),
    "invalid CI test concurrency",
  );

  const files = partitionTests(
    selected,
    globSync("src/**/*.test.ts", { cwd: workspace }),
  )[Number(shard) - 1]!;
  console.log(
    `Test plan ${selected}, shard ${shard}/${count}, ${files.length} files, concurrency ${workers}`,
  );
  const reporter = new URL("./ci-test-timing.ts", import.meta.url).href;
  const options = [
    "--test",
    `--test-concurrency=${workers}`,
    "--test-reporter=spec",
    "--test-reporter-destination=stdout",
    `--test-reporter=${reporter}`,
    "--test-reporter-destination=stderr",
  ];
  if (coverage) {
    mkdirSync(path.join(workspace, "coverage"), { recursive: true });
    options.push(
      "--experimental-test-coverage",
      "--test-coverage-exclude=**/*.test.ts",
      "--test-coverage-exclude=**/testing/**",
      "--test-reporter=lcov",
      "--test-reporter-destination=coverage/lcov.info",
    );
  }
  const result = spawnSync(process.execPath, [...options, "--", ...files], {
    cwd: workspace,
    stdio: "inherit",
  });
  if (result.error) console.error(result.error.message);
  return result.status ?? 1;
}

if (import.meta.main) {
  process.exit(main(process.argv.slice(2)));
}
