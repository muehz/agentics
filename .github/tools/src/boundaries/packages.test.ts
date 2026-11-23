import assert from "node:assert/strict";
import { test } from "node:test";
import { packageViolations } from "./packages.ts";

const root = { workspaces: [".github/tools"] };
const pkg = (name: string, extra: Record<string, unknown> = {}) => ({
  name,
  ...extra,
});
const clean = () => ({ tools: pkg("agentics-tools") });

test("a clean workspace set has no violations", () => {
  assert.deepEqual(packageViolations(root, clean()), []);
});

test("third-party dependencies and workspace ranges are ignored", () => {
  for (const spec of ["^5.0.0", "workspace:*", "workspace:^", "workspace:~"]) {
    const packages = {
      tools: pkg("agentics-tools", { dependencies: { typescript: spec } }),
    };
    assert.deepEqual(packageViolations(root, packages), [], spec);
  }
});

test("a file:, link:, or workspace: path is a violation in any dependency field", () => {
  for (const field of [
    "dependencies",
    "devDependencies",
    "peerDependencies",
    "optionalDependencies",
  ]) {
    for (const spec of [
      "file:../../evals",
      "link:../../evals",
      "workspace:../../evals",
    ]) {
      const violations = packageViolations(root, {
        tools: pkg("agentics-tools", { [field]: { shared: spec } }),
      });
      assert.equal(violations.length, 1, `${field} ${spec}`);
      assert.equal(violations[0]!.rule, "area-ts-dependency");
      assert.equal(violations[0]!.path, ".github/tools/package.json");
      assert.equal(
        violations[0]!.message,
        `${field} entry shared uses ${spec}, which targets evals; tools depends on no local directory`,
      );
    }
  }
});

test("a local path is judged after normalising, and an absolute path is a violation", () => {
  const sneaky = packageViolations(root, {
    tools: pkg("agentics-tools", {
      dependencies: { shared: "file:./x/../../../hooks" },
    }),
  });
  assert.equal(sneaky.length, 1);
  assert.match(sneaky[0]!.message, /targets hooks;/);
  const own = packageViolations(root, {
    tools: pkg("agentics-tools", { dependencies: { shared: "file:./vendor" } }),
  });
  assert.match(own[0]!.message, /targets tools;/);
  assert.equal(
    packageViolations(root, {
      tools: pkg("agentics-tools", { dependencies: { shared: "file:/abs" } }),
    }).length,
    1,
  );
});

test("evals, hooks, lib or skills as a workspace, and a missing package.json, are violations", () => {
  for (const extra of ["evals", "hooks", "lib", "skills"]) {
    const withExtra = packageViolations(
      { workspaces: [".github/tools", extra] },
      clean(),
    );
    assert.equal(withExtra.length, 1, extra);
    assert.equal(withExtra[0]!.path, "package.json");
    assert.equal(
      withExtra[0]!.message,
      `workspaces must be exactly .github/tools; found .github/tools, ${extra}`,
    );
  }
  const missing = packageViolations(root, { tools: null });
  assert.equal(missing.length, 1);
  assert.equal(missing[0]!.path, ".github/tools/package.json");
});

test("a workspaces list that omits tools, or is absent, is a violation", () => {
  assert.equal(packageViolations({ workspaces: [] }, clean()).length, 1);
  assert.equal(
    packageViolations({}, clean())[0]!.message,
    "workspaces must be exactly .github/tools; found none",
  );
});
