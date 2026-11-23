import {
  EXIT_FAILURE,
  EXIT_USAGE,
  type Env,
  type Io,
  type Run,
  type RunOptions,
} from "./io.ts";

// The subcommands, and the only names that are ever turned into a module path.
export const COMMANDS: readonly string[] = [
  "set-version",
  "resume-check",
  "check-commit",
  "bundle",
  "context",
  "verify",
  "stage",
  "asset-roster",
];

// The usage shows the flags of set-version, the subcommand the release plugin
// runs, and names every subcommand on the second line.
export const USAGE = `usage: release set-version --repo ROOT --version MAJOR.MINOR.PATCH
subcommands: ${COMMANDS.join(", ")}`;

export type Loader = (name: string) => Promise<{ run: Run }>;

// Each command is a module under commands/ that exports `run`. The name has
// been checked against COMMANDS before it reaches the import, so no argument
// can name another file.
const loadCommand: Loader = async (name) => {
  const module: unknown = await import(`./commands/${name}.ts`);
  const run = (module as { run?: unknown }).run;
  if (typeof run !== "function") {
    throw new Error(`commands/${name}.ts does not export run`);
  }
  return { run: run as Run };
};

// Runs the subcommand named by argv[0] with the rest of argv. No argument or an
// unknown name prints the two usage lines to stderr and exits 2. A command that
// throws is reported with its own prefix and exits 1 instead of crashing the
// process.
export async function dispatch(
  argv: readonly string[],
  io: Io,
  env: Env,
  options: RunOptions | undefined,
  load: Loader,
): Promise<number> {
  const name = argv[0];
  if (name === undefined || !COMMANDS.includes(name)) {
    io.stderr.write(`${USAGE}\n`);
    return EXIT_USAGE;
  }
  try {
    const { run } = await load(name);
    return await run(argv.slice(1), io, env, options);
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    io.stderr.write(`release: ${name}: ${message}\n`);
    return EXIT_FAILURE;
  }
}

export function main(
  argv: readonly string[],
  io: Io,
  env: Env,
  options?: RunOptions,
): Promise<number> {
  return dispatch(argv, io, env, options, loadCommand);
}

// Only the entry point runs a command. A test or the plugin that imports this
// module must not.
if (import.meta.main) {
  process.exitCode = await main(
    process.argv.slice(2),
    { stdout: process.stdout, stderr: process.stderr },
    process.env,
  );
}
