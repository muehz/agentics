import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { FenceContainer, FenceState } from "./markdown-fence.ts";

interface Stepper {
  step(line: string): boolean;
}

type Case = readonly [name: string, lines: string[], skip: boolean[]];

// Each case drives a fresh tracker through its lines and names, per line, whether the tracker skips it.
function runCases(
  title: string,
  newStepper: () => Stepper,
  cases: readonly Case[],
): void {
  describe(title, () => {
    for (const [name, lines, skip] of cases) {
      it(name, () => {
        assert.equal(lines.length, skip.length, "table row is malformed");
        const stepper = newStepper();
        lines.forEach((line, index) => {
          assert.equal(
            stepper.step(line),
            skip[index],
            `line ${index} ${JSON.stringify(line)}`,
          );
        });
      });
    }
  });
}

// Non-ASCII inputs are built from code points so the source stays ASCII.
const NBSP = String.fromCodePoint(0x00a0);
const E_ACUTE = String.fromCodePoint(0x00e9);

const stateCases: readonly Case[] = [
  ["tilde fence", ["~~~", "## x", "~~~", "## y"], [true, true, true, false]],
  [
    "four backticks hold three",
    ["````", "```", "## x", "````", "## y"],
    [true, true, true, true, false],
  ],
  ["backtick info with backtick", ["``` `x`", "## y"], [false, false]],
  ["four-space indent", ["    ```", "## y"], [false, false]],
  [
    "closer needs same char",
    ["```", "~~~", "## x", "```", "## y"],
    [true, true, true, true, false],
  ],
  [
    "closer with info stays open",
    ["```", "``` go", "## x", "```"],
    [true, true, true, true],
  ],
  [
    "CRLF closer",
    ["```\r", "## x\r", "```\r", "## y\r"],
    [true, true, true, false],
  ],
  [
    "three-space indent opens",
    ["   ```", "## x", "```", "## y"],
    [true, true, true, false],
  ],
  [
    "longer closer closes",
    ["```", "## x", "````", "## y"],
    [true, true, true, false],
  ],
  [
    "shorter closer stays open",
    ["````", "## x", "```", "## y", "````", "## z"],
    [true, true, true, true, true, false],
  ],
  [
    "tilde info with backtick",
    ["~~~ `x`", "## x", "~~~", "## y"],
    [true, true, true, false],
  ],
  ["two tildes are not a fence", ["~~ x", "## y"], [false, false]],
];

