package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/unxed/vtui"
)

type F4ClipboardAuth struct {
	mu      sync.Mutex
	autheds map[string]bool
	path    string
}

func NewF4ClipboardAuth() *F4ClipboardAuth {
	p := filepath.Join(GetF4ConfigDir(), "tty_clipboard", "autheds")
	os.MkdirAll(filepath.Dir(p), 0755)

	auths := make(map[string]bool)
	if data, err := os.ReadFile(p); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, l := range lines {
			if l != "" {
				auths[strings.TrimSpace(l)] = true
			}
		}
	}
	return &F4ClipboardAuth{autheds: auths, path: p}
}

func (a *F4ClipboardAuth) Authorize(clientID string) int {
	vtui.DebugLog("CLIPBOARD_AUTH: Authorize request from clientID: %q", clientID)
	a.mu.Lock()
	authed := a.autheds[clientID]
	a.mu.Unlock()

	if authed {
		vtui.DebugLog("CLIPBOARD_AUTH: Client %q already in autheds cache", clientID)
		return 1
	}

	resChan := make(chan int, 1)
	vtui.FrameManager.PostTask(func() {
		dlg := vtui.ShowMessage(
			" Clipboard Access ",
			"External application requests clipboard access.\nClient ID: "+clientID,
			[]string{"&Reject", "Allow &Once", "Allow &Always", "Use &Local"},
		)
		dlg.OnResult = func(c int) { resChan <- c }
	})

	ans := <-resChan
	vtui.DebugLog("CLIPBOARD_AUTH: User dialog result for %q: %d", clientID, ans)

	switch ans {
	case 1:
		return 1 // Allow Once
	case 2: // Allow Always
		vtui.DebugLog("CLIPBOARD_AUTH: Adding %q to persistent autheds", clientID)
		a.mu.Lock()
		a.autheds[clientID] = true
		a.mu.Unlock()
		f, _ := os.OpenFile(a.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
		if f != nil {
			_ = f.Chmod(0600)
			f.WriteString(clientID + "\n")
			f.Close()
		}
		return 1
	case 3:
		vtui.DebugLog("CLIPBOARD_AUTH: Switching to Local mode for %q", clientID)
		return -1 // Use Local
	default:
		return 0 // Reject
	}
}
