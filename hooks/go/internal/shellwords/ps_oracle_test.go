package shellwords_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/scriptsnap"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

const oracleMarker = "agentics-file-oracle-marker"

type fileOracle struct {
	path, name, dir string
}

func (o fileOracle) run(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, o.path, args...)
	cmd.Dir = o.dir
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	for _, entry := range cmd.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		key = strings.ToUpper(key)
		switch key {
		case "BASH_ENV", "ENV", "SHELLOPTS", "BASHOPTS", "CDPATH":
			continue
		}
		if !strings.HasPrefix(key, "BASH_FUNC_") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("%s probe timed out: %v", o.name, ctx.Err())
	}
	return strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()), err
}

func (o fileOracle) write(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(o.dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func (o fileOracle) inspect(command string, dialect shellwords.Dialect) shellwords.Result {
	return shellwords.ParseScripts(context.Background(), command, dialect, shellwords.ScriptOptions{
		Reader: scriptsnap.New(), Cwd: o.dir,
		Context: shellwords.ExecutionContext{Dialect: dialect, Proven: true},
	})
}

func scriptIsFile(recordPath, path string) bool {
	record, recordErr := os.Stat(recordPath)
	target, targetErr := os.Stat(path)
	return recordErr == nil && targetErr == nil && os.SameFile(record, target)
}

func requireOracleScript(
	t *testing.T, parsed shellwords.Result, path, text, executable string, dialect shellwords.Dialect,
) {
	t.Helper()
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(text)))
	recorded, expanded := false, false
	for _, record := range parsed.Scripts {
		if scriptIsFile(record.Path, path) {
			recorded = true
			if record.SHA256 != digest || record.Bytes != len(text) {
				t.Errorf("script record for %q has digest %q and %d bytes, want %q and %d bytes",
					path, record.SHA256, record.Bytes, digest, len(text))
			}
		}
	}
	for _, inv := range parsed.Invocations {
		if !inv.Inspectable || inv.Reason.Refused() {
			t.Errorf("executed inspectable fixture was refused: %v", inv.Reason)
		}
		if inv.Script != nil && scriptIsFile(inv.Script.Path, path) && inv.Dialect == dialect &&
			len(inv.Words) > 0 && strings.EqualFold(shellwords.ExecutableName(inv.Words[0].Text), executable) {
			expanded = true
		}
	}
	if !recorded || !expanded {
		t.Errorf("shell executed %q but parser recorded file=%t and expanded marker invocation=%t", path, recorded, expanded)
	}
}

type oracleFileForm struct {
	name, command, refusal string
	args                   []string
	dialect                shellwords.Dialect
	mayReject              bool
}

func (o fileOracle) check(t *testing.T, form oracleFileForm, path, body, executable string, dialect shellwords.Dialect) {
	t.Helper()
	stdout, stderr, err := o.run(t, form.args...)
	if stdout != oracleMarker || err != nil {
		var exit *exec.ExitError
		if !form.mayReject || !errors.As(err, &exit) || strings.Contains(stdout+stderr, oracleMarker) || stdout+stderr == "" {
			t.Fatalf("%s did not execute the marker exactly once: stdout=%q stderr=%q error=%v", o.name, stdout, stderr, err)
		}
		t.Logf("%s rejected this form: %v, stdout=%q stderr=%q", o.name, err, stdout, stderr)
	}
	parsed := o.inspect(form.command, form.dialect)
	if form.refusal == "" {
		requireOracleScript(t, parsed, path, body, executable, dialect)
		return
	}
	for _, inv := range parsed.Invocations {
		if !inv.Inspectable && inv.Reason.String() == form.refusal {
			return
		}
	}
	t.Errorf("expected this form to fail closed with %q, got %+v", form.refusal, parsed.Invocations)
}

