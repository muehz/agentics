import type { Failure } from "./check.ts";
import { readRepoFile } from "./domains/skills/files.ts";
import { oneLine } from "./text.ts";

/** One pin before applying the family's file selection, matching, and diagnostic policy. */
export type TextPin = { rel: string; literal: string };

/** How the literal is tested against the family's text projection. */
export type PinCheck =
  | { kind: "required" | "forbidden" | "line" }
  | { kind: "count"; times: number };

/** A pin that did not match, with the observed count when the check counts occurrences. */
export type PinMismatch =
  | { kind: "required" | "forbidden" | "line" }
  | { kind: "count"; actual: number; expected: number };

/** The differences among families that use the same pinned-text check. */
export type PinPolicy<Pin extends TextPin> = {
  projection: "folded-source" | "folded" | "folded-insensitive";
  check?: (pin: Pin) => PinCheck;
  directory?: {
    matches: (pin: Pin) => boolean;
    files: (root: string, rel: string) => readonly string[];
  };
  ignoreText?: (text: string) => boolean;
  readOrder?: "before-pins";
  mismatch: (pin: Pin, rel: string, mismatch: PinMismatch) => Failure;
  sourceError: (
    pin: Pin,
    rel: string,
    action: "reading" | "scanning",
    error: unknown,
  ) => Failure;
};

type ReadText = { raw: string; folded: string };

function mismatch(
  pin: TextPin,
  text: ReadText,
  check: PinCheck,
  projection: PinPolicy<TextPin>["projection"],
): PinMismatch | undefined {
  if (check.kind === "line") {
    return text.raw.split(/\r?\n/).includes(pin.literal)
      ? undefined
      : { kind: "line" };
  }
  const literal =
    projection === "folded-source"
      ? pin.literal
      : projection === "folded"
        ? oneLine(pin.literal)
        : oneLine(pin.literal).toLowerCase();
  if (check.kind === "count") {
    const actual = text.folded.split(literal).length - 1;
    return actual === check.times
      ? undefined
      : { kind: "count", actual, expected: check.times };
  }
  const found = text.folded.includes(literal);
  return found === (check.kind === "required")
    ? undefined
    : { kind: check.kind };
}

/**
 * Checks pins in table order and reads each file once. A policy converts a source error to a finding or throws
 * an audit error. A directory selector can supply an ordered set of files for a pin.
 */
export function checkPinnedText<Pin extends TextPin>(
  root: string,
  pins: readonly Pin[],
  policy: PinPolicy<Pin>,
  read: (rel: string) => string = (rel) => readRepoFile(root, rel),
): Failure[] {
  const failures: Failure[] = [];
  const texts = new Map<string, ReadText>();
  const missing = new Set<string>();

  function sourceError(
    pin: Pin,
    rel: string,
    action: "reading" | "scanning",
    error: unknown,
  ): void {
    const failure = policy.sourceError(pin, rel, action, error);
    if (missing.has(rel)) return;
    missing.add(rel);
    failures.push(failure);
  }

  function load(pin: Pin, rel: string): ReadText | undefined {
    const cached = texts.get(rel);
    if (cached !== undefined) return cached;
    if (missing.has(rel)) return undefined;

    let raw: string;
    try {
      raw = read(rel);
    } catch (error) {
      sourceError(pin, rel, "reading", error);
      return undefined;
    }
    let folded = oneLine(raw);
    if (policy.projection === "folded-insensitive")
      folded = folded.toLowerCase();
    const text = { raw, folded };
    texts.set(rel, text);
    return text;
  }

  const selected = new Map<string, readonly string[]>();

  function filesFor(pin: Pin): readonly string[] {
    if (!policy.directory?.matches(pin)) return [pin.rel];
    const cached = selected.get(pin.rel);
    if (cached !== undefined) return cached;
    try {
      const files = policy.directory.files(root, pin.rel);
      selected.set(pin.rel, files);
      return files;
    } catch (error) {
      sourceError(pin, pin.rel, "scanning", error);
      return [];
    }
  }

  if (policy.readOrder === "before-pins")
    for (const pin of pins) for (const rel of filesFor(pin)) load(pin, rel);

  for (const pin of pins) {
    const check = policy.check?.(pin) ?? { kind: "required" };
    for (const rel of filesFor(pin)) {
      const text = load(pin, rel);
      if (text === undefined || policy.ignoreText?.(text.raw)) continue;
      const failure = mismatch(pin, text, check, policy.projection);
      if (failure !== undefined)
        failures.push(policy.mismatch(pin, rel, failure));
    }
  }
  return failures;
}
