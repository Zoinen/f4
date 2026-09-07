package netfox

import (
	"context"
	"strings"
	"testing"
)

// TestFishCloneStartsOnTheLiveSession: f4 clones a panel's file system in
// several places, and a clone made after some other view reconnected must not
// be handed the session that view had before.
func TestFishCloneStartsOnTheLiveSession(t *testing.T) {
	dial := localShellDialer(t)
	v, err := NewFishVFSOnDialer(context.Background(), nil, dial, "local")
	if err != nil {
		if strings.Contains(err.Error(), "base64") {
			t.Skipf("no base64 on this host: %v", err)
		}
		t.Fatalf("open: %v", err)
	}
	defer func() {
		if err := v.Close(); err != nil {
			t.Errorf("close FISH+ filesystem: %v", err)
		}
	}()

	// One view reconnects; every other view of the connection follows, which
	// is what the clone then has to be born on too.
	reconnecting, ok := v.Clone().(*FishVFS)
	if !ok {
		t.Fatal("Clone did not hand back a FishVFS")
	}
	defer func() {
		if err := reconnecting.Close(); err != nil {
			t.Errorf("close FISH+ filesystem: %v", err)
		}
	}()

	dead := v.Client()
	dead.Session().MarkBroken()
	if err := reconnecting.Reconnect(context.Background()); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if v.Client() == dead {
		t.Fatal("the view that did not ask for the reconnect kept the dead session")
	}

	fresh, ok := v.Clone().(*FishVFS)
	if !ok {
		t.Fatal("Clone did not hand back a FishVFS")
	}
	defer func() {
		if err := fresh.Close(); err != nil {
			t.Errorf("close FISH+ filesystem: %v", err)
		}
	}()

	if fresh.Client() == dead {
		t.Fatal("the clone was born holding the session that had died")
	}
	if fresh.Client() != reconnecting.Client() {
		t.Fatal("the clone landed on a session of its own")
	}
	if fresh.Client().Session().Broken() {
		t.Fatal("the clone points at a broken session")
	}
}
