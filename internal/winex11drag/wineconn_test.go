package winex11drag

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	winescape "github.com/unxed/libwinescape/go"
)

// The CI runner -- and every other place this test suite runs -- is not
// Wine, so hostmode.Posix() is false and OpenWineX11Connection must refuse
// before it ever touches winescape.DialUnix or the filesystem. This is the
// one thing about OpenWineX11Connection this package can test without a
// live Wine + X11 environment; see its doc comment for the rest.
func TestOpenWineX11ConnectionRefusesOutsidePosixMode(t *testing.T) {
	_, err := OpenWineX11Connection()
	if !errors.Is(err, ErrNotWineHost) {
		t.Fatalf("OpenWineX11Connection() error = %v, want %v", err, ErrNotWineHost)
	}
}

func TestWineConnSatisfiesNetConn(t *testing.T) {
	var c wineConn
	var _ net.Conn = c

	if c.LocalAddr() == nil || c.RemoteAddr() == nil {
		t.Fatalf("wineConn address stubs returned nil")
	}
	if err := c.SetDeadline(time.Time{}); err != nil {
		t.Errorf("SetDeadline: %v", err)
	}
	if err := c.SetReadDeadline(time.Time{}); err != nil {
		t.Errorf("SetReadDeadline: %v", err)
	}
	if err := c.SetWriteDeadline(time.Time{}); err != nil {
		t.Errorf("SetWriteDeadline: %v", err)
	}
}

func TestWineConnAddr(t *testing.T) {
	addr := wineConnAddr{}
	if addr.Network() == "" {
		t.Error("wineConnAddr.Network() is empty")
	}
	if addr.String() == "" {
		t.Error("wineConnAddr.String() is empty")
	}
}

// zeroEOFReader mimics winescape.File.Read: the data, then (0, nil) forever.
type zeroEOFReader struct{ r *bytes.Reader }

func (z zeroEOFReader) Read(p []byte) (int, error) {
	n, err := z.r.Read(p)
	if errors.Is(err, io.EOF) {
		return 0, nil
	}
	return n, err
}

func TestEOFReaderEndsParseXauthorityOnZeroRead(t *testing.T) {
	done := make(chan error, 1)
	go func() {
		entries, err := ParseXauthority(eofReader{zeroEOFReader{bytes.NewReader(nil)}})
		if err == nil && len(entries) != 0 {
			err = errors.New("unexpected entries")
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ParseXauthority over an empty (0, nil) reader: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("ParseXauthority hung on a (0, nil) read at end of file")
	}
}

func TestEOFReaderKeepsDataAndErrors(t *testing.T) {
	got, err := io.ReadAll(eofReader{zeroEOFReader{bytes.NewReader([]byte("abc"))}})
	if err != nil || string(got) != "abc" {
		t.Fatalf("ReadAll = %q, %v; want abc, nil", got, err)
	}
	if n, err := (eofReader{zeroEOFReader{bytes.NewReader([]byte("x"))}}).Read(nil); n != 0 || err != nil {
		t.Fatalf("Read of an empty buffer = %d, %v; want 0, nil", n, err)
	}
}

// waitReadable must keep polling in short slices while the socket is quiet and
// return as soon as poll reports readiness: a blocking wait would hold the
// thread's P and hang the first stop-the-world (issue #566).
func TestWaitReadablePollsInSlicesUntilReady(t *testing.T) {
	calls := 0
	poll := func(fds []winescape.PollFd, d time.Duration) (int, error) {
		calls++
		if d != wineReadSlice {
			t.Fatalf("poll timeout = %v, want %v", d, wineReadSlice)
		}
		if len(fds) != 1 || fds[0].Fd != 7 || fds[0].Events != winescape.POLLIN {
			t.Fatalf("poll fds = %+v, want one POLLIN entry for fd 7", fds)
		}
		if calls < 3 {
			return 0, nil
		}
		return 1, nil
	}
	if err := waitReadable(7, poll); err != nil {
		t.Fatalf("waitReadable() = %v, want nil", err)
	}
	if calls != 3 {
		t.Fatalf("poll called %d times, want 3", calls)
	}
}

func TestWaitReadableReportsPollError(t *testing.T) {
	boom := errors.New("boom")
	poll := func([]winescape.PollFd, time.Duration) (int, error) { return -1, boom }
	if err := waitReadable(3, poll); !errors.Is(err, boom) {
		t.Fatalf("waitReadable() = %v, want %v", err, boom)
	}
}

// Where poll is unavailable the wait is skipped, so Read falls back to the
// blocking read it always did.
func TestWaitReadableSkipsWhenPollUnavailable(t *testing.T) {
	poll := func([]winescape.PollFd, time.Duration) (int, error) { return -1, winescape.ErrUnavailable }
	if err := waitReadable(3, poll); err != nil {
		t.Fatalf("waitReadable() = %v, want nil", err)
	}
}
