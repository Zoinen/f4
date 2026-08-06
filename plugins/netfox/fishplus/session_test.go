package fishplus

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type mockRequest struct {
	ID    string
	Cmd   string
	Args  []string
	Paths []string
}

type gateBlockingReader struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (r *gateBlockingReader) Read([]byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	<-r.release
	return 0, io.EOF
}

// decodePaths returns the path lines of a mock request, undoing the tilde
// escape where the client had to fall back to base64.
func (r mockRequest) decodePaths(t *testing.T) []string {
	t.Helper()
	out := make([]string, 0, len(r.Paths))
	for _, line := range r.Paths {
		if !strings.HasPrefix(line, "~") {
			out = append(out, line)
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(line, "~"))
		if err != nil {
			t.Fatalf("path line %q is not valid base64: %v", line, err)
		}
		out = append(out, string(raw))
	}
	return out
}

// newMockPeer returns a session wired to an in-memory peer. The peer greets
// the client with banner (unless empty) and then answers every request via
// handle. Reading and writing happen in separate goroutines, so the client
// may keep writing while the peer is answering.
func newMockPeer(t *testing.T, banner string, handle func(w io.Writer, token string, req mockRequest), pathLines ...int) *Session {
	t.Helper()
	extra := 0
	if len(pathLines) > 0 {
		extra = pathLines[0]
	}
	peerR, cliW := io.Pipe()
	cliR, peerW := io.Pipe()
	sess := NewSession(cliW, cliR, nil)

	reqs := make(chan mockRequest, 32)
	uploaded := make(chan struct{})
	var uploadedOnce sync.Once
	go func() {
		defer close(reqs)
		sc := bufio.NewScanner(peerR)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		scriptDone := false
		for sc.Scan() {
			if !scriptDone {
				if sc.Text() == HelperEndMarker {
					// The helper is in; a real shell would start it now.
					scriptDone = true
					uploadedOnce.Do(func() { close(uploaded) })
				}
				continue
			}
			fields := strings.Fields(sc.Text())
			if len(fields) < 2 {
				continue
			}
			if _, err := strconv.Atoi(fields[0]); err != nil {
				continue // a line of the helper script, not a request
			}
			req := mockRequest{ID: fields[0], Cmd: fields[1], Args: fields[2:]}
			for i := 0; i < extra && sc.Scan(); i++ {
				req.Paths = append(req.Paths, sc.Text())
			}
			reqs <- req
		}
	}()
	go func() {
		if banner != "" {
			fmt.Fprintf(peerW, "Last login: never, this host is a fake\n")
			// The bootstrap announces itself, takes the script, and only
			// then does the helper get to speak.
			fmt.Fprintf(peerW, "%s\n", ReadyMarker(sess.Token()))
			<-uploaded
			fmt.Fprintf(peerW, ".%s 0 %s\n", sess.Token(), banner)
		}
		for req := range reqs {
			if handle != nil {
				handle(peerW, sess.Token(), req)
			}
		}
	}()
	t.Cleanup(func() {
		cliW.Close()
		peerW.Close()
	})
	return sess
}

func TestHandshakeParsesBannerAndSkipsNoise(t *testing.T) {
	sess := newMockPeer(t, "ok FISHPLUS 1 dd base64 stat", nil)
	if err := sess.Handshake(context.Background()); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	feats := sess.Features()
	if feats.Proto != ProtocolVersion {
		t.Errorf("Proto = %d, want %d", feats.Proto, ProtocolVersion)
	}
	for _, name := range []string{"dd", "base64", "stat"} {
		if !feats.Has(name) {
			t.Errorf("feature %q not announced, raw = %q", name, feats.Raw)
		}
	}
	if feats.Has("find") {
		t.Error("feature \"find\" reported but never announced")
	}
	if got := strings.Join(feats.Names(), " "); got != "base64 dd stat" {
		t.Errorf("Names() = %q, want %q", got, "base64 dd stat")
	}
}

