# Redesign From First Principles

_Apply when integrating a new requirement into an existing design._

When you integrate a change, do not bolt it onto the existing design. Redesign it as if the requirement had been there
from the start. The result should look like what we would have built if we had known on day one.

- Read all affected files. Understand the current design holistically
- Ask: "if we wrote this from scratch with this new requirement, what would we build?"
- Propagate the change through every reference: types, docs, examples, rationale sections
- Think about the redesign holistically. Deliver it incrementally

This preserves option value when you integrate changes into an existing design.

## Self-test

- Did I read every file the new requirement touches before I designed? List them.
- If I wrote this from scratch today and knew this requirement, would I produce this shape? If no, name the difference.
- Does the diff add a branch or flag named after the new requirement? That is a bolt-on, not a redesign.
- Did the change reach the types, the docs, the examples, and the rationale, or only the code?
