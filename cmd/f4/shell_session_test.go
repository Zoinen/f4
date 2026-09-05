package main

import (
	"testing"
	"time"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestPanelsFrame_ExitResetsLocalShell(t *testing.T) {
	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	oldSpawn := spawnLocalShellPTY
	oldFactory := newLocalPTY
	t.Cleanup(func() {
		spawnLocalShellPTY = oldSpawn
		newLocalPTY = oldFactory
	})

	oldPTY := pf.localPTY().(*mockPty)
	spawnLocalShellPTY = true
	created := make(chan *mockPty, 1)
	newLocalPTY = func() (PtyBackend, error) {
		pty := &mockPty{}
		created <- pty
		return pty, nil
	}

	pf.cmdLine.Edit.SetText("exit")
	pressKey(pf, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	if !oldPTY.IsClosed() {
		t.Fatal("exit did not close the old local shell")
	}

	var replacement *mockPty
	select {
	case replacement = <-created:
	case <-time.After(time.Second):
		t.Fatal("exit did not start a replacement local shell")
	}

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if pf.localPTY() == replacement {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("replacement local shell was not published to the panels frame")
}

func TestPanelsFrame_ExitF4RequestsApplicationQuit(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldConfirm := AppConfig.ConfirmExit
	AppConfig.ConfirmExit = true
	t.Cleanup(func() { AppConfig.ConfirmExit = oldConfirm })

	pf := setupMockPanelsFrame(t)
	defer pf.Close()
	pty := pf.localPTY().(*mockPty)
	pf.cmdLine.Edit.SetText("exit f4")

	pressKey(pf, &vtinput.InputEvent{
		Type:           vtinput.KeyEventType,
		KeyDown:        true,
		VirtualKeyCode: vtinput.VK_RETURN,
	})

	top := vtui.FrameManager.GetTopFrame()
	if top == nil || top.GetTitle() != Msg("Quit.Title") {
		t.Fatalf("exit f4 did not open the application quit confirmation: top=%T title=%q", top, func() string {
			if top == nil {
				return ""
			}
			return top.GetTitle()
		}())
	}
	if got := pty.String(); got != "" {
		t.Fatalf("exit f4 was sent to the shell instead of requesting f4 quit: %q", got)
	}

	if dialog, ok := top.(*vtui.Window); ok && dialog.OnResult != nil {
		dialog.OnResult(1)
	}
}
