package main

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// kittyEnv wires a terminal view to a parser and a fake pty, which is how the
// graphics receiver sees the world in production.
type kittyEnv struct {
	tv  *TerminalView
	p   *AnsiParser
	pty *mockPty
}

func newKittyEnv(t *testing.T) *kittyEnv {
	t.Helper()
	tv := NewTerminalView(80, 24)
	pty := &mockPty{}
	tv.pty = pty
	return &kittyEnv{tv: tv, p: NewAnsiParser(tv, pty), pty: pty}
}

// send feeds one graphics escape code to the parser.
func (e *kittyEnv) send(control, payload string) {
	e.p.Process([]byte("\x1b_G" + control + ";" + payload + "\x1b\\"))
}

func kittyB64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func TestKittyParseCommand(t *testing.T) {
	cmd := parseKittyCommand("a=T,f=24,s=10,v=20,z=-3;SGVsbG8=")
	if cmd.Char('a', 't') != 'T' {
		t.Errorf("action: expected T, got %c", cmd.Char('a', 't'))
	}
	if cmd.Int('f', 32) != 24 || cmd.Int('s', 0) != 10 || cmd.Int('v', 0) != 20 {
		t.Errorf("format/geometry parsed wrong: %v", cmd.keys)
	}
	if cmd.Int('z', 0) != -3 {
		t.Errorf("z: expected -3, got %d", cmd.Int('z', 0))
	}
	if cmd.Payload != "SGVsbG8=" {
		t.Errorf("payload: got %q", cmd.Payload)
	}
	if cmd.Has('i') {
		t.Error("i must be reported as absent")
	}
	if cmd.Uint32('i', 7) != 7 {
		t.Error("a missing key must fall back to the default")
	}

	// A code without a payload is still a valid command.
	cmd = parseKittyCommand("a=d,d=I,i=5")
	if cmd.Char('d', 'a') != 'I' || cmd.Uint32('i', 0) != 5 || cmd.Payload != "" {
		t.Errorf("delete command parsed wrong: %v", cmd.keys)
	}
}

func TestKittyQueryDoesNotStore(t *testing.T) {
	e := newKittyEnv(t)
	e.send("i=31,a=q,t=d,f=24,s=1,v=1", kittyB64([]byte{1, 2, 3}))

	if got, want := e.pty.String(), "\x1b_Gi=31;OK\x1b\\"; got != want {
		t.Errorf("query answer: expected %q, got %q", want, got)
	}
	if e.tv.kitty == nil || e.tv.kitty.Len() != 0 {
		t.Error("a query must not store the image")
	}
}

func TestKittyTransmitChunkedRGBA(t *testing.T) {
	e := newKittyEnv(t)

	// Two pixels: opaque red, half transparent blue.
	pix := []byte{0xFF, 0x00, 0x00, 0xFF, 0x00, 0x00, 0xFF, 0x80}
	payload := kittyB64(pix)
	half := len(payload) / 2

	e.send("a=t,i=9,f=32,s=2,v=1,m=1", payload[:half])
	if e.tv.kitty.Len() != 0 {
		t.Fatal("an incomplete upload must not be stored")
	}
	e.send("m=0", payload[half:])

	img := e.tv.kitty.Image(9)
	if img == nil {
		t.Fatal("the image was not stored")
	}
	if img.Surface.Width != 2 || img.Surface.Height != 1 {
		t.Fatalf("geometry: got %dx%d", img.Surface.Width, img.Surface.Height)
	}
	if r, g, b, a := img.Surface.PixelAt(0, 0); r != 0xFF || g != 0 || b != 0 || a != 0xFF {
		t.Errorf("first pixel: got %d %d %d %d", r, g, b, a)
	}
	if r, g, b, a := img.Surface.PixelAt(1, 0); r != 0 || g != 0 || b != 0xFF || a != 0x80 {
		t.Errorf("second pixel: got %d %d %d %d", r, g, b, a)
	}
	if got, want := e.pty.String(), "\x1b_Gi=9;OK\x1b\\"; got != want {
		t.Errorf("answer: expected %q, got %q", want, got)
	}
}

