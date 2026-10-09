package panel

import (
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
)

// The redraw scheduler fires from a timer goroutine, so it has to be bound to
// the FrameManager its frame was created under and not look the global up when
// it fires; a test swapping the global in its cleanup raced with it.
func TestTerminalRedrawTargetsTheFrameManagerItWasCreatedUnder(t *testing.T) {
	defer testutil.SwapFrameManager(t)()
	first := vtui.FrameManager
	first.RedrawChan = make(chan struct{}, 1)

	pf := NewPanelsFrame()
	defer pf.terminalRedraw.Stop()

	second := vtui.NewFrameManager()
	second.RedrawChan = make(chan struct{}, 1)
	vtui.FrameManager = second

	pf.terminalRedraw.Request()

	select {
	case <-first.RedrawChan:
	default:
		t.Error("the redraw was not requested from the manager the frame was created under")
	}
	select {
	case <-second.RedrawChan:
		t.Error("the redraw went to whichever manager is the global now")
	default:
	}
}
