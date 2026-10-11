package netfox

import (
	"context"
	"path/filepath"
	"testing"
)

// TestSecondHopPasswordOffByDefault is the security-relevant regression test
// for f4#370: a saved connection's password must never be handed to a
// server-to-server transfer's second hop unless the opt-in "password auth
// for server-to-server transfers" setting has actually been turned on. A
// missing behavior file -- the state of every install that never opened
// this setting -- must behave exactly like an explicit "off".
func TestSecondHopPasswordOffByDefault(t *testing.T) {
	dir := t.TempDir()
	behaviorPath := filepath.Join(dir, "NetFoxBehavior.json")
	connectionsPath := filepath.Join(dir, "NetFox.json")

	store := NewNetFoxVFS(connectionsPath)
	if err := store.SaveConfig("target", NetFoxConfig{Type: "fish+", Host: "hostb", Port: "22", User: "userb", Pass: "hunter2"}); err != nil {
		t.Fatal(err)
	}

	// No behavior file at all: the default, off state.
	if pass, ok := secondHopPasswordAt(behaviorPath, connectionsPath, "hostb", "22", "userb"); ok {
		t.Fatalf("expected no password with the setting left at its default, got %q", pass)
	}

	// Explicitly off is the same as absent.
	if err := writeS2SBehaviorSettingsAt(behaviorPath, s2sBehaviorSettings{AllowServerToServerPasswordAuth: false}); err != nil {
		t.Fatal(err)
	}
	if pass, ok := secondHopPasswordAt(behaviorPath, connectionsPath, "hostb", "22", "userb"); ok {
		t.Fatalf("expected no password with the setting explicitly off, got %q", pass)
	}
}

// TestSecondHopPasswordWhenEnabled is the companion positive case: once the
// setting is on, a matching saved fish+ connection's password is offered
// back, using the default port (22) when either side omits it, and is
// refused when host, port or user does not match, or the type is not fish+.
func TestSecondHopPasswordWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	behaviorPath := filepath.Join(dir, "NetFoxBehavior.json")
	connectionsPath := filepath.Join(dir, "NetFox.json")

	if err := writeS2SBehaviorSettingsAt(behaviorPath, s2sBehaviorSettings{AllowServerToServerPasswordAuth: true}); err != nil {
		t.Fatal(err)
	}

	store := NewNetFoxVFS(connectionsPath)
	if err := store.SaveConfig("target", NetFoxConfig{Type: "fish+", Host: "HostB", User: "userb", Pass: "hunter2"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveConfig("ftp-sibling", NetFoxConfig{Type: "ftp", Host: "hostb", Port: "22", User: "userb", Pass: "wrong-kind"}); err != nil {
		t.Fatal(err)
	}

	if pass, ok := secondHopPasswordAt(behaviorPath, connectionsPath, "hostb", "", "userb"); !ok || pass != "hunter2" {
		t.Fatalf("secondHopPasswordAt = %q, %v, want %q, true", pass, ok, "hunter2")
	}
	if pass, ok := secondHopPasswordAt(behaviorPath, connectionsPath, "hostb", "22", "userb"); !ok || pass != "hunter2" {
		t.Fatalf("secondHopPasswordAt with explicit default port = %q, %v, want %q, true", pass, ok, "hunter2")
	}
	if _, ok := secondHopPasswordAt(behaviorPath, connectionsPath, "hostb", "2222", "userb"); ok {
		t.Fatal("expected no match for a different port")
	}
	if _, ok := secondHopPasswordAt(behaviorPath, connectionsPath, "hostb", "", "someoneelse"); ok {
		t.Fatal("expected no match for a different user")
	}
	if _, ok := secondHopPasswordAt(behaviorPath, connectionsPath, "otherhost", "", "userb"); ok {
		t.Fatal("expected no match for a different host")
	}
}

// TestFishVFSStageSecret exercises the real staging mechanism -- mktemp,
// then the same encrypted FISH+ write channel every ordinary file transfer
// uses, then cleanup -- against a real local POSIX shell (localShellDialer),
// the same fixture the rest of this package's FISH+ tests use in place of a
// live SSH server.
func TestFishVFSStageSecret(t *testing.T) {
	dial := localShellDialer(t)
	v, err := NewFishVFSOnDialer(context.Background(), nil, dial, "local")
	if err != nil {
		t.Skipf("no local FISH+ helper available: %v", err)
	}
	defer func() {
		if err := v.Close(); err != nil {
			t.Errorf("close FISH+ filesystem: %v", err)
		}
	}()

	ctx := context.Background()
	ref, cleanup, err := v.StageSecret(ctx, "hunter2")
	if err != nil {
		t.Fatalf("StageSecret: %v", err)
	}
	if ref == "" {
		t.Fatal("StageSecret returned an empty reference")
	}

	lines, code, err := v.Client().RunOutput(ctx, "", "cat "+posixSingleQuote(ref))
	if err != nil || code != 0 {
		t.Fatalf("reading the staged secret back: lines=%v code=%d err=%v", lines, code, err)
	}
	if got := lines[0]; got != "hunter2" {
		t.Fatalf("staged secret content = %q, want %q", got, "hunter2")
	}

	cleanup(ctx)

	_, code, err = v.Client().RunOutput(ctx, "", "test -e "+posixSingleQuote(ref))
	if err == nil && code == 0 {
		t.Fatal("expected the staged secret file to be gone after cleanup")
	}
}
