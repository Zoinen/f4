package app

import (
	"fmt"
	"github.com/unxed/f4/internal/panel"
	"strings"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/media"
	"github.com/unxed/vtui"
)

// commandPaletteFrameEntries exposes commands owned by full-screen frames
// that are not represented by the application action.Action registry. Text-entry
// and cursor-only primitives are intentionally left to the frame itself.
func commandPaletteFrameEntries() []commandPaletteEntry {
	if vtui.FrameManager == nil {
		return nil
	}
	switch frame := vtui.FrameManager.GetTopFrame().(type) {
	case *panel.PanelsFrame:
		return commandPalettePanelsContextEntries(frame)
	case commandPaletteHelpFrame:
		return commandPaletteHelpEntries(frame)
	case *media.ImageView:
		return append(commandPaletteImageEntries(frame), commandPaletteImageGalleryOpenEntry(frame)...)
	case *fileops.QueueFrame:
		return append(commandPaletteQueueEntries(frame), commandPaletteQueueZoomEntry(frame)...)
	case *GrabberFrame:
		return commandPaletteGrabberEntries(frame)
	case *ArkanoidFrame:
		return commandPaletteArkanoidEntries(frame)
	case *SheetFrame:
		return commandPaletteSheetEntries(frame)
	default:
		return nil
	}
}

func commandPaletteImageEntries(image *media.ImageView) []commandPaletteEntry {
	if image == nil {
		return nil
	}
	category := i18n.Msg("CommandPalette.CategoryImageViewer")
	type spec struct {
		id, labelKey, english, description, shortcut string
		checked                                      func(*media.ImageView) bool
		run                                          func(*media.ImageView)
	}
	specs := []spec{
		{"Reload", "CommandPalette.Image.Reload", "Reload image", "Reload the current image from disk", "Ctrl+R", nil, func(iv *media.ImageView) { iv.Reload() }},
		{"FullScreen", "CommandPalette.Image.FullScreen", "Toggle full screen", "Show or hide the image viewer chrome", "F, Ctrl+F", func(iv *media.ImageView) bool { return iv.Full }, func(iv *media.ImageView) { iv.SetFullScreen(!iv.Full) }},
		{"Overlay", "CommandPalette.Image.Overlay", "Toggle image information", "Show or hide the image information overlay", "I, Ctrl+I", func(iv *media.ImageView) bool { return iv.Overlay }, func(iv *media.ImageView) { iv.ToggleOverlay() }},
		{"SlideShow", "CommandPalette.Image.SlideShow", "Toggle slide show", "Start or stop automatic image advance", "Ctrl+S", func(iv *media.ImageView) bool { return iv.SlideStop != nil }, func(iv *media.ImageView) { iv.ToggleSlideShow() }},
		{"ZoomIn", "CommandPalette.Image.ZoomIn", "Zoom in", "Increase image zoom", "+", nil, func(iv *media.ImageView) { iv.SetZoom(iv.Zoom * 1.25) }},
		{"ZoomOut", "CommandPalette.Image.ZoomOut", "Zoom out", "Decrease image zoom", "-", nil, func(iv *media.ImageView) { iv.SetZoom(iv.Zoom / 1.25) }},
		{"ActualSize", "CommandPalette.Image.ActualSize", "Toggle actual size", "Switch between fit and actual-size zoom", "Tab", nil, func(iv *media.ImageView) { iv.ToggleActualSize() }},
		{"RotateClockwise", "CommandPalette.Image.RotateClockwise", "Rotate clockwise", "Rotate the image clockwise", ".", nil, func(iv *media.ImageView) { iv.Rotate(90) }},
		{"RotateCounterClockwise", "CommandPalette.Image.RotateCounterClockwise", "Rotate counterclockwise", "Rotate the image counterclockwise", ",", nil, func(iv *media.ImageView) { iv.Rotate(-90) }},
		{"FlipHorizontal", "CommandPalette.Image.FlipHorizontal", "Flip horizontally", "Mirror the image horizontally", "Alt+.", nil, func(iv *media.ImageView) { iv.Flip(true, false) }},
		{"FlipVertical", "CommandPalette.Image.FlipVertical", "Flip vertically", "Mirror the image vertically", "Alt+,", nil, func(iv *media.ImageView) { iv.Flip(false, true) }},
		{"Next", "CommandPalette.Image.Next", "Next image", "Open the next image", "PgDn, Space", nil, func(iv *media.ImageView) { iv.ProcessKey(keymap.ParseFarKey("Space")) }},
		{"Previous", "CommandPalette.Image.Previous", "Previous image", "Open the previous image", "PgUp", nil, func(iv *media.ImageView) { iv.ProcessKey(keymap.ParseFarKey("PgUp")) }},
		{"First", "CommandPalette.Image.First", "First image", "Open the first image", "Home", nil, func(iv *media.ImageView) { iv.ProcessKey(keymap.ParseFarKey("Home")) }},
		{"Last", "CommandPalette.Image.Last", "Last image", "Open the last image", "End", nil, func(iv *media.ImageView) { iv.ProcessKey(keymap.ParseFarKey("End")) }},
		{"Gallery", "CommandPalette.Image.Gallery", "Toggle gallery", "Show or hide the image gallery", "F12", func(iv *media.ImageView) bool { return iv.Gal != nil }, func(iv *media.ImageView) { iv.ToggleGallery() }},
		{"Select", "CommandPalette.Image.Select", "Toggle image selection", "Toggle selection and advance to the next image", "Ins", nil, func(iv *media.ImageView) { iv.ProcessKey(keymap.ParseFarKey("Ins")) }},
		{"ClearSelection", "CommandPalette.Image.ClearSelection", "Clear image selection", "Clear selection and advance to the next image", "Del", nil, func(iv *media.ImageView) { iv.ProcessKey(keymap.ParseFarKey("Del")) }},
		{"Close", "CommandPalette.Image.Close", "Close image viewer", "Close the image viewer", "Esc, F10", nil, func(iv *media.ImageView) { iv.Close() }},
	}
	entries := make([]commandPaletteEntry, 0, len(specs))
	for _, command := range specs {
		command := command
		label := i18n.Msg(command.labelKey)
		if label == "" || strings.HasPrefix(label, "{") {
			label = command.english
		}
		entry := commandPaletteEntry{
			Key:                "image:" + strings.ToLower(command.id),
			Label:              label,
			EnglishLabel:       command.english,
			Description:        label,
			EnglishDescription: command.description,
			ID:                 "Image." + command.id,
			Category:           category,
			Shortcut:           command.shortcut,
			SearchFields:       append(commandPaletteTranslations("CommandPalette.CategoryImageViewer", command.labelKey), "media.ImageView"),
			run: func() bool {
				if vtui.FrameManager == nil || vtui.FrameManager.GetTopFrame() != image {
					return false
				}
				command.run(image)
				return true
			},
		}
		if command.checked != nil {
			entry.Checked = command.checked(image)
		}
		entries = append(entries, entry)
	}
	return entries
}

