package provider

import (
	"fmt"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/contract"
	"github.com/muehz/agentics/hooks/go/internal/hookspec"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

func TestRegisterRejectsDuplicatesAndIncompleteAdapters(t *testing.T) {
	expectPanic := func(needle string, f func()) {
		t.Helper()
		defer func() {
			r := recover()
			if r == nil {
				t.Errorf("expected a panic mentioning %q", needle)
			} else if !strings.Contains(fmt.Sprint(r), needle) {
				t.Errorf("panic %q does not mention %q", fmt.Sprint(r), needle)
			}
		}()
		f()
	}
	expectPanic("registered twice", func() { register(surface.ClaudeCode, registry[surface.ClaudeCode]) })
	expectPanic("incomplete adapter", func() { register("half-surface", adapter{traits: traits{name: "half-surface"}}) })
	expectPanic("registered with traits named", func() { register("mislabeled-surface", registry[surface.Codex]) })
	expectPanic("names no tool-use event", func() {
		register("no-event-surface", adapter{
			traits:    traits{name: "no-event-surface", label: "No Event"},
			normalize: codexNormalize,
			render:    codexRender,
		})
	})
	if len(registry) != len(surface.All()) {
		t.Errorf("registry holds %d adapters after the refused registrations, want %d", len(registry), len(surface.All()))
	}
}

func TestEverySurfaceHasAnAdapter(t *testing.T) {
	all := surface.All()
	for _, id := range all {
		a, ok := registry[id]
		if !ok {
			t.Errorf("surface %q has no adapter", id)
			continue
		}
		if a.traits.name != string(id) {
			t.Errorf("surface %q adapter traits.name = %q", id, a.traits.name)
		}
		if a.traits.label != id.Label() {
			t.Errorf("surface %q adapter traits.label = %q, want %q", id, a.traits.label, id.Label())
		}
	}
	for id := range registry {
		if _, ok := surface.Parse(string(id)); !ok {
			t.Errorf("adapter registered under %q, which is not a surface", id)
		}
	}
	if len(registry) != len(all) {
		t.Errorf("registry holds %d adapters, surface roster has %d", len(registry), len(all))
	}
}

func TestUnregisteredProviderFailsClosed(t *testing.T) {
	check := func(stage string, r *Rendered) {
		t.Helper()
		if r == nil || r.Exit != hookspec.ExitUsage || !strings.Contains(string(r.Stderr), `no adapter registered for provider "bogus"`) {
			t.Errorf("%s: %+v", stage, r)
		}
	}
	_, reject := Dispatch("bogus", "command-guardian", nil)
	check("Dispatch", reject)
	_, malformed := Normalize("bogus", "PreToolUse", []byte("{}"))
	check("Normalize", malformed)
	rendered := Render("bogus", "PreToolUse", contract.Pass(), nil)
	check("Render", &rendered)
}
