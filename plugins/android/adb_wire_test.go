package androidfs

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

// This file covers the ADB smart-socket and shell-v2 wire format directly:
// request/reply framing, device-list parsing, packet framing, and the
// ShellStream state machine built on top of it. All of it is pure,
// deterministic protocol logic that needs only an io.Reader/io.Writer or a
// net.Pipe, never a real adb server or device - the same reasoning already
// applied to plugins/ios's AFC wire tests.

func TestServiceErrorMessage(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  *ServiceError
		want string
	}{
		{name: "WithMessage", err: &ServiceError{Service: "host:transport:S", Message: "device offline"}, want: `adb service "host:transport:S" failed: device offline`},
		{name: "WithoutMessage", err: &ServiceError{Service: "host:transport:S"}, want: `adb service "host:transport:S" failed`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Fatalf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReadLengthPrefixedErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		wantErr string
	}{
		{name: "ShortHeader", payload: "12", wantErr: "read length"},
		{name: "InvalidHexLength", payload: "ZZZZtail", wantErr: "invalid length"},
		{name: "TruncatedPayload", payload: "0005ab", wantErr: "5-byte payload"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readLengthPrefixed(strings.NewReader(tc.payload), "sync:")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("readLengthPrefixed error = %v, want it to contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestReadLengthPrefixedSuccess(t *testing.T) {
	got, err := readLengthPrefixed(strings.NewReader("0005helloXXX"), "sync:")
	if err != nil || string(got) != "hello" {
		t.Fatalf("readLengthPrefixed = %q, %v; want \"hello\", nil", got, err)
	}
}

func TestRequestServicePropagatesRequestWriteFailure(t *testing.T) {
	// writeADBRequest rejects service names above the 4-hex-digit length
	// header before touching rw at all; requestService must surface that
	// error rather than continue on to read a reply that was never sent.
	err := requestService(&bytes.Buffer{}, strings.Repeat("x", maxADBServiceLength+1))
	if err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("requestService error = %v, want it to mention the length limit", err)
	}
}

func TestRequestServiceHandlesUnexpectedStatus(t *testing.T) {
	server, dialer := testServer(t, func(conn net.Conn) {
		expectTestRequest(t, conn, "host:transport:WEIRD")
		writeTestStatus(t, conn, "SPAM")
	})
	_, err := server.OpenService(context.Background(), "WEIRD", "sync:")
	if err == nil || !strings.Contains(err.Error(), "unexpected status") {
		t.Fatalf("OpenService error = %v, want it to mention the unexpected status", err)
	}
	dialer.assertDone()
}

func TestRequestServiceWrapsTruncatedFailureMessage(t *testing.T) {
	server, dialer := testServer(t, func(conn net.Conn) {
		expectTestRequest(t, conn, "host:transport:BROKEN")
		writeTestStatus(t, conn, "FAIL")
		// A well-formed FAIL reply carries a 4-hex-digit length header; send
		// only half of it and let the handler return, which closes the
		// connection out from under requestService's second read.
		_, _ = io.WriteString(conn, "12")
	})
	_, err := server.OpenService(context.Background(), "BROKEN", "sync:")
	if err == nil || !strings.Contains(err.Error(), "read failure") {
		t.Fatalf("OpenService error = %v, want it to mention the failed failure-message read", err)
	}
	dialer.assertDone()
}

type writeFailAtCall struct {
	failAt int
	calls  int
	err    error
}

func (w *writeFailAtCall) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == w.failAt {
		return 0, w.err
	}
	return len(p), nil
}

func TestWriteADBRequestPropagatesWriteFailures(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name    string
		failAt  int
		wantErr string
	}{
		{name: "HeaderWriteFails", failAt: 1, wantErr: "write length"},
		{name: "PayloadWriteFails", failAt: 2, wantErr: "write request"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &writeFailAtCall{failAt: tc.failAt, err: boom}
			err := writeADBRequest(w, "host:features")
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !errors.Is(err, boom) {
				t.Fatalf("writeADBRequest error = %v, want it to wrap %v and mention %q", err, boom, tc.wantErr)
			}
		})
	}
}

