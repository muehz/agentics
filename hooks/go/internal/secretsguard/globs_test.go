package secretsguard

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

func globFixture(t *testing.T, names ...string) string {
	t.Helper()
	root := t.TempDir()
	// A Windows runner's temp directory has an 8.3 short name such as RUNNER~1. The guard
	// fails closed on a "~" in a cd destination, so the fixture uses the long name.
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	for _, name := range names {
		name = filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte("placeholder\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestGuardGlobWorkingDirectory(t *testing.T) {
	root := globFixture(t, "safe/value.go", "secret/.env")
	for _, tc := range []struct{ command, want, fragment string }{
		{"cd secret && cat .e[n]v", "deny", "(dotenv, critical)"},
		{"cd safe && cat *", "pass", ""},
		{"cd safe && cat * && cd ../secret && cat .e[n]v", "deny", "(dotenv, critical)"},
		{"cd safe; cat *", "deny", "could not inspect glob"},
		{"cd $TARGET && cat *", "deny", "could not inspect glob"},
		{"cat $TARGET/*", "deny", "could not inspect glob"},
		{"(cd secret; cat .e[n]v)", "deny", "could not inspect glob"},
		{"bash -c 'cat *'", "deny", "could not inspect glob"},
		{"shopt -s dotglob; cat *", "deny", "could not inspect glob"},
		{"set -f; cat *", "deny", "could not inspect glob"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": tc.command}, root), nil)
			if kind != tc.want || !strings.Contains(message, tc.fragment) {
				t.Fatalf("kind=%s message=%s want=%s %s; parse=%#v", kind, message, tc.want, tc.fragment, shellwords.Parse(tc.command, shellwords.POSIX))
			}
		})
	}
}

func TestGuardGlobConditionalDirectory(t *testing.T) {
	root := globFixture(t, "secret/.env", "safe/value.go", "external/safe/.env")
	secret := filepath.ToSlash(filepath.Join(root, "secret"))
	safe := filepath.ToSlash(filepath.Join(root, "safe"))
	external := filepath.ToSlash(filepath.Join(root, "external"))
	for _, tc := range []struct {
		name, tool, command string
		env                 map[string]string
	}{
		{"or and chain", "Bash", "cd '" + secret + "' || cd '" + safe + "' && cat .e[n]v", nil},
		{"location alias", "PowerShell", "sl secret; Get-Content .e[n]v", nil},
		{"push location alias", "PowerShell", "push-location secret; Get-Content .e[n]v", nil},
		{"inline cdpath", "Bash", "CDPATH='" + external + "' cd safe && cat .e[n]v", nil},
		{"host cdpath", "Bash", "cd safe && cat .e[n]v", map[string]string{"CDPATH": external}},
		{"bare cdpath", "Bash", "CDPATH='" + external + "'; cd safe && cat .e[n]v", nil},
		{"exported cdpath", "Bash", "export CDPATH='" + external + "'; cd safe && cat .e[n]v", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind, message := run(t, buildEventAt(t, tc.tool, map[string]any{"command": tc.command}, root), tc.env)
			if kind != "deny" || !strings.Contains(message, "could not inspect glob") {
				t.Fatalf("kind=%s message=%s; want distinct uncertain-location denial", kind, message)
			}
		})
	}
}

func TestGuardGlobANSIQuote(t *testing.T) {
	root := globFixture(t, ".env", "safe/value.go")
	for _, tc := range []struct{ command, want, fragment string }{
		{`cat $'.e'[n]v`, "deny", "(dotenv, critical)"},
		{`cat $'\x2ee'[n]v`, "deny", "(dotenv, critical)"},
		{`cat $'safe/'*`, "pass", ""},
		{`cat $'.e[n]'*`, "pass", ""},
	} {
		t.Run(tc.command, func(t *testing.T) {
			kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": tc.command}, root), nil)
			if kind != tc.want || !strings.Contains(message, tc.fragment) {
				t.Fatalf("kind=%s message=%s", kind, message)
			}
		})
	}
}

