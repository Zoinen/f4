package fishplus

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestFeaturesWriteMode(t *testing.T) {
	feats, err := parseBanner("FISHPLUS 1 dd base64 truncate write:ddbytes read:dd")
	if err != nil {
		t.Fatalf("parseBanner: %v", err)
	}
	if got := feats.WriteMode(); got != "ddbytes" {
		t.Errorf("WriteMode = %q, want %q", got, "ddbytes")
	}
	if !feats.Has("truncate") {
		t.Error("the truncate feature was not picked up")
	}
	none, err := parseBanner("FISHPLUS 1 dd base64")
	if err != nil {
		t.Fatalf("parseBanner: %v", err)
	}
	if got := none.WriteMode(); got != "" {
		t.Errorf("WriteMode = %q, want empty", got)
	}
}

// TestWriteWithoutBackendSendsNothing makes sure the payload never reaches a
// host that cannot consume it: the bytes would be read as the next request.
func TestWriteWithoutBackendSendsNothing(t *testing.T) {
	sess := newMockPeer(t, "ok FISHPLUS 1 dd base64 read:dd", func(w io.Writer, token string, req mockRequest) {
		t.Errorf("the client sent %q although no write backend was announced", req.Cmd)
	})
	if err := sess.Handshake(context.Background()); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	c := NewClient(sess)
	if err := c.Write(context.Background(), "/tmp/x", 0, []byte("data")); err != ErrNoWrite {
		t.Errorf("Write = %v, want %v", err, ErrNoWrite)
	}
	if _, err := c.Create(context.Background(), "/tmp/x"); err != ErrNoWrite {
		t.Errorf("Create = %v, want %v", err, ErrNoWrite)
	}
}

// TestWriteEncodedRequest checks the shape of a b64 write on the wire: the
// arguments the helper parses, and the payload as a single line after the
// path, which is what makes it consumable by the remote shell alone.
func TestWriteEncodedRequest(t *testing.T) {
	payload := []byte("one\ntwo\x00three")
	var gotArgs []string
	var gotPath, gotPayload string
	sess := newMockPeer(t, "ok FISHPLUS 1 dd base64 write:b64", func(w io.Writer, token string, req mockRequest) {
		gotArgs = req.Args
		lines := req.decodePaths(t)
		gotPath = lines[0]
		raw, err := base64.StdEncoding.DecodeString(req.Paths[1])
		if err != nil {
			t.Errorf("payload line is not base64: %v", err)
		}
		gotPayload = string(raw)
		if _, err := fmt.Fprintf(w, "D\n.%s %s ok\n", token, req.ID); err != nil {
			t.Errorf("write base64 response: %v", err)
		}
	}, 2)
	if err := sess.Handshake(context.Background()); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	c := NewClient(sess)
	if c.WriteMode() != "b64" {
		t.Fatalf("write mode = %q", c.WriteMode())
	}
	if c.WriteChunk() != EncodedWriteChunk {
		t.Errorf("chunk = %d, want %d", c.WriteChunk(), EncodedWriteChunk)
	}
	if err := c.Write(context.Background(), "/a dir/a file", 512, payload); err != nil {
		t.Fatalf("write: %v", err)
	}
	want := []string{"512", strconv.Itoa(len(payload)), "b64"}
	if len(gotArgs) != 3 || gotArgs[0] != want[0] || gotArgs[1] != want[1] || gotArgs[2] != want[2] {
		t.Errorf("args = %v, want %v", gotArgs, want)
	}
	if gotPath != "/a dir/a file" {
		t.Errorf("path = %q", gotPath)
	}
	if gotPayload != string(payload) {
		t.Errorf("payload = %q, want %q", gotPayload, payload)
	}
}

// TestWriteBreaksSessionWithoutDrainMarker is the whole point of the "D"
// line: a failure the helper caught before touching the payload keeps the
// session usable, one that happened halfway through it does not.
func TestWriteBreaksSessionWithoutDrainMarker(t *testing.T) {
	for _, tc := range []struct {
		name   string
		lines  string
		broken bool
	}{
		{"drained", "D\n", false},
		{"halfway", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sess := newMockPeer(t, "ok FISHPLUS 1 dd base64 write:b64", func(w io.Writer, token string, req mockRequest) {
				if _, err := fmt.Fprintf(w, "%s.%s %s err disk on fire\n", tc.lines, token, req.ID); err != nil {
					t.Errorf("write failure response: %v", err)
				}
			}, 2)
			if err := sess.Handshake(context.Background()); err != nil {
				t.Fatalf("handshake: %v", err)
			}
			c := NewClient(sess)
			if err := c.Write(context.Background(), "/tmp/x", 0, []byte("data")); err == nil {
				t.Fatal("a failed write was reported as successful")
			}
			if sess.Broken() != tc.broken {
				t.Errorf("Broken = %v, want %v", sess.Broken(), tc.broken)
			}
		})
	}
}

