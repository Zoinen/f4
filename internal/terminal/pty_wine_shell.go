package terminal

import (
	"runtime"
	"strings"
)

// The native terminal under Wine (WINE.md §18.3): with libwinescape, f4 can run
// the host's own shell on a real pseudo-terminal instead of cmd.exe. The
// decisions that need no Wine to make live here so they run in every test.

// WindowsShellSyntax reports whether the local command language is cmd.exe.
// UseWinescape selects a process personality, not just a faster filesystem:
// when it is active under Wine, every local command path speaks POSIX even if
// the PTY probe failed and execution has degraded to a simple mode.
func WindowsShellSyntax() bool {
	return runtime.GOOS == "windows" && !nativeShellActive()
}

// NewLocalPTY allocates the pseudo-terminal the local shell runs on: the native
// host pty under Wine when libwinescape is allowed and works, the platform's
// own otherwise.
func NewLocalPTY() (PtyBackend, error) {
	if p, ok, err := newWinePTY(); ok {
		if err != nil {
			return nil, err
		}
		return p, nil
	}
	p, err := NewPTY()
	if err != nil {
		return nil, err
	}
	return p, nil
}

// nativeShellName picks the host shell from $SHELL, the way GetSystemShell does
// on Unix: fish, csh and tcsh are not the shell f4's integration speaks, so bash
// stands in for them.
func nativeShellName(hostShell string) string {
	if hostShell == "" {
		return "/bin/sh"
	}
	base := hostShell
	if i := strings.LastIndexByte(hostShell, '/'); i >= 0 {
		base = hostShell[i+1:]
	}
	switch base {
	case "fish", "csh", "tcsh":
		return "bash"
	}
	return hostShell
}

// resolveHostExecutable finds name for execve, which does not search PATH: a
// name with a slash is taken as it is, anything else is looked up in each
// directory of pathEnv in turn. isExecutable answers for one candidate.
func resolveHostExecutable(name, pathEnv string, isExecutable func(string) bool) (string, bool) {
	if name == "" {
		return "", false
	}
	if strings.Contains(name, "/") {
		return name, isExecutable(name)
	}
	for _, dir := range strings.Split(pathEnv, ":") {
		if dir == "" {
			continue
		}
		candidate := strings.TrimRight(dir, "/") + "/" + name
		if isExecutable(candidate) {
			return candidate, true
		}
	}
	return "", false
}

// hostEnvValue returns the value of key in a KEY=VALUE environment.
func hostEnvValue(env []string, key string) string {
	prefix := key + "="
	for _, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			return kv[len(prefix):]
		}
	}
	return ""
}

// withDefaultTerm gives a shell started from a desktop launcher a terminal type;
// the host environment has none when f4 was not started from a terminal.
func withDefaultTerm(env []string) []string {
	for _, kv := range env {
		if strings.HasPrefix(kv, "TERM=") {
			return env
		}
	}
	return append(env, "TERM=xterm-256color")
}
