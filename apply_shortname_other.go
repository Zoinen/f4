//go:build !windows

package main

func applyCommandShortPath(path string) string { return path }
