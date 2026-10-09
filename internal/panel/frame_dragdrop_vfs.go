package panel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unxed/f4/internal/terminal/far2ldnd"
	"github.com/unxed/f4/vfs"
)

// The receiving half of far2l DND (unxed/f4#1628, step 5): a program in the
// embedded terminal, itself an f4, offers files by INPUT_DND. On the
// application side the offer is adapted to a VFS, and the copy into the panel
// is done by the ordinary file-operation machinery (fileops.ExecuteFileOp), so
// progress, overwrite questions and cancelling behave as for any other copy.
//
// DropSourceVFS is that adapter: a read-only, flat VFS whose root lists the
// items of one offer and whose files read through DND/READ. It never writes,
// and the names it shows are the offer's display names made safe to use as
// destination names (the protocol says they are never destination paths).

// dndOfferReader is what DropSourceVFS needs of the far2l channel; the
// terminal package's DNDClient is one.
type dndOfferReader interface {
	ListAll(ctx context.Context, offer far2ldnd.ID) ([]far2ldnd.Entry, error)
	Read(ctx context.Context, offer far2ldnd.ID, itemID, offset uint64, length uint32) (far2ldnd.ReadReply, error)
}

var errDropSourceReadOnly = errors.New("a dropped offer is read-only")

var _ vfs.VFS = (*DropSourceVFS)(nil)

type dropSourceItem struct {
	entry far2ldnd.Entry
	name  string
}

// DropSourceVFS exposes the files of one DND offer as a VFS.
type DropSourceVFS struct {
	client   dndOfferReader
	offer    far2ldnd.ID
	maxChunk uint32
	items    []dropSourceItem
	byName   map[string]*dropSourceItem
	cwd      string
	// cancelled is set when a read ended because the copy was cancelled, so
	// the offer can be closed with CloseCancelled rather than CloseProcessed.
	cancelled atomic.Bool
}

// Cancelled reports whether the user cancelled a copy that was reading this
// offer.
func (s *DropSourceVFS) Cancelled() bool { return s.cancelled.Load() }

// NewDropSourceVFS lists the offer and builds the VFS. maxChunk is the
// max_chunk the binding was granted; a read is never longer. Items without a
// STREAM representation cannot be read and are left out; so are items whose
// name cannot be a single file name.
func NewDropSourceVFS(ctx context.Context, client dndOfferReader, offer far2ldnd.ID, maxChunk uint32) (*DropSourceVFS, error) {
	if maxChunk == 0 {
		return nil, errors.New("drop source: no read chunk size granted")
	}
	entries, err := client.ListAll(ctx, offer)
	if err != nil {
		return nil, err
	}
	s := &DropSourceVFS{client: client, offer: offer, maxChunk: maxChunk, byName: make(map[string]*dropSourceItem, len(entries)), cwd: "/"}
	used := make(map[string]bool, len(entries))
	for _, e := range entries {
		if e.Kind != far2ldnd.KindFile || e.Flags&far2ldnd.ItemStream == 0 {
			continue
		}
		name := safeDropName(e.Name)
		if name == "" {
			continue
		}
		// Two items with one name must both arrive: number the later ones.
		unique := name
		for n := 2; used[unique]; n++ {
			ext := path.Ext(name)
			unique = fmt.Sprintf("%s (%d)%s", strings.TrimSuffix(name, ext), n, ext)
		}
		used[unique] = true
		s.items = append(s.items, dropSourceItem{entry: e, name: unique})
	}
	for i := range s.items {
		s.byName[s.items[i].name] = &s.items[i]
	}
	return s, nil
}

// safeDropName reduces an offered display name to a name that is one path
// element, or "" when nothing usable is left.
func safeDropName(raw string) string {
	name := strings.ToValidUTF8(raw, "?")
	name = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '/' || r == '\\' {
			return '_'
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." {
		return ""
	}
	return name
}

// item resolves a path: nil, nil for the root, the item for a file name.
func (s *DropSourceVFS) item(p string) (*dropSourceItem, error) {
	p = path.Clean("/" + strings.TrimPrefix(strings.ReplaceAll(p, "\\", "/"), "/"))
	if p == "/" {
		return nil, nil
	}
	it := s.byName[strings.TrimPrefix(p, "/")]
	if it == nil {
		return nil, fmt.Errorf("%s: %w", p, fs.ErrNotExist)
	}
	return it, nil
}

func (it *dropSourceItem) vfsItem() vfs.VFSItem {
	return vfs.VFSItem{
		KnownMetadata: vfs.MetadataExplicit,
		Name:          it.name,
		Size:          int64(it.entry.Size), //nolint:gosec // a size the sender announced; copies check it against the bytes read
		SizeKnown:     it.entry.Flags&far2ldnd.ItemSizeKnown != 0,
		MTime:         time.Time{},
	}
}

func (s *DropSourceVFS) IsAtRoot() bool         { return true }
func (s *DropSourceVFS) GetPath() string        { return s.cwd }
func (s *DropSourceVFS) IsAbs(p string) bool    { return strings.HasPrefix(p, "/") }
func (s *DropSourceVFS) SetPath(p string) error { s.cwd = path.Clean("/" + p); return nil }
func (s *DropSourceVFS) Join(e ...string) string {
	return path.Join(e...)
}
func (s *DropSourceVFS) Abs(p string) (string, error) {
	if path.IsAbs(p) {
		return path.Clean(p), nil
	}
	return path.Join(s.cwd, p), nil
}
func (s *DropSourceVFS) Base(p string) string { return path.Base(p) }
func (s *DropSourceVFS) Dir(p string) string  { return path.Dir(p) }

