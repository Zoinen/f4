package main

import (
	"io"
	"testing"

	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/sdk/f4plugin"
	"github.com/unxed/f4/sdk/f4rpc"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// panelHost is the smallest vfs.HostAPI that accepts panel providers.
type panelHost struct{ panels []vfs.PanelProvider }

func (h *panelHost) GetVersion() string                                                  { return "test" }
func (h *panelHost) Log(string)                                                          {}
func (h *panelHost) Message(string)                                                      {}
func (h *panelHost) RegisterHighlighter(vtui.HighlighterProvider)                        {}
func (h *panelHost) RegisterVFSProvider(vfs.VFSProvider)                                 {}
func (h *panelHost) RegisterURIProvider(vfs.URIProvider) error                           { return nil }
func (h *panelHost) RegisterDrive(string, func() vfs.VFS)                                {}
func (h *panelHost) RegisterPluginMenuItem(string, func(vfs.App))                        {}
func (h *panelHost) RunAction(string) bool                                               { return false }
func (h *panelHost) RegisterGlobalHotkey(uint16, vtinput.ControlKeyState, func(vfs.App)) {}

type panelRegistration struct{}

func (panelRegistration) Unregister() {}

func (h *panelHost) RegisterPanelProvider(provider vfs.PanelProvider) (vfs.Registration, error) {
	h.panels = append(h.panels, provider)
	return panelRegistration{}, nil
}

// TestCounterPanelKeysReachTheHostOverRPC runs the dummy plugin behind the
// real SDK and the host's real RPC panel adapter, connected by pipes, and
// checks the key declaration end to end: it arrives with the panel, a
// declared key runs in the plugin, and the state change (Shift+F8 becoming
// enabled) comes back without the host asking.
func TestCounterPanelKeysReachTheHostOverRPC(t *testing.T) {
	hostToPluginR, hostToPluginW := io.Pipe()
	pluginToHostR, pluginToHostW := io.Pipe()
	plugin := &DummyPlugin{}
	pluginDone := make(chan error, 1)
	go func() { pluginDone <- f4plugin.Serve(plugin, hostToPluginR, pluginToHostW) }()
	sess := f4rpc.NewSession(pluginToHostR, hostToPluginW)
	hostDone := make(chan error, 1)
	go func() { hostDone <- sess.Serve() }()
	t.Cleanup(func() {
		_ = hostToPluginW.Close()
		_ = pluginToHostW.Close()
		<-pluginDone
		<-hostDone
	})

	var initRes struct {
		Drives []string
		Panels []plughost.PluginPanelDescriptor
	}
	if err := sess.Call("Plugin.Init", nil, &initRes); err != nil {
		t.Fatalf("Plugin.Init: %v", err)
	}
	host := &panelHost{}
	registrations := &plughost.PluginSessionRegistrations{}
	defer registrations.Unregister()
	if err := plughost.RegisterRPCPluginPanels(host, sess, "dummy", initRes.Panels, registrations); err != nil {
		t.Fatalf("register panels: %v", err)
	}
	if len(host.panels) != 1 || host.panels[0].ID != dummyPanelID {
		t.Fatalf("panels = %+v", host.panels)
	}
	controller, err := host.panels[0].Open(vfs.PanelContext{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() {
		if err := controller.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()
	provider, ok := controller.(vfs.PanelKeyProvider)
	if !ok {
		t.Fatal("the RPC panel does not declare keys to the host")
	}
	keys := provider.PanelKeys()
	if len(keys) != 2 || keys[0].Label != "Count" || keys[1].Label != "Reset" {
		t.Fatalf("keys = %+v", keys)
	}
	if keys[1].Enabled == nil || keys[1].Enabled() {
		t.Fatal("Reset should start disabled")
	}

	shiftF8 := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F8,
		ControlKeyState: vtinput.ShiftPressed}
	f5 := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F5}
	for press := 1; press <= 2; press++ {
		if !vfs.DispatchPanelKey(provider.PanelKeys(), f5) {
			t.Fatalf("F5 press %d was not dispatched", press)
		}
	}
	if plugin.count != 2 {
		t.Fatalf("count = %d after two F5, want 2", plugin.count)
	}
	keys = provider.PanelKeys()
	if keys[1].Enabled != nil {
		t.Fatal("Reset stayed disabled after the plugin's state changed")
	}
	if !vfs.DispatchPanelKey(keys, shiftF8) || plugin.count != 0 {
		t.Fatalf("Shift+F8 did not reset: count = %d", plugin.count)
	}
	if keys := provider.PanelKeys(); keys[1].Enabled == nil || keys[1].Enabled() {
		t.Fatal("Reset should be disabled again")
	}
}
