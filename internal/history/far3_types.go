package history

// Far3History is a read-only snapshot of the three portable Far history kinds.
// Merging and persisting an existing snapshot does not require a SQLite reader.
type Far3History struct {
	Commands, Folders []HistoryRecord
	Files             []ViewerEditorRecord
	Skipped           int
}
