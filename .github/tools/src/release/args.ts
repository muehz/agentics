import { EXIT_USAGE, report, type Io } from "./io.ts";

export type ParsedArgs =
  | { ok: true; values: Readonly<Record<string, string>> }
  | { ok: false; message: string };

// Accepts `--name value` and `--name=value` for the names in `allowed`. A bare
// `--` ends the flags, so it is fine as the last argument and a usage error
// when anything follows it. The last occurrence of a flag wins. A flag that
// never appears is absent from `values`; one given an empty string is present
// with "", and callers treat both as missing. Single-dash flags and -h are
// rejected, because no caller uses them, and each diagnostic is one line
// naming the bad argument.
export function parseArgs(
  argv: readonly string[],
  allowed: readonly string[],
): ParsedArgs {
  const values = new Map<string, string>();
  let index = 0;
  while (index < argv.length) {
    const arg = argv[index] ?? "";
    index += 1;
    if (arg === "--") {
      const extra = argv[index];
      if (extra !== undefined) {
        return { ok: false, message: `unexpected argument ${extra}` };
      }
      break;
    }
    if (!arg.startsWith("-") || arg === "-") {
      return { ok: false, message: `unexpected argument ${arg}` };
    }
    if (!arg.startsWith("--")) {
      return {
        ok: false,
        message: `unsupported flag ${arg} (flags take the form --name value)`,
      };
    }
    const body = arg.slice(2);
    const equals = body.indexOf("=");
    const name = equals < 0 ? body : body.slice(0, equals);
    if (!allowed.includes(name)) {
      return { ok: false, message: `unknown flag --${name}` };
    }
    if (equals >= 0) {
      values.set(name, body.slice(equals + 1));
      continue;
    }
    const value = argv[index];
    if (value === undefined) {
      return { ok: false, message: `flag --${name} needs a value` };
    }
    index += 1;
    values.set(name, value);
  }
  return { ok: true, values: Object.fromEntries(values) };
}

// The opening every command shares. When argv does not parse it writes the one
// diagnostic line naming the bad flag, ahead of the command's own "required"
// line, and returns undefined.
export function parseFlags(
  argv: readonly string[],
  allowed: readonly string[],
  io: Io,
  prefix: string,
): Readonly<Record<string, string>> | undefined {
  const parsed = parseArgs(argv, allowed);
  if (!parsed.ok) {
    report(io, prefix, parsed.message, EXIT_USAGE);
    return undefined;
  }
  return parsed.values;
}
