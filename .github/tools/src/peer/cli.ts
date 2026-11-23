import { processHost } from "./host.ts";
import { installInterruptHandlers } from "./interrupt.ts";
import { stopGroup } from "./process.ts";
import { main } from "./run.ts";

// Without these handlers an interrupt kills the launcher but leaves its detached reviewers, and the timeout that
// bounds them, behind.
installInterruptHandlers({
  stop: stopGroup,
  report: (line) => void process.stderr.write(`${line}\n`),
  exit: (status) => process.exit(status),
});

process.exitCode = await main(process.argv.slice(2), processHost());
