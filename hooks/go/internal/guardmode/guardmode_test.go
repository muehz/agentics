package guardmode

import "testing"

const (
	testSkipVar = "SKIP_TEST_GUARD"
	testModeVar = "TEST_GUARD_MODE"
)

func warningFor(value string) string {
	return "test-guardian: TEST_GUARD_MODE=" + quote(value) + " is not warn or block; using block"
}

// quote mirrors %q without importing fmt into the expectation table, so a
// change to the warning format cannot hide behind the same helper.
func quote(value string) string {
	switch value {
	case "Warn":
		return `"Warn"`
	case "WARN":
		return `"WARN"`
	case "blcok":
		return `"blcok"`
	case " warn":
		return `" warn"`
	case "warn\n":
		return `"warn\n"`
	case "w\x00arn":
		return `"w\x00arn"`
	}
	panic("no expected quoting for " + value)
}

func TestResolve(t *testing.T) {
	control := Control{Guard: "test-guardian", SkipVar: testSkipVar, ModeVar: testModeVar}
	rows := []struct {
		name        string
		skip, mode  string
		wantSkipped bool
		wantMode    Mode
		wantWarning string
	}{
		{"skip true", "true", "", true, Block, ""},
		{"skip upper", "TRUE", "", false, Block, ""},
		{"skip title", "True", "", false, Block, ""},
		{"skip one", "1", "", false, Block, ""},
		{"skip yes", "yes", "", false, Block, ""},
		{"skip leading space", " true", "", false, Block, ""},
		{"skip trailing newline", "true\n", "", false, Block, ""},
		{"skip empty", "", "", false, Block, ""},

		{"mode warn", "", "warn", false, Warn, ""},
		{"mode block", "", "block", false, Block, ""},
		{"mode empty", "", "", false, Block, ""},
		{"mode Warn", "", "Warn", false, Block, warningFor("Warn")},
		{"mode WARN", "", "WARN", false, Block, warningFor("WARN")},
		{"mode typo", "", "blcok", false, Block, warningFor("blcok")},
		{"mode leading space", "", " warn", false, Block, warningFor(" warn")},
		{"mode trailing newline", "", "warn\n", false, Block, warningFor("warn\n")},
		{"mode with NUL", "", "w\x00arn", false, Block, warningFor("w\x00arn")},

		{"skip wins over warn", "true", "warn", true, Block, ""},
		{"skip wins over invalid mode", "true", "blcok", true, Block, ""},
		{"near skip with invalid mode", "TRUE", "blcok", false, Block, warningFor("blcok")},
		{"near skip with warn", "1", "warn", false, Warn, ""},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			env := map[string]string{testSkipVar: row.skip, testModeVar: row.mode}
			got := control.Resolve(func(name string) string { return env[name] })
			want := Resolution{Skipped: row.wantSkipped, Mode: row.wantMode, Warning: row.wantWarning}
			if got != want {
				t.Fatalf("Resolve(skip=%q, mode=%q) = %+v, want %+v", row.skip, row.mode, got, want)
			}
		})
	}
}

func TestResolveReadsOnlyItsOwnVariables(t *testing.T) {
	control := Control{Guard: "test-guardian", SkipVar: testSkipVar, ModeVar: testModeVar}
	var asked []string
	control.Resolve(func(name string) string {
		asked = append(asked, name)
		return ""
	})
	if len(asked) != 2 || asked[0] != testSkipVar || asked[1] != testModeVar {
		t.Fatalf("Resolve read %v, want the skip variable first and then the mode variable", asked)
	}
}

func TestResolveSkipDoesNotReadMode(t *testing.T) {
	control := Control{Guard: "test-guardian", SkipVar: testSkipVar, ModeVar: testModeVar}
	got := control.Resolve(func(name string) string {
		if name == testModeVar {
			t.Errorf("a skipped resolution read %s", name)
		}
		return "true"
	})
	if !got.Skipped {
		t.Fatalf("Skipped = false, want true")
	}
}

func TestModeValues(t *testing.T) {
	rows := []struct {
		name       string
		mode       Mode
		wantBlocks bool
		wantString string
	}{
		{"block", Block, true, "block"},
		{"warn", Warn, false, "warn"},
		{"zero value", Mode(0), true, "block"},
		{"out of range value", Mode(99), true, "block"},
	}
	for _, row := range rows {
		t.Run(row.name, func(t *testing.T) {
			if got := row.mode.Blocks(); got != row.wantBlocks {
				t.Errorf("Blocks() = %v, want %v", got, row.wantBlocks)
			}
			if got := row.mode.String(); got != row.wantString {
				t.Errorf("String() = %q, want %q", got, row.wantString)
			}
		})
	}
	var zero Resolution
	if zero.Mode != Block || !zero.Mode.Blocks() {
		t.Errorf("zero Resolution has mode %v, want block", zero.Mode)
	}
}
