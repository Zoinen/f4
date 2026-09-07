package main

import "github.com/unxed/vtui"

const editorMappingWorkBytes = 64 * 1024

// The byte anchor survives both source waits and further geometry changes.
// Row coordinates are installed only after the new mapping is authoritative.
type editorReflow struct {
	top, anchor int
	anchorReady bool
	session     int
}

func (ev *EditorView) continueEditorMapping(progress bool, err error) {
	if err != nil {
		ev.semanticLoadError = err.Error()
		return
	}
	if !progress || ev.reflowTaskPending || vtui.FrameManager == nil || ev.IsDone() {
		return
	}
	ev.reflowTaskPending = true
	session := ev.editSession
	vtui.FrameManager.PostTaskWithRedrawDecision(func() bool {
		ev.reflowTaskPending = false
		// A newer resize may have reused this wakeup while preserving the same
		// source anchor. Wake the current mapping, never advance captured layout.
		return !ev.IsDone() && ev.editSession == session
	})
}
