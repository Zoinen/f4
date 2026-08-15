package vtui

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
)

var (
	backendMu      sync.RWMutex
	activeBackend  string
	backendDetails []string
)

// SetActiveBackend records which rendering backend the process ended up in and
// logs it.
//
// With four GUI backends and an automatic fallback chain, the one actually in
// use is not obvious from the outside: a machine where gogpu fails quietly
// lands on another and looks identical. The name goes into the window title
// and into the debug log, so a bug report says what it was running on without
// anyone having to reproduce it first.
//
// details are optional extra facts about the backend, shown by BackendAbout.
func SetActiveBackend(name string, details ...string) {
	backendMu.Lock()
	activeBackend = name
	backendDetails = append([]string(nil), details...)
	backendMu.Unlock()

	DebugLog("BACKEND: active rendering backend is %q", name)
	for _, d := range details {
		DebugLog("BACKEND: %s", d)
	}
}

// ActiveBackend returns the name of the backend in use, or "" before a host
// has claimed one, which is the case in a plain terminal.
func ActiveBackend() string {
	backendMu.RLock()
	defer backendMu.RUnlock()
	return activeBackend
}

// WindowTitleWithBackend appends the backend name to a window title.
//
// This is the far2l habit: the title says what is drawing it, so a screenshot
// carries that information too.
func WindowTitleWithBackend(title string) string {
	name := ActiveBackend()
	if name == "" {
		return title
	}
	if title == "" {
		return "[" + name + "]"
	}
	return title + " [" + name + "]"
}

// BackendAbout returns a short human readable description of what the process
// is running on, in the spirit of far2l's about box: the backend, the platform
// and whatever the backend chose to report about itself.
func BackendAbout() string {
	backendMu.RLock()
	name := activeBackend
	details := append([]string(nil), backendDetails...)
	backendMu.RUnlock()

	if name == "" {
		name = "terminal"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", AppName)
	fmt.Fprintf(&b, "Backend:  %s\n", name)
	fmt.Fprintf(&b, "Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "Go:       %s\n", runtime.Version())
	for _, d := range details {
		fmt.Fprintf(&b, "          %s\n", d)
	}
	return b.String()
}
