package extui

import (
	"encoding/binary"
	"hash/fnv"
	"strconv"
)

const (
	Schema       = "app"
	SceneVersion = 4
)

type M = map[string]any

// Scene является корневым объектом для экспорта состояния f4
type Scene struct {
	Revision        uint64
	Width           int
	Height          int
	ActiveScreen    int
	WorkspaceCount  int
	WorkspaceTabs   M
	Presentation    string
	QmlIconSet      string
	Shell           *ShellModel
	MenuBar         *MenuModel
	KeyBar          *KeyBarModel
	Toast           *ToastModel
	Dialogs         []DialogModel
	Menus           []MenuModel
	Surface         *SurfaceModel
	OperationsQueue *OperationsQueueModel
	Legacy          M
}

type ShellModel struct {
	ID             string
	Title          string
	Mode           string
	ActivePanel    int
	ShowPanels     bool
	ShowLeftPanel  bool
	ShowRightPanel bool
	Wide           bool
	WidePanel      int
	PanelLayout    PanelLayoutModel
	ShowKeyBar     bool
	TerminalBusy   bool
	TerminalActive bool
	MacroRecording bool
	Fallback       bool
	FallbackReason string
	Panels         []PanelModel
	InfoPanels     []InfoPanelModel
	QuickViews     []QuickViewModel
	CommandLine    *CommandLineModel
	Terminal       *TerminalModel
}

// PanelLayoutModel is the small, presentation-independent part of commander
// geometry. SplitColumn is the first column owned by the right panel. The
// bottom insets describe rows exposed beneath each panel for the terminal.
// Keeping this in the shell header lets native frontends reproduce a saved
// layout without receiving either panel's file catalog.
type PanelLayoutModel struct {
	Columns              int
	SplitColumn          int
	LeftBottomInsetRows  int
	RightBottomInsetRows int
}

type InfoPanelModel struct {
	ID         string
	Side       int
	Active     bool
	Title      string
	BottomHint string
	Rows       []InfoPanelRowModel
}

type InfoPanelRowModel struct {
	Kind  string
	Label string
	Value string
}

// QuickViewModel is the embedded Ctrl+Q panel.  Its chrome and source
// identity live beside a regular bounded SurfaceModel so native frontends can
// reuse the same smooth-scrolling viewport as Viewer and Editor without
// mistaking the preview for a standalone document frame.
type QuickViewModel struct {
	ID          string
	Side        int
	SourceSide  int
	Active      bool
	Title       string
	BottomHint  string
	ContentKey  string
	Name        string
	Path        string
	SizeText    string
	PreviewKind string
	Label       string
	Error       string
	Loading     bool
	Wrap        bool
	HeaderRows  []TextRowModel
	ImageSource string
	ImageWidth  int
	ImageHeight int
	Surface     SurfaceModel
}

type PanelModel struct {
	ID                    string
	Side                  int
	Active                bool
	Path                  string
	Title                 string
	ShowFileInfo          bool
	GalleryLayoutMode     string
	GalleryColumnCount    int
	GalleryDensity        int
	GalleryLayoutRevision int64
	SourceKind            string
	PreviewCapable        bool
	CatalogRevision       int64
	SelectionRevision     int64
	// MetadataDeferred means Entries contains only the interactive catalog
	// identity.  Expensive filesystem/display metadata is fetched separately
	// with the exact CatalogRevision and MetadataRevision advertised here.
	MetadataDeferred       bool
	MetadataRevision       int64
	HighlightRevision      int64
	HighlightStyles        map[string]HighlightStyleModel
	CursorEntryID          string
	SortMode               string
	SortReverse            bool
	SeparateFileExtensions bool
	Cursor                 int
	Loading                bool
	CatalogProvisional     bool
	FastFind               bool
	FastFindText           string
	FastFindMatchColor     string
	FastFindMatches        map[string]FastFindMatchModel
	SelectedCount          int
	SelectedSize           int64
	TotalCount             int
	TotalSize              int64
	GalleryColumns         []PanelColumnModel
	Entries                []FileEntryModel
}

// FastFindMatchModel identifies the contiguous rune span painted by f4's
// quick-search matcher for one stable catalog entry.
type FastFindMatchModel struct {
	Start  int
	Length int
}

type PanelColumnModel struct {
	ID        string
	Role      string
	Index     int
	Title     string
	Width     int
	Alignment string
	SortMode  string
	Sortable  bool
}

type FileEntryModel struct {
	Index            int
	EntryID          string
	Name             string
	DisplayBaseName  string
	DisplayExtension string
	Path             string
	LocalPath        string
	Size             int64
	SizeText         string
	IsDir            bool
	IsUp             bool
	IsHidden         bool
	IsExecutable     bool
	IsCached         bool
	IsImage          bool
	Selected         bool
	SizeCalculated   bool
	MTime            string
	MTimeNanos       int64
	Version          string
	Source           *ImageSourceModel
	Mode             string
	HighlightStyleID string
}

