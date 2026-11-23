package pathshape

import (
	"strings"
	"testing"
	"time"
)

// rules applies each rule to one input on one host, so a single table can hold every rule's expectations.
var rules = map[string]func(Host, string) any{
	"Windows":                func(h Host, _ string) any { return h.Windows() },
	"FoldsCase":              func(h Host, _ string) any { return h.FoldsCase() },
	"Native":                 func(h Host, p string) any { return h.Native(p) },
	"ChangedByCleaning":      func(h Host, p string) any { return h.ChangedByCleaning(p) },
	"MayNameAnotherFile":     func(h Host, p string) any { return h.MayNameAnotherFile(p) },
	"DrivePathOffHost":       func(h Host, p string) any { return h.DrivePathOffHost(p) },
	"ParentAfterNameOffHost": func(h Host, p string) any { return h.ParentAfterNameOffHost(p) },
	"LocalFileProblem":       func(_ Host, p string) any { return LocalFileProblem(p) },
	"HasDrive":               func(_ Host, p string) any { return HasDrive(p) },
	"ReservedDeviceName":     func(_ Host, p string) any { return ReservedDeviceName(p) },
}

// pathRow is one input to one rule, with the answer expected on Linux, macOS, and Windows in that order.
type pathRow struct {
	rule string
	in   string
	want [3]any
}

func all(v any) [3]any                        { return [3]any{v, v, v} }
func hosts(linux, darwin, windows any) [3]any { return [3]any{linux, darwin, windows} }

