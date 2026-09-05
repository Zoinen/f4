package fishplus

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestParseSizeLine(t *testing.T) {
	size, err := parseSizeLine([]string{"dd: warning: something", "S 4096"})
	if err != nil {
		t.Fatalf("parseSizeLine: %v", err)
	}
	if size != 4096 {
		t.Errorf("size = %d, want 4096", size)
	}
	if _, err := parseSizeLine([]string{"nothing here"}); err == nil {
		t.Error("a reply without a size line was accepted")
	}
	if _, err := parseSizeLine([]string{"S not a number"}); err == nil {
		t.Error("a bad size line was accepted")
	}
}

func TestFeaturesReadMode(t *testing.T) {
	feats, err := parseBanner("FISHPLUS 1 dd base64 mode:find read:ddbytes")
	if err != nil {
		t.Fatalf("parseBanner: %v", err)
	}
	if got := feats.ReadMode(); got != "ddbytes" {
		t.Errorf("ReadMode = %q, want %q", got, "ddbytes")
	}
	if got := feats.ListingMode(); got != "find" {
		t.Errorf("ListingMode = %q, want %q", got, "find")
	}
}

// TestReadRejectsOversizedFrame makes sure a helper claiming an absurd frame
// size cannot make the client allocate it.
func TestReadRejectsOversizedFrame(t *testing.T) {
	sess := newMockPeer(t, "ok FISHPLUS 1 dd base64 read:dd", func(w io.Writer, token string, req mockRequest) {
		if _, err := fmt.Fprintf(w, "S 10\n#%d\n", int64(MaxFrameLen)+1); err != nil {
			t.Errorf("write oversized frame header: %v", err)
			return
		}
		if _, err := fmt.Fprintf(w, ".%s %s ok\n", token, req.ID); err != nil && !errors.Is(err, io.ErrClosedPipe) {
			t.Errorf("write oversized frame terminator: %v", err)
		}
	}, 1)
	if err := sess.Handshake(context.Background()); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if _, err := sess.ExecPathData(context.Background(), "read", "/x", "0", "0"); err == nil {
		t.Error("an oversized data frame was accepted")
	}
	if !sess.Broken() {
		t.Error("the session should be broken after an oversized frame")
	}
}

// TestReadAgainstLocalShell drives every read backend the local host
// provides against real data, which is the only way to prove that the block
// size arithmetic inside the helper lands on the right bytes.
func TestReadAgainstLocalShell(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()

	dir := t.TempDir()
	// Keep the length odd. Plain dd must not fall back to bs=1 when a read
	// reaches EOF merely because the final byte count has no useful divisor.
	blob := make([]byte, 300001)
	rng := fillDeterministicBytes(t, 7, blob)
	blobPath := filepath.Join(dir, "a blob.bin")
	if err := os.WriteFile(blobPath, blob, 0600); err != nil {
		t.Fatal(err)
	}
	emptyPath := filepath.Join(dir, "empty")
	if err := os.WriteFile(emptyPath, nil, 0600); err != nil {
		t.Fatal(err)
	}

	if mode := c.Session().Features().ReadMode(); mode == "" {
		t.Fatal("handshake announced no read backend")
	}

	tried := 0
	for _, mode := range ReadModes {
		if err := c.SetReadMode(ctx, mode); err != nil {
			t.Logf("read backend %q unavailable here: %v", mode, err)
			continue
		}
		tried++
		t.Run(mode, func(t *testing.T) {
			whole, size, err := c.Read(ctx, blobPath, 0, 0)
			if err != nil {
				t.Fatalf("read whole file: %v", err)
			}
			if size != int64(len(blob)) {
				t.Errorf("size = %d, want %d", size, len(blob))
			}
			if !bytes.Equal(whole, blob) {
				t.Fatalf("whole file mismatch: got %d bytes", len(whole))
			}

			if mode == "cat" {
				// cat can only ever deliver a whole file, and the helper is
				// expected to say so rather than to send the wrong bytes.
				if _, _, err := c.Read(ctx, blobPath, 10, 10); err == nil {
					t.Error("a byte range was served by the cat backend")
				}
				return
			}

			ranges := [][2]int64{
				{0, 16}, {1, 1}, {65536, 65536}, {65535, 3}, {12345, 54321},
				{int64(len(blob)) - 1, 1}, {int64(len(blob)) - 10, 100},
				{int64(len(blob)), 10}, {0, int64(len(blob)) + 1000}, {100000, 0},
			}
			for _, r := range ranges {
				off, length := r[0], r[1]
				got, _, err := c.Read(ctx, blobPath, off, length)
				if err != nil {
					t.Fatalf("read(%d, %d): %v", off, length, err)
				}
				end := int64(len(blob))
				if length > 0 && off+length < end {
					end = off + length
				}
				var want []byte
				if off < int64(len(blob)) {
					want = blob[off:end]
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("read(%d, %d) = %d bytes, want %d", off, length, len(got), len(want))
				}
			}

			for i := 0; i < 20; i++ {
				off := rng.Int63n(int64(len(blob)))
				length := rng.Int63n(70000) + 1
				got, _, err := c.Read(ctx, blobPath, off, length)
				if err != nil {
					t.Fatalf("read(%d, %d): %v", off, length, err)
				}
				end := off + length
				if end > int64(len(blob)) {
					end = int64(len(blob))
				}
				if !bytes.Equal(got, blob[off:end]) {
					t.Fatalf("random read(%d, %d) returned the wrong bytes", off, length)
				}
			}

			if data, _, err := c.Read(ctx, emptyPath, 0, 100); err != nil || len(data) != 0 {
				t.Errorf("empty file: %d bytes, err %v", len(data), err)
			}
			if _, _, err := c.Read(ctx, dir, 0, 1); err == nil {
				t.Error("reading a directory succeeded")
			}
			if _, _, err := c.Read(ctx, filepath.Join(dir, "no such file"), 0, 1); err == nil {
				t.Error("reading a missing file succeeded")
			}
		})
	}
	if tried == 0 {
		t.Fatal("no read backend available on this host")
	}

	// The session has to be usable after all of that: a desynchronized
	// stream is the failure mode binary framing is most prone to.
	if err := c.Session().Noop(ctx); err != nil {
		t.Fatalf("session out of sync after the reads: %v", err)
	}
}

