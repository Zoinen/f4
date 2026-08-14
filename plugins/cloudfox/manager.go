package cloudfox

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// ManagerStrings keeps the plugin usable without depending on package main's
// localization catalog. The host may supply translated strings in Options.
type ManagerStrings struct {
	AddConnection  string
	Title          string
	ChooseType     string
	ConnectionName string
	ErrorTitle     string
}

func defaultManagerStrings() ManagerStrings {
	return ManagerStrings{
		AddConnection:  AddConnectionLabel,
		Title:          "Cloud storage connections",
		ChooseType:     "Storage type",
		ConnectionName: "Connection name",
		ErrorTitle:     "CloudFox",
	}
}

// ProfileEditor owns profile UI. It is injectable because OAuth and provider
// setting panes are provider-specific, while ManagerVFS only routes semantic
// panel actions.
type ProfileEditor interface {
	EditProfile(app vfs.App, manager *ManagerVFS, existing *Connection)
}

type ProfileEditorFunc func(vfs.App, *ManagerVFS, *Connection)

func (f ProfileEditorFunc) EditProfile(app vfs.App, manager *ManagerVFS, existing *Connection) {
	f(app, manager, existing)
}

// ManagerVFS is the top-level CloudFox drive. Saved connections are virtual
// directories mounted by connectionProvider; the add-connection row remains a
// semantic action rather than a directory.
type ManagerVFS struct {
	plugin  *Plugin
	repo    *Repository
	editor  ProfileEditor
	strings ManagerStrings

	mu   sync.RWMutex
	rows map[string]Connection // exact displayed name -> metadata clone
}

func NewManagerVFS(repo *Repository, editor ProfileEditor) *ManagerVFS {
	return newManagerVFS(nil, repo, editor, defaultManagerStrings())
}

func newManagerVFS(plugin *Plugin, repo *Repository, editor ProfileEditor, strings ManagerStrings) *ManagerVFS {
	return &ManagerVFS{plugin: plugin, repo: repo, editor: editor, strings: strings, rows: make(map[string]Connection)}
}

func managerVisualRoot() string { return DriveName + ":" + string(os.PathSeparator) }

func (m *ManagerVFS) IsAtRoot() bool           { return true }
func (m *ManagerVFS) GetPath() string          { return managerVisualRoot() }
func (m *ManagerVFS) GetTitle() string         { return DriveName }
func (m *ManagerVFS) PanelTitle(string) string { return m.strings.Title }
func (m *ManagerVFS) IsAbs(p string) bool {
	return strings.HasPrefix(strings.ToLower(p), ManagerRoot) || strings.HasPrefix(p, DriveName+":")
}

func (m *ManagerVFS) SetPath(p string) error {
	if p == "" || p == "." || p == "/" || strings.EqualFold(p, ManagerRoot) || p == managerVisualRoot() {
		return nil
	}
	return fmt.Errorf("cloudfox: manager has no directory %q: %w", p, os.ErrNotExist)
}

func (m *ManagerVFS) ReadDir(ctx context.Context, _ string, onChunk func([]vfs.VFSItem)) error {
	if m.repo == nil {
		return errors.New("cloudfox: repository is not configured")
	}
	connections, err := m.repo.List(ctx)
	if err != nil {
		return err
	}
	items := make([]vfs.VFSItem, 0, len(connections)+1)
	items = append(items, vfs.VFSItem{Name: m.strings.AddConnection, IsExecutable: true, NoExtension: true})
	rows := make(map[string]Connection, len(connections))
	for _, connection := range connections {
		rows[connection.Name] = connection.Clone()
		items = append(items, vfs.VFSItem{Name: connection.Name, IsDir: true, NoExtension: true})
	}
	sort.SliceStable(items[1:], func(i, j int) bool {
		return strings.ToLower(items[i+1].Name) < strings.ToLower(items[j+1].Name)
	})
	m.mu.Lock()
	m.rows = rows
	m.mu.Unlock()
	if onChunk != nil {
		onChunk(items)
	}
	return nil
}

