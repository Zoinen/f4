//go:build !windows

package cmdline

func ApplyCommandShortPath(path string) string { return path }
