package main

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestGroupDropSources(t *testing.T) {
	paths := []string{
		filepath.FromSlash("/tmp/one/b.txt"),
		filepath.FromSlash("/tmp/one/a.txt"),
		filepath.FromSlash("/tmp/one/a.txt"),
		filepath.FromSlash("/tmp/two/dir/"),
		"   ",
	}
	got := groupDropSources(paths)
	want := []dropSourceGroup{
		{dir: filepath.FromSlash("/tmp/one"), names: []string{"a.txt", "b.txt"}},
		{dir: filepath.FromSlash("/tmp/two"), names: []string{"dir"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("groups = %#v, want %#v", got, want)
	}
	if len(groupDropSources(nil)) != 0 {
		t.Fatal("nothing dropped means nothing to do")
	}
}

func TestChooseDropAction(t *testing.T) {
	both := vtui.DropCopy | vtui.DropMove
	cases := []struct {
		name               string
		allowed, suggested vtui.DropAction
		mods               vtinput.ControlKeyState
		want               vtui.DropAction
	}{
		{"shift moves", both, vtui.DropCopy, vtinput.ShiftPressed, vtui.DropMove},
		{"ctrl copies", both, vtui.DropMove, vtinput.LeftCtrlPressed, vtui.DropCopy},
		{"right ctrl copies too", both, vtui.DropMove, vtinput.RightCtrlPressed, vtui.DropCopy},
		{"suggestion wins when free", both, vtui.DropMove, 0, vtui.DropMove},
		{"copy is the fallback", both, vtui.DropNone, 0, vtui.DropCopy},
		{"move only source", vtui.DropMove, vtui.DropNone, 0, vtui.DropMove},
		{"shift cannot invent move", vtui.DropCopy, vtui.DropNone, vtinput.ShiftPressed, vtui.DropCopy},
		{"nothing allowed", vtui.DropNone, vtui.DropCopy, 0, vtui.DropNone},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := chooseDropAction(c.allowed, c.suggested, c.mods); got != c.want {
				t.Fatalf("action = %s, want %s", got, c.want)
			}
		})
	}
}

type readOnlyTestVFS struct {
	vfs.VFS
}

func (readOnlyTestVFS) IsReadOnly() bool { return true }

func TestVFSAcceptsDrop(t *testing.T) {
	if vfsAcceptsDrop(nil) {
		t.Fatal("no file system accepts nothing")
	}
	local := vfs.NewOSVFS(t.TempDir())
	if !vfsAcceptsDrop(local) {
		t.Fatal("a writable file system accepts a drop")
	}
	if vfsAcceptsDrop(readOnlyTestVFS{local}) {
		t.Fatal("a read-only file system must refuse before the drop")
	}
}

func TestHandleDragWithoutTargetPanel(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	defer vtui.SetDropTarget(nil)

	ev := &vtui.DragEvent{
		Phase:   vtui.DragEnter,
		X:       5,
		Y:       5,
		Allowed: vtui.DropCopy,
		Payload: vtui.DragPayload{Paths: []string{filepath.FromSlash("/tmp/a.txt")}},
	}
	if got := pf.HandleDrag(ev); got != vtui.DropNone {
		t.Fatalf("action = %s, want none over no panel", got)
	}

	ev.Payload = vtui.DragPayload{Text: "hello"}
	if got := pf.HandleDrag(ev); got != vtui.DropNone {
		t.Fatalf("action = %s, want none for a payload without files", got)
	}

	ev.Phase = vtui.DragLeave
	if got := pf.HandleDrag(ev); got != vtui.DropNone {
		t.Fatalf("action = %s, want none on leave", got)
	}
}
func TestLocalDragPaths(t *testing.T) {
	dir := t.TempDir()
	fsp := &FileSystemPanel{vfs: vfs.NewOSVFS(dir)}
	paths, ok := localDragPaths(fsp, []string{"a.txt", "b.txt"})
	if !ok {
		t.Fatal("a local panel can be dragged out of")
	}
	want := []string{filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}

	remote := &FileSystemPanel{vfs: vfs.NewNullVFS(0)}
	if _, ok := localDragPaths(remote, []string{"a.txt"}); ok {
		t.Fatal("a panel without real paths must refuse the drag")
	}
}

func TestDragOutGestureIgnoresPlainPress(t *testing.T) {
	pf := NewPanelsFrame()
	defer pf.Close()
	defer vtui.SetDropTarget(nil)

	press := &vtinput.InputEvent{
		Type:        vtinput.MouseEventType,
		KeyDown:     true,
		ButtonState: vtinput.FromLeft1stButtonPressed,
		MouseX:      5,
		MouseY:      5,
	}
	if pf.processDragOutGesture(press, 5, 5) {
		t.Fatal("a press must never be swallowed")
	}
	if pf.dragOut.armed {
		t.Fatal("a press outside a marked file must not arm the gesture")
	}

	move := &vtinput.InputEvent{
		Type:            vtinput.MouseEventType,
		ButtonState:     vtinput.FromLeft1stButtonPressed,
		MouseEventFlags: vtinput.MouseMoved,
		MouseX:          7,
		MouseY:          9,
	}
	if pf.processDragOutGesture(move, 7, 9) {
		t.Fatal("an unarmed drag belongs to the panel")
	}
}

// dragBackendStub is a backend that supports both directions and does
// nothing, which is all dragOutRefusal asks of one.
type dragBackendStub struct{}

func (dragBackendStub) AcceptsDrops() bool { return true }

func (dragBackendStub) StartDrag(vtui.DragPayload, vtui.DropAction) (vtui.DropAction, error) {
	return vtui.DropCopy, nil
}

func TestDragOutRefusal(t *testing.T) {
	vtui.SetDragBackend(nil)
	if got := dragOutRefusal(nil); got != "no panel under the pointer" {
		t.Fatalf("reason = %q, want the missing panel", got)
	}

	fsp := &FileSystemPanel{vfs: vfs.NewOSVFS(t.TempDir())}
	if got := dragOutRefusal(fsp); got != "the backend offers no drag source" {
		t.Fatalf("reason = %q, want the missing backend", got)
	}

	vtui.SetDragBackend(dragBackendStub{})
	defer vtui.SetDragBackend(nil)
	if got := dragOutRefusal(fsp); got != "nothing is marked" {
		t.Fatalf("reason = %q, want the empty selection", got)
	}
}
