package toolcall

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

// manyKeys returns n keys K00000, K00001, ... in byte order.
func manyKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("K%05d", i)
	}
	return keys
}

// objectOf maps every key to the same value.
func objectOf(keys []string, value any) map[string]any {
	m := make(map[string]any, len(keys))
	for _, key := range keys {
		m[key] = value
	}
	return m
}

// TestOverridesAccessors states all four accessors for each container shape.
// Every row probes Values with GIT_PAGER unless it names another variable.
func TestOverridesAccessors(t *testing.T) {
	const pager = "GIT_PAGER"
	large := manyKeys(10000)
	largeVariant := append(manyKeys(9999), "git_pager")

	tests := []struct {
		name      string
		input     map[string]any
		probe     string
		present   bool
		canonical bool
		names     []string
		values    []string
		uncertain bool
	}{
		{
			name: "no container", input: obj("command", "ls"),
			present: false, canonical: true, names: nil, values: nil, uncertain: false,
		},
		{
			name: "env", input: obj("env", obj("GIT_PAGER", "", "A", "1")),
			present: true, canonical: true, names: []string{"A", "GIT_PAGER"}, values: []string{""}, uncertain: false,
		},
		{
			name: "environment", input: obj("environment", obj("GIT_PAGER", "cat")),
			present: true, canonical: true, names: []string{"GIT_PAGER"}, values: []string{"cat"}, uncertain: false,
		},
		{
			name:    "env and environment in one layer read env first",
			input:   obj("environment", obj("GIT_PAGER", "b"), "env", obj("GIT_PAGER", "a")),
			present: true, canonical: false, names: []string{"GIT_PAGER", "GIT_PAGER"}, values: []string{"a", "b"}, uncertain: false,
		},
		{
			name: "ENV object", input: obj("ENV", obj("GIT_PAGER", "a")),
			present: true, canonical: false, names: []string{"GIT_PAGER"}, values: []string{"a"}, uncertain: true,
		},
		{
			name: "Environment object without the name", input: obj("Environment", obj("B", "1")),
			present: true, canonical: false, names: []string{"B"}, values: nil, uncertain: true,
		},
		{
			name: "ENV string", input: obj("ENV", "x"),
			present: true, canonical: false, names: nil, values: nil, uncertain: true,
		},
		{
			name: "container in the value layer", input: obj("command", "ls", "value", obj("env", obj("GIT_PAGER", ""))),
			present: true, canonical: true, names: []string{"GIT_PAGER"}, values: []string{""}, uncertain: false,
		},
		{
			name:    "container in both layers reads the root first",
			input:   obj("env", obj("GIT_PAGER", "a"), "value", obj("env", obj("GIT_PAGER", "b"))),
			present: true, canonical: false, names: []string{"GIT_PAGER", "GIT_PAGER"}, values: []string{"a", "b"}, uncertain: false,
		},
		{
			name: "null container", input: obj("env", nil),
			present: true, canonical: false, names: nil, values: nil, uncertain: true,
		},
		{
			name: "string container", input: obj("env", "GIT_PAGER="),
			present: true, canonical: false, names: nil, values: nil, uncertain: true,
		},
		{
			name: "list container", input: obj("environment", []any{"GIT_PAGER="}),
			present: true, canonical: false, names: nil, values: nil, uncertain: true,
		},
		{
			name: "number container", input: obj("env", json.Number("1")),
			present: true, canonical: false, names: nil, values: nil, uncertain: true,
		},
		{
			name: "empty object", input: obj("env", obj()),
			present: true, canonical: true, names: nil, values: nil, uncertain: false,
		},
		{
			name: "null value", input: obj("env", obj("GIT_PAGER", nil)),
			present: true, canonical: false, names: []string{"GIT_PAGER"}, values: nil, uncertain: true,
		},
		{
			name: "number value", input: obj("env", obj("GIT_PAGER", json.Number("0"))),
			present: true, canonical: false, names: []string{"GIT_PAGER"}, values: nil, uncertain: true,
		},
		{
			name: "empty-string value", input: obj("env", obj("GIT_PAGER", "")),
			present: true, canonical: true, names: []string{"GIT_PAGER"}, values: []string{""}, uncertain: false,
		},
		{
			name: "case-variant name", input: obj("env", obj("git_pager", "cat")),
			present: true, canonical: true, names: []string{"git_pager"}, values: nil, uncertain: true,
		},
		{
			name: "name beside its case variant", input: obj("env", obj("GIT_PAGER", "", "git_pager", "")),
			present: true, canonical: true, names: []string{"GIT_PAGER", "git_pager"}, values: []string{""}, uncertain: true,
		},
		{
			name: "name absent beside other keys", input: obj("env", obj("A", "1", "B", "2")),
			present: true, canonical: true, names: []string{"A", "B"}, values: nil, uncertain: false,
		},
		{
			name: "a non-string value under another name", input: obj("env", obj("A", json.Number("1"))), probe: "B",
			present: true, canonical: false, names: []string{"A"}, values: nil, uncertain: false,
		},
		{
			name: "ten thousand keys", input: obj("env", objectOf(large, "v")),
			present: true, canonical: true, names: large, values: nil, uncertain: false,
		},
		{
			name: "ten thousand keys with a variant of the name last", input: obj("env", objectOf(largeVariant, "v")),
			present: true, canonical: true, names: largeVariant, values: nil, uncertain: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			call, err := Read(buildEvent(t, tt.input))
			if err != nil {
				t.Fatal(err)
			}
			probe := tt.probe
			if probe == "" {
				probe = pager
			}
			overrides := call.Environment()
			if got := overrides.Present(); got != tt.present {
				t.Errorf("Present() = %v, want %v", got, tt.present)
			}
			if got := overrides.Canonical(); got != tt.canonical {
				t.Errorf("Canonical() = %v, want %v", got, tt.canonical)
			}
			if got := overrides.Names(); !reflect.DeepEqual(got, tt.names) {
				t.Errorf("Names() = %d names %.80q, want %d names %.80q", len(got), got, len(tt.names), tt.names)
			}
			values, uncertain := overrides.Values(probe)
			if !reflect.DeepEqual(values, tt.values) || uncertain != tt.uncertain {
				t.Errorf("Values(%q) = %q, %v, want %q, %v", probe, values, uncertain, tt.values, tt.uncertain)
			}
		})
	}
}

