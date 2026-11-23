import { readdirSync, readFileSync, writeFileSync } from "node:fs";

// Release production code may import only node: modules and relative files, so it keeps its own copy of the file
// error sentences conformance uses rather than importing them.

/**
 * Why a path could not be read or written:
 *
 * - `missing`: nothing exists at the path.
 * - `not-a-file`: a regular file was needed and the path is something else.
 * - `not-a-directory`: a directory was needed and the path, or a component of it, is something else.
 * - `symlink`: the path is, or passes through, a link.
 * - `outside-repo`: the path does not stay below the checkout root.
 * - `unreadable`: any other failure.
 */
export type FileErrorKind =
  | "missing"
  | "not-a-file"
  | "not-a-directory"
  | "symlink"
  | "outside-repo"
  | "unreadable";

const SENTENCES: Readonly<
  Record<Exclude<FileErrorKind, "unreadable">, string>
> = {
  missing: "does not exist",
  "not-a-file": "is not a regular file",
  "not-a-directory": "is not a directory",
  symlink: "passes through a link",
  "outside-repo": "is outside the checkout",
};

const KIND_BY_CODE: Readonly<Record<string, FileErrorKind>> = {
  ENOENT: "missing",
  EISDIR: "not-a-file",
  ENOTDIR: "not-a-directory",
};

/**
 * A failure to read or write one path. The message is `<path>: <sentence>`, with the path as the caller gave it and
 * one fixed sentence per kind. For `unreadable` the sentence is `options.detail`, normally the Node error code, or the
 * word `unreadable` when there is none; `detail` is ignored for every other kind. The error that led here, if any, is
 * kept only as `cause`.
 */
export class FileError extends Error {
  readonly kind: FileErrorKind;
  readonly path: string;

  constructor(
    kind: FileErrorKind,
    path: string,
    options?: { cause?: unknown; detail?: string },
  ) {
    const sentence =
      kind === "unreadable" ? options?.detail || "unreadable" : SENTENCES[kind];
    super(
      `${path}: ${sentence}`,
      options !== undefined && "cause" in options
        ? { cause: options.cause }
        : undefined,
    );
    this.name = "FileError";
    this.kind = kind;
    this.path = path;
  }
}

/**
 * Turns a value thrown by a file-system call on `path` into a `FileError` with that value as its `cause`. `ENOENT`
 * gives `missing`, `EISDIR` gives `not-a-file` and `ENOTDIR` gives `not-a-directory`. Any other code gives
 * `unreadable` showing the code, and a value with no string code gives plain `unreadable`. A `FileError` is returned
 * unchanged.
 */
export function fileErrorOf(error: unknown, path: string): FileError {
  if (error instanceof FileError) return error;
  const code =
    typeof error === "object" && error !== null && "code" in error
      ? error.code
      : undefined;
  if (typeof code !== "string") {
    return new FileError("unreadable", path, { cause: error });
  }
  const kind = Object.hasOwn(KIND_BY_CODE, code)
    ? KIND_BY_CODE[code]
    : undefined;
  return new FileError(kind ?? "unreadable", path, {
    cause: error,
    detail: code,
  });
}

/** The bytes of the file at `path`. Any failure throws a `FileError` for `path`. */
export function readFile(path: string): Buffer {
  try {
    return readFileSync(path);
  } catch (error) {
    throw fileErrorOf(error, path);
  }
}

/**
 * The names of the entries in the directory at `path`, dot names included, in default sort order (UTF-16 code unit
 * order). Any failure throws a `FileError` for `path`.
 */
export function readDirectory(path: string): string[] {
  try {
    return readdirSync(path).sort();
  } catch (error) {
    throw fileErrorOf(error, path);
  }
}

/**
 * Writes `data` to `path` with mode 0o644, replacing any file there. Any failure throws a `FileError` for `path`. A
 * path below a regular file is `not-a-directory` on Linux and `missing` on Windows, which reports it as absent.
 */
export function writeFile(path: string, data: Buffer | string): void {
  try {
    writeFileSync(path, data, { mode: 0o644 });
  } catch (error) {
    throw fileErrorOf(error, path);
  }
}
