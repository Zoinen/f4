package cloudfox

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
)

// CloudVFS adapts one authenticated Backend session to f4's VFS interface.
// It has an independent cwd while clones share the pooled remote session.
type CloudVFS struct {
	connection Connection
	manager    *ManagerVFS
	session    *pooledSession
	location   string

	mu      sync.RWMutex
	aliases map[string]RemoteEntry // canonical parent + NUL + displayed name
	reverse map[string]string      // canonical location -> displayed name
	entries map[string]RemoteEntry // canonical location -> complete entry metadata
	remaps  map[string]string      // guessed/obsolete location -> learned stable identity
	closed  bool
}

func newCloudVFS(connection Connection, manager *ManagerVFS, session *pooledSession, location string) (*CloudVFS, error) {
	backend := session.backendSnapshot()
	if backend == nil {
		return nil, errors.New("cloudfox: backend session is unavailable")
	}
	normalized, err := backend.Normalize(location)
	if err != nil {
		return nil, err
	}
	return &CloudVFS{
		connection: connection.Clone(), manager: manager, session: session, location: normalized,
		aliases: make(map[string]RemoteEntry), reverse: make(map[string]string), entries: make(map[string]RemoteEntry), remaps: make(map[string]string),
	}, nil
}

func (c *CloudVFS) canonicalLocation(backend Backend, location string) string {
	for range 32 {
		previous := location
		c.mu.RLock()
		if mapped := c.remaps[location]; mapped != "" {
			location = mapped
		}
		c.mu.RUnlock()
		if canonicalizer, ok := backend.(BackendCanonicalizer); ok {
			if mapped := canonicalizer.CanonicalLocation(location); mapped != "" {
				location = mapped
			}
		}
		if location == previous {
			break
		}
	}
	return location
}

func (c *CloudVFS) rememberCanonicalLocation(backend Backend, requested string) string {
	actual := c.canonicalLocation(backend, requested)
	if actual == "" || actual == requested {
		return requested
	}
	c.mu.Lock()
	if !c.closed {
		c.remaps[requested] = actual
	}
	c.mu.Unlock()
	return actual
}

func (c *CloudVFS) forgetLocation(location string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, location)
	delete(c.reverse, location)
	delete(c.remaps, location)
	for key, entry := range c.aliases {
		if entry.Location == location {
			delete(c.aliases, key)
		}
	}
}

func (c *CloudVFS) backend() (Backend, error) {
	c.mu.RLock()
	closed := c.closed
	session := c.session
	c.mu.RUnlock()
	if closed || session == nil {
		return nil, os.ErrClosed
	}
	backend := session.backendSnapshot()
	if backend == nil {
		return nil, os.ErrClosed
	}
	return backend, nil
}

// operation retains the pooled session for the duration of one backend call
// and merges caller cancellation with plugin/session shutdown. This prevents a
// panel close from tearing down a backend under an active request, while pool
// shutdown still interrupts network I/O promptly.
func (c *CloudVFS) operation(ctx context.Context) (Backend, context.Context, func(), error) {
	c.mu.RLock()
	closed, session := c.closed, c.session
	c.mu.RUnlock()
	if closed || session == nil || session.pool == nil || !session.pool.retain(session) {
		return nil, nil, nil, os.ErrClosed
	}
	backend := session.backendSnapshot()
	if backend == nil {
		session.pool.release(session)
		return nil, nil, nil, os.ErrClosed
	}
	lifetime := session.pool.lifetime()
	opCtx, cancel := providerOperationContext(ctx, lifetime)
	opCtx = context.WithValue(opCtx, providerSessionLifetimeContextKey{}, lifetime)
	var once sync.Once
	finish := func() {
		once.Do(func() {
			cancel()
			session.pool.release(session)
		})
	}
	return backend, opCtx, finish, nil
}

type cloudReadHandle struct {
	vfs.ReadAtCloser
	finish func()
	once   sync.Once
	err    error
}

func (r *cloudReadHandle) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	// The context passed to CloudVFS.Open belongs only to opening the handle
	// (often a short-lived progress task). Each read has its own caller
	// context; the provider reader itself owns the handle lifetime and Close
	// cancels it. Reusing the Open context here makes every later range read
	// fail as soon as the progress task completes.
	return r.ReadAtCloser.ReadAt(ctx, p, off)
}