func commandPaletteQueueEntries(queue *fileops.QueueFrame) []commandPaletteEntry {
	if queue == nil {
		return nil
	}
	category := i18n.Msg("CommandPalette.CategoryQueue")
	newEntry := func(id, labelKey, english, description, shortcut string, run func(*fileops.QueueFrame) bool) commandPaletteEntry {
		label := i18n.Msg(labelKey)
		if label == "" || strings.HasPrefix(label, "{") {
			label = english
		}
		return commandPaletteEntry{
			Key:                "queue:" + strings.ToLower(id),
			Label:              action.PlainLabel(label),
			EnglishLabel:       english,
			Description:        action.PlainLabel(label),
			EnglishDescription: description,
			ID:                 "Queue." + id,
			Category:           category,
			Shortcut:           shortcut,
			SearchFields:       commandPaletteTranslations("CommandPalette.CategoryQueue", labelKey),
			run: func() bool {
				if vtui.FrameManager == nil || vtui.FrameManager.GetTopFrame() != queue {
					return false
				}
				return run(queue)
			},
		}
	}
	return []commandPaletteEntry{
		newEntry("OpenDetails", "CommandPalette.Queue.OpenDetails", "Open task details", "Open details for the selected queue task", "Enter", func(qf *fileops.QueueFrame) bool {
			index := qf.SelectedIndex()
			if index < 0 {
				return false
			}
			qf.OpenTaskDetails(index)
			return true
		}),
		newEntry("Cancel", "Queue.BtnCancel", "Cancel task", "Cancel the selected queue task", "", commandPaletteCancelQueueTask),
		newEntry("Clear", "Queue.BtnClear", "Clear completed tasks", "Remove completed tasks from the queue", "", commandPaletteClearQueueTasks),
		newEntry("Close", "CommandPalette.Queue.Close", "Close queue", "Close the operations queue", "Esc, F10, Ctrl+W", func(qf *fileops.QueueFrame) bool {
			return actionCloseQueueWorkspace(qf)
		}),
	}
}

// actionCloseQueueWorkspace is shared by Queue.Close and the generic
// Workspace.Close action. Active operations keep fileops.QueueFrame's exact veto and
// toast; once the queue is idle the screen is actually removed instead of
// stopping at BaseWindow's (unhandled) Ctrl+W path.
func actionCloseQueueWorkspace(queue *fileops.QueueFrame) bool {
	if queue == nil || vtui.FrameManager == nil || vtui.FrameManager.GetTopFrame() != queue {
		return false
	}
	return ActionWorkspaceClose()
}

func commandPaletteCancelQueueTask(queue *fileops.QueueFrame) bool {
	task := queue.SelectedTask()
	if task == nil {
		return false
	}
	state, id, _ := task.Status()
	if !fileops.QueueTaskCancellable(state) {
		return false
	}
	vtui.ShowMessageOn(queue, " "+i18n.Msg("CommandPalette.Confirm")+" ", fmt.Sprintf(i18n.Msg("CommandPalette.Queue.CancelQuestion"), id), []string{i18n.Msg("CommandPalette.Yes"), i18n.Msg("CommandPalette.No")}).OnResult = func(choice int) {
		if choice == 0 {
			fileops.GlobalQueueManager.Cancel(id)
		}
	}
	return true
}

func commandPaletteClearQueueTasks(*fileops.QueueFrame) bool {
	return fileops.GlobalQueueManager.ClearFinished()
}
