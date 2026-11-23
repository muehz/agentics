import assert from "node:assert/strict";
import { test } from "node:test";
import { auditRepo, withRepoRepo } from "./repo-repo.ts";

test("auditRepo finds no failures and no error on the clean fixture", async () => {
  await withRepoRepo(async (root) => {
    assert.deepEqual(await auditRepo(root), { failures: [] });
  });
});
