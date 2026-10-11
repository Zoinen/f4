package mongofs

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
)

const (
	// maxDocs is how many documents one listing shows; a collection that has
	// more says so instead of pretending to be complete.
	maxDocs = 1000
	// docSuffix makes a document a file.
	docSuffix = ".json"
	// statTimeout bounds the quick calls made from SetPath.
	statTimeout = 30 * time.Second
)

var (
	errNotADirectory = errors.New("not a directory")
	errIsADirectory  = errors.New("is a directory")
)

// unsupportedError is what every change gets; it is os.ErrPermission to the
// file operations that ask.
type unsupportedError struct{}

func (unsupportedError) Error() string {
	return mongoText("Mongo.NotSupported",
		"The MongoDB panel cannot do this: documents can be edited, created and deleted and collections created, nothing else",
		"Панель MongoDB этого не умеет: документы можно править, создавать и удалять, коллекции создавать, остального нельзя")
}

func (unsupportedError) Is(target error) bool { return target == os.ErrPermission }

// truncatedError ends a listing of a collection with more than maxDocs
// documents.
type truncatedError struct{}

func (truncatedError) Error() string {
	return mongoText("Mongo.ListingTruncated",
		"The collection has more documents than the panel lists; only the first ones are shown",
		"В коллекции больше документов, чем показывает панель; показаны только первые")
}

// mongoVFS is the MongoDB panel. Paths are POSIX: "/" lists databases, "/<db>"
// its collections, "/<db>/<collection>" its documents as <id>.json files.
type mongoVFS struct {
	open func(context.Context) (*conn, error)

	cmu    sync.Mutex // one command at a time
	mu     sync.Mutex // guards the fields below
	c      *conn
	ids    map[string]any // "db/collection/file" -> the _id it was listed for
	cwd    string
	closed bool
}

func newMongoVFS(open func(context.Context) (*conn, error)) *mongoVFS {
	return &mongoVFS{open: open, cwd: "/", ids: map[string]any{}}
}

// run sends one command, connecting first when there is no connection, and
// drops the connection when it fails at the network level so the next call
// reconnects. Commands are one at a time (cmu); the state the UI reads is
// under mu, which is never held across network I/O.
func (v *mongoVFS) run(ctx context.Context, db string, cmd bsonD) (bsonD, error) {
	v.cmu.Lock()
	defer v.cmu.Unlock()
	v.mu.Lock()
	closed, c := v.closed, v.c
	v.mu.Unlock()
	if closed {
		return nil, errors.New("MongoDB: the panel is closed")
	}
	if c == nil {
		var err error
		if c, err = v.open(ctx); err != nil {
			return nil, err
		}
		v.mu.Lock()
		v.c = c
		v.mu.Unlock()
	}
	reply, err := c.command(ctx, db, cmd)
	if err != nil && !strings.HasPrefix(err.Error(), "mongodb: ") {
		c.close()
		v.mu.Lock()
		if v.c == c {
			v.c = nil
		}
		v.mu.Unlock()
	}
	return reply, err
}

// cursor runs a command that returns a cursor and gathers up to limit
// documents, following getMore. more reports that the cursor had more.
func (v *mongoVFS) cursor(ctx context.Context, db string, cmd bsonD, coll string, limit int) (docs []bsonD, more bool, err error) {
	reply, err := v.run(ctx, db, cmd)
	if err != nil {
		return nil, false, err
	}
	for {
		cur, _ := reply.get("cursor").(bsonD)
		batch := "firstBatch"
		if _, ok := cur.get("nextBatch").([]any); ok {
			batch = "nextBatch"
		}
		items, _ := cur.get(batch).([]any)
		for _, item := range items {
			if d, ok := item.(bsonD); ok {
				docs = append(docs, d)
			}
		}
		id, _ := cur.get("id").(int64)
		if id == 0 {
			return docs, false, nil
		}
		if len(docs) > limit {
			_, _ = v.run(ctx, db, bsonD{{"killCursors", coll}, {"cursors", []any{id}}})
			return docs, true, nil
		}
		if ns, _ := cur.get("ns").(string); ns != "" {
			_, coll, _ = strings.Cut(ns, ".")
		}
		if reply, err = v.run(ctx, db, bsonD{{"getMore", id}, {"collection", coll}}); err != nil {
			return nil, false, err
		}
	}
}

