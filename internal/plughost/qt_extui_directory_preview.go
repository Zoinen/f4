package plughost

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/internal/mediatiming"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

const directoryPreviewFileLimit = 200
const directoryPreviewImageLimit = 16
const directoryPreviewTimeout = 30 * time.Second

var errDirectoryQuotaReached = errors.New("directory preview file quota reached")

// DirectorySourceDescriptor grants enumeration, never byte-read authority.
type DirectorySourceDescriptor struct {
	ResourceID string
	SourceKey  string
	Version    string
}

type directoryPreviewSource struct {
	id          string
	reg         MediaSourceRegistration
	fs          vfs.VFS
	vfsLeaseKey string
	laneKey     string
	valid       bool
	users       int
	ctx         context.Context
	cancel      context.CancelFunc
}

type directoryPreviewLease struct {
	source    *directoryPreviewSource
	items     []vfs.VFSItem
	children  []string
	timer     *time.Timer
	resolving bool
}

// All registry state except channel admission is protected by broker.mu.
type directoryPreviewRegistry struct {
	sources     map[string]*directoryPreviewSource
	panels      map[string]map[string]struct{}
	leases      map[string]*directoryPreviewLease
	retired     []*directoryPreviewSource
	lane        chan struct{}
	remoteLanes map[string]chan struct{}
}

func newDirectoryPreviewRegistry() *directoryPreviewRegistry {
	return &directoryPreviewRegistry{
		sources: make(map[string]*directoryPreviewSource), panels: make(map[string]map[string]struct{}),
		leases: make(map[string]*directoryPreviewLease), lane: make(chan struct{}, 2),
		remoteLanes: make(map[string]chan struct{}),
	}
}

func (r *directoryPreviewRegistry) cancelAll() {
	for _, source := range r.sources {
		source.cancel()
	}
	for _, source := range r.retired {
		source.cancel()
	}
	for _, lease := range r.leases {
		if lease.timer != nil {
			lease.timer.Stop()
		}
	}
}

func (b *ExtUiMediaBroker) RegisterDirectory(reg MediaSourceRegistration) DirectorySourceDescriptor {
	if reg.FS == nil || !reg.Item.IsDir || reg.Item.Name == ".." || reg.Item.Name == "." || reg.PanelID == "" {
		return DirectorySourceDescriptor{}
	}
	reg.Path = mediaCanonicalPath(reg.FS, reg.Path)
	key := MediaSourceKey(reg.FS, reg.Path)
	version := fmt.Sprintf("%s:%d", mediaAccessEpoch(reg.FS), reg.SourceEpoch)
	id := "directory-" + b.resourceID(key, version, reg.PanelID)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return DirectorySourceDescriptor{}
	}
	if source := b.directoryPreviews.sources[id]; source != nil {
		source.reg = reg
	} else {
		leaseKey, lease, err := b.vfsLeaseLocked(reg.FS)
		if err != nil {
			return DirectorySourceDescriptor{}
		}
		ctx, cancel := context.WithCancel(context.Background())
		laneKey := fmt.Sprintf("%T:%s", reg.FS, mediaAccessEpoch(reg.FS))
		b.directoryPreviews.sources[id] = &directoryPreviewSource{id: id, reg: reg, fs: lease.fs, vfsLeaseKey: leaseKey, laneKey: laneKey, ctx: ctx, cancel: cancel}
	}
	return DirectorySourceDescriptor{ResourceID: id, SourceKey: key, Version: version}
}

func (b *ExtUiMediaBroker) CommitDirectoryPanel(panelID string, revision int64, ids []string) {
	next := directoryPreviewIDSet(ids)
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	r := b.directoryPreviews
	var release []string
	for id, source := range r.sources {
		if source.reg.PanelID != panelID {
			continue
		}
		_, keep := next[id]
		if keep && source.reg.CatalogVersion == revision {
			source.valid = true
			continue
		}
		source.valid = false
		source.cancel()
		delete(r.sources, id)
		r.retired = append(r.retired, source)
		for leaseID, lease := range r.leases {
			if lease.source == source {
				release = append(release, leaseID)
			}
		}
	}
	r.panels[panelID] = next
	toClose := b.collectRetiredDirectoriesLocked()
	b.mu.Unlock()
	for _, leaseID := range release {
		b.releaseDirectoryPreview("", leaseID)
	}
	closeDirectoryVFS(toClose)
}

