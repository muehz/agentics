export const SOURCE_ROOTS = [".github/tools/src"] as const;
export const MUTATION_CONFIG_FILE =
  ".github/tools/src/scripts/mutation-config.ts";
export const MUTATION_CONFIG_FILES: readonly string[] = [
  MUTATION_CONFIG_FILE,
  ".github/tools/src/scripts/mutation-config-values.ts",
  ".github/tools/src/scripts/mutation-source.ts",
];

export function isMutationConfigFile(file: string): boolean {
  return MUTATION_CONFIG_FILES.includes(file);
}

const TEST_SUFFIX = ".test.ts";
const TESTING_DIR = "testing";
export const EXCLUDE_PATTERNS = [
  `**/*${TEST_SUFFIX}`,
  `**/${TESTING_DIR}/**`,
] as const;

export function strykerSourceGlobs(dir: string): string[] {
  return [
    `${dir}/**/*.ts`,
    ...EXCLUDE_PATTERNS.map((pattern) => `!${dir}/${pattern}`),
  ];
}

// Coverage's glob matcher uses "." for "**", which does not span line breaks in a path.
const coverageExclusions = EXCLUDE_PATTERNS.map(
  (pattern) =>
    new RegExp(
      `^${pattern
        .split("**")
        .map((part) =>
          part
            .split("*")
            .map((text) => text.replace(/[.+?^${}()|[\]\\]/g, "\\$&"))
            .join("[^/]*"),
        )
        .join(".*")}$`,
    ),
);

export function isProductionSource(
  file: string,
  policy: "coverage" | "stryker",
): boolean {
  if (!SOURCE_ROOTS.some((root) => file.startsWith(`${root}/`))) return false;
  if (policy === "coverage")
    return !coverageExclusions.some((pattern) => pattern.test(file));
  return (
    file.endsWith(".ts") &&
    !file.endsWith(TEST_SUFFIX) &&
    !file.split("/").includes(TESTING_DIR) &&
    !isMutationConfigFile(file)
  );
}

export const GO_MODULE = "hooks/go/";

export function isGremlinsSource(file: string): boolean {
  return (
    file.startsWith(GO_MODULE) &&
    file.endsWith(".go") &&
    !file.endsWith("_test.go") &&
    !file.split("/").includes("testdata")
  );
}
