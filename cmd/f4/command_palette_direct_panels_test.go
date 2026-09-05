package main

import (
	"context"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type directPaletteRemoteVFS struct {
	vfs.VFS
	interrupt []byte
}

func (*directPaletteRemoteVFS) PtyChangeDirCommand(string) []byte { return nil }
func (*directPaletteRemoteVFS) PtyRunCommand(string, string) []byte {
	return nil
}
func (remote *directPaletteRemoteVFS) PtyInterrupt() []byte {
	return append([]byte(nil), remote.interrupt...)
}
func (*directPaletteRemoteVFS) PtyInitSequence() []byte { return nil }

type directPalettePTY struct{ writes []byte }

func (*directPalettePTY) Read([]byte) (int, error) { return 0, nil }
func (pty *directPalettePTY) Write(data []byte) (int, error) {
	pty.writes = append(pty.writes, data...)
	return len(data), nil
}
func (*directPalettePTY) Close() error                { return nil }
func (*directPalettePTY) SetSize(int, int)            {}
func (*directPalettePTY) Wait() error                 { return nil }
func (*directPalettePTY) Run(string, ...string) error { return nil }
func (*directPalettePTY) IsBusy() bool                { return true }

func newDirectPalettePanelsFrame(left, right *FileSystemPanel) *PanelsFrame {
	return &PanelsFrame{
		panels:         [2]Panel{left, right},
		activeIdx:      0,
		showPanels:     true,
		showLeftPanel:  true,
		showRightPanel: true,
		cmdLine:        NewCommandLine("$ "),
		termView:       NewTerminalView(80, 24),
	}
}

func TestCommandPaletteRemoteInterruptHonorsPluginPriorityAndStalePTY(t *testing.T) {
	oldHotkeys := GlobalHotkeys
	GlobalHotkeys = nil
	t.Cleanup(func() { GlobalHotkeys = oldHotkeys })

	remote := &directPaletteRemoteVFS{VFS: vfs.NewNullVFS(0), interrupt: []byte{0x03}}
	left := &FileSystemPanel{vfs: remote}
	right := &FileSystemPanel{vfs: vfs.NewNullVFS(0)}
	pty := &directPalettePTY{}
	pf := newDirectPalettePanelsFrame(left, right)
	pf.remotePtys = map[vfs.VFS]PtyBackend{remote: pty}
	setDirectPaletteTopFrame(t, pf)

	pluginCalls := 0
	RegisterGlobalHotkey(vtinput.VK_C, vtinput.LeftCtrlPressed, func(vfs.App) { pluginCalls++ })
	ctrlC := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_C,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}
	if !pf.InterceptPluginKey(ctrlC) || pluginCalls != 1 || len(pty.writes) != 0 {
		t.Fatalf("plugin priority = calls %d, PTY writes %v", pluginCalls, pty.writes)
	}
	GlobalHotkeys = nil
	if !pf.InterceptPluginKey(ctrlC) || string(pty.writes) != string([]byte{0x03}) {
		t.Fatalf("remote Ctrl+C fallback writes = %v", pty.writes)
	}

	entry, found := commandPaletteTestEntryByID(commandPalettePanelsContextEntries(pf), "Panel.InterruptRemoteCommand")
	if !found {
		t.Fatal("Panel.InterruptRemoteCommand is missing for a live remote PTY")
	}
	replacement := &directPalettePTY{}
	pf.remotePtys[remote] = replacement
	if executeCommandPaletteEntry(entry) || len(replacement.writes) != 0 {
		t.Fatal("stale remote interrupt targeted a replacement PTY")
	}
	entry, _ = commandPaletteTestEntryByID(commandPalettePanelsContextEntries(pf), "Panel.InterruptRemoteCommand")
	if !executeCommandPaletteEntry(entry) || string(replacement.writes) != string([]byte{0x03}) {
		t.Fatalf("palette remote interrupt writes = %v", replacement.writes)
	}
}

