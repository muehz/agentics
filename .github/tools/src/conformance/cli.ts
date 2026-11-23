import { processHost } from "./host.ts";
import { main } from "./run.ts";

process.exitCode = await main(process.argv.slice(2), processHost());
