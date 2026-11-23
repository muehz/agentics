import assert from "node:assert/strict";
import { writeFileSync } from "node:fs";
import { describe, test } from "node:test";
import { FOOTER_PATTERN, HIGHLIGHTS_HEADING } from "./notes-contract.ts";
import {
  FOOTER,
  newNotesAdapterFixture,
  TARGET_SHA,
} from "./testing/notes-adapter.ts";

const item = (body: string): Record<string, unknown> => ({
  type: "update_release_notes",
  body,
});

// The body the adapter composes for HIGHLIGHTS over the fixture's baseline. The heading and the footer shape come from
// the notes contract, so a drift between the contract and the adapter fails the byte comparison below.
const composed =
  HIGHLIGHTS_HEADING +
  "HIGHLIGHTS\n\n" +
  FOOTER +
  "\n\n<details>\n<summary>Full changelog</summary>\n\nBASELINE NOTES\n\n\n</details>\n";

test("the footer the fixture expects matches the contract's footer pattern", () => {
  assert.match(FOOTER, FOOTER_PATTERN);
  assert.ok(composed.startsWith("## 🌟 Release Highlights\n\nHIGHLIGHTS"));
});

describe("stage-bound release state", () => {
  for (const [name, overrides, diagnostic] of [
    [
      "SHA shape",
      { sha: TARGET_SHA.toUpperCase() },
      /RELEASE_STATE_FILE.sha must/,
    ],
    [
      "SHA binding",
      { sha: "b".repeat(40) },
      /RELEASE_STATE_FILE.sha does not match TARGET_SHA/,
    ],
    ["tag", { tag: "1.2.3" }, /RELEASE_STATE_FILE.tag must/],
    ["empty ID", { release_id: "" }, /RELEASE_STATE_FILE.release_id must/],
    [
      "Unicode digits",
      { release_id: "４２" },
      /RELEASE_STATE_FILE.release_id must/,
    ],
    ["numeric ID", { release_id: 42 }, /RELEASE_STATE_FILE.release_id must/],
    [
      "missing prerelease",
      { prerelease: undefined },
      /RELEASE_STATE_FILE.prerelease must/,
    ],
    [
      "string prerelease",
      { prerelease: "false" },
      /RELEASE_STATE_FILE.prerelease must/,
    ],
  ] as const) {
    test(`validates ${name} before either workflow no-op`, () => {
      const f = newNotesAdapterFixture();
      f.useState(overrides);
      for (const staged of ["true", "false"]) {
        f.env["GH_AW_SAFE_OUTPUTS_STAGED"] = staged;
        const result = f.run();
        assert.equal(result.status, 1, result.output);
        assert.match(result.output, diagnostic);
        assert.deepEqual(f.calls(), []);
      }
    });
  }

  test("validates the checked SHA before staged output", () => {
    const f = newNotesAdapterFixture();
    f.useState();
    f.env["GH_AW_SAFE_OUTPUTS_STAGED"] = "true";
    for (const [sha, diagnostic] of [
      [undefined, /TARGET_SHA is required/],
      [TARGET_SHA.toUpperCase(), /TARGET_SHA must be exactly 40/],
    ] as const) {
      f.env["TARGET_SHA"] = sha;
      const result = f.run();
      assert.equal(result.status, 1, result.output);
      assert.match(result.output, diagnostic);
      assert.deepEqual(f.calls(), []);
    }
  });

  test("a malformed state file cannot fall back to direct inputs", () => {
    const f = newNotesAdapterFixture();
    const path = f.useState();
    for (const [raw, diagnostic] of [
      ["null", /RELEASE_STATE_FILE must contain an object/],
      ["[]", /RELEASE_STATE_FILE must contain an object/],
      ["{}\n{}", /RELEASE_STATE_FILE is not valid JSON/],
    ] as const) {
      writeFileSync(path, raw);
      const result = f.run(item("HIGHLIGHTS"));
      assert.deepEqual(f.calls(), []);
      assert.equal(result.status, 1, result.output);
      assert.match(result.output, diagnostic);
    }
  });

  for (const id of ["0", "01"]) {
    test(`ID ${id} allows staged and unmatched output but fails on an update before GET`, () => {
      const f = newNotesAdapterFixture();
      f.useState({ release_id: id });
      f.env["GH_AW_SAFE_OUTPUTS_STAGED"] = "true";
      const staged = f.run(item("HIGHLIGHTS"));
      assert.equal(staged.status, 0, staged.output);
      assert.match(staged.output, /safe outputs are staged/);
      delete f.env["GH_AW_SAFE_OUTPUTS_STAGED"];
      const unmatched = f.run({ type: "other" });
      assert.equal(unmatched.status, 0, unmatched.output);
      assert.match(unmatched.output, /no matching update_release_notes item/);
      const update = f.run(item("HIGHLIGHTS"));
      assert.deepEqual(f.calls(), []);
      assert.equal(update.status, 1, update.output);
      assert.match(update.output, /must be a positive decimal number/);
    });
  }

  test("the bound artifact supplies the identity for the body-only PATCH and reread", () => {
    const f = newNotesAdapterFixture();
    f.useState({ prerelease: false });
    Object.assign(f.env, {
      RELEASE_ID: "9",
      TAG: "v9.9.9",
      RELEASE_PRERELEASE: "true",
    });
    f.release.prerelease = false;
    f.writeRelease();
    const result = f.run({
      ...item("HIGHLIGHTS"),
      release_id: "9",
      repo: "other/project",
    });
    assert.equal(result.status, 0, result.output);
    assert.equal(f.readRelease().body, composed);
    const endpoint = "repos/muehz/agentics/releases/42";
    assert.deepEqual(f.calls(), [
      { args: ["api", endpoint], input: "" },
      {
        args: ["api", "--method", "PATCH", endpoint, "--input", "-"],
        input: JSON.stringify({ body: composed }),
      },
      { args: ["api", endpoint], input: "" },
    ]);
  });

  test("a remote release that disagrees with the bound tag fails before PATCH", () => {
    const f = newNotesAdapterFixture();
    f.useState({ tag: "v9.9.9" });
    const result = f.run(item("HIGHLIGHTS"));
    assert.equal(result.status, 1, result.output);
    assert.match(result.output, /release identity mismatch/);
    assert.deepEqual(f.calls(), [
      { args: ["api", "repos/muehz/agentics/releases/42"], input: "" },
    ]);
    assert.equal(f.readRelease().body, f.release.body);
  });
});

