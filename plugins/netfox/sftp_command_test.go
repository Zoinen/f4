package netfox

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"

	"github.com/unxed/f4/vfs"
)

type fakeSFTPExitError struct{ status int }

func (e fakeSFTPExitError) Error() string   { return "remote exit" }
func (e fakeSFTPExitError) ExitStatus() int { return e.status }

type fakeSFTPCommandSession struct {
	stdout io.Writer
	stderr io.Writer

	startErr error
	waitErr  error
	command  string
	started  chan struct{}
	waitGate chan struct{}

	mu       sync.Mutex
	signals  []ssh.Signal
	closed   chan struct{}
	closeOne sync.Once
}

func newFakeSFTPCommandSession() *fakeSFTPCommandSession {
	return &fakeSFTPCommandSession{started: make(chan struct{}), closed: make(chan struct{})}
}

func (s *fakeSFTPCommandSession) Configure(stdout, stderr io.Writer) {
	s.stdout, s.stderr = stdout, stderr
}

func (s *fakeSFTPCommandSession) Start(command string) error {
	s.command = command
	close(s.started)
	return s.startErr
}

func (s *fakeSFTPCommandSession) Wait() error {
	if s.waitGate != nil {
		<-s.waitGate
	}
	return s.waitErr
}

func (s *fakeSFTPCommandSession) Signal(signal ssh.Signal) error {
	s.mu.Lock()
	s.signals = append(s.signals, signal)
	s.mu.Unlock()
	return nil
}

func (s *fakeSFTPCommandSession) Close() error {
	s.closeOne.Do(func() {
		close(s.closed)
		if s.waitGate != nil {
			close(s.waitGate)
		}
	})
	return nil
}

func TestRunSFTPCommandSessionStreamsCombinedLinesAndStatus(t *testing.T) {
	session := newFakeSFTPCommandSession()
	session.waitErr = fakeSFTPExitError{status: 9}
	var got []string

	// Write while Wait is blocked so the test observes that callbacks stream
	// during execution rather than being collected after the remote exit.
	session.waitGate = make(chan struct{})
	done := make(chan struct {
		code int
		err  error
	}, 1)
	go func() {
		code, err := runSFTPCommandSession(context.Background(), session, "/srv/a'b", "do work # note", func(line string) {
			got = append(got, line)
		})
		done <- struct {
			code int
			err  error
		}{code, err}
	}()
	<-session.started
	_, _ = io.WriteString(session.stdout, "one\r\npart")
	_, _ = io.WriteString(session.stderr, "ial\ntail")
	if want := []string{"one", "partial"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("streamed lines before exit = %#v, want %#v", got, want)
	}
	_ = session.Close()
	result := <-done
	if result.err != nil || result.code != 9 {
		t.Fatalf("result = (%d, %v), want (9, nil)", result.code, result.err)
	}
	if want := []string{"one", "partial", "tail"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("all lines = %#v, want %#v", got, want)
	}
	if want := "cd '/srv/a'\"'\"'b' && (\ndo work # note\n)"; session.command != want {
		t.Fatalf("remote command = %q, want %q", session.command, want)
	}
}

func TestRunSFTPCommandSessionCancellationSignalsAndCloses(t *testing.T) {
	session := newFakeSFTPCommandSession()
	session.waitGate = make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := runSFTPCommandSession(ctx, session, "/", "sleep 30", nil)
		done <- err
	}()
	<-session.started
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("command did not return after cancellation")
	}
	select {
	case <-session.closed:
	default:
		t.Fatal("SSH command session was not closed")
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if !reflect.DeepEqual(session.signals, []ssh.Signal{ssh.SIGKILL}) {
		t.Fatalf("signals = %#v, want SIGKILL", session.signals)
	}
}

func TestSFTPCommandLineWriterNormalizesInvalidOutput(t *testing.T) {
	var got string
	w := newSFTPCommandLineWriter(func(line string) { got = line })
	if _, err := w.Write([]byte{'x', 0xff, '\n'}); err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("output is not valid UTF-8: %q", got)
	}
}

