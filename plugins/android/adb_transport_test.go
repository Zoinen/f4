package androidfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type adbHandler func(net.Conn)

type scriptedADBDialer struct {
	t        *testing.T
	mu       sync.Mutex
	handlers []adbHandler
}

func (d *scriptedADBDialer) dial(ctx context.Context, network, address string) (net.Conn, error) {
	d.t.Helper()
	if network != "tcp" {
		d.t.Errorf("network = %q, want tcp", network)
	}
	d.mu.Lock()
	if len(d.handlers) == 0 {
		d.mu.Unlock()
		return nil, errors.New("unexpected dial")
	}
	handler := d.handlers[0]
	d.handlers = d.handlers[1:]
	d.mu.Unlock()
	client, server := net.Pipe()
	go func() {
		defer func() {
			_ = server.Close() // connection cleanup only
		}()
		handler(server)
	}()
	return client, nil
}

func (d *scriptedADBDialer) assertDone() {
	d.t.Helper()
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.handlers) != 0 {
		d.t.Errorf("%d scripted ADB connections were not used", len(d.handlers))
	}
}

func readTestRequest(t *testing.T, conn io.Reader) string {
	t.Helper()
	var header [4]byte
	if _, err := io.ReadFull(conn, header[:]); err != nil {
		t.Errorf("read request header: %v", err)
		return ""
	}
	var n int
	if _, err := fmt.Sscanf(string(header[:]), "%04X", &n); err != nil {
		t.Errorf("parse request header %q: %v", header, err)
		return ""
	}
	payload := make([]byte, n)
	if _, err := io.ReadFull(conn, payload); err != nil {
		t.Errorf("read request payload: %v", err)
		return ""
	}
	return string(payload)
}

func expectTestRequest(t *testing.T, conn io.Reader, want string) {
	t.Helper()
	if got := readTestRequest(t, conn); got != want {
		t.Errorf("ADB request = %q, want %q", got, want)
	}
}

func writeTestHostReply(t *testing.T, conn io.Writer, payload string) {
	t.Helper()
	if _, err := fmt.Fprintf(conn, "OKAY%04X%s", len(payload), payload); err != nil {
		t.Errorf("write host reply: %v", err)
	}
}

func writeTestStatus(t *testing.T, conn io.Writer, status string) {
	t.Helper()
	if _, err := io.WriteString(conn, status); err != nil {
		t.Errorf("write status: %v", err)
	}
}

func testServer(t *testing.T, handlers ...adbHandler) (*Server, *scriptedADBDialer) {
	t.Helper()
	dialer := &scriptedADBDialer{t: t, handlers: handlers}
	server := NewServer(
		WithADBAddress("adb.test:5037"),
		WithADBDialer(dialer.dial),
		WithADBLookup(func() (string, error) { return "", errors.New("should not look up adb") }),
	)
	return server, dialer
}

func TestDevicesParsesLongListing(t *testing.T) {
	server, dialer := testServer(t, func(conn net.Conn) {
		expectTestRequest(t, conn, "host:devices-l")
		writeTestHostReply(t, conn,
			"988673333359424b4d     device product:heroltexx model:SM_G930F device:herolte transport_id:14\n"+
				"R3CN\tunauthorized usb:1-2 transport_id:9\n")
	})

	devices, err := server.Devices(context.Background())
	if err != nil {
		t.Fatalf("Devices: %v", err)
	}
	want := []Device{
		{Serial: "988673333359424b4d", State: "device", Product: "heroltexx", Model: "SM_G930F", Device: "herolte", TransportID: "14"},
		{Serial: "R3CN", State: "unauthorized", TransportID: "9"},
	}
	if !reflect.DeepEqual(devices, want) {
		t.Fatalf("devices = %#v, want %#v", devices, want)
	}
	if !devices[0].Online() || devices[1].Online() {
		t.Fatalf("unexpected Online results: %v, %v", devices[0].Online(), devices[1].Online())
	}
	dialer.assertDone()
}

