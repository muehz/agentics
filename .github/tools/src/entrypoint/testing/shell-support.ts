export function quotePosix(text: string): string {
  return `'${text.replaceAll("'", "'\\''")}'`;
}

/** Replaces PATH without leaving aliases that a Windows child could also see. */
export function withFixturePath(
  inherited: NodeJS.ProcessEnv,
  path: string | undefined,
): NodeJS.ProcessEnv {
  const env = Object.fromEntries(
    Object.entries(inherited).filter(([key]) => key.toUpperCase() !== "PATH"),
  );
  return { ...env, PATH: path };
}
