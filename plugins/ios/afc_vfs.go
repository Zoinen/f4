package iosfs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"path"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unxed/f4/plugins/ios/internal/afcproto"
	"github.com/unxed/f4/vfs"
)

const (
	afcTransferConnections = 2
	afcReadDirChunk        = 128
	afcIdleTTL             = 60 * time.Second
	afcMaxIdleSessions     = 4
	afcHandleCloseTimeout  = 10 * time.Second
	afcHandleWriteTimeout  = 30 * time.Second
)

var ErrSearchUnsupported = errors.New("ios: server-side search is unavailable")

type afcDialer func(context.Context) (io.ReadWriteCloser, error)

type afcSession struct {
	key    string
	dial   afcDialer
	metaMu sync.Mutex
	meta   *afcproto.Client

	transferMu    sync.Mutex
	transferIdle  chan *afcproto.Client
	transferWake  chan struct{}
	transferAll   map[*afcproto.Client]struct{}
	transferCount int
	transferEpoch uint64
	done          chan struct{}
	closed        atomic.Bool

	refMu    sync.Mutex
	refs     int
	idleAt   time.Time
	infoMu   sync.RWMutex
	info     vfs.PanelInfoSnapshot
	infoTime time.Time
}

func newAFCSession(key string, dial afcDialer) *afcSession {
	return &afcSession{
		key: key, dial: dial, transferIdle: make(chan *afcproto.Client, afcTransferConnections),
		transferWake: make(chan struct{}, afcTransferConnections),
		transferAll:  make(map[*afcproto.Client]struct{}), done: make(chan struct{}), refs: 1,
	}
}

func (s *afcSession) retain() bool {
	s.refMu.Lock()
	defer s.refMu.Unlock()
	if s.closed.Load() {
		return false
	}
	s.refs++
	s.idleAt = time.Time{}
	return true
}

func (s *afcSession) release() {
	s.refMu.Lock()
	if s.refs > 0 {
		s.refs--
	}
	if s.refs == 0 {
		s.idleAt = time.Now()
	}
	s.refMu.Unlock()
}

func (s *afcSession) idleState() (int, time.Time) {
	s.refMu.Lock()
	defer s.refMu.Unlock()
	return s.refs, s.idleAt
}

func (s *afcSession) metadata(ctx context.Context) (*afcproto.Client, error) {
	s.metaMu.Lock()
	defer s.metaMu.Unlock()
	if s.closed.Load() {
		return nil, afcproto.ErrClosed
	}
	if s.meta != nil {
		return s.meta, nil
	}
	conn, err := s.dial(ctx)
	if err != nil {
		return nil, err
	}
	if s.closed.Load() {
		_ = conn.Close()
		return nil, afcproto.ErrClosed
	}
	s.meta = afcproto.New(conn)
	return s.meta, nil
}

func (s *afcSession) markMetadataError(client *afcproto.Client, err error) {
	if !afcproto.IsConnectionLost(err) {
		return
	}
	s.metaMu.Lock()
	if s.meta == client {
		s.meta = nil
	}
	s.metaMu.Unlock()
	_ = client.Close()
}

func (s *afcSession) leaseTransfer(ctx context.Context) (*afcproto.Client, error) {
	for {
		client, acquired, err := s.tryLeaseTransfer(ctx)
		if err != nil || acquired {
			return client, err
		}
		select {
		case <-s.transferWake:
			// A client was returned or a lost/dial-failed client released a
			// capacity slot. Retry under transferMu instead of waiting forever.
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.done:
			return nil, afcproto.ErrClosed
		}
	}
}

