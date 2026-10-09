package settings

import (
	"context"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtui"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/plughost"
)

// Host supplies application orchestration; the settings package owns drafts and UI.
type Host interface {
	SaveSession() error
	SessionPath() string
	SaveGeometry() error
	ApplyRuntime(config.F4Config, []string)
	CheckUpdates(context.Context) error
	GuiBackends() []string
	PluginPackage(bool, *panel.PanelsFrame, plughost.PlugRingItem, func())
}

var host Host

// Configure is called by the composition root before settings can open.
func Configure(h Host) { host = h }

// HotkeyPageHost embeds the application's existing shortcut configurator.
type HotkeyPageHost interface {
	HotkeyPage(*vtui.Window, func(*keymap.HotkeyManager)) vtui.UIElement
}

// HotkeyTableSizeHost reports how wide the embedded hotkey table's own page
// area needs to be for its columns to stay at their readable minimum.
// Settings uses it, when the Hotkey Configurator is the category about to be
// shown, to size its auto-maximize threshold from the table's own
// column-width logic instead of a guessed terminal-width constant (#1239
// follow-up).
type HotkeyTableSizeHost interface {
	HotkeyTableMinPageWidth() int
}
