package winshell

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/unxed/f4/vfs"
)

type shellClient interface {
	Describe(context.Context, string) (Node, error)
	Enumerate(context.Context, string) ([]Node, error)
	CreateDir(context.Context, string, string) error
	Rename(context.Context, string, string) error
	Delete(context.Context, string, bool) error
	ImportPath(context.Context, string, string, string, bool) error
	Transfer(context.Context, string, string, string, bool) error
	Materialize(context.Context, string) (MaterializedFile, error)
}

type cacheEntry struct {
	node        Node
	displayName string
}

type shellCache struct {
	mu       sync.RWMutex
	byURI    map[string]cacheEntry
	children map[string]map[string]cacheEntry
}

func newShellCache() *shellCache {
	return &shellCache{
		byURI:    make(map[string]cacheEntry),
		children: make(map[string]map[string]cacheEntry),
	}
}

func cacheKey(value string) string { return strings.ToLower(value) }

func (c *shellCache) remember(node Node, displayName string) cacheEntry {
	if displayName == "" {
		displayName = node.Name
	}
	entry := cacheEntry{node: node, displayName: displayName}
	c.mu.Lock()
	c.byURI[cacheKey(node.URI)] = entry
	c.mu.Unlock()
	return entry
}

func (c *shellCache) rememberChildren(parentParsingName string, nodes []Node) []cacheEntry {
	counts := make(map[string]int, len(nodes))
	entries := make([]cacheEntry, 0, len(nodes))
	byName := make(map[string]cacheEntry, len(nodes))
	for _, node := range nodes {
		if node.Separator || node.URI == "" {
			continue
		}
		base := strings.TrimSpace(node.Name)
		if base == "" {
			base = node.ParsingName
		}
		nameKey := cacheKey(base)
		counts[nameKey]++
		display := base
		if counts[nameKey] > 1 {
			display = fmt.Sprintf("%s (%d)", base, counts[nameKey])
		}
		entry := cacheEntry{node: node, displayName: display}
		entries = append(entries, entry)
		byName[cacheKey(display)] = entry
	}
	c.mu.Lock()
	for _, entry := range entries {
		c.byURI[cacheKey(entry.node.URI)] = entry
	}
	c.children[cacheKey(parentParsingName)] = byName
	c.mu.Unlock()
	return entries
}

func (c *shellCache) entryByURI(uri string) (cacheEntry, bool) {
	c.mu.RLock()
	entry, ok := c.byURI[cacheKey(uri)]
	c.mu.RUnlock()
	return entry, ok
}

func (c *shellCache) child(parentParsingName, displayName string) (cacheEntry, bool) {
	c.mu.RLock()
	children := c.children[cacheKey(parentParsingName)]
	entry, ok := children[cacheKey(displayName)]
	c.mu.RUnlock()
	return entry, ok
}

func (c *shellCache) invalidate(parentParsingName string) {
	c.mu.Lock()
	delete(c.children, cacheKey(parentParsingName))
	c.mu.Unlock()
}

// ShellVFS presents arbitrary namespace items as a regular f4 filesystem.
// Paths remain persistent Windows URIs (with readable aliases for well-known
// roots) so virtual locations survive history/session restoration without
// being mistaken for host filesystem paths.
type ShellVFS struct {
	client shellClient
	cache  *shellCache

	mu      sync.RWMutex
	current Node
}

var _ vfs.VFS = (*ShellVFS)(nil)
var _ vfs.OptimisticPathSetter = (*ShellVFS)(nil)
var _ vfs.ServerSideCopier = (*ShellVFS)(nil)
var _ vfs.SessionIdentity = (*ShellVFS)(nil)
var _ vfs.TransferNameProvider = (*ShellVFS)(nil)
var _ vfs.TrashVFS = (*ShellVFS)(nil)

func newShellVFS(client shellClient, node Node, cache *shellCache) (*ShellVFS, error) {
	if client == nil {
		return nil, ErrUnavailable
	}
	if !node.Folder {
		return nil, fmt.Errorf("Windows Shell item %q is not a folder", node.Name)
	}
	if cache == nil {
		cache = newShellCache()
	}
	cache.remember(node, node.Name)
	return &ShellVFS{client: client, cache: cache, current: node}, nil
}

type uriProvider struct {
	client func() (*Client, error)
}

func (*uriProvider) Scheme() string { return Scheme }

func (p *uriProvider) OpenURI(ctx context.Context, _ vfs.VFS, raw string) (vfs.VFS, error) {
	parsingName, err := ParsingNameFromURI(raw)
	if err != nil {
		return nil, err
	}
	client, err := p.client()
	if err != nil {
		return nil, err
	}
	node, err := client.Describe(ctx, parsingName)
	if err != nil {
		return nil, err
	}
	return newShellVFS(client, node, nil)
}

