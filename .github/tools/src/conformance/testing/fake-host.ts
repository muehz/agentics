import type { Host } from "../host.ts";

export interface FakeHost extends Host {
  stdout: () => string;
  stderr: () => string;
}

export function fakeHost(overrides: Partial<Host> = {}): FakeHost {
  let out = "";
  let err = "";
  return {
    env: {},
    cwd: process.cwd(),
    out: (text) => void (out += text),
    err: (text) => void (err += text),
    ...overrides,
    stdout: () => out,
    stderr: () => err,
  };
}
