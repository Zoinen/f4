package terminal

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/internal/terminal/far2ldnd"
)

// The terminal side of the DnD protocol, end to end at the protocol level:
// request bytes go through the ANSI parser the way a program inside f4 writes
// them, and the reply bytes the program would read are checked.

var (
	dndBindingA = far2ldnd.ID{0xa0, 0xa1, 0xa2, 0xa3, 0xa4, 0xa5, 0xa6, 0xa7, 0xa8, 0xa9, 0xaa, 0xab, 0xac, 0xad, 0xae, 0xaf}
	dndBindingB = far2ldnd.ID{0xb0, 0xb1, 0xb2, 0xb3, 0xb4, 0xb5, 0xb6, 0xb7, 0xb8, 0xb9, 0xba, 0xbb, 0xbc, 0xbd, 0xbe, 0xbf}
	// The offer of the vectors in § 14.
	dndSpecOffer = far2ldnd.ID{0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f}
)

type dndEnv struct {
	tv  *TerminalView
	p   *AnsiParser
	pty *mockPty

	mu      sync.Mutex
	now     time.Time
	nextID  byte
	fixedID *far2ldnd.ID
}

func newDNDEnv(t *testing.T) *dndEnv {
	t.Helper()
	tv := NewTerminalView(80, 24)
	pty := &mockPty{}
	tv.Pty = pty
	e := &dndEnv{tv: tv, p: NewAnsiParser(tv, pty), pty: pty, now: time.Unix(1_000_000, 0)}
	tv.dnd.now = func() time.Time {
		e.mu.Lock()
		defer e.mu.Unlock()
		return e.now
	}
	tv.dnd.newID = func() (far2ldnd.ID, error) {
		e.mu.Lock()
		defer e.mu.Unlock()
		if e.fixedID != nil {
			return *e.fixedID, nil
		}
		e.nextID++
		return far2ldnd.ID{0xee, e.nextID}, nil
	}
	return e
}

func (e *dndEnv) advance(d time.Duration) {
	e.mu.Lock()
	e.now = e.now.Add(d)
	e.mu.Unlock()
}

// send feeds raw bytes to the parser after clearing what the program has
// read so far.
func (e *dndEnv) send(apc string) {
	e.pty.Reset()
	e.p.Process([]byte(apc))
}

