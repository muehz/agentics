//go:build race

package scriptsnap

// raceSlowdown scales wall-clock bounds in tests: the race detector slows reads and scheduling several times over, so
// a bound tuned for a normal build fails under it without any change in behavior.
const raceSlowdown = 10
