package panel

import (
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/vtui"
	"strings"
	"testing"
	"testing/synctest"
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

func TestPanelsFrame_TerminalCommandOutputBatchesEchoWithFastOutput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
		defer vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
		select {
		case <-vtui.FrameManager.RedrawChan:
		default:
		}
		vtui.SetDefaultPalette()
		view := terminal.NewTerminalView(80, 24)
		defer view.Close()
		view.OwnsCmdPrompt = func() bool { return true }
		frame := &PanelsFrame{TermView: view, Parser: terminal.NewAnsiParser(view, nil)}
		defer frame.closeTerminalOutputRedraw()
		const prompt = "\x1b]133;A;f4-cmd\x1b\\C:\\Test>\x1b]133;B;f4-cmd\x1b\\"
		frame.Parser.Process([]byte(prompt))
		line := &extui.CommandLineModel{Visible: true}
		history := func() string {
			var out strings.Builder
			for _, row := range frame.terminalSemanticModel(line, true).WindowRows {
				for _, run := range row.Runs {
					out.WriteString(run.Text)
				}
				out.WriteByte('\n')
			}
			return out.String()
		}
		baseline := frame.terminalSemanticModel(line, false).ContentExtent
		frame.batchTerminalCommandOutput()
		view.ExpectCommandEcho()
		frame.displayLocalOutput(true, []byte("\x1b]0;cmd.exe - echo fast\x07echo fast\r\n"))
		if text := history(); strings.Contains(text, "echo fast") {
			t.Fatalf("another UI redraw exposed the standalone command echo: %q", text)
		}
		time.Sleep(5 * time.Millisecond)
		frame.displayLocalOutput(true, []byte("fast\r\n\r\n"+prompt))
		if got := frame.terminalSemanticModel(line, true).ContentExtent; got != baseline {
			t.Fatalf("batch published early, extent %d, want %d", got, baseline)
		}
		select {
		case <-vtui.FrameManager.RedrawChan:
			t.Fatal("the echo or fast output requested a standalone redraw")
		default:
		}
		time.Sleep(terminalOutputRedrawInterval)
		synctest.Wait()
		select {
		case <-vtui.FrameManager.RedrawChan:
		default:
			t.Fatal("the batch never requested a redraw")
		}
		if text := history(); !strings.Contains(text, "C:\\Test>echo fast\nfast\n") {
			t.Fatalf("the first published history missed echo or output: %q", text)
		}

		frame.batchTerminalCommandOutput()
		view.ExpectCommandEcho()
		frame.displayLocalOutput(true, []byte("slow-command\r\n"))
		time.Sleep(terminalOutputRedrawInterval)
		synctest.Wait()
		if text := history(); !strings.Contains(text, "C:\\Test>slow-command\n") {
			t.Fatalf("the slow command echo was held beyond the initial batch: %q", text)
		}
		frame.displayLocalOutput(true, []byte("first chunk\r\n"))
		time.Sleep(terminalOutputRedrawInterval)
		synctest.Wait()
		if text := history(); !strings.Contains(text, "first chunk\n") {
			t.Fatalf("streaming output was not published while the command was running: %q", text)
		}

		frame.batchTerminalCommandOutput()
		frame.displayLocalOutput(true, []byte("\x1b[?1049hinteractive"))
		if model := frame.terminalSemanticModel(line, true); !model.AltScreen {
			t.Fatal("alternate-screen transition was held by the batch")
		}
	})
}

type terminalPresentationRecorder struct {
	searchFirstActivationRenderer
	covered bool
}

func (r *terminalPresentationRecorder) CanDeferCoveredTerminalRedraw() bool { return r.covered }
func (r *terminalPresentationRecorder) CoalesceTerminalOutputRedraw() bool  { return true }

type nativePanelRenderer struct{ searchFirstActivationRenderer }

func (*nativePanelRenderer) VirtualizePanelTableRows() bool { return true }
