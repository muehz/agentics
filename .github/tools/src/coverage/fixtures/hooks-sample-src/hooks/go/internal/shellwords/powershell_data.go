package shellwords

import (
	"context"
	"strings"
)

// PowerShellFileData proves a whole call consists of a literal here-string
// piped directly to the closed plain-file Set-Content sink. Authority comes
// from the original request, never from parsing the masked text. Unsupported
// source or consumer syntax retains the original evidence in every caller.
func PowerShellFileData(ctx context.Context, source string, execution ExecutionContext) (string, bool, error) {
	if execution.Dialect != PowerShell || !execution.AllowsNewProof() {
		return source, false, nil
	}
	view, err := PowerShellLiteralData(ctx, source)
	if err != nil || !view.Complete {
		return source, false, err
	}
	ok, err := plainPowerShellFileSink(ctx, source[view.End:])
	if err != nil || !ok {
		return source, false, err
	}
	return view.Text, true, nil
}

// plainPowerShellFileSink reads the entire original suffix. This grammar has
// no aliases, parameters with unknown arity, providers, expansions or extra
// operations; the literal source is the sole pipeline predecessor.
func plainPowerShellFileSink(ctx context.Context, suffix string) (bool, error) {
	i := 0
	space := func(trailing bool) error {
		for i < len(suffix) && (suffix[i] == ' ' || suffix[i] == '\t' || trailing && (suffix[i] == '\r' || suffix[i] == '\n')) {
			if i%(64<<10) == 0 {
				if err := ctx.Err(); err != nil {
					return err
				}
			}
			i++
		}
		return ctx.Err()
	}
	if err := space(false); err != nil {
		return false, err
	}
	if i == len(suffix) || suffix[i] != '|' {
		return false, nil
	}
	i++
	if err := space(false); err != nil {
		return false, err
	}
	for _, word := range []string{"Set-Content", "-LiteralPath"} {
		if len(suffix)-i < len(word) || !strings.EqualFold(suffix[i:i+len(word)], word) {
			return false, nil
		}
		i += len(word)
		before := i
		if err := space(false); err != nil {
			return false, err
		}
		if i == before {
			return false, nil
		}
	}
	var quote byte
	if i < len(suffix) && (suffix[i] == '\'' || suffix[i] == '"') {
		quote = suffix[i]
		i++
	}
	start, segment := i, i
	for i < len(suffix) {
		if i%(64<<10) == 0 {
			if err := ctx.Err(); err != nil {
				return false, err
			}
		}
		c := suffix[i]
		if c == quote && quote != 0 || quote == 0 && (c == ' ' || c == '\t' || c == '\r' || c == '\n') {
			break
		}
		if c == '/' || c == '\\' {
			if !plainPowerShellFileSegment(suffix[segment:i]) {
				return false, nil
			}
			segment = i + 1
		} else if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '.' || c == '_' || c == '-') {
			return false, nil
		}
		i++
	}
	if i == start || suffix[start] == '-' || !plainPowerShellFileSegment(suffix[segment:i]) {
		return false, nil
	}
	if quote != 0 {
		if i == len(suffix) || suffix[i] != quote {
			return false, nil
		}
		i++
	}
	if err := space(true); err != nil {
		return false, err
	}
	return i == len(suffix), nil
}

func plainPowerShellFileSegment(segment string) bool {
	if segment == "" || segment[len(segment)-1] == '.' {
		return false
	}
	// Windows device names retain their meaning with an extension and inside
	// directories. Reject them on every host so provider tests agree.
	base, _, _ := strings.Cut(segment[:min(len(segment), 5)], ".")
	if len(base) > 4 {
		return true
	}
	base = strings.ToUpper(base)
	switch base {
	case "CON", "PRN", "AUX", "NUL":
		return false
	}
	return !(len(base) == 4 && (base[:3] == "COM" || base[:3] == "LPT") && base[3] >= '0' && base[3] <= '9')
}

// PowerShellLiteralView describes lexical termination, not consumer safety.
// Complete never grants permission to ignore the literal or its surrounding code.
// Offsets are bytes in the supplied source. Text preserves the opener, closing
// marker, suffix, and all line endings; only body bytes are blanked.
type PowerShellLiteralView struct {
	Text                    string
	BodyStart, BodyEnd, End int
	Complete                bool
}

// PowerShellLiteralData returns a lexical view of a literal beginning at byte zero.
// The first physical closing marker is final even if its suffix is invalid or
// executable. Callers must independently prove the entire source and consumer;
// this view does not parse or authorize the suffix. Incomplete or canceled work
// returns the original source with zero offsets and Complete false.
func PowerShellLiteralData(ctx context.Context, source string) (PowerShellLiteralView, error) {
	original := PowerShellLiteralView{Text: source}
	if err := ctx.Err(); err != nil {
		return original, err
	}
	start := 0
	switch {
	case strings.HasPrefix(source, "@'\n"):
		start = 3
	case strings.HasPrefix(source, "@'\r\n"):
		start = 4
	default:
		return original, nil
	}
	close, err := firstPowerShellLiteralClose(ctx, source)
	if err != nil || close < start {
		return original, err
	}
	// Build in bounded pieces so cancellation never publishes a partial view.
	// Builder.String avoids an additional uninterruptible full-source copy.
	var out strings.Builder
	out.Grow(len(source))
	var buf [64 << 10]byte
	for offset := 0; offset < len(source); {
		if err := ctx.Err(); err != nil {
			return original, err
		}
		end := min(offset+len(buf), len(source))
		n := copy(buf[:], source[offset:end])
		for i := max(start, offset); i < min(close, end); i++ {
			if buf[i-offset] != '\r' && buf[i-offset] != '\n' {
				buf[i-offset] = ' '
			}
		}
		out.Write(buf[:n])
		offset = end
	}
	if err := ctx.Err(); err != nil {
		return original, err
	}
	return PowerShellLiteralView{Text: out.String(), BodyStart: start, BodyEnd: close, End: close + 2, Complete: true}, nil
}

func firstPowerShellLiteralClose(ctx context.Context, text string) (int, error) {
	const chunk = 64 << 10
	for start := 0; start < len(text); {
		if err := ctx.Err(); err != nil {
			return -1, err
		}
		end := min(start+chunk, len(text))
		offset := strings.IndexByte(text[start:end], '\n')
		if offset < 0 {
			start = end
			continue
		}
		start += offset + 1
		if strings.HasPrefix(text[start:], "'@") {
			return start, nil
		}
	}
	return -1, nil
}
