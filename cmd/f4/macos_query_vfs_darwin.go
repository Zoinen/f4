//go:build darwin

package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

var errMacOSQueryReadOnly = errors.New("a macOS search view has no destination folder")

// NSMetadataQuery exposes filesystem dates as NSDate values. NSDate stores its
// interval as a floating-point number, while os.Stat returns APFS' integer
// nanoseconds. Converting the same timestamp through those two APIs can differ
// by a few hundred nanoseconds (44 ns on the Red-tagged live-test image). Keep
// the query snapshot's timestamp when the live stat is equivalent at NSDate's
// practical precision; otherwise return the live value so the media broker
// still rejects genuinely changed content.
const macOSQueryMTimeTolerance = time.Microsecond

func macOSQueryEquivalentMTime(listed, actual time.Time) bool {
	if listed.IsZero() {
		// Spotlight can omit the modification date. In that case the query's
		// source version intentionally falls back to the remaining metadata.
		return true
	}
	delta := actual.Sub(listed)
	if actual.Before(listed) {
		delta = listed.Sub(actual)
	}
	return delta <= macOSQueryMTimeTolerance
}

type macOSQueryEntry struct {
	item          vfs.VFSItem
	realPath      string
	virtualURI    string
	queryKind     string
	queryTag      string
	action        string
	serviceName   string
	serviceType   string
	serviceDomain string
	scheme        string
}

type macOSQueryVFS struct {
	mu        sync.RWMutex
	uri       string
	kind      string
	tag       string
	title     string
	parent    vfs.VFS
	client    *platformIPCClient
	entries   map[string]*macOSQueryEntry
	readStop  context.CancelFunc
	watchStop context.CancelFunc
	readGen   uint64
	watchGen  uint64
	closed    bool
}

var _ vfs.VFS = (*macOSQueryVFS)(nil)
var _ vfs.LocalPathProvider = (*macOSQueryVFS)(nil)
var _ vfs.TransferNameProvider = (*macOSQueryVFS)(nil)
var _ vfs.TitleProvider = (*macOSQueryVFS)(nil)
var _ vfs.PanelTitleProvider = (*macOSQueryVFS)(nil)
var _ vfs.PanelActionHandler = (*macOSQueryVFS)(nil)
var _ vfs.DirectoryWatcher = (*macOSQueryVFS)(nil)

func newMacOSQueryVFS(uri, kind, tag, title string, parent vfs.VFS) *macOSQueryVFS {
	return &macOSQueryVFS{
		uri: uri, kind: kind, tag: tag, title: title, parent: parent,
		client: currentPlatformIPC(), entries: make(map[string]*macOSQueryEntry),
	}
}

func (q *macOSQueryVFS) IsAtRoot() bool  { return true }
func (q *macOSQueryVFS) GetPath() string { return q.uri }
func (q *macOSQueryVFS) IsAbs(value string) bool {
	return strings.HasPrefix(value, "macos://")
}
func (q *macOSQueryVFS) SetPath(value string) error {
	if value == q.uri || strings.TrimSuffix(value, "/") == strings.TrimSuffix(q.uri, "/") {
		return nil
	}
	return fmt.Errorf("%s is a search result, not a directory in %s", value, q.uri)
}
func (q *macOSQueryVFS) Join(elem ...string) string {
	if len(elem) == 0 {
		return q.uri
	}
	last := elem[len(elem)-1]
	if strings.HasPrefix(last, "macos://") {
		return last
	}
	if last == "" || last == "." {
		return q.uri
	}
	if last == ".." {
		return q.uri
	}
	return strings.TrimSuffix(q.uri, "/") + "/" + path.Base(last)
}
func (q *macOSQueryVFS) Abs(value string) (string, error) {
	if q.IsAbs(value) {
		return value, nil
	}
	return q.Join(value), nil
}
func (q *macOSQueryVFS) Base(value string) string {
	if strings.TrimSuffix(value, "/") == strings.TrimSuffix(q.uri, "/") {
		return q.title
	}
	return path.Base(value)
}
func (q *macOSQueryVFS) Dir(value string) string {
	if q.IsAbs(value) {
		return q.uri
	}
	return path.Dir(value)
}

