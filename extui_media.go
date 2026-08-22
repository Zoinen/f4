package main

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	extUiMediaProtocolVersion    = 1
	extUiMediaMaxMessageSize     = 2 * 1024 * 1024
	extUiMediaMaxRangeSize       = 1024 * 1024
	extUiMediaMaxMaterializeSize = int64(2 * 1024 * 1024 * 1024)
	extUiMediaRangeCacheSize     = 8 * 1024 * 1024
	extUiMediaTotalRangeCache    = 64 * 1024 * 1024
	extUiMediaMaxIdleHandles     = 64
	extUiMediaLeaseAckTimeout    = 30 * time.Second
)

var (
	errMediaUnknownResource = errors.New("media resource is unknown or stale")
	errMediaSourceChanged   = errors.New("media source version changed")
	errMediaTooLarge        = errors.New("media request exceeds the configured limit")
)

// extUiImageSourceDescriptor is serialized as FileEntryModel.source. The
// resource id is deliberately scoped to one authenticated media server; the
// source key and opaque content version are the identities used by Gallery's
// derived caches.
type extUiImageSourceDescriptor struct {
	ResourceID      string
	SourceKey       string
	Version         string
	VersionStrength string
	Size            int64
	SizeKnown       bool
	AccessProfile   string
	StorageClass    string
}

type mediaSourceRegistration struct {
	PanelID        string
	CatalogVersion int64
	SourceEpoch    int64
	FS             vfs.VFS
	Path           string
	Item           vfs.VFSItem
	LocalPath      string
}

type mediaRangeKey struct {
	offset int64
	length int
}

type mediaRangeFlight struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	data    []byte
	err     error
}

type mediaMaterializeFlight struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	path    string
	size    int64
	err     error
}

type mediaOpenFlight struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	handle  vfs.ReadAtCloser
	err     error
}

type extUiMediaResource struct {
	broker          *extUiMediaBroker
	id              string
	sourceKey       string
	version         string
	versionStrength string
	size            int64
	sizeKnown       bool
	accessProfile   vfs.ReadAccessProfile
	storageClass    vfs.StorageClass
	fs              vfs.VFS
	path            string
	expected        vfs.VFSItem

	mu              sync.Mutex
	validRefs       int
	active          int
	handle          vfs.ReadAtCloser
	openFlight      *mediaOpenFlight
	validated       bool
	ioMu            sync.Mutex
	ranges          map[mediaRangeKey][]byte
	rangeBytes      int
	rangeFlights    map[mediaRangeKey]*mediaRangeFlight
	materialized    string
	materializedOwn bool
	materializedLen int64
	materializing   *mediaMaterializeFlight
	leaseIDs        map[string]struct{}
	vfsLeaseKey     string
	releasePending  bool
	lastUsed        time.Time

	// Broker accounting is mirrored per resource so a concurrent cache mutation
	// can reconcile the global counters and candidate sets from current state,
	// rather than applying a stale delta after another eviction.
	accountedHandle          bool
	accountedRangeBytes      int
	accountedMaterializedLen int64
}

type extUiMediaPanelState struct {
	revision int64
	ids      map[string]struct{}
}

type extUiMediaVFSLease struct {
	original vfs.VFS
	fs       vfs.VFS
	owned    bool
	refs     int
}

type extUiMediaBroker struct {
	mu        sync.Mutex
	secret    [32]byte
	tempDir   string
	closed    bool
	resources map[string]*extUiMediaResource
	panels    map[string]extUiMediaPanelState
	vfsLeases map[string]*extUiMediaVFSLease
	leases    map[string]string // lease id -> resource id

	openHandles             int
	rangeCacheBytes         int
	ownMaterializationBytes int64
	openHandleCandidates    map[*extUiMediaResource]struct{}
	rangeCacheCandidates    map[*extUiMediaResource]struct{}
	materializeCandidates   map[*extUiMediaResource]struct{}
	globalSweepVisits       uint64 // diagnostics/tests: resources visited by threshold sweeps
	operationWG             sync.WaitGroup
	flightWG                sync.WaitGroup
}

func newExtUiMediaBroker() (*extUiMediaBroker, error) {
	tempDir, err := os.MkdirTemp("", "f4-gallery-media-*")
	if err != nil {
		return nil, err
	}
	b := &extUiMediaBroker{
		tempDir:               tempDir,
		resources:             make(map[string]*extUiMediaResource),
		panels:                make(map[string]extUiMediaPanelState),
		vfsLeases:             make(map[string]*extUiMediaVFSLease),
		leases:                make(map[string]string),
		openHandleCandidates:  make(map[*extUiMediaResource]struct{}),
		rangeCacheCandidates:  make(map[*extUiMediaResource]struct{}),
		materializeCandidates: make(map[*extUiMediaResource]struct{}),
	}
	if _, err := rand.Read(b.secret[:]); err != nil {
		_ = os.RemoveAll(tempDir)
		return nil, err
	}
	return b, nil
}

func mediaComparableIdentity(value any) string {
	if value == nil {
		return "nil"
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Chan, reflect.Func, reflect.Map, reflect.Slice, reflect.UnsafePointer:
		if rv.IsNil() {
			return fmt.Sprintf("%T:nil", value)
		}
		return fmt.Sprintf("%T:%x", value, rv.Pointer())
	default:
		return fmt.Sprintf("%T:%v", value, value)
	}
}

func mediaVFSInstanceKey(filesystem vfs.VFS) string {
	return mediaComparableIdentity(filesystem)
}

func mediaAccessEpoch(filesystem vfs.VFS) string {
	if filesystem == nil {
		return "nil"
	}
	caps := filesystem.GetCapabilities()
	if caps.StorageClass == vfs.StorageClassLocal || (caps.StorageClass == "" && caps.ReadAccess == vfs.ReadAccessDirectLocal) {
		return "local"
	}
	if session, ok := filesystem.(vfs.SessionIdentity); ok {
		if key := session.SessionKey(); key != nil {
			return mediaComparableIdentity(key)
		}
	}
	return mediaVFSInstanceKey(filesystem)
}

func mediaSourceNamespace(filesystem vfs.VFS) string {
	if filesystem != nil {
		caps := filesystem.GetCapabilities()
		if caps.StorageClass == vfs.StorageClassLocal || (caps.StorageClass == "" && caps.ReadAccess == vfs.ReadAccessDirectLocal) {
			return "local"
		}
	}
	if stable, ok := filesystem.(vfs.DirectoryCacheIdentity); ok {
		if key := stable.DirectoryCacheKey(); key != nil {
			return "stable:" + mediaComparableIdentity(key)
		}
	}
	if session, ok := filesystem.(vfs.SessionIdentity); ok {
		if key := session.SessionKey(); key != nil {
			return "session:" + mediaComparableIdentity(key)
		}
	}
	return "instance:" + mediaVFSInstanceKey(filesystem)
}

func mediaCanonicalPath(filesystem vfs.VFS, path string) string {
	if filesystem == nil {
		return path
	}
	if absolute, err := filesystem.Abs(path); err == nil && absolute != "" {
		return absolute
	}
	return path
}

func mediaSourceKey(filesystem vfs.VFS, path string) string {
	h := sha256.New()
	_, _ = io.WriteString(h, fmt.Sprintf("%T", filesystem))
	_, _ = h.Write([]byte{0})
	_, _ = io.WriteString(h, mediaSourceNamespace(filesystem))
	_, _ = h.Write([]byte{0})
	_, _ = io.WriteString(h, mediaCanonicalPath(filesystem, path))
	return fmt.Sprintf("source-%x", h.Sum(nil)[:20])
}

