export interface Writer {
  write(chunk: string): unknown;
}

export interface Io {
  stdout: Writer;
  stderr: Writer;
}

export type Env = Readonly<Record<string, string | undefined>>;

// The tests inject retryDelayMs: 0; only stage reads it.
export interface RunOptions {
  retryDelayMs?: number;
}

export type Run = (
  argv: readonly string[],
  io: Io,
  env: Env,
  options?: RunOptions,
) => Promise<number>;

export const EXIT_OK = 0;
export const EXIT_FAILURE = 1;
export const EXIT_USAGE = 2;

// Writes one diagnostic line, `<prefix>: <message>`, and returns code so a
// command can `return report(...)`.
export function report(
  io: Io,
  prefix: string,
  message: string,
  code: number,
): number {
  io.stderr.write(`${prefix}: ${message}\n`);
  return code;
}
