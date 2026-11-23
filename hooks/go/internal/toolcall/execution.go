package toolcall

import (
	"path/filepath"
	"strings"

	"github.com/muehz/agentics/hooks/go/internal/pathshape"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
	"github.com/muehz/agentics/hooks/go/internal/surface"
	"github.com/muehz/agentics/hooks/go/internal/toolname"
)

// maxEvidenceKeys bounds the keys of one argument layer. A layer with more is
// refused rather than walked, so hostile input cannot make the read costly.
const maxEvidenceKeys = 64

// Execution resolves the call's shell dialect and whether its arguments give
// authority to establish a new exemption. Exact PowerShell is a neutral source
// contract, not evidence of a native Codex PowerShell tool, and Codex
// exec_command remains Bash. No provider shell-selector binding is qualified:
// even a well-formed selector withholds proof.
//
// The checks run in a fixed order and the first that fails names the reason,
// so a call that fails several always gets the same one. The dialect is set
// before any check, so a refused call still reports it.
func (c Call) Execution() shellwords.ExecutionContext {
	execution := shellwords.ExecutionContext{Dialect: shellwords.POSIX}
	if strings.EqualFold(toolname.Base(c.name), "PowerShell") {
		execution.Dialect = shellwords.PowerShell
	}
	refuse := func(reason string) shellwords.ExecutionContext {
		execution.Reason = reason
		return execution
	}
	if c.name != "PowerShell" && c.name != "Bash" {
		return refuse("noncanonical shell tool")
	}
	provider, known := surface.Parse(c.surface)
	if !known {
		return refuse("unqualified provider")
	}
	layers, readable := c.ProofLayers()
	if !readable {
		if c.envelopeFault() == faultDeep {
			return refuse("deep argument envelope")
		}
		return refuse("malformed argument envelope")
	}
	if len(layers) > 1 && !c.commandEvidenceAgrees(provider, layers[1]) {
		return refuse("competing command evidence")
	}
	selectorSeen := false
	for _, layer := range layers {
		// No nested metadata container has execution authority. Reject it
		// instead of walking arbitrary metadata looking for hidden selectors.
		// The sole permitted container is the validated canonical value layer.
		if len(layer) > maxEvidenceKeys {
			return refuse("argument evidence limit")
		}
		for key, value := range layer {
			if key != "value" {
				switch value.(type) {
				case map[string]any, []any:
					return refuse("unqualified nested arguments")
				}
			}
			if strings.EqualFold(key, "shell") {
				selectorSeen = true
			}
		}
	}
	if selectorSeen {
		return refuse("unqualified shell selector")
	}
	// An override may replace the inherited environment, changing PATH or
	// PATHEXT even without an explicit entry. No merge contract is qualified
	// here, so no override shape establishes authority.
	if c.Environment().Present() {
		return refuse("unqualified execution environment")
	}
	if _, ok := c.Command(); !ok {
		return refuse("missing command evidence")
	}
	execution.Proven = true
	return execution
}

// commandEvidenceAgrees reports whether the value layer names the same command
// as the root. Copilot and Antigravity carry the provider's own command key,
// cmd and CommandLine, beside command; every key present must be the root
// command, and the provider's key takes precedence over command.
func (c Call) commandEvidenceAgrees(provider surface.ID, args map[string]any) bool {
	rootCommand, rootOK := c.Command()
	childCommand, childOK := args["command"].(string)
	var altKey string
	switch provider {
	case surface.Copilot:
		altKey = "cmd"
	case surface.Antigravity:
		altKey = "CommandLine"
	}
	if altKey != "" {
		if alt, present := args[altKey]; present {
			childCommand, childOK = alt.(string)
		}
		for _, key := range []string{altKey, "command"} {
			if value, present := args[key]; present {
				text, ok := value.(string)
				if !ok || text != rootCommand {
					return false
				}
			}
		}
	}
	return rootOK && childOK && rootCommand == childCommand
}

// ProvenCwd returns the event cwd only when it is an absolute path and every
// cwd or workdir argument in Layers agrees with it, and the empty string
// otherwise, including for an absent or empty event cwd. The event contract
// accepts any string, and a relative one names no single directory. A tool's
// directory override need not be reflected in the event cwd, so a disagreeing
// or malformed override leaves the working directory unproven.
func (c Call) ProvenCwd() string {
	if c.cwd == "" {
		return ""
	}
	host := pathshape.Current()
	native := host.Native(c.cwd)
	base := filepath.ToSlash(native)
	if !filepath.IsAbs(native) && !shellwords.IsAbsoluteShellPath(base) {
		return ""
	}
	for _, args := range c.Layers() {
		for _, key := range []string{"workdir", "cwd"} {
			value, present := args[key]
			if !present || value == nil {
				continue
			}
			dir, ok := value.(string)
			if !ok {
				return ""
			}
			if dir == "" {
				continue
			}
			native := host.Native(dir)
			explicit := filepath.ToSlash(native)
			equal := explicit == base
			if host.Windows() {
				equal = strings.EqualFold(explicit, base)
			}
			if !filepath.IsAbs(native) || !equal {
				return ""
			}
		}
	}
	return c.cwd
}

// ScriptOptions returns the parser options for one guard's script inspection.
// A nil reader disables it, so the result is the zero value. Otherwise the
// options carry the reader, the execution context, and the proven working
// directory, and leave Host zero, which is the running host.
func (c Call) ScriptOptions(reader shellwords.ScriptReader) shellwords.ScriptOptions {
	if reader == nil {
		return shellwords.ScriptOptions{}
	}
	return shellwords.ScriptOptions{Reader: reader, Context: c.Execution(), Cwd: c.ProvenCwd()}
}
