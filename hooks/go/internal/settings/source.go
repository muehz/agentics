package settings

import "sync"

// Source resolves workflow settings lazily and at most once per working
// directory, so every check of one request shares a single reading of the
// settings files. It reads nothing when constructed, and it is safe for
// concurrent use because the checks of a grouped event run on separate
// goroutines.
//
// A nil *Source calls Resolve on every call, so a hand-built request keeps the
// behaviour of reading the files each time and a repaired file takes effect on
// the next call.
type Source struct {
	resolve func(cwd string) Result

	mu      sync.Mutex
	entries map[string]*sourceEntry
}

type sourceEntry struct {
	once sync.Once
	res  Result
}

// NewSource returns a Source that calls resolve for each distinct directory it
// is asked about. It does not call resolve itself.
func NewSource(resolve func(cwd string) Result) *Source {
	return &Source{resolve: resolve, entries: map[string]*sourceEntry{}}
}

// Resolve returns the settings for cwd. The first call for a directory runs
// the resolver; concurrent and later calls for it wait for and return that same
// result. A nil Source returns Resolve(cwd).
func (s *Source) Resolve(cwd string) Result {
	if s == nil {
		return Resolve(cwd)
	}
	s.mu.Lock()
	e, ok := s.entries[cwd]
	if !ok {
		e = &sourceEntry{}
		s.entries[cwd] = e
	}
	s.mu.Unlock()
	e.once.Do(func() { e.res = s.resolve(cwd) })
	return e.res
}