func TestWriteShellPacketPropagatesWriteFailures(t *testing.T) {
	boom := errors.New("boom")
	for _, tc := range []struct {
		name    string
		failAt  int
		wantErr string
	}{
		{name: "HeaderWriteFails", failAt: 1, wantErr: "write packet header"},
		{name: "BodyWriteFails", failAt: 2, wantErr: "write packet body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := &writeFailAtCall{failAt: tc.failAt, err: boom}
			err := writeShellPacket(w, shellIDStdin, []byte("payload"))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !errors.Is(err, boom) {
				t.Fatalf("writeShellPacket error = %v, want it to wrap %v and mention %q", err, boom, tc.wantErr)
			}
		})
	}
}

func TestReadShellPacketRejectsOversizedAndTruncatedPayload(t *testing.T) {
	t.Run("OversizedPayload", func(t *testing.T) {
		var header [5]byte
		header[0] = shellIDStdout
		binary.LittleEndian.PutUint32(header[1:], uint32(maxShellPacket+1))
		_, _, err := readShellPacket(bytes.NewReader(header[:]))
		if err == nil || !strings.Contains(err.Error(), "too large") {
			t.Fatalf("readShellPacket error = %v, want it to mention the size limit", err)
		}
	})

	t.Run("TruncatedPayload", func(t *testing.T) {
		var header [5]byte
		header[0] = shellIDStdout
		binary.LittleEndian.PutUint32(header[1:], 10)
		r := io.MultiReader(bytes.NewReader(header[:]), bytes.NewReader([]byte("abc")))
		_, _, err := readShellPacket(r)
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("readShellPacket error = %v, want io.ErrUnexpectedEOF", err)
		}
	})
}

func TestParseShellExitAcceptsOneAndFourByteLengths(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload []byte
		want    int
		wantErr bool
	}{
		{name: "OneByte", payload: []byte{5}, want: 5},
		{name: "FourBytes", payload: func() []byte {
			b := make([]byte, 4)
			binary.LittleEndian.PutUint32(b, 700)
			return b
		}(), want: 700},
		{name: "InvalidLength", payload: []byte{1, 2}, wantErr: true},
		{name: "EmptyPayload", payload: nil, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseShellExit(tc.payload)
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "invalid exit packet length") {
					t.Fatalf("parseShellExit error = %v, want an invalid-length error", err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("parseShellExit = %d, %v; want %d, nil", got, err, tc.want)
			}
		})
	}
}

type invalidCountWriter struct{}

func (invalidCountWriter) Write(p []byte) (int, error) { return len(p) + 1, nil }

type partialThenErrWriter struct{ err error }

func (w partialThenErrWriter) Write(p []byte) (int, error) {
	if len(p) < 3 {
		return len(p), nil
	}
	return 3, w.err
}

type zeroProgressWriter struct{}

func (zeroProgressWriter) Write([]byte) (int, error) { return 0, nil }

func TestWriteFullHandlesMisbehavingWriters(t *testing.T) {
	boom := errors.New("boom")

	t.Run("InvalidWriteCount", func(t *testing.T) {
		err := writeFull(invalidCountWriter{}, []byte("hello"))
		if err == nil || !strings.Contains(err.Error(), "invalid write count") {
			t.Fatalf("writeFull error = %v, want an invalid write count error", err)
		}
	})

	t.Run("PartialWriteThenError", func(t *testing.T) {
		err := writeFull(partialThenErrWriter{err: boom}, []byte("hello"))
		if !errors.Is(err, boom) {
			t.Fatalf("writeFull error = %v, want %v", err, boom)
		}
	})

	t.Run("NoProgress", func(t *testing.T) {
		err := writeFull(zeroProgressWriter{}, []byte("hello"))
		if !errors.Is(err, io.ErrNoProgress) {
			t.Fatalf("writeFull error = %v, want io.ErrNoProgress", err)
		}
	})
}