func TestWriteRejectsBadArguments(t *testing.T) {
	sess := newMockPeer(t, "ok FISHPLUS 1 dd base64 write:ddbytes", func(w io.Writer, token string, req mockRequest) {
		t.Errorf("a rejected write reached the wire as %q", req.Cmd)
	})
	if err := sess.Handshake(context.Background()); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	c := NewClient(sess)
	ctx := context.Background()
	if err := c.Write(ctx, "/tmp/x", -1, []byte("x")); err == nil {
		t.Error("a negative offset was accepted")
	}
	if err := c.Write(ctx, "/tmp/x", 0, make([]byte, MaxWriteLen+1)); err == nil {
		t.Error("an oversized payload was accepted")
	}
	if err := c.Truncate(ctx, "/tmp/x", -1); err == nil {
		t.Error("a negative size was accepted")
	}
}

// TestWriteAgainstLocalShell drives every write backend the local host
// provides. Only a real shell can show whether the helper consumes exactly
// the announced number of bytes, which is what the rest of the session
// depends on.
func TestWriteAgainstLocalShell(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	root := t.TempDir()

	if c.Session().Features().WriteMode() == "" {
		t.Fatal("handshake announced no write backend")
	}

	blob := make([]byte, 200000)
	fillDeterministicBytes(t, 11, blob)

	tried := 0
	for _, mode := range WriteModes {
		if err := c.SetWriteMode(ctx, mode); err != nil {
			t.Logf("write backend %q unavailable here: %v", mode, err)
			continue
		}
		tried++
		t.Run(mode, func(t *testing.T) {
			if c.WriteMode() != mode {
				t.Fatalf("client write mode = %q, want %q", c.WriteMode(), mode)
			}
			file := filepath.Join(root, "a file "+mode+".bin")

			if err := c.WriteFile(ctx, file, blob); err != nil {
				t.Fatalf("write file: %v", err)
			}
			got, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, blob) {
				t.Fatalf("file mismatch: got %d bytes, want %d", len(got), len(blob))
			}

			patches := []struct {
				offset  int64
				length  int
				pattern byte
			}{
				{0, 1, 0}, {1, 1, 1}, {65535, 3, 255}, {65536, 65536, 0}, {12345, 4321, 57}, {199999, 1, 63},
			}
			want := append([]byte(nil), blob...)
			for _, p := range patches {
				data := make([]byte, p.length)
				var indexByte byte
				for i := range data {
					data[i] = p.pattern ^ indexByte
					indexByte++
				}
				if err := c.Write(ctx, file, p.offset, data); err != nil {
					t.Fatalf("write %d+%d: %v", p.offset, p.length, err)
				}
				copy(want[p.offset:], data)
			}
			if got, err = os.ReadFile(file); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				for i := range want {
					if i < len(got) && got[i] != want[i] {
						t.Fatalf("patched file differs at byte %d", i)
					}
				}
				t.Fatalf("patched file has %d bytes, want %d", len(got), len(want))
			}

			tail := []byte("the end")
			if err := c.Write(ctx, file, int64(len(want))+4096, tail); err != nil {
				t.Fatalf("write past the end: %v", err)
			}
			if got, err = os.ReadFile(file); err != nil {
				t.Fatal(err)
			}
			if len(got) != len(want)+4096+len(tail) {
				t.Errorf("size after a sparse write = %d, want %d", len(got), len(want)+4096+len(tail))
			}
			if !bytes.Equal(got[len(want)+4096:], tail) {
				t.Errorf("sparse write landed on the wrong bytes")
			}
			if !bytes.Equal(got[len(want):len(want)+4096], make([]byte, 4096)) {
				t.Errorf("the gap of a sparse write is not zero filled")
			}

			if err := c.Write(ctx, file, 0, nil); err != nil {
				t.Errorf("empty write: %v", err)
			}
		})
	}
	if tried == 0 {
		t.Fatal("no write backend could be selected at all")
	}
}