func TestFileReadAtAndCache(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	dir := t.TempDir()
	blob := make([]byte, 200000)
	fillDeterministicBytes(t, 11, blob)
	p := filepath.Join(dir, "handle.bin")
	if err := os.WriteFile(p, blob, 0600); err != nil {
		t.Fatal(err)
	}

	f, err := c.Open(ctx, p)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()
	f.SetChunkSize(4096)
	if f.Size() != int64(len(blob)) {
		t.Errorf("Size = %d, want %d", f.Size(), len(blob))
	}

	buf := make([]byte, 5000)
	if n, err := f.ReadAt(ctx, buf, 8000); err != nil || n != len(buf) {
		t.Fatalf("ReadAt = %d, %v", n, err)
	}
	if !bytes.Equal(buf, blob[8000:13000]) {
		t.Error("ReadAt crossing a chunk boundary returned the wrong bytes")
	}
	if n, err := f.ReadAt(ctx, buf, 8000); err != nil || n != len(buf) {
		t.Fatalf("cached ReadAt = %d, %v", n, err)
	}
	if !bytes.Equal(buf, blob[8000:13000]) {
		t.Error("the cached copy differs from the first read")
	}

	tail := make([]byte, 100)
	n, err := f.ReadAt(ctx, tail, int64(len(blob))-10)
	if n != 10 || err != io.EOF {
		t.Errorf("ReadAt past the end = %d, %v, want 10, io.EOF", n, err)
	}
	if !bytes.Equal(tail[:10], blob[len(blob)-10:]) {
		t.Error("the last ten bytes came back wrong")
	}

	seq := make([]byte, 1000)
	if _, err := f.Read(ctx, seq); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !bytes.Equal(seq, blob[:1000]) {
		t.Error("sequential Read did not start at zero")
	}
	if _, err := f.Read(ctx, seq); err != nil {
		t.Fatalf("second Read: %v", err)
	}
	if !bytes.Equal(seq, blob[1000:2000]) {
		t.Error("sequential Read did not advance")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seek: %v", err)
	}
	if _, err := f.Read(ctx, seq); err != nil || !bytes.Equal(seq, blob[:1000]) {
		t.Error("Seek did not rewind the handle")
	}

	all, err := c.ReadFile(ctx, p)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(all, blob) {
		t.Errorf("ReadFile returned %d bytes, want %d", len(all), len(blob))
	}

	if err := f.Close(); err != nil {
		t.Fatalf("close file before closed-state check: %v", err)
	}
	if _, err := f.ReadAt(ctx, buf, 0); err != ErrClosed {
		t.Errorf("ReadAt after Close = %v, want ErrClosed", err)
	}
}