func (s *afcSession) tryLeaseTransfer(ctx context.Context) (*afcproto.Client, bool, error) {
	s.transferMu.Lock()
	if s.closed.Load() {
		s.transferMu.Unlock()
		return nil, false, afcproto.ErrClosed
	}
	select {
	case client := <-s.transferIdle:
		if _, known := s.transferAll[client]; known {
			s.transferMu.Unlock()
			return client, true, nil
		}
		// reset may have invalidated a queued client. It cannot be leased,
		// but it also must not consume a capacity slot or poison the pool.
		s.transferMu.Unlock()
		_ = client.Close()
		return s.tryLeaseTransfer(ctx)
	default:
	}
	if s.transferCount < afcTransferConnections {
		s.transferCount++
		epoch := s.transferEpoch
		s.transferMu.Unlock()
		conn, err := s.dial(ctx)
		if err != nil {
			s.transferMu.Lock()
			if s.transferEpoch == epoch {
				s.transferCount--
			}
			s.transferMu.Unlock()
			s.signalTransfer()
			return nil, false, err
		}
		client := afcproto.New(conn)
		s.transferMu.Lock()
		if s.closed.Load() || s.transferEpoch != epoch {
			if s.transferEpoch == epoch {
				s.transferCount--
			}
			s.transferMu.Unlock()
			_ = client.Close()
			s.signalTransfer()
			if s.closed.Load() {
				return nil, false, afcproto.ErrClosed
			}
			return s.tryLeaseTransfer(ctx)
		}
		s.transferAll[client] = struct{}{}
		s.transferMu.Unlock()
		return client, true, nil
	}
	s.transferMu.Unlock()
	return nil, false, nil
}

func (s *afcSession) returnTransfer(client *afcproto.Client, lost bool) {
	if client == nil {
		return
	}
	s.transferMu.Lock()
	_, known := s.transferAll[client]
	if !known {
		s.transferMu.Unlock()
		_ = client.Close()
		return
	}
	if lost || s.closed.Load() {
		delete(s.transferAll, client)
		s.transferCount--
		s.transferMu.Unlock()
		_ = client.Close()
		s.signalTransfer()
		return
	}
	select {
	case s.transferIdle <- client:
		s.transferMu.Unlock()
		s.signalTransfer()
	default:
		// Every client is leased exclusively, so a full idle queue means a
		// duplicate return. Discard it rather than blocking while holding the
		// session's capacity forever.
		delete(s.transferAll, client)
		s.transferCount--
		s.transferMu.Unlock()
		_ = client.Close()
		s.signalTransfer()
	}
}

func (s *afcSession) signalTransfer() {
	select {
	case s.transferWake <- struct{}{}:
	default:
	}
}

func (s *afcSession) reset(ctx context.Context) error {
	s.metaMu.Lock()
	meta := s.meta
	s.meta = nil
	s.metaMu.Unlock()
	if meta != nil {
		_ = meta.Close()
	}

	s.transferMu.Lock()
	clients := make([]*afcproto.Client, 0, len(s.transferAll))
	for client := range s.transferAll {
		clients = append(clients, client)
	}
	s.transferAll = make(map[*afcproto.Client]struct{})
	s.transferCount = 0
	s.transferEpoch++
	for len(s.transferIdle) > 0 {
		<-s.transferIdle
	}
	s.transferMu.Unlock()
	for range afcTransferConnections {
		s.signalTransfer()
	}
	for _, client := range clients {
		_ = client.Close()
	}
	client, err := s.metadata(ctx)
	if err != nil {
		return err
	}
	_, err = client.Stat(ctx, "/")
	if err != nil {
		s.markMetadataError(client, err)
	}
	return err
}

func (s *afcSession) Close() error {
	if !s.closed.CompareAndSwap(false, true) {
		return nil
	}
	s.transferMu.Lock()
	close(s.done)
	clients := make([]*afcproto.Client, 0, len(s.transferAll))
	for client := range s.transferAll {
		clients = append(clients, client)
	}
	s.transferAll = nil
	s.transferCount = 0
	s.transferEpoch++
	s.transferMu.Unlock()

	s.metaMu.Lock()
	meta := s.meta
	s.meta = nil
	s.metaMu.Unlock()
	var result error
	if meta != nil {
		result = errors.Join(result, meta.Close())
	}
	for _, client := range clients {
		result = errors.Join(result, client.Close())
	}
	return result
}