func TestParseDevicesRejectsMalformedRecordsAndSkipsUnkeyedProperties(t *testing.T) {
	t.Run("TooFewFields", func(t *testing.T) {
		_, err := parseDevices([]byte("onlyserial\n"))
		if err == nil || !strings.Contains(err.Error(), "malformed device record") {
			t.Fatalf("parseDevices error = %v, want a malformed-record error", err)
		}
	})

	t.Run("MissingState", func(t *testing.T) {
		// The first field after the serial contains ':' itself, so no field
		// is left to serve as the device state.
		_, err := parseDevices([]byte("serial123 key:value\n"))
		if err == nil || !strings.Contains(err.Error(), "missing device state") {
			t.Fatalf("parseDevices error = %v, want a missing-state error", err)
		}
	})

	t.Run("SkipsUnkeyedPropertyTokens", func(t *testing.T) {
		devices, err := parseDevices([]byte(
			"SERIAL123 device product:Pixel badtoken model:Pixel4 transport_id:7\n",
		))
		if err != nil {
			t.Fatalf("parseDevices: %v", err)
		}
		want := []Device{{
			Serial: "SERIAL123", State: "device",
			Product: "Pixel", Model: "Pixel4", TransportID: "7",
		}}
		if len(devices) != 1 || devices[0] != want[0] {
			t.Fatalf("parseDevices = %#v, want %#v", devices, want)
		}
	})
}

func newAndroidShellStreamHarness(t *testing.T) (*ShellStream, net.Conn) {
	t.Helper()
	client, server := net.Pipe()
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	return newShellStream(client), server
}

func TestShellStreamReadNoopsOnEmptyBuffer(t *testing.T) {
	stream, _ := newAndroidShellStreamHarness(t)
	n, err := stream.Read(nil)
	if n != 0 || err != nil {
		t.Fatalf("Read(nil) = %d, %v; want 0, nil", n, err)
	}
}

func TestShellStreamReadServesPendingAcrossCallsThenCachesTerminalError(t *testing.T) {
	stream, server := newAndroidShellStreamHarness(t)
	go func() {
		_ = writeShellPacket(server, shellIDStdout, []byte("hello-world"))
		_ = writeShellPacket(server, shellIDExit, []byte{0})
	}()

	first := make([]byte, 5)
	n, err := stream.Read(first)
	if err != nil || string(first[:n]) != "hello" {
		t.Fatalf("first Read = %q, %v; want \"hello\", nil", first[:n], err)
	}

	// The rest of the 11-byte packet must come out of the buffered
	// remainder, without another conn read.
	second := make([]byte, 20)
	n, err = stream.Read(second)
	if err != nil || string(second[:n]) != "-world" {
		t.Fatalf("second Read = %q, %v; want \"-world\", nil", second[:n], err)
	}

	third := make([]byte, 20)
	n, err = stream.Read(third)
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("third Read = %d, %v; want 0, io.EOF", n, err)
	}

	// The exit packet already reported io.EOF; a further Read must return
	// the cached error rather than block on the now-idle connection.
	n, err = stream.Read(third)
	if n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("cached Read = %d, %v; want 0, io.EOF", n, err)
	}
}

func TestShellStreamReadWrapsAbruptCloseAsTruncatedStream(t *testing.T) {
	stream, server := newAndroidShellStreamHarness(t)
	_ = server.Close()

	_, err := stream.Read(make([]byte, 4))
	if err == nil || !strings.Contains(err.Error(), "truncated stream") {
		t.Fatalf("Read error = %v, want a truncated-stream error", err)
	}

	// The failure is cached, so a second call must not try the connection
	// again.
	if _, err := stream.Read(make([]byte, 4)); err == nil {
		t.Fatal("cached Read unexpectedly succeeded")
	}
}

