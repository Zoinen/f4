package main

import (
	"errors"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

// fakeSessionVFS is a reconnectable file system that also shares a connection,
// which is what makes its jobs findable when that connection dies.
type fakeSessionVFS struct {
	*fakeReconnectVFS
	conn *int
}

func newFakeSessionVFS() *fakeSessionVFS {
	conn := 0
	return &fakeSessionVFS{fakeReconnectVFS: newFakeReconnectVFS(), conn: &conn}
}

func (f *fakeSessionVFS) SessionKey() any { return f.conn }

func TestSessionLostEndsTheJobsThatRanOnIt(t *testing.T) {
	r := NewBackgroundJobRegistry()
	here := r.Start("local work", nil)
	otherConn := 0
	cancelled := false
	mine := r.StartOn(&otherConn, "elsewhere", func() { cancelled = true })

	if got := r.SessionLost(&otherConn); got != 1 {
		t.Fatalf("%d jobs lost, want 1", got)
	}
	states := map[int]BackgroundJobState{}
	for _, s := range r.List() {
		states[s.ID] = s
	}
	if s := states[mine.ID()]; !s.Done || !strings.Contains(s.Status, "lost") {
		t.Fatalf("the job on the dead connection reads %+v", s)
	}
	if s := states[here.ID()]; s.Done {
		t.Fatal("work running here was ended by a connection it never used")
	}
	if r.Cancel(mine.ID()) {
		t.Error("a job that is already gone was offered a cancel")
	}
	if cancelled {
		t.Error("a job that is already gone was cancelled on the far side")
	}
	if !r.Open(mine.ID()) {
		t.Error("the news could not be dismissed from the list")
	}
}

func TestSessionLostLeavesAFinishedResultAlone(t *testing.T) {
	r := NewBackgroundJobRegistry()
	conn := 0
	job := r.StartOn(&conn, "elsewhere", nil)
	job.FinishWith("42 duplicates", func() {})

	if got := r.SessionLost(&conn); got != 0 {
		t.Fatalf("%d jobs lost, want 0: an answer already here does not die with the session", got)
	}
	list := r.List()
	if len(list) != 1 || list[0].Result != "42 duplicates" {
		t.Fatalf("the finished result was disturbed: %+v", list)
	}
}

func TestSessionLostIgnoresWorkWithNoConnection(t *testing.T) {
	r := NewBackgroundJobRegistry()
	r.Start("local work", nil)
	if got := r.SessionLost(nil); got != 0 {
		t.Fatalf("%d jobs lost for a nil owner, want 0", got)
	}
	if r.List()[0].Done {
		t.Error("local work was ended by a session that does not exist")
	}
}

func TestSessionKeyOfSharesOneKeyPerConnection(t *testing.T) {
	fs := newFakeSessionVFS()
	if sessionKeyOf(fs) != fs.SessionKey() {
		t.Error("a file system with a connection did not answer with it")
	}
	if sessionKeyOf(vfs.NewNullVFS(0)) != nil {
		t.Error("a file system with no connection claimed one")
	}
	if sessionKeyOf(nil) != nil {
		t.Error("nothing claimed a connection")
	}
}

func TestOfferReconnectTellsTheJobsTheyAreGone(t *testing.T) {
	shown := withReconnectStubs(t, 1)

	oldRegistry := GlobalBackgroundJobs
	GlobalBackgroundJobs = NewBackgroundJobRegistry()
	t.Cleanup(func() { GlobalBackgroundJobs = oldRegistry })

	fs := newFakeSessionVFS()
	job := GlobalBackgroundJobs.StartOn(fs.SessionKey(), "Duplicates in /var", func() {})

	ch := make(chan reconnectChoice, 1)
	offerReconnect(fs, errors.New("session died"), "reading the directory", true, func(c reconnectChoice, err error) {
		ch <- c
	})
	if got := waitChoice(t, ch); got != reconnectOffline {
		t.Fatalf("choice %v, want reconnectOffline", got)
	}

	// Working offline is the answer that changes the least, and even it must
	// not leave a job waiting for an answer the far side cannot send.
	for _, s := range GlobalBackgroundJobs.List() {
		if s.ID == job.ID() && !s.Done {
			t.Fatal("the job survived the session it was running on")
		}
	}
	if !strings.Contains(*shown, "background job") {
		t.Errorf("the dialog did not mention the job that was lost: %q", *shown)
	}
}

// TestReconnectStillWorksWithoutASession keeps the job bookkeeping from
// getting in the way of a file system that has no connection to lose.
func TestReconnectStillWorksWithoutASession(t *testing.T) {
	withReconnectStubs(t, 0)
	fs := newFakeReconnectVFS()
	ch := make(chan reconnectChoice, 1)
	offerReconnect(fs, errors.New("session died"), "reading the directory", true, func(c reconnectChoice, err error) {
		ch <- c
	})
	if got := waitChoice(t, ch); got != reconnectRetry {
		t.Fatalf("choice %v, want reconnectRetry", got)
	}
}