func TestKittyTransmitRGBFillsAlpha(t *testing.T) {
	e := newKittyEnv(t)
	e.send("a=T,i=3,f=24,s=1,v=2", kittyB64([]byte{10, 20, 30, 40, 50, 60}))

	img := e.tv.kitty.Image(3)
	if img == nil {
		t.Fatal("the image was not stored")
	}
	if _, _, _, a := img.Surface.PixelAt(0, 0); a != 0xFF {
		t.Errorf("alpha: expected 255, got %d", a)
	}
	if r, g, b, _ := img.Surface.PixelAt(0, 1); r != 40 || g != 50 || b != 60 {
		t.Errorf("second row: got %d %d %d", r, g, b)
	}
}

func TestKittyTransmitPNG(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	src.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		t.Fatalf("png.Encode: %v", err)
	}

	e := newKittyEnv(t)
	e.send("a=t,i=11,f=100", kittyB64(buf.Bytes()))

	img := e.tv.kitty.Image(11)
	if img == nil {
		t.Fatal("the image was not stored")
	}
	if img.Surface.Width != 3 || img.Surface.Height != 2 {
		t.Errorf("geometry: got %dx%d", img.Surface.Width, img.Surface.Height)
	}
}

func TestKittyCompressedPayload(t *testing.T) {
	pix := make([]byte, 4*4*4)
	for i := range pix {
		pix[i] = byte(i)
	}
	var buf bytes.Buffer
	zw := zlib.NewWriter(&buf)
	if _, err := zw.Write(pix); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	zw.Close()

	e := newKittyEnv(t)
	e.send("a=t,i=12,f=32,s=4,v=4,o=z", kittyB64(buf.Bytes()))

	img := e.tv.kitty.Image(12)
	if img == nil {
		t.Fatal("the compressed image was not stored")
	}
	if r, g, b, a := img.Surface.PixelAt(1, 0); r != 4 || g != 5 || b != 6 || a != 7 {
		t.Errorf("pixel: got %d %d %d %d", r, g, b, a)
	}
}

func TestKittyFileMedium(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "pixels.bin")
	if err := os.WriteFile(path, []byte{9, 9, 1, 2, 3}, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	e := newKittyEnv(t)
	// Skip the two leading bytes through the offset key.
	e.send("a=t,i=13,f=24,s=1,v=1,t=f,O=2", kittyB64([]byte(path)))

	img := e.tv.kitty.Image(13)
	if img == nil {
		t.Fatal("the image was not read from the file")
	}
	if r, g, b, _ := img.Surface.PixelAt(0, 0); r != 1 || g != 2 || b != 3 {
		t.Errorf("pixel: got %d %d %d", r, g, b)
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("a t=f file must not be deleted")
	}
}

// kittyFakeShm points the shared memory lookup at a directory the test owns,
// so that nothing here depends on the machine having /dev/shm.
func kittyFakeShm(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	saved := kittyShmDir
	kittyShmDir = dir
	t.Cleanup(func() { kittyShmDir = saved })
	return dir
}

func TestKittyShmPath(t *testing.T) {
	dir := kittyFakeShm(t)

	got, err := kittyShmPath("/picture")
	if err != nil || got != filepath.Join(dir, "picture") {
		t.Fatalf("got %q %v", got, err)
	}
	if got, err := kittyShmPath("picture"); err != nil || got != filepath.Join(dir, "picture") {
		t.Fatalf("a name without the leading slash: got %q %v", got, err)
	}
	for _, bad := range []string{"", "   ", "/", ".", "..", "a/b", "..\\windows", "/../etc/passwd"} {
		if _, err := kittyShmPath(bad); err == nil {
			t.Errorf("%q must be refused", bad)
		}
	}

	kittyShmDir = filepath.Join(dir, "no-such-place")
	if _, err := kittyShmPath("picture"); err == nil {
		t.Error("a system without shared memory objects must say so")
	}
}

func TestKittySharedMemoryMedium(t *testing.T) {
	dir := kittyFakeShm(t)
	path := filepath.Join(dir, "f4-shm-picture")
	if err := os.WriteFile(path, []byte{4, 5, 6}, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	e := newKittyEnv(t)
	e.send("a=t,i=21,f=24,s=1,v=1,t=s", kittyB64([]byte("/f4-shm-picture")))

	img := e.tv.kitty.Image(21)
	if img == nil {
		t.Fatalf("the image was not read from shared memory, answer was %q", e.pty.String())
	}
	if r, g, b, _ := img.Surface.PixelAt(0, 0); r != 4 || g != 5 || b != 6 {
		t.Errorf("pixel: got %d %d %d", r, g, b)
	}
	// The protocol makes the terminal responsible for the shm_unlink.
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("a t=s object must be unlinked once it has been read")
	}
}

