import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import { test } from "node:test";
import { fileURLToPath } from "node:url";

// The tool must never carry a flag that skips a safety boundary. The reason the hook-trust bypass is absent is in
// .github/tools/README.md, so this file and the sources under test do not repeat the flag's name in prose.
test("the tool sources carry no bypass flags", () => {
  const dir = fileURLToPath(new URL(".", import.meta.url));
  const forbidden = [
    "--dangerously-bypass-hook-trust",
    "--dangerously-skip-permissions",
    "stream-json",
  ];
  const sources = readdirSync(dir).filter(
    (name) => name.endsWith(".ts") && !name.endsWith(".test.ts"),
  );
  assert.ok(
    sources.length >= 10,
    `expected the tool sources, found ${sources.join(", ")}`,
  );
  for (const name of sources) {
    const text = readFileSync(`${dir}${name}`, "utf8");
    for (const flag of forbidden)
      assert.equal(text.includes(flag), false, `${name} contains ${flag}`);
  }
});
