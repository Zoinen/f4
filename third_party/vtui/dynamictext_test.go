package vtui

import (
	"runtime"
	"testing"
)

func TestDynamicText_Update(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Skipping on Windows due to different terminal reset behavior")
	}
	SetDefaultPalette()
	counter := 0
	dt := NewDynamicText(0, 0, 10, 0, func() string {
		counter++
		return "val"
	})

	scr := NewSilentScreenBuf()
	scr.AllocBuf(10, 1)

	// Every Show() call should trigger the callback
	dt.Show(scr)
	if counter != 1 {
		t.Errorf("Callback not called on first Show, count: %d", counter)
	}
	if dt.GetText() != "val" {
		t.Error("Content not updated from callback")
	}

	// Verify that text is properly rendered to screen cells
	checkCell(t, scr, 0, 0, 'v', Palette[ColDialogText])
	checkCell(t, scr, 1, 0, 'a', Palette[ColDialogText])
	checkCell(t, scr, 2, 0, 'l', Palette[ColDialogText])

	dt.Show(scr)
	if counter != 2 {
		t.Errorf("Callback not called on second Show, count: %d", counter)
	}
}
