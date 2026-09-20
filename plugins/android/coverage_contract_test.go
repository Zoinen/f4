package androidfs

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestSyncProtocolErrorAndRemoteErrnoContracts(t *testing.T) {
	entry := SyncEntry{Name: "gone", Errno: linuxENOENT}
	if !errors.Is(entry.Err(), os.ErrNotExist) || !strings.Contains(entry.Err().Error(), `"gone"`) {
		t.Fatalf("entry error = %v", entry.Err())
	}
	if (SyncEntry{}).Err() != nil {
		t.Fatal("zero errno must not produce an error")
	}

	for _, tc := range []struct {
		errno uint32
		want  error
	}{
		{0, nil}, {linuxENOENT, os.ErrNotExist}, {linuxENOTDIR, os.ErrNotExist},
		{linuxEPERM, os.ErrPermission}, {linuxEACCES, os.ErrPermission}, {linuxEROFS, os.ErrPermission},
		{linuxEEXIST, os.ErrExist}, {999, LinuxErrno(999)},
	} {
		err := (&SyncRemoteError{Operation: "open", Path: "/x", Errno: tc.errno}).Unwrap()
		if !errors.Is(err, tc.want) {
			t.Errorf("errno %d unwrap = %v, want %v", tc.errno, err, tc.want)
		}
	}
	for _, tc := range []struct {
		err  SyncRemoteError
		want string
	}{
		{SyncRemoteError{}, "adb sync: remote operation failed"},
		{SyncRemoteError{Operation: "stat", Path: "/a", Message: "bad"}, `adb sync stat "/a": bad`},
		{SyncRemoteError{Operation: "stat", Errno: 7}, "adb sync stat: remote errno 7"},
	} {
		if got := tc.err.Error(); got != tc.want {
			t.Errorf("SyncRemoteError = %q, want %q", got, tc.want)
		}
	}
	if got := (&SyncProtocolError{Detail: "bad"}).Error(); got != "adb sync protocol error: bad" {
		t.Errorf("protocol error = %q", got)
	}
	if got := (&SyncProtocolError{Operation: "list", Detail: "bad"}).Error(); got != "adb sync list protocol error: bad" {
		t.Errorf("operation protocol error = %q", got)
	}
	if got := LinuxErrno(17).Error(); got != "Linux errno 17" {
		t.Errorf("LinuxErrno = %q", got)
	}
}

func TestSyncWireValidationAndTimeHelpers(t *testing.T) {
	for _, path := range []string{"", "a\x00b", strings.Repeat("x", SyncMaxPath+1)} {
		if err := validateSyncPath(path); err == nil {
			t.Errorf("validateSyncPath(%q) succeeded", path)
		}
	}
	if err := validateSyncPath(strings.Repeat("x", SyncMaxPath)); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		when time.Time
		want uint32
	}{
		{time.Time{}, 0}, {time.Unix(42, 0), 42}, {time.Unix(math.MaxUint32, 0), math.MaxUint32},
	} {
		got, err := syncTimestamp(tc.when)
		if err != nil || got != tc.want {
			t.Errorf("syncTimestamp(%v) = %d, %v; want %d", tc.when, got, err, tc.want)
		}
	}
	for _, when := range []time.Time{time.Unix(-1, 0), time.Unix(math.MaxUint32, 0).Add(time.Second)} {
		if _, err := syncTimestamp(when); err == nil {
			t.Errorf("syncTimestamp(%v) succeeded", when)
		}
	}
	for _, tc := range []struct {
		seconds uint64
		want    int64
	}{{0, 0}, {42, 42}, {math.MaxInt64, math.MaxInt64}} {
		if got, err := syncUnixTime(tc.seconds); err != nil || got.Unix() != tc.want {
			t.Errorf("syncUnixTime(%d) = %v, %v", tc.seconds, got, err)
		}
	}
	if _, err := syncUnixTime(math.MaxInt64 + 1); err == nil {
		t.Fatal("out-of-range Unix time succeeded")
	}

	var request bytes.Buffer
	if err := writeSyncRequest(&request, "LIST", "/sdcard"); err != nil {
		t.Fatal(err)
	}
	if got := request.String()[8:]; got != "/sdcard" {
		t.Errorf("request path = %q", got)
	}
	for _, id := range []string{"", "BAD", "TOOLONG"} {
		if err := writeSyncRequest(io.Discard, id, "/x"); err == nil {
			t.Errorf("writeSyncRequest accepted id %q", id)
		}
	}
	if err := writeSyncRequest(io.Discard, "LIST", strings.Repeat("x", SyncMaxPath+1)); err == nil {
		t.Fatal("an overlong request path was accepted")
	}
	if got, err := readSyncID(strings.NewReader("STATrest")); err != nil || got != "STAT" {
		t.Fatalf("readSyncID = %q, %v", got, err)
	}
	if _, err := readSyncID(strings.NewReader("x")); err == nil {
		t.Fatal("short message id was accepted")
	}
}

