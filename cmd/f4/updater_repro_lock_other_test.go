//go:build !windows

package main

import "errors"

// lockFileExclusively only exists on Windows; the unix branch of the repro
// test blocks the update through directory permissions instead.
func lockFileExclusively(string) (func(), error) {
	return nil, errors.New("exclusive file locks are a Windows concept")
}
