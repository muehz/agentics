/** Everything main() needs from the outside, so tests can drive it without a child process. */
export interface Host {
  env: Record<string, string | undefined>;
  cwd: string;
  out: (text: string) => void;
  err: (text: string) => void;
}

export function processHost(): Host {
  return {
    env: process.env,
    cwd: process.cwd(),
    out: (text) => void process.stdout.write(text),
    err: (text) => void process.stderr.write(text),
  };
}
