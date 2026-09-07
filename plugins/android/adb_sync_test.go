package androidfs

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type syncTestConn struct {
	mu sync.Mutex

	response   []byte
	readPos    int
	readChunk  int
	writeChunk int
	writes     bytes.Buffer
	closed     bool
}

func (c *syncTestConn) Read(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.readPos == len(c.response) {
		return 0, io.EOF
	}
	limit := len(p)
	if c.readChunk > 0 && limit > c.readChunk {
		limit = c.readChunk
	}
	if remaining := len(c.response) - c.readPos; limit > remaining {
		limit = remaining
	}
	n := copy(p[:limit], c.response[c.readPos:c.readPos+limit])
	c.readPos += n
	return n, nil
}

func (c *syncTestConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	limit := len(p)
	if c.writeChunk > 0 && limit > c.writeChunk {
		limit = c.writeChunk
	}
	return c.writes.Write(p[:limit])
}

func (c *syncTestConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	return nil
}

func (c *syncTestConn) writtenBytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.writes.Bytes()...)
}

func (c *syncTestConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

type syncTestOpener struct {
	conn    io.ReadWriteCloser
	err     error
	calls   int
	serial  string
	service string
}

type syncCancelConn struct {
	closeOnce sync.Once
	closed    chan struct{}
}

func (c *syncCancelConn) Read([]byte) (int, error) { return 0, io.EOF }
func (c *syncCancelConn) Write(p []byte) (int, error) {
	return len(p), nil
}
func (c *syncCancelConn) Close() error {
	c.closeOnce.Do(func() { close(c.closed) })
	return nil
}

func (o *syncTestOpener) OpenService(_ context.Context, serial, service string) (io.ReadWriteCloser, error) {
	o.calls++
	o.serial = serial
	o.service = service
	return o.conn, o.err
}

func syncTestRequest(id, payload string) []byte {
	var result bytes.Buffer
	result.WriteString(id)
	_ = binary.Write(&result, binary.LittleEndian, syncTestStringLength(payload))
	result.WriteString(payload)
	return result.Bytes()
}

func syncTestHeader(id string, value uint32) []byte {
	var result [8]byte
	copy(result[:4], id)
	binary.LittleEndian.PutUint32(result[4:], value)
	return result[:]
}

func syncTestDentV1(name string, mode, size, mtime uint32) []byte {
	var result bytes.Buffer
	result.WriteString(syncIDDentV1)
	_ = binary.Write(&result, binary.LittleEndian, mode)
	_ = binary.Write(&result, binary.LittleEndian, size)
	_ = binary.Write(&result, binary.LittleEndian, mtime)
	_ = binary.Write(&result, binary.LittleEndian, syncTestStringLength(name))
	result.WriteString(name)
	return result.Bytes()
}

type syncTestV2Metadata struct {
	errno  uint32
	device uint64
	inode  uint64
	mode   uint32
	nlink  uint32
	uid    uint32
	gid    uint32
	size   uint64
	atime  int64
	mtime  int64
	ctime  int64
}

func syncTestV2Body(meta syncTestV2Metadata) []byte {
	var result bytes.Buffer
	_ = binary.Write(&result, binary.LittleEndian, meta.errno)
	_ = binary.Write(&result, binary.LittleEndian, meta.device)
	_ = binary.Write(&result, binary.LittleEndian, meta.inode)
	_ = binary.Write(&result, binary.LittleEndian, meta.mode)
	_ = binary.Write(&result, binary.LittleEndian, meta.nlink)
	_ = binary.Write(&result, binary.LittleEndian, meta.uid)
	_ = binary.Write(&result, binary.LittleEndian, meta.gid)
	_ = binary.Write(&result, binary.LittleEndian, meta.size)
	_ = binary.Write(&result, binary.LittleEndian, meta.atime)
	_ = binary.Write(&result, binary.LittleEndian, meta.mtime)
	_ = binary.Write(&result, binary.LittleEndian, meta.ctime)
	return result.Bytes()
}

func syncTestDentV2(name string, meta syncTestV2Metadata) []byte {
	var result bytes.Buffer
	result.WriteString(syncIDDentV2)
	result.Write(syncTestV2Body(meta))
	_ = binary.Write(&result, binary.LittleEndian, syncTestStringLength(name))
	result.WriteString(name)
	return result.Bytes()
}

func syncTestStringLength(value string) uint32 {
	return uint32(len(value)) // #nosec G115 -- sync protocol test strings are bounded well below its uint32 length field.
}

func TestSyncListV1HandlesFragmentedResponses(t *testing.T) {
	response := append(syncTestDentV1("hello.txt", 0100644, 123, 456), syncTestDentV1("sub", 0040755, 0, 789)...)
	response = append(response, []byte(syncIDDone)...)
	conn := &syncTestConn{response: response, readChunk: 1, writeChunk: 2}
	opener := &syncTestOpener{conn: conn}
	client := NewSyncClient(opener, "device-1", nil)

	entries, err := client.List(context.Background(), "/sdcard")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}
	if got := entries[0]; got.Name != "hello.txt" || got.Mode != 0100644 || got.Size != 123 || got.ModTime.Unix() != 456 {
		t.Fatalf("first entry = %#v", got)
	}
	if got := entries[1]; got.Name != "sub" || got.Mode != 0040755 || got.ModTime.Unix() != 789 {
		t.Fatalf("second entry = %#v", got)
	}
	if got, want := conn.writtenBytes(), syncTestRequest(syncIDListV1, "/sdcard"); !bytes.Equal(got, want) {
		t.Fatalf("request = %x, want %x", got, want)
	}
	if opener.calls != 1 || opener.serial != "device-1" || opener.service != "sync:" {
		t.Fatalf("open calls=%d serial=%q service=%q", opener.calls, opener.serial, opener.service)
	}
	if !conn.isClosed() {
		t.Fatal("LIST connection was not closed")
	}
}

