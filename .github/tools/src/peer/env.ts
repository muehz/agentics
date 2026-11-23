import type { Target } from "./constants.ts";
import type { Host } from "./host.ts";

// A review target is another vendor's binary. Inheriting this process's environment hands that vendor every
// secret the host session holds: a Claude Code session exports CLAUDE_CODE_SESSION_ACCESS_TOKEN, so an inherited
// environment can send a live Anthropic credential to OpenAI. The child environment is built from an allowlist,
// and each target gets only its own vendor's credentials. The evals runner keeps the same base allowlist
// (evals/src/provider/environment.ts in muehz/evals), so change both together.
//
// __CF_USER_TEXT_ENCODING is required on macOS: CoreFoundation reads it before touching the login keychain,
// where each CLI stores its credential. Without it the child starts unauthenticated and fails as "Not logged in".
const BASE = [
  "HOME",
  "LANG",
  "LC_ALL",
  "LC_CTYPE",
  "LOGNAME",
  "PATH",
  "SHELL",
  "TERM",
  "TMPDIR",
  "TZ",
  "USER",
  "__CF_USER_TEXT_ENCODING",
];

// A namespace the test harness owns, passed whole. The fake launchers read FAKE_* to choose a failure mode, so
// withholding it would make a truncation or hang test silently exercise the success path. Nothing that holds a
// vendor credential is named FAKE_*.
const PREFIXES = ["FAKE_"];

const PROVIDER: Record<Target, string[]> = {
  claude: ["ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_BASE_URL"],
  codex: ["CODEX_HOME", "OPENAI_API_KEY", "OPENAI_BASE_URL"],
};

// A name that is unset is not exported. A name that is set and empty is, because an empty credential reads as
// configured-but-blank to some CLIs, which is the host's choice to make.
export function childEnvironment(
  target: Target,
  env: Host["env"],
): Record<string, string> {
  const child: Record<string, string> = {};
  for (const name of [...BASE, ...PROVIDER[target]]) {
    const value = env[name];
    if (value !== undefined) child[name] = value;
  }
  for (const [name, value] of Object.entries(env)) {
    if (
      value !== undefined &&
      PREFIXES.some((prefix) => name.startsWith(prefix))
    )
      child[name] = value;
  }
  return child;
}
