package panel

import (
	"testing"
	"time"

	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtinput"
)

// f4#128, RUP step 1: f4 proactively enables the kitty keyboard protocol's
// disambiguate flag for its own embedded terminal right when a fresh local
// shell's PTY comes up, instead of only ever reacting to a nested program
// (far2l) requesting it itself.
//
// This drives the real startup path -- InitPTY's shell-spawning goroutine,
// with newLocalPTY swapped for a mock the way shell_session_test.go does --
// so it is exercised end to end: after a bare shell session starts, with
// nothing written to the PTY by any nested program, TermView.KittyFlags must
// already be set, and the existing Ctrl+Tab-forwarding decision in
// frame.go's HandleKey must treat that exactly the way it already treats
// far2l's own request (TestPanelsFrame_TerminalForwarding_Advanced).
func TestPanelsFrame_FreshLocalShellEnablesKittyProtocol(t *testing.T) {
	if terminal.WindowsShellSyntax() {
		t.Skip("cmd.exe-syntax local shells are covered by the Windows-only skip test instead")
	}

	pf := setupMockPanelsFrame(t)
	defer pf.Close()

	oldSpawn := SpawnLocalShellPTY
	oldFactory := newLocalPTY
	t.Cleanup(func() {
		SpawnLocalShellPTY = oldSpawn
		newLocalPTY = oldFactory
	})

	SpawnLocalShellPTY = true
	created := make(chan *mockPty, 1)
	newLocalPTY = func() (terminal.PtyBackend, error) {
		pty := &mockPty{}
		created <- pty
		return pty, nil
	}

	// Nothing is running yet; InitPTY will see pf.Pty == nil and spawn a
	// fresh one, exactly like the first shell of a new terminal session.
	pf.Pty = nil
	pf.InitPTY()

	var fresh *mockPty
	select {
	case fresh = <-created:
	case <-time.After(time.Second):
		t.Fatal("InitPTY did not spawn a fresh local shell")
	}

	deadline := time.Now().Add(time.Second)
	for pf.localPTY() != fresh {
		if time.Now().After(deadline) {
			t.Fatalf("fresh local shell was never published to the panels frame (current=%T)", pf.localPTY())
		}
		time.Sleep(time.Millisecond)
	}

	// The bare shell itself never wrote anything -- the enable request must
	// have come from f4, not from a nested program.
	deadline = time.Now().Add(time.Second)
	for pf.TermView.KittyFlags.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("a fresh bare local shell session did not have the kitty keyboard protocol enabled")
		}
		time.Sleep(time.Millisecond)
	}

	// Now drive the exact decision TestPanelsFrame_TerminalForwarding_Advanced
	// exercises for far2l's own request: with the protocol active, plain
	// Ctrl+Tab must go to whatever runs inside instead of switching
	// workspaces, while Ctrl+Shift+Tab must still be f4's own.
	pf.ShowPanels = false
	pf.TermView.UseAltScreen = true

	handled := pressKey(pf, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_TAB, ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if !handled {
		t.Error("Ctrl+Tab should be forwarded to term.PTY once the bare shell's own protocol request has taken effect")
	}
	fresh.mu.Lock()
	gotBytes := len(fresh.written) != 0
	fresh.written = nil
	fresh.mu.Unlock()
	if !gotBytes {
		t.Error("PTY did not receive bytes for Ctrl+Tab once the bare shell's protocol request took effect")
	}

	handled = pressKey(pf, &vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_TAB, ControlKeyState: vtinput.LeftCtrlPressed | vtinput.ShiftPressed,
	})
	if handled {
		t.Error("Shift+Ctrl+Tab was erroneously forwarded to term.PTY")
	}
}

// A local Windows shell (cmd.exe/PowerShell) is deliberately left alone: f4
// cannot claim win32-input-mode is active without the real console child
// having actually turned it on for its own console handle (see the comment
// on KittyEnableDisambiguateSeq's use in InitPTY), so on that platform this
// step must be a no-op.
func TestPanelsFrame_FreshLocalShellSkipsProtocolEnableOnWindowsShellSyntax(t *testing.T) {
	if !terminal.WindowsShellSyntax() {
		t.Skip("only meaningful where the local shell speaks cmd.exe syntax")
	}

	pf := setupMockPanelsFrame(t)
	defer pf.Close()

	oldSpawn := SpawnLocalShellPTY
	oldFactory := newLocalPTY
	t.Cleanup(func() {
		SpawnLocalShellPTY = oldSpawn
		newLocalPTY = oldFactory
	})

	SpawnLocalShellPTY = true
	created := make(chan *mockPty, 1)
	newLocalPTY = func() (terminal.PtyBackend, error) {
		pty := &mockPty{}
		created <- pty
		return pty, nil
	}

	pf.Pty = nil
	pf.InitPTY()

	var fresh *mockPty
	select {
	case fresh = <-created:
	case <-time.After(time.Second):
		t.Fatal("InitPTY did not spawn a fresh local shell")
	}

	deadline := time.Now().Add(time.Second)
	for pf.localPTY() != fresh {
		if time.Now().After(deadline) {
			t.Fatalf("fresh local shell was never published to the panels frame (current=%T)", pf.localPTY())
		}
		time.Sleep(time.Millisecond)
	}

	// Give the startup goroutine a moment to have done whatever it does,
	// then confirm neither keyboard protocol flag was claimed.
	time.Sleep(20 * time.Millisecond)
	if pf.TermView.KittyFlags.Load() != 0 || pf.TermView.Win32InputMode {
		t.Fatalf("a cmd.exe-syntax local shell must not have either advanced protocol claimed for it: kitty=%d win32=%v",
			pf.TermView.KittyFlags.Load(), pf.TermView.Win32InputMode)
	}
}