const containerCases: readonly Case[] = [
  [
    "list-nested fence",
    ["1. step", "    ```bash", "    [x](a.md)", "    ```", "[y](b.md)"],
    [false, true, true, true, false],
  ],
  [
    "quoted fence ends with quote",
    ["> ```", "> [x](a.md)", "[y](b.md)"],
    [true, true, false],
  ],
  [
    "tab in a list item",
    ["1. step", "\t~~~", "\t[x](a.md)", "\t~~~", "[y](b.md)"],
    [false, true, true, true, false],
  ],
  [
    "top-level 4-space marker",
    ["Para.", "", "    ```", "    code", "[a](a.md)"],
    [false, false, false, false, false],
  ],
  ["top-level tab marker", ["\t~~~", "[a](a.md)"], [false, false]],
  [
    "three-space marker opens",
    ["   ```", "[x](a.md)", "```", "[y](b.md)"],
    [true, true, true, false],
  ],
  [
    "nested bullet fence",
    [
      "- a",
      "  - b",
      "",
      "      ```",
      "      [x](a.md)",
      "      ```",
      "[y](b.md)",
    ],
    [false, false, false, true, true, true, false],
  ],
  [
    "list ends at an outdent",
    ["- a", "", "para", "", "    ```", "[y](b.md)"],
    [false, false, false, false, false, false],
  ],
  [
    "quoted list fence",
    ["> 1. step", ">    ```", ">    [x](a.md)", ">    ```", "[y](b.md)"],
    [false, true, true, true, false],
  ],
  [
    "list ends with its quote",
    ["> - a", "", "    ```", "[y](b.md)"],
    [false, false, false, false],
  ],
  [
    "quoted 4-space marker",
    ["> para", ">     ```", "> [x](a.md)"],
    [false, false, false],
  ],
  [
    "quote line inside fence",
    ["```", "> q", "x", "```", "[y](b.md)"],
    [true, true, true, true, false],
  ],
  [
    "tilde closer needs tilde",
    ["~~~", "```", "x", "~~~", "[y](b.md)"],
    [true, true, true, true, false],
  ],
  [
    "shorter closer stays open",
    ["- a", "  ````", "  x", "  ```", "  y", "  ````", "[z](c.md)"],
    [false, true, true, true, true, true, false],
  ],
  [
    "lazy line after list item",
    ["- a", "lazy", "    ```", "[y](b.md)"],
    [false, false, false, false],
  ],
  [
    "backtick info in list item",
    ["- item", "  ``` `x`", "  [y](b.md)"],
    [false, false, false],
  ],
  [
    "tab after spaces reaches next stop",
    ["- item", "   \t```", "   \tcode", "   \t```", "[y](b.md)"],
    [false, true, true, true, false],
  ],
  [
    "list marker four columns in is code",
    ["    - ```", "[y](b.md)"],
    [false, false],
  ],
  [
    "list marker three columns in opens",
    ["   - ```", "     code", "[y](b.md)"],
    [true, true, false],
  ],
  [
    "plain fence",
    ["a", "```", "b", "```", "c"],
    [false, true, true, true, false],
  ],
  [
    "list fence",
    ["- item", "  ```", "  code", "  ```", "after"],
    [false, true, true, true, false],
  ],
  [
    "quoted fence",
    ["> ```", "> code", "> ```", "after"],
    [true, true, true, false],
  ],
  [
    "tab indent",
    ["- item", "\t```", "\tcode", "\t```"],
    [false, true, true, true],
  ],
  [
    "four spaces after a bullet set the content column",
    ["-    item", "    ```", "[y](b.md)"],
    [false, false, false],
  ],
  [
    "empty ordered item sets a content column",
    ["1.", "    ```", "    x", "    ```", "[y](b.md)"],
    [false, true, true, true, false],
  ],
  [
    "star and plus bullets",
    ["* ```", "  x", "  ```", "+ ~~~", "  x", "  ~~~", "[y](b.md)"],
    [true, true, true, true, true, true, false],
  ],
  [
    "four-digit ordered marker",
    ["1234. ```", "      x", "      ```", "[y](b.md)"],
    [true, true, true, false],
  ],
  [
    "optional space after a quote marker",
    ["> para", ">    ```", "> x", "> ```", "y"],
    [false, true, true, true, false],
  ],
  [
    "nested markers tolerate three spaces",
    [">    > ```", ">    > x", "> [y](b.md)"],
    [true, true, false],
  ],
  [
    "nested markers four spaces apart are code",
    ["> para", ">     > ```", "> [y](b.md)"],
    [false, false, false],
  ],
  ["tab before a quote marker", ["\t> ```", "\t> x", "y"], [true, true, false]],
  [
    "closer measured from the content column",
    ["1. step", "    ```", "    x", "    ```", "   [y](b.md)"],
    [false, true, true, true, false],
  ],
  [
    "closer four columns past the content column",
    ["- a", "  ```", "      ```", "  x", "  ```", "[y](b.md)"],
    [false, true, true, true, true, false],
  ],
  [
    "closer three columns past the content column",
    ["- a", "  ```", "     ```", "  [y](b.md)"],
    [false, true, true, false],
  ],
  [
    "outdent at a deeper quote opens its own fence",
    ["> - item", ">   ```", "> > ```", "> > x"],
    [false, true, true, true],
  ],
];

