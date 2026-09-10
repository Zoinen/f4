package terminal

import (
	"strings"
	"testing"
)

func TestConsoleProbeWindowDimensions(t *testing.T) {
	probe := consoleProbe{WinLeft: 5, WinTop: 3, WinRight: 20, WinBottom: 14}
	if got := probe.WinCols(); got != 16 {
		t.Fatalf("WinCols() = %d, want 16", got)
	}
	if got := probe.WinRows(); got != 12 {
		t.Fatalf("WinRows() = %d, want 12", got)
	}
}

func TestOrEmptyMarker(t *testing.T) {
	if got := orEmptyMarker(""); got != "(empty)" {
		t.Fatalf("orEmptyMarker(\"\") = %q, want (empty)", got)
	}
	if got := orEmptyMarker("value"); got != "value" {
		t.Fatalf("orEmptyMarker(\"value\") = %q, want value", got)
	}
}

func TestWineProbeReportIncludesDiagnosticSections(t *testing.T) {
	report := wineProbeReport()
	for _, section := range []string{
		"f4 version:",
		"GOOS/GOARCH:",
		"vtui.IsWine:",
		"console buffer info:",
		"ProbePTYUsable:",
		"ConsoleMode=own:",
	} {
		if !strings.Contains(report, section) {
			t.Errorf("wineProbeReport() does not contain %q", section)
		}
	}
}