func (s *ShellVFS) currentNode() Node {
	s.mu.RLock()
	node := s.current
	s.mu.RUnlock()
	return node
}

func (s *ShellVFS) IsAtRoot() bool {
	return s.currentNode().ParentParsingName == ""
}

func (s *ShellVFS) GetPath() string { return s.currentNode().URI }

func (*ShellVFS) IsAbs(path string) bool { return IsURI(path) }

func (s *ShellVFS) SetPath(path string) error {
	parsingName, err := ParsingNameFromURI(path)
	if err != nil {
		return err
	}
	node, err := s.client.Describe(context.Background(), parsingName)
	if err != nil {
		return err
	}
	if !node.Folder {
		return fmt.Errorf("Windows Shell item %q is not a folder", node.Name)
	}
	s.cache.remember(node, node.Name)
	s.mu.Lock()
	s.current = node
	s.mu.Unlock()
	return nil
}

func (s *ShellVFS) SetPathOptimistic(path string) error {
	entry, ok := s.cache.entryByURI(path)
	if !ok || !entry.node.Folder {
		return s.SetPath(path)
	}
	s.mu.Lock()
	s.current = entry.node
	s.mu.Unlock()
	return nil
}

func (s *ShellVFS) ReadDir(ctx context.Context, path string, onChunk func([]vfs.VFSItem)) error {
	parsingName, err := s.parsingName(path)
	if err != nil {
		return err
	}
	nodes, err := s.client.Enumerate(ctx, parsingName)
	if err != nil {
		return err
	}
	entries := s.cache.rememberChildren(parsingName, nodes)
	const chunkSize = 64
	for first := 0; first < len(entries); first += chunkSize {
		if err := ctx.Err(); err != nil {
			return err
		}
		last := min(first+chunkSize, len(entries))
		chunk := make([]vfs.VFSItem, 0, last-first)
		for _, entry := range entries[first:last] {
			chunk = append(chunk, nodeVFSItem(entry.node, entry.displayName))
		}
		if onChunk != nil {
			onChunk(chunk)
		}
	}
	return nil
}

func nodeVFSItem(node Node, name string) vfs.VFSItem {
	return vfs.VFSItem{
		Name:        name,
		Size:        node.Size,
		SizeKnown:   node.SizeKnown,
		IsDir:       node.Folder,
		MTime:       node.Modified,
		IsHidden:    node.Hidden,
		NoExtension: node.Folder && node.FileSystemPath == "",
	}
}

func (s *ShellVFS) Stat(ctx context.Context, path string) (vfs.VFSItem, error) {
	entry, err := s.resolveEntry(ctx, path)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	return nodeVFSItem(entry.node, entry.displayName), nil
}

func (s *ShellVFS) Join(elem ...string) string {
	if len(elem) == 0 {
		return s.GetPath()
	}
	result := s.GetPath()
	for index, part := range elem {
		if part == "" || part == "." {
			continue
		}
		if IsURI(part) {
			result = part
			continue
		}
		if index == 0 && IsURI(elem[0]) {
			result = elem[0]
			continue
		}
		if part == ".." {
			result = s.Dir(result)
			continue
		}
		parent, err := s.parsingName(result)
		if err != nil {
			return result
		}
		if entry, ok := s.cache.child(parent, part); ok {
			result = entry.node.URI
		} else {
			result = DestinationURI(parent, part)
		}
	}
	return result
}

func (s *ShellVFS) Abs(path string) (string, error) {
	if IsURI(path) {
		return path, nil
	}
	if strings.TrimSpace(path) == "" {
		return s.GetPath(), nil
	}
	return s.Join(s.GetPath(), path), nil
}

func (s *ShellVFS) Base(path string) string {
	if entry, ok := s.cache.entryByURI(path); ok {
		return entry.displayName
	}
	if _, name, err := DestinationFromURI(path); err == nil {
		return name
	}
	if path == s.GetPath() {
		return s.currentNode().Name
	}
	return path
}

func (s *ShellVFS) Dir(path string) string {
	if parent, _, err := DestinationFromURI(path); err == nil {
		return URIFromParsingName(parent)
	}
	if entry, ok := s.cache.entryByURI(path); ok {
		if entry.node.ParentParsingName == "" {
			return path
		}
		return URIFromParsingName(entry.node.ParentParsingName)
	}
	if path == s.GetPath() {
		node := s.currentNode()
		if node.ParentParsingName != "" {
			return URIFromParsingName(node.ParentParsingName)
		}
	}
	return path
}

func (s *ShellVFS) MkDir(ctx context.Context, path string) error {
	parent, name, err := s.destination(ctx, path)
	if err != nil {
		return err
	}
	if _, resolveErr := s.resolveEntry(ctx, path); resolveErr == nil {
		return os.ErrExist
	} else if !errors.Is(resolveErr, os.ErrNotExist) {
		return resolveErr
	}
	if err := s.client.CreateDir(ctx, parent, name); err != nil {
		return err
	}
	s.cache.invalidate(parent)
	return nil
}