const listFenceBoundaryCases: readonly Case[] = [
  [
    "outdent ends unclosed fence",
    [
      "1. Example:",
      "    ```markdown",
      "    [example](fake.md)",
      "",
      "[real](real.md)",
    ],
    [false, true, true, true, false],
  ],
  [
    "nested outdent returns to outer item",
    [
      "- outer",
      "  - inner",
      "    ~~~",
      "    example",
      "  [real](real.md)",
      "  ~~~",
      "  example",
      "  ~~~",
      "[after](after.md)",
    ],
    [false, false, true, true, false, true, true, true, false],
  ],
  [
    "same-line bullet fence",
    ["- ```markdown", "  [example](fake.md)", "  ```", "[real](real.md)"],
    [true, true, true, false],
  ],
  [
    "same-line ordered tilde fence",
    ["1) ~~~markdown", "   [example](fake.md)", "   ~~~", "[real](real.md)"],
    [true, true, true, false],
  ],
  [
    "same-line fence ends on outdent",
    ["- ```markdown", "  [example](fake.md)", "[real](real.md)"],
    [true, true, false],
  ],
  [
    "same-line indented code is not fence",
    ["- item", "      ```markdown", "[real](real.md)"],
    [false, false, false],
  ],
  [
    "same-line invalid info is not fence",
    ["- ``` `info`", "  [real](real.md)"],
    [false, false],
  ],
];

const quoteDepthCases: readonly Case[] = [
  [
    "nested quote ends",
    ["> > ```", "> > [example](example.md)", "> [real](real.md)"],
    [true, true, false],
  ],
  ["nested quote drops to zero", ["> > ```", "[real](real.md)"], [true, false]],
  [
    "deeper marker is literal content",
    [
      "> ```",
      "> > literal",
      "> [example](example.md)",
      "> ```",
      "> [real](real.md)",
    ],
    [true, true, true, true, false],
  ],
  [
    "nested quoted list ends",
    ["> > - item", ">     ```", "> [real](real.md)"],
    [false, false, false],
  ],
  [
    "quote markers are stripped",
    [">```", ">x", ">```", "after"],
    [true, true, true, false],
  ],
];

const quotedListLiteralMarkerCases: readonly Case[] = [
  [
    "multiline",
    [
      "> - item",
      ">   ```",
      ">   > literal",
      ">   [example](fake.md)",
      ">   ```",
      "> [real](real.md)",
    ],
    [false, true, true, true, true, false],
  ],
  [
    "same-line",
    [
      "> - ```",
      ">   > literal",
      ">   [example](fake.md)",
      ">   ```",
      "> [real](real.md)",
    ],
    [true, true, true, true, false],
  ],
  [
    "literal fence token",
    [
      "> - ```",
      ">   > ```",
      ">   [example](fake.md)",
      ">   ```",
      "> [real](real.md)",
    ],
    [true, true, true, true, false],
  ],
  [
    "top-level literal fence token",
    ["```", "> ```", "[example](fake.md)", "```", "[real](real.md)"],
    [true, true, true, true, false],
  ],
];

// Whitespace and non-ASCII edges both trackers share: indent counts spaces only but trim()
// strips U+00A0, and a non-ASCII first character is never a marker.
const whitespaceAndNonAsciiCases: readonly Case[] = [
  [
    "marker preceded by U+00A0 is a fence",
    [`${NBSP}\`\`\``, "x", "```", "y"],
    [true, true, true, false],
  ],
  [
    "marker followed by U+00A0 is a fence",
    [`\`\`\`${NBSP}`, "x", "```", "y"],
    [true, true, true, false],
  ],
  [
    "closer followed by U+00A0 closes",
    ["```", "x", `\`\`\`${NBSP}`, "y"],
    [true, true, true, false],
  ],
  [
    "marker with a trailing CR only is a fence",
    ["```\r", "x", "```\r", "y"],
    [true, true, true, false],
  ],
  [
    "non-ASCII first character is not a marker",
    [`${E_ACUTE}\`\`\``, "y"],
    [false, false],
  ],
  [
    "non-ASCII content inside a fence is skipped",
    ["```", `${E_ACUTE}x`, `${E_ACUTE}\`\`\``, "```", "y"],
    [true, true, true, true, false],
  ],
  [
    "tab then a non-ASCII marker is not a marker",
    [`\t${E_ACUTE}\`\`\``, "y"],
    [false, false],
  ],
  [
    "non-ASCII list item text keeps the item open",
    ["- " + E_ACUTE, "  ```", `  ${E_ACUTE}`, "  ```", "y"],
    [false, true, true, true, false],
  ],
];