type afcRegistry struct {
	mu       sync.Mutex
	sessions map[string]*afcSession
	closed   bool
}

func newAFCRegistry() *afcRegistry { return &afcRegistry{sessions: make(map[string]*afcSession)} }

func (r *afcRegistry) acquire(ctx context.Context, key string, dial afcDialer) (*afcSession, bool, error) {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, false, afcproto.ErrClosed
	}
	stale := r.pruneLocked(time.Now())
	if session := r.sessions[key]; session != nil && session.retain() {
		r.mu.Unlock()
		for _, old := range stale {
			_ = old.Close()
		}
		return session, false, nil
	}
	session := newAFCSession(key, dial)
	r.sessions[key] = session
	r.mu.Unlock()
	for _, old := range stale {
		_ = old.Close()
	}
	return session, true, nil
}

func (r *afcRegistry) forget(key string, session *afcSession) {
	r.mu.Lock()
	if r.sessions[key] == session {
		delete(r.sessions, key)
	}
	r.mu.Unlock()
	_ = session.Close()
}

func (r *afcRegistry) pruneLocked(now time.Time) []*afcSession {
	type idleSession struct {
		key     string
		session *afcSession
		idleAt  time.Time
	}
	idle := make([]idleSession, 0)
	remove := make([]*afcSession, 0)
	for key, session := range r.sessions {
		refs, idleAt := session.idleState()
		if refs != 0 || idleAt.IsZero() {
			continue
		}
		if now.Sub(idleAt) >= afcIdleTTL {
			delete(r.sessions, key)
			remove = append(remove, session)
			continue
		}
		idle = append(idle, idleSession{key: key, session: session, idleAt: idleAt})
	}
	if len(idle) > afcMaxIdleSessions {
		sort.Slice(idle, func(i, j int) bool { return idle[i].idleAt.Before(idle[j].idleAt) })
		for _, candidate := range idle[:len(idle)-afcMaxIdleSessions] {
			delete(r.sessions, candidate.key)
			remove = append(remove, candidate.session)
		}
	}
	return remove
}

func (r *afcRegistry) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	sessions := make([]*afcSession, 0, len(r.sessions))
	for _, session := range r.sessions {
		sessions = append(sessions, session)
	}
	r.sessions = nil
	r.mu.Unlock()
	var result error
	for _, session := range sessions {
		result = errors.Join(result, session.Close())
	}
	return result
}

// AFCVFS exposes the complete writable AFC contract for Media and permitted
// House Arrest containers. Crash reports use the same reader with mutations
// disabled at this layer.
type AFCVFS struct {
	parent           vfs.VFS
	device           DeviceInfo
	registry         *afcRegistry
	session          *afcSession
	key              string
	title            string
	readOnly         bool
	rootOpener       SelectorOpener
	rootCapabilities map[string]Capability
	pathMu           sync.RWMutex
	path             string
	closeOnce        sync.Once
}

func (v *AFCVFS) SetVirtualRoot(opener SelectorOpener, capabilities ...Capability) {
	v.rootOpener = opener
	v.rootCapabilities = make(map[string]Capability, len(capabilities))
	for _, capability := range capabilities {
		switch capability {
		case CapabilityApplications:
			v.rootCapabilities[ApplicationsSelector] = capability
		case CapabilityCrashReports:
			v.rootCapabilities[CrashReportsSelector] = capability
		}
	}
}

func (v *AFCVFS) rootCapabilityForPath(p string) (Capability, bool) {
	clean, err := cleanIOSPath(p)
	if err != nil || path.Dir(clean) != "/" {
		return 0, false
	}
	capability, ok := v.rootCapabilities[path.Base(clean)]
	return capability, ok
}

func (v *AFCVFS) virtualRootItems() []vfs.VFSItem {
	items := make([]vfs.VFSItem, 0, len(v.rootCapabilities))
	for _, name := range []string{ApplicationsSelector, CrashReportsSelector} {
		if _, ok := v.rootCapabilities[name]; ok {
			items = append(items, vfs.VFSItem{Name: name, IsDir: true, IsExecutable: true, NoExtension: true})
		}
	}
	return items
}

