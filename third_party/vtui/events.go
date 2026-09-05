package vtui

import "fmt"

// UIEvent represents a semantic event emitted by the UI framework to the host application.
type UIEvent struct {
	Kind  string    // "command" | "changed" | "selected" | "closed" | "focus" | "key" | "resize"
	SrcID string    // ID of the source element or frame
	Cmd   int       // Command ID (for "command" events) or virtual key code (for "key" events)
	Value PropValue // Value associated with the event (e.g. text/data)
	Index int       // Numeric index or exit code
}

func (e UIEvent) String() string {
	return fmt.Sprintf("UIEvent{Kind:%q SrcID:%q Cmd:%d Index:%d Value:%v}", e.Kind, e.SrcID, e.Cmd, e.Index, e.Value)
}
