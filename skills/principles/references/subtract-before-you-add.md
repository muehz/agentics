# Subtract Before You Add

_Apply when you sequence an addition, a refactor, or a rewrite._

When you evolve a system, remove complexity first. Then build. Deletion gives you a simpler base, which makes the next
addition smaller and less brittle.

If you add to a complex system, you compound its complexity. If you remove first, you cut the surface area, reveal the
essential structure, and usually make the next design obvious.

Make simplification a continual investment. Leave the design slightly simpler and more capable behind the same or
smaller surface than you found it.

Pattern:

- Cut before you polish (reach the minimum before you invest in quality)
- Design for observed usage, not speculative edge cases
- Do not add speculative validators, parsers, or guards beyond what the spec demands
- Out-of-spec features drag validators behind them. Persistence, retry-on-startup, and schema migration each need guards
  to defend their inputs.
- Simplify prompts (remove redundant instructions, excessive templates)
- When a reference has no novel content, delete it. Do not leave a stub.

## Self-test

- How many lines or files did I delete before I added anything? Zero means look again.
- Does this validator, parser, or guard defend an input the spec actually allows? If no, delete it.
- Is the public surface after this change the same size or smaller than before? Count the exported names.
- Does this reference file, template, or instruction say anything the reader cannot get elsewhere? If no, delete it. Do
  not leave a stub.
