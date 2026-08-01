package vfs

import (
	"context"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"io"
	"os"
	"sync"
	"time"
)

var CustomConfigDir string

// App defines the interface for plugin-to-core UI interactions.
type App interface {
	GetActivePanelVFS() VFS
	GetPassivePanelVFS() VFS
	GetSelectedNames() []string
	GetSelectedName() string
	RefreshAll()
	SetPendingSelection(name string)
	RunProgressTask(title, startMsg string, forked bool, worker func(ctx context.Context, update func(msg string, percent int)) error, onComplete func(err error))
	RunAdvancedProgressTask(title string, forked bool, worker func(ctx context.Context, reporter TaskReporter) error, onComplete func(err error))
	// UI Bridge
	Message(title, msg string, buttons []string) int
	InputBox(title, prompt, history string, callback func(string))
	Menu(title string, items []string, callback func(int))
}

// HostAPI defines the functions f4 exposes to plugins.
type HostAPI interface {
	GetVersion() string
	Log(msg string)
	Message(msg string)

	RegisterHighlighter(p vtui.HighlighterProvider)
	RegisterVFSProvider(p VFSProvider)
	RegisterDrive(name string, factory func() VFS)
	RegisterGlobalHotkey(vk uint16, mods vtinput.ControlKeyState, handler func(app App))
	RegisterPluginMenuItem(label string, handler func(app App))
}

// VFSItem represents a generic file or directory entry.
type VFSItem struct {
	Name         string
	Size         int64
	IsDir        bool
	MTime        time.Time
	Mode         string
	IsExecutable bool
	IsHidden     bool
	// Metadata for Attributes dialog
	ATime    time.Time // Last Access
	CTime    time.Time // Creation (Win) or Status Change (Unix)
	UnixMode uint32    // Raw numeric mode for chmod
	Uid, Gid int       // Ownership
	WinAttrs uint32    // Windows file attributes
}

// VFSCapabilities defines what the current VFS implementation can do efficiently.
type VFSCapabilities struct {
	HasServerSideCopy  bool
	HasServerSideMove  bool
	HasRandomAccess    bool // Supports ReadAt
	HasSearch          bool // Supports server-side search
	HasUnixPermissions bool // Indicates if VFS natively supports Unix-style permissions
}

// VFS is the core interface for file operations in f4.
type VFS interface {
	IsAtRoot() bool
	GetPath() string
	IsAbs(path string) bool
	SetPath(path string) error
	ReadDir(ctx context.Context, path string, onChunk func([]VFSItem)) error
	Stat(ctx context.Context, path string) (VFSItem, error)
	Join(elem ...string) string
	Abs(path string) (string, error)
	Base(path string) string
	Dir(path string) string

	// Mutations
	MkDir(ctx context.Context, path string) error
	Remove(ctx context.Context, path string) error
	Rename(ctx context.Context, oldpath, newpath string) error

	// Advanced / Remote Operations
	GetCapabilities() VFSCapabilities
	Search(ctx context.Context, path string, pattern string) (chan int64, error)

	// Random Access (required for high-performance Viewer/Editor)
	// Open returns a ReadAtCloser for the file.
	Open(ctx context.Context, path string) (ReadAtCloser, error)

	// Create returns a WriteCloser for new files.
	Create(ctx context.Context, path string) (io.WriteCloser, error)

	// SetAttributes updates file metadata (mode, ownership, times)
	SetAttributes(ctx context.Context, path string, item VFSItem) error

	ParentVFS() VFS // Returns the underlying VFS if this is a virtual mount, or nil

	Clone() VFS
	Close() error
}

// TitleProvider allows a VFS to provide a custom display prefix (e.g. "user@host" for network drives).
type TitleProvider interface {
	GetTitle() string
}
type BulkCopier interface {
	CopyBulk(ctx context.Context, srcPaths []string, dstVfs VFS, dstDir string, reporter TaskReporter) error
}
type ArchiveLockManager struct {
	mu    sync.Mutex
	conds map[string]*sync.Cond
	busy  map[string]bool
}

var GlobalArchiveLockManager = &ArchiveLockManager{
	conds: make(map[string]*sync.Cond),
	busy:  make(map[string]bool),
}

func (m *ArchiveLockManager) Lock(path string) {
	m.mu.Lock()
	for m.busy[path] {
		cond, ok := m.conds[path]
		if !ok {
			cond = sync.NewCond(&m.mu)
			m.conds[path] = cond
		}
		cond.Wait()
	}
	m.busy[path] = true
	m.mu.Unlock()
}

func (m *ArchiveLockManager) TryLock(path string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.busy[path] {
		return false
	}
	m.busy[path] = true
	return true
}

func (m *ArchiveLockManager) Unlock(path string) {
	m.mu.Lock()
	m.busy[path] = false
	if cond, ok := m.conds[path]; ok {
		cond.Broadcast()
	}
	m.mu.Unlock()
}

// PtyProvider allows a VFS to provide its own PTY implementation
// (e.g. an SSH session for remote systems).
type PtyProvider interface {
	OpenPty(cols, rows int) (any, error)
}

// VFSProvider умеет определять, может ли он открыть путь, и создавать экземпляр VFS.
type VFSProvider interface {
	Name() string
	// Priority: чем выше, тем раньше провайдер опрашивается (архивы обычно имеют низкий приоритет)
	Priority() int
	// CanOpen возвращает true, если провайдер понимает этот путь.
	// parent — текущая VFS, в которой находится объект.
	CanOpen(ctx context.Context, parent VFS, path string) bool
	// Open создает новый экземпляр VFS.
	Open(ctx context.Context, parent VFS, path string) (VFS, error)
}

var providers []VFSProvider

func RegisterProvider(p VFSProvider) {
	providers = append(providers, p)
	// Сортируем по приоритету
}

func FindProvider(ctx context.Context, parent VFS, path string) VFSProvider {
	for _, p := range providers {
		if p.CanOpen(ctx, parent, path) {
			return p
		}
	}
	return nil
}

// ReadAtCloser combines reader interfaces with context support.
type ReadAtCloser interface {
	ReadAt(ctx context.Context, p []byte, off int64) (n int, err error)
	Read(ctx context.Context, p []byte) (n int, err error)
	io.Closer
	Size() int64
} // TempFileWrapper is a helper for VFS that need to extract files to temp storage.
type TempFileWrapper struct {
	*os.File
	SizeVal  int64
	TempPath string
}

func (w *TempFileWrapper) Size() int64 { return w.SizeVal }
func (w *TempFileWrapper) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	return w.File.ReadAt(p, off)
}
func (w *TempFileWrapper) Read(ctx context.Context, p []byte) (int, error) {
	return w.File.Read(p)
}

func (w *TempFileWrapper) Close() error {
	err := w.File.Close()
	os.Remove(w.TempPath)
	return err
}

type progressKeyType struct{}
type reporterKeyType struct{}

var ProgressKey = progressKeyType{}
var ReporterKey = reporterKeyType{}

type ProgressCallback func(msg string, percent int)

type TaskReporter interface {
	UpdateScan(currentPath string, files, dirs int64)
	UpdateTransfer(action string, filename string, currentPct int, totalText string, totalPct int, speedText string)
	IsCancelled() bool
}

type FileProgress interface {
	StartFile(name string, size int64)
	UpdateBytes(n int)
	FileDone()
	DirDone()
	FileSkipped()
}