func TestGuardGlobCDPATHExplicitDestination(t *testing.T) {
	root := globFixture(t, "safe/value.go", "external/safe/.env")
	for _, destination := range []string{"./safe", filepath.ToSlash(filepath.Join(root, "safe"))} {
		command := "cd '" + destination + "' && cat *"
		kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": command}, root), map[string]string{"CDPATH": filepath.Join(root, "external")})
		if kind != "pass" {
			t.Fatalf("explicit destination kind=%s message=%s", kind, message)
		}
	}
}

func TestGuardGlobSafeMatches(t *testing.T) {
	root := globFixture(t, "scratch/go.mod", "scratch/value.go", "scratch/value_test.go", "scratch/zealot-tests/go.mod", "scratch/zealot-tests/value.go", "scratch/zealot-tests/value_test.go", ".claude-plugin/plugin.json", ".claude-plugin/marketplace.json", ".github/plugin/plugin.json", ".github/plugin/marketplace.json")
	for _, command := range []string{"cat scratch/*", "cp scratch/zealot-tests/* destination/", "grep name .github/plugin/*", "cat .claude-plugin/*.json", "cat .github/plugin/*.json"} {
		t.Run(command, func(t *testing.T) {
			kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": command}, root), nil)
			if kind != "pass" {
				t.Fatalf("kind=%s message=%s; want safe metadata matches to pass", kind, message)
			}
		})
	}
}

