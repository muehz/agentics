// Package provider is the surface-adapter layer between the agx entry
// point and the hook cores. Each host surface — claude-code, copilot, codex —
// has its own argv conventions, payload shape, and response protocol; this
// package owns all three so that a core sees only the neutral contract.
//
// Three functions make up the public surface, and they are the three stages
// a hook invocation passes through:
//
//	Dispatch   raw argv        -> surface event name
//	Normalize  raw payload     -> a provider-neutral hookspec.Event
//	Render     one outcome     -> the surface's stdout/stderr/exit
//
// Dispatch returns only the event: the surface configuration passes --mode to
// every hook, and the cores read no argv, so the word is validated and then
// dropped. The grouped pre-tool event takes no flags at all.
// Normalize returns a *Rendered rather than an error because a malformed
// payload is rejected in the surface's own shape — claude-code exits 1 with no
// body, copilot exits 0 with no body, codex exits 0 after `{}` — and only the
// surface knows that shape. Render takes the core's captured stderr because on
// the error branch that chatter is the diagnostic the host sees, ahead of the
// outcome's own message; every other branch discards it.
//
// This file holds what every surface shares: the response type, the adapter
// registry, the traits each surface declares, payload decoding, document
// emission, and the neutral-event part builders. Everything a surface owns
// lives in its own file, which registers itself from init(), so adding a
// surface never edits this file.
package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"unicode/utf8"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// Rendered is one surface-shaped response: the exact bytes to write to each
// stream and the exit code to leave with.
//
// It is returned by value rather than written to streams directly so a
// rejection can be built deep inside an adapter, carried back out, and emitted
// in exactly one place, which is what keeps the exit-code invariant auditable.
type Rendered struct {
	// Stdout is the response document, if this surface emits one. Empty for
	// claude-code and copilot on most paths; codex always emits an object.
	Stdout []byte

	// Stderr is the diagnostic, already newline-terminated when non-empty.
	Stderr []byte

	// Exit is the process exit code.
	Exit int
}

// adapter is one surface's function table. Fields rather than methods, so a
// surface file registers plain functions and no type is exported.
type adapter struct {
	// traits is the surface's response shape. register requires its name to
	// equal the surface ID, which is what stops a surface from being
	// registered with another surface's response shape or event.
	traits traits

	// normalize turns a decoded payload object into the typed event parts. A
	// non-nil error is the diagnostic that follows the malformed-payload
	// prefix; the shared Normalize builds the event and shapes the rejection.
	normalize func(providerEvent string, payload map[string]any) (contract.EventParts, error)

	// render turns one validated typed outcome into a surface response.
	render func(providerEvent string, outcome hookspec.Outcome, coreStderr []byte) Rendered
}

// registry holds one adapter per surface. Surface files write into it from
// init(); nothing else does, which is why this file names no surface.
var registry = map[surface.ID]adapter{}

// register installs one surface's table. A duplicate registration would
// silently shadow a whole surface, so it panics at init rather than at the
// first hook invocation.
func register(id surface.ID, a adapter) {
	if _, duplicate := registry[id]; duplicate {
		panic(fmt.Sprintf("provider: %q registered twice", id))
	}
	if a.traits.name == "" || a.traits.label == "" || a.normalize == nil || a.render == nil {
		panic(fmt.Sprintf("provider: %q registered an incomplete adapter", id))
	}
	if a.traits.name != string(id) {
		panic(fmt.Sprintf("provider: %q registered with traits named %q", id, a.traits.name))
	}
	// Dispatch returns the surface's event without checking it, so a surface
	// with no event must never be registered.
	if id.PreToolEvent() == "" {
		panic(fmt.Sprintf("provider: %q names no tool-use event", id))
	}
	registry[id] = a
}

// adapterFor resolves a surface name from the command line to its adapter. A
// name that surface.Parse refuses has no adapter, so every entry point treats
// it as unregistered rather than reading the registry with a raw string.
func adapterFor(name string) (adapter, bool) {
	id, ok := surface.Parse(name)
	if !ok {
		return adapter{}, false
	}
	a, ok := registry[id]
	return a, ok
}

// Dispatch resolves one invocation: which surface event this hook serves.
// Every hook serves the surface's one tool-use event, so the hook name selects
// only the argv grammar; the entry point refuses an unknown hook name before
// it calls this.
//
// A non-nil reject means the invocation is over: the caller emits it and
// exits, without reading stdin and without touching a handler.
func Dispatch(provider, hook string, args []string) (providerEvent string, reject *Rendered) {
	a, ok := adapterFor(provider)
	if !ok {
		unregistered := unregisteredProvider(provider)
		return "", &unregistered
	}
	id, _ := surface.Parse(provider)
	return dispatchForSurface(a.traits, id, hook, args)
}

