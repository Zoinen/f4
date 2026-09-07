package main

import (
	"strings"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func commandPaletteLocalizedDirectEntry(
	key, id, labelKey, englishLabel, descKey, englishDescription, categoryKey, englishCategory, shortcut string,
	run func() bool,
	aliasKeys ...string,
) commandPaletteEntry {
	label := Msg(labelKey)
	if label == "" || strings.HasPrefix(label, "{") {
		label = englishLabel
	}
	description := Msg(descKey)
	if description == "" || strings.HasPrefix(description, "{") {
		description = englishDescription
	}
	category := Msg(categoryKey)
	if category == "" || strings.HasPrefix(category, "{") {
		category = englishCategory
	}
	translationKeys := append([]string{categoryKey, labelKey, descKey}, aliasKeys...)
	return commandPaletteEntry{
		Key:                key,
		Label:              plainLabel(label),
		EnglishLabel:       englishLabel,
		Description:        plainLabel(description),
		EnglishDescription: englishDescription,
		ID:                 id,
		Category:           plainLabel(category),
		Shortcut:           shortcut,
		SearchFields:       commandPaletteTranslations(translationKeys...),
		run:                run,
	}
}

func commandPaletteGrabberEntries(grabber *GrabberFrame) []commandPaletteEntry {
	if grabber == nil {
		return nil
	}
	newEntry := func(id, labelKey, englishLabel, descKey, englishDescription, shortcut, key string) commandPaletteEntry {
		return commandPaletteLocalizedDirectEntry(
			"grabber:"+strings.ToLower(id), "Grabber."+id,
			labelKey, englishLabel, descKey, englishDescription,
			"CommandPalette.CategoryGrabber", "Screen Grabber", shortcut,
			func() bool {
				if vtui.FrameManager == nil || vtui.FrameManager.GetTopFrame() != grabber {
					return false
				}
				return grabber.ProcessKey(ParseFarKey(key))
			},
			"Action.App.ScreenGrab", "Action.App.ScreenGrab.Desc",
		)
	}
	return []commandPaletteEntry{
		newEntry("CopyAndClose", "CommandPalette.Grabber.CopyAndClose", "Copy selection and close", "CommandPalette.Grabber.CopyAndClose.Desc", "Copy the selected screen text to the clipboard and close the grabber", "Enter, Ctrl+Ins", "Enter"),
		newEntry("Cancel", "CommandPalette.Grabber.Cancel", "Cancel screen grab", "CommandPalette.Grabber.Cancel.Desc", "Close the grabber without changing the clipboard", "Esc, Alt+Ins", "Esc"),
		newEntry("SelectAll", "CommandPalette.Grabber.SelectAll", "Select entire screen", "CommandPalette.Grabber.SelectAll.Desc", "Extend the grabber selection to the entire screen", "A", "A"),
		newEntry("ResetSelectionAnchor", "CommandPalette.Grabber.ResetSelectionAnchor", "Reset selection anchor", "CommandPalette.Grabber.ResetSelectionAnchor.Desc", "Move the selection anchor to the current grabber cursor", "U", "U"),
	}
}

func commandPaletteArkanoidEntries(arkanoid *ArkanoidFrame) []commandPaletteEntry {
	if arkanoid == nil {
		return nil
	}
	arkanoid.mu.Lock()
	autoPlay := arkanoid.autoPlay
	arkanoid.mu.Unlock()

	newEntry := func(id, labelKey, englishLabel, descKey, englishDescription, shortcut string, event *vtinput.InputEvent, valid func() bool) commandPaletteEntry {
		return commandPaletteLocalizedDirectEntry(
			"arkanoid:"+strings.ToLower(id), "Arkanoid."+id,
			labelKey, englishLabel, descKey, englishDescription,
			"CommandPalette.CategoryArkanoid", "Arkanoid", shortcut,
			func() bool {
				if vtui.FrameManager == nil || vtui.FrameManager.GetTopFrame() != arkanoid || arkanoid.IsDone() {
					return false
				}
				if valid != nil && !valid() {
					return false
				}
				return arkanoid.ProcessKey(event)
			},
			"Action.App.Arkanoid", "Action.App.Arkanoid.Desc",
		)
	}
	autoPlayStillCurrent := func() bool {
		arkanoid.mu.Lock()
		defer arkanoid.mu.Unlock()
		return arkanoid.autoPlay == autoPlay
	}
	entryAutoPlay := newEntry(
		"ToggleAutoPlay", "CommandPalette.Arkanoid.ToggleAutoPlay", "Toggle auto-play",
		"CommandPalette.Arkanoid.ToggleAutoPlay.Desc", "Let the Arkanoid paddle play automatically",
		"Ctrl+Alt+A", ParseFarKey("CtrlAltA"), autoPlayStillCurrent,
	)
	entryAutoPlay.Checked = autoPlay
	return []commandPaletteEntry{
		entryAutoPlay,
		newEntry("HighScores", "CommandPalette.Arkanoid.HighScores", "Show high scores", "CommandPalette.Arkanoid.HighScores.Desc", "Open the Arkanoid high-score table", "Ctrl+H", ParseFarKey("CtrlH"), nil),
		newEntry("SpeedUp", "CommandPalette.Arkanoid.SpeedUp", "Increase game speed", "CommandPalette.Arkanoid.SpeedUp.Desc", "Increase the Arkanoid game-loop speed", "+, =", &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: '+'}, nil),
		newEntry("SpeedDown", "CommandPalette.Arkanoid.SpeedDown", "Decrease game speed", "CommandPalette.Arkanoid.SpeedDown.Desc", "Decrease the Arkanoid game-loop speed", "-, _", &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: '-'}, nil),
		newEntry("Close", "CommandPalette.Arkanoid.Close", "Close Arkanoid", "CommandPalette.Arkanoid.Close.Desc", "Close the Arkanoid game", "Esc", ParseFarKey("Esc"), nil),
	}
}

