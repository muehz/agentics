import { createHash } from "node:crypto";
import { chmodSync, mkdirSync, writeFileSync } from "node:fs";
import { delimiter, dirname, join } from "node:path";
import { fixtureGitEnv } from "../../conformance/testing/git-checkout.ts";
import { HOOK_BINARIES } from "../assets.ts";
import type { Env } from "../io.ts";
import {
  commitFiles,
  initRepo,
  scratchDir,
  writeManifests,
} from "./support.ts";

// A git repository whose hooks/go, hooks/SHA256SUMS and bin/agx.exe a stand-in verifier "builds", so the bundle
// tests need neither a Go toolchain nor six cross-compiles. The stand-ins are POSIX shell scripts,
// so tests that run them skip on Windows.

export const PINNED_GO = "1.27.3";
export const TAG = "v1.2.3";
export const NOTICES = "# Third party notices\n\nfixture\n";
export const MAIN_GO = "package main\n\nfunc main() {}\n";

// The committed launcher. The stand-in verifier's launcher "build" copies cmd/launcher/main.go, so a
// fixture whose bin/agx.exe holds these bytes is one whose launcher reproduces.
export const LAUNCHER = "package main\n\n// the agx launcher\nfunc main() {}\n";

export function binaryContent(name: string): Buffer {
  return Buffer.from(`fixture hook binary ${name}\n`);
}

export function sha256(data: Buffer | string): string {
  return createHash("sha256").update(data).digest("hex");
}

// MODE=correct writes the fixture binaries, wrong writes a different first binary, missing omits
// the first, fail exits 3 after printing the diagnostic hooktool prints for a toolchain pin.
// Both launcher modes write the launcher it "rebuilds" from cmd/launcher to
// O/bin/agx.exe: launcher-wrong writes other bytes there and launcher-missing writes none.
// It records what it was given in $RECORD (the --out path), $RECORD.real (its physical path), $RECORD.main,
// $RECORD.manifest, $RECORD.launcher (the --launcher path) and $RECORD.committed (that file's bytes), and
// it requires either --launcher L or --build-launcher and an empty output directory.
const VERIFIER = `#!/bin/sh
if [ "$1" != build ] || [ "$2" != --source ] || [ "$4" != --manifest ] || [ "$6" != --out ]; then
  echo "unexpected arguments: $*" >&2
  exit 64
fi
case "$#:$8" in
  9:--launcher) cat "$9" > "$RECORD.committed" || exit 1 ;;
  8:--build-launcher) ;;
  *) echo "unsupported launcher arguments: $*" >&2; exit 64 ;;
esac
source=$3
manifest=$5
out=$7
launcher=$9
if [ -n "$(ls -A "$out")" ]; then
  echo "output directory is not empty" >&2
  exit 65
fi
printf '%s' "$out" > "$RECORD"
(cd "$out" && pwd -P) > "$RECORD.real"
cat "$source/cmd/hook/main.go" > "$RECORD.main"
cat "$manifest" > "$RECORD.manifest"
printf '%s' "$launcher" > "$RECORD.launcher"
mode=\${MODE:-correct}
if [ "$mode" = fail ]; then
  echo "verifier refused: built with go1.26.0, but go.mod pins go1.27.3" >&2
  exit 3
fi
if [ "$mode" != launcher-missing ]; then
  mkdir "$out/bin"
  if [ "$mode" = launcher-wrong ]; then
    printf wrong > "$out/bin/agx.exe"
  else
    cat "$source/cmd/launcher/main.go" > "$out/bin/agx.exe"
  fi
fi
first=1
for name in $NAMES; do
  if [ "$first" = 1 ] && [ "$mode" = missing ]; then
    first=0
    continue
  fi
  if [ "$first" = 1 ] && [ "$mode" = wrong ]; then
    printf wrong > "$out/$name"
  else
    cp "$FIXTURES/$name" "$out/$name"
  fi
  first=0
done
`;

// Answers \`go version <file>\` the way the real command does, with the version in $FAKE_GO_VERSION.
const FAKE_GO = `#!/bin/sh
if [ -n "$FAKE_GO_FAIL" ]; then
  echo "fake go: $FAKE_GO_FAIL" >&2
  exit 1
fi
if [ "$1" = version ] && [ "$#" -eq 2 ]; then
  pwd -P > "$RECORD.gocwd"
  echo "$2: go$FAKE_GO_VERSION"
  exit 0
fi
echo "fake go: unsupported arguments: $*" >&2
exit 1
`;

export interface HookFixture {
  root: string;
  repo: string;
  sha: string;
  verifier: string;
  binDir: string;
  fixtures: string;
  record: string;
}

export function createHookFixture(): HookFixture {
  const root = scratchDir("agx-fixture-");
  const repo = join(root, "repo");
  const fixtures = join(root, "fixtures");
  const binDir = join(root, "bin");
  for (const dir of [repo, fixtures, binDir]) mkdirSync(dir);
  initRepo(repo);

  const lines = HOOK_BINARIES.map((name) => {
    const data = binaryContent(name);
    writeFileSync(join(fixtures, name), data);
    return `${sha256(data)}  bin/${name}\n`;
  });
  writeManifests(repo, "1.2.3");
  for (const path of [
    "bin/agx",
    "hooks/run-hook.cmd",
    "skills/example/scripts/tool.sh",
  ]) {
    const file = join(repo, path);
    mkdirSync(dirname(file), { recursive: true });
    writeFileSync(file, `#!/bin/sh\n# ${path}\n`);
    chmodSync(file, 0o755);
  }
  const sha = commitFiles(
    repo,
    {
      "THIRD_PARTY_NOTICES.md": NOTICES,
      LICENSE: "fixture license\n",
      ".agents/plugins/marketplace.json": "{}\n",
      "assets/logo.svg": "<svg/>\n",
      "hooks.json": "{}\n",
      "hooks/hooks.claude.json": "{}\n",
      "hooks/hooks.codex.json": "{}\n",
      "hooks/hooks.copilot.json": "{}\n",
      "hooks/fetch-hook.ps1": "exit 0\n",
      "skills/example/SKILL.md": "# Example\n",
      "skills/example/references/example.md": "fixture reference\n",
      "hooks/go/go.mod": `module example.com/tinyhook\n\ngo ${PINNED_GO}\n`,
      "hooks/go/cmd/hook/main.go": MAIN_GO,
      "hooks/go/cmd/launcher/main.go": LAUNCHER,
      "hooks/SHA256SUMS": lines.join(""),
      "bin/agx.exe": LAUNCHER,
    },
    "initial",
  );

  const verifier = join(root, "verifier");
  writeFileSync(verifier, VERIFIER);
  chmodSync(verifier, 0o755);
  const fakeGo = join(binDir, "go");
  writeFileSync(fakeGo, FAKE_GO);
  chmodSync(fakeGo, 0o755);
  return {
    root,
    repo,
    sha,
    verifier,
    binDir,
    fixtures,
    record: join(root, "record"),
  };
}

// Isolated Git with the fake `go` first on PATH and the stand-in verifier's inputs.
export function hookEnv(
  fixture: HookFixture,
  extra: Readonly<Record<string, string>> = {},
): Env {
  return {
    ...fixtureGitEnv(),
    PATH: `${fixture.binDir}${delimiter}${process.env.PATH ?? ""}`,
    AGX_HOOK_VERIFIER: fixture.verifier,
    FIXTURES: fixture.fixtures,
    NAMES: HOOK_BINARIES.join(" "),
    RECORD: fixture.record,
    MODE: "correct",
    FAKE_GO_VERSION: PINNED_GO,
    ...extra,
  };
}