func (r *cloudReadHandle) Read(ctx context.Context, p []byte) (int, error) {
	return r.ReadAtCloser.Read(ctx, p)
}

func (r *cloudReadHandle) LocalPath() (string, bool) {
	local, ok := r.ReadAtCloser.(interface{ LocalPath() (string, bool) })
	if !ok {
		return "", false
	}
	return local.LocalPath()
}

func (r *cloudReadHandle) Close() error {
	r.once.Do(func() {
		r.err = r.ReadAtCloser.Close()
		r.finish()
	})
	return r.err
}

type cloudWriteHandle struct {
	io.WriteCloser
	finish      func()
	abortFinish func()
	once        sync.Once
	err         error
}

func (w *cloudWriteHandle) TransferProgressManaged() bool {
	managed, ok := w.WriteCloser.(vfs.ManagedTransferWriter)
	return ok && managed.TransferProgressManaged()
}

func (w *cloudWriteHandle) Close() error {
	w.once.Do(func() {
		w.err = w.WriteCloser.Close()
		w.finish()
	})
	return w.err
}

func (w *cloudWriteHandle) Abort() error {
	w.once.Do(func() {
		aborter, ok := w.WriteCloser.(vfs.AbortableWriter)
		if !ok {
			w.err = ErrUnsupportedOperation
		} else {
			w.err = aborter.Abort()
		}
		if w.abortFinish != nil {
			w.abortFinish()
		} else {
			w.finish()
		}
	})
	return w.err
}

func (c *CloudVFS) IsAtRoot() bool {
	backend, err := c.backend()
	return err == nil && backend.IsRoot(c.currentLocation())
}

func (c *CloudVFS) ManagedTransferWrites() bool {
	// These providers stage locally and perform the network commit from Close,
	// with byte-accurate ReporterKey progress. Google and S3 stream while Write
	// is running, so treating them as a later phase would double-count.
	return c.connection.Provider == ProviderYandexDisk || c.connection.Provider == ProviderWebDAV
}

func (*CloudVFS) RemoteTransfer() bool { return true }

func (c *CloudVFS) GetPath() string {
	backend, err := c.backend()
	if err != nil {
		return c.visualPath(nil)
	}
	return c.visualPath(c.visualPartsForLocation(backend, c.currentLocation()))
}

func (c *CloudVFS) GetTitle() string { return c.connection.Name }

func (c *CloudVFS) PanelTitle(p string) string {
	backend, err := c.backend()
	if err != nil {
		return c.connection.Name
	}
	if parts, ok := c.visualAbsoluteParts(p); ok {
		return c.visualPath(parts)
	}
	location := c.currentLocation()
	if u, parseErr := ParseURI(p); parseErr == nil && u.Provider == c.connection.Provider && strings.EqualFold(u.ConnectionID, c.connection.ID) {
		location = u.Location
	}
	return c.visualPath(c.visualPartsForLocation(backend, location))
}

func (c *CloudVFS) visualRootPrefix() string {
	return c.connection.Name + ":" + string(os.PathSeparator)
}

func (c *CloudVFS) visualPath(parts []string) string {
	root := c.visualRootPrefix()
	if len(parts) == 0 {
		return root
	}
	return root + strings.Join(parts, string(os.PathSeparator))
}

func visualPathParts(raw string) []string {
	return applyVisualPathParts(nil, raw)
}

func applyVisualPathParts(base []string, raw string) []string {
	fields := strings.FieldsFunc(raw, func(r rune) bool { return r == '/' || r == '\\' })
	parts := append([]string(nil), base...)
	for _, field := range fields {
		switch field {
		case "", ".":
			continue
		case "..":
			if len(parts) != 0 {
				parts = parts[:len(parts)-1]
			}
		default:
			parts = append(parts, field)
		}
	}
	return parts
}

func (c *CloudVFS) visualAbsoluteParts(p string) ([]string, bool) {
	prefix := c.connection.Name + ":"
	if !strings.HasPrefix(p, prefix) {
		return nil, false
	}
	rest := p[len(prefix):]
	if rest != "" && rest[0] != '/' && rest[0] != '\\' {
		return nil, false
	}
	return visualPathParts(rest), true
}

