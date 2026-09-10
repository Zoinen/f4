//go:build !race

package testutil

// RaceEnabled reports whether the binary was built with the race detector.
// See race_enabled.go.
const RaceEnabled = false