func (s *DropSourceVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	it, err := s.item(p)
	if err != nil {
		return err
	}
	if it != nil {
		return fmt.Errorf("%s: not a directory", p)
	}
	out := make([]vfs.VFSItem, 0, len(s.items))
	for i := range s.items {
		out = append(out, s.items[i].vfsItem())
	}
	if len(out) > 0 {
		onChunk(out)
	}
	return ctx.Err()
}

func (s *DropSourceVFS) Stat(_ context.Context, p string) (vfs.VFSItem, error) {
	it, err := s.item(p)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	if it == nil {
		return vfs.VFSItem{KnownMetadata: vfs.MetadataExplicit, Name: "/", IsDir: true}, nil
	}
	return it.vfsItem(), nil
}

func (s *DropSourceVFS) MkDir(context.Context, string) error  { return errDropSourceReadOnly }
func (s *DropSourceVFS) Remove(context.Context, string) error { return errDropSourceReadOnly }
func (s *DropSourceVFS) Rename(context.Context, string, string) error {
	return errDropSourceReadOnly
}
func (s *DropSourceVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return errDropSourceReadOnly
}
func (s *DropSourceVFS) Create(context.Context, string) (io.WriteCloser, error) {
	return nil, errDropSourceReadOnly
}
func (s *DropSourceVFS) Search(context.Context, string, string) (chan int64, error) {
	return nil, vfs.ErrFindOptionsUnsupported
}

// GetCapabilities is the conservative answer: random access is a property of
// each item (the sender marks it), not of the whole offer.
func (s *DropSourceVFS) GetCapabilities() vfs.VFSCapabilities { return vfs.VFSCapabilities{} }
func (s *DropSourceVFS) ParentVFS() vfs.VFS                   { return nil }
func (s *DropSourceVFS) Clone() vfs.VFS                       { return s }

// Close does nothing: the offer belongs to the terminal that received it and
// is closed by whoever finished the drop (DND/CLOSE), not by the VFS.
func (s *DropSourceVFS) Close() error { return nil }

func (s *DropSourceVFS) Open(_ context.Context, p string) (vfs.ReadAtCloser, error) {
	it, err := s.item(p)
	if err != nil {
		return nil, err
	}
	if it == nil {
		return nil, fmt.Errorf("%s: is a directory", p)
	}
	return &dropSourceFile{s: s, it: it}, nil
}

// dropSourceFile reads one item. A sequential item (no RANDOM_ACCESS) can
// only be read from where the last read ended; asking for another place is an
// error rather than a silent restart.
type dropSourceFile struct {
	s   *DropSourceVFS
	it  *dropSourceItem
	mu  sync.Mutex
	pos uint64
	eof bool
}

func (f *dropSourceFile) Size() int64 {
	if f.it.entry.Flags&far2ldnd.ItemSizeKnown == 0 {
		return -1
	}
	return int64(f.it.entry.Size) //nolint:gosec // see vfsItem
}

func (f *dropSourceFile) Close() error { return nil }

func (f *dropSourceFile) Read(ctx context.Context, p []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(p) == 0 {
		return 0, nil
	}
	if f.eof {
		return 0, io.EOF
	}
	n, err := f.readLocked(ctx, p, f.pos)
	if err == nil && n == 0 && f.eof {
		return 0, io.EOF
	}
	return n, err
}

func (f *dropSourceFile) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	o := uint64(off)
	if f.it.entry.Flags&far2ldnd.ItemRandomAccess == 0 && o != f.pos {
		return 0, fmt.Errorf("%s can only be read in order (at %d, asked for %d)", f.it.name, f.pos, o)
	}
	n, err := f.readLocked(ctx, p, o)
	if err == nil && n < len(p) && f.eof {
		err = io.EOF
	}
	return n, err
}

func (s *DropSourceVFS) noteCancel(err error) {
	if errors.Is(err, context.Canceled) {
		s.cancelled.Store(true)
	}
}

// readLocked fills p from off in chunks of at most max_chunk, stopping at
// EOF. It leaves pos after the bytes read.
func (f *dropSourceFile) readLocked(ctx context.Context, p []byte, off uint64) (int, error) {
	total := 0
	for total < len(p) {
		if err := ctx.Err(); err != nil {
			f.s.noteCancel(err)
			return total, err
		}
		want := len(p) - total
		if uint64(want) > uint64(f.s.maxChunk) {
			want = int(f.s.maxChunk)
		}
		reply, err := f.s.client.Read(ctx, f.s.offer, f.it.entry.ItemID, off+uint64(total), uint32(want)) //nolint:gosec // want <= max_chunk
		if err != nil {
			f.s.noteCancel(err)
			f.pos = off + uint64(total)
			return total, err
		}
		if len(reply.Data) > want {
			return total, fmt.Errorf("%s: the sender returned %d bytes for a request of %d", f.it.name, len(reply.Data), want)
		}
		total += copy(p[total:], reply.Data)
		if reply.Flags&far2ldnd.ReadEOF != 0 {
			f.eof = true
			break
		}
	}
	f.pos = off + uint64(total)
	return total, nil
}