// Panel paths are written as mongo:///<path> (uriPrefix and the POSIX path),
// so that a bookmark, a folder history entry or a restored session can open the
// panel again through the URI provider (f4#1669); the methods accept both that
// form and the plain path, and keep the form they were given.
const uriPrefix = "mongo://"

func stripURI(p string) (plain string, wasURI bool) {
	if rest, ok := strings.CutPrefix(p, uriPrefix); ok {
		return rest, true
	}
	return p, false
}

func withURI(p string, uri bool) string {
	if uri {
		return uriPrefix + p
	}
	return p
}

func (v *mongoVFS) IsAtRoot() bool { return v.plainPath() == "/" }

func (v *mongoVFS) IsAbs(p string) bool {
	return strings.HasPrefix(p, "/") || strings.HasPrefix(p, uriPrefix)
}

// plainPath is the current folder as a POSIX path.
func (v *mongoVFS) plainPath() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.cwd
}

// GetPath is the current folder as a URI.
func (v *mongoVFS) GetPath() string { return uriPrefix + v.plainPath() }

func (v *mongoVFS) Join(elem ...string) string {
	if len(elem) == 0 {
		return ""
	}
	first, uri := stripURI(elem[0])
	return withURI(path.Join(append([]string{first}, elem[1:]...)...), uri)
}

func (v *mongoVFS) Base(p string) string {
	plain, _ := stripURI(p)
	return path.Base(path.Clean(plain))
}

func (v *mongoVFS) Dir(p string) string {
	plain, uri := stripURI(p)
	return withURI(path.Dir(path.Clean(plain)), uri)
}

// Abs is always the plain POSIX path: it is what the rest of the panel works with.
func (v *mongoVFS) Abs(p string) (string, error) {
	if p == "" {
		return v.plainPath(), nil
	}
	plain, _ := stripURI(p)
	if strings.HasPrefix(plain, "/") {
		return path.Clean(plain), nil
	}
	return path.Join(v.plainPath(), plain), nil
}

// location is a panel path taken apart; depth counts how many of database,
// collection and document it names.
type location struct {
	db, coll, doc string
	depth         int
}

func parseLocation(abs string) location {
	rest := strings.Trim(path.Clean(abs), "/")
	if rest == "" {
		return location{}
	}
	parts := strings.SplitN(rest, "/", 3)
	loc := location{db: parts[0], depth: 1}
	if len(parts) > 1 {
		loc.coll, loc.depth = parts[1], 2
	}
	if len(parts) > 2 {
		loc.doc, loc.depth = parts[2], 3
	}
	return loc
}

func (v *mongoVFS) SetPath(p string) error {
	abs, err := v.Abs(p)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), statTimeout)
	defer cancel()
	item, err := v.Stat(ctx, abs)
	if err != nil {
		return err
	}
	if !item.IsDir {
		return fmt.Errorf("%s: %w", abs, errNotADirectory)
	}
	v.mu.Lock()
	v.cwd = abs
	v.mu.Unlock()
	return nil
}

func dirItem(name string) vfs.VFSItem {
	return vfs.VFSItem{KnownMetadata: vfs.MetadataExplicit, Name: name, IsDir: true, NoExtension: true}
}

func (v *mongoVFS) databases(ctx context.Context) ([]string, error) {
	reply, err := v.run(ctx, "admin", bsonD{{"listDatabases", int32(1)}, {"nameOnly", true}})
	if err != nil {
		return nil, err
	}
	var names []string
	list, _ := reply.get("databases").([]any)
	for _, item := range list {
		if d, ok := item.(bsonD); ok {
			if n, _ := d.get("name").(string); n != "" {
				names = append(names, n)
			}
		}
	}
	return names, nil
}