func TestDevicesFallsBackToShortListing(t *testing.T) {
	server, dialer := testServer(t,
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host:devices-l")
			_, _ = io.WriteString(conn, "FAIL0014unknown host service")
		},
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host:devices")
			writeTestHostReply(t, conn, "SERIAL\tdevice\n")
		},
	)

	devices, err := server.Devices(context.Background())
	if err != nil {
		t.Fatalf("Devices: %v", err)
	}
	if want := []Device{{Serial: "SERIAL", State: "device"}}; !reflect.DeepEqual(devices, want) {
		t.Fatalf("devices = %#v, want %#v", devices, want)
	}
	dialer.assertDone()
}

func TestRestartForAuthorizationUsesInstalledADB(t *testing.T) {
	var gotPath string
	server := NewServer(
		WithADBLookup(func() (string, error) { return "/sdk/platform-tools/adb", nil }),
		WithADBRestarter(func(_ context.Context, path string) error {
			gotPath = path
			return nil
		}),
	)

	if err := server.RestartForAuthorization(context.Background()); err != nil {
		t.Fatalf("RestartForAuthorization: %v", err)
	}
	if gotPath != "/sdk/platform-tools/adb" {
		t.Fatalf("restart path = %q", gotPath)
	}
}

func TestFeaturesUsesTransportSpecificQuery(t *testing.T) {
	server, dialer := testServer(t, func(conn net.Conn) {
		expectTestRequest(t, conn, "host-serial:SERIAL:features")
		writeTestHostReply(t, conn, "shell_v2,stat_v2,sendrecv_v2")
	})

	features, err := server.Features(context.Background(), "SERIAL")
	if err != nil {
		t.Fatalf("Features: %v", err)
	}
	for _, name := range []string{"shell_v2", "stat_v2", "sendrecv_v2"} {
		if !features[name] {
			t.Errorf("feature %q is absent in %#v", name, features)
		}
	}
	dialer.assertDone()
}

func TestOpenServiceSelectsTransportThenService(t *testing.T) {
	server, dialer := testServer(t, func(conn net.Conn) {
		expectTestRequest(t, conn, "host:transport:SERIAL")
		writeTestStatus(t, conn, "OKAY")
		expectTestRequest(t, conn, "sync:")
		writeTestStatus(t, conn, "OKAY")
		_, _ = io.WriteString(conn, "service bytes")
	})

	stream, err := server.OpenService(context.Background(), "SERIAL", "sync:")
	if err != nil {
		t.Fatalf("OpenService: %v", err)
	}
	defer func() {
		_ = stream.Close() // connection cleanup only
	}()
	got := make([]byte, len("service bytes"))
	if _, err := io.ReadFull(stream, got); err != nil {
		t.Fatalf("read service: %v", err)
	}
	if string(got) != "service bytes" {
		t.Fatalf("service data = %q", got)
	}
	dialer.assertDone()
}

func TestRunShellV2SeparatesChannelsAndExitCode(t *testing.T) {
	server, dialer := testServer(t,
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host-serial:SERIAL:features")
			writeTestHostReply(t, conn, "shell_v2")
		},
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host:transport:SERIAL")
			writeTestStatus(t, conn, "OKAY")
			expectTestRequest(t, conn, "shell,v2,raw:printf hello")
			writeTestStatus(t, conn, "OKAY")
			if err := writeShellPacket(conn, shellIDStdout, []byte("hello")); err != nil {
				t.Errorf("write stdout: %v", err)
			}
			if err := writeShellPacket(conn, shellIDStderr, []byte("warning")); err != nil {
				t.Errorf("write stderr: %v", err)
			}
			if err := writeShellPacket(conn, shellIDExit, []byte{7}); err != nil {
				t.Errorf("write exit: %v", err)
			}
		},
	)

	stdout, stderr, exitCode, err := server.RunShell(context.Background(), "SERIAL", "printf hello")
	if err != nil {
		t.Fatalf("RunShell: %v", err)
	}
	if string(stdout) != "hello" || string(stderr) != "warning" || exitCode != 7 {
		t.Fatalf("RunShell = (%q, %q, %d), want (hello, warning, 7)", stdout, stderr, exitCode)
	}
	dialer.assertDone()
}

