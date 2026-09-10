package plughost

import (
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

type rpcPanelCoverageRegistration struct {
	removed bool
}

func (r *rpcPanelCoverageRegistration) Unregister() { r.removed = true }

type rpcPanelCoverageHost struct {
	*luaTestHostAPI
	panels []vfs.PanelProvider
}

func (h *rpcPanelCoverageHost) RegisterPanelProvider(provider vfs.PanelProvider) (vfs.Registration, error) {
	h.panels = append(h.panels, provider)
	return &rpcPanelCoverageRegistration{}, nil
}

type rpcPanelCoverageTransport struct {
	calls    []string
	openDoc  []byte
	eventRes RPCPanelEventResponse
}

func (t *rpcPanelCoverageTransport) Call(method string, params, result any) error {
	t.calls = append(t.calls, method)
	switch method {
	case "Plugin.OpenPanel":
		*(result.(*RPCPanelOpenResponse)) = RPCPanelOpenResponse{Document: t.openDoc}
	case "Plugin.PanelEvent":
		*(result.(*RPCPanelEventResponse)) = t.eventRes
	}
	return nil
}

func TestRegisterRPCPluginPanelsValidationAndOpen(t *testing.T) {
	descriptor := PluginPanelDescriptor{ID: " notes ", Title: " Notes "}
	noPanels := newLuaTestHostAPI()
	if err := RegisterRPCPluginPanels(noPanels, &rpcPanelCoverageTransport{}, "plugin", []PluginPanelDescriptor{descriptor}, &PluginSessionRegistrations{}); err == nil {
		t.Fatal("panel declaration was accepted by a host without panel API")
	}

	host := &rpcPanelCoverageHost{luaTestHostAPI: newLuaTestHostAPI()}
	registrations := &PluginSessionRegistrations{}
	if err := RegisterRPCPluginPanels(host, nil, "plugin", []PluginPanelDescriptor{descriptor}, registrations); err == nil {
		t.Fatal("nil panel transport was accepted")
	}
	if err := RegisterRPCPluginPanels(host, &rpcPanelCoverageTransport{}, "plugin", []PluginPanelDescriptor{{Title: "Title"}}, registrations); err == nil {
		t.Fatal("empty panel ID was accepted")
	}
	if err := RegisterRPCPluginPanels(host, &rpcPanelCoverageTransport{}, "plugin", []PluginPanelDescriptor{{ID: "id"}}, registrations); err == nil {
		t.Fatal("empty panel title was accepted")
	}

	transport := &rpcPanelCoverageTransport{}
	if err := RegisterRPCPluginPanels(host, transport, "plugin", []PluginPanelDescriptor{descriptor}, registrations); err != nil {
		t.Fatalf("register panel: %v", err)
	}
	if len(host.panels) != 1 || host.panels[0].ID != "notes" || host.panels[0].Title != "Notes" {
		t.Fatalf("registered panels = %+v", host.panels)
	}
	if _, err := host.panels[0].Open(vfs.PanelContext{}); err == nil {
		t.Fatal("empty panel document was accepted")
	}
	registrations.Unregister()

	if _, err := newRPCVUIPanel(transport, "panel", nil); err == nil {
		t.Fatal("empty VUI document was accepted")
	}
	if _, err := newRPCVUIPanel(transport, "panel", []byte("{")); err == nil {
		t.Fatal("invalid VUI document was accepted")
	}
}

func TestRPCVUIPanelEventsStateAndClose(t *testing.T) {
	transport := &rpcPanelCoverageTransport{
		eventRes: RPCPanelEventResponse{Handled: true},
	}
	document := []byte(`{"vuiVersion":1,"root":{"type":"Window"}}`)
	panel, err := newRPCVUIPanel(transport, "panel", document)
	if err != nil {
		t.Fatalf("newRPCVUIPanel: %v", err)
	}

	if panel.ProcessKey(nil) || panel.ProcessMouse(nil) {
		t.Fatal("nil panel events were accepted")
	}
	event := &vtinput.InputEvent{}
	if !panel.ProcessKey(event) || !panel.ProcessMouse(event) {
		t.Fatal("panel events were not forwarded")
	}
	panel.SetFocus(true)
	if !panel.IsFocused() {
		t.Fatal("panel focus was not recorded")
	}
	panel.SetPosition(1, 2, 11, 12)
	if x1, y1, x2, y2 := panel.GetPosition(); x1 != 1 || y1 != 2 || x2 != 11 || y2 != 12 {
		t.Fatalf("panel position = %d,%d,%d,%d", x1, y1, x2, y2)
	}
	panel.SetContext(vfs.PanelContext{Side: 1})
	panel.mu.Lock()
	side := panel.context.Side
	panel.mu.Unlock()
	if side != 1 {
		t.Fatalf("panel context side = %d", side)
	}
	if panel.GetSelectedName() != "" {
		t.Fatal("RPC panel returned a selected name")
	}
	if err := panel.replaceDocument([]byte("{")); err == nil {
		t.Fatal("invalid replacement document was accepted")
	}
	if err := panel.Close(); err != nil {
		t.Fatalf("close panel: %v", err)
	}
	callCount := len(transport.calls)
	if err := panel.Close(); err != nil {
		t.Fatalf("close panel twice: %v", err)
	}
	if len(transport.calls) != callCount {
		t.Fatal("closing panel twice called the transport twice")
	}
	if panel.ProcessKey(event) {
		t.Fatal("closed panel accepted an event")
	}

	var nilPanel *rpcVUIPanel
	if nilPanel.ProcessKey(event) || nilPanel.ProcessMouse(event) {
		t.Fatal("nil panel accepted an event")
	}
	if err := (&rpcVUIPanel{}).Close(); err != nil {
		t.Fatalf("closing panel without transport: %v", err)
	}
}
