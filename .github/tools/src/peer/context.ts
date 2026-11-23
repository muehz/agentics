import type { Options } from "./args.ts";
import type { Platform, Tunables } from "./config.ts";
import type { Host } from "./host.ts";

export interface Excludes {
  // One `:(exclude,glob)<pattern>` pathspec per exclusion, for the tool's own git calls.
  args: string[];
  // The same pathspecs, single-quoted for the reviewer to copy into its shell.
  pathspecs: string;
}

export interface ReviewContext {
  host: Host;
  options: Options;
  tunables: Tunables;
  platform: Platform;
  range: string;
  exclude: Excludes;
}
