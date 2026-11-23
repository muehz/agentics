import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import { test } from "node:test";
import { toCobertura } from "./cobertura.ts";

test("Cobertura escapes filenames and derives totals from merged hits", () => {
  const xml = toCobertura(
    new Map([
      [
        ".github/tools/src/a&b.ts",
        {
          lines: new Map([
            [1, 3],
            [2, 0],
          ]),
          branches: new Map([
            ["1,0,0", 2],
            ["1,0,1", 0],
          ]),
        },
      ],
    ]),
  );
  assert.match(xml, /filename="\.github\/tools\/src\/a&amp;b.ts"/);
  assert.match(
    xml,
    /lines-covered="1" lines-valid="2" branches-covered="1" branches-valid="2"/,
  );
  assert.match(
    xml,
    /<line number="1" hits="3" branch="true" condition-coverage="50% \(1\/2\)"/,
  );
});

test("empty coverage has finite zero rates", () => {
  const xml = toCobertura(new Map());
  assert.match(xml, /line-rate="0.00000000" branch-rate="0.00000000"/);
  assert.doesNotMatch(xml, /NaN|Infinity/);
});

test("LCOV keeps the exact XML for ordered files, branches and escaped attributes", () => {
  const xml = toCobertura(
    new Map([
      [
        ".github/tools/src/z.ts",
        { lines: new Map([[6, 0]]), branches: new Map() },
      ],
      [
        ".github/tools/src/a&b'c.ts",
        {
          lines: new Map([
            [2, 0],
            [1, 3],
          ]),
          branches: new Map([
            ["1,0,0", 2],
            ["1,0,1", 0],
            ["1,0,2", 0],
          ]),
        },
      ],
    ]),
  );
  assert.match(xml, /filename="\.github\/tools\/src\/a&amp;b&apos;c\.ts"/);
  assert.match(xml, /condition-coverage="33% \(1\/3\)"/);
  // Captured from the LCOV writer before extraction, including the final LF.
  assert.equal(
    createHash("sha256").update(xml).digest("hex"),
    "e315cd1bece62af0eb82d2dfaf1acd585d2aeb584553e5dbf89c6bb244968b30",
  );
});
