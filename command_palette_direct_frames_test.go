package main

import (
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type directPaletteOtherFrame struct{ vtui.BaseFrame }

func (*directPaletteOtherFrame) GetType() vtui.FrameType { return vtui.TypeUser + 19 }

func setDirectPaletteTopFrame(t *testing.T, frame vtui.Frame) {
	t.Helper()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 30)
	vtui.FrameManager.Init(scr)
	vtui.FrameManager.Push(frame)
	t.Cleanup(func() { vtui.FrameManager.Init(vtui.NewSilentScreenBuf()) })
}

func requireDirectPaletteIDs(t *testing.T, entries []commandPaletteEntry, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if !commandPaletteTestHasID(entries, id) {
			t.Errorf("command palette entry %s is missing", id)
		}
	}
}

func TestCommandPaletteGrabberProviderAndScreenGrabToggle(t *testing.T) {
	scr := setupGrabberScreen(t)
	grabber := NewGrabberFrame()
	grabber.Show(scr)
	vtui.FrameManager.Push(grabber)

	entries := commandPaletteFrameEntries()
	requireDirectPaletteIDs(t, entries,
		"Grabber.CopyAndClose", "Grabber.Cancel", "Grabber.SelectAll", "Grabber.ResetSelectionAnchor",
	)
	selectAll, _ := commandPaletteTestEntryByID(entries, "Grabber.SelectAll")
	if !executeCommandPaletteEntry(selectAll) {
		t.Fatal("Grabber.SelectAll was not handled")
	}
	if grabber.anchorX != 0 || grabber.anchorY != 0 || grabber.curX != testGrabberW-1 || grabber.curY != testGrabberH-1 {
		t.Fatalf("Grabber.SelectAll selection = (%d,%d)-(%d,%d)", grabber.anchorX, grabber.anchorY, grabber.curX, grabber.curY)
	}

	altIns := &vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vtinput.VK_INSERT,
		ControlKeyState: vtinput.LeftAltPressed,
	}
	if !grabber.VetoActionKey(altIns) {
		t.Fatal("open grabber did not reserve physical Alt+Ins")
	}
	if !RunAction("App.ScreenGrab") || !grabber.IsDone() {
		t.Fatal("App.ScreenGrab did not toggle the existing grabber closed")
	}
	if got := len(vtui.FrameManager.Screens[0].Frames); got != 1 {
		t.Fatalf("App.ScreenGrab stacked another grabber: frame count %d", got)
	}
}

func TestCommandPaletteGrabberEntryRejectsStaleTopFrame(t *testing.T) {
	setupGrabberScreen(t)
	grabber := NewGrabberFrame()
	setDirectPaletteTopFrame(t, grabber)
	entry, found := commandPaletteTestEntryByID(commandPaletteGrabberEntries(grabber), "Grabber.SelectAll")
	if !found {
		t.Fatal("Grabber.SelectAll is missing")
	}
	vtui.FrameManager.Push(&directPaletteOtherFrame{})
	if executeCommandPaletteEntry(entry) {
		t.Fatal("stale Grabber entry ran after another frame became top")
	}
}

