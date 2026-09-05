package main

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
	// Appended to preserve numeric values of existing public commands.
	CmLeftGallery
	CmRightGallery
)
