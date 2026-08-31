package main

import (
	"testing"
	"time"

	"github.com/unxed/vtui"
)

func terminalOutputRedrawTestScene() map[string]any {
	return panelActivationFastPathScene(0, `Panels: D:\Code\f4`)
}

func TestExtUiRenderer_CoveredTerminalRedrawDeferralRequiresNegotiatedCoveredScene(t *testing.T) {
	tests := []struct {
		name       string
		negotiated bool
		suppressed bool
		mutate     func(map[string]any)
		want       bool
	}{
		{name: "owned native panels", suppressed: true, want: true},
		{name: "legacy cell surface", suppressed: false, want: false},
		{
			name: "text presentation", negotiated: true, suppressed: false,
			mutate: func(scene map[string]any) { scene["presentation"] = "text" },
			want:   false,
		},
		{
			name: "negotiated fallback scene", negotiated: true, suppressed: false,
			mutate: func(scene map[string]any) {
				scene["shell"].(map[string]any)["fallback"] = true
			},
			want: true,
		},
		{
			name: "legacy fallback scene", suppressed: false,
			mutate: func(scene map[string]any) {
				scene["shell"].(map[string]any)["fallback"] = true
			},
			want: false,
		},
		{
			name: "terminal revealed beside one panel", suppressed: true,
			mutate: func(scene map[string]any) {
				scene["shell"].(map[string]any)["showRightPanel"] = false
			},
			want: false,
		},
		{
			name: "terminal revealed below shortened panel", suppressed: true,
			mutate: func(scene map[string]any) {
				scene["shell"].(map[string]any)["panelLayout"] = map[string]any{
					"columns": 100, "splitColumn": 50,
					"leftBottomInsetRows": 1, "rightBottomInsetRows": 0,
				}
			},
			want: false,
		},
		{
			name: "wide panel exposes terminal area", suppressed: true,
			mutate: func(scene map[string]any) {
				scene["shell"].(map[string]any)["wide"] = true
			},
			want: false,
		},
		{
			name: "terminal mode", suppressed: true,
			mutate: func(scene map[string]any) {
				scene["shell"].(map[string]any)["mode"] = "terminal"
			},
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scene := terminalOutputRedrawTestScene()
			if tc.mutate != nil {
				tc.mutate(scene)
			}
			renderer := &ExtUiRenderer{
				lastScene:                    scene,
				nativeSemanticSurfaceEnabled: tc.negotiated,
				nativeCellFrameSuppressed:    tc.suppressed,
			}
			if got := renderer.CanDeferCoveredTerminalRedraw(); got != tc.want {
				t.Fatalf("CanDeferCoveredTerminalRedraw() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPanelsFrame_TerminalOutputRedrawDefersOnlyCoveredSurface(t *testing.T) {
	output := captureNavigationBenchmark(t)
	renderer := &ExtUiRenderer{
		lastScene:                 terminalOutputRedrawTestScene(),
		nativeCellFrameSuppressed: true,
	}
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

	renderer.mu.Lock()
	renderer.lastScene["shell"].(map[string]any)["showRightPanel"] = false
	renderer.mu.Unlock()
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
			results = append(results, navigationBenchmarkString(record["result"]))
		}
	}
	if len(results) != 2 || results[0] != "deferred_covered" || results[1] != "requested" {
		t.Fatalf("terminal redraw trace results = %#v, want deferred then requested", results)
	}
}

func TestPanelsFrame_TerminalSemanticStateChangeNeverDefersRedraw(t *testing.T) {
	renderer := &ExtUiRenderer{
		lastScene:                 terminalOutputRedrawTestScene(),
		nativeCellFrameSuppressed: true,
	}
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
	screen.Renderer = &ExtUiRenderer{}
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
	screen.Renderer = &ExtUiRenderer{}
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
