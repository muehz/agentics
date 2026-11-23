package scriptsnap

import (
	"path/filepath"
	"sync"
	"time"

	"github.com/muehz/agentics/hooks/go/internal/pathshape"
	"github.com/muehz/agentics/hooks/go/internal/shellwords"
)

// readCeiling bounds one file read so a stuck filesystem call cannot hold every waiter on the entry.
var readCeiling = 1500 * time.Millisecond

// readBudget bounds all reads of one snapshot together, from its first read. A read that times out is abandoned, not
// stopped, so without it a stalled mount would cost readCeiling for each distinct path, and each abandoned read keeps
// a goroutine; once the budget is spent every new path is refused without a read.
var readBudget = 2 * time.Second

// Snapshot is a once-only cache of script reads shared by every check of one request.
//
// mu guards the maps and counters only. It is never held across an entry's once, the read goroutine or a timer, and
// a read never calls ReadScript, so no lock order exists.
type Snapshot struct {
	mu       sync.Mutex
	entries  map[string]*entry
	byID     map[string]shellwords.ScriptRead // first successful read of each distinct file
	policy   func(path string) string         // nil allows every path
	bytes    int                              // bytes of the successful reads counted toward MaxEventBytes
	deadline time.Time                        // end of readBudget, set by the first ReadScript
}

type entry struct {
	once sync.Once
	read shellwords.ScriptRead
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
	s := &Snapshot{entries: map[string]*entry{}, byID: map[string]shellwords.ScriptRead{}}
	for _, o := range opts {
		o(s)
	}
	return s
}

// ReadScript implements shellwords.ScriptReader. The result, including any error, is final for the request, and a
// distinct file is read at most once however many paths reach it.
//
// Every distinct path counts toward MaxFiles when its entry is created, whatever its read later returns, so a refusal
// or a timeout spends a slot too; a path past the limit is refused without an entry, so it stays refused.
func (s *Snapshot) ReadScript(path string) shellwords.ScriptRead {
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
			return refuse(key, shellwords.RuleTooManyFiles, "exceeds the per-event file limit")
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
			e.read = s.dedupe(r)
		case <-timer.C:
			e.read = refuse(key, shellwords.RuleBudget, "read exceeded the time ceiling")
		}
	})
	return e.read
}

// dedupe returns the first stored read of the file r came from, so an alias of an already-read file neither counts
// twice. An identity alone is not trusted: filesystems with a degenerate index and inode reuse within one request
// give different files the same identity, so the alias is answered with the first read only when its freshly read
// bytes match; otherwise it is refused, never answered with another script's text.
//
// A new file's bytes count toward MaxEventBytes here, before it is stored, so a read refused for the total is never
// answered to a later alias. A read without an identity cannot be matched and counts again for every path.
func (s *Snapshot) dedupe(r shellwords.ScriptRead) shellwords.ScriptRead {
	if r.Limit != shellwords.RuleNone {
		return r
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if r.ID != "" {
		if first, ok := s.byID[r.ID]; ok {
			if first.SHA256 != r.SHA256 {
				return refuse(r.Path, shellwords.RuleChanging, "file differs from an earlier read of the same file")
			}
			return first
		}
	}
	if s.bytes+r.Bytes > MaxEventBytes {
		return refuse(r.Path, shellwords.RuleTooLarge, "exceeds the per-event limit")
	}
	s.bytes += r.Bytes
	if r.ID != "" {
		s.byID[r.ID] = r
	}
	return r
}