func TestSyncListV2PreservesMetadataAndEntryErrno(t *testing.T) {
	good := syncTestV2Metadata{
		device: 11, inode: 22, mode: 0100600, nlink: 2, uid: 1000, gid: 1001,
		size: 1<<40 + 7, atime: 100, mtime: 200, ctime: 300,
	}
	missing := syncTestV2Metadata{errno: 2}
	response := append(syncTestDentV2("large.bin", good), syncTestDentV2("vanished", missing)...)
	response = append(response, []byte(syncIDDone)...)
	conn := &syncTestConn{response: response, readChunk: 3}
	client := NewSyncClient(&syncTestOpener{conn: conn}, "serial", map[string]bool{"ls_v2": true})

	entries, err := client.List(context.Background(), "/data/local/tmp")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entry count = %d, want 2", len(entries))
	}
	got := entries[0]
	if got.Name != "large.bin" || got.Device != 11 || got.Inode != 22 || got.Mode != 0100600 || got.NLink != 2 || got.UID != 1000 || got.GID != 1001 || got.Size != 1<<40+7 {
		t.Fatalf("v2 metadata = %#v", got)
	}
	if got.AccessTime.Unix() != 100 || got.ModTime.Unix() != 200 || got.ChangeTime.Unix() != 300 {
		t.Fatalf("v2 times = %v %v %v", got.AccessTime, got.ModTime, got.ChangeTime)
	}
	if !errors.Is(entries[1].Err(), fs.ErrNotExist) {
		t.Fatalf("entry error = %v, want errno 2", entries[1].Err())
	}
	if got, want := conn.writtenBytes(), syncTestRequest(syncIDListV2, "/data/local/tmp"); !bytes.Equal(got, want) {
		t.Fatalf("request = %x, want %x", got, want)
	}
}