// wait returns the n whole frames the terminal wrote, failing if they do not
// come or if more come.
func (e *dndEnv) wait(t *testing.T, n int) []string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var out string
	for time.Now().Before(deadline) {
		out = e.pty.String()
		if strings.Count(out, "\x1b_") >= n && strings.HasSuffix(out, "\x07") {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	frames := strings.SplitAfter(out, "\x07")
	if frames[len(frames)-1] == "" {
		frames = frames[:len(frames)-1]
	}
	if len(frames) != n {
		t.Fatalf("want %d frame(s), got %d: %q", n, len(frames), out)
	}
	return frames
}

// request sends one request and returns the one reply frame.
func (e *dndEnv) request(t *testing.T, rid uint8, q far2ldnd.Request) string {
	t.Helper()
	stack, err := far2ldnd.EncodeRequest(rid, q)
	if err != nil {
		t.Fatalf("encode %T: %v", q, err)
	}
	return e.raw(t, stack)
}

// raw sends a request stack as it is and returns the one reply frame.
func (e *dndEnv) raw(t *testing.T, stack []byte) string {
	t.Helper()
	e.send(far2ldnd.Frame(far2ldnd.FrameRequest, stack, far2ldnd.BEL))
	return e.wait(t, 1)[0]
}

// silent sends a request that must not be answered, then proves it was
// served by a probe that must be answered alone.
func (e *dndEnv) silent(t *testing.T, stack []byte) {
	t.Helper()
	probe, err := far2ldnd.EncodeRequest(250, &far2ldnd.CloseRequest{Offer: far2ldnd.ID{0xff}})
	if err != nil {
		t.Fatal(err)
	}
	e.send(far2ldnd.Frame(far2ldnd.FrameRequest, stack, far2ldnd.BEL) + far2ldnd.Frame(far2ldnd.FrameRequest, probe, far2ldnd.BEL))
	if got := e.wait(t, 1)[0]; replyRID(t, got) != 250 {
		t.Fatalf("the request was answered: %q", got)
	}
}

func (e *dndEnv) bind(t *testing.T, binding far2ldnd.ID, frame, chunk uint32, wanted uint32) far2ldnd.BindReply {
	t.Helper()
	q := &far2ldnd.BindRequest{Version: 1, Enable: true, Binding: binding, MaxFrame: frame, MaxChunk: chunk, Window: 4, WantedFeatures: wanted}
	f := mustReply(t, e.request(t, 1, q))
	b, err := far2ldnd.DecodeBindReply(f, q)
	if err != nil {
		t.Fatalf("BIND reply: %v", err)
	}
	return b
}

func (e *dndEnv) offer(t *testing.T, src DNDSource) far2ldnd.ID {
	t.Helper()
	id, err := e.tv.OfferDrop(src, 1, 1, 0, false)
	if err != nil {
		t.Fatalf("OfferDrop: %v", err)
	}
	return id
}

func parseReply(t *testing.T, frame string) (far2ldnd.ReplyFrame, error) {
	t.Helper()
	k, stack, term, err := far2ldnd.ParseFrame(frame)
	if err != nil || k != far2ldnd.FrameReply || term != far2ldnd.BEL {
		t.Fatalf("not a reply frame: %q (%v)", frame, err)
	}
	return far2ldnd.DecodeReply(stack)
}

func replyRID(t *testing.T, frame string) uint8 {
	t.Helper()
	f, _ := parseReply(t, frame)
	return f.RID
}

func mustReply(t *testing.T, frame string) far2ldnd.ReplyFrame {
	t.Helper()
	f, err := parseReply(t, frame)
	if err != nil {
		t.Fatalf("reply %q: %v", frame, err)
	}
	return f
}

func statusOf(t *testing.T, frame string) far2ldnd.Status {
	t.Helper()
	_, err := parseReply(t, frame)
	var se *far2ldnd.StatusError
	switch {
	case errors.As(err, &se):
		if len(se.Message) == 0 {
			t.Errorf("error reply without diagnostic: %q", frame)
		}
		return se.Status
	case err != nil:
		t.Fatalf("reply %q: %v", frame, err)
	}
	return far2ldnd.StatusOK
}

func wantStatus(t *testing.T, frame string, want far2ldnd.Status) {
	t.Helper()
	if got := statusOf(t, frame); got != want {
		t.Fatalf("status %d, want %d: %q", got, want, frame)
	}
}

func replyFrame(t *testing.T, rid uint8, body far2ldnd.Reply) string {
	t.Helper()
	stack, err := far2ldnd.EncodeReply(rid, body)
	if err != nil {
		t.Fatal(err)
	}
	return far2ldnd.Frame(far2ldnd.FrameReply, stack, far2ldnd.BEL)
}

// popOrder lays fields given in pop order out the way StackSerializer does,
// for requests the codec refuses to build.
func popOrder(fields ...[]byte) []byte {
	var out []byte
	for i := len(fields) - 1; i >= 0; i-- {
		out = append(out, fields[i]...)
	}
	return out
}

func le16(v uint16) []byte { return binary.LittleEndian.AppendUint16(nil, v) }
func le32(v uint32) []byte { return binary.LittleEndian.AppendUint32(nil, v) }
func le64(v uint64) []byte { return binary.LittleEndian.AppendUint64(nil, v) }

// memSource is the test drag source: files in memory.
type memSource struct {
	entries []far2ldnd.Entry
	data    map[uint64][]byte
	err     error
	started chan struct{} // ReadAt signals here, when set
	block   chan struct{} // and then waits here

	mu      sync.Mutex
	closed  int
	reading bool
}

func (m *memSource) Entries() []far2ldnd.Entry { return m.entries }

func (m *memSource) ReadAt(id uint64, p []byte, off uint64) (int, bool, error) {
	m.mu.Lock()
	m.reading = true
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.reading = false
		m.mu.Unlock()
	}()
	if m.started != nil {
		m.started <- struct{}{}
	}
	if m.block != nil {
		<-m.block
	}
	if m.err != nil {
		return 0, false, m.err
	}
	b := m.data[id]
	if off >= uint64(len(b)) {
		return 0, true, nil
	}
	rest := b[off:]
	n := copy(p, rest)
	return n, n == len(rest), nil
}

func (m *memSource) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.reading {
		panic("Close during ReadAt")
	}
	m.closed++
}

func (m *memSource) closeCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func fileEntry(id uint64, name string, flags uint16, size uint64) far2ldnd.Entry {
	return far2ldnd.Entry{ItemID: id, Kind: far2ldnd.KindFile, Flags: flags, Size: size, Name: name}
}

func oneFile(id uint64, flags uint16, data string) *memSource {
	var size uint64
	if flags&far2ldnd.ItemSizeKnown != 0 {
		size = uint64(len(data))
	}
	return &memSource{
		entries: []far2ldnd.Entry{fileEntry(id, "f", flags, size)},
		data:    map[uint64][]byte{id: []byte(data)},
	}
}

const streamFlags = far2ldnd.ItemStream | far2ldnd.ItemSizeKnown