test("the adapter updates a matching pre-release from the baseline", () => {
  const f = newNotesAdapterFixture();
  const result = f.run(item("HIGHLIGHTS"));
  assert.equal(result.status, 0, result.output);
  assert.equal(f.readRelease().body, composed);
  const calls = f.calls();
  assert.equal(calls.length, 3, `want GET/PATCH/GET: ${JSON.stringify(calls)}`);
  const patch = JSON.parse(calls[1]?.input ?? "") as Record<string, unknown>;
  assert.ok(calls[1]?.args.includes("PATCH"), JSON.stringify(calls[1]));
  assert.deepEqual(Object.keys(patch), ["body"]);
  assert.equal(patch["body"], composed);
  assert.ok(
    !result.output.includes("HIGHLIGHTS") &&
      !result.output.includes("BASELINE NOTES"),
    result.output,
  );
});

test("an already enriched body is left alone after one GET", () => {
  const f = newNotesAdapterFixture();
  f.release.body = composed;
  f.writeRelease();
  const result = f.run(item("HIGHLIGHTS"));
  assert.equal(result.status, 0, result.output);
  assert.equal(f.calls().length, 1);
});

test("staged safe outputs make no gh call", () => {
  const f = newNotesAdapterFixture();
  f.env["GH_AW_SAFE_OUTPUTS_STAGED"] = "true";
  const result = f.run(item("HIGHLIGHTS"));
  assert.equal(result.status, 0, result.output);
  assert.equal(f.calls().length, 0);
});

test("an output with no update_release_notes item makes no gh call", () => {
  const f = newNotesAdapterFixture();
  const result = f.run({ type: "other", body: "HIGHLIGHTS" });
  assert.equal(result.status, 0, result.output);
  assert.equal(f.calls().length, 0);
});

test("two update_release_notes items fail before any gh call", () => {
  const f = newNotesAdapterFixture();
  const result = f.run(item("ONE"), item("TWO"));
  assert.notEqual(result.status, 0);
  assert.match(result.output, /multiple update_release_notes items/);
  assert.equal(f.calls().length, 0);
});

// stage publishes before the notes job runs, so a draft here is a release this run did not publish, whatever its
// tag_name says.
test("a draft release is refused after one GET", () => {
  const f = newNotesAdapterFixture();
  f.release.draft = true;
  f.writeRelease();
  const result = f.run(item("HIGHLIGHTS"));
  assert.notEqual(result.status, 0);
  assert.match(
    result.output,
    /is still a draft; stage publishes it before notes run/,
  );
  assert.equal(f.calls().length, 1);
});

test("a published release under a placeholder tag is refused after one GET", () => {
  const f = newNotesAdapterFixture();
  f.release.tag_name = "untagged-d65d2809ddfe7f913581";
  f.writeRelease();
  const result = f.run(item("HIGHLIGHTS"));
  assert.notEqual(result.status, 0);
  assert.match(
    result.output,
    /is published under placeholder tag untagged-d65d2809ddfe7f913581/,
  );
  assert.equal(f.calls().length, 1);
});

test("a release at another commit is refused without a PATCH", () => {
  const f = newNotesAdapterFixture();
  f.release.target_commitish = "a".repeat(40);
  f.writeRelease();
  const result = f.run(item("HIGHLIGHTS"));
  assert.notEqual(result.status, 0);
  assert.match(result.output, /release identity mismatch/);
  assert.equal(f.calls().length, 1);
});

