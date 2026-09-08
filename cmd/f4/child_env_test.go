package main

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func envHas(env []string, want string) bool {
	for _, kv := range env {
		if kv == want {
			return true
		}
	}
	return false
}

func envHasKey(env []string, key string) bool {
	for _, kv := range env {
		if strings.HasPrefix(kv, key+"=") {
			return true
		}
	}
	return false
}

func TestChildEnvAdvertisesGraphics(t *testing.T) {
	base := []string{"PATH=/bin", "KITTY_WINDOW_ID=77", "TERM_PROGRAM=far2l"}
	env := buildChildEnv(base, true, false)

	if !envHas(env, "PATH=/bin") {
		t.Error("the inherited environment must survive")
	}
	if !envHas(env, "F4_NESTED=1") {
		t.Error("F4_NESTED must still be exported")
	}
	if !envHas(env, "KITTY_WINDOW_ID=1") || envHas(env, "KITTY_WINDOW_ID=77") {
		t.Errorf("the graphics advertisement must be ours, got %v", env)
	}
	if !envHas(env, "TERM_PROGRAM=f4") || envHas(env, "TERM_PROGRAM=far2l") {
		t.Errorf("the program talks to f4, not to what started it: %v", env)
	}
}

func TestChildEnvKeepsQuietWithoutGraphics(t *testing.T) {
	base := []string{"PATH=/bin", "KITTY_WINDOW_ID=77", "TERM=xterm-256color"}
	env := buildChildEnv(base, false, false)

	if envHasKey(env, "KITTY_WINDOW_ID") {
		t.Errorf("a terminal that cannot show pictures must not claim it can: %v", env)
	}
	if !envHas(env, "TERM=xterm-256color") {
		t.Errorf("TERM must be left alone: %v", env)
	}
	if !envHas(env, "F4_NESTED=1") || !envHas(env, "PATH=/bin") {
		t.Errorf("the rest of the environment must be untouched: %v", env)
	}
}

func TestChildEnvAnnouncesKittyTerm(t *testing.T) {
	base := []string{"PATH=/bin", "TERM=xterm-256color"}
	env := buildChildEnv(base, true, true)

	if !envHas(env, "TERM=xterm-kitty") || envHas(env, "TERM=xterm-256color") {
		t.Errorf("TERM must name a terminal that draws pictures: %v", env)
	}

	count := 0
	for _, kv := range env {
		if strings.HasPrefix(kv, "TERM=") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("TERM must be exported exactly once, got %d times", count)
	}
}

func TestTerminfoExists(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TERMINFO", dir)
	t.Setenv("TERMINFO_DIRS", "")

	if terminfoExists("f4-no-such-terminal") {
		t.Error("an unknown terminal must not be reported as installed")
	}

	// The database keeps an entry under the first letter of its name.
	if err := os.MkdirAll(filepath.Join(dir, "x"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x", "xterm-kitty"), []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if !terminfoExists("xterm-kitty") {
		t.Error("an installed description must be found by its letter")
	}

	// And on some systems under the hexadecimal code of that letter.
	other := t.TempDir()
	t.Setenv("TERMINFO", other)
	if err := os.MkdirAll(filepath.Join(other, "78"), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(other, "78", "xterm-kitty"), []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if !terminfoExists("xterm-kitty") {
		t.Error("an installed description must be found by its hex directory")
	}
}

// chafa matches kitty on TERM being exactly xterm-kitty or on KITTY_PID being
// set, and does not look at KITTY_WINDOW_ID at all. A machine without the
// kitty terminfo entry installed — which is most machines that do not have
// kitty — therefore had nothing to go on and chafa drew characters until it
// was told "-f kitty" by hand. See chafa/chafa-term-db.c.
func TestChildEnvAnnouncesKittyPid(t *testing.T) {
	env := buildChildEnv([]string{"PATH=/usr/bin"}, true, false)

	var pid string
	for _, kv := range env {
		if strings.HasPrefix(kv, "KITTY_PID=") {
			pid = strings.TrimPrefix(kv, "KITTY_PID=")
		}
	}
	if pid == "" {
		t.Fatalf("KITTY_PID must be announced: %v", env)
	}
	if pid != strconv.Itoa(os.Getpid()) {
		t.Errorf("KITTY_PID = %q, want this process", pid)
	}
	// Both, because different tools look at different ones.
	if !slices.Contains(env, "KITTY_WINDOW_ID=1") {
		t.Errorf("KITTY_WINDOW_ID must still be announced: %v", env)
	}
}

// Claiming it where no picture can be shown would only make programs produce
// output nobody sees.
func TestChildEnvWithoutGraphicsClaimsNothing(t *testing.T) {
	env := buildChildEnv([]string{"PATH=/usr/bin"}, false, false)
	for _, kv := range env {
		if strings.HasPrefix(kv, "KITTY_PID=") || strings.HasPrefix(kv, "KITTY_WINDOW_ID=") {
			t.Errorf("nothing may be claimed: %q", kv)
		}
	}
}

// Whatever was inherited describes the terminal that started f4; the program
// about to start talks to us instead.
func TestChildEnvDropsInheritedKittyPid(t *testing.T) {
	env := buildChildEnv([]string{"KITTY_PID=999", "KITTY_WINDOW_ID=7", "PATH=/usr/bin"}, false, false)
	for _, kv := range env {
		if strings.HasPrefix(kv, "KITTY_PID=") || strings.HasPrefix(kv, "KITTY_WINDOW_ID=") {
			t.Errorf("the inherited value must not survive: %q", kv)
		}
	}
}

// A universal build reaches its libc by re-execing through the host loader
// and says so in its own environment. A child that inherits the guard is told
// the loader has already run when it has not, and dies before main -- which
// is what starting f4 from f4 did on Linux in issue #87.
func TestChildEnvDropsUniversalBridgeVariables(t *testing.T) {
	base := []string{
		"PATH=/usr/bin",
		"GOFFI_UNIVERSAL_REEXEC=1",
		"GOFFI_UNIVERSAL_EXE=42:/home/u/f4",
		"GOFFI_UNIVERSAL_ARGV0=42:/proc/self/fd/3",
		"F4_EXE=/home/u/f4",
	}
	env := buildChildEnv(base, false, false)

	if !envHas(env, "PATH=/usr/bin") {
		t.Error("the inherited environment must otherwise survive")
	}
	for _, key := range privateToThisProcess {
		if envHasKey(env, key) {
			t.Errorf("%s describes this process and must not be passed on: %v", key, env)
		}
	}
}

// The same holds when the host console, rather than the built-in terminal,
// hands the child its environment: the program still gets a libc of its own
// or none at all.
func TestChildEnvDropsUniversalBridgeVariablesInHostMode(t *testing.T) {
	env := buildChildEnv([]string{"GOFFI_UNIVERSAL_REEXEC=1", "PATH=/usr/bin"}, false, false)
	if envHasKey(env, "GOFFI_UNIVERSAL_REEXEC") {
		t.Errorf("host mode must drop the guard too: %v", env)
	}
}

// A partial entry has no name to match, and dropping it would quietly lose
// something the child was meant to have.
func TestPrivateEnvEntryIgnoresEntriesWithoutAName(t *testing.T) {
	if privateEnvEntry("GOFFI_UNIVERSAL_REEXEC") {
		t.Error("an entry with no '=' names no variable")
	}
	if privateEnvEntry("F4_EXECUTABLE=/home/u/f4") {
		t.Error("only the exact names are private, not everything starting with one")
	}
	if !privateEnvEntry("F4_EXE=") {
		t.Error("an empty value is still that variable")
	}
}