func (m *ManagerVFS) connectionForPath(ctx context.Context, p string) (Connection, bool) {
	name := m.Base(p)
	if name == "" || name == "." || name == m.strings.AddConnection {
		return Connection{}, false
	}
	m.mu.RLock()
	connection, ok := m.rows[name]
	m.mu.RUnlock()
	if ok {
		return connection.Clone(), true
	}
	if m.repo == nil {
		return Connection{}, false
	}
	connections, err := m.repo.List(ctx)
	if err != nil {
		return Connection{}, false
	}
	for _, candidate := range connections {
		if candidate.Name == name {
			m.mu.Lock()
			m.rows[name] = candidate.Clone()
			m.mu.Unlock()
			return candidate.Clone(), true
		}
	}
	return Connection{}, false
}

func (m *ManagerVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	if p == "" || p == "." || p == "/" || strings.EqualFold(p, ManagerRoot) || p == managerVisualRoot() {
		return vfs.VFSItem{Name: DriveName, IsDir: true, NoExtension: true}, nil
	}
	if m.Base(p) == m.strings.AddConnection {
		return vfs.VFSItem{Name: m.strings.AddConnection, IsExecutable: true, NoExtension: true}, nil
	}
	connection, ok := m.connectionForPath(ctx, p)
	if !ok {
		return vfs.VFSItem{}, os.ErrNotExist
	}
	return vfs.VFSItem{Name: connection.Name, IsDir: true, NoExtension: true}, nil
}

func (m *ManagerVFS) Join(elem ...string) string {
	if len(elem) == 0 {
		return ""
	}
	parts := elem
	first := parts[0]
	if strings.HasPrefix(strings.ToLower(first), ManagerRoot) {
		first = strings.TrimPrefix(first, ManagerRoot)
	} else if strings.HasPrefix(first, DriveName+":") {
		first = strings.TrimLeft(first[len(DriveName)+1:], "/\\")
	}
	parts = append([]string{first}, parts[1:]...)
	joined := strings.Trim(strings.ReplaceAll(path.Join(parts...), "\\", "/"), "/")
	if joined == "" || joined == "." {
		return managerVisualRoot()
	}
	return managerVisualRoot() + strings.ReplaceAll(joined, "/", string(os.PathSeparator))
}

func (m *ManagerVFS) Abs(p string) (string, error) {
	if m.IsAbs(p) {
		return m.Join(p), nil
	}
	if p == "" || p == "." || p == "/" {
		return managerVisualRoot(), nil
	}
	return m.Join(managerVisualRoot(), p), nil
}

func (m *ManagerVFS) Base(p string) string {
	if strings.HasPrefix(strings.ToLower(p), ManagerRoot) {
		p = strings.TrimPrefix(p, ManagerRoot)
	} else if strings.HasPrefix(p, DriveName+":") {
		p = strings.TrimLeft(p[len(DriveName)+1:], "/\\")
	}
	p = strings.ReplaceAll(p, "\\", "/")
	return path.Base(p)
}

func (m *ManagerVFS) Dir(string) string { return managerVisualRoot() }

func (m *ManagerVFS) MkDir(context.Context, string) error { return ErrManagerReadOnly }

func (m *ManagerVFS) Remove(ctx context.Context, p string) error {
	if m.Base(p) == m.strings.AddConnection {
		return ErrManagerReadOnly
	}
	connection, ok := m.connectionForPath(ctx, p)
	if !ok {
		return ErrConnectionNotFound
	}
	if err := m.repo.Delete(ctx, connection.ID); err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.rows, connection.Name)
	m.mu.Unlock()
	return nil
}

