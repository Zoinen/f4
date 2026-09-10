package dialog

import (
	"github.com/unxed/vtui"
	"testing"
)

func TestFileDialogWidth(t *testing.T) {
	cases := []struct{ screen, want int }{
		{0, 50},
		{40, 40},
		{50, 40},
		{60, 40},
		{80, 40},
		{120, 60},
		{200, 100},
	}
	for _, c := range cases {
		if got := FileDialogWidth(c.screen); got != c.want {
			t.Errorf("FileDialogWidth(%d) = %d, want %d", c.screen, got, c.want)
		}
	}
}

func firstDialogEdit(dlg vtui.Container) *vtui.Edit {
	for _, item := range dlg.GetChildren() {
		if edit, ok := item.(*vtui.Edit); ok {
			return edit
		}
	}
	return nil
}

func assertFileDialogGeometry(t *testing.T, dlg *FileDialog, edit *vtui.Edit, screenWidth int) {
	t.Helper()

	want := FileDialogWidth(screenWidth)
	width, _ := dlg.Size()
	if width != want {
		t.Fatalf("dialog width on a %d column screen = %d, want %d", screenWidth, width, want)
	}
	if dlg.X1 != (screenWidth-want)/2 {
		t.Fatalf("dialog starts at column %d on a %d column screen, want %d", dlg.X1, screenWidth, (screenWidth-want)/2)
	}
	if dlg.X1 < 0 || dlg.X2 >= screenWidth {
		t.Fatalf("dialog columns %d..%d fall outside a %d column screen", dlg.X1, dlg.X2, screenWidth)
	}
	if edit.X1 != dlg.X1+2 || edit.X2 != dlg.X2-2 {
		t.Fatalf("text field columns %d..%d, want it stretched over %d..%d", edit.X1, edit.X2, dlg.X1+2, dlg.X2-2)
	}
}