// CommitDirectoryPanelPage admits the directory authorities encountered in a
// paged catalog response without treating that response as the complete
// catalog. A semantic catalog page is often only a viewport-sized slice; an
// incomplete page must not revoke previews for folders which have not been
// requested in this revision yet.
func (b *ExtUiMediaBroker) CommitDirectoryPanelPage(panelID string, revision int64, ids []string, complete bool) {
	if complete {
		vtui.DebugLog("[FIX:directory-preview-page] complete panel=%s revision=%d ids=%d",
			panelID, revision, len(ids))
		b.CommitDirectoryPanel(panelID, revision, ids)
		return
	}
	next := directoryPreviewIDSet(ids)
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	r := b.directoryPreviews
	merged := make(map[string]struct{}, len(r.panels[panelID])+len(next))
	for id := range r.panels[panelID] {
		merged[id] = struct{}{}
	}
	accepted := 0
	for id := range next {
		source := r.sources[id]
		if source == nil || source.reg.PanelID != panelID || source.reg.CatalogVersion != revision {
			continue
		}
		source.valid = true
		merged[id] = struct{}{}
		accepted++
	}
	r.panels[panelID] = merged
	b.mu.Unlock()
	vtui.DebugLog("[FIX:directory-preview-page] partial panel=%s revision=%d page=%d accepted=%d retained=%d",
		panelID, revision, len(ids), accepted, len(merged))
}

func directoryPreviewIDSet(ids []string) map[string]struct{} {
	set := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id != "" {
			set[id] = struct{}{}
		}
	}
	return set
}

func (b *ExtUiMediaBroker) retainDirectoryPanels(panelIDs []string) {
	keep := make(map[string]bool, len(panelIDs))
	for _, id := range panelIDs {
		keep[id] = true
	}
	b.mu.Lock()
	var removed []string
	for id := range b.directoryPreviews.panels {
		if !keep[id] {
			removed = append(removed, id)
		}
	}
	b.mu.Unlock()
	for _, id := range removed {
		b.CommitDirectoryPanel(id, 0, nil)
		b.mu.Lock()
		delete(b.directoryPreviews.panels, id)
		b.mu.Unlock()
	}
}

func (b *ExtUiMediaBroker) collectRetiredDirectoriesLocked() []vfs.VFS {
	if b.closed {
		return nil
	}
	r := b.directoryPreviews
	retained := r.retired[:0]
	var toClose []vfs.VFS
	for _, source := range r.retired {
		if source.users != 0 {
			retained = append(retained, source)
			continue
		}
		if lease := b.vfsLeases[source.vfsLeaseKey]; lease != nil {
			lease.refs--
			if lease.refs == 0 {
				delete(b.vfsLeases, source.vfsLeaseKey)
				if lease.owned {
					toClose = append(toClose, lease.fs)
				}
			}
		}
	}
	r.retired = retained
	usedLanes := make(map[string]bool)
	for _, source := range r.sources {
		usedLanes[source.laneKey] = true
	}
	for _, source := range r.retired {
		usedLanes[source.laneKey] = true
	}
	for key := range r.remoteLanes {
		if !usedLanes[key] {
			delete(r.remoteLanes, key)
		}
	}
	return toClose
}

func closeDirectoryVFS(filesystems []vfs.VFS) {
	for _, fs := range filesystems {
		_ = fs.Close()
	}
}

