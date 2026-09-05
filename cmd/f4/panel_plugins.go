package main

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type registeredPanelProvider struct {
	provider vfs.PanelProvider
	token    *struct{}
}

var panelProviderRegistry = struct {
	sync.RWMutex
	byID map[string]registeredPanelProvider
}{byID: make(map[string]registeredPanelProvider)}

const panelProviderCommandPrefix = "panel."

// RegisterPanelProvider implements the optional panel-only contribution API.
// A provider is intentionally also exposed as a regular command so it is
// available from the plugin menu and command palette without a second API.
func (c *coreAPI) RegisterPanelProvider(provider vfs.PanelProvider) (vfs.Registration, error) {
	provider.ID = strings.TrimSpace(provider.ID)
	provider.Title = strings.TrimSpace(provider.Title)
	if provider.ID == "" {
		return nil, errors.New("panel provider ID is empty")
	}
	if provider.Title == "" {
		return nil, fmt.Errorf("panel provider %q has an empty title", provider.ID)
	}
	if provider.Open == nil {
		return nil, fmt.Errorf("panel provider %q has no open handler", provider.ID)
	}

	registryID := strings.ToLower(provider.ID)
	commandID := panelProviderCommandPrefix + registryID
	token := &struct{}{}
	panelProviderRegistry.Lock()
	if _, exists := panelProviderRegistry.byID[registryID]; exists {
		panelProviderRegistry.Unlock()
		return nil, fmt.Errorf("panel provider %q is already registered", provider.ID)
	}
	panelProviderRegistry.byID[registryID] = registeredPanelProvider{provider: provider, token: token}
	panelProviderRegistry.Unlock()

	command, err := c.RegisterPluginCommand(vfs.PluginCommand{
		ID:          commandID,
		Location:    vfs.PluginCommandPanel,
		Label:       "Open " + provider.Title,
		Description: provider.Description,
		SearchTerms: []string{"panel", "plugin", provider.ID},
		Run: func(app vfs.App) {
			openRegisteredPanelProvider(app, registryID)
		},
	})
	if err != nil {
		panelProviderRegistry.Lock()
		if current, ok := panelProviderRegistry.byID[registryID]; ok && current.token == token {
			delete(panelProviderRegistry.byID, registryID)
		}
		panelProviderRegistry.Unlock()
		return nil, fmt.Errorf("register panel provider %q command: %w", provider.ID, err)
	}

	return &unregisterFunc{fn: func() {
		command.Unregister()
		panelProviderRegistry.Lock()
		if current, ok := panelProviderRegistry.byID[registryID]; ok && current.token == token {
			delete(panelProviderRegistry.byID, registryID)
		}
		panelProviderRegistry.Unlock()
	}}, nil
}

func lookupPanelProvider(id string) (vfs.PanelProvider, bool) {
	panelProviderRegistry.RLock()
	entry, ok := panelProviderRegistry.byID[strings.ToLower(strings.TrimSpace(id))]
	panelProviderRegistry.RUnlock()
	return entry.provider, ok
}

func openRegisteredPanelProvider(app vfs.App, providerID string) {
	pf, ok := app.(*PanelsFrame)
	if !ok || pf == nil || !pf.showPanels || pf.closed {
		return
	}
	provider, ok := lookupPanelProvider(providerID)
	if !ok {
		return
	}

	slot := pf.activeIdx
	ctx := pf.panelPluginContext(slot)
	controller, err := provider.Open(ctx)
	if err != nil {
		vtui.DebugLog("PANEL [%s]: open failed: %v", provider.ID, err)
		showToast(fmt.Sprintf("Panel %s: %v", provider.Title, err), 3e9)
		return
	}
	if controller == nil {
		vtui.DebugLog("PANEL [%s]: open returned nil controller", provider.ID)
		return
	}

	if old := pf.altPanels[slot]; old != nil {
		if closer, ok := old.(interface{ Close() }); ok {
			closer.Close()
		} else {
			pf.altPanels[slot] = nil
		}
	}
	instance := &pluginPanelInstance{
		providerID: provider.ID,
		title:      provider.Title,
		host:       pf,
		slot:       slot,
		controller: controller,
	}
	instance.closeFn = func() {
		if pf.altPanels[slot] == instance {
			pf.altPanels[slot] = nil
			// PanelsFrame.Close holds ptyMutex while closing overlays. Do not
			// re-enter ResizeConsole from that path.
			if !pf.closed {
				pf.ResizeConsole(pf.lastW, pf.lastH)
				vtui.FrameManager.HardRefresh()
			}
		}
	}
	instance.SetPositionForPanel()
	instance.SetContext(ctx)
	pf.altPanels[slot] = instance
	if slot == 0 {
		pf.showLeftPanel = true
	} else {
		pf.showRightPanel = true
	}
	pf.ResizeConsole(pf.lastW, pf.lastH)
	vtui.FrameManager.HardRefresh()
}

