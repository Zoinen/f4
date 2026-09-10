package settings

import (
	"fmt"
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

func TestSettingsCollectionInputSurfacePalette(t *testing.T) {
	palette := append([]uint64(nil), vtui.Palette...)
	defer func() { vtui.Palette = palette }()
	col := f4settings.Collection{ID: "bindings", Category: "keyboard", Group: "Bindings", Label: f4settings.Text{English: "Bindings"}, NameField: "name", Fixed: true}
	var records []f4settings.Record
	for i := 0; i < 10; i++ {
		records = append(records, f4settings.Record{ID: fmt.Sprint(i), Values: map[string]string{"name": fmt.Sprintf("Binding %d", i)}})
	}
	d := f4settings.NewDraft(nil, map[string][]f4settings.Record{"bindings": records})
	defer d.Close()
	c := newSettingsCenter([]*settingsSession{{catalog: f4settings.Catalog{ID: "test", Categories: Categories, Collections: []f4settings.Collection{col}}, draft: d}})
	c.selectCategory("keyboard")
	c.SetPosition(0, 0, 149, 39)
	var table *vtui.Table
	for _, r := range c.page.rows {
		if t, ok := r.control.(*vtui.Table); ok {
			table = t
			break
		}
	}
	if table == nil {
		t.Fatal("missing collection")
	}
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(150, 40)
	for iteration := 0; iteration < 2; iteration++ {
		for _, slot := range []int{vtui.ColDialogText, vtui.ColDialogEdit, vtui.ColDialogSelectedButton, vtui.ColDialogHighlightText, vtui.ColDialogHighlightSelectedButton, vtui.ColDialogBox, vtui.ColDialogBoxTitle} {
			vtui.Palette[slot] = vtui.SetRGBBoth(0, testutil.Uint32(0x8090a0+slot*100+iteration*0x101010), testutil.Uint32(0x102030+slot*100+iteration*0x101010))
		}
		for _, active := range []bool{false, true} {
			if active {
				c.SetFocusedItem(c.page)
				c.page.SetFocusedItem(table)
			} else {
				c.SetFocusedItem(c.sidebar)
			}
			for _, query := range []string{"", "no-match-xyz"} {
				c.query = query
				c.updateMatches()
				c.Show(scr)
				for row := 0; row < 2; row++ {
					want := vtui.Palette[vtui.ColDialogEdit]
					if row == 0 && active {
						want = vtui.Palette[vtui.ColDialogSelectedButton]
					}
					if query != "" {
						want = vtui.DimColor(vtui.DimColor(want))
					}
					if got := scr.GetCell(table.X1, table.Y1+row).Attributes; got != want {
						t.Fatalf("row %d active=%v query=%q: %x want %x", row, active, query, got, want)
					}
				}
				border := vtui.Palette[vtui.ColDialogBox]
				if query != "" {
					border = vtui.DimColor(border)
				}
				if scr.GetCell(c.page.X1, table.Y1).Attributes != border {
					t.Fatal("group border palette")
				}
				bar := table.ScrollBar
				if scr.GetCell(bar.X1, bar.Y1).Attributes != border {
					t.Fatal("list scrollbar palette")
				}
			}
		}
		table.SetRows(nil)
		c.query = ""
		c.updateMatches()
		c.Show(scr)
		if scr.GetCell(table.X1, table.Y1+1).Attributes != vtui.Palette[vtui.ColDialogEdit] {
			t.Fatal("empty list must keep input surface")
		}
		var rows []vtui.TableRow
		for _, record := range records {
			rows = append(rows, settingsRecordRow{c, col, record})
		}
		table.SetRows(rows)
	}
}