func openAFCVFS(ctx context.Context, parent vfs.VFS, device DeviceInfo, registry *afcRegistry, key, title string, readOnly bool, dial afcDialer) (*AFCVFS, error) {
	session, created, err := registry.acquire(ctx, key, dial)
	if err != nil {
		return nil, err
	}
	client, err := session.metadata(ctx)
	if err == nil {
		_, err = client.Stat(ctx, "/")
		session.markMetadataError(client, err)
	}
	if err != nil {
		session.release()
		if created {
			registry.forget(key, session)
		}
		return nil, fmt.Errorf("ios: probe AFC root: %w", err)
	}
	return &AFCVFS{parent: parent, device: device, registry: registry, session: session, key: key, title: title, readOnly: readOnly, path: "/"}, nil
}

func (v *AFCVFS) IsAtRoot() bool { return v.GetPath() == "/" }
func (v *AFCVFS) GetPath() string {
	v.pathMu.RLock()
	defer v.pathMu.RUnlock()
	return v.path
}
func (v *AFCVFS) IsAbs(p string) bool { return strings.HasPrefix(p, "/") }
func (v *AFCVFS) SetPath(p string) error {
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
	return v.SetPathOptimistic(clean)
}
func (v *AFCVFS) SetPathOptimistic(p string) error {
	clean, err := cleanIOSPath(p)
	if err != nil {
		return err
	}
	v.pathMu.Lock()
	v.path = clean
	v.pathMu.Unlock()
	return nil
}
func (*AFCVFS) Join(elem ...string) string { return path.Join(elem...) }
func (v *AFCVFS) Abs(p string) (string, error) {
	if v.IsAbs(p) {
		return cleanIOSPath(p)
	}
	return cleanIOSPath(path.Join(v.GetPath(), p))
}
func (*AFCVFS) Base(p string) string { return path.Base(p) }
func (*AFCVFS) Dir(p string) string  { return path.Dir(p) }

func (v *AFCVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	clean, err := v.resolve(p)
	if err != nil {
		return err
	}
	if clean == "/" && onChunk != nil {
		if virtual := v.virtualRootItems(); len(virtual) != 0 {
			onChunk(virtual)
		}
	}
	client, err := v.session.metadata(ctx)
	if err != nil {
		return err
	}
	names, err := client.List(ctx, clean)
	if err != nil {
		v.session.markMetadataError(client, err)
		return err
	}
	sort.Strings(names)

	workers := []*afcStatWorker{{client: client, metadata: true}}
	defer func() {
		for _, worker := range workers[1:] {
			v.session.returnTransfer(worker.client, worker.lost)
		}
	}()
	for len(workers) <= afcTransferConnections {
		transfer, acquired, leaseErr := v.session.tryLeaseTransfer(ctx)
		if leaseErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			break
		}
		if !acquired {
			break
		}
		workers = append(workers, &afcStatWorker{client: transfer})
	}

	for start := 0; start < len(names); start += afcReadDirChunk {
		end := min(start+afcReadDirChunk, len(names))
		chunk, statErr := v.statBatch(ctx, clean, names[start:end], workers)
		if statErr != nil {
			return statErr
		}
		if len(chunk) != 0 && onChunk != nil {
			onChunk(chunk)
		}
	}
	return nil
}

type afcStatWorker struct {
	client   *afcproto.Client
	metadata bool
	lost     bool
}

