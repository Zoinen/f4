package macro

import (
	"fmt"
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
	return count, loadErr
}
