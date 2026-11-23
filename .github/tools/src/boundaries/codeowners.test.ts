import assert from "node:assert/strict";
import { test } from "node:test";
import { codeownersViolations } from "./codeowners.ts";

const FULL = [
  "# comment",
  "* @owner",
  "",
  "/hooks/ @owner",
  "/evals/ @owner",
  "/.github/tools/ @owner # trailing comment",
  "/skills/ @owner",
].join("\n");

test("an entry per area passes", () => {
  assert.deepEqual(codeownersViolations(FULL), []);
});

test("each missing area is its own violation, and an entry with no owner counts as missing", () => {
  const text = FULL.replace(
    "/.github/tools/ @owner # trailing comment",
    "/.github/tools/",
  )
    .replace("/skills/ @owner\n", "")
    .replace("/skills/ @owner", "");
  const violations = codeownersViolations(text);
  assert.deepEqual(
    violations.map((v) => v.message),
    ["no owner entry for /.github/tools/", "no owner entry for /skills/"],
  );
  assert.ok(
    violations.every(
      (v) => v.rule === "area-codeowners" && v.path === "CODEOWNERS",
    ),
  );
});

test("every area is checked", () => {
  const violations = codeownersViolations("* @owner\n");
  assert.deepEqual(
    violations.map((v) => v.message),
    [
      "no owner entry for /hooks/",
      "no owner entry for /evals/",
      "no owner entry for /.github/tools/",
      "no owner entry for /skills/",
    ],
  );
});

test("commented-out entries and an owner that is only a comment do not count", () => {
  const text = FULL.replace("/hooks/ @owner", "# /hooks/ @owner").replace(
    "/evals/ @owner",
    "/evals/ # @owner",
  );
  assert.deepEqual(
    codeownersViolations(text).map((v) => v.message),
    ["no owner entry for /hooks/", "no owner entry for /evals/"],
  );
});

test("CRLF line endings are accepted", () => {
  assert.deepEqual(codeownersViolations(FULL.replace(/\n/g, "\r\n")), []);
});