// Normalize validates one raw surface payload into the provider-neutral event
// a core consumes. A non-nil *Rendered is the surface's own malformed-payload
// rejection, already shaped for that surface.
//
// The checks every surface shares run here: the payload must be valid UTF-8,
// exactly one JSON document, and an object. The surface's normalize then
// checks the fields it reads.
func Normalize(provider, providerEvent string, raw []byte) (hookspec.Event, *Rendered) {
	a, ok := adapterFor(provider)
	if !ok {
		unregistered := unregisteredProvider(provider)
		return hookspec.Event{}, &unregistered
	}
	payload, err := decodePayload(raw)
	if errors.Is(err, errPayloadNotObject) {
		err = fmt.Errorf("expected %s %s payload", a.traits.label, providerEvent)
	}
	if err != nil {
		return hookspec.Event{}, rejectMalformed(a.traits, providerEvent, err)
	}
	parts, err := a.normalize(providerEvent, payload)
	if err != nil {
		return hookspec.Event{}, rejectMalformed(a.traits, providerEvent, err)
	}
	event, err := neutralEvent(parts)
	if err != nil {
		return hookspec.Event{}, rejectMalformed(a.traits, providerEvent, err)
	}
	return event, nil
}

// Render turns one validated typed outcome into the surface's response.
//
// The caller checks Outcome.Valid and substitutes an error for an invalid value.
// coreStderr is the core's captured chatter: discarded on every
// branch but error, where it takes precedence over the outcome's own message.
func Render(provider, providerEvent string, outcome hookspec.Outcome, coreStderr []byte) Rendered {
	a, ok := adapterFor(provider)
	if !ok {
		return unregisteredProvider(provider)
	}
	return a.render(providerEvent, outcome, coreStderr)
}

// GroupedContextOnStderr reports whether the grouped event shows the joined
// context on stderr for this surface. A name that surface.Parse refuses has no
// such channel.
func GroupedContextOnStderr(provider string) bool {
	a, ok := adapterFor(provider)
	return ok && a.traits.groupedContextOnStderr
}

// unregisteredProvider is defense in depth. The entry point rejects a name
// that surface.Parse refuses before it gets here, and the registry holds an
// adapter for every surface, so reaching this means a caller skipped that
// check. Fail closed and say so.
func unregisteredProvider(provider string) Rendered {
	return Rendered{
		Stderr: []byte(fmt.Sprintf("agx: no adapter registered for provider %q\n", provider)),
		Exit:   hookspec.ExitUsage,
	}
}

// ---------------------------------------------------------------------------
// Surface traits and the render branches every surface shares
// ---------------------------------------------------------------------------

// traits are the few constants that differ between surfaces on the paths every
// surface shares. A surface's own file supplies one; the helpers below consume
// it.
type traits struct {
	// name is the surface's registry key and the prefix on its diagnostics.
	name string

	// label is the surface's proper name inside a field diagnostic:
	// "Claude field tool_name must be a present string".
	label string

	// emptyBody is what this surface writes to stdout when it has no document
	// to emit. Empty for claude-code and copilot; codex reads a decision from
	// the document, so its empty body is `{}`.
	emptyBody []byte

	// errorExit is this surface's exit code for a fault: an unusable payload,
	// a bad argv, a crashed core, an unrenderable outcome. claude-code exits 1,
	// which Claude Code treats as non-blocking; copilot and codex exit 0,
	// because on copilot any non-zero preToolUse exit is a deny and on codex a
	// decision is read from the document instead (docs/surfaces/*.md, Hook
	// protocol).
	errorExit int

	// failedMessage is the diagnostic used when neither captured core stderr
	// nor the outcome message carries one.
	failedMessage string

	// groupedContextOnStderr is true when the grouped event shows the joined
	// context on stderr. Copilot renders a context outcome as no document, so
	// stderr is the only place the context can appear; the single check stays
	// silent. Antigravity also has no context channel, yet its grouped context
	// stays off stderr, so this is a trait and not a property of the surface.
	groupedContextOnStderr bool
}

// renderPass is the pass/continue branch: no diagnostic, no objection, and
// this surface's empty body.
func renderPass(t traits) Rendered {
	return Rendered{Stdout: t.emptyBody, Exit: hookspec.ExitOK}
}

// renderError is the error branch. The diagnostic is the core's captured
// stderr with its trailing newlines trimmed, then the outcome's message, then
// the surface's last-resort message; exactly one newline follows it.
func renderError(t traits, message string, coreStderr []byte) Rendered {
	diagnostic := string(bytes.TrimRight(coreStderr, "\n"))
	if diagnostic == "" {
		diagnostic = message
	}
	if diagnostic == "" {
		diagnostic = t.failedMessage
	}
	return Rendered{
		Stdout: t.emptyBody,
		Stderr: []byte(diagnostic + "\n"),
		Exit:   t.errorExit,
	}
}