func (c *CloudVFS) visualPartsForLocation(backend Backend, location string) []string {
	type visualLink struct {
		parent, name, location string
	}
	parts := make([]string, 0, 8)
	links := make([]visualLink, 0, 8)
	seen := make(map[string]struct{})
	for !backend.IsRoot(location) {
		if location == "" {
			break
		}
		if _, duplicate := seen[location]; duplicate {
			break
		}
		seen[location] = struct{}{}

		c.mu.RLock()
		name := c.reverse[location]
		c.mu.RUnlock()
		if name == "" {
			name = backend.Base(location)
		}
		if name != "" && name != "." && name != "/" && name != "\\" {
			parts = append(parts, name)
		}
		parent := backend.Dir(location)
		if name != "" && name != "." && name != "/" && name != "\\" {
			links = append(links, visualLink{parent: parent, name: name, location: location})
		}
		if parent == location {
			break
		}
		location = parent
	}
	for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
		parts[left], parts[right] = parts[right], parts[left]
	}
	// Every visual path we expose must be reversible without leaking the
	// opaque location back into the caller. Remember the parent/name mapping as
	// part of producing that path; the actual provider API still receives only
	// the internal location resolved from this table.
	c.mu.Lock()
	if !c.closed {
		for _, link := range links {
			key := aliasKey(link.parent, link.name)
			if _, exists := c.aliases[key]; !exists {
				entry := c.entries[link.location]
				entry.Name = link.name
				entry.Location = link.location
				c.aliases[key] = entry
			}
			if c.reverse[link.location] == "" {
				c.reverse[link.location] = link.name
			}
		}
	}
	c.mu.Unlock()
	return parts
}

func (c *CloudVFS) currentLocation() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.location
}

func (c *CloudVFS) IsAbs(p string) bool {
	if _, ok := c.visualAbsoluteParts(p); ok {
		return true
	}
	u, err := ParseURI(p)
	return err == nil && u.Provider == c.connection.Provider && strings.EqualFold(u.ConnectionID, c.connection.ID)
}

func (c *CloudVFS) SetPath(p string) error {
	location, err := c.resolvePath(p)
	if err != nil {
		return err
	}
	backend, err := c.backend()
	if err != nil {
		return err
	}
	location, err = backend.Normalize(location)
	if err != nil {
		return err
	}
	location = c.canonicalLocation(backend, location)
	c.mu.Lock()
	c.location = location
	c.mu.Unlock()
	return nil
}

func (c *CloudVFS) SetPathOptimistic(p string) error { return c.SetPath(p) }

func aliasKey(parent, name string) string { return parent + "\x00" + name }

func (c *CloudVFS) resolvePath(p string) (string, error) {
	backend, err := c.backend()
	if err != nil {
		return "", err
	}
	if u, parseErr := ParseURI(p); parseErr == nil && u.Provider == c.connection.Provider && strings.EqualFold(u.ConnectionID, c.connection.ID) {
		normalized, err := backend.Normalize(u.Location)
		if err != nil {
			return "", err
		}
		return c.canonicalLocation(backend, normalized), nil
	}
	if p == "" || p == "." {
		return c.currentLocation(), nil
	}
	if parts, ok := c.visualAbsoluteParts(p); ok {
		if p == c.GetPath() {
			return c.currentLocation(), nil
		}
		return c.locationForVisualParts(backend, parts)
	}
	base := c.visualPartsForLocation(backend, c.currentLocation())
	parts := applyVisualPathParts(base, p)
	return c.locationForVisualParts(backend, parts)
}

func (c *CloudVFS) locationForVisualParts(backend Backend, parts []string) (string, error) {
	location := backend.Root()
	for _, name := range parts {
		c.mu.RLock()
		entry, ok := c.aliases[aliasKey(location, name)]
		c.mu.RUnlock()
		if ok {
			location = c.canonicalLocation(backend, entry.Location)
			continue
		}
		location = backend.Join(c.canonicalLocation(backend, location), name)
	}
	normalized, err := backend.Normalize(location)
	if err != nil {
		return "", err
	}
	return c.canonicalLocation(backend, normalized), nil
}

func (c *CloudVFS) publicPath(location string) string {
	backend, err := c.backend()
	if err != nil {
		return c.visualPath(nil)
	}
	return c.visualPath(c.visualPartsForLocation(backend, location))
}

func (c *CloudVFS) canonicalURI(location string) string {
	return URI{Provider: c.connection.Provider, ConnectionID: c.connection.ID, Location: location}.String()
}