func (m *ManagerVFS) Rename(ctx context.Context, oldpath, newpath string) error {
	connection, ok := m.connectionForPath(ctx, oldpath)
	if !ok {
		return ErrConnectionNotFound
	}
	newName := strings.TrimSpace(m.Base(newpath))
	if newName == m.strings.AddConnection {
		return ErrReservedName
	}
	oldName := connection.Name
	connection.Name = newName
	updated, err := m.repo.Save(ctx, connection, nil, "")
	if err != nil {
		return err
	}
	m.mu.Lock()
	delete(m.rows, oldName)
	m.rows[updated.Name] = updated
	m.mu.Unlock()
	return nil
}

func (m *ManagerVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return ErrManagerReadOnly
}

func (m *ManagerVFS) GetCapabilities() vfs.VFSCapabilities { return vfs.VFSCapabilities{} }
func (m *ManagerVFS) Search(context.Context, string, string) (chan int64, error) {
	return nil, ErrManagerReadOnly
}
func (m *ManagerVFS) Open(context.Context, string) (vfs.ReadAtCloser, error) {
	return nil, ErrManagerReadOnly
}
func (m *ManagerVFS) Create(context.Context, string) (io.WriteCloser, error) {
	return nil, ErrManagerReadOnly
}
func (m *ManagerVFS) ParentVFS() vfs.VFS { return nil }
func (m *ManagerVFS) Clone() vfs.VFS {
	return newManagerVFS(m.plugin, m.repo, m.editor, m.strings)
}
func (m *ManagerVFS) Close() error {
	m.mu.Lock()
	m.rows = make(map[string]Connection)
	m.mu.Unlock()
	return nil
}

func (m *ManagerVFS) HandlePanelAction(app vfs.App, action vfs.PanelAction, paths []string) bool {
	switch action {
	case vfs.PanelActionCreate:
		if m.editor == nil {
			return false
		}
		m.editor.EditProfile(app, m, nil)
		return true
	case vfs.PanelActionActivate:
		if m.editor != nil && len(paths) != 0 && m.Base(paths[0]) == m.strings.AddConnection {
			m.editor.EditProfile(app, m, nil)
			return true
		}
	case vfs.PanelActionEdit:
		if m.editor == nil {
			return false
		}
		if len(paths) != 1 {
			return false
		}
		connection, ok := m.connectionForPath(context.Background(), paths[0])
		if !ok {
			return m.Base(paths[0]) == m.strings.AddConnection
		}
		m.editor.EditProfile(app, m, &connection)
		return true
	case vfs.PanelActionDelete:
		connections := make([]Connection, 0, len(paths))
		for _, p := range paths {
			if m.Base(p) == m.strings.AddConnection {
				continue
			}
			if connection, ok := m.connectionForPath(context.Background(), p); ok {
				connections = append(connections, connection)
			}
		}
		if len(connections) == 0 {
			return true
		}
		if vtui.FrameManager == nil {
			return false
		}
		message := fmt.Sprintf("Delete connection %q?", connections[0].Name)
		if len(connections) > 1 {
			message = fmt.Sprintf("Delete %d cloud storage connections?", len(connections))
		}
		dialog := vtui.ShowMessage(" CloudFox ", message, []string{"&Delete", "Cancel"})
		dialog.OnResult = func(code int) {
			if code != 0 {
				return
			}
			vtui.RunAsync(func(task *vtui.TaskContext) {
				var deleteErrors []error
				for _, connection := range connections {
					if err := m.repo.Delete(task, connection.ID); err != nil {
						deleteErrors = append(deleteErrors, fmt.Errorf("%s: %w", connection.Name, err))
					}
				}
				err := errors.Join(deleteErrors...)
				task.RunOnUI(func() {
					if err != nil {
						vtui.ShowMessage(" CloudFox ", "Could not delete one or more connections:\n"+err.Error(), []string{"&OK"})
					}
					app.RefreshAll()
				})
			})
		}
		return true
	}
	return false
}

var (
	_ vfs.VFS                = (*ManagerVFS)(nil)
	_ vfs.TitleProvider      = (*ManagerVFS)(nil)
	_ vfs.PanelTitleProvider = (*ManagerVFS)(nil)
	_ vfs.PanelActionHandler = (*ManagerVFS)(nil)
)
