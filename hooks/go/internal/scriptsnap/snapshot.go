package scriptsnap

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/pathshape"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

var (
	_ shellwords.ScriptReader     = (*Snapshot)(nil)
	_ shellwords.ScriptProber     = (*Snapshot)(nil)
	_ shellwords.ScriptClassifier = (*Snapshot)(nil)
)

// readCeiling bounds one file read or existence probe so a stuck filesystem call cannot hold every waiter on the entry.
var readCeiling = 1500 * time.Millisecond

// readBudget bounds all reads, classifications, and existence probes of one snapshot together, from its first such
// call. An operation that times out is abandoned, not stopped, so without it a stalled mount would cost readCeiling for
// each distinct path, and each abandoned call keeps a goroutine; once the budget is spent every new path is refused
// without touching the filesystem.
var readBudget = 2 * time.Second

// Snapshot is a once-only cache of script reads and existence probes shared by every check of one request.
//
// mu guards the maps and counters only. It is never held across once.Do, goroutines, or timers. The first read,
// classification, or probe sets the deadline.
type Snapshot struct {
	mu       sync.Mutex
	entries  map[string]*entry
	probes   map[string]*probeEntry
	observed map[string]shellwords.ScriptRead // first stable read of each file, including classification
	byID     map[string]struct{}              // file identities already charged as scripts
	policy   func(path string) string         // nil allows every path
	bytes    int                              // bytes charged toward MaxEventBytes
	deadline time.Time                        // end of readBudget
}

type entry struct {
	once    sync.Once
	read    shellwords.ScriptRead
	charge  sync.Once
	charged shellwords.ScriptRead
}

type probeEntry struct {
	once   sync.Once
	exists bool
	rule   shellwords.Rule
}

// Option configures a Snapshot.
type Option func(*Snapshot)

// WithPathPolicy sets a classifier that returns a non-empty reason for a path that must never be read. It runs
// inside the read, before any stat, open or read, on the lexical path and on the symlink-resolved path, and again on
// the path as it resolves after the open. The classifier is injected because the packages that know which paths are
// credentials import this one.
func WithPathPolicy(policy func(path string) string) Option {
	return func(s *Snapshot) { s.policy = policy }
}

// New returns an empty snapshot.
func New(opts ...Option) *Snapshot {
	s := &Snapshot{
		entries:  map[string]*entry{},
		probes:   map[string]*probeEntry{},
		observed: map[string]shellwords.ScriptRead{},
		byID:     map[string]struct{}{},
	}
	for _, o := range opts {
		o(s)
	}
	return s
}

// ProbeExists implements shellwords.ScriptProber. It reports whether path exists without opening file contents,
// consuming a MaxFiles slot, or counting toward MaxEventBytes, along with any refusal rule raised while probing.
func (s *Snapshot) ProbeExists(path string) (bool, shellwords.Rule) {
	if path == "" || pathshape.LocalFileProblem(path) != pathshape.NoProblem ||
		pathshape.Current().DrivePathOffHost(path) || pathshape.Current().ParentAfterNameOffHost(path) {
		return false, shellwords.RuleUnreadable
	}
	key := filepath.Clean(path)
	s.mu.Lock()
	if s.deadline.IsZero() {
		s.deadline = time.Now().Add(readBudget)
	}
	deadline := s.deadline
	pe := s.probes[key]
	if pe == nil {
		pe = &probeEntry{}
		s.probes[key] = pe
	}
	s.mu.Unlock()
	pe.once.Do(func() {
		// A panic below must not leave a zero rule, which reads as a clean non-existence verdict.
		pe.rule = shellwords.RuleUnreadable
		remaining := time.Until(deadline)
		if remaining <= 0 {
			pe.rule = shellwords.RuleBudget
			return
		}
		h := currentHooks()
		h.policy = s.policy
		h.beforeRead()
		ceiling := min(readCeiling, remaining)
		type probeOutcome struct {
			exists bool
			rule   shellwords.Rule
		}
		ch := make(chan probeOutcome, 1)
		go func() {
			defer func() {
				if recover() != nil {
					ch <- probeOutcome{exists: false, rule: shellwords.RuleUnreadable}
				}
			}()
			exists, rule := probePath(key, h)
			ch <- probeOutcome{exists: exists, rule: rule}
		}()
		timer := time.NewTimer(ceiling)
		defer timer.Stop()
		select {
		case out := <-ch:
			pe.exists, pe.rule = out.exists, out.rule
		case <-timer.C:
			pe.exists, pe.rule = false, shellwords.RuleBudget
		}
	})
	return pe.exists, pe.rule
}