func (v *AFCVFS) statBatch(ctx context.Context, directory string, names []string, workers []*afcStatWorker) ([]vfs.VFSItem, error) {
	items := make([]vfs.VFSItem, len(names))
	valid := make([]bool, len(names))
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error
	for workerIndex, worker := range workers {
		workerIndex, worker := workerIndex, worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := workerIndex; index < len(names); index += len(workers) {
				if err := ctx.Err(); err != nil {
					return
				}
				errMu.Lock()
				failed := firstErr != nil
				errMu.Unlock()
				if failed {
					return
				}
				name := names[index]
				if unsafeRemoteName(name) {
					continue
				}
				info, err := worker.client.Stat(ctx, path.Join(directory, name))
				if err != nil {
					if worker.metadata {
						v.session.markMetadataError(worker.client, err)
					} else if afcHandleConnectionLost(err) {
						worker.lost = true
					}
					// Some House Arrest roots enumerate private container-manager
					// metadata that the same service intentionally refuses to stat.
					// Concurrently removed entries are equally safe to omit.
					if errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrNotExist) {
						continue
					}
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
					return
				}
				items[index] = afcVFSItem(name, info)
				valid[index] = true
			}
		}()
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if firstErr != nil {
		return nil, firstErr
	}
	result := make([]vfs.VFSItem, 0, len(items))
	for index, item := range items {
		if valid[index] {
			result = append(result, item)
		}
	}
	return result, nil
}

func (v *AFCVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	clean, err := v.resolve(p)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	if capability, ok := v.rootCapabilityForPath(clean); ok && capability != 0 {
		return vfs.VFSItem{Name: path.Base(clean), IsDir: true, IsExecutable: true, NoExtension: true}, nil
	}
	client, err := v.session.metadata(ctx)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	info, err := client.Stat(ctx, clean)
	if err != nil {
		v.session.markMetadataError(client, err)
		return vfs.VFSItem{}, err
	}
	name := path.Base(clean)
	if clean == "/" {
		name = "/"
	}
	return afcVFSItem(name, info), nil
}

func afcVFSItem(name string, info afcproto.FileInfo) vfs.VFSItem {
	isDir := info.IsDir()
	modeText := "-rw-r--r--"
	if isDir {
		modeText = "drwxr-xr-x"
	} else if info.IsSymlink() {
		modeText = "lrwxrwxrwx"
	}
	return vfs.VFSItem{
		Name: name, Size: info.Size, IsDir: isDir, MTime: info.ModTime, Mode: modeText,
		IsExecutable: info.Mode&0111 != 0, IsHidden: strings.HasPrefix(name, "."),
		IsSymlink: info.IsSymlink(), UnixMode: info.Mode,
	}
}

func (v *AFCVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	clean, err := v.resolve(p)
	if err != nil {
		return nil, err
	}
	client, err := v.session.leaseTransfer(ctx)
	if err != nil {
		return nil, err
	}
	file, err := client.Open(ctx, clean, afcproto.ModeReadOnly)
	if err != nil {
		v.session.returnTransfer(client, afcproto.IsConnectionLost(err))
		return nil, err
	}
	v.session.retain()
	return &afcReadHandle{file: file, client: client, session: v.session}, nil
}

func (v *AFCVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	if v.readOnly {
		return nil, ErrReadOnlyDomain
	}
	clean, err := v.resolve(p)
	if err != nil {
		return nil, err
	}
	if clean == "/" {
		return nil, fs.ErrInvalid
	}
	client, err := v.session.leaseTransfer(ctx)
	if err != nil {
		return nil, err
	}
	file, err := client.Open(ctx, clean, afcproto.ModeWriteOnlyCreateTruncate)
	if err != nil {
		v.session.returnTransfer(client, afcproto.IsConnectionLost(err))
		return nil, err
	}
	v.session.retain()
	return &afcWriteHandle{file: file, client: client, session: v.session}, nil
}

func (v *AFCVFS) MkDir(ctx context.Context, p string) error {
	if v.readOnly {
		return ErrReadOnlyDomain
	}
	clean, err := v.mutationPath(p)
	if err != nil {
		return err
	}
	client, err := v.session.metadata(ctx)
	if err != nil {
		return err
	}
	err = client.MkDir(ctx, clean)
	v.session.markMetadataError(client, err)
	return err
}

func (v *AFCVFS) Remove(ctx context.Context, p string) error {
	if v.readOnly {
		return ErrReadOnlyDomain
	}
	clean, err := v.mutationPath(p)
	if err != nil {
		return err
	}
	client, err := v.session.metadata(ctx)
	if err != nil {
		return err
	}
	err = client.RemoveAll(ctx, clean)
	v.session.markMetadataError(client, err)
	return err
}

