package settings

import (
	"context"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

// OpenRecord edits one record without opening unrelated providers or settings.
// The full owning draft is retained so saving cannot discard sibling records.
func OpenRecord(collection, record string, create bool, applied func()) bool {
	if vtui.FrameManager == nil {
		return false
	}
	settingsProviders.RLock()
	providers := append([]f4settings.Provider{}, settingsProviders.providers...)
	settingsProviders.RUnlock()
	for _, provider := range providers {
		catalog := provider.Catalog()
		for _, col := range catalog.Collections {
			if col.ID != collection {
				continue
			}
			draft, err := provider.Begin(context.Background())
			if err != nil {
				vtui.ShowMessage(col.Label.Resolve(config.App.Language, i18n.Msg),
					err.Error(), []string{i18n.Msg("vtui.Ok")})
				return true
			}
			if !create {
				found := false
				for _, r := range draft.Records[col.ID] {
					found = found || r.ID == record || r.Values[col.NameField] == record || r.Values["__id"] == record
				}
				if !found {
					draft.Close()
					return false
				}
			}
			catalog.Fields, catalog.Commands = nil, nil
			catalog.Collections = []f4settings.Collection{col}
			session := &settingsSession{provider: provider, catalog: catalog, draft: draft, contributed: true}
			c := newSettingsCenter([]*settingsSession{session})
			c.navigate(col.Category, collection, record, create)
			c.onApplied = applied
			c.configureRecordDialog(col.Label.Resolve(config.App.Language, i18n.Msg))
			vtui.FrameManager.Push(c)
			vtui.DebugLog("[FIX:connection-dialog] opened collection=%s create=%v", collection, create)
			return true
		}
	}
	return false
}

// configureRecordDialog reuses the Settings fields and transaction lifecycle
// while presenting only the selected record, without global navigation.
func (c *settingsCenter) configureRecordDialog(title string) {
	c.recordOnly = true
	c.recordTitle = title
	c.SetId("settings-record-dialog")
	for _, item := range []vtui.UIElement{c.search, c.sidebar, c.clearSearch, c.previous, c.next} {
		item.SetVisible(false)
		item.SetDisabled(true)
	}
	c.rebuildCategory()
	c.page.scroll = 0
	c.page.positionRows()
	c.ResizeConsole(vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight())
	c.SetFocusedItem(c.page)
}