// TestOverridesZeroValueReadsAsNoContainer pins the zero Overrides.
func TestOverridesZeroValueReadsAsNoContainer(t *testing.T) {
	var overrides Overrides
	if overrides.Present() {
		t.Error("Present() = true, want false")
	}
	if !overrides.Canonical() {
		t.Error("Canonical() = false, want true")
	}
	if names := overrides.Names(); names != nil {
		t.Errorf("Names() = %q, want nil", names)
	}
	if values, uncertain := overrides.Values("GIT_PAGER"); values != nil || uncertain {
		t.Errorf("Values() = %q, %v, want nil, false", values, uncertain)
	}
}

// TestOverridesReadTheInputOnce shows that the accessors answer from what
// Environment collected: a write to the input afterwards changes nothing, so
// no accessor walks the input again.
func TestOverridesReadTheInputOnce(t *testing.T) {
	env := obj("GIT_PAGER", "")
	call := Call{input: obj("env", env)}
	overrides := call.Environment()

	env["GIT_PAGER"] = nil
	env["git_pager"] = "cat"
	call.input["ENV"] = "x"

	if !overrides.Canonical() {
		t.Error("Canonical() = false after a later write, want true")
	}
	if names := overrides.Names(); !reflect.DeepEqual(names, []string{"GIT_PAGER"}) {
		t.Errorf("Names() = %q after a later write, want [GIT_PAGER]", names)
	}
	if values, uncertain := overrides.Values("GIT_PAGER"); !reflect.DeepEqual(values, []string{""}) || uncertain {
		t.Errorf("Values() = %q, %v after a later write, want [\"\"], false", values, uncertain)
	}
}

// TestOverridesNamesReturnsAFreshSlice shows that a caller writing to the
// returned names cannot change a later answer.
func TestOverridesNamesReturnsAFreshSlice(t *testing.T) {
	overrides := Call{input: obj("env", obj("A", "1"))}.Environment()
	overrides.Names()[0] = "B"
	if names := overrides.Names(); !reflect.DeepEqual(names, []string{"A"}) {
		t.Errorf("Names() = %q, want [A]", names)
	}
}
