package iosfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
)

// CoreVFS is a read-only view of a CoreDevice FileService domain. Apple does
// not expose a stable random-access handle here, so Open materializes once to
// a private temporary file and then serves all viewer seeks locally.
type CoreVFS struct {
	parent     vfs.VFS
	device     DeviceInfo
	domain     coreDomain
	identifier string
	title      string
	panelInfo  vfs.PanelInfoProvider
	mount      *coreMount
	path       string
	mu         sync.RWMutex
	closeOnce  sync.Once
	closeErr   error
}

type coreSessionKey struct{ marker byte }

type coreDirCache struct {
	entries []coreEntry
	at      time.Time
}

type coreMount struct {
	mu         sync.RWMutex
	service    coreFileService
	access     coreAccess
	device     DeviceInfo
	domain     coreDomain
	identifier string
	refs       int
	closed     bool
	key        *coreSessionKey
	cache      map[string]coreDirCache
}

func newCoreVFS(parent vfs.VFS, device DeviceInfo, domain coreDomain, identifier, title string, service coreFileService, access ...coreAccess) *CoreVFS {
	var opener coreAccess
	if len(access) != 0 {
		opener = access[0]
	}
	mount := &coreMount{
		service: service, access: opener, device: device, domain: domain, identifier: identifier,
		refs: 1, key: &coreSessionKey{marker: 1}, cache: make(map[string]coreDirCache),
	}
	return &CoreVFS{
		parent: parent, device: device, domain: domain, identifier: identifier,
		title: title, panelInfo: newDevicePanelInfoProvider(device, "CoreDevice"), mount: mount, path: "/",
	}
}

func (m *coreMount) current() (coreFileService, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.closed || m.service == nil {
		return nil, ErrCoreDeviceConnection
	}
	return m.service, nil
}

func (m *coreMount) retain() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return false
	}
	m.refs++
	return true
}

func (m *coreMount) release() error {
	m.mu.Lock()
	if m.refs > 0 {
		m.refs--
	}
	if m.refs != 0 || m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	service := m.service
	m.service = nil
	m.cache = nil
	m.mu.Unlock()
	if service != nil {
		return service.Close()
	}
	return nil
}

func (m *coreMount) reconnect(ctx context.Context) error {
	m.mu.RLock()
	access := m.access
	device, domain, identifier := m.device, m.domain, m.identifier
	closed := m.closed
	m.mu.RUnlock()
	if closed || access == nil {
		return ErrCoreDeviceUnavailable
	}
	service, err := access.Open(ctx, device, domain, identifier)
	if err != nil {
		return err
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		_ = service.Close()
		return ErrCoreDeviceUnavailable
	}
	previous := m.service
	m.service = service
	m.cache = make(map[string]coreDirCache)
	m.mu.Unlock()
	if previous != nil {
		_ = previous.Close()
	}
	return nil
}

const coreMetadataCacheTTL = 2 * time.Second

func (m *coreMount) cached(directory string) ([]coreEntry, bool) {
	m.mu.RLock()
	cached, ok := m.cache[directory]
	m.mu.RUnlock()
	if !ok || time.Since(cached.at) >= coreMetadataCacheTTL {
		return nil, false
	}
	return append([]coreEntry(nil), cached.entries...), true
}

func (m *coreMount) remember(directory string, entries []coreEntry) {
	m.mu.Lock()
	if !m.closed {
		m.cache[directory] = coreDirCache{entries: append([]coreEntry(nil), entries...), at: time.Now()}
	}
	m.mu.Unlock()
}

func (v *CoreVFS) IsAtRoot() bool { return v.GetPath() == "/" }
func (v *CoreVFS) GetPath() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.path
}
func (v *CoreVFS) IsAbs(p string) bool { return strings.HasPrefix(p, "/") }
func (v *CoreVFS) SetPath(p string) error {
	clean, err := cleanIOSPath(p)
	if err != nil {
		return err
	}
	item, err := v.Stat(context.Background(), clean)
	if err != nil {
		return err
	}
	if !item.IsDir {
		return fmt.Errorf("ios: %q is not a directory", clean)
	}
	v.mu.Lock()
	v.path = clean
	v.mu.Unlock()
	return nil
}
func (v *CoreVFS) SetPathOptimistic(p string) error {
	clean, err := cleanIOSPath(p)
	if err != nil {
		return err
	}
	v.mu.Lock()
	v.path = clean
	v.mu.Unlock()
	return nil
}
func (*CoreVFS) Join(elem ...string) string { return path.Join(elem...) }
func (v *CoreVFS) Abs(p string) (string, error) {
	if v.IsAbs(p) {
		return cleanIOSPath(p)
	}
	return cleanIOSPath(path.Join(v.GetPath(), p))
}
func (*CoreVFS) Base(p string) string { return path.Base(p) }
func (*CoreVFS) Dir(p string) string  { return path.Dir(p) }

