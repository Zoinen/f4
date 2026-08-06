package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

// fakeReconnectVFS is a file system that lives on a connection, built on
// NullVFS so that only the three questions this feature asks have to be
// answered here.
type fakeReconnectVFS struct {
	*vfs.NullVFS
	lost     bool
	can      bool
	attempts int
	failWith error
}

func newFakeReconnectVFS() *fakeReconnectVFS {
	return &fakeReconnectVFS{NullVFS: vfs.NewNullVFS(0), lost: true, can: true}
}

func (f *fakeReconnectVFS) SessionLost(err error) bool {
	return err != nil && f.lost
}
func (f *fakeReconnectVFS) CanReconnect() bool { return f.can }
func (f *fakeReconnectVFS) Reconnect(ctx context.Context) error {
	f.attempts++
	return f.failWith
}

// withReconnectStubs replaces the two hooks that need a screen and a frame
// manager, and hands back the last message the dialog was asked to show.
func withReconnectStubs(t *testing.T, answer int) *string {
	t.Helper()
	oldAsk, oldPost := reconnectAsk, reconnectPostUI
	t.Cleanup(func() { reconnectAsk, reconnectPostUI = oldAsk, oldPost })
	var shown string
	reconnectAsk = func(title, message string, buttons []string, onChoice func(int)) {
		shown = message
		onChoice(answer)
	}
	reconnectPostUI = func(f func()) { f() }
	return &shown
}

// waitChoice runs an offer and waits for the answer, since a reconnect happens
// off the calling goroutine even when the posting back is immediate.
func waitChoice(t *testing.T, ch chan reconnectChoice) reconnectChoice {
	t.Helper()
	select {
	case c := <-ch:
		return c
	case <-time.After(5 * time.Second):
		t.Fatal("the offer never answered")
		return reconnectOffline
	}
}

func TestReconnectorForOffersOnlyWhereItCan(t *testing.T) {
	boom := errors.New("session died")

	if r := reconnectorFor(vfs.NewNullVFS(0), boom); r != nil {
		t.Error("a file system with no session was offered a reconnect")
	}

	fs := newFakeReconnectVFS()
	if r := reconnectorFor(fs, nil); r != nil {
		t.Error("a reconnect was offered without an error")
	}
	if r := reconnectorFor(fs, boom); r == nil {
		t.Error("a lost session that can be rebuilt was not offered")
	}

	fs.lost = false
	if r := reconnectorFor(fs, boom); r != nil {
		t.Error("an error that is not a lost session was offered a reconnect")
	}

	fs.lost, fs.can = true, false
	if r := reconnectorFor(fs, boom); r != nil {
		t.Error("a session that cannot be rebuilt was offered a reconnect")
	}
}

func TestOfferReconnectSaysNothingWhenItCannotHelp(t *testing.T) {
	shown := withReconnectStubs(t, 0)
	if offerReconnect(vfs.NewNullVFS(0), errors.New("no such file"), "reading the directory", true, func(reconnectChoice, error) {
		t.Error("the caller was answered for an error it owns")
	}) {
		t.Error("the offer claimed an error it cannot help with")
	}
	if *shown != "" {
		t.Error("a dialog was shown for a file system with no session")
	}
}

func TestOfferReconnectRetriesAReadableOperation(t *testing.T) {
	withReconnectStubs(t, 0)
	fs := newFakeReconnectVFS()
	ch := make(chan reconnectChoice, 1)

	if !offerReconnect(fs, errors.New("session died"), "reading the directory", true, func(c reconnectChoice, err error) {
		if err != nil {
			t.Errorf("a reconnect that worked reported %v", err)
		}
		ch <- c
	}) {
		t.Fatal("the offer was not taken")
	}
	if got := waitChoice(t, ch); got != reconnectRetry {
		t.Fatalf("choice %v, want reconnectRetry", got)
	}
	if fs.attempts != 1 {
		t.Fatalf("%d reconnect attempts, want 1", fs.attempts)
	}
}

func TestOfferReconnectDoesNotRepeatAWrite(t *testing.T) {
	shown := withReconnectStubs(t, 0)
	fs := newFakeReconnectVFS()
	ch := make(chan reconnectChoice, 1)

	offerReconnect(fs, errors.New("session died"), "saving the file", false, func(c reconnectChoice, err error) {
		ch <- c
	})
	if got := waitChoice(t, ch); got != reconnectOffline {
		t.Fatalf("choice %v, want reconnectOffline: a half written file must not be repeated", got)
	}
	if fs.attempts != 1 {
		t.Fatalf("%d reconnect attempts, want 1: the panel is still worth having back", fs.attempts)
	}
	if !strings.Contains(*shown, "cannot") {
		t.Errorf("the dialog did not say the operation cannot be resumed: %q", *shown)
	}
}

func TestOfferReconnectReportsAFailedAttempt(t *testing.T) {
	withReconnectStubs(t, 0)
	fs := newFakeReconnectVFS()
	fs.failWith = errors.New("host still unreachable")
	ch := make(chan reconnectChoice, 1)

	offerReconnect(fs, errors.New("session died"), "reading the directory", true, func(c reconnectChoice, err error) {
		if err == nil || !strings.Contains(err.Error(), "unreachable") {
			t.Errorf("the failure was reported as %v", err)
		}
		ch <- c
	})
	if got := waitChoice(t, ch); got != reconnectOffline {
		t.Fatalf("choice %v, want reconnectOffline", got)
	}
}

func TestOfferReconnectHonoursTheOtherAnswers(t *testing.T) {
	for _, tc := range []struct {
		name   string
		answer int
		want   reconnectChoice
	}{
		{"work offline", 1, reconnectOffline},
		{"close the panel", 2, reconnectLeave},
		{"dismissed", -1, reconnectOffline},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withReconnectStubs(t, tc.answer)
			fs := newFakeReconnectVFS()
			ch := make(chan reconnectChoice, 1)
			offerReconnect(fs, errors.New("session died"), "reading the directory", true, func(c reconnectChoice, err error) {
				ch <- c
			})
			if got := waitChoice(t, ch); got != tc.want {
				t.Fatalf("choice %v, want %v", got, tc.want)
			}
			if fs.attempts != 0 {
				t.Fatalf("%d reconnect attempts, want none", fs.attempts)
			}
		})
	}
}
