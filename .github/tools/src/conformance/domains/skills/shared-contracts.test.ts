import assert from "node:assert/strict";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import test from "node:test";
import type { RunOptions } from "../../check.ts";
import { mutateFile, writeFile } from "../../testing/files.ts";
import {
  addSkill,
  evalsCheck,
  link,
  mkdir,
  remove,
  withSkillsRepo,
} from "../../testing/skills-repo.ts";
import { writeStandardsBaseline } from "../../testing/standards-baseline.ts";
import { DOMAIN_FAMILIES, run } from "./index.ts";

const options: RunOptions = {
  short: false,
  fast: false,
  diagnostics: () => {},
  env: {},
};

const rule = "shared-contracts";
const contractDir = "skills/routing/references/contracts";
const tddPrinciplesLink =
  "[principles check](../routing/references/contracts/principles-check.md)";
const contracts = {
  "workflow-steps.md":
    "# Workflow steps\n\n## Tracking steps\n\nShared policy.\n",
  "principles-check.md":
    "# Principles check\n\n## Shortlist\n\nShared policy.\n",
  "empirical-questions.md":
    "# Empirical questions\n\n## Cheap probes\n\nShared policy.\n",
};

const workflowCopy =
  "Once the mode is known, add its steps to the session\u0027s todo list after any items already there. Do not replace the list.";
const principlesCopy =
  'Scan the \u0060/principles\u0060 index. Shortlist the principles whose "Applies when" matches the plan or the diff. ' +
  "Read each shortlisted leaf before you act on it. An empty shortlist is a valid result.";
const empiricalCopy =
  "A question whose answer a run can observe (behavior, timing, output shape, whether an approach works) is settled by \u0060prototype\u0060, not by a question to the user. " +
  "Record the observed answer in the plan, cited as a lookup. Run a cheap probe (one command or one throwaway script) unasked. " +
  "For a costly one, ask, and offer the experiment as the recommended answer.";

function seedContracts(root: string): void {
  for (const [filename, content] of Object.entries(contracts))
    writeFile(root, `${contractDir}/${filename}`, content);
}

async function audit(root: string) {
  const family = DOMAIN_FAMILIES.find((item) => item.name === rule);
  assert.ok(family, "the public skills audit must register shared-contracts");
  return run(root, options, { families: [family], evalsCheck });
}

function duplicateAt(path: string, contract: string) {
  return {
    rule,
    path,
    message: `replace the local policy copy with a link to ${contractDir}/${contract}`,
  };
}

function missingLinkAt(path: string, contract: string) {
  return { rule, path, message: `must link to ${contractDir}/${contract}` };
}

function citeContract(consumer: string, filename: string): string {
  const target = path.posix.relative(
    path.posix.dirname(consumer),
    `${contractDir}/${filename}`,
  );
  return `[Shared policy](${target})`;
}

test("the registered audit reports a restored todo policy in an unknown consumer", async () => {
  await withSkillsRepo(async (root) => {
    seedContracts(root);
    addSkill(root, "unlisted", { body: workflowCopy + "\n" });
    writeStandardsBaseline(root);
    const result = await run(root, options, { evalsCheck });
    assert.equal(result.error, undefined);
    assert.ok(
      result.failures.some(
        (failure) =>
          failure.rule === rule &&
          failure.path === "skills/unlisted/SKILL.md" &&
          failure.message ===
            duplicateAt("skills/unlisted/SKILL.md", "workflow-steps.md")
              .message,
      ),
      JSON.stringify(result.failures),
    );
  });
});

test("recognizable rules in an active reference survive headings, lists, emphasis, wrapping and CRLF", async () => {
  await withSkillsRepo(async (root) => {
    seedContracts(root);
    const reference = "skills/unlisted/references/notes.md";
    addSkill(root, "unlisted");
    writeFile(
      root,
      reference,
      [
        "# Notes",
        "",
        "1. **PRINCIPLES CHECK.** " +
          principlesCopy.replace(" Shortlist", "\r\n   Shortlist"),
        "",
        "## The empirical fork",
        "",
        empiricalCopy.replace(" Record", "\r\nRecord"),
        "",
      ].join("\r\n"),
    );
    const result = await audit(root);
    assert.equal(result.error, undefined);
    const atReference = result.failures.filter(
      (failure) => failure.path === reference,
    );
    assert.deepEqual(
      atReference.sort((a, b) => a.message.localeCompare(b.message)),
      [
        duplicateAt(reference, "empirical-questions.md"),
        duplicateAt(reference, "principles-check.md"),
      ],
    );
  });
});

for (const [consumer, contract, harmlessProse] of [
  [
    "skills/tdd/SKILL.md",
    "workflow-steps.md",
    "Track Step 0 and the Verification Checklist.",
  ],
  [
    "skills/deepen/SKILL.md",
    "principles-check.md",
    "This phase still inspects the proposed diff.",
  ],
  [
    "skills/routing/SKILL.md",
    "empirical-questions.md",
    "Hand the observed answer back to the caller.",
  ],
] as const) {
  test(`removing the ${contract} link from ${consumer} fails with no restored copy`, async () => {
    await withSkillsRepo(async (root) => {
      seedContracts(root);
      const skill = consumer.split("/")[1]!;
      const links = Object.keys(contracts).map((name) =>
        citeContract(consumer, name),
      );
      addSkill(root, skill, { body: links.join("\n\n") + "\n" });
      const control = await audit(root);
      assert.equal(control.error, undefined);
      assert.deepEqual(
        control.failures.filter((failure) => failure.path === consumer),
        [],
      );
      mutateFile(
        root,
        consumer,
        citeContract(consumer, contract),
        harmlessProse,
      );
      const result = await audit(root);
      assert.equal(result.error, undefined);
      assert.ok(
        result.failures.some(
          (failure) =>
            failure.rule === rule &&
            failure.path === consumer &&
            failure.message === missingLinkAt(consumer, contract).message,
        ),
        JSON.stringify(result.failures),
      );
    });
  });
}