func (v *CoreVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	clean, err := v.resolve(p)
	if err != nil {
		return err
	}
	service, err := v.mount.current()
	if err != nil {
		return err
	}
	entries, err := service.List(ctx, clean)
	if err != nil {
		return mapRemoteError(err)
	}
	safeEntries := make([]coreEntry, 0, len(entries))
	for _, entry := range entries {
		if unsafeRemoteName(entry.Name) {
			continue
		}
		safeEntries = append(safeEntries, entry)
	}
	v.mount.remember(clean, safeEntries)
	items := make([]vfs.VFSItem, 0, len(safeEntries))
	for _, entry := range safeEntries {
		items = append(items, coreVFSItem(entry))
	}
	if onChunk != nil {
		const chunkSize = 128
		for len(items) > 0 {
			count := min(len(items), chunkSize)
			onChunk(items[:count])
			items = items[count:]
		}
	}
	return nil
}

func (v *CoreVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	clean, err := v.resolve(p)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	if clean == "/" {
		return vfs.VFSItem{Name: "/", IsDir: true, Mode: "dr-xr-xr-x"}, nil
	}
	parent := path.Dir(clean)
	entries, ok := v.mount.cached(parent)
	if !ok {
		service, serviceErr := v.mount.current()
		if serviceErr != nil {
			return vfs.VFSItem{}, serviceErr
		}
		entries, err = service.List(ctx, parent)
		if err != nil {
			return vfs.VFSItem{}, mapRemoteError(err)
		}
		v.mount.remember(parent, entries)
	}
	base := path.Base(clean)
	for _, entry := range entries {
		if entry.Name == base {
			return coreVFSItem(entry), nil
		}
	}
	return vfs.VFSItem{}, os.ErrNotExist
}

func coreVFSItem(entry coreEntry) vfs.VFSItem {
	mode := "-r--r--r--"
	if entry.IsDir {
		mode = "dr-xr-xr-x"
	}
	var mtime time.Time
	if entry.ModUnix != 0 {
		mtime = time.Unix(entry.ModUnix, 0)
	}
	return vfs.VFSItem{
		Name: entry.Name, Size: entry.Size, IsDir: entry.IsDir, MTime: mtime,
		Mode: mode, IsHidden: entry.Hidden || strings.HasPrefix(entry.Name, "."),
		IsSymlink: entry.IsLink, UnixMode: entry.Mode,
	}
}

func (v *CoreVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	clean, err := v.resolve(p)
	if err != nil {
		return nil, err
	}
	temp, err := os.CreateTemp("", "f4-ios-core-*")
	if err != nil {
		return nil, err
	}
	remove := func() {
		name := temp.Name()
		_ = temp.Close()
		_ = os.Remove(name)
	}
	service, err := v.mount.current()
	if err != nil {
		remove()
		return nil, err
	}
	if err := service.Pull(ctx, clean, contextWriter{ctx: ctx, writer: temp}); err != nil {
		remove()
		return nil, mapRemoteError(err)
	}
	info, err := temp.Stat()
	if err != nil {
		remove()
		return nil, err
	}
	if _, err := temp.Seek(0, io.SeekStart); err != nil {
		remove()
		return nil, err
	}
	return &coreTempFile{file: temp, path: temp.Name(), size: info.Size()}, nil
}

// contextWriter makes cancellation observable while FileService is streaming
// a large file. The upstream API has no context parameter of its own, but it
// stops the transfer as soon as the destination rejects the next chunk.
type contextWriter struct {
	ctx    context.Context
	writer io.Writer
}

func (w contextWriter) Write(p []byte) (int, error) {
	if err := w.ctx.Err(); err != nil {
		return 0, err
	}
	return w.writer.Write(p)
}

type coreTempFile struct {
	file      *os.File
	path      string
	size      int64
	closeOnce sync.Once
	closeErr  error
}

func (f *coreTempFile) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return f.file.ReadAt(p, off)
}
func (f *coreTempFile) Read(ctx context.Context, p []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return f.file.Read(p)
}
func (f *coreTempFile) Size() int64 { return f.size }
func (f *coreTempFile) Close() error {
	f.closeOnce.Do(func() {
		f.closeErr = errors.Join(f.file.Close(), os.Remove(f.path))
	})
	return f.closeErr
}