func TestKittySharedMemoryRefusesAPath(t *testing.T) {
	kittyFakeShm(t)

	e := newKittyEnv(t)
	e.send("a=t,i=22,f=24,s=1,v=1,t=s", kittyB64([]byte("../../etc/passwd")))

	if !strings.HasPrefix(e.pty.String(), "\x1b_Gi=22;EBADF:") {
		t.Errorf("expected an EBADF answer, got %q", e.pty.String())
	}
	if e.tv.kitty.Len() != 0 {
		t.Error("nothing must be stored")
	}
}

func TestKittyRejectsSpecialFiles(t *testing.T) {
	e := newKittyEnv(t)
	e.send("a=t,i=14,f=24,s=1,v=1,t=f", kittyB64([]byte("/proc/self/mem")))

	if !strings.HasPrefix(e.pty.String(), "\x1b_Gi=14;EBADF:") {
		t.Errorf("expected an EBADF answer, got %q", e.pty.String())
	}
	if e.tv.kitty.Len() != 0 {
		t.Error("nothing must be stored")
	}
}

func TestKittyErrorsAndSuppression(t *testing.T) {
	e := newKittyEnv(t)

	e.send("a=t,i=15,f=7,s=1,v=1", "")
	if !strings.HasPrefix(e.pty.String(), "\x1b_Gi=15;EINVAL:") {
		t.Errorf("unsupported format: got %q", e.pty.String())
	}

	// q=1 silences success only, q=2 silences everything.
	e.pty.Reset()
	e.send("a=t,i=16,f=32,s=1,v=1,q=1", kittyB64([]byte{1, 2, 3, 4}))
	if e.pty.String() != "" {
		t.Errorf("q=1 must silence OK, got %q", e.pty.String())
	}

	e.pty.Reset()
	e.send("a=t,i=17,f=7,s=1,v=1,q=2", "")
	if e.pty.String() != "" {
		t.Errorf("q=2 must silence errors, got %q", e.pty.String())
	}

	// Without an id there is nobody to answer to.
	e.pty.Reset()
	e.send("a=t,f=7,s=1,v=1", "")
	if e.pty.String() != "" {
		t.Errorf("an anonymous command must not be answered, got %q", e.pty.String())
	}
}

func TestKittyPutAndDelete(t *testing.T) {
	e := newKittyEnv(t)
	e.send("a=t,i=21,f=32,s=1,v=1", kittyB64([]byte{1, 2, 3, 4}))

	// The placement layer turns a put into a picture on the grid.
	e.pty.Reset()
	e.send("a=p,i=21,p=5", "")
	if got, want := e.pty.String(), "\x1b_Gi=21,p=5;OK\x1b\\"; got != want {
		t.Errorf("put answer: expected %q, got %q", want, got)
	}
	if len(e.tv.images) != 1 || e.tv.images[0].Placement != 5 {
		t.Errorf("a put must create a placement: %+v", e.tv.images)
	}

	e.pty.Reset()
	e.send("a=p,i=99", "")
	if !strings.HasPrefix(e.pty.String(), "\x1b_Gi=99;ENOENT:") {
		t.Errorf("unknown id: got %q", e.pty.String())
	}

	// The lowercase form keeps the data, the uppercase form frees it.
	e.send("a=d,d=i,i=21", "")
	if e.tv.kitty.Image(21) == nil {
		t.Error("d=i must keep the image data")
	}
	e.send("a=d,d=I,i=21", "")
	if e.tv.kitty.Image(21) != nil {
		t.Error("d=I must free the image data")
	}
}

func TestKittyImageNumberAndEviction(t *testing.T) {
	e := newKittyEnv(t)
	e.send("a=t,I=42,f=32,s=1,v=1", kittyB64([]byte{1, 2, 3, 4}))

	answer := e.pty.String()
	if !strings.HasPrefix(answer, "\x1b_Gi=") || !strings.Contains(answer, ",I=42;OK") {
		t.Fatalf("the assigned id must be reported back, got %q", answer)
	}
	if e.tv.kitty.Len() != 1 {
		t.Fatal("the image was not stored")
	}

	for i := 0; i < kittyMaxImages+8; i++ {
		e.send("a=t,f=32,s=1,v=1", kittyB64([]byte{1, 2, 3, 4}))
	}
	if n := e.tv.kitty.Len(); n > kittyMaxImages {
		t.Errorf("the store grew to %d images", n)
	}
}