func (c *CloudVFS) Join(elem ...string) string {
	backend, err := c.backend()
	if err != nil || len(elem) == 0 {
		return ""
	}
	var parts []string
	start := 0
	if visual, ok := c.visualAbsoluteParts(elem[0]); ok {
		parts = append(parts, visual...)
		start = 1
	} else if u, parseErr := ParseURI(elem[0]); parseErr == nil && u.Provider == c.connection.Provider && strings.EqualFold(u.ConnectionID, c.connection.ID) {
		parts = c.visualPartsForLocation(backend, u.Location)
		start = 1
	} else {
		parts = c.visualPartsForLocation(backend, c.currentLocation())
	}
	for _, piece := range elem[start:] {
		parts = applyVisualPathParts(parts, piece)
	}
	return c.visualPath(parts)
}

func (c *CloudVFS) Abs(p string) (string, error) {
	location, err := c.resolvePath(p)
	if err != nil {
		return "", err
	}
	return c.publicPath(location), nil
}

func (c *CloudVFS) Base(p string) string {
	if parts, ok := c.visualAbsoluteParts(p); ok {
		if len(parts) == 0 {
			return c.connection.Name
		}
		return parts[len(parts)-1]
	}
	location, err := c.resolvePath(p)
	if err != nil {
		return ""
	}
	c.mu.RLock()
	name := c.reverse[location]
	c.mu.RUnlock()
	if name != "" {
		return name
	}
	backend, err := c.backend()
	if err != nil {
		return ""
	}
	return backend.Base(location)
}

func (c *CloudVFS) Dir(p string) string {
	if parts, ok := c.visualAbsoluteParts(p); ok {
		if len(parts) != 0 {
			parts = parts[:len(parts)-1]
		}
		return c.visualPath(parts)
	}
	location, err := c.resolvePath(p)
	if err != nil {
		return c.GetPath()
	}
	backend, err := c.backend()
	if err != nil {
		return c.GetPath()
	}
	return c.publicPath(backend.Dir(location))
}

func (c *CloudVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	location, err := c.resolvePath(p)
	if err != nil {
		return err
	}
	backend, opCtx, finish, err := c.operation(ctx)
	if err != nil {
		return err
	}
	defer finish()
	// A duplicate may arrive on a later provider page. Buffer one directory
	// snapshot so every member of a duplicate-name group receives a stable
	// alias before any row becomes visible or enters the lookup cache.
	var pages [][]RemoteEntry
	err = backend.ReadDir(opCtx, location, func(entries []RemoteEntry) {
		if len(entries) != 0 {
			pages = append(pages, append([]RemoteEntry(nil), entries...))
		}
	})
	if err != nil {
		return err
	}
	pages = disambiguateEntries(pages, location, backend)
	itemPages := make([][]vfs.VFSItem, 0, len(pages))
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return os.ErrClosed
	}
	for key := range c.aliases {
		if strings.HasPrefix(key, location+"\x00") {
			delete(c.aliases, key)
		}
	}
	for _, entries := range pages {
		items := make([]vfs.VFSItem, 0, len(entries))
		for _, entry := range entries {
			entry.VFSItem.SizeKnown = entry.SizeKnown || entry.Size != 0 || entry.IsDir
			c.aliases[aliasKey(location, entry.Name)] = entry
			c.reverse[entry.Location] = entry.Name
			c.entries[entry.Location] = entry
			items = append(items, entry.VFSItem)
		}
		itemPages = append(itemPages, items)
	}
	c.mu.Unlock()
	for _, items := range itemPages {
		if len(items) != 0 && onChunk != nil {
			onChunk(items)
		}
	}
	return nil
}

// restoreVisualPath resolves a persisted user-facing directory path one
// component at a time. This is the only path restoration step that needs
// provider I/O; after it completes normal VFS operations translate cached
// display names to opaque backend locations immediately before each API call.
func (c *CloudVFS) restoreVisualPath(ctx context.Context, parts []string) error {
	for _, name := range parts {
		parent := c.currentLocation()
		if err := c.ReadDir(ctx, c.GetPath(), func([]vfs.VFSItem) {}); err != nil {
			return err
		}
		c.mu.RLock()
		entry, ok := c.aliases[aliasKey(parent, name)]
		c.mu.RUnlock()
		if !ok {
			return fmt.Errorf("cloudfox: folder %q does not exist: %w", name, os.ErrNotExist)
		}
		if !entry.IsDir {
			return fmt.Errorf("cloudfox: %q is not a folder: %w", name, os.ErrInvalid)
		}
		backend, err := c.backend()
		if err != nil {
			return err
		}
		location := c.canonicalLocation(backend, entry.Location)
		c.mu.Lock()
		c.location = location
		c.mu.Unlock()
	}
	return nil
}

