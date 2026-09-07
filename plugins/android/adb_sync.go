package androidfs

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	pathpkg "path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Limits imposed by the ADB sync wire protocol.
const (
	SyncMaxPath = 1024
	SyncMaxData = 64 * 1024

	syncMaxName    = SyncMaxPath
	syncMaxMessage = SyncMaxData
)

const (
	syncIDLstatV1 = "STAT"
	syncIDStatV2  = "STA2"
	syncIDLstatV2 = "LST2"
	syncIDListV1  = "LIST"
	syncIDListV2  = "LIS2"
	syncIDDentV1  = "DENT"
	syncIDDentV2  = "DNT2"
	syncIDSendV1  = "SEND"
	syncIDSendV2  = "SND2"
	syncIDRecvV1  = "RECV"
	syncIDRecvV2  = "RCV2"
	syncIDData    = "DATA"
	syncIDDone    = "DONE"
	syncIDOkay    = "OKAY"
	syncIDFail    = "FAIL"
)

// SyncServiceOpener opens an already-selected ADB device service. A Server
// satisfies this interface by transporting the "sync:" service over the ADB
// smart socket protocol.
type SyncServiceOpener interface {
	OpenService(ctx context.Context, serial, service string) (io.ReadWriteCloser, error)
}

// SyncServiceOpenFunc adapts a function to SyncServiceOpener. It is also useful
// to supply deterministic in-memory connections in tests.
type SyncServiceOpenFunc func(context.Context, string, string) (io.ReadWriteCloser, error)

func (f SyncServiceOpenFunc) OpenService(ctx context.Context, serial, service string) (io.ReadWriteCloser, error) {
	return f(ctx, serial, service)
}

// SyncEntry is metadata returned by STAT or LIST. Mode contains the remote
// POSIX file type and permission bits. Errno is non-zero only for a v2 LIST
// entry whose lstat failed while the directory was being enumerated.
type SyncEntry struct {
	Name string

	// metadataV2 distinguishes a real uid/gid of zero from legacy Sync v1,
	// whose metadata packets do not carry ownership at all.
	metadataV2 bool

	Device uint64
	Inode  uint64
	Mode   uint32
	NLink  uint32
	UID    uint32
	GID    uint32
	Size   uint64

	AccessTime time.Time
	ModTime    time.Time
	ChangeTime time.Time
	Errno      uint32
}

// Err reports a metadata error carried by a v2 directory entry.
func (e SyncEntry) Err() error {
	if e.Errno == 0 {
		return nil
	}
	return &SyncRemoteError{Operation: "lstat", Path: e.Name, Errno: e.Errno}
}

// SyncRemoteError is an error reported by adbd, either as a numeric errno in
// a v2 metadata response or as the text of a FAIL packet.
type SyncRemoteError struct {
	Operation string
	Path      string
	Errno     uint32
	Message   string
}

func (e *SyncRemoteError) Error() string {
	prefix := "adb sync"
	if e.Operation != "" {
		prefix += " " + e.Operation
	}
	if e.Path != "" {
		prefix += " " + strconv.Quote(e.Path)
	}
	if e.Message != "" {
		return prefix + ": " + e.Message
	}
	if e.Errno != 0 {
		return fmt.Sprintf("%s: remote errno %d", prefix, e.Errno)
	}
	return prefix + ": remote operation failed"
}

// Unwrap maps common Linux wire errno values to portable filesystem sentinels.
// Less common values remain available as LinuxErrno and in the Errno field.
func (e *SyncRemoteError) Unwrap() error {
	switch e.Errno {
	case 0:
		return nil
	case linuxENOENT, linuxENOTDIR:
		return fs.ErrNotExist
	case linuxEPERM, linuxEACCES, linuxEROFS:
		return fs.ErrPermission
	case linuxEEXIST:
		return fs.ErrExist
	default:
		return LinuxErrno(e.Errno)
	}
}

// LinuxErrno preserves an errno that has no portable fs sentinel. Sync v2
// always puts Linux wire values on the connection, even when f4 runs on
// Windows; converting them to syscall.Errno would incorrectly reinterpret them
// as Win32 error numbers.
type LinuxErrno uint32

func (e LinuxErrno) Error() string { return fmt.Sprintf("Linux errno %d", uint32(e)) }

