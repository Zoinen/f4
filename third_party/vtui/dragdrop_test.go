package vtui

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDropActionHasAndString(t *testing.T) {
	set := DropCopy | DropMove
	if !set.Has(DropCopy) || !set.Has(DropMove) {
		t.Fatalf("set %s must contain copy and move", set)
	}
	if set.Has(DropLink) {
		t.Fatal("set must not contain link")
	}
	if set.Has(DropNone) {
		t.Fatal("nothing is not an action a set contains")
	}
	if got := set.String(); got != "copy|move" {
		t.Fatalf("String() = %q, want copy|move", got)
	}
	if got := DropNone.String(); got != "none" {
		t.Fatalf("DropNone.String() = %q, want none", got)
	}
}

func TestParseURIListSplitsLocalAndRemote(t *testing.T) {
	body := "#comment\r\n" +
		"file:///tmp/one%20two.txt\r\n" +
		"file://localhost/tmp/three.txt\r\n" +
		"\r\n" +
		"https://example.org/x\r\n" +
		"file://otherhost/tmp/four.txt\r\n"

	p := ParseURIList(body)
	wantPaths := []string{
		filepath.FromSlash("/tmp/one two.txt"),
		filepath.FromSlash("/tmp/three.txt"),
	}
	if !reflect.DeepEqual(p.Paths, wantPaths) {
		t.Fatalf("Paths = %v, want %v", p.Paths, wantPaths)
	}
	wantURIs := []string{"https://example.org/x", "file://otherhost/tmp/four.txt"}
	if !reflect.DeepEqual(p.URIs, wantURIs) {
		t.Fatalf("URIs = %v, want %v", p.URIs, wantURIs)
	}
	if !p.HasFiles() || p.IsEmpty() {
		t.Fatal("payload with files must report files")
	}
}

func TestURIToLocalPathDriveLetter(t *testing.T) {
	got, ok := URIToLocalPath("file:///C:/dir/file.txt")
	if !ok {
		t.Fatal("a drive letter URI names a local file")
	}
	if want := filepath.FromSlash("C:/dir/file.txt"); got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
	if _, ok := URIToLocalPath("http://example.org/x"); ok {
		t.Fatal("http is not a local file")
	}
}

func TestFormatURIListRoundTrip(t *testing.T) {
	paths := []string{
		filepath.FromSlash("/tmp/a b.txt"),
		filepath.FromSlash("/tmp/c#d.txt"),
	}
	list := FormatURIList(paths)
	if !strings.HasSuffix(list, "\r\n") {
		t.Fatalf("uri-list must be CRLF terminated: %q", list)
	}
	if strings.Contains(list, " ") {
		t.Fatalf("spaces must be escaped: %q", list)
	}
	back := ParseURIList(list)
	if !reflect.DeepEqual(back.Paths, paths) {
		t.Fatalf("round trip = %v, want %v", back.Paths, paths)
	}
}

func TestDeliverDragEventInline(t *testing.T) {
	prev := DragDeliverToUI
	DragDeliverToUI = false
	defer func() {
		DragDeliverToUI = prev
		SetDropTarget(nil)
	}()

	SetDropTarget(nil)
	if got := DeliverDragEvent(&DragEvent{Phase: DragEnter}); got != DropNone {
		t.Fatalf("no target must mean no action, got %s", got)
	}

	var seen DragPhase
	SetDropTarget(DropTargetFunc(func(ev *DragEvent) DropAction {
		seen = ev.Phase
		return DropMove
	}))
	if got := DeliverDragEvent(&DragEvent{Phase: DragDrop}); got != DropMove {
		t.Fatalf("action = %s, want move", got)
	}
	if seen != DragDrop {
		t.Fatalf("target saw %s, want drop", seen)
	}

	SetDropTarget(DropTargetFunc(func(ev *DragEvent) DropAction { panic("boom") }))
	if got := DeliverDragEvent(&DragEvent{Phase: DragOver}); got != DropNone {
		t.Fatalf("a panicking target must report no action, got %s", got)
	}
}

type fakeDragBackend struct {
	accepts bool
	payload DragPayload
	allowed DropAction
	result  DropAction
}

func (b *fakeDragBackend) AcceptsDrops() bool { return b.accepts }

func (b *fakeDragBackend) StartDrag(p DragPayload, a DropAction) (DropAction, error) {
	b.payload, b.allowed = p, a
	return b.result, nil
}

func TestStartDragWithoutBackend(t *testing.T) {
	SetDragBackend(nil)
	if _, err := StartDrag(DragPayload{}, DropCopy); !errors.Is(err, ErrDragUnsupported) {
		t.Fatalf("err = %v, want ErrDragUnsupported", err)
	}
	if DropSupported() || DragOutSupported() {
		t.Fatal("without a backend neither direction is supported")
	}
}

func TestStartDragUsesBackend(t *testing.T) {
	b := &fakeDragBackend{accepts: true, result: DropMove}
	SetDragBackend(b)
	defer SetDragBackend(nil)

	if !DropSupported() || !DragOutSupported() {
		t.Fatal("a registered backend supports both directions")
	}
	got, err := StartDrag(DragPayload{Paths: []string{"/tmp/x"}}, DropCopy|DropMove)
	if err != nil {
		t.Fatalf("StartDrag: %v", err)
	}
	if got != DropMove {
		t.Fatalf("action = %s, want move", got)
	}
	if len(b.payload.Paths) != 1 || b.allowed != DropCopy|DropMove {
		t.Fatalf("backend got payload %v allowed %s", b.payload.Paths, b.allowed)
	}
}

// memLogHas reports whether a line holding want reached the in-memory log
// ring, which is where DebugLog writes whether or not a file is open.
func memLogHas(want string) bool {
	for _, line := range getMemLogs() {
		if strings.Contains(line, want) {
			return true
		}
	}
	return false
}

func TestDeliverDragEventLogsAMissingTarget(t *testing.T) {
	SetDropTarget(nil)
	ev := &DragEvent{
		Phase:   DragDrop,
		X:       3,
		Y:       4,
		Allowed: DropCopy,
		Payload: DragPayload{Paths: []string{filepath.FromSlash("/tmp/a.txt")}},
	}
	if got := DeliverDragEvent(ev); got != DropNone {
		t.Fatalf("action = %s, want none without a target", got)
	}
	if !memLogHas("no drop target is installed") {
		t.Fatal("a payload with nowhere to go must say so in the log")
	}
}

func TestStartDragLogsAMissingBackend(t *testing.T) {
	SetDragBackend(nil)
	_, err := StartDrag(DragPayload{Paths: []string{"/tmp/x"}}, DropCopy)
	if !errors.Is(err, ErrDragUnsupported) {
		t.Fatalf("err = %v, want ErrDragUnsupported", err)
	}
	if !memLogHas("no backend registered") {
		t.Fatal("a drag out with no backend must say so in the log")
	}
}
