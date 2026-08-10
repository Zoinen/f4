//go:build !windows

package vtui

func getSystemScrollLines() int {
	return 3
}
