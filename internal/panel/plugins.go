package panel

import (
	"fmt"
	"sync"

	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func OpenRegisteredPanelProvider(app vfs.App, providerID string) {
	pf, ok := app.(*PanelsFrame)
	if !ok || pf == nil || !pf.ShowPanels || pf.Closed {
		return
	}
	provider, ok := plughost.LookupPanelProvider(providerID)
	if !ok {
		return
	}

	slot := pf.ActiveIdx
	ctx := pf.panelPluginContext(slot)
	controller, err := provider.Open(ctx)
	if err != nil {
		vtui.DebugLog("PANEL [%s]: open failed: %v", provider.ID, err)
		toast.Show(fmt.Sprintf("Panel %s: %v", provider.Title, err), 3e9)
		return
	}
	if controller == nil {
		vtui.DebugLog("PANEL [%s]: open returned nil controller", provider.ID)
		return
	}

	if old := pf.AltPanels[slot]; old != nil {
		if closer, ok := old.(interface{ Close() }); ok {
			closer.Close()
		} else {
			pf.AltPanels[slot] = nil
		}
	}
	instance := &PluginPanelInstance{
		providerID: provider.ID,
		title:      provider.Title,
		host:       pf,
		slot:       slot,
		controller: controller,
	}
	instance.closeFn = func() {
		if pf.AltPanels[slot] == instance {
			pf.AltPanels[slot] = nil
			// PanelsFrame.Close holds ptyMutex while closing overlays. Do not
			// re-enter ResizeConsole from that path.
			if !pf.Closed {
				pf.ResizeConsole(pf.LastW, pf.LastH)
				vtui.FrameManager.HardRefresh()
			}
		}
	}
	instance.SetPositionForPanel()
	instance.SetContext(ctx)
	pf.AltPanels[slot] = instance
	if slot == 0 {
		pf.ShowLeftPanel = true
	} else {
		pf.ShowRightPanel = true
	}
	pf.ResizeConsole(pf.LastW, pf.LastH)
	vtui.FrameManager.HardRefresh()
}

// pluginPanelInstance adapts the public vfs panel contract to f4's existing
// AltPanel slot. The FileSystemPanel remains underneath as the logical panel,
// so ordinary file actions and VFS providers keep their existing assumptions.
type PluginPanelInstance struct {
	providerID string
	title      string
	host       *PanelsFrame
	slot       int
	controller vfs.PanelController
	closeFn    func()
	closeOnce  sync.Once
}

func (p *PluginPanelInstance) Source() *FileSystemPanel {
	if p == nil || p.host == nil || p.slot < 0 || p.slot >= len(p.host.Panels) {
		return nil
	}
	fsp, _ := p.host.Panels[p.slot].(*FileSystemPanel)
	return fsp
}

func (p *PluginPanelInstance) Kind() string { return "plugin:" + p.providerID }

func (p *PluginPanelInstance) Show(scr *vtui.ScreenBuf) {
	if p == nil || p.controller == nil {
		return
	}
	if p.host != nil {
		p.SetContext(p.host.panelPluginContext(p.slot))
	}
	p.controller.Show(scr)
}

func (p *PluginPanelInstance) ProcessKey(e *vtinput.InputEvent) bool {
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

func (p *PluginPanelInstance) ProcessMouse(e *vtinput.InputEvent) bool {
	if p == nil || p.controller == nil {
		return false
	}
	if p.host != nil {
		p.SetContext(p.host.panelPluginContext(p.slot))
	}
	return p.controller.ProcessMouse(e)
}

func (p *PluginPanelInstance) SetFocus(focused bool) {
	if p != nil && p.controller != nil {
		p.controller.SetFocus(focused)
	}
}

func (p *PluginPanelInstance) IsFocused() bool {
	return p != nil && p.controller != nil && p.controller.IsFocused()
}

func (p *PluginPanelInstance) SetPosition(x1, y1, x2, y2 int) {
	if p != nil && p.controller != nil {
		p.controller.SetPosition(x1, y1, x2, y2)
	}
}

func (p *PluginPanelInstance) SetPositionForPanel() {
	if p == nil || p.host == nil || p.controller == nil {
		return
	}
	x1, y1, x2, y2 := p.host.Panels[p.slot].GetPosition()
	p.controller.SetPosition(x1, y1, x2, y2)
}

func (p *PluginPanelInstance) GetPosition() (int, int, int, int) {
	if p == nil || p.controller == nil {
		return 0, 0, 0, 0
	}
	return p.controller.GetPosition()
}

func (p *PluginPanelInstance) GetSelectedName() string {
	if p == nil || p.controller == nil {
		return ""
	}
	return p.controller.GetSelectedName()
}

func (p *PluginPanelInstance) SetContext(ctx vfs.PanelContext) {
	if p != nil && p.controller != nil {
		p.controller.SetContext(ctx)
	}
}

func (p *PluginPanelInstance) Close() {
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
	ctx := vfs.PanelContext{Side: slot, ActiveSide: pf.ActiveIdx}
	if slot == pf.ActiveIdx {
		ctx.Current = pf.panelPluginState(slot, true)
		ctx.Other = pf.panelPluginState(1-slot, false)
	} else {
		ctx.Current = pf.panelPluginState(slot, false)
		ctx.Other = pf.panelPluginState(1-slot, true)
	}
	if alt, ok := pf.AltPanels[slot].(*PluginPanelInstance); ok && alt.controller != nil {
		ctx.Bounds[0], ctx.Bounds[1], ctx.Bounds[2], ctx.Bounds[3] = alt.controller.GetPosition()
	} else if panel := pf.Panels[slot]; panel != nil {
		ctx.Bounds[0], ctx.Bounds[1], ctx.Bounds[2], ctx.Bounds[3] = panel.GetPosition()
	}
	return ctx
}

func (pf *PanelsFrame) panelPluginState(slot int, active bool) vfs.PanelState {
	state := vfs.PanelState{Side: slot, Active: active}
	if slot < 0 || slot >= len(pf.Panels) {
		return state
	}
	fsp, ok := pf.Panels[slot].(*FileSystemPanel)
	if !ok || fsp == nil || fsp.Vfs == nil {
		return state
	}
	state.Path = fsp.Vfs.GetPath()
	state.SelectedName = fsp.GetSelectedName()
	state.SelectedNames = append([]string(nil), fsp.GetSelectedNames()...)
	return state
}
