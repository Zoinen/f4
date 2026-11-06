package filegroups

import (
	"sync"

	"github.com/unxed/vtui"
)

// Runtime separates reversible appearance from committed command dispatch.
// The caller supplies a UI-thread notification; no renderer is imported here.
type Runtime struct {
	mu                         sync.RWMutex
	committed, preview         *Snapshot
	committedView, previewView *Snapshot
	iconTheme                  string
	changed                    func(*Snapshot)
	nextOwner, previewOwner    uint64
}

func NewRuntime(groups []Group, changed func(*Snapshot), themes ...[]ThemeSet) (*Runtime, error) {
	snapshot, err := Compile(groups, themes...)
	if err != nil {
		return nil, err
	}
	return &Runtime{committed: snapshot, committedView: snapshot.ForIconTheme("generic"), iconTheme: "generic", changed: changed}, nil
}

func (r *Runtime) IconTheme() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.iconTheme
}

// SetIconTheme changes application context, independently of editor ownership.
// Cancel can remove a draft, but cannot restore a stale application theme.
func (r *Runtime) SetIconTheme(theme string) {
	if theme == "" {
		theme = "generic"
	}
	r.mu.Lock()
	if r.iconTheme == theme {
		r.mu.Unlock()
		return
	}
	r.iconTheme = theme
	r.committedView = r.committed.ForIconTheme(theme)
	snapshot := r.committedView
	if r.preview != nil {
		r.previewView = r.preview.ForIconTheme(theme)
		snapshot = r.previewView
	}
	r.mu.Unlock()
	vtui.DebugLog("FILEGROUPS: active icon theme=%s revision=%s", theme, snapshot.Revision())
	if r.changed != nil {
		r.changed(snapshot)
	}
}

func (r *Runtime) Appearance() *Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.preview != nil {
		return r.previewView
	}
	return r.committedView
}

func (r *Runtime) Committed() *Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.committedView
}

func (r *Runtime) Preview(groups []Group) error {
	snapshot, err := Compile(groups, r.committedThemes())
	if err != nil {
		return err
	}
	snapshot, err = r.withCommittedColors(snapshot)
	if err != nil {
		return err
	}
	r.publishPreview(snapshot, 0)
	return nil
}

func (r *Runtime) publishPreview(snapshot *Snapshot, owner uint64) {
	r.mu.Lock()
	current := r.preview
	if current == nil {
		current = r.committed
	}
	if current.Revision() == snapshot.Revision() {
		if r.preview != nil {
			r.previewOwner = owner
		}
		r.mu.Unlock()
		return
	}
	r.preview = snapshot
	r.previewView = snapshot.ForIconTheme(r.iconTheme)
	view := r.previewView
	r.previewOwner = owner
	r.mu.Unlock()
	if r.changed != nil {
		r.changed(view)
	}
}

func (r *Runtime) Apply(groups []Group, themes ...[]ThemeSet) error {
	if len(themes) == 0 {
		themes = [][]ThemeSet{r.committedThemes()}
	}
	snapshot, err := Compile(groups, themes...)
	if err != nil {
		return err
	}
	snapshot, err = r.withCommittedColors(snapshot)
	if err != nil {
		return err
	}
	r.applySnapshot(snapshot)
	return nil
}

func (r *Runtime) ApplyColors(groups []Group, iconThemes []ThemeSet, colorThemes []ColorTheme, selected string) error {
	snapshot, err := Compile(groups, iconThemes)
	if err != nil {
		return err
	}
	snapshot, err = snapshot.WithColorThemes(colorThemes, selected)
	if err != nil {
		return err
	}
	r.applySnapshot(snapshot)
	return nil
}

func (r *Runtime) withCommittedColors(snapshot *Snapshot) (*Snapshot, error) {
	r.mu.RLock()
	colors, selected := r.committed.ColorThemes(), r.committed.ColorThemeID()
	r.mu.RUnlock()
	if len(colors) == 0 && selected == "" {
		return snapshot, nil
	}
	return snapshot.WithColorThemes(colors, selected)
}

func (r *Runtime) applySnapshot(snapshot *Snapshot) {
	r.mu.Lock()
	r.committed = snapshot
	r.committedView = snapshot.ForIconTheme(r.iconTheme)
	view := r.committedView
	r.preview = nil
	r.previewView = nil
	r.mu.Unlock()
	if r.changed != nil {
		r.changed(view)
	}
}

// SetColorTheme changes only colour selection, sharing all compiled predicates.
// An active preview keeps its edits; Cancel reveals the new committed selection.
func (r *Runtime) SetColorTheme(selected string) error {
	r.mu.Lock()
	committed, err := r.committed.WithColorThemes(r.committed.colorThemes, selected)
	if err != nil {
		r.mu.Unlock()
		return err
	}
	var preview *Snapshot
	if r.preview != nil {
		preview, err = r.preview.WithColorThemes(r.preview.colorThemes, selected)
		if err != nil {
			r.mu.Unlock()
			return err
		}
	}
	r.committed = committed
	r.committedView = committed.ForIconTheme(r.iconTheme)
	view := r.committedView
	if preview != nil {
		r.preview = preview
		r.previewView = preview.ForIconTheme(r.iconTheme)
		view = r.previewView
	}
	r.mu.Unlock()
	if r.changed != nil {
		r.changed(view)
	}
	return nil
}

func (r *Runtime) committedThemes() []ThemeSet {
	r.mu.RLock()
	defer r.mu.RUnlock()
	sets := []ThemeSet{}
	for _, set := range r.committed.themes {
		sets = append(sets, set)
	}
	return sets
}

func (r *Runtime) CancelPreview() {
	r.cancelPreview(0, true)
}

func (r *Runtime) cancelPreview(owner uint64, force bool) {
	r.mu.Lock()
	if !force && r.previewOwner != owner {
		r.mu.Unlock()
		return
	}
	hadPreview := r.preview != nil
	r.preview = nil
	r.previewView = nil
	snapshot := r.committedView
	r.mu.Unlock()
	if hadPreview && r.changed != nil {
		r.changed(snapshot)
	}
}

// PreviewSession owns only its own unapplied snapshot. Closing an older editor
// cannot cancel a newer editor's preview or roll back an applied baseline.
type PreviewSession struct {
	runtime *Runtime
	owner   uint64
}

func (r *Runtime) BeginPreview() *PreviewSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextOwner++
	return &PreviewSession{runtime: r, owner: r.nextOwner}
}

func (s *PreviewSession) Publish(snapshot *Snapshot) { s.runtime.publishPreview(snapshot, s.owner) }
func (s *PreviewSession) Cancel()                    { s.runtime.cancelPreview(s.owner, false) }
