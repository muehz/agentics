import assert from "node:assert/strict";
import { spawnSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { test, type TestContext } from "node:test";
import { writeFile } from "../../testing/files.ts";
import { git } from "../../testing/git-checkout.ts";
import {
  contractFixtureMD,
  withSkillsRepo,
} from "../../testing/skills-repo.ts";
import { assetFiles, seams, walkSkillText, withSeams } from "./files.ts";
import { DOMAIN_FAMILIES, run } from "./index.ts";
import { questionFormatRules } from "./question-format.ts";
import { FLOW_STATE_PATH, RECORD_KEEPING_FILES } from "./record-keeping.ts";
import { FAMILY as sharedContracts } from "./shared-contracts.ts";
import { readDeclarations } from "./skill-meta.ts";
import { collectStandardFindings } from "./standards.ts";
import { SkillsTreeModel, skillNamePattern } from "./tree-model.ts";
import { proseLines, skillDocs } from "./xref-md.ts";

function fixture(t: TestContext): string {
  const root = mkdtempSync(path.join(tmpdir(), "agentics-skills-model-"));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  return root;
}

const skill = "skills/alpha/SKILL.md";
const reference = "skills/only-refs/references/notes.md";

test("reference links and record-keeping pins read each shared source once", async () => {
  await withSkillsRepo(async (root) => {
    writeFile(
      root,
      FLOW_STATE_PATH,
      "[Broken](unwritten.md)\ncarries that approval forward\n",
    );
    const counts = new Map<string, number>();
    const read = seams.readFile;
    const result = await withSeams(
      {
        readFile(file) {
          const rel = path.relative(root, file).split(path.sep).join("/");
          counts.set(rel, (counts.get(rel) ?? 0) + 1);
          return read(file);
        },
      },
      () =>
        run(
          root,
          {
            short: false,
            fast: false,
            env: process.env,
            diagnostics: () => {},
          },
          {
            families: DOMAIN_FAMILIES.filter(
              (f) =>
                f.name === "reference-link" || f.name === "gate-record-keeping",
            ),
          },
        ),
    );
    assert.deepEqual(result, {
      failures: [
        {
          rule: "reference-link",
          path: FLOW_STATE_PATH,
          message: "dangling link -> unwritten.md",
        },
        {
          rule: "gate-record-keeping",
          path: FLOW_STATE_PATH,
          message:
            FLOW_STATE_PATH +
            ' carries "carries that approval forward" again; gate never carries an approval over its own edit',
        },
      ],
    });
    assert.deepEqual(
      RECORD_KEEPING_FILES.map((rel) => counts.get(rel)),
      [1, 1, 1, 1],
    );
  });
});

test("standalone text families load and audit without the YAML dependency", (t) => {
  const root = fixture(t);
  writeFile(root, skill, "events.tsv");
  const families = [
    "legacy-names",
    "independence",
    "references",
    "shell-data",
    "waive-operands",
  ];
  const probe = `
    import { registerHooks } from "node:module";
    registerHooks({resolve(specifier, context, next) {
      if (specifier === "yaml")
        throw Object.assign(new Error("Cannot find package 'yaml' imported from " + context.parentURL), {code: "ERR_MODULE_NOT_FOUND"});
      return next(specifier, context);
    }});
    const [root, legacyNames, ...others] = process.argv.slice(1);
    const {legacyNamesRules} = await import(legacyNames);
    for (const family of others) await import(family);
    process.stdout.write(JSON.stringify(legacyNamesRules(root)));
  `;
  const result = spawnSync(
    process.execPath,
    [
      "--input-type=module",
      "-e",
      probe,
      root,
      ...families.map(
        (family) => new URL(`./${family}.ts`, import.meta.url).href,
      ),
    ],
    { encoding: "utf8" },
  );
  assert.ifError(result.error);
  assert.equal(result.status, 0, result.stderr);
  assert.deepEqual(JSON.parse(result.stdout), [
    {
      rule: "skill-legacy-names",
      path: skill,
      message:
        'line 1 names "events.tsv": this state mechanism is retired; use state.tsv and the flow helper',
    },
  ]);
});

test("standards and tracked inbound anchors share one target read and prose parse per domain run", async (t) => {
  const root = fixture(t);
  const source = "README.md";
  const target = "## Don't use when\n/alpha\n## References\n";
  writeFile(root, skill, target);
  writeFile(root, source, `[good](${skill}#references) [bad](${skill}#absent)`);
  git(root, "init", "--quiet");
  git(root, "add", "--", source, skill);
  const options = {
    short: false,
    fast: false,
    env: process.env,
    diagnostics: () => {},
  };
  const families = DOMAIN_FAMILIES.filter(
    (f) => f.name === "skill-standards" || f.name === "skill-inbound-anchor",
  );
  let reads = 0;
  let parses = 0;
  const read = seams.readFile;
  await withSeams(
    {
      readFile(file) {
        if (file === path.join(root, skill)) reads++;
        return read(file);
      },
    },
    async () => {
      const parseLines = (text: string) => {
        if (text === target) parses++;
        return proseLines(text);
      };
      assert.deepEqual(await run(root, options, { families: [], parseLines }), {
        failures: [],
      });
      assert.equal(reads, 0);
      assert.equal(parses, 0);
      const first = await run(root, options, { families, parseLines });
      const bare = {
        rule: "skill-command-bare",
        path: skill,
        message:
          "line 2: /alpha is a bare skill command; write /agentics:alpha",
      };
      assert.deepEqual(first, {
        failures: [
          bare,
          {
            rule: "skill-inbound-anchor",
            path: source,
            message: `${source} links to ${skill}#absent, which names no heading there`,
          },
        ],
      });
      assert.equal(reads, 1);
      assert.equal(parses, 1);
      // Both families must consume the common parse: this line removes the bare command and supplies the missing heading.
      const projected = await run(root, options, {
        families,
        parseLines: (text) =>
          proseLines(
            text === target ? text.replace("/alpha", "# Absent") : text,
          ),
      });
      assert.deepEqual(projected, { failures: [] });
      writeFile(root, skill, target + "# Absent\n");
      assert.deepEqual(await run(root, options, { families, parseLines }), {
        failures: [bare],
      });
      assert.equal(reads, 3);
      await withSeams(
        {
          readFile(file) {
            if (file === path.join(root, source))
              throw Object.assign(new Error(), { code: "EACCES" });
            return read(file);
          },
        },
        async () => {
          assert.deepEqual(await run(root, options, { families }), {
            failures: [bare],
            error: `${source}: EACCES`,
          });
        },
      );
    },
  );
});

test("text and Markdown projections preserve walk order, skipped inputs and target-only docs", (t) => {
  const root = fixture(t);
  for (const [rel, text] of [
    [reference, "# Notes"],
    ["skills/README.md", "skip"],
    ["skills/loose.md", "top-level"],
    ["skills/alpha/scripts/tool.sh", ""],
    ["skills/alpha/assets/target.md", "# Target"],
    ["skills/alpha/binary.md", "binary\0"],
    [skill, "\ufeff# Alpha\r\n"],
  ] as const)
    writeFile(root, rel, text);
  const old: [string, string][] = [];
  walkSkillText(root, "comparison", (rel, text) => old.push([rel, text]));
  const model = new SkillsTreeModel(root);
  const rows: [string, string][] = [];
  model.walkText("comparison", (rel, text) => rows.push([rel, text]));
  assert.deepEqual(rows, old);
  assert.deepEqual(rows, [
    [skill, "\ufeff# Alpha\r\n"],
    ["skills/alpha/assets/target.md", "# Target"],
    ["skills/alpha/scripts/tool.sh", ""],
    ["skills/loose.md", "top-level"],
    [reference, "# Notes"],
  ]);
  assert.deepEqual([...model.docs()], [...skillDocs(root)]);
  assert.deepEqual(
    [...model.docs().values()].map((doc) => [doc.rel, doc.skill, doc.scanned]),
    [
      [skill, "alpha", true],
      ["skills/alpha/assets/target.md", "alpha", false],
      [reference, "only-refs", true],
    ],
  );
});

test("grammar follows declaration directories used by the command, mode and step rules", (t) => {
  const root = fixture(t);
  const names = ["a", "a-long", "a.b"];
  for (const name of names)
    writeFile(root, `skills/${name}/SKILL.md`, "---\nname: unrelated\n---\n");
  const candidates = [...names, "axb", "only-refs", "unrelated"];
  writeFile(
    root,
    reference,
    candidates
      .map((name) => `/${name} /agentics:${name} lost; ${name} Step 99`)
      .join("\n"),
  );
  const model = new SkillsTreeModel(root);
  assert.ok(model.docs().has(reference));
  assert.deepEqual(model.declarationFiles(), assetFiles(root));
  const { declarations } = readDeclarations(root);
  const pattern = skillNamePattern(declarations.keys());
  assert.equal(pattern, "a-long|a\\.b|a");
  const matcher = new RegExp(`^(${pattern})$`);
  const matched = candidates.filter((name) => matcher.test(name));
  assert.deepEqual(matched, names);
  const findings = collectStandardFindings(root).filter(
    (f) => f.path === reference,
  );
  for (const [rule, keys] of [
    ["skill-command-bare", matched.map((n) => `/${n}`)],
    ["skill-xref-mode", matched.map((n) => `/agentics:${n} lost`)],
    ["skill-xref-step", matched.map((n) => `${n} Step 99`)],
  ] as const)
    assert.deepEqual(
      findings.filter((f) => f.rule === rule).map((f) => f.key),
      keys,
    );
});

test("question and shared contract policies preserve different fence gaps and visible text", async (t) => {
  const root = fixture(t);
  const contracts = "skills/routing/references/contracts";
  for (const name of [
    "workflow-steps",
    "principles-check",
    "empirical-questions",
  ])
    writeFile(root, `${contracts}/${name}.md`, "# Contract");
  writeFile(root, `${contracts}/question-format.md`, contractFixtureMD());
  const text = [
    "add this run's steps",
    "```",
    "<question title> <question body>",
    "<your recommended answer>",
    "```",
    "to the todo list",
    "> quoted",
    "a<!-- hidden -->b",
  ].join("\r\n");
  writeFile(root, reference, text);
  const model = new SkillsTreeModel(root);
  const question = model.questionProse(text);
  assert.ok(question.visible.includes("<your recommended answer>"));
  const hidden = " ".repeat("<!-- hidden -->".length);
  assert.equal(
    question.prose,
    `add this run's steps\nto the todo list\n\na${hidden}b`,
  );
  assert.equal(
    model.contractProse(text),
    `add this run's steps\n\nto the todo list\n\na${hidden}b`,
  );
  assert.deepEqual(
    questionFormatRules(root).filter((f) => f.path === reference),
    [
      {
        rule: "question-format",
        path: reference,
        message: `replace the local decision-question format with a link to ${contracts}/question-format.md`,
      },
    ],
  );
  const shared = await sharedContracts.run(root, {
    families: [],
    evalsCheck: () => [],
  });
  assert.deepEqual(
    shared.filter((f) => f.path === reference),
    [],
  );
});

test("unused projections stay lazy and explicit and walked selections reuse reads and prose within one model", (t) => {
  const root = fixture(t);
  writeFile(root, skill, "# Old");
  writeFile(root, reference, "# Unused");
  const reads: string[] = [];
  const parses: string[] = [];
  let directoryReads = 0;
  let declarationListings = 0;
  const { readDir, readSkillDirectory, readFile } = seams;
  withSeams(
    {
      readFile(file) {
        reads.push(path.relative(root, file).split(path.sep).join("/"));
        return readFile(file);
      },
      readDir(dir) {
        directoryReads++;
        return readDir(dir);
      },
      readSkillDirectory(dir) {
        declarationListings++;
        return readSkillDirectory(dir);
      },
    },
    () => {
      const model = new SkillsTreeModel(root, (text) => {
        parses.push(text);
        return proseLines(text);
      });
      assert.deepEqual(reads, []);
      assert.equal(directoryReads, 0);
      assert.equal(declarationListings, 0);
      assert.deepEqual(model.declarationFiles(), [skill]);
      assert.ok(declarationListings > 0);
      assert.deepEqual(reads, []);
      const target = model.read(skill);
      assert.deepEqual(reads, [skill]);
      assert.equal(directoryReads, 0);
      const doc = model.docs().get(skill);
      assert.equal(doc?.text, target);
      assert.deepEqual(model.lines(target), [{ number: 1, text: "# Old" }]);
      assert.equal(model.questionProse(target).prose, "# Old");
      assert.equal(model.contractProse(target), "# Old");
      assert.deepEqual(parses, ["# Old"]);
      const walked = directoryReads;
      assert.ok(walked > 0);
      model.walkText("other", () => {});
      assert.equal(directoryReads, walked);
      assert.deepEqual(reads, [skill, reference]);
      writeFile(root, skill, "# New");
      assert.equal(model.read(skill), "# Old");
      assert.equal(new SkillsTreeModel(root).docs().get(skill)?.text, "# New");
      assert.deepEqual(reads, [skill, reference, skill, reference]);
    },
  );
});

test("a visitor can stop the first walk before the next file without poisoning later visitors", (t) => {
  const root = fixture(t);
  writeFile(root, skill, "first");
  writeFile(root, reference, "second");
  const reads: string[] = [];
  const read = seams.readFile;
  withSeams(
    {
      readFile(file) {
        reads.push(file);
        return read(file);
      },
    },
    () => {
      const model = new SkillsTreeModel(root);
      const stop = new Error("stop visiting");
      assert.throws(
        () =>
          model.walkText("first", () => {
            throw stop;
          }),
        (error) => error === stop,
      );
      assert.deepEqual(reads, [path.join(root, skill)]);
      assert.deepEqual([...model.docs().keys()], [skill, reference]);
      assert.deepEqual(reads, [
        path.join(root, skill),
        path.join(root, reference),
      ]);
    },
  );
});

test("missing and empty selections retain the existing audit dispositions", (t) => {
  const root = fixture(t);
  const missing = new SkillsTreeModel(root);
  for (const read of [() => missing.docs(), () => skillDocs(root)])
    assert.throws(read, {
      name: "RepoFileError",
      message: "skills: does not exist",
    });
  writeFile(root, "skills/README.md", "exempt");
  writeFile(root, "skills/bin", "\0");
  const model = new SkillsTreeModel(root);
  for (const audit of ["independence", "skill standards"])
    for (const run of [
      () => walkSkillText(root, audit, () => {}),
      () => model.walkText(audit, () => {}),
    ])
      assert.throws(run, {
        message: `skills: no files under skills/ to audit for ${audit}`,
      });
  assert.throws(() => model.read(skill), {
    name: "RepoFileError",
    message: "skills/alpha: does not exist",
  });
  writeFile(root, skill, "new");
  assert.throws(() => model.read(skill), {
    message: "skills/alpha: does not exist",
  });
  assert.equal(new SkillsTreeModel(root).read(skill), "new");
});

test("a failed read retains the old visit prefix and fault for each family", (t) => {
  const root = fixture(t);
  writeFile(root, skill, "first");
  writeFile(root, reference, "unreadable");
  let attempts = 0;
  const read = seams.readFile;
  withSeams(
    {
      readFile(file) {
        if (file !== path.join(root, reference)) return read(file);
        attempts += 1;
        throw Object.assign(new Error(), { code: "EACCES" });
      },
    },
    () => {
      const model = new SkillsTreeModel(root);
      const error = { name: "RepoFileError", message: `${reference}: EACCES` };
      for (const walk of [
        walkSkillText.bind(null, root),
        model.walkText.bind(model),
        model.walkText.bind(model),
      ]) {
        const visited: string[] = [];
        assert.throws(
          () => walk("comparison", (rel) => visited.push(rel)),
          error,
        );
        assert.deepEqual(visited, [skill]);
      }
      assert.throws(() => model.read(reference), error);
      assert.equal(attempts, 2);
    },
  );
});