func disambiguateEntries(pages [][]RemoteEntry, parent string, backend Backend) [][]RemoteEntry {
	type position struct{ page, item int }
	groups := make(map[string][]position)
	seenIdentity := make(map[string]struct{})
	usedNames := make(map[string]struct{})

	filtered := make([][]RemoteEntry, 0, len(pages))
	for _, page := range pages {
		out := make([]RemoteEntry, 0, len(page))
		for _, entry := range page {
			if entry.Name == "" {
				continue
			}
			originalName := entry.Name
			if entry.Location == "" {
				entry.Location = backend.Join(parent, originalName)
			}
			normalized, err := backend.Normalize(entry.Location)
			if err != nil {
				continue
			}
			entry.Location = normalized
			entry.Name = safeCloudPanelName(originalName, normalized)
			identity := originalName + "\x00" + normalized
			if _, duplicatePageEntry := seenIdentity[identity]; duplicatePageEntry {
				continue
			}
			seenIdentity[identity] = struct{}{}
			if entry.TransferName == "" {
				entry.TransferName = originalName
			}
			pageIndex := len(filtered)
			itemIndex := len(out)
			groups[entry.Name] = append(groups[entry.Name], position{pageIndex, itemIndex})
			usedNames[entry.Name] = struct{}{}
			out = append(out, entry)
		}
		filtered = append(filtered, out)
	}

	groupNames := make([]string, 0, len(groups))
	for name, positions := range groups {
		if len(positions) > 1 {
			groupNames = append(groupNames, name)
		}
	}
	sort.Strings(groupNames)
	for _, name := range groupNames {
		positions := groups[name]
		sort.SliceStable(positions, func(i, j int) bool {
			a := filtered[positions[i].page][positions[i].item].Location
			b := filtered[positions[j].page][positions[j].item].Location
			return a < b
		})
		for _, pos := range positions {
			entry := &filtered[pos.page][pos.item]
			hash := stableLocationHash(entry.Location)
			alias := ""
			for length := 6; length <= len(hash); length += 2 {
				alias = fmt.Sprintf("%s [%s]", name, hash[:min(length, len(hash))])
				if _, exists := usedNames[alias]; !exists {
					break
				}
				alias = ""
			}
			if alias == "" {
				for suffix := 2; ; suffix++ {
					candidate := fmt.Sprintf("%s [%s-%d]", name, hash, suffix)
					if _, exists := usedNames[candidate]; !exists {
						alias = candidate
						break
					}
				}
			}
			entry.Name = alias
			usedNames[alias] = struct{}{}
		}
	}
	return filtered
}

func safeCloudPanelName(name, location string) string {
	original := name
	var b strings.Builder
	for _, r := range name {
		switch r {
		case '/':
			b.WriteRune('∕')
		case '\\':
			b.WriteRune('⧵')
		case 0:
			b.WriteRune('�')
		default:
			if r < 0x20 || r == 0x7f {
				b.WriteRune('�')
			} else {
				b.WriteRune(r)
			}
		}
	}
	name = b.String()
	if name == "." {
		name = "．"
	} else if name == ".." {
		name = "．．"
	} else if name == "" {
		name = "unnamed"
	}
	if name != original {
		name += " [" + stableLocationHash(location)[:6] + "]"
	}
	return name
}

func stableLocationHash(location string) string {
	sum := sha256.Sum256([]byte(location))
	return strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(sum[:10]))
}

func (c *CloudVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	location, err := c.resolvePath(p)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	backend, opCtx, finish, err := c.operation(ctx)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	defer finish()
	entry, err := backend.Stat(opCtx, location)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	entry.VFSItem.SizeKnown = entry.SizeKnown || entry.Size != 0 || entry.IsDir
	if entry.Location != "" {
		actual := entry.Location
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			return vfs.VFSItem{}, os.ErrClosed
		}
		if cached, ok := c.entries[entry.Location]; ok {
			if entry.TransferName == "" {
				entry.TransferName = cached.TransferName
			}
			if entry.Name == "" {
				entry.Name = cached.Name
			}
		}
		c.reverse[entry.Location] = entry.Name
		c.entries[entry.Location] = entry
		if actual != location {
			c.remaps[location] = actual
		}
		c.mu.Unlock()
	}
	return entry.VFSItem, nil
}

