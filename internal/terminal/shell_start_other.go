//go:build !darwin

package terminal

// InteractiveShellArgs leaves the existing shell startup policy unchanged on
// platforms other than macOS, including Windows and Wine shell transports.
func InteractiveShellArgs() []string {
	return []string{}
}
