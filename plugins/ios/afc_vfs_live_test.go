package iosfs

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

// This file drives AFCVFS end to end over a real, deterministic in-process
// wire connection instead of mocking the AFC client, so it can exercise the
// success paths that only ever run once a probe/stat/mutation round-trip
// actually completes. It re-implements just enough of the tiny wire format
// from plugins/ios/internal/afcproto/protocol.go (a 40-byte little-endian
// header followed by an optional header payload and payload) to script a
// fake AFC device; it never reaches into that package's unexported
// identifiers.
const (
	fakeAFCMagic      uint64 = 0x4141504c36414643
	fakeAFCHeaderSize uint64 = 40
)

const (
	fakeOpStatus                uint64 = 0x01
	fakeOpData                  uint64 = 0x02
	fakeOpReadDir               uint64 = 0x03
	fakeOpMakeDir               uint64 = 0x09
	fakeOpGetFileInfo           uint64 = 0x0a
	fakeOpGetDeviceInfo         uint64 = 0x0b
	fakeOpRenamePath            uint64 = 0x18
	fakeOpSetFileModTime        uint64 = 0x1e
	fakeOpRemovePathAndContents uint64 = 0x22
)

// fakeAFCStatusOK is the 8-byte, all-zero status header payload that the real
// client (see (*Client).exchange in internal/afcproto) reads as success.
func fakeAFCStatusOK() []byte { return make([]byte, 8) }

// fakeAFCNULItems concatenates each item with a trailing NUL, the wire
// encoding internal/afcproto uses both for plain string lists (readdir) and
// for flattened key/value dictionaries (stat, device info).
func fakeAFCNULItems(items ...string) []byte {
	var buf []byte
	for _, item := range items {
		buf = append(buf, item...)
		buf = append(buf, 0)
	}
	return buf
}

func writeFakeAFCPacket(w io.Writer, number, operation uint64, headerPayload, payload []byte) error {
	thisLen := fakeAFCHeaderSize + uint64(len(headerPayload))
	entireLen := thisLen + uint64(len(payload))
	var raw [40]byte
	binary.LittleEndian.PutUint64(raw[0:8], fakeAFCMagic)
	binary.LittleEndian.PutUint64(raw[8:16], entireLen)
	binary.LittleEndian.PutUint64(raw[16:24], thisLen)
	binary.LittleEndian.PutUint64(raw[24:32], number)
	binary.LittleEndian.PutUint64(raw[32:40], operation)
	if _, err := w.Write(raw[:]); err != nil {
		return err
	}
	if len(headerPayload) != 0 {
		if _, err := w.Write(headerPayload); err != nil {
			return err
		}
	}
	if len(payload) != 0 {
		if _, err := w.Write(payload); err != nil {
			return err
		}
	}
	return nil
}

// readFakeAFCRequest reads one full incoming request packet and reports only
// its operation code: the scripted server below does not need to inspect the
// request body to answer deterministically, since the test drives the exact
// call sequence that produced it.
func readFakeAFCRequest(r io.Reader) (operation uint64, err error) {
	var raw [40]byte
	if _, err := io.ReadFull(r, raw[:]); err != nil {
		return 0, err
	}
	entireLen := binary.LittleEndian.Uint64(raw[8:16])
	operation = binary.LittleEndian.Uint64(raw[32:40])
	if rest := entireLen - fakeAFCHeaderSize; rest > 0 {
		if _, err := io.CopyN(io.Discard, r, int64(rest)); err != nil {
			return 0, err
		}
	}
	return operation, nil
}

type fakeAFCStep struct {
	wantOp        uint64
	respOp        uint64
	headerPayload []byte
	payload       []byte
}

// runFakeAFCServer plays back steps in order over conn, one request/response
// exchange at a time, mirroring how internal/afcproto.Client itself
// serializes every exchange behind a single mutex. It reports the first
// mismatch (or transport error) on the returned channel.
func runFakeAFCServer(conn io.ReadWriteCloser, steps []fakeAFCStep) <-chan error {
	done := make(chan error, 1)
	go func() {
		defer func() { _ = conn.Close() }()
		for i, step := range steps {
			op, err := readFakeAFCRequest(conn)
			if err != nil {
				done <- fmt.Errorf("step %d: read request: %w", i, err)
				return
			}
			if op != step.wantOp {
				done <- fmt.Errorf("step %d: request op %#x, want %#x", i, op, step.wantOp)
				return
			}
			if err := writeFakeAFCPacket(conn, uint64(i+1), step.respOp, step.headerPayload, step.payload); err != nil {
				done <- fmt.Errorf("step %d: write response: %w", i, err)
				return
			}
		}
		done <- nil
	}()
	return done
}

// fakeAFCSingleDial provisions exactly one pre-built connection: every dial
// after the first fails, which keeps AFCVFS's opportunistic transfer-pool
// growth (see AFCVFS.ReadDir) from trying to open a second connection this
// scripted server was never told to expect.
func fakeAFCSingleDial(conn io.ReadWriteCloser) afcDialer {
	var used atomic.Bool
	return func(context.Context) (io.ReadWriteCloser, error) {
		if !used.CompareAndSwap(false, true) {
			return nil, errors.New("fakeAFCSingleDial: only one connection is provisioned for this test")
		}
		return conn, nil
	}
}

