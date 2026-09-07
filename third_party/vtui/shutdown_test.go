package vtui

import (
	"github.com/unxed/vtinput"
	"testing"
)

func TestShutdown_Idempotent(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)

	fm := &frameManager{}
	fm.Init(scr)
	fm.Push(NewDesktop())

	// Calling Shutdown multiple times must not panic
	fm.Shutdown()
	fm.Shutdown()
	fm.Shutdown()

	if !fm.IsShutdown() {
		t.Error("Expected IsShutdown to be true")
	}
}

type panicFrame struct {
	BaseFrame
}

func (p *panicFrame) ProcessKey(e *vtinput.InputEvent) bool {
	panic("simulated widget panic")
}

func TestShutdown_RestoresTerminalOnPanic(t *testing.T) {
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)

	fm := &frameManager{}
	fm.Init(scr)

	isPrepared = true
	inAltScreen = true

	// Simulate panic recovery in Step/Run
	func() {
		defer func() {
			if r := recover(); r != nil {
				fm.Shutdown()
			}
		}()
		f := &panicFrame{}
		f.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true})
	}()

	if isPrepared {
		t.Error("Terminal should not remain prepared after panic shutdown")
	}
	if inAltScreen {
		t.Error("Terminal should not remain in alt screen after panic shutdown")
	}
	if !fm.IsShutdown() {
		t.Error("FrameManager should be marked as shutdown after panic")
	}
}