// ImageSourceModel is an opaque, broker-backed source descriptor. Native
// frontends use ResourceID only on the separately authenticated media channel;
// SourceKey and Version identify reusable derived artifacts.
type ImageSourceModel struct {
	ResourceID      string
	SourceKey       string
	Version         string
	VersionStrength string
	Size            int64
	SizeKnown       bool
	AccessProfile   string
	StorageClass    string
}

func (s ImageSourceModel) ToMap() M {
	return M{
		"resourceId":      s.ResourceID,
		"sourceKey":       s.SourceKey,
		"version":         s.Version,
		"versionStrength": s.VersionStrength,
		"size":            s.Size,
		"sizeKnown":       s.SizeKnown,
		"accessProfile":   s.AccessProfile,
		"storageClass":    s.StorageClass,
	}
}

// FileEntryMetadataModel is the deferred, revision-bound portion of a file
// catalog row consumed by the native gallery. EntryID and Index join it back
// to the minimal row in PanelModel; identity/type fields already carried by
// that base row are deliberately not repeated here.
type FileEntryMetadataModel struct {
	Index            int
	EntryID          string
	LocalPath        string
	Size             int64
	SizeText         string
	MTime            string
	MTimeNanos       int64
	Mode             string
	HighlightStyleID string
}

// PanelCatalogMetadataModel is one bounded response to a frontend pull.  All
// chunks for a catalog share the same revisions; Final marks the last chunk.
type PanelCatalogMetadataModel struct {
	PanelID           string
	Path              string
	CatalogRevision   int64
	MetadataRevision  int64
	HighlightRevision int64
	Offset            int
	Limit             int
	Total             int
	TotalSize         int64
	Final             bool
	Entries           []FileEntryMetadataModel
	HighlightStyles   map[string]HighlightStyleModel
}

type HighlightGroupModel struct {
	ID   string
	Name string
}

type HighlightColorPatchModel struct {
	Foreground string
	Background string
}

type HighlightStyleModel struct {
	Groups         []HighlightGroupModel
	Marker         string
	Icon           string
	Normal         HighlightColorPatchModel
	Selected       HighlightColorPatchModel
	Cursor         HighlightColorPatchModel
	SelectedCursor HighlightColorPatchModel
}

type CommandLineModel struct {
	ID               string
	Visible          bool
	Focused          bool
	Prompt           string
	PromptRuns       []RunModel
	Text             string
	Empty            bool
	Runs             []RunModel
	InputX           int
	CursorPrefixRuns []RunModel
	CursorX          int
	CursorPosition   int
	SelectionStart   int
	SelectionEnd     int
	CursorVisible    bool
	CursorShape      string
}

type TerminalModel struct {
	ID        string
	Title     string
	Visible   bool
	Focused   bool
	AltScreen bool
	Busy      bool
	CursorX   int
	CursorY   int
	Rows      []TextRowModel
}

type SurfaceModel struct {
	ID                 string
	Kind               string
	DefaultBackground  string
	Title              string
	Path               string
	BaseName           string
	Mode               string
	Busy               bool
	Dirty              bool
	Saving             bool
	HexMode            bool
	WrapMode           bool
	WordWrap           bool
	Overtype           bool
	TopOffset          int64
	Size               int64
	CursorLine         int
	CursorPos          int
	CursorVisualRow    int
	CursorVisualColumn int
	CursorVisible      bool
	CursorShape        string
	ScrollTop          int
	ScrollLeft         int
	// DocumentKey changes when the content represented by a persistent native
	// viewport changes. ScrollAction names the semantic action used to request
	// another bounded window.
	DocumentKey  string
	ScrollAction string
	// ScrollUnit describes the absolute coordinate space used by the bounded
	// semantic text window: "bytes" for Viewer and "rows" for Editor.
	ScrollUnit         string
	WindowStart        int64
	WindowEnd          int64
	ViewportStart      int64
	ViewportSpan       int64
	ContentExtent      int64
	ContentExtentKnown bool
	ViewportRow        int
	ViewportRows       int
	CursorAbsoluteRow  int64
	WindowGeneration   uint64
	// WindowContentKey fingerprints every row and styled run in WindowRows.
	// Native renderers use it to avoid re-serializing a multi-megabyte model
	// merely to discover that an unrelated scene update left the window intact.
	WindowContentKey string
	Selection        bool
	Autocomplete     M
	Rows             []TextRowModel
	WindowRows       []TextRowModel
}