func (v *AFCVFS) Rename(ctx context.Context, oldPath, newPath string) error {
	if v.readOnly {
		return ErrReadOnlyDomain
	}
	oldClean, err := v.mutationPath(oldPath)
	if err != nil {
		return err
	}
	newClean, err := v.mutationPath(newPath)
	if err != nil {
		return err
	}
	client, err := v.session.metadata(ctx)
	if err != nil {
		return err
	}
	err = client.Rename(ctx, oldClean, newClean)
	v.session.markMetadataError(client, err)
	return err
}

func (v *AFCVFS) SetAttributes(ctx context.Context, p string, item vfs.VFSItem) error {
	if v.readOnly {
		return ErrReadOnlyDomain
	}
	if item.MTime.IsZero() {
		return errors.ErrUnsupported
	}
	clean, err := v.mutationPath(p)
	if err != nil {
		return err
	}
	client, err := v.session.metadata(ctx)
	if err != nil {
		return err
	}
	err = client.SetModTime(ctx, clean, item.MTime)
	v.session.markMetadataError(client, err)
	return err
}

func (v *AFCVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasServerSideMove: !v.readOnly, HasRandomAccess: true, ReadAccess: vfs.ReadAccessNativeRange, StorageClass: vfs.StorageClassVirtual}
}
func (*AFCVFS) Search(context.Context, string, string) (chan int64, error) {
	return nil, ErrSearchUnsupported
}
func (v *AFCVFS) ParentVFS() vfs.VFS { return v.parent }
func (v *AFCVFS) Clone() vfs.VFS {
	if !v.session.retain() {
		return vfs.NewNullVFS(0)
	}
	return &AFCVFS{
		parent: v.parent, device: v.device, registry: v.registry, session: v.session,
		key: v.key, title: v.title, readOnly: v.readOnly,
		rootOpener: v.rootOpener, rootCapabilities: maps.Clone(v.rootCapabilities), path: v.GetPath(),
	}
}
func (v *AFCVFS) Close() error {
	v.closeOnce.Do(v.session.release)
	return nil
}
func (v *AFCVFS) GetTitle() string                    { return "ios:" + v.key }
func (v *AFCVFS) PanelTitle(p string) string          { return iosPanelTitle(v.title, p) }
func (v *AFCVFS) SessionKey() any                     { return v.session }
func (v *AFCVFS) SessionLost(err error) bool          { return afcproto.IsConnectionLost(err) }
func (v *AFCVFS) CanReconnect() bool                  { return v.session.dial != nil }
func (v *AFCVFS) Reconnect(ctx context.Context) error { return v.session.reset(ctx) }

func (v *AFCVFS) PanelInfoKey(req vfs.PanelInfoRequest) string {
	return v.GetTitle() + ":" + req.Path + ":" + req.SelectedName
}
func (v *AFCVFS) CachedPanelInfo(vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, bool) {
	v.session.infoMu.RLock()
	defer v.session.infoMu.RUnlock()
	if v.session.infoTime.IsZero() {
		return deviceBaselineSnapshot(v.device), false
	}
	return v.session.info, time.Since(v.session.infoTime) < 10*time.Second
}
func (v *AFCVFS) RefreshPanelInfo(ctx context.Context, _ vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
	client, err := v.session.metadata(ctx)
	if err != nil {
		return vfs.PanelInfoSnapshot{}, err
	}
	info, err := client.DeviceInfo(ctx)
	if err != nil {
		v.session.markMetadataError(client, err)
		return vfs.PanelInfoSnapshot{}, err
	}
	snapshot := deviceBaselineSnapshot(v.device)
	if len(snapshot.Sections) == 0 {
		snapshot.Sections = append(snapshot.Sections, vfs.PanelInfoSection{ID: "ios.device", Title: "Apple mobile device"})
	}
	fields := &snapshot.Sections[0].Fields
	*fields = append(*fields, vfs.PanelInfoField{ID: "backend", Label: "Backend", Value: "AFC", Kind: vfs.PanelInfoText})
	if info.TotalBytes > 0 {
		*fields = append(*fields, vfs.PanelInfoField{ID: "storage", Label: "Storage", Kind: vfs.PanelInfoUsage, TotalBytes: info.TotalBytes, AvailableBytes: info.FreeBytes})
	}
	snapshot.RefreshedAt = time.Now()
	v.session.infoMu.Lock()
	v.session.info = snapshot
	v.session.infoTime = snapshot.RefreshedAt
	v.session.infoMu.Unlock()
	return snapshot, nil
}

