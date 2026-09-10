package app

import (
	"context"
	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/plughost"
	"testing"

	"github.com/unxed/f4/internal/terminal"
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

func newDirectPalettePanelsFrame(left, right *panel.FileSystemPanel) *panel.PanelsFrame {
	return &panel.PanelsFrame{
		Panels:         [2]panel.Panel{left, right},
		ActiveIdx:      0,
		ShowPanels:     true,
		ShowLeftPanel:  true,
		ShowRightPanel: true,
		CmdLine:        cmdline.NewCommandLine("$ "),
		TermView:       terminal.NewTerminalView(80, 24),
	}
}

func TestCommandPaletteRemoteInterruptHonorsPluginPriorityAndStalePTY(t *testing.T) {
	oldHotkeys := plughost.GlobalHotkeys
	plughost.GlobalHotkeys = nil
	t.Cleanup(func() { plughost.GlobalHotkeys = oldHotkeys })

	remote := &directPaletteRemoteVFS{VFS: vfs.NewNullVFS(0), interrupt: []byte{0x03}}
	left := &panel.FileSystemPanel{Vfs: remote}
	right := &panel.FileSystemPanel{Vfs: vfs.NewNullVFS(0)}
	pty := &directPalettePTY{}
	pf := newDirectPalettePanelsFrame(left, right)
	pf.RemotePtys = map[vfs.VFS]terminal.PtyBackend{remote: pty}
	setDirectPaletteTopFrame(t, pf)

	pluginCalls := 0
	plughost.RegisterGlobalHotkey(vtinput.VK_C, vtinput.LeftCtrlPressed, func(vfs.App) { pluginCalls++ })
	ctrlC := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_C,
		ControlKeyState: vtinput.LeftCtrlPressed,
	}
	if !pf.InterceptPluginKey(ctrlC) || pluginCalls != 1 || len(pty.writes) != 0 {
		t.Fatalf("plugin priority = calls %d, term.PTY writes %v", pluginCalls, pty.writes)
	}
	plughost.GlobalHotkeys = nil
	if !pf.InterceptPluginKey(ctrlC) || string(pty.writes) != string([]byte{0x03}) {
		t.Fatalf("remote Ctrl+C fallback writes = %v", pty.writes)
	}

	entry, found := commandPaletteTestEntryByID(commandPalettePanelsContextEntries(pf), "Panel.InterruptRemoteCommand")
	if !found {
		t.Fatal("Panel.InterruptRemoteCommand is missing for a live remote term.PTY")
	}
	replacement := &directPalettePTY{}
	pf.RemotePtys[remote] = replacement
	if executeCommandPaletteEntry(entry) || len(replacement.writes) != 0 {
		t.Fatal("stale remote interrupt targeted a replacement term.PTY")
	}
	entry, _ = commandPaletteTestEntryByID(commandPalettePanelsContextEntries(pf), "Panel.InterruptRemoteCommand")
	if !executeCommandPaletteEntry(entry) || string(replacement.writes) != string([]byte{0x03}) {
		t.Fatalf("palette remote interrupt writes = %v", replacement.writes)
	}
}

func TestCommandPaletteFastFindSurvivesOpeningAndTogglesMatchMode(t *testing.T) {
	_, pnl := newFastFindPanelsFrame(t)
	original := pnl.FastFindStr
	if !RunAction(CommandPaletteActionName) {
		t.Fatal("App.CommandPalette was not handled")
	}
	dialog, ok := vtui.FrameManager.GetTopFrame().(*CommandPaletteDialog)
	if !ok {
		t.Fatalf("top frame = %T, want command palette", vtui.FrameManager.GetTopFrame())
	}
	entry, found := commandPaletteTestEntryByID(dialog.entries, "FastFind.ToggleMatchMode")
	if !found {
		t.Fatal("palette opened from Fast Find without FastFind.ToggleMatchMode")
	}
	if !pnl.FastFindMode || pnl.FastFindStr != original {
		t.Fatalf("opening palette canceled Fast Find: mode=%v query=%q", pnl.FastFindMode, pnl.FastFindStr)
	}

	vtui.FrameManager.Pop()
	if !executeCommandPaletteEntry(entry) || pnl.FastFindStr != "*"+original {
		t.Fatalf("FastFind.ToggleMatchMode query = %q, want %q", pnl.FastFindStr, "*"+original)
	}

	ordinaryRan := false
	if !executeCommandPaletteEntry(commandPaletteEntry{ID: "Dynamic.Ordinary", run: func() bool {
		ordinaryRan = true
		return true
	}}) || !ordinaryRan {
		t.Fatal("ordinary dynamic palette command did not run")
	}
	if pnl.FastFindMode {
		t.Fatal("ordinary dynamic palette command left Fast Find active")
	}
}