func mediaVersion(item vfs.VFSItem, local bool, catalogVersion int64) (string, string) {
	if item.Revision != "" {
		return item.Revision, "strong"
	}
	mtime := semanticMTimeNanos(item.MTime)
	sizeKnown := item.SizeKnown || item.Size != 0
	if mtime != 0 || sizeKnown {
		if local {
			return fmt.Sprintf("%d:%d", mtime, item.Size), "localStat"
		}
		return fmt.Sprintf("%d:%d", mtime, item.Size), "weakRemote"
	}
	return fmt.Sprintf("session:%d", catalogVersion), "session"
}

func mediaSourceVersion(filesystem vfs.VFS, item vfs.VFSItem, local bool, catalogVersion, sourceEpoch int64) (string, string) {
	version, strength := mediaVersion(item, local, catalogVersion)
	if strength != "session" {
		return version, strength
	}
	// With no provider revision or useful stat tuple, the panel catalog is the
	// only observation that bounds the bytes. Namespace it by the access epoch,
	// but never reuse it across catalog refreshes: the next listing may describe
	// different bytes even when the VFS session itself is unchanged.
	identity := fmt.Sprintf("%s\x00%s\x00%d", mediaAccessEpoch(filesystem), version, sourceEpoch)
	sum := sha256.Sum256([]byte(identity))
	return fmt.Sprintf("session-%x", sum[:16]), strength
}

func mediaStorageClass(caps vfs.VFSCapabilities, localPath string) vfs.StorageClass {
	if localPath != "" {
		return vfs.StorageClassLocal
	}
	if caps.StorageClass != "" {
		return caps.StorageClass
	}
	if caps.ReadAccess == vfs.ReadAccessDirectLocal {
		return vfs.StorageClassLocal
	}
	return vfs.StorageClassUnknown
}

func (b *extUiMediaBroker) resourceID(sourceKey, version, accessEpoch string) string {
	mac := hmac.New(sha256.New, b.secret[:])
	_, _ = io.WriteString(mac, sourceKey)
	_, _ = mac.Write([]byte{0})
	_, _ = io.WriteString(mac, version)
	_, _ = mac.Write([]byte{0})
	_, _ = io.WriteString(mac, accessEpoch)
	return fmt.Sprintf("resource-%x", mac.Sum(nil)[:20])
}

func mediaVFSLeaseKey(filesystem vfs.VFS) string {
	return mediaVFSInstanceKey(filesystem) + "\x00" + mediaAccessEpoch(filesystem)
}

func (b *extUiMediaBroker) vfsLeaseLocked(filesystem vfs.VFS) (string, *extUiMediaVFSLease, error) {
	key := mediaVFSLeaseKey(filesystem)
	if lease := b.vfsLeases[key]; lease != nil {
		lease.refs++
		return key, lease, nil
	}
	clone := filesystem.Clone()
	if clone == nil {
		return "", nil, errors.New("VFS clone returned nil")
	}
	owned := mediaVFSInstanceKey(clone) != mediaVFSInstanceKey(filesystem)
	lease := &extUiMediaVFSLease{original: filesystem, fs: clone, owned: owned, refs: 1}
	b.vfsLeases[key] = lease
	return key, lease, nil
}

func (b *extUiMediaBroker) Register(reg mediaSourceRegistration) extUiImageSourceDescriptor {
	if reg.FS == nil || reg.Item.IsDir {
		return extUiImageSourceDescriptor{}
	}
	path := mediaCanonicalPath(reg.FS, reg.Path)
	caps := reg.FS.GetCapabilities()
	profile := caps.ReadAccess
	storage := mediaStorageClass(caps, reg.LocalPath)
	version, strength := mediaSourceVersion(reg.FS, reg.Item, storage == vfs.StorageClassLocal, reg.CatalogVersion, reg.SourceEpoch)
	sourceKey := mediaSourceKey(reg.FS, path)
	id := b.resourceID(sourceKey, version, mediaAccessEpoch(reg.FS))

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return extUiImageSourceDescriptor{}
	}
	resource := b.resources[id]
	if resource == nil {
		leaseKey, lease, err := b.vfsLeaseLocked(reg.FS)
		if err != nil {
			return extUiImageSourceDescriptor{
				SourceKey: sourceKey, Version: version, VersionStrength: strength,
				Size: reg.Item.Size, SizeKnown: reg.Item.SizeKnown || reg.Item.Size != 0,
				AccessProfile: profile.String(), StorageClass: storage.String(),
			}
		}
		resource = &extUiMediaResource{
			broker: b,
			id:     id, sourceKey: sourceKey, version: version, versionStrength: strength,
			size: reg.Item.Size, sizeKnown: reg.Item.SizeKnown || reg.Item.Size != 0,
			accessProfile: profile, storageClass: storage, fs: lease.fs, path: path,
			expected: reg.Item, ranges: make(map[mediaRangeKey][]byte),
			rangeFlights: make(map[mediaRangeKey]*mediaRangeFlight),
			leaseIDs:     make(map[string]struct{}), vfsLeaseKey: leaseKey, lastUsed: time.Now(),
		}
		b.resources[id] = resource
	}
	return extUiImageSourceDescriptor{
		ResourceID: id, SourceKey: sourceKey, Version: version, VersionStrength: strength,
		Size: resource.size, SizeKnown: resource.sizeKnown,
		AccessProfile: profile.String(), StorageClass: storage.String(),
	}
}

// CommitPanel atomically advances a panel's catalog binding. Resource ids from
// an older catalog stop authorizing reads immediately, while sources shared by
// another panel stay valid.
func (b *extUiMediaBroker) CommitPanel(panelID string, revision int64, ids []string) {
	if panelID == "" {
		return
	}
	next := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id != "" {
			next[id] = struct{}{}
		}
	}
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	previous := b.panels[panelID]
	// Catalog revisions are monotonic only for one FileSystemPanel lifetime.
	// Semantic ids are pointer-derived, so a panel created after a workspace is
	// closed may legitimately reuse the same id with a lower revision. Scene
	// export is synchronous and authoritative; treating that commit as a new
	// binding also prevents a stale panel record from denying every resource in
	// the replacement panel.
	toRetire := make([]*extUiMediaResource, 0)
	for id := range previous.ids {
		if _, retained := next[id]; retained {
			continue
		}
		if resource := b.resources[id]; resource != nil {
			resource.mu.Lock()
			resource.validRefs--
			if resource.validRefs <= 0 {
				resource.validRefs = 0
				resource.releasePending = true
				if resource.active == 0 && len(resource.leaseIDs) == 0 {
					toRetire = append(toRetire, resource)
				}
			}
			resource.mu.Unlock()
		}
	}
	for id := range next {
		if _, retained := previous.ids[id]; retained {
			continue
		}
		if resource := b.resources[id]; resource != nil {
			resource.mu.Lock()
			resource.validRefs++
			resource.mu.Unlock()
		}
	}
	b.panels[panelID] = extUiMediaPanelState{revision: revision, ids: next}
	b.mu.Unlock()
	for _, resource := range toRetire {
		b.retireResource(resource)
	}
}