const (
	linuxEPERM   = 1
	linuxENOENT  = 2
	linuxEACCES  = 13
	linuxEEXIST  = 17
	linuxENOTDIR = 20
	linuxEROFS   = 30
)

// SyncProtocolError identifies malformed or out-of-sequence data from adbd.
type SyncProtocolError struct {
	Operation string
	Detail    string
}

func (e *SyncProtocolError) Error() string {
	if e.Operation == "" {
		return "adb sync protocol error: " + e.Detail
	}
	return "adb sync " + e.Operation + " protocol error: " + e.Detail
}

// SyncClient performs one sync request per ADB service connection. This keeps
// operations independent and makes cancellation and protocol failures local to
// the operation that caused them.
type SyncClient struct {
	opener   SyncServiceOpener
	serial   string
	features map[string]bool
}

func NewSyncClient(opener SyncServiceOpener, serial string, features map[string]bool) *SyncClient {
	featureCopy := make(map[string]bool, len(features))
	for feature, enabled := range features {
		featureCopy[feature] = enabled
	}
	return &SyncClient{opener: opener, serial: serial, features: featureCopy}
}

func (c *SyncClient) hasFeature(feature string) bool {
	return c != nil && c.features[feature]
}

// List returns a snapshot of path. A v2 entry can have Errno set when the
// directory entry existed but its metadata disappeared during enumeration.
func (c *SyncClient) List(ctx context.Context, path string) ([]SyncEntry, error) {
	if err := validateSyncPath(path); err != nil {
		return nil, err
	}

	conn, err := c.open(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }() // The protocol response determines metadata success.

	v2 := c.hasFeature("ls_v2")
	requestID := syncIDListV1
	entryID := syncIDDentV1
	if v2 {
		requestID = syncIDListV2
		entryID = syncIDDentV2
	}
	if err := writeSyncRequest(conn, requestID, path); err != nil {
		return nil, syncContextError(ctx, fmt.Errorf("adb sync list request: %w", err))
	}

	var entries []SyncEntry
	for {
		id, err := readSyncID(conn)
		if err != nil {
			return nil, syncContextError(ctx, fmt.Errorf("adb sync list response: %w", err))
		}
		switch id {
		case syncIDDone:
			return entries, nil
		case syncIDFail:
			return nil, readSyncFailAfterID(conn, "list", path)
		case entryID:
			var entry SyncEntry
			if v2 {
				entry, err = readSyncDentV2AfterID(conn)
			} else {
				entry, err = readSyncDentV1AfterID(conn)
			}
			if err != nil {
				return nil, syncContextError(ctx, fmt.Errorf("adb sync list response: %w", err))
			}
			entries = append(entries, entry)
		default:
			return nil, unexpectedSyncID("list", id, entryID, syncIDDone, syncIDFail)
		}
	}
}

// Lstat returns metadata without following the final symlink. On a device that
// lacks stat_v2, the legacy STAT request is used.
func (c *SyncClient) Lstat(ctx context.Context, path string) (SyncEntry, error) {
	id := syncIDLstatV1
	if c.hasFeature("stat_v2") {
		id = syncIDLstatV2
	}
	return c.stat(ctx, path, id)
}

// Stat follows the final symlink when stat_v2 is available. Legacy ADB only
// exposes STAT (which is lstat-like), so Stat degrades to that request.
func (c *SyncClient) Stat(ctx context.Context, path string) (SyncEntry, error) {
	id := syncIDLstatV1
	if c.hasFeature("stat_v2") {
		id = syncIDStatV2
	}
	return c.stat(ctx, path, id)
}