// BIND grants limits no larger than asked and than its own profile, a window of
// at most 4 (LIST/READ are served concurrently up to it), the lease, and only
// features both sides know.
func TestDNDBindNegotiates(t *testing.T) {
	e := newDNDEnv(t)
	q := &far2ldnd.BindRequest{Version: 1, Enable: true, Binding: dndBindingA, MaxFrame: 100000, MaxChunk: 100000, Window: 8, WantedFeatures: 7}
	got := e.request(t, 5, q)
	want := replyFrame(t, 5, &far2ldnd.BindReply{Version: 1, Binding: dndBindingA, MaxFrame: 65536, MaxChunk: 32768, Window: 4, IdleSeconds: 600, Features: 3})
	if got != want {
		t.Fatalf("BIND reply\n got %q\nwant %q", got, want)
	}
	if _, err := far2ldnd.DecodeBindReply(mustReply(t, got), q); err != nil {
		t.Fatalf("the application side refuses the reply: %v", err)
	}
	if !e.tv.DropBound() {
		t.Fatal("not bound after BIND")
	}

	// The same BIND again is idempotent; other parameters for the same
	// binding are refused and change nothing.
	if again := e.request(t, 6, q); again != replyFrame(t, 6, &far2ldnd.BindReply{Version: 1, Binding: dndBindingA, MaxFrame: 65536, MaxChunk: 32768, Window: 4, IdleSeconds: 600, Features: 3}) {
		t.Fatalf("repeated BIND: %q", again)
	}
	other := *q
	other.MaxChunk = 1000
	wantStatus(t, e.request(t, 7, &other), far2ldnd.StatusBadRequest)
	if !e.tv.DropBound() {
		t.Fatal("a refused BIND dropped the binding")
	}
}

// A small frame shrinks the chunk until a READ reply fits it.
func TestDNDBindShrinksChunkToFrame(t *testing.T) {
	e := newDNDEnv(t)
	b := e.bind(t, dndBindingA, 4096, 100000, far2ldnd.FeatureStream)
	if b.MaxFrame != 4096 || b.MaxChunk != far2ldnd.MaxReadData(4096, far2ldnd.ST) {
		t.Fatalf("granted frame %d chunk %d", b.MaxFrame, b.MaxChunk)
	}
	if b.Features != far2ldnd.FeatureStream {
		t.Fatalf("features %#x", b.Features)
	}
}

func TestDNDBindRefusals(t *testing.T) {
	e := newDNDEnv(t)
	// Only an unknown feature: nothing to agree on.
	wantStatus(t, e.request(t, 2, &far2ldnd.BindRequest{Version: 1, Enable: true, Binding: dndBindingA, MaxFrame: 4096, MaxChunk: 1, Window: 1, WantedFeatures: 4}), far2ldnd.StatusUnsupported)
	// Version 2: -5, not a guess at another layout.
	v2 := popOrder([]byte{3}, []byte{'d'}, []byte{'b'}, le16(2), []byte{1}, dndBindingA[:], le32(4096), le32(1), le16(1), le32(1))
	wantStatus(t, e.raw(t, v2), far2ldnd.StatusUnsupported)
	// A frame below the protocol minimum.
	low := popOrder([]byte{4}, []byte{'d'}, []byte{'b'}, le16(1), []byte{1}, dndBindingA[:], le32(4095), le32(1), le16(1), le32(1))
	wantStatus(t, e.raw(t, low), far2ldnd.StatusBadRequest)
	if e.tv.DropBound() {
		t.Fatal("bound after refused BINDs")
	}
}

// A new binding revokes the offers of the old one; a late disable of the old
// generation leaves the new one alone; RID 0 disables without a reply.
func TestDNDBindGenerations(t *testing.T) {
	e := newDNDEnv(t)
	e.bind(t, dndBindingA, 65536, 32768, far2ldnd.FeatureStream)
	src := oneFile(1, streamFlags, "abc")
	offer := e.offer(t, src)

	e.bind(t, dndBindingB, 65536, 32768, far2ldnd.FeatureStream)
	if src.closeCount() != 1 {
		t.Fatalf("source of the old binding closed %d times", src.closeCount())
	}
	wantStatus(t, e.request(t, 3, &far2ldnd.ListRequest{Offer: offer}), far2ldnd.StatusOfferGone)

	off := &far2ldnd.BindRequest{Version: 1, Binding: dndBindingA}
	if got := e.request(t, 4, off); got != replyFrame(t, 4, nil) {
		t.Fatalf("disable of the old binding: %q", got)
	}
	if !e.tv.DropBound() {
		t.Fatal("a late disable of binding A dropped binding B")
	}
	stack, err := far2ldnd.EncodeRequest(0, &far2ldnd.BindRequest{Version: 1, Binding: dndBindingB})
	if err != nil {
		t.Fatal(err)
	}
	e.silent(t, stack)
	if e.tv.DropBound() {
		t.Fatal("still bound after the disable")
	}
}

// Switching far2l extensions off revokes every offer.
func TestDNDFar2lOffRevokes(t *testing.T) {
	e := newDNDEnv(t)
	e.bind(t, dndBindingA, 65536, 32768, far2ldnd.FeatureStream)
	src := oneFile(1, streamFlags, "abc")
	e.offer(t, src)
	e.send("\x1b_far2l0\x07")
	if e.tv.DropBound() || src.closeCount() != 1 {
		t.Fatalf("after far2l0: bound %v, closed %d", e.tv.DropBound(), src.closeCount())
	}
}

