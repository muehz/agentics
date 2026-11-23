import { writeFileSync } from "node:fs";

// Imported by the test-owned Node executable before it tries to load `audit`.
if (process.env.HOOK_TEST_ARGS) {
  writeFileSync(process.env.HOOK_TEST_ARGS, process.argv.slice(1).join("\n"));
}
writeFileSync(1, process.env.HOOK_TEST_OUTPUT ?? "");
writeFileSync(2, process.env.HOOK_TEST_NOTICE ?? "");
if (process.env.HOOK_TEST_EXIT === "1") {
  writeFileSync(2, "fixture diagnostic");
  process.exit(1);
}
process.exit(0);
