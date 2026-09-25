package settings

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/gui"
	"github.com/unxed/f4/internal/settingstest"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestSettingsChoiceDescriptionsCoverageAndLocalization(t *testing.T) {
	for key, description := range settingsChoiceDescriptions {
		parts := strings.SplitN(key, "|", 2)
		field := f4settings.Scalar(parts[0], "startup", "Launch defaults", "Graphical renderer", description.English, f4settings.ChoiceKind)
		field.Description = description
		settingstest.RussianCatalog(t, f4settings.Catalog{ID: key, Fields: []f4settings.Field{field}})
	}
	for _, field := range (coreSettingsProvider{}).Catalog().Fields {
		for _, choice := range field.Choices {
			if choice.Description.English == "" {
				t.Errorf("%s/%s has no choice help", field.ID, choice.Value)
			}
		}
	}
	field := settingsFieldChoiceHelp(f4settings.Field{ID: "GuiBackend", Choices: []f4settings.Choice{{Value: "gogpu"}}})
	if !f4settings.Matches("drivers FFI", field, "", "en", nil) {
		t.Fatal("choice explanations are not searchable")
	}
	provided := f4settings.Text{English: "Frontend-owned explanation", Translations: map[string]string{"ru": "Описание от графического интерфейса"}}
	field.Choices[0].Description = provided
	field = settingsFieldChoiceHelp(field)
	if field.Choices[0].Description.Resolve("ru", nil) != provided.Translations["ru"] {
		t.Fatal("provider description was overwritten")
	}
}

func TestSettingsDropdownHoverHelpDoesNotChangeDraft(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	oldConfig := config.App
	defer func() { config.App = oldConfig; InitLang() }()
	palette := append([]uint64(nil), vtui.Palette...)
	defer copy(vtui.Palette, palette)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(150, 40)
	vtui.FrameManager.Init(scr)
	d := f4settings.NewDraft(map[string]string{"GuiBackend": "win32"}, nil)
	defer d.Close()
	field := f4settings.Scalar("GuiBackend", "startup", "Launch defaults", "Graphical renderer", "Default graphical renderer.", f4settings.ChoiceKind)
	field.Choices = f4settings.Choices("win32:win32", "gogpu:gogpu", "ebiten:ebiten")
	field.Choices = append(field.Choices, f4settings.Choices("a:A", "b:B", "c:C", "d:D", "e:E", "f:F", "g:G", "h:H")...)
	c := newSettingsCenter([]*settingsSession{{catalog: f4settings.Catalog{ID: "test", Categories: Categories, Fields: []f4settings.Field{field}}, draft: d}})
	c.selectCategory("startup")
	c.SetPosition(0, 0, 149, 39)
	vtui.FrameManager.Push(c)
	defer vtui.FrameManager.RemoveFrame(c)
	var combo *vtui.ComboBox
	for _, row := range c.page.rows {
		if b, ok := row.control.(*vtui.ComboBox); ok {
			combo = b
			break
		}
	}
	if combo == nil {
		t.Fatal("missing dropdown")
	}
	defer vtui.FrameManager.RemoveFrame(combo.Menu)
	c.SetFocusedItem(c.page)
	c.page.SetFocusedItem(combo)
	for iteration, language := range []string{"en", "ru"} {
		config.App.Language = language
		config.App.UseLocalLanguageFiles = false
		InitLang()
		vtui.Palette[vtui.ColDialogText] = vtui.SetRGBBoth(0, uint32(0xc0c0c0+iteration*0x101010), 0x202020)
		vtui.Palette[vtui.ColDialogComboSelectedText] = vtui.SetRGBBoth(0, 0xffffff, uint32(0x304080+iteration*0x101010))
		vtui.Palette[vtui.ColDialogComboBox] = vtui.SetRGBBoth(0, uint32(0x808080+iteration*0x101010), 0x202020)
		vtui.Palette[vtui.ColDialogComboScrollbar] = vtui.SetRGBBoth(0, uint32(0xb09070+iteration*0x101010), 0x282828)
		combo.Open()
		c.Show(scr)
		if !strings.Contains(c.help.text, "GDI") {
			t.Fatal("opening dropdown did not explain initial choice")
		}
		combo.Menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
		if !strings.Contains(c.help.text, "FFI") {
			t.Fatal("choice callback did not update help before repaint")
		}
		c.Show(scr)
		combo.Menu.Show(scr)
		if !strings.Contains(c.help.text, "GoGPU") || !strings.Contains(c.help.text, "FFI") {
			t.Fatal("keyboard highlight did not explain renderer tradeoffs")
		}
		if scr.GetCell(c.help.X1, c.help.Y1).Attributes != vtui.Palette[vtui.ColDialogText] {
			t.Fatal("choice help does not follow dialog palette")
		}
		if scr.GetCell(combo.Menu.X1, combo.Menu.Y1).Attributes != vtui.Palette[vtui.ColDialogComboBox] {
			t.Fatal("dropdown border does not follow dialog palette")
		}
		if scr.GetCell(combo.Menu.X1+1, combo.Menu.Y1+2).Attributes != vtui.Palette[vtui.ColDialogComboSelectedText] {
			t.Fatal("highlighted choice does not follow dialog palette")
		}
		bar := combo.Menu.ScrollBar
		if scr.GetCell(bar.X1, bar.Y1).Attributes != vtui.Palette[vtui.ColDialogComboScrollbar] {
			t.Fatal("dropdown scrollbar does not follow dialog palette")
		}
		combo.Menu.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: testutil.Int16(combo.Menu.X1 + 1), MouseY: testutil.Int16(combo.Menu.Y1 + 3), MouseEventFlags: vtinput.MouseMoved})
		c.Show(scr)
		if !strings.Contains(c.help.text, "Ebitengine") {
			t.Fatal("mouse hover did not update explanation")
		}
		if d.Values["GuiBackend"] != "win32" || len(d.Changed()) != 0 {
			t.Fatal("browsing modified the draft")
		}
		combo.Menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE})
		vtui.FrameManager.RemoveFrame(combo.Menu)
		c.Show(scr)
		if combo.Menu.SelectPos != 0 || strings.Contains(c.help.text, "Ebitengine") {
			t.Fatal("Escape did not restore the field explanation and original selection")
		}
	}
	c.SetPosition(0, 0, 79, 24)
	combo.Open()
	c.Show(scr)
	if combo.Menu.Y2 >= c.help.Y1 || combo.Menu.X1 < c.page.X1 || combo.Menu.X2 > c.page.X2 {
		t.Fatal("narrow dropdown covers the explanation pane or category list")
	}
	combo.Menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DOWN})
	c.Show(scr)
	if !strings.Contains(c.help.text, "GoGPU") {
		t.Fatal("narrow dropdown stopped updating help")
	}
	combo.Menu.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN})
	vtui.FrameManager.RemoveFrame(combo.Menu)
	c.Show(scr)
	if d.Values["GuiBackend"] != "gogpu" {
		t.Fatal("confirming dropdown did not stage the highlighted choice")
	}
}

