//go:build race

package shellwords

// raceSlowdown scales wall-clock bounds in tests: the race detector slows this parser several times over, so a bound
// tuned for a normal build fails under it without any change in complexity.
const raceSlowdown = 10