func TestRunShellStreamV2PreservesPacketOrderAndStreamsLive(t *testing.T) {
	releaseExit := make(chan struct{})
	server, dialer := testServer(t,
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host-serial:SERIAL:features")
			writeTestHostReply(t, conn, "shell_v2")
		},
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host:transport:SERIAL")
			writeTestStatus(t, conn, "OKAY")
			expectTestRequest(t, conn, "shell,v2,raw:do work")
			writeTestStatus(t, conn, "OKAY")
			_ = writeShellPacket(conn, shellIDStdout, []byte("out-"))
			_ = writeShellPacket(conn, shellIDStderr, []byte("err\n"))
			<-releaseExit
			_ = writeShellPacket(conn, shellIDStdout, []byte("tail"))
			_ = writeShellPacket(conn, shellIDExit, []byte{7})
		},
	)

	chunks := make(chan string, 3)
	done := make(chan struct {
		code int
		err  error
	}, 1)
	go func() {
		code, err := server.RunShellStream(context.Background(), "SERIAL", "do work", func(chunk []byte) {
			chunks <- string(chunk)
		})
		done <- struct {
			code int
			err  error
		}{code, err}
	}()

	for i, want := range []string{"out-", "err\n"} {
		select {
		case got := <-chunks:
			if got != want {
				t.Fatalf("chunk %d = %q, want %q", i, got, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("chunk %d was not streamed before exit", i)
		}
	}
	select {
	case result := <-done:
		t.Fatalf("RunShellStream returned before exit packet: %+v", result)
	default:
	}
	close(releaseExit)
	if got := <-chunks; got != "tail" {
		t.Fatalf("final chunk = %q, want tail", got)
	}
	result := <-done
	if result.err != nil || result.code != 7 {
		t.Fatalf("RunShellStream = (%d, %v), want (7, nil)", result.code, result.err)
	}
	dialer.assertDone()
}

func TestRunShellStreamV2BoundsCallbackChunks(t *testing.T) {
	payload := bytes.Repeat([]byte{'x'}, shellWriteChunk*2+17)
	server, dialer := testServer(t,
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host-serial:SERIAL:features")
			writeTestHostReply(t, conn, "shell_v2")
		},
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host:transport:SERIAL")
			writeTestStatus(t, conn, "OKAY")
			expectTestRequest(t, conn, "shell,v2,raw:large")
			writeTestStatus(t, conn, "OKAY")
			_ = writeShellPacket(conn, shellIDStdout, payload)
			_ = writeShellPacket(conn, shellIDExit, []byte{0})
		},
	)

	total, calls := 0, 0
	code, err := server.RunShellStream(context.Background(), "SERIAL", "large", func(chunk []byte) {
		calls++
		total += len(chunk)
		if len(chunk) > shellWriteChunk {
			t.Errorf("callback chunk = %d bytes", len(chunk))
		}
	})
	if err != nil || code != 0 {
		t.Fatalf("RunShellStream = (%d, %v)", code, err)
	}
	if total != len(payload) || calls != 3 {
		t.Fatalf("streamed %d bytes in %d calls, want %d bytes in 3", total, calls, len(payload))
	}
	dialer.assertDone()
}

func TestRunShellStreamCancellationInterruptsPacketRead(t *testing.T) {
	serviceReady := make(chan struct{})
	server, dialer := testServer(t,
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host-serial:SERIAL:features")
			writeTestHostReply(t, conn, "shell_v2")
		},
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host:transport:SERIAL")
			writeTestStatus(t, conn, "OKAY")
			expectTestRequest(t, conn, "shell,v2,raw:sleep")
			writeTestStatus(t, conn, "OKAY")
			close(serviceReady)
			_, _ = io.Copy(io.Discard, conn)
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := server.RunShellStream(ctx, "SERIAL", "sleep", nil)
		done <- err
	}()
	<-serviceReady
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RunShellStream error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("RunShellStream did not unblock after cancellation")
	}
	dialer.assertDone()
}

