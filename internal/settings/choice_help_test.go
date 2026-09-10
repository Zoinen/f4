package settings

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
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