// pathRows returns the table TestPathRules runs.
func pathRows() []pathRow {
	rows := []pathRow{
		{"Windows", "", hosts(false, false, true)},
		{"FoldsCase", "", hosts(false, true, true)},

		{"Native", "/c/Users/x", hosts("/c/Users/x", "/c/Users/x", "c:/Users/x")},
		{"Native", "/C/Users/x", hosts("/C/Users/x", "/C/Users/x", "C:/Users/x")},
		{"Native", "C:/Users/x", all("C:/Users/x")},
		{"Native", "/c/", hosts("/c/", "/c/", "c:/")},
		{"Native", "/c", all("/c")},
		{"Native", "/cd/x", all("/cd/x")},
		{"Native", "/1/x", all("/1/x")},
		{"Native", "c/x/y", all("c/x/y")},
		{"Native", "", all("")},

		// ChangedByCleaning: a leading backslash or double slash on every host.
		{"ChangedByCleaning", `\srv\share\a.sh`, all(true)},
		{"ChangedByCleaning", `//srv/share/a.sh`, all(true)},
		{"ChangedByCleaning", `/\srv/share/a.sh`, all(true)},
		{"ChangedByCleaning", `/ws/a.sh`, all(false)},
		{"ChangedByCleaning", `C:/ws/a.sh`, all(false)},
		// A backslash anywhere, off Windows.
		{"ChangedByCleaning", `dir\a.sh`, hosts(true, true, false)},
		{"ChangedByCleaning", `dir/a.sh`, all(false)},
		// A .. after a named component, off Windows.
		{"ChangedByCleaning", `a/../b.sh`, hosts(true, true, false)},
		{"ChangedByCleaning", `../b.sh`, all(false)},
		{"ChangedByCleaning", ``, all(false)},

		// MayNameAnotherFile: everything ChangedByCleaning reports.
		{"MayNameAnotherFile", `\srv\share\a.sh`, all(true)},
		{"MayNameAnotherFile", `//srv/share/a.sh`, all(true)},
		{"MayNameAnotherFile", `dir\a.sh`, hosts(true, true, false)},
		{"MayNameAnotherFile", `a/../b.sh`, hosts(true, true, false)},
		{"MayNameAnotherFile", `/ws/a.sh`, all(false)},
		// A colon past the drive, on Windows.
		{"MayNameAnotherFile", `C:/a.sh:stream`, hosts(false, false, true)},
		{"MayNameAnotherFile", `a.sh::$DATA`, hosts(false, false, true)},
		{"MayNameAnotherFile", `C:/a.sh`, all(false)},
		{"MayNameAnotherFile", `C:`, all(false)},
		// A component ending in a dot or a space, on Windows.
		{"MayNameAnotherFile", `dir/a.sh.`, hosts(false, false, true)},
		{"MayNameAnotherFile", `dir\a.sh.`, hosts(true, true, true)},
		{"MayNameAnotherFile", `dir /a.sh`, hosts(false, false, true)},
		{"MayNameAnotherFile", `./a.sh`, all(false)},
		{"MayNameAnotherFile", `../a.sh`, all(false)},
		{"MayNameAnotherFile", `dir/.hidden`, all(false)},
		// A ~ followed by a digit (an 8.3 short name), on Windows.
		{"MayNameAnotherFile", `PROGRA~1/a.sh`, hosts(false, false, true)},
		{"MayNameAnotherFile", `a~b.sh`, all(false)},
		{"MayNameAnotherFile", `a~`, all(false)},
		{"MayNameAnotherFile", ``, all(false)},

		{"DrivePathOffHost", `C:/Users/x/a.sh`, hosts(true, true, false)},
		{"DrivePathOffHost", `c:a.sh`, hosts(true, true, false)},
		{"DrivePathOffHost", `/c/Users/x/a.sh`, all(false)},
		{"DrivePathOffHost", `1:/a.sh`, all(false)},

		{"HasDrive", `C:`, all(true)},
		{"HasDrive", `z:x`, all(true)},
		{"HasDrive", `C`, all(false)},
		{"HasDrive", ``, all(false)},
		{"HasDrive", `1:`, all(false)},
		{"HasDrive", `@:`, all(false)},
		{"HasDrive", `[:`, all(false)},
		{"HasDrive", "\u00e9:", all(false)},
		{"HasDrive", `:C`, all(false)},

		{"ReservedDeviceName", `COM`, all(false)},
		{"ReservedDeviceName", `COM10`, all(false)},
		{"ReservedDeviceName", `COMA`, all(false)},
		{"ReservedDeviceName", `CONX`, all(false)},
		{"ReservedDeviceName", `xcon`, all(false)},
		{"ReservedDeviceName", ``, all(false)},
		{"ReservedDeviceName", `..`, all(false)},
		{"ReservedDeviceName", `con.sh`, all(true)},
		{"ReservedDeviceName", `con  .tar.sh`, all(true)},
		{"ReservedDeviceName", "con\t", all(false)},
		{"ReservedDeviceName", "con\u00a0", all(false)},
		// Folding follows the regular expression's simple folding: the long s and the Kelvin sign fold to s and k,
		// which no device name holds, and the dotless i folds to nothing.
		{"ReservedDeviceName", "CON\u0131N$", all(false)},
		{"ReservedDeviceName", "\u017fON", all(false)},
		{"ReservedDeviceName", "\u212aON", all(false)},
	}

	// Every TestParentAfterName literal from scriptsnap; Windows resolves .. by spelling, so it never reports.
	for p, want := range map[string]bool{
		"/ws/a/../b.sh": true, "a/../b.sh": true, "./a/../b.sh": true, "a/b/../../b.sh": true, "/ws/..": true,
		"../b.sh": false, "../../b.sh": false, "./../b.sh": false, "/../b.sh": false, "/ws/a/b.sh": false,
		"": false, "/": false, "/ws/..x/b.sh": false, "/ws/x../b.sh": false,
	} {
		rows = append(rows, pathRow{"ParentAfterNameOffHost", p, hosts(want, want, false)})
	}

	// Every TestPathShapeProblem literal from scriptsnap.
	for p, want := range map[string]Problem{
		`\\srv\share\a.sh`:        ProblemNamespace,
		`//srv/share/a.sh`:        ProblemNamespace,
		`\\?\C:\a.sh`:             ProblemNamespace,
		`//?/C:/a.sh`:             ProblemNamespace,
		`\\.\pipe\x`:              ProblemNamespace,
		`//./pipe/x`:              ProblemNamespace,
		`\\localhost\c$\x\a.sh`:   ProblemNamespace,
		`/\srv/share/a.sh`:        ProblemNamespace,
		`C:\a.sh:stream`:          ProblemStream,
		`C:\a.sh::$DATA`:          ProblemStream,
		`/home/u/a.sh:stream`:     ProblemStream,
		`/home/u:x/a.sh`:          ProblemStream,
		`C:`:                      ProblemDriveRelative,
		`C:a.sh`:                  ProblemDriveRelative,
		`CON.sh`:                  ProblemReservedName,
		`nul`:                     ProblemReservedName,
		`COM1.sh`:                 ProblemReservedName,
		`LPT9.bash`:               ProblemReservedName,
		`dir\con.sh`:              ProblemReservedName,
		`dir/Aux/a.sh`:            ProblemReservedName,
		`con.`:                    ProblemReservedName,
		`NUL.tar.sh`:              ProblemReservedName,
		`aux .sh`:                 ProblemReservedName,
		`prn  .tar.sh`:            ProblemReservedName,
		"COM\u00b9.sh":            ProblemReservedName,
		`CONIN$`:                  ProblemReservedName,
		`conout$.sh`:              ProblemReservedName,
		`dir\a.sh.`:               ProblemTrailingDotOrSpace,
		"dir/a.sh ":               ProblemTrailingDotOrSpace,
		`dir /a.sh`:               ProblemTrailingDotOrSpace,
		`.../a.sh`:                ProblemTrailingDotOrSpace,
		`/proc/self/fd/0`:         ProblemProcessOrDevice,
		`/proc/self/environ`:      ProblemProcessOrDevice,
		`/proc`:                   ProblemProcessOrDevice,
		`/PROC/self/environ`:      ProblemProcessOrDevice,
		`\proc\self\environ`:      ProblemProcessOrDevice,
		`/dev/stdin`:              ProblemProcessOrDevice,
		`/dev`:                    ProblemProcessOrDevice,
		`/Dev/null`:               ProblemProcessOrDevice,
		`/dev/./stdin`:            ProblemProcessOrDevice,
		`C:\Users\x\a.sh::$DATA`:  ProblemStream,
		`C:\Users\x\CON\a.sh`:     ProblemReservedName,
		`\??\UNC\srv\share\a.sh`:  ProblemNamespace,
		`\??\GLOBALROOT\a.sh`:     ProblemNamespace,
		`/??/C:/a.sh`:             ProblemNamespace,
		`/??`:                     ProblemNamespace,
		`\\?\GLOBALROOT\a.sh`:     ProblemNamespace,
		`//?/UNC/srv/share/a.sh`:  ProblemNamespace,
		`\\.\C:\a.sh`:             ProblemNamespace,
		`C:/Users/x/a.sh:Zone.Id`: ProblemStream,
		`LPT0.sh`:                 ProblemReservedName,
		`COM0.sh`:                 ProblemReservedName,
		`lpt0/a.sh`:               ProblemReservedName,
	} {
		rows = append(rows, pathRow{"LocalFileProblem", p, all(want)})
	}
	for _, p := range []string{
		`C:\Users\x\a.sh`, `C:/Users/x/a.sh`, `/c/Users/x/a.sh`, `./scripts/a.sh`, `/home/u/a.sh`, `a.sh`, `../a.sh`,
		`C:\`, `C:/`, `C:\a.sh`, `console.sh`, `network.sh`, `/home/u/network.sh`, `COM10.sh`, `com.sh`,
		`conx.sh`, `auxiliary/a.sh`, `nullable.sh`, `/proc-notes/a.sh`, `/devices/a.sh`, `/developer/a.sh`,
		`/home/u/.hidden/a.sh`, `/home/u/..a/b.sh`, `/home/ünï/скрипт.sh`, `/home/u/日本語.sh`, `/home/u/a b.sh`,
		`dir\sub\a.sh`, `/home/u/proc/a.sh`, `/home/u/dev/a.sh`, `/`, ``,
		`C:/proc/a.sh`, `C:/dev/a.sh`,
	} {
		rows = append(rows, pathRow{"LocalFileProblem", p, all(NoProblem)})
	}

	// Each device name in both letter cases, alone, with an extension, and as a directory.
	devices := []string{"CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$"}
	for _, prefix := range []string{"COM", "LPT"} {
		for _, d := range []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9", "\u00b9", "\u00b2", "\u00b3"} {
			devices = append(devices, prefix+d)
		}
	}
	for _, d := range devices {
		for _, name := range []string{strings.ToUpper(d), strings.ToLower(d)} {
			rows = append(rows,
				pathRow{"ReservedDeviceName", name, all(true)},
				pathRow{"ReservedDeviceName", name + ".sh", all(true)},
				pathRow{"ReservedDeviceName", name + " ", all(true)},
				pathRow{"LocalFileProblem", name + ".sh", all(ProblemReservedName)},
				pathRow{"LocalFileProblem", "dir/" + name + "/a.sh", all(ProblemReservedName)},
			)
		}
	}
	return rows
}

// TestPathRules runs every rule on Linux, macOS, and Windows. Any host other than Windows and macOS follows the Linux
// rules, and the zero Host answers as the running one does.
func TestPathRules(t *testing.T) {
	named := [3]Host{For("linux"), For("darwin"), For("windows")}
	for _, row := range pathRows() {
		rule, ok := rules[row.rule]
		if !ok {
			t.Fatalf("no rule %q", row.rule)
		}
		for i, h := range named {
			if got := rule(h, row.in); got != row.want[i] {
				t.Errorf("%s(%q) on %s: got %#v, want %#v", row.rule, row.in, h.goos, got, row.want[i])
			}
		}
		if got := rule(For("freebsd"), row.in); got != row.want[0] {
			t.Errorf("%s(%q) on freebsd: got %#v, want the Linux answer %#v", row.rule, row.in, got, row.want[0])
		}
		current := rule(Current(), row.in)
		for _, h := range []Host{{}, For("")} {
			if got := rule(h, row.in); got != current {
				t.Errorf("%s(%q) on the zero host: got %#v, want Current()'s %#v", row.rule, row.in, got, current)
			}
		}
	}
}

// TestPathRulesAreLinear keeps each rule linear in its input, so a hostile path cannot spend a check's deadline.
func TestPathRulesAreLinear(t *testing.T) {
	inputs := map[string]string{
		"path of 50,000 components": strings.Repeat("a/", 50_000),
		"100 KB element":            strings.Repeat("a", 100_000),
	}
	for name, in := range inputs {
		for rule, fn := range rules {
			for _, h := range []Host{For("linux"), For("darwin"), For("windows")} {
				start := time.Now()
				fn(h, in)
				if elapsed := time.Since(start); elapsed > time.Second {
					t.Errorf("%s on a %s on %s took %v", rule, name, h.goos, elapsed)
				}
			}
		}
	}
}

// TestProblemTexts pins the detail texts byte for byte, since the reader renders them into its refusals.
func TestProblemTexts(t *testing.T) {
	for p, want := range map[Problem]string{
		NoProblem:                 "",
		ProblemNamespace:          "network or device namespace path",
		ProblemDriveRelative:      "drive-relative path",
		ProblemStream:             "alternate data stream",
		ProblemProcessOrDevice:    "process or device object",
		ProblemReservedName:       "reserved device name",
		ProblemTrailingDotOrSpace: "trailing dot or space",
	} {
		if got := string(p); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
