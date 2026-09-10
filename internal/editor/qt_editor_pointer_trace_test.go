package editor

import (
	"github.com/unxed/f4/internal/navtrace"
	"github.com/unxed/f4/internal/semantic"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
	"strings"
	"testing"
)

func TestEditorPointerTraceJoinsIntentAppliedSourceAndWindowWithoutText(t *testing.T) {
	if !testutil.RunWithEnvironment(t, "F4_NAV_BENCHMARK_TRACE", "1") {
		return
	}
	const privateText = "PRIVATE_DOCUMENT_CONTENT_MUST_NOT_BE_TRACED"
	ev := projectionTestEditor(t, privateText+strings.Repeat("x", 120)+"\nshort\n\n")
	ev.Highlighter = nil
	ev.NativeViewportColumns = 60
	ev.EnsureEngineWidth()
	ev.ScrollLeft = 50
	output := captureNavigationBenchmark(t)
	anchor := 58
	for index, line := range []int{0, 1, 2} {
		traceID := "qt:pointer:source:" + string(rune('0'+index))
		action := map[string]any{
			"action": "editor.mouse", "target": vtui.SemanticID(ev),
			"documentKey": vtui.SemanticID(ev), "layoutRevision": ev.semanticLayoutRevision,
			"rowOffset": int64(ev.Li.GetLineOffset(line)), "column": 8, "scrollLeft": 50,
			"phase": "press", "button": "left", "moved": index != 0,
		}
		if index != 0 {
			action["phase"] = "move"
		}
		trace := navtrace.NavigationBenchmarkTraceForAction(map[string]any{"benchmarkTraceId": traceID}, action, nil)
		if trace == nil || trace.Id != traceID {
			t.Fatal("native pointer action did not retain its outgoing Qt trace identity")
		}
		previous := navtrace.NavigationBenchmarkSetCurrentUI(trace)
		ev.HandleSemanticAction(action)
		navtrace.NavigationBenchmarkSetCurrentUI(previous)
		// Production exports the resulting window after input dispatch, when
		// the current UI trace has already been restored.
		ev.SemanticNode(&vtui.SemanticContext{})
		events := navigationBenchmarkEvents(decodeNavigationBenchmarkRecords(t, output))
		for _, name := range []string{"go.editor.pointer.received", "go.editor.pointer.applied", "go.editor.pointer.window"} {
			event := events[name]
			if event == nil || event["benchmarkTraceId"] != traceID || event["documentKey"] != vtui.SemanticID(ev) {
				t.Fatalf("%s lost action/document identity: %#v", name, event)
			}
		}
		applied, window := events["go.editor.pointer.applied"], events["go.editor.pointer.window"]
		wantOffset := ev.Li.GetLineOffset(ev.CursorLine) + ev.CursorPos
		if semantic.Int(applied["cursorOffset"]) != wantOffset || semantic.Int(window["cursorOffset"]) != wantOffset ||
			semantic.Int(applied["selectionAnchor"]) != anchor || semantic.Int(window["selectionAnchor"]) != anchor ||
			semantic.Int(applied["scrollLeft"]) != 50 || semantic.Int(window["scrollLeft"]) != 50 {
			t.Fatalf("applied/window source positions differ: applied=%#v window=%#v", applied, window)
		}
		if semantic.Bool(applied["inputWindowGenerationKnown"]) || !semantic.Bool(window["ready"]) {
			t.Fatal("trace invented an absent input generation or lost ready-window state")
		}
		if strings.Contains(output.read(t), privateText) {
			t.Fatal("pointer timing diagnostics recorded document text")
		}
		output.offset += len(output.read(t))
	}
}

func TestEditorPointerTraceDisabled(t *testing.T) {
	if !testutil.RunWithoutEnvironment(t, "F4_NAV_BENCHMARK_TRACE") {
		return
	}
	output := captureNavigationBenchmark(t)
	ev := projectionTestEditor(t, "text")
	ev.tracePointerAction(map[string]any{}, "applied", "unchanged")
	ev.SemanticNode(&vtui.SemanticContext{})
	if output.read(t) != "" {
		t.Fatal("disabled pointer diagnostics emitted records")
	}
}
