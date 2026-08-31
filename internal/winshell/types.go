package winshell

import (
	"errors"
	"time"
)

const Scheme = "windows"
const BrokerArg = "--shell-broker"

var ErrUnavailable = errors.New("Windows Shell integration is unavailable")
var ErrGalleryIndexingRequired = errors.New("Windows Search indexing must be turned on to use File Explorer Gallery")

func IsBrokerInvocation(args []string) bool {
	for _, arg := range args[1:] {
		if arg == BrokerArg {
			return true
		}
	}
	return false
}

// Section describes the visual group a navigation node belongs to. Explorer
// flattens Quick Access into the navigation pane, so section membership is
// carried separately from the Shell item's parent relationship.
type Section uint8

const (
	SectionPrimary Section = iota
	SectionQuickAccess
	SectionNamespace
)

// Node is the process-safe representation of one Windows Shell item. COM
// interface pointers and PIDLs never cross the broker boundary.
type Node struct {
	URI               string
	ParsingName       string
	Name              string
	FileSystemPath    string
	ParentParsingName string
	Section           Section
	Separator         bool
	Pinned            bool
	RequiresIndexing  bool
	Folder            bool
	HasChildren       bool
	Hidden            bool
	ReadOnly          bool
	CanCopy           bool
	CanMove           bool
	CanLink           bool
	CanRename         bool
	CanDelete         bool
	DropTarget        bool
	Size              int64
	SizeKnown         bool
	Modified          time.Time
	IconRGBA          []byte
	IconWidth         int
	IconHeight        int
}

// MaterializedFile is a local representation of a Shell stream. Owned files
// are temporary and must be removed by the receiver after closing them.
type MaterializedFile struct {
	Path  string
	Size  int64
	Owned bool
}

// ContextCommand is one entry from the item's real IContextMenu. Command IDs
// are opaque and remain valid only while the associated ContextMenu token is
// alive in the broker.
type ContextCommand struct {
	ID        uint32
	Text      string
	Separator bool
	Enabled   bool
	Default   bool
	Children  []ContextCommand
}

type ContextMenu struct {
	Token    uint64
	Commands []ContextCommand
}

type describeRequest struct {
	ParsingName string
}

type enumerateRequest struct {
	ParsingName string
}

type enumerateStatus uint8

const (
	enumerateStatusOK enumerateStatus = iota
	enumerateStatusGalleryIndexingRequired
)

type enumerateResponse struct {
	Nodes  []Node
	Status enumerateStatus
}

type newItemRequest struct {
	ParentParsingName string
	Name              string
}

type renameRequest struct {
	ParsingName string
	NewName     string
}

type deleteRequest struct {
	ParsingName string
	Recycle     bool
}

type importRequest struct {
	SourcePath        string
	ParentParsingName string
	Name              string
	Move              bool
}

type transferRequest struct {
	SourceParsingName      string
	DestinationParsingName string
	Name                   string
	Move                   bool
}

type contextMenuRequest struct {
	ParsingName string
}

type contextInvokeRequest struct {
	Token     uint64
	CommandID uint32
}

type contextDismissRequest struct {
	Token uint64
}