func TestHandshakeBase64WritesOneLine(t *testing.T) {
	var stdin bytes.Buffer
	var stdout bytes.Buffer
	sess := NewSession(&stdin, &stdout, nil)
	fmt.Fprintf(&stdout, "login noise\n%s\n.%s 0 ok FISHPLUS 1 dd base64\n", ReadyMarker(sess.Token()), sess.Token())

	err := sess.HandshakeWithOptions(context.Background(), HandshakeOptions{Bootstrap: BootstrapBase64Line})
	if err != nil {
		t.Fatalf("base64 handshake: %v", err)
	}
	if got := strings.Count(stdin.String(), "\n"); got != 1 {
		t.Fatalf("base64 handshake wrote %d lines, want 1", got)
	}
	if strings.Contains(stdin.String(), HelperEndMarker+"\n") {
		t.Fatal("base64 handshake also uploaded the streaming end marker")
	}
	if !sess.Features().Has("base64") {
		t.Fatal("base64 handshake did not parse the helper banner")
	}
}

func TestHandshakeBase64ReportsDecoderFailure(t *testing.T) {
	var stdin bytes.Buffer
	var stdout bytes.Buffer
	sess := NewSession(&stdin, &stdout, nil)
	fmt.Fprintf(&stdout, "%s\n.%s 0 err bootstrap base64 decoder unavailable\n", ReadyMarker(sess.Token()), sess.Token())

	err := sess.HandshakeWithOptions(context.Background(), HandshakeOptions{Bootstrap: BootstrapBase64Line})
	if err == nil || !strings.Contains(err.Error(), "decoder unavailable") {
		t.Fatalf("base64 handshake error = %v, want framed decoder failure", err)
	}
	if !sess.Broken() {
		t.Fatal("failed base64 handshake did not mark the session broken")
	}
}

func TestHandshakeRejectsUnknownBootstrapBeforeWriting(t *testing.T) {
	var stdin bytes.Buffer
	sess := NewSession(&stdin, strings.NewReader(""), nil)
	err := sess.HandshakeWithOptions(context.Background(), HandshakeOptions{Bootstrap: BootstrapMethod(255)})
	if err == nil || !strings.Contains(err.Error(), "unsupported bootstrap") {
		t.Fatalf("unknown bootstrap error = %v", err)
	}
	if stdin.Len() != 0 {
		t.Fatalf("unknown bootstrap wrote %d bytes", stdin.Len())
	}
	if sess.Broken() {
		t.Fatal("a locally rejected bootstrap method should not break the untouched session")
	}
}