func TestPSOracle(t *testing.T) {
	// A launch before environment isolation would load this file in Bash, including for a version probe.
	startup := fileOracle{dir: t.TempDir()}
	canary := filepath.Join(startup.dir, "startup-ran")
	rc := startup.write(t, "startup.sh", "printf '%s' startup > \"$AGX_PS_ORACLE_STARTUP\"\n")
	t.Setenv("BASH_ENV", filepath.ToSlash(rc))
	t.Setenv("AGX_PS_ORACLE_STARTUP", filepath.ToSlash(canary))
	t.Cleanup(func() {
		if _, err := os.Stat(canary); err == nil {
			t.Error("oracle launched Bash with inherited startup code")
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("could not check Bash startup canary: %v", err)
		}
	})

	pathList := filepath.SplitList(os.Getenv("PATH"))
	if runtime.GOOS == "windows" {
		// Prefer the native Git Bash distribution over the Windows WSL launcher.
		git := filepath.Join(os.Getenv("ProgramFiles"), "Git")
		pathList = append([]string{filepath.Join(git, "bin"), filepath.Join(git, "usr", "bin")}, pathList...)
	}
	for _, name := range []string{"powershell", "pwsh", "bash", "sh", "dash"} {
		t.Run(name, func(t *testing.T) {
			path, err := oracleInterpreterPath(name, pathList)
			if err != nil {
				t.Fatalf("could not inspect native interpreter %s: %v", name, err)
			}
			if path == "" {
				for _, required := range strings.Split(os.Getenv("AGX_REQUIRED_INTERPRETERS"), ",") {
					if strings.TrimSpace(required) == name {
						t.Fatalf("required native interpreter %s is absent from PATH on %s", name, runtime.GOOS)
					}
				}
				t.Skipf("native interpreter %s is absent from PATH on %s", name, runtime.GOOS)
			}
			// Version probes are the first child launches. A present executable that fails its isolated probe
			// fails the subtest, so it cannot be mistaken for an absent interpreter.
			o := fileOracle{name: name, dir: t.TempDir(), path: path}
			if name == "powershell" || name == "pwsh" {
				o.powerShell(t)
				return
			}
			o.posix(t)
		})
	}
}

func oracleInterpreterPath(name string, pathList []string) (string, error) {
	executable := name
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	for _, dir := range pathList {
		if !filepath.IsAbs(dir) {
			continue
		}
		path := filepath.Join(dir, executable)
		if runtime.GOOS == "windows" && name == "bash" {
			// WSL launchers do not run native host paths. Continue looking for a native shell farther down PATH.
			if strings.EqualFold(path, filepath.Join(os.Getenv("SystemRoot"), "System32", "bash.exe")) ||
				strings.EqualFold(path, filepath.Join(os.Getenv("LOCALAPPDATA"), "Microsoft", "WindowsApps", "bash.exe")) {
				continue
			}
		}
		candidate, err := exec.LookPath(path)
		if err == nil {
			return candidate, nil
		}
		if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, os.ErrPermission) && !errors.Is(err, exec.ErrNotFound) && !errors.Is(err, syscall.EISDIR) {
			return "", err
		}
	}
	return "", nil
}

