package main

// Receiving half of the kitty graphics protocol. The terminal emulator built
// into f4 accepts image data over APC escape sequences, so that a program
// running inside it can show pictures exactly as it would in kitty itself.
//
// This file owns the transmission layer only: parsing the control data,
// reassembling chunked uploads, decoding pixels and keeping the image store.
// Where an image ends up on the screen is the business of the placement
// layer, which plugs in through KittyDisplay.

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/unxed/vtui"
)

const (
	// kittyMaxImages bounds the store, so that a client which never deletes
	// anything cannot exhaust our memory.
	kittyMaxImages = 64

	// kittyMaxImageBytes caps one upload, before and after decompression.
	kittyMaxImageBytes = 64 << 20

	// kittyMaxPixels and kittyMaxSide cap the geometry a client may declare.
	kittyMaxPixels = 16 << 20
	kittyMaxSide   = 65535
)

// kittyCommand is one parsed graphics escape code: the comma separated
// key=value control data plus the base64 payload following the semicolon.
type kittyCommand struct {
	keys    map[byte]string
	Payload string
}

// parseKittyCommand takes the body of an APC sequence with the leading G
// already stripped.
func parseKittyCommand(s string) kittyCommand {
	cmd := kittyCommand{keys: make(map[byte]string)}
	control := s
	if i := strings.IndexByte(s, ';'); i >= 0 {
		control = s[:i]
		cmd.Payload = s[i+1:]
	}
	for _, part := range strings.Split(control, ",") {
		// Keys are single characters, so "k=v" is the only valid shape.
		if len(part) < 3 || part[1] != '=' {
			continue
		}
		cmd.keys[part[0]] = part[2:]
	}
	return cmd
}

// Has reports whether the key was present at all, which matters for the keys
// whose default is indistinguishable from a legal value.
func (c kittyCommand) Has(k byte) bool {
	_, ok := c.keys[k]
	return ok
}

func (c kittyCommand) Char(k byte, def byte) byte {
	if v, ok := c.keys[k]; ok && v != "" {
		return v[0]
	}
	return def
}

func (c kittyCommand) Int(k byte, def int) int {
	v, ok := c.keys[k]
	if !ok {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 32)
	if err != nil {
		return def
	}
	return int(n)
}

func (c kittyCommand) Uint32(k byte, def uint32) uint32 {
	v, ok := c.keys[k]
	if !ok {
		return def
	}
	n, err := strconv.ParseUint(v, 10, 32)
	if err != nil {
		return def
	}
	return uint32(n)
}

// kittyImage is one stored image. Placements refer to it by id.
type kittyImage struct {
	ID      uint32
	Number  uint32
	Surface *vtui.ImageSurface
}

// kittyTransfer accumulates a chunked upload. The control data of the first
// chunk describes the whole image, later chunks carry only m and q.
type kittyTransfer struct {
	cmd  kittyCommand
	data []byte
	tail string
}

// appendPayload decodes one base64 chunk. The protocol wants every chunk but
// the last to be a multiple of four characters long; keeping the remainder in
// tail costs nothing and makes a client that ignores that rule work anyway.
func (x *kittyTransfer) appendPayload(s string, last bool) error {
	s = x.tail + s
	if last {
		x.tail = ""
		if m := len(s) % 4; m != 0 {
			s += strings.Repeat("=", 4-m)
		}
	} else {
		n := len(s) - len(s)%4
		x.tail = s[n:]
		s = s[:n]
	}
	if s == "" {
		return nil
	}
	chunk, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return fmt.Errorf("the payload is not valid base64")
	}
	if len(x.data)+len(chunk) > kittyMaxImageBytes {
		return fmt.Errorf("the image is too large")
	}
	x.data = append(x.data, chunk...)
	return nil
}

// KittyDisplay is the placement half of the protocol. It is optional: a
// receiver without one still accepts and stores images, it simply has nowhere
// to show them. Implementations must not call back into KittyGraphics.
type KittyDisplay interface {
	// Put creates or replaces a placement of an already stored image. It
	// returns an empty string on success or a protocol error message.
	Put(img *kittyImage, cmd kittyCommand) string
	// Delete removes the placements selected by an a=d command and returns
	// the ids of the images left without any placement.
	Delete(cmd kittyCommand) []uint32
	// DropImage forgets every placement of an image that no longer exists.
	DropImage(id uint32)
}

