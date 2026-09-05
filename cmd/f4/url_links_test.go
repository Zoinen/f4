package main

import (
	"context"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestFindURLLinks_TrimsPunctuationAndRejectsOtherSchemes(t *testing.T) {
	links := findURLLinks(`see https://example.com/path?q=1. www.example.org, ftp://example.org`)
	if len(links) != 2 {
		t.Fatalf("got %d links, want 2: %#v", len(links), links)
	}
	if links[0].URL != "https://example.com/path?q=1" {
		t.Errorf("first URL = %q", links[0].URL)
	}
	if links[1].URL != "https://www.example.org" {
		t.Errorf("www URL = %q", links[1].URL)
	}
}

func TestOpenExternalURL_OnlyAllowsWebURLs(t *testing.T) {
	old := launchExternalURL
	defer func() { launchExternalURL = old }()
	var got string
	launchExternalURL = func(raw string) error {
		got = raw
		return nil
	}
	if err := openExternalURL("https://example.org/a"); err != nil {
		t.Fatalf("openExternalURL returned error: %v", err)
	}
	if got != "https://example.org/a" {
		t.Fatalf("launcher received %q", got)
	}
	if err := openExternalURL("file:///etc/passwd"); err == nil {
		t.Fatal("file URL was accepted")
	}
}

func TestCtrlMouseClickRequiresLeftButtonAndCtrl(t *testing.T) {
	base := &vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true}
	if ctrlMouseClick(base) {
		t.Fatal("plain mouse event was treated as Ctrl+click")
	}
	e := *base
	e.ButtonState = vtinput.FromLeft1stButtonPressed
	e.ControlKeyState = vtinput.LeftCtrlPressed
	if !ctrlMouseClick(&e) {
		t.Fatal("Ctrl+left click was not recognized")
	}
	e.MouseEventFlags = vtinput.MouseMoved
	if ctrlMouseClick(&e) {
		t.Fatal("drag event was treated as a click")
	}
}

func TestEditorURLHoverAddsUnderlineOnlyToHoveredLink(t *testing.T) {
	const text = "https://example.org"
	links := findURLLinks(text)
	ev := &EditorView{hoverURL: links[0].URL, TabSize: 8}
	cells := ev.fillCellsWithLinks(nil, []byte(text), DefaultTermAttr, DefaultTermAttr, 0, false, 0, 0, nil, links, 0, false, -1, 0, 0, 0)
	if len(cells) != len(text) {
		t.Fatalf("rendered %d cells, want %d", len(cells), len(text))
	}
	for i, cell := range cells {
		if cell.Attributes&vtui.CommonLvbUnderscore == 0 {
			t.Errorf("cell %d was not underlined", i)
		}
	}
	ev.hoverURL = "https://other.example"
	cells = ev.fillCellsWithLinks(nil, []byte(text), DefaultTermAttr, DefaultTermAttr, 0, false, 0, 0, nil, links, 0, false, -1, 0, 0, 0)
	for i, cell := range cells {
		if cell.Attributes&vtui.CommonLvbUnderscore != 0 {
			t.Errorf("cell %d was underlined for a different URL", i)
		}
	}
}

func TestTerminalURLHoverUnderlinesVisibleLink(t *testing.T) {
	tv := NewTerminalView(40, 3)
	defer tv.Close()
	tv.SetPosition(0, 0, 39, 2)
	tv.SetVisible(true)
	for i, r := range "https://example.org" {
		tv.Lines[0][i] = vtui.CharInfo{Char: testUint64Rune(r), Attributes: DefaultTermAttr}
	}
	if !tv.UpdateURLHover(4, 0) {
		t.Fatal("hover state did not change")
	}
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(40, 3)
	SetDefaultF4Palette()
	tv.Show(scr)
	if scr.GetCell(4, 0).Attributes&vtui.CommonLvbUnderscore == 0 {
		t.Fatal("hovered terminal URL was not underlined")
	}
	if tv.UpdateURLHover(30, 0) != true {
		t.Fatal("moving off the URL did not clear hover state")
	}
}

func TestViewerURLHoverMapsScreenCellToLink(t *testing.T) {
	data := []byte("https://example.org\n")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	backend := &ViewerBackend{
		file:      &vfs.MemoryReadAtCloser{Data: data},
		size:      int64(len(data)),
		cacheData: data,
		ctx:       ctx,
		cancelCtx: cancel,
	}
	vv := &ViewerView{backend: backend}
	vv.SetPosition(0, 0, 39, 2)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(40, 3)
	vv.renderText(scr, 40, 2)

	link, ok := vv.urlLinkAtMouse(4, 1)
	if !ok || link.URL != "https://example.org" {
		t.Fatalf("screen cell did not resolve URL: %#v, %v", link, ok)
	}
	if !vv.updateURLHover(4, 1) {
		t.Fatal("viewer hover did not change")
	}
	vv.renderText(scr, 40, 2)
	if scr.GetCell(4, 1).Attributes&vtui.CommonLvbUnderscore == 0 {
		t.Fatal("hovered viewer URL was not underlined")
	}
}