func (b *ExtUiMediaBroker) beginDirectoryOperation(ctx context.Context, id string) (*directoryPreviewSource, context.Context, func(), error) {
	b.mu.Lock()
	r := b.directoryPreviews
	source := r.sources[id]
	if b.closed || source == nil || !source.valid {
		b.mu.Unlock()
		return nil, nil, nil, errMediaUnknownResource
	}
	source.users++
	b.operationWG.Add(1)
	var remote chan struct{}
	if MediaStorageClass(source.fs.GetCapabilities(), "") != vfs.StorageClassLocal {
		remote = r.remoteLanes[source.laneKey]
		if remote == nil {
			remote = make(chan struct{}, 1)
			r.remoteLanes[source.laneKey] = remote
		}
	}
	b.mu.Unlock()
	workCtx, cancel := context.WithTimeout(ctx, directoryPreviewTimeout)
	stop := context.AfterFunc(source.ctx, cancel)
	globalHeld, remoteHeld := false, false
	queuedAt := time.Now()
	finish := func() {
		stop()
		cancel()
		if globalHeld {
			<-r.lane
		}
		if remoteHeld {
			<-remote
		}
		b.mu.Lock()
		source.users--
		toClose := b.collectRetiredDirectoriesLocked()
		b.mu.Unlock()
		closeDirectoryVFS(toClose)
		b.operationWG.Done()
	}
	// Acquire the source lane first: queued requests to one slow server must
	// not consume both global slots and starve independent directories.
	if remote != nil {
		select {
		case remote <- struct{}{}:
			remoteHeld = true
		case <-workCtx.Done():
			finish()
			return nil, nil, nil, workCtx.Err()
		}
	}
	select {
	case r.lane <- struct{}{}:
		globalHeld = true
	case <-workCtx.Done():
		finish()
		return nil, nil, nil, workCtx.Err()
	}
	mediatiming.MediaTimingEmit(ctx, "directory.admitted", "go.media", "resourceId", id, "queueMs", time.Since(queuedAt).Milliseconds())
	return source, workCtx, finish, nil
}

func readDirectoryPreview(ctx context.Context, fs vfs.VFS, path string) ([]vfs.VFSItem, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	scanCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	items := make([]vfs.VFSItem, 0, directoryPreviewFileLimit)
	var consumeMu sync.Mutex
	finished := false
	err := fs.ReadDir(scanCtx, path, func(chunk []vfs.VFSItem) {
		consumeMu.Lock()
		defer consumeMu.Unlock()
		if finished {
			return
		}
		for _, item := range chunk {
			if len(items) == directoryPreviewFileLimit || ctx.Err() != nil {
				return
			}
			if item.IsDir {
				continue
			}
			items = append(items, item)
			if len(items) == directoryPreviewFileLimit {
				cancel(errDirectoryQuotaReached)
				return
			}
		}
	})
	consumeMu.Lock()
	defer consumeMu.Unlock()
	finished = true
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if len(items) == directoryPreviewFileLimit {
		return items, nil
	}
	if err != nil {
		return nil, err
	}
	return items, nil
}

func newDirectoryLeaseID() string {
	var token [16]byte
	if _, err := rand.Read(token[:]); err != nil {
		panic(err)
	}
	return fmt.Sprintf("directory-lease-%x", token)
}

func (b *ExtUiMediaBroker) EnumerateDirectoryPreview(ctx context.Context, resourceID string) ([]vfs.VFSItem, string, error) {
	source, workCtx, finish, err := b.beginDirectoryOperation(ctx, resourceID)
	if err != nil {
		return nil, "", err
	}
	defer finish()
	b.mu.Lock()
	path := source.reg.Path
	b.mu.Unlock()
	items, err := readDirectoryPreview(workCtx, source.fs, path)
	mediatiming.MediaTimingEmit(ctx, "directory.enumerated", "go.media", "resourceId", resourceID, "files", len(items), "error", mediatiming.MediaTimingError(err))
	if err != nil {
		return nil, "", err
	}
	id := newDirectoryLeaseID()
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || !source.valid || workCtx.Err() != nil {
		return nil, "", errMediaUnknownResource
	}
	lease := &directoryPreviewLease{source: source, items: items}
	source.users++
	b.directoryPreviews.leases[id] = lease
	mediatiming.MediaTimingEmit(ctx, "directory.lease.created", "go.media", "resourceId", resourceID, "leaseId", id, "kind", "listing")
	lease.timer = time.AfterFunc(directoryPreviewTimeout, func() { b.releaseDirectoryPreview(resourceID, id) })
	return items, id, nil
}

type DirectoryPreviewImage struct {
	Name   string
	Source ImageSourceDescriptor
}