func TestGuardGlobMatching(t *testing.T) {
	root := globFixture(t, ".env", "safe/value.go", "safe/.env.example", "secret/id_rsa", ".claude-plugin/application_default_credentials.json", "Case/ID_RSA", "custom")
	if err := os.Mkdir(filepath.Join(root, "safe", ".env"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ tool, command, want, fragment string }{
		{"Bash", "cat .env*", "deny", "(dotenv, critical)"},
		{"Bash", "cat .*", "deny", "(dotenv, critical)"},
		{"Bash", "cat .e[n]v", "deny", "(dotenv, critical)"},
		{"Bash", "cat safe/* secret/*", "deny", "(ssh_private_key, critical)"},
		{"Bash", "cp secret/* destination/", "deny", "(ssh_private_key, critical)"},
		{"Bash", "grep name .claude-plugin/*.json", "deny", "(cloud_credentials, critical)"},
		{"Bash", "cat safe/.*", "pass", ""},
		{"Bash", "cat Case/id_*", "pass", ""},
		{"PowerShell", "Get-Content Case/id_*", "deny", "(ssh_private_key, critical)"},
		{"PowerShell", "Get-Content '.e[n]v'", "deny", "(dotenv, critical)"},
		{"PowerShell", "Get-Content -LiteralPath '.e[n]v'", "pass", ""},
		{"Bash", `cat '.e[n]v'`, "pass", ""},
		{"Bash", `cat .e\[n\]v`, "pass", ""},
		{"Bash", `cat '.e'[n]v`, "deny", "(dotenv, critical)"},
		{"Bash", `cat .e"[n]"*`, "pass", ""},
		{"Bash", "cat absent/*.json", "pass", ""},
		{"Bash", "cat safe/no-match*", "pass", ""},
		{"Bash", "cat .[!a]nv", "deny", "(dotenv, critical)"},
		{"Bash", "PGPASSFILE=custom cat cust*", "deny", "(credential_path_override, critical)"},
		{"Bash", "git show HEAD:.e[n]v", "deny", "(dotenv, critical)"},
		{"Bash", "git show HEAD:keys/id_ed25519_s[k]", "deny", "(ssh_private_key, critical)"},
		{"Bash", "git show HEAD:id_ecdsa_s[k]", "deny", "(ssh_private_key, critical)"},
		{"Bash", "git show HEAD:keys/id_ed25519_s[j]", "pass", ""},
		{"Bash", "git show HEAD:keys/id_ed25519_sk.pu[b]", "pass", ""},
	} {
		t.Run(tc.command, func(t *testing.T) {
			kind, message := run(t, buildEventAt(t, tc.tool, map[string]any{"command": tc.command}, root), nil)
			if kind != tc.want || !strings.Contains(message, tc.fragment) {
				t.Fatalf("kind=%s message=%s want=%s %s", kind, message, tc.want, tc.fragment)
			}
		})
	}
}

func TestGuardGlobInspectionLimits(t *testing.T) {
	root := globFixture(t, "safe/value.go")
	for _, mode := range []string{"block", "warn"} {
		for _, command := range []string{"cat safe/[", "cat safe/**", "cat " + strings.Repeat("part/", 33) + "*", "shopt -s dotglob; cat safe/*"} {
			t.Run(mode+command, func(t *testing.T) {
				kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": command}, root), map[string]string{"SECRETS_GUARD_MODE": mode})
				if kind != "deny" || !strings.Contains(message, "could not inspect glob") {
					t.Fatalf("kind=%s message=%s", kind, message)
				}
			})
		}
	}
	kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": "cat *"}, ""), nil)
	if kind != "deny" || !strings.Contains(message, "could not inspect glob") {
		t.Fatalf("missing cwd: kind=%s message=%s", kind, message)
	}
}

func TestGuardGlobSymlinks(t *testing.T) {
	root := globFixture(t, "secret/.env", "secret/deep/value.go", "safe/value.go", "store/.aws/config")
	for _, link := range []struct{ name, target string }{{"safe/alias.txt", "secret/.env"}, {"linked", "secret"}, {"linkedstore", "store/.aws"}, {"linkeddeep", "secret/deep"}, {"safe/broken.txt", "missing"}, {"brokenparent", "missing"}} {
		if err := os.Symlink(filepath.Join(root, link.target), filepath.Join(root, link.name)); err != nil {
			t.Fatalf("symlink fixture unavailable: %v", err)
		}
	}
	for _, tc := range []struct{ command, fragment string }{
		{"cat safe/alias*", "(dotenv, critical)"}, {"cat linked/.*", "(dotenv, critical)"}, {"cat safe/broken*", "could not inspect glob"},
		{"cat linkedstore/*", "(cloud_credentials, critical)"}, {"cat brokenparent/*", "could not inspect glob"},
		{"cat linkeddeep/../.e[n]v", "(dotenv, critical)"},
	} {
		t.Run(tc.command, func(t *testing.T) {
			kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": tc.command}, root), nil)
			if kind != "deny" || !strings.Contains(message, tc.fragment) {
				t.Fatalf("kind=%s message=%s", kind, message)
			}
		})
	}
}

func TestGuardGlobSharedBudgets(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 129; i++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("d%03d", i)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	var commands []string
	for i := 0; i < 129; i++ {
		commands = append(commands, fmt.Sprintf("cat d%03d/*", i))
	}
	kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": strings.Join(commands, "; ")}, root), nil)
	if kind != "deny" || !strings.Contains(message, "could not inspect glob") || !strings.Contains(message, "directory open limit") {
		t.Fatalf("kind=%s message=%s", kind, message)
	}
	for i := 0; i < 4097; i++ {
		if err := os.WriteFile(filepath.Join(root, "d000", fmt.Sprintf("f%04d", i)), nil, 0600); err != nil {
			t.Fatal(err)
		}
	}
	kind, message = run(t, buildEventAt(t, "Bash", map[string]any{"command": "cat d000/*"}, root), nil)
	if kind != "deny" || !strings.Contains(message, "could not inspect glob") || !strings.Contains(message, "entry limit") {
		t.Fatalf("kind=%s message=%s", kind, message)
	}
	if err := os.Remove(filepath.Join(root, "d000", "f4096")); err != nil {
		t.Fatal(err)
	}
	kind, message = run(t, buildEventAt(t, "Bash", map[string]any{"command": "cat d000/*"}, root), nil)
	if kind != "pass" {
		t.Fatalf("exact entry/match budget: kind=%s message=%s", kind, message)
	}
	if err := os.WriteFile(filepath.Join(root, "d001", "extra"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	kind, message = run(t, buildEventAt(t, "Bash", map[string]any{"command": "cat d000/* d001/*"}, root), nil)
	if kind != "deny" || !strings.Contains(message, "entry limit") {
		t.Fatalf("shared entry budget: kind=%s message=%s", kind, message)
	}
}

func TestGuardGlobGitBashAbsolute(t *testing.T) {
	if runtime.GOOS != "windows" {
		if got := nativeShellPath("/c/workspace/*"); got != "/c/workspace/*" {
			t.Fatalf("non-Windows native path changed to %q", got)
		}
		return
	}
	root := globFixture(t, ".env")
	p := filepath.ToSlash(root)
	p = "/" + strings.ToLower(p[:1]) + p[2:]
	kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": "cat '" + p + "/'.e[n]v"}, ""), nil)
	if kind != "deny" || !strings.Contains(message, "(dotenv, critical)") {
		t.Fatalf("kind=%s message=%s", kind, message)
	}
}

func TestGuardGlobMetadataAndModes(t *testing.T) {
	root := globFixture(t, ".env", "safe/value.go", "store/.aws/config")
	if err := os.Mkdir(filepath.Join(root, "safe", ".env"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "cycle"), filepath.Join(root, "cycle")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "safe", "value.go"), filepath.Join(root, "safe", ".env.link")); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ command, mode, want, fragment string }{
		{"cat .e[n]v", "warn", "context", "(dotenv, critical)"},
		{"cat safe/.*", "block", "deny", "(dotenv, critical)"},
		{"cat store/.*", "block", "deny", "(cloud_credentials, critical)"},
		{"cat cycle/*", "warn", "deny", "could not inspect glob"},
		{"cat cycle/*", "block", "deny", "could not inspect glob"},
		{"cat *", "block", "deny", "could not inspect glob"},
		{"cat absent/*; printf '%s' '" + fakeGitHubPAT + "'", "block", "deny", "potential secret(s) found before writing"},
		{"cat ~/does-not-exist/*", "block", "deny", "could not inspect glob"},
	} {
		t.Run(tc.command+tc.mode, func(t *testing.T) {
			kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": tc.command}, root), map[string]string{"SECRETS_GUARD_MODE": tc.mode})
			if kind != tc.want || !strings.Contains(message, tc.fragment) {
				t.Fatalf("kind=%s message=%s want=%s %s", kind, message, tc.want, tc.fragment)
			}
		})
	}
}

func TestGuardGlobAbsoluteUnaffectedByUnknownCwd(t *testing.T) {
	root := globFixture(t, "safe/value.go", ".env")
	for _, prefix := range []string{"cd $UNKNOWN && ", "(echo harmless); "} {
		command := prefix + "cat '" + filepath.ToSlash(root) + "/safe/'*"
		kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": command}, ""), nil)
		if kind != "pass" {
			t.Fatalf("kind=%s message=%s", kind, message)
		}
	}
}

func TestGuardGlobAllowlistAndCommandScan(t *testing.T) {
	root := globFixture(t, "ignored/.env", "secret/.env")
	for _, tc := range []struct{ command, want, fragment string }{
		{"cat ignored/.*", "pass", ""},
		{"cat ignored/.* secret/.*", "deny", "(dotenv, critical)"},
		{"cat ignored/.*; printf '%s' '" + fakeGitHubPAT + "'", "deny", "potential secret(s) found before writing"},
	} {
		kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": tc.command}, root), map[string]string{"SECRETS_GUARD_ALLOWLIST": "ignored/"})
		if kind != tc.want || !strings.Contains(message, tc.fragment) {
			t.Fatalf("kind=%s message=%s", kind, message)
		}
	}
}

func TestGuardGlobMatchesPreserveSpelling(t *testing.T) {
	root := globFixture(t, "MixedCase.TXT")
	b := globBudget{}
	matches, err := b.resolveShellGlob(shellPath{Text: "mixed*.txt", Cwd: root, Dialect: shellwords.PowerShell, ExpandPattern: true})
	if err != nil || len(matches) != 1 || filepath.Base(matches[0]) != "MixedCase.TXT" {
		t.Fatalf("matches=%v err=%v", matches, err)
	}
	b = globBudget{matches: globMatchLimit}
	_, err = b.resolveShellGlob(shellPath{Text: "*", Cwd: root, ExpandPattern: true})
	if err == nil || err.Error() != "match limit exceeded" {
		t.Fatalf("match boundary error=%v", err)
	}
}

func TestGuardGlobNativeAbsolute(t *testing.T) {
	root := globFixture(t, ".env")
	for _, command := range []string{"cat '" + root + string(filepath.Separator) + "'.e[n]v", "Get-Content '" + root + string(filepath.Separator) + ".e[n]v'"} {
		tool := "Bash"
		if strings.HasPrefix(command, "Get-Content") {
			tool = "PowerShell"
		}
		kind, message := run(t, buildEventAt(t, tool, map[string]any{"command": command}, ""), nil)
		if kind != "deny" || !strings.Contains(message, "(dotenv, critical)") {
			t.Fatalf("command=%s kind=%s message=%s", command, kind, message)
		}
	}
}

func TestGuardGlobUnsupportedLocation(t *testing.T) {
	root := globFixture(t, "safe/value.go")
	for _, command := range []string{"Get-Content Env:*", "Get-Content C:*.json", "Get-Content ~/x*", "Get-Content '~/.e[n]v'"} {
		kind, message := run(t, buildEventAt(t, "PowerShell", map[string]any{"command": command}, root), nil)
		if kind != "deny" || !strings.Contains(message, "could not inspect glob") {
			t.Fatalf("command=%s kind=%s message=%s", command, kind, message)
		}
	}
}

func TestGuardGlobActualCredential(t *testing.T) {
	root := globFixture(t, ".env")
	command := "cat '" + filepath.ToSlash(root) + "/'.e[n]v"
	kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": command}, root), nil)
	if kind != "deny" || !strings.Contains(message, "credential file read(s) blocked") || !strings.Contains(message, "(dotenv, critical)") {
		t.Fatalf("kind=%s message=%s; want distinctive dotenv read deny", kind, message)
	}
}

func TestGuardGlobToolCDPATH(t *testing.T) {
	root := globFixture(t, "safe/value.go", "external/safe/.env")
	external := filepath.ToSlash(filepath.Join(root, "external"))
	unknown := "relative wildcard has no known absolute working directory"
	for _, layer := range []string{"direct", "value"} {
		for _, field := range []string{"env", "environment"} {
			for _, tc := range []struct {
				name  string
				value any
				deny  bool
			}{
				{"nonempty", map[string]any{"CDPATH": external}, true},
				{"nonstring", map[string]any{"CDPATH": json.Number("7")}, true},
				{"null value", map[string]any{"CDPATH": nil}, true},
				{"null container", nil, true},
				{"string container", "CDPATH=" + external, true},
				{"list container", []any{"CDPATH=" + external}, true},
				{"empty", map[string]any{"CDPATH": ""}, false},
				{"unrelated", map[string]any{"PUBLIC": "example"}, false},
			} {
				t.Run(layer+"/"+field+"/"+tc.name, func(t *testing.T) {
					input := map[string]any{"command": "cd safe && cat .e[n]v"}
					args := input
					if layer == "value" {
						args = map[string]any{}
						input["value"] = args
					}
					args[field] = tc.value
					kind, message := run(t, buildEventAt(t, "Bash", input, root), map[string]string{"CDPATH": ""})
					want := "pass"
					if tc.deny {
						want = "deny"
					}
					if kind != want || tc.deny && (!strings.Contains(message, "could not inspect glob") || !strings.Contains(message, unknown)) {
						t.Fatalf("kind=%s message=%s; want %s %s", kind, message, want, unknown)
					}
				})
			}
		}
	}
	// A container or name in another letter case is uncertain by itself, so
	// even an empty one withholds the proof that CDPATH is unset.
	for _, layer := range []string{"direct", "value"} {
		for _, field := range []string{"Env", "ENV", "Environment", "ENVIRONMENT"} {
			for _, tc := range []struct {
				name  string
				value any
			}{
				{"empty", map[string]any{}},
				{"empty cdpath", map[string]any{"CDPATH": ""}},
				{"unrelated", map[string]any{"PUBLIC": "example"}},
			} {
				t.Run("variant container/"+layer+"/"+field+"/"+tc.name, func(t *testing.T) {
					input := map[string]any{"command": "cd safe && cat .e[n]v"}
					args := input
					if layer == "value" {
						args = map[string]any{}
						input["value"] = args
					}
					args[field] = tc.value
					kind, message := run(t, buildEventAt(t, "Bash", input, root), map[string]string{"CDPATH": ""})
					if kind != "deny" || !strings.Contains(message, "could not inspect glob") || !strings.Contains(message, unknown) {
						t.Fatalf("kind=%s message=%s; want deny %s", kind, message, unknown)
					}
				})
			}
		}
	}
	for _, name := range []string{"cdpath", "Cdpath"} {
		for _, value := range []string{"", external} {
			t.Run("variant name/"+name+"/"+value, func(t *testing.T) {
				input := map[string]any{"command": "cd safe && cat .e[n]v", "env": map[string]any{name: value}}
				kind, message := run(t, buildEventAt(t, "Bash", input, root), map[string]string{"CDPATH": ""})
				if kind != "deny" || !strings.Contains(message, "could not inspect glob") || !strings.Contains(message, unknown) {
					t.Fatalf("kind=%s message=%s; want deny %s", kind, message, unknown)
				}
			})
		}
	}
	// A variant container is uncertain for every name, so the commands that
	// pass beside a canonical CDPATH entry deny beside a variant container.
	for _, command := range []string{"cat safe/*", "cd ./safe && cat .e[n]v"} {
		t.Run("variant container/"+command, func(t *testing.T) {
			input := map[string]any{"command": command, "ENV": map[string]any{"CDPATH": external}}
			kind, message := run(t, buildEventAt(t, "Bash", input, root), map[string]string{"CDPATH": ""})
			if kind != "deny" || !strings.Contains(message, "could not inspect glob") {
				t.Fatalf("kind=%s message=%s; want deny", kind, message)
			}
		})
	}
	for _, tc := range []struct {
		name, command, host, want string
		direct, nested            map[string]any
	}{
		{"competing aliases", "cd safe && cat .e[n]v", "", "deny", map[string]any{"env": map[string]any{"CDPATH": ""}, "environment": map[string]any{"CDPATH": external}}, nil},
		{"competing layers", "cd safe && cat .e[n]v", "", "deny", map[string]any{"env": map[string]any{"CDPATH": ""}}, map[string]any{"env": map[string]any{"CDPATH": external}}},
		{"reverse competing layers", "cd safe && cat .e[n]v", "", "deny", map[string]any{"env": map[string]any{"CDPATH": external}}, map[string]any{"environment": map[string]any{"CDPATH": ""}}},
		{"tool cannot clear host", "cd safe && cat .e[n]v", external, "deny", map[string]any{"env": map[string]any{"CDPATH": ""}}, nil},
		{"no cd", "cat safe/*", "", "pass", map[string]any{"env": map[string]any{"CDPATH": external}}, nil},
		{"explicit dot", "cd ./safe && cat .e[n]v", "", "pass", map[string]any{"env": map[string]any{"CDPATH": external}}, nil},
		{"parent destination", "cd ./safe && cd ../safe && cat .e[n]v", "", "pass", map[string]any{"env": map[string]any{"CDPATH": external}}, nil},
		{"absolute destination", "cd '" + filepath.ToSlash(filepath.Join(root, "safe")) + "' && cat .e[n]v", "", "pass", map[string]any{"env": map[string]any{"CDPATH": external}}, nil},
		{"absent", "cd safe && cat .e[n]v", "", "pass", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := map[string]any{"command": tc.command}
			for key, value := range tc.direct {
				input[key] = value
			}
			if tc.nested != nil {
				input["value"] = tc.nested
			}
			kind, message := run(t, buildEventAt(t, "Bash", input, root), map[string]string{"CDPATH": tc.host})
			if kind != tc.want || tc.want == "deny" && (!strings.Contains(message, "could not inspect glob") || !strings.Contains(message, unknown)) {
				t.Fatalf("kind=%s message=%s; want %s %s", kind, message, tc.want, unknown)
			}
		})
	}
	t.Run("request isolation", func(t *testing.T) {
		for _, hazard := range []bool{true, false, true, false} {
			input := map[string]any{"command": "cd safe && cat .e[n]v"}
			if hazard {
				input["env"] = map[string]any{"CDPATH": external}
			}
			kind, message := run(t, buildEventAt(t, "Bash", input, root), map[string]string{"CDPATH": ""})
			want := "pass"
			if hazard {
				want = "deny"
			}
			if kind != want || hazard && (!strings.Contains(message, "could not inspect glob") || !strings.Contains(message, unknown)) {
				t.Fatalf("hazard=%t kind=%s message=%s", hazard, kind, message)
			}
			if os.Getenv("CDPATH") != "" {
				t.Fatal("request mutated inherited CDPATH")
			}
		}
	})
}

