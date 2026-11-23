import assert from "node:assert/strict";
import { test } from "node:test";
import { readTunables } from "./config.ts";
import type { Host } from "./host.ts";

function hostWith(env: Record<string, string>): { host: Host; err: string[] } {
  const err: string[] = [];
  const host: Host = {
    env,
    cwd: "/",
    platform: "linux",
    out: () => undefined,
    err: (text) => void err.push(text),
  };
  return { host, err };
}

test("unset tunables take the launcher's defaults without a warning", () => {
  const { host, err } = hostWith({});
  assert.deepEqual(readTunables(host), {
    maxDiffBytes: 256000,
    preflightTimeout: 15,
    heartbeatInterval: 30,
    claudeModel: "fable",
    claudeFallbackModel: "opus",
    codexModel: "gpt-6.1-sol",
  });
  assert.deepEqual(err, []);
});

test("a heartbeat of 0 turns the ticker off and is not a warning", () => {
  const { host, err } = hostWith({ PEER_HEARTBEAT_INTERVAL: "0" });
  assert.equal(readTunables(host).heartbeatInterval, 0);
  assert.deepEqual(err, []);
});

// The watchdog reads 0 seconds as an immediate timeout, not as no limit, so a zero preflight timeout would fail every
// run. It falls back to the default instead.
test("a preflight timeout of 0 or a non-integer warns and uses the default", () => {
  for (const value of ["0", "soon", "1.5", "-3"]) {
    const { host, err } = hostWith({ PEER_PREFLIGHT_TIMEOUT: value });
    assert.equal(readTunables(host).preflightTimeout, 15, value);
    assert.deepEqual(err, [
      `WARNING: PEER_PREFLIGHT_TIMEOUT='${value}' is not a positive integer; using 15\n`,
    ]);
  }
});

test("a non-integer diff limit warns and uses the default, and an empty value takes it silently", () => {
  const bad = hostWith({ PEER_MAX_DIFF_BYTES: "lots" });
  assert.equal(readTunables(bad.host).maxDiffBytes, 256000);
  assert.deepEqual(bad.err, [
    "WARNING: PEER_MAX_DIFF_BYTES='lots' is not a non-negative integer; using 256000\n",
  ]);
  const empty = hostWith({ PEER_MAX_DIFF_BYTES: "" });
  assert.equal(readTunables(empty.host).maxDiffBytes, 256000);
  assert.deepEqual(empty.err, []);
});

// A digit string past Number.MAX_SAFE_INTEGER converts to a rounded value or Infinity, and Infinity would leave the
// preflight watchdog unbounded. A heartbeat above the timer ceiling would fire every millisecond.
test("an oversized tunable warns and uses the default", () => {
  const huge = "9".repeat(400);
  for (const [name, value, fallback] of [
    ["PEER_PREFLIGHT_TIMEOUT", huge, 15],
    ["PEER_MAX_DIFF_BYTES", huge, 256000],
    ["PEER_HEARTBEAT_INTERVAL", huge, 30],
    ["PEER_HEARTBEAT_INTERVAL", "2147484", 30],
    ["PEER_PREFLIGHT_TIMEOUT", "9007199254740993", 15],
  ] as const) {
    const { host, err } = hostWith({ [name]: value });
    const tunables = readTunables(host);
    const got = {
      PEER_PREFLIGHT_TIMEOUT: tunables.preflightTimeout,
      PEER_MAX_DIFF_BYTES: tunables.maxDiffBytes,
      PEER_HEARTBEAT_INTERVAL: tunables.heartbeatInterval,
    }[name];
    assert.equal(got, fallback, `${name}=${value}`);
    assert.equal(err.length, 1, `${name}=${value}`);
  }
});

test("the largest heartbeat the timer can hold is accepted", () => {
  const { host, err } = hostWith({ PEER_HEARTBEAT_INTERVAL: "2147483" });
  assert.equal(readTunables(host).heartbeatInterval, 2147483);
  assert.deepEqual(err, []);
});

test("model overrides apply, and an empty override takes the default", () => {
  const { host } = hostWith({
    PEER_CODEX_MODEL: "gpt-x",
    PEER_CLAUDE_MODEL: "",
    PEER_CLAUDE_FALLBACK_MODEL: "sonnet",
  });
  const tunables = readTunables(host);
  assert.equal(tunables.codexModel, "gpt-x");
  assert.equal(tunables.claudeModel, "fable");
  assert.equal(tunables.claudeFallbackModel, "sonnet");
});
