#!/usr/bin/env node
// Rejects a pull request that changes the plugin version in any of the five plugin manifests, for the validate
// workflow's pr-version-fields job.
//   BASE_SHA=<sha> node .github/tools/src/scripts/ci-version-fields.ts
// BASE_SHA comes from the event payload and is validated before it reaches git; it is the baseline only when HEAD is
// not a merge commit. A manifest that is deleted or unparsable fails closed: skipping it would let a version edit hide
// behind a broken file. The job runs no `npm ci`, so its imports must resolve without installed packages.

import { execFileSync, spawnSync } from "node:child_process";
import {
  CANONICAL_PLUGIN_MANIFEST as CANONICAL_MANIFEST,
  VERSIONED_PLUGIN_MANIFESTS,
} from "../plugin-manifests.ts";

export type Env = Readonly<Record<string, string | undefined>>;

// Stops the check with a message the step prints as a workflow error.
class Rejection extends Error {}

const isObject = (value: unknown): value is Record<string, unknown> =>
  value !== null && typeof value === "object" && !Array.isArray(value);

const parse = (text: string): unknown => {
  try {
    return JSON.parse(text);
  } catch {
    return undefined;
  }
};

const show = (version: unknown): string =>
  version === null ? "absent" : JSON.stringify(version);

function verify(env: Env, cwd: string): void {
  const fail: (message: string) => never = (message) => {
    throw new Rejection(message);
  };

  const baseSha = env["BASE_SHA"] || "";
  if (!/^[0-9a-f]{40}$/.test(baseSha)) {
    fail("BASE_SHA must be a 40-character lowercase hex commit id.");
  }

  const git = (...args: string[]): string => {
    try {
      return execFileSync("git", args, {
        cwd,
        encoding: "utf8",
        stdio: ["ignore", "pipe", "pipe"],
      });
    } catch (error) {
      const failure = error as { stderr?: unknown; message: string };
      return fail(
        `git ${args[0]} failed: ${String(failure.stderr || failure.message).trim()}`,
      );
    }
  };

  // On the merge ref HEAD is a merge commit and its first parent is the
  // base tip the ref was built on. The payload's base.sha can be older
  // than that after the base branch advances, so it is only the
  // baseline for a checkout that is not a merge commit.
  const parentCount =
    git("rev-list", "--parents", "-n", "1", "HEAD").trim().split(/\s+/).length -
    1;
  const mergeBuilt = parentCount === 2;
  if (!mergeBuilt) {
    const baseKnown =
      spawnSync("git", ["cat-file", "-e", `${baseSha}^{commit}`], {
        cwd,
        stdio: "ignore",
      }).status === 0;
    if (!baseKnown) {
      fail(
        `The base commit ${baseSha} is not in this checkout; fetch the full history.`,
      );
    }
  }
  const baseRev = mergeBuilt ? "HEAD^1" : baseSha;

  const readDocument = (rev: string, where: string, file: string) => {
    if (!git("ls-tree", "-z", "--name-only", rev, "--", file)) {
      return undefined;
    }
    const doc = parse(git("show", `${rev}:${file}`));
    if (!isObject(doc)) {
      fail(
        `${file} ${where} is not a JSON object, so its plugin version cannot be read.`,
      );
    }
    return doc;
  };

  // Read the identity from the same revision as the marketplace so a rename
  // still compares the plugin's versions, rather than the catalog's version.
  const read = (rev: string, where: string, file: string) => {
    const doc = readDocument(rev, where, file);
    if (!doc) return { exists: false, version: null };

    let entry: Record<string, unknown> | undefined = doc;
    if (file.endsWith("marketplace.json")) {
      // The loop below validates the canonical manifest itself, including
      // deletion. Preserve the marketplace comparison when that file is absent.
      const name =
        readDocument(rev, where, CANONICAL_MANIFEST)?.name ?? "agentics";
      entry = (Array.isArray(doc.plugins) ? doc.plugins : []).find(
        (plugin) => isObject(plugin) && plugin.name === name,
      );
    }
    return {
      exists: true,
      version: entry && entry.version !== undefined ? entry.version : null,
    };
  };

  // A manifest that is already unreadable on the base fails closed here
  // too, so the pull request that repairs it needs a maintainer override.
  for (const { path: file } of VERSIONED_PLUGIN_MANIFESTS) {
    const before = read(baseRev, "at the base commit", file);
    const after = read("HEAD", "in this pull request", file);
    if (before.exists && !after.exists) {
      fail(
        `${file} is deleted by this pull request; the plugin version must stay readable from all five manifests.`,
      );
    }
    if (!before.exists && after.exists) {
      const canonicalBefore = read(
        baseRev,
        "at the base commit",
        CANONICAL_MANIFEST,
      );
      const canonicalAfter = read(
        "HEAD",
        "in this pull request",
        CANONICAL_MANIFEST,
      );
      const canonicalUnchanged =
        canonicalBefore.exists &&
        canonicalAfter.exists &&
        typeof canonicalBefore.version === "string" &&
        canonicalBefore.version === canonicalAfter.version;
      if (
        canonicalUnchanged &&
        typeof after.version === "string" &&
        after.version === canonicalBefore.version
      ) {
        continue;
      }
      fail(
        `${file} is added by this pull request with plugin version ${show(after.version)}; a newly added manifest must match the unchanged canonical version ${show(canonicalBefore.version)} in ${CANONICAL_MANIFEST}.`,
      );
    }
    if (JSON.stringify(before.version) !== JSON.stringify(after.version)) {
      fail(
        `${file} changes the plugin version from ${show(before.version)} to ${show(after.version)}; only the release commit changes the plugin version. Revert that field.`,
      );
    }
  }
}

// Returns the exit status of the step; every rejection is printed on stdout as a workflow error.
export function checkVersionFields({
  env = process.env,
  cwd = process.cwd(),
  write = console.log,
}: {
  env?: Env;
  cwd?: string;
  write?: (line: string) => void;
} = {}): number {
  try {
    verify(env, cwd);
  } catch (error) {
    if (!(error instanceof Rejection)) throw error;
    write(`::error::${error.message}`);
    return 1;
  }
  write("Version fields: unchanged.");
  return 0;
}

if (import.meta.main) process.exitCode = checkVersionFields();
