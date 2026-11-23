package hookcost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/hosttest"
	"github.com/muehz/agentics/hooks/go/internal/surface"
)

// Config is one shipped hook registration as the capture stores and replays
// it.
type Config struct {
	File           string
	Provider       string
	Event          string
	Hook           string
	Field          string
	Command        string
	Argv           []string
	Matcher        string
	TimeoutSeconds int
}

// ConfigsFrom converts shipped registrations to the records the capture
// stores and replays: the event is upper-cased at its first letter, and Argv
// is the surface, the hook, and the hook's own arguments, as the binary
// receives them. A fractional timeout rounds up to whole seconds.
func ConfigsFrom(registrations []hosttest.Registration) []Config {
	files := map[string]string{"antigravity": "hooks.json", "claude-code": "hooks.claude.json", "codex": "hooks.codex.json", "copilot": "hooks.copilot.json"}
	configs := make([]Config, 0, len(registrations))
	for _, r := range registrations {
		event := r.Event
		if event != "" {
			event = strings.ToUpper(event[:1]) + event[1:]
		}
		configs = append(configs, Config{
			File:           files[string(r.Surface)],
			Provider:       string(r.Surface),
			Event:          event,
			Hook:           r.Hook,
			Field:          r.Field,
			Command:        r.Command,
			Argv:           append([]string{string(r.Surface), r.Hook}, r.Args...),
			Matcher:        r.Matcher,
			TimeoutSeconds: int(math.Ceil(r.Timeout.Seconds())),
		})
	}
	return configs
}

// Selected returns the registrations the surface's host runs on goos for a
// neutral tool name, with the harness record of each at the same index.
func Selected(tb testing.TB, registrations []hosttest.Registration, provider surface.ID, tool, goos string) ([]hosttest.Registration, []Config) {
	tb.Helper()
	selected, err := hosttest.Selected(registrations, provider, hosttest.NativeTool(provider, tool), goos)
	if err != nil {
		tb.Fatal(err)
	}
	return selected, ConfigsFrom(selected)
}

// Launch resolves the host shell for a registration on the running OS. The
// expanded config is the text the shell receives.
func Launch(r hosttest.Registration) (Command, error) {
	name, args, err := r.HostCommand(runtime.GOOS)
	if err != nil {
		return Command{}, err
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return Command{}, err
	}
	expanded := r.Command
	if r.Field != "powershell" {
		expanded = args[len(args)-1]
	}
	return Command{Executable: path, Args: args, RawConfig: r.Command, ExpandedConfig: expanded, TimeoutSeconds: int(math.Ceil(r.Timeout.Seconds()))}, nil
}

// Summary holds the descriptive statistics of one sample set, in
// nanoseconds.
type Summary struct {
	Count    int
	MeanNS   float64
	MedianNS float64
	P95NS    int64
	MinNS    int64
	MaxNS    int64
	StddevNS float64
}

