//go:build windows

package main

import (
	"errors"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/winshell"
	"github.com/unxed/vtui"
)

func TestWindowsLocationsMenuResolvesThemeAtRenderTime(t *testing.T) {
	indices := []int{
		vtui.ColMenuText,
		vtui.ColMenuSelectedText,
		vtui.ColMenuHighlight,
		vtui.ColMenuSelectedHighlight,
		vtui.ColMenuBox,
		vtui.ColMenuTitle,
	}
	original := make(map[int]uint64, len(indices))
	for _, index := range indices {
		original[index] = vtui.Palette[index]
	}
	defer func() {
		for index, value := range original {
			vtui.Palette[index] = value
		}
	}()

	menu := &windowsLocationsMenu{
		VMenu:    vtui.NewVMenu(" Locations "),
		expanded: make(map[string]bool),
	}
	configureWindowsShellMenu(menu.VMenu)
	rows := make([]windowsLocationRow, 0, 8)
	for index, name := range []string{"Alpha", "Bravo", "Charlie", "Delta", "Echo", "Foxtrot", "Golf", "Hotel"} {
		rows = append(rows, windowsLocationRow{node: winshell.Node{
			URI: winshell.URIFromParsingName(name), ParsingName: name, Name: name, Folder: true,
		}, depth: index % 2})
	}
	menu.rebuild(rows)
	menu.SetPosition(2, 2, 29, 7)
	menu.SetSelectPos(1)

	assertRender := func(normal, selected, box uint64) {
		t.Helper()
		scr := vtui.NewScreenBuf()
		scr.AllocBuf(40, 12)
		menu.Show(scr)
		if got := scr.GetCell(menu.X1+2, menu.Y1+1).Attributes; got != normal {
			t.Fatalf("normal row attr = %#x, want %#x", got, normal)
		}
		if got := scr.GetCell(menu.X1+2, menu.Y1+2).Attributes; got != selected {
			t.Fatalf("selected row attr = %#x, want %#x", got, selected)
		}
		if got := scr.GetCell(menu.X1, menu.Y1).Attributes; got != box {
			t.Fatalf("border attr = %#x, want %#x", got, box)
		}
		if got := scr.GetCell(menu.X2, menu.Y1+1).Attributes; got != box {
			t.Fatalf("scrollbar attr = %#x, want menu box %#x", got, box)
		}
	}

	firstNormal := vtui.SetRGBBoth(0, 0x101112, 0x202122)
	firstSelected := vtui.SetRGBBoth(0, 0x303132, 0x404142)
	firstBox := vtui.SetRGBBoth(0, 0x505152, 0x606162)
	vtui.Palette[vtui.ColMenuText] = firstNormal
	vtui.Palette[vtui.ColMenuSelectedText] = firstSelected
	vtui.Palette[vtui.ColMenuBox] = firstBox
	assertRender(firstNormal, firstSelected, firstBox)

	secondNormal := vtui.SetRGBBoth(0, 0x717273, 0x818283)
	secondSelected := vtui.SetRGBBoth(0, 0x919293, 0xa1a2a3)
	secondBox := vtui.SetRGBBoth(0, 0xb1b2b3, 0xc1c2c3)
	vtui.Palette[vtui.ColMenuText] = secondNormal
	vtui.Palette[vtui.ColMenuSelectedText] = secondSelected
	vtui.Palette[vtui.ColMenuBox] = secondBox
	assertRender(secondNormal, secondSelected, secondBox)
}