// KittyGraphics receives the graphics escape codes of one terminal session.
type KittyGraphics struct {
	mu      sync.Mutex
	images  map[uint32]*kittyImage
	order   []uint32
	xfer    *kittyTransfer
	nextID  uint32
	display KittyDisplay
	write   func([]byte)
}

// NewKittyGraphics creates a receiver that answers the client through write.
func NewKittyGraphics(write func([]byte)) *KittyGraphics {
	return &KittyGraphics{
		images: make(map[uint32]*kittyImage),
		nextID: 1,
		write:  write,
	}
}

// SetDisplay attaches the placement layer.
func (kg *KittyGraphics) SetDisplay(d KittyDisplay) {
	kg.mu.Lock()
	defer kg.mu.Unlock()
	kg.display = d
}

// Image returns a stored image, or nil when the id is unknown.
func (kg *KittyGraphics) Image(id uint32) *kittyImage {
	kg.mu.Lock()
	defer kg.mu.Unlock()
	return kg.images[id]
}

// Len reports how many images are currently stored.
func (kg *KittyGraphics) Len() int {
	kg.mu.Lock()
	defer kg.mu.Unlock()
	return len(kg.images)
}

// Handle consumes one graphics escape code, without the leading G.
func (kg *KittyGraphics) Handle(s string) {
	cmd := parseKittyCommand(s)

	kg.mu.Lock()
	defer kg.mu.Unlock()

	if kg.xfer != nil && kg.isContinuation(cmd) {
		kg.continueTransfer(cmd)
		return
	}

	switch cmd.Char('a', 't') {
	case 't', 'T', 'q':
		kg.beginTransfer(cmd)
	case 'p':
		kg.put(cmd)
	case 'd':
		kg.remove(cmd)
	default:
		kg.reply(cmd, "EINVAL:unsupported action")
	}
}

// isContinuation reports whether a command belongs to the upload in flight.
// A chunk carries no action of its own, but some clients repeat it, so a
// command declaring no new geometry counts as a chunk as well.
func (kg *KittyGraphics) isContinuation(cmd kittyCommand) bool {
	if !cmd.Has('a') {
		return true
	}
	if cmd.Char('a', 't') != kg.xfer.cmd.Char('a', 't') {
		return false
	}
	return !cmd.Has('s') && !cmd.Has('v') && !cmd.Has('f')
}

func (kg *KittyGraphics) beginTransfer(cmd kittyCommand) {
	// A fresh transmission aborts an incomplete one, as the protocol says.
	kg.xfer = nil

	switch cmd.Int('f', 32) {
	case 24, 32, 100:
	default:
		kg.reply(cmd, "EINVAL:unsupported image format")
		return
	}
	switch cmd.Char('t', 'd') {
	case 'd', 'f', 't', 's':
	default:
		kg.reply(cmd, "EINVAL:unsupported transmission medium")
		return
	}
	if cmd.Has('i') && cmd.Has('I') {
		kg.reply(cmd, "EINVAL:i and I are mutually exclusive")
		return
	}

	xfer := &kittyTransfer{cmd: cmd}
	more := cmd.Int('m', 0) == 1
	if err := xfer.appendPayload(cmd.Payload, !more); err != nil {
		kg.reply(cmd, "EINVAL:"+err.Error())
		return
	}
	if more {
		kg.xfer = xfer
		return
	}
	kg.finishTransfer(xfer)
}

func (kg *KittyGraphics) continueTransfer(cmd kittyCommand) {
	x := kg.xfer
	more := cmd.Int('m', 0) == 1
	if err := x.appendPayload(cmd.Payload, !more); err != nil {
		kg.xfer = nil
		kg.reply(x.cmd, "EINVAL:"+err.Error())
		return
	}
	if more {
		return
	}
	kg.xfer = nil
	kg.finishTransfer(x)
}

