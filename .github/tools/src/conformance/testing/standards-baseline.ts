import { writeBaselineFiles } from "../domains/skills/baseline.ts";
import { collectStandardFindings } from "../domains/skills/standards.ts";

/**
 * Writes the baseline that accepts every current standards finding in a fixture, so a whole-domain test sees the
 * fixture pass `skill-standards` the way the real checkout passes it with its committed baseline. A test that then
 * breaks the fixture sees only the new finding.
 */
export function writeStandardsBaseline(root: string): void {
  writeBaselineFiles(root, collectStandardFindings(root));
}
