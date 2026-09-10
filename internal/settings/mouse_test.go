package settings

import (
	"fmt"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestSettingsMouseCaptureAcrossPanes(t *testing.T) {
	for _, releaseButtons := range []uint32{0, vtinput.FromLeft1stButtonPressed} {
		values := map[string]string{}
		var fields []f4settings.Field
		for i := 0; i < 40; i++ {
			id := fmt.Sprint(i)
			values[id] = "false"
			fields = append(fields, f4settings.Scalar(id, "panels", "Options", "Option "+id, "Explanation", f4settings.Boolean))
		}
		d := f4settings.NewDraft(values, nil)
		c := newSettingsCenter([]*settingsSession{{catalog: f4settings.Catalog{ID: "mouse", Categories: Categories, Fields: fields}, draft: d}})
		c.selectCategory("panels")
		c.SetPosition(0, 0, 149, 39)
		scr := vtui.NewSilentScreenBuf()
		scr.AllocBuf(150, 40)
		c.Show(scr)
		send := func(x, y int, down bool, buttons uint32, moved bool) {
			e := &vtinput.InputEvent{Type: vtinput.MouseEventType, MouseX: testutil.Int16(x), MouseY: testutil.Int16(y), KeyDown: down, ButtonState: buttons}
			if moved {
				e.MouseEventFlags = vtinput.MouseMoved
			}
			c.ProcessMouse(e)
		}
		for _, bar := range []*vtui.ScrollBar{c.page.bar, c.help.bar} {
			c.help.text = strings.Repeat("A long explanation to scroll. ", 400)
			c.Show(scr)
			send(bar.X1, bar.Y1+1, true, 1, false)
			if !bar.IsMouseCaptured() {
				t.Fatal("scrollbar did not capture initial press")
			}
			send(c.page.X1+5, bar.Y1+5, true, 1, true)
			send(c.sidebar.X1, bar.Y1+8, true, 1, true)
			send(c.X2, c.Y2, true, 1, true)
			if c.resizing {
				t.Fatal("scrollbar drag was stolen by the resize corner")
			}
			if !bar.IsMouseCaptured() || len(d.Changed()) != 0 {
				t.Fatal("scrollbar drag leaked into other controls")
			}
			send(c.X2+10, c.Y2+10, false, releaseButtons, false)
			if bar.IsMouseCaptured() {
				t.Fatal("release outside settings did not clear scrollbar capture")
			}
		}
		c.page.scroll = 0
		c.page.positionRows()
		c.Show(scr)
		var row *settingsRow
		for _, candidate := range c.page.rows {
			if candidate.control != nil {
				row = candidate
				break
			}
		}
		x, y, _, _ := row.control.GetPosition()
		send(x, y, true, 1, false)
		send(x+2, y, true, 1, true)
		send(x+4, y+1, true, 1, true)
		send(x+3, y, true, 1, true)
		send(x+3, y, false, releaseButtons, false)
		if d.Values[row.field.ID] != "true" || len(d.Changed()) != 1 {
			t.Fatal("checkbox drag toggled repeatedly or changed adjacent options")
		}
		send(x, y, true, 1, false)
		send(x, y, false, releaseButtons, false)
		if len(d.Changed()) != 0 {
			t.Fatal("next physical click was blocked by stale capture")
		}
		d.Close()
	}
}
