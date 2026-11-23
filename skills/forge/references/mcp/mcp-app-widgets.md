# MCP App Widgets

MCP app widgets are interactive UI components that render inline in Claude. When the server needs structured input
mid-call (forms, pickers, confirm dialogs) or rich display (charts, maps, diffs), use them.

## When Widgets Fit

- A tool needs structured input that the LLM cannot reliably extract from chat (date ranges, multi-select)
- A tool requires explicit user confirmation before a destructive action
- A tool result is best visualized (chart, diff, map) rather than rendered as text
- A multi-step flow benefits from a wizard-style picker

## When Widgets Don't Fit

- Simple text input: use the standard elicitation protocol instead
- Output that the LLM should reason over: return structured data, not a chart
- Server-to-server automation: widgets require a human in the loop

## Architecture

```
LLM → MCP server → returns tool result with UI resource URI
                ↓
LLM → MCP client → fetches UI resource (HTML)
                ↓
Client renders widget in iframe → user interacts
                ↓
Widget posts message back → MCP server → continues tool execution
```

The server side has two concerns:

1. The tool returns a result that references the UI resource
2. The resource handler serves the HTML and JavaScript bundle for the widget

## Critical: The Bundle Placeholder

The official ext-apps SDK ships HTML templates with a `/*__EXT_APPS_BUNDLE__*/` placeholder. The placeholder must be
replaced **server-side** before the server sends the HTML to the client. CDN imports fail because the iframe enforces
strict CSP.

```javascript
const html = template.replace("/*__EXT_APPS_BUNDLE__*/", bundleSource);
return { contents: [{ mimeType: "text/html", text: html }] };
```

Without this step, the widget renders blank and the browser console shows a CSP error.

## Widget Catalog

### Form

A form takes multi-field input. Fields can be text, number, date, select, checkbox, or textarea.

```
{
  type: 'form',
  fields: [
    { name: 'title', type: 'text', required: true },
    { name: 'priority', type: 'select', options: ['low', 'med', 'high'] },
    { name: 'due', type: 'date' }
  ],
  submitLabel: 'Create'
}
```

### Picker

A picker offers single or multi-select from a list. When the LLM has a candidate list but the user needs to choose, use
a picker.

```
{
  type: 'picker',
  items: [{ id, label, description, metadata }],
  mode: 'single' | 'multi',
  searchable: true
}
```

### Confirm Dialog

A confirm dialog shows a message with a yes or no choice. Use it before destructive actions (delete, force-push, deploy
to prod).

```
{
  type: 'confirm',
  title: 'Delete branch?',
  message: 'This will permanently delete the branch on the remote.',
  confirmLabel: 'Delete',
  cancelLabel: 'Cancel',
  destructive: true
}
```

### Display Widgets

- **Chart**: bar, line, or scatter. Pass the data and the chart config.
- **Diff**: a side-by-side or unified diff view
- **Map**: a geographic visualization with markers
- **Progress**: indeterminate or determinate progress for long-running tasks

## Elicitation Flow

Two patterns handle multi-step interactions:

### Pattern A: Single tool, multiple widgets

The tool execution sequences the widgets:

1. The tool starts → returns a Form widget
2. The user submits the Form → the tool resumes with the form data → returns a Confirm widget
3. The user confirms → the tool completes the action → returns success

### Pattern B: Multiple tools, one widget each

The LLM orchestrates the flow. It calls the tools in sequence:

1. `prepare_action(...)` → returns a Form widget for parameters
2. The LLM extracts the form data and calls `confirm_action(...)` → returns a Confirm widget
3. The LLM extracts the confirmation and calls `execute_action(...)` → returns the result

Pattern B is simpler to implement but gives the LLM more chances to misorder the flow. Pattern A keeps state on the
server but requires the SDK's session-resumption support.

## Error Handling

- If the user cancels the widget, the tool returns a structured cancellation, not an error
- If the widget times out (no response after N seconds), the server decides: cancel, fall back to LLM input, or fail
- If the form input has validation errors, return the same form with error messages, not a fresh form

## Testing Widgets

The MCP Inspector renders widgets in a debug panel. Test these points:

- Each widget renders without CSP errors
- Form validation surfaces correctly
- Cancel paths return structured cancellations
- If a multi-step flow uses server-side state, it survives page reloads