// OperationsQueueModel is the native representation of the background
// operations workspace.  It deliberately exposes only the operations that
// exist in the terminal UI: selecting a row, opening its details, cancelling
// a cancellable task and clearing terminal tasks.
type OperationsQueueModel struct {
	ID              string
	Title           string
	Selected        int
	SelectedTaskID  int
	Top             int
	WorkspaceIndex  int
	WorkspaceNumber int
	TabID           string
	ActiveCount     int
	QueuedCount     int
	RunningCount    int
	CompletedCount  int
	ErrorCount      int
	CancelledCount  int
	HasActive       bool
	CanClear        bool
	CanClose        bool
	CancelText      string
	ClearText       string
	EmptyText       string
	DetailsText     string
	Columns         []OperationsQueueColumnModel
	Items           []OperationsQueueItemModel
}

type OperationsQueueColumnModel struct {
	ID        string
	Title     string
	Width     int
	Alignment string
}

type OperationsQueueItemModel struct {
	ID              string
	TaskID          int
	Index           int
	Type            string
	Description     string
	State           string
	StateClass      string
	Action          string
	CurrentFile     string
	DisplayText     string
	CurrentProgress int
	Progress        int
	TotalText       string
	Elapsed         string
	ETA             string
	Speed           string
	Error           string
	Cancellable     bool
	HasDetails      bool
	Terminal        bool
	Active          bool
	CancelPrompt    string
}

type TextRowModel struct {
	Index       int
	VisualRow   int
	LogicalLine int
	Offset      int64
	EndOffset   int64
	Text        string
	Runs        []RunModel
}

type RunModel struct {
	Text       string
	Attr       uint64
	Foreground string
	Background string
	Bold       bool
	Underline  bool
	Strikeout  bool
}

type MenuModel struct {
	ID          string
	Role        string
	Title       string
	Active      bool
	Selected    int
	ParentID    string
	AnchorIndex int
	Items       []MenuItemModel
	Legacy      M
}

type MenuItemModel struct {
	Index      int
	ID         string
	Text       string
	RawText    string
	Hotkey     string
	Icon       string
	IconColor  string
	Shortcut   string
	Command    int
	Separator  bool
	Header     bool
	Disabled   bool
	Checked    bool
	HasSubmenu bool
	Items      []MenuItemModel
	Legacy     M
}

type KeyBarModel struct {
	ID       string
	Visible  bool
	Modifier string
	Items    []KeyBarItemModel
}

type KeyBarItemModel struct {
	Index int
	Key   string
	Text  string
}

type DialogModel struct {
	ID        string
	Kind      string
	Title     string
	Modal     bool
	Busy      bool
	Progress  int
	ShowClose bool
	Controls  []ControlModel
	Legacy    M
}

type ControlModel struct {
	ID         string
	Kind       string
	Visible    bool
	Focused    bool
	Disabled   bool
	Text       string
	Title      string
	Hotkey     string
	State      int
	ThreeState bool
	Default    bool
	Password   bool
	Cursor     int
	Left       int
	Selected   []int
	Items      []string
	Rows       []M
	Children   []ControlModel
	Legacy     M
}

type ToastModel struct {
	Message string
}

func (s Scene) ToMap() M {
	out := M{
		"type":         "scene",
		"schema":       Schema,
		"version":      SceneVersion,
		"width":        s.Width,
		"height":       s.Height,
		"activeScreen": s.ActiveScreen,
	}
	if s.Revision > 0 {
		out["revision"] = s.Revision
	}
	if s.Presentation != "" {
		out["presentation"] = s.Presentation
	}
	if s.QmlIconSet != "" {
		out["qmlIconSet"] = s.QmlIconSet
	}
	if s.WorkspaceCount > 0 {
		out["workspaceCount"] = s.WorkspaceCount
	}
	if s.WorkspaceTabs != nil {
		out["workspaceTabs"] = s.WorkspaceTabs
	}
	if s.Shell != nil {
		out["shell"] = s.Shell.ToMap()
	}
	if s.MenuBar != nil {
		out["menuBar"] = s.MenuBar.ToMap()
	}
	if s.KeyBar != nil {
		out["keyBar"] = s.KeyBar.ToMap()
	}
	if s.Toast != nil {
		out["toast"] = M{"message": s.Toast.Message}
	}
	if len(s.Dialogs) > 0 {
		out["dialogs"] = dialogsToMaps(s.Dialogs)
	}
	if len(s.Menus) > 0 {
		out["menus"] = menusToMaps(s.Menus)
	}
	if s.Surface != nil {
		out["surface"] = s.Surface.ToMap()
	}
	if s.OperationsQueue != nil {
		out["operationsQueue"] = s.OperationsQueue.ToMap()
	}
	if s.Legacy != nil {
		out["legacy"] = s.Legacy
		if frames, ok := s.Legacy["frames"]; ok {
			out["frames"] = frames
		}
		if screens, ok := s.Legacy["screens"]; ok {
			out["screens"] = screens
		}
	}
	return out
}

