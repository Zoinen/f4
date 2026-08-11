package extui

const (
	Schema       = "app"
	SceneVersion = 3
)

type M = map[string]any

// Scene является корневым объектом для экспорта состояния f4
type Scene struct {
	Width          int
	Height         int
	ActiveScreen   int
	WorkspaceCount int
	WorkspaceTabs  M
	Presentation   string
	QmlIconSet     string
	Shell          *ShellModel
	MenuBar        *MenuModel
	KeyBar         *KeyBarModel
	Toast          *ToastModel
	Dialogs        []DialogModel
	Menus          []MenuModel
	Surface        *SurfaceModel
	Legacy         M
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
	ShowKeyBar     bool
	TerminalBusy   bool
	TerminalActive bool
	MacroRecording bool
	Fallback       bool
	FallbackReason string
	Panels         []PanelModel
	InfoPanels     []InfoPanelModel
	CommandLine    *CommandLineModel
	Terminal       *TerminalModel
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

type PanelModel struct {
	ID                     string
	Side                   int
	Active                 bool
	Path                   string
	Title                  string
	ViewMode               string
	Presentation           string
	GalleryLayoutMode      string
	GalleryColumnCount     int
	GalleryDensity         int
	GalleryLayoutRevision  int64
	SourceKind             string
	PreviewCapable         bool
	CatalogRevision        int64
	SelectionRevision      int64
	HighlightRevision      int64
	HighlightStyles        map[string]HighlightStyleModel
	CursorEntryID          string
	SortMode               string
	SortReverse            bool
	SeparateFileExtensions bool
	Cursor                 int
	Top                    int
	Loading                bool
	FastFind               bool
	FastFindText           string
	SelectedCount          int
	SelectedSize           int64
	TotalCount             int
	TotalSize              int64
	Columns                []PanelColumnModel
	GalleryColumns         []PanelColumnModel
	Entries                []FileEntryModel
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
	LocalPath        string
	Size             int64
	SizeText         string
	IsDir            bool
	IsUp             bool
	IsHidden         bool
	IsExecutable     bool
	IsCached         bool
	Selected         bool
	SizeCalculated   bool
	MTime            string
	MTimeNanos       int64
	Version          string
	Mode             string
	HighlightStyleID string
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
	Selection          bool
	Autocomplete       M
	Rows               []TextRowModel
}

type TextRowModel struct {
	Index       int
	VisualRow   int
	LogicalLine int
	Offset      int64
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
	ID       string
	Role     string
	Title    string
	Active   bool
	Selected int
	Items    []MenuItemModel
	Legacy   M
}

type MenuItemModel struct {
	Index     int
	Text      string
	RawText   string
	Hotkey    string
	Shortcut  string
	Command   int
	Separator bool
	Disabled  bool
	Checked   bool
	Items     []MenuItemModel
	Legacy    M
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
	if s.CommandLine != nil {
		out["commandLine"] = s.CommandLine.ToMap()
	}
	if s.Terminal != nil {
		out["terminal"] = s.Terminal.ToMap()
	}
	return out
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
	columns := columnsToMaps(p.Columns)
	out := M{
		"id":                     p.ID,
		"kind":                   "filePanel",
		"side":                   p.Side,
		"active":                 p.Active,
		"path":                   p.Path,
		"title":                  p.Title,
		"viewModeName":           p.ViewMode,
		"presentation":           p.Presentation,
		"galleryLayoutMode":      p.GalleryLayoutMode,
		"galleryColumnCount":     p.GalleryColumnCount,
		"galleryDensity":         p.GalleryDensity,
		"galleryLayoutRevision":  p.GalleryLayoutRevision,
		"sourceKind":             p.SourceKind,
		"previewCapable":         p.PreviewCapable,
		"catalogRevision":        p.CatalogRevision,
		"selectionRevision":      p.SelectionRevision,
		"highlightRevision":      p.HighlightRevision,
		"cursorEntryId":          p.CursorEntryID,
		"sortModeName":           p.SortMode,
		"sortReverse":            p.SortReverse,
		"separateFileExtensions": p.SeparateFileExtensions,
		"cursor":                 p.Cursor,
		"top":                    p.Top,
		"loading":                p.Loading,
		"fastFind":               p.FastFind,
		"fastFindText":           p.FastFindText,
		"selectedCount":          p.SelectedCount,
		"selectedSize":           p.SelectedSize,
		"totalCount":             p.TotalCount,
		"totalSize":              p.TotalSize,
		"columns":                columns,
		"galleryColumns":         columnsToMaps(p.GalleryColumns),
		"entries":                entriesToMaps(p.Entries),
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
		"localPath":        e.LocalPath,
		"size":             e.Size,
		"sizeText":         e.SizeText,
		"isDir":            e.IsDir,
		"isUp":             e.IsUp,
		"isHidden":         e.IsHidden,
		"isExecutable":     e.IsExecutable,
		"isCached":         e.IsCached,
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
		"selection":          d.Selection,
		"rows":               rowsToMaps(d.Rows),
	}
	if d.Autocomplete != nil {
		out["autocomplete"] = d.Autocomplete
	}
	return out
}

func (r TextRowModel) ToMap() M {
	out := M{
		"index":       r.Index,
		"visualRow":   r.VisualRow,
		"logicalLine": r.LogicalLine,
		"offset":      r.Offset,
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

func entriesToMaps(items []FileEntryModel) []M {
	out := make([]M, 0, len(items))
	for _, item := range items {
		out = append(out, item.ToMap())
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
