import { mkdirSync, symlinkSync } from "node:fs";
import path from "node:path";
import {
  CLAUDE_CATALOG,
  CODEX_PLUGIN,
  COPILOT_PLUGIN,
  editJson,
  mkdir,
  remove,
  writeFile,
} from "./manifests-repo.ts";

/** The absolute path of the slash-separated `rel` inside `root`. */
export const fixturePath = (root: string, rel: string): string =>
  path.join(root, ...rel.split("/"));

/** Makes `rel` inside `root` a symlink of `type` to `target`, creating its parent directory first. */
export const link = (
  root: string,
  target: string,
  rel: string,
  type: "dir" | "file",
): void => {
  mkdirSync(path.dirname(fixturePath(root, rel)), { recursive: true });
  symlinkSync(target, fixturePath(root, rel), type);
};

/** A JSON array nested `n` deep. */
export const nest = (n: number): string => "[".repeat(n) + "]".repeat(n);

/** A Claude catalog text whose plugin list holds the JSON text `entry`. */
export const claudeCatalog = (entry: string): string =>
  `{"name":"muehz","plugins":[${entry}]}`;

/** An agentics catalog entry with the JSON members `fields` (each followed by a comma) before its `./` source. */
export const agenticsEntry = (fields: string): string =>
  `{"name":"agentics",${fields}"source":"./"}`;

/** A Copilot manifest text whose version is the JSON text `version`. */
const copilotManifest = (version: string): string =>
  `{"name":"agentics","version":${version},"skills":["./skills/tdd"],"hooks":"./hooks/hooks.copilot.json"}`;

/** Writes the Claude catalog's agentics version and the Copilot manifest's version as the given JSON text. */
export const versions = (
  root: string,
  catalog: string,
  copilot: string,
): void => {
  writeFile(
    root,
    CLAUDE_CATALOG,
    claudeCatalog(agenticsEntry(`"version":${catalog},`)),
  );
  writeFile(root, COPILOT_PLUGIN, copilotManifest(copilot));
};

/** Edits the Codex manifest's parsed value in place and writes it back. */
export const editCodex = (
  root: string,
  edit: (v: Record<string, unknown>) => void,
): void => editJson(root, CODEX_PLUGIN, edit);

/** Replaces the file at `rel` with an empty directory. */
export const asDirectory = (root: string, rel: string): void => {
  remove(root, rel);
  mkdir(root, rel);
};
