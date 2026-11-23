import { spawnSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { createRequire } from "node:module";

const root = path.resolve(import.meta.dirname, "../../../..");
const hooksPath = ".github/husky/_";
// A launcher runs whatever `.github/husky/<hook>` the checked-out branch holds,
// so only reviewed hooks get one. A hook joins this roster in the same change
// that adds its tracked `.github/husky/<hook>` file.
const hookNames = ["pre-commit"];
const launcher = '#!/usr/bin/env sh\n. "$(dirname "$0")/h"';

// Only the exact launcher text marks a file as ours; anything else in the
// directory may belong to another tool and must survive reinstalls.
//
// The file is judged through one descriptor so a swap between the type check
// and the read cannot make another file's content count. Windows has no
// O_NOFOLLOW, so a path lstat taken after the open must still name the opened
// file and not be a symbolic link. O_NONBLOCK keeps the open of a named pipe
// with no writer from waiting forever; the type check then rejects it.
function isLauncher(file: string): boolean {
  let fd: number | undefined;
  try {
    fd = fs.openSync(
      file,
      fs.constants.O_RDONLY |
        (fs.constants.O_NOFOLLOW ?? 0) |
        (fs.constants.O_NONBLOCK ?? 0),
    );
    const opened = fs.fstatSync(fd);
    if (!opened.isFile()) return false;
    const text = fs.readFileSync(fd, "utf8");
    const named = fs.lstatSync(file);
    if (
      named.isSymbolicLink() ||
      named.dev !== opened.dev ||
      named.ino !== opened.ino
    )
      return false;
    return text === launcher || text === `${launcher}\n`;
  } catch {
    return false;
  } finally {
    if (fd !== undefined) fs.closeSync(fd);
  }
}

function isMissing(file: string): boolean {
  try {
    fs.lstatSync(file);
    return false;
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") return true;
    throw error;
  }
}

function installHooks(required = false): void {
  if (process.env.HUSKY === "0") {
    if (required)
      throw new Error(
        "HUSKY=0 disables Git hooks; unset it before worktree setup.",
      );
    console.log("HUSKY=0 skip install");
    return;
  }
  // Git configuration is shared by worktrees; generated hook files are not.
  const current = spawnSync("git", ["config", "--get", "core.hooksPath"], {
    cwd: root,
    encoding: "utf8",
  });
  if (current.error) throw current.error;
  if (current.status !== 0 && current.status !== 1)
    throw new Error(`Cannot read Git hook configuration: ${current.stderr}`);
  if (current.stdout.trim() !== hooksPath) {
    const activated = spawnSync(
      "git",
      ["config", "core.hooksPath", hooksPath],
      { cwd: root, encoding: "utf8" },
    );
    if (activated.error) throw activated.error;
    if (activated.status !== 0)
      throw new Error(
        `Cannot activate Git hooks: ${activated.stderr.trim()}\nRun npm run prepare from a terminal with write access to the repository's shared Git configuration, then retry worktree setup.`,
      );
  }
  const generated = path.join(root, hooksPath);
  fs.mkdirSync(generated, { recursive: true });
  fs.writeFileSync(path.join(generated, ".gitignore"), "*\n");
  // Resolve Husky only after skip and configuration checks; bootstrap may have
  // no installed dependencies yet when HUSKY=0 is set.
  const runtime = path.join(
    path.dirname(createRequire(import.meta.url).resolve("husky")),
    "husky",
  );
  fs.copyFileSync(runtime, path.join(generated, "h"));
  // Launchers left by earlier rosters would keep running unreviewed hooks.
  for (const entry of fs.readdirSync(generated)) {
    const file = path.join(generated, entry);
    if (!hookNames.includes(entry) && isLauncher(file)) fs.unlinkSync(file);
  }
  for (const name of hookNames) {
    const file = path.join(generated, name);
    if (isMissing(file) || isLauncher(file)) {
      fs.writeFileSync(file, `${launcher}\n`);
      fs.chmodSync(file, 0o755);
    } else {
      // Another tool owns this hook, so it is kept; setup still succeeds
      // because commits work, only without the repository's checks. The path
      // is built with forward slashes so Windows prints it as Git shows it.
      console.error(
        `${hooksPath}/${name} belongs to another tool and was left in place; lint-staged may not run through Husky.`,
      );
    }
  }
}

try {
  installHooks(process.argv.includes("--required"));
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
}
