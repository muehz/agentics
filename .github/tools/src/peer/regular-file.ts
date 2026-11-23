import {
  closeSync,
  constants,
  fstatSync,
  openSync,
  readFileSync,
} from "node:fs";

export type RegularFileRead =
  { kind: "text"; text: string } | { kind: "missing" } | { kind: "unreadable" };

// Reads a regular file through one descriptor, so the type check and the read describe the same file even if the path
// is replaced in between. `missing` means no file is at the path; `unreadable` means something is there that cannot be
// read as a file (a directory, a permission failure, a device). O_NONBLOCK keeps opening a FIFO from waiting for a
// writer; the option does not exist on Windows.
export function readRegularFile(file: string): RegularFileRead {
  let fd: number;
  try {
    fd = openSync(file, constants.O_RDONLY | (constants.O_NONBLOCK ?? 0));
  } catch (error) {
    const code = (error as NodeJS.ErrnoException).code;
    return code === "ENOENT" || code === "ENOTDIR"
      ? { kind: "missing" }
      : { kind: "unreadable" };
  }
  try {
    if (!fstatSync(fd).isFile()) return { kind: "unreadable" };
    return { kind: "text", text: readFileSync(fd, "utf8") };
  } catch {
    return { kind: "unreadable" };
  } finally {
    closeSync(fd);
  }
}
