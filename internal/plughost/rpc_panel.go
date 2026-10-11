package plughost

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/unxed/f4/internal/i18n"
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
	// Help is the panel's help, Markdown, shown on F1 in f4's Markdown
	// viewer (f4#272). LocalizedHelp maps language codes to translations and
	// wins over Help when it has the interface language (or its fallbacks).
	// A panel that declares its own F1 key keeps it; one without any help
	// leaves F1 to the global Help binding.
	Help          string
	LocalizedHelp map[string]string
}

type RPCPanelOpenRequest struct {
	ID      string
	Context vfs.PanelContext
}

// Document is UTF-8 JSON in the vtui .vui document format. Keeping the wire
// payload as bytes lets MessagePack carry the exact document without making
// the RPC protocol depend on vtui's Go struct field names.
//
// Keys is the panel's key declaration (f4#312, see RPCPanelKey). HasKeys says
// whether the plugin sent one at all: a plugin built before panel keys never
// sets it and keeps the old behavior, while HasKeys with an empty Keys clears
// a previous declaration.
type RPCPanelOpenResponse struct {
	Document []byte
	Keys     []RPCPanelKey
	HasKeys  bool
}

// RPCPanelKey is the transport-safe form of vfs.PanelKey. VK and Mods are the
// vtinput key code and modifier bits, Label the localized keybar caption.
// Disabled dims the caption and makes the host consume the key without
// calling the plugin; the flag is inverted relative to vfs.PanelKey.Enabled so
// a zero value is an enabled key.
//
// A remote plugin cannot answer a synchronous question every time f4 draws
// its keybar, so the declaration travels with the plugin's answers instead:
// Plugin.OpenPanel and every Plugin.PanelEvent response may carry a fresh
// set, exactly like they may carry a fresh .vui document, and the host keeps
// the last one it got. A declared key is sent to the plugin as an ordinary
// Plugin.PanelEvent of Kind "key".
type RPCPanelKey struct {
	VK       uint16
	Mods     uint32
	Label    string
	Disabled bool
}

// maxRPCPanelKeys bounds one declaration; the keybar has 48 visible slots and
// anything past a few dozen keys is a plugin bug, not a key map.
const maxRPCPanelKeys = 64

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
	// Keys and HasKeys replace the panel's key declaration when HasKeys is
	// set; see RPCPanelOpenResponse.
	Keys    []RPCPanelKey
	HasKeys bool
}

func RegisterRPCPluginPanels(
	api vfs.HostAPI,
	back PluginTransport,
	pluginName string,
	descriptors []PluginPanelDescriptor,
	registrations *PluginSessionRegistrations,
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
				panel, err := newRPCVUIPanel(back, panelID, response.Document)
				if err != nil {
					return nil, err
				}
				panel.help = rpcPanelHelpKey(descriptor)
				if response.HasKeys {
					panel.setKeys(response.Keys)
				}
				return panel, nil
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
//
// It implements vfs.PanelKeyProvider for plugins that declare keys. The host
// asks for PanelKeys on every key and every keybar draw, so the declaration is
// kept here, converted once when the plugin sends a different one, and never
// fetched over the transport on demand.
type rpcVUIPanel struct {
	sess     PluginTransport
	id       string
	window   *vtui.Window
	context  vfs.PanelContext
	focused  bool
	mu       sync.Mutex
	closed   bool
	keysDecl []RPCPanelKey
	keys     []vfs.PanelKey
	// help is the F1 key made from the descriptor's help text, nil without one.
	help *vfs.PanelKey
}

// rpcPanelHelpKey is the F1 key of a panel whose descriptor carries help.
func rpcPanelHelpKey(descriptor PluginPanelDescriptor) *vfs.PanelKey {
	if strings.TrimSpace(descriptor.Help) == "" && len(descriptor.LocalizedHelp) == 0 {
		return nil
	}
	key := vfs.PanelHelpKey(i18n.Msg("KeyBar.F1"),
		func() string { return descriptor.Title },
		func() string {
			return pluginCommandDisplayText("", descriptor.LocalizedHelp, descriptor.Help)
		})
	return &key
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
	if response.HasKeys && p.setKeys(response.Keys) {
		vtui.FrameManager.Redraw()
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

// PanelKeys returns the last key set the plugin declared. The slice is shared
// and must not be modified; a panel that never declared keys returns nil,
// which the host treats exactly like a controller without PanelKeys.
func (p *rpcVUIPanel) PanelKeys() []vfs.PanelKey {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	if p.help == nil {
		return p.keys
	}
	for _, key := range p.keys {
		if key.VK == vtinput.VK_F1 && key.Mods == 0 {
			return p.keys
		}
	}
	// A fresh slice: p.keys is shared with the caller of an earlier call.
	return append([]vfs.PanelKey{*p.help}, p.keys...)
}

// setKeys installs a declaration received from the plugin and reports whether
// it differs from the current one. Keys without a key code are dropped and the
// set is capped at maxRPCPanelKeys.
func (p *rpcVUIPanel) setKeys(declared []RPCPanelKey) bool {
	clean := make([]RPCPanelKey, 0, len(declared))
	for _, key := range declared {
		if key.VK == 0 {
			continue
		}
		key.Label = strings.TrimSpace(key.Label)
		clean = append(clean, key)
		if len(clean) == maxRPCPanelKeys {
			break
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || equalRPCPanelKeys(p.keysDecl, clean) {
		return false
	}
	keys := make([]vfs.PanelKey, 0, len(clean))
	for _, key := range clean {
		wire := key
		panelKey := vfs.PanelKey{
			VK:    wire.VK,
			Mods:  vtinput.ControlKeyState(wire.Mods),
			Label: wire.Label,
			Run:   func() { p.runKey(wire) },
		}
		if wire.Disabled {
			panelKey.Enabled = func() bool { return false }
		}
		keys = append(keys, panelKey)
	}
	if len(clean) == 0 {
		clean, keys = nil, nil
	}
	p.keysDecl = clean
	p.keys = keys
	return true
}

func equalRPCPanelKeys(a, b []RPCPanelKey) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// runKey sends a declared key to the plugin as the key event it names.
func (p *rpcVUIPanel) runKey(key RPCPanelKey) {
	event := vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		RepeatCount:     1,
		VirtualKeyCode:  key.VK,
		ControlKeyState: vtinput.ControlKeyState(key.Mods),
	}
	p.processEvent("key", &event)
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
