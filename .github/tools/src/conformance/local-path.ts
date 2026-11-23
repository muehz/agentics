/** The platform whose rules apply; only `win32` has Windows rules, every other value has Unix rules. */
export type Platform = string;

/**
 * Whether `name` is a local path on `platform`: the path is not empty, not rooted, and does not escape the directory
 * it is relative to after cleaning. `name` is taken as written, so on `win32` either separator splits it.
 *
 * Windows adds three rules: no colon anywhere, no reserved device name in any component, and `\` or `/` at the start
 * means rooted. A component is reserved when the text before its first `.` or `:` (trailing spaces ignored) is CON,
 * PRN, AUX, NUL, COM1 to COM9, LPT1 to LPT9 (the superscripts one to three count as digits), CONIN$ or CONOUT$, in any
 * ASCII case.
 *
 * A reserved name followed by more text is answered as Windows 11 answers it: `nul` followed only by dots and spaces
 * (`nul.`, `nul ..`) is reserved, and `nul.txt` is not. Windows 10 also treats `nul.txt` as reserved.
 */
export function isLocal(
  name: string,
  platform: Platform = process.platform,
): boolean {
  return platform === "win32" ? windowsIsLocal(name) : unixIsLocal(name);
}

/** Whether cleaning `parts` lexically leaves a path that starts with `..`. */
function escapesRoot(parts: readonly string[]): boolean {
  let depth = 0;
  for (const part of parts) {
    if (part === "" || part === ".") continue;
    if (part !== "..") depth += 1;
    else if (depth === 0) return true;
    else depth -= 1;
  }
  return false;
}

function unixIsLocal(name: string): boolean {
  if (name === "" || name.startsWith("/")) return false;
  return !escapesRoot(name.split("/"));
}

const asciiUpper = (text: string): string =>
  text.replace(/[a-z]/g, (char) => char.toUpperCase());

function isReservedBaseName(base: string): boolean {
  const upper = asciiUpper(base);
  if (["CON", "PRN", "AUX", "NUL"].includes(upper)) return true;
  const device = upper.slice(0, 3);
  if (base.length >= 4 && (device === "COM" || device === "LPT")) {
    const digit = base.slice(3);
    return (
      /^[1-9]$/.test(digit) || ["\u00b2", "\u00b3", "\u00b9"].includes(digit)
    );
  }
  return upper === "CONIN$" || upper === "CONOUT$";
}

function isReservedName(name: string): boolean {
  const cut = name.search(/[:.]/);
  const head = cut < 0 ? name : name.slice(0, cut);
  const base = head.replace(/ +$/, "");
  if (!isReservedBaseName(base)) return false;
  // The name is a device name with more text after it. Windows 11 treats it as the device only when that text is dots
  // and spaces, which the API drops.
  return name.replace(/[. ]+$/, "") === base;
}

function windowsIsLocal(name: string): boolean {
  if (name === "" || name.startsWith("\\") || name.startsWith("/"))
    return false;
  if (name.includes(":")) return false;
  const parts = name.split(/[\\/]/);
  if (parts.some(isReservedName)) return false;
  return !escapesRoot(parts);
}