// CommitScenePanels retires bindings for panels which no longer exist in the
// complete semantic scene. ExportSemanticScene visits every workspace before
// the application adapter calls this method, so an inactive workspace remains
// observed while a closed workspace releases its resource registry and VFS
// clone/session leases promptly.
func (b *extUiMediaBroker) CommitScenePanels(panelIDs []string) {
	observed := make(map[string]struct{}, len(panelIDs))
	for _, panelID := range panelIDs {
		if panelID != "" {
			observed[panelID] = struct{}{}
		}
	}

	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	toRetire := make([]*extUiMediaResource, 0)
	for panelID, panel := range b.panels {
		if _, retained := observed[panelID]; retained {
			continue
		}
		delete(b.panels, panelID)
		for id := range panel.ids {
			resource := b.resources[id]
			if resource == nil {
				continue
			}
			resource.mu.Lock()
			resource.validRefs--
			if resource.validRefs <= 0 {
				resource.validRefs = 0
				resource.releasePending = true
				if resource.active == 0 && len(resource.leaseIDs) == 0 {
					toRetire = append(toRetire, resource)
				}
			}
			resource.mu.Unlock()
		}
	}
	b.mu.Unlock()
	for _, resource := range toRetire {
		b.retireResource(resource)
	}
}

func (b *extUiMediaBroker) acquire(id string) (*extUiMediaResource, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, errMediaUnknownResource
	}
	resource := b.resources[id]
	if resource != nil {
		b.operationWG.Add(1)
	}
	b.mu.Unlock()
	if resource == nil {
		return nil, errMediaUnknownResource
	}
	resource.mu.Lock()
	defer resource.mu.Unlock()
	if resource.validRefs <= 0 {
		b.operationWG.Done()
		return nil, errMediaUnknownResource
	}
	resource.active++
	resource.lastUsed = time.Now()
	return resource, nil
}

func (r *extUiMediaResource) finish() {
	r.mu.Lock()
	r.active--
	retireNow := r.active == 0 && r.validRefs == 0 && len(r.leaseIDs) == 0
	closeNow := !retireNow && r.active == 0 && r.releasePending && len(r.leaseIDs) == 0
	if closeNow {
		r.releasePending = false
	}
	r.mu.Unlock()
	if retireNow {
		r.broker.retireResource(r)
	} else if closeNow {
		r.closeCached()
	}
}

func (r *extUiMediaResource) profileAndValid() (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.accessProfile.String(), r.validRefs > 0
}

