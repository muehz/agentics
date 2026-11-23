import path from "node:path";
import type { TestEvent } from "node:test/reporters";

export default async function* timings(events: AsyncIterable<TestEvent>) {
  for await (const event of events) {
    if (event.type !== "test:dequeue" && event.type !== "test:complete")
      continue;
    const { name, file } = event.data;
    if (!file || name !== path.relative(process.cwd(), file)) continue;
    if (event.type === "test:dequeue") yield `START ${JSON.stringify(name)}\n`;
    else {
      const { duration_ms, passed } = event.data.details;
      yield `FINISH ${JSON.stringify(name)} ${Math.round(duration_ms)}ms ${passed ? "passed" : "failed"}\n`;
    }
  }
}