// Summarize computes the statistics of samples. It keeps every sample, with
// no outlier removal, and returns the zero Summary for none.
func Summarize(samples []int64) Summary {
	if len(samples) == 0 {
		return Summary{}
	}
	ordered := append([]int64(nil), samples...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	s := Summary{Count: len(samples), MinNS: ordered[0], MaxNS: ordered[len(ordered)-1], P95NS: ordered[(95*len(ordered)+99)/100-1]}
	for _, ns := range samples {
		s.MeanNS += float64(ns)
	}
	s.MeanNS /= float64(len(samples))
	s.MedianNS = float64(ordered[len(ordered)/2])
	if len(ordered)%2 == 0 {
		s.MedianNS = (float64(ordered[len(ordered)/2-1]) + s.MedianNS) / 2
	}
	for _, ns := range samples {
		delta := float64(ns) - s.MeanNS
		s.StddevNS += delta * delta
	}
	s.StddevNS = math.Sqrt(s.StddevNS / float64(len(samples)))
	return s
}

// Command is one process launch: what to run, where, and what to feed it.
type Command struct {
	Executable     string
	Args           []string
	WorkingDir     string `json:",omitempty"`
	RawConfig      string `json:",omitempty"`
	ExpandedConfig string `json:",omitempty"`
	Payload        string
	TimeoutSeconds int
}

// workingDir is the directory the launch runs in: its own, else the fixture.
func (c Command) workingDir(fixture string) string {
	if c.WorkingDir != "" {
		return c.WorkingDir
	}
	return fixture
}

// Span is one named interval inside a sequential replay, in nanoseconds from
// the replay's start.
type Span struct {
	Name    string
	StartNS int64
	EndNS   int64
}

// Record is the samples one arm produced.
type Record struct {
	ID                string
	Layer             string
	Commands          []Command
	SamplesNS         []int64
	Summary           Summary
	Spans             [][]Span `json:",omitempty"`
	Error             string   `json:",omitempty"`
	RejectedSamplesNS []int64  `json:",omitempty"`
}

// Delta is the per-sample difference between two records of one pairing.
type Delta struct {
	ID             string
	Interpretation string
	SamplesNS      []int64
	Summary        Summary
}

// Capture is the JSON document one run writes.
type Capture struct {
	StartedUTC   string
	RecordedUTC  string
	Platform     string
	Method       string
	Condition    string
	ColdEvidence string
	Warmups      int
	Units        string
	FixtureFiles map[string]string
	FixtureDir   string
	Environment  map[string]string
	Metadata     map[string]string
	Hashes       map[string]string
	Configs      []Config
	Records      []Record
	PairedDeltas []Delta
	Gaps         []string
}

// Arm is one measured route to a hook. Run returns one sample, Preflight
// runs before warm samples, and Validate checks a cold sample afterwards.
type Arm struct {
	ID        string
	Layer     string
	Commands  []Command
	Config    *Config
	Run       func() (int64, error)
	Spans     func() []Span
	Preflight func() error
	Validate  func() error
}

// Control returns the arm that times c alone, so a paired delta can subtract
// the cost of the shell or platform detection around a hook.
func Control(id string, c Command, fixture string, env []string) Arm {
	return Arm{ID: id, Layer: "component-control", Commands: []Command{c}, Preflight: func() error { return nil }, Run: func() (int64, error) {
		ns, _, stderr, err := Exec(c, fixture, env)
		if err != nil {
			return ns, fmt.Errorf("%w stderr=%q", err, stderr)
		}
		return ns, nil
	}}
}

// LaunchArm returns the arm that times p through l. Preflight and Validate
// are l.Prepare, so a cold sample taken before them is judged against the
// reference afterwards. A launch or output failure is prefixed with id; a
// missing binary is reported in CheckBinary's own words.
func LaunchArm(id, layer string, l *LaunchRoute, p Probe, c Config) Arm {
	arm := Arm{ID: id, Layer: layer, Commands: []Command{l.Command(p)}, Config: &c}
	arm.Run = func() (int64, error) {
		if err := l.check(p); err != nil {
			return 0, err
		}
		elapsed, err := l.invoke(p)
		if err != nil {
			return elapsed.Nanoseconds(), fmt.Errorf("%s: %w", id, err)
		}
		return elapsed.Nanoseconds(), nil
	}
	arm.Preflight = func() error { return l.Prepare(p) }
	arm.Validate = arm.Preflight
	arm.Spans = func() []Span { return nil }
	return arm
}

// Measure runs arms and returns one Record each. A warm run preflights each
// arm and discards five warmup samples; a cold run does neither and checks
// Validate afterwards, moving rejected samples out of the evidence. Rounds
// interleave the arms so paired arms see the same time-order drift.
func Measure(arms []Arm, cold bool, n int, progress func(int)) []Record {
	result := make([]Record, len(arms))
	for i, arm := range arms {
		result[i] = Record{ID: arm.ID, Layer: arm.Layer, Commands: arm.Commands}
		if !cold {
			if err := arm.Preflight(); err != nil {
				result[i].Error = err.Error()
				continue
			}
			for k := 0; k < 5; k++ {
				if _, err := arm.Run(); err != nil {
					result[i].Error = err.Error()
					break
				}
			}
		}
	}
	// Nearby paired arms run in the same recorded round to reduce time-order drift.
	for k := 0; k < n; k++ {
		for i, arm := range arms {
			if result[i].Error != "" {
				continue
			}
			ns, err := arm.Run()
			if err != nil {
				result[i].Error = err.Error()
				continue
			}
			result[i].SamplesNS = append(result[i].SamplesNS, ns)
			if arm.Spans != nil {
				result[i].Spans = append(result[i].Spans, arm.Spans())
			}
		}
		if progress != nil {
			progress(k + 1)
		}
	}
	for i, arm := range arms {
		if cold && result[i].Error == "" && arm.Validate != nil {
			if err := arm.Validate(); err != nil {
				result[i].Error = err.Error()
				result[i].RejectedSamplesNS = result[i].SamplesNS
				result[i].SamplesNS = nil
				result[i].Spans = nil
			}
		}
		result[i].Summary = Summarize(result[i].SamplesNS)
	}
	return result
}

// PairedDeltas subtracts each record's partner, sample by sample: native
// launch from in-process run, dispatcher from native, config replay from
// dispatcher, and platform detection from the host shell. A pair is skipped
// when either side failed or the sample counts differ.
func PairedDeltas(records []Record) []Delta {
	index := map[string]Record{}
	for _, r := range records {
		index[r.ID] = r
	}
	var deltas []Delta
	for _, r := range records {
		if r.Error != "" {
			continue
		}
		var baseID, interpretation string
		switch r.Layer {
		case "native-launch":
			baseID = strings.Replace(r.ID, "/native/", "/in-process/", 1)
			interpretation = "native launch minus in-process run; combined process/runtime initialization and harness-boundary estimate, not pure binary startup"
		case "dispatcher-launch":
			baseID = strings.Replace(r.ID, "/dispatcher/", "/native/", 1)
			interpretation = "dispatcher minus native launch; combined shell/wrapper/platform selection estimate"
		case "config-command-replay":
			baseID = strings.Replace(r.ID, "/config/", "/dispatcher/", 1)
			interpretation = "config-shell replay minus dispatcher launch; route-dependent outer shell estimate, not real host overhead"
		default:
			if r.ID == "control/platform-detection" {
				baseID = "control/host-shell"
				interpretation = "shell plus platform detection control minus shell no-op"
			}
		}
		base, ok := index[baseID]
		if !ok || base.Error != "" || len(r.SamplesNS) != len(base.SamplesNS) {
			continue
		}
		d := Delta{ID: r.ID + " minus " + baseID, Interpretation: interpretation}
		for i, ns := range r.SamplesNS {
			d.SamplesNS = append(d.SamplesNS, ns-base.SamplesNS[i])
		}
		d.Summary = Summarize(d.SamplesNS)
		deltas = append(deltas, d)
	}
	return deltas
}

// Metadata describes the machine and the inputs of a capture. It returns the
// environment facts, each read through a short-lived command whose failure is
// recorded as "unavailable: ..." rather than aborting, and the SHA-256 of the
// measured binary, the shipped wrapper and configs, and each entry of
// sources, keyed by the path as given.
func Metadata(root, hooksDir, binary string, sources []string) (map[string]string, map[string]string) {
	meta := map[string]string{"go_test_runtime": runtime.Version(), "architecture": runtime.GOARCH, "machine_name": os.Getenv("COMPUTERNAME"), "controller": "go testing; standalone process/config replay, no model requests"}
	read := func(executable string, args ...string) string {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := Process(ctx, executable, args)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Sprintf("unavailable: %v: %s", err, out)
		}
		return strings.TrimSpace(string(out))
	}
	meta["source_revision"] = read("git", "rev-parse", "HEAD")
	meta["dirty_paths"] = read("git", "status", "--porcelain")
	meta["tracked_dirty_diff"] = read("git", "diff", "HEAD", "--", "hooks/go", "hooks")
	meta["git_version"] = read("git", "--version")
	meta["go_binary_build_info"] = read("go", "version", "-m", binary)
	if runtime.GOOS == "windows" {
		meta["os_machine_shell"] = read("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", `$PSVersionTable.PSVersion.ToString(); Get-CimInstance Win32_OperatingSystem | Format-List Caption,Version,OSArchitecture; Get-CimInstance Win32_ComputerSystem | Format-List Manufacturer,Model,TotalPhysicalMemory; Get-CimInstance Win32_Processor | Format-List Name,NumberOfCores,NumberOfLogicalProcessors`)
		meta["cmd_version"] = read(os.Getenv("ComSpec"), "/d", "/s", "/c", "ver")
	} else {
		meta["os_arch"] = read("uname", "-a")
		meta["shell_version"] = read("bash", "--version")
		if runtime.GOOS == "darwin" {
			meta["machine"] = read("sysctl", "-n", "hw.model", "hw.memsize", "machdep.cpu.brand_string")
		} else {
			meta["machine"] = read("sh", "-c", "cat /proc/cpuinfo /proc/meminfo")
		}
	}
	for _, host := range []string{"claude", "codex", "copilot"} {
		if runtime.GOOS == "windows" && host == "copilot" {
			meta[host+"_version"] = read(os.Getenv("ComSpec"), "/d", "/s", "/c", "copilot --version")
		} else {
			meta[host+"_version"] = read(host, "--version")
		}
	}
	hashes := map[string]string{}
	paths := []string{binary, filepath.Join(hooksDir, "run-hook.cmd")}
	for _, name := range []string{"hooks.claude.json", "hooks.codex.json", "hooks.copilot.json"} {
		paths = append(paths, filepath.Join(hooksDir, name))
	}
	paths = append(paths, filepath.Join(filepath.Dir(hooksDir), "hooks.json"))
	paths = append(paths, sources...)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			hashes[path] = "unavailable: " + err.Error()
			continue
		}
		sum := sha256.Sum256(data)
		hashes[path] = hex.EncodeToString(sum[:])
	}
	return meta, hashes
}
