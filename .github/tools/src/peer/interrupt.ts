import { constants as osConstants } from "node:os";

// Reviewers run detached, each in a process group of its own, so a signal sent to the launcher never reaches them
// and the watchdog that bounds them dies with the launcher. This registry lets the launcher stop every reviewer
// group it still owns before it exits.
const active = new Set<number>();
let interrupted = false;

// Records an owned process group and returns the function that forgets it once the group is confirmed gone.
export function trackGroup(group: number): () => void {
  active.add(group);
  return () => void active.delete(group);
}

// True once an interrupt was received. A launcher that is shutting down must not start another reviewer.
export function isInterrupted(): boolean {
  return interrupted;
}

export const INTERRUPT_SIGNALS = ["SIGINT", "SIGTERM", "SIGHUP"] as const;

export interface InterruptDeps {
  stop: (group: number, report: (line: string) => void) => Promise<boolean>;
  report: (line: string) => void;
  exit: (status: number) => void;
}

// Stops every tracked group on the first interrupt, then exits with 128 plus the signal number, the status a shell
// reports for a process killed by that signal. A repeated signal while cleanup runs is ignored: the cleanup is bounded,
// and abandoning it would leave the reviewers running. Returns the function that removes the handlers.
export function installInterruptHandlers(deps: InterruptDeps): () => void {
  const handlers = INTERRUPT_SIGNALS.map((signal) => {
    const handler = (): void => {
      if (interrupted) return;
      interrupted = true;
      const groups = [...active];
      deps.report(
        `peer: ${signal} received; stopping ${groups.length} reviewer process group(s)`,
      );
      void Promise.all(groups.map((group) => deps.stop(group, deps.report)))
        .catch(() => undefined)
        .then(() => deps.exit(128 + (osConstants.signals[signal] ?? 0)));
    };
    process.on(signal, handler);
    return [signal, handler] as const;
  });
  return () => {
    for (const [signal, handler] of handlers) process.off(signal, handler);
  };
}
