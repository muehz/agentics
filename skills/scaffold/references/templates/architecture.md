# Scaffold reference: docs/ARCHITECTURE.md

## Early-stage advisory

> **Heads up:** ARCHITECTURE.md works best once architecture exists. If this is a brand-new repo, ask: **"Do you have
> components, services, or boundaries to document? (yes/skip)"** On skip, note it in the summary and move on.

If yes, or this is an existing repo, ask the questions below.

## Questions to ask

Ask what's open as one ❓ round, each question with a ➡️ recommendation drawn from the request and the repo. Skip a
topic the request already settles.

- **High-level architecture:** components, services, and major dependencies.
- **Diagrams to include,** if any: ASCII or Mermaid.
- **Key architectural boundaries or integration points.**
- **Infrastructure dependencies** of this application.
- **External systems** this service integrates with, if any.

## Structure

```markdown
[Home](../README.md) > Architecture

# {Project Name}: architecture

## Overview

{One paragraph summary of the system}

## Component diagram

{Mermaid or ASCII diagram}

## Key components

### {Component Name}

{Description, responsibilities, and dependencies}

## Infrastructure dependencies

{List of shared infrastructure this service relies on}

## Integration points

{External systems, APIs, or services this connects to}
```

## Notes

- Use Mermaid for diagrams when possible, because GitHub renders it
- Link to `docs/CONSTITUTION.md` for architectural principles
- Link to `docs/CONFIGURATION.md` if a configuration guide exists

## Conformance check (audit mode)

✅ Conformant if the file exists and is non-empty.
