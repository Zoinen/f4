package panel

import (
	"bytes"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFar3MenuImportPreservesTree(t *testing.T) {
	text := "F1: Tools\r\n{\r\n:: Colon key\r\n     echo !.!\r\n     echo !#\r\n--: separator\r\ne: Empty\r\n{\r\n}\r\n}\r\n}: Brace key\r\n     echo done\r\n"
	for _, data := range [][]byte{[]byte(text), append([]byte{0xef, 0xbb, 0xbf}, []byte(text)...), EncodeUTF16LEWithBOM(text)} {
		incoming, err := ParseFarMenu(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if len(incoming) != 2 || incoming[1].HotKey != "}" || incoming[0].Submenu[0].HotKey != ":" {
			t.Fatalf("wrong tree: %#v", incoming)
		}
		if !reflect.DeepEqual(incoming[0].Submenu[0].Commands, []string{"echo !.!", "echo !#"}) {
			t.Fatal("commands changed")
		}
		if incoming[0].Submenu[2].Submenu == nil {
			t.Fatal("empty submenu lost")
		}
		existing := []UserMenuItem{{HotKey: "F1", Label: "Keep", Commands: []string{"echo old"}}}
		merged, added := MergeUserMenus(existing, incoming)
		if added != 2 || !reflect.DeepEqual(merged[0], existing[0]) {
			t.Fatal("existing item replaced")
		}
		twice, added := MergeUserMenus(merged, incoming)
		if added != 0 || !reflect.DeepEqual(merged, twice) {
			t.Fatal("repeat import duplicated entries")
		}
		path := t.TempDir() + "/main_menu.ini"
		if err := SaveMainMenu(path, merged); err != nil {
			t.Fatal(err)
		}
		restored, err := LoadMainMenu(path)
		if err != nil || !reflect.DeepEqual(restored, merged) {
			t.Fatalf("round trip: %#v, %v", restored, err)
		}
	}
}

func TestFar3MenuImportDialogStagesAndResizes(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	path := filepath.Join(t.TempDir(), "FarMenu.ini")
	if err := os.WriteFile(path, []byte("a: Imported\n    echo !.!\n"), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	dlg := ShowFar3UserMenuImport(func(items []UserMenuItem) error {
		calls++
		if len(items) != 1 || items[0].Label != "Imported" {
			t.Fatalf("imported %#v", items)
		}
		return nil
	})
	defer dlg.Close()
	var source *vtui.Edit
	var button *vtui.Button
	for _, item := range dlg.GetChildren() {
		if edit, ok := item.(*vtui.Edit); ok {
			source = edit
		}
		if b, ok := item.(*vtui.Button); ok && b.GetId() == "far3-usermenu-import" {
			button = b
		}
	}
	if source == nil || button == nil {
		t.Fatal("missing import controls")
	}
	for _, width := range []int{80, 160, 80} {
		dlg.ResizeConsole(width, 30)
		for _, item := range dlg.GetChildren() {
			x1, y1, x2, y2 := item.GetPosition()
			if x1 <= dlg.X1 || x2 >= dlg.X2 || y1 <= dlg.Y1 || y2 >= dlg.Y2 {
				t.Fatalf("control outside dialog: %T %d,%d,%d,%d", item, x1, y1, x2, y2)
			}
		}
	}
	if calls != 0 {
		t.Fatal("import ran without an explicit click")
	}
	source.SetText(path)
	button.OnClick()
	if calls != 1 {
		t.Fatal("import callback not called")
	}
}

func TestFar3ImportKeepsAndRestoresRepeatedSeparators(t *testing.T) {
	a := UserMenuItem{HotKey: "a", Label: "A", Commands: []string{"echo a"}}
	b := UserMenuItem{HotKey: "b", Label: "B", Commands: []string{"echo b"}}
	c := UserMenuItem{HotKey: "c", Label: "C", Commands: []string{"echo c"}}
	sep := UserMenuItem{HotKey: "--"}
	source := []UserMenuItem{a, sep, b, sep, c, sep}
	imported, _ := MergeUserMenus(nil, source)
	if !reflect.DeepEqual(imported, source) {
		t.Fatalf("separators lost: %#v", imported)
	}
	again, added := MergeUserMenus(imported, source)
	if added != 0 || !reflect.DeepEqual(again, source) {
		t.Fatal("reimport changed menu")
	}
	broken := []UserMenuItem{a, sep, b, c}
	repaired, added := MergeUserMenus(broken, source)
	if added != 2 || !reflect.DeepEqual(repaired, source) {
		t.Fatalf("repair misplaced separators: %#v", repaired)
	}
}

func TestFar3MenuLeadingSeparatorsAreIdempotent(t *testing.T) {
	source := []UserMenuItem{{HotKey: "--"}, {HotKey: "--"}, {HotKey: "x", Label: "Command", Commands: []string{"echo x"}}}
	first, _ := MergeUserMenus(nil, source)
	second, count := MergeUserMenus(first, source)
	if count != 0 || !reflect.DeepEqual(first, second) {
		t.Fatalf("leading separators duplicated: %#v", second)
	}
}

func TestFar3ExternalMenuImport(t *testing.T) {
	path := os.Getenv("F4_FAR3_USERMENU_FIXTURE")
	if path == "" {
		t.Skip("optional external Far menu fixture")
	}
	source, err := LoadFarMenuFile(path)
	if err != nil {
		t.Fatal(err)
	}
	imported, _ := MergeUserMenus(nil, source)
	if !reflect.DeepEqual(imported, source) {
		t.Fatal("import changed source tree")
	}
	repeated, added := MergeUserMenus(imported, source)
	if added != 0 || !reflect.DeepEqual(repeated, source) {
		t.Fatal("repeated import changed source tree")
	}
	count := 0
	var walk func([]UserMenuItem)
	walk = func(items []UserMenuItem) {
		for _, item := range items {
			if item.IsSeparator() {
				count++
			}
			walk(item.Submenu)
		}
	}
	walk(imported)
	t.Logf("Preserved %d separators, including nested entries", count)
}
