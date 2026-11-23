// Package contract owns typed provider-neutral events and hook outcomes.
package contract

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// ---------------------------------------------------------------------------
// Emission
// ---------------------------------------------------------------------------

// encode renders v compact, with HTML escaping off and one trailing newline.
// json.Encoder is the only encoder that offers all three: json.Marshal always
// escapes '<', '>' and '&' and never appends the newline.
func encode(v any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// jsonSpace reports whether r is JSON whitespace: the four characters a JSON
// parser may skip between tokens. No wider set is tolerated anywhere in this
// package.
func jsonSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n'
}

// HasTrailingBytes reports whether anything but JSON whitespace follows the
// document dec has just decoded from raw, the whole input dec was built on.
// dec.InputOffset is the end of that document, so the tail is the rest of
// raw: the same bytes dec.Buffered and the unread reader would yield,
// without copying them. Decoder.More cannot answer this question — it
// reports false at a stray ']' or '}', which is trailing content.
func HasTrailingBytes(dec *json.Decoder, raw []byte) bool {
	return len(bytes.TrimFunc(raw[dec.InputOffset():], jsonSpace)) != 0
}

// rejectNonFinite walks a decoded document for a float literal that does not
// fit a float64. Only a literal containing '.', 'e' or 'E' is a float; an
// integer literal has no range limit here and is preserved verbatim, so 1e999
// is rejected while a 400-digit integer passes.
func rejectNonFinite(value any) error {
	switch v := value.(type) {
	case nil, bool, string:
		return nil
	case json.Number:
		if !strings.ContainsAny(v.String(), ".eE") {
			return nil
		}
		f, err := v.Float64()
		if err != nil || math.IsInf(f, 0) || math.IsNaN(f) {
			return fmt.Errorf("contract: non-finite JSON constant: %s", v.String())
		}
	case map[string]any:
		for _, item := range v {
			if err := rejectNonFinite(item); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range v {
			if err := rejectNonFinite(item); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("contract: unexpected decoded JSON type %T", value)
	}
	return nil
}
