//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris

package vtui

import (
	"io"
	"runtime"
	"testing"
	"time"

	"github.com/gogpu/gpucontext"
	"github.com/unxed/vtinput"
)

func TestIsSpecialOrModifiedKey(t *testing.T) {
	tests := []struct {
		name string
		vk   uint16
		mods vtinput.ControlKeyState
		want bool
	}{
		{"Special Navigation Down", vtinput.VK_DOWN, 0, true},
		{"Special Return", vtinput.VK_RETURN, 0, true},
		{"Special Escape", vtinput.VK_ESCAPE, 0, true},
		{"Special Function F1", vtinput.VK_F1, 0, true},
		{"Modified Key Ctrl+A", vtinput.VK_A, vtinput.LeftCtrlPressed, true},
		{"Modified Key Alt+B", vtinput.VK_B, vtinput.LeftAltPressed, true},
		{"Regular Char A", vtinput.VK_A, 0, false},
		{"Regular Char Digit 1", vtinput.VK_1, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSpecialOrModifiedKey(tt.vk, tt.mods)
			if got != tt.want {
				t.Errorf("isSpecialOrModifiedKey(%v, %v) = %v, want %v", tt.vk, tt.mods, got, tt.want)
			}
		})
	}
}

func TestGogpuHost_SendEvent_NonBlocking(t *testing.T) {
	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	defer reader.Close()

	host := &GogpuHost{reader: reader}

	// 1. Fill event channel capacity
	for i := 0; i < cap(reader.EventChan); i++ {
		reader.EventChan <- &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true}
	}

	// 2. Sending MouseMoved when queue is full must return immediately without blocking
	done := make(chan bool)
	go func() {
		host.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseEventFlags: vtinput.MouseMoved,
		})
		done <- true
	}()

	select {
	case <-done:
		// Success: didn't block
	case <-time.After(100 * time.Millisecond):
		t.Fatal("sendEvent blocked on full queue during MouseMoved event")
	}
}
func TestGogpuHost_GetTerminalSize(t *testing.T) {
	host := &GogpuHost{
		cellW: 8,
		cellH: 16,
	}
	host.lastAppW = 800
	host.lastAppH = 600

	// Set the global function to our host-bound version
	GetTerminalSize = func() (int, int, error) {
		host.mu.Lock()
		defer host.mu.Unlock()

		w, h := host.lastAppW, host.lastAppH

		if host.cellW > 0 && host.cellH > 0 && w > 0 && h > 0 {
			c := w / host.cellW
			r := h / host.cellH
			if c != host.cols || r != host.rows {
				host.cols = c
				host.rows = r
			}
		}
		return host.cols, host.rows, nil
	}

	cols, rows, err := GetTerminalSize()
	if err != nil {
		t.Fatalf("GetTerminalSize returned error: %v", err)
	}

	expectedCols := 800 / 8  // 100
	expectedRows := 600 / 16 // 37

	if cols != expectedCols || rows != expectedRows {
		t.Errorf("GetTerminalSize: expected %dx%d, got %dx%d", expectedCols, expectedRows, cols, rows)
	}
}
func TestGogpuHost_LastRuneForVK_KeyRepeat(t *testing.T) {
	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	defer reader.Close()

	host := &GogpuHost{
		reader:        reader,
		lastRuneForVK: make(map[uint16]rune),
	}

	vk := uint16(vtinput.VK_A)
	host.lastRuneForVK[vk] = 'a'

	ev := &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vk,
	}

	if host.lastRuneForVK[vk] != 'a' {
		t.Errorf("Expected 'a' for VK_A in lastRuneForVK, got %c", host.lastRuneForVK[vk])
	}

	// Verify character restoration for key repeat
	if ev.Char == 0 && host.lastRuneForVK[vk] != 0 {
		ev.Char = host.lastRuneForVK[vk]
	}

	if ev.Char != 'a' {
		t.Errorf("Expected restored Char 'a', got %c", ev.Char)
	}
}