// TestAFCVFSLiveProtocolMutationsAndInfo drives openAFCVFS's root probe, a
// successful Stat, every mutating operation (MkDir/Remove/Rename/
// SetAttributes) and RefreshPanelInfo across one real, scripted AFC
// connection. These are exactly the success paths that error-injection tests
// elsewhere in this package cannot reach, because they never get a
// well-formed response back from the wire.
func TestAFCVFSLiveProtocolMutationsAndInfo(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	steps := []fakeAFCStep{
		{wantOp: fakeOpGetFileInfo, respOp: fakeOpData, payload: fakeAFCNULItems("st_ifmt", "S_IFDIR")},
		{wantOp: fakeOpGetFileInfo, respOp: fakeOpData, payload: fakeAFCNULItems(
			"st_ifmt", "S_IFREG", "st_size", "4096", "st_mtime", "1700000000000000000")},
		{wantOp: fakeOpMakeDir, respOp: fakeOpStatus, headerPayload: fakeAFCStatusOK()},
		{wantOp: fakeOpRemovePathAndContents, respOp: fakeOpStatus, headerPayload: fakeAFCStatusOK()},
		{wantOp: fakeOpRenamePath, respOp: fakeOpStatus, headerPayload: fakeAFCStatusOK()},
		{wantOp: fakeOpSetFileModTime, respOp: fakeOpStatus, headerPayload: fakeAFCStatusOK()},
		{wantOp: fakeOpGetDeviceInfo, respOp: fakeOpData, payload: fakeAFCNULItems(
			"Model", "iPhone", "FSTotalBytes", "1000000", "FSFreeBytes", "500000", "FSBlockSize", "4096")},
	}
	serverDone := runFakeAFCServer(serverConn, steps)

	registry := newAFCRegistry()
	ctx := context.Background()
	v, err := openAFCVFS(ctx, nil, DeviceInfo{Name: "Test Phone"}, registry, "device", "Phone", false, fakeAFCSingleDial(clientConn))
	if err != nil {
		t.Fatalf("openAFCVFS: %v", err)
	}
	t.Cleanup(func() {
		if err := v.Close(); err != nil {
			t.Errorf("close AFCVFS: %v", err)
		}
		if err := registry.Close(); err != nil {
			t.Errorf("close registry: %v", err)
		}
	})

	item, err := v.Stat(ctx, "/Documents")
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if item.IsDir || item.Size != 4096 || item.Name != "Documents" {
		t.Errorf("unexpected Stat result: %#v", item)
	}
	if item.MTime.IsZero() {
		t.Error("Stat result did not carry a modification time")
	}

	if err := v.MkDir(ctx, "/NewDir"); err != nil {
		t.Fatalf("MkDir: %v", err)
	}
	if err := v.Remove(ctx, "/NewDir"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := v.Rename(ctx, "/a", "/b"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if err := v.SetAttributes(ctx, "/a", vfs.VFSItem{MTime: time.Unix(1700000000, 0)}); err != nil {
		t.Fatalf("SetAttributes: %v", err)
	}

	snapshot, err := v.RefreshPanelInfo(ctx, vfs.PanelInfoRequest{})
	if err != nil {
		t.Fatalf("RefreshPanelInfo: %v", err)
	}
	if len(snapshot.Sections) == 0 {
		t.Fatal("RefreshPanelInfo returned no sections")
	}
	var foundStorage bool
	for _, field := range snapshot.Sections[0].Fields {
		if field.ID == "storage" {
			foundStorage = true
			if field.TotalBytes != 1000000 || field.AvailableBytes != 500000 {
				t.Errorf("storage field = %#v", field)
			}
		}
	}
	if !foundStorage {
		t.Error("RefreshPanelInfo did not report a storage field")
	}

	if err := <-serverDone; err != nil {
		t.Fatalf("fake AFC server: %v", err)
	}
}

// TestAFCVFSLiveProtocolReadDir drives a successful ReadDir - list the
// directory, then stat every entry - across the same single scripted
// connection used for the root probe, covering AFCVFS.ReadDir's and
// statBatch's success path that afc_vfs_test.go's pool-focused tests never
// reach.
func TestAFCVFSLiveProtocolReadDir(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	steps := []fakeAFCStep{
		{wantOp: fakeOpGetFileInfo, respOp: fakeOpData, payload: fakeAFCNULItems("st_ifmt", "S_IFDIR")},
		{wantOp: fakeOpReadDir, respOp: fakeOpData, payload: fakeAFCNULItems("fileA")},
		{wantOp: fakeOpGetFileInfo, respOp: fakeOpData, payload: fakeAFCNULItems("st_ifmt", "S_IFREG", "st_size", "42")},
	}
	serverDone := runFakeAFCServer(serverConn, steps)

	registry := newAFCRegistry()
	ctx := context.Background()
	v, err := openAFCVFS(ctx, nil, DeviceInfo{}, registry, "device", "Phone", false, fakeAFCSingleDial(clientConn))
	if err != nil {
		t.Fatalf("openAFCVFS: %v", err)
	}
	t.Cleanup(func() {
		if err := v.Close(); err != nil {
			t.Errorf("close AFCVFS: %v", err)
		}
		if err := registry.Close(); err != nil {
			t.Errorf("close registry: %v", err)
		}
	})

	var chunks [][]vfs.VFSItem
	if err := v.ReadDir(ctx, "/", func(items []vfs.VFSItem) {
		chunks = append(chunks, items)
	}); err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	var all []vfs.VFSItem
	for _, chunk := range chunks {
		all = append(all, chunk...)
	}
	if len(all) != 1 || all[0].Name != "fileA" || all[0].IsDir || all[0].Size != 42 {
		t.Fatalf("unexpected ReadDir result: %#v", all)
	}

	if err := <-serverDone; err != nil {
		t.Fatalf("fake AFC server: %v", err)
	}
}
