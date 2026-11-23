package toolcall

import (
	"sort"
	"strings"
)

// Overrides is one reading of the environment override containers of a call.
// A container is any key of any layer of Layers that equals env or environment
// in any letter case. The containers are collected once, by Environment, and
// no accessor reads the input again.
//
// Each consumer keeps its own posture for what it reads here:
//
//   - command-guardian's empty-pager proof needs Canonical and a certain
//     Values reading.
//   - command-guardian's alternate-config reading uses Names and ignores
//     uncertainty, so a variant or malformed container still yields its keys.
//   - secrets-guardian treats an uncertain Values reading as unknown CDPATH,
//     glob options, or ripgrep configuration.
//   - The execution context withholds proof whenever Present is true.
//
// The zero Overrides reads as a call with no container.
type Overrides struct {
	containers []container
}

// container is one override container as Environment found it.
type container struct {
	// canonical is true when the key is spelled exactly env or environment.
	canonical bool
	// object is false when the container is not a JSON object, null included.
	object bool
	// entries holds an object container's entries sorted by key bytes.
	entries []entry
}

type entry struct {
	key   string
	value any
}

// Environment collects the override containers in layer order. Inside one
// layer the containers sort by key bytes, so env comes before environment.
func (c Call) Environment() Overrides {
	var overrides Overrides
	for _, layer := range c.Layers() {
		var keys []string
		for key := range layer {
			if strings.EqualFold(key, "env") || strings.EqualFold(key, "environment") {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		for _, key := range keys {
			found := container{canonical: key == "env" || key == "environment"}
			if values, ok := layer[key].(map[string]any); ok {
				found.object = true
				found.entries = make([]entry, 0, len(values))
				for name, value := range values {
					found.entries = append(found.entries, entry{name, value})
				}
				sort.Slice(found.entries, func(i, j int) bool { return found.entries[i].key < found.entries[j].key })
			}
			overrides.containers = append(overrides.containers, found)
		}
	}
	return overrides
}

// Present reports whether the call has at least one container.
func (o Overrides) Present() bool { return len(o.containers) != 0 }

// Canonical reports whether the containers have the one shape whose effect is
// known: no container, or exactly one, spelled exactly env or environment, that
// is an object whose values are all strings.
func (o Overrides) Canonical() bool {
	switch len(o.containers) {
	case 0:
		return true
	case 1:
	default:
		return false
	}
	only := o.containers[0]
	if !only.canonical || !only.object {
		return false
	}
	for _, e := range only.entries {
		if _, ok := e.value.(string); !ok {
			return false
		}
	}
	return true
}

// Names returns every key of every object container, whatever the container's
// spelling, in layer order with each container's keys sorted. Duplicates are
// kept. It returns nil when no object container has a key, and a new slice on
// every call.
func (o Overrides) Names() []string {
	var names []string
	for _, found := range o.containers {
		for _, e := range found.entries {
			names = append(names, e.key)
		}
	}
	return names
}

// Values returns every string value an object container gives under the key
// spelled exactly name, in layer order, or nil when none does.
//
// uncertain is true when a container is not an object, null included, when a
// container is spelled other than env or environment, when a container gives
// name a null or non-string value, or when a container has a key that equals
// name in another letter case. The executor may still set the variable from
// any of those, so none can be read as absent, and a variant container makes
// the reading uncertain even when it lacks name. uncertain is the "not
// readable" result and never means "nothing found": a caller that sees it must
// not trust values, whether or not values is empty.
func (o Overrides) Values(name string) (values []string, uncertain bool) {
	for _, found := range o.containers {
		if !found.canonical || !found.object {
			uncertain = true
		}
		for _, e := range found.entries {
			if e.key != name {
				if strings.EqualFold(e.key, name) {
					uncertain = true
				}
				continue
			}
			text, ok := e.value.(string)
			if !ok {
				uncertain = true
				continue
			}
			values = append(values, text)
		}
	}
	return values, uncertain
}