func (r *extUiMediaResource) validate(ctx context.Context) error {
	r.mu.Lock()
	if r.validated {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()
	item, err := r.fs.Stat(ctx, r.path)
	if err != nil {
		return err
	}
	valid := false
	switch r.versionStrength {
	case "strong":
		valid = item.Revision != "" && item.Revision == r.expected.Revision
	case "localStat", "weakRemote":
		valid = item.Size == r.expected.Size && semanticMTimeNanos(item.MTime) == semanticMTimeNanos(r.expected.MTime)
	case "session":
		valid = true
	}
	if !valid {
		return errMediaSourceChanged
	}
	r.mu.Lock()
	r.validated = true
	r.mu.Unlock()
	return nil
}

func (r *extUiMediaResource) ensureHandle(ctx context.Context) (vfs.ReadAtCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r.mu.Lock()
	if r.handle != nil {
		handle := r.handle
		r.mu.Unlock()
		return handle, nil
	}
	if flight := r.openFlight; flight != nil {
		flight.waiters++
		r.mu.Unlock()
		return waitMediaOpen(ctx, r, flight)
	}
	flightCtx, cancel := context.WithCancel(context.Background())
	flight := &mediaOpenFlight{done: make(chan struct{}), cancel: cancel, waiters: 1}
	r.openFlight = flight
	r.mu.Unlock()
	r.broker.flightWG.Add(1)
	go func() {
		defer r.broker.flightWG.Done()
		r.performOpen(flightCtx, flight)
	}()
	return waitMediaOpen(ctx, r, flight)
}

func waitMediaOpen(ctx context.Context, resource *extUiMediaResource, flight *mediaOpenFlight) (vfs.ReadAtCloser, error) {
	select {
	case <-flight.done:
		return flight.handle, flight.err
	case <-ctx.Done():
		resource.mu.Lock()
		if resource.openFlight == flight {
			flight.waiters--
			if flight.waiters == 0 {
				flight.cancel()
			}
		}
		resource.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (r *extUiMediaResource) performOpen(ctx context.Context, flight *mediaOpenFlight) {
	defer flight.cancel()
	err := r.validate(ctx)
	var handle vfs.ReadAtCloser
	if err == nil {
		handle, err = r.fs.Open(ctx, r.path)
	}
	if err == nil && ctx.Err() != nil {
		err = ctx.Err()
	}
	r.broker.mu.Lock()
	r.mu.Lock()
	if err == nil {
		if flight.waiters <= 0 {
			err = context.Canceled
		} else if r.broker.closed || r.broker.resources[r.id] != r || r.validRefs <= 0 {
			err = errMediaUnknownResource
		} else {
			r.handle = handle
			if !r.accountedHandle {
				r.broker.openHandles++
			}
			r.accountedHandle = true
			r.broker.openHandleCandidates[r] = struct{}{}
			if profiler, ok := handle.(vfs.ReadAccessProfiler); ok {
				r.accessProfile = profiler.ReadAccessProfile()
			}
		}
	}
	var rejectedHandle vfs.ReadAtCloser
	if err != nil && handle != nil {
		rejectedHandle = handle
		handle = nil
	}
	flight.handle = handle
	flight.err = err
	if r.openFlight == flight {
		r.openFlight = nil
	}
	close(flight.done)
	r.mu.Unlock()
	r.broker.mu.Unlock()
	if rejectedHandle != nil {
		_ = rejectedHandle.Close()
	}
}

func (r *extUiMediaResource) readAt(ctx context.Context, data []byte, offset int64) (int, error) {
	for {
		handle, err := r.ensureHandle(ctx)
		if err != nil {
			return 0, err
		}

		// A caller can obtain a shared handle just before another operation is
		// canceled. Serialize the current-handle check, I/O, detach and Close so
		// the canceled operation cannot close a handle underneath the next one.
		r.ioMu.Lock()
		r.mu.Lock()
		if r.handle != handle {
			r.mu.Unlock()
			r.ioMu.Unlock()
			continue
		}
		r.mu.Unlock()

		n, readErr := handle.ReadAt(ctx, data, offset)
		if ctx.Err() != nil {
			r.mu.Lock()
			detached := r.handle == handle
			if detached {
				r.handle = nil
				r.validated = false
			}
			r.mu.Unlock()
			if detached {
				_ = handle.Close()
				r.broker.syncResourceAccounting(r)
			}
			r.ioMu.Unlock()
			return n, ctx.Err()
		}
		r.ioMu.Unlock()
		return n, readErr
	}
}

func (b *extUiMediaBroker) ReadRange(ctx context.Context, id string, offset int64, length int) ([]byte, string, error) {
	if offset < 0 || length <= 0 || length > extUiMediaMaxRangeSize || offset > int64(^uint64(0)>>1)-int64(length) {
		return nil, "", errMediaTooLarge
	}
	resource, err := b.acquire(id)
	if err != nil {
		return nil, "", err
	}
	defer func() {
		resource.finish()
		b.operationWG.Done()
		b.evictIdleHandles(nil)
	}()
	if resource.sizeKnown && offset >= resource.size {
		profile, valid := resource.profileAndValid()
		if !valid {
			return nil, profile, errMediaUnknownResource
		}
		return []byte{}, profile, nil
	}
	if resource.sizeKnown && offset+int64(length) > resource.size {
		length = int(resource.size - offset)
	}
	key := mediaRangeKey{offset: offset, length: length}
	resource.mu.Lock()
	if cached, ok := resource.ranges[key]; ok {
		data := append([]byte(nil), cached...)
		profile := resource.accessProfile.String()
		valid := resource.validRefs > 0
		resource.mu.Unlock()
		if !valid {
			return nil, profile, errMediaUnknownResource
		}
		return data, profile, nil
	}
	for cachedKey, cached := range resource.ranges {
		start := key.offset - cachedKey.offset
		if start < 0 || start+int64(key.length) > int64(len(cached)) {
			continue
		}
		data := append([]byte(nil), cached[start:start+int64(key.length)]...)
		profile := resource.accessProfile.String()
		valid := resource.validRefs > 0
		resource.mu.Unlock()
		if !valid {
			return nil, profile, errMediaUnknownResource
		}
		return data, profile, nil
	}
	if flight := resource.rangeFlights[key]; flight != nil {
		flight.waiters++
		resource.mu.Unlock()
		return waitMediaRange(ctx, resource, key, flight)
	}
	flightCtx, cancel := context.WithCancel(context.Background())
	flight := &mediaRangeFlight{done: make(chan struct{}), cancel: cancel, waiters: 1}
	resource.rangeFlights[key] = flight
	// The shared operation owns an activity reference independently of its
	// callers, so canceling one waiter cannot let release close its handle.
	resource.active++
	resource.mu.Unlock()
	resource.broker.flightWG.Add(1)
	go func() {
		defer resource.broker.flightWG.Done()
		resource.performRange(flightCtx, key, flight)
	}()
	return waitMediaRange(ctx, resource, key, flight)
}

func waitMediaRange(ctx context.Context, resource *extUiMediaResource, key mediaRangeKey, flight *mediaRangeFlight) ([]byte, string, error) {
	select {
	case <-flight.done:
		profile, valid := resource.profileAndValid()
		if !valid {
			return nil, profile, errMediaUnknownResource
		}
		return append([]byte(nil), flight.data...), profile, flight.err
	case <-ctx.Done():
		resource.mu.Lock()
		if resource.rangeFlights[key] == flight {
			flight.waiters--
			if flight.waiters == 0 {
				flight.cancel()
			}
		}
		profile := resource.accessProfile.String()
		resource.mu.Unlock()
		return nil, profile, ctx.Err()
	}
}

func (resource *extUiMediaResource) performRange(ctx context.Context, key mediaRangeKey, flight *mediaRangeFlight) {
	defer flight.cancel()
	defer func() {
		resource.finish()
		resource.broker.evictIdleHandles(nil)
	}()
	var err error
	var data []byte
	if err == nil && key.length > 0 {
		buf := make([]byte, key.length)
		n, readErr := resource.readAt(ctx, buf, key.offset)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			err = readErr
		} else {
			data = buf[:n]
		}
	}
	resource.mu.Lock()
	if err == nil {
		if resource.rangeBytes+len(data) > extUiMediaRangeCacheSize {
			resource.ranges = make(map[mediaRangeKey][]byte)
			resource.rangeBytes = 0
		}
		resource.ranges[key] = append([]byte(nil), data...)
		resource.rangeBytes += len(data)
	}
	flight.data = append([]byte(nil), data...)
	flight.err = err
	delete(resource.rangeFlights, key)
	close(flight.done)
	resource.mu.Unlock()
	resource.broker.syncResourceAccounting(resource)
	resource.broker.evictRangeCaches(resource)
}

// syncResourceAccounting updates counters and eviction membership from one
// coherent resource snapshot. Calls are made after mutating the resource and
// use the broker -> resource lock order shared by registration/retirement.
// Per-resource mirrors prevent a delayed sync from replaying a stale delta if
// another goroutine evicted the same cache in the meantime.
func (b *extUiMediaBroker) syncResourceAccounting(resource *extUiMediaResource) {
	if resource == nil {
		return
	}
	b.mu.Lock()
	resource.mu.Lock()
	registered := !b.closed && b.resources[resource.id] == resource

	hasHandle := registered && resource.handle != nil
	if hasHandle != resource.accountedHandle {
		if hasHandle {
			b.openHandles++
		} else {
			b.openHandles--
		}
		resource.accountedHandle = hasHandle
	}
	if hasHandle {
		b.openHandleCandidates[resource] = struct{}{}
	} else {
		delete(b.openHandleCandidates, resource)
	}

	rangeBytes := 0
	if registered {
		rangeBytes = resource.rangeBytes
	}
	b.rangeCacheBytes += rangeBytes - resource.accountedRangeBytes
	resource.accountedRangeBytes = rangeBytes
	if rangeBytes > 0 {
		b.rangeCacheCandidates[resource] = struct{}{}
	} else {
		delete(b.rangeCacheCandidates, resource)
	}

	materializedLen := int64(0)
	if registered && resource.materializedOwn && resource.materializedLen > 0 {
		materializedLen = resource.materializedLen
	}
	b.ownMaterializationBytes += materializedLen - resource.accountedMaterializedLen
	resource.accountedMaterializedLen = materializedLen
	if materializedLen > 0 {
		b.materializeCandidates[resource] = struct{}{}
	} else {
		delete(b.materializeCandidates, resource)
	}

	if b.openHandles < 0 {
		b.openHandles = 0
	}
	if b.rangeCacheBytes < 0 {
		b.rangeCacheBytes = 0
	}
	if b.ownMaterializationBytes < 0 {
		b.ownMaterializationBytes = 0
	}
	resource.mu.Unlock()
	b.mu.Unlock()
}

func (b *extUiMediaBroker) evictRangeCaches(except *extUiMediaResource) {
	b.mu.Lock()
	if b.closed || b.rangeCacheBytes <= extUiMediaTotalRangeCache {
		b.mu.Unlock()
		return
	}
	total := b.rangeCacheBytes
	resources := make([]*extUiMediaResource, 0, len(b.rangeCacheCandidates))
	for resource := range b.rangeCacheCandidates {
		resources = append(resources, resource)
	}
	b.globalSweepVisits += uint64(len(resources))
	b.mu.Unlock()
	type candidate struct {
		resource *extUiMediaResource
		bytes    int
		lastUsed time.Time
	}
	candidates := make([]candidate, 0, len(resources))
	for _, resource := range resources {
		resource.mu.Lock()
		if resource.rangeBytes > 0 {
			if resource != except {
				candidates = append(candidates, candidate{resource: resource, bytes: resource.rangeBytes, lastUsed: resource.lastUsed})
			}
		}
		resource.mu.Unlock()
	}
	if total <= extUiMediaTotalRangeCache {
		return
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].lastUsed.Before(candidates[j].lastUsed) })
	for _, current := range candidates {
		if total <= extUiMediaTotalRangeCache {
			break
		}
		current.resource.mu.Lock()
		removed := current.resource.rangeBytes
		current.resource.ranges = make(map[mediaRangeKey][]byte)
		current.resource.rangeBytes = 0
		current.resource.mu.Unlock()
		total -= removed
		b.syncResourceAccounting(current.resource)
	}
}

func localBackingPath(handle vfs.ReadAtCloser) (string, bool) {
	backing, ok := handle.(vfs.LocalBackingReader)
	if !ok {
		return "", false
	}
	path, ok := backing.LocalPath()
	if !ok || path == "" || !filepath.IsAbs(path) {
		return "", false
	}
	return filepath.Clean(path), true
}

func mediaHandleSize(handle vfs.ReadAtCloser, fallback int64) int64 {
	if size := handle.Size(); size >= 0 {
		return size
	}
	return fallback
}