func (c *CloudVFS) MkDir(ctx context.Context, p string) error {
	location, err := c.resolvePath(p)
	if err != nil {
		return err
	}
	backend, opCtx, finish, err := c.operation(ctx)
	if err != nil {
		return err
	}
	defer finish()
	if err := backend.MkDir(opCtx, location); err != nil {
		return err
	}
	c.rememberCanonicalLocation(backend, location)
	return nil
}

func (c *CloudVFS) Remove(ctx context.Context, p string) error {
	location, err := c.resolvePath(p)
	if err != nil {
		return err
	}
	backend, opCtx, finish, err := c.operation(ctx)
	if err != nil {
		return err
	}
	defer finish()
	if err := backend.Remove(opCtx, location); err != nil {
		return err
	}
	c.forgetLocation(location)
	return nil
}

type trashCloudVFS struct{ *CloudVFS }

func wrapCloudVFS(cloud *CloudVFS) vfs.VFS {
	backend, err := cloud.backend()
	if err == nil {
		if _, ok := backend.(BackendTrasher); ok {
			return &trashCloudVFS{CloudVFS: cloud}
		}
	}
	return cloud
}

func (c *trashCloudVFS) MoveToTrash(ctx context.Context, p string) error {
	location, err := c.resolvePath(p)
	if err != nil {
		return err
	}
	backend, opCtx, finish, err := c.operation(ctx)
	if err != nil {
		return err
	}
	defer finish()
	trasher := backend.(BackendTrasher) // guaranteed by wrapCloudVFS
	if err := trasher.MoveToTrash(opCtx, location); err != nil {
		return err
	}
	c.forgetLocation(location)
	return nil
}

func (c *trashCloudVFS) Clone() vfs.VFS {
	cloned := c.CloudVFS.Clone()
	clone, ok := cloned.(*CloudVFS)
	if !ok {
		return cloned
	}
	return wrapCloudVFS(clone)
}

func (c *CloudVFS) Rename(ctx context.Context, oldpath, newpath string) error {
	oldLocation, err := c.resolvePath(oldpath)
	if err != nil {
		return err
	}
	newLocation, err := c.resolvePath(newpath)
	if err != nil {
		return err
	}
	backend, opCtx, finish, err := c.operation(ctx)
	if err != nil {
		return err
	}
	defer finish()
	if err := backend.Rename(opCtx, oldLocation, newLocation); err != nil {
		return err
	}
	actual := c.rememberCanonicalLocation(backend, newLocation)
	c.mu.Lock()
	if actual != "" && actual != newLocation {
		c.remaps[newLocation] = actual
	}
	delete(c.entries, oldLocation)
	delete(c.reverse, oldLocation)
	for key, entry := range c.aliases {
		if entry.Location == oldLocation {
			delete(c.aliases, key)
		}
	}
	c.mu.Unlock()
	return nil
}

func (c *CloudVFS) Copy(ctx context.Context, oldpath, newpath string) error {
	oldLocation, err := c.resolvePath(oldpath)
	if err != nil {
		return err
	}
	newLocation, err := c.resolvePath(newpath)
	if err != nil {
		return err
	}
	backend, opCtx, finish, err := c.operation(ctx)
	if err != nil {
		return err
	}
	defer finish()
	copier, ok := backend.(BackendCopier)
	if !ok {
		return errors.New("cloudfox: server-side copy is unsupported")
	}
	if err := copier.Copy(opCtx, oldLocation, newLocation); err != nil {
		return err
	}
	c.rememberCanonicalLocation(backend, newLocation)
	return nil
}

func (c *CloudVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	location, err := c.resolvePath(p)
	if err != nil {
		return nil, err
	}
	backend, opCtx, finish, err := c.operation(ctx)
	if err != nil {
		return nil, err
	}
	reader, err := backend.Open(opCtx, location)
	if err != nil {
		finish()
		return nil, err
	}
	return &cloudReadHandle{ReadAtCloser: reader, finish: finish}, nil
}

