//go:build !race

package main

// raceEnabled reports whether the binary was built with the race detector.
// See race_enabled_test.go.
const raceEnabled = false