func (c *SyncClient) stat(ctx context.Context, path, requestID string) (SyncEntry, error) {
	if err := validateSyncPath(path); err != nil {
		return SyncEntry{}, err
	}

	conn, err := c.open(ctx)
	if err != nil {
		return SyncEntry{}, err
	}
	defer func() { _ = conn.Close() }() // The protocol response determines metadata success.

	if err := writeSyncRequest(conn, requestID, path); err != nil {
		return SyncEntry{}, syncContextError(ctx, fmt.Errorf("adb sync stat request: %w", err))
	}
	id, err := readSyncID(conn)
	if err != nil {
		return SyncEntry{}, syncContextError(ctx, fmt.Errorf("adb sync stat response: %w", err))
	}
	if id == syncIDFail {
		return SyncEntry{}, readSyncFailAfterID(conn, "stat", path)
	}
	if id != requestID {
		return SyncEntry{}, unexpectedSyncID("stat", id, requestID, syncIDFail)
	}

	name := pathpkg.Base(path)
	if requestID == syncIDLstatV1 {
		entry, err := readSyncStatV1AfterID(conn)
		entry.Name = name
		if err != nil {
			return SyncEntry{}, syncContextError(ctx, fmt.Errorf("adb sync stat response: %w", err))
		}
		// Legacy adbd has no FAIL/errno form for STAT. It reports lstat(2)
		// failure as an all-zero struct; a real filesystem object always has
		// file-type bits in st_mode, even when its size and timestamp are zero.
		if entry.Mode == 0 {
			return SyncEntry{}, &SyncRemoteError{Operation: "stat", Path: path, Errno: linuxENOENT}
		}
		return entry, nil
	}

	entry, err := readSyncStatV2AfterID(conn)
	entry.Name = name
	if err != nil {
		return SyncEntry{}, syncContextError(ctx, fmt.Errorf("adb sync stat response: %w", err))
	}
	if entry.Errno != 0 {
		return SyncEntry{}, &SyncRemoteError{Operation: "stat", Path: path, Errno: entry.Errno}
	}
	return entry, nil
}

// Receive starts a streaming pull. The caller must close the returned reader.
// DATA frames are validated and exposed as one continuous byte stream.
func (c *SyncClient) Receive(ctx context.Context, path string) (*SyncReceiveReader, error) {
	if err := validateSyncPath(path); err != nil {
		return nil, err
	}
	conn, err := c.open(ctx)
	if err != nil {
		return nil, err
	}

	v2 := c.hasFeature("sendrecv_v2")
	requestID := syncIDRecvV1
	if v2 {
		requestID = syncIDRecvV2
	}
	if err := writeSyncRequest(conn, requestID, path); err != nil {
		_ = conn.Close() // Preserve the receive-request failure.
		return nil, syncContextError(ctx, fmt.Errorf("adb sync receive request: %w", err))
	}
	if v2 {
		var setup [8]byte
		copy(setup[:4], syncIDRecvV2)
		// flags remains zero: no compression is requested.
		if err := writeSyncFull(conn, setup[:]); err != nil {
			_ = conn.Close() // Preserve the receive-setup failure.
			return nil, syncContextError(ctx, fmt.Errorf("adb sync receive setup: %w", err))
		}
	}

	return &SyncReceiveReader{conn: conn, ctx: ctx, path: path}, nil
}

// ReceiveTo copies a remote file to dst and closes the sync connection.
func (c *SyncClient) ReceiveTo(ctx context.Context, path string, dst io.Writer) (int64, error) {
	reader, err := c.Receive(ctx, path)
	if err != nil {
		return 0, err
	}
	n, copyErr := io.Copy(dst, reader)
	closeErr := reader.Close()
	if copyErr != nil {
		return n, copyErr
	}
	return n, closeErr
}

// Send starts a streaming push. Close writes DONE, waits for OKAY, and returns
// any remote FAIL. Call Abort to discard an incomplete transfer without DONE.
func (c *SyncClient) Send(ctx context.Context, path string, mode uint32, mtime time.Time) (*SyncSendWriter, error) {
	if err := validateSyncPath(path); err != nil {
		return nil, err
	}
	timestamp, err := syncTimestamp(mtime)
	if err != nil {
		return nil, err
	}

	v2 := c.hasFeature("sendrecv_v2")
	requestID := syncIDSendV1
	payload := path + "," + strconv.FormatUint(uint64(mode), 10)
	if v2 {
		requestID = syncIDSendV2
		payload = path
	} else if len(payload) > SyncMaxPath {
		return nil, fmt.Errorf("adb sync path and mode are too long: %d bytes (maximum %d)", len(payload), SyncMaxPath)
	}

	conn, err := c.open(ctx)
	if err != nil {
		return nil, err
	}
	if err := writeSyncRequest(conn, requestID, payload); err != nil {
		_ = conn.Close() // Preserve the send-request failure.
		return nil, syncContextError(ctx, fmt.Errorf("adb sync send request: %w", err))
	}
	if v2 {
		var setup [12]byte
		copy(setup[:4], syncIDSendV2)
		binary.LittleEndian.PutUint32(setup[4:8], mode)
		// flags remains zero: no compression and no dry-run.
		if err := writeSyncFull(conn, setup[:]); err != nil {
			_ = conn.Close() // Preserve the send-setup failure.
			return nil, syncContextError(ctx, fmt.Errorf("adb sync send setup: %w", err))
		}
	}

	return &SyncSendWriter{conn: conn, ctx: ctx, path: path, timestamp: timestamp}, nil
}