// INPUT_DND carries the binding, the fresh offer, the cell and the
// modifiers, and nothing else.
func TestDNDOfferSendsEvent(t *testing.T) {
	e := newDNDEnv(t)
	if _, err := e.tv.OfferDrop(oneFile(1, streamFlags, "x"), 0, 0, 0, false); !errors.Is(err, ErrDNDNotBound) {
		t.Fatalf("offer without binding: %v", err)
	}
	e.bind(t, dndBindingA, 65536, 32768, far2ldnd.FeatureStream)

	e.pty.Reset()
	id, err := e.tv.OfferDrop(oneFile(1, streamFlags, "x"), 5, 2, 0x0008, true)
	if err != nil {
		t.Fatal(err)
	}
	stack, err := far2ldnd.EncodeEvent(&far2ldnd.Event{Binding: dndBindingA, Offer: id, X: 5, Y: 2, Modifiers: 8, Flags: far2ldnd.EventModifiersKnown})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := e.pty.String(), far2ldnd.Frame(far2ldnd.FrameEvent, stack, far2ldnd.BEL); got != want {
		t.Fatalf("event\n got %q\nwant %q", got, want)
	}
	if !strings.HasPrefix(e.pty.String(), "\x1b_f2l") || strings.HasPrefix(e.pty.String(), "\x1b_f2l:") {
		t.Fatalf("event prefix must be f2l without a colon: %q", e.pty.String())
	}

	// An unknown position is (-1,-1), never a half-known one; unknown
	// modifiers are sent as zero with the known bit clear.
	e.pty.Reset()
	id, err = e.tv.OfferDrop(oneFile(1, streamFlags, "x"), -1, 7, 0x0008, false)
	if err != nil {
		t.Fatal(err)
	}
	k, raw, _, err := far2ldnd.ParseFrame(e.pty.String())
	if err != nil || k != far2ldnd.FrameEvent {
		t.Fatalf("event frame: %v", err)
	}
	ev, err := far2ldnd.DecodeEvent(raw)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Offer != id || ev.X != -1 || ev.Y != -1 || ev.Modifiers != 0 || ev.Flags != 0 {
		t.Fatalf("event %+v", ev)
	}
}

func TestDNDOfferRefusals(t *testing.T) {
	e := newDNDEnv(t)
	e.bind(t, dndBindingA, 65536, 32768, far2ldnd.FeatureStream)
	dir := &memSource{entries: []far2ldnd.Entry{{ItemID: 1, Kind: 2, Name: "d"}}}
	if _, err := e.tv.OfferDrop(dir, 0, 0, 0, false); err == nil {
		t.Error("a directory was offered in the base profile")
	}
	dup := &memSource{entries: []far2ldnd.Entry{fileEntry(1, "a", streamFlags, 0), fileEntry(1, "b", streamFlags, 0)}}
	if _, err := e.tv.OfferDrop(dup, 0, 0, 0, false); err == nil {
		t.Error("an item_id was offered twice")
	}
	zero := &memSource{entries: []far2ldnd.Entry{fileEntry(0, "a", streamFlags, 0)}}
	if _, err := e.tv.OfferDrop(zero, 0, 0, 0, false); err == nil {
		t.Error("item_id 0 was offered")
	}
	for i := 0; i < dndMaxOffers; i++ {
		e.offer(t, oneFile(1, streamFlags, "x"))
	}
	if _, err := e.tv.OfferDrop(oneFile(1, streamFlags, "x"), 0, 0, 0, false); !errors.Is(err, ErrDNDTooManyOffers) {
		t.Fatalf("offer over the limit: %v", err)
	}
}

// LIST pages by 64 entries, repeats a page byte for byte, and ends with
// UINT64_MAX.
func TestDNDListPages(t *testing.T) {
	e := newDNDEnv(t)
	e.bind(t, dndBindingA, 65536, 32768, far2ldnd.FeatureStream)
	src := &memSource{}
	for i := uint64(1); i <= 70; i++ {
		src.entries = append(src.entries, fileEntry(i, fmt.Sprintf("file-%d", i), streamFlags, i))
	}
	offer := e.offer(t, src)

	first := e.request(t, 8, &far2ldnd.ListRequest{Offer: offer})
	page, err := far2ldnd.DecodeListReply(mustReply(t, first))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 64 || page.NextCursor != 64 || page.Entries[0].ItemID != 1 || page.Entries[63].Name != "file-64" {
		t.Fatalf("first page: %d entries, next %d", len(page.Entries), page.NextCursor)
	}
	if again := e.request(t, 8, &far2ldnd.ListRequest{Offer: offer}); again != first {
		t.Fatal("the same LIST returned another page")
	}
	last, err := far2ldnd.DecodeListReply(mustReply(t, e.request(t, 9, &far2ldnd.ListRequest{Offer: offer, Cursor: page.NextCursor})))
	if err != nil {
		t.Fatal(err)
	}
	if len(last.Entries) != 6 || last.NextCursor != far2ldnd.CursorLastPage || last.Entries[5].ItemID != 70 || last.Entries[5].Size != 70 {
		t.Fatalf("last page: %+v", last)
	}

	wantStatus(t, e.request(t, 10, &far2ldnd.ListRequest{Offer: offer, Cursor: 70}), far2ldnd.StatusUnknownItem)
	wantStatus(t, e.request(t, 11, &far2ldnd.ListRequest{Offer: offer, ParentID: 1}), far2ldnd.StatusUnknownItem)
	wantStatus(t, e.request(t, 12, &far2ldnd.ListRequest{Offer: far2ldnd.ID{1}}), far2ldnd.StatusOfferGone)
}

