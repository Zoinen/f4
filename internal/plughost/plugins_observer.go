//go:build !extralite

package plughost

import (
	"github.com/unxed/f4/internal/config"
	observerplugin "github.com/unxed/f4/plugins/observer"
)

// optionalObserverPlugins is the Observer module provider (f4#1563): an
// Observer module is a wazero wasm reactor, so the extra-lite profile leaves
// it out (plugins_observer_extralite.go).
func optionalObserverPlugins() []Plugin {
	return []Plugin{observerplugin.NewPlugin(config.GetF4ConfigDir())}
}
