package appcmd

// The application's own frame commands, numbered from vtui.CmApp. They are the
// protocol every frame speaks through FrameManager.EmitCommand and
// HandleCommand: a panel raises CmEdit, the editor answers it, and neither
// needs to know the other's type.
//
// The package holds nothing else and imports nothing of ours, which is what
// lets the panels, the editor, the viewer and the command line all name the
// same numbers without importing one another.
//
// The values are positional — CmCopy = vtui.CmApp + iota and the rest follow —
// so inserting a line renumbers every command below it. Add at the end.

import "github.com/unxed/vtui"

const (
	CmCopy = vtui.CmApp + iota
	CmMove
	CmRename
	CmDelete
	CmView
	CmEdit
	CmSearch
	CmBackground
	CmWorkspaceNew
	CmWorkspaceClose
	CmMkDir
	CmNew
	CmLeftBrief
	CmLeftMedium
	CmLeftDetailed
	CmLeftWide
	CmRightBrief
	CmRightMedium
	CmRightDetailed
	CmRightWide
	CmFileChanged
	CmFindFile
	CmSortName
	CmSortExt
	CmSortTime
	CmSortSize
	CmSortUnsorted
	CmLeftSortName
	CmLeftSortExt
	CmLeftSortTime
	CmLeftSortSize
	CmLeftSortUnsorted
	CmRightSortName
	CmRightSortExt
	CmRightSortTime
	CmRightSortSize
	CmRightSortUnsorted
	CmSwapPanels
	CmAddArchive
	CmLeftDriveMenu
	CmRightDriveMenu
	CmLeftAIContext
	CmLeftAIChat
	CmLeftAIOut
	CmLeftAIMem
	CmRightAIContext
	CmRightAIChat
	CmRightAIOut
	CmRightAIMem
	CmExtractArchive
	CmPanelSettings
	CmEditorSettings
	CmColorerSettings
	CmAppearanceSettings
	CmConfirmationsSettings
	CmLanguage
	CmHelpLanguage
	CmPlugins
	CmHotkeyConfig
	CmUpdateSettings
	CmProxySettings
	CmBookmarks
	CmSwitchToViewer
	CmSwitchToEditor
	CmReplace
	CmPlugRing
	// CmBookmarkEmptySlot is never emitted: the bookmarks dialog tags its
	// empty rows with it and keeps it in FrameManager.DisabledCommands so
	// vtui renders them dimmed and ignores Enter on them.
	CmBookmarkEmptySlot
	// CmDriveBookmarksHeader marks the non-selectable section caption in the
	// drive menu. It is kept separate from empty folder-bookmark rows so the
	// two menus can be shown in any order.
	CmDriveBookmarksHeader
	// Sort-group toggles are appended here rather than next to the other sort
	// commands so the existing command numbers keep their values.
	CmSortGroups
	CmLeftSortGroups
	CmRightSortGroups
	// Appended for the same reason: opening the terminal in a workspace of
	// its own is new, and the commands above keep the numbers they had.
	CmWorkspaceNewTerminal
	CmLeftGroupMenu
	CmRightGroupMenu
	// Appended for the same reason: numeric ("natural") name sort is new
	// (f4#1471) and the commands above keep the numbers they had.
	CmSortNumeric
	CmLeftSortNumeric
	CmRightSortNumeric
	// Appended for the same reason: the side menus list far2l's other six
	// panel modes (f4#410), in the order of the ViewMode5 .. ViewMode0 slots.
	CmLeftViewMode5
	CmLeftViewMode6
	CmLeftViewMode7
	CmLeftViewMode8
	CmLeftViewMode9
	CmLeftViewMode0
	CmRightViewMode5
	CmRightViewMode6
	CmRightViewMode7
	CmRightViewMode8
	CmRightViewMode9
	CmRightViewMode0
	// Appended for the same reason: "show selected first" (far's Shift+F12)
	// is new and the commands above keep the numbers they had.
	CmSortSelectedFirst
	CmLeftSortSelectedFirst
	CmRightSortSelectedFirst
)