func (r *extUiMediaResource) cachedContiguousPrefix() []byte {
	type segment struct {
		offset int64
		data   []byte
	}
	r.mu.Lock()
	segments := make([]segment, 0, len(r.ranges))
	for key, data := range r.ranges {
		if len(data) > 0 {
			segments = append(segments, segment{offset: key.offset, data: append([]byte(nil), data...)})
		}
	}
	r.mu.Unlock()
	sort.Slice(segments, func(i, j int) bool {
		if segments[i].offset == segments[j].offset {
			return len(segments[i].data) > len(segments[j].data)
		}
		return segments[i].offset < segments[j].offset
	})
	var prefix []byte
	for _, current := range segments {
		next := int64(len(prefix))
		if current.offset > next {
			break
		}
		end := current.offset + int64(len(current.data))
		if end <= next {
			continue
		}
		prefix = append(prefix, current.data[next-current.offset:]...)
	}
	return prefix
}

func (r *extUiMediaResource) materialize(ctx context.Context, tempDir string) (string, int64, error) {
	r.mu.Lock()
	if r.materialized != "" {
		path, size := r.materialized, r.materializedLen
		r.mu.Unlock()
		return path, size, nil
	}
	if flight := r.materializing; flight != nil {
		flight.waiters++
		r.mu.Unlock()
		return waitMediaMaterialization(ctx, r, flight)
	}
	flightCtx, cancel := context.WithCancel(context.Background())
	flight := &mediaMaterializeFlight{done: make(chan struct{}), cancel: cancel, waiters: 1}
	r.materializing = flight
	r.active++
	r.mu.Unlock()
	r.broker.flightWG.Add(1)
	go func() {
		defer r.broker.flightWG.Done()
		r.performMaterialization(flightCtx, tempDir, flight)
	}()
	return waitMediaMaterialization(ctx, r, flight)
}

func waitMediaMaterialization(ctx context.Context, resource *extUiMediaResource, flight *mediaMaterializeFlight) (string, int64, error) {
	select {
	case <-flight.done:
		_, valid := resource.profileAndValid()
		if !valid {
			return "", 0, errMediaUnknownResource
		}
		return flight.path, flight.size, flight.err
	case <-ctx.Done():
		resource.mu.Lock()
		if resource.materializing == flight {
			flight.waiters--
			if flight.waiters == 0 {
				flight.cancel()
			}
		}
		resource.mu.Unlock()
		return "", 0, ctx.Err()
	}
}

func (r *extUiMediaResource) performMaterialization(ctx context.Context, tempDir string, flight *mediaMaterializeFlight) {
	defer flight.cancel()
	defer func() {
		r.finish()
		r.broker.evictIdleHandles(nil)
	}()
	path, size, own, err := r.materializeOnce(ctx, tempDir)
	r.mu.Lock()
	if err == nil {
		r.materialized = path
		r.materializedLen = size
		r.materializedOwn = own
	}
	flight.path, flight.size, flight.err = path, size, err
	r.materializing = nil
	close(flight.done)
	r.mu.Unlock()
	r.broker.syncResourceAccounting(r)
}

func (r *extUiMediaResource) materializeOnce(ctx context.Context, tempDir string) (string, int64, bool, error) {
	if r.sizeKnown && r.size > extUiMediaMaxMaterializeSize {
		return "", 0, false, errMediaTooLarge
	}
	handle, err := r.ensureHandle(ctx)
	if err != nil {
		return "", 0, false, err
	}
	if path, ok := localBackingPath(handle); ok {
		return path, mediaHandleSize(handle, r.size), false, nil
	}
	tmp, err := os.CreateTemp(tempDir, "source-*")
	if err != nil {
		return "", 0, false, err
	}
	tmpPath := tmp.Name()
	keep := false
	defer func() {
		_ = tmp.Close()
		if !keep {
			_ = os.Remove(tmpPath)
		}
	}()

	prefix := r.cachedContiguousPrefix()
	if len(prefix) > 0 {
		if _, err := tmp.Write(prefix); err != nil {
			return "", 0, false, err
		}
	}
	buffer := make([]byte, extUiMediaMaxRangeSize)
	offset := int64(len(prefix))
	checkedBackingAfterRead := false
	for {
		if err := ctx.Err(); err != nil {
			return "", 0, false, err
		}
		want := len(buffer)
		if r.sizeKnown {
			remaining := r.size - offset
			if remaining <= 0 {
				break
			}
			if remaining < int64(want) {
				want = int(remaining)
			}
		}
		n, readErr := r.readAt(ctx, buffer[:want], offset)
		if !checkedBackingAfterRead {
			checkedBackingAfterRead = true
			currentHandle, currentErr := r.ensureHandle(ctx)
			if currentErr != nil {
				return "", 0, false, currentErr
			}
			if path, ok := localBackingPath(currentHandle); ok {
				return path, mediaHandleSize(currentHandle, r.size), false, nil
			}
		}
		if n > 0 {
			if offset+int64(n) > extUiMediaMaxMaterializeSize {
				return "", 0, false, errMediaTooLarge
			}
			if _, err := tmp.Write(buffer[:n]); err != nil {
				return "", 0, false, err
			}
			offset += int64(n)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return "", 0, false, readErr
		}
		if n == 0 {
			return "", 0, false, io.ErrNoProgress
		}
	}
	if err := tmp.Sync(); err != nil {
		return "", 0, false, err
	}
	if err := tmp.Close(); err != nil {
		return "", 0, false, err
	}
	keep = true
	return tmpPath, offset, true, nil
}

func (b *extUiMediaBroker) newLease(resource *extUiMediaResource) (string, error) {
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	id := fmt.Sprintf("lease-%x", random[:])
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return "", errMediaUnknownResource
	}
	resource.mu.Lock()
	if resource.validRefs <= 0 {
		resource.mu.Unlock()
		b.mu.Unlock()
		return "", errMediaUnknownResource
	}
	b.leases[id] = resource.id
	resource.leaseIDs[id] = struct{}{}
	resource.mu.Unlock()
	b.mu.Unlock()
	return id, nil
}

func (b *extUiMediaBroker) hasLease(resourceID, leaseID string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return leaseID != "" && b.leases[leaseID] == resourceID
}

func (b *extUiMediaBroker) Materialize(ctx context.Context, id string) (string, string, int64, string, error) {
	resource, err := b.acquire(id)
	if err != nil {
		return "", "", 0, "", err
	}
	defer func() {
		resource.finish()
		b.operationWG.Done()
		b.evictIdleHandles(nil)
	}()
	path, size, err := resource.materialize(ctx, b.tempDir)
	profile, valid := resource.profileAndValid()
	if !valid {
		return "", "", 0, profile, errMediaUnknownResource
	}
	if err != nil {
		return "", "", 0, profile, err
	}
	b.evictOwnMaterializations(resource)
	leaseID, err := b.newLease(resource)
	if err != nil {
		return "", "", 0, profile, err
	}
	return path, leaseID, size, profile, nil
}

func (b *extUiMediaBroker) Release(resourceID, leaseID string) {
	b.mu.Lock()
	resource := b.resources[resourceID]
	if leaseID != "" {
		if owner := b.leases[leaseID]; owner != resourceID {
			b.mu.Unlock()
			return
		}
		delete(b.leases, leaseID)
	}
	b.mu.Unlock()
	if resource == nil {
		return
	}
	resource.mu.Lock()
	if leaseID != "" {
		delete(resource.leaseIDs, leaseID)
	}
	if leaseID == "" {
		resource.releasePending = true
	}
	retireNow := resource.validRefs == 0 && resource.active == 0 && len(resource.leaseIDs) == 0
	closeNow := !retireNow && resource.releasePending && resource.active == 0 && len(resource.leaseIDs) == 0
	if closeNow {
		resource.releasePending = false
	}
	resource.mu.Unlock()
	if retireNow {
		b.retireResource(resource)
	} else if closeNow {
		resource.closeCached()
	}
	b.evictOwnMaterializations(nil)
	b.evictIdleHandles(nil)
}