// rejectInvalidOutcome is the response when a core returns an outcome kind
// that has no rendering on this event — a deny after the tool already ran,
// for instance. It is a fault, not a decision: the surface's empty body, a
// diagnostic naming the event and the kind, and the surface's error exit.
func rejectInvalidOutcome(t traits, providerEvent, kind string) Rendered {
	return Rendered{
		Stdout: t.emptyBody,
		Stderr: []byte(fmt.Sprintf("%s: %s core produced an invalid %s outcome\n", t.name, providerEvent, kind)),
		Exit:   t.errorExit,
	}
}

// rejectMalformed is the malformed-payload rejection: the same text on every
// surface — `<surface>: malformed <Event> payload: <diagnostic>` — in the
// surface's own shape, which traits carries.
func rejectMalformed(t traits, providerEvent string, diagnostic error) *Rendered {
	return &Rendered{
		Stdout: t.emptyBody,
		Stderr: []byte(fmt.Sprintf("%s: malformed %s payload: %v\n", t.name, providerEvent, diagnostic)),
		Exit:   t.errorExit,
	}
}

// ---------------------------------------------------------------------------
// Payload decoding
// ---------------------------------------------------------------------------

// errTrailingData is what decodeJSONDocument returns when a payload carries a
// second document or any other non-whitespace tail.
var errTrailingData = errors.New("trailing data after the JSON document")

// errPayloadNotObject is what decodePayload returns for a document that
// parsed but is not an object. Normalize words it per surface.
var errPayloadNotObject = errors.New("payload is not an object")

// decodePayload runs the checks every surface applies to its raw stdin: valid
// UTF-8, exactly one JSON document, an object. Number literals are kept as
// json.Number so they reach the contract verbatim.
func decodePayload(raw []byte) (map[string]any, error) {
	if !utf8.Valid(raw) {
		return nil, errors.New("payload is not valid UTF-8")
	}
	decoded, err := decodeJSONDocument(raw)
	if err != nil {
		return nil, fmt.Errorf("payload is not valid JSON: %w", err)
	}
	payload, ok := decoded.(map[string]any)
	if !ok {
		return nil, errPayloadNotObject
	}
	return payload, nil
}

// decodeJSONDocument parses exactly one JSON document with number literals
// preserved, rejecting any non-whitespace tail as trailing data.
func decodeJSONDocument(raw []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	if contract.HasTrailingBytes(dec, raw) {
		return nil, errTrailingData
	}
	return value, nil
}

// requiredString reads a field that must be present and a string. Null,
// absent and any other type share one diagnostic.
func requiredString(t traits, payload map[string]any, name string) (string, error) {
	value, ok := payload[name].(string)
	if !ok {
		return "", fieldError(t, name, "a present string")
	}
	return value, nil
}

// optionalString reads a field that may be absent or null but must be a
// string when it carries a value. nil is JSON null.
func optionalString(t traits, payload map[string]any, name string) (*string, error) {
	value := payload[name]
	if value == nil {
		return nil, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, fieldError(t, name, "a string or null")
	}
	return &text, nil
}

// ---------------------------------------------------------------------------
// Document emission
// ---------------------------------------------------------------------------

// encodeDocument renders one response document: compact, HTML escaping off,
// exactly one trailing newline. json.Encoder is the only encoder that offers
// all three.
//
// It returns an error rather than panicking: a panic would exit 1 through the
// recover in internal/checkrun on every surface, and copilot and codex must
// fail open at 0. The failure posture belongs to the caller's own error branch.
func encodeDocument(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ---------------------------------------------------------------------------
// Neutral-event parts
// ---------------------------------------------------------------------------

// neutralEvent adds the provider diagnostic context to typed construction.
func neutralEvent(parts contract.EventParts) (hookspec.Event, error) {
	normalized, err := contract.NewEvent(parts)
	if err != nil {
		return hookspec.Event{}, fmt.Errorf("payload cannot be normalized: %w", err)
	}
	return normalized, nil
}

// runtimeRoots builds the runtime part from the environment. Each host
// exports its own pair of root variables (docs/surfaces/*.md); the vault
// override is Agentics's own and common to every surface. An unset and an
// exported-but-empty variable both become nil, and LogRoot is the data root
// when there is one, else the plugin root.
func runtimeRoots(pluginRootVar, dataRootVar string) contract.NeutralRuntime {
	pluginRoot := rootOrNil(pluginRootVar)
	dataRoot := rootOrNil(dataRootVar)
	logRoot := dataRoot
	if logRoot == nil {
		logRoot = pluginRoot
	}
	return contract.NeutralRuntime{
		PluginRoot:    pluginRoot,
		DataRoot:      dataRoot,
		VaultOverride: rootOrNil("AGX_VAULT"),
		LogRoot:       logRoot,
	}
}

// rootOrNil reads one runtime variable; unset and empty both become nil.
func rootOrNil(name string) *string {
	if value := os.Getenv(name); value != "" {
		return &value
	}
	return nil
}

// fieldError is the diagnostic for a payload field that fails its rule:
// `<Label> field <name> must be <requirement>`.
func fieldError(t traits, name, requirement string) error {
	return fmt.Errorf("%s field %s must be %s", t.label, name, requirement)
}