// SendFrom copies src to a remote file and waits for the final status.
func (c *SyncClient) SendFrom(ctx context.Context, path string, mode uint32, mtime time.Time, src io.Reader) (int64, error) {
	writer, err := c.Send(ctx, path, mode, mtime)
	if err != nil {
		return 0, err
	}
	n, copyErr := io.Copy(writer, src)
	if copyErr != nil {
		_ = writer.Abort()
		return n, copyErr
	}
	return n, writer.Close()
}

func (c *SyncClient) open(ctx context.Context) (*syncConnection, error) {
	if c == nil || c.opener == nil {
		return nil, errors.New("adb sync: nil service opener")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	rwc, err := c.opener.OpenService(ctx, c.serial, "sync:")
	if err != nil {
		return nil, fmt.Errorf("open adb sync service for %q: %w", c.serial, err)
	}
	if rwc == nil {
		return nil, errors.New("open adb sync service: opener returned a nil connection")
	}
	conn := &syncConnection{ReadWriteCloser: rwc, cancelWatch: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-conn.cancelWatch:
		}
	}()
	return conn, nil
}

// SyncReceiveReader turns framed RECV output into a regular byte stream.
type SyncReceiveReader struct {
	mu sync.Mutex

	conn      *syncConnection
	ctx       context.Context
	path      string
	remaining uint32
	closed    bool
	terminal  error
}

func (r *SyncReceiveReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	for {
		if r.terminal != nil {
			return 0, r.terminal
		}
		if r.closed {
			return 0, io.ErrClosedPipe
		}
		if r.remaining != 0 {
			want := len(p)
			if want > int(r.remaining) {
				want = int(r.remaining)
			}
			n, err := io.ReadFull(r.conn, p[:want])
			// #nosec G115 -- io.ReadFull cannot return more than want, which is capped at r.remaining.
			r.remaining -= uint32(n)
			if err != nil {
				err = syncContextError(r.ctx, fmt.Errorf("adb sync receive data: %w", err))
				r.finishLocked(err)
				return n, err
			}
			return n, nil
		}

		var header [8]byte
		if _, err := io.ReadFull(r.conn, header[:]); err != nil {
			err = syncContextError(r.ctx, fmt.Errorf("adb sync receive response: %w", err))
			r.finishLocked(err)
			return 0, err
		}
		id := string(header[:4])
		size := binary.LittleEndian.Uint32(header[4:])
		switch id {
		case syncIDData:
			if size > SyncMaxData {
				err := &SyncProtocolError{Operation: "receive", Detail: fmt.Sprintf("DATA frame is %d bytes (maximum %d)", size, SyncMaxData)}
				r.finishLocked(err)
				return 0, err
			}
			r.remaining = size
			// Ignore empty DATA frames rather than returning (0, nil).
			continue
		case syncIDDone:
			if size != 0 {
				err := &SyncProtocolError{Operation: "receive", Detail: fmt.Sprintf("DONE has non-zero value %d", size)}
				r.finishLocked(err)
				return 0, err
			}
			r.finishLocked(io.EOF)
			return 0, io.EOF
		case syncIDFail:
			err := readSyncFailBody(r.conn, "receive", r.path, size)
			r.finishLocked(err)
			return 0, err
		default:
			err := unexpectedSyncID("receive", id, syncIDData, syncIDDone, syncIDFail)
			r.finishLocked(err)
			return 0, err
		}
	}
}

func (r *SyncReceiveReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return nil
	}
	r.closed = true
	return r.conn.Close()
}

