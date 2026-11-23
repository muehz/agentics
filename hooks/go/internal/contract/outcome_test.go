package contract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOutcomeConstructors(t *testing.T) {
	for _, text := range []string{"", "review line 1\n\"line 2\" <>& café 🚀", strings.Repeat("x", 1100000)} {
		cases := []struct {
			value         Outcome
			kind, payload string
		}{
			{Pass(), KindPass, ""}, {Continue(), KindContinue, ""},
			{Deny(text), KindDeny, text},
			{Context(text), KindContext, text}, {Error(text), KindError, text},
		}
		for _, tc := range cases {
			if !tc.value.Valid() || tc.value.Kind() != tc.kind || tc.value.Payload() != tc.payload {
				t.Fatalf("kind %s: result does not preserve kind and payload", tc.kind)
			}
		}
	}
}

func TestInvalidOutcomes(t *testing.T) {
	for _, value := range []Outcome{{}, {kind: "unknown"}, {kind: KindPass, payload: "unexpected"}} {
		if value.Valid() {
			t.Fatalf("invalid outcome accepted: %#v", value)
		}
	}
}

func TestConstructorsReplaceEachInvalidUTF8Byte(t *testing.T) {
	for _, input := range []string{"bad\xffbytes", "\xff\xfe", "a\xe2\x82z", "\xc0\xaf", "valid � café"} {
		encoded, err := json.Marshal(input)
		if err != nil {
			t.Fatal(err)
		}
		var want string
		if err := json.Unmarshal(encoded, &want); err != nil {
			t.Fatal(err)
		}
		for _, build := range []func(string) Outcome{Deny, Context, Error} {
			got := build(input)
			if !got.Valid() || got.Payload() != want {
				t.Fatalf("input %q: got %q, want %q", input, got.Payload(), want)
			}
		}
	}
}