runCases("FenceState", () => new FenceState(), stateCases);
runCases("FenceContainer", () => new FenceContainer(), containerCases);
runCases(
  "FenceContainer list fence boundaries",
  () => new FenceContainer(),
  listFenceBoundaryCases,
);
runCases(
  "FenceContainer quote depth",
  () => new FenceContainer(),
  quoteDepthCases,
);
runCases(
  "FenceContainer quoted list fence literal markers",
  () => new FenceContainer(),
  quotedListLiteralMarkerCases,
);
runCases(
  "FenceState whitespace and non-ASCII edges",
  () => new FenceState(),
  whitespaceAndNonAsciiCases,
);
runCases(
  "FenceContainer whitespace and non-ASCII edges",
  () => new FenceContainer(),
  whitespaceAndNonAsciiCases,
);

describe("FenceContainer U+00A0 after a list marker", () => {
  const cases: readonly Case[] = [
    [
      "U+00A0 after a bullet marker is not a list marker",
      [`-${NBSP}\`\`\``, "y"],
      [false, false],
    ],
    [
      "same-line fence after a bullet and U+00A0 opens",
      [`- ${NBSP}\`\`\``, "y"],
      [true, false],
    ],
  ];
  for (const [name, lines, skip] of cases) {
    it(name, () => {
      const tracker = new FenceContainer();
      lines.forEach((line, index) => {
        assert.equal(tracker.step(line), skip[index], `line ${index}`);
      });
    });
  }
});

describe("fence mutation kills", () => {
  const cases: readonly (readonly [
    name: string,
    newStepper: () => Stepper,
    lines: string[],
    skip: boolean[],
  ])[] = [
    [
      "a run of another character is not a fence marker",
      () => new FenceState(),
      ["aaa", "## y"],
      [false, false],
    ],
    [
      "a list marker inside a line is not a list item",
      () => new FenceContainer(),
      ["ab```- x", "text"],
      [false, false],
    ],
    [
      "three leading spaces before a quote marker still open a quoted fence",
      () => new FenceContainer(),
      ["   > ```", "   > code", "after"],
      [true, true, false],
    ],
    [
      "an indented line inside a quote keeps its indentation for the list column",
      () => new FenceContainer(),
      [">  - a", ">       ```", "y"],
      [false, true, false],
    ],
    [
      "spaces before a tab count toward the tab stop",
      () => new FenceContainer(),
      ["  \t```", "y"],
      [false, false],
    ],
  ];
  for (const [name, newStepper, lines, skip] of cases) {
    it(name, () => {
      const tracker = newStepper();
      lines.forEach((line, index) => {
        assert.equal(
          tracker.step(line),
          skip[index],
          `line ${index} ${JSON.stringify(line)}`,
        );
      });
    });
  }
});

describe("FenceState open", () => {
  it("follows an opener and a closer", () => {
    const state = new FenceState();
    assert.equal(state.open(), false, "a new tracker is closed");
    state.step("~~~~");
    assert.equal(state.open(), true, "open after an opener");
    state.step("~~~~");
    assert.equal(state.open(), false, "closed after a closer");
  });
});

describe("fence vector tables", () => {
  it("keep their case counts, so a dropped case fails", () => {
    assert.equal(stateCases.length, 12);
    assert.equal(containerCases.length, 35);
    assert.equal(listFenceBoundaryCases.length, 7);
    assert.equal(quoteDepthCases.length, 5);
    assert.equal(quotedListLiteralMarkerCases.length, 4);
  });
});
