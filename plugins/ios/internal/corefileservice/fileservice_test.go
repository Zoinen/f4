package corefileservice

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeXPCConnection struct {
	controlResponses chan receiveResult
	listResponses    chan receiveResult
	closed           chan struct{}
	closeOnce        sync.Once
	listReaders      atomic.Int32
}

func TestReceiveFileDataToWriterValidatesAndStreams(t *testing.T) {
	const payloadText = "core-device-data"
	payload := []byte(payloadText)
	header := make([]byte, 40)
	copy(header[:8], "rwb!FILE")
	binary.BigEndian.PutUint32(header[36:40], uint32(len(payloadText)))
	var output bytes.Buffer
	if err := receiveFileDataToWriter(bytes.NewReader(append(header, payload...)), &output); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(output.Bytes(), payload) {
		t.Fatalf("payload = %q, want %q", output.Bytes(), payload)
	}
	header[0] = 'x'
	if err := receiveFileDataToWriter(bytes.NewReader(header), io.Discard); err == nil {
		t.Fatal("invalid data-service magic was accepted")
	}
}

func newFakeXPCConnection() *fakeXPCConnection {
	return &fakeXPCConnection{
		controlResponses: make(chan receiveResult, 1),
		listResponses:    make(chan receiveResult, 1),
		closed:           make(chan struct{}),
	}
}

func (*fakeXPCConnection) Send(map[string]interface{}, ...uint32) error { return nil }
func (f *fakeXPCConnection) ReceiveOnClientServerStream() (map[string]interface{}, error) {
	f.listReaders.Add(1)
	select {
	case result := <-f.listResponses:
		return result.response, result.err
	case <-f.closed:
		return nil, errors.New("connection closed")
	}
}
func (f *fakeXPCConnection) ReceiveOnServerClientStream() (map[string]interface{}, error) {
	select {
	case result := <-f.controlResponses:
		return result.response, result.err
	case <-f.closed:
		return nil, errors.New("connection closed")
	}
}
func (f *fakeXPCConnection) Close() error {
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

func testConnection(t *testing.T, fake *fakeXPCConnection, timeout time.Duration) *Connection {
	t.Helper()
	t.Cleanup(func() { _ = fake.Close() })
	return &Connection{conn: fake, sessionID: "test-session", receiveTimeout: timeout}
}

func listWithDeadline(t *testing.T, connection *Connection, path string) ([]string, error) {
	t.Helper()
	type result struct {
		files []string
		err   error
	}
	done := make(chan result, 1)
	go func() {
		files, err := connection.ListDirectory(path)
		done <- result{files: files, err: err}
	}()
	select {
	case result := <-done:
		return result.files, result.err
	case <-time.After(5 * time.Second):
		t.Fatal("ListDirectory did not return within 5 seconds")
		return nil, nil
	}
}

func TestListDirectorySurfacesControlStreamError(t *testing.T) {
	fake := newFakeXPCConnection()
	fake.controlResponses <- receiveResult{response: map[string]interface{}{
		"EncodedError": map[string]interface{}{
			"Code": uint64(11007), "LocalizedDescription": "File paths cannot contain '..'.",
		},
	}}
	connection := testConnection(t, fake, 5*time.Second)

	files, err := listWithDeadline(t, connection, "shared-data")
	if err == nil || files != nil {
		t.Fatalf("ListDirectory = %#v, %v; want nil, error", files, err)
	}
	var deviceErr *DeviceError
	if !errors.As(err, &deviceErr) || deviceErr.Description != "File paths cannot contain '..'." {
		t.Fatalf("error = %v, DeviceError = %#v", err, deviceErr)
	}
}

func TestListDirectoryTimesOutWhenNeitherStreamResponds(t *testing.T) {
	fake := newFakeXPCConnection()
	connection := testConnection(t, fake, 50*time.Millisecond)
	start := time.Now()

	files, err := listWithDeadline(t, connection, ".")
	if files != nil || !errors.Is(err, ErrTimeout) {
		t.Fatalf("ListDirectory = %#v, %v; want ErrTimeout", files, err)
	}
	if elapsed := time.Since(start); elapsed >= 5*time.Second {
		t.Fatalf("timeout took %v", elapsed)
	}
}

func TestListDirectoryReturnsClientServerStreamList(t *testing.T) {
	fake := newFakeXPCConnection()
	fake.listResponses <- receiveResult{response: map[string]interface{}{
		"FileList": []interface{}{"manifest.json", "sub"},
	}}
	connection := testConnection(t, fake, time.Second)

	files, err := listWithDeadline(t, connection, ".")
	if err != nil || !reflect.DeepEqual(files, []string{"manifest.json", "sub"}) {
		t.Fatalf("ListDirectory = %#v, %v", files, err)
	}
}

func TestPullFileConsumesPendingControlReadAfterList(t *testing.T) {
	fake := newFakeXPCConnection()
	fake.listResponses <- receiveResult{response: map[string]interface{}{"FileList": []interface{}{}}}
	connection := testConnection(t, fake, time.Second)
	if _, err := listWithDeadline(t, connection, "."); err != nil {
		t.Fatalf("ListDirectory: %v", err)
	}

	fake.controlResponses <- receiveResult{response: map[string]interface{}{
		"EncodedError": map[string]interface{}{"LocalizedDescription": "No such file or directory"},
	}}
	err := connection.PullFile("missing.txt", io.Discard)
	var deviceErr *DeviceError
	if !errors.As(err, &deviceErr) || deviceErr.Description != "No such file or directory" {
		t.Fatalf("PullFile error = %v, DeviceError = %#v", err, deviceErr)
	}
}

func TestListDirectoryReusesPendingListReaderAfterControlError(t *testing.T) {
	fake := newFakeXPCConnection()
	connection := testConnection(t, fake, time.Second)
	fake.controlResponses <- receiveResult{response: map[string]interface{}{
		"EncodedError": map[string]interface{}{"LocalizedDescription": "first request rejected"},
	}}

	if files, err := listWithDeadline(t, connection, "rejected"); files != nil || err == nil {
		t.Fatalf("first ListDirectory = %#v, %v; want control-stream error", files, err)
	}
	waitForListReaders(t, fake, 1)

	// The first request returned through the control stream, so its list-stream
	// reader is still blocked. The second request must reuse exactly that reader;
	// otherwise one of two readers can steal this response nondeterministically.
	fake.listResponses <- receiveResult{response: map[string]interface{}{
		"FileList": []interface{}{"next.txt"},
	}}
	files, err := listWithDeadline(t, connection, "next")
	if err != nil || !reflect.DeepEqual(files, []string{"next.txt"}) {
		t.Fatalf("second ListDirectory = %#v, %v", files, err)
	}
	if readers := fake.listReaders.Load(); readers != 1 {
		t.Fatalf("list-stream readers = %d, want exactly one reused reader", readers)
	}
}

func waitForListReaders(t *testing.T, fake *fakeXPCConnection, want int32) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for fake.listReaders.Load() < want && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := fake.listReaders.Load(); got != want {
		t.Fatalf("list-stream readers = %d, want %d", got, want)
	}
}