func (v *mongoVFS) collections(ctx context.Context, db string) ([]string, error) {
	docs, _, err := v.cursor(ctx, db, bsonD{{"listCollections", int32(1)}, {"nameOnly", true}}, "$cmd.listCollections", 1<<30)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, d := range docs {
		if n, _ := d.get("name").(string); n != "" {
			names = append(names, n)
		}
	}
	return names, nil
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// docFileName is how a document is named in the panel: its _id, spelled so
// that the name can be turned back into the _id.
func docFileName(id any) string {
	switch x := id.(type) {
	case objectID:
		return x.hex() + docSuffix
	case string:
		return "s_" + url.PathEscape(x) + docSuffix
	case int32:
		return "i_" + strconv.FormatInt(int64(x), 10) + docSuffix
	case int64:
		return "i_" + strconv.FormatInt(x, 10) + docSuffix
	}
	return "j_" + url.PathEscape(toJSON(id, "")) + docSuffix
}

// idFromName undoes docFileName for the forms that can be undone without a
// listing: an ObjectId, a string, an integer.
func idFromName(name string) (any, bool) {
	base := strings.TrimSuffix(name, docSuffix)
	if base == name {
		return nil, false
	}
	switch {
	case len(base) == 24 && isHex(base):
		raw, err := hex.DecodeString(base)
		if err != nil {
			return nil, false
		}
		var o objectID
		copy(o[:], raw)
		return o, true
	case strings.HasPrefix(base, "s_"):
		s, err := url.PathUnescape(base[2:])
		return s, err == nil
	case strings.HasPrefix(base, "i_"):
		n, err := strconv.ParseInt(base[2:], 10, 64)
		return n, err == nil
	}
	return nil, false
}

func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func (v *mongoVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	abs, err := v.Abs(p)
	if err != nil {
		return err
	}
	loc := parseLocation(abs)
	send := func(items []vfs.VFSItem) {
		if len(items) > 0 && onChunk != nil {
			onChunk(items)
		}
	}
	switch loc.depth {
	case 0:
		names, err := v.databases(ctx)
		if err != nil {
			return err
		}
		items := make([]vfs.VFSItem, 0, len(names))
		for _, n := range names {
			items = append(items, dirItem(n))
		}
		send(items)
		return nil
	case 1:
		names, err := v.collections(ctx, loc.db)
		if err != nil {
			return err
		}
		items := make([]vfs.VFSItem, 0, len(names))
		for _, n := range names {
			items = append(items, dirItem(n))
		}
		send(items)
		return nil
	case 2:
		docs, more, err := v.cursor(ctx, loc.db, bsonD{
			{"find", loc.coll}, {"projection", bsonD{{"_id", int32(1)}}},
			{"limit", int32(maxDocs + 1)}, {"batchSize", int32(maxDocs + 1)},
		}, loc.coll, maxDocs)
		if err != nil {
			return err
		}
		if len(docs) > maxDocs {
			docs, more = docs[:maxDocs], true
		}
		items := make([]vfs.VFSItem, 0, len(docs))
		v.mu.Lock()
		for _, d := range docs {
			id := d.get("_id")
			name := docFileName(id)
			v.ids[loc.db+"/"+loc.coll+"/"+name] = id
			items = append(items, vfs.VFSItem{KnownMetadata: vfs.MetadataExplicit, Name: name})
		}
		v.mu.Unlock()
		send(items)
		if more {
			return truncatedError{}
		}
		return nil
	}
	return fmt.Errorf("%s: %w", abs, errNotADirectory)
}

// document fetches one document by its file name.
func (v *mongoVFS) document(ctx context.Context, loc location) (bsonD, error) {
	v.mu.Lock()
	id, ok := v.ids[loc.db+"/"+loc.coll+"/"+loc.doc]
	v.mu.Unlock()
	if !ok {
		if id, ok = idFromName(loc.doc); !ok {
			return nil, fmt.Errorf("%s: %w", loc.doc, os.ErrNotExist)
		}
	}
	docs, _, err := v.cursor(ctx, loc.db, bsonD{
		{"find", loc.coll}, {"filter", bsonD{{"_id", id}}}, {"limit", int32(1)}, {"singleBatch", true},
	}, loc.coll, 1)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("%s: %w", loc.doc, os.ErrNotExist)
	}
	return docs[0], nil
}

func (v *mongoVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	abs, err := v.Abs(p)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	loc := parseLocation(abs)
	switch loc.depth {
	case 0:
		return dirItem("MongoDB"), nil
	case 1:
		names, err := v.databases(ctx)
		if err != nil {
			return vfs.VFSItem{}, err
		}
		if !contains(names, loc.db) {
			return vfs.VFSItem{}, fmt.Errorf("%s: %w", loc.db, os.ErrNotExist)
		}
		return dirItem(loc.db), nil
	case 2:
		names, err := v.collections(ctx, loc.db)
		if err != nil {
			return vfs.VFSItem{}, err
		}
		if !contains(names, loc.coll) {
			return vfs.VFSItem{}, fmt.Errorf("%s: %w", abs, os.ErrNotExist)
		}
		return dirItem(loc.coll), nil
	}
	doc, err := v.document(ctx, loc)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	return vfs.VFSItem{
		KnownMetadata: vfs.MetadataExplicit, Name: loc.doc,
		Size: int64(len(toJSON(doc, "  ")) + 1), SizeKnown: true,
	}, nil
}

