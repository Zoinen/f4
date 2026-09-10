package app

import (
	"encoding/json"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"testing"

	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	msgpack "github.com/vmihailenco/msgpack/v5"
)

type panelPluginTestController struct {
	vtui.ScreenObject
	context vfs.PanelContext
	keys    int
	closed  bool
}

func (p *panelPluginTestController) SetContext(ctx vfs.PanelContext) { p.context = ctx }
func (p *panelPluginTestController) ProcessKey(event *vtinput.InputEvent) bool {
	p.keys++
	return event == nil || event.VirtualKeyCode != vtinput.VK_ESCAPE
}
func (p *panelPluginTestController) Close() error {
	p.closed = true
	return nil
}
func (p *panelPluginTestController) GetSelectedName() string { return "plugin-row" }

func TestPanelProviderOpensInActiveSlotAndReceivesContext(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := paneltest.SetupMockPanelsFrame(t)
	pf.ResizeConsole(80, 25)
	defer pf.Close()

	controller := &panelPluginTestController{}
	registration, err := (&CoreAPI{}).RegisterPanelProvider(vfs.PanelProvider{
		ID:    "test.panel.active-slot",
		Title: "Test panel",
		Open: func(ctx vfs.PanelContext) (vfs.PanelController, error) {
			controller.SetContext(ctx)
			return controller, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer registration.Unregister()

	panel.OpenRegisteredPanelProvider(pf, "test.panel.active-slot")
	instance, ok := pf.AltPanels[1].(*panel.PluginPanelInstance)
	if !ok {
		t.Fatalf("active slot contains %T, want pluginPanelInstance", pf.AltPanels[1])
	}
	if pf.Panels[1] == nil {
		t.Fatal("logical file panel was removed")
	}
	pf.Show(vtui.NewSilentScreenBuf())
	if controller.context.Side != 1 || controller.context.ActiveSide != 1 || !controller.context.Current.Active {
		t.Fatalf("unexpected panel context: %+v", controller.context)
	}
	if controller.context.Other.Side != 0 || controller.context.Other.Active {
		t.Fatalf("unexpected neighbouring panel context: %+v", controller.context.Other)
	}

	if !instance.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F4}) {
		t.Fatal("panel did not consume its key")
	}
	if controller.keys != 1 {
		t.Fatalf("panel key count = %d, want 1", controller.keys)
	}
	instance.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE})
	if pf.AltPanels[1] != nil || !controller.closed {
		t.Fatal("Escape did not close an unhandled plugin panel")
	}
}

func TestRPCPanelProviderOpensVUIAndForwardsEvent(t *testing.T) {
	coreSess, pluginSess := testutil.RPCSessionPair(t)

	document := []byte(`{"vuiVersion":1,"root":{"type":"Dialog","props":{"title":"Panel"}}}`)
	pluginSess.Register("Plugin.OpenPanel", func(data msgpack.RawMessage) (any, error) {
		var request plughost.RPCPanelOpenRequest
		if err := msgpack.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		if request.ID != "remote.panel" || request.Context.Side != 1 {
			t.Fatalf("unexpected open request: %+v", request)
		}
		return plughost.RPCPanelOpenResponse{Document: document}, nil
	})
	pluginSess.Register("Plugin.PanelEvent", func(data msgpack.RawMessage) (any, error) {
		var request plughost.RPCPanelEventRequest
		if err := msgpack.Unmarshal(data, &request); err != nil {
			return nil, err
		}
		if request.Kind != "key" || request.Event.VirtualKeyCode != vtinput.VK_F4 {
			t.Fatalf("unexpected event request: %+v", request)
		}
		return plughost.RPCPanelEventResponse{Handled: true, Document: document}, nil
	})
	pluginSess.Register("Plugin.ClosePanel", func(data msgpack.RawMessage) (any, error) { return nil, nil })

	api := &CoreAPI{}
	registrations := &plughost.PluginSessionRegistrations{}
	if err := plughost.RegisterRPCPluginPanels(api, coreSess, "test-rpc", []plughost.PluginPanelDescriptor{{
		ID: "remote.panel", Title: "Remote panel",
	}}, registrations); err != nil {
		t.Fatal(err)
	}
	defer registrations.Unregister()

	provider, ok := plughost.LookupPanelProvider("remote.panel")
	if !ok {
		t.Fatal("RPC panel was not registered")
	}
	pnl, err := provider.Open(vfs.PanelContext{Side: 1, ActiveSide: 1})
	if err != nil {
		t.Fatal(err)
	}
	pnl.SetPosition(0, 0, 39, 19)
	if !pnl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F4}) {
		t.Fatal("RPC panel did not return handled=true")
	}
	if err := pnl.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestRPCPanelDocumentRoundTripsAsJSON(t *testing.T) {
	var doc vtui.VuiDocument
	if err := json.Unmarshal([]byte(`{"vuiVersion":1,"root":{"type":"Dialog"}}`), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Root == nil || doc.Root.Type != "Dialog" {
		t.Fatalf("bad .vui document: %+v", doc)
	}
}