func (s ShellModel) ToMap() M {
	out := M{
		"id":             s.ID,
		"kind":           "shell",
		"title":          s.Title,
		"mode":           s.Mode,
		"activePanel":    s.ActivePanel,
		"showPanels":     s.ShowPanels,
		"showLeftPanel":  s.ShowLeftPanel,
		"showRightPanel": s.ShowRightPanel,
		"wide":           s.Wide,
		"panelLayout":    s.PanelLayout.ToMap(),
		"showKeyBar":     s.ShowKeyBar,
		"terminalBusy":   s.TerminalBusy,
		"terminalActive": s.TerminalActive,
		"macroRecording": s.MacroRecording,
		"panels":         panelsToMaps(s.Panels),
	}
	if s.Wide {
		out["widePanel"] = s.WidePanel
	}
	if s.Fallback {
		out["fallback"] = true
		out["reason"] = s.FallbackReason
	}
	if len(s.InfoPanels) > 0 {
		out["infoPanels"] = infoPanelsToMaps(s.InfoPanels)
	}
	if len(s.QuickViews) > 0 {
		out["quickViews"] = quickViewsToMaps(s.QuickViews)
	}
	if s.CommandLine != nil {
		out["commandLine"] = s.CommandLine.ToMap()
	}
	if s.Terminal != nil {
		out["terminal"] = s.Terminal.ToMap()
	}
	return out
}

func (p PanelLayoutModel) ToMap() M {
	return M{
		"columns":              p.Columns,
		"splitColumn":          p.SplitColumn,
		"leftBottomInsetRows":  p.LeftBottomInsetRows,
		"rightBottomInsetRows": p.RightBottomInsetRows,
	}
}

func (p InfoPanelModel) ToMap() M {
	rows := make([]M, 0, len(p.Rows))
	for _, row := range p.Rows {
		rows = append(rows, M{
			"kind":  row.Kind,
			"label": row.Label,
			"value": row.Value,
		})
	}
	return M{
		"id":         p.ID,
		"kind":       "infoPanel",
		"side":       p.Side,
		"active":     p.Active,
		"title":      p.Title,
		"bottomHint": p.BottomHint,
		"rows":       rows,
	}
}

func (q QuickViewModel) ToMap() M {
	return M{
		"id":          q.ID,
		"kind":        "quickViewPanel",
		"side":        q.Side,
		"sourceSide":  q.SourceSide,
		"active":      q.Active,
		"title":       q.Title,
		"bottomHint":  q.BottomHint,
		"contentKey":  q.ContentKey,
		"name":        q.Name,
		"path":        q.Path,
		"sizeText":    q.SizeText,
		"previewKind": q.PreviewKind,
		"label":       q.Label,
		"error":       q.Error,
		"loading":     q.Loading,
		"wrap":        q.Wrap,
		"headerRows":  rowsToMaps(q.HeaderRows),
		"imageSource": q.ImageSource,
		"imageWidth":  q.ImageWidth,
		"imageHeight": q.ImageHeight,
		"surface":     q.Surface.ToMap(),
	}
}

func (p PanelModel) ToMap() M {
	columnsToMaps := func(source []PanelColumnModel) []M {
		columns := make([]M, 0, len(source))
		for _, column := range source {
			columns = append(columns, M{
				"id":        column.ID,
				"role":      column.Role,
				"index":     column.Index,
				"title":     column.Title,
				"width":     column.Width,
				"alignment": column.Alignment,
				"sortMode":  column.SortMode,
				"sortable":  column.Sortable,
			})
		}
		return columns
	}
	out := M{
		"id":                     p.ID,
		"kind":                   "filePanel",
		"side":                   p.Side,
		"active":                 p.Active,
		"path":                   p.Path,
		"title":                  p.Title,
		"showFileInfo":           p.ShowFileInfo,
		"galleryLayoutMode":      p.GalleryLayoutMode,
		"galleryColumnCount":     p.GalleryColumnCount,
		"galleryDensity":         p.GalleryDensity,
		"galleryLayoutRevision":  p.GalleryLayoutRevision,
		"sourceKind":             p.SourceKind,
		"previewCapable":         p.PreviewCapable,
		"catalogRevision":        p.CatalogRevision,
		"selectionRevision":      p.SelectionRevision,
		"cursorEntryId":          p.CursorEntryID,
		"sortModeName":           p.SortMode,
		"sortReverse":            p.SortReverse,
		"separateFileExtensions": p.SeparateFileExtensions,
		"cursor":                 p.Cursor,
		"loading":                p.Loading,
		"catalogProvisional":     p.CatalogProvisional,
		"fastFind":               p.FastFind,
		"fastFindText":           p.FastFindText,
		"selectedCount":          p.SelectedCount,
		"totalCount":             p.TotalCount,
		"galleryColumns":         columnsToMaps(p.GalleryColumns),
	}
	if p.FastFindMatchColor != "" {
		out["fastFindMatchColor"] = p.FastFindMatchColor
	}
	if len(p.FastFindMatches) > 0 {
		matches := make(M, len(p.FastFindMatches))
		for entryID, match := range p.FastFindMatches {
			matches[entryID] = M{
				"start":  match.Start,
				"length": match.Length,
			}
		}
		out["fastFindMatches"] = matches
	}
	if p.MetadataDeferred {
		out["metadataDeferred"] = true
		out["metadataRevision"] = p.MetadataRevision
		out["entries"] = minimalEntriesToMaps(p.Entries)
	} else {
		out["highlightRevision"] = p.HighlightRevision
		out["selectedSize"] = p.SelectedSize
		out["totalSize"] = p.TotalSize
		out["entries"] = entriesToMaps(p.Entries)
	}
	if len(p.HighlightStyles) > 0 {
		styles := make(M, len(p.HighlightStyles))
		for id, style := range p.HighlightStyles {
			styles[id] = style.ToMap()
		}
		out["highlightStyles"] = styles
	}
	return out
}

