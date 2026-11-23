# Forging MCP servers

MCP (Model Context Protocol) servers expose tools, resources, and prompts to LLM clients. The right shape depends on
what you wrap, who consumes it, and how it deploys.

## Discovery first

The answers to five questions determine which deployment model fits.

1. **What upstream system do you wrap?** (REST API, database, internal service, local file system, desktop app)
2. **Who will consume this server?** (a single developer, a team, end users via Claude Desktop, automated agents)
3. **How many distinct operations?** (under 15 → one tool per action; many or unbounded → search + execute pattern)
4. **Does the server need mid-call user input or rich UI?** (forms, pickers, confirm dialogs)
5. **What authentication model?** (API key, OAuth, service principal, none)

Load `../mcp/mcp-deployment-decision-tree.md` for the routing rules.

## Deployment models

| Model                      | Use when                                                      | Reference                              |
| -------------------------- | ------------------------------------------------------------- | -------------------------------------- |
| **Remote streamable HTTP** | A wrapped cloud API, multiple consumers, central auth         | ../mcp/mcp-deployment-decision-tree.md |
| **Local stdio**            | Developer-local prototype, single user, fast iteration        | ../mcp/mcp-deployment-decision-tree.md |
| **MCP app**                | Server needs interactive widgets (forms, pickers) in chat     | ../mcp/mcp-app-widgets.md              |
| **MCPB bundle**            | Distributable local server with bundled runtime, drag-install | ../mcp/mcpb-manifest-schema.md         |

## Workflow

### 1. Discover (always start here)

Answer the five discovery questions. Recommend a deployment model with rationale before you write any code.

### 2. Design tools

Load `../mcp/mcp-tool-design-patterns.md` for the tool-granularity patterns (one-per-action vs search+execute). The
patterns file and the [MCP specification](https://modelcontextprotocol.io/specification/latest) cover these topics:

- naming with a consistent service prefix
- response formats (text + structured)
- pagination
- transport selection
- error messages that tell the agent what to do next
- the `readOnlyHint`/`destructiveHint`/`idempotentHint`/`openWorldHint` annotations

Define parameters with typed schemas. Plan error responses and pagination.

Every file this workflow writes, server source and manifest included, follows the skill's
[Writes to an asset](../../SKILL.md#shared-rules) rule: show the complete file or a diff and write it only after the
user says yes to that exact text in this session.

### 3. Scaffold the server

Use the official TypeScript SDK (`@modelcontextprotocol/sdk`) or the official Python SDK. The SDK repository is the
build guide for each language.

- **TypeScript / Node:** the [TypeScript SDK](https://github.com/modelcontextprotocol/typescript-sdk)
- **Python:** the [Python SDK](https://github.com/modelcontextprotocol/python-sdk)

Set up the chosen transport (stdio for local, streamable HTTP for remote). Build the shared infrastructure once: the
authenticated client, error helpers, response formatting, and pagination. Do not copy-paste it across tools.

### 4. Add authentication (if needed)

For remote servers, integrate the chosen auth model. For OAuth, document the redirect URI and scopes. For API keys, read
them from environment variables. Never hardcode API keys.

### 5. Apply security hardening

Load `../mcp/mcp-security-checklist.md`. Do these steps:

- Validate paths.
- Prevent traversal.
- Sanitize inputs.
- Redact secrets from logs.
- Scope file system access.

### 6. Test

Test with MCP Inspector (`npx @modelcontextprotocol/inspector`) or the equivalent Python tool. Exercise every tool.
Verify that error paths return helpful messages, not stack traces.

Then run a real evaluation. A server that compiles does not show that an LLM can _use_ it. A real evaluation is the only
way to know. Run it in four steps:

1. Write about 10 questions. Make each one independent, read-only, realistic, verifiable, and stable, and make each one
   need several tool calls.
2. Solve each question by hand to fix its answer.
3. Run the questions through an agent session that has only the server under test connected.
4. Compare the agent's answers with the fixed answers.

The upstream [mcp-builder skill](https://github.com/anthropics/skills/tree/main/skills/mcp-builder) has an optional
automated runner for these questions, which Agentics does not maintain. `/agentics:forge eval` applies the same
realistic-independent-verifiable discipline to skills, agents, and instruction files.

### 7. Document

Provide installation instructions, configuration examples, and a tool catalog. For remote servers, document the
deployment URL. For MCPB, document the install flow.

## Branch: MCP app (interactive widgets)

When the discovery step indicates that rich in-chat UI is required, do these steps:

- Load `../mcp/mcp-app-widgets.md`
- Use the `App` class from `@modelcontextprotocol/ext-apps`
- Replace the `/*__EXT_APPS_BUNDLE__*/` placeholder server-side (CDN imports fail due to iframe CSP)
- Design the elicitation flow. Decide which widgets fire when and how user input feeds back into the tool call

## Branch: MCPB bundle

When the discovery step indicates a distributable local server, do these steps:

- Load `../mcp/mcpb-manifest-schema.md` and `../mcp/mcp-security-checklist.md`
- MCPB has no permissions model, so security is the developer's responsibility
- Bundle the runtime (Node.js or Python) with the server
- Pack with `npx @anthropic-ai/mcpb pack`
- If you distribute the bundle publicly, sign it

## When to stop

The server is ready when:

- All discovery questions are answered with documented decisions
- Tools follow the chosen design pattern consistently
- Every tool has been exercised in the inspector or test harness
- The evaluation in step 6 passes: an LLM completes realistic tasks with only the server's tools
- The security checklist passes
- Installation and configuration are documented
- For MCPB: drag-install is tested on a clean machine