func (c *CloudVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	location, err := c.resolvePath(p)
	if err != nil {
		return nil, err
	}
	backend, opCtx, finish, err := c.operation(ctx)
	if err != nil {
		return nil, err
	}
	writer, err := backend.Create(opCtx, location)
	if err != nil {
		finish()
		return nil, err
	}
	return &cloudWriteHandle{WriteCloser: writer, abortFinish: finish, finish: func() {
		c.rememberCanonicalLocation(backend, location)
		finish()
	}}, nil
}

func (c *CloudVFS) SetAttributes(ctx context.Context, p string, item vfs.VFSItem) error {
	location, err := c.resolvePath(p)
	if err != nil {
		return err
	}
	backend, opCtx, finish, err := c.operation(ctx)
	if err != nil {
		return err
	}
	defer finish()
	return backend.SetAttributes(opCtx, location, item)
}

func validateShareURL(raw string) error {
	if err := vfs.ValidateShareURL(raw); err != nil {
		return errors.New("cloudfox: provider returned an invalid share URL")
	}
	return nil
}

func validateHTTPSShareURL(raw string) error {
	if err := validateShareURL(raw); err != nil {
		return err
	}
	target, err := url.Parse(raw)
	if err != nil || !strings.EqualFold(target.Scheme, "https") {
		return errors.New("cloudfox: provider returned a non-HTTPS share URL")
	}
	return nil
}

func (c *CloudVFS) ShareLinkInfo(ctx context.Context, p string) (vfs.ShareLinkInfo, error) {
	location, err := c.resolvePath(p)
	if err != nil {
		return vfs.ShareLinkInfo{}, err
	}
	backend, opCtx, finish, err := c.operation(ctx)
	if err != nil {
		return vfs.ShareLinkInfo{}, err
	}
	defer finish()
	linker, ok := backend.(BackendShareLinker)
	if !ok {
		return vfs.ShareLinkInfo{}, ErrShareLinksUnsupported
	}
	info, err := linker.ShareLinkInfo(opCtx, location)
	if err != nil {
		return vfs.ShareLinkInfo{}, err
	}
	if info.Link != nil {
		if err := validateShareURL(info.Link.URL); err != nil {
			return vfs.ShareLinkInfo{}, err
		}
		copy := *info.Link
		info.Link = &copy
	}
	info.Roles = append([]vfs.ShareRole(nil), info.Roles...)
	info.ExpirationOptions = append([]time.Duration(nil), info.ExpirationOptions...)
	return info, nil
}

func (c *CloudVFS) CreateShareLink(ctx context.Context, p string, request vfs.ShareLinkRequest) (vfs.ShareLink, error) {
	location, err := c.resolvePath(p)
	if err != nil {
		return vfs.ShareLink{}, err
	}
	backend, opCtx, finish, err := c.operation(ctx)
	if err != nil {
		return vfs.ShareLink{}, err
	}
	defer finish()
	linker, ok := backend.(BackendShareLinker)
	if !ok {
		return vfs.ShareLink{}, ErrShareLinksUnsupported
	}
	issuedAfter := time.Now()
	link, err := linker.CreateShareLink(opCtx, location, request)
	if err != nil {
		return vfs.ShareLink{}, err
	}
	if err := vfs.ValidateCreatedShareLink(link, request, issuedAfter, time.Now()); err != nil {
		return vfs.ShareLink{}, &vfs.UnknownOperationStateError{Operation: "create cloud share link", Err: err}
	}
	return link, nil
}

func (c *CloudVFS) RevokeShareLink(ctx context.Context, p string) error {
	location, err := c.resolvePath(p)
	if err != nil {
		return err
	}
	backend, opCtx, finish, err := c.operation(ctx)
	if err != nil {
		return err
	}
	defer finish()
	linker, ok := backend.(BackendShareLinker)
	if !ok {
		return ErrShareLinksUnsupported
	}
	return linker.RevokeShareLink(opCtx, location)
}

func (c *CloudVFS) GetCapabilities() vfs.VFSCapabilities {
	backend, err := c.backend()
	if err != nil {
		return vfs.VFSCapabilities{}
	}
	return backend.Capabilities()
}

func (c *CloudVFS) Search(context.Context, string, string) (chan int64, error) {
	return nil, errors.New("cloudfox: server-side search is unsupported")
}

func (c *CloudVFS) ParentVFS() vfs.VFS { return c.manager }

