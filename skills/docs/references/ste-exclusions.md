# Preserve protected text during an STE pass

Apply this preservation list before an STE pass. A pass can run in Unslop, Build, or Maintain mode, or in an Agentics
prose sweep. Keep excluded spans byte-exact. Rewrite only the surrounding editable technical prose. For a new document,
preserve the supplied protected text. Do not restyle it. Repository style and reserved vocabulary win.

| Category                   | Preserve byte-exact                                                                                                                                                                            |
| -------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Code and literals          | Fenced code, inline code, commands, flags, paths, globs, regular expressions, JSON, YAML, TOML, environment variables, Mermaid, and HTML comments, including attribution. These are not prose. |
| Frontmatter contracts      | All frontmatter keys and the values of `description` and `argument-hint`. Routing contracts need a separate evaluation pass.                                                                   |
| Proper names               | Products, providers, model IDs, tools, standards, methods, people, organizations, and repositories. These are technical names.                                                                 |
| Agentics names             | Skill and agent names, modes, phases, artifact names, labels, and conformance rule IDs.                                                                                                        |
| Observable literals        | Error strings, hook output, status values, event names, and other literals that tests or conformance assert.                                                                                   |
| User phrasing              | Quoted user phrasing, trigger phrases, and example requests.                                                                                                                                   |
| Wrong examples             | Wrong or before examples in catalogs and worked examples. Keep the example wrong.                                                                                                              |
| Third-party text           | Verbatim or vendored text that attribution, provenance, or license records identify. Adapted prose remains in scope.                                                                           |
| Standard and template text | Standard licenses, codes of conduct, and template placeholders. Surrounding instructions remain in scope.                                                                                      |
| Output templates           | Fixed-shape lines that a skill tells an agent to emit, such as Gate, Why, and Next. The instruction that introduces the line remains in scope.                                                 |
| Non-Markdown files         | Files outside Markdown, including `.py`, `.sh`, `.ps1`, `.yaml`, and `.txt`.                                                                                                                   |
| External references        | External work titles, URLs, and link text that mirrors an external title.                                                                                                                      |

Headings remain in scope. If a heading changes, update every anchor link to it in the same change.

A protected span keeps its punctuation, em dashes included. The punctuation precedence rule in
[technical-writing.md](technical-writing.md) applies only to the editable prose around it.
