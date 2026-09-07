package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// PluginPanelDescriptor is the transport-safe declaration returned from
// Plugin.Init. The panel document itself is opened lazily when the user runs
// the generated command.
type PluginPanelDescriptor struct {
	ID          string
	Title       string
	Description string
}

type RPCPanelOpenRequest struct {
	ID      string
	Context vfs.PanelContext
}

// Document is UTF-8 JSON in the vtui .vui document format. Keeping the wire
// payload as bytes lets MessagePack carry the exact document without making
// the RPC protocol depend on vtui's Go struct field names.
type RPCPanelOpenResponse struct {
	Document []byte
}

type RPCPanelEventRequest struct {
	ID      string
	Kind    string // "key" or "mouse"
	Event   vtinput.InputEvent
	Context vfs.PanelContext
}

type RPCPanelEventResponse struct {
	Handled  bool
	Document []byte
	Close    bool
}

func registerRPCPluginPanels(
	api vfs.HostAPI,
	back PluginTransport,
	pluginName string,
	descriptors []PluginPanelDescriptor,
	registrations *pluginSessionRegistrations,
) error {
	if len(descriptors) == 0 {
		return nil
	}
	contributions, ok := api.(vfs.PanelContributionHost)
	if !ok {
		return fmt.Errorf("plugin %q declares panels, but the host has no panel contribution API", pluginName)
	}
	if back == nil {
		return fmt.Errorf("plugin %q panel transport is nil", pluginName)
	}
	for _, raw := range descriptors {
		descriptor := raw
		descriptor.ID = strings.TrimSpace(descriptor.ID)
		descriptor.Title = strings.TrimSpace(descriptor.Title)
		if descriptor.ID == "" {
			return fmt.Errorf("plugin %q declares a panel with an empty ID", pluginName)
		}
		if descriptor.Title == "" {
			return fmt.Errorf("plugin %q panel %q has no title", pluginName, descriptor.ID)
		}
		panelID := descriptor.ID
		registration, err := contributions.RegisterPanelProvider(vfs.PanelProvider{
			ID:          panelID,
			Title:       descriptor.Title,
			Description: descriptor.Description,
			Open: func(ctx vfs.PanelContext) (vfs.PanelController, error) {
				var response RPCPanelOpenResponse
				if err := back.Call("Plugin.OpenPanel", RPCPanelOpenRequest{ID: panelID, Context: ctx}, &response); err != nil {
					return nil, fmt.Errorf("open panel %q: %w", panelID, err)
				}
				return newRPCVUIPanel(back, panelID, response.Document)
			},
		})
		if err != nil {
			return fmt.Errorf("register plugin %q panel %q: %w", pluginName, panelID, err)
		}
		if !registrations.Add(registration) {
			return fmt.Errorf("plugin %q disconnected while registering panel %q", pluginName, panelID)
		}
	}
	return nil
}

// rpcVUIPanel is a small host-side adapter for a vtui .vui tree. The remote
// plugin owns behavior: f4 forwards raw key/mouse events plus the latest panel
// context and replaces the tree when the plugin returns a new document.
type rpcVUIPanel struct {
	sess    PluginTransport
	id      string
	window  *vtui.Window
	context vfs.PanelContext
	focused bool
	mu      sync.Mutex
	closed  bool
}

func newRPCVUIPanel(sess PluginTransport, id string, document []byte) (*rpcVUIPanel, error) {
	if len(document) == 0 {
		return nil, fmt.Errorf("panel %q returned an empty .vui document", id)
	}
	window, err := loadRPCVUIDocument(document)
	if err != nil {
		return nil, fmt.Errorf("panel %q returned invalid .vui document: %w", id, err)
	}
	return &rpcVUIPanel{sess: sess, id: id, window: window}, nil
}

func loadRPCVUIDocument(document []byte) (*vtui.Window, error) {
	var parsed vtui.VuiDocument
	if err := json.Unmarshal(document, &parsed); err != nil {
		return nil, err
	}
	return vtui.LoadVuiDocument(&parsed)
}

func (p *rpcVUIPanel) Show(scr *vtui.ScreenBuf) { p.window.Show(scr) }

func (p *rpcVUIPanel) ProcessKey(e *vtinput.InputEvent) bool {
	return p.processEvent("key", e)
}

func (p *rpcVUIPanel) ProcessMouse(e *vtinput.InputEvent) bool {
	return p.processEvent("mouse", e)
}

func (p *rpcVUIPanel) processEvent(kind string, event *vtinput.InputEvent) bool {
	if p == nil || event == nil {
		return false
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return false
	}
	ctx := p.context
	sess := p.sess
	id := p.id
	p.mu.Unlock()
	var response RPCPanelEventResponse
	err := sess.Call("Plugin.PanelEvent", RPCPanelEventRequest{
		ID: id, Kind: kind, Event: *event, Context: ctx,
	}, &response)
	if err != nil {
		vtui.DebugLog("PANEL [%s]: event failed: %v", id, err)
		return false
	}
	if len(response.Document) > 0 {
		if err := p.replaceDocument(response.Document); err != nil {
			vtui.DebugLog("PANEL [%s]: invalid render document: %v", id, err)
			return false
		}
	}
	if response.Close {
		_ = p.Close()
		return true
	}
	return response.Handled
}

func (p *rpcVUIPanel) replaceDocument(document []byte) error {
	window, err := loadRPCVUIDocument(document)
	if err != nil {
		return err
	}
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	x1, y1, x2, y2 := p.window.GetPosition()
	focused := p.focused
	window.SetPosition(x1, y1, x2, y2)
	window.SetFocus(focused)
	p.window = window
	p.mu.Unlock()
	vtui.FrameManager.Redraw()
	return nil
}

func (p *rpcVUIPanel) SetFocus(focused bool) {
	p.mu.Lock()
	p.focused = focused
	if p.window != nil {
		p.window.SetFocus(focused)
	}
	p.mu.Unlock()
}

func (p *rpcVUIPanel) IsFocused() bool {
	p.mu.Lock()
	focused := p.focused
	p.mu.Unlock()
	return focused
}

func (p *rpcVUIPanel) SetPosition(x1, y1, x2, y2 int) {
	p.mu.Lock()
	if p.window != nil {
		p.window.SetPosition(x1, y1, x2, y2)
	}
	p.mu.Unlock()
}

func (p *rpcVUIPanel) GetPosition() (int, int, int, int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.window == nil {
		return 0, 0, 0, 0
	}
	return p.window.GetPosition()
}

func (p *rpcVUIPanel) GetSelectedName() string { return "" }

func (p *rpcVUIPanel) SetContext(ctx vfs.PanelContext) {
	ctx.Current.SelectedNames = append([]string(nil), ctx.Current.SelectedNames...)
	ctx.Other.SelectedNames = append([]string(nil), ctx.Other.SelectedNames...)
	p.mu.Lock()
	p.context = ctx
	p.mu.Unlock()
}

func (p *rpcVUIPanel) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	sess := p.sess
	id := p.id
	p.mu.Unlock()
	if sess == nil {
		return nil
	}
	return sess.Call("Plugin.ClosePanel", map[string]string{"ID": id}, nil)
}
