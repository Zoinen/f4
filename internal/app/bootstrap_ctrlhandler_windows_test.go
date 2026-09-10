//go:build windows

package app

import (
	"testing"
)

func TestConsoleCtrlHandlerRoutineHandlesInterrupts(t *testing.T) {
	for _, event := range []uintptr{ctrlCEvent, ctrlBreakEvent} {
		if got := consoleCtrlHandlerRoutine(event); got != 1 {
			t.Errorf("consoleCtrlHandlerRoutine(%d) = %d, want 1", event, got)
		}
	}
}

func TestConsoleCtrlHandlerRoutineIgnoresOtherEvents(t *testing.T) {
	if got := consoleCtrlHandlerRoutine(ctrlCloseEvent); got != 0 {
		t.Fatalf("consoleCtrlHandlerRoutine(%d) = %d, want 0", ctrlCloseEvent, got)
	}
}
