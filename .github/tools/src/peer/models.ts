// Cross-model review bindings are hardcoded by design. Peer wants an independent read from a specific strong
// alternate, not a cost-scaled one, so these do not follow the routing ladder's per-surface chains (see
// skills/routing/references/model-ladders-examples.md, "Non-consumers"). The model-ids conformance rule reads this file.
export const DEFAULT_CLAUDE_MODEL = "fable";
export const DEFAULT_CLAUDE_FALLBACK_MODEL = "opus";
export const DEFAULT_CODEX_MODEL = "gpt-6.1-sol";
