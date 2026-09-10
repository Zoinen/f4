package panel

import (
	"errors"
	"github.com/unxed/f4/internal/terminal"
	"strings"
	"testing"
	"time"
)

func TestLocalPTYFailureMessageIncludesAllocationStep(t *testing.T) {
	got := localPTYFailureMessage(errors.New("open term.PTY master: permission denied"))
	if !strings.Contains(got, "open term.PTY master: permission denied") {
		t.Fatalf("PTY failure message lost the actionable cause: %q", got)
	}
}

// The local terminal.PTY is published by the goroutine initPTY starts, so every other
// goroutine has to read the field under ptyMutex. Reading it directly is not
// merely stale prone: the field is an interface, two words wide, and a reader
// can catch the type word of a *terminal.PTY with the data word still zero. That value
// passes an "!= nil" guard and calls the method on a nil receiver. F10 in the
// first milliseconds of a session did exactly that and crashed in terminal.PTY.Close.

func TestPanelsFrame_LocalPTYWaitsForThePublisher(t *testing.T) {
	pf := &PanelsFrame{}

	pf.PtyMutex.Lock()
	seen := make(chan terminal.PtyBackend, 1)
	go func() { seen <- pf.localPTY() }()

	select {
	case <-seen:
		pf.PtyMutex.Unlock()
		t.Fatal("localPTY returned while the publisher still held ptyMutex")
	case <-time.After(50 * time.Millisecond):
	}

	pty := &mockPty{}
	pf.Pty = pty
	pf.PtyMutex.Unlock()

	if got := <-seen; got != pty {
		t.Fatalf("localPTY returned %#v, expected the term.PTY the publisher set", got)
	}
}

func TestPanelsFrame_TakeLocalPTYHandsOverOnce(t *testing.T) {
	pf := &PanelsFrame{}
	pty := &mockPty{}
	pf.Pty = pty

	if got := pf.takeLocalPTY(); got != pty {
		t.Fatalf("takeLocalPTY returned %#v, expected the local term.PTY", got)
	}
	if pf.Pty != nil {
		t.Error("takeLocalPTY left the field set, so Close would shut the same term.PTY down twice")
	}
	if got := pf.takeLocalPTY(); got != nil {
		t.Errorf("takeLocalPTY handed the same term.PTY out twice: %#v", got)
	}
}

// TestPanelsFrame_PTYHandoverRacesThePublisher is the shape of the crash:
// a frame is created, its terminal.PTY is published from another goroutine, and the
// shutdown path reaches for it at the same moment. It passes trivially on its
// own and earns its keep under -race, which reports the unsynchronized field
// access the moment either accessor loses its lock again.
func TestPanelsFrame_PTYHandoverRacesThePublisher(t *testing.T) {
	for i := 0; i < 200; i++ {
		pf := &PanelsFrame{}

		published := make(chan struct{})
		go func() {
			pf.PtyMutex.Lock()
			pf.Pty = &mockPty{}
			pf.PtyMutex.Unlock()
			close(published)
		}()

		if pty := pf.takeLocalPTY(); pty != nil {
			if err := pty.Close(); err != nil {
				t.Fatal(err)
			}
		}
		<-published
	}
}