func TestSyncListRejectsMalformedResponseAndFAIL(t *testing.T) {
	t.Run("oversized name", func(t *testing.T) {
		var response bytes.Buffer
		response.WriteString(syncIDDentV1)
		_ = binary.Write(&response, binary.LittleEndian, uint32(0100644))
		_ = binary.Write(&response, binary.LittleEndian, uint32(1))
		_ = binary.Write(&response, binary.LittleEndian, uint32(2))
		_ = binary.Write(&response, binary.LittleEndian, uint32(syncMaxName+1))
		client := NewSyncClient(&syncTestOpener{conn: &syncTestConn{response: response.Bytes()}}, "serial", nil)
		_, err := client.List(context.Background(), "/x")
		var protocolErr *SyncProtocolError
		if !errors.As(err, &protocolErr) {
			t.Fatalf("error = %v, want SyncProtocolError", err)
		}
	})

	t.Run("path separator in name", func(t *testing.T) {
		response := append(syncTestDentV1("../../outside", 0100644, 1, 2), []byte(syncIDDone)...)
		client := NewSyncClient(&syncTestOpener{conn: &syncTestConn{response: response}}, "serial", nil)
		_, err := client.List(context.Background(), "/x")
		var protocolErr *SyncProtocolError
		if !errors.As(err, &protocolErr) {
			t.Fatalf("error = %v, want SyncProtocolError", err)
		}
	})

	if runtime.GOOS == "windows" {
		t.Run("Windows path separator in name", func(t *testing.T) {
			response := append(syncTestDentV1(`..\outside`, 0100644, 1, 2), []byte(syncIDDone)...)
			client := NewSyncClient(&syncTestOpener{conn: &syncTestConn{response: response}}, "serial", nil)
			_, err := client.List(context.Background(), "/x")
			var protocolErr *SyncProtocolError
			if !errors.As(err, &protocolErr) {
				t.Fatalf("error = %v, want SyncProtocolError", err)
			}
		})
	}

	t.Run("remote fail", func(t *testing.T) {
		message := "permission denied"
		response := append(syncTestHeader(syncIDFail, syncTestStringLength(message)), message...)
		client := NewSyncClient(&syncTestOpener{conn: &syncTestConn{response: response, readChunk: 1}}, "serial", nil)
		_, err := client.List(context.Background(), "/root")
		var remoteErr *SyncRemoteError
		if !errors.As(err, &remoteErr) || remoteErr.Message != message || remoteErr.Operation != "list" || remoteErr.Path != "/root" {
			t.Fatalf("error = %#v", err)
		}
	})
}

func TestSyncStatV1AndV2(t *testing.T) {
	t.Run("legacy lstat", func(t *testing.T) {
		var response bytes.Buffer
		response.WriteString(syncIDLstatV1)
		_ = binary.Write(&response, binary.LittleEndian, uint32(0100640))
		_ = binary.Write(&response, binary.LittleEndian, uint32(77))
		_ = binary.Write(&response, binary.LittleEndian, uint32(1234))
		conn := &syncTestConn{response: response.Bytes(), readChunk: 2}
		client := NewSyncClient(&syncTestOpener{conn: conn}, "serial", nil)
		entry, err := client.Lstat(context.Background(), "/a/file")
		if err != nil {
			t.Fatalf("Lstat: %v", err)
		}
		if entry.Name != "file" || entry.Mode != 0100640 || entry.Size != 77 || entry.ModTime.Unix() != 1234 {
			t.Fatalf("entry = %#v", entry)
		}
		if got, want := conn.writtenBytes(), syncTestRequest(syncIDLstatV1, "/a/file"); !bytes.Equal(got, want) {
			t.Fatalf("request = %x, want %x", got, want)
		}
	})

	t.Run("legacy missing path", func(t *testing.T) {
		response := append([]byte(syncIDLstatV1), make([]byte, 12)...)
		client := NewSyncClient(&syncTestOpener{conn: &syncTestConn{response: response}}, "serial", nil)
		_, err := client.Lstat(context.Background(), "/missing")
		if !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("error = %v, want ENOENT", err)
		}
	})

	t.Run("v2 stat follows symlink", func(t *testing.T) {
		meta := syncTestV2Metadata{device: 3, inode: 4, mode: 0120777, nlink: 1, uid: 2000, gid: 2001, size: 12, atime: 10, mtime: 20, ctime: 30}
		response := append([]byte(syncIDStatV2), syncTestV2Body(meta)...)
		conn := &syncTestConn{response: response, readChunk: 1}
		client := NewSyncClient(&syncTestOpener{conn: conn}, "serial", map[string]bool{"stat_v2": true})
		entry, err := client.Stat(context.Background(), "/link")
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if entry.Name != "link" || entry.Device != 3 || entry.Inode != 4 || entry.UID != 2000 || entry.Size != 12 {
			t.Fatalf("entry = %#v", entry)
		}
		if got, want := conn.writtenBytes(), syncTestRequest(syncIDStatV2, "/link"); !bytes.Equal(got, want) {
			t.Fatalf("request = %x, want %x", got, want)
		}
	})

	t.Run("v2 errno", func(t *testing.T) {
		response := append([]byte(syncIDLstatV2), syncTestV2Body(syncTestV2Metadata{errno: 13})...)
		client := NewSyncClient(&syncTestOpener{conn: &syncTestConn{response: response}}, "serial", map[string]bool{"stat_v2": true})
		_, err := client.Lstat(context.Background(), "/secret")
		if !errors.Is(err, fs.ErrPermission) {
			t.Fatalf("error = %v, want permission error", err)
		}
	})
}