func (e FileEntryModel) ToMap() M {
	out := M{
		"index":            e.Index,
		"entryId":          e.EntryID,
		"name":             e.Name,
		"displayBaseName":  e.DisplayBaseName,
		"displayExtension": e.DisplayExtension,
		"path":             e.Path,
		"localPath":        e.LocalPath,
		"size":             e.Size,
		"sizeText":         e.SizeText,
		"isDir":            e.IsDir,
		"isUp":             e.IsUp,
		"isHidden":         e.IsHidden,
		"isExecutable":     e.IsExecutable,
		"isCached":         e.IsCached,
		"isImage":          e.IsImage,
		"selected":         e.Selected,
		"sizeCalculated":   e.SizeCalculated,
		"mtime":            e.MTime,
		"mtimeNanos":       e.MTimeNanos,
		"version":          e.Version,
		"mode":             e.Mode,
	}
	if e.HighlightStyleID != "" {
		out["highlightStyleId"] = e.HighlightStyleID
	}
	if e.Source != nil {
		out["source"] = e.Source.ToMap()
	}
	return out
}

// MinimalToMap serializes only fields needed to paint and interact with a
// catalog immediately. Stable entry identity plus the catalog-scoped row is
// sufficient for cursor, selection and open intents; the potentially long
// logical/local paths arrive in the revision-bound metadata stream. In
// particular this never exposes zero-valued aliases for deferred metadata,
// because consumers could mistake those for final values while the matching
// metadata chunk is still pending.
func (e FileEntryModel) MinimalToMap() M {
	out := M{
		"index":            e.Index,
		"entryId":          e.EntryID,
		"name":             e.Name,
		"displayBaseName":  e.DisplayBaseName,
		"displayExtension": e.DisplayExtension,
		"isDir":            e.IsDir,
		"isUp":             e.IsUp,
		"isImage":          e.IsImage,
		"selected":         e.Selected,
	}
	// Hidden entries are normally sparse. Absence is the canonical false value,
	// so ordinary large directories pay no per-row payload cost for this
	// first-frame presentation bit.
	if e.IsHidden {
		out["isHidden"] = true
	}
	// The fast base pass already resolves name/dir/hidden-based highlight
	// rules (see FileHighlighter.SemanticStyle's metadataKnown parameter);
	// omitting this field here would silently discard that color/icon until
	// the deferred metadata pass recomputes it.
	if e.HighlightStyleID != "" {
		out["highlightStyleId"] = e.HighlightStyleID
	}
	// Broker-backed source identities are part of the immediately usable
	// catalog even when filesystem/stat metadata is deferred. Without this
	// descriptor virtual-panel images have no readable source for thumbnails.
	if e.Source != nil {
		out["source"] = e.Source.ToMap()
	}
	return out
}

func (e FileEntryMetadataModel) ToMap() M {
	out := M{
		"index":      e.Index,
		"entryId":    e.EntryID,
		"localPath":  e.LocalPath,
		"size":       e.Size,
		"sizeText":   e.SizeText,
		"mtime":      e.MTime,
		"mtimeNanos": e.MTimeNanos,
		"mode":       e.Mode,
	}
	if e.HighlightStyleID != "" {
		out["highlightStyleId"] = e.HighlightStyleID
	}
	return out
}

func (p PanelCatalogMetadataModel) ToMap() M {
	entries := make([]M, 0, len(p.Entries))
	for _, entry := range p.Entries {
		entries = append(entries, entry.ToMap())
	}
	out := M{
		"type":              "panel_catalog_metadata",
		"panelId":           p.PanelID,
		"path":              p.Path,
		"catalogRevision":   p.CatalogRevision,
		"metadataRevision":  p.MetadataRevision,
		"highlightRevision": p.HighlightRevision,
		"offset":            p.Offset,
		"limit":             p.Limit,
		"total":             p.Total,
		"totalSize":         p.TotalSize,
		"final":             p.Final,
		"entries":           entries,
	}
	if len(p.HighlightStyles) > 0 {
		styles := make(M, len(p.HighlightStyles))
		for id, style := range p.HighlightStyles {
			styles[id] = style.ToMap()
		}
		out["highlightStyles"] = styles
	}
	return out
}