func (kg *KittyGraphics) finishTransfer(x *kittyTransfer) {
	cmd := x.cmd
	data := x.data

	if medium := cmd.Char('t', 'd'); medium != 'd' {
		name := string(data)
		if medium == 's' {
			shm, err := kittyShmPath(name)
			if err != nil {
				kg.reply(cmd, "EBADF:"+err.Error())
				return
			}
			name = shm
		}
		var err error
		data, err = kittyReadFile(name, medium, cmd.Int('O', 0), cmd.Int('S', 0))
		if err != nil {
			kg.reply(cmd, "EBADF:"+err.Error())
			return
		}
	}
	if cmd.Char('o', 0) == 'z' {
		var err error
		data, err = kittyInflate(data)
		if err != nil {
			kg.reply(cmd, "EINVAL:"+err.Error())
			return
		}
	}

	surf, errMsg := kittySurface(cmd, data)
	if errMsg != "" {
		kg.reply(cmd, errMsg)
		return
	}

	// A query loads the image only to prove that it can be loaded.
	if cmd.Char('a', 't') == 'q' {
		kg.reply(cmd, "OK")
		return
	}

	img := kg.store(cmd, surf)
	msg := "OK"
	if cmd.Char('a', 't') == 'T' && kg.display != nil {
		if e := kg.display.Put(img, cmd); e != "" {
			msg = e
		}
	}
	kg.replyID(cmd, img.ID, msg)
}

func (kg *KittyGraphics) store(cmd kittyCommand, surf *vtui.ImageSurface) *kittyImage {
	id := cmd.Uint32('i', 0)
	if id == 0 {
		id = kg.allocID()
	}
	// Re-transmitting under an existing id replaces the image, and its old
	// placements go with it.
	if _, ok := kg.images[id]; ok && kg.display != nil {
		kg.display.DropImage(id)
	}
	img := &kittyImage{ID: id, Number: cmd.Uint32('I', 0), Surface: surf}
	kg.images[id] = img
	kg.touch(id)
	kg.evict()
	return img
}

// touch moves an image to the young end of the eviction order.
func (kg *KittyGraphics) touch(id uint32) {
	for i, v := range kg.order {
		if v == id {
			kg.order = append(kg.order[:i], kg.order[i+1:]...)
			break
		}
	}
	kg.order = append(kg.order, id)
}

func (kg *KittyGraphics) allocID() uint32 {
	for i := 0; i < 2*kittyMaxImages+2; i++ {
		if kg.nextID == 0 {
			kg.nextID = 1
		}
		id := kg.nextID
		kg.nextID++
		if _, taken := kg.images[id]; !taken {
			return id
		}
	}
	return kg.nextID
}

func (kg *KittyGraphics) evict() {
	for len(kg.order) > kittyMaxImages {
		kg.forget(kg.order[0])
	}
}

func (kg *KittyGraphics) forget(id uint32) {
	if _, ok := kg.images[id]; !ok {
		return
	}
	delete(kg.images, id)
	for i, v := range kg.order {
		if v == id {
			kg.order = append(kg.order[:i], kg.order[i+1:]...)
			break
		}
	}
	if kg.display != nil {
		kg.display.DropImage(id)
	}
}

func (kg *KittyGraphics) put(cmd kittyCommand) {
	img := kg.lookup(cmd)
	if img == nil {
		kg.reply(cmd, "ENOENT:there is no image with that id")
		return
	}
	msg := "OK"
	if kg.display == nil {
		msg = "ENOTSUP:this terminal cannot display images"
	} else if e := kg.display.Put(img, cmd); e != "" {
		msg = e
	}
	kg.touch(img.ID)
	kg.replyID(cmd, img.ID, msg)
}

// lookup resolves the i key, falling back to the newest image carrying the
// number given by I.
func (kg *KittyGraphics) lookup(cmd kittyCommand) *kittyImage {
	if id := cmd.Uint32('i', 0); id != 0 {
		return kg.images[id]
	}
	if num := cmd.Uint32('I', 0); num != 0 {
		return kg.byNumber(num)
	}
	return nil
}

func (kg *KittyGraphics) byNumber(num uint32) *kittyImage {
	for i := len(kg.order) - 1; i >= 0; i-- {
		if img, ok := kg.images[kg.order[i]]; ok && img.Number == num {
			return img
		}
	}
	return nil
}

