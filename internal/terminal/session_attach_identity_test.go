//go:build !windows

package terminal

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/ttyx"
)

// The daemon finds the terminal window for whoever attached, so the client
// has to say who that is: its pid and every variable the window is identified
// by (issue #980). The identity rides after the existing payload, which an
// older daemon parses exactly as before.
func TestAttachClientIdentityRoundTrip(t *testing.T) {
	env := map[string]string{"DISPLAY": ":1", "WINDOWID": "4242"}
	wire := string(attachPayload("", "/home/u/a", "/home/u/b", nil)) +
		string(attachClientIdentity(777, func(k string) string { return env[k] }))

	edit, left, right, _ := parseAttachPayload(wire)
	if edit != "" || left != "/home/u/a" || right != "/home/u/b" {
		t.Fatalf("the directories no longer survive the identity lines: (%q, %q, %q)", edit, left, right)
	}

	pid, got := parseAttachClientIdentity(wire)
	if pid != 777 {
		t.Fatalf("pid = %d, want 777", pid)
	}
	for _, name := range ttyx.IdentityEnv() {
		v, sent := got[name]
		if !sent {
			t.Errorf("%s was not sent; an unset variable must arrive as empty, not be left to the daemon's own", name)
		}
		if v != env[name] {
			t.Errorf("%s = %q, want %q", name, v, env[name])
		}
	}
}

// With a file to edit the first line is "ATTACH <path>", and the identity
// must not end up inside the path.
func TestAttachClientIdentityAfterEditPath(t *testing.T) {
	wire := string(attachPayload("/tmp/file.txt", "", "", nil)) +
		string(attachClientIdentity(5, func(string) string { return "" }))
	if edit, _, _, _ := parseAttachPayload(wire); edit != "/tmp/file.txt" {
		t.Fatalf("edit path = %q", edit)
	}
	if pid, _ := parseAttachClientIdentity(wire); pid != 5 {
		t.Fatalf("pid = %d, want 5", pid)
	}
}

// An older client sends no identity; the daemon must see that as "nobody"
// and keep the session it already has.
func TestAttachClientIdentityAbsentFromOldClients(t *testing.T) {
	pid, env := parseAttachClientIdentity("ATTACH\nCWD /tmp")
	if pid != 0 || env != nil {
		t.Fatalf("got pid %d env %v from a payload with no identity", pid, env)
	}
}

// A value with a line break in it cannot be allowed to forge another line.
func TestAttachClientIdentityKeepsValuesOnOneLine(t *testing.T) {
	wire := string(attachClientIdentity(9, func(k string) string {
		if k == "DISPLAY" {
			return ":0\nPID 1"
		}
		return ""
	}))
	if strings.Count(wire, "\nPID ") != 1 {
		t.Fatalf("a newline in a value produced an extra line: %q", wire)
	}
	if pid, _ := parseAttachClientIdentity("ATTACH" + wire); pid != 9 {
		t.Fatalf("pid = %d, want 9", pid)
	}
}

// A client that started the daemon says so, after the identity, and the daemon
// must read it past the edit path and the other lines (issue #1974). Without
// the line -- an older client, or one joining a running session -- the client
// is a guest.
func TestAttachFreshRoundTrip(t *testing.T) {
	identity := string(attachClientIdentity(5, func(string) string { return "" }))
	fresh := string(attachPayload("/tmp/file.txt", "", "", nil)) + identity + attachFreshLine
	guest := string(attachPayload("/tmp/file.txt", "", "", nil)) + identity

	if !parseAttachFresh(fresh) {
		t.Error("the FRESH line was not read")
	}
	if parseAttachFresh(guest) {
		t.Error("a client that did not say it started the daemon was taken for its owner")
	}
	if parseAttachFresh("ATTACH /tmp/FRESH") {
		t.Error("a file named FRESH on the first line is not the marker")
	}
	if edit, _, _, _ := parseAttachPayload(fresh); edit != "/tmp/file.txt" {
		t.Errorf("edit path = %q, the marker must not change it", edit)
	}
	if pid, _ := parseAttachClientIdentity(fresh); pid != 5 {
		t.Errorf("pid = %d, the marker must not change it", pid)
	}
}

func TestClientOwnsSessionFollowsTheAttach(t *testing.T) {
	t.Cleanup(func() { setClientOwnsSession(true) })
	if !ClientOwnsSession() {
		t.Fatal("a process that is its own session must own it")
	}
	setClientOwnsSession(false)
	if ClientOwnsSession() {
		t.Fatal("a guest was taken for the owner")
	}
}
