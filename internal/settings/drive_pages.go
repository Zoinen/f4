package settings

import (
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

// The drive menu's F9 window shows the Drive chooser category as separate
// pages, Drive options, Bookmarks and Links (f4#1148), instead of one long page
// with a five-line list in the middle. The category itself, in the main
// Settings window, is left as it is: these are only the pages of a copy of the
// catalog made for that one window.
const (
	driveOptionsPage   = "drives.options"
	driveBookmarksPage = "drives.bookmarks"
	driveLinksPage     = "drives.links"
)

var driveChooserPages = []f4settings.Category{
	{ID: driveOptionsPage, Label: f4settings.Text{Key: "SettingsCenter.DriveTab.Options", English: "Drive options"}},
	{ID: driveBookmarksPage, Label: f4settings.Text{Key: "SettingsCenter.DriveTab.Bookmarks", English: "Bookmarks"}},
	{ID: driveLinksPage, Label: f4settings.Text{Key: "SettingsCenter.DriveTab.Links", English: "Links"}},
}

func driveChooserPageIDs() []string {
	ids := make([]string, len(driveChooserPages))
	for i, page := range driveChooserPages {
		ids[i] = page.ID
	}
	return ids
}

func driveChooserPageOf(collection string) string {
	switch collection {
	case "bookmarks":
		return driveBookmarksPage
	case "drive-links":
		return driveLinksPage
	}
	return driveOptionsPage
}

// splitDriveChooser moves what the sessions keep in the "drives" category onto
// the pages above. The catalogs are copied, never edited in place.
func splitDriveChooser(sessions []*settingsSession) {
	for _, s := range sessions {
		cat := s.catalog
		cat.Categories = nil
		for _, c := range s.catalog.Categories {
			if c.ID == "drives" {
				cat.Categories = append(cat.Categories, driveChooserPages...)
				continue
			}
			cat.Categories = append(cat.Categories, c)
		}
		cat.Fields = append([]f4settings.Field(nil), s.catalog.Fields...)
		for i := range cat.Fields {
			if cat.Fields[i].Category == "drives" {
				cat.Fields[i].Category = driveOptionsPage
			}
		}
		cat.Collections = append([]f4settings.Collection(nil), s.catalog.Collections...)
		for i := range cat.Collections {
			if cat.Collections[i].Category == "drives" {
				cat.Collections[i].Category = driveChooserPageOf(cat.Collections[i].ID)
			}
		}
		cat.Commands = append([]f4settings.Command(nil), s.catalog.Commands...)
		for i := range cat.Commands {
			if cat.Commands[i].Category == "drives" {
				cat.Commands[i].Category = driveOptionsPage
			}
		}
		s.catalog = cat
	}
}

func showDriveChooser(sessions []*settingsSession) bool {
	splitDriveChooser(sessions)
	c := newSettingsCenter(sessions)
	c.fullLists = true
	c.fieldTitle = Phrase("Drive chooser")
	c.restrictTo(driveChooserPageIDs()...)
	c.navigate(driveOptionsPage, "", "", false)
	c.ResizeConsole(vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight())
	vtui.FrameManager.Push(c)
	c.refreshSchemeChoices()
	return true
}