func TestCommandPaletteArkanoidProviderRoutesCommandsAndGuardsState(t *testing.T) {
	oldLanguage := AppConfig.Language
	oldFallback := AppConfig.FallbackLanguage
	AppConfig.Language = "ru"
	AppConfig.FallbackLanguage = ""
	t.Cleanup(func() {
		AppConfig.Language = oldLanguage
		AppConfig.FallbackLanguage = oldFallback
	})

	arkanoid := &ArkanoidFrame{BaseWindow: *vtui.NewBaseWindow(1, 1, 63, 20, " Arkanoid ")}
	setDirectPaletteTopFrame(t, arkanoid)
	entries := commandPaletteFrameEntries()
	requireDirectPaletteIDs(t, entries,
		"Arkanoid.ToggleAutoPlay", "Arkanoid.HighScores", "Arkanoid.SpeedUp", "Arkanoid.SpeedDown", "Arkanoid.Close",
	)
	autoPlay, _ := commandPaletteTestEntryByID(entries, "Arkanoid.ToggleAutoPlay")
	if autoPlay.Label != Msg("CommandPalette.Arkanoid.ToggleAutoPlay") || autoPlay.Description != Msg("CommandPalette.Arkanoid.ToggleAutoPlay.Desc") {
		t.Fatalf("Arkanoid command did not use current UI language: %#v", autoPlay)
	}
	if !executeCommandPaletteEntry(autoPlay) || !arkanoid.autoPlay {
		t.Fatal("Arkanoid.ToggleAutoPlay did not route through ArkanoidFrame")
	}
	speedUp, _ := commandPaletteTestEntryByID(commandPaletteArkanoidEntries(arkanoid), "Arkanoid.SpeedUp")
	if !executeCommandPaletteEntry(speedUp) || arkanoid.autoSpeed != 1 {
		t.Fatalf("Arkanoid.SpeedUp result = %d, want 1", arkanoid.autoSpeed)
	}

	staleAutoPlay := autoPlay
	if executeCommandPaletteEntry(staleAutoPlay) {
		t.Fatal("stale Arkanoid auto-play snapshot toggled newer state")
	}
	vtui.FrameManager.Push(&directPaletteOtherFrame{})
	if executeCommandPaletteEntry(speedUp) || arkanoid.autoSpeed != 1 {
		t.Fatal("Arkanoid command ran after the game stopped being top")
	}
}

func TestCommandPaletteImageGalleryOpenIsTargetSpecific(t *testing.T) {
	gallery := &imageGallery{cursor: 1}
	image := &ImageView{
		path:     "second.png",
		siblings: []string{"first.png", "second.png"},
		index:    1,
		gal:      gallery,
	}
	setDirectPaletteTopFrame(t, image)
	entry, found := commandPaletteTestEntryByID(commandPaletteFrameEntries(), "Image.Gallery.Open")
	if !found {
		t.Fatal("Image.Gallery.Open is missing while the gallery is active")
	}

	gallery.cursor = 0
	if executeCommandPaletteEntry(entry) || image.gal == nil {
		t.Fatal("stale gallery command opened a different cursor target")
	}
	gallery.cursor = 1
	entry, _ = commandPaletteTestEntryByID(commandPaletteFrameEntries(), "Image.Gallery.Open")
	if !executeCommandPaletteEntry(entry) || image.gal != nil {
		t.Fatal("Image.Gallery.Open did not open the captured gallery target")
	}
	if commandPaletteTestHasID(commandPaletteFrameEntries(), "Image.Gallery.Open") {
		t.Fatal("Image.Gallery.Open remained visible after leaving the gallery")
	}
}

func TestCommandPaletteQueueToggleZoomTracksWindowState(t *testing.T) {
	setDirectPaletteTopFrame(t, &directPaletteOtherFrame{})
	queue := NewQueueFrame()
	vtui.FrameManager.Push(queue)

	entry, found := commandPaletteTestEntryByID(commandPaletteFrameEntries(), "Queue.ToggleZoom")
	if !found || entry.Checked {
		t.Fatalf("initial Queue.ToggleZoom entry = %#v", entry)
	}
	if !executeCommandPaletteEntry(entry) || queue.SavedBounds == nil {
		t.Fatal("Queue.ToggleZoom did not maximize the queue")
	}
	if executeCommandPaletteEntry(entry) {
		t.Fatal("stale Queue.ToggleZoom snapshot restored a newer window state")
	}

	restore, found := commandPaletteTestEntryByID(commandPaletteFrameEntries(), "Queue.ToggleZoom")
	if !found || !restore.Checked {
		t.Fatalf("maximized Queue.ToggleZoom entry = %#v", restore)
	}
	if !executeCommandPaletteEntry(restore) || queue.SavedBounds != nil {
		t.Fatal("Queue.ToggleZoom did not restore the queue bounds")
	}
}