// TestSettingsDropdownArrowClickKeepsMenuOpen covers issue #1156. A press on
// a dropdown's arrow opens the list below the field and hands the press over
// to it; the Center then fits the list into the content pane before the
// release arrives. It used to do so by pulling the list up over the field, so
// the release landed on an item: the list picked it and closed at once. The
// Appearance page is exercised at the default GUI size, with font names wider
// than the field, at every scroll offset that shows the font field.
func TestSettingsDropdownArrowClickKeepsMenuOpen(t *testing.T) {
	oldDiscover := gui.DiscoverInstalledGuiFonts
	gui.DiscoverInstalledGuiFonts = func(string) []string {
		fonts := make([]string, 30)
		for i := range fonts {
			fonts[i] = fmt.Sprintf("/fonts/Cascadia Code ExtraLight Italic %02d.ttf", i)
		}
		return fonts
	}
	t.Cleanup(func() { gui.DiscoverInstalledGuiFonts = oldDiscover })

	const width, height = 100, 30
	mouse := func(x, y int, down, moved bool) {
		e := &vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: testutil.Int16(x), MouseY: testutil.Int16(y), KeyDown: down}
		if down {
			e.ButtonState = vtinput.FromLeft1stButtonPressed
		}
		if moved {
			e.MouseEventFlags = vtinput.MouseMoved
		}
		vtui.FrameManager.InjectEvents([]*vtinput.InputEvent{e})
		vtui.FrameManager.Step(0)
	}
	checked, maxScroll := 0, 0
	for scroll := 0; scroll <= maxScroll; scroll++ {
		t.Run(fmt.Sprintf("scroll %d", scroll), func(t *testing.T) {
			t.Cleanup(testutil.SwapFrameManager(t))
			scr := vtui.NewSilentScreenBuf()
			scr.AllocBuf(width, height)
			vtui.FrameManager.Init(scr)
			provider := coreSettingsProvider{}
			d, err := provider.Begin(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			c := newSettingsCenter([]*settingsSession{{catalog: provider.Catalog(), draft: d}})
			c.navigate("appearance", "", "", false)
			c.ResizeConsole(width, height)
			vtui.FrameManager.Push(c)
			c.Show(scr)
			maxScroll = c.page.bar.Max
			c.page.scroll = scroll
			c.page.positionRows()
			c.Show(scr)
			var combo *vtui.ComboBox
			for _, row := range c.page.rows {
				if row.field.ID == "GuiFont" {
					combo, _ = row.control.(*vtui.ComboBox)
				}
			}
			if combo == nil {
				t.Fatal("the Appearance page has no graphical font dropdown")
			}
			if combo.Y1 < c.page.Y1 || combo.Y1 > c.page.Y2 {
				return
			}
			checked++
			text := combo.Edit.GetText()

			mouse(combo.X2, combo.Y1, true, false)
			mouse(combo.X2, combo.Y1, false, false)
			menu := combo.Menu
			if vtui.FrameManager.GetTopFrame() != menu {
				t.Fatalf("releasing the button on the arrow closed the list; the field now reads %q", combo.Edit.GetText())
			}
			if combo.Edit.GetText() != text {
				t.Fatalf("releasing the button on the arrow picked %q", combo.Edit.GetText())
			}
			if menu.Y1 <= combo.Y1 && combo.Y1 <= menu.Y2 {
				t.Fatalf("list rows %d-%d cover the field row %d", menu.Y1, menu.Y2, combo.Y1)
			}
			if menu.Y1 < c.page.Y1 || menu.Y2 > c.page.Y2 || menu.X1 < c.page.X1 || menu.X2 > c.page.X2 {
				t.Fatalf("list %d,%d-%d,%d leaves the content pane %d,%d-%d,%d", menu.X1, menu.Y1, menu.X2, menu.Y2, c.page.X1, c.page.Y1, c.page.X2, c.page.Y2)
			}

			// Pressing on the arrow and dragging onto an item still picks it.
			vtui.FrameManager.RemoveFrame(menu)
			mouse(combo.X2, combo.Y1, true, false)
			vtui.FrameManager.Step(0) // the render that fits the list into the pane
			itemY := menu.Y1 + 2
			index := menu.GetClickIndex(itemY)
			if index < 0 || index >= len(menu.Items) {
				t.Fatalf("row %d of list %d-%d is not an item", itemY, menu.Y1, menu.Y2)
			}
			mouse(menu.X1+2, itemY, true, true)
			mouse(menu.X1+2, itemY, false, false)
			if vtui.FrameManager.GetTopFrame() == menu {
				t.Fatal("releasing the drag on an item left the list open")
			}
			if got, want := combo.Edit.GetText(), menu.Items[index].Text; got != want {
				t.Fatalf("the drag onto %q left the field reading %q", want, got)
			}
		})
	}
	if checked == 0 {
		t.Fatal("no scroll offset showed the graphical font dropdown")
	}
}

func TestSettingsDropdownRowsNeverCoverTheField(t *testing.T) {
	cases := []struct {
		name                       string
		field, height, top, bottom int
		wantY, wantHeight          int
	}{
		{"fits below", 5, 10, 0, 20, 6, 10},
		{"fits above only", 15, 10, 0, 20, 5, 10},
		{"shortened below", 9, 10, 2, 17, 10, 8},
		{"shortened above", 11, 10, 2, 17, 2, 9},
		{"too short on either side", 5, 10, 3, 7, 6, 10},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			y, h := settingsDropdownRows(tc.field, tc.height, tc.top, tc.bottom)
			if y != tc.wantY || h != tc.wantHeight {
				t.Fatalf("got rows %d+%d, want %d+%d", y, h, tc.wantY, tc.wantHeight)
			}
			if y <= tc.field && tc.field <= y+h-1 {
				t.Fatalf("rows %d-%d cover the field row %d", y, y+h-1, tc.field)
			}
		})
	}
}
