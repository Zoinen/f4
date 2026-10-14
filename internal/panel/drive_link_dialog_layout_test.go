package panel

import (
	"testing"

	"github.com/unxed/vtui"
)

// f4#1148 (mitek1977-coder): the Ins/F4 dialog of the drive menu's Links has
// Name: right under the top border, one blank row between the path and the
// hot key and none elsewhere, and its buttons right under a rule and right
// above the bottom border (12.10 list, point 7).
func TestDriveLinkDialogRows(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	d := NewDriveBookmarkEditDialog(DriveBookmark{}, "/tmp", nil)
	var sep *vtui.Separator
	var buttons []*vtui.Button
	for _, child := range d.GetChildren() {
		switch c := child.(type) {
		case *vtui.Separator:
			sep = c
		case *vtui.Button:
			buttons = append(buttons, c)
		}
	}
	if sep == nil || len(buttons) != 2 {
		t.Fatalf("separator %v, %d buttons", sep != nil, len(buttons))
	}
	_, nameY, _, _ := d.nameEdit.GetPosition()
	_, pathY, _, _ := d.pathEdit.GetPosition()
	_, hotkeyY, _, _ := d.HotkeyEdit.GetPosition()
	_, sepY, _, _ := sep.GetPosition()
	_, buttonY, _, _ := buttons[0].GetPosition()
	if pathY != nameY+2 {
		t.Errorf("Path field at row %d, Name field at %d: want only the Path label between them", pathY, nameY)
	}
	if nameY != d.Y1+2 {
		t.Errorf("Name field at row %d, top border at %d: want only the Name label between them", nameY, d.Y1)
	}
	if hotkeyY != pathY+2 {
		t.Errorf("Hot key at row %d, Path field at %d: want one blank row between", hotkeyY, pathY)
	}
	if sepY != hotkeyY+2 {
		t.Errorf("rule at row %d, Hot key at %d: want only the hint between them", sepY, hotkeyY)
	}
	if buttonY != sepY+1 {
		t.Errorf("buttons at row %d, rule at %d: want them right under it", buttonY, sepY)
	}
	if buttonY != d.Y2-1 {
		t.Errorf("buttons at row %d, bottom border at %d: want them right above it", buttonY, d.Y2)
	}
}