func platformItems(value any) []map[string]any {
	raw, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]map[string]any); ok {
			return typed
		}
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if mapped := platformMessageMap(item); mapped != nil {
			out = append(out, mapped)
		}
	}
	return out
}

func platformInt64(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int8:
		return int64(typed)
	case int16:
		return int64(typed)
	case int32:
		return int64(typed)
	case int64:
		return typed
	case uint:
		return int64(typed)
	case uint8:
		return int64(typed)
	case uint16:
		return int64(typed)
	case uint32:
		return int64(typed)
	case uint64:
		if typed <= uint64(^uint64(0)>>1) {
			return int64(typed)
		}
	}
	return 0
}

func macOSQuerySyntheticName(id, realPath string, isDir bool) string {
	if id == "" {
		digest := sha256.Sum256([]byte(realPath))
		id = fmt.Sprintf("item-%x", digest[:10])
	}
	id = strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || r >= '0' && r <= '9' ||
			r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' {
			return r
		}
		return '-'
	}, id)
	if isDir {
		return id
	}
	return id + filepath.Ext(realPath)
}

func (q *macOSQueryVFS) decodeEntry(raw map[string]any) (*macOSQueryEntry, bool) {
	realPath := platformAnyString(raw["path"])
	virtualURI := platformAnyString(raw["uri"])
	queryKind := platformAnyString(raw["queryKind"])
	queryTag := platformAnyString(raw["tag"])
	isVirtualQuery := virtualURI != "" && queryKind != ""
	action := platformAnyString(raw["action"])
	isNetwork := platformAnyBool(raw["networkService"])
	if realPath == "" && action == "" && !isNetwork && !isVirtualQuery {
		return nil, false
	}
	isDir := platformAnyBool(raw["isDir"]) || isVirtualQuery
	display := platformAnyString(raw["displayName"])
	if display == "" {
		display = platformAnyString(raw["label"])
	}
	if display == "" {
		display = filepath.Base(realPath)
	}
	identityPath := realPath
	if identityPath == "" {
		identityPath = virtualURI
	}
	name := macOSQuerySyntheticName(platformAnyString(raw["id"]), identityPath, isDir)
	mtimeNanos := platformInt64(raw["mtimeNanos"])
	mtime := time.Time{}
	if mtimeNanos != 0 {
		mtime = time.Unix(0, mtimeNanos)
	}
	entry := &macOSQueryEntry{
		item: vfs.VFSItem{
			Name: name, DisplayName: display, IsDir: isDir,
			Size: platformInt64(raw["size"]), SizeKnown: platformAnyBool(raw["sizeKnown"]),
			MTime: mtime, NoExtension: action != "" || isVirtualQuery,
		},
		realPath: realPath, virtualURI: virtualURI,
		queryKind: queryKind, queryTag: queryTag, action: action,
		serviceName:   platformAnyString(raw["serviceName"]),
		serviceType:   platformAnyString(raw["serviceType"]),
		serviceDomain: platformAnyString(raw["serviceDomain"]),
		scheme:        platformAnyString(raw["scheme"]),
	}
	return entry, true
}

func (q *macOSQueryVFS) ReadDir(
	ctx context.Context, _ string, onChunk func([]vfs.VFSItem),
) error {
	if q.client == nil || !q.client.Available() {
		return errPlatformServicesUnavailable
	}
	readCtx, cancel := context.WithCancel(ctx)
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		cancel()
		return os.ErrClosed
	}
	if q.readStop != nil {
		q.readStop()
	}
	if q.watchStop != nil {
		q.watchStop()
		q.watchStop = nil
	}
	q.watchGen++
	q.readStop = cancel
	q.readGen++
	readGen := q.readGen
	q.mu.Unlock()
	nextEntries := make(map[string]*macOSQueryEntry)
	defer func() {
		q.mu.Lock()
		if q.readGen == readGen {
			q.readStop = nil
		}
		q.mu.Unlock()
		cancel()
	}()

	payload := map[string]any{"kind": q.kind}
	if q.tag != "" {
		payload["tag"] = q.tag
	}
	err := q.client.Request(readCtx, "macos.query", payload, func(response platformResponse) error {
		if response.Error != nil {
			return nil
		}
		rawItems := platformItems(response.Payload["items"])
		items := make([]vfs.VFSItem, 0, len(rawItems))
		q.mu.Lock()
		for _, raw := range rawItems {
			entry, ok := q.decodeEntry(raw)
			if !ok {
				continue
			}
			nextEntries[entry.item.Name] = entry
			q.entries[entry.item.Name] = entry
			items = append(items, entry.item)
		}
		q.mu.Unlock()
		if len(items) != 0 && onChunk != nil {
			onChunk(items)
		}
		return nil
	})
	if err != nil {
		return err
	}
	q.mu.Lock()
	if !q.closed && q.readGen == readGen {
		q.entries = nextEntries
	}
	q.mu.Unlock()
	return nil
}

