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

func TestConsoleCtrlHandlerRoutineSavesBeforeTermination(t *testing.T) {
	oldSave := saveSessionOnConsoleTermination
	t.Cleanup(func() { saveSessionOnConsoleTermination = oldSave })

	for _, event := range []uintptr{ctrlCloseEvent, ctrlLogoffEvent, ctrlShutdownEvent} {
		saved := false
		saveSessionOnConsoleTermination = func() { saved = true }
		if got := consoleCtrlHandlerRoutine(event); got != 0 {
			t.Errorf("consoleCtrlHandlerRoutine(%d) = %d, want 0", event, got)
		}
		if !saved {
			t.Errorf("consoleCtrlHandlerRoutine(%d) did not save before termination", event)
		}
	}
}