func (r *SyncReceiveReader) finishLocked(err error) {
	r.terminal = err
	_ = r.conn.Close()
}

// SyncSendWriter frames writes as DATA packets and finalizes the transfer on
// Close. It is safe for serialized use; concurrent writes are also guarded.
type SyncSendWriter struct {
	mu sync.Mutex

	conn      *syncConnection
	ctx       context.Context
	path      string
	timestamp uint32
	closed    bool
	closeErr  error
}

func (w *SyncSendWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		if w.closeErr != nil {
			return 0, w.closeErr
		}
		return 0, io.ErrClosedPipe
	}

	written := 0
	for len(p) != 0 {
		chunk := len(p)
		if chunk > SyncMaxData {
			chunk = SyncMaxData
		}
		var header [8]byte
		copy(header[:4], syncIDData)
		binary.LittleEndian.PutUint32(header[4:], uint32(chunk))
		if err := writeSyncFull(w.conn, header[:]); err != nil {
			err = syncContextError(w.ctx, fmt.Errorf("adb sync send DATA header: %w", err))
			w.abortLocked(err)
			return written, err
		}
		if err := writeSyncFull(w.conn, p[:chunk]); err != nil {
			err = syncContextError(w.ctx, fmt.Errorf("adb sync send DATA body: %w", err))
			w.abortLocked(err)
			return written, err
		}
		written += chunk
		p = p[chunk:]
	}
	return written, nil
}

func (w *SyncSendWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return w.closeErr
	}
	w.closed = true

	var done [8]byte
	copy(done[:4], syncIDDone)
	binary.LittleEndian.PutUint32(done[4:], w.timestamp)
	if err := writeSyncFull(w.conn, done[:]); err != nil {
		w.closeErr = syncContextError(w.ctx, fmt.Errorf("adb sync send DONE: %w", err))
		_ = w.conn.Close()
		return w.closeErr
	}

	var status [8]byte
	if _, err := io.ReadFull(w.conn, status[:]); err != nil {
		w.closeErr = syncContextError(w.ctx, fmt.Errorf("adb sync send status: %w", err))
		_ = w.conn.Close()
		return w.closeErr
	}
	id := string(status[:4])
	size := binary.LittleEndian.Uint32(status[4:])
	switch id {
	case syncIDOkay:
		if size != 0 {
			w.closeErr = &SyncProtocolError{Operation: "send", Detail: fmt.Sprintf("OKAY has non-zero value %d", size)}
		}
	case syncIDFail:
		w.closeErr = readSyncFailBody(w.conn, "send", w.path, size)
	default:
		w.closeErr = unexpectedSyncID("send", id, syncIDOkay, syncIDFail)
	}
	closeErr := w.conn.Close()
	if w.closeErr == nil {
		w.closeErr = closeErr
	}
	return w.closeErr
}

// Abort closes the ADB service without sending DONE. It is idempotent.
func (w *SyncSendWriter) Abort() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return w.closeErr
	}
	w.closed = true
	w.closeErr = w.conn.Close()
	return w.closeErr
}

func (w *SyncSendWriter) abortLocked(err error) {
	w.closed = true
	w.closeErr = err
	_ = w.conn.Close()
}

type syncConnection struct {
	io.ReadWriteCloser
	once        sync.Once
	closeErr    error
	cancelWatch chan struct{}
}

func (c *syncConnection) Close() error {
	c.once.Do(func() {
		close(c.cancelWatch)
		c.closeErr = c.ReadWriteCloser.Close()
	})
	return c.closeErr
}

func validateSyncPath(path string) error {
	if path == "" {
		return errors.New("adb sync path is empty")
	}
	if strings.IndexByte(path, 0) >= 0 {
		return errors.New("adb sync path contains NUL")
	}
	if len(path) > SyncMaxPath {
		return fmt.Errorf("adb sync path is too long: %d bytes (maximum %d)", len(path), SyncMaxPath)
	}
	return nil
}

func syncTimestamp(t time.Time) (uint32, error) {
	if t.IsZero() {
		return 0, nil
	}
	seconds := t.Unix()
	if seconds < 0 || seconds > math.MaxUint32 {
		return 0, fmt.Errorf("adb sync modification time %s is outside the uint32 wire range", t.Format(time.RFC3339))
	}
	return uint32(seconds), nil
}