func (q *macOSQueryVFS) WatchDirectory(
	ctx context.Context, _ string, onChange func(),
) error {
	if q.kind == "network" {
		return nil
	}
	if q.client == nil || !q.client.Available() {
		return errPlatformServicesUnavailable
	}
	watchCtx, cancel := context.WithCancel(ctx)
	q.mu.Lock()
	if q.closed {
		q.mu.Unlock()
		cancel()
		return os.ErrClosed
	}
	if q.watchStop != nil {
		q.watchStop()
	}
	q.watchStop = cancel
	q.watchGen++
	watchGen := q.watchGen
	q.mu.Unlock()
	defer func() {
		q.mu.Lock()
		if q.watchGen == watchGen {
			q.watchStop = nil
		}
		q.mu.Unlock()
		cancel()
	}()

	payload := map[string]any{"kind": q.kind}
	if q.tag != "" {
		payload["tag"] = q.tag
	}
	return q.client.Request(watchCtx, "macos.watch", payload, func(response platformResponse) error {
		if response.Event && platformAnyBool(response.Payload["refresh"]) && onChange != nil {
			onChange()
		}
		return nil
	})
}

func (q *macOSQueryVFS) lookup(value string) (*macOSQueryEntry, error) {
	name := path.Base(value)
	q.mu.RLock()
	entry := q.entries[name]
	q.mu.RUnlock()
	if entry == nil {
		return nil, os.ErrNotExist
	}
	return entry, nil
}

func (q *macOSQueryVFS) Stat(_ context.Context, value string) (vfs.VFSItem, error) {
	if strings.TrimSuffix(value, "/") == strings.TrimSuffix(q.uri, "/") {
		return vfs.VFSItem{
			Name: q.Base(q.uri), DisplayName: q.title, IsDir: true,
			SizeKnown: true,
		}, nil
	}
	entry, err := q.lookup(value)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	if entry.realPath == "" {
		return entry.item, nil
	}
	info, err := os.Stat(entry.realPath)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	item := entry.item
	actualMTime := info.ModTime()
	if macOSQueryEquivalentMTime(entry.item.MTime, actualMTime) {
		actualMTime = entry.item.MTime
	}
	item.Size, item.SizeKnown, item.IsDir, item.MTime = info.Size(), true, info.IsDir(), actualMTime
	item.Mode = info.Mode().String()
	return item, nil
}

