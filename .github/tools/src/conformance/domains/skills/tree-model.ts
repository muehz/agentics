import {
  assetFiles,
  noSkillTextError,
  readRepoFile,
  seams,
  visitSkillText,
} from "./files.ts";
import {
  proseLines,
  skillDocs,
  type ProseLine,
  type SkillDoc,
} from "./xref-md.ts";

type Result<T> = { value: T } | { error: unknown };

/** A failed observation belongs to the same run as a successful one. Callers still decide how to report it. */
function cached<T>(read: (key: string) => T): (key: string) => T {
  const results = new Map<string, Result<T>>();
  return (key) => {
    let result = results.get(key);
    if (!result) {
      try {
        result = { value: read(key) };
      } catch (error) {
        result = { error };
      }
      results.set(key, result);
    }
    if ("error" in result) throw result.error;
    return result.value;
  };
}

/** Longest-first literal alternatives used by the command, mode and step grammars. */
export function skillNamePattern(declarationKeys: Iterable<string>): string {
  return [...declarationKeys]
    .sort((a, b) => b.length - a.length)
    .map((name) => name.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"))
    .join("|");
}

function hideComments(text: string): string {
  return text.replace(/<!--[\s\S]*?(?:-->|$)/g, (comment) =>
    comment.replace(/[^\r\n]/g, " "),
  );
}

type Visitor = (rel: string, text: string) => void;
type Selection = { rows: Map<string, string>; fault?: { error: unknown } };

/** The texts, selections and Markdown projections observed by one skills audit. Construction performs no I/O. */
export class SkillsTreeModel {
  readonly #root: string;
  readonly #physical = cached((file) => seams.readFile(file));
  readonly read: (rel: string) => string;
  readonly lines: (text: string) => ProseLine[];
  #selection?: Selection;
  #docs?: Map<string, SkillDoc>;
  readonly #declarationFiles = cached(assetFiles);

  constructor(root: string, parseLines = proseLines) {
    this.#root = root;
    // Explicit paths validate every component even if an enumerated leaf already supplied the bytes.
    this.read = cached((rel) =>
      readRepoFile(root, rel, { readFile: this.#physical }),
    );
    this.lines = cached(parseLines);
  }

  declarationFiles(): readonly string[] {
    return this.#declarationFiles(this.#root);
  }

  /** Replays the visited prefix before a walk fault. A visitor that aborts owns its exception and stops I/O. */
  walkText(audit: string, visit: Visitor): void {
    let selected = this.#selection;
    if (selected) {
      for (const [rel, text] of selected.rows) visit(rel, text);
    } else {
      const rows = new Map<string, string>();
      selected = { rows };
      let visiting = false;
      try {
        visitSkillText(
          this.#root,
          (rel, text) => {
            rows.set(rel, text);
            visiting = true;
            visit(rel, text);
            visiting = false;
          },
          { readFile: this.#physical },
        );
      } catch (error) {
        if (visiting) throw error;
        selected.fault = { error };
      }
      this.#selection = selected;
    }
    if (selected.fault) throw selected.fault.error;
    if (!selected.rows.size) throw noSkillTextError(audit);
  }

  docs(): ReadonlyMap<string, SkillDoc> {
    return (this.#docs ??= skillDocs(this.#root, this));
  }

  /** Shared policy copies cannot join paragraphs across a removed fence. Quoted lines leave a blank. */
  contractProse(text: string): string {
    const out: string[] = [];
    let previous = 0;
    for (const line of this.lines(hideComments(text))) {
      if (line.number > previous + 1) out.push("");
      out.push(/^\s*>/.test(line.text) ? "" : line.text);
      previous = line.number;
    }
    return out.join("\n");
  }

  /** Question templates inspect visible fenced text too. Their prose joins the remaining lines across a fence. */
  questionProse(text: string): { visible: string; prose: string } {
    const visible = hideComments(text).replace(/^[ \t]*>[^\r\n]*/gm, "");
    const prose = this.lines(visible)
      .map((line) => (/^\s*>/.test(line.text) ? "" : line.text))
      .join("\n");
    return { visible, prose };
  }
}
