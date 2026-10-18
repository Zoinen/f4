//go:build (linux || openbsd || netbsd || dragonfly || darwin || freebsd || windows || illumos || solaris) && !android

package winex11drag

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jezek/xgb/xproto"

	"github.com/unxed/vtui"
)

// scriptedPointer returns its samples in order and then repeats the last one.
type scriptedPointer struct {
	mu      sync.Mutex
	samples []pointerSample
}

type pointerSample struct {
	x, y int
	down bool
}

func (p *scriptedPointer) Position() (int, int, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := p.samples[0]
	if len(p.samples) > 1 {
		p.samples = p.samples[1:]
	}
	return s.x, s.y, s.down
}

// fakeSession records what Backend sends and lets Drag return after the
// second release, which is what a lost first release looks like to Session.
type fakeSession struct {
	mu       sync.Mutex
	motions  [][2]int
	releases int
	closed   bool
	enough   chan struct{}
}

func newFakeSession() *fakeSession { return &fakeSession{enough: make(chan struct{})} }

func (s *fakeSession) Drag(vtui.DragPayload, vtui.DropAction) (vtui.DropAction, error) {
	select {
	case <-s.enough:
		return vtui.DropCopy, nil
	case <-time.After(5 * time.Second):
		return vtui.DropNone, errors.New("drag never saw the release")
	}
}

func (s *fakeSession) Motion(x, y int, _ xproto.Timestamp) {
	s.mu.Lock()
	s.motions = append(s.motions, [2]int{x, y})
	s.mu.Unlock()
}

func (s *fakeSession) Release(xproto.Timestamp) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releases++
	if s.releases == 2 {
		close(s.enough)
	}
}

func (s *fakeSession) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
}

type innerBackend struct {
	accepts bool
	drags   int
}

func (i *innerBackend) AcceptsDrops() bool { return i.accepts }

func (i *innerBackend) StartDrag(vtui.DragPayload, vtui.DropAction) (vtui.DropAction, error) {
	i.drags++
	return vtui.DropMove, nil
}

func TestBackendFollowsPointerUntilRelease(t *testing.T) {
	sess := newFakeSession()
	b := &Backend{
		Pointer:  &scriptedPointer{samples: []pointerSample{{10, 20, true}, {30, 40, true}, {50, 60, false}}},
		Interval: time.Millisecond,
		open:     func() (dragSession, error) { return sess, nil },
	}
	action, err := b.StartDrag(vtui.DragPayload{}, vtui.DropCopy)
	if err != nil || action != vtui.DropCopy {
		t.Fatalf("StartDrag = %v, %v; want copy, nil", action, err)
	}
	sess.mu.Lock()
	defer sess.mu.Unlock()
	if len(sess.motions) != 2 || sess.motions[0] != [2]int{10, 20} || sess.motions[1] != [2]int{30, 40} {
		t.Errorf("motions = %v; want [10 20] then [30 40]", sess.motions)
	}
	if sess.releases < 2 {
		t.Errorf("releases = %d; the release must be repeated until Drag returns", sess.releases)
	}
	if !sess.closed {
		t.Error("the session was not closed after the gesture")
	}
}

func TestBackendFallsBackToInnerWhenNoSession(t *testing.T) {
	inner := &innerBackend{}
	b := &Backend{
		Inner:   inner,
		Pointer: &scriptedPointer{samples: []pointerSample{{0, 0, true}}},
		open:    func() (dragSession, error) { return nil, ErrNotWineHost },
	}
	action, err := b.StartDrag(vtui.DragPayload{}, vtui.DropMove)
	if err != nil || action != vtui.DropMove || inner.drags != 1 {
		t.Fatalf("StartDrag = %v, %v, inner drags %d; want move, nil, 1", action, err, inner.drags)
	}
}

func TestBackendReportsCauseWithoutInner(t *testing.T) {
	b := &Backend{
		Pointer: &scriptedPointer{samples: []pointerSample{{0, 0, true}}},
		open:    func() (dragSession, error) { return nil, ErrNotWineHost },
	}
	if _, err := b.StartDrag(vtui.DragPayload{}, vtui.DropCopy); !errors.Is(err, ErrNotWineHost) {
		t.Fatalf("StartDrag error = %v; want ErrNotWineHost", err)
	}
}

func TestBackendWithoutPointerIsUnsupported(t *testing.T) {
	b := NewBackend(nil, nil)
	if b.CanStartDrag() {
		t.Error("CanStartDrag is true without a Pointer")
	}
	if _, err := b.StartDrag(vtui.DragPayload{}, vtui.DropCopy); !errors.Is(err, vtui.ErrDragUnsupported) {
		t.Fatalf("StartDrag error = %v; want ErrDragUnsupported", err)
	}
}

func TestBackendAcceptsDropsFollowsInner(t *testing.T) {
	if NewBackend(nil, nil).AcceptsDrops() {
		t.Error("AcceptsDrops is true without an inner backend")
	}
	if !NewBackend(&innerBackend{accepts: true}, nil).AcceptsDrops() {
		t.Error("AcceptsDrops does not follow the inner backend")
	}
}
