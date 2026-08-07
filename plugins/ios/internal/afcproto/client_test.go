package afcproto

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"
)

type serverStep struct {
	op       opcode
	check    func(packet) error
	response packet
}

func dataResponse(payload []byte) packet {
	return packet{header: packetHeader{Operation: opData}, payload: payload}
}

func statusResponse(code uint64) packet {
	return packet{header: packetHeader{Operation: opStatus}, headerPayload: putUint64s(code)}
}

func openResponse(handle uint64) packet {
	return packet{header: packetHeader{Operation: opFileOpenResult}, headerPayload: putUint64s(handle)}
}

func runServer(conn net.Conn, steps []serverStep) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer conn.Close()
		for i, step := range steps {
			number := uint64(i + 1)
			req, err := readPacket(conn, number)
			if err != nil {
				done <- fmt.Errorf("step %d read: %w", i, err)
				return
			}
			if req.header.Operation != step.op {
				done <- fmt.Errorf("step %d opcode = %#x, want %#x", i, req.header.Operation, step.op)
				return
			}
			if step.check != nil {
				if err := step.check(req); err != nil {
					done <- fmt.Errorf("step %d: %w", i, err)
					return
				}
			}
			resp := step.response
			if err := writePacket(conn, number, resp.header.Operation, resp.headerPayload, resp.payload); err != nil {
				done <- fmt.Errorf("step %d write: %w", i, err)
				return
			}
		}
		done <- nil
	}()
	return done
}

func newScriptedClient(t *testing.T, steps []serverStep) (*Client, <-chan error) {
	t.Helper()
	clientConn, serverConn := net.Pipe()
	return New(clientConn), runServer(serverConn, steps)
}

