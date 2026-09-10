package settings

import (
	"github.com/unxed/f4/internal/panel"

	"context"
	"fmt"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
	"strconv"
	"strings"
)

func OpenUserMenu(state panel.MenuSettingsSource, current *vtui.VMenu, index int, create, submenu bool) bool {
	if vtui.FrameManager == nil {
		return false
	}
	scope := "local"
	switch state.Mode {
	case panel.MenuModeMain:
		scope = "main"
	case panel.MenuModeFar:
		scope = "binary"
	}
	prefix := "menu." + scope + "."
	collection := "usermenu." + scope
	store := newUserMenuSettingsStore(settingsMenuSource{scope, state.RootTitle, state.SourcePath, state.Mode})
	target := ""
	parent := ""
	var rows []f4settings.Record
	var walk func([]panel.UserMenuItem, []int, string)
	walk = func(items []panel.UserMenuItem, path []int, parentID string) {
		if fmt.Sprint(path) == fmt.Sprint(state.Path) {
			parent = parentID
		}
		for i, item := range items {
			id := fmt.Sprintf("%s:%d", scope, len(rows))
			itemPath := append(append([]int(nil), path...), i)
			if fmt.Sprint(path) == fmt.Sprint(state.Path) && i == index {
				target = id
				parent = parentID
			}
			rows = append(rows, f4settings.Record{ID: id, Values: map[string]string{prefix + "Label": item.Label, prefix + "HotKey": item.HotKey, prefix + "Commands": strings.Join(item.Commands, "\n"), prefix + "Submenu": strconv.FormatBool(item.Submenu != nil), prefix + "Parent": parentID}})
			walk(item.Submenu, itemPath, id)
		}
	}
	walk(state.RootItems, nil, "")
	store.load = func() ([]f4settings.Record, error) { return rows, nil }
	store.afterSave = func(rows []f4settings.Record) {
		if items, err := settingsMenuTree(rows, prefix); err == nil {
			state.Saved(items)
		}
	}
	sessions, err := beginSettingsSessions(context.Background())
	if err != nil {
		vtui.ShowMessage("Settings", err.Error(), []string{i18n.Msg("vtui.Ok")})
		return true
	}
	for i, s := range sessions {
		p, ok := s.provider.(coreRecordSettingsProvider)
		if !ok {
			continue
		}
		s.draft.Close()
		for j, old := range p.stores {
			if old.collection.ID == collection {
				p.stores[j] = store
			}
		}
		d, err := p.Begin(context.Background())
		if err != nil {
			for _, s := range sessions {
				s.draft.Close()
			}
			vtui.ShowMessage("Settings", err.Error(), []string{i18n.Msg("vtui.Ok")})
			return true
		}
		sessions[i] = &settingsSession{provider: p, catalog: p.Catalog(), draft: d}
	}
	showSettingsCenter(sessions, "menus", collection, target, create)
	if center, ok := vtui.FrameManager.GetTopFrame().(*settingsCenter); ok && state.Closed != nil {
		closed := center.OnResult
		center.OnResult = func(result int) { closed(result); state.Closed(index) }
	}

	if create {
		if c, ok := vtui.FrameManager.GetTopFrame().(*settingsCenter); ok {
			for _, s := range c.sessions {
				records := s.draft.Records[collection]
				if len(records) > 0 {
					r := records[len(records)-1]
					r.Values[prefix+"Submenu"] = strconv.FormatBool(submenu)
					r.Values[prefix+"Parent"] = parent
				}
			}
			c.rebuildCategory()
		}
	}
	return true
}