func TestRunShellStreamLegacyRecoversSuccessStatusAndStreamsLive(t *testing.T) {
	releaseStatus := make(chan struct{})
	wantOutput := strings.Repeat("x", 256) + "\ntail"
	server, dialer := testServer(t,
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host-serial:OLD:features")
			writeTestHostReply(t, conn, "stat_v2")
		},
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host:transport:OLD")
			writeTestStatus(t, conn, "OKAY")
			service := readTestRequest(t, conn)
			const prefix = "shell:sh -c 'do work' </dev/null; __f4_apply_status=$?; printf '"
			const suffix = "%u' \"$__f4_apply_status\""
			if !strings.HasPrefix(service, prefix) || !strings.HasSuffix(service, suffix) {
				t.Errorf("legacy shell service = %q", service)
				return
			}
			marker := strings.TrimSuffix(strings.TrimPrefix(service, prefix), suffix)
			if len(marker) != len(legacyShellStatusMarkerPrefix)+legacyShellStatusNonceBytes*2+2 ||
				!strings.HasPrefix(marker, legacyShellStatusMarkerPrefix) || !strings.HasSuffix(marker, "__") {
				t.Errorf("legacy status marker = %q", marker)
				return
			}
			writeTestStatus(t, conn, "OKAY")
			_, _ = io.WriteString(conn, wantOutput)
			<-releaseStatus
			// Split the marker to exercise framing independently of socket reads.
			mid := len(marker) / 2
			_, _ = io.WriteString(conn, marker[:mid])
			_, _ = io.WriteString(conn, marker[mid:]+"0")
		},
	)

	var (
		mu     sync.Mutex
		output bytes.Buffer
	)
	live := make(chan struct{}, 1)
	done := make(chan struct {
		code int
		err  error
	}, 1)
	go func() {
		code, err := server.RunShellStream(context.Background(), "OLD", "do work", func(chunk []byte) {
			if len(chunk) > shellWriteChunk {
				t.Errorf("callback chunk = %d bytes", len(chunk))
			}
			mu.Lock()
			_, _ = output.Write(chunk)
			mu.Unlock()
			select {
			case live <- struct{}{}:
			default:
			}
		})
		done <- struct {
			code int
			err  error
		}{code, err}
	}()

	select {
	case <-live:
		// Output larger than the bounded marker tail arrives before status.
	case <-time.After(time.Second):
		t.Fatal("legacy output was not streamed before command completion")
	}
	close(releaseStatus)
	var result struct {
		code int
		err  error
	}
	select {
	case result = <-done:
	case <-time.After(time.Second):
		t.Fatal("legacy RunShellStream did not finish")
	}
	if result.err != nil || result.code != 0 {
		t.Fatalf("RunShellStream = (%d, %v), want (0, nil)", result.code, result.err)
	}
	mu.Lock()
	gotOutput := output.String()
	mu.Unlock()
	if gotOutput != wantOutput {
		t.Fatalf("legacy output = %q, want %q", gotOutput, wantOutput)
	}
	dialer.assertDone()
}

func TestRunShellStreamLegacyCancellationInterruptsRead(t *testing.T) {
	serviceReady := make(chan struct{})
	server, dialer := testServer(t,
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host-serial:OLD:features")
			writeTestHostReply(t, conn, "stat_v2")
		},
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host:transport:OLD")
			writeTestStatus(t, conn, "OKAY")
			service := readTestRequest(t, conn)
			if !strings.HasPrefix(service, "shell:sh -c 'sleep' </dev/null;") {
				t.Errorf("legacy shell service = %q", service)
				return
			}
			writeTestStatus(t, conn, "OKAY")
			close(serviceReady)
			_, _ = io.Copy(io.Discard, conn)
		},
	)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := server.RunShellStream(ctx, "OLD", "sleep", nil)
		done <- err
	}()
	<-serviceReady
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RunShellStream error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("legacy RunShellStream did not unblock after cancellation")
	}
	dialer.assertDone()
}