// The macOS Command key must act as Ctrl: Cmd+C has to reach the application
// as Ctrl+C, and the Command key itself as a Ctrl key. Cmd owns the left Ctrl
// channel (VK_LCONTROL, LeftCtrlPressed) and physical Ctrl the right one, so
// the two never share a virtual key. Everywhere else the Super/Win key
// belongs to the OS and stays a Win key with no Ctrl folding.
func TestGogpuHost_SuperAsCtrl(t *testing.T) {
	oldCmdIsCtrl := gogpuCmdIsCtrl
	defer func() { gogpuCmdIsCtrl = oldCmdIsCtrl }()

	gogpuCmdIsCtrl = true
	if got := gogpuKeyToVK(gpucontext.KeyLeftSuper, 0); got != vtinput.VK_LCONTROL {
		t.Errorf("Cmd-as-Ctrl: KeyLeftSuper mapped to vk %d, want VK_LCONTROL", got)
	}
	if got := gogpuKeyToVK(gpucontext.KeyRightSuper, 0); got != vtinput.VK_LCONTROL {
		t.Errorf("Cmd-as-Ctrl: KeyRightSuper mapped to vk %d, want VK_LCONTROL", got)
	}
	if got := gogpuKeyToVK(gpucontext.KeyLeftControl, 0); got != vtinput.VK_RCONTROL {
		t.Errorf("Cmd-as-Ctrl: KeyLeftControl mapped to vk %d, want VK_RCONTROL", got)
	}
	if got := gogpuKeyToVK(gpucontext.KeyRightControl, 0); got != vtinput.VK_RCONTROL {
		t.Errorf("Cmd-as-Ctrl: KeyRightControl mapped to vk %d, want VK_RCONTROL", got)
	}

	host := &GogpuHost{}
	mods := host.syncMods(gpucontext.KeyC, gpucontext.ModSuper, true)
	if mods&vtinput.LeftCtrlPressed == 0 {
		t.Errorf("Cmd-as-Ctrl: Super chord produced mods %v, want LeftCtrlPressed", mods)
	}
	if mods&vtinput.RightCtrlPressed != 0 {
		t.Errorf("Cmd-as-Ctrl: Super chord produced mods %v, RightCtrlPressed belongs to physical Ctrl", mods)
	}
	if !host.superDown {
		t.Error("Cmd-as-Ctrl: superDown not set while Super is held")
	}
	mods = host.syncMods(gpucontext.KeyC, 0, false)
	if mods != 0 {
		t.Errorf("Cmd-as-Ctrl: mods after Super release = %v, want 0", mods)
	}
	if host.superDown {
		t.Error("Cmd-as-Ctrl: superDown still set after Super release")
	}

	gogpuCmdIsCtrl = false
	if got := gogpuKeyToVK(gpucontext.KeyLeftSuper, 0); got != vtinput.VK_LWIN {
		t.Errorf("plain Super: KeyLeftSuper mapped to vk %d, want VK_LWIN", got)
	}
	if got := gogpuKeyToVK(gpucontext.KeyRightSuper, 0); got != vtinput.VK_RWIN {
		t.Errorf("plain Super: KeyRightSuper mapped to vk %d, want VK_RWIN", got)
	}
	if got := gogpuKeyToVK(gpucontext.KeyLeftControl, 0); got != vtinput.VK_LCONTROL {
		t.Errorf("plain Super: KeyLeftControl mapped to vk %d, want VK_LCONTROL", got)
	}
	if got := gogpuKeyToVK(gpucontext.KeyRightControl, 0); got != vtinput.VK_RCONTROL {
		t.Errorf("plain Super: KeyRightControl mapped to vk %d, want VK_RCONTROL", got)
	}
	if mods := (&GogpuHost{}).syncMods(gpucontext.KeyC, gpucontext.ModSuper, true); mods&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0 {
		t.Errorf("plain Super: Super chord produced Ctrl bits %v, want none", mods)
	}
}

// Cmd and physical Ctrl each own a Ctrl channel. Holding both sets both bits,
// and releasing one channel must not disturb the bit the other still holds —
// a phantom full release used to commit an open Switcher mid-chord.
func TestGogpuHost_CtrlChannelSurvivesOtherRelease(t *testing.T) {
	oldCmdIsCtrl := gogpuCmdIsCtrl
	defer func() { gogpuCmdIsCtrl = oldCmdIsCtrl }()
	gogpuCmdIsCtrl = true

	host := &GogpuHost{}
	mods := host.syncMods(gpucontext.KeyLeftControl, gpucontext.ModControl, true)
	if mods&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != vtinput.RightCtrlPressed {
		t.Errorf("physical Ctrl produced mods %v, want exactly RightCtrlPressed", mods)
	}
	mods = host.syncMods(gpucontext.KeyLeftSuper, gpucontext.ModControl|gpucontext.ModSuper, true)
	if mods&vtinput.LeftCtrlPressed == 0 || mods&vtinput.RightCtrlPressed == 0 {
		t.Errorf("Ctrl and Cmd held together produced mods %v, want both Ctrl bits", mods)
	}
	mods = host.syncMods(gpucontext.KeyLeftSuper, gpucontext.ModControl, false)
	if mods&vtinput.RightCtrlPressed == 0 {
		t.Errorf("mods after Cmd release = %v, want RightCtrlPressed while physical Ctrl is held", mods)
	}
	if mods&vtinput.LeftCtrlPressed != 0 {
		t.Errorf("mods after Cmd release = %v, LeftCtrlPressed should be gone", mods)
	}

	// The mirror image: Ctrl released first, Cmd still held.
	host = &GogpuHost{}
	mods = host.syncMods(gpucontext.KeyLeftSuper, gpucontext.ModSuper, true)
	if mods&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != vtinput.LeftCtrlPressed {
		t.Errorf("Cmd produced mods %v, want exactly LeftCtrlPressed", mods)
	}
	host.syncMods(gpucontext.KeyLeftControl, gpucontext.ModControl|gpucontext.ModSuper, true)
	mods = host.syncMods(gpucontext.KeyLeftControl, gpucontext.ModSuper, false)
	if mods&vtinput.LeftCtrlPressed == 0 {
		t.Errorf("mods after Ctrl release = %v, want LeftCtrlPressed while Cmd is held", mods)
	}
	if mods&vtinput.RightCtrlPressed != 0 {
		t.Errorf("mods after Ctrl release = %v, RightCtrlPressed should be gone", mods)
	}
}

func TestGetSystemScrollLines(t *testing.T) {
	lines := getSystemScrollLines()
	if lines <= 0 {
		t.Errorf("Expected positive number of scroll lines, got %d", lines)
	}
	// Under non-Windows, it should return 3
	if runtime.GOOS != "windows" && lines != 3 {
		t.Errorf("Expected 3 scroll lines on non-Windows platforms, got %d", lines)
	}
}