func TestSyncRemoteErrorMapsLinuxWireErrnoPortably(t *testing.T) {
	for wire, want := range map[uint32]error{
		linuxENOENT:  fs.ErrNotExist,
		linuxENOTDIR: fs.ErrNotExist,
		linuxEPERM:   fs.ErrPermission,
		linuxEACCES:  fs.ErrPermission,
		linuxEROFS:   fs.ErrPermission,
		linuxEEXIST:  fs.ErrExist,
	} {
		err := &SyncRemoteError{Errno: wire}
		if !errors.Is(err, want) {
			t.Errorf("wire errno %d does not map to %v: %v", wire, want, err)
		}
	}
	var linuxErr LinuxErrno
	if err := (&SyncRemoteError{Errno: 123}); !errors.As(err, &linuxErr) || linuxErr != 123 {
		t.Fatalf("unknown wire errno = %v, %v", linuxErr, err)
	}
}

func TestSyncReceiveV1AndV2(t *testing.T) {
	t.Run("v1 fragmented DATA", func(t *testing.T) {
		response := append(syncTestHeader(syncIDData, 5), []byte("hello")...)
		response = append(response, syncTestHeader(syncIDData, 6)...)
		response = append(response, []byte(" world")...)
		response = append(response, syncTestHeader(syncIDDone, 0)...)
		conn := &syncTestConn{response: response, readChunk: 1, writeChunk: 1}
		client := NewSyncClient(&syncTestOpener{conn: conn}, "serial", nil)
		reader, err := client.Receive(context.Background(), "/remote/file")
		if err != nil {
			t.Fatalf("Receive: %v", err)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		if string(data) != "hello world" {
			t.Fatalf("data = %q", data)
		}
		if got, want := conn.writtenBytes(), syncTestRequest(syncIDRecvV1, "/remote/file"); !bytes.Equal(got, want) {
			t.Fatalf("request = %x, want %x", got, want)
		}
		if !conn.isClosed() {
			t.Fatal("RECV connection was not closed at DONE")
		}
	})

	t.Run("v2 request and remote fail", func(t *testing.T) {
		message := "not a file"
		response := append(syncTestHeader(syncIDFail, syncTestStringLength(message)), message...)
		conn := &syncTestConn{response: response, readChunk: 2}
		client := NewSyncClient(&syncTestOpener{conn: conn}, "serial", map[string]bool{"sendrecv_v2": true})
		reader, err := client.Receive(context.Background(), "/dir")
		if err != nil {
			t.Fatalf("Receive: %v", err)
		}
		_, err = io.ReadAll(reader)
		var remoteErr *SyncRemoteError
		if !errors.As(err, &remoteErr) || remoteErr.Message != message {
			t.Fatalf("error = %#v", err)
		}
		want := append(syncTestRequest(syncIDRecvV2, "/dir"), syncTestHeader(syncIDRecvV2, 0)...)
		if got := conn.writtenBytes(); !bytes.Equal(got, want) {
			t.Fatalf("request = %x, want %x", got, want)
		}
	})

	t.Run("oversized DATA", func(t *testing.T) {
		response := syncTestHeader(syncIDData, SyncMaxData+1)
		client := NewSyncClient(&syncTestOpener{conn: &syncTestConn{response: response}}, "serial", nil)
		reader, err := client.Receive(context.Background(), "/file")
		if err != nil {
			t.Fatalf("Receive: %v", err)
		}
		_, err = reader.Read(make([]byte, 1))
		var protocolErr *SyncProtocolError
		if !errors.As(err, &protocolErr) {
			t.Fatalf("error = %v, want SyncProtocolError", err)
		}
	})
}

func TestSyncSendV1SplitsLargeWritesAndFinalizes(t *testing.T) {
	conn := &syncTestConn{response: syncTestHeader(syncIDOkay, 0), readChunk: 1, writeChunk: 3}
	client := NewSyncClient(&syncTestOpener{conn: conn}, "serial", nil)
	mtime := time.Unix(123456, 0)
	writer, err := client.Send(context.Background(), "/remote/file", 0100644, mtime)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	data := bytes.Repeat([]byte{'x'}, SyncMaxData+5)
	if n, err := writer.Write(data); err != nil || n != len(data) {
		t.Fatalf("Write = %d, %v; want %d, nil", n, err, len(data))
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	payload := "/remote/file," + "33188" // decimal form of 0100644
	want := append([]byte(nil), syncTestRequest(syncIDSendV1, payload)...)
	want = append(want, syncTestHeader(syncIDData, SyncMaxData)...)
	want = append(want, data[:SyncMaxData]...)
	want = append(want, syncTestHeader(syncIDData, 5)...)
	want = append(want, data[SyncMaxData:]...)
	want = append(want, syncTestHeader(syncIDDone, 123456)...)
	if got := conn.writtenBytes(); !bytes.Equal(got, want) {
		t.Fatalf("wire output differs: got %d bytes, want %d", len(got), len(want))
	}
	if !conn.isClosed() {
		t.Fatal("SEND connection was not closed")
	}
}

func TestSyncSendV2SetupAndFAIL(t *testing.T) {
	message := "read-only file system"
	response := append(syncTestHeader(syncIDFail, syncTestStringLength(message)), message...)
	conn := &syncTestConn{response: response, readChunk: 2, writeChunk: 2}
	client := NewSyncClient(&syncTestOpener{conn: conn}, "serial", map[string]bool{"sendrecv_v2": true})
	writer, err := client.Send(context.Background(), "/system/file", 0100600, time.Unix(99, 0))
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if _, err := writer.Write([]byte("abc")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	err = writer.Close()
	var remoteErr *SyncRemoteError
	if !errors.As(err, &remoteErr) || remoteErr.Message != message || remoteErr.Path != "/system/file" {
		t.Fatalf("Close error = %#v", err)
	}

	want := append([]byte(nil), syncTestRequest(syncIDSendV2, "/system/file")...)
	setup := make([]byte, 12)
	copy(setup[:4], syncIDSendV2)
	binary.LittleEndian.PutUint32(setup[4:8], 0100600)
	want = append(want, setup...)
	want = append(want, syncTestHeader(syncIDData, 3)...)
	want = append(want, "abc"...)
	want = append(want, syncTestHeader(syncIDDone, 99)...)
	if got := conn.writtenBytes(); !bytes.Equal(got, want) {
		t.Fatalf("wire output = %x, want %x", got, want)
	}
}

func TestSyncInputAndProtocolBounds(t *testing.T) {
	opener := &syncTestOpener{conn: &syncTestConn{}}
	client := NewSyncClient(opener, "serial", nil)
	tooLong := "/" + strings.Repeat("x", SyncMaxPath)
	if _, err := client.List(context.Background(), tooLong); err == nil {
		t.Fatal("List accepted an oversized path")
	}
	if opener.calls != 0 {
		t.Fatalf("opener called %d times for invalid path", opener.calls)
	}
	if _, err := client.Stat(context.Background(), "/bad\x00path"); err == nil {
		t.Fatal("Stat accepted NUL in path")
	}
	if _, err := client.Send(context.Background(), "/file", 0, time.Unix(-1, 0)); err == nil {
		t.Fatal("Send accepted a negative timestamp")
	}
}

func TestSyncOpenAndContextErrors(t *testing.T) {
	t.Run("open error", func(t *testing.T) {
		want := errors.New("server unavailable")
		client := NewSyncClient(&syncTestOpener{err: want}, "serial", nil)
		_, err := client.List(context.Background(), "/")
		if !errors.Is(err, want) {
			t.Fatalf("error = %v, want %v", err, want)
		}
	})

	t.Run("already canceled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		opener := &syncTestOpener{conn: &syncTestConn{}}
		client := NewSyncClient(opener, "serial", nil)
		_, err := client.List(ctx, "/")
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
		if opener.calls != 0 {
			t.Fatalf("opener called %d times", opener.calls)
		}
	})

	t.Run("cancel closes active stream", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		conn := &syncCancelConn{closed: make(chan struct{})}
		client := NewSyncClient(&syncTestOpener{conn: conn}, "serial", nil)
		reader, err := client.Receive(ctx, "/file")
		if err != nil {
			t.Fatalf("Receive: %v", err)
		}
		cancel()
		select {
		case <-conn.closed:
		case <-time.After(time.Second):
			t.Fatal("context cancellation did not close the sync stream")
		}
		if err := reader.Close(); err != nil {
			t.Fatalf("reader Close after cancellation: %v", err)
		}
	})
}
