import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";

const BLOCK = 512;

// The device names Windows reserves, with or without an extension. They are refused on every
// platform, although the release runs on Linux, so a bundle never holds a name Windows cannot
// extract.
const RESERVED = new Set([
  "CON",
  "PRN",
  "AUX",
  "NUL",
  "CONIN$",
  "CONOUT$",
  ...Array.from({ length: 9 }, (_, i) => `COM${i + 1}`),
  ...Array.from({ length: 9 }, (_, i) => `LPT${i + 1}`),
]);

function isReserved(segment: string): boolean {
  const stem = segment.split(/[.:]/, 1)[0] ?? "";
  return RESERVED.has(stem.trimEnd().toUpperCase());
}

// A tar entry name is local when joining it under the destination cannot leave the destination and
// names something every host can create. Empty segments and "." segments are harmless and ignored.
export function isLocalTarPath(name: string): boolean {
  if (name === "" || name.includes("\0") || name.includes("\\")) return false;
  if (name.startsWith("/") || /^[A-Za-z]:/.test(name)) return false;
  return name
    .split("/")
    .every((segment) => segment !== ".." && !isReserved(segment));
}

function text(buffer: Buffer, start: number, end: number): string {
  const field = buffer.subarray(start, end);
  const nul = field.indexOf(0);
  return field.subarray(0, nul < 0 ? field.length : nul).toString("utf8");
}

function octal(buffer: Buffer, start: number, end: number): number {
  const field = buffer.subarray(start, end).toString("latin1");
  const nul = field.indexOf("\0");
  const digits = (nul < 0 ? field : field.slice(0, nul)).trim();
  if (digits === "") return 0;
  if (!/^[0-7]+$/.test(digits))
    throw new Error("extractTar: invalid tar header");
  return parseInt(digits, 8);
}

function checksumMatches(block: Buffer): boolean {
  const stored = octal(block, 148, 156);
  let unsigned = 0;
  let signed = 0;
  for (let i = 0; i < BLOCK; i++) {
    const byte = i >= 148 && i < 156 ? 0x20 : (block[i] as number);
    unsigned += byte;
    signed += byte > 127 ? byte - 256 : byte;
  }
  return stored === unsigned || stored === signed;
}

// A pax extended header body is a run of "<length> <key>=<value>\n" records, where length counts the
// whole record including its own digits and the newline.
function parsePax(body: Buffer): Map<string, string> {
  const records = new Map<string, string>();
  let position = 0;
  while (position < body.length) {
    const space = body.indexOf(0x20, position);
    const digits =
      space < 0 ? "" : body.subarray(position, space).toString("latin1");
    if (!/^[0-9]+$/.test(digits))
      throw new Error("extractTar: invalid pax header");
    const length = Number(digits);
    const end = position + length;
    if (
      end > body.length ||
      length <= digits.length + 2 ||
      body[end - 1] !== 0x0a
    ) {
      throw new Error("extractTar: invalid pax header");
    }
    const record = body.subarray(space + 1, end - 1).toString("utf8");
    const equals = record.indexOf("=");
    if (equals <= 0) throw new Error("extractTar: invalid pax header");
    records.set(record.slice(0, equals), record.slice(equals + 1));
    position = end;
  }
  return records;
}

// Long names are decoded from ustar prefixes, pax path records and GNU long-name entries.
export function* tarEntries(archive: Buffer) {
  let offset = 0;
  let pax = new Map<string, string>();
  let longName: string | undefined;

  while (offset < archive.length) {
    if (archive.length - offset < BLOCK)
      throw new Error("extractTar: unexpected EOF in a tar header");
    const block = archive.subarray(offset, offset + BLOCK);
    offset += BLOCK;
    if (block.every((byte) => byte === 0)) return;
    if (!checksumMatches(block))
      throw new Error("extractTar: invalid tar header");

    const typeflag = String.fromCharCode(block[156] as number);
    const ustar = block.subarray(257, 263).toString("latin1") === "ustar\0";
    const prefix = ustar ? text(block, 345, 500) : "";
    const headerName =
      prefix === "" ? text(block, 0, 100) : `${prefix}/${text(block, 0, 100)}`;
    const headerOnly = "123456".includes(typeflag);
    let size = headerOnly ? 0 : octal(block, 124, 136);

    const paxSize =
      headerOnly || typeflag === "x" || typeflag === "g"
        ? undefined
        : pax.get("size");
    if (paxSize !== undefined) {
      if (!/^[0-9]+$/.test(paxSize))
        throw new Error("extractTar: invalid pax header");
      size = Number(paxSize);
    }
    if (size > archive.length - offset)
      throw new Error("extractTar: unexpected EOF in a tar entry");
    const body = archive.subarray(offset, offset + size);
    offset += Math.ceil(size / BLOCK) * BLOCK;

    if (typeflag === "x") {
      pax = parsePax(body);
      continue;
    }
    if (typeflag === "g") continue;
    if (typeflag === "L") {
      longName = text(body, 0, body.length);
      continue;
    }

    const name = pax.get("path") ?? longName ?? headerName;
    pax = new Map();
    longName = undefined;

    yield { name, typeflag, mode: octal(block, 100, 108), body };
  }
}

// Extracts only the regular files and directories the hook build needs.
export function extractTar(tarPath: string, dest: string): void {
  for (const { name, typeflag, body } of tarEntries(readFileSync(tarPath))) {
    if (!isLocalTarPath(name))
      throw new Error(`extractTar: ${name} escapes ${dest}`);
    const target = join(
      dest,
      ...name.split("/").filter((segment) => segment !== "" && segment !== "."),
    );
    if (typeflag === "5" || (typeflag === "\0" && name.endsWith("/"))) {
      mkdirSync(target, { recursive: true, mode: 0o755 });
    } else if (typeflag === "0" || typeflag === "\0") {
      mkdirSync(dirname(target), { recursive: true, mode: 0o755 });
      writeFileSync(target, body, { mode: 0o644 });
    }
  }
}