func TestShellContextMenuUsesSelectedHighlightTheme(t *testing.T) {
	originalText := vtui.Palette[vtui.ColMenuSelectedText]
	originalHighlight := vtui.Palette[vtui.ColMenuSelectedHighlight]
	defer func() {
		vtui.Palette[vtui.ColMenuSelectedText] = originalText
		vtui.Palette[vtui.ColMenuSelectedHighlight] = originalHighlight
	}()

	menu := &shellContextMenu{VMenu: vtui.NewVMenu("")}
	configureWindowsShellMenu(menu.VMenu)
	menu.AddItem(vtui.MenuItem{Text: "&Open"})
	menu.rows = []shellContextRow{{command: winshell.ContextCommand{ID: 1, Text: "&Open", Enabled: true}}}
	menu.SetPosition(1, 1, 20, 4)
	menu.SetSelectPos(0)

	for pass, colors := range [][2]uint64{
		{vtui.SetRGBBoth(0, 0x112233, 0x223344), vtui.SetRGBBoth(0, 0x334455, 0x223344)},
		{vtui.SetRGBBoth(0, 0x556677, 0x667788), vtui.SetRGBBoth(0, 0x778899, 0x667788)},
	} {
		vtui.Palette[vtui.ColMenuSelectedText] = colors[0]
		vtui.Palette[vtui.ColMenuSelectedHighlight] = colors[1]
		scr := vtui.NewScreenBuf()
		scr.AllocBuf(24, 8)
		menu.Show(scr)
		if got := scr.GetCell(menu.X1+2, menu.Y1+1).Attributes; got != colors[1] {
			t.Fatalf("pass %d selected hotkey attr = %#x, want %#x", pass, got, colors[1])
		}
	}
}

func assertWarningDialogResolvesThemeAtRenderTime(t *testing.T, dialog *vtui.Window, wantButtons int) {
	t.Helper()
	indices := []int{
		vtui.ColWarnText,
		vtui.ColWarnBox,
		vtui.ColWarnButton,
		vtui.ColWarnSelectedButton,
		vtui.ColWarnHighlightButton,
		vtui.ColWarnHighlightSelectedButton,
	}
	original := make(map[int]uint64, len(indices))
	for _, index := range indices {
		original[index] = vtui.Palette[index]
	}
	defer func() {
		for index, value := range original {
			vtui.Palette[index] = value
		}
	}()

	defer dialog.SetExitCode(-1)

	var text *vtui.Text
	var buttons []*vtui.Button
	for _, child := range dialog.GetChildren() {
		switch control := child.(type) {
		case *vtui.Text:
			if text == nil {
				text = control
			}
		case *vtui.Button:
			buttons = append(buttons, control)
		}
	}
	if text == nil || len(buttons) != wantButtons {
		t.Fatalf("warning dialog controls: text=%v buttons=%d, want %d", text != nil, len(buttons), wantButtons)
	}
	dialog.SetFocusedItem(buttons[0])

	containsAttr := func(scr *vtui.ScreenBuf, control interface{ GetPosition() (int, int, int, int) }, attr uint64) bool {
		x1, y1, x2, _ := control.GetPosition()
		for x := x1; x <= x2; x++ {
			if scr.GetCell(x, y1).Attributes == attr {
				return true
			}
		}
		return false
	}
	assertRender := func(textAttr, boxAttr, normalButton, selectedButton, normalHighlight, selectedHighlight uint64) {
		t.Helper()
		scr := vtui.NewScreenBuf()
		scr.AllocBuf(100, 30)
		dialog.Show(scr)
		tx, ty, _, _ := text.GetPosition()
		if got := scr.GetCell(tx, ty).Attributes; got != textAttr {
			t.Fatalf("dialog text attr = %#x, want %#x", got, textAttr)
		}
		if got := scr.GetCell(dialog.X1, dialog.Y1).Attributes; got != boxAttr {
			t.Fatalf("dialog border attr = %#x, want %#x", got, boxAttr)
		}
		selected := buttons[0]
		selected.SetFocus(true)
		bx, by, _, _ := selected.GetPosition()
		if got := scr.GetCell(bx, by).Attributes; got != selectedButton {
			t.Fatalf("selected button attr = %#x, want %#x", got, selectedButton)
		}
		if !containsAttr(scr, selected, selectedHighlight) {
			t.Fatalf("selected button does not use highlight attr %#x", selectedHighlight)
		}

		normal := selected
		if len(buttons) > 1 {
			normal = buttons[1]
		} else {
			normal.SetFocus(false)
			scr = vtui.NewScreenBuf()
			scr.AllocBuf(100, 30)
			dialog.Show(scr)
		}
		bx, by, _, _ = normal.GetPosition()
		if got := scr.GetCell(bx, by).Attributes; got != normalButton {
			t.Fatalf("normal button attr = %#x, want %#x", got, normalButton)
		}
		if !containsAttr(scr, normal, normalHighlight) {
			t.Fatalf("normal button does not use highlight attr %#x", normalHighlight)
		}
		selected.SetFocus(true)
	}

	first := []uint64{
		vtui.SetRGBBoth(0, 0x101112, 0x202122),
		vtui.SetRGBBoth(0, 0x303132, 0x404142),
		vtui.SetRGBBoth(0, 0x505152, 0x606162),
		vtui.SetRGBBoth(0, 0x707172, 0x808182),
		vtui.SetRGBBoth(0, 0x909192, 0xa0a1a2),
		vtui.SetRGBBoth(0, 0xb0b1b2, 0xc0c1c2),
	}
	for index, paletteIndex := range indices {
		vtui.Palette[paletteIndex] = first[index]
	}
	assertRender(first[0], first[1], first[2], first[3], first[4], first[5])

	second := append([]uint64(nil), first...)
	for index := range second {
		second[index] = vtui.SetRGBBoth(0, uint32(0x111111+index*0x10101), uint32(0xd0d0d0-index*0x10101))
		vtui.Palette[indices[index]] = second[index]
	}
	if reflect.DeepEqual(first, second) {
		t.Fatal("test palettes unexpectedly match")
	}
	assertRender(second[0], second[1], second[2], second[3], second[4], second[5])
}

