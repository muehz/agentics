# Cursor Surface

Agentics no longer ships a Cursor plugin package. #1175 removed `.cursor-plugin/plugin.json` and
`.cursor-plugin/marketplace.json`, because Cursor support is being deprecated and the package was one more file that
carried the Agentics version. The canonical `skills/` tree is unchanged. Agentics packages nothing for Cursor and makes
no claim about what Cursor loads. The removal is recorded in [RENAMES.md](../RENAMES.md).

## Last observation before removal

Verified on Cursor 3.15.6 (2026-08-08): a symlink in `~/.cursor/plugins/local` that pointed at the repository was
rejected with `loadUserLocalPlugin agentics rejected: symlink target ... is outside ~/.cursor/plugins/local`, so the
documented local install was a copied `git archive` snapshot. That observation describes a package that no longer exists
and is kept as history. End-to-end skill invocation was never confirmed.