func TestShellStreamReadSkipsNoiseAndAppliesRemoteCloseStdin(t *testing.T) {
	stream, server := newAndroidShellStreamHarness(t)
	go func() {
		_ = writeShellPacket(server, shellIDWindowSize, nil)
		_ = writeShellPacket(server, shellIDStdout, nil) // empty stdout chunk, skipped
		_ = writeShellPacket(server, shellIDCloseStdin, nil)
		_ = writeShellPacket(server, shellIDStdout, []byte("ok"))
	}()

	buf := make([]byte, 16)
	n, err := stream.Read(buf)
	if err != nil || string(buf[:n]) != "ok" {
		t.Fatalf("Read = %q, %v; want \"ok\", nil", buf[:n], err)
	}

	// The peer's close-stdin packet must be reflected locally: further
	// writes are rejected instead of being sent into the void.
	if _, err := stream.Write([]byte("x")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Write after remote close-stdin = %v, want io.ErrClosedPipe", err)
	}
}

func TestShellStreamReadRejectsUnexpectedPacketID(t *testing.T) {
	stream, server := newAndroidShellStreamHarness(t)
	go func() {
		_ = writeShellPacket(server, 77, []byte("x"))
	}()

	_, err := stream.Read(make([]byte, 4))
	if err == nil || !strings.Contains(err.Error(), "unexpected packet id") {
		t.Fatalf("Read error = %v, want an unexpected-packet-id error", err)
	}
}

func TestShellStreamReadReturnsExitParseError(t *testing.T) {
	stream, server := newAndroidShellStreamHarness(t)
	go func() {
		_ = writeShellPacket(server, shellIDExit, []byte{1, 2}) // neither 1 nor 4 bytes
	}()

	_, err := stream.Read(make([]byte, 4))
	if err == nil || !strings.Contains(err.Error(), "invalid exit packet length") {
		t.Fatalf("Read error = %v, want an invalid-exit-length error", err)
	}
	if _, err := stream.Read(make([]byte, 4)); err == nil {
		t.Fatal("cached Read after exit-parse error unexpectedly succeeded")
	}
}

func TestShellStreamWriteRejectsWhenClosedOrStdinEOF(t *testing.T) {
	t.Run("Closed", func(t *testing.T) {
		stream, _ := newAndroidShellStreamHarness(t)
		if err := stream.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		if _, err := stream.Write([]byte("x")); !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Write after Close = %v, want net.ErrClosed", err)
		}
	})

	t.Run("StdinEOF", func(t *testing.T) {
		stream, server := newAndroidShellStreamHarness(t)
		drained := make(chan struct{})
		go func() {
			_, _, _ = readShellPacket(server)
			close(drained)
		}()
		if err := stream.CloseWrite(); err != nil {
			t.Fatalf("CloseWrite: %v", err)
		}
		select {
		case <-drained:
		case <-time.After(time.Second):
			t.Fatal("peer never received the close-stdin packet")
		}
		if _, err := stream.Write([]byte("x")); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("Write after CloseWrite = %v, want io.ErrClosedPipe", err)
		}
	})
}

func TestShellStreamCloseWriteIsIdempotentAndNoopAfterClose(t *testing.T) {
	t.Run("AlreadyStdinEOF", func(t *testing.T) {
		stream, server := newAndroidShellStreamHarness(t)
		drained := make(chan struct{})
		go func() {
			_, _, _ = readShellPacket(server)
			close(drained)
		}()
		if err := stream.CloseWrite(); err != nil {
			t.Fatalf("first CloseWrite: %v", err)
		}
		select {
		case <-drained:
		case <-time.After(time.Second):
			t.Fatal("peer never received the close-stdin packet")
		}

		// A second CloseWrite must return immediately without writing
		// another packet - if it did, this call would block forever on the
		// now-unattended pipe and the test would time out.
		done := make(chan error, 1)
		go func() { done <- stream.CloseWrite() }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("second CloseWrite: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("second CloseWrite did not return; it likely tried to write again")
		}
	})

	t.Run("AlreadyClosed", func(t *testing.T) {
		stream, _ := newAndroidShellStreamHarness(t)
		if err := stream.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		done := make(chan error, 1)
		go func() { done <- stream.CloseWrite() }()
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("CloseWrite after Close: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("CloseWrite after Close did not return; it likely tried to write on a closed conn")
		}
	})
}
