package main

import (
	"fmt"
	"strings"

	"github.com/unxed/vtui"
)

// commandPaletteFrameEntries exposes commands owned by full-screen frames
// that are not represented by the application Action registry. Text-entry
// and cursor-only primitives are intentionally left to the frame itself.
func commandPaletteFrameEntries() []commandPaletteEntry {
	if vtui.FrameManager == nil {
		return nil
	}
	switch frame := vtui.FrameManager.GetTopFrame().(type) {
	case *PanelsFrame:
		return commandPalettePanelsContextEntries(frame)
	case commandPaletteHelpFrame:
		return commandPaletteHelpEntries(frame)
	case *ImageView:
		return append(commandPaletteImageEntries(frame), commandPaletteImageGalleryOpenEntry(frame)...)
	case *QueueFrame:
		return append(commandPaletteQueueEntries(frame), commandPaletteQueueZoomEntry(frame)...)
	case *GrabberFrame:
		return commandPaletteGrabberEntries(frame)
	case *ArkanoidFrame:
		return commandPaletteArkanoidEntries(frame)
	default:
		return nil
	}
}

func commandPaletteImageEntries(image *ImageView) []commandPaletteEntry {
	if image == nil {
		return nil
	}
	category := Msg("CommandPalette.CategoryImageViewer")
	type spec struct {
		id, labelKey, english, description, shortcut string
		checked                                      func(*ImageView) bool
		run                                          func(*ImageView)
	}
	specs := []spec{
		{"Reload", "CommandPalette.Image.Reload", "Reload image", "Reload the current image from disk", "Ctrl+R", nil, func(iv *ImageView) { iv.Reload() }},
		{"FullScreen", "CommandPalette.Image.FullScreen", "Toggle full screen", "Show or hide the image viewer chrome", "F, Ctrl+F", func(iv *ImageView) bool { return iv.full }, func(iv *ImageView) { iv.SetFullScreen(!iv.full) }},
		{"Overlay", "CommandPalette.Image.Overlay", "Toggle image information", "Show or hide the image information overlay", "I, Ctrl+I", func(iv *ImageView) bool { return iv.overlay }, func(iv *ImageView) { iv.ToggleOverlay() }},
		{"SlideShow", "CommandPalette.Image.SlideShow", "Toggle slide show", "Start or stop automatic image advance", "Ctrl+S", func(iv *ImageView) bool { return iv.slideStop != nil }, func(iv *ImageView) { iv.ToggleSlideShow() }},
		{"ZoomIn", "CommandPalette.Image.ZoomIn", "Zoom in", "Increase image zoom", "+", nil, func(iv *ImageView) { iv.SetZoom(iv.zoom * 1.25) }},
		{"ZoomOut", "CommandPalette.Image.ZoomOut", "Zoom out", "Decrease image zoom", "-", nil, func(iv *ImageView) { iv.SetZoom(iv.zoom / 1.25) }},
		{"ActualSize", "CommandPalette.Image.ActualSize", "Toggle actual size", "Switch between fit and actual-size zoom", "Tab", nil, func(iv *ImageView) { iv.ToggleActualSize() }},
		{"RotateClockwise", "CommandPalette.Image.RotateClockwise", "Rotate clockwise", "Rotate the image clockwise", ".", nil, func(iv *ImageView) { iv.Rotate(90) }},
		{"RotateCounterClockwise", "CommandPalette.Image.RotateCounterClockwise", "Rotate counterclockwise", "Rotate the image counterclockwise", ",", nil, func(iv *ImageView) { iv.Rotate(-90) }},
		{"FlipHorizontal", "CommandPalette.Image.FlipHorizontal", "Flip horizontally", "Mirror the image horizontally", "Alt+.", nil, func(iv *ImageView) { iv.Flip(true, false) }},
		{"FlipVertical", "CommandPalette.Image.FlipVertical", "Flip vertically", "Mirror the image vertically", "Alt+,", nil, func(iv *ImageView) { iv.Flip(false, true) }},
		{"Next", "CommandPalette.Image.Next", "Next image", "Open the next image", "PgDn, Space", nil, func(iv *ImageView) { iv.ProcessKey(ParseFarKey("Space")) }},
		{"Previous", "CommandPalette.Image.Previous", "Previous image", "Open the previous image", "PgUp", nil, func(iv *ImageView) { iv.ProcessKey(ParseFarKey("PgUp")) }},
		{"First", "CommandPalette.Image.First", "First image", "Open the first image", "Home", nil, func(iv *ImageView) { iv.ProcessKey(ParseFarKey("Home")) }},
		{"Last", "CommandPalette.Image.Last", "Last image", "Open the last image", "End", nil, func(iv *ImageView) { iv.ProcessKey(ParseFarKey("End")) }},
		{"Gallery", "CommandPalette.Image.Gallery", "Toggle gallery", "Show or hide the image gallery", "F12", func(iv *ImageView) bool { return iv.gal != nil }, func(iv *ImageView) { iv.ToggleGallery() }},
		{"Select", "CommandPalette.Image.Select", "Toggle image selection", "Toggle selection and advance to the next image", "Ins", nil, func(iv *ImageView) { iv.ProcessKey(ParseFarKey("Ins")) }},
		{"ClearSelection", "CommandPalette.Image.ClearSelection", "Clear image selection", "Clear selection and advance to the next image", "Del", nil, func(iv *ImageView) { iv.ProcessKey(ParseFarKey("Del")) }},
		{"Close", "CommandPalette.Image.Close", "Close image viewer", "Close the image viewer", "Esc, F10", nil, func(iv *ImageView) { iv.Close() }},
	}
	entries := make([]commandPaletteEntry, 0, len(specs))
	for _, command := range specs {
		command := command
		label := Msg(command.labelKey)
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
			SearchFields:       append(commandPaletteTranslations("CommandPalette.CategoryImageViewer", command.labelKey), "ImageView"),
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

func commandPaletteQueueEntries(queue *QueueFrame) []commandPaletteEntry {
	if queue == nil {
		return nil
	}
	category := Msg("CommandPalette.CategoryQueue")
	newEntry := func(id, labelKey, english, description, shortcut string, run func(*QueueFrame) bool) commandPaletteEntry {
		label := Msg(labelKey)
		if label == "" || strings.HasPrefix(label, "{") {
			label = english
		}
		return commandPaletteEntry{
			Key:                "queue:" + strings.ToLower(id),
			Label:              plainLabel(label),
			EnglishLabel:       english,
			Description:        plainLabel(label),
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
		newEntry("OpenDetails", "CommandPalette.Queue.OpenDetails", "Open task details", "Open details for the selected queue task", "Enter", func(qf *QueueFrame) bool {
			index := qf.table.SelectPos
			if index < 0 || index >= len(qf.tasks) {
				return false
			}
			qf.openTaskDetails(index)
			return true
		}),
		newEntry("Cancel", "Queue.BtnCancel", "Cancel task", "Cancel the selected queue task", "", commandPaletteCancelQueueTask),
		newEntry("Clear", "Queue.BtnClear", "Clear completed tasks", "Remove completed tasks from the queue", "", commandPaletteClearQueueTasks),
		newEntry("Close", "CommandPalette.Queue.Close", "Close queue", "Close the operations queue", "Esc, F10, Ctrl+W", func(qf *QueueFrame) bool {
			return actionCloseQueueWorkspace(qf)
		}),
	}
}

// actionCloseQueueWorkspace is shared by Queue.Close and the generic
// Workspace.Close action. Active operations keep QueueFrame's exact veto and
// toast; once the queue is idle the screen is actually removed instead of
// stopping at BaseWindow's (unhandled) Ctrl+W path.
func actionCloseQueueWorkspace(queue *QueueFrame) bool {
	if queue == nil || vtui.FrameManager == nil || vtui.FrameManager.GetTopFrame() != queue {
		return false
	}
	return actionWorkspaceClose()
}

func commandPaletteCancelQueueTask(queue *QueueFrame) bool {
	index := queue.table.SelectPos
	if index < 0 || index >= len(queue.tasks) {
		return false
	}
	task := queue.tasks[index]
	task.mu.Lock()
	state, id := task.State, task.ID
	task.mu.Unlock()
	if !queueTaskCancellable(state) {
		return false
	}
	vtui.ShowMessageOn(queue, " "+Msg("CommandPalette.Confirm")+" ", fmt.Sprintf(Msg("CommandPalette.Queue.CancelQuestion"), id), []string{Msg("CommandPalette.Yes"), Msg("CommandPalette.No")}).OnResult = func(choice int) {
		if choice == 0 {
			GlobalQueueManager.Cancel(id)
		}
	}
	return true
}

func commandPaletteClearQueueTasks(*QueueFrame) bool {
	GlobalQueueManager.mu.Lock()
	active := make([]*QueueTask, 0, len(GlobalQueueManager.tasks))
	removed := false
	for _, task := range GlobalQueueManager.tasks {
		task.mu.Lock()
		terminal := queueTaskTerminal(task.State)
		task.mu.Unlock()
		if terminal {
			removed = true
			continue
		}
		active = append(active, task)
	}
	GlobalQueueManager.tasks = active
	GlobalQueueManager.mu.Unlock()
	if removed {
		GlobalQueueManager.RefreshUI()
	}
	return removed
}