func writeSyncRequest(w io.Writer, id, path string) error {
	if len(id) != 4 {
		return &SyncProtocolError{Operation: "request", Detail: fmt.Sprintf("invalid local message id %q", id)}
	}
	if len(path) > SyncMaxPath {
		return fmt.Errorf("request payload is too long: %d bytes (maximum %d)", len(path), SyncMaxPath)
	}
	var header [8]byte
	copy(header[:4], id)
	// #nosec G115 -- SyncMaxPath is 1024, and the length was checked above.
	binary.LittleEndian.PutUint32(header[4:], uint32(len(path)))
	if err := writeSyncFull(w, header[:]); err != nil {
		return err
	}
	return writeSyncFull(w, []byte(path))
}

func writeSyncFull(w io.Writer, p []byte) error {
	for len(p) != 0 {
		n, err := w.Write(p)
		if n > 0 {
			p = p[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
	}
	return nil
}

func readSyncID(r io.Reader) (string, error) {
	var id [4]byte
	if _, err := io.ReadFull(r, id[:]); err != nil {
		return "", err
	}
	return string(id[:]), nil
}

func readSyncStatV1AfterID(r io.Reader) (SyncEntry, error) {
	var body [12]byte
	if _, err := io.ReadFull(r, body[:]); err != nil {
		return SyncEntry{}, err
	}
	return SyncEntry{
		Mode:    binary.LittleEndian.Uint32(body[0:4]),
		Size:    uint64(binary.LittleEndian.Uint32(body[4:8])),
		ModTime: time.Unix(int64(binary.LittleEndian.Uint32(body[8:12])), 0),
	}, nil
}

func readSyncStatV2AfterID(r io.Reader) (SyncEntry, error) {
	var body [68]byte
	if _, err := io.ReadFull(r, body[:]); err != nil {
		return SyncEntry{}, err
	}
	accessTime, err := syncUnixTime(binary.LittleEndian.Uint64(body[44:52]))
	if err != nil {
		return SyncEntry{}, err
	}
	modTime, err := syncUnixTime(binary.LittleEndian.Uint64(body[52:60]))
	if err != nil {
		return SyncEntry{}, err
	}
	changeTime, err := syncUnixTime(binary.LittleEndian.Uint64(body[60:68]))
	if err != nil {
		return SyncEntry{}, err
	}
	return SyncEntry{
		metadataV2: true,
		Errno:      binary.LittleEndian.Uint32(body[0:4]),
		Device:     binary.LittleEndian.Uint64(body[4:12]),
		Inode:      binary.LittleEndian.Uint64(body[12:20]),
		Mode:       binary.LittleEndian.Uint32(body[20:24]),
		NLink:      binary.LittleEndian.Uint32(body[24:28]),
		UID:        binary.LittleEndian.Uint32(body[28:32]),
		GID:        binary.LittleEndian.Uint32(body[32:36]),
		Size:       binary.LittleEndian.Uint64(body[36:44]),
		AccessTime: accessTime,
		ModTime:    modTime,
		ChangeTime: changeTime,
	}, nil
}

func readSyncDentV1AfterID(r io.Reader) (SyncEntry, error) {
	var body [16]byte
	if _, err := io.ReadFull(r, body[:]); err != nil {
		return SyncEntry{}, err
	}
	nameLen := binary.LittleEndian.Uint32(body[12:16])
	name, err := readSyncName(r, nameLen)
	if err != nil {
		return SyncEntry{}, err
	}
	return SyncEntry{
		Name:    name,
		Mode:    binary.LittleEndian.Uint32(body[0:4]),
		Size:    uint64(binary.LittleEndian.Uint32(body[4:8])),
		ModTime: time.Unix(int64(binary.LittleEndian.Uint32(body[8:12])), 0),
	}, nil
}

func readSyncDentV2AfterID(r io.Reader) (SyncEntry, error) {
	var body [72]byte
	if _, err := io.ReadFull(r, body[:]); err != nil {
		return SyncEntry{}, err
	}
	nameLen := binary.LittleEndian.Uint32(body[68:72])
	name, err := readSyncName(r, nameLen)
	if err != nil {
		return SyncEntry{}, err
	}
	accessTime, err := syncUnixTime(binary.LittleEndian.Uint64(body[44:52]))
	if err != nil {
		return SyncEntry{}, err
	}
	modTime, err := syncUnixTime(binary.LittleEndian.Uint64(body[52:60]))
	if err != nil {
		return SyncEntry{}, err
	}
	changeTime, err := syncUnixTime(binary.LittleEndian.Uint64(body[60:68]))
	if err != nil {
		return SyncEntry{}, err
	}
	return SyncEntry{
		Name:       name,
		metadataV2: true,
		Errno:      binary.LittleEndian.Uint32(body[0:4]),
		Device:     binary.LittleEndian.Uint64(body[4:12]),
		Inode:      binary.LittleEndian.Uint64(body[12:20]),
		Mode:       binary.LittleEndian.Uint32(body[20:24]),
		NLink:      binary.LittleEndian.Uint32(body[24:28]),
		UID:        binary.LittleEndian.Uint32(body[28:32]),
		GID:        binary.LittleEndian.Uint32(body[32:36]),
		Size:       binary.LittleEndian.Uint64(body[36:44]),
		AccessTime: accessTime,
		ModTime:    modTime,
		ChangeTime: changeTime,
	}, nil
}

func syncUnixTime(seconds uint64) (time.Time, error) {
	if seconds > math.MaxInt64 {
		return time.Time{}, &SyncProtocolError{Operation: "metadata", Detail: fmt.Sprintf("timestamp %d is outside the int64 range", seconds)}
	}
	// #nosec G115 -- the explicit MaxInt64 check above makes this conversion lossless.
	return time.Unix(int64(seconds), 0), nil
}

func readSyncName(r io.Reader, size uint32) (string, error) {
	if size == 0 {
		return "", &SyncProtocolError{Operation: "list", Detail: "directory entry has an empty name"}
	}
	if size > syncMaxName {
		return "", &SyncProtocolError{Operation: "list", Detail: fmt.Sprintf("directory entry name is %d bytes (maximum %d)", size, syncMaxName)}
	}
	name := make([]byte, size)
	if _, err := io.ReadFull(r, name); err != nil {
		return "", err
	}
	nameString := string(name)
	if strings.IndexByte(nameString, 0) >= 0 {
		return "", &SyncProtocolError{Operation: "list", Detail: "directory entry name contains NUL"}
	}
	if strings.Contains(nameString, "/") {
		return "", &SyncProtocolError{Operation: "list", Detail: "directory entry name contains a path separator"}
	}
	// A backslash is legal on Android, but accepting one on a Windows host
	// would turn it into a separator when a generic VFS copy joins the name to
	// a local destination. AOSP's own adb client applies the same host-side rule.
	if runtime.GOOS == "windows" && strings.Contains(nameString, `\`) {
		return "", &SyncProtocolError{Operation: "list", Detail: "directory entry name contains a Windows path separator"}
	}
	return nameString, nil
}

func readSyncFailAfterID(r io.Reader, operation, path string) error {
	var sizeBytes [4]byte
	if _, err := io.ReadFull(r, sizeBytes[:]); err != nil {
		return fmt.Errorf("adb sync %s FAIL response: %w", operation, err)
	}
	return readSyncFailBody(r, operation, path, binary.LittleEndian.Uint32(sizeBytes[:]))
}

func readSyncFailBody(r io.Reader, operation, path string, size uint32) error {
	if size > syncMaxMessage {
		return &SyncProtocolError{Operation: operation, Detail: fmt.Sprintf("FAIL message is %d bytes (maximum %d)", size, syncMaxMessage)}
	}
	message := make([]byte, size)
	if _, err := io.ReadFull(r, message); err != nil {
		return fmt.Errorf("adb sync %s FAIL response: %w", operation, err)
	}
	return &SyncRemoteError{Operation: operation, Path: path, Message: string(message)}
}

func unexpectedSyncID(operation, got string, expected ...string) error {
	return &SyncProtocolError{
		Operation: operation,
		Detail:    fmt.Sprintf("unexpected message id %q (expected %s)", got, strings.Join(expected, ", ")),
	}
}

func syncContextError(ctx context.Context, err error) error {
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	return err
}