func TestPSOracleDiscoverySkipsNonExecutableEntries(t *testing.T) {
	name := "agentics-oracle-shell"
	exe := name
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	directoryEntry, executableEntry := t.TempDir(), t.TempDir()
	if err := os.Mkdir(filepath.Join(directoryEntry, exe), 0o755); err != nil {
		t.Fatal(err)
	}
	pathList := []string{directoryEntry}
	if runtime.GOOS != "windows" {
		nonExecutableEntry := t.TempDir()
		if err := os.WriteFile(filepath.Join(nonExecutableEntry, exe), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		pathList = append(pathList, nonExecutableEntry)
	}
	pathList = append(pathList, executableEntry)
	want := filepath.Join(executableEntry, exe)
	if err := os.WriteFile(want, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := oracleInterpreterPath(name, pathList); err != nil || got != want {
		t.Fatalf("discovery chose %q (%v), want %q", got, err, want)
	}
}

func (o fileOracle) powerShell(t *testing.T) {
	t.Helper()
	base := []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass"}
	args := func(flags ...string) []string { return append(append([]string(nil), base...), flags...) }
	version, diag, err := o.run(t, args("-Command", "$PSVersionTable.PSVersion.ToString()")...)
	if err != nil || version == "" {
		t.Fatalf("%s version: %q %q %v", o.path, version, diag, err)
	}
	t.Logf("%s %s (%s)", o.name, version, o.path)

	const body = "#!/usr/bin/env python3\nWrite-Output '" + oracleMarker + "'\n"
	path := o.write(t, "probe.ps1", body)
	o.write(t, "probe.txt", body)
	cli := o.name + " -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass "
	forms := []oracleFileForm{
		{name: "File", command: cli + "-File ./probe.ps1", args: args("-File", "./probe.ps1"), dialect: shellwords.POSIX},
		{name: "slash File", command: cli + "/File ./probe.ps1", args: args("/File", "./probe.ps1"), dialect: shellwords.POSIX,
			refusal: "script-form-unsupported-option", mayReject: runtime.GOOS != "windows"},
		{name: "Unicode dash File", command: cli + "\u2013File ./probe.ps1", args: args("\u2013File", "./probe.ps1"), dialect: shellwords.POSIX,
			refusal: "script-form-unsupported-option"},
		{name: "non-ps1 File", command: cli + "-File probe.txt", args: args("-File", "probe.txt"), dialect: shellwords.POSIX,
			refusal: "script-extension", mayReject: runtime.GOOS == "windows"},
		{name: "Command wrapper", command: cli + "-Command '& ./probe.ps1'", args: args("-Command", "& ./probe.ps1"), dialect: shellwords.POSIX},
	}
	for _, command := range []string{"./probe.ps1", "& ./probe.ps1", "& './probe.ps1'", ". ./probe.ps1", "./probe"} {
		forms = append(forms, oracleFileForm{name: command, command: command, args: args("-Command", command), dialect: shellwords.PowerShell})
	}
	if runtime.GOOS == "windows" {
		const command = `.\probe`
		forms = append(forms, oracleFileForm{name: "backslash omitted extension", command: command,
			args: args("-Command", command), dialect: shellwords.PowerShell})
	}
	for _, form := range forms {
		t.Run(form.name, func(t *testing.T) {
			o.check(t, form, path, body, "Write-Output", shellwords.PowerShell)
		})
	}

	for _, expression := range []string{`'./probe.ps1'`, `"./probe.ps1"`} {
		t.Run("inert caller "+expression, func(t *testing.T) {
			o.requireInertLiteral(t, o.inspect(expression, shellwords.PowerShell), "", args("-Command", expression)...)
		})
		t.Run("inert script "+expression, func(t *testing.T) {
			outer := o.write(t, "literal.ps1", expression+"\n")
			o.requireInertLiteral(t, o.inspect(cli+"-File ./literal.ps1", shellwords.POSIX), outer, args("-File", "./literal.ps1")...)
		})
	}
}

func (o fileOracle) requireInertLiteral(t *testing.T, parsed shellwords.Result, outer string, args ...string) {
	t.Helper()
	stdout, stderr, err := o.run(t, args...)
	if err != nil || stdout != "./probe.ps1" || stderr != "" {
		t.Fatalf("expected a literal path without marker execution: stdout=%q stderr=%q error=%v", stdout, stderr, err)
	}
	for _, inv := range parsed.Invocations {
		if !inv.Inspectable || inv.Reason.Refused() {
			t.Errorf("literal expression was refused: %v", inv.Reason)
		}
	}
	if outer == "" {
		if len(parsed.Scripts) != 0 {
			t.Errorf("literal expression caused %d script reads", len(parsed.Scripts))
		}
		return
	}
	if len(parsed.Scripts) != 1 || !scriptIsFile(parsed.Scripts[0].Path, outer) {
		t.Error("expected exactly one script record for the outer script containing the literal expression")
	}
}

func (o fileOracle) posix(t *testing.T) {
	t.Helper()
	version, diag, err := o.run(t, "-c", `printf '%s' "${BASH_VERSION-}"`)
	if err != nil {
		t.Fatalf("%s version probe: %q %q %v", o.path, version, diag, err)
	}
	if version == "" {
		resolved, err := filepath.EvalSymlinks(o.path)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s (%s -> %s) has no shell version variable", o.name, o.path, resolved)
	} else {
		t.Logf("%s %s (%s)", o.name, version, o.path)
	}
	const body = "printf '%s\\n' '" + oracleMarker + "'\n"
	path := o.write(t, "probe.sh", body)
	extensionless := o.write(t, "probe", body)
	redirect := "'" + filepath.ToSlash(o.path) + "' < ./probe.sh"
	forms := []oracleFileForm{
		{name: "script operand", command: o.name + " ./probe.sh", args: []string{"./probe.sh"}},
		{name: "direct", command: "./probe.sh", args: []string{"-c", "./probe.sh"}},
		{name: "quoted direct", command: "'./probe.sh'", args: []string{"-c", "'./probe.sh'"}},
		{name: "extensionless direct", command: "./probe", args: []string{"-c", "./probe"}},
		{name: "dot", command: ". ./probe.sh", args: []string{"-c", ". ./probe.sh"}},
		{name: "dot in command wrapper", command: o.name + " -c '. ./probe.sh'", args: []string{"-c", ". ./probe.sh"}},
		{name: "redirected shell stdin", command: redirect,
			args:    []string{"-c", redirect},
			refusal: o.name + " requires exactly -c or -lc and a literal command string"},
	}
	if o.name == "bash" {
		forms = append(forms, oracleFileForm{name: "source", command: "source ./probe.sh", args: []string{"-c", "source ./probe.sh"}})
	}
	for _, form := range forms {
		t.Run(form.name, func(t *testing.T) {
			probe := path
			if form.name == "extensionless direct" {
				probe = extensionless
			}
			o.check(t, form, probe, body, "printf", shellwords.POSIX)
		})
	}
}
