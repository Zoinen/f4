package vtui

import (
	"runtime"
	"strings"
	"testing"
)

func withBackend(t *testing.T, name string, details ...string) {
	t.Helper()
	backendMu.RLock()
	prevName, prevDetails := activeBackend, append([]string(nil), backendDetails...)
	backendMu.RUnlock()
	SetActiveBackend(name, details...)
	t.Cleanup(func() {
		backendMu.Lock()
		activeBackend, backendDetails = prevName, prevDetails
		backendMu.Unlock()
	})
}

func TestSetAndReadActiveBackend(t *testing.T) {
	withBackend(t, "ebiten")
	if got := ActiveBackend(); got != "ebiten" {
		t.Errorf("ActiveBackend() = %q, want \"ebiten\"", got)
	}
}

// With four backends and an automatic fallback chain, the title is the only
// place the user can see which one actually came up.
func TestWindowTitleWithBackend(t *testing.T) {
	withBackend(t, "ebiten")
	if got, want := WindowTitleWithBackend("f4"), "f4 [ebiten]"; got != want {
		t.Errorf("WindowTitleWithBackend = %q, want %q", got, want)
	}
	if got, want := WindowTitleWithBackend(""), "[ebiten]"; got != want {
		t.Errorf("empty title = %q, want %q", got, want)
	}
}

// In a terminal no host claims a backend, and the title must be left alone
// rather than decorated with an empty pair of brackets.
func TestWindowTitleWithBackend_UnclaimedIsUntouched(t *testing.T) {
	backendMu.RLock()
	prev := activeBackend
	backendMu.RUnlock()
	backendMu.Lock()
	activeBackend = ""
	backendMu.Unlock()
	t.Cleanup(func() {
		backendMu.Lock()
		activeBackend = prev
		backendMu.Unlock()
	})

	if got := WindowTitleWithBackend("f4"); got != "f4" {
		t.Errorf("WindowTitleWithBackend = %q, want it unchanged", got)
	}
}

func TestBackendAbout(t *testing.T) {
	withBackend(t, "ebiten", "cell 8x16, scale 1", "cgo-free")

	about := BackendAbout()
	for _, want := range []string{"ebiten", runtime.GOOS, runtime.GOARCH, runtime.Version(), "cgo-free", "cell 8x16"} {
		if !strings.Contains(about, want) {
			t.Errorf("BackendAbout() is missing %q:\n%s", want, about)
		}
	}
}

func TestBackendAbout_TerminalSaysSo(t *testing.T) {
	backendMu.Lock()
	prev := activeBackend
	activeBackend = ""
	backendMu.Unlock()
	t.Cleanup(func() {
		backendMu.Lock()
		activeBackend = prev
		backendMu.Unlock()
	})

	if !strings.Contains(BackendAbout(), "terminal") {
		t.Errorf("with no backend claimed, about should say terminal:\n%s", BackendAbout())
	}
}
