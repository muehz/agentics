import { parse, YAMLError } from "yaml";
import { ReleaseEngineModel } from "../engine-model.ts";

export function releaseEngineModel(): string {
  return ReleaseEngineModel;
}

// A frontmatter value of the wrong type, reported as invalid frontmatter.
class ShapeError extends Error {}

type Mapping = Readonly<Record<string, unknown>>;

function isMapping(value: unknown): value is Mapping {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function valueAt(parent: Mapping | undefined, key: string): unknown {
  return parent !== undefined && Object.hasOwn(parent, key)
    ? parent[key]
    : undefined;
}

function mapAt(parent: Mapping | undefined, key: string): Mapping | undefined {
  const value = valueAt(parent, key);
  if (value === undefined || value === null) return undefined;
  if (!isMapping(value)) throw new ShapeError(`${key} must be a mapping`);
  return value;
}

function textAt(parent: Mapping | undefined, key: string): string {
  const value = valueAt(parent, key);
  if (value === undefined || value === null) return "";
  if (typeof value === "object")
    throw new ShapeError(`${key} must be a scalar`);
  return String(value);
}

function flagAt(parent: Mapping | undefined, key: string): boolean {
  const value = valueAt(parent, key);
  if (value === undefined || value === null) return false;
  if (typeof value !== "boolean")
    throw new ShapeError(`${key} must be a boolean`);
  return value;
}

// A cap must be a YAML integer. The frontmatter is parsed with intAsBigInt, so only an !!int node arrives as a bigint;
// that is what rejects 100.5, 100.0 and "100" while it accepts 0x64.
function creditsMatch(
  parent: Mapping | undefined,
  key: string,
  want: number,
): boolean {
  const value = valueAt(parent, key);
  return typeof value === "bigint" && value === BigInt(want);
}

// Returns every way a release workflow's frontmatter breaks its model and credit policy: the engine and the threat
// detector both run `model`, detection stays on, and the run and the detector are capped at 100 and 50 AI credits. The
// policy is read as YAML, so a key that survives only in a comment or under the wrong parent does not count. An empty
// result means the policy holds.
export function releaseFrontmatterPolicyViolations(
  source: string,
  model: string = releaseEngineModel(),
): string[] {
  const lines = source.replace(/\r\n/g, "\n").split("\n");
  if (lines[0] !== "---") return ["missing workflow frontmatter"];
  let end = 1;
  while (end < lines.length && lines[end] !== "---") end++;
  if (end === lines.length) return ["unclosed workflow frontmatter"];

  const violations: string[] = [];
  try {
    const parsed: unknown = parse(lines.slice(1, end).join("\n"), {
      intAsBigInt: true,
    });
    let doc: Mapping | undefined;
    if (isMapping(parsed)) doc = parsed;
    else if (parsed !== null && parsed !== undefined) {
      throw new ShapeError("the frontmatter must be a mapping");
    }
    const engine = mapAt(doc, "engine");
    const detection = mapAt(mapAt(doc, "safe-outputs"), "threat-detection");
    const detectorEngine = mapAt(detection, "engine");
    if (textAt(engine, "model") !== model)
      violations.push("engine.model must be " + model);
    if (!creditsMatch(doc, "max-ai-credits", 100))
      violations.push("max-ai-credits must be 100");
    if (!flagAt(detection, "enabled")) {
      violations.push("safe-outputs.threat-detection.enabled must be true");
    }
    if (textAt(detectorEngine, "model") !== model) {
      violations.push(
        "safe-outputs.threat-detection.engine.model must be " + model,
      );
    }
    if (!creditsMatch(detection, "max-ai-credits", 50)) {
      violations.push(
        "safe-outputs.threat-detection.max-ai-credits must be 50",
      );
    }
  } catch (error) {
    if (error instanceof YAMLError || error instanceof ShapeError) {
      return [`invalid workflow frontmatter: ${error.message}`];
    }
    throw error;
  }
  return violations;
}