// Open renders the document as indented JSON into a temporary file, which is
// what gives F3 and F5 random access.
func (v *mongoVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	abs, err := v.Abs(p)
	if err != nil {
		return nil, err
	}
	loc := parseLocation(abs)
	if loc.depth < 3 {
		return nil, fmt.Errorf("%s: %w", abs, errIsADirectory)
	}
	doc, err := v.document(ctx, loc)
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp("", "f4-mongo-*.json")
	if err != nil {
		return nil, err
	}
	tempPath := file.Name()
	text := toJSON(doc, "  ") + "\n"
	_, err = io.WriteString(file, text)
	if err == nil {
		_, err = file.Seek(0, io.SeekStart)
	}
	if err != nil {
		_ = file.Close()
		_ = os.Remove(tempPath)
		return nil, err
	}
	return &vfs.TempFileWrapper{File: file, SizeVal: int64(len(text)), TempPath: tempPath}, nil
}

// MkDir creates an empty collection; a database exists only once it holds one,
// so there is nothing to create at the top.
func (v *mongoVFS) MkDir(ctx context.Context, p string) error {
	abs, err := v.Abs(p)
	if err != nil {
		return err
	}
	loc := parseLocation(abs)
	if loc.depth != 2 {
		return unsupportedError{}
	}
	_, err = v.run(ctx, loc.db, bsonD{{"create", loc.coll}})
	return err
}

// Remove deletes one document. Dropping a collection or a database is not done
// from a panel where one keypress deletes a selection.
func (v *mongoVFS) Remove(ctx context.Context, p string) error {
	abs, err := v.Abs(p)
	if err != nil {
		return err
	}
	loc := parseLocation(abs)
	if loc.depth != 3 {
		return unsupportedError{}
	}
	id, err := v.docID(loc)
	if err != nil {
		return err
	}
	reply, err := v.run(ctx, loc.db, bsonD{
		{"delete", loc.coll}, {"deletes", []any{bsonD{{"q", bsonD{{"_id", id}}}, {"limit", int32(1)}}}},
	})
	if err != nil {
		return err
	}
	if err := writeError(reply); err != nil {
		return err
	}
	if n, _ := numberOf(reply.get("n")); n < 1 {
		return fmt.Errorf("%s: %w", loc.doc, os.ErrNotExist)
	}
	return nil
}

func (v *mongoVFS) Rename(context.Context, string, string) error { return unsupportedError{} }
func (v *mongoVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return unsupportedError{}
}

// docID is the _id a file name stands for.
func (v *mongoVFS) docID(loc location) (any, error) {
	v.mu.Lock()
	id, ok := v.ids[loc.db+"/"+loc.coll+"/"+loc.doc]
	v.mu.Unlock()
	if ok {
		return id, nil
	}
	if id, ok = idFromName(loc.doc); ok {
		return id, nil
	}
	return nil, fmt.Errorf("%s: %w", loc.doc, os.ErrNotExist)
}

// writeError turns the writeErrors of an insert/update/delete reply into an error.
func writeError(reply bsonD) error {
	list, _ := reply.get("writeErrors").([]any)
	for _, item := range list {
		if d, ok := item.(bsonD); ok {
			if msg, _ := d.get("errmsg").(string); msg != "" {
				return fmt.Errorf("mongodb: %s", msg)
			}
		}
	}
	return nil
}

// maxDocumentSize is the largest document the panel takes back (MongoDB's own
// limit is 16 MiB).
const maxDocumentSize = 16 << 20

// Create returns a writer that collects the text of a document and, on Close,
// replaces the document of that name (or inserts it when there is none). The
// text is the relaxed extended JSON the panel shows, so a document that was
// opened, edited and saved keeps its types.
func (v *mongoVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	abs, err := v.Abs(p)
	if err != nil {
		return nil, err
	}
	loc := parseLocation(abs)
	if loc.depth != 3 || !strings.HasSuffix(loc.doc, docSuffix) {
		return nil, unsupportedError{}
	}
	return &docWriter{ctx: ctx, v: v, loc: loc}, nil
}

