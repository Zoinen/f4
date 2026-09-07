package main

import (
	"strings"
	"testing"

	"github.com/unxed/f4/sdk/extui"
)

func TestDocumentWindowTraceDistinguishesStylesAtSameCoordinates(t *testing.T) {
	output := captureNavigationBenchmark(t)
	const privateText = "PRIVATE_WINDOW_TEXT_NOT_LOGGED"
	surface := extui.SurfaceModel{DocumentKey: "editor:test", Kind: "editor",
		LayoutRevision: 1, WindowStart: 0, WindowEnd: 1,
		WindowRows: []extui.TextRowModel{{Runs: []extui.RunModel{{
			Text: privateText, Foreground: "#ffffff", Background: "#191d23",
		}}}}}
	traceDocumentWindow(&surface)
	first := navigationBenchmarkEvents(decodeNavigationBenchmarkRecords(t, output))["go.document.window.projected"]
	firstKey := semanticString(first["windowContentKey"])
	if firstKey == "" || firstKey != extui.WindowRowsContentKey(surface.WindowRows) {
		t.Fatal("trace lost the exact published window fingerprint")
	}
	output.Reset()
	surface.WindowRows[0].Runs[0].Background = "#3b6290"
	traceDocumentWindow(&surface)
	second := navigationBenchmarkEvents(decodeNavigationBenchmarkRecords(t, output))["go.document.window.projected"]
	if semanticString(second["windowContentKey"]) == firstKey {
		t.Fatal("same-coordinate selection update reused the old trace identity")
	}
	if strings.Contains(output.String(), privateText) {
		t.Fatal("window trace emitted private document text")
	}
}
