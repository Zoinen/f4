package plughost

import (
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

// rpcPanelKeysTransport answers like a plugin that declares keys: the open
// answer carries openKeys and every event answer carries eventKeys.
type rpcPanelKeysTransport struct {
	calls    []string
	events   []RPCPanelEventRequest
	openKeys *RPCPanelOpenResponse
	eventRes RPCPanelEventResponse
}

func (t *rpcPanelKeysTransport) Call(method string, params, result any) error {
	t.calls = append(t.calls, method)
	switch method {
	case "Plugin.OpenPanel":
		*(result.(*RPCPanelOpenResponse)) = *t.openKeys
	case "Plugin.PanelEvent":
		t.events = append(t.events, params.(RPCPanelEventRequest))
		*(result.(*RPCPanelEventResponse)) = t.eventRes
	}
	return nil
}

func openRPCPanelForKeys(t *testing.T, transport *rpcPanelKeysTransport) vfs.PanelController {
	t.Helper()
	host := &rpcPanelCoverageHost{luaTestHostAPI: newLuaTestHostAPI()}
	registrations := &PluginSessionRegistrations{}
	t.Cleanup(registrations.Unregister)
	if err := RegisterRPCPluginPanels(host, transport, "plugin", []PluginPanelDescriptor{{ID: "keys", Title: "Keys"}}, registrations); err != nil {
		t.Fatalf("register panel: %v", err)
	}
	controller, err := host.panels[0].Open(vfs.PanelContext{})
	if err != nil {
		t.Fatalf("open panel: %v", err)
	}
	return controller
}

func TestRPCPanelKeysAreCachedAndRunThroughPanelEvent(t *testing.T) {
	transport := &rpcPanelKeysTransport{
		openKeys: &RPCPanelOpenResponse{
			Document: []byte(`{"vuiVersion":1,"root":{"type":"Window"}}`),
			HasKeys:  true,
			Keys: []RPCPanelKey{
				{VK: vtinput.VK_F5, Label: " Count "},
				{VK: 0, Label: "no key code"},
				{VK: vtinput.VK_F8, Mods: uint32(vtinput.ShiftPressed), Label: "Reset", Disabled: true},
			},
		},
	}
	controller := openRPCPanelForKeys(t, transport)
	provider, ok := controller.(vfs.PanelKeyProvider)
	if !ok {
		t.Fatal("RPC panel does not implement vfs.PanelKeyProvider")
	}
	keys := provider.PanelKeys()
	if len(keys) != 2 || keys[0].Label != "Count" || keys[1].VK != vtinput.VK_F8 {
		t.Fatalf("declared keys = %+v", keys)
	}
	if keys[0].Enabled != nil || keys[1].Enabled == nil || keys[1].Enabled() {
		t.Fatal("Disabled did not map to Enabled")
	}
	callsAfterOpen := len(transport.calls)
	for i := 0; i < 10; i++ {
		_ = provider.PanelKeys()
	}
	if len(transport.calls) != callsAfterOpen {
		t.Fatal("PanelKeys reached the plugin; it must be answered from the host's cache")
	}

	// A disabled key is consumed by the host without a call.
	shiftF8 := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F8,
		ControlKeyState: vtinput.ShiftPressed}
	if !vfs.DispatchPanelKey(provider.PanelKeys(), shiftF8) || len(transport.events) != 0 {
		t.Fatalf("disabled key: events %+v", transport.events)
	}

	// An enabled key is sent as a key event; the answer may re-declare keys.
	transport.eventRes = RPCPanelEventResponse{Handled: true, HasKeys: true, Keys: []RPCPanelKey{
		{VK: vtinput.VK_F5, Label: "Count"},
		{VK: vtinput.VK_F8, Mods: uint32(vtinput.ShiftPressed), Label: "Reset"},
	}}
	f5 := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F5}
	if !vfs.DispatchPanelKey(provider.PanelKeys(), f5) {
		t.Fatal("F5 was not dispatched")
	}
	if len(transport.events) != 1 || transport.events[0].Kind != "key" || transport.events[0].ID != "keys" ||
		!transport.events[0].Event.KeyDown || transport.events[0].Event.VirtualKeyCode != vtinput.VK_F5 {
		t.Fatalf("F5 event = %+v", transport.events)
	}
	keys = provider.PanelKeys()
	if len(keys) != 2 || keys[1].Enabled != nil {
		t.Fatalf("key set was not refreshed from the event answer: %+v", keys)
	}
	if !vfs.DispatchPanelKey(keys, shiftF8) || len(transport.events) != 2 {
		t.Fatal("the now-enabled Shift+F8 did not reach the plugin")
	}

	// An answer without HasKeys (any older plugin) keeps the set; one with
	// HasKeys and no keys clears it.
	transport.eventRes = RPCPanelEventResponse{Handled: true}
	controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if len(provider.PanelKeys()) != 2 {
		t.Fatal("an answer without a declaration dropped the key set")
	}
	transport.eventRes = RPCPanelEventResponse{Handled: true, HasKeys: true}
	controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	if keys := provider.PanelKeys(); keys != nil {
		t.Fatalf("an empty declaration did not clear the key set: %+v", keys)
	}

	if err := controller.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if provider.PanelKeys() != nil {
		t.Fatal("a closed panel still declares keys")
	}
}

