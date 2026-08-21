package main

import (
	"errors"
	"sync"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// Keep optional plugin observers outside PanelsFrame itself.  A PanelsFrame is
// cloned by the workspace code and tests construct lightweight values directly;
// an external registry avoids copying mutexes or subscriptions into a clone.
type panelObserverState struct {
	mu        sync.Mutex
	nextID    uint64
	observers map[uint64]vfs.PanelObserver
	last      [2]vfs.PanelSnapshot
	haveLast  [2]bool
}

var panelObserverStates sync.Map // map[*PanelsFrame]*panelObserverState

func panelObserverStateFor(pf *PanelsFrame) *panelObserverState {
	if pf == nil {
		return nil
	}
	created := &panelObserverState{observers: make(map[uint64]vfs.PanelObserver)}
	actual, _ := panelObserverStates.LoadOrStore(pf, created)
	return actual.(*panelObserverState)
}

// PanelSnapshot implements vfs.PanelHost.  It has no I/O side effects and is
// intentionally safe to call from a plugin command before it starts a worker.
func (pf *PanelsFrame) PanelSnapshot(side vfs.PanelSide) vfs.PanelSnapshot {
	snapshot := vfs.PanelSnapshot{Side: side}
	if pf == nil {
		return snapshot
	}
	var panel Panel
	if side == vfs.PanelPassive {
		panel = pf.Passive()
	} else {
		panel = pf.Active()
	}
	fsp, ok := panel.(*FileSystemPanel)
	if !ok || fsp == nil || fsp.vfs == nil {
		return snapshot
	}
	snapshot.VFS = fsp.vfs
	snapshot.Path = fsp.persistentPath()
	snapshot.SelectedName = fsp.GetSelectedName()
	return snapshot
}

// ObservePanelChanges implements vfs.PanelHost.  Notifications occur on the
// UI goroutine and are coalesced to real snapshot changes during rendering.
func (pf *PanelsFrame) ObservePanelChanges(observer vfs.PanelObserver) vfs.Registration {
	if pf == nil || observer == nil {
		return &unregisterFunc{}
	}
	state := panelObserverStateFor(pf)
	state.mu.Lock()
	state.nextID++
	id := state.nextID
	state.observers[id] = observer
	state.mu.Unlock()

	return &unregisterFunc{fn: func() {
		state.mu.Lock()
		delete(state.observers, id)
		state.mu.Unlock()
	}}
}

func samePanelSnapshot(a, b vfs.PanelSnapshot) bool {
	return a.Side == b.Side && a.Path == b.Path && a.SelectedName == b.SelectedName && sameVFSInstance(a.VFS, b.VFS)
}

// samePanelNavigationTarget intentionally excludes SelectedName. A directory
// reload can briefly select the synthetic ".." row while restoring the prior
// cursor, and treating that display-only transition as navigation creates a
// feedback loop for cache warmers which refresh the current panel on completion.
func samePanelNavigationTarget(a, b vfs.PanelSnapshot) bool {
	return a.Side == b.Side && a.Path == b.Path && sameVFSInstance(a.VFS, b.VFS)
}

type panelSnapshotChange struct {
	snapshot   vfs.PanelSnapshot
	navigation bool
}

// publishPanelSnapshots is called from PanelsFrame.Show, where panel paths and
// active/passive orientation are already stable.  A slow plugin callback is a
// plugin bug, but dispatching outside the lock prevents a callback from
// deadlocking while it unregisters itself.
func (pf *PanelsFrame) publishPanelSnapshots() {
	stateValue, hasState := panelObserverStates.Load(pf)
	if !hasState && !hasPanelNavigationProviders() {
		return
	}
	state := panelObserverStateFor(pf)
	if hasState {
		state = stateValue.(*panelObserverState)
	}
	snapshots := [2]vfs.PanelSnapshot{
		pf.PanelSnapshot(vfs.PanelActive),
		pf.PanelSnapshot(vfs.PanelPassive),
	}

	state.mu.Lock()
	changed := make([]panelSnapshotChange, 0, 2)
	for index, snapshot := range snapshots {
		previous := state.last[index]
		hadPrevious := state.haveLast[index]
		if !hadPrevious || !samePanelSnapshot(previous, snapshot) {
			state.last[index] = snapshot
			state.haveLast[index] = true
			changed = append(changed, panelSnapshotChange{
				snapshot:   snapshot,
				navigation: !hadPrevious || !samePanelNavigationTarget(previous, snapshot),
			})
		}
	}
	observers := make([]vfs.PanelObserver, 0, len(state.observers))
	for _, observer := range state.observers {
		observers = append(observers, observer)
	}
	state.mu.Unlock()

	for _, change := range changed {
		for _, observer := range observers {
			observer(change.snapshot)
		}
		if change.navigation {
			notifyPanelNavigationProviders(pf, change.snapshot)
		}
	}
}

// OpenPassiveVFS implements vfs.PanelHost.  It replaces only the passive file
// panel, preserving the source panel and using f4's normal close/read lifecycle.
func (pf *PanelsFrame) OpenPassiveVFS(filesystem vfs.VFS) error {
	if pf == nil {
		return errors.New("panel host is unavailable")
	}
	if filesystem == nil {
		return errors.New("cannot open a nil VFS")
	}
	passive := pf.getInactivePanel()
	if passive == nil {
		return errors.New("passive panel is unavailable")
	}
	pf.switchToVFS(passive, filesystem)
	pf.publishPanelSnapshots()
	return nil
}

// RefreshVFS implements vfs.PanelHost.  It is safe for a background worker to
// call: the actual directory reload is posted to f4's UI goroutine.
func (pf *PanelsFrame) RefreshVFS(filesystem vfs.VFS) {
	if pf == nil || filesystem == nil {
		return
	}
	vtui.FrameManager.PostTask(func() {
		if pf.closed {
			return
		}
		for _, panel := range pf.panels {
			fsp, ok := panel.(*FileSystemPanel)
			if !ok || fsp == nil || !sameVFSInstance(fsp.vfs, filesystem) {
				continue
			}
			// Git and other cache-backed virtual VFSes may change without their
			// path changing.  Drop only this panel's current cached listing.
			delete(fsp.dirCache, fsp.cacheKey(fsp.vfs.GetPath()))
			fsp.ReadDirectory()
		}
		pf.publishPanelSnapshots()
		// ReadDirectory already schedules its own repaint. A global RefreshAll
		// here would reload unrelated panels and turn a cache-only decoration
		// update into a repeated selection/navigation feedback loop.
		vtui.FrameManager.Redraw()
	})
}

var _ vfs.PanelHost = (*PanelsFrame)(nil)