func (s *ShellVFS) Remove(ctx context.Context, path string) error {
	entry, err := s.resolveEntry(ctx, path)
	if err != nil {
		return err
	}
	if !entry.node.CanDelete {
		return os.ErrPermission
	}
	if err := s.client.Delete(ctx, entry.node.ParsingName, false); err != nil {
		return err
	}
	s.cache.invalidate(entry.node.ParentParsingName)
	return nil
}

func (s *ShellVFS) MoveToTrash(ctx context.Context, path string) error {
	entry, err := s.resolveEntry(ctx, path)
	if err != nil {
		return err
	}
	if !entry.node.CanDelete {
		return os.ErrPermission
	}
	if err := s.client.Delete(ctx, entry.node.ParsingName, true); err != nil {
		return err
	}
	s.cache.invalidate(entry.node.ParentParsingName)
	return nil
}

func (s *ShellVFS) Rename(ctx context.Context, oldpath, newpath string) error {
	oldEntry, err := s.resolveEntry(ctx, oldpath)
	if err != nil {
		return err
	}
	if !oldEntry.node.CanRename && !oldEntry.node.CanMove {
		return os.ErrPermission
	}
	newParent, newName, err := s.destination(ctx, newpath)
	if err != nil {
		return err
	}
	if strings.EqualFold(newParent, oldEntry.node.ParentParsingName) {
		if err := s.client.Rename(ctx, oldEntry.node.ParsingName, newName); err != nil {
			return err
		}
		s.cache.invalidate(newParent)
		return nil
	}
	if err := s.client.Transfer(ctx, oldEntry.node.ParsingName, newParent, newName, true); err != nil {
		return err
	}
	s.cache.invalidate(oldEntry.node.ParentParsingName)
	s.cache.invalidate(newParent)
	return nil
}

func (s *ShellVFS) Copy(ctx context.Context, oldpath, newpath string) error {
	oldEntry, err := s.resolveEntry(ctx, oldpath)
	if err != nil {
		return err
	}
	parent, name, err := s.destination(ctx, newpath)
	if err != nil {
		return err
	}
	if err := s.client.Transfer(ctx, oldEntry.node.ParsingName, parent, name, false); err != nil {
		return err
	}
	s.cache.invalidate(parent)
	return nil
}

func (s *ShellVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{
		HasServerSideCopy: true,
		HasServerSideMove: true,
		HasRandomAccess:   true,
		HasWrite:          !s.currentNode().ReadOnly,
	}
}

func (*ShellVFS) Search(context.Context, string, string) (chan int64, error) {
	return nil, fmt.Errorf("Windows Shell search is not available")
}

type shellReader struct {
	file  *os.File
	size  int64
	path  string
	owned bool
	once  sync.Once
	err   error
}

func (r *shellReader) Size() int64 { return r.size }

func (r *shellReader) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.file.ReadAt(p, off)
}

func (r *shellReader) Read(ctx context.Context, p []byte) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.file.Read(p)
}

func (r *shellReader) Close() error {
	r.once.Do(func() {
		r.err = r.file.Close()
		if r.owned {
			r.err = errors.Join(r.err, removeIfPresent(r.path))
		}
	})
	return r.err
}

func (s *ShellVFS) Open(ctx context.Context, path string) (vfs.ReadAtCloser, error) {
	entry, err := s.resolveEntry(ctx, path)
	if err != nil {
		return nil, err
	}
	if entry.node.Folder {
		return nil, fmt.Errorf("cannot open Windows Shell folder %q as a file", entry.node.Name)
	}
	materialized, err := s.client.Materialize(ctx, entry.node.ParsingName)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(materialized.Path)
	if err != nil {
		if materialized.Owned {
			_ = removeIfPresent(materialized.Path)
		}
		return nil, err
	}
	return &shellReader{file: file, size: materialized.Size, path: materialized.Path, owned: materialized.Owned}, nil
}

type shellWriter struct {
	ctx    context.Context
	file   *os.File
	path   string
	client shellClient
	parent string
	name   string
	cache  *shellCache

	mu      sync.Mutex
	closed  bool
	aborted bool
	err     error
	done    chan struct{}
}

var _ vfs.AbortableWriter = (*shellWriter)(nil)
var _ vfs.ManagedTransferWriter = (*shellWriter)(nil)

func (*shellWriter) TransferProgressManaged() bool { return true }

func (w *shellWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, os.ErrClosed
	}
	if err := w.ctx.Err(); err != nil {
		w.err = err
		return 0, err
	}
	n, err := w.file.Write(p)
	if err != nil {
		w.err = err
	}
	return n, err
}

