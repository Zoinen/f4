package visren

import (
	"context"
	"fmt"
	"sync"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type EditorRequest struct {
	Title      string
	Content    []byte
	CursorLine int
	CursorCol  int
	OnClose    func([]byte, error)
}

type PanelHost interface {
	vfs.App
	GetMarkedNames() []string
	ReplaceMarkedNames([]string)
	OpenVisRenEditor(EditorRequest) error
}

type Plugin struct {
	mu           sync.Mutex
	undoDir      string
	undoLog      []RenamePair
	registration vfs.Registration
}

func (p *Plugin) Init(api vfs.HostAPI) error {
	if contributions, ok := api.(vfs.ContributionHost); ok {
		registration, err := contributions.RegisterPluginCommand(vfs.PluginCommand{
			ID:             "visren.open",
			Location:       vfs.PluginCommandPanel,
			Label:          "&Visual File Renamer",
			LabelKey:       "VisRen.Menu",
			Description:    "Visually rename the selected files and directories",
			DescriptionKey: "VisRen.Command.Open.Desc",
			SearchKeys:     []string{"VisRen.Title", "VisRen.Rename"},
			Run:            p.open,
		})
		if err != nil {
			return fmt.Errorf("VisRen: register command: %w", err)
		}
		p.mu.Lock()
		p.registration = registration
		p.mu.Unlock()
		return nil
	}
	api.RegisterPluginMenuItem(tr("VisRen.Menu", "&Visual File Renamer"), p.open)
	return nil
}

func (p *Plugin) Close() error {
	p.mu.Lock()
	registration := p.registration
	p.registration = nil
	p.undoDir, p.undoLog = "", nil
	p.mu.Unlock()
	if registration != nil {
		registration.Unregister()
	}
	return nil
}

func (p *Plugin) GetName() string { return "VisRen" }

func tr(key, fallback string) string {
	value := vtui.Msg(key)
	if value == "{"+key+"}" {
		return fallback
	}
	return value
}

func (p *Plugin) open(app vfs.App) {
	host, ok := app.(PanelHost)
	if !ok {
		vtui.ShowMessage(" "+tr("VisRen.Title", "Visual File Renamer")+" ", tr("VisRen.HostUnsupported", "This F4 host does not provide the VisRen UI bridge."), []string{"&Ok"})
		return
	}
	fs, ok := app.GetActivePanelVFS().(*vfs.OSVFS)
	if !ok {
		vtui.ShowMessage(" "+tr("VisRen.Title", "Visual File Renamer")+" ", tr("VisRen.LocalOnly", "VisRen works only on a local file panel."), []string{"&Ok"})
		return
	}
	names := selectedForRename(host.GetMarkedNames(), host.GetSelectedName())
	if len(names) == 0 {
		vtui.ShowMessage(" "+tr("VisRen.Title", "Visual File Renamer")+" ", tr("VisRen.SelectFiles", "Select one or more files or directories first."), []string{"&Ok"})
		return
	}
	dir := fs.GetPath()
	items := make([]*Item, 0, len(names))
	for _, name := range names {
		info, err := fs.Stat(context.Background(), fs.Join(dir, name))
		if err != nil {
			continue
		}
		items = append(items, NewItem(dir, name, info.MTime, info.IsDir))
	}
	if len(items) == 0 {
		vtui.ShowMessage(" "+tr("VisRen.Title", "Visual File Renamer")+" ", tr("VisRen.NoFiles", "None of the selected items are available."), []string{"&Ok"})
		return
	}
	showDialog(host, p, fs, dir, items, false)
}

func selectedForRename(marked []string, current string) []string {
	if len(marked) != 0 {
		return marked
	}
	if current == "" || current == ".." {
		return nil
	}
	return []string{current}
}

func (p *Plugin) getUndo() (string, []RenamePair) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.undoDir, append([]RenamePair(nil), p.undoLog...)
}

func (p *Plugin) setUndo(dir string, log []RenamePair) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.undoDir = dir
	p.undoLog = append(p.undoLog[:0], log...)
	if len(log) == 0 {
		p.undoDir = ""
	}
}

func errorHandler(app vfs.App) ErrorHandler {
	return func(source, destination string, err error) ErrorAction {
		choice := app.Message(tr("VisRen.ErrorTitle", "Rename error"), fmt.Sprintf("%s → %s\n\n%v", source, destination, err), []string{
			tr("VisRen.Retry", "&Retry"), tr("VisRen.Skip", "&Skip"), tr("VisRen.SkipAll", "Skip &all"), tr("VisRen.Cancel", "Cancel"),
		})
		switch choice {
		case 0:
			return Retry
		case 2:
			return SkipAll
		case 3, -1:
			return Cancel
		default:
			return Skip
		}
	}
}
