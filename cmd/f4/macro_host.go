package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// uiDeadline bounds a round trip to the UI goroutine, so a macro cannot hang
// forever if the UI is busy or gone.
const uiDeadline = 5 * time.Second

// onUI runs fn on the UI goroutine and returns its result.
//
// Macros run on their own goroutine precisely so that this is safe: the UI
// goroutine is free to answer. Calling it from the UI goroutine would
// deadlock, which is why nothing in the macro path does.
func onUI[T any](fn func() T) T {
	var zero T
	if vtui.FrameManager == nil {
		return zero
	}
	result := make(chan T, 1)
	vtui.FrameManager.PostTask(func() {
		result <- fn()
	})
	select {
	case value := <-result:
		return value
	case <-time.After(uiDeadline):
		return zero
	}
}

// f4MacroHost is the real MacroHost, bound to f4's panels and screen.
type f4MacroHost struct{}

func (f4MacroHost) CurrentArea() string {
	return onUI(func() string {
		if MacroMgr == nil {
			return "Common"
		}
		return MacroMgr.GetCurrentArea()
	})
}

func (f4MacroHost) Panel(active bool) MacroPanelInfo {
	return onUI(func() (info MacroPanelInfo) {
		// Panel contents are replaced wholesale by directory reads. Today
		// those land on this goroutine, but a future background reader would
		// not, and a macro is not worth a crash: report what is safe.
		defer func() {
			if recovered := recover(); recovered != nil {
				vtui.DebugLog("MACRO: panel state unavailable: %v", recovered)
			}
		}()

		frame := findPanelsFrame()
		if frame == nil {
			return MacroPanelInfo{}
		}

		index := frame.activeIdx
		if !active {
			index = 1 - index
		}

		info = MacroPanelInfo{
			Left:    index == 0,
			Visible: frame.showPanels,
		}

		panel, ok := frame.panels[index].(*FileSystemPanel)
		if !ok {
			return info
		}

		info.Path = panel.vfs.GetPath()
		if panel.isLoading {
			// Mid-read the entry list means nothing: its length and its
			// contents belong to different directories.
			return info
		}

		// One read of the slice header, so length and indexing below cannot
		// disagree even if the field is reassigned underneath.
		entries := panel.entries

		info.ItemCount = len(entries)
		info.SelCount = len(panel.GetSelectedNames())
		info.Current = panel.GetSelectedName()

		cursor := panel.GetCursorIndex()
		info.CurPos = cursor + 1
		if cursor >= 0 && cursor < len(entries) && entries[cursor] != nil {
			info.IsFolder = entries[cursor].IsDir
		}

		info.Empty = info.ItemCount == 0
		info.Bof = info.CurPos <= 1
		info.Eof = info.CurPos >= info.ItemCount
		info.Root = info.Path == "" || filepath.Dir(info.Path) == info.Path
		return info
	})
}

func (f4MacroHost) CommandLine() string {
	return onUI(func() string {
		frame := findPanelsFrame()
		if frame == nil || frame.cmdLine == nil {
			return ""
		}
		return frame.cmdLine.Edit.GetText()
	})
}

func (f4MacroHost) ScreenSize() (int, int) {
	type size struct{ width, height int }
	got := onUI(func() size {
		frame := findPanelsFrame()
		if frame == nil {
			return size{}
		}
		return size{frame.lastW, frame.lastH}
	})
	return got.width, got.height
}

func (f4MacroHost) Version() string {
	return getShortVersionInfo()
}

func (f4MacroHost) WindowTitle() string {
	return onUI(currentWindowTitle)
}

func (f4MacroHost) Message(title, text string) {
	if vtui.FrameManager == nil {
		return
	}
	vtui.FrameManager.PostTask(func() {
		vtui.ShowMessage(title, text, []string{"&Ok"})
	})
}

func (f4MacroHost) InjectKeys(keys []*vtinput.InputEvent) {
	if vtui.FrameManager == nil || len(keys) == 0 {
		return
	}
	vtui.FrameManager.PostTask(func() {
		vtui.FrameManager.InjectEvents(keys)
	})
}

func (f4MacroHost) Log(format string, args ...any) {
	vtui.DebugLog(format, args...)
}
func (f4MacroHost) RunAction(name string) bool {
	return onUI(func() bool {
		return RunAction(name)
	})
}

func (f4MacroHost) CallPlugin(ctx context.Context, id string, args []any) ([]any, error) {
	callContext := onUI(func() (snapshot vfs.MacroCallContext) {
		frame := findPanelsFrame()
		if frame == nil {
			return snapshot
		}
		panel := frame.getActivePanel()
		if panel == nil || panel.vfs == nil {
			return snapshot
		}
		dir := panel.vfs.GetPath()
		name := panel.GetSelectedName()
		path := ""
		if name != "" && name != ".." {
			path = panel.vfs.Join(dir, name)
		}
		snapshot.Current = vfs.FileRef{VFS: panel.vfs, Dir: dir, Name: name, Path: path}
		return snapshot
	})
	return dispatchMacroPluginCall(ctx, id, callContext, args)
}

// LoadLuaMacros starts the Far-compatible macro engine and reads dir, which is
// the equivalent of Far's Macros/scripts. A missing directory is not an error:
// most users have no macros, and they should pay nothing for the feature.
func (m *MacroManager) LoadLuaMacros(dir string) {
	count, err := m.ReloadLuaMacros(dir)
	if err != nil {
		vtui.DebugLog("MACRO: %v", err)
	}
	if count > 0 {
		vtui.DebugLog("MACRO: loaded %d Lua macro(s) from %s", count, dir)
	}
}

// ReloadLuaMacros builds a fresh interpreter from disk, then swaps it in as a
// single pointer update. A macro already running on the old interpreter is
// allowed to finish; closing that interpreter happens asynchronously so a
// reload cannot deadlock while the old macro is waiting for the UI goroutine.
func (m *MacroManager) ReloadLuaMacros(dir string) (int, error) {
	engine, err := NewLuaMacroEngine(f4MacroHost{})
	if err != nil {
		return 0, fmt.Errorf("cannot start the Lua macro engine: %w", err)
	}
	loadErr := engine.LoadDir(dir)
	count := engine.Count()

	old := m.Lua
	if count == 0 {
		m.Lua = nil
		_ = engine.Close()
	} else {
		m.Lua = engine
	}
	if old != nil {
		go func() {
			if closeErr := old.Close(); closeErr != nil {
				vtui.DebugLog("MACRO: closing replaced Lua engine: %v", closeErr)
			}
		}()
	}
	return count, loadErr
}

func actionReloadLuaMacros() bool {
	if MacroMgr == nil {
		return false
	}
	dir := filepath.Join(GetF4ConfigDir(), "Macros", "scripts")
	count, err := MacroMgr.ReloadLuaMacros(dir)
	if err != nil {
		vtui.DebugLog("MACRO: reload: %v", err)
		showToast(fmt.Sprintf("%s (%d loaded)", Msg("Macro.ReloadFailed"), count), 3*time.Second)
		return true
	}
	showToast(fmt.Sprintf(Msg("Macro.Reloaded"), count), 3*time.Second)
	return true
}
