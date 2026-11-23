package contract

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrEventInvalid is the handler-facing event rejection. Each handler maps
// this sentinel to its own fixed diagnostic.
var ErrEventInvalid = errors.New("contract: normalized event is invalid")

// NeutralEvent contains validated fields used by hook cores.
type NeutralEvent struct {
	Surface   string
	SessionID *string
	TurnID    *string
	Cwd       *string
	Runtime   NeutralRuntime
	Tool      NeutralTool
}

// NeutralRuntime preserves null separately from an explicitly empty root.
type NeutralRuntime struct {
	PluginRoot    *string
	DataRoot      *string
	VaultOverride *string
	LogRoot       *string
}

// NeutralTool holds the tool identity and its canonical input text.
type NeutralTool struct {
	Name *string
	// InputRaw retains number literals and unescaped HTML characters. The
	// command guardian scans this text, so rebuilding it changes behavior.
	InputRaw json.RawMessage
}

// DecodeInput uses plain encoding/json semantics: numbers become float64.
// An enormous integer is valid at construction but may fail here. The caller
// controls when decoding happens; cores that ignore input never incur its cost.
func (t NeutralTool) DecodeInput() (map[string]any, error) {
	var input map[string]any
	if err := json.Unmarshal(t.InputRaw, &input); err != nil {
		return nil, err
	}
	return input, nil
}

func holdsNUL(value any) bool {
	switch v := value.(type) {
	case nil, bool, json.Number:
		return false
	case string:
		return strings.ContainsRune(v, 0)
	case map[string]any:
		for key, item := range v {
			if strings.ContainsRune(key, 0) || holdsNUL(item) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if holdsNUL(item) {
				return true
			}
		}
	default:
		panic(fmt.Sprintf("contract: unexpected decoded JSON type %T", value))
	}
	return false
}
