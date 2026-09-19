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
			if state.Saved != nil {
				state.Saved(items)
			}
		}
	}
	provider := coreRecordSettingsProvider{stores: []settingsRecordStore{store}}
	draft, err := provider.Begin(context.Background())
	if err != nil {
		vtui.ShowMessage(i18n.Msg("UserMenu.EditTitle"), err.Error(), []string{i18n.Msg("vtui.Ok")})
		return true
	}
	session := &settingsSession{provider: provider, catalog: provider.Catalog(), draft: draft}
	center := newSettingsCenter([]*settingsSession{session})
	center.navigate("menus", collection, target, create)
	if create {
		records := draft.Records[collection]
		if len(records) > 0 {
			record := records[len(records)-1]
			record.Values[prefix+"Submenu"] = strconv.FormatBool(submenu)
			record.Values[prefix+"Parent"] = parent
		}
	}
	title := i18n.Msg("UserMenu.EditTitle")
	if create {
		title = i18n.Msg("UserMenu.CreateTitle")
		if submenu {
			title = i18n.Msg("UserMenu.CreateSubmenuTitle")
		}
	}
	center.configureRecordDialog(title)
	if state.Closed != nil {
		closed := center.OnResult
		center.OnResult = func(result int) { closed(result); state.Closed(index) }
	}
	vtui.FrameManager.Push(center)
	return true
}

func (c *settingsCenter) importFar3UserMenu() {
	panel.ShowFar3UserMenuImport(func(items []panel.UserMenuItem) error {
		for _, session := range c.sessions {
			if _, exists := session.draft.Records["usermenu.main"]; !exists {
				continue
			}
			if err := stageFar3UserMenu(session.draft, items); err != nil {
				return err
			}
			c.rebuildCategory()
			return nil
		}
		return fmt.Errorf("global user menu is unavailable")
	})
}

func stageFar3UserMenu(draft *f4settings.Draft, items []panel.UserMenuItem) error {
	const prefix = "menu.main."
	existing, err := settingsMenuTree(draft.Records["usermenu.main"], prefix)
	if err != nil {
		return err
	}
	merged, added := panel.MergeUserMenus(existing, items)
	if added == 0 {
		return nil
	}
	var rows []f4settings.Record
	var walk func([]panel.UserMenuItem, string)
	walk = func(items []panel.UserMenuItem, parent string) {
		for _, item := range items {
			id := fmt.Sprintf("main:%d", len(rows))
			rows = append(rows, f4settings.Record{ID: id, Values: map[string]string{
				prefix + "Label": item.Label, prefix + "HotKey": item.HotKey,
				prefix + "Commands": strings.Join(item.Commands, "\n"),
				prefix + "Submenu":  strconv.FormatBool(item.Submenu != nil), prefix + "Parent": parent,
			}})
			walk(item.Submenu, id)
		}
	}
	walk(merged, "")
	draft.Records["usermenu.main"] = rows
	return nil
}
