package skillcompat

import (
	"testing"
)

// TestCorrectionsThroughTheCommittedHook sends file content to the built hook to exercise two narrow exemptions of
// the secrets check through the Claude Code dispatch path. The check reports a variable with a
// key-style name that is assigned a value, and exempts a value that only says where the secret is read from: one of
// six environment accessors, unquoted and directly followed by its own bracket, and a member whose whole value is a
// user-config reference. Every near miss must stay denied, so that the exemption cannot be read as "any
// identifier before a bracket" or "any value that starts with a reference".
//
// The secrets check also guards writes to this checkout, so every content is put together from pieces and this
// file holds no line it would refuse.
func TestCorrectionsThroughTheCommittedHook(t *testing.T) {
	variable := "to" + "ken"
	assign := variable + " = "
	member := "api" + "_key"
	reference := "${user_config." + member + "}"
	// object returns a JSON object whose one member has the key-style name and the given value.
	object := func(value string) string {
		return "{\n  \"" + member + "\"" + ": \"" + value + "\"\n}\n"
	}
	accessors := []struct{ name, open, close string }{
		{"os.environ", "[", "]"},
		{"env::var", "(", ")"},
		{"std::env::var", "(", ")"},
		{"System.getenv", "(", ")"},
		{"Environment.GetEnvironmentVariable", "(", ")"},
		{"ENV.fetch", "(", ")"},
	}

	type row struct {
		name    string
		content string
		verdict verdict
	}
	var rows []row
	allowed := func(name, content string) {
		rows = append(rows, row{name: name, content: content, verdict: verdictAllowed})
	}
	denied := func(name, content string) {
		rows = append(rows, row{name: name, content: content, verdict: verdictDenied})
	}

	for _, accessor := range accessors {
		name, open := accessor.name, accessor.open
		allowed("read through "+name, assign+name+open+`"HOME"`+accessor.close+"\n")

		// A quote before the name makes the value a string literal, whatever follows it. In the opened forms the
		// bracket stands directly after the name, so only the quote keeps them denied.
		for _, quote := range []struct{ name, mark string }{{"double", `"`}, {"single", `'`}} {
			denied(name+" wrapped in "+quote.name+" quotes", assign+quote.mark+name+quote.mark+open)
			denied(name+" after an opening "+quote.name+" quote", assign+quote.mark+name+open)
		}

		// os.environ without its bracket is a plain dotted name, which the check has always let through as a code
		// expression; the rows after the loop pin that. The other five hold a colon pair or start with a capital
		// letter, so only the accessor exemption can let them through, and only with their own bracket.
		if name == "os.environ" {
			continue
		}
		denied(name+" followed by a space", assign+name+" ")
		denied(name+" followed by a semicolon", assign+name+";")
		denied(name+" at the end of the content", assign+name)
		denied(name+" followed by a member call", assign+name+".x(")
		denied(name+" followed by the other bracket", assign+name+"[")
	}

	allowed("os.environ with nothing after it", assign+"os.environ")
	allowed("os.environ.copy", assign+"os.environ.copy")
	allowed("os.environ followed by the other bracket", assign+"os.environ(")

	allowed("member whose whole value is a user-config reference", object(reference))
	denied("member with text after the user-config reference", object(reference+"abcd"+"1234"))

	// The exemptions leave the rule itself in place: a literal value under the same name is still refused.
	denied("literal value of twelve characters with a digit", assign+"canon"+"ical"+"7ab"+"\n")

	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := block{path: "corrections_test.go", tool: toolWrite, body: tc.content}
			got := runBlock(t, b, t.TempDir(), outsideRepository)
			category := ""
			if tc.verdict == verdictDenied {
				category = categorySecrets
			}
			if got.verdict != tc.verdict || got.category != category {
				t.Errorf("%s: content %q: got %s %q, want %s %q: exit %d, %s\n%s",
					tc.name, tc.content, got.verdict, got.category, tc.verdict, category, got.exitCode, got.message, got.output)
			}
		})
	}
}