// pluginPanelInstance adapts the public vfs panel contract to f4's existing
// AltPanel slot. The FileSystemPanel remains underneath as the logical panel,
// so ordinary file actions and VFS providers keep their existing assumptions.
type pluginPanelInstance struct {
	providerID string
	title      string
	host       *PanelsFrame
	slot       int
	controller vfs.PanelController
	closeFn    func()
	closeOnce  sync.Once
}

func (p *pluginPanelInstance) Source() *FileSystemPanel {
	if p == nil || p.host == nil || p.slot < 0 || p.slot >= len(p.host.panels) {
		return nil
	}
	fsp, _ := p.host.panels[p.slot].(*FileSystemPanel)
	return fsp
}

func (p *pluginPanelInstance) Kind() string { return "plugin:" + p.providerID }

func (p *pluginPanelInstance) Show(scr *vtui.ScreenBuf) {
	if p == nil || p.controller == nil {
		return
	}
	if p.host != nil {
		p.SetContext(p.host.panelPluginContext(p.slot))
	}
	p.controller.Show(scr)
}

func (p *pluginPanelInstance) ProcessKey(e *vtinput.InputEvent) bool {
	if p == nil || p.controller == nil {
		return false
	}
	if p.host != nil {
		p.SetContext(p.host.panelPluginContext(p.slot))
	}
	if p.controller.ProcessKey(e) {
		return true
	}
	// Escape is the common close gesture for a panel plugin that does not
	// claim it. A plugin that wants to own Escape simply returns true.
	if e != nil && e.KeyDown && e.VirtualKeyCode == vtinput.VK_ESCAPE {
		p.Close()
		return true
	}
	return false
}

func (p *pluginPanelInstance) ProcessMouse(e *vtinput.InputEvent) bool {
	if p == nil || p.controller == nil {
		return false
	}
	if p.host != nil {
		p.SetContext(p.host.panelPluginContext(p.slot))
	}
	return p.controller.ProcessMouse(e)
}

func (p *pluginPanelInstance) SetFocus(focused bool) {
	if p != nil && p.controller != nil {
		p.controller.SetFocus(focused)
	}
}

func (p *pluginPanelInstance) IsFocused() bool {
	return p != nil && p.controller != nil && p.controller.IsFocused()
}

func (p *pluginPanelInstance) SetPosition(x1, y1, x2, y2 int) {
	if p != nil && p.controller != nil {
		p.controller.SetPosition(x1, y1, x2, y2)
	}
}

func (p *pluginPanelInstance) SetPositionForPanel() {
	if p == nil || p.host == nil || p.controller == nil {
		return
	}
	x1, y1, x2, y2 := p.host.panels[p.slot].GetPosition()
	p.controller.SetPosition(x1, y1, x2, y2)
}

func (p *pluginPanelInstance) GetPosition() (int, int, int, int) {
	if p == nil || p.controller == nil {
		return 0, 0, 0, 0
	}
	return p.controller.GetPosition()
}

func (p *pluginPanelInstance) GetSelectedName() string {
	if p == nil || p.controller == nil {
		return ""
	}
	return p.controller.GetSelectedName()
}

func (p *pluginPanelInstance) SetContext(ctx vfs.PanelContext) {
	if p != nil && p.controller != nil {
		p.controller.SetContext(ctx)
	}
}

func (p *pluginPanelInstance) Close() {
	if p == nil {
		return
	}
	p.closeOnce.Do(func() {
		if p.controller != nil {
			_ = p.controller.Close()
		}
		if p.closeFn != nil {
			p.closeFn()
		}
	})
}

func (pf *PanelsFrame) panelPluginContext(slot int) vfs.PanelContext {
	ctx := vfs.PanelContext{Side: slot, ActiveSide: pf.activeIdx}
	if slot == pf.activeIdx {
		ctx.Current = pf.panelPluginState(slot, true)
		ctx.Other = pf.panelPluginState(1-slot, false)
	} else {
		ctx.Current = pf.panelPluginState(slot, false)
		ctx.Other = pf.panelPluginState(1-slot, true)
	}
	if alt, ok := pf.altPanels[slot].(*pluginPanelInstance); ok && alt.controller != nil {
		ctx.Bounds[0], ctx.Bounds[1], ctx.Bounds[2], ctx.Bounds[3] = alt.controller.GetPosition()
	} else if panel := pf.panels[slot]; panel != nil {
		ctx.Bounds[0], ctx.Bounds[1], ctx.Bounds[2], ctx.Bounds[3] = panel.GetPosition()
	}
	return ctx
}

func (pf *PanelsFrame) panelPluginState(slot int, active bool) vfs.PanelState {
	state := vfs.PanelState{Side: slot, Active: active}
	if slot < 0 || slot >= len(pf.panels) {
		return state
	}
	fsp, ok := pf.panels[slot].(*FileSystemPanel)
	if !ok || fsp == nil || fsp.vfs == nil {
		return state
	}
	state.Path = fsp.vfs.GetPath()
	state.SelectedName = fsp.GetSelectedName()
	state.SelectedNames = append([]string(nil), fsp.GetSelectedNames()...)
	return state
}