func TestDNDListEmptyOffer(t *testing.T) {
	e := newDNDEnv(t)
	e.bind(t, dndBindingA, 65536, 32768, far2ldnd.FeatureStream)
	offer := e.offer(t, &memSource{})
	got := e.request(t, 2, &far2ldnd.ListRequest{Offer: offer})
	if want := replyFrame(t, 2, &far2ldnd.ListReply{NextCursor: far2ldnd.CursorLastPage}); got != want {
		t.Fatalf("empty offer: %q", got)
	}
}

// A page stops where the next entry would overflow max_frame; an entry that
// does not fit alone is -7, never cut.
func TestDNDListRespectsFrame(t *testing.T) {
	e := newDNDEnv(t)
	e.bind(t, dndBindingA, 4096, 1024, far2ldnd.FeatureStream)
	long := strings.Repeat("n", 1500)
	src := &memSource{entries: []far2ldnd.Entry{
		fileEntry(1, long, streamFlags, 0), fileEntry(2, long, streamFlags, 0), fileEntry(3, long, streamFlags, 0),
	}}
	offer := e.offer(t, src)
	var cursor uint64
	for i, want := range []uint64{1, 2, far2ldnd.CursorLastPage} {
		frame := e.request(t, 3, &far2ldnd.ListRequest{Offer: offer, Cursor: cursor})
		if len(frame) > 4096 {
			t.Fatalf("page %d: frame of %d bytes", i, len(frame))
		}
		page, err := far2ldnd.DecodeListReply(mustReply(t, frame))
		if err != nil {
			t.Fatal(err)
		}
		if len(page.Entries) != 1 || page.NextCursor != want || page.Entries[0].Name != long {
			t.Fatalf("page %d: %d entries, next %d", i, len(page.Entries), page.NextCursor)
		}
		cursor = page.NextCursor
	}

	huge := e.offer(t, &memSource{entries: []far2ldnd.Entry{fileEntry(1, strings.Repeat("h", 4000), streamFlags, 0)}})
	wantStatus(t, e.request(t, 4, &far2ldnd.ListRequest{Offer: huge}), far2ldnd.StatusLimit)
}

// The vectors of § 14.1 and § 14.2, byte for byte: the READ request of the
// specification is served as it stands, and a READ of the six bytes gets the
// specification's reply.
func TestDNDReadSpecVectors(t *testing.T) {
	e := newDNDEnv(t)
	e.fixedID = &dndSpecOffer
	e.bind(t, dndBindingA, 65536, 32768, far2ldnd.FeatureStream)
	src := oneFile(7, far2ldnd.ItemStream|far2ldnd.ItemRandomAccess, "\x00\x0a\x0d\x1b\x07\xff")
	if offer := e.offer(t, src); offer != dndSpecOffer {
		t.Fatalf("offer %x", offer)
	}

	// § 14.1: item 7 at 4096, 32768 bytes -- past the end of a six byte
	// file, so zero bytes with EOF.
	e.send("\x1b_far2l:AIAAAAAQAAAAAAAABwAAAAAAAAAAAQIDBAUGBwgJCgsMDQ4PcmQq\x07")
	if got, want := e.wait(t, 1)[0], replyFrame(t, 42, &far2ldnd.ReadReply{Flags: far2ldnd.ReadEOF}); got != want {
		t.Fatalf("§ 14.1\n got %q\nwant %q", got, want)
	}

	// § 14.2: offset 0, length 6, EOF, size not declared.
	got := e.request(t, 42, &far2ldnd.ReadRequest{Offer: dndSpecOffer, ItemID: 7, Offset: 0, Length: 6})
	if want := "\x1b_far2lAAoNGwf/BgAAAAEAAAAAAAAAAAEq\x07"; got != want {
		t.Fatalf("§ 14.2\n got %q\nwant %q", got, want)
	}
}

