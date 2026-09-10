package plughost

import (
	"context"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// Application is what the plugin host needs from the application above it.
//
// Two interfaces meet here and they face opposite ways. vfs.HostAPI is what
// the host gives a plugin: public, narrowed by the permission list, reachable
// by any plugin installed from the catalogue. Application is what the
// application gives the host: internal, and no plugin ever sees it.
//
// Nothing declared here may be added to vfs.HostAPI. SetClipboard hands over
// the user's data and SetupUI rebuilds the whole interface; a permission the
// host cannot enforce is worse than no permission at all.
type Application interface {
	// Current returns the object plugin commands and host RPC methods run
	// against — the panels frame — or nil when none is on screen. It is also
	// how the host reaches RunProgressTask and Menu, which vfs.App declares.
	//
	// The lookup walks the frame stack rather than reading the top frame: a
	// menu or the command palette is what sits on top when a plugin command
	// runs.
	Current() vfs.App

	// OpenPanelProvider shows a registered panel provider in the active slot.
	// The host owns the provider registry, because both the application and
	// the panels reach it; opening one is panel work and stays above.
	OpenPanelProvider(app vfs.App, providerID string)

	// IsStale reports that the application has discarded this app object — a
	// panels frame that was closed or replaced — so a command registered
	// against it must not run: its Run closure still holds the old frame. An
	// object the application does not manage is never stale.
	IsStale(app vfs.App) bool

	// SetupUI rebuilds the interface. The external-UI protocol calls it once
	// the host owns the screen.
	SetupUI()

	// SetClipboard writes f4's clipboard, honouring the configured backend.
	SetClipboard(text string)

	// RunSemanticAction executes an action posted over the external-UI
	// protocol and reports whether the screen needs a redraw.
	RunSemanticAction(action map[string]any) bool

	// AskOverwrite asks the user what to do about an existing destination.
	// The returned int is the chosen disposition; remember says the choice
	// applies to the rest of the operation.
	AskOverwrite(ctx context.Context, destPath string, src, dst vfs.VFSItem, anchor vtui.Frame) (choice int, remember bool)

	// AskError asks the user how to continue after a failed operation.
	AskError(ctx context.Context, op string, err error, anchor vtui.Frame) int
}

// App is the live application. The composition root sets it once, before any
// plugin is loaded; it stays nil in tests that never call back into the
// application, so every use here is guarded.
var App Application

// currentApp is App.Current with the nil-host case folded in.
func currentApp() vfs.App {
	if App == nil {
		return nil
	}
	app := App.Current()
	// A typed nil pointer in a vfs.App interface is not == nil, and every
	// caller here tests the result before using it.
	if app == nil {
		return nil
	}
	return app
}