func (q *macOSQueryVFS) MkDir(context.Context, string) error { return errMacOSQueryReadOnly }
func (q *macOSQueryVFS) Create(context.Context, string) (io.WriteCloser, error) {
	return nil, errMacOSQueryReadOnly
}
func (q *macOSQueryVFS) Remove(_ context.Context, value string) error {
	entry, err := q.lookup(value)
	if err != nil {
		return err
	}
	if entry.realPath == "" {
		return errMacOSQueryReadOnly
	}
	if err := os.RemoveAll(entry.realPath); err != nil {
		return err
	}
	q.mu.Lock()
	delete(q.entries, entry.item.Name)
	q.mu.Unlock()
	return nil
}
func (q *macOSQueryVFS) Rename(_ context.Context, oldValue, newValue string) error {
	entry, err := q.lookup(oldValue)
	if err != nil {
		return err
	}
	if entry.realPath == "" {
		return errMacOSQueryReadOnly
	}
	newName := path.Base(newValue)
	if existing, lookupErr := q.lookup(newValue); lookupErr == nil {
		newName = existing.item.DisplayName
	}
	if newName == "" || newName == "." || newName == ".." {
		return os.ErrInvalid
	}
	target := filepath.Join(filepath.Dir(entry.realPath), newName)
	if err := os.Rename(entry.realPath, target); err != nil {
		return err
	}
	entry.realPath = target
	entry.item.DisplayName = newName
	return nil
}
func (q *macOSQueryVFS) SetAttributes(ctx context.Context, value string, item vfs.VFSItem) error {
	entry, err := q.lookup(value)
	if err != nil {
		return err
	}
	if entry.realPath == "" {
		return errMacOSQueryReadOnly
	}
	local := vfs.NewOSVFS(filepath.Dir(entry.realPath))
	return local.SetAttributes(ctx, entry.realPath, item)
}
func (q *macOSQueryVFS) Open(ctx context.Context, value string) (vfs.ReadAtCloser, error) {
	entry, err := q.lookup(value)
	if err != nil {
		return nil, err
	}
	if entry.realPath == "" || entry.item.IsDir {
		return nil, os.ErrInvalid
	}
	local := vfs.NewOSVFS(filepath.Dir(entry.realPath))
	return local.Open(ctx, entry.realPath)
}
func (q *macOSQueryVFS) Search(context.Context, string, string) (chan int64, error) {
	return nil, errors.New("searching inside a macOS query view is unsupported")
}
func (q *macOSQueryVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{
		HasRandomAccess: true,
		ReadAccess:      vfs.ReadAccessDirectLocal,
		StorageClass:    vfs.StorageClassVirtual,
	}
}
func (q *macOSQueryVFS) ParentVFS() vfs.VFS { return q.parent }
func (q *macOSQueryVFS) Clone() vfs.VFS {
	clone := newMacOSQueryVFS(q.uri, q.kind, q.tag, q.title, nil)
	if q.parent != nil {
		clone.parent = q.parent.Clone()
	}
	q.mu.RLock()
	for name, entry := range q.entries {
		copied := *entry
		clone.entries[name] = &copied
	}
	q.mu.RUnlock()
	return clone
}
func (q *macOSQueryVFS) Close() error {
	q.mu.Lock()
	q.closed = true
	if q.readStop != nil {
		q.readStop()
		q.readStop = nil
	}
	if q.watchStop != nil {
		q.watchStop()
		q.watchStop = nil
	}
	q.watchGen++
	q.mu.Unlock()
	return nil
}
func (q *macOSQueryVFS) LocalPath(value string) (string, error) {
	entry, err := q.lookup(value)
	if err != nil {
		return "", err
	}
	if entry.realPath == "" {
		return "", os.ErrNotExist
	}
	return entry.realPath, nil
}
func (q *macOSQueryVFS) TransferName(value string, _ vfs.VFS) string {
	entry, err := q.lookup(value)
	if err != nil || entry.realPath == "" {
		return path.Base(value)
	}
	return filepath.Base(entry.realPath)
}
func (q *macOSQueryVFS) GetTitle() string         { return "macOS" }
func (q *macOSQueryVFS) PanelTitle(string) string { return q.title }

func (q *macOSQueryVFS) HandlePanelAction(app vfs.App, action vfs.PanelAction, values []string) bool {
	if action != vfs.PanelActionActivate || len(values) == 0 {
		return false
	}
	entry, err := q.lookup(values[0])
	if err != nil || entry.action != "connectServer" {
		return false
	}
	q.connectToServer(app)
	return true
}