func TestShellStreamFramesStdin(t *testing.T) {
	server, dialer := testServer(t,
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host-serial:SERIAL:features")
			writeTestHostReply(t, conn, "shell_v2")
		},
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host:transport:SERIAL")
			writeTestStatus(t, conn, "OKAY")
			expectTestRequest(t, conn, "shell,v2,raw:exec /system/bin/sh")
			writeTestStatus(t, conn, "OKAY")
			id, payload, err := readShellPacket(conn)
			if err != nil {
				t.Errorf("read stdin packet: %v", err)
				return
			}
			if id != shellIDStdin || string(payload) != "request" {
				t.Errorf("stdin packet = (%d, %q)", id, payload)
			}
			_ = writeShellPacket(conn, shellIDStdout, []byte("response"))
			_ = writeShellPacket(conn, shellIDExit, []byte{0})
		},
	)

	raw, err := server.OpenShellRaw(context.Background(), "SERIAL", "exec /system/bin/sh")
	if err != nil {
		t.Fatalf("OpenShellRaw: %v", err)
	}
	defer func() {
		_ = raw.Close() // connection cleanup only
	}()
	if _, err := raw.Write([]byte("request")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	response, err := io.ReadAll(raw)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(response) != "response" {
		t.Fatalf("response = %q", response)
	}
	dialer.assertDone()
}

func TestRunShellLegacyFallback(t *testing.T) {
	server, dialer := testServer(t,
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host-serial:OLD:features")
			writeTestHostReply(t, conn, "stat_v2")
		},
		func(conn net.Conn) {
			expectTestRequest(t, conn, "host:transport:OLD")
			writeTestStatus(t, conn, "OKAY")
			expectTestRequest(t, conn, "shell:echo old")
			writeTestStatus(t, conn, "OKAY")
			_, _ = io.WriteString(conn, "old\n")
		},
	)

	stdout, stderr, exitCode, err := server.RunShell(context.Background(), "OLD", "echo old")
	if err != nil {
		t.Fatalf("RunShell: %v", err)
	}
	if string(stdout) != "old\n" || stderr != nil || exitCode != -1 {
		t.Fatalf("RunShell = (%q, %q, %d)", stdout, stderr, exitCode)
	}
	dialer.assertDone()
}

func TestServiceFailureIsTyped(t *testing.T) {
	server, dialer := testServer(t, func(conn net.Conn) {
		expectTestRequest(t, conn, "host:transport:OFFLINE")
		_, _ = io.WriteString(conn, "FAIL000Edevice offline")
	})

	_, err := server.OpenService(context.Background(), "OFFLINE", "sync:")
	var serviceErr *ServiceError
	if !errors.As(err, &serviceErr) {
		t.Fatalf("error = %v, want *ServiceError", err)
	}
	if serviceErr.Service != "host:transport:OFFLINE" || serviceErr.Message != "device offline" {
		t.Fatalf("ServiceError = %#v", serviceErr)
	}
	dialer.assertDone()
}

func TestFailedConnectStartsInstalledADBAndRetries(t *testing.T) {
	var calls atomic.Int32
	var starts atomic.Int32
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		call := calls.Add(1)
		if call <= 2 {
			return nil, errors.New("connection refused")
		}
		client, peer := net.Pipe()
		go func() {
			defer func() {
				_ = peer.Close() // connection cleanup only
			}()
			expectTestRequest(t, peer, "host:devices-l")
			writeTestHostReply(t, peer, "")
		}()
		return client, nil
	}
	server := NewServer(
		WithADBDialer(dial),
		WithADBLookup(func() (string, error) { return "/sdk/platform-tools/adb", nil }),
		WithADBStarter(func(ctx context.Context, path string) error {
			starts.Add(1)
			if path != "/sdk/platform-tools/adb" {
				t.Errorf("adb path = %q", path)
			}
			return nil
		}),
	)

	devices, err := server.Devices(context.Background())
	if err != nil {
		t.Fatalf("Devices: %v", err)
	}
	if len(devices) != 0 || calls.Load() != 3 || starts.Load() != 1 {
		t.Fatalf("devices=%v calls=%d starts=%d", devices, calls.Load(), starts.Load())
	}
}

func TestCancelledHostRequestUnblocksRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	server, dialer := testServer(t, func(conn net.Conn) {
		expectTestRequest(t, conn, "host:devices-l")
		cancel()
		_, _ = io.Copy(io.Discard, conn)
	})
	_, err := server.Devices(ctx)
	if err == nil {
		t.Fatal("Devices unexpectedly succeeded")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Devices error = %v, want context canceled", err)
	}
	dialer.assertDone()
}

type shortWriteBuffer struct {
	bytes.Buffer
	limit int
}

func (w *shortWriteBuffer) Write(p []byte) (int, error) {
	if len(p) > w.limit {
		p = p[:w.limit]
	}
	return w.Buffer.Write(p)
}

func TestTransportWritesCompleteFramesThroughShortWriter(t *testing.T) {
	w := &shortWriteBuffer{limit: 2}
	if err := writeADBRequest(w, "host:features"); err != nil {
		t.Fatalf("writeADBRequest: %v", err)
	}
	if got, want := w.String(), "000Dhost:features"; got != want {
		t.Fatalf("ADB frame = %q, want %q", got, want)
	}
	w.Reset()
	if err := writeShellPacket(w, shellIDStdin, []byte("payload")); err != nil {
		t.Fatalf("writeShellPacket: %v", err)
	}
	id, payload, err := readShellPacket(bytes.NewReader(w.Bytes()))
	if err != nil || id != shellIDStdin || string(payload) != "payload" {
		t.Fatalf("shell frame = id %d payload %q err %v", id, payload, err)
	}
}

func TestReadAllWithLimit(t *testing.T) {
	got, err := readAllWithLimit(strings.NewReader("abc"), 3)
	if err != nil || string(got) != "abc" {
		t.Fatalf("exact limit = %q, %v", got, err)
	}
	if got, err := readAllWithLimit(strings.NewReader("abcd"), 3); err == nil || got != nil || !strings.Contains(err.Error(), "exceeds 3 bytes") {
		t.Fatalf("over limit = %q, %v", got, err)
	}
}

func writeFakeADBExecutable(t *testing.T, root string) string {
	t.Helper()
	name := "adb"
	if runtime.GOOS == "windows" {
		name = "adb.exe"
	}
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("fake"), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chmod(path, 0700); err != nil { // #nosec G302 -- the fake adb must be executable for command-discovery tests.
		t.Fatalf("Chmod: %v", err)
	}
	return path
}

func TestFindADBExecutableHonorsOverrideAndSDKRoots(t *testing.T) {
	t.Run("F4_ADB_PATH", func(t *testing.T) {
		path := writeFakeADBExecutable(t, t.TempDir())
		t.Setenv("F4_ADB_PATH", path)
		got, err := findADBExecutable()
		if err != nil || got != path {
			t.Fatalf("findADBExecutable = %q, %v; want %q", got, err, path)
		}
	})

	t.Run("ANDROID_SDK_ROOT", func(t *testing.T) {
		root := t.TempDir()
		path := writeFakeADBExecutable(t, filepath.Join(root, "platform-tools"))
		t.Setenv("F4_ADB_PATH", "")
		t.Setenv("PATH", "")
		t.Setenv("ANDROID_SDK_ROOT", root)
		t.Setenv("ANDROID_HOME", "")
		got, err := findADBExecutable()
		if err != nil || got != path {
			t.Fatalf("findADBExecutable = %q, %v; want %q", got, err, path)
		}
	})
}
