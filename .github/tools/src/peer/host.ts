// Everything main() needs from the outside, so tests can drive it without a child process.
export interface Host {
  env: Record<string, string | undefined>;
  cwd: string;
  platform: NodeJS.Platform;
  out: (data: string | Uint8Array) => void;
  err: (text: string) => void;
}

export function processHost(): Host {
  return {
    env: process.env,
    cwd: process.cwd(),
    platform: process.platform,
    out: (data) => void process.stdout.write(data),
    err: (text) => void process.stderr.write(text),
  };
}