func TestSFTPCommandLineWriterBoundsUnterminatedOutput(t *testing.T) {
	var chunks []string
	w := newSFTPCommandLineWriter(func(line string) { chunks = append(chunks, line) })
	payload := bytes.Repeat([]byte{'x'}, sftpCommandOutputChunkBytes*2+17)
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	if len(w.pending) > sftpCommandOutputChunkBytes {
		t.Fatalf("pending output = %d bytes", len(w.pending))
	}
	w.Flush()
	total := 0
	for _, chunk := range chunks {
		if len(chunk) > sftpCommandOutputChunkBytes {
			t.Fatalf("callback chunk = %d bytes", len(chunk))
		}
		total += len(chunk)
	}
	if total != len(payload) {
		t.Fatalf("streamed bytes = %d, want %d", total, len(payload))
	}
}

func TestSFTPCommandLineWriterDoesNotSplitUTF8RuneAtChunkBoundary(t *testing.T) {
	var chunks []string
	w := newSFTPCommandLineWriter(func(line string) { chunks = append(chunks, line) })
	payload := append(bytes.Repeat([]byte{'x'}, sftpCommandOutputChunkBytes-1), []byte("яz")...)
	if _, err := w.Write(payload); err != nil {
		t.Fatal(err)
	}
	w.Flush()
	if got := strings.Join(chunks, ""); got != string(payload) {
		t.Fatalf("joined chunks lost UTF-8 data at boundary: %q", got[len(got)-8:])
	}
}

func TestSFTPCommandRunnerInfo(t *testing.T) {
	info := (&SFTPVFS{}).CommandRunnerInfo()
	if info.Dialect != vfs.CommandDialectPOSIX || info.MaxParallel != 4 {
		t.Fatalf("runner info = %+v", info)
	}
}

func TestSFTPCommandListANSIUsesConfiguredCodepage(t *testing.T) {
	v := &SFTPVFS{codepage: "1251"}
	input := []byte("имя.txt\n")
	got, err := v.EncodeCommandListANSI(input)
	if err != nil {
		t.Fatal(err)
	}
	want, err := vfs.EncodeBytes(input, 1251)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("encoded list = %x, want %x", got, want)
	}
}

func TestSFTPCommandCodecUsesConfiguredPanelCodepage(t *testing.T) {
	v := &SFTPVFS{codepage: "1251"}
	codec, err := v.commandCodec()
	if err != nil {
		t.Fatal(err)
	}
	wire := "cd '/папка' && ( echo имя )"
	encoded, err := codec.encode(wire)
	if err != nil {
		t.Fatal(err)
	}
	wantWire, err := vfs.EncodeBytes([]byte(wire), 1251)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual([]byte(encoded), wantWire) {
		t.Fatalf("encoded command = %x, want %x", []byte(encoded), wantWire)
	}
	remoteOutput, err := vfs.EncodeBytes([]byte("готово"), 1251)
	if err != nil {
		t.Fatal(err)
	}
	if got := codec.decode(remoteOutput); got != "готово" {
		t.Fatalf("decoded output = %q", got)
	}
	if _, err := (&SFTPVFS{codepage: "1200"}).commandCodec(); err == nil {
		t.Fatal("UTF-16 command transport was accepted")
	}
}

func TestSFTPVFSCloneRetainsSharedConnectionLifetime(t *testing.T) {
	shared := &sftpConnectionRefs{refs: 1}
	original := &SFTPVFS{shared: shared, path: "/captured", codepage: "65001"}
	clone, ok := original.Clone().(*SFTPVFS)
	if !ok || clone == original || clone.GetPath() != "/captured" {
		t.Fatalf("clone = %#v", clone)
	}
	shared.Lock()
	refs := shared.refs
	shared.Unlock()
	if refs != 2 {
		t.Fatalf("refs after clone = %d", refs)
	}
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}
	shared.Lock()
	refs = shared.refs
	shared.Unlock()
	if refs != 1 {
		t.Fatalf("refs after original close = %d", refs)
	}
	if err := clone.Close(); err != nil {
		t.Fatal(err)
	}
	shared.Lock()
	refs = shared.refs
	shared.Unlock()
	if refs != 0 {
		t.Fatalf("refs after clone close = %d", refs)
	}
}

func TestFishCommandRunnerInfo(t *testing.T) {
	info := (&FishVFS{}).CommandRunnerInfo()
	if info.Dialect != vfs.CommandDialectPOSIX || info.MaxParallel != 1 {
		t.Fatalf("runner info = %+v", info)
	}
}

func TestFishCommandRunnerUnavailableWithoutNegotiatedJobs(t *testing.T) {
	if (&FishVFS{}).CommandRunnerAvailable() {
		t.Fatal("empty FISH+ connection reported a command executor")
	}
}