// retireResource removes a terminally unreferenced descriptor from the
// session registry as well as its cached bytes. The pointer check prevents an
// old resource from deleting a newly registered resource with the same id.
// Lock ordering is broker -> resource, matching registration and lease paths.
func (b *extUiMediaBroker) retireResource(resource *extUiMediaResource) bool {
	if resource == nil {
		return false
	}
	b.mu.Lock()
	if b.closed || b.resources[resource.id] != resource {
		b.mu.Unlock()
		return false
	}
	resource.mu.Lock()
	if resource.validRefs != 0 || resource.active != 0 || len(resource.leaseIDs) != 0 {
		resource.mu.Unlock()
		b.mu.Unlock()
		return false
	}
	delete(b.resources, resource.id)
	var vfsToClose vfs.VFS
	if lease := b.vfsLeases[resource.vfsLeaseKey]; lease != nil {
		lease.refs--
		if lease.refs <= 0 {
			delete(b.vfsLeases, resource.vfsLeaseKey)
			if lease.owned {
				vfsToClose = lease.fs
			}
		}
	}
	resource.releasePending = false
	handle := resource.handle
	resource.handle = nil
	path, own := resource.materialized, resource.materializedOwn
	if resource.accountedHandle && b.openHandles > 0 {
		b.openHandles--
	}
	delete(b.openHandleCandidates, resource)
	resource.accountedHandle = false
	if resource.accountedRangeBytes > 0 {
		b.rangeCacheBytes -= resource.accountedRangeBytes
		if b.rangeCacheBytes < 0 {
			b.rangeCacheBytes = 0
		}
	}
	delete(b.rangeCacheCandidates, resource)
	resource.accountedRangeBytes = 0
	if resource.accountedMaterializedLen > 0 {
		b.ownMaterializationBytes -= resource.accountedMaterializedLen
		if b.ownMaterializationBytes < 0 {
			b.ownMaterializationBytes = 0
		}
	}
	delete(b.materializeCandidates, resource)
	resource.accountedMaterializedLen = 0
	resource.materialized = ""
	resource.materializedOwn = false
	resource.materializedLen = 0
	resource.validated = false
	resource.ranges = make(map[mediaRangeKey][]byte)
	resource.rangeBytes = 0
	resource.mu.Unlock()
	b.mu.Unlock()
	if handle != nil {
		_ = handle.Close()
	}
	if own && path != "" {
		_ = os.Remove(path)
	}
	if vfsToClose != nil {
		_ = vfsToClose.Close()
	}
	return true
}

func (b *extUiMediaBroker) evictIdleHandles(except *extUiMediaResource) {
	b.mu.Lock()
	if b.closed || b.openHandles <= extUiMediaMaxIdleHandles {
		b.mu.Unlock()
		return
	}
	openCount := b.openHandles
	resources := make([]*extUiMediaResource, 0, len(b.openHandleCandidates))
	for resource := range b.openHandleCandidates {
		resources = append(resources, resource)
	}
	b.globalSweepVisits += uint64(len(resources))
	b.mu.Unlock()
	type candidate struct {
		resource *extUiMediaResource
		lastUsed time.Time
	}
	candidates := make([]candidate, 0, len(resources))
	for _, resource := range resources {
		resource.mu.Lock()
		if resource.handle != nil {
			canCloseWithLease := resource.materializedOwn && resource.materialized != ""
			if resource != except && resource.active == 0 && (len(resource.leaseIDs) == 0 || canCloseWithLease) {
				candidates = append(candidates, candidate{resource: resource, lastUsed: resource.lastUsed})
			}
		}
		resource.mu.Unlock()
	}
	if openCount <= extUiMediaMaxIdleHandles {
		return
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].lastUsed.Before(candidates[j].lastUsed) })
	for _, current := range candidates {
		if openCount <= extUiMediaMaxIdleHandles {
			break
		}
		if current.resource.closeIdleHandle() {
			openCount--
		}
	}
}

func (r *extUiMediaResource) closeIdleHandle() bool {
	r.mu.Lock()
	canCloseWithLease := r.materializedOwn && r.materialized != ""
	if r.handle == nil || r.active != 0 || (len(r.leaseIDs) != 0 && !canCloseWithLease) {
		r.mu.Unlock()
		return false
	}
	handle := r.handle
	r.handle = nil
	r.validated = false
	if !r.materializedOwn {
		r.materialized = ""
		r.materializedLen = 0
	}
	r.mu.Unlock()
	_ = handle.Close()
	r.broker.syncResourceAccounting(r)
	return true
}

func (b *extUiMediaBroker) evictOwnMaterializations(except *extUiMediaResource) {
	b.mu.Lock()
	if b.closed || b.ownMaterializationBytes <= extUiMediaMaxMaterializeSize {
		b.mu.Unlock()
		return
	}
	total := b.ownMaterializationBytes
	resources := make([]*extUiMediaResource, 0, len(b.materializeCandidates))
	for resource := range b.materializeCandidates {
		resources = append(resources, resource)
	}
	b.globalSweepVisits += uint64(len(resources))
	b.mu.Unlock()
	type candidate struct {
		resource  *extUiMediaResource
		path      string
		size      int64
		lastUsed  time.Time
		evictable bool
	}
	candidates := make([]candidate, 0, len(resources))
	for _, resource := range resources {
		resource.mu.Lock()
		if resource.materializedOwn && resource.materialized != "" {
			current := candidate{
				resource: resource, path: resource.materialized, size: resource.materializedLen,
				lastUsed:  resource.lastUsed,
				evictable: resource != except && resource.active == 0 && len(resource.leaseIDs) == 0,
			}
			candidates = append(candidates, current)
		}
		resource.mu.Unlock()
	}
	if total <= extUiMediaMaxMaterializeSize {
		return
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].lastUsed.Before(candidates[j].lastUsed) })
	for _, current := range candidates {
		if total <= extUiMediaMaxMaterializeSize {
			break
		}
		if !current.evictable {
			continue
		}
		current.resource.mu.Lock()
		if !current.resource.materializedOwn || current.resource.materialized != current.path ||
			current.resource.active != 0 || len(current.resource.leaseIDs) != 0 {
			current.resource.mu.Unlock()
			continue
		}
		current.resource.materialized = ""
		current.resource.materializedOwn = false
		current.resource.materializedLen = 0
		current.resource.mu.Unlock()
		_ = os.Remove(current.path)
		total -= current.size
		b.syncResourceAccounting(current.resource)
	}
}

func (r *extUiMediaResource) closeCached() {
	r.ioMu.Lock()
	r.mu.Lock()
	handle := r.handle
	r.handle = nil
	path, own := r.materialized, r.materializedOwn
	r.materialized = ""
	r.materializedOwn = false
	r.materializedLen = 0
	r.validated = false
	r.ranges = make(map[mediaRangeKey][]byte)
	r.rangeBytes = 0
	r.mu.Unlock()
	if handle != nil {
		_ = handle.Close()
	}
	if own {
		if path != "" {
			_ = os.Remove(path)
		}
	}
	r.ioMu.Unlock()
	r.broker.syncResourceAccounting(r)
}