func probePath(key string, h readHooks) (bool, shellwords.Rule) {
	if refusal, refused := h.checkPath(key); refused {
		return false, refusal.Limit
	}
	info, err := os.Lstat(key)
	if errors.Is(err, os.ErrNotExist) {
		return false, shellwords.RuleNone
	}
	if err != nil {
		return false, shellwords.RuleUnreadable
	}
	h.afterLstat()
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, ok := resolveExisting(key)
		if !ok {
			return false, shellwords.RuleUnreadable
		}
		if refusal, refused := h.checkResolved(key, resolved); refused {
			return false, refusal.Limit
		}
		targetInfo, err := os.Lstat(resolved)
		if errors.Is(err, os.ErrNotExist) {
			return false, shellwords.RuleNone
		}
		if err != nil || targetInfo.Mode()&os.ModeSymlink != 0 {
			return false, shellwords.RuleUnreadable
		}
		return true, shellwords.RuleNone
	}
	return true, shellwords.RuleNone
}

// ReadScript implements shellwords.ScriptReader. It charges the script-byte
// budget once for a cached file. The result, including an error, is final for
// this path for the request.
func (s *Snapshot) ReadScript(path string) shellwords.ScriptRead {
	e := s.inspectEntry(path)
	e.charge.Do(func() { e.charged = s.chargeScript(e.read) })
	return e.charged
}

// InspectScript implements shellwords.ScriptClassifier. It returns metadata
// from the same file slot and cached read as ReadScript. The body is available
// only from ReadScript, which charges the script-byte budget.
func (s *Snapshot) InspectScript(path string) shellwords.ScriptRead {
	read := s.inspectEntry(path).read
	read.Raw, read.Text = "", ""
	return read
}

// Every distinct path counts toward MaxFiles when its entry is created, whatever its read later returns, so a refusal
// or a timeout spends a slot too; a path past the limit is refused without an entry, so it stays refused.
func (s *Snapshot) inspectEntry(path string) *entry {
	key := filepath.Clean(path)
	s.mu.Lock()
	if s.deadline.IsZero() {
		s.deadline = time.Now().Add(readBudget)
	}
	deadline := s.deadline
	e := s.entries[key]
	if e == nil {
		if len(s.entries) >= MaxFiles {
			s.mu.Unlock()
			return &entry{read: refuse(key, shellwords.RuleTooManyFiles, "exceeds the per-event file limit")}
		}
		e = &entry{}
		s.entries[key] = e
	}
	s.mu.Unlock()
	e.once.Do(func() {
		// A panic below must not leave a zero ScriptRead, which reads as an empty successful script.
		e.read = refuse(key, shellwords.RuleUnreadable, "read did not complete")
		// The spelling is judged as given as well as after cleaning, because cleaning collapses a leading double
		// slash into a plain absolute path on unix.
		if problem := pathshape.LocalFileProblem(path); problem != pathshape.NoProblem {
			e.read = refuse(key, shellwords.RuleUnreadable, string(problem))
			return
		}
		if pathshape.Current().ParentAfterNameOffHost(path) {
			e.read = refuse(key, shellwords.RuleUnreadable, "parent reference after a path component")
			return
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			// The result is stored like any other, so later reads of this path short-circuit as well.
			e.read = refuse(key, shellwords.RuleBudget, "read budget exhausted")
			return
		}
		h := currentHooks()
		h.policy = s.policy
		h.beforeRead()
		ceiling := min(readCeiling, remaining)
		// The buffer lets an abandoned read finish without a receiver; it then only closes its file.
		ch := make(chan shellwords.ScriptRead, 1)
		go func() {
			defer func() {
				if recover() != nil {
					ch <- refuse(key, shellwords.RuleUnreadable, "read failed unexpectedly")
				}
			}()
			ch <- readFile(key, MaxScriptBytes, h)
		}()
		timer := time.NewTimer(ceiling)
		defer timer.Stop()
		select {
		case r := <-ch:
			e.read = s.observe(r)
		case <-timer.C:
			e.read = refuse(key, shellwords.RuleBudget, "read exceeded the time ceiling")
		}
	})
	return e
}

// An alias uses the first read only when its freshly read bytes match. File
// identities alone can collide, for example after inode reuse.
func (s *Snapshot) observe(r shellwords.ScriptRead) shellwords.ScriptRead {
	if r.Limit != shellwords.RuleNone || r.ID == "" {
		return r
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if first, ok := s.observed[r.ID]; ok {
		if first.SHA256 != r.SHA256 {
			return refuse(r.Path, shellwords.RuleChanging, "file differs from an earlier read of the same file")
		}
		return first
	}
	s.observed[r.ID] = r
	return r
}

func (s *Snapshot) chargeScript(r shellwords.ScriptRead) shellwords.ScriptRead {
	if r.Limit != shellwords.RuleNone {
		return r
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, charged := s.byID[r.ID]; charged {
		return r
	}
	if s.bytes+r.Bytes > MaxEventBytes {
		return refuse(r.Path, shellwords.RuleTooLarge, "exceeds the per-event limit")
	}
	s.bytes += r.Bytes
	if r.ID != "" {
		s.byID[r.ID] = struct{}{}
	}
	return r
}