func TestGuardGlobInheritedOptions(t *testing.T) {
	root := globFixture(t, ".env", "visible.txt")
	for _, tc := range []struct{ name, command, tool, key, value, want string }{
		{"dotglob", "cat *", "Bash", "BASHOPTS", "checkwinsize:dotglob:extquote", "deny"},
		{"nocaseglob", "cat .ENV*", "Bash", "BASHOPTS", "nocaseglob", "deny"},
		{"globignore", "cat *", "Bash", "GLOBIGNORE", "ignored.txt", "deny"},
		{"globstar", "cat **/*", "Bash", "BASHOPTS", "globstar", "deny"},
		{"extglob", "cat *", "Bash", "BASHOPTS", "extglob", "deny"},
		{"default", "cat *", "Bash", "BASHOPTS", "checkwinsize:cmdhist:complete_fullquote:extquote:force_fignore:globasciiranges:globskipdots:hostcomplete:interactive_comments:patsub_replacement:progcomp:promptvars:sourcepath", "pass"},
		{"empty", "cat *", "Bash", "BASHOPTS", "", "pass"},
		{"quoted", "cat '*'", "Bash", "BASHOPTS", "dotglob", "pass"},
		{"literal safe", "cat visible.txt", "Bash", "BASHOPTS", "dotglob", "pass"},
		{"literal credential", "cat '.env'", "Bash", "BASHOPTS", "dotglob", "deny"},
		{"powershell", "Get-Content visible*", "PowerShell", "BASHOPTS", "dotglob", "pass"},
		{"redirect", "cat < *", "Bash", "BASHOPTS", "dotglob", "deny"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kind, message := run(t, buildEventAt(t, tc.tool, map[string]any{"command": tc.command}, root), map[string]string{"BASHOPTS": "", "GLOBIGNORE": "", tc.key: tc.value})
			if kind != tc.want || tc.want == "deny" && tc.name != "literal credential" && !strings.Contains(message, "could not inspect glob") {
				t.Fatalf("kind=%s message=%s want=%s", kind, message, tc.want)
			}
		})
	}
}

