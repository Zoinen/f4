package app

import (
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/navtrace"
	"github.com/unxed/vtinput"
)

func applicationActivationFilter(next func(*vtinput.InputEvent) bool) func(*vtinput.InputEvent) bool {
	focused := true // Startup already loaded the macros.
	return func(event *vtinput.InputEvent) bool {
		if event != nil && event.Type == vtinput.FocusEventType {
			if event.SetFocus && !focused && macro.MacroMgr != nil {
				macro.MacroMgr.Load()
				navtrace.NavigationBenchmarkUIEvent("application.activation.macros_reload")
			}
			focused = event.SetFocus
		}
		return next != nil && next(event)
	}
}
