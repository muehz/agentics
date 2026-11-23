package contract

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
)

// readerTail reads the tail through dec.Buffered and the unread reader;
// HasTrailingBytes must agree with it on every payload.
func readerTail(dec *json.Decoder, rest io.Reader) bool {
	tail, _ := io.ReadAll(io.MultiReader(dec.Buffered(), rest))
	return len(bytes.TrimFunc(tail, jsonSpace)) != 0
}

func TestHasTrailingBytes(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		want    bool
	}{
		{"no tail", `{"kind":"pass"}`, false},
		{"json whitespace tail", "{\"kind\":\"pass\"} \t\r\n", false},
		{"information separator tail", "{\"kind\":\"pass\"}\x1c", true},
		{"no-break space tail", "{\"kind\":\"pass\"}\xc2\xa0", true},
		{"stray close brace", `{"kind":"pass"} }`, true},
		{"second document", `{"kind":"pass"}{"kind":"pass"}`, true},
		{"long whitespace tail", `{"kind":"pass"}` + strings.Repeat(" ", 8192), false},
		{"tail past the first buffer", `{"kind":"pass"}` + strings.Repeat(" ", 8192) + "x", true},
		{"scalar document", `12 `, false},
		{"leading whitespace", "  {\"a\":1}\n", false},
		{"large document, whitespace tail", `{"a":"` + strings.Repeat("x", 10000) + `"}  `, false},
		{"large document, non-whitespace tail", `{"a":"` + strings.Repeat("x", 10000) + `"} y`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			raw := []byte(c.payload)
			dec := json.NewDecoder(bytes.NewReader(raw))
			var first json.RawMessage
			if err := dec.Decode(&first); err != nil {
				t.Fatalf("decoding the first document: %v", err)
			}
			if got := HasTrailingBytes(dec, raw); got != c.want {
				t.Errorf("HasTrailingBytes(%q) = %v, want %v", c.payload, got, c.want)
			}
			reader := bytes.NewReader(raw)
			refDec := json.NewDecoder(reader)
			if err := refDec.Decode(&first); err != nil {
				t.Fatal(err)
			}
			if ref := readerTail(refDec, reader); ref != c.want {
				t.Errorf("reference reader tail(%q) = %v, want %v", c.payload, ref, c.want)
			}
		})
	}
}
