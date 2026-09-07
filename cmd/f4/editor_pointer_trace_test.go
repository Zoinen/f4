package main

import (
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

func TestEditorPointerTraceJoinsIntentAppliedSourceAndWindowWithoutText(t *testing.T) {
	const privateText = "PRIVATE_DOCUMENT_CONTENT_MUST_NOT_BE_TRACED"
	ev := projectionTestEditor(t, privateText+strings.Repeat("x", 120)+"\nshort\n\n")
	ev.highlighter = nil
	ev.nativeViewportColumns = 60
	ev.ensureEngineWidth()
	ev.ScrollLeft = 50
	output := captureNavigationBenchmark(t)
	anchor := 58
	for index, line := range []int{0, 1, 2} {
		traceID := "qt:pointer:source:" + string(rune('0'+index))
		action := map[string]any{
			"action": "editor.mouse", "target": vtui.SemanticID(ev),
			"documentKey": vtui.SemanticID(ev), "layoutRevision": ev.semanticLayoutRevision,
			"rowOffset": int64(ev.li.GetLineOffset(line)), "column": 8, "scrollLeft": 50,
			"phase": "press", "button": "left", "moved": index != 0,
		}
		if index != 0 {
			action["phase"] = "move"
		}
		trace := navigationBenchmarkTraceForAction(map[string]any{"benchmarkTraceId": traceID}, action, nil)
		if trace == nil || trace.id != traceID {
			t.Fatal("native pointer action did not retain its outgoing Qt trace identity")
		}
		previous := navigationBenchmarkSetCurrentUI(trace)
		ev.HandleSemanticAction(action)
		navigationBenchmarkSetCurrentUI(previous)
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
		wantOffset := ev.li.GetLineOffset(ev.CursorLine) + ev.CursorPos
		if semanticInt(applied["cursorOffset"]) != wantOffset || semanticInt(window["cursorOffset"]) != wantOffset ||
			semanticInt(applied["selectionAnchor"]) != anchor || semanticInt(window["selectionAnchor"]) != anchor ||
			semanticInt(applied["scrollLeft"]) != 50 || semanticInt(window["scrollLeft"]) != 50 {
			t.Fatalf("applied/window source positions differ: applied=%#v window=%#v", applied, window)
		}
		if semanticBool(applied["inputWindowGenerationKnown"]) || !semanticBool(window["ready"]) {
			t.Fatal("trace invented an absent input generation or lost ready-window state")
		}
		if strings.Contains(output.String(), privateText) {
			t.Fatal("pointer timing diagnostics recorded document text")
		}
		output.Reset()
	}
	// Disabled timing must not allocate/log a replacement input identity or
	// emit source coordinates from either helper.
	navigationBenchmarkEnabled.Store(false)
	defer navigationBenchmarkEnabled.Store(true)
	ev.tracePointerAction(map[string]any{}, "applied", "unchanged")
	ev.SemanticNode(&vtui.SemanticContext{})
	if output.Len() != 0 {
		t.Fatal("disabled pointer diagnostics emitted records")
	}
}