func finishScript(t *testing.T, client *Client, done <-chan error) {
	t.Helper()
	_ = client.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func nulDict(values ...string) []byte {
	var out []byte
	for _, value := range values {
		out = append(out, value...)
		out = append(out, 0)
	}
	return out
}

func TestClientMetadataOperations(t *testing.T) {
	mtime := int64(1_725_000_000_123_456_789)
	steps := []serverStep{
		{op: opReadDir, check: headerEquals("/Downloads\x00"), response: dataResponse(nulDict(".", "..", "a.txt", "Folder"))},
		{op: opGetFileInfo, check: headerEquals("/Downloads/a.txt\x00"), response: dataResponse(nulDict(
			"st_ifmt", "S_IFREG", "st_size", "42", "st_mode", "0644", "st_mtime", fmt.Sprint(mtime),
		))},
		{op: opGetDeviceInfo, response: dataResponse(nulDict(
			"Model", "iPhone15,2", "FSTotalBytes", "1000", "FSFreeBytes", "400", "FSBlockSize", "4096",
		))},
	}
	client, done := newScriptedClient(t, steps)

	entries, err := client.List(context.Background(), "/Downloads")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(entries, []string{"a.txt", "Folder"}) {
		t.Fatalf("entries = %#v", entries)
	}
	info, err := client.Stat(context.Background(), "/Downloads/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if info.Name != "a.txt" || info.Type != TypeRegular || info.Size != 42 || info.Mode != 0644 || info.ModTime.UnixNano() != mtime {
		t.Fatalf("unexpected file info: %#v", info)
	}
	device, err := client.DeviceInfo(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if device.Model != "iPhone15,2" || device.TotalBytes != 1000 || device.FreeBytes != 400 || device.BlockSize != 4096 {
		t.Fatalf("unexpected device info: %#v", device)
	}
	finishScript(t, client, done)
}

func TestClientMutationsWireFormat(t *testing.T) {
	mtime := time.Unix(1_700_000_000, 123)
	steps := []serverStep{
		{op: opMakeDir, check: headerEquals("/Downloads/new\x00"), response: statusResponse(0)},
		{op: opRenamePath, check: headerEquals("/Downloads/new\x00/Downloads/moved\x00"), response: statusResponse(0)},
		{op: opSetFileModTime, check: func(p packet) error {
			if len(p.headerPayload) < 8 || binary.LittleEndian.Uint64(p.headerPayload) != uint64(mtime.UnixNano()) {
				return fmt.Errorf("wrong mtime header %x", p.headerPayload)
			}
			if string(p.headerPayload[8:]) != "/Downloads/moved\x00" {
				return fmt.Errorf("wrong mtime path %q", p.headerPayload[8:])
			}
			return nil
		}, response: statusResponse(0)},
		{op: opRemovePath, check: headerEquals("/Downloads/moved\x00"), response: statusResponse(0)},
		{op: opRemovePathAndContents, check: headerEquals("/Downloads/tree\x00"), response: statusResponse(0)},
	}
	client, done := newScriptedClient(t, steps)
	ctx := context.Background()
	for _, call := range []func() error{
		func() error { return client.MkDir(ctx, "/Downloads/new") },
		func() error { return client.Rename(ctx, "/Downloads/new", "/Downloads/moved") },
		func() error { return client.SetModTime(ctx, "/Downloads/moved", mtime) },
		func() error { return client.Remove(ctx, "/Downloads/moved") },
		func() error { return client.RemoveAll(ctx, "/Downloads/tree") },
	} {
		if err := call(); err != nil {
			t.Fatal(err)
		}
	}
	finishScript(t, client, done)
}

func TestFileReadReadAtWriteTruncateAndClose(t *testing.T) {
	steps := []serverStep{
		{op: opGetFileInfo, response: dataResponse(nulDict("st_ifmt", "S_IFREG", "st_size", "6"))},
		{op: opFileOpen, check: func(p packet) error {
			if binary.LittleEndian.Uint64(p.headerPayload) != uint64(ModeReadWriteCreate) || string(p.headerPayload[8:]) != "/file\x00" {
				return fmt.Errorf("bad open payload %x", p.headerPayload)
			}
			return nil
		}, response: openResponse(42)},
		{op: opFileRead, check: fileRequest(42, 2), response: dataResponse([]byte("ab"))},
		{op: opFileSeek, check: seekRequest(42, 4), response: statusResponse(0)},
		{op: opFileRead, check: fileRequest(42, 2), response: dataResponse([]byte("ef"))},
		{op: opFileSeek, check: seekRequest(42, 2), response: statusResponse(0)},
		{op: opFileWrite, check: func(p packet) error {
			if len(p.headerPayload) != 8 || binary.LittleEndian.Uint64(p.headerPayload) != 42 || string(p.payload) != "XYZ" {
				return fmt.Errorf("bad write request")
			}
			return nil
		}, response: statusResponse(0)},
		{op: opFileSetSize, check: fileRequest(42, 4), response: statusResponse(0)},
		{op: opFileClose, check: fileRequest(42), response: statusResponse(0)},
	}
	client, done := newScriptedClient(t, steps)
	file, err := client.Open(context.Background(), "/file", ModeReadWriteCreate)
	if err != nil {
		t.Fatal(err)
	}
	if file.Size() != 6 {
		t.Fatalf("initial size = %d", file.Size())
	}
	b := make([]byte, 2)
	if n, err := file.ReadContext(context.Background(), b); n != 2 || err != nil || string(b) != "ab" {
		t.Fatalf("Read = %d, %v, %q", n, err, b)
	}
	if n, err := file.ReadAtContext(context.Background(), b, 4); n != 2 || err != nil || string(b) != "ef" {
		t.Fatalf("ReadAt = %d, %v, %q", n, err, b)
	}
	if n, err := file.WriteContext(context.Background(), []byte("XYZ")); n != 3 || err != nil {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if file.Size() != 6 {
		t.Fatalf("size after overwrite = %d", file.Size())
	}
	if err := file.TruncateContext(context.Background(), 4); err != nil {
		t.Fatal(err)
	}
	if file.Size() != 4 {
		t.Fatalf("size after truncate = %d", file.Size())
	}
	if err := file.CloseContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	finishScript(t, client, done)
}

func TestFileWriteIsChunked(t *testing.T) {
	payload := make([]byte, maxIOChunk+9)
	steps := []serverStep{
		{op: opGetFileInfo, response: statusResponse(8)},
		{op: opFileOpen, response: openResponse(9)},
		{op: opFileWrite, check: payloadLength(maxIOChunk), response: statusResponse(0)},
		{op: opFileWrite, check: payloadLength(9), response: statusResponse(0)},
		{op: opFileClose, response: statusResponse(0)},
	}
	client, done := newScriptedClient(t, steps)
	file, err := client.Open(context.Background(), "/new", ModeWriteOnlyCreateTruncate)
	if err != nil {
		t.Fatal(err)
	}
	if n, err := file.Write(payload); n != len(payload) || err != nil {
		t.Fatalf("Write = %d, %v", n, err)
	}
	if file.Size() != int64(len(payload)) {
		t.Fatalf("size = %d", file.Size())
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	finishScript(t, client, done)
}

func TestAppendOpenStartsAtExistingSize(t *testing.T) {
	steps := []serverStep{
		{op: opGetFileInfo, response: dataResponse(nulDict("st_ifmt", "S_IFREG", "st_size", "12"))},
		{op: opFileOpen, response: openResponse(10)},
		{op: opFileWrite, response: statusResponse(0)},
		{op: opFileClose, response: statusResponse(0)},
	}
	client, done := newScriptedClient(t, steps)
	file, err := client.Open(context.Background(), "/append", ModeWriteOnlyCreateAppend)
	if err != nil {
		t.Fatal(err)
	}
	if file.offset != 12 {
		t.Fatalf("append offset = %d", file.offset)
	}
	if _, err := file.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if file.Size() != 13 {
		t.Fatalf("append size = %d", file.Size())
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	finishScript(t, client, done)
}

func TestStatusMappingDoesNotPoisonConnection(t *testing.T) {
	steps := []serverStep{
		{op: opGetFileInfo, response: statusResponse(8)},
		{op: opReadDir, response: dataResponse(nulDict("ok"))},
	}
	client, done := newScriptedClient(t, steps)
	_, err := client.Stat(context.Background(), "/missing")
	if !errors.Is(err, fs.ErrNotExist) || IsConnectionLost(err) {
		t.Fatalf("Stat error = %v", err)
	}
	entries, err := client.List(context.Background(), "/")
	if err != nil || !slices.Equal(entries, []string{"ok"}) {
		t.Fatalf("List after status error = %#v, %v", entries, err)
	}
	finishScript(t, client, done)
}

func TestMalformedResponsesPoisonConnection(t *testing.T) {
	tests := map[string]func(packetHeader) packetHeader{
		"magic":             func(h packetHeader) packetHeader { h.Magic = 0; return h },
		"packet number":     func(h packetHeader) packetHeader { h.PacketNum++; return h },
		"short this length": func(h packetHeader) packetHeader { h.ThisLen = headerSize - 1; return h },
		"inverted lengths":  func(h packetHeader) packetHeader { h.EntireLen = headerSize; h.ThisLen = headerSize + 1; return h },
		"oversized":         func(h packetHeader) packetHeader { h.EntireLen = maxPacketSize + 1; return h },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			clientConn, serverConn := net.Pipe()
			client := New(clientConn)
			done := make(chan error, 1)
			go func() {
				defer serverConn.Close()
				if _, err := readPacket(serverConn, 1); err != nil {
					done <- err
					return
				}
				h := packetHeader{Magic: magic, EntireLen: headerSize, ThisLen: headerSize, PacketNum: 1, Operation: opData}
				h = mutate(h)
				done <- binary.Write(serverConn, binary.LittleEndian, h)
			}()
			_, err := client.List(context.Background(), "/")
			if !IsConnectionLost(err) || !errors.Is(err, ErrProtocol) {
				t.Fatalf("error = %v", err)
			}
			if _, again := client.List(context.Background(), "/"); !IsConnectionLost(again) {
				t.Fatalf("subsequent error = %v", again)
			}
			_ = client.Close()
			if serverErr := <-done; serverErr != nil {
				t.Fatal(serverErr)
			}
		})
	}
}

func TestUnsafePathsAndDirectoryEntries(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := New(clientConn)
	defer client.Close()
	defer serverConn.Close()
	for _, call := range []func() error{
		func() error { _, err := client.List(context.Background(), "/safe/../secret"); return err },
		func() error { return client.Remove(context.Background(), "/") },
		func() error { return client.RemoveAll(context.Background(), ".") },
		func() error { return client.Rename(context.Background(), "/safe", "../escape") },
		func() error { return client.MkDir(context.Background(), "bad\x00name") },
	} {
		if err := call(); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("unsafe path error = %v", err)
		}
	}
	if err := client.SetModTime(context.Background(), "/safe", time.Date(2500, 1, 1, 0, 0, 0, 0, time.UTC)); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("out-of-range mtime error = %v", err)
	}

	client.Close()
	client, done := newScriptedClient(t, []serverStep{{op: opReadDir, response: dataResponse(nulDict("safe", "nested/name"))}})
	_, err := client.List(context.Background(), "/")
	if !IsConnectionLost(err) || !errors.Is(err, ErrProtocol) {
		t.Fatalf("unsafe entry error = %v", err)
	}
	finishScript(t, client, done)
}

func TestMalformedDataShapePoisonsConnection(t *testing.T) {
	client, done := newScriptedClient(t, []serverStep{{
		op: opReadDir,
		response: packet{
			header:        packetHeader{Operation: opData},
			headerPayload: []byte("unexpected"),
			payload:       nulDict("file"),
		},
	}})
	_, err := client.List(context.Background(), "/")
	if !IsConnectionLost(err) || !errors.Is(err, ErrProtocol) {
		t.Fatalf("error = %v", err)
	}
	finishScript(t, client, done)
}

func TestProtocolStatusFailurePoisonsConnection(t *testing.T) {
	client, done := newScriptedClient(t, []serverStep{{op: opReadDir, response: statusResponse(2)}})
	_, err := client.List(context.Background(), "/")
	if !IsConnectionLost(err) || !errors.Is(err, ErrProtocol) {
		t.Fatalf("error = %v", err)
	}
	finishScript(t, client, done)
}

func TestCancellationClosesAndPoisonsConnection(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	client := New(clientConn)
	requestRead := make(chan error, 1)
	go func() {
		defer serverConn.Close()
		_, err := readPacket(serverConn, 1)
		requestRead <- err
		if err == nil {
			var one [1]byte
			_, _ = serverConn.Read(one[:]) // waits for the cancellation-driven close
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	_, err := client.List(ctx, "/")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancelled List error = %v", err)
	}
	if !IsConnectionLost(err) {
		t.Fatalf("cancelled List did not report a poisoned connection: %v", err)
	}
	if serverErr := <-requestRead; serverErr != nil {
		t.Fatal(serverErr)
	}
	if _, err = client.List(context.Background(), "/"); !IsConnectionLost(err) {
		t.Fatalf("subsequent error = %v", err)
	}
	_ = client.Close()
}

func TestClientSerializesWholeExchange(t *testing.T) {
	clientConn, rawServer := net.Pipe()
	serverConn := rawServer.(net.Conn)
	client := New(clientConn)
	serverDone := make(chan error, 1)
	go func() {
		defer serverConn.Close()
		if _, err := readPacket(serverConn, 1); err != nil {
			serverDone <- err
			return
		}
		_ = serverConn.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
		var b [1]byte
		if _, err := serverConn.Read(b[:]); err == nil {
			serverDone <- errors.New("second request arrived before first response")
			return
		} else if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
			serverDone <- fmt.Errorf("deadline read: %w", err)
			return
		}
		_ = serverConn.SetReadDeadline(time.Time{})
		if err := writePacket(serverConn, 1, opData, nil, nulDict("first")); err != nil {
			serverDone <- err
			return
		}
		if _, err := readPacket(serverConn, 2); err != nil {
			serverDone <- err
			return
		}
		serverDone <- writePacket(serverConn, 2, opData, nil, nulDict("second"))
	}()

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := client.List(context.Background(), "/")
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	_ = client.Close()
}

func TestStatusSentinelMappings(t *testing.T) {
	tests := []struct {
		code uint64
		want error
	}{
		{7, fs.ErrInvalid}, {8, fs.ErrNotExist}, {10, fs.ErrPermission}, {11, ErrConnectionLost},
		{14, io.EOF}, {15, errors.ErrUnsupported}, {16, fs.ErrExist}, {18, syscall.ENOSPC},
		{33, syscall.ENOTEMPTY},
	}
	for _, tt := range tests {
		if err := statusError(tt.code); !errors.Is(err, tt.want) {
			t.Errorf("status %d: %v does not wrap %v", tt.code, err, tt.want)
		}
	}
}

func headerEquals(want string) func(packet) error {
	return func(p packet) error {
		if got := string(p.headerPayload); got != want {
			return fmt.Errorf("header payload = %q, want %q", got, want)
		}
		return nil
	}
}

func fileRequest(values ...uint64) func(packet) error {
	want := putUint64s(values...)
	return func(p packet) error {
		if !slices.Equal(p.headerPayload, want) {
			return fmt.Errorf("file payload = %x, want %x", p.headerPayload, want)
		}
		return nil
	}
}

func seekRequest(handle uint64, offset int64) func(packet) error {
	return fileRequest(handle, uint64(io.SeekStart), uint64(offset))
}

func payloadLength(want int) func(packet) error {
	return func(p packet) error {
		if len(p.payload) != want {
			return fmt.Errorf("payload length = %d, want %d", len(p.payload), want)
		}
		return nil
	}
}
