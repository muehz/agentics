import {
  contractCase,
  replaceIn,
  type MutationCase,
} from "./mutation-support.ts";
import { replaceFirst } from "./workflow.ts";

// Mutation cases 1 to 21 of the table: where the App token reaches, the job gates and conditions, secrets,
// environments, and the main-only and major-only guards.
export const gateCases: readonly MutationCase[] = [
  contractCase(
    "moving the install after the token mint lets npm code see the token",
    (s) => {
      const install =
        "            - name: Install release tooling\n" +
        "              if: env.MODE == 'analyze'\n" +
        "              working-directory: .github/release\n" +
        "              run: npm ci --ignore-scripts\n";
      const anchor = "            - name: Upload the baseline notes\n";
      return replaceIn(
        "release",
        "prepare",
        anchor,
        install + anchor,
      )(replaceIn("release", "prepare", install, "")(s));
    },
    "release must install npm packages before it mints the App token",
  ),
  contractCase(
    "handing the App token to the install step lets npm code see it",
    replaceIn(
      "release",
      "prepare",
      "              working-directory: .github/release\n",
      "              working-directory: .github/release\n              env:\n                  GITHUB_TOKEN: ${{ steps.app-token.outputs.token }}\n",
    ),
    "the App token may reach only the semantic-release step and the result step",
  ),
  contractCase(
    "moving the App token from the result step to the identity step widens its reach",
    (s) => {
      s = replaceIn(
        "release",
        "prepare",
        "                  GITHUB_TOKEN: ${{ steps.app-token.outputs.token }}\n                  PLAN:",
        "                  PLAN:",
      )(s);
      return replaceIn(
        "release",
        "prepare",
        "                  APP_SLUG:",
        "                  GITHUB_TOKEN: ${{ steps.app-token.outputs.token }}\n                  APP_SLUG:",
      )(s);
    },
    "the result step must receive the App token",
  ),
  contractCase(
    "moving the App token from the semantic-release step to the identity step widens its reach",
    (s) => {
      s = replaceIn(
        "release",
        "prepare",
        "                  GITHUB_TOKEN: ${{ steps.app-token.outputs.token }}\n                  GIT_AUTHOR_NAME:",
        "                  GIT_AUTHOR_NAME:",
      )(s);
      return replaceIn(
        "release",
        "prepare",
        "                  APP_SLUG:",
        "                  GITHUB_TOKEN: ${{ steps.app-token.outputs.token }}\n                  APP_SLUG:",
      )(s);
    },
    "the semantic-release step must receive the App token",
  ),
  contractCase(
    "a job token on the level install step reaches npm code",
    replaceIn(
      "level",
      "approve-major",
      "              working-directory: .github/release\n",
      "              working-directory: .github/release\n              env:\n                  GH_TOKEN: ${{ github.token }}\n",
    ),
    "level: the npm install step must exist and run with no token in its environment",
  ),
  contractCase(
    "a job token on the release install step reaches npm code",
    replaceIn(
      "release",
      "prepare",
      "              working-directory: .github/release\n",
      "              working-directory: .github/release\n              env:\n                  GH_TOKEN: ${{ github.token }}\n",
    ),
    "release: the npm install step must exist and run with no token in its environment",
  ),
  contractCase(
    "listing drafts as published releases in the level job skips a stranded stage",
    replaceIn(
      "level",
      "approve-major",
      "--limit 1000 --exclude-drafts",
      "--limit 1000",
    ),
    'level missing "--limit 1000 --exclude-drafts"',
  ),
  contractCase(
    "listing drafts as published releases in the release job reports a draft as released",
    replaceIn(
      "release",
      "prepare",
      "--limit 1000 --exclude-drafts",
      "--limit 1000",
    ),
    'release missing "--limit 1000 --exclude-drafts"',
  ),
  contractCase(
    "contents: write on the release job lets the job token push",
    replaceIn(
      "release",
      "prepare",
      "            contents: read\n",
      "            contents: write\n",
    ),
    "release must not hold contents: write",
  ),
  contractCase(
    "prepare without !cancelled() skips whenever approve-major was skipped",
    replaceIn("prepare", "stage", "if: ${{ !cancelled() && ", "if: ${{ "),
    "prepare must run after a skipped approve-major",
  ),
  contractCase(
    "stage without !cancelled() skips whenever approve-major was skipped",
    replaceIn("stage", "agent", "if: ${{ !cancelled() && ", "if: ${{ "),
    "stage must run after a skipped approve-major",
  ),
  contractCase(
    "agent without !cancelled() skips whenever approve-major was skipped",
    replaceIn("agent", "", "if: ${{ !cancelled() && ", "if: ${{ "),
    "agent must run after a skipped approve-major",
  ),
  contractCase(
    "an unconditional major approval lets a major release skip the reviewer",
    replaceIn(
      "release",
      "prepare",
      "AGX_MAJOR_APPROVED: ${{ needs.approve-major.result == 'success' && 'true' || 'false' }}",
      "AGX_MAJOR_APPROVED: 'true'",
    ),
    "release must derive AGX_MAJOR_APPROVED from the approve-major result",
  ),
  contractCase(
    "dropping the approve-major result clause lets release run after a failed approval",
    replaceIn(
      "release",
      "prepare",
      'contains(fromJSON(\'["success","skipped"]\'), needs.approve-major.result) && ',
      "",
    ),
    "release must run only after approve-major succeeded or was skipped",
  ),
  contractCase(
    "dropping the level result clause lets release run after a failed level job",
    replaceIn("release", "prepare", "needs.level.result == 'success' && ", ""),
    "release must run only after level succeeded",
  ),
  contractCase(
    "a secret in the level job reaches an unprivileged job",
    replaceIn(
      "level",
      "approve-major",
      "                  command: resume-check\n",
      "                  command: resume-check\n                  token: ${{ secrets.AGX_RELEASE_PRIVATE_KEY }}\n",
    ),
    "secrets reach only the release job",
  ),
  contractCase(
    "a secret in the approve-major job reaches a job that only waits for a reviewer",
    replaceIn(
      "approve-major",
      "release",
      "              run: echo",
      "              env:\n                  KEY: ${{ secrets.AGX_RELEASE_PRIVATE_KEY }}\n              run: echo",
    ),
    "secrets reach only the release job",
  ),
  contractCase(
    "an environment on the prepare job would pause every release",
    (s) =>
      replaceFirst(
        s,
        "    prepare:\n",
        "    prepare:\n        environment: github-release\n",
      ),
    "only approve-major and release may name an environment",
  ),
  contractCase(
    "dropping the release job's environment leaves the App secrets ungated",
    replaceIn(
      "release",
      "prepare",
      "        environment: release-commit\n",
      "",
    ),
    "exactly approve-major and release may name an environment",
  ),
  contractCase(
    "running the release job off main lets a branch dispatch commit a release",
    replaceIn("release", "prepare", "github.ref == 'refs/heads/main' && ", ""),
    `release missing "github.ref == 'refs/heads/main'"`,
  ),
  contractCase(
    "gating approve-major on something other than a major level",
    replaceIn(
      "approve-major",
      "release",
      "needs.level.outputs.level == 'major'",
      "needs.level.outputs.level != 'none'",
    ),
    `approve-major missing "needs.level.outputs.level == 'major'"`,
  ),
];