// A release whose state no longer matches the state stage published was changed by someone after publication, so the
// adapter leaves it alone.
test("a release whose state changed since publication is left alone", () => {
  const f = newNotesAdapterFixture();
  f.release.prerelease = false;
  f.release.body = "human-edited promoted prose";
  f.writeRelease();
  const result = f.run(item("HIGHLIGHTS"));
  assert.equal(result.status, 0, result.output);
  assert.match(result.output, /release state changed since publication; no-op/);
  assert.equal(f.calls().length, 1);
  assert.equal(f.readRelease().body, "human-edited promoted prose");
});

test("the adapter enriches a real release when stage published a real release", () => {
  const f = newNotesAdapterFixture();
  f.env["RELEASE_PRERELEASE"] = "false";
  f.release.prerelease = false;
  f.writeRelease();
  const result = f.run(item("HIGHLIGHTS"));
  assert.equal(result.status, 0, result.output);
  assert.equal(f.readRelease().body, composed);
  assert.equal(f.calls().length, 3);
});

const stateCases: { name: string; value?: string; want: string }[] = [
  { name: "missing", want: "RELEASE_PRERELEASE is required" },
  { name: "empty", value: "", want: "RELEASE_PRERELEASE is required" },
  {
    name: "yes",
    value: "yes",
    want: "RELEASE_PRERELEASE must be true or false",
  },
  {
    name: "upper case",
    value: "TRUE",
    want: "RELEASE_PRERELEASE must be true or false",
  },
];
describe("a missing or malformed RELEASE_PRERELEASE fails before any gh call", () => {
  for (const { name, value, want } of stateCases) {
    test(name, () => {
      const f = newNotesAdapterFixture();
      if (value === undefined) delete f.env["RELEASE_PRERELEASE"];
      else f.env["RELEASE_PRERELEASE"] = value;
      const result = f.run(item("HIGHLIGHTS"));
      assert.notEqual(result.status, 0);
      assert.ok(result.output.includes(want), result.output);
      assert.equal(f.calls().length, 0);
    });
  }
});

// After the PATCH the body is already written, so a state that flipped in that window fails the job.
test("a state that flips during the PATCH fails verification", () => {
  const f = newNotesAdapterFixture();
  f.env["FAKE_GH_FLIP_ON_PATCH"] = "1";
  const result = f.run(item("HIGHLIGHTS"));
  assert.notEqual(result.status, 0);
  assert.match(
    result.output,
    /verification release state changed since publication/,
  );
});

describe("an arbitrary RELEASE_ID fails before any gh call", () => {
  for (const id of ["", "-1", "42;touch", "1/2", "0x2a"]) {
    test(JSON.stringify(id), () => {
      const f = newNotesAdapterFixture();
      f.env["RELEASE_ID"] = id;
      const result = f.run(item("HIGHLIGHTS"));
      assert.notEqual(result.status, 0);
      assert.ok(result.output.includes("RELEASE_ID"), result.output);
      assert.equal(f.calls().length, 0);
    });
  }
});

test("a human-written body is never overwritten", () => {
  const f = newNotesAdapterFixture();
  f.release.body = "human-approved notes";
  f.writeRelease();
  const result = f.run(item("HIGHLIGHTS"));
  assert.equal(result.status, 0, result.output);
  assert.match(result.output, /refusing to overwrite/);
  assert.equal(f.calls().length, 1);
  assert.equal(f.readRelease().body, "human-approved notes");
});

test("a failed PATCH fails the job after two calls", () => {
  const f = newNotesAdapterFixture();
  f.env["FAKE_GH_PATCH_FAIL"] = "1";
  const result = f.run(item("HIGHLIGHTS"));
  assert.notEqual(result.status, 0);
  assert.match(result.output, /PATCH failed/);
  assert.equal(f.calls().length, 2);
});

test("a failed verification GET fails the job after three calls", () => {
  const f = newNotesAdapterFixture();
  f.env["FAKE_GH_REREAD_FAIL"] = "1";
  const result = f.run(item("HIGHLIGHTS"));
  assert.notEqual(result.status, 0);
  assert.match(result.output, /verification GET failed/);
  assert.equal(f.calls().length, 3, JSON.stringify(f.calls()));
});

// The heading is part of the composed shape. A body enriched without it is neither the baseline nor the composed form,
// so the adapter reads it as notes a person wrote and leaves them alone: exit 0, one GET, no PATCH.
// The body keeps the composed suffix and breaks only the heading, so removing the heading check fails this test.
test("a body without the highlights heading is read as human notes", () => {
  const f = newNotesAdapterFixture();
  f.release.body =
    "#".repeat(HIGHLIGHTS_HEADING.length) +
    composed.slice(HIGHLIGHTS_HEADING.length);
  f.writeRelease();
  const result = f.run(item("HIGHLIGHTS"));
  assert.equal(result.status, 0, result.output);
  assert.match(
    result.output,
    /refusing to overwrite notes this run did not compose/,
  );
  assert.equal(f.calls().length, 1);
});
