package vtui

import "testing"

func TestCommandSet(t *testing.T) {
	const first, second = CmApp, CmApp + 1

	var zero CommandSet
	if zero.IsDisabled(first) {
		t.Fatal("zero-value CommandSet disabled a command")
	}
	zero.Enable(first) // Enabling a command before initialization is a no-op.
	if zero.IsDisabled(first) {
		t.Fatal("Enable on a zero-value CommandSet changed its state")
	}

	cs := NewCommandSet()
	cs.Disable(first)
	if !cs.IsDisabled(first) || cs.IsDisabled(second) {
		t.Fatal("Disable did not affect only the requested command")
	}

	cs.Enable(first)
	cs.Enable(second) // Enabling a command that was not disabled is safe.
	if cs.IsDisabled(first) || cs.IsDisabled(second) {
		t.Fatal("Enable did not clear the requested command")
	}

	cs.Disable(first)
	cs.Disable(second)
	cs.Clear()
	if cs.IsDisabled(first) || cs.IsDisabled(second) {
		t.Fatal("Clear left commands disabled")
	}
	cs.Disable(first)
	if !cs.IsDisabled(first) {
		t.Fatal("CommandSet could not be reused after Clear")
	}
}

func TestCommandSetClearZeroValue(t *testing.T) {
	var cs CommandSet
	cs.Clear()
	cs.Disable(CmQuit)
	if !cs.IsDisabled(CmQuit) {
		t.Fatal("zero-value CommandSet did not initialize after Clear")
	}
}