type docWriter struct {
	ctx    context.Context
	v      *mongoVFS
	loc    location
	buf    bytes.Buffer
	closed bool
}

func (w *docWriter) Write(p []byte) (int, error) {
	if w.buf.Len()+len(p) > maxDocumentSize {
		return 0, errors.New("mongodb: the document is larger than 16 MiB")
	}
	return w.buf.Write(p)
}

func (w *docWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	return w.v.saveDocument(w.ctx, w.loc, w.buf.Bytes())
}

var errIDMismatch = errors.New("the _id in the text is not the one in the file name")

func (v *mongoVFS) saveDocument(ctx context.Context, loc location, text []byte) error {
	parsed, err := parseEJSON(text)
	if err != nil {
		return err
	}
	doc, ok := parsed.(bsonD)
	if !ok {
		return fmt.Errorf("%w: a document is a JSON object", errEJSON)
	}
	nameID, nameErr := v.docID(loc)
	textID := doc.get("_id")
	switch {
	case textID != nil && nameErr == nil && !sameID(textID, nameID):
		return fmt.Errorf("%s: %w", loc.doc, errIDMismatch)
	case textID == nil && nameErr == nil:
		doc = append(bsonD{{"_id", nameID}}, doc...)
	case textID == nil:
		doc = append(bsonD{{"_id", newObjectID()}}, doc...)
	}
	id := doc.get("_id")
	exists := false
	if _, err := v.document(ctx, location{db: loc.db, coll: loc.coll, doc: docFileName(id), depth: 3}); err == nil {
		exists = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	var reply bsonD
	if exists {
		reply, err = v.run(ctx, loc.db, bsonD{
			{"update", loc.coll},
			{"updates", []any{bsonD{{"q", bsonD{{"_id", id}}}, {"u", doc}, {"upsert", false}}}},
		})
	} else {
		reply, err = v.run(ctx, loc.db, bsonD{{"insert", loc.coll}, {"documents", []any{doc}}})
	}
	if err != nil {
		return err
	}
	if err := writeError(reply); err != nil {
		return err
	}
	v.mu.Lock()
	v.ids[loc.db+"/"+loc.coll+"/"+docFileName(id)] = id
	v.mu.Unlock()
	return nil
}

// sameID compares two _id values; an int32 and an int64 of the same value are
// the same id, because that is how a number in a file name and one in the text
// meet.
func sameID(a, b any) bool {
	na, aok := integerOf(a)
	nb, bok := integerOf(b)
	if aok && bok {
		return na == nb
	}
	return toJSON(a, "") == toJSON(b, "")
}

func integerOf(v any) (int64, bool) {
	switch x := v.(type) {
	case int32:
		return int64(x), true
	case int64:
		return x, true
	}
	return 0, false
}

// newObjectID makes an id for a document saved under a name that is not one.
func newObjectID() objectID {
	var o objectID
	binary.BigEndian.PutUint32(o[:4], uint32(time.Now().Unix())) // #nosec G115 -- seconds fit until 2106
	_, _ = rand.Read(o[4:])
	return o
}

func (v *mongoVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true, HasWrite: true}
}

func (v *mongoVFS) Search(context.Context, string, string) (chan int64, error) { return nil, nil }

func (v *mongoVFS) ParentVFS() vfs.VFS { return nil }

// PanelTitle names the panel by where it is, "MongoDB:shop/orders".
func (v *mongoVFS) PanelTitle(p string) string {
	abs, err := v.Abs(p)
	if err != nil || abs == "/" {
		return "MongoDB"
	}
	return "MongoDB:" + strings.TrimPrefix(abs, "/")
}

// Clone opens its own connection when first used.
func (v *mongoVFS) Clone() vfs.VFS {
	clone := newMongoVFS(v.open)
	clone.cwd = v.plainPath()
	return clone
}

func (v *mongoVFS) Close() error {
	v.mu.Lock()
	c := v.c
	v.c = nil
	v.closed = true
	v.mu.Unlock()
	c.close()
	return nil
}

var (
	_ vfs.VFS                = (*mongoVFS)(nil)
	_ vfs.PanelTitleProvider = (*mongoVFS)(nil)
)