func (b *extUiMediaBroker) Close() error {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil
	}
	b.closed = true
	resources := make([]*extUiMediaResource, 0, len(b.resources))
	for _, resource := range b.resources {
		resources = append(resources, resource)
	}
	vfsLeases := make([]*extUiMediaVFSLease, 0, len(b.vfsLeases))
	for _, lease := range b.vfsLeases {
		vfsLeases = append(vfsLeases, lease)
	}
	b.resources = nil
	b.panels = nil
	b.leases = nil
	b.mu.Unlock()
	b.operationWG.Wait()
	b.flightWG.Wait()
	for _, resource := range resources {
		resource.closeCached()
	}
	for _, lease := range vfsLeases {
		if lease.owned {
			_ = lease.fs.Close()
		}
	}
	return os.RemoveAll(b.tempDir)
}

var activeExtUiMedia struct {
	sync.RWMutex
	broker *extUiMediaBroker
}

func setActiveExtUiMediaBroker(broker *extUiMediaBroker) func() {
	activeExtUiMedia.Lock()
	previous := activeExtUiMedia.broker
	activeExtUiMedia.broker = broker
	activeExtUiMedia.Unlock()
	return func() {
		activeExtUiMedia.Lock()
		if activeExtUiMedia.broker == broker {
			activeExtUiMedia.broker = previous
		}
		activeExtUiMedia.Unlock()
	}
}

func currentExtUiMediaBroker() *extUiMediaBroker {
	activeExtUiMedia.RLock()
	defer activeExtUiMedia.RUnlock()
	return activeExtUiMedia.broker
}

type extUiMediaServer struct {
	listener net.Listener
	nonce    string
	broker   *extUiMediaBroker
	done     chan struct{}
	once     sync.Once
	mu       sync.Mutex
	conn     net.Conn
	pending  map[net.Conn]struct{}
	clients  map[net.Conn]*extUiMediaConn
	closed   bool
	closeErr error
	acceptWG sync.WaitGroup
	serveWG  sync.WaitGroup
}

func newExtUiMediaServer() (*extUiMediaServer, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	nonce, err := extUiNewNonce()
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	broker, err := newExtUiMediaBroker()
	if err != nil {
		_ = listener.Close()
		return nil, err
	}
	server := &extUiMediaServer{
		listener: listener, nonce: nonce, broker: broker, done: make(chan struct{}),
		pending: make(map[net.Conn]struct{}), clients: make(map[net.Conn]*extUiMediaConn),
	}
	server.acceptWG.Add(1)
	go server.accept()
	return server, nil
}

func (s *extUiMediaServer) Endpoint() string { return s.listener.Addr().String() }

func extUiMediaSendMessage(w io.Writer, msg map[string]any) error {
	payload, err := msgpack.Marshal(msg)
	if err != nil {
		return err
	}
	if len(payload) == 0 || len(payload) > extUiMediaMaxMessageSize {
		return fmt.Errorf("media message too large: %d bytes", len(payload))
	}
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

func extUiMediaReadMessage(r io.Reader) (map[string]any, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 || size > extUiMediaMaxMessageSize {
		return nil, fmt.Errorf("invalid media message size: %d", size)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	var message map[string]any
	if err := msgpack.Unmarshal(payload, &message); err != nil {
		return nil, err
	}
	return message, nil
}

func extUiMediaInt64(value any) int64 {
	switch number := value.(type) {
	case int:
		return int64(number)
	case int8:
		return int64(number)
	case int16:
		return int64(number)
	case int32:
		return int64(number)
	case int64:
		return number
	case uint:
		if uint64(number) <= uint64(^uint64(0)>>1) {
			return int64(number)
		}
	case uint8:
		return int64(number)
	case uint16:
		return int64(number)
	case uint32:
		return int64(number)
	case uint64:
		if number <= uint64(^uint64(0)>>1) {
			return int64(number)
		}
	}
	return -1
}

func (s *extUiMediaServer) accept() {
	defer s.acceptWG.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			_ = conn.Close()
			return
		}
		s.pending[conn] = struct{}{}
		s.mu.Unlock()

		if !s.authenticate(conn) {
			s.mu.Lock()
			delete(s.pending, conn)
			s.mu.Unlock()
			_ = conn.Close()
			continue
		}

		client := &extUiMediaConn{
			conn: conn, broker: s.broker, cancels: make(map[string]*extUiMediaRequestState),
			provisional: make(map[string]*extUiMediaProvisionalLease), sem: make(chan struct{}, 8),
		}
		s.mu.Lock()
		delete(s.pending, conn)
		if s.closed {
			s.mu.Unlock()
			_ = conn.Close()
			return
		}
		previous := s.conn
		s.conn = conn
		s.clients[conn] = client
		s.serveWG.Add(1)
		s.mu.Unlock()
		go s.serve(conn, client)
		if previous != nil && previous != conn {
			_ = previous.Close()
		}
	}
}

func (s *extUiMediaServer) authenticate(conn net.Conn) bool {
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	message, err := extUiMediaReadMessage(conn)
	if err != nil || extUiString(message, "type") != "hello" || extUiString(message, "nonce") != s.nonce || extUiInt(message, "protocol") != extUiMediaProtocolVersion {
		return false
	}
	if err := extUiMediaSendMessage(conn, map[string]any{
		"type": "hello", "protocol": extUiMediaProtocolVersion, "maxChunkSize": extUiMediaMaxRangeSize,
	}); err != nil {
		return false
	}
	_ = conn.SetDeadline(time.Time{})
	return true
}

type extUiMediaConn struct {
	conn        net.Conn
	broker      *extUiMediaBroker
	sendMu      sync.Mutex
	mu          sync.Mutex
	cancels     map[string]*extUiMediaRequestState
	provisional map[string]*extUiMediaProvisionalLease // request id -> unacknowledged lease
	sem         chan struct{}
	closed      bool
	requestWG   sync.WaitGroup
	shutdown    sync.Once
}

type extUiMediaRequestState struct {
	cancel context.CancelFunc
}

type extUiMediaProvisionalLease struct {
	requestID  string
	resourceID string
	leaseID    string
	timer      *time.Timer
}

func (s *extUiMediaServer) serve(conn net.Conn, client *extUiMediaConn) {
	defer func() {
		client.cancelAll()
		_ = conn.Close()
		s.mu.Lock()
		if s.conn == conn {
			s.conn = nil
		}
		delete(s.clients, conn)
		s.mu.Unlock()
		s.serveWG.Done()
	}()
	for {
		message, err := extUiMediaReadMessage(conn)
		if err != nil {
			return
		}
		switch extUiString(message, "type") {
		case "request":
			client.start(message)
		case "cancel":
			client.cancel(extUiString(message, "requestId"))
		case "ack":
			client.ack(message)
		case "release":
			resourceID := extUiString(message, "resourceId")
			leaseID := extUiString(message, "leaseId")
			client.broker.Release(resourceID, leaseID)
			if requestID := extUiString(message, "requestId"); requestID != "" {
				_ = client.send(map[string]any{
					"type": "releaseAck", "requestId": requestID,
					"leaseId": leaseID, "ok": true,
				})
			}
		}
	}
}

func (c *extUiMediaConn) send(message map[string]any) error {
	c.sendMu.Lock()
	defer c.sendMu.Unlock()
	return extUiMediaSendMessage(c.conn, message)
}

