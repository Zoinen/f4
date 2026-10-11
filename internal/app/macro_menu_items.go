package app

import (
	"fmt"
	"sync"

	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/vfs"
)

// MenuItem{} declarations of Lua macro scripts (f4#1686) show up where plugin
// commands do: those for "Plugins" in the F11 menu and the command palette,
// those for "Config" in the plugin configuration menu. They are registered
// as plugin commands, which can be taken back, so a reload of the macros
// replaces them instead of adding them a second time. "Disks" has no menu in
// f4 to show them in and is left out. CommandLine{} prefixes are registered
// the same way, as command-line prefixes, and taken back with them.

var (
	macroMenuMu   sync.Mutex
	macroMenuRegs []vfs.Registration
)

// syncMacroMenuItems replaces the registered menu entries with those of the
// engine now current (nil: none).
func syncMacroMenuItems(engine *macro.LuaMacroEngine) {
	macroMenuMu.Lock()
	defer macroMenuMu.Unlock()
	for _, reg := range macroMenuRegs {
		reg.Unregister()
	}
	macroMenuRegs = nil
	if engine == nil {
		return
	}
	for _, line := range engine.CommandLinePrefixes() {
		id, prefix := line.ID, line.Prefix
		reg, err := (&coreAPI{}).RegisterCommandPrefix(fmt.Sprintf("macro.cmdline.%d.%s", id, prefix), prefix,
			func(_ vfs.App, argument string) { engine.RunCommandLine(id, prefix, argument) })
		if err == nil {
			macroMenuRegs = append(macroMenuRegs, reg)
		}
	}
	for _, menu := range []struct {
		name     string
		location vfs.PluginCommandLocation
	}{{"plugins", vfs.PluginCommandPanel}, {"config", vfs.PluginCommandConfig}} {
		menuName, location := menu.name, menu.location
		for _, item := range engine.MenuItems(menuName, "") {
			id := item.ID
			reg, err := plughost.RegisterPluginCommand(vfs.PluginCommand{
				ID:          fmt.Sprintf("macro.menuitem.%s.%d", menuName, id),
				Location:    location,
				Label:       item.Description,
				Description: item.Source,
				Visible: func(vfs.App) bool {
					for _, shown := range engine.MenuItems(menuName, macroCurrentArea()) {
						if shown.ID == id {
							return true
						}
					}
					return false
				},
				Run: func(vfs.App) { engine.RunMenuItem(id, menuName, macroCurrentArea()) },
			})
			if err == nil {
				macroMenuRegs = append(macroMenuRegs, reg)
			}
		}
	}
}
