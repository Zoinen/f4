//go:build !windows

package terminal

// Off Windows there is no libwinescape and no second file-layer personality:
// os.* and path/filepath already speak POSIX, so there is nothing to report.
func winescapeFacts() [][2]string { return nil }
