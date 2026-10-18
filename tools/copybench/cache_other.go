//go:build !linux

package main

// dropCache does nothing here: the files the benchmark has just written may
// still be in memory, and the first runs may read them from there.
func dropCache([]string) error { return nil }

const cacheNote = "source cache: NOT dropped on this system; the source files may be read from memory, so -src numbers can be far above the disk"
