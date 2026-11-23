import { spawnSync } from "node:child_process";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import {
  findInterpreter,
  requireInterpreter,
} from "../../entrypoint/interpreters.ts";

export function extractBashBlock(
  markdown: string,
  heading: string,
  index = 0,
): string {
  let inSection = false;
  let fence: { marker: string; bash: boolean; body: string[] } | undefined;
  let found = 0;
  for (const line of markdown.split(/\r?\n/)) {
    const marker = /^ {0,3}(`{3,}|~{3,})(.*)$/.exec(line);
    if (fence) {
      if (marker?.[1]?.startsWith(fence.marker) && marker[2]?.trim() === "") {
        if (inSection && fence.bash && found++ === index)
          return fence.body.join("\n") + "\n";
        fence = undefined;
      } else if (inSection && fence.bash) {
        fence.body.push(line);
      }
      continue;
    }
    const title = /^ {0,3}#{1,6}\s+(.*?)\s*$/.exec(line)?.[1];
    if (title !== undefined) {
      if (inSection) break;
      inSection = title === heading;
    } else if (marker?.[1]) {
      fence = {
        marker: marker[1],
        bash: marker[2]?.trim() === "bash",
        body: [],
      };
    }
  }
  throw new Error(
    `Missing Bash fence ${index} in Markdown section ${JSON.stringify(heading)}`,
  );
}

// The first working bash on PATH, probed so the WSL launcher in System32 is passed over. The `skip` for tests that run
// templates: a reason when bash is missing, and a thrown error when the job lists bash in AGX_REQUIRED_INTERPRETERS.
export const bashSkip = requireInterpreter("bash");
const bash = findInterpreter("bash");

export function templateRepo(t: { after(cleanup: () => void): void }) {
  const base = mkdtempSync(path.join(tmpdir(), "agentics-template-"));
  t.after(() => rmSync(base, { recursive: true, force: true }));
  const root = path.join(base, "repo");
  const bin = path.join(base, "bin");
  mkdirSync(root);
  mkdirSync(bin);
  const env = Object.fromEntries(
    Object.entries(process.env).filter(([key]) => !/^GIT_/i.test(key)),
  );
  const pathKey =
    Object.keys(env).find((key) => key.toUpperCase() === "PATH") ?? "PATH";
  env[pathKey] = [bin, env[pathKey]].filter(Boolean).join(path.delimiter);
  env["GIT_CONFIG_GLOBAL"] = path.join(base, "empty-gitconfig");
  env["GIT_CONFIG_NOSYSTEM"] = "1";
  if (process.platform === "win32") env["MSYS"] = "noglob";
  const git = (...args: string[]): string => {
    const result = spawnSync("git", args, { cwd: root, env, encoding: "utf8" });
    if (result.status !== 0)
      throw new Error(
        result.stderr || result.error?.message || "Fixture Git failed",
      );
    return result.stdout;
  };
  git("init", "-q", "--template=", "-b", "main");
  git("config", "user.name", "Template fixture");
  git("config", "user.email", "fixture@example.invalid");
  const script = path.join(base, "template.sh");
  return {
    base,
    root,
    git,
    stub(name: "agx" | "gh", code: string) {
      writeFileSync(path.join(bin, name), `#!/usr/bin/env bash\n${code}\n`, {
        mode: 0o700,
      });
    },
    run(
      code: string,
      args: string[] = [],
      cwd = root,
      extraEnv: Record<string, string> = {},
    ) {
      if (bash === undefined) throw new Error("Bash is unavailable");
      writeFileSync(script, code);
      return spawnSync(
        bash,
        ["--noprofile", "--norc", script.replaceAll("\\", "/"), ...args],
        {
          cwd,
          env: { ...env, ...extraEnv },
          encoding: "utf8",
          timeout: 15_000,
        },
      );
    },
  };
}