func TestRPCPanelWithoutKeyDeclarationBehavesAsBefore(t *testing.T) {
	transport := &rpcPanelKeysTransport{
		openKeys: &RPCPanelOpenResponse{Document: []byte(`{"vuiVersion":1,"root":{"type":"Window"}}`)},
		eventRes: RPCPanelEventResponse{Handled: true},
	}
	controller := openRPCPanelForKeys(t, transport)
	if keys := controller.(vfs.PanelKeyProvider).PanelKeys(); keys != nil {
		t.Fatalf("a plugin that declares nothing got keys: %+v", keys)
	}
	if !controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F5}) {
		t.Fatal("F5 did not reach the plugin through ProcessKey")
	}
}

func TestRPCPanelKeyDeclarationIsCapped(t *testing.T) {
	panel := &rpcVUIPanel{}
	declared := make([]RPCPanelKey, maxRPCPanelKeys+10)
	for i := range declared {
		declared[i] = RPCPanelKey{VK: uint16(0x41 + i%26), Mods: uint32(i / 26)}
	}
	if !panel.setKeys(declared) || len(panel.PanelKeys()) != maxRPCPanelKeys {
		t.Fatalf("keys = %d, want %d", len(panel.PanelKeys()), maxRPCPanelKeys)
	}
	if panel.setKeys(declared) {
		t.Fatal("an unchanged declaration was reported as a change")
	}
}

func TestRPCPanelHelpKeyFromDescriptor(t *testing.T) {
	transport := &rpcPanelKeysTransport{
		openKeys: &RPCPanelOpenResponse{
			Document: []byte(`{"vuiVersion":1,"root":{"type":"Window"}}`),
			HasKeys:  true,
			Keys:     []RPCPanelKey{{VK: vtinput.VK_F5, Label: "Run"}},
		},
	}
	host := &rpcPanelCoverageHost{luaTestHostAPI: newLuaTestHostAPI()}
	registrations := &PluginSessionRegistrations{}
	t.Cleanup(registrations.Unregister)
	descriptors := []PluginPanelDescriptor{{ID: "helped", Title: "Helped", Help: "# Help", LocalizedHelp: map[string]string{"xx": "# Aide"}}}
	if err := RegisterRPCPluginPanels(host, transport, "plugin", descriptors, registrations); err != nil {
		t.Fatalf("register panel: %v", err)
	}
	controller, err := host.panels[0].Open(vfs.PanelContext{})
	if err != nil {
		t.Fatalf("open panel: %v", err)
	}
	panel := controller.(*rpcVUIPanel)
	keys := panel.PanelKeys()
	if len(keys) != 2 || keys[0].VK != vtinput.VK_F1 || keys[1].VK != vtinput.VK_F5 {
		t.Fatalf("keys = %+v, want the help key ahead of the declared F5", keys)
	}
	// A declared plain F1 keeps the key.
	panel.setKeys([]RPCPanelKey{{VK: vtinput.VK_F1, Label: "Own"}})
	if keys := panel.PanelKeys(); len(keys) != 1 || keys[0].Label != "Own" {
		t.Fatalf("declared F1 was replaced: %+v", keys)
	}
	// A panel without help is untouched.
	if rpcPanelHelpKey(PluginPanelDescriptor{ID: "x", Title: "X"}) != nil {
		t.Fatal("a descriptor without help produced a key")
	}
}
