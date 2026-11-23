/**
 * Why a file or directory in the checkout could not be read:
 *
 * - `missing`: nothing exists at the path.
 * - `not-a-file`: the path exists but is not a regular file.
 * - `not-a-directory`: a directory was needed and the path, or a component of it, is something else.
 * - `symlink`: the path is, or passes through, a link.
 * - `outside-repo`: the path does not stay below the checkout root.
 * - `unreadable`: any other failure.
 */
export type RepoFileErrorKind =
  | "missing"
  | "not-a-file"
  | "not-a-directory"
  | "symlink"
  | "outside-repo"
  | "unreadable";

const UNREADABLE = "unreadable";

const SENTENCES: Readonly<
  Record<Exclude<RepoFileErrorKind, "unreadable">, string>
> = {
  missing: "does not exist",
  "not-a-file": "is not a regular file",
  "not-a-directory": "is not a directory",
  symlink: "passes through a link",
  "outside-repo": "is outside the checkout",
};

const KIND_BY_CODE: Readonly<Record<string, RepoFileErrorKind>> = {
  ENOENT: "missing",
  EISDIR: "not-a-file",
  ENOTDIR: "not-a-directory",
};

/**
 * A failure to read one path in the checkout. `kind` says why and `path` says where, written with forward slashes.
 * The path is repo-relative, except that an `outside-repo` error carries the path as the caller gave it and a `symlink`
 * error carries the path up to and including the link.
 *
 * The message is `<path>: <sentence>`, with one fixed sentence per kind. For `unreadable` the sentence is
 * `options.detail`, normally the Node error code, or the word `unreadable` when there is none; `detail` is ignored for
 * every other kind. The error that led here, if any, is kept only as `cause`.
 */
export class RepoFileError extends Error {
  readonly kind: RepoFileErrorKind;
  readonly path: string;

  constructor(
    kind: RepoFileErrorKind,
    path: string,
    options?: { cause?: unknown; detail?: string },
  ) {
    const slashPath = path.replaceAll("\\", "/");
    const sentence =
      kind === "unreadable" ? options?.detail || UNREADABLE : SENTENCES[kind];
    super(
      `${slashPath}: ${sentence}`,
      options !== undefined && "cause" in options
        ? { cause: options.cause }
        : undefined,
    );
    this.name = "RepoFileError";
    this.kind = kind;
    this.path = slashPath;
  }
}

/**
 * Turns a value thrown by a file-system call on `path` into a `RepoFileError` with that value as its `cause`. `ENOENT`
 * gives `missing`, `EISDIR` gives `not-a-file` and `ENOTDIR` gives `not-a-directory`. Any other code gives `unreadable`
 * showing the code, and a value with no string code gives plain `unreadable`. A `RepoFileError` is returned unchanged,
 * keeping its own path.
 */
export function repoFileErrorOf(error: unknown, path: string): RepoFileError {
  if (error instanceof RepoFileError) return error;
  const code =
    typeof error === "object" && error !== null && "code" in error
      ? error.code
      : undefined;
  if (typeof code !== "string") {
    return new RepoFileError("unreadable", path, { cause: error });
  }
  const kind = Object.hasOwn(KIND_BY_CODE, code)
    ? KIND_BY_CODE[code]
    : undefined;
  return new RepoFileError(kind ?? "unreadable", path, {
    cause: error,
    detail: code,
  });
}