func TestGuardGlobToolOptions(t *testing.T) {
	root := globFixture(t, ".env", "visible.txt")
	for _, layer := range []string{"direct", "value"} {
		for _, field := range []string{"env", "environment"} {
			for _, tc := range []struct {
				name  string
				value any
				deny  bool
			}{
				{"dotglob", map[string]any{"BASHOPTS": "dotglob"}, true},
				{"nocaseglob", map[string]any{"BASHOPTS": "nocaseglob"}, true},
				{"globignore", map[string]any{"GLOBIGNORE": "ignored"}, true},
				{"malformed options", map[string]any{"BASHOPTS": "extquote::checkwinsize"}, true},
				{"invalid option characters", map[string]any{"BASHOPTS": "extquote checkwinsize"}, true},
				{"nonstring options", map[string]any{"BASHOPTS": json.Number("7")}, true},
				{"null globignore", map[string]any{"GLOBIGNORE": nil}, true},
				{"null container", nil, true},
				{"string container", "BASHOPTS=dotglob", true},
				{"array container", []any{"BASHOPTS=dotglob"}, true},
				{"default", map[string]any{"BASHOPTS": "extquote:checkwinsize"}, false},
				{"unrelated", map[string]any{"PUBLIC": "example"}, false},
				{"empty", map[string]any{"BASHOPTS": "", "GLOBIGNORE": ""}, false},
			} {
				t.Run(layer+"/"+field+"/"+tc.name, func(t *testing.T) {
					for _, command := range []string{"cat *", "cat visible.txt", "cat '*'"} {
						input := map[string]any{"command": command}
						args := input
						if layer == "value" {
							args = map[string]any{}
							input["value"] = args
						}
						args[field] = tc.value
						kind, message := run(t, buildEventAt(t, "Bash", input, root), map[string]string{"BASHOPTS": "", "GLOBIGNORE": ""})
						want := "pass"
						if tc.deny && command == "cat *" {
							want = "deny"
						}
						if kind != want || want == "deny" && !strings.Contains(message, "could not inspect glob") {
							t.Fatalf("%s kind=%s message=%s want=%s", command, kind, message, want)
						}
					}
				})
			}
		}
	}
	// A container or name in another letter case is uncertain by itself, so
	// only a command that expands a glob is denied.
	for _, layer := range []string{"direct", "value"} {
		for _, field := range []string{"Env", "ENV", "Environment", "ENVIRONMENT"} {
			for _, tc := range []struct {
				name  string
				value any
			}{
				{"empty", map[string]any{}},
				{"empty options", map[string]any{"BASHOPTS": "", "GLOBIGNORE": ""}},
				{"unrelated", map[string]any{"PUBLIC": "example"}},
			} {
				t.Run("variant container/"+layer+"/"+field+"/"+tc.name, func(t *testing.T) {
					for _, command := range []string{"cat *", "cat visible.txt", "cat '*'"} {
						input := map[string]any{"command": command}
						args := input
						if layer == "value" {
							args = map[string]any{}
							input["value"] = args
						}
						args[field] = tc.value
						kind, message := run(t, buildEventAt(t, "Bash", input, root), map[string]string{"BASHOPTS": "", "GLOBIGNORE": ""})
						want := "pass"
						if command == "cat *" {
							want = "deny"
						}
						if kind != want || want == "deny" && !strings.Contains(message, "could not inspect glob") {
							t.Fatalf("%s kind=%s message=%s want=%s", command, kind, message, want)
						}
					}
				})
			}
		}
	}
	for _, name := range []string{"BASHOPTS", "GLOBIGNORE"} {
		for _, variant := range []string{strings.ToLower(name), strings.ToUpper(name[:1]) + strings.ToLower(name[1:])} {
			for _, value := range []string{"", "x"} {
				t.Run("variant name/"+variant+"/"+value, func(t *testing.T) {
					for _, command := range []string{"cat *", "cat visible.txt", "cat '*'"} {
						input := map[string]any{"command": command, "env": map[string]any{variant: value}}
						kind, message := run(t, buildEventAt(t, "Bash", input, root), map[string]string{"BASHOPTS": "", "GLOBIGNORE": ""})
						want := "pass"
						if command == "cat *" {
							want = "deny"
						}
						if kind != want || want == "deny" && !strings.Contains(message, "could not inspect glob") {
							t.Fatalf("%s kind=%s message=%s want=%s", command, kind, message, want)
						}
					}
				})
			}
		}
	}
	for _, input := range []map[string]any{
		{"command": "cat *", "env": map[string]any{"BASHOPTS": ""}, "environment": map[string]any{"BASHOPTS": "nocaseglob"}},
		{"command": "cat *", "env": map[string]any{"BASHOPTS": ""}, "value": map[string]any{"env": map[string]any{"GLOBIGNORE": "ignored"}}},
	} {
		kind, message := run(t, buildEventAt(t, "Bash", input, root), map[string]string{"BASHOPTS": "", "GLOBIGNORE": ""})
		if kind != "deny" || !strings.Contains(message, "could not inspect glob") {
			t.Fatalf("kind=%s message=%s", kind, message)
		}
	}
	for _, hazard := range []bool{true, false, true, false} {
		input := map[string]any{"command": "cat *"}
		if hazard {
			input["env"] = map[string]any{"BASHOPTS": "dotglob"}
		}
		kind, _ := run(t, buildEventAt(t, "Bash", input, root), map[string]string{"BASHOPTS": "", "GLOBIGNORE": ""})
		if (kind == "deny") != hazard || os.Getenv("BASHOPTS") != "" {
			t.Fatal("glob provenance leaked across requests")
		}
	}
	kind, message := run(t, buildEventAt(t, "Bash", map[string]any{"command": "cat *", "env": map[string]any{"BASHOPTS": ""}}, root), map[string]string{"BASHOPTS": "dotglob", "GLOBIGNORE": ""})
	if kind != "deny" || !strings.Contains(message, "could not inspect glob") {
		t.Fatalf("tool cleared host uncertainty: kind=%s message=%s", kind, message)
	}
}
