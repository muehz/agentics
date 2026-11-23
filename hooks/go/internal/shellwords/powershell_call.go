package shellwords

import (
	"context"
	"strings"
)

// PowerShellEnvironmentCall holds a complete literal setter's scalar arguments.
// A non-nil call with no arguments records a recognized but unresolved setter.
// Unresolved calls retain ordinary splitting and their executable children.
type PowerShellEnvironmentCall struct {
	Arguments []string
	// NullValue distinguishes the exact unquoted null second argument.
	NullValue bool
}

const environmentMethod = "::SetEnvironmentVariable"

func environmentMethodEnd(s string) int {
	for _, prefix := range []string{"[Environment]", "[System.Environment]"} {
		n := len(prefix) + len(environmentMethod)
		if len(s) >= n && strings.EqualFold(s[:len(prefix)], prefix) && strings.EqualFold(s[len(prefix):n], environmentMethod) {
			if len(s) == n || strings.ContainsRune(" (\t\r\n", rune(s[n])) {
				return n
			}
		}
	}
	return 0
}

// literalEnvironmentCall proves literal setter arguments, permitting an exact
// unquoted null only in the second argument position.
// Polling every 64 KiB bounds cancellation latency without imposing a new
// candidate-size limit. Failed proof never consumes any source text.
func literalEnvironmentCall(ctx context.Context, s string) (int, []string) {
	end, args, _ := literalEnvironmentCallValue(ctx, s)
	return end, args
}

func literalEnvironmentCallValue(ctx context.Context, s string) (int, []string, bool) {
	if ctx.Err() != nil {
		return 0, nil, false
	}
	i := environmentMethodEnd(s)
	if i == 0 {
		return 0, nil, false
	}
	nextCheck := 0
	check := func() bool {
		if i >= nextCheck {
			// A doubled quote can advance two bytes between checks.
			nextCheck = i + 65534
			return ctx.Err() == nil
		}
		return true
	}
	space := func() bool {
		for i < len(s) && strings.ContainsRune(" \t\r\n", rune(s[i])) {
			if !check() {
				return false
			}
			i++
		}
		return check()
	}
	if !space() || i >= len(s) || s[i] != '(' {
		return 0, nil, false
	}
	i++
	var args []string
	nullValue := false
	for len(args) < 3 {
		if !space() || i >= len(s) {
			return 0, nil, false
		}
		if len(args) == 1 && strings.HasPrefix(s[i:], "$null") {
			i += len("$null")
			if !space() || i >= len(s) || s[i] != ')' && s[i] != ',' {
				return 0, nil, false
			}
			args = append(args, "")
			nullValue = true
			if s[i] == ')' {
				if ctx.Err() != nil {
					return 0, nil, false
				}
				return i + 1, args, true
			}
			i++
			continue
		}
		if s[i] != '\'' && s[i] != '"' {
			return 0, nil, false
		}
		quote := s[i]
		i++
		var value strings.Builder
		closed := false
		for i < len(s) {
			if !check() {
				return 0, nil, false
			}
			c := s[i]
			i++
			if c == quote {
				if i < len(s) && s[i] == quote {
					value.WriteByte(quote)
					i++
					continue
				}
				closed = true
				break
			}
			if quote == '"' && (c == '$' || c == '`') {
				return 0, nil, false
			}
			value.WriteByte(c)
		}
		if !closed || !space() || i >= len(s) {
			return 0, nil, false
		}
		args = append(args, value.String())
		if s[i] == ')' {
			i++
			if len(args) < 2 || ctx.Err() != nil {
				return 0, nil, false
			}
			return i, args, nullValue
		}
		if s[i] != ',' {
			return 0, nil, false
		}
		i++
	}
	return 0, nil, false
}

func environmentCallMetadata(ctx context.Context, raw string, words []Word) *PowerShellEnvironmentCall {
	raw = strings.TrimSpace(raw)
	if end, args, nullValue := literalEnvironmentCallValue(ctx, raw); end > 0 && strings.TrimSpace(raw[end:]) == "" && ctx.Err() == nil {
		return &PowerShellEnvironmentCall{Arguments: args, NullValue: nullValue}
	}
	if environmentMethodEnd(raw) > 0 {
		return &PowerShellEnvironmentCall{}
	}
	for i, word := range words {
		if i%256 == 0 && ctx.Err() != nil {
			return &PowerShellEnvironmentCall{}
		}
		// Expression assignment may precede a call. Its result is not needed
		// to recognize the unresolved executable setter. Quoted examples never
		// qualify, and unrelated method names remain ordinary words.
		if !word.Quoted && environmentMethodEnd(word.Raw) > 0 && (i == 0 || words[i-1].Text == "=") {
			return &PowerShellEnvironmentCall{}
		}
		if i == 0 && !word.Quoted && strings.HasPrefix(word.Raw, "$") {
			// The lexer can retain a tight assignment as one word. Inspect
			// only the expression immediately after its assignment operator.
			for j := 1; j < len(word.Raw); j++ {
				if j%32768 == 0 && ctx.Err() != nil {
					return &PowerShellEnvironmentCall{}
				}
				c := word.Raw[j]
				if c == '=' {
					if environmentMethodEnd(word.Raw[j+1:]) > 0 {
						return &PowerShellEnvironmentCall{}
					}
					break
				}
				if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
					break
				}
			}
		}
	}
	return nil
}
