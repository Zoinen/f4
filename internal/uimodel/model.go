package uimodel

const (
	Schema       = "app"
	SceneVersion = 2
)

type M = map[string]any

type Scene struct {
	Width          int
	Height         int
	ActiveScreen   int
	WorkspaceCount int
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
	ShowKeyBar     bool
	TerminalBusy   bool
	TerminalActive bool
	MacroRecording bool
	Panels         []PanelModel
	CommandLine    *CommandLineModel
	Terminal       *TerminalModel
}

type PanelModel struct {
	ID            string
	Side          int
	Active        bool
	Path          string
	Title         string
	ViewMode      string
	SortMode      string
	SortReverse   bool
	Cursor        int
	Top           int
	Loading       bool
	FastFind      bool
	FastFindText  string
	SelectedCount int
	SelectedSize  int64
	TotalCount    int
	TotalSize     int64
	Entries       []FileEntryModel
}

type FileEntryModel struct {
	Index          int
	Name           string
	Size           int64
	SizeText       string
	IsDir          bool
	IsUp           bool
	IsHidden       bool
	IsExecutable   bool
	IsCached       bool
	Selected       bool
	SizeCalculated bool
	MTime          string
	Mode           string
}

type CommandLineModel struct {
	ID         string
	Visible    bool
	Focused    bool
	Prompt     string
	PromptRuns []RunModel
	Text       string
	Empty      bool
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
	ID           string
	Kind         string
	Title        string
	Path         string
	BaseName     string
	Mode         string
	Busy         bool
	Dirty        bool
	Saving       bool
	HexMode      bool
	WrapMode     bool
	WordWrap     bool
	Overtype     bool
	TopOffset    int64
	Size         int64
	CursorLine   int
	CursorPos    int
	ScrollTop    int
	ScrollLeft   int
	Selection    bool
	Autocomplete M
	Rows         []TextRowModel
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
	Text string
	Attr uint64
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
	if s.WorkspaceCount > 0 {
		out["workspaceCount"] = s.WorkspaceCount
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
		"showKeyBar":     s.ShowKeyBar,
		"terminalBusy":   s.TerminalBusy,
		"terminalActive": s.TerminalActive,
		"macroRecording": s.MacroRecording,
		"panels":         panelsToMaps(s.Panels),
	}
	if s.CommandLine != nil {
		out["commandLine"] = s.CommandLine.ToMap()
	}
	if s.Terminal != nil {
		out["terminal"] = s.Terminal.ToMap()
	}
	return out
}

func (p PanelModel) ToMap() M {
	return M{
		"id":            p.ID,
		"kind":          "filePanel",
		"side":          p.Side,
		"active":        p.Active,
		"path":          p.Path,
		"title":         p.Title,
		"viewModeName":  p.ViewMode,
		"sortModeName":  p.SortMode,
		"sortReverse":   p.SortReverse,
		"cursor":        p.Cursor,
		"top":           p.Top,
		"loading":       p.Loading,
		"fastFind":      p.FastFind,
		"fastFindText":  p.FastFindText,
		"selectedCount": p.SelectedCount,
		"selectedSize":  p.SelectedSize,
		"totalCount":    p.TotalCount,
		"totalSize":     p.TotalSize,
		"entries":       entriesToMaps(p.Entries),
	}
}

func (e FileEntryModel) ToMap() M {
	return M{
		"index":          e.Index,
		"name":           e.Name,
		"size":           e.Size,
		"sizeText":       e.SizeText,
		"isDir":          e.IsDir,
		"isUp":           e.IsUp,
		"isHidden":       e.IsHidden,
		"isExecutable":   e.IsExecutable,
		"isCached":       e.IsCached,
		"selected":       e.Selected,
		"sizeCalculated": e.SizeCalculated,
		"mtime":          e.MTime,
		"mode":           e.Mode,
	}
}

func (c CommandLineModel) ToMap() M {
	return M{
		"id":         c.ID,
		"kind":       "commandLine",
		"visible":    c.Visible,
		"focused":    c.Focused,
		"prompt":     c.Prompt,
		"promptRuns": runsToMaps(c.PromptRuns),
		"text":       c.Text,
		"empty":      c.Empty,
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
		"id":         d.ID,
		"kind":       d.Kind,
		"title":      d.Title,
		"path":       d.Path,
		"baseName":   d.BaseName,
		"mode":       d.Mode,
		"busy":       d.Busy,
		"dirty":      d.Dirty,
		"saving":     d.Saving,
		"hexMode":    d.HexMode,
		"wrapMode":   d.WrapMode,
		"wordWrap":   d.WordWrap,
		"overtype":   d.Overtype,
		"topOffset":  d.TopOffset,
		"size":       d.Size,
		"cursorLine": d.CursorLine,
		"cursorPos":  d.CursorPos,
		"scrollTop":  d.ScrollTop,
		"scrollLeft": d.ScrollLeft,
		"selection":  d.Selection,
		"rows":       rowsToMaps(d.Rows),
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
	return M{"text": r.Text, "attr": r.Attr}
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
		"selected":   c.Selected,
		"items":      c.Items,
		"rows":       c.Rows,
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
