package luaplug

import (
	"errors"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

func TestWhileWaitingStopsTheCallDeadline(t *testing.T) {
	newRT := func() *Runtime {
		rt, err := New(Options{Name: "deadline", CallTimeout: 150 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = rt.Close() })
		return rt
	}

	rt := newRT()
	if err := rt.Do(func(*lua.LState) error {
		rt.WhileWaiting(func() { time.Sleep(500 * time.Millisecond) })
		return nil
	}); err != nil || rt.Interrupted() {
		t.Fatalf("waiting inside WhileWaiting: err=%v interrupted=%v", err, rt.Interrupted())
	}

	// The time the script itself takes still counts.
	rt = newRT()
	err := rt.Do(func(*lua.LState) error {
		time.Sleep(400 * time.Millisecond)
		return nil
	})
	if !errors.Is(err, ErrInterrupted) || !rt.Interrupted() {
		t.Fatalf("a call over its deadline: err=%v interrupted=%v", err, rt.Interrupted())
	}

	// Outside a call it just runs the function.
	ran := false
	newRT().WhileWaiting(func() { ran = true })
	if !ran {
		t.Fatal("WhileWaiting outside a call did not run its function")
	}
}
