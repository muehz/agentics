# MCP Deployment Decision Tree

Five discovery questions resolve to one deployment model. Run them in order.

## Question 1: Upstream system

**What is the server wrapping?**

| Answer                             | Implication                                                                         |
| ---------------------------------- | ----------------------------------------------------------------------------------- |
| Cloud API (REST, GraphQL)          | Remote streamable HTTP fits                                                         |
| Internal service behind a VPN      | Remote streamable HTTP if reachable; otherwise local stdio with credential proxying |
| Local file system or local process | Local stdio or MCPB                                                                 |
| Desktop application                | Local stdio or MCPB                                                                 |
| Database                           | Remote HTTP if hosted; local stdio if local-only                                    |

## Question 2: Consumer

**Who will use this server?**

| Answer                       | Implication                                        |
| ---------------------------- | -------------------------------------------------- |
| Single developer, local-only | Local stdio (no need for distribution)             |
| A team, central deployment   | Remote streamable HTTP                             |
| End users via Claude Desktop | MCPB (drag-install) or Remote HTTP if cloud-hosted |
| Automated agents in CI       | Remote streamable HTTP                             |

## Question 3: Operation count

**How many distinct actions does the server expose?**

| Range                             | Tool design pattern                                             |
| --------------------------------- | --------------------------------------------------------------- |
| 1-15                              | One tool per action (verb_noun naming)                          |
| 15+                               | Search + execute pattern (search returns options, execute acts) |
| Unbounded (e.g., entire database) | Search + execute, with pagination                               |

This answer affects tool design, not deployment, but it shapes the developer experience.

## Question 4: Interactive UI

**Does the server need to collect input from the user mid-call or render rich UI?**

| Answer                                       | Deployment direction                                          |
| -------------------------------------------- | ------------------------------------------------------------- |
| No, all inputs come from the model           | Standard server (HTTP or stdio)                               |
| Yes, simple text input mid-call              | Use the elicitation protocol (built into the standard server) |
| Yes, forms, pickers, confirm dialogs, charts | MCP app (with widgets)                                        |

## Question 5: Authentication

**What auth model does the upstream system require?**

| Answer                               | Notes                                                              |
| ------------------------------------ | ------------------------------------------------------------------ |
| None                                 | Trivial: no auth layer is needed                                   |
| API key (static)                     | Source the key from an env var. Never hardcode it.                 |
| OAuth 2.0                            | Implement the flow with a redirect URI. Document the scopes.       |
| Service principal / managed identity | A cloud-hosted server reads from the cloud identity                |
| Mutual TLS / client certs            | Document cert provisioning. The deployment is usually local stdio. |

## Routing Matrix

After the five answers, the deployment model is usually one of:

| Profile                                                   | Model                                             |
| --------------------------------------------------------- | ------------------------------------------------- |
| Cloud API + team consumer + standard tools + OAuth        | **Remote streamable HTTP**                        |
| Local files + single developer + standard tools + no auth | **Local stdio**                                   |
| Anything + needs widgets in chat                          | **MCP app**                                       |
| Local process + end-user distribution + drag-install      | **MCPB**                                          |
| Local process + single developer + fast iteration         | **Local stdio prototype** (upgrade to MCPB later) |

## Tiebreakers

If multiple models fit:

1. When the upstream system is reachable from a cloud, **prefer remote HTTP**. It gives central auth and easier updates.
2. For distribution to non-developers, **prefer MCPB over local stdio**. Drag-install removes the Node.js/Python setup.
3. For prototypes, **prefer local stdio**. It gives the fastest feedback loop. Upgrade to MCPB once the prototype is
   stable.
4. **An MCP app is additive.** First pick a base deployment (HTTP or MCPB). Then add widgets.

## Anti-Patterns

- Local stdio for production end-user distribution → users need Node.js installed. Use MCPB instead.
- Remote HTTP for local-only desktop apps → adds latency and exposes attack surface for no benefit
- MCPB for cloud-only APIs → bundles a runtime that users don't need. Use remote HTTP.
- One-per-action pattern with 50+ tools → bloats the LLM's tool list. Switch to search + execute.
