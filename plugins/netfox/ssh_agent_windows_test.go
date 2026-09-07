//go:build windows

package netfox

import "testing"

func TestIsWindowsNamedPipe(t *testing.T) {
	tests := map[string]bool{
		`\\.\pipe\openssh-ssh-agent`: true,
		`//./pipe/pageant-user-key`:  true,
		`\\?\pipe\custom-agent`:      true,
		`C:\Users\user\.ssh\agent`:   false,
		`/tmp/ssh-agent.sock`:        false,
	}
	for path, want := range tests {
		if got := isWindowsNamedPipe(path); got != want {
			t.Errorf("isWindowsNamedPipe(%q) = %v, want %v", path, got, want)
		}
	}
}