func (w *shellWriter) Close() error { return w.finish(false) }
func (w *shellWriter) Abort() error { return w.finish(true) }

func (w *shellWriter) finish(abort bool) error {
	w.mu.Lock()
	if w.closed {
		done := w.done
		w.mu.Unlock()
		<-done
		w.mu.Lock()
		err := w.err
		w.mu.Unlock()
		return err
	}
	w.closed = true
	w.aborted = abort
	w.mu.Unlock()

	err := w.file.Close()
	if err == nil && !abort {
		if ctxErr := w.ctx.Err(); ctxErr != nil {
			err = ctxErr
		} else {
			err = w.client.ImportPath(w.ctx, w.path, w.parent, w.name, false)
		}
	}
	err = errors.Join(err, removeIfPresent(w.path))
	if err == nil && !abort {
		w.cache.invalidate(w.parent)
	}
	w.mu.Lock()
	w.err = errors.Join(w.err, err)
	result := w.err
	close(w.done)
	w.mu.Unlock()
	return result
}

func (s *ShellVFS) Create(ctx context.Context, path string) (io.WriteCloser, error) {
	parent, name, err := s.destination(ctx, path)
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp("", "f4-shell-write-*")
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		_ = removeIfPresent(file.Name())
		return nil, err
	}
	return &shellWriter{
		ctx: ctx, file: file, path: file.Name(), client: s.client,
		parent: parent, name: name, cache: s.cache, done: make(chan struct{}),
	}, nil
}

func (s *ShellVFS) SetAttributes(ctx context.Context, path string, item vfs.VFSItem) error {
	entry, err := s.resolveEntry(ctx, path)
	if err != nil {
		return err
	}
	if entry.node.FileSystemPath == "" {
		return fmt.Errorf("attributes are not writable for virtual Windows Shell item %q", entry.node.Name)
	}
	host := vfs.NewOSVFS(filepath.Dir(entry.node.FileSystemPath))
	return host.SetAttributes(ctx, entry.node.FileSystemPath, item)
}

func (*ShellVFS) ParentVFS() vfs.VFS { return nil }

func (s *ShellVFS) Clone() vfs.VFS {
	clone, err := newShellVFS(s.client, s.currentNode(), s.cache)
	if err != nil {
		return s
	}
	return clone
}

func (*ShellVFS) Close() error { return nil }

func (s *ShellVFS) SessionKey() any { return s.client }

func (*ShellVFS) DirectoryCacheKey() any { return "windows-shell" }

func (s *ShellVFS) PanelTitle(path string) string {
	if entry, ok := s.cache.entryByURI(path); ok {
		return entry.node.Name
	}
	return s.currentNode().Name
}

func (s *ShellVFS) TransferName(srcPath string, _ vfs.VFS) string {
	if entry, ok := s.cache.entryByURI(srcPath); ok {
		return entry.node.Name
	}
	return s.Base(srcPath)
}

func (s *ShellVFS) parsingName(path string) (string, error) {
	if path == "" || path == "." {
		path = s.GetPath()
	}
	return ParsingNameFromURI(path)
}

func (s *ShellVFS) resolveEntry(ctx context.Context, path string) (cacheEntry, error) {
	if path == "" || path == "." {
		path = s.GetPath()
	}
	if entry, ok := s.cache.entryByURI(path); ok {
		return entry, nil
	}
	if parsingName, err := ParsingNameFromURI(path); err == nil {
		node, describeErr := s.client.Describe(ctx, parsingName)
		if describeErr != nil {
			return cacheEntry{}, describeErr
		}
		return s.cache.remember(node, node.Name), nil
	}
	parent, name, err := DestinationFromURI(path)
	if err != nil {
		return cacheEntry{}, err
	}
	if entry, ok := s.cache.child(parent, name); ok {
		return entry, nil
	}
	nodes, err := s.client.Enumerate(ctx, parent)
	if err != nil {
		return cacheEntry{}, err
	}
	entries := s.cache.rememberChildren(parent, nodes)
	for _, entry := range entries {
		if strings.EqualFold(entry.node.Name, name) || strings.EqualFold(entry.displayName, name) {
			return entry, nil
		}
	}
	return cacheEntry{}, os.ErrNotExist
}

func (s *ShellVFS) destination(ctx context.Context, path string) (parent, name string, err error) {
	if parent, name, err = DestinationFromURI(path); err == nil {
		return parent, name, nil
	}
	entry, resolveErr := s.resolveEntry(ctx, path)
	if resolveErr != nil {
		return "", "", resolveErr
	}
	if entry.node.ParentParsingName == "" || strings.TrimSpace(entry.node.Name) == "" {
		return "", "", fmt.Errorf("Windows Shell item %q has no writable parent", entry.node.Name)
	}
	return entry.node.ParentParsingName, entry.node.Name, nil
}

func removeIfPresent(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
