# Design red flags

Screen every design candidate against these red flags before synthesis. Do so in the grilling loop, an interface-design
comparison, or a write-time sketch. A red flag is a reason to revise or reject the shape, not an automatic veto. Name
the flag. Make the revision or rejection explicit. Vocabulary per [language.md](language.md): **module**, **interface**,
**seam**, and **depth**.

## Shallow module

A shallow module exposes a large interface and hides little complexity. Judge depth by the capability and policy hidden
behind the public interface, relative to the size of that interface. Prefer a simple interface backed by substantial
behavior.

Do not confuse a deep module with a deep call chain. A deep call chain scatters understanding across layers. A deep
module concentrates capability behind one interface. Signs:

- Callers coordinate several methods to complete one operation.
- Public options expose internal stages or implementation choices.
- A caller who learns the interface still has to learn the implementation.

## Information leakage

Information leakage makes multiple modules depend on the same internal decision. A representation, policy, or protocol
detail appears in more than one place, so a change to it requires coordinated edits.

Public re-exports of transport or wire types are leakage. Parse external data into domain types behind the interface.
Keep storage schemas, framework objects, and protocol details private.

## Temporal decomposition

Temporal decomposition organizes modules by execution order instead of the knowledge they own. Separate load, validate,
transform, and save stages often repeat one representation and its invariants across several seams.

Group code around domain knowledge and ownership. Methods that run at different times can still belong to one module
when they protect the same decisions.

## Pass-through method

A pass-through method forwards the same arguments to another method with the same shape. It adds a layer and hides no
complexity.

Remove it, or move responsibility to the module that can complete the operation. Keep a forwarding seam only when it
adds policy, adaptation, or a distinct abstraction.
