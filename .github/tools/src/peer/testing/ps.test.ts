import assert from "node:assert/strict";
import { test } from "node:test";
import { pidState } from "./ps.ts";

const cases: Array<{
  name: string;
  out: string;
  diagnostic: string;
  code: number;
  alive: boolean;
  wantError: boolean;
}> = [
  {
    name: "live",
    out: "123 Sl+\n",
    diagnostic: "",
    code: 0,
    alive: true,
    wantError: false,
  },
  {
    name: "zombie",
    out: "123 Z+\n",
    diagnostic: "",
    code: 0,
    alive: false,
    wantError: false,
  },
  {
    name: "absent",
    out: "",
    diagnostic: "",
    code: 1,
    alive: false,
    wantError: false,
  },
  {
    name: "empty success",
    out: "",
    diagnostic: "",
    code: 0,
    alive: false,
    wantError: true,
  },
  {
    name: "stderr failure",
    out: "",
    diagnostic: "permission denied",
    code: 1,
    alive: false,
    wantError: true,
  },
  {
    name: "other process",
    out: "124 Z\n",
    diagnostic: "",
    code: 0,
    alive: false,
    wantError: true,
  },
  {
    name: "malformed zombie",
    out: "123 Z!\n",
    diagnostic: "",
    code: 0,
    alive: false,
    wantError: true,
  },
  {
    name: "duplicate rows",
    out: "123 Z\n123 Z\n",
    diagnostic: "",
    code: 0,
    alive: false,
    wantError: true,
  },
];

for (const tc of cases) {
  test(`pid observation rejects unknown visibility: ${tc.name}`, () => {
    if (tc.wantError) {
      assert.throws(() => pidState(123, tc.out, tc.diagnostic, tc.code));
    } else {
      assert.equal(pidState(123, tc.out, tc.diagnostic, tc.code), tc.alive);
    }
  });
}