func (p HighlightColorPatchModel) ToMap() M {
	out := M{}
	if p.Foreground != "" {
		out["foreground"] = p.Foreground
	}
	if p.Background != "" {
		out["background"] = p.Background
	}
	return out
}

func (s HighlightStyleModel) ToMap() M {
	groups := make([]M, 0, len(s.Groups))
	for _, group := range s.Groups {
		groups = append(groups, M{"id": group.ID, "name": group.Name})
	}
	return M{
		"groups":         groups,
		"marker":         s.Marker,
		"icon":           s.Icon,
		"normal":         s.Normal.ToMap(),
		"selected":       s.Selected.ToMap(),
		"cursor":         s.Cursor.ToMap(),
		"selectedCursor": s.SelectedCursor.ToMap(),
	}
}

func (c CommandLineModel) ToMap() M {
	return M{
		"id":               c.ID,
		"kind":             "commandLine",
		"visible":          c.Visible,
		"focused":          c.Focused,
		"prompt":           c.Prompt,
		"promptRuns":       runsToMaps(c.PromptRuns),
		"text":             c.Text,
		"empty":            c.Empty,
		"runs":             runsToMaps(c.Runs),
		"inputX":           c.InputX,
		"cursorPrefixRuns": runsToMaps(c.CursorPrefixRuns),
		"cursorX":          c.CursorX,
		"cursorPosition":   c.CursorPosition,
		"selectionStart":   c.SelectionStart,
		"selectionEnd":     c.SelectionEnd,
		"cursorVisible":    c.CursorVisible,
		"cursorShape":      c.CursorShape,
	}
}

func (t TerminalModel) ToMap() M {
	return M{
		"id":        t.ID,
		"kind":      "terminal",
		"title":     t.Title,
		"visible":   t.Visible,
		"focused":   t.Focused,
		"altScreen": t.AltScreen,
		"busy":      t.Busy,
		"cursorX":   t.CursorX,
		"cursorY":   t.CursorY,
		"rows":      rowsToMaps(t.Rows),
	}
}

func (d SurfaceModel) ToMap() M {
	out := M{
		"id":                 d.ID,
		"kind":               d.Kind,
		"defaultBackground":  d.DefaultBackground,
		"title":              d.Title,
		"path":               d.Path,
		"baseName":           d.BaseName,
		"mode":               d.Mode,
		"busy":               d.Busy,
		"dirty":              d.Dirty,
		"saving":             d.Saving,
		"hexMode":            d.HexMode,
		"wrapMode":           d.WrapMode,
		"wordWrap":           d.WordWrap,
		"overtype":           d.Overtype,
		"topOffset":          d.TopOffset,
		"size":               d.Size,
		"cursorLine":         d.CursorLine,
		"cursorPos":          d.CursorPos,
		"cursorVisualRow":    d.CursorVisualRow,
		"cursorVisualColumn": d.CursorVisualColumn,
		"cursorVisible":      d.CursorVisible,
		"cursorShape":        d.CursorShape,
		"scrollTop":          d.ScrollTop,
		"scrollLeft":         d.ScrollLeft,
		"documentKey":        d.DocumentKey,
		"scrollAction":       d.ScrollAction,
		"scrollUnit":         d.ScrollUnit,
		"windowStart":        d.WindowStart,
		"windowEnd":          d.WindowEnd,
		"viewportStart":      d.ViewportStart,
		"viewportSpan":       d.ViewportSpan,
		"contentExtent":      d.ContentExtent,
		"contentExtentKnown": d.ContentExtentKnown,
		"viewportRow":        d.ViewportRow,
		"viewportRows":       d.ViewportRows,
		"cursorAbsoluteRow":  d.CursorAbsoluteRow,
		"windowGeneration":   d.WindowGeneration,
		"windowContentKey":   d.windowContentKey(),
		"selection":          d.Selection,
		"rows":               rowsToMaps(d.Rows),
		"windowRows":         rowsToMaps(d.WindowRows),
	}
	if d.Autocomplete != nil {
		out["autocomplete"] = d.Autocomplete
	}
	return out
}