func TestCommandPaletteFastFindSurvivesOpeningAndTogglesMatchMode(t *testing.T) {
	_, panel := newFastFindPanelsFrame(t)
	original := panel.fastFindStr
	if !RunAction(commandPaletteActionName) {
		t.Fatal("App.CommandPalette was not handled")
	}
	dialog, ok := vtui.FrameManager.GetTopFrame().(*commandPaletteDialog)
	if !ok {
		t.Fatalf("top frame = %T, want command palette", vtui.FrameManager.GetTopFrame())
	}
	entry, found := commandPaletteTestEntryByID(dialog.entries, "FastFind.ToggleMatchMode")
	if !found {
		t.Fatal("palette opened from Fast Find without FastFind.ToggleMatchMode")
	}
	if !panel.fastFindMode || panel.fastFindStr != original {
		t.Fatalf("opening palette canceled Fast Find: mode=%v query=%q", panel.fastFindMode, panel.fastFindStr)
	}

	vtui.FrameManager.Pop()
	if !executeCommandPaletteEntry(entry) || panel.fastFindStr != "*"+original {
		t.Fatalf("FastFind.ToggleMatchMode query = %q, want %q", panel.fastFindStr, "*"+original)
	}

	ordinaryRan := false
	if !executeCommandPaletteEntry(commandPaletteEntry{ID: "Dynamic.Ordinary", run: func() bool {
		ordinaryRan = true
		return true
	}}) || !ordinaryRan {
		t.Fatal("ordinary dynamic palette command did not run")
	}
	if panel.fastFindMode {
		t.Fatal("ordinary dynamic palette command left Fast Find active")
	}
}

func TestCommandPalettePendingProviderCancelRevalidatesTask(t *testing.T) {
	pf, panel, _ := newSearchFirstTestFrame(t)
	setDirectPaletteTopFrame(t, pf)

	newTask := func() *vtui.TaskContext {
		ctx, cancel := context.WithCancel(context.Background())
		return &vtui.TaskContext{Context: ctx, Cancel: cancel}
	}
	first := newTask()
	panel.providerOpenTask = first
	panel.providerOpenSourceSelect = "source"
	entry, found := commandPaletteTestEntryByID(commandPalettePanelsContextEntries(pf), "Provider.CancelOpen")
	if !found {
		t.Fatal("Provider.CancelOpen is missing while a provider is pending")
	}
	second := newTask()
	panel.providerOpenTask = second
	if executeCommandPaletteEntry(entry) || panel.providerOpenTask != second {
		t.Fatal("stale provider cancel stopped a replacement task")
	}

	entry, _ = commandPaletteTestEntryByID(commandPalettePanelsContextEntries(pf), "Provider.CancelOpen")
	if !executeCommandPaletteEntry(entry) || panel.providerOpenTask != nil {
		t.Fatal("Provider.CancelOpen did not cancel the current task")
	}
}

func TestCommandPaletteSearchFirstFocusToggleIsStateSpecific(t *testing.T) {
	oldMode := AppConfig.NavigationMode
	AppConfig.NavigationMode = NavigationSearchFirst
	t.Cleanup(func() { AppConfig.NavigationMode = oldMode })

	pf, _, _ := newSearchFirstTestFrame(t)
	setDirectPaletteTopFrame(t, pf)
	entry, found := commandPaletteTestEntryByID(commandPalettePanelsContextEntries(pf), "Panel.ToggleCommandLineFocus")
	if !found || entry.Checked || entry.Shortcut != "` / ~ / ё" {
		t.Fatalf("initial focus-toggle entry = %#v", entry)
	}
	if !executeCommandPaletteEntry(entry) || !pf.commandLineFocused {
		t.Fatal("focus-toggle command did not focus the command line")
	}
	if executeCommandPaletteEntry(entry) {
		t.Fatal("stale focus-toggle entry changed a newer focus state")
	}

	entry, _ = commandPaletteTestEntryByID(commandPalettePanelsContextEntries(pf), "Panel.ToggleCommandLineFocus")
	if !entry.Checked || !executeCommandPaletteEntry(entry) || pf.commandLineFocused {
		t.Fatal("fresh focus-toggle command did not restore panel focus")
	}
}

func TestCommandPaletteAISendDraftOnlyForCurrentNonEmptyInput(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	left := NewFileSystemPanel(0, 0, 40, 20, vfs.NewNullVFS(0))
	right := NewFileSystemPanel(40, 0, 80, 20, vfs.NewNullVFS(0))
	waitForLoad(t, left)
	waitForLoad(t, right)
	pf := newDirectPalettePanelsFrame(left, right)
	chat := NewAIChatPanel(left)
	chat.SetFocus(true)
	chat.focusedLinkIdx = -1
	chat.input.SetText("review this patch")
	pf.altPanels[0] = chat
	vtui.FrameManager.Push(pf)

	entry, found := commandPaletteTestEntryByID(commandPalettePanelsContextEntries(pf), "AI.SendDraft")
	if !found || entry.Description != Msg("CommandPalette.AI.SendDraft.Desc") {
		t.Fatalf("AI.SendDraft entry = %#v", entry)
	}
	chat.input.SetText("newer draft")
	if executeCommandPaletteEntry(entry) {
		t.Fatal("stale AI.SendDraft submitted a newer draft")
	}
	chat.input.SetText("   ")
	if commandPaletteTestHasID(commandPalettePanelsContextEntries(pf), "AI.SendDraft") {
		t.Fatal("blank AI draft exposed a submit command")
	}
}
