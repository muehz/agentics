/** The ledger's path in a docs fixture. */
export const LEDGER = "docs/surfaces/README.md";

/** A probe stamp to append to a surface doc so `probe-stamp-present` stays quiet. */
export const STAMP = "\n\nVerified on Cursor 3.15.6 (2026-08-09).\n";

/**
 * A ledger in the six-column form, with every surface reviewed on 2026-09-12 and a probe noted, except codex, which is
 * reviewed on `codexStamp` with `result` in its result column.
 */
export const sixColumnLedger = (codexStamp: string, result: string): string =>
  [
    "| Surface | Agentics doc | Upstream requirement sources | Last reviewed | Requirements review / retained probe | Result |",
    "| --- | --- | --- | --- | --- | --- |",
    ...["claude-code", "codex", "copilot-cli", "cursor"].map(
      (name) =>
        `| ${name} | [${name}.md](${name}.md) | [docs](https://example.com/${name}) | ${name === "codex" ? codexStamp : "2026-09-12"} | Published docs; retained probe: CLI 1.0 (2026-08-09) | ${name === "codex" ? result : "No drift."} |`,
    ),
    "",
  ].join("\n");

/** The rules a docs fixture reports when every one of its four ledger rows is older than the age budget. */
export const staleRows = [
  "review-age",
  "review-age",
  "review-age",
  "review-age",
];