func TestGalleryIndexingDialogResolvesThemeAtRenderTime(t *testing.T) {
	screen := vtui.NewScreenBuf()
	screen.AllocBuf(100, 30)
	vtui.FrameManager.Init(screen)
	assertWarningDialogResolvesThemeAtRenderTime(t, showGalleryIndexingRequired(nil), 2)
}

func TestWindowsLocationNavigationErrorPreservesMissingFolderReason(t *testing.T) {
	target := filepath.Join(t.TempDir(), "Idea Creative")
	err := windowsLocationNavigationError(winshell.Node{FileSystemPath: target}, target)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("navigation error = %v, want missing-path error", err)
	}
}

func TestWindowsLocationAccessErrorDialogShowsReasonAndResolvesThemeAtRenderTime(t *testing.T) {
	screen := vtui.NewScreenBuf()
	screen.AllocBuf(100, 30)
	vtui.FrameManager.Init(screen)

	const target = `C:\Users\xs\Downloads\Idea Creative\Idea Creative`
	const reason = "The system cannot find the path specified."
	dialog := showWindowsLocationAccessError(
		winshell.Node{Name: "Idea Creative", FileSystemPath: target},
		target,
		errors.New(reason),
	)
	if got := dialog.GetTitle(); got != " Idea Creative " {
		t.Fatalf("dialog title = %q, want %q", got, " Idea Creative ")
	}
	var lines []string
	for _, child := range dialog.GetChildren() {
		if text, ok := child.(*vtui.Text); ok {
			lines = append(lines, text.GetText())
		}
	}
	message := strings.Join(lines, "\n")
	if !strings.Contains(message, target) || !strings.Contains(message, reason) {
		t.Fatalf("dialog message = %q, want path %q and reason %q", message, target, reason)
	}

	assertWarningDialogResolvesThemeAtRenderTime(t, dialog, 1)
}

func TestGalleryIndexingDialogOpensIndexingOptions(t *testing.T) {
	screen := vtui.NewScreenBuf()
	screen.AllocBuf(100, 30)
	vtui.FrameManager.Init(screen)

	originalRunner := defaultExternalUICommandRunner
	defer func() { defaultExternalUICommandRunner = originalRunner }()
	type invocation struct {
		command string
		args    []string
		dir     string
	}
	called := make(chan invocation, 1)
	defaultExternalUICommandRunner = func(command string, args []string, dir string) error {
		called <- invocation{command: command, args: append([]string(nil), args...), dir: dir}
		return nil
	}

	dialog := showGalleryIndexingRequired(nil)
	dialog.SetExitCode(0)
	select {
	case got := <-called:
		want := invocation{command: "control.exe", args: []string{"/name", "Microsoft.IndexingOptions"}}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("indexing settings invocation = %#v, want %#v", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("indexing settings command was not invoked")
	}
}
