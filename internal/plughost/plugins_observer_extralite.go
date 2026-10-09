//go:build extralite

package plughost

// optionalObserverPlugins is empty in the extra-lite profile (unxed/f4#1671):
// Observer modules are WebAssembly and the profile carries no wasm runtime.
func optionalObserverPlugins() []Plugin { return nil }
