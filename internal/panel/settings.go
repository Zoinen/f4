package panel

import (
	"github.com/unxed/vtui"
)

// OpenSettingsAt is wired by the application to the canonical Settings Center.
var OpenSettingsAt func(category, collection, record string, create bool) bool

var OpenSettingsRecordAt func(collection, record string, create bool, applied func()) bool

func (pf *PanelsFrame) OpenSettingsRecord(collection, record string, create bool) bool {
	return OpenSettingsRecordAt != nil && OpenSettingsRecordAt(collection, record, create, pf.RefreshAll)
}

func (*PanelsFrame) OpenSettings(category, collection, record string, create bool) bool {
	if OpenSettingsAt == nil {
		return false
	}
	return OpenSettingsAt(category, collection, record, create)
}

// SetSettingsRecordDefault seeds a newly opened record without persisting it.
var SetSettingsRecordDefault func(collection, field, value string)

// MenuSettingsSource captures the scope and draft tree of the active user menu.
type MenuSettingsSource struct {
	Mode                  MenuMode
	RootTitle, SourcePath string
	Path                  []int
	RootItems             []UserMenuItem
	Saved                 func([]UserMenuItem)
	Closed                func(int)
}

var OpenUserMenuSettings func(MenuSettingsSource, *vtui.VMenu, int, bool, bool) bool