// A sequential item is read from zero, each range after the bytes received;
// reads past EOF return zero bytes with EOF.
func TestDNDReadSequential(t *testing.T) {
	e := newDNDEnv(t)
	e.bind(t, dndBindingA, 65536, 32768, far2ldnd.FeatureStream)
	offer := e.offer(t, oneFile(1, streamFlags, "0123456789"))
	read := func(rid uint8, off uint64, n uint32) string {
		return e.request(t, rid, &far2ldnd.ReadRequest{Offer: offer, ItemID: 1, Offset: off, Length: n})
	}

	wantStatus(t, read(2, 4, 4), far2ldnd.StatusBadRequest) // must start at 0
	q := &far2ldnd.ReadRequest{Offer: offer, ItemID: 1, Offset: 0, Length: 4}
	d, err := far2ldnd.DecodeReadReply(mustReply(t, read(3, 0, 4)), q)
	if err != nil {
		t.Fatal(err)
	}
	if string(d.Data) != "0123" || d.Flags != far2ldnd.ReadSizeKnown || d.ObservedSize != 10 {
		t.Fatalf("first range: %+v", d)
	}
	wantStatus(t, read(4, 0, 4), far2ldnd.StatusBadRequest) // already read
	if got, want := read(5, 4, 100), replyFrame(t, 5, &far2ldnd.ReadReply{ObservedSize: 10, Flags: far2ldnd.ReadEOF | far2ldnd.ReadSizeKnown, Data: []byte("456789")}); got != want {
		t.Fatalf("tail\n got %q\nwant %q", got, want)
	}
	if got, want := read(6, 10, 5), replyFrame(t, 6, &far2ldnd.ReadReply{ObservedSize: 10, Flags: far2ldnd.ReadEOF | far2ldnd.ReadSizeKnown}); got != want {
		t.Fatalf("past EOF\n got %q\nwant %q", got, want)
	}
}

func TestDNDReadRefusals(t *testing.T) {
	e := newDNDEnv(t)
	e.bind(t, dndBindingA, 65536, 16, far2ldnd.FeatureStream)
	offer := e.offer(t, oneFile(1, streamFlags|far2ldnd.ItemRandomAccess, "0123456789"))

	wantStatus(t, e.request(t, 2, &far2ldnd.ReadRequest{Offer: offer, ItemID: 1, Length: 17}), far2ldnd.StatusBadRequest)
	wantStatus(t, e.request(t, 3, &far2ldnd.ReadRequest{Offer: offer, ItemID: 2, Length: 1}), far2ldnd.StatusUnknownItem)
	wantStatus(t, e.request(t, 4, &far2ldnd.ReadRequest{Offer: far2ldnd.ID{9}, ItemID: 1, Length: 1}), far2ldnd.StatusOfferGone)
	// Zero length is an error, not "read everything" as in FISH+.
	zero := popOrder([]byte{5}, []byte{'d'}, []byte{'r'}, offer[:], le64(1), le64(0), le32(0))
	wantStatus(t, e.raw(t, zero), far2ldnd.StatusBadRequest)
	// Random access: any range.
	d, err := far2ldnd.DecodeReadReply(mustReply(t, e.request(t, 6, &far2ldnd.ReadRequest{Offer: offer, ItemID: 1, Offset: 7, Length: 2})), &far2ldnd.ReadRequest{Length: 2})
	if err != nil || string(d.Data) != "78" {
		t.Fatalf("random access: %q %v", d.Data, err)
	}
}

func TestDNDReadSourceErrors(t *testing.T) {
	e := newDNDEnv(t)
	e.bind(t, dndBindingA, 65536, 32768, far2ldnd.FeatureStream)
	changed := oneFile(1, streamFlags, "abc")
	changed.err = fmt.Errorf("mtime moved: %w", ErrDNDSourceChanged)
	offer := e.offer(t, changed)
	wantStatus(t, e.request(t, 2, &far2ldnd.ReadRequest{Offer: offer, ItemID: 1, Length: 3}), far2ldnd.StatusChanged)

	broken := oneFile(1, streamFlags, "abc")
	broken.err = errors.New("disk on fire \xff")
	offer = e.offer(t, broken)
	wantStatus(t, e.request(t, 3, &far2ldnd.ReadRequest{Offer: offer, ItemID: 1, Length: 3}), far2ldnd.StatusIOError)
}

// Representations are limited to the negotiated features: without STREAM an
// item cannot be read, and LIST does not claim it can.
func TestDNDReferenceOnlyBinding(t *testing.T) {
	e := newDNDEnv(t)
	e.bind(t, dndBindingA, 65536, 32768, far2ldnd.FeatureReference|far2ldnd.FeatureStream<<4)
	src := oneFile(1, far2ldnd.ItemReference|far2ldnd.ItemStream|far2ldnd.ItemRandomAccess, "abc")
	src.entries[0].ReferenceURI = "file:///tmp/f"
	offer := e.offer(t, src)
	page, err := far2ldnd.DecodeListReply(mustReply(t, e.request(t, 2, &far2ldnd.ListRequest{Offer: offer})))
	if err != nil {
		t.Fatal(err)
	}
	if got := page.Entries[0]; got.Flags != far2ldnd.ItemReference || got.ReferenceURI != "file:///tmp/f" {
		t.Fatalf("entry %+v", got)
	}
	wantStatus(t, e.request(t, 3, &far2ldnd.ReadRequest{Offer: offer, ItemID: 1, Length: 3}), far2ldnd.StatusUnsupported)
}