func (b *ExtUiMediaBroker) ResolveDirectoryPreview(ctx context.Context, resourceID, listingID string, names []string) ([]DirectoryPreviewImage, string, error) {
	if len(names) > directoryPreviewImageLimit {
		return nil, "", errMediaTooLarge
	}
	source, workCtx, finish, err := b.beginDirectoryOperation(ctx, resourceID)
	if err != nil {
		return nil, "", err
	}
	defer finish()
	b.mu.Lock()
	listing := b.directoryPreviews.leases[listingID]
	if listing == nil || listing.source != source || listing.items == nil || listing.resolving {
		b.mu.Unlock()
		return nil, "", errMediaUnknownResource
	}
	items := append([]vfs.VFSItem(nil), listing.items...)
	listing.resolving = true
	reg := source.reg
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		if b.directoryPreviews.leases[listingID] == listing {
			listing.resolving = false
		}
		b.mu.Unlock()
	}()
	selected := make(map[string]vfs.VFSItem, len(items))
	for _, item := range items {
		selected[item.Name] = item
	}
	result := make([]DirectoryPreviewImage, 0, len(names))
	var children []string
	defer func() {
		if children != nil {
			b.dropPreviewReferences(children)
		}
	}()
	for _, name := range names {
		expected, ok := selected[name]
		if !ok || name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") {
			return nil, "", errMediaUnknownResource
		}
		delete(selected, name)
		path := source.fs.Join(reg.Path, name)
		if source.fs.Dir(path) != source.fs.Dir(source.fs.Join(reg.Path, ".preview-child")) || source.fs.Base(path) != name {
			return nil, "", errMediaUnknownResource
		}
		item, statErr := source.fs.Stat(workCtx, path)
		if statErr != nil {
			return nil, "", statErr
		}
		if item.IsDir || item.Size != expected.Size || item.SizeKnown != expected.SizeKnown || !item.MTime.Equal(expected.MTime) || item.Revision != expected.Revision {
			return nil, "", errMediaSourceChanged
		}
		childReg := reg
		childReg.Path, childReg.Item = path, item
		descriptor := b.registerMediaSource(childReg, true)
		if descriptor.ResourceID == "" {
			return nil, "", errMediaUnknownResource
		}
		children = append(children, descriptor.ResourceID)
		result = append(result, DirectoryPreviewImage{Name: name, Source: descriptor})
	}
	id := newDirectoryLeaseID()
	b.mu.Lock()
	if b.closed || !source.valid || workCtx.Err() != nil || b.directoryPreviews.leases[listingID] != listing {
		b.mu.Unlock()
		return nil, "", errMediaUnknownResource
	}
	source.users++
	b.directoryPreviews.leases[id] = &directoryPreviewLease{source: source, children: children}
	children = nil
	b.mu.Unlock()
	mediatiming.MediaTimingEmit(ctx, "directory.lease.created", "go.media", "resourceId", resourceID, "leaseId", id, "kind", "preview")
	b.releaseDirectoryPreview(resourceID, listingID)
	mediatiming.MediaTimingEmit(ctx, "directory.resolved", "go.media", "resourceId", resourceID, "selected", len(result))
	return result, id, nil
}

func (b *ExtUiMediaBroker) dropPreviewReferences(ids []string) {
	var retired []*ExtUiMediaResource
	b.mu.Lock()
	for _, id := range ids {
		if resource := b.resources[id]; resource != nil {
			resource.mu.Lock()
			if resource.validRefs > 0 {
				resource.validRefs--
			}
			if resource.validRefs == 0 {
				resource.releasePending = true
				retired = append(retired, resource)
			}
			resource.mu.Unlock()
		}
	}
	b.mu.Unlock()
	for _, resource := range retired {
		b.retireResource(resource)
	}
}

func (b *ExtUiMediaBroker) releaseDirectoryPreview(resourceID, leaseID string) bool {
	if !strings.HasPrefix(leaseID, "directory-lease-") {
		return false
	}
	b.mu.Lock()
	lease := b.directoryPreviews.leases[leaseID]
	if lease == nil || (resourceID != "" && lease.source.id != resourceID) {
		b.mu.Unlock()
		return true
	}
	delete(b.directoryPreviews.leases, leaseID)
	if lease.timer != nil {
		lease.timer.Stop()
	}
	lease.source.users--
	toClose := b.collectRetiredDirectoriesLocked()
	b.mu.Unlock()
	b.dropPreviewReferences(lease.children)
	mediatiming.MediaTimingEmit(context.Background(), "directory.lease.released", "go.media", "resourceId", lease.source.id, "leaseId", leaseID)
	closeDirectoryVFS(toClose)
	return true
}