func (c *extUiMediaConn) start(message map[string]any) {
	requestID := extUiString(message, "requestId")
	if requestID == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	state := &extUiMediaRequestState{cancel: cancel}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		cancel()
		return
	}
	old := c.cancels[requestID]
	orphan := c.provisional[requestID]
	delete(c.provisional, requestID)
	c.cancels[requestID] = state
	c.requestWG.Add(1)
	c.mu.Unlock()
	if old != nil {
		old.cancel()
	}
	if orphan != nil {
		orphan.timer.Stop()
		c.broker.Release(orphan.resourceID, orphan.leaseID)
	}
	go func() {
		defer c.requestWG.Done()
		select {
		case c.sem <- struct{}{}:
		case <-ctx.Done():
			c.respondError(requestID, ctx.Err())
			c.finishRequest(requestID, state)
			return
		}
		defer func() { <-c.sem }()
		defer c.finishRequest(requestID, state)
		c.handle(ctx, requestID, message)
	}()
}

func (c *extUiMediaConn) handle(ctx context.Context, requestID string, message map[string]any) {
	resourceID := extUiString(message, "resourceId")
	switch extUiString(message, "op") {
	case "readRange":
		data, profile, err := c.broker.ReadRange(ctx, resourceID, extUiMediaInt64(message["offset"]), extUiInt(message, "length"))
		if err != nil {
			c.respondError(requestID, err)
			return
		}
		_ = c.send(map[string]any{"type": "response", "requestId": requestID, "ok": true, "data": data, "accessProfile": profile})
	case "materialize":
		path, leaseID, size, profile, err := c.broker.Materialize(ctx, resourceID)
		if err != nil {
			c.respondError(requestID, err)
			return
		}
		if err := ctx.Err(); err != nil {
			c.broker.Release(resourceID, leaseID)
			c.respondError(requestID, err)
			return
		}
		provisional := c.provision(requestID, resourceID, leaseID)
		if provisional == nil {
			return
		}
		if err := c.send(map[string]any{"type": "response", "requestId": requestID, "ok": true, "path": path, "leaseId": leaseID, "size": size, "accessProfile": profile}); err != nil {
			c.dropProvisional(provisional)
		}
	default:
		c.respondError(requestID, errors.New("unsupported media operation"))
	}
}

func mediaErrorCode(err error) string {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "cancelled"
	case errors.Is(err, errMediaUnknownResource):
		return "staleResource"
	case errors.Is(err, errMediaSourceChanged):
		return "sourceChanged"
	case errors.Is(err, errMediaTooLarge):
		return "tooLarge"
	default:
		return "ioError"
	}
}

func (c *extUiMediaConn) respondError(requestID string, err error) {
	_ = c.send(map[string]any{
		"type": "response", "requestId": requestID, "ok": false,
		"errorCode": mediaErrorCode(err), "error": err.Error(),
	})
}

func (c *extUiMediaConn) provision(requestID, resourceID, leaseID string) *extUiMediaProvisionalLease {
	provisional := &extUiMediaProvisionalLease{requestID: requestID, resourceID: resourceID, leaseID: leaseID}
	provisional.timer = time.AfterFunc(extUiMediaLeaseAckTimeout, func() {
		c.dropProvisional(provisional)
	})
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		provisional.timer.Stop()
		c.broker.Release(resourceID, leaseID)
		return nil
	}
	previous := c.provisional[requestID]
	c.provisional[requestID] = provisional
	c.mu.Unlock()
	if previous != nil {
		previous.timer.Stop()
		c.broker.Release(previous.resourceID, previous.leaseID)
	}
	return provisional
}

func (c *extUiMediaConn) dropProvisional(provisional *extUiMediaProvisionalLease) {
	if provisional == nil {
		return
	}
	c.mu.Lock()
	if c.provisional[provisional.requestID] != provisional {
		c.mu.Unlock()
		return
	}
	delete(c.provisional, provisional.requestID)
	c.mu.Unlock()
	provisional.timer.Stop()
	c.broker.Release(provisional.resourceID, provisional.leaseID)
}

func (c *extUiMediaConn) ack(message map[string]any) {
	requestID := extUiString(message, "requestId")
	resourceID := extUiString(message, "resourceId")
	leaseID := extUiString(message, "leaseId")
	c.mu.Lock()
	provisional := c.provisional[requestID]
	if provisional != nil && (provisional.resourceID != resourceID || provisional.leaseID != leaseID) {
		provisional = nil
	}
	if provisional != nil {
		delete(c.provisional, requestID)
		provisional.timer.Stop()
	}
	c.mu.Unlock()

	ok := provisional != nil || c.broker.hasLease(resourceID, leaseID)
	response := map[string]any{"type": "ack", "requestId": requestID, "leaseId": leaseID, "ok": ok}
	if !ok {
		response["errorCode"] = "staleResource"
		response["error"] = "materialized lease is unknown or expired"
	}
	if err := c.send(response); err != nil && provisional != nil {
		// The client only exposes the path after receiving this confirmation.
		// If confirmation itself cannot be delivered, no one owns the lease.
		c.broker.Release(resourceID, leaseID)
	}
}

func (c *extUiMediaConn) finishRequest(requestID string, state *extUiMediaRequestState) {
	c.mu.Lock()
	if current := c.cancels[requestID]; current == state {
		delete(c.cancels, requestID)
	}
	c.mu.Unlock()
	state.cancel()
}

func (c *extUiMediaConn) cancel(requestID string) {
	c.mu.Lock()
	state := c.cancels[requestID]
	provisional := c.provisional[requestID]
	delete(c.provisional, requestID)
	c.mu.Unlock()
	if state != nil {
		state.cancel()
	}
	if provisional != nil {
		provisional.timer.Stop()
		c.broker.Release(provisional.resourceID, provisional.leaseID)
	}
}

func (c *extUiMediaConn) cancelAll() {
	c.shutdown.Do(func() {
		c.mu.Lock()
		c.closed = true
		cancels := make([]context.CancelFunc, 0, len(c.cancels))
		for _, state := range c.cancels {
			cancels = append(cancels, state.cancel)
		}
		provisional := make([]*extUiMediaProvisionalLease, 0, len(c.provisional))
		for _, lease := range c.provisional {
			provisional = append(provisional, lease)
		}
		c.cancels = make(map[string]*extUiMediaRequestState)
		c.provisional = make(map[string]*extUiMediaProvisionalLease)
		c.mu.Unlock()
		for _, cancel := range cancels {
			cancel()
		}
		for _, lease := range provisional {
			lease.timer.Stop()
			c.broker.Release(lease.resourceID, lease.leaseID)
		}
		c.requestWG.Wait()
	})
}

func (s *extUiMediaServer) Close() error {
	s.once.Do(func() {
		close(s.done)
		s.mu.Lock()
		s.closed = true
		s.conn = nil
		connections := make([]net.Conn, 0, len(s.pending)+len(s.clients))
		for conn := range s.pending {
			connections = append(connections, conn)
		}
		clients := make([]*extUiMediaConn, 0, len(s.clients))
		for conn, client := range s.clients {
			connections = append(connections, conn)
			clients = append(clients, client)
		}
		s.mu.Unlock()

		listenerErr := s.listener.Close()
		if errors.Is(listenerErr, net.ErrClosed) {
			listenerErr = nil
		}
		for _, conn := range connections {
			_ = conn.Close()
		}
		for _, client := range clients {
			client.cancelAll()
		}
		s.acceptWG.Wait()
		s.serveWG.Wait()
		brokerErr := s.broker.Close()
		s.mu.Lock()
		s.closeErr = errors.Join(listenerErr, brokerErr)
		s.mu.Unlock()
	})
	s.mu.Lock()
	err := s.closeErr
	s.mu.Unlock()
	return err
}

// Stable ordering is useful in tests and diagnostics without exposing paths.
func (b *extUiMediaBroker) resourceIDs() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	ids := make([]string, 0, len(b.resources))
	for id := range b.resources {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