func (v *AFCVFS) resolve(p string) (string, error) {
	if p == "" || p == "." {
		return v.GetPath(), nil
	}
	return v.Abs(p)
}
func (v *AFCVFS) mutationPath(p string) (string, error) {
	clean, err := v.resolve(p)
	if err != nil {
		return "", err
	}
	if clean == "/" {
		return "", fmt.Errorf("ios: refusing to mutate domain root")
	}
	if _, virtual := v.rootCapabilityForPath(clean); virtual {
		return "", ErrSelectorReadOnly
	}
	return clean, nil
}

type afcReadHandle struct {
	file      *afcproto.File
	client    *afcproto.Client
	session   *afcSession
	mu        sync.Mutex
	lost      bool
	closeOnce sync.Once
	closeErr  error
}

func (h *afcReadHandle) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	n, err := h.file.ReadAtContext(ctx, p, off)
	h.note(err)
	return n, err
}
func (h *afcReadHandle) Read(ctx context.Context, p []byte) (int, error) {
	n, err := h.file.ReadContext(ctx, p)
	h.note(err)
	return n, err
}
func (h *afcReadHandle) Size() int64 { return h.file.Size() }
func (h *afcReadHandle) note(err error) {
	if afcHandleConnectionLost(err) {
		h.mu.Lock()
		h.lost = true
		h.mu.Unlock()
	}
}
func (h *afcReadHandle) Close() error {
	h.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), afcHandleCloseTimeout)
		defer cancel()
		h.closeErr = h.file.CloseContext(ctx)
		h.note(h.closeErr)
		h.mu.Lock()
		lost := h.lost
		h.mu.Unlock()
		h.session.returnTransfer(h.client, lost)
		h.session.release()
	})
	return h.closeErr
}

type afcWriteHandle struct {
	file      *afcproto.File
	client    *afcproto.Client
	session   *afcSession
	mu        sync.Mutex
	lost      bool
	closeOnce sync.Once
	closeErr  error
}

func (h *afcWriteHandle) Write(p []byte) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), afcHandleWriteTimeout)
	defer cancel()
	n, err := h.file.WriteContext(ctx, p)
	if afcHandleConnectionLost(err) {
		h.mu.Lock()
		h.lost = true
		h.mu.Unlock()
	}
	return n, err
}
func (h *afcWriteHandle) Close() error {
	h.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), afcHandleCloseTimeout)
		defer cancel()
		h.closeErr = h.file.CloseContext(ctx)
		if afcHandleConnectionLost(h.closeErr) {
			h.mu.Lock()
			h.lost = true
			h.mu.Unlock()
		}
		h.mu.Lock()
		lost := h.lost
		h.mu.Unlock()
		h.session.returnTransfer(h.client, lost)
		h.session.release()
	})
	return h.closeErr
}

func afcHandleConnectionLost(err error) bool {
	return afcproto.IsConnectionLost(err) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

var (
	_ vfs.OptimisticPathSetter = (*AFCVFS)(nil)
	_ vfs.PanelTitleProvider   = (*AFCVFS)(nil)
	_ vfs.PanelInfoProvider    = (*AFCVFS)(nil)
	_ vfs.SessionIdentity      = (*AFCVFS)(nil)
	_ vfs.SessionReconnector   = (*AFCVFS)(nil)
)