func TestHandshakeOwnsCancellableRequestGate(t *testing.T) {
	reader := &gateBlockingReader{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(reader.release) }) }
	defer release()

	sess := NewSession(io.Discard, reader, nil)
	handshakeDone := make(chan error, 1)
	go func() { handshakeDone <- sess.Handshake(context.Background()) }()
	select {
	case <-reader.started:
	case <-time.After(time.Second):
		t.Fatal("handshake did not start reading")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	execDone := make(chan error, 1)
	go func() {
		_, err := sess.Exec(ctx, "queued")
		execDone <- err
	}()
	select {
	case err := <-execDone:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("request queued behind handshake = %v, want context deadline", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("request cancellation could not bypass the active handshake")
	}

	release()
	select {
	case err := <-handshakeDone:
		if !errors.Is(err, io.EOF) {
			t.Fatalf("handshake after release = %v, want EOF", err)
		}
	case <-time.After(time.Second):
		t.Fatal("handshake did not stop after reader release")
	}
}

func TestTryNoopDoesNotQueueBehindBusySession(t *testing.T) {
	sess := NewSession(io.Discard, strings.NewReader(""), io.NopCloser(strings.NewReader("")))
	sess.mu.Lock()
	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	attempted, err := sess.TryNoop(ctx)
	sess.mu.Unlock()
	if err != nil {
		t.Fatalf("TryNoop on busy session: %v", err)
	}
	if attempted {
		t.Fatal("TryNoop queued behind a busy session")
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("busy TryNoop took %s", elapsed)
	}
}

func TestCanceledRequestLeavesBusyQueueImmediately(t *testing.T) {
	clientConn, peerConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = peerConn.Close()
	})
	sess := NewSession(clientConn, clientConn, clientConn)
	activeSeen := make(chan struct{})
	releaseActive := make(chan struct{})
	latestSeen := make(chan string, 1)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseActive) }) }
	t.Cleanup(release)

	go func() {
		reader := bufio.NewReader(peerConn)
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return
		}
		close(activeSeen)
		<-releaseActive
		_, _ = fmt.Fprintf(peerConn, ".%s %s ok\n", sess.Token(), fields[0])

		line, err = reader.ReadString('\n')
		if err != nil {
			return
		}
		fields = strings.Fields(line)
		if len(fields) < 2 {
			return
		}
		latestSeen <- fields[1]
		_, _ = fmt.Fprintf(peerConn, ".%s %s ok\n", sess.Token(), fields[0])
	}()

	activeDone := make(chan error, 1)
	go func() {
		_, err := sess.Exec(context.Background(), "active")
		activeDone <- err
	}()
	select {
	case <-activeSeen:
	case <-time.After(time.Second):
		t.Fatal("active request did not reach the peer")
	}

	cancelled, cancel := context.WithCancel(context.Background())
	staleStarted := make(chan struct{})
	staleDone := make(chan error, 1)
	go func() {
		close(staleStarted)
		_, err := sess.Exec(cancelled, "stale")
		staleDone <- err
	}()
	<-staleStarted
	// Give the request a chance to enter the gate wait before cancelling it.
	// The assertion below still has a generous margin for a loaded test host.
	time.Sleep(10 * time.Millisecond)
	start := time.Now()
	cancel()
	select {
	case err := <-staleDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled queued request = %v, want context.Canceled", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("cancelled queued request remained behind the active request")
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("cancelled queued request waited %s for the active request", elapsed)
	}

	latestDone := make(chan error, 1)
	go func() {
		_, err := sess.Exec(context.Background(), "latest")
		latestDone <- err
	}()
	release()

	select {
	case err := <-activeDone:
		if err != nil {
			t.Fatalf("active request: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("active request did not finish")
	}
	select {
	case command := <-latestSeen:
		if command != "latest" {
			t.Fatalf("peer received %q after active request, want latest", command)
		}
	case <-time.After(time.Second):
		t.Fatal("latest request did not reach the peer")
	}
	select {
	case err := <-latestDone:
		if err != nil {
			t.Fatalf("latest request: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("latest request did not finish")
	}
}

func TestCloseWakesRequestWaitingForGate(t *testing.T) {
	clientConn, peerConn := net.Pipe()
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = peerConn.Close()
	})
	// Deliberately omit a closer: Close cannot interrupt the active read, so
	// only closeCh can release the request waiting behind it.
	sess := NewSession(clientConn, clientConn, nil)
	activeSeen := make(chan string, 1)
	releaseActive := make(chan struct{})
	go func() {
		reader := bufio.NewReader(peerConn)
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return
		}
		activeSeen <- fields[0]
		<-releaseActive
		_, _ = fmt.Fprintf(peerConn, ".%s %s ok\n", sess.Token(), fields[0])
	}()

	activeDone := make(chan error, 1)
	go func() {
		_, err := sess.Exec(context.Background(), "active")
		activeDone <- err
	}()
	var activeID string
	select {
	case activeID = <-activeSeen:
		if activeID == "" {
			t.Fatal("peer received an empty active request id")
		}
	case <-time.After(time.Second):
		t.Fatal("active request did not reach the peer")
	}

	waiterStarted := make(chan struct{})
	waiterDone := make(chan error, 1)
	go func() {
		close(waiterStarted)
		_, err := sess.Exec(context.Background(), "waiting")
		waiterDone <- err
	}()
	<-waiterStarted
	time.Sleep(10 * time.Millisecond)
	if err := sess.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	select {
	case err := <-waiterDone:
		if !errors.Is(err, ErrBroken) {
			t.Fatalf("waiting request after Close = %v, want ErrBroken", err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Close did not wake the request waiting for the protocol gate")
	}

	close(releaseActive)
	select {
	case err := <-activeDone:
		if err != nil {
			t.Fatalf("active request after release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("active request did not finish after release")
	}
}

func TestTryNoopTimeoutInterruptsSilentTransport(t *testing.T) {
	clientConn, peerConn := net.Pipe()
	defer peerConn.Close()
	sess := NewSession(clientConn, clientConn, clientConn)
	requestSeen := make(chan struct{})
	go func() {
		reader := bufio.NewReader(peerConn)
		_, _ = reader.ReadString('\n')
		close(requestSeen)
		_, _ = io.Copy(io.Discard, reader)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	type result struct {
		attempted bool
		err       error
	}
	done := make(chan result, 1)
	go func() {
		attempted, err := sess.TryNoop(ctx)
		done <- result{attempted: attempted, err: err}
	}()
	select {
	case <-requestSeen:
	case <-time.After(time.Second):
		t.Fatal("TryNoop did not send its request")
	}
	select {
	case got := <-done:
		if !got.attempted || !errors.Is(got.err, context.DeadlineExceeded) {
			t.Fatalf("TryNoop = attempted %t, err %v", got.attempted, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("TryNoop timeout did not interrupt the blocked transport read")
	}
	if !sess.Broken() {
		t.Fatal("timed-out probe left the half-read session reusable")
	}
}

func TestTryNoopReleasesSessionBeforeReturning(t *testing.T) {
	clientConn, peerConn := net.Pipe()
	defer clientConn.Close()
	defer peerConn.Close()
	sess := NewSession(clientConn, clientConn, clientConn)
	go func() {
		reader := bufio.NewReader(peerConn)
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		fields := strings.Fields(line)
		if len(fields) < 1 {
			return
		}
		_, _ = fmt.Fprintf(peerConn, ".%s %s ok\n", sess.Token(), fields[0])
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	attempted, err := sess.TryNoop(ctx)
	if err != nil || !attempted {
		t.Fatalf("TryNoop = attempted %t, err %v", attempted, err)
	}
	if !sess.mu.TryLock() {
		t.Fatal("TryNoop returned before releasing the protocol mutex")
	}
	sess.mu.Unlock()
	if !sess.tryAcquireRequest() {
		t.Fatal("TryNoop returned before releasing the request gate")
	}
	sess.releaseRequest()
}

func TestTryNoopWithoutCloserDoesNotAttempt(t *testing.T) {
	var output bytes.Buffer
	sess := NewSession(&output, strings.NewReader(""), nil)
	start := time.Now()
	attempted, err := sess.TryNoop(context.Background())
	if err != nil {
		t.Fatalf("TryNoop without closer: %v", err)
	}
	if attempted {
		t.Fatal("TryNoop used a transport that cannot be interrupted")
	}
	if output.Len() != 0 {
		t.Fatalf("TryNoop without closer wrote %q", output.String())
	}
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("TryNoop without closer took %s", elapsed)
	}
}

func TestHandshakeRejectsForeignProtocol(t *testing.T) {
	sess := newMockPeer(t, "ok FISHPLUS 999 dd", nil)
	err := sess.Handshake(context.Background())
	if err == nil {
		t.Fatal("handshake accepted an unknown protocol version")
	}
	if !strings.Contains(err.Error(), "999") {
		t.Errorf("error %v does not mention the remote version", err)
	}
	if !sess.Broken() {
		t.Error("session with a foreign protocol must be marked broken")
	}
}

func TestHandshakeReportsRemoteFailure(t *testing.T) {
	sess := newMockPeer(t, "err no base64 decoder found on remote host", nil)
	err := sess.Handshake(context.Background())
	if err == nil {
		t.Fatal("handshake accepted a failing remote")
	}
	if !strings.Contains(err.Error(), "base64") {
		t.Errorf("error %v does not carry the remote message", err)
	}
}

func TestExecCollectsTextLines(t *testing.T) {
	sess := newMockPeer(t, "ok FISHPLUS 1 stat", func(w io.Writer, token string, req mockRequest) {
		fmt.Fprintf(w, "first\n")
		fmt.Fprintf(w, "#not a data frame in text mode\n")
		fmt.Fprintf(w, "last\n")
		fmt.Fprintf(w, ".%s %s ok\n", token, req.ID)
	})
	if err := sess.Handshake(context.Background()); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	resp, err := sess.Exec(context.Background(), "enum", "/tmp")
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if !resp.OK() {
		t.Fatalf("status = %q, msg = %q", resp.Status, resp.Msg)
	}
	want := []string{"first", "#not a data frame in text mode", "last"}
	if strings.Join(resp.Lines, "|") != strings.Join(want, "|") {
		t.Errorf("Lines = %q, want %q", resp.Lines, want)
	}
	if len(resp.Data) != 0 {
		t.Errorf("Data = %q, want empty in text mode", resp.Data)
	}
}

func TestExecPathEscapesOnlyWhenNecessary(t *testing.T) {
	var mu sync.Mutex
	var seen []mockRequest
	done := make(chan struct{}, 4)
	sess := newMockPeer(t, "ok FISHPLUS 1", func(w io.Writer, token string, req mockRequest) {
		mu.Lock()
		seen = append(seen, req)
		mu.Unlock()
		fmt.Fprintf(w, ".%s %s ok\n", token, req.ID)
		done <- struct{}{}
	}, 1)
	if err := sess.Handshake(context.Background()); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	// Everything but a newline has to travel raw: base64 costs a fork on
	// the remote host, so it stays a last resort.
	const raw = "/tmp/two words/пример\tтаб\\с чертой "
	const escaped = "/tmp/two\nlines"
	if _, err := sess.ExecPath(context.Background(), "info", raw, "-L"); err != nil {
		t.Fatalf("exec raw: %v", err)
	}
	if _, err := sess.ExecPath(context.Background(), "info", escaped); err != nil {
		t.Fatalf("exec escaped: %v", err)
	}
	<-done
	<-done
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 2 {
		t.Fatalf("peer saw %d requests, want 2", len(seen))
	}
	if seen[0].Cmd != "info" || len(seen[0].Args) != 1 || seen[0].Args[0] != "-L" {
		t.Errorf("request = %q %q", seen[0].Cmd, seen[0].Args)
	}
	if len(seen[0].Paths) != 1 || seen[0].Paths[0] != raw {
		t.Errorf("path line = %q, want it raw: %q", seen[0].Paths, raw)
	}
	if len(seen[1].Paths) != 1 || !strings.HasPrefix(seen[1].Paths[0], "~") {
		t.Fatalf("path with a newline was not escaped: %q", seen[1].Paths)
	}
	if got := seen[1].decodePaths(t); got[0] != escaped {
		t.Errorf("escaped path decoded to %q, want %q", got[0], escaped)
	}
}

func TestExecRejectsWhitespaceArguments(t *testing.T) {
	sess := newMockPeer(t, "ok FISHPLUS 1", nil)
	if err := sess.Handshake(context.Background()); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if _, err := sess.Exec(context.Background(), "mode", "two words"); err == nil {
		t.Error("an argument with a space must not be sent as a bare token")
	}
	if _, err := sess.Exec(context.Background(), "mode", ""); err == nil {
		t.Error("an empty argument must be rejected")
	}
}

func TestExecDataReadsBinaryFrames(t *testing.T) {
	payload := []byte{0x00, 0x01, '\n', 0xff, '.', '#', 'x'}
	sess := newMockPeer(t, "ok FISHPLUS 1 dd", func(w io.Writer, token string, req mockRequest) {
		fmt.Fprintf(w, "#%d\n", len(payload))
		w.Write(payload)
		fmt.Fprintf(w, "#%d\n", 3)
		w.Write([]byte("abc"))
		fmt.Fprintf(w, ".%s %s ok\n", token, req.ID)
	})
	if err := sess.Handshake(context.Background()); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	resp, err := sess.ExecData(context.Background(), "read", "/tmp/x", "0", "7")
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	want := append(append([]byte{}, payload...), []byte("abc")...)
	if !bytes.Equal(resp.Data, want) {
		t.Errorf("Data = %q, want %q", resp.Data, want)
	}
	if len(resp.Lines) != 0 {
		t.Errorf("Lines = %q, want none", resp.Lines)
	}
}

func TestRemoteErrorIsReported(t *testing.T) {
	sess := newMockPeer(t, "ok FISHPLUS 1", func(w io.Writer, token string, req mockRequest) {
		fmt.Fprintf(w, ".%s %s err No such file or directory\n", token, req.ID)
	})
	if err := sess.Handshake(context.Background()); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	resp, err := sess.Exec(context.Background(), "stat", "/nope")
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if resp.OK() {
		t.Fatal("failing request reported as ok")
	}
	err = resp.Err("stat")
	if err == nil {
		t.Fatal("Err() returned nil for a failed response")
	}
	var remote *RemoteError
	if !errors.As(err, &remote) {
		t.Fatalf("error %v is not a *RemoteError", err)
	}
	if remote.Msg != "No such file or directory" {
		t.Errorf("Msg = %q", remote.Msg)
	}
}

func TestClosedSessionRefusesRequests(t *testing.T) {
	sess := newMockPeer(t, "ok FISHPLUS 1", nil)
	if err := sess.Handshake(context.Background()); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := sess.Exec(context.Background(), "noop"); err != ErrBroken {
		t.Errorf("Exec after Close = %v, want ErrBroken", err)
	}
}

func TestCancelledContextIsNotSentToRemote(t *testing.T) {
	sess := newMockPeer(t, "ok FISHPLUS 1", nil)
	if err := sess.Handshake(context.Background()); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := sess.Exec(ctx, "noop"); err != context.Canceled {
		t.Errorf("Exec with cancelled context = %v, want context.Canceled", err)
	}
	if sess.Broken() {
		t.Error("session must survive a request that was never sent")
	}
}

// syncBuffer collects the child shell's stderr without racing the test.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestBase64BootstrapAgainstLocalShell proves that the generated one-line
// command is valid POSIX shell, finds a real decoder, evaluates the embedded
// helper without a temporary file, and leaves the protocol synchronized.
func TestBase64BootstrapAgainstLocalShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX shell on Windows")
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell available")
	}
	cmd := exec.Command(shell)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	stderr := &syncBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", shell, err)
	}
	sess := NewSession(stdin, stdout, stdin)
	t.Cleanup(func() {
		sess.Close()
		done := make(chan struct{})
		go func() {
			cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			cmd.Process.Kill()
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := sess.HandshakeWithOptions(ctx, HandshakeOptions{Bootstrap: BootstrapBase64Line}); err != nil {
		if strings.Contains(err.Error(), "base64") {
			t.Skipf("no base64 decoder on this host: %v", err)
		}
		t.Fatalf("base64 handshake: %v (shell stderr: %s)", err, stderr.String())
	}
	const payload = "one-line bootstrap: spaces, '$VARS', and\na newline"
	got, err := sess.Ping(ctx, payload)
	if err != nil {
		t.Fatalf("ping after base64 handshake: %v (shell stderr: %s)", err, stderr.String())
	}
	if got != payload {
		t.Fatalf("ping after base64 handshake = %q, want %q", got, payload)
	}
}

// TestHelperAgainstLocalShell runs the real helper script in a local POSIX
// shell. It is the only test that proves the script and the Go client agree
// on the wire format.
func TestHelperAgainstLocalShell(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("no POSIX shell on Windows")
	}
	shell, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell available")
	}
	cmd := exec.Command(shell)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout: %v", err)
	}
	stderr := &syncBuffer{}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", shell, err)
	}
	sess := NewSession(stdin, stdout, stdin)
	t.Cleanup(func() {
		sess.Close()
		done := make(chan struct{})
		go func() {
			cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			cmd.Process.Kill()
		}
	})

	ctx := context.Background()
	if err := sess.Handshake(ctx); err != nil {
		if strings.Contains(err.Error(), "base64") {
			t.Skipf("no base64 on this host: %v", err)
		}
		t.Fatalf("handshake: %v (shell stderr: %s)", err, stderr.String())
	}
	feats := sess.Features()
	if feats.Proto != ProtocolVersion {
		t.Fatalf("Proto = %d, want %d", feats.Proto, ProtocolVersion)
	}
	if !feats.Has("base64") {
		t.Errorf("base64 not announced although the handshake succeeded, raw = %q", feats.Raw)
	}

	if err := sess.Noop(ctx); err != nil {
		t.Errorf("noop: %v", err)
	}

	const payload = "spaces and юникод and 'quotes' and $VARS"
	got, err := sess.Ping(ctx, payload)
	if err != nil {
		t.Fatalf("ping: %v", err)
	}
	if got != payload {
		t.Errorf("ping echoed %q, want %q", got, payload)
	}

	resp, err := sess.Exec(ctx, "feats")
	if err != nil {
		t.Fatalf("feats: %v", err)
	}
	if len(resp.Lines) != 1 || !strings.HasPrefix(resp.Lines[0], strconv.Itoa(ProtocolVersion)) {
		t.Errorf("feats payload = %q", resp.Lines)
	}

	resp, err = sess.Exec(ctx, "frobnicate")
	if err != nil {
		t.Fatalf("unknown command must not break the session: %v", err)
	}
	if resp.OK() || resp.Msg != "unknown command" {
		t.Errorf("unknown command: status = %q, msg = %q", resp.Status, resp.Msg)
	}

	// The session has to stay usable after a rejected command.
	if err := sess.Noop(ctx); err != nil {
		t.Errorf("noop after error: %v", err)
	}
}