func (q OperationsQueueModel) ToMap() M {
	columns := make([]M, 0, len(q.Columns))
	for _, column := range q.Columns {
		columns = append(columns, M{
			"id":        column.ID,
			"title":     column.Title,
			"width":     column.Width,
			"alignment": column.Alignment,
		})
	}
	items := make([]M, 0, len(q.Items))
	for _, item := range q.Items {
		items = append(items, item.ToMap())
	}
	return M{
		"id":              q.ID,
		"kind":            "operationsQueue",
		"title":           q.Title,
		"selected":        q.Selected,
		"selectedTaskId":  q.SelectedTaskID,
		"top":             q.Top,
		"workspaceIndex":  q.WorkspaceIndex,
		"workspaceNumber": q.WorkspaceNumber,
		"tabId":           q.TabID,
		"activeCount":     q.ActiveCount,
		"queuedCount":     q.QueuedCount,
		"runningCount":    q.RunningCount,
		"completedCount":  q.CompletedCount,
		"errorCount":      q.ErrorCount,
		"cancelledCount":  q.CancelledCount,
		"hasActive":       q.HasActive,
		"canClear":        q.CanClear,
		"canClose":        q.CanClose,
		"cancelText":      q.CancelText,
		"clearText":       q.ClearText,
		"emptyText":       q.EmptyText,
		"detailsText":     q.DetailsText,
		"columns":         columns,
		"items":           items,
	}
}

func (i OperationsQueueItemModel) ToMap() M {
	return M{
		"id":              i.ID,
		"taskId":          i.TaskID,
		"index":           i.Index,
		"type":            i.Type,
		"description":     i.Description,
		"state":           i.State,
		"stateClass":      i.StateClass,
		"action":          i.Action,
		"currentFile":     i.CurrentFile,
		"displayText":     i.DisplayText,
		"currentProgress": i.CurrentProgress,
		"progress":        i.Progress,
		"totalText":       i.TotalText,
		"elapsed":         i.Elapsed,
		"eta":             i.ETA,
		"speed":           i.Speed,
		"error":           i.Error,
		"cancellable":     i.Cancellable,
		"hasDetails":      i.HasDetails,
		"terminal":        i.Terminal,
		"active":          i.Active,
		"cancelPrompt":    i.CancelPrompt,
	}
}

func (d SurfaceModel) windowContentKey() string {
	if d.WindowContentKey != "" {
		return d.WindowContentKey
	}
	return WindowRowsContentKey(d.WindowRows)
}

// WindowRowsContentKey returns a deterministic, allocation-light fingerprint
// of the complete semantic window, including every rendering attribute. Row
// extents alone are insufficient: an edit, selection or syntax-state update
// can repaint an existing coordinate range without moving it.
func WindowRowsContentKey(rows []TextRowModel) string {
	h := fnv.New64a()
	var number [8]byte
	writeUint64 := func(value uint64) {
		binary.LittleEndian.PutUint64(number[:], value)
		_, _ = h.Write(number[:])
	}
	writeString := func(value string) {
		writeUint64(uint64(len(value)))
		_, _ = h.Write([]byte(value))
	}
	writeBool := func(value bool) {
		if value {
			_, _ = h.Write([]byte{1})
		} else {
			_, _ = h.Write([]byte{0})
		}
	}

	writeString("f4-window-content-v1")
	writeUint64(uint64(len(rows)))
	for _, row := range rows {
		writeUint64(uint64(int64(row.Index)))
		writeUint64(uint64(int64(row.VisualRow)))
		writeUint64(uint64(int64(row.LogicalLine)))
		writeUint64(uint64(row.Offset))
		writeUint64(uint64(row.EndOffset))
		writeString(row.Text)
		writeUint64(uint64(len(row.Runs)))
		for _, run := range row.Runs {
			writeString(run.Text)
			writeUint64(run.Attr)
			writeString(run.Foreground)
			writeString(run.Background)
			writeBool(run.Bold)
			writeBool(run.Underline)
			writeBool(run.Strikeout)
		}
	}
	return "w1-" + strconv.FormatUint(h.Sum64(), 16)
}

func (r TextRowModel) ToMap() M {
	out := M{
		"index":       r.Index,
		"visualRow":   r.VisualRow,
		"logicalLine": r.LogicalLine,
		"offset":      r.Offset,
		"endOffset":   r.EndOffset,
	}
	if r.Text != "" {
		out["text"] = r.Text
	}
	if len(r.Runs) > 0 {
		out["runs"] = runsToMaps(r.Runs)
	}
	return out
}

func (r RunModel) ToMap() M {
	return M{
		"text":       r.Text,
		"attr":       r.Attr,
		"foreground": r.Foreground,
		"background": r.Background,
		"bold":       r.Bold,
		"underline":  r.Underline,
		"strikeout":  r.Strikeout,
	}
}

func (m MenuModel) ToMap() M {
	out := M{
		"id":       m.ID,
		"kind":     "menu",
		"role":     m.Role,
		"title":    m.Title,
		"active":   m.Active,
		"selected": m.Selected,
		"items":    menuItemsToMaps(m.Items),
	}
	if m.ParentID != "" {
		out["parentId"] = m.ParentID
		out["anchorIndex"] = m.AnchorIndex
	}
	for k, v := range m.Legacy {
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}
	return out
}

