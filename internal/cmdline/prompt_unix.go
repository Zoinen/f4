//go:build !windows

package cmdline

import "os"

// PromptUserIsAdmin reports whether this session runs with the rights that
// make a shell prompt end in '#' rather than '$'.
func PromptUserIsAdmin() bool {
	return os.Geteuid() == 0
}

// PromptAdminLabel is what $@xx puts between its brackets.
func PromptAdminLabel() string {
	return "Root"
}
