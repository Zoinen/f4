//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package vtui

import (
	"reflect"
	"testing"

	"github.com/jezek/xgb/xproto"
)

// testDnd builds an instance with made up atoms, so the parts that only do
// protocol arithmetic can be tested without an X server.
func testDnd() *x11Dnd {
	return &x11Dnd{a: x11DndAtoms{
		uriList:    101,
		plainUTF8:  102,
		plain:      103,
		utf8:       104,
		actCopy:    201,
		actMove:    202,
		actLink:    203,
		actAsk:     204,
		actPrivate: 205,
		incr:       301,
	}}
}

func TestXdndPickTypePrefersFiles(t *testing.T) {
	d := testDnd()
	got := d.pickType([]xproto.Atom{103, 101, 999})
	if got != d.a.uriList {
		t.Fatalf("type = %d, want the uri list %d", got, d.a.uriList)
	}
	if got := d.pickType([]xproto.Atom{103, 102}); got != d.a.plainUTF8 {
		t.Fatalf("type = %d, want utf-8 plain text %d", got, d.a.plainUTF8)
	}
	if got := d.pickType([]xproto.Atom{777}); got != 0 {
		t.Fatalf("type = %d, want nothing for an unknown offer", got)
	}
}

func TestXdndEnterTypesFromMessage(t *testing.T) {
	d := testDnd()
	data := []uint32{42, uint32(5) << 24, 101, 0, 103}
	got := d.enterTypes(data)
	want := []xproto.Atom{101, 103}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("types = %v, want %v", got, want)
	}
	if got := d.enterTypes([]uint32{1, 2}); got != nil {
		t.Fatalf("a short message carries no types, got %v", got)
	}
}

func TestXdndActionMapping(t *testing.T) {
	d := testDnd()
	cases := []struct {
		atom xproto.Atom
		want DropAction
	}{
		{d.a.actCopy, DropCopy},
		{d.a.actMove, DropMove},
		{d.a.actLink, DropLink},
		{d.a.actAsk, DropCopy},
		{d.a.actPrivate, DropCopy},
		{0, DropNone},
		{999, DropNone},
	}
	for _, c := range cases {
		if got := d.actionOf(c.atom); got != c.want {
			t.Fatalf("actionOf(%d) = %s, want %s", c.atom, got, c.want)
		}
	}

	if got := d.atomOf(DropMove); got != d.a.actMove {
		t.Fatalf("atomOf(move) = %d, want %d", got, d.a.actMove)
	}
	if got := d.atomOf(DropCopy | DropMove); got != d.a.actMove {
		t.Fatalf("a set with move must report move, got %d", got)
	}
	if got := d.atomOf(DropNone); got != 0 {
		t.Fatalf("atomOf(none) = %d, want 0", got)
	}
}

func TestXdndKinds(t *testing.T) {
	d := testDnd()
	d.chosen = d.a.uriList
	if got := d.kinds(); !reflect.DeepEqual(got, []string{"text/uri-list"}) {
		t.Fatalf("kinds = %v", got)
	}
	d.chosen = d.a.utf8
	if got := d.kinds(); !reflect.DeepEqual(got, []string{"text/plain;charset=utf-8"}) {
		t.Fatalf("kinds = %v", got)
	}
	d.chosen = 0
	if got := d.kinds(); got != nil {
		t.Fatalf("kinds = %v, want none", got)
	}
}

func TestDndCell(t *testing.T) {
	if got := dndCell(25, 10); got != 2 {
		t.Fatalf("cell = %d, want 2", got)
	}
	if got := dndCell(25, 0); got != 0 {
		t.Fatalf("a zero cell size must not divide, got %d", got)
	}
	if got := dndCell(-5, 10); got != 0 {
		t.Fatalf("cell = %d, want 0", got)
	}
}

func TestPayloadOffersFiles(t *testing.T) {
	if (DragPayload{Paths: []string{"/tmp/x"}}).IsEmpty() {
		t.Fatal("a payload with paths is not empty")
	}
	if (DragPayload{}).OffersFiles() {
		t.Fatal("an empty payload offers nothing")
	}
	if !(DragPayload{Kinds: []string{"TEXT/URI-LIST"}}).OffersFiles() {
		t.Fatal("an announced uri list counts as an offer of files")
	}
	if !(DragPayload{Paths: []string{"/tmp/x"}}).OffersFiles() {
		t.Fatal("actual paths count too")
	}
	if (DragPayload{Kinds: []string{"text/plain"}, Text: "hi"}).OffersFiles() {
		t.Fatal("plain text is not a file offer")
	}
}

