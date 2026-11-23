// Evaluates the engines.node grammar used by setup-worktree.ts and cloud-setup.sh.
// Reject other forms rather than silently disagreeing with the package manager.
const COMPARATOR = /^(>=|\^)(\d+)\.(\d+)\.(\d+)$/;

type Version = readonly [number, number, number];

interface RangePart {
  operator: ">=" | "^";
  minimum: Version;
}

function parseVersion(text: string | undefined): Version {
  const match = /^(\d+)\.(\d+)\.(\d+)$/.exec(String(text));
  if (!match) throw new Error(`Not a Node version: ${text}`);
  return [Number(match[1]), Number(match[2]), Number(match[3])];
}

function compare(a: Version, b: Version): number {
  return (
    [a[0] - b[0], a[1] - b[1], a[2] - b[2]].find((part) => part !== 0) || 0
  );
}

export function parseRange(range: unknown): RangePart[] {
  return String(range)
    .split("||")
    .map((part) => {
      const match = COMPARATOR.exec(part.trim());
      if (!match) {
        throw new Error(
          `Unsupported Node range "${range}": use alternatives of >=X.Y.Z or ^X.Y.Z, or extend node-range.ts`,
        );
      }
      const minimum: Version = [
        Number(match[2]),
        Number(match[3]),
        Number(match[4]),
      ];
      const operator = part.trim().startsWith("^") ? "^" : ">=";
      // ^0.Y.Z pins the minor, which this evaluator does not model.
      if (operator === "^" && minimum[0] === 0)
        throw new Error(`Unsupported Node range "${range}": ^0.Y.Z`);
      return { operator, minimum };
    });
}

export function satisfies(
  range: unknown,
  version: string | undefined,
): boolean {
  const actual = parseVersion(version);
  return parseRange(range).some(
    ({ operator, minimum }) =>
      compare(actual, minimum) >= 0 &&
      (operator === ">=" || actual[0] === minimum[0]),
  );
}

if (import.meta.main) {
  const [range, version] = process.argv.slice(2);
  try {
    process.exit(satisfies(range, version) ? 0 : 1);
  } catch (error) {
    console.error(error instanceof Error ? error.message : String(error));
    process.exit(2);
  }
}
