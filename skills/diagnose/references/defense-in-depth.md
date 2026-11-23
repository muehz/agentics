# Defense-in-depth validation

## Overview

Validation in one place feels sufficient after a bug from invalid data. But other code paths, refactoring, or mocks can
bypass that single check.

Validate at every layer the data passes through. Make the invalid state structurally impossible.

## Corroborate with multiple signals

Do not trust a single signal during debugging. One green test, one clean log line, or one healthy API response can hide
the real failure mode.

Check at least these evidence sources together:

- **Logs** - what the system says happened
- **State** - the actual files, database rows, memory values, or config in effect
- **Behavior** - what a user, caller, or automated test can observe end-to-end

When those signals disagree, investigate further. Misalignment is often the clue that separates a symptom from the root
cause.

## The four layers

### Layer 1: Entry point validation

Purpose: Reject invalid input at the API boundary.

```typescript
function createProject(name: string, workingDirectory: string) {
    if (!workingDirectory || workingDirectory.trim() === "") {
        throw new Error("workingDirectory cannot be empty");
    }
    if (!existsSync(workingDirectory)) {
        throw new Error(`workingDirectory does not exist: ${workingDirectory}`);
    }
    if (!statSync(workingDirectory).isDirectory()) {
        throw new Error(`workingDirectory is not a directory: ${workingDirectory}`);
    }
    // ... proceed
}
```

### Layer 2: Business logic validation

Purpose: Ensure the data makes sense for this operation.

```typescript
function initializeWorkspace(projectDir: string, sessionId: string) {
    if (!projectDir) {
        throw new Error("projectDir required for workspace initialization");
    }
    // ... proceed
}
```

### Layer 3: Environment guards

Purpose: Prevent dangerous operations in specific contexts.

```typescript
async function gitInit(directory: string) {
    // In tests, refuse git init outside temp directories
    if (process.env.NODE_ENV === "test") {
        const normalized = normalize(resolve(directory));
        const tmpDir = normalize(resolve(tmpdir()));

        if (!normalized.startsWith(tmpDir)) {
            throw new Error(`Refusing git init outside temp dir during tests: ${directory}`);
        }
    }
    // ... proceed
}
```

### Layer 4: Debug instrumentation

Purpose: Capture context for forensics.

```typescript
async function gitInit(directory: string) {
    const stack = new Error().stack;
    logger.debug("About to git init", {
        directory,
        cwd: process.cwd(),
        stack,
    });
    // ... proceed
}
```

## Symptoms vs. root cause

Use layered validation to prove the causal story, not just to patch the failing line.
[root-cause-tracing.md](root-cause-tracing.md) covers how to distinguish the symptom from the root cause and trace back
to the original trigger.

## Apply the pattern

When you find a bug, do these steps:

1. **Trace the data flow** - Where does the bad value originate? Where is it used?
2. **Map all checkpoints** - List every point that data passes through
3. **Add validation at each layer** - Entry, business, environment, debug
4. **Test each layer** - Try to bypass layer 1. Verify that layer 2 catches it

## Worked example

Bug: Empty `projectDir` caused `git init` in source code

**Data flow:**

1. Test setup → empty string
2. `Project.create(name, '')`
3. `WorkspaceManager.createWorkspace('')`
4. `git init` runs in `process.cwd()`

**Four layers added:**

- Layer 1: `Project.create()` validates not empty/exists/writable
- Layer 2: `WorkspaceManager` validates projectDir not empty
- Layer 3: `WorktreeManager` refuses git init outside tmpdir in tests
- Layer 4: Stack trace logging before git init

**Result:** An empty `projectDir` is rejected at the first layer. If a caller skips that check, layer 2 rejects it, and
if that is skipped too, layer 3 does. Layer 4 rejects nothing: its stack trace shows which caller sent the empty value.

## Verify why the fix worked

After the fix appears to work, do these steps:

1. Re-run the original reproduction
2. Check that logs, state, and behavior all changed in the expected way
3. Remove or tighten temporary instrumentation only after you understand the result
4. Write down why the system now behaves correctly so future regressions are easier to diagnose

A passing test without an explanation is fragile confidence.

## Key insight

Each layer covers a way the others can be bypassed:

- Other code paths can reach the operation without going through entry validation
- Mocks can skip business logic checks
- Platform edge cases can slip past both, so the operation needs an environment guard
- Debug logging shows the structural misuse that the other layers only reject
