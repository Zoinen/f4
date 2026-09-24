//go:build windows

package cmdline

import "golang.org/x/sys/windows"

// PromptUserIsAdmin reports whether this session runs elevated, which is what
// $# and $@ ask about.
func PromptUserIsAdmin() bool {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}

// PromptAdminLabel is what $@xx puts between its brackets.
func PromptAdminLabel() string {
	return "Administrator"
}