func (c *CloudVFS) Clone() vfs.VFS {
	c.mu.RLock()
	if c.closed || c.session == nil || !c.session.pool.retain(c.session) {
		c.mu.RUnlock()
		return c
	}
	clone := &CloudVFS{
		connection: c.connection.Clone(), manager: c.manager, session: c.session, location: c.location,
		aliases: make(map[string]RemoteEntry, len(c.aliases)), reverse: make(map[string]string, len(c.reverse)), entries: make(map[string]RemoteEntry, len(c.entries)), remaps: make(map[string]string, len(c.remaps)),
	}
	for key, entry := range c.aliases {
		clone.aliases[key] = entry
	}
	for key, name := range c.reverse {
		clone.reverse[key] = name
	}
	for key, entry := range c.entries {
		clone.entries[key] = entry
	}
	for key, location := range c.remaps {
		clone.remaps[key] = location
	}
	c.mu.RUnlock()
	return clone
}

func (c *CloudVFS) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	session := c.session
	c.session = nil
	c.aliases = nil
	c.reverse = nil
	c.entries = nil
	c.remaps = nil
	c.mu.Unlock()
	if session != nil {
		session.pool.release(session)
	}
	return nil
}

func (c *CloudVFS) SessionKey() any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.session
}

type cloudDirectoryCacheKey struct {
	Provider  ProviderType
	ID        string
	UpdatedAt int64
}

func (c *CloudVFS) DirectoryCacheKey() any {
	return cloudDirectoryCacheKey{
		Provider:  c.connection.Provider,
		ID:        c.connection.ID,
		UpdatedAt: c.connection.UpdatedAt.UnixNano(),
	}
}

func (c *CloudVFS) TransferName(srcPath string, dst vfs.VFS) string {
	location, err := c.resolvePath(srcPath)
	if err != nil {
		return ""
	}
	backend, err := c.backend()
	if err != nil {
		return ""
	}
	if dst != nil && vfs.SameSession(c, dst) {
		if namer, ok := backend.(BackendIntraSessionNamer); ok {
			if name := namer.IntraSessionTransferName(location); name != "" {
				return name
			}
		}
	}
	if namer, ok := backend.(BackendTransferNamer); ok {
		if name := namer.TransferName(location); name != "" {
			return name
		}
	}
	c.mu.RLock()
	entry, ok := c.entries[location]
	c.mu.RUnlock()
	if ok && entry.TransferName != "" {
		return entry.TransferName
	}
	return c.Base(srcPath)
}

func (c *CloudVFS) panelInfoBackend() BackendPanelInfo {
	backend, err := c.backend()
	if err != nil {
		return nil
	}
	provider, _ := backend.(BackendPanelInfo)
	return provider
}

func (c *CloudVFS) PanelInfoKey(req vfs.PanelInfoRequest) string {
	if provider := c.panelInfoBackend(); provider != nil {
		return provider.PanelInfoKey(req)
	}
	return fmt.Sprintf("cloudfox:%s:%s", c.connection.ID, req.Path)
}

func (c *CloudVFS) CachedPanelInfo(req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, bool) {
	if provider := c.panelInfoBackend(); provider != nil {
		return provider.CachedPanelInfo(req)
	}
	return vfs.PanelInfoSnapshot{Authoritative: true}, true
}

func (c *CloudVFS) RefreshPanelInfo(ctx context.Context, req vfs.PanelInfoRequest) (vfs.PanelInfoSnapshot, error) {
	backend, opCtx, finish, err := c.operation(ctx)
	if err != nil {
		return vfs.PanelInfoSnapshot{}, err
	}
	defer finish()
	if provider, ok := backend.(BackendPanelInfo); ok {
		return provider.RefreshPanelInfo(opCtx, req)
	}
	return vfs.PanelInfoSnapshot{Authoritative: true}, nil
}

var (
	_ vfs.VFS                  = (*CloudVFS)(nil)
	_ vfs.OptimisticPathSetter = (*CloudVFS)(nil)
	_ vfs.TitleProvider        = (*CloudVFS)(nil)
	_ vfs.PanelTitleProvider   = (*CloudVFS)(nil)
	_ vfs.PanelInfoProvider    = (*CloudVFS)(nil)
	_ vfs.SessionIdentity      = (*CloudVFS)(nil)
	_ vfs.ServerSideCopier     = (*CloudVFS)(nil)
	_ vfs.TransferNameProvider = (*CloudVFS)(nil)
	_ vfs.ShareLinkProvider    = (*CloudVFS)(nil)
	_ vfs.TrashVFS             = (*trashCloudVFS)(nil)
)
