import { readFileSync, writeFileSync } from "node:fs";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { parseLcov, mergeLcov } from "./lcov.ts";
import { toCobertura } from "./cobertura.ts";
import { convertGoProfile } from "./goprofile.ts";
import { listSourceFiles, summarize } from "./summary.ts";

export function parseInput(value: string): { workspace: string; file: string } {
  const separators: number[] = [];
  for (let index = 0; index < value.length; index++) {
    if (value[index] !== ":") continue;
    const start = separators.length === 0 ? 0 : separators[0]! + 1;
    if (
      index === start + 1 &&
      /^[A-Za-z]$/.test(value[start]!) &&
      /[\\/]/.test(value[index + 1] ?? "")
    )
      continue;
    separators.push(index);
  }
  if (separators.length > 1)
    throw new Error(`ambiguous workspace:lcov input: ${value}`);
  const separator = separators[0];
  if (
    separator === undefined ||
    separator === 0 ||
    separator === value.length - 1
  )
    throw new Error(`expected workspace:lcov input: ${value}`);
  return {
    workspace: value.slice(0, separator),
    file: value.slice(separator + 1),
  };
}

export function main(argv: string[]): void {
  const [command, ...args] = argv;
  if (command === "goprofile") {
    const options = new Map<string, string>();
    for (let index = 0; index < args.length; index += 2) {
      const flag = args[index],
        value = args[index + 1];
      if (
        !["--module", "--profile", "--source-root", "--out"].includes(
          flag ?? "",
        )
      )
        throw new Error(`unknown option: ${flag}`);
      if (!value) throw new Error(`missing value for ${flag}`);
      if (options.has(flag!)) throw new Error(`duplicate option: ${flag}`);
      options.set(flag!, value);
    }
    const module = options.get("--module"),
      profile = options.get("--profile"),
      output = options.get("--out");
    if (!module || !profile || !output)
      throw new Error("goprofile requires --module, --profile and --out");
    writeFileSync(
      output,
      convertGoProfile(readFileSync(profile, "utf8"), {
        module,
        sourceRoot: options.get("--source-root"),
        label: profile,
      }),
    );
    return;
  }
  if (command !== "lcov" && command !== "summary")
    throw new Error(
      "usage: cli.ts lcov --root <repo> --input <workspace>:<lcov> ... --out <file>",
    );
  let root: string | undefined, output: string | undefined;
  const inputs: { workspace: string; file: string }[] = [];
  for (let index = 0; index < args.length; index += 2) {
    const flag = args[index],
      value = args[index + 1];
    if (!value) throw new Error(`missing value for ${flag}`);
    if (flag === "--root") root = resolve(value);
    else if (flag === "--out" && command === "lcov") output = value;
    else if (flag === "--input") inputs.push(parseInput(value));
    else throw new Error(`unknown option: ${flag}`);
  }
  if (command === "summary") {
    if (!root || !inputs.length)
      throw new Error("summary requires --root and --input");
    const reportRoot = root;
    const reports = inputs.map(({ workspace, file }) => ({
      workspace,
      coverage: parseLcov(readFileSync(file, "utf8"), {
        root: reportRoot,
        workspace,
        label: file,
      }),
    }));
    console.log(summarize(reports, listSourceFiles(reportRoot)));
    return;
  }
  if (!root || !output || !inputs.length)
    throw new Error("lcov requires --root, --input and --out");
  const reports = inputs.map(({ workspace, file }) =>
    parseLcov(readFileSync(file, "utf8"), { root, workspace, label: file }),
  );
  writeFileSync(output, toCobertura(mergeLcov(reports)));
}

if (
  process.argv[1] &&
  import.meta.url === pathToFileURL(resolve(process.argv[1])).href
) {
  try {
    main(process.argv.slice(2));
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exitCode = 1;
  }
}