// CLOSE releases the source once, answers with an empty body, and is safe to
// repeat; with RID 0 it releases without a reply.
func TestDNDClose(t *testing.T) {
	e := newDNDEnv(t)
	e.bind(t, dndBindingA, 65536, 32768, far2ldnd.FeatureStream)
	src := oneFile(1, streamFlags, "abc")
	offer := e.offer(t, src)
	closeReq := &far2ldnd.CloseRequest{Offer: offer, Reason: far2ldnd.CloseProcessed}
	if got := e.request(t, 9, closeReq); got != replyFrame(t, 9, nil) {
		t.Fatalf("CLOSE: %q", got)
	}
	if got := e.request(t, 10, closeReq); got != replyFrame(t, 10, nil) {
		t.Fatalf("repeated CLOSE: %q", got)
	}
	if src.closeCount() != 1 {
		t.Fatalf("source closed %d times", src.closeCount())
	}
	wantStatus(t, e.request(t, 11, &far2ldnd.ReadRequest{Offer: offer, ItemID: 1, Length: 1}), far2ldnd.StatusOfferGone)
	// A reason above the protocol's own range is clamped to CloseFailed, not
	// refused (owner's answer 9): CLOSE is a cleanup barrier, and this one
	// still just repeats the already-closed offer, empty body and all.
	badReason := popOrder([]byte{12}, []byte{'d'}, []byte{'c'}, offer[:], []byte{4})
	if got := e.raw(t, badReason); got != replyFrame(t, 12, nil) {
		t.Fatalf("CLOSE reason 4 (clamped to 3): %q", got)
	}

	src2 := oneFile(1, streamFlags, "abc")
	offer2 := e.offer(t, src2)
	stack, err := far2ldnd.EncodeRequest(0, &far2ldnd.CloseRequest{Offer: offer2, Reason: far2ldnd.CloseCancelled})
	if err != nil {
		t.Fatal(err)
	}
	e.silent(t, stack)
	if src2.closeCount() != 1 {
		t.Fatalf("RID 0 CLOSE: source closed %d times", src2.closeCount())
	}
}

// A READ that is running when its offer is revoked gets -8, and the source
// is closed only after the read returns.
func TestDNDRevokeDuringRead(t *testing.T) {
	e := newDNDEnv(t)
	e.bind(t, dndBindingA, 65536, 32768, far2ldnd.FeatureStream)
	src := oneFile(1, streamFlags, "abc")
	src.started = make(chan struct{})
	src.block = make(chan struct{})
	offer := e.offer(t, src)

	stack, err := far2ldnd.EncodeRequest(7, &far2ldnd.ReadRequest{Offer: offer, ItemID: 1, Length: 3})
	if err != nil {
		t.Fatal(err)
	}
	e.send(far2ldnd.Frame(far2ldnd.FrameRequest, stack, far2ldnd.BEL))
	select {
	case <-src.started:
	case <-time.After(5 * time.Second):
		t.Fatal("READ never reached the source")
	}
	e.p.Process([]byte("\x1b_far2l0\x07"))
	if src.closeCount() != 0 {
		t.Fatal("source closed while it was being read")
	}
	close(src.block)
	frame := e.wait(t, 1)[0]
	if replyRID(t, frame) != 7 {
		t.Fatalf("reply to another RID: %q", frame)
	}
	wantStatus(t, frame, far2ldnd.StatusCancelled)
	if src.closeCount() != 1 {
		t.Fatalf("source closed %d times", src.closeCount())
	}
}

// An offer nobody touches for idle_seconds is revoked; LIST and READ extend
// the lease.
func TestDNDIdleLease(t *testing.T) {
	e := newDNDEnv(t)
	e.bind(t, dndBindingA, 65536, 32768, far2ldnd.FeatureStream)
	src := oneFile(1, streamFlags, "abc")
	offer := e.offer(t, src)
	for i := 0; i < 3; i++ {
		e.advance(500 * time.Second)
		wantStatus(t, e.request(t, 2, &far2ldnd.ListRequest{Offer: offer}), far2ldnd.StatusOK)
	}
	e.advance(601 * time.Second)
	wantStatus(t, e.request(t, 3, &far2ldnd.ListRequest{Offer: offer}), far2ldnd.StatusOfferGone)
	if src.closeCount() != 1 {
		t.Fatalf("expired source closed %d times", src.closeCount())
	}
}

