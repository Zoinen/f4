package terminal

import (
	"runtime"
	"testing"
)

func TestNativeShellName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "/bin/sh"},
		{"/bin/bash", "/bin/bash"},
		{"/usr/bin/zsh", "/usr/bin/zsh"},
		{"/usr/bin/fish", "bash"},
		{"/bin/tcsh", "bash"},
		{"/bin/csh", "bash"},
		{"fish", "bash"},
	}
	for _, tc := range tests {
		if got := nativeShellName(tc.in); got != tc.want {
			t.Errorf("nativeShellName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveHostExecutable(t *testing.T) {
	present := map[string]bool{"/usr/bin/bash": true, "/bin/sh": true, "/opt/x/tool": true}
	has := func(p string) bool { return present[p] }

	tests := []struct {
		name, path string
		want       string
		ok         bool
	}{
		{"/bin/sh", "", "/bin/sh", true},
		{"/bin/missing", "/bin", "/bin/missing", false},
		{"bash", "/usr/local/bin:/usr/bin:/bin", "/usr/bin/bash", true},
		{"bash", "/usr/local/bin/:/usr/bin/", "/usr/bin/bash", true},
		{"tool", ":/opt/x", "/opt/x/tool", true},
		{"nothere", "/usr/bin:/bin", "", false},
		{"bash", "", "", false},
		{"", "/bin", "", false},
	}
	for _, tc := range tests {
		got, ok := resolveHostExecutable(tc.name, tc.path, has)
		if got != tc.want || ok != tc.ok {
			t.Errorf("resolveHostExecutable(%q, %q) = (%q, %v), want (%q, %v)", tc.name, tc.path, got, ok, tc.want, tc.ok)
		}
	}
}

func TestHostEnvValueAndDefaultTerm(t *testing.T) {
	env := []string{"HOME=/home/u", "PATH=/usr/bin:/bin", "EMPTY="}
	if got := hostEnvValue(env, "PATH"); got != "/usr/bin:/bin" {
		t.Errorf("PATH = %q", got)
	}
	if got := hostEnvValue(env, "MISSING"); got != "" {
		t.Errorf("MISSING = %q", got)
	}
	if got := hostEnvValue(env, "HOM"); got != "" {
		t.Errorf("a key that is only a prefix of another matched: %q", got)
	}

	withTerm := withDefaultTerm([]string{"TERM=linux", "A=b"})
	if hostEnvValue(withTerm, "TERM") != "linux" {
		t.Errorf("an existing TERM was replaced: %v", withTerm)
	}
	without := withDefaultTerm([]string{"A=b"})
	if hostEnvValue(without, "TERM") != "xterm-256color" {
		t.Errorf("no default TERM was added: %v", without)
	}
}

// Off Windows the shell is never cmd.exe, and the native terminal does not exist.
func TestWindowsShellSyntaxOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("decided by the Wine probe on Windows")
	}
	if WindowsShellSyntax() {
		t.Fatal("WindowsShellSyntax is true off Windows")
	}
	if _, ok, _ := newWinePTY(); ok {
		t.Fatal("a native Wine pty was offered off Windows")
	}
}