func commandPaletteImageGalleryOpenEntry(image *ImageView) []commandPaletteEntry {
	if image == nil || image.gal == nil {
		return nil
	}
	gallery := image.gal
	cursor := gallery.cursor
	path := image.galleryPath()
	if path == "" {
		return nil
	}
	return []commandPaletteEntry{commandPaletteLocalizedDirectEntry(
		"image:gallery.open", "Image.Gallery.Open",
		"CommandPalette.Image.Gallery.Open", "Open selected gallery image",
		"CommandPalette.Image.Gallery.Open.Desc", "Open the image under the gallery cursor and leave the gallery",
		"CommandPalette.CategoryImageViewer", "Image Viewer", "Enter",
		func() bool {
			if vtui.FrameManager == nil || vtui.FrameManager.GetTopFrame() != image ||
				image.gal != gallery || gallery.cursor != cursor || image.galleryPath() != path {
				return false
			}
			return image.ProcessKey(ParseFarKey("Enter"))
		},
		"CommandPalette.Image.Gallery",
	)}
}

func commandPaletteQueueZoomEntry(queue *QueueFrame) []commandPaletteEntry {
	if queue == nil || !queue.ShowZoom {
		return nil
	}
	zoomed := queue.SavedBounds != nil
	entry := commandPaletteLocalizedDirectEntry(
		"queue:togglezoom", "Queue.ToggleZoom",
		"CommandPalette.Queue.ToggleZoom", "Toggle queue zoom",
		"CommandPalette.Queue.ToggleZoom.Desc", "Maximize the operations queue or restore its previous bounds",
		"CommandPalette.CategoryQueue", "Operations Queue", "F5",
		func() bool {
			if vtui.FrameManager == nil || vtui.FrameManager.GetTopFrame() != queue ||
				queue.IsDone() || !queue.ShowZoom || (queue.SavedBounds != nil) != zoomed {
				return false
			}
			return queue.ProcessKey(ParseFarKey("F5"))
		},
	)
	entry.Checked = zoomed
	return []commandPaletteEntry{entry}
}