// remove executes an a=d command. Which placements go away is decided by the
// placement layer, here we only free the image data that the uppercase forms
// of the d key ask us to free.
func (kg *KittyGraphics) remove(cmd kittyCommand) {
	// Any delete command aborts an upload in flight.
	kg.xfer = nil

	what := cmd.Char('d', 'a')
	free := what >= 'A' && what <= 'Z'

	var orphaned []uint32
	if kg.display != nil {
		orphaned = kg.display.Delete(cmd)
	}
	if !free {
		kg.reply(cmd, "OK")
		return
	}

	switch what {
	case 'I':
		if img := kg.lookup(cmd); img != nil && !cmd.Has('p') {
			kg.forget(img.ID)
		}
	case 'N':
		if img := kg.byNumber(cmd.Uint32('I', 0)); img != nil && !cmd.Has('p') {
			kg.forget(img.ID)
		}
	case 'R':
		lo, hi := cmd.Uint32('x', 0), cmd.Uint32('y', 0)
		for _, id := range append([]uint32(nil), kg.order...) {
			if id >= lo && id <= hi {
				kg.forget(id)
			}
		}
	default:
		for _, id := range orphaned {
			kg.forget(id)
		}
	}
	kg.reply(cmd, "OK")
}

func (kg *KittyGraphics) reply(cmd kittyCommand, msg string) {
	kg.replyID(cmd, cmd.Uint32('i', 0), msg)
}

// replyID answers the client. The id is passed separately because a client
// that used an image number gets back the id we assigned to it.
func (kg *KittyGraphics) replyID(cmd kittyCommand, id uint32, msg string) {
	if kg.write == nil {
		return
	}
	q := cmd.Int('q', 0)
	if q >= 2 || (q == 1 && msg == "OK") {
		return
	}
	num := cmd.Uint32('I', 0)
	if id == 0 && num == 0 {
		return
	}

	var sb strings.Builder
	sb.WriteString("\x1b_Gi=")
	sb.WriteString(strconv.FormatUint(uint64(id), 10))
	if num != 0 {
		sb.WriteString(",I=")
		sb.WriteString(strconv.FormatUint(uint64(num), 10))
	}
	if pid := cmd.Uint32('p', 0); pid != 0 {
		sb.WriteString(",p=")
		sb.WriteString(strconv.FormatUint(uint64(pid), 10))
	}
	sb.WriteByte(';')
	sb.WriteString(msg)
	sb.WriteString("\x1b\\")
	kg.write([]byte(sb.String()))
}

// kittySurface turns the received bytes into pixels.
func kittySurface(cmd kittyCommand, data []byte) (*vtui.ImageSurface, string) {
	if cmd.Int('f', 32) == 100 {
		surf, err := decodeImageWithStdlib(data)
		if err != nil || !surf.Valid() {
			return nil, "EINVAL:the image could not be decoded"
		}
		return surf, ""
	}

	w, h := cmd.Int('s', 0), cmd.Int('v', 0)
	if w <= 0 || h <= 0 {
		return nil, "EINVAL:the image dimensions are missing"
	}
	if w > kittyMaxSide || h > kittyMaxSide || w*h > kittyMaxPixels {
		return nil, "EINVAL:the image is too large"
	}

	bpp := 4
	if cmd.Int('f', 32) == 24 {
		bpp = 3
	}
	if len(data) < w*h*bpp {
		return nil, "EINVAL:the pixel data is truncated"
	}

	pix := make([]byte, w*h*4)
	if bpp == 4 {
		copy(pix, data[:w*h*4])
	} else {
		for i, j := 0, 0; j < len(pix); i, j = i+3, j+4 {
			pix[j] = data[i]
			pix[j+1] = data[i+1]
			pix[j+2] = data[i+2]
			pix[j+3] = 0xFF
		}
	}
	surf := vtui.NewImageSurfaceFromPix(w, h, w*4, pix)
	if surf == nil {
		return nil, "EINVAL:the pixel data is truncated"
	}
	return surf, ""
}