// TestWriterAgainstLocalShell exercises the buffering handle, including a
// chunk size small enough to make the sequence of writes overlap several
// requests.
func TestWriterAgainstLocalShell(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	file := filepath.Join(t.TempDir(), "streamed")

	w, err := c.Create(ctx, file)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	w.chunk = 1000
	var want []byte
	for i := 0; i < 50; i++ {
		line := []byte(fmt.Sprintf("line %d of a file that is written in pieces\n", i))
		n, err := w.Write(line)
		if err != nil {
			t.Fatalf("write: %v", err)
		}
		if n != len(line) {
			t.Fatalf("Write returned %d, want %d", n, len(line))
		}
		want = append(want, line...)
	}
	if err := w.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if got := w.Offset(); got != int64(len(want)) {
		t.Errorf("offset = %d, want %d", got, len(want))
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Errorf("closing twice: %v", err)
	}
	if _, err := w.Write([]byte("x")); err != ErrClosed {
		t.Errorf("write after close = %v, want %v", err, ErrClosed)
	}
	got, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("streamed file mismatch: got %d bytes, want %d", len(got), len(want))
	}

	if err := c.WriteFile(ctx, file, []byte("short")); err != nil {
		t.Fatalf("rewrite: %v", err)
	}
	if got, err = os.ReadFile(file); err != nil {
		t.Fatal(err)
	}
	if string(got) != "short" {
		t.Errorf("rewritten file = %q", got)
	}
}

// TestWriteErrorsKeepSessionUsable is the check that matters most for a
// protocol where the payload has no terminator of its own: every refusal the
// helper can foresee must still take the bytes off the wire.
func TestWriteErrorsKeepSessionUsable(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	root := t.TempDir()
	dir := filepath.Join(root, "a directory")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("payload "), 1000)

	for _, mode := range WriteModes {
		if err := c.SetWriteMode(ctx, mode); err != nil {
			continue
		}
		t.Run(mode, func(t *testing.T) {
			for _, tc := range []struct {
				name string
				path string
			}{
				{"directory", dir},
				{"relative", "not/absolute"},
				{"dotdot", root + "/../escape"},
			} {
				if err := c.Write(ctx, tc.path, 0, payload); err == nil {
					t.Errorf("%s: the write was accepted", tc.name)
				}
				if c.Session().Broken() {
					t.Fatalf("%s: the session broke over a refusal", tc.name)
				}
				if got, err := c.Session().Ping(ctx, "still here"); err != nil || got != "still here" {
					t.Fatalf("%s: session out of sync: %q %v", tc.name, got, err)
				}
			}
		})
	}
}

func TestTruncateAgainstLocalShell(t *testing.T) {
	c := newLocalShellClient(t)
	ctx := context.Background()
	root := t.TempDir()
	file := filepath.Join(root, "sized")

	if err := c.Truncate(ctx, file, 0); err != nil {
		t.Fatalf("truncate a missing file: %v", err)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatalf("truncate did not create the file: %v", err)
	}
	if info.Size() != 0 {
		t.Errorf("size = %d, want 0", info.Size())
	}

	if err := os.WriteFile(file, bytes.Repeat([]byte("x"), 100), 0600); err != nil {
		t.Fatal(err)
	}
	if c.Session().Features().Has("truncate") {
		if err := c.Truncate(ctx, file, 10); err != nil {
			t.Fatalf("truncate to 10: %v", err)
		}
		if info, err = os.Stat(file); err != nil || info.Size() != 10 {
			t.Errorf("size = %v (%v), want 10", info.Size(), err)
		}
	} else {
		if err := c.Truncate(ctx, file, 10); err == nil {
			t.Error("a non-zero truncate was accepted without the truncate utility")
		}
	}
	if err := c.Truncate(ctx, file, 0); err != nil {
		t.Fatalf("truncate to 0: %v", err)
	}
	if info, err = os.Stat(file); err != nil || info.Size() != 0 {
		t.Errorf("size = %v (%v), want 0", info.Size(), err)
	}
	if err := c.Truncate(ctx, root, 0); err == nil {
		t.Error("a directory was truncated")
	}
	if got, err := c.Session().Ping(ctx, "alive"); err != nil || got != "alive" {
		t.Fatalf("session out of sync after truncate: %q %v", got, err)
	}
}
