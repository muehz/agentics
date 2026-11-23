import type { ReviewContext } from "./context.ts";

// `date -u +%Y%m%dT%H%M%SZ`
export function compactStamp(now = new Date()): string {
  return now
    .toISOString()
    .replace(/[-:]/g, "")
    .replace(/\.\d{3}Z$/, "Z");
}

// `date -u +%Y-%m-%dT%H:%M:%SZ`
export function isoStamp(now = new Date()): string {
  return now.toISOString().replace(/\.\d{3}Z$/, "Z");
}

// Reports must survive the process without writing into protected git metadata. Every byte outside the safe
// set becomes a hyphen, as `tr -c 'A-Za-z0-9._-' '-'` does.
export function defaultOutputDir(ctx: ReviewContext): string {
  const slug = [...Buffer.from(ctx.range)]
    .map((byte) =>
      /[A-Za-z0-9._-]/.test(String.fromCharCode(byte))
        ? String.fromCharCode(byte)
        : "-",
    )
    .join("");
  return `${ctx.host.cwd}/docs/reviews/peer/${compactStamp()}-${process.pid}-${slug}`;
}