func (q *macOSQueryVFS) connectToServer(app vfs.App) {
	panels, ok := app.(*PanelsFrame)
	if !ok {
		return
	}
	panels.InputBox(" Connect to Server ", "Server URL (smb://, afp://, nfs://, WebDAV):", "macos.network.server", func(server string) {
		server = strings.TrimSpace(server)
		if server == "" {
			return
		}
		frames := vtui.FrameManager
		go func() {
			mountPath, err := q.mount(context.Background(), server)
			frames.PostTask(func() {
				if err != nil {
					vtui.ShowMessage(" Connection Error ", err.Error(), []string{"&Ok"})
					return
				}
				for _, panel := range panels.panels {
					if fsp, ok := panel.(*FileSystemPanel); ok && fsp.vfs == q {
						panels.switchToVFS(fsp, vfs.NewOSVFS(mountPath))
						return
					}
				}
			})
		}()
	})
}

func (q *macOSQueryVFS) mount(ctx context.Context, server string) (string, error) {
	return q.mountPayload(ctx, map[string]any{"url": server})
}

func (q *macOSQueryVFS) mountPayload(
	ctx context.Context, payload map[string]any,
) (string, error) {
	if q.client == nil || !q.client.Available() {
		return "", errPlatformServicesUnavailable
	}
	mountPath := ""
	err := q.client.Request(ctx, "macos.mount", payload, func(response platformResponse) error {
		var values []any
		switch typed := response.Payload["mountPaths"].(type) {
		case []any:
			values = typed
		case []string:
			values = make([]any, len(typed))
			for i := range typed {
				values[i] = typed[i]
			}
		}
		for _, value := range values {
			if candidate := platformAnyString(value); candidate != "" && mountPath == "" {
				mountPath = candidate
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if mountPath == "" {
		return "", errors.New("the server returned no mounted volume")
	}
	return mountPath, nil
}

type macOSQueryDirectoryProvider struct{}

func (macOSQueryDirectoryProvider) Name() string                  { return "macOS query result" }
func (macOSQueryDirectoryProvider) Priority() int                 { return 1000 }
func (macOSQueryDirectoryProvider) OpensVirtualDirectories() bool { return true }
func (macOSQueryDirectoryProvider) CanOpen(_ context.Context, parent vfs.VFS, value string) bool {
	query, ok := parent.(*macOSQueryVFS)
	if !ok {
		return false
	}
	entry, err := query.lookup(value)
	return err == nil && entry.item.IsDir
}
func (macOSQueryDirectoryProvider) Open(ctx context.Context, parent vfs.VFS, value string) (vfs.VFS, error) {
	query, ok := parent.(*macOSQueryVFS)
	if !ok {
		return nil, os.ErrInvalid
	}
	entry, err := query.lookup(value)
	if err != nil {
		return nil, err
	}
	if entry.virtualURI != "" {
		if entry.queryKind == "" {
			return nil, os.ErrInvalid
		}
		return newMacOSQueryVFS(entry.virtualURI, entry.queryKind, entry.queryTag,
			entry.item.DisplayName, query), nil
	}
	localPath := entry.realPath
	if localPath == "" && entry.serviceName != "" {
		localPath, err = query.mountPayload(ctx, map[string]any{
			"serviceName": entry.serviceName, "serviceType": entry.serviceType,
			"serviceDomain": entry.serviceDomain, "scheme": entry.scheme,
		})
		if err != nil {
			return nil, err
		}
	}
	if localPath == "" {
		return nil, os.ErrNotExist
	}
	local := vfs.NewOSVFS(localPath)
	return &macOSParentedOSVFS{
		OSVFS: local, parent: query, root: local.GetPath(),
	}, nil
}

type macOSParentedOSVFS struct {
	*vfs.OSVFS
	parent vfs.VFS
	root   string
}

func (v *macOSParentedOSVFS) ParentVFS() vfs.VFS { return v.parent }
func (v *macOSParentedOSVFS) IsAtRoot() bool {
	return filepath.Clean(v.GetPath()) == filepath.Clean(v.root)
}
func (v *macOSParentedOSVFS) Clone() vfs.VFS {
	var parent vfs.VFS
	if v.parent != nil {
		parent = v.parent.Clone()
	}
	return &macOSParentedOSVFS{
		OSVFS: vfs.NewOSVFS(v.GetPath()), parent: parent, root: v.root,
	}
}

func init() {
	vfs.RegisterProvider(macOSQueryDirectoryProvider{})
}
