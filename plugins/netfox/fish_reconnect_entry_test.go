package netfox

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestFishReconnectRepointsTheView is what separates this from the connection
// level swap: the panel that asked for the reconnect must be talking to the
// session it just built, not to the one that died.
func TestFishReconnectRepointsTheView(t *testing.T) {
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

	was := v.GetPath()
	dead := v.Client()
	dead.Session().MarkBroken()

	if err := v.Reconnect(context.Background()); err != nil {
		t.Fatalf("reconnect: %v", err)
	}
	if v.Client() == dead {
		t.Fatal("the view still points at the session that died")
	}
	if v.Client().Session().Broken() {
		t.Fatal("the view points at a broken session")
	}
	if v.GetPath() != was {
		t.Fatalf("the panel moved from %q to %q", was, v.GetPath())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := v.client().Pwd(ctx); err != nil {
		t.Fatalf("the reconnected view cannot make a request: %v", err)
	}
}

// TestFishCanReconnectFollowsTheDialer keeps the offer honest: a site that
// cannot be rebuilt must not be offered a reconnect.
func TestFishCanReconnectFollowsTheDialer(t *testing.T) {
	dial := localShellDialer(t)
	withDialer, err := NewFishVFSOnDialer(context.Background(), nil, dial, "local")
	if err != nil {
		if strings.Contains(err.Error(), "base64") {
			t.Skipf("no base64 on this host: %v", err)
		}
		t.Fatalf("open: %v", err)
	}
	if !withDialer.CanReconnect() {
		t.Error("a site opened through a dialer says it cannot reconnect")
	}
	if err := withDialer.Close(); err != nil {
		t.Fatalf("close reconnectable filesystem: %v", err)
	}
	if withDialer.CanReconnect() {
		t.Error("a closed connection still offers a reconnect")
	}

	onStreams := newLocalFishVFS(t)
	if onStreams.CanReconnect() {
		t.Error("a session handed a pair of streams claims it can reconnect")
	}
	if err := onStreams.Reconnect(context.Background()); !errors.Is(err, ErrNoDialer) {
		t.Fatalf("reconnect answered %v, want ErrNoDialer", err)
	}
}

// TestFishReconnectTwiceSharesOneSession: two views of one connection both
// meeting a dead session must end up on the same replacement.
func TestFishReconnectTwiceSharesOneSession(t *testing.T) {
	dial := localShellDialer(t)
	first, err := NewFishVFSOnDialer(context.Background(), nil, dial, "local")
	if err != nil {
		if strings.Contains(err.Error(), "base64") {
			t.Skipf("no base64 on this host: %v", err)
		}
		t.Fatalf("open: %v", err)
	}
	defer func() {
		if err := first.Close(); err != nil {
			t.Errorf("close FISH+ filesystem: %v", err)
		}
	}()

	second, ok := first.Clone().(*FishVFS)
	if !ok {
		t.Fatal("Clone did not hand back a FishVFS")
	}
	defer func() {
		if err := second.Close(); err != nil {
			t.Errorf("close FISH+ filesystem: %v", err)
		}
	}()

	first.Client().Session().MarkBroken()

	if err := first.Reconnect(context.Background()); err != nil {
		t.Fatalf("first reconnect: %v", err)
	}
	if err := second.Reconnect(context.Background()); err != nil {
		t.Fatalf("second reconnect: %v", err)
	}
	if first.Client() != second.Client() {
		t.Fatal("two views of one connection ended up on different sessions")
	}
}