func TestXdndStatusAction(t *testing.T) {
	d := testDnd()
	if got := d.statusAction([]uint32{0, 0, 0, 0, uint32(d.a.actCopy)}); got != DropNone {
		t.Fatalf("a refusal is a refusal even with an action, got %s", got)
	}
	if got := d.statusAction([]uint32{0, 1, 0, 0, uint32(d.a.actMove)}); got != DropMove {
		t.Fatalf("action = %s, want move", got)
	}
	if got := d.statusAction([]uint32{0, 1, 0, 0, 0}); got != DropCopy {
		t.Fatalf("an acceptance without an action means copy, got %s", got)
	}
	if got := d.statusAction([]uint32{0, 1}); got != DropNone {
		t.Fatalf("a short message says nothing, got %s", got)
	}
}

func TestXdndSourceTargets(t *testing.T) {
	d := testDnd()
	want := []xproto.Atom{d.a.targets, d.a.uriList}
	if got := d.sourceTargets(); !reflect.DeepEqual(got, want) {
		t.Fatalf("targets = %v, want %v", got, want)
	}
}

func TestStartDragRefusesEmptyPayload(t *testing.T) {
	d := testDnd()
	if _, err := d.startDrag(DragPayload{}, DropCopy); err != ErrDragNoData {
		t.Fatalf("err = %v, want ErrDragNoData", err)
	}
	if d.draggingOut() {
		t.Fatal("a refused drag must leave no state behind")
	}
}

func TestHostStartDragWithoutDnd(t *testing.T) {
	h := &X11Host{}
	if h.AcceptsDrops() || h.CanStartDrag() {
		t.Fatal("a host without a dnd instance supports neither direction")
	}
	if _, err := h.StartDrag(DragPayload{Paths: []string{"/tmp/x"}}, DropCopy); err != ErrDragUnsupported {
		t.Fatalf("err = %v, want ErrDragUnsupported", err)
	}
}
func TestFindTargetNeedsNoHost(t *testing.T) {
	d := &x11Dnd{}
	if w, v := d.findTarget(10, 10); w != 0 || v != 0 {
		t.Fatalf("target = %d version %d, want nothing without a root", w, v)
	}
}

func TestNewX11DndFillsTheSourceWindow(t *testing.T) {
	if d := newX11Dnd(nil); d != nil {
		t.Fatal("without a host there is no window to be a source on")
	}
}

func TestX11DragSourceNeedsConnAndWindow(t *testing.T) {
	if s := NewX11DragSource(nil, 1, 2, nil); s != nil {
		t.Fatal("without a connection there is nothing to drag from")
	}
}

func TestX11DragSourceWithoutState(t *testing.T) {
	var s *X11DragSource
	if _, err := s.StartDrag(DragPayload{Paths: []string{"/tmp/x"}}, DropCopy); err != ErrDragUnsupported {
		t.Fatalf("err = %v, want ErrDragUnsupported", err)
	}
	if s.DraggingOut() || s.HandleClientMessage(&xproto.ClientMessageEvent{Format: 32}) ||
		s.HandleSelectionRequest(&xproto.SelectionRequestEvent{}) {
		t.Fatal("a nil source must ignore everything")
	}
}

func TestX11DragSourceIgnoresForeignMessages(t *testing.T) {
	s := &X11DragSource{d: testDnd()}
	s.d.a.status, s.d.a.finished, s.d.a.enter = 401, 402, 403
	if s.HandleClientMessage(&xproto.ClientMessageEvent{Format: 32, Type: 403}) {
		t.Fatal("XdndEnter belongs to the receiving half, which this source does not run")
	}
	if !s.HandleClientMessage(&xproto.ClientMessageEvent{Format: 32, Type: 401}) {
		t.Fatal("XdndStatus must be consumed even when no drag is running")
	}
	if _, err := s.StartDrag(DragPayload{}, DropCopy); err != ErrDragNoData {
		t.Fatalf("err = %v, want ErrDragNoData", err)
	}
}

func TestX11DragSourceBorrowedPointerLeavesGrabAlone(t *testing.T) {
	var none *X11DragSource
	none.BorrowPointer()

	s := &X11DragSource{d: &x11Dnd{}}
	s.BorrowPointer()
	// With no connection either call would panic if it tried to talk to the
	// server, which a borrowed pointer must never do.
	if err := s.d.grabSourcePointer(); err != nil {
		t.Fatalf("grab of a borrowed pointer: %v", err)
	}
	s.d.ungrabSourcePointer(0)
}

func TestX11DragSourceSkipTargetsWithoutState(t *testing.T) {
	var none *X11DragSource
	none.SkipTargets(func(xproto.Window) bool { return true })

	s := &X11DragSource{d: &x11Dnd{}}
	s.SkipTargets(func(w xproto.Window) bool { return w == 7 })
	if s.d.srcSkip == nil || !s.d.srcSkip(7) || s.d.srcSkip(8) {
		t.Fatal("the predicate was not stored as given")
	}
	s.SkipTargets(nil)
	if s.d.srcSkip != nil {
		t.Fatal("nil must clear the predicate")
	}
}
