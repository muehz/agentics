export const EXIT_OK = 0;
export const EXIT_FAILURE = 1;
export const EXIT_USAGE = 2;
export const EXIT_NOTHING_TO_REVIEW = 3;
export const EXIT_VALIDATION_FAILED = 4;
export const EXIT_TIMEOUT = 124;

export const TARGETS = ["claude", "codex"] as const;
export type Target = (typeof TARGETS)[number];

export const ALLOWED_TOOLS =
  "Bash(git diff:*),Bash(git show:*),Bash(git status:*),Bash(git log:*),Bash(git blame:*),Read,Grep,Glob";

export const DEFAULT_EXCLUDES = [
  "**/package-lock.json",
  "**/yarn.lock",
  "**/pnpm-lock.yaml",
  "**/*.min.*",
  "**/dist/**",
  "**/build/**",
  "**/node_modules/**",
  "**/vendor/**",
];
