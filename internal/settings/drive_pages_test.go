package settings

import (
	"context"
	"fmt"
	"testing"

	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

// f4#1148: F9 in the drive menu shows Drive options, Bookmarks and Links as
// pages of their own, and each record list in full.
func TestDriveChooserPagesSplitTheCategory(t *testing.T) {
	var records []f4settings.Record
	for i := 0; i < 9; i++ {
		records = append(records, f4settings.Record{ID: fmt.Sprint(i), Values: map[string]string{
			"link.Name": fmt.Sprintf("Link %d", i), "link.Path": fmt.Sprintf("/path/%d", i)}})
	}
	d := f4settings.NewDraft(nil, map[string][]f4settings.Record{"drive-links": records})
	defer d.Close()
	session := &settingsSession{catalog: f4settings.Catalog{ID: "test", Categories: Categories,
		Collections: []f4settings.Collection{driveLinksCollection()}}, draft: d}
	original := session.catalog

	splitDriveChooser([]*settingsSession{session})
	if original.Collections[0].Category != "drives" {
		t.Fatalf("the shared catalog was edited in place: %q", original.Collections[0].Category)
	}
	c := newSettingsCenter([]*settingsSession{session})
	c.fullLists = true
	c.restrictTo(driveChooserPageIDs()...)
	c.SetPosition(0, 0, 129, 44)

	var ids []string
	for _, cat := range c.categories {
		ids = append(ids, cat.ID)
	}
	if fmt.Sprint(ids) != fmt.Sprint(driveChooserPageIDs()) {
		t.Fatalf("pages = %v, want %v", ids, driveChooserPageIDs())
	}
	if c.category != driveOptionsPage {
		t.Fatalf("opened on %q", c.category)
	}
	for _, row := range c.page.rows {
		if row.control != nil && row.control.GetId() == "collection:drive-links" {
			t.Fatal("the Links list is on the Drive options page")
		}
	}

	c.selectCategory(driveLinksPage)
	found := false
	for _, row := range c.page.rows {
		if row.control != nil && row.control.GetId() == "collection:drive-links" {
			found = true
			if row.controlHeight != len(records) {
				t.Fatalf("list height %d, want all %d rows", row.controlHeight, len(records))
			}
		}
	}
	if !found {
		t.Fatal("the Links page has no list")
	}
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(130, 45)
	c.Show(scr)
	if got := c.categoryLabel(driveLinksPage); got != "Links" {
		t.Fatalf("page label %q", got)
	}
}

// The real catalogs put bookmarks and drive links on their own pages and keep
// the drive menu's options on the first one.
func TestDriveChooserPagesOnRealCatalogs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	sessions, err := beginSettingsSessions(context.Background())
	if err != nil {
		t.Skipf("settings sessions are not available here: %v", err)
	}
	defer func() {
		for _, s := range sessions {
			s.draft.Close()
		}
	}()
	splitDriveChooser(sessions)
	pages := map[string]int{}
	for _, s := range sessions {
		for _, f := range s.catalog.Fields {
			if f.Category == "drives" {
				t.Errorf("field %s is still in the drives category", f.ID)
			}
			pages[f.Category]++
		}
		for _, col := range s.catalog.Collections {
			if col.Category == "drives" {
				t.Errorf("collection %s is still in the drives category", col.ID)
			}
			pages[col.Category]++
		}
	}
	for _, page := range driveChooserPageIDs() {
		if pages[page] == 0 {
			t.Errorf("page %s has nothing on it", page)
		}
	}
}