func TestSyncMetadataReadersAndFailures(t *testing.T) {
	var v1 bytes.Buffer
	for _, value := range []uint32{0100644, 12, 99} {
		var b [4]byte
		binaryLittleEndianPut(b[:], value)
		v1.Write(b[:])
	}
	entry, err := readSyncStatV1AfterID(bytes.NewReader(v1.Bytes()))
	if err != nil || entry.Mode != 0100644 || entry.Size != 12 || entry.ModTime.Unix() != 99 {
		t.Fatalf("v1 stat = %#v, %v", entry, err)
	}
	if _, err := readSyncStatV1AfterID(bytes.NewReader([]byte{1})); err == nil {
		t.Fatal("short v1 stat was accepted")
	}

	for _, tc := range []struct {
		data []byte
		size uint32
		want string
	}{
		{[]byte("name"), 4, "name"}, {nil, 0, ""}, {[]byte{0, 'x'}, 2, ""}, {[]byte("a/b"), 3, ""},
	} {
		name, err := readSyncName(bytes.NewReader(tc.data), tc.size)
		if tc.want != "" {
			if err != nil || name != tc.want {
				t.Errorf("readSyncName = %q, %v", name, err)
			}
		} else if err == nil {
			t.Errorf("readSyncName(%q) unexpectedly succeeded", tc.data)
		}
	}
	for _, size := range []uint32{0, syncMaxName + 1} {
		if _, err := readSyncName(bytes.NewReader(nil), size); err == nil {
			t.Errorf("readSyncName size %d succeeded", size)
		}
	}
	if _, err := readSyncName(bytes.NewReader([]byte("abc")), 4); err == nil {
		t.Fatal("short name payload was accepted")
	}

	if err := readSyncFailAfterID(bytes.NewReader(append([]byte{3, 0, 0, 0}, []byte("bad")...)), "read", "/x"); err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("FAIL response = %v", err)
	}
	if err := readSyncFailBody(bytes.NewReader(nil), "read", "/x", syncMaxMessage+1); err == nil {
		t.Fatal("oversized FAIL was accepted")
	}
	if err := readSyncFailBody(bytes.NewReader([]byte("x")), "read", "/x", 2); err == nil {
		t.Fatal("short FAIL body was accepted")
	}
	if got := unexpectedSyncID("stat", "NOPE", "STAT").Error(); !strings.Contains(got, "NOPE") || !strings.Contains(got, "STAT") {
		t.Errorf("unexpected id error = %q", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	want := errors.New("wire")
	if got := syncContextError(ctx, want); !errors.Is(got, context.Canceled) {
		t.Fatalf("context error = %v", got)
	}
	if got := syncContextError(context.Background(), want); got != want {
		t.Fatalf("wire error = %v", got)
	}
}

// Keep the protocol test independent of encoding/binary details that are not
// part of the assertion above while still constructing the legacy body.
func binaryLittleEndianPut(dst []byte, value uint32) {
	binary.LittleEndian.PutUint32(dst, value)
}

func TestSyncVFSCommandAndEntryEdges(t *testing.T) {
	if syncEntrySize(math.MaxUint64) != math.MaxInt64 || syncEntrySize(12) != 12 {
		t.Fatal("syncEntrySize did not saturate at MaxInt64")
	}
	entry := syncEntryItem(SyncEntry{Name: ".run", Mode: remoteModeLink | 0755, Size: math.MaxUint64, UID: 7, GID: 8, metadataV2: true, Device: 2, Inode: 3})
	if entry.Name != ".run" || !entry.IsSymlink || !entry.IsExecutable || !entry.IsHidden || entry.Size != math.MaxInt64 || entry.Uid != 7 || entry.Gid != 8 || entry.Device != 2 || entry.Inode != 3 {
		t.Fatalf("sync entry item = %#v", entry)
	}

	fs := newSyncVFS(nil, "serial", "Phone", &fakeSyncFS{}, nil)
	root := (vfs.DevicePath{Scheme: "android", Device: "Phone"}).Root()
	if !fs.IsAtRoot() || fs.GetPath() != root || fs.GetTitle() != "Phone" || fs.SessionKey() != "android:serial" || !fs.IsAbs("/x") || fs.IsAbs("x") || fs.Base("/x/y") != "y" || fs.Dir("/x/y") != "/x" {
		t.Fatal("SyncVFS path contract is inconsistent")
	}
	if got := fs.PanelTitle("/sdcard/Download"); got != (vfs.DevicePath{Scheme: "android", Device: "Phone"}).Public("/sdcard/Download") {
		t.Fatalf("PanelTitle = %q", got)
	}
	if got := fs.CommandRunnerInfo(); got.Dialect != vfs.CommandDialectPOSIX || got.MaxParallel != 4 {
		t.Fatalf("CommandRunnerInfo = %#v", got)
	}
	if ch, err := fs.Search(context.Background(), "/", "x"); err != nil || ch != nil {
		t.Fatalf("Search = %v, %v", ch, err)
	}
	if _, err := fs.RunCommand(context.Background(), "/", "", nil); err == nil {
		t.Fatal("empty command was accepted")
	}
	if _, err := fs.RunCommand(context.Background(), "/", "echo hi", nil); err == nil {
		t.Fatal("missing shell runner was accepted")
	}
	if ok, err := fs.shellTestDir(context.Background(), "/x"); err != nil || ok {
		t.Fatalf("shellTestDir without runner = %v, %v", ok, err)
	}

	var lines []string
	emitShellLines([]byte("one\r\ntwo\nthree"), func(line string) { lines = append(lines, line) })
	if !reflect.DeepEqual(lines, []string{"one", "two", "three"}) {
		t.Fatalf("shell lines = %#v", lines)
	}
	emitShellLines(nil, nil)
	var chunks []string
	w := newAndroidCommandLineWriter(func(line string) { chunks = append(chunks, line) })
	if n, err := w.Write([]byte("one\r\ntwo")); err != nil || n != 8 {
		t.Fatalf("line writer = %d, %v", n, err)
	}
	w.Flush()
	if !reflect.DeepEqual(chunks, []string{"one", "two"}) {
		t.Fatalf("line writer chunks = %#v", chunks)
	}
	nilWriter := newAndroidCommandLineWriter(nil)
	if n, err := nilWriter.Write([]byte("ignored")); err != nil || n != 7 {
		t.Fatalf("nil line writer = %d, %v", n, err)
	}
	if androidCommandOutputChunkEnd([]byte("short"), 10) != 5 {
		t.Fatal("short chunk end changed")
	}
	longUTF8 := append(bytes.Repeat([]byte{'x'}, androidCommandOutputChunkBytes-1), []byte("€")...)
	if got := androidCommandOutputChunkEnd(longUTF8, androidCommandOutputChunkBytes); got != androidCommandOutputChunkBytes-1 {
		t.Fatalf("UTF-8 chunk end = %d", got)
	}
}
