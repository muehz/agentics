//go:build !race

package shellwords

// raceSlowdown scales wall-clock bounds in tests; see race_test.go.
const raceSlowdown = 1
