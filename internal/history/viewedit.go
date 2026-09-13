package history

import "time"

const ViewerEditorHistoryID = "viewer-editor"

type ViewerEditorMode string

const (
	HistoryModeView ViewerEditorMode = "view"
	HistoryModeEdit ViewerEditorMode = "edit"
)

type ViewerEditorRecord struct {
	Path     string           `json:"path"`
	Display  string           `json:"display"`
	Mode     ViewerEditorMode `json:"mode"`
	Local    bool             `json:"local,omitempty"`
	VFSType  string           `json:"vfs_type,omitempty"`
	VFSTitle string           `json:"vfs_title,omitempty"`
	Lock     bool             `json:"lock,omitempty"`
	// Timestamp is when the file was last opened. far2l stamps every
	// history record it stores; the dialog shows the stamp so a file can be
	// found by when it was opened rather than by its name (#408). Records
	// written before this existed have a zero timestamp and keep working.
	Timestamp time.Time `json:"timestamp,omitempty"`
}