// With window 1 the replies keep the order of the requests, even when they
// arrive in one read (a larger window serves LIST/READ concurrently, and
// replies then follow their RIDs, not their order).
func TestDNDRepliesInOrder(t *testing.T) {
	e := newDNDEnv(t)
	_ = mustReply(t, e.request(t, 30, &far2ldnd.BindRequest{Version: 1, Enable: true, Binding: dndBindingA, MaxFrame: 65536, MaxChunk: 32768, Window: 1, WantedFeatures: far2ldnd.FeatureStream}))
	offer := e.offer(t, oneFile(1, streamFlags, "abc"))
	var in bytes.Buffer
	for rid := uint8(1); rid <= 20; rid++ {
		stack, err := far2ldnd.EncodeRequest(rid, &far2ldnd.ListRequest{Offer: offer})
		if err != nil {
			t.Fatal(err)
		}
		in.WriteString(far2ldnd.Frame(far2ldnd.FrameRequest, stack, far2ldnd.BEL))
	}
	e.send(in.String())
	for i, frame := range e.wait(t, 20) {
		if rid := replyRID(t, frame); rid != uint8(i+1) {
			t.Fatalf("reply %d has RID %d", i, rid)
		}
	}
}

// A DnD frame over the limit -- 512 bytes before BIND, max_frame after -- is
// dropped whole before decoding and answered with -7; a smaller malformed
// one is decoded and refused as -4.
func TestDNDFrameLimit(t *testing.T) {
	e := newDNDEnv(t)
	closeStack, err := far2ldnd.EncodeRequest(3, &far2ldnd.CloseRequest{Offer: far2ldnd.ID{1}})
	if err != nil {
		t.Fatal(err)
	}
	padded := func(n int) []byte { return append(make([]byte, n), closeStack...) }

	wantStatus(t, e.raw(t, padded(400)), far2ldnd.StatusLimit)
	wantStatus(t, e.raw(t, padded(100)), far2ldnd.StatusBadRequest)

	e.bind(t, dndBindingA, 4096, 1024, far2ldnd.FeatureStream)
	wantStatus(t, e.raw(t, padded(400)), far2ldnd.StatusBadRequest)
	wantStatus(t, e.raw(t, padded(3100)), far2ldnd.StatusLimit)
}

func TestDNDMalformedRequests(t *testing.T) {
	e := newDNDEnv(t)
	unknown := popOrder([]byte{4}, []byte{'d'}, []byte{'z'})
	// A well-formed request for an operation this version does not have --
	// -5, not -4 (owner's answer 2): -4 would tell a new client this
	// terminal has no DND family at all, which ErrNoDND's empty reply is
	// reserved for.
	wantStatus(t, e.raw(t, unknown), far2ldnd.StatusUnsupported)
	// LIST must be answered, so RID 0 is malformed -- and there is no one to
	// tell.
	e.silent(t, popOrder([]byte{0}, []byte{'d'}, []byte{'l'}, make([]byte, 16), le64(0), le64(0)))
}

// ProcessFar2lInteract, the entry point of already decoded requests, serves
// the DnD family as well.
func TestDNDProcessFar2lInteract(t *testing.T) {
	e := newDNDEnv(t)
	stack, err := far2ldnd.EncodeRequest(9, &far2ldnd.BindRequest{Version: 1, Enable: true, Binding: dndBindingA, MaxFrame: 4096, MaxChunk: 1, Window: 1, WantedFeatures: 1})
	if err != nil {
		t.Fatal(err)
	}
	e.tv.ProcessFar2lInteract(stack)
	if replyRID(t, e.wait(t, 1)[0]) != 9 || !e.tv.DropBound() {
		t.Fatal("BIND through ProcessFar2lInteract was not served")
	}
}

// dndErrorLocked shortens a diagnostic to whatever the *current* frame limit
// carries, not just far2ldnd's own flat MaxMessageLen ceiling (owner's
// answer 3): pre-BIND, that is BindFrameLimit (512 bytes), well under 1024;
// after BIND negotiates a large enough max_frame, the full 1024-byte
// ceiling fits.
func TestDNDErrorFitsFrame(t *testing.T) {
	e := newDNDEnv(t)
	long := strings.Repeat("я", 600) // 1200 bytes, over both ceilings

	e.pty.Reset()
	e.tv.dndAsyncError(9, far2ldnd.StatusUnsupported, long)
	frame := e.wait(t, 1)[0]
	if n := len(frame); n > far2ldnd.BindFrameLimit {
		t.Fatalf("pre-BIND error reply is %d bytes, over BindFrameLimit %d", n, far2ldnd.BindFrameLimit)
	}
	var se *far2ldnd.StatusError
	if _, err := parseReply(t, frame); !errors.As(err, &se) || !strings.HasPrefix(long, se.Message) {
		t.Fatalf("pre-BIND message not shortened from the original: %v", err)
	}

	e.bind(t, dndBindingA, far2ldnd.DefaultMaxFrame, far2ldnd.DefaultMaxChunk, far2ldnd.FeatureStream)
	e.pty.Reset()
	e.tv.dndAsyncError(10, far2ldnd.StatusIOError, long)
	frame2 := e.wait(t, 1)[0]
	if _, err := parseReply(t, frame2); !errors.As(err, &se) || len(se.Message) != far2ldnd.MaxMessageLen {
		t.Fatalf("post-BIND message not shortened to MaxMessageLen: %v", err)
	}
}
