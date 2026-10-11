package macro

import (
	"fmt"
	"time"

	"github.com/unxed/vtui"
)

// LoadLuaMacros starts the Far-compatible macro engine and reads dir, which is
// the equivalent of Far's Macros/scripts. A missing directory is not an error:
// most users have no macros, and they should pay nothing for the feature.
func (m *MacroManager) LoadLuaMacros(host MacroHost, dir string) {
	count, err := m.ReloadLuaMacros(host, dir)
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
func (m *MacroManager) ReloadLuaMacros(host MacroHost, dir string) (int, error) {
	m.luaHost, m.luaDir = host, dir
	Engine, err := NewLuaMacroEngine(host)
	if err != nil {
		return 0, fmt.Errorf("cannot start the Lua macro engine: %w", err)
	}
	loadErr := Engine.LoadDir(dir)
	count := Engine.Count()

	old := m.Lua
	if count == 0 {
		m.Lua = nil
		_ = Engine.Close()
	} else {
		m.Lua = Engine
	}
	if old != nil {
		go func() {
			if closeErr := old.Close(); closeErr != nil {
				vtui.DebugLog("MACRO: closing replaced Lua engine: %v", closeErr)
			}
		}()
	}
	if m.OnLuaLoaded != nil {
		m.OnLuaLoaded(m.Lua)
	}
	return count, loadErr
}

// RefreshInterruptedLua replaces a Lua engine whose interpreter hit a call
// deadline -- a macro that never returned -- with a fresh one built from disk.
// The interrupted interpreter stopped at an arbitrary instruction and refuses
// all further work (luaplug.ErrInterrupted), so without this one runaway macro
// would silence every other macro until f4 restarted. It runs on the goroutine
// that dispatches keys, before a key is offered to the engine, so the swap of
// m.Lua races with nothing. It reports whether it rebuilt the engine.
func (m *MacroManager) RefreshInterruptedLua() bool {
	if m == nil || m.Lua == nil || !m.Lua.Interrupted() || m.luaHost == nil {
		return false
	}
	vtui.DebugLog("MACRO: a Lua macro hit its deadline; rebuilding the macro engine")
	count, err := m.ReloadLuaMacros(m.luaHost, m.luaDir)
	if err != nil {
		vtui.DebugLog("MACRO: rebuilding the macro engine: %v", err)
	}
	vtui.DebugLog("MACRO: %d Lua macro(s) loaded after the rebuild", count)
	return true
}

// RunExitEvents raises the ExitFAR group of the Lua macros, giving them a couple
// of seconds in all: f4 is on its way out and a script that does not finish
// does not hold it up.
func (m *MacroManager) RunExitEvents() {
	if m == nil || m.Lua == nil {
		return
	}
	m.Lua.RunEvents("ExitFAR", 2*time.Second)
}

// RaiseEvent raises an Event{} group of the Lua macros in the background (see
// LuaMacroEngine.RaiseEvent); safe on a manager without Lua macros.
func (m *MacroManager) RaiseEvent(group string) bool {
	if m == nil || m.Lua == nil {
		return false
	}
	return m.Lua.RaiseEvent(group)
}

// RaiseEditorEvent raises the EditorEvent of the Lua macros for the editor with
// the given id: event is one of Far's EE_* numbers (read 0, save 1, close 3).
func (m *MacroManager) RaiseEditorEvent(id, event int) bool {
	if m == nil || m.Lua == nil {
		return false
	}
	return m.Lua.RaiseEventNumbers("EditorEvent", id, event, 0)
}

// RaiseViewerEvent raises the ViewerEvent of the Lua macros for the viewer with
// the given id: event is one of Far's VE_* numbers (read 0, close 1).
func (m *MacroManager) RaiseViewerEvent(id, event int) bool {
	if m == nil || m.Lua == nil {
		return false
	}
	return m.Lua.RaiseEventNumbers("ViewerEvent", id, event, 0)
}