test("an unrelated Markdown path sharing the basename cannot clear a required link", async () => {
  await withSkillsRepo(async (root) => {
    seedContracts(root);
    const consumer = "skills/tdd/SKILL.md";
    addSkill(root, "tdd", {
      body: `[workflow steps](references/workflow-steps.md)\n\n${tddPrinciplesLink}\n`,
    });
    writeFile(
      root,
      "skills/tdd/references/workflow-steps.md",
      "# An unrelated guide\n",
    );
    const result = await audit(root);
    assert.equal(result.error, undefined);
    assert.deepEqual(
      result.failures.filter((failure) => failure.path === consumer),
      [missingLinkAt(consumer, "workflow-steps.md")],
    );
  });
});

test("quoted contract links, links in code, images, and unmatched labels cannot clear a required link", async () => {
  await withSkillsRepo(async (root) => {
    seedContracts(root);
    const consumer = "skills/tdd/SKILL.md";
    const citation =
      "[workflow steps](../routing/references/contracts/workflow-steps.md)";
    addSkill(root, "tdd", {
      body: [
        tddPrinciplesLink,
        "",
        "> " + citation,
        "",
        `Example: \`${citation}\`.`,
        "",
        "~~~markdown",
        citation,
        "~~~",
        "",
        "!" + citation,
        "",
        citation.slice(1),
        "",
      ].join("\n"),
    });
    const result = await audit(root);
    assert.equal(result.error, undefined);
    assert.deepEqual(
      result.failures.filter((failure) => failure.path === consumer),
      [missingLinkAt(consumer, "workflow-steps.md")],
    );
  });
});

test("checked links, examples, quotes and workflow-specific steps remain valid", async () => {
  await withSkillsRepo(async (root) => {
    seedContracts(root);
    const consumer = "skills/tdd/SKILL.md";
    addSkill(root, "tdd", {
      body: [
        "[Workflow steps](../routing/references/contracts/./workflow-steps.md#tracking-steps)",
        tddPrinciplesLink,
        "",
        "1. Before seeing the report, add Step 0 and the Verification Checklist to this run\u0027s list.",
        "2. When the report format remains undecided, record that item as blocked.",
        "",
        "A worked example can ask how long a single command takes. Record its measured result in the report.",
        "",
        "Example archive text: " +
          String.fromCharCode(96) +
          workflowCopy +
          String.fromCharCode(96),
        "",
        "> " + workflowCopy,
        "",
        "~~~markdown",
        principlesCopy,
        empiricalCopy,
        "~~~",
        "",
      ].join("\n"),
    });
    const result = await audit(root);
    assert.equal(result.error, undefined);
    assert.deepEqual(
      result.failures.filter((failure) => failure.path === consumer),
      [],
    );
  });
});

test("only the owning contract may carry its policy block", async () => {
  await withSkillsRepo(async (root) => {
    seedContracts(root);
    const workflow = `${contractDir}/workflow-steps.md`;
    writeFile(root, workflow, contracts["workflow-steps.md"] + workflowCopy);
    const control = await audit(root);
    assert.equal(control.error, undefined);
    assert.deepEqual(
      control.failures.filter((failure) => failure.path === workflow),
      [],
    );
    writeFile(
      root,
      workflow,
      contracts["workflow-steps.md"] + workflowCopy + "\n\n" + principlesCopy,
    );
    const result = await audit(root);
    assert.equal(result.error, undefined);
    assert.deepEqual(
      result.failures.filter((failure) => failure.path === workflow),
      [duplicateAt(workflow, "principles-check.md")],
    );
  });
});

for (const [filename, broken] of [
  ["empirical-questions.md", "missing"],
  ["principles-check.md", "directory"],
] as const) {
  test(`a ${broken} canonical ${filename} fails the audit`, async () => {
    await withSkillsRepo(async (root) => {
      seedContracts(root);
      const contract = `${contractDir}/${filename}`;
      remove(root, contract);
      if (broken === "directory") mkdir(root, contract);
      const result = await audit(root);
      assert.ok(
        result.error?.startsWith(
          `skills: shared-contracts: reading ${contract}: `,
        ),
        String(result.error),
      );
    });
  });
}

test("a junction or directory link to canonical contracts outside the checkout cannot supply policy", async (t) => {
  const namedTempRoot = path.resolve(tmpdir());
  const outside = mkdtempSync(
    path.join(namedTempRoot, "agentics-shared-contracts-outside-"),
  );
  assert.equal(path.dirname(outside), namedTempRoot);
  t.after(() => rmSync(outside, { recursive: true, force: true }));
  for (const [file, body] of Object.entries(contracts))
    writeFile(outside, file, body);
  await withSkillsRepo(async (root) => {
    remove(root, contractDir);
    if (
      !link(
        root,
        contractDir,
        outside,
        process.platform === "win32" ? "junction" : "dir",
      )
    ) {
      t.skip("the host refused a directory link");
      return;
    }
    const result = await audit(root);
    const prefix = "skills: shared-contracts: reading ";
    assert.ok(result.error?.startsWith(prefix), String(result.error));
    assert.ok(result.error?.includes(`${contractDir}/`), String(result.error));
    assert.ok(
      result.error?.endsWith(`${contractDir}: passes through a link`),
      String(result.error),
    );
  });
});
