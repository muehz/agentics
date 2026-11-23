# UI prototype

Generate **several structurally different UI variations** on a single route, switchable from a floating bottom bar. The
user flips between variants in the browser and picks one or combines parts of several. Then the user throws the rest
away.

If the question is about logic or state rather than appearance, use [logic.md](logic.md) instead.

## When this is the right shape

- "What should this page look like?"
- "I want to see a few options for this dashboard before committing."
- "Try a different layout for the settings screen."
- Any time the user would otherwise spend a day on a choice between three vague mockups in their head.

## Two sub-shapes, with sub-shape A as the default

A UI prototype is easier to judge when it **fits into the rest of the app**, with the real header, sidebar, data, and
density. A throwaway route on its own hides problems because every variant looks fine in isolation. Whenever an existing
page can host the variants, default to sub-shape A. Use sub-shape B only for a prototype that has no suitable existing
page. If you have no repository context, ask for the relevant page before you choose a sub-shape. In an
[unattended](../../routing/references/contracts/unattended.md) run, state the page you assume at the top of the
prototype and in the report.

### Sub-shape A, adjustment to an existing page (preferred)

The route already exists. Render variants **on the same route** and gate them by a `?variant=` URL search param. Keep
the existing data fetching and params, and swap only the rendering. Never remove or weaken the page's auth check. When
the page's auth already runs against a local stub, keep it as it is. When it reaches a real identity provider or tenant,
even a test one, use a local stub that returns an invented user, as [rule 4](../SKILL.md#rules-for-every-mode) says, and
only in a development build. Gate the stub and the `?variant=` branch on the server, with the project's build-mode
check, so that outside development the route runs its original auth and renders the original page, whatever the URL
says.

If the prototype is for something that has no page yet but _would naturally live inside one_, use sub-shape A. Examples
are a new section of the dashboard, a new card on the settings screen, or a new step in an existing flow. Mount the
variants inside the host page.

### Sub-shape B, a new page (last resort)

Use this only when the thing that you prototype genuinely has no existing page to live inside. Examples are an entirely
new top-level page and a flow that you cannot embed anywhere sensible.

Create a **throwaway route** that follows the project's routing convention. Do not invent a new top-level structure.
Name it so readers can identify it as a prototype, for example with `prototype` in the path or filename. Use the same
`?variant=` pattern.

Before you commit to sub-shape B, check whether an existing page could host it. An empty route hides design problems
that a populated one would expose.

In both sub-shapes the floating bottom bar is identical.

## Process

### 1. State the question and pick N

Default to **3 variants**. Beyond 5 variants, they no longer differ meaningfully and add noise, so cap the count at 5.

Write down the plan in one line, in a top-of-file comment, and say it before you write. Name the files you will create
and the existing files you will edit:

> "Three variants of the settings page, switchable via `?variant=`, on the existing `/settings` route. New:
> `prototype-settings-variants.tsx`, `prototype-switcher.tsx`. Edited: `app/settings/page.tsx`."

### 2. Generate structurally different variants

Draft each variant. Check each one against:

- The page's purpose and the data it has access to.
- The project's component library / styling system (TailwindCSS, shadcn, MUI, plain CSS, whatever).
- A clear exported component name, e.g. `VariantA`, `VariantB`, `VariantC`.

Variants must be **structurally different**, with different layouts, information hierarchies, and primary affordances,
and not only different colours. Three slightly tweaked card grids do not make a UI prototype. If two drafts are too
similar, redo one with explicit "do not use a card grid" guidance.

### 3. Wire them together

Create a single switcher component on the route:

```tsx
// pseudo-code. Adapt to the project's framework.
// Fail closed: outside a development build, the route ignores ?variant=.
if (!isDevelopmentBuild) return <OriginalPage {...data} />;
const variant = searchParams.get("variant") ?? "A";
return (
    <>
        {variant === "A" && <VariantA {...data} />}
        {variant === "B" && <VariantB {...data} />}
        {variant === "C" && <VariantC {...data} />}
        <PrototypeSwitcher variants={["A", "B", "C"]} current={variant} />
    </>
);
```

For sub-shape A (existing page), keep all the existing data fetching above the switcher. Only the rendered subtree
changes per variant.

For sub-shape B (new page), the throwaway route under `/prototype/<name>` mounts the same switcher, and outside a
development build it returns not found.

### 4. Build the floating switcher

Build a small fixed-position bar at the bottom-centre of the screen. The bar has three pieces:

- **Left arrow** cycles to the previous variant (wraps around).
- **Variant label** shows the current variant key and, if the variant exports a name, that name too. For example,
  `B: Sidebar layout`.
- **Right arrow** cycles forward (wraps around).

Behaviour:

- A click on an arrow updates the URL search param. Use the framework's router, such as `router.replace` on Next or
  `navigate` on React Router, so the variant is shareable and reload-stable.
- The `←` and `→` arrow keys also cycle. When an `<input>`, `<textarea>`, or `[contenteditable]` is focused, do not
  intercept arrow keys.
- Make the switcher visually distinct from the page, for example with a high-contrast pill or subtle shadow. The
  distinct look lets users tell that the switcher is not part of the design under evaluation.
- Hide the switcher in non-development builds. Gate it on the project's standard build-mode check, for example
  `NODE_ENV !== 'production'` or an equivalent feature flag. The gate means that a stray prototype merge cannot ship the
  bar to users.

Put the switcher in one prototype-marked file next to the variants, such as `prototype-switcher.tsx`, so removing the
prototype removes it too.

### 5. Hand it over

Share the URL and the `?variant=` keys, so the user can flip through the variants. The verdict is the user's pick, so do
not report one yet. The useful feedback is usually **"I want the header from B with the sidebar from C"**. That
identifies the design they want. In an unattended run, return the URL and keys with the verdict marked open.

### 6. Capture the answer and clean up

Once a variant has won, record which one and why, as [When done](../SKILL.md#when-done) says. Then remove the prototype
after the confirmation that section asks for:

- **Sub-shape A.** Remove the variant components and the switcher, and restore the host page's edits.
- **Sub-shape B.** Remove the throwaway route, the variant components and the switcher.

Building the winner for real is [the throwaway line](../SKILL.md#the-throwaway-line): ordinary implementation, which may
start from the winning variant as a draft. Do not leave variant components or the switcher behind. They rot fast and
confuse the next reader.

## Anti-patterns

- **Variants that differ only in colour or copy.** That is a tweak, not a prototype. Real variants disagree about
  structure.
- **Too much shared code between variants.** A shared `<Header>` is fine, and a shared `<Layout>` defeats the point.
  Each variant should be free to throw out the layout.
- **Variants wired to real mutations.** Read-only prototypes are fine. If a variant needs to mutate, point it at a stub.
  The question is "what should this look like," not "does the backend work".
- **A prototype promoted directly to production.** Prototype constraints (no tests, minimal error handling) shaped the
  variant code. When you fold it in, rewrite it properly.