func (i MenuItemModel) ToMap() M {
	out := M{
		"index":     i.Index,
		"text":      i.Text,
		"rawText":   i.RawText,
		"hotkey":    i.Hotkey,
		"shortcut":  i.Shortcut,
		"command":   i.Command,
		"separator": i.Separator,
		"disabled":  i.Disabled,
		"checked":   i.Checked,
	}
	if i.Icon != "" {
		out["icon"] = i.Icon
	}
	if i.ID != "" {
		out["id"] = i.ID
	}
	if i.IconColor != "" {
		out["iconColor"] = i.IconColor
	}
	if i.Header {
		out["header"] = true
	}
	if i.HasSubmenu {
		out["hasSubmenu"] = true
	}
	if len(i.Items) > 0 {
		out["items"] = menuItemsToMaps(i.Items)
	}
	for k, v := range i.Legacy {
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}
	return out
}

func (k KeyBarModel) ToMap() M {
	return M{
		"id":       k.ID,
		"kind":     "keyBar",
		"visible":  k.Visible,
		"modifier": k.Modifier,
		"items":    keyBarItemsToMaps(k.Items),
	}
}

func (i KeyBarItemModel) ToMap() M {
	return M{"index": i.Index, "key": i.Key, "text": i.Text}
}

func (d DialogModel) ToMap() M {
	out := M{
		"id":        d.ID,
		"kind":      d.Kind,
		"title":     d.Title,
		"modal":     d.Modal,
		"busy":      d.Busy,
		"progress":  d.Progress,
		"showClose": d.ShowClose,
		"children":  controlsToMaps(d.Controls),
	}
	for k, v := range d.Legacy {
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}
	return out
}

func (c ControlModel) ToMap() M {
	out := M{
		"id":         c.ID,
		"kind":       c.Kind,
		"visible":    c.Visible,
		"focused":    c.Focused,
		"disabled":   c.Disabled,
		"text":       c.Text,
		"title":      c.Title,
		"hotkey":     c.Hotkey,
		"state":      c.State,
		"threeState": c.ThreeState,
		"default":    c.Default,
		"password":   c.Password,
		"cursor":     c.Cursor,
		"left":       c.Left,
	}
	if len(c.Selected) > 0 {
		out["selected"] = c.Selected
	}
	if len(c.Items) > 0 {
		out["items"] = c.Items
	}
	if len(c.Rows) > 0 {
		out["rows"] = c.Rows
	}
	if len(c.Children) > 0 {
		out["children"] = controlsToMaps(c.Children)
	}
	for k, v := range c.Legacy {
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}
	return out
}

func panelsToMaps(items []PanelModel) []M {
	out := make([]M, 0, len(items))
	for _, item := range items {
		out = append(out, item.ToMap())
	}
	return out
}

func infoPanelsToMaps(items []InfoPanelModel) []M {
	out := make([]M, 0, len(items))
	for _, item := range items {
		out = append(out, item.ToMap())
	}
	return out
}

func quickViewsToMaps(items []QuickViewModel) []M {
	out := make([]M, 0, len(items))
	for _, item := range items {
		out = append(out, item.ToMap())
	}
	return out
}

func entriesToMaps(items []FileEntryModel) []M {
	out := make([]M, 0, len(items))
	for _, item := range items {
		out = append(out, item.ToMap())
	}
	return out
}

func minimalEntriesToMaps(items []FileEntryModel) []M {
	out := make([]M, 0, len(items))
	for _, item := range items {
		out = append(out, item.MinimalToMap())
	}
	return out
}

func rowsToMaps(items []TextRowModel) []M {
	out := make([]M, 0, len(items))
	for _, item := range items {
		out = append(out, item.ToMap())
	}
	return out
}

func runsToMaps(items []RunModel) []M {
	out := make([]M, 0, len(items))
	for _, item := range items {
		out = append(out, item.ToMap())
	}
	return out
}

func menusToMaps(items []MenuModel) []M {
	out := make([]M, 0, len(items))
	for _, item := range items {
		out = append(out, item.ToMap())
	}
	return out
}

func menuItemsToMaps(items []MenuItemModel) []M {
	out := make([]M, 0, len(items))
	for _, item := range items {
		out = append(out, item.ToMap())
	}
	return out
}

func keyBarItemsToMaps(items []KeyBarItemModel) []M {
	out := make([]M, 0, len(items))
	for _, item := range items {
		out = append(out, item.ToMap())
	}
	return out
}

func dialogsToMaps(items []DialogModel) []M {
	out := make([]M, 0, len(items))
	for _, item := range items {
		out = append(out, item.ToMap())
	}
	return out
}

func controlsToMaps(items []ControlModel) []M {
	out := make([]M, 0, len(items))
	for _, item := range items {
		out = append(out, item.ToMap())
	}
	return out
}
