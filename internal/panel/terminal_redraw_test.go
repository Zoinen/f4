package panel

import (
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
	"testing"
	"time"
)

func TestPanelsFrame_TerminalOutputRedrawDefersOnlyCoveredSurface(t *testing.T) {
	if !testutil.RunWithEnvironment(t, "F4_NAV_BENCHMARK_TRACE", "1") {
		return
	}
	output := captureNavigationBenchmark(t)
	renderer := &terminalPresentationRecorder{covered: true}
	screen := vtui.NewSilentScreenBuf()
	screen.Renderer = renderer
	vtui.FrameManager.Init(screen)
	t.Cleanup(func() { vtui.FrameManager.Init(vtui.NewSilentScreenBuf()) })

	drainRedraw := func() {
		for {
			select {
			case <-vtui.FrameManager.RedrawChan:
			default:
				return
			}
		}
	}
	drainRedraw()

	frame := &PanelsFrame{}
	frame.redrawAfterTerminalOutput(23, false)
	select {
	case <-vtui.FrameManager.RedrawChan:
		t.Fatal("covered native terminal output queued a redundant redraw")
	default:
	}

	renderer.covered = false
	frame.redrawAfterTerminalOutput(29, false)
	select {
	case <-vtui.FrameManager.RedrawChan:
		// The revealed terminal retained the ordinary redraw path.
	default:
		t.Fatal("revealed terminal output did not queue a redraw")
	}

	records := decodeNavigationBenchmarkRecords(t, output)
	var results []string
	for _, record := range records {
		if record["event"] == "go.terminal.output.redraw" {
			results = append(results, semantic.String(record["result"]))
		}
	}
	if len(results) != 2 || results[0] != "deferred_covered" || results[1] != "requested" {
		t.Fatalf("terminal redraw trace results = %#v, want deferred then requested", results)
	}
}

func TestPanelsFrame_TerminalSemanticStateChangeNeverDefersRedraw(t *testing.T) {
	renderer := &terminalPresentationRecorder{covered: true}
	screen := vtui.NewSilentScreenBuf()
	screen.Renderer = renderer
	vtui.FrameManager.Init(screen)
	t.Cleanup(func() { vtui.FrameManager.Init(vtui.NewSilentScreenBuf()) })
	for {
		select {
		case <-vtui.FrameManager.RedrawChan:
		default:
			goto drained
		}
	}

drained:
	(&PanelsFrame{}).redrawAfterTerminalOutput(7, true)
	select {
	case <-vtui.FrameManager.RedrawChan:
		// Title, visibility, focus, alternate-screen, and busy changes are
		// semantic state and therefore retain an immediate correction render.
	default:
		t.Fatal("terminal semantic-state change was hidden by covered-output deferral")
	}
}

func TestPanelsFrame_TerminalOutputRedrawCoalescesBurstAndPublishesTail(t *testing.T) {
	screen := vtui.NewSilentScreenBuf()
	screen.Renderer = &plughost.ExtUiRenderer{}
	vtui.FrameManager.Init(screen)
	t.Cleanup(func() { vtui.FrameManager.Init(vtui.NewSilentScreenBuf()) })
	for {
		select {
		case <-vtui.FrameManager.RedrawChan:
		default:
			goto drained
		}
	}

drained:
	frame := &PanelsFrame{}
	t.Cleanup(frame.closeTerminalOutputRedraw)
	frame.terminalOutputRedrawMu.Lock()
	frame.terminalOutputRedrawLast = time.Now()
	frame.terminalOutputRedrawMu.Unlock()

	for i := 0; i < 1_000; i++ {
		frame.redrawAfterTerminalOutput(32*1024, false)
	}
	select {
	case <-vtui.FrameManager.RedrawChan:
		t.Fatal("terminal burst bypassed the presentation cadence")
	default:
	}

	select {
	case <-vtui.FrameManager.RedrawChan:
		// The one trailing redraw publishes the newest terminal state.
	case <-time.After(10 * terminalOutputRedrawInterval):
		t.Fatal("coalesced terminal burst never published its trailing state")
	}
	select {
	case <-vtui.FrameManager.RedrawChan:
		t.Fatal("one terminal burst queued more than one trailing redraw")
	case <-time.After(2 * terminalOutputRedrawInterval):
	}
}

func TestPanelsFrame_TerminalSemanticStateChangeCancelsOlderTrailingRedraw(t *testing.T) {
	screen := vtui.NewSilentScreenBuf()
	screen.Renderer = &plughost.ExtUiRenderer{}
	vtui.FrameManager.Init(screen)
	t.Cleanup(func() { vtui.FrameManager.Init(vtui.NewSilentScreenBuf()) })
	for {
		select {
		case <-vtui.FrameManager.RedrawChan:
		default:
			goto drained
		}
	}

drained:
	frame := &PanelsFrame{}
	t.Cleanup(frame.closeTerminalOutputRedraw)
	frame.terminalOutputRedrawMu.Lock()
	frame.terminalOutputRedrawLast = time.Now()
	frame.terminalOutputRedrawMu.Unlock()
	frame.redrawAfterTerminalOutput(32*1024, false)
	frame.redrawAfterTerminalOutput(7, true)

	select {
	case <-vtui.FrameManager.RedrawChan:
		// The state transition remains immediate.
	default:
		t.Fatal("terminal semantic-state change was delayed by stream coalescing")
	}
	select {
	case <-vtui.FrameManager.RedrawChan:
		t.Fatal("cancelled stream timer published a stale terminal frame")
	case <-time.After(2 * terminalOutputRedrawInterval):
	}
}

type terminalPresentationRecorder struct {
	searchFirstActivationRenderer
	covered bool
}

func (r *terminalPresentationRecorder) CanDeferCoveredTerminalRedraw() bool { return r.covered }
func (r *terminalPresentationRecorder) CoalesceTerminalOutputRedraw() bool  { return true }

type nativePanelRenderer struct{ searchFirstActivationRenderer }

func (*nativePanelRenderer) VirtualizePanelTableRows() bool { return true }