func (*CoreVFS) MkDir(context.Context, string) error          { return ErrReadOnlyDomain }
func (*CoreVFS) Remove(context.Context, string) error         { return ErrReadOnlyDomain }
func (*CoreVFS) Rename(context.Context, string, string) error { return ErrReadOnlyDomain }
func (*CoreVFS) Create(context.Context, string) (io.WriteCloser, error) {
	return nil, ErrReadOnlyDomain
}
func (*CoreVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return ErrReadOnlyDomain
}
func (*CoreVFS) Search(context.Context, string, string) (chan int64, error) {
	return nil, ErrReadOnlyDomain
}
func (*CoreVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true}
}
func (v *CoreVFS) ParentVFS() vfs.VFS { return v.parent }
func (v *CoreVFS) Clone() vfs.VFS {
	if !v.mount.retain() {
		return vfs.NewNullVFS(0)
	}
	return &CoreVFS{
		parent: v.parent, device: v.device, domain: v.domain,
		identifier: v.identifier, title: v.title, panelInfo: v.panelInfo, mount: v.mount, path: v.GetPath(),
	}
}
func (v *CoreVFS) Close() error {
	v.closeOnce.Do(func() { v.closeErr = v.mount.release() })
	return v.closeErr
}
func (v *CoreVFS) GetTitle() string {
	return fmt.Sprintf("ios:%s:core:%d:%s", v.device.UDID, v.domain, v.identifier)
}
func (v *CoreVFS) PanelTitle(p string) string { return iosPanelTitle(v.title, p) }
func (v *CoreVFS) PanelInfoKey(req vfs.PanelInfoRequest) string {
	return v.panelInfo.PanelInfoKey(req)
}
func (v *CoreVFS) CachedPanelInfo(req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, bool) {
	return v.panelInfo.CachedPanelInfo(req)
}
func (v *CoreVFS) RefreshPanelInfo(ctx context.Context, req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
	return v.panelInfo.RefreshPanelInfo(ctx, req)
}
func (v *CoreVFS) SessionKey() any            { return v.mount.key }
func (v *CoreVFS) SessionLost(err error) bool { return errors.Is(err, ErrCoreDeviceConnection) }
func (v *CoreVFS) CanReconnect() bool {
	v.mount.mu.RLock()
	defer v.mount.mu.RUnlock()
	return !v.mount.closed && v.mount.access != nil
}
func (v *CoreVFS) Reconnect(ctx context.Context) error { return v.mount.reconnect(ctx) }

func (v *CoreVFS) resolve(p string) (string, error) {
	if p == "" || p == "." {
		return v.GetPath(), nil
	}
	return v.Abs(p)
}

func cleanIOSPath(p string) (string, error) {
	if strings.IndexByte(p, 0) >= 0 {
		return "", fmt.Errorf("ios: path contains NUL")
	}
	if p == "" || p == "." {
		return "/", nil
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	for _, element := range strings.Split(p, "/") {
		if element == ".." {
			return "", fmt.Errorf("ios: path escapes domain root")
		}
	}
	return path.Clean(p), nil
}

func unsafeRemoteName(name string) bool {
	if name == "" || name == "." || name == ".." || strings.IndexByte(name, 0) >= 0 || strings.Contains(name, "/") {
		return true
	}
	return path.IsAbs(name)
}

func mapRemoteError(err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "not found"), strings.Contains(message, "no such"):
		return fmt.Errorf("%w: %v", os.ErrNotExist, err)
	case strings.Contains(message, "permission"), strings.Contains(message, "denied"), strings.Contains(message, "sandbox"):
		return fmt.Errorf("%w: %v", os.ErrPermission, err)
	default:
		return err
	}
}

func iosDeviceTitle(device DeviceInfo, components ...string) string {
	title := deviceLabel(device) + ":/"
	clean := make([]string, 0, len(components))
	for _, component := range components {
		component = strings.Trim(component, "/")
		if component != "" && component != "." {
			clean = append(clean, component)
		}
	}
	return title + strings.Join(clean, "/")
}

func iosPanelTitle(title, canonicalPath string) string {
	displayPath := strings.Trim(path.Clean(canonicalPath), "/")
	title = strings.TrimSuffix(strings.TrimSpace(title), "/")
	if displayPath == "" || displayPath == "." {
		return title + "/"
	}
	return title + "/" + displayPath
}

var _ vfs.OptimisticPathSetter = (*CoreVFS)(nil)
var _ vfs.PanelTitleProvider = (*CoreVFS)(nil)
var _ vfs.PanelInfoProvider = (*CoreVFS)(nil)
var _ vfs.SessionIdentity = (*CoreVFS)(nil)
var _ vfs.SessionReconnector = (*CoreVFS)(nil)