func TestCommandPalettePendingProviderCancelRevalidatesTask(t *testing.T) {
	pf, pnl, _ := newSearchFirstTestFrame(t)
	setDirectPaletteTopFrame(t, pf)

	newTask := func() *vtui.TaskContext {
		ctx, cancel := context.WithCancel(context.Background())
		return &vtui.TaskContext{Context: ctx, Cancel: cancel}
	}
	first := newTask()
	pnl.ProviderOpenTask = first
	pnl.ProviderOpenSourceSelect = "source"
	entry, found := commandPaletteTestEntryByID(commandPalettePanelsContextEntries(pf), "Provider.CancelOpen")
	if !found {
		t.Fatal("Provider.CancelOpen is missing while a provider is pending")
	}
	second := newTask()
	pnl.ProviderOpenTask = second
	if executeCommandPaletteEntry(entry) || pnl.ProviderOpenTask != second {
		t.Fatal("stale provider cancel stopped a replacement task")
	}

	entry, _ = commandPaletteTestEntryByID(commandPalettePanelsContextEntries(pf), "Provider.CancelOpen")
	if !executeCommandPaletteEntry(entry) || pnl.ProviderOpenTask != nil {
		t.Fatal("Provider.CancelOpen did not cancel the current task")
	}
}

func TestCommandPaletteSearchFirstFocusToggleIsStateSpecific(t *testing.T) {
	oldMode := config.App.NavigationMode
	config.App.NavigationMode = config.NavigationSearchFirst
	t.Cleanup(func() { config.App.NavigationMode = oldMode })

	pf, _, _ := newSearchFirstTestFrame(t)
	setDirectPaletteTopFrame(t, pf)
	entry, found := commandPaletteTestEntryByID(commandPalettePanelsContextEntries(pf), "Panel.ToggleCommandLineFocus")
	if !found || entry.Checked || entry.Shortcut != "` / ~ / ё" {
		t.Fatalf("initial focus-toggle entry = %#v", entry)
	}
	if !executeCommandPaletteEntry(entry) || !pf.CommandLineFocused {
		t.Fatal("focus-toggle command did not focus the command line")
	}
	if executeCommandPaletteEntry(entry) {
		t.Fatal("stale focus-toggle entry changed a newer focus state")
	}

	entry, _ = commandPaletteTestEntryByID(commandPalettePanelsContextEntries(pf), "Panel.ToggleCommandLineFocus")
	if !entry.Checked || !executeCommandPaletteEntry(entry) || pf.CommandLineFocused {
		t.Fatal("fresh focus-toggle command did not restore panel focus")
	}
}

func TestCommandPaletteAISendDraftOnlyForCurrentNonEmptyInput(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	left := panel.NewFileSystemPanel(0, 0, 40, 20, vfs.NewNullVFS(0))
	right := panel.NewFileSystemPanel(40, 0, 80, 20, vfs.NewNullVFS(0))
	paneltest.WaitForLoad(t, left)
	paneltest.WaitForLoad(t, right)
	pf := newDirectPalettePanelsFrame(left, right)
	chat := NewAIChatPanel(left)
	chat.SetFocus(true)
	chat.focusedLinkIdx = -1
	chat.input.SetText("review this patch")
	pf.AltPanels[0] = chat
	vtui.FrameManager.Push(pf)

	entry, found := commandPaletteTestEntryByID(commandPalettePanelsContextEntries(pf), "AI.SendDraft")
	if !found || entry.Description != i18n.Msg("CommandPalette.AI.SendDraft.Desc") {
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
