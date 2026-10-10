package plughost

import (
	"errors"
	"testing"
	"time"
)

type guardTestTransport struct{ during func() }

func (t *guardTestTransport) Call(method string, params any, result any) error {
	if t.during != nil {
		t.during()
	}
	return nil
}

// A call made from the UI goroutine marks the plugin as blocking it until the
// call returns; a call from any other goroutine does not.
func TestUIGuardTracksCallsFromTheUIGoroutine(t *testing.T) {
	old := uiGoroutine.Load()
	t.Cleanup(func() { uiGoroutine.Store(old) })

	var inside bool
	tr := &guardTestTransport{}
	g := &uiGuard{inner: tr}
	tr.during = func() { inside = g.uiBlocked() }

	uiGoroutine.Store(0) // not learned: nothing is known to be the UI goroutine
	if err := g.Call("VFS.ReadDir", nil, nil); err != nil || inside {
		t.Fatalf("call with an unknown UI goroutine: err=%v blocked=%v", err, inside)
	}

	uiGoroutine.Store(currentGoroutineID())
	if err := g.Call("VFS.ReadDir", nil, nil); err != nil || !inside {
		t.Fatalf("call from the UI goroutine: err=%v blocked=%v, want blocked", err, inside)
	}
	if g.uiBlocked() {
		t.Fatal("still blocked after the call returned")
	}

	uiGoroutine.Store(currentGoroutineID() + 1) // some other goroutine is the UI one
	if err := g.Call("VFS.ReadDir", nil, nil); err != nil || inside {
		t.Fatalf("call from a background goroutine: err=%v blocked=%v", err, inside)
	}
}

// Host.InputBox and Host.Menu refuse instead of deadlocking while the UI
// goroutine waits for the plugin.
func TestHostDialogsRefusedWhileUIWaitsForPlugin(t *testing.T) {
	old := uiGoroutine.Load()
	t.Cleanup(func() { uiGoroutine.Store(old) })
	uiGoroutine.Store(currentGoroutineID())

	tr := &guardTestTransport{}
	g := &uiGuard{inner: tr}
	methods := newHostMethods(newLuaTestHostAPI(), g, "test", nil)

	tr.during = func() {
		for _, m := range []string{"Host.InputBox", "Host.Menu"} {
			if _, err := methods[m](nil); !errors.Is(err, errUIBlocked) {
				t.Errorf("%s while the UI goroutine waits: err = %v, want errUIBlocked", m, err)
			}
		}
	}
	if err := g.Call("VFS.ReadDir", nil, nil); err != nil {
		t.Fatal(err)
	}
	if uiBlockedFor(&guardTestTransport{}) {
		t.Error("a bare transport reported as blocking the UI")
	}
}

// f4#1710: the permission prompt is a dialog only the UI goroutine can show, so
// asking it from the UI goroutine has to be refused at once. It used to post
// the dialog and wait for the answer for the whole prompt timeout (two
// minutes) while holding the goroutine that would have to show it.
func TestPermissionPromptRefusedOnTheUIGoroutine(t *testing.T) {
	old := uiGoroutine.Load()
	t.Cleanup(func() { uiGoroutine.Store(old) })

	result := make(chan bool, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		// The goroutine that counts as the UI one is this one; the test
		// goroutine only bounds the wait.
		uiGoroutine.Store(currentGoroutineID())
		result <- uiPermissionPrompt{}.Ask(PermissionRequest{Plugin: "android", Permission: PermissionNative})
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Ask waited for a dialog that the asking goroutine itself has to show")
	}
	if <-result {
		t.Fatal("a question that could not be shown was answered yes")
	}
}
