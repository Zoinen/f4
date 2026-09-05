//go:build race

package main

// raceEnabled reports whether the binary was built with the race detector.
// The detector instruments every memory access, so wall-clock assertions
// measure the instrumentation rather than the code under test.
const raceEnabled = true