func kittyInflate(data []byte) ([]byte, error) {
	zr, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("the payload could not be decompressed")
	}
	defer zr.Close()
	out, err := io.ReadAll(io.LimitReader(zr, kittyMaxImageBytes+1))
	if err != nil {
		return nil, fmt.Errorf("the payload could not be decompressed")
	}
	if len(out) > kittyMaxImageBytes {
		return nil, fmt.Errorf("the image is too large")
	}
	return out, nil
}

// kittyShmDir is where POSIX shared memory objects appear in the file system
// on the systems that have them. It is a variable so that the tests need no
// /dev/shm of their own.
var kittyShmDir = "/dev/shm"

// kittyShmPath turns the name of a POSIX shared memory object into the path
// it has on disk. A name is a single component by definition — shm_open(3)
// allows nothing else — and letting a separator through here would turn t=s
// into a way of reading any file on the machine, which the checks meant for
// t=f would then have to catch on their own.
func kittyShmPath(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.TrimPrefix(name, "/")
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("the shared memory name is not valid")
	}
	if strings.ContainsAny(name, "/\\") {
		return "", fmt.Errorf("the shared memory name is not valid")
	}
	st, err := os.Stat(kittyShmDir)
	if err != nil || !st.IsDir() {
		return "", fmt.Errorf("this system has no shared memory objects")
	}
	return filepath.Join(kittyShmDir, name), nil
}

// kittyReadFile loads pixel data that a client left in the file system. Only
// plain files are read: a client must not be able to make the terminal open a
// device or a socket. Files sent as t=t are removed afterwards, but only from
// a temporary directory and only when the path is marked as belonging to the
// protocol.
func kittyReadFile(path string, medium byte, offset, size int) ([]byte, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("the file name is empty")
	}
	clean := filepath.Clean(path)
	if strings.HasPrefix(clean, "/proc/") || strings.HasPrefix(clean, "/sys/") {
		return nil, fmt.Errorf("this location may not be read")
	}
	if strings.HasPrefix(clean, "/dev/") && !strings.HasPrefix(clean, "/dev/shm/") {
		return nil, fmt.Errorf("this location may not be read")
	}

	// Stat follows symlinks on purpose: the protocol asks for it, and the
	// check below still rejects everything that is not a plain file.
	st, err := os.Stat(clean)
	if err != nil {
		return nil, fmt.Errorf("the file could not be opened")
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("only regular files can be read")
	}

	f, err := os.Open(clean)
	if err != nil {
		return nil, fmt.Errorf("the file could not be opened")
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(int64(offset), io.SeekStart); err != nil {
			return nil, fmt.Errorf("the file is shorter than the offset")
		}
	}
	limit := int64(kittyMaxImageBytes)
	if size > 0 && int64(size) < limit {
		limit = int64(size)
	}
	data, err := io.ReadAll(io.LimitReader(f, limit))
	if err != nil {
		return nil, fmt.Errorf("the file could not be read")
	}
	// The handle must drop before the unlink: Windows keeps a file deleted
	// while open visible until the last handle closes. (The deferred Close
	// then returns ErrClosed, which it ignores.)
	f.Close()
	// A shared memory object belongs to the terminal once it has been read:
	// the protocol makes us responsible for the shm_unlink. A t=t file is
	// the client saying we may have that one too.
	if medium == 's' || (medium == 't' && kittyIsTempPath(clean)) {
		os.Remove(clean)
	}
	return data, nil
}

// kittyIsTempPath guards the deletion of a t=t file: the path has to live in
// a temporary directory and to be marked as belonging to this protocol.
func kittyIsTempPath(path string) bool {
	if !strings.Contains(path, "tty-graphics-protocol") {
		return false
	}
	dirs := []string{os.TempDir(), "/tmp", "/dev/shm"}
	if tmp := os.Getenv("TMPDIR"); tmp != "" {
		dirs = append(dirs, tmp)
	}
	for _, d := range dirs {
		d = filepath.Clean(d)
		if d == "" || d == "/" || d == "." {
			continue
		}
		if strings.HasPrefix(path, d+string(filepath.Separator)) {
			return true
		}
	}
	return false
}
