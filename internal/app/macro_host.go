package app

import (
	"context"
	"fmt"
	"github.com/unxed/f4/internal/panel"
	"path/filepath"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/toast"
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

// f4MacroHost is the real macro.MacroHost, bound to f4's panels and screen.
type F4MacroHost struct{}

func (F4MacroHost) CurrentArea() string {
	return onUI(func() string {
		if macro.MacroMgr == nil {
			return "Common"
		}
		return macroCurrentArea()
	})
}

func (F4MacroHost) Panel(active bool) macro.MacroPanelInfo {
	return onUI(func() (info macro.MacroPanelInfo) {
		// Panel contents are replaced wholesale by directory reads. Today
		// those land on this goroutine, but a future background reader would
		// not, and a macro is not worth a crash: report what is safe.
		defer func() {
			if recovered := recover(); recovered != nil {
				vtui.DebugLog("MACRO: panel state unavailable: %v", recovered)
			}
		}()

		frame := panel.FindPanelsFrame()
		if frame == nil {
			return macro.MacroPanelInfo{}
		}

		index := frame.ActiveIdx
		if !active {
			index = 1 - index
		}

		info = macro.MacroPanelInfo{
			Left:    index == 0,
			Visible: frame.ShowPanels,
		}

		pnl, ok := frame.Panels[index].(*panel.FileSystemPanel)
		if !ok {
			return info
		}

		info.Path = pnl.Vfs.GetPath()
		if pnl.IsLoading {
			// Mid-read the entry list means nothing: its length and its
			// contents belong to different directories.
			return info
		}

		// One read of the slice header, so length and indexing below cannot
		// disagree even if the field is reassigned underneath.
		entries := pnl.Entries

		info.ItemCount = len(entries)
		info.SelCount = len(pnl.GetSelectedNames())
		info.Current = pnl.GetSelectedName()

		cursor := pnl.GetCursorIndex()
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

func (F4MacroHost) CommandLine() string {
	return onUI(func() string {
		frame := panel.FindPanelsFrame()
		if frame == nil || frame.CmdLine == nil {
			return ""
		}
		return frame.CmdLine.Edit.GetText()
	})
}

func (F4MacroHost) ScreenSize() (int, int) {
	type size struct{ width, height int }
	got := onUI(func() size {
		frame := panel.FindPanelsFrame()
		if frame == nil {
			return size{}
		}
		return size{frame.LastW, frame.LastH}
	})
	return got.width, got.height
}

func (F4MacroHost) Version() string {
	return getShortVersionInfo()
}

func (F4MacroHost) WindowTitle() string {
	return onUI(currentWindowTitle)
}

func (F4MacroHost) Message(title, text string) {
	if vtui.FrameManager == nil {
		return
	}
	vtui.FrameManager.PostTask(func() {
		vtui.ShowMessage(title, text, []string{"&Ok"})
	})
}

func (F4MacroHost) InjectKeys(keys []*vtinput.InputEvent) {
	if vtui.FrameManager == nil || len(keys) == 0 {
		return
	}
	vtui.FrameManager.PostTask(func() {
		vtui.FrameManager.InjectEvents(keys)
	})
}

func (F4MacroHost) Log(format string, args ...any) {
	vtui.DebugLog(format, args...)
}
func (F4MacroHost) RunAction(name string) bool {
	return onUI(func() bool {
		return RunAction(name)
	})
}

func (F4MacroHost) CallPlugin(ctx context.Context, id string, args []any) ([]any, error) {
	callContext := onUI(func() (snapshot vfs.MacroCallContext) {
		frame := panel.FindPanelsFrame()
		if frame == nil {
			return snapshot
		}
		pnl := frame.GetActivePanel()
		if pnl == nil || pnl.Vfs == nil {
			return snapshot
		}
		dir := pnl.Vfs.GetPath()
		name := pnl.GetSelectedName()
		path := ""
		if name != "" && name != ".." {
			path = pnl.Vfs.Join(dir, name)
		}
		snapshot.Current = vfs.FileRef{VFS: pnl.Vfs, Dir: dir, Name: name, Path: path}
		return snapshot
	})
	return macro.DispatchMacroPluginCall(ctx, id, callContext, args)
}

func actionReloadLuaMacros() bool {
	if macro.MacroMgr == nil {
		return false
	}
	dir := filepath.Join(config.GetF4ConfigDir(), "Macros", "scripts")
	count, err := macro.MacroMgr.ReloadLuaMacros(F4MacroHost{}, dir)
	if err != nil {
		vtui.DebugLog("MACRO: reload: %v", err)
		toast.Show(fmt.Sprintf("%s (%d loaded)", i18n.Msg("Macro.ReloadFailed"), count), 3*time.Second)
		return true
	}
	toast.Show(fmt.Sprintf(i18n.Msg("Macro.Reloaded"), count), 3*time.Second)
	return true
}
