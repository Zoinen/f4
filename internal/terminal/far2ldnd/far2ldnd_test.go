package far2ldnd

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.Join(strings.Fields(s), ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func seqID(start byte) ID {
	var id ID
	for i := range id {
		id[i] = start + byte(i)
	}
	return id
}

// § 14.1 of the specification: a READ request, byte for byte, as far2l's own
// StackSerializer and base64 produce it.
func TestSpecVectorRead(t *testing.T) {
	q := &ReadRequest{Offer: seqID(0), ItemID: 7, Offset: 4096, Length: 32768}
	stack, err := EncodeRequest(42, q)
	if err != nil {
		t.Fatal(err)
	}
	want := mustHex(t, `
		00 80 00 00
		00 10 00 00 00 00 00 00
		07 00 00 00 00 00 00 00
		00 01 02 03 04 05 06 07 08 09 0a 0b 0c 0d 0e 0f
		72 64 2a`)
	if !bytes.Equal(stack, want) {
		t.Fatalf("stack\n got % x\nwant % x", stack, want)
	}
	const apc = "\x1b_far2l:AIAAAAAQAAAAAAAABwAAAAAAAAAAAQIDBAUGBwgJCgsMDQ4PcmQq\x07"
	if got := Frame(FrameRequest, stack, BEL); got != apc {
		t.Fatalf("APC\n got %q\nwant %q", got, apc)
	}
	if n := FrameLen(FrameRequest, len(stack), BEL); n != len(apc) {
		t.Fatalf("FrameLen = %d, want %d", n, len(apc))
	}

	k, back, term, err := ParseFrame(apc)
	if err != nil || k != FrameRequest || term != BEL || !bytes.Equal(back, want) {
		t.Fatalf("ParseFrame = %v, % x, %v, %v", k, back, term, err)
	}
	rid, got, err := DecodeRequest(back)
	if err != nil || rid != 42 {
		t.Fatalf("DecodeRequest: rid %d, %v", rid, err)
	}
	if !reflect.DeepEqual(got, q) {
		t.Fatalf("decoded %+v, want %+v", got, q)
	}
}

// § 14.2: a successful READ reply with six awkward bytes and EOF.
func TestSpecVectorReadReply(t *testing.T) {
	data := []byte{0x00, 0x0a, 0x0d, 0x1b, 0x07, 0xff}
	stack, err := EncodeReply(42, &ReadReply{ObservedSize: 0, Flags: ReadEOF, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	const apc = "\x1b_far2lAAoNGwf/BgAAAAEAAAAAAAAAAAEq\x07"
	if got := Frame(FrameReply, stack, BEL); got != apc {
		t.Fatalf("APC\n got %q\nwant %q", got, apc)
	}

	k, back, _, err := ParseFrame(apc)
	if err != nil || k != FrameReply {
		t.Fatalf("ParseFrame = %v, %v", k, err)
	}
	f, err := DecodeReply(back)
	if err != nil || f.RID != 42 || f.Status != StatusOK {
		t.Fatalf("DecodeReply = %+v, %v", f, err)
	}
	d, err := DecodeReadReply(f, &ReadRequest{Offer: seqID(0), ItemID: 7, Offset: 0, Length: 6})
	if err != nil {
		t.Fatal(err)
	}
	if d.ObservedSize != 0 || d.Flags != ReadEOF || !bytes.Equal(d.Data, data) {
		t.Fatalf("decoded %+v", d)
	}
}

// § 14.3: the length of a string lies after its bytes in the buffer and is
// popped before them.
func TestSpecVectorString(t *testing.T) {
	var w stackWriter
	w.str("a\nb")
	stack, err := w.bytes()
	if err != nil {
		t.Fatal(err)
	}
	if want := mustHex(t, "61 0a 62 03 00 00 00"); !bytes.Equal(stack, want) {
		t.Fatalf("got % x, want % x", stack, want)
	}
	if got := base64.StdEncoding.EncodeToString(stack); got != "YQpiAwAAAA==" {
		t.Fatalf("base64 %q", got)
	}
	r := stackReader{b: stack}
	if s := r.str(16); s != "a\nb" || r.end() != nil {
		t.Fatalf("popped %q, %v", s, r.err)
	}
}

// The physical layout of the messages the specification gives no vector
// for, spelled out by hand from the field tables (§ 6.1, § 6.2, § 6.5).
func TestLayoutByHand(t *testing.T) {
	bind, err := EncodeRequest(1, &BindRequest{
		Version: 1, Enable: true, Binding: seqID(0x10),
		MaxFrame: 65536, MaxChunk: 32768, Window: 4, WantedFeatures: FeatureStream | FeatureReference,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantBind := mustHex(t, `
		03 00 00 00
		04 00
		00 80 00 00
		00 00 01 00
		10 11 12 13 14 15 16 17 18 19 1a 1b 1c 1d 1e 1f
		01
		01 00
		62 64 01`)
	if !bytes.Equal(bind, wantBind) {
		t.Errorf("BIND\n got % x\nwant % x", bind, wantBind)
	}

	closeStack, err := EncodeRequest(5, &CloseRequest{Offer: seqID(0), Reason: CloseCancelled})
	if err != nil {
		t.Fatal(err)
	}
	wantClose := mustHex(t, "02 00 01 02 03 04 05 06 07 08 09 0a 0b 0c 0d 0e 0f 63 64 05")
	if !bytes.Equal(closeStack, wantClose) {
		t.Errorf("CLOSE\n got % x\nwant % x", closeStack, wantClose)
	}

	ev, err := EncodeEvent(&Event{
		Binding: seqID(0x10), Offer: seqID(0x20), X: 3, Y: 4,
		Modifiers: 0x10, Flags: EventModifiersKnown,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantEv := mustHex(t, `
		01 00
		10 00 00 00
		04 00
		03 00
		20 21 22 23 24 25 26 27 28 29 2a 2b 2c 2d 2e 2f
		10 11 12 13 14 15 16 17 18 19 1a 1b 1c 1d 1e 1f
		44`)
	if !bytes.Equal(ev, wantEv) {
		t.Errorf("event\n got % x\nwant % x", ev, wantEv)
	}
	if got := Frame(FrameEvent, ev, BEL); !strings.HasPrefix(got, "\x1b_f2l") || strings.HasPrefix(got, "\x1b_f2l:") {
		t.Errorf("event APC %q: events go out as f2l without a colon", got)
	}

	unknown, err := EncodeEvent(&Event{X: -1, Y: -1})
	if err != nil {
		t.Fatal(err)
	}
	if want := mustHex(t, "00 00 00 00 00 00 ff ff ff ff"); !bytes.Equal(unknown[:10], want) {
		t.Errorf("unknown position % x, want % x", unknown[:10], want)
	}

	// The first entry of a LIST page is popped first, so it lies last.
	list, err := EncodeReply(9, &ListReply{NextCursor: CursorLastPage, Entries: []Entry{
		{ItemID: 1, Kind: KindFile, Name: "a"},
		{ItemID: 2, Kind: KindFile, Name: "b"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	tail := mustHex(t, "02 00 00 00 ff ff ff ff ff ff ff ff 01 09")
	if !bytes.HasSuffix(list, tail) {
		t.Errorf("LIST header % x", list[len(list)-len(tail):])
	}
	first := bytes.LastIndexByte(list[:len(list)-len(tail)], 'a')
	second := bytes.LastIndexByte(list, 'b')
	if first < second {
		t.Errorf("entry order: 'a' at %d, 'b' at %d", first, second)
	}
}

func roundTripRequest(t *testing.T, rid uint8, q Request) []byte {
	t.Helper()
	stack, err := EncodeRequest(rid, q)
	if err != nil {
		t.Fatal(err)
	}
	gotRID, got, err := DecodeRequest(stack)
	if err != nil || gotRID != rid || !reflect.DeepEqual(got, q) {
		t.Fatalf("round trip: rid %d, %+v, %v; want %+v", gotRID, got, err, q)
	}
	return stack
}

func TestRequestsRoundTrip(t *testing.T) {
	stacks := [][]byte{
		roundTripRequest(t, 1, &BindRequest{Version: 1, Enable: true, Binding: seqID(3),
			MaxFrame: 4096, MaxChunk: 1, Window: 32, WantedFeatures: FeatureStream}),
		roundTripRequest(t, 0, &BindRequest{Version: 1, Binding: seqID(3)}),
		roundTripRequest(t, 2, &ListRequest{Offer: seqID(7), ParentID: 0, Cursor: 12}),
		roundTripRequest(t, 3, &ReadRequest{Offer: seqID(7), ItemID: 1, Offset: math.MaxUint64 - 1, Length: 1}),
		roundTripRequest(t, 0, &CloseRequest{Offer: seqID(7), Reason: CloseRejected}),
		roundTripRequest(t, 255, &CloseRequest{Offer: seqID(7), Reason: CloseFailed}),
	}
	// Every cut of a valid stack and every extra byte in front of it is
	// refused: the reader starts at the end, so both are a wrong frame.
	for _, s := range stacks {
		for i := 1; i < len(s); i++ {
			if _, _, err := DecodeRequest(s[i:]); err == nil {
				t.Errorf("cut %d of % x accepted", i, s)
			}
		}
		if _, _, err := DecodeRequest(append([]byte{0}, s...)); !errors.Is(err, ErrTrailing) {
			t.Errorf("extra byte before % x: %v", s, err)
		}
	}
}

func TestRequestRejections(t *testing.T) {
	enc := func(rid uint8, q Request) []byte {
		t.Helper()
		// Build without the encoder's own checks, to test the decoder.
		var w stackWriter
		w.u8(rid)
		w.u8(InteractDND)
		w.u8(q.Subcommand())
		q.put(&w)
		b, err := w.bytes()
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	cases := []struct {
		name   string
		stack  []byte
		err    error
		status Status
	}{
		{"read of zero bytes", enc(4, &ReadRequest{ItemID: 1}), ErrInvalid, StatusBadRequest},
		{"read overflow", enc(4, &ReadRequest{ItemID: 1, Offset: math.MaxUint64, Length: 1}), ErrInvalid, StatusBadRequest},
		{"read of item 0", enc(4, &ReadRequest{Length: 1}), ErrInvalid, StatusBadRequest},
		{"list without reply", enc(0, &ListRequest{}), ErrInvalid, StatusBadRequest},
		{"read without reply", enc(0, &ReadRequest{ItemID: 1, Length: 1}), ErrInvalid, StatusBadRequest},
		{"bind on without reply", enc(0, &BindRequest{Version: 1, Enable: true, MaxFrame: 4096, MaxChunk: 1, Window: 1}), ErrInvalid, StatusBadRequest},
		{"bind off with limits", enc(4, &BindRequest{Version: 1, MaxFrame: 4096}), ErrInvalid, StatusBadRequest},
		{"bind frame below 4096", enc(4, &BindRequest{Version: 1, Enable: true, MaxFrame: 4095, MaxChunk: 1, Window: 1}), ErrInvalid, StatusBadRequest},
		{"bind chunk 0", enc(4, &BindRequest{Version: 1, Enable: true, MaxFrame: 4096, Window: 1}), ErrInvalid, StatusBadRequest},
		{"bind window 0", enc(4, &BindRequest{Version: 1, Enable: true, MaxFrame: 4096, MaxChunk: 1, Window: 0}), ErrInvalid, StatusBadRequest},
		{"bind version 2", enc(4, &BindRequest{Version: 2, Enable: true}), ErrUnsupportedVersion, StatusUnsupported},
		{"unknown subcommand", []byte{'z', 'd', 4}, ErrUnknownSubcommand, StatusUnsupported},
		{"clipboard", []byte{'c', 'c', 4}, ErrNotDND, StatusBadRequest},
		{"empty", nil, ErrTruncated, StatusBadRequest},
	}
	for _, c := range cases {
		rid, _, err := DecodeRequest(c.stack)
		if !errors.Is(err, c.err) {
			t.Errorf("%s: %v, want %v", c.name, err, c.err)
			continue
		}
		if len(c.stack) > 0 && rid != 4 && rid != 0 {
			t.Errorf("%s: rid %d lost", c.name, rid)
		}
		if s := StatusFor(err); s != c.status {
			t.Errorf("%s: status %d, want %d", c.name, s, c.status)
		}
	}

	// BIND enable must be exactly 0 or 1.
	bad := enc(4, &BindRequest{Version: 1, Enable: true, MaxFrame: 4096, MaxChunk: 1, Window: 1})
	bad[len(bad)-3-2-1] = 2 // the enable byte, just below version
	if _, _, err := DecodeRequest(bad); !errors.Is(err, ErrInvalid) {
		t.Errorf("enable=2: %v", err)
	}

	// The encoder refuses the same things.
	if _, err := EncodeRequest(4, &ReadRequest{ItemID: 1}); !errors.Is(err, ErrInvalid) {
		t.Errorf("encode zero READ: %v", err)
	}
	if _, err := EncodeRequest(0, &ListRequest{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("encode LIST with RID 0: %v", err)
	}
	if _, err := EncodeRequest(4, &BindRequest{Version: 2}); !errors.Is(err, ErrUnsupportedVersion) {
		t.Errorf("encode BIND v2: %v", err)
	}
}

// TestRequestDecodeLeniency covers the owner's answers that make request
// decoding more tolerant than the strict encoder: a BIND window above the
// protocol's reply ceiling is a profile to negotiate down, not a malformed
// request (answer 4), and a CLOSE reason outside 0..3 is clamped to 3
// rather than refused, so the barrier still runs (answer 9).
func TestRequestDecodeLeniency(t *testing.T) {
	q := &BindRequest{Version: 1, Enable: true, Binding: seqID(6), MaxFrame: 4096, MaxChunk: 1, Window: 1000, WantedFeatures: FeatureStream}
	if _, err := EncodeRequest(9, q); err != nil {
		t.Fatalf("encode BIND window 1000: %v", err)
	}
	stack := roundTripRequest(t, 9, q)
	if _, _, err := DecodeRequest(stack); err != nil {
		t.Fatalf("decode BIND window 1000: %v", err)
	}

	// Laid out by hand with Reason=4 (the encoder refuses this; only a
	// decoder should ever meet it, from another implementation).
	var w stackWriter
	w.u8(7)
	w.u8(InteractDND)
	w.u8(SubClose)
	w.id(seqID(1))
	w.u8(4)
	bad, err := w.bytes()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := EncodeRequest(7, &CloseRequest{Offer: seqID(1), Reason: 4}); !errors.Is(err, ErrInvalid) {
		t.Errorf("encode CLOSE reason 4: %v", err)
	}
	rid, got, err := DecodeRequest(bad)
	if err != nil {
		t.Fatalf("decode CLOSE reason 4: %v", err)
	}
	c, ok := got.(*CloseRequest)
	if !ok || rid != 7 || c.Reason != CloseFailed {
		t.Fatalf("CLOSE reason 4 not clamped: rid %d, %+v", rid, got)
	}
}

func TestBindReply(t *testing.T) {
	q := &BindRequest{Version: 1, Enable: true, Binding: seqID(0x40),
		MaxFrame: 65536, MaxChunk: 32768, Window: 4, WantedFeatures: FeatureStream | FeatureReference}
	ok := BindReply{Version: 1, Binding: q.Binding, MaxFrame: 8192, MaxChunk: 4096,
		Window: 1, IdleSeconds: 600, Features: FeatureStream}

	decode := func(b BindReply) (BindReply, error) {
		t.Helper()
		var w stackWriter
		w.u8(7)
		w.i8(int8(StatusOK))
		// Laid out without the encoder's checks, so the decoder is what is tested.
		w.u16(b.Version)
		w.id(b.Binding)
		w.u32(b.MaxFrame)
		w.u32(b.MaxChunk)
		w.u16(b.Window)
		w.u32(b.IdleSeconds)
		w.u32(b.Features)
		stack, _ := w.bytes()
		f, err := DecodeReply(stack)
		if err != nil {
			t.Fatal(err)
		}
		return DecodeBindReply(f, q)
	}

	stack, err := EncodeReply(7, &ok)
	if err != nil {
		t.Fatal(err)
	}
	f, err := DecodeReply(stack)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeBindReply(f, q); err != nil || got != ok {
		t.Fatalf("round trip %+v, %v", got, err)
	}
	if n := FrameLen(FrameReply, len(stack), ST); n > BindFrameLimit {
		t.Errorf("BIND reply APC %d bytes, limit %d", n, BindFrameLimit)
	}

	bad := map[string]func(*BindReply){
		"version":         func(b *BindReply) { b.Version = 2 },
		"binding echo":    func(b *BindReply) { b.Binding[0] ^= 1 },
		"frame above ask": func(b *BindReply) { b.MaxFrame = q.MaxFrame + 1 },
		"chunk above ask": func(b *BindReply) { b.MaxChunk = q.MaxChunk + 1 },
		"window above":    func(b *BindReply) { b.Window = q.Window + 1 },
		"frame below min": func(b *BindReply) { b.MaxFrame = MinMaxFrame - 1 },
		"chunk zero":      func(b *BindReply) { b.MaxChunk = 0 },
		"window zero":     func(b *BindReply) { b.Window = 0 },
		"unwanted bit":    func(b *BindReply) { b.Features |= 4 },
		"no feature":      func(b *BindReply) { b.Features = 0 },
		"chunk overflows max_frame": func(b *BindReply) {
			b.MaxFrame = MinMaxFrame
			b.MaxChunk = MinMaxFrame // far above what MinMaxFrame can carry
		},
	}
	for name, mut := range bad {
		b := ok
		mut(&b)
		if _, err := decode(b); err == nil {
			t.Errorf("%s accepted", name)
		}
	}

	// Switching off: empty body both ways.
	off := &BindRequest{Version: 1, Binding: q.Binding}
	stack, err = EncodeReply(8, nil)
	if err != nil {
		t.Fatal(err)
	}
	if f, err = DecodeReply(stack); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeBindReply(f, off); err != nil {
		t.Errorf("empty off reply: %v", err)
	}
	if _, err := DecodeBindReply(ReplyFrame{RID: 8, Status: StatusOK, body: []byte{0}}, off); !errors.Is(err, ErrTrailing) {
		t.Errorf("off reply with a body: %v", err)
	}
}

func TestOldServerAndErrors(t *testing.T) {
	// A far2l that does not know 'd' answers with the RID alone.
	f, err := DecodeReply([]byte{42})
	if !errors.Is(err, ErrNoDND) || f.RID != 42 {
		t.Fatalf("old server: %+v, %v", f, err)
	}

	stack, err := EncodeError(42, StatusOfferGone, "offer expired")
	if err != nil {
		t.Fatal(err)
	}
	f, err = DecodeReply(stack)
	var se *StatusError
	if !errors.As(err, &se) || se.Status != StatusOfferGone || se.Message != "offer expired" || f.RID != 42 {
		t.Fatalf("error reply: %+v, %v", f, err)
	}

	for s := StatusCancelled; s <= StatusIOError; s++ {
		stack, err := EncodeError(1, s, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := DecodeReply(stack); !errors.As(err, &se) || se.Status != s {
			t.Errorf("status %d: %v", s, err)
		}
	}

	if _, err := EncodeError(1, StatusIOError, strings.Repeat("x", MaxMessageLen)); err != nil {
		t.Errorf("message of exactly %d bytes: %v", MaxMessageLen, err)
	}
	// A message over MaxMessageLen is shortened, not refused (owner's answer
	// 3): the caller's RID must not hang forever just because the terminal
	// had something long to say.
	long, err := EncodeError(1, StatusIOError, strings.Repeat("x", MaxMessageLen+1))
	if err != nil {
		t.Errorf("long message: %v", err)
	}
	if f, err := DecodeReply(long); err == nil || !errors.As(err, &se) || len(se.Message) != MaxMessageLen {
		t.Errorf("long message not shortened to %d bytes: %+v, %v", MaxMessageLen, f, err)
	}
	if _, err := EncodeError(1, StatusOK, ""); !errors.Is(err, ErrInvalid) {
		t.Errorf("error with status 1: %v", err)
	}
	// Invalid UTF-8 is replaced, not refused, the same as a message that is
	// merely too long: TruncateText already guarantees valid UTF-8 before
	// EncodeError ever sees it (dndErrorLocked, far2l_dnd.go, calls the same
	// helper for its own frame-aware shortening).
	invalid, err := EncodeError(1, StatusIOError, "a\xffb")
	if err != nil {
		t.Errorf("non-UTF-8 message: %v", err)
	}
	if f, err := DecodeReply(invalid); err == nil || !errors.As(err, &se) || se.Message != "a?b" {
		t.Errorf("non-UTF-8 message not replaced: %+v, %v", f, err)
	}
	if _, err := EncodeReply(0, nil); !errors.Is(err, ErrInvalid) {
		t.Errorf("reply to RID 0: %v", err)
	}

	// An error carries the message and nothing else: RID 1, status -1,
	// message "x", and one stray byte in front.
	stack = []byte{0, 'x', 1, 0, 0, 0, 0xff, 1}
	if _, err := DecodeReply(stack); !errors.Is(err, ErrTrailing) {
		t.Errorf("error with trailing bytes: %v", err)
	}
}

func TestLengthsCheckedBeforeAllocation(t *testing.T) {
	// A str claiming 4 GiB in a 6-byte stack must fail without allocating.
	huge := []byte{'a', 'b', 0xff, 0xff, 0xff, 0xff}
	r := stackReader{b: huge}
	if s := r.str(math.MaxUint32); s != "" || !errors.Is(r.err, ErrTruncated) {
		t.Errorf("huge str: %q, %v", s, r.err)
	}
	r = stackReader{b: huge}
	if r.blob(16); !errors.Is(r.err, ErrTooLong) {
		t.Errorf("str over its limit: %v", r.err)
	}
	allocs := testing.AllocsPerRun(10, func() {
		r := stackReader{b: huge}
		r.blob(math.MaxUint32)
	})
	if allocs != 0 {
		t.Errorf("%v allocations for a refused length", allocs)
	}

	// Error message over 1024 bytes on the wire.
	var w stackWriter
	w.u8(1)
	w.i8(int8(StatusIOError))
	w.str(strings.Repeat("x", MaxMessageLen+1))
	stack, _ := w.bytes()
	if _, err := DecodeReply(stack); !errors.Is(err, ErrTooLong) {
		t.Errorf("long message on the wire: %v", err)
	}
}

func TestListReply(t *testing.T) {
	entries := []Entry{
		{ItemID: 1, Kind: KindFile, Flags: ItemStream | ItemRandomAccess | ItemFrozen | ItemSizeKnown,
			Size: 1 << 40, Name: "report.txt", NativeEncoding: NativePOSIX, NativeName: []byte("rep\xffort.txt"),
			SourceNamespace: "host:a"},
		{ItemID: 2, Kind: KindFile, Flags: ItemReference | ItemStream, Name: "a\nb\x1b\x07",
			ReferenceURI: "file:///tmp/a%0Ab", SourceNamespace: ""},
		{ItemID: math.MaxUint64, Kind: KindFile, Flags: ItemStream, Name: "",
			NativeEncoding: NativeUTF16LE, NativeName: []byte{'x', 0}},
	}
	l := &ListReply{NextCursor: 3, Entries: entries}
	stack, err := EncodeReply(5, l)
	if err != nil {
		t.Fatal(err)
	}
	f, err := DecodeReply(stack)
	if err != nil {
		t.Fatal(err)
	}
	got, err := DecodeListReply(f)
	if err != nil || !reflect.DeepEqual(&got, l) {
		t.Fatalf("round trip %+v, %v", got, err)
	}

	last, err := EncodeReply(5, &ListReply{NextCursor: CursorLastPage})
	if err != nil {
		t.Fatal(err)
	}
	if f, err = DecodeReply(last); err != nil {
		t.Fatal(err)
	}
	if got, err := DecodeListReply(f); err != nil || len(got.Entries) != 0 {
		t.Errorf("empty last page: %+v, %v", got, err)
	}

	if _, err := EncodeReply(5, &ListReply{NextCursor: 1}); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty non-last page: %v", err)
	}
	many := make([]Entry, MaxListEntries+1)
	id := uint64(0)
	for i := range many {
		id++
		many[i] = Entry{ItemID: id, Kind: KindFile}
	}
	if _, err := EncodeReply(5, &ListReply{NextCursor: CursorLastPage, Entries: many}); !errors.Is(err, ErrTooLong) {
		t.Errorf("65 entries: %v", err)
	}
	if _, err := EncodeReply(5, &ListReply{NextCursor: CursorLastPage, Entries: many[:MaxListEntries]}); err != nil {
		t.Errorf("64 entries: %v", err)
	}
	// Count over the limit on the wire, refused before any entry is read.
	if _, err := DecodeListReply(ReplyFrame{body: mustHex(t, "41 00 00 00 ff ff ff ff ff ff ff ff")}); !errors.Is(err, ErrTooLong) {
		t.Errorf("wire count 65: %v", err)
	}
	// Count larger than the entries present.
	if _, err := DecodeListReply(ReplyFrame{body: mustHex(t, "01 00 00 00 ff ff ff ff ff ff ff ff")}); !errors.Is(err, ErrTruncated) {
		t.Errorf("missing entry: %v", err)
	}

	big := Entry{ItemID: 1, Kind: KindFile, Name: strings.Repeat("n", MaxEntryLen)}
	if _, err := EncodeReply(5, &ListReply{NextCursor: CursorLastPage, Entries: []Entry{big}}); !errors.Is(err, ErrTooLong) {
		t.Errorf("entry over 16 KiB: %v", err)
	}

	// The encoder still refuses all of these (Entry.check, unchanged); a
	// decoder only ever meets them from another implementation. Everything
	// but item_id 0 and invalid UTF-8 (a file name is never safe to guess a
	// replacement for, unlike a diagnostic message) is normalized instead of
	// refused (owner's answer 7 and the bullet list after the nine
	// questions).
	badEntries := []struct {
		name    string
		in      Entry
		want    Entry // zero Entry{} for the two still-rejected cases
		decodes bool
	}{
		{"item 0", Entry{Kind: KindFile}, Entry{}, false},
		{"size without flag", Entry{ItemID: 1, Size: 5}, Entry{ItemID: 1}, true},
		{"random without stream", Entry{ItemID: 1, Flags: ItemRandomAccess}, Entry{ItemID: 1}, true},
		{"frozen without stream", Entry{ItemID: 1, Flags: ItemFrozen}, Entry{ItemID: 1}, true},
		{"native name, no enc", Entry{ItemID: 1, NativeName: []byte("x")}, Entry{ItemID: 1}, true},
		{"uri without reference", Entry{ItemID: 1, ReferenceURI: "file:///x"}, Entry{ItemID: 1}, true},
		{"reference without uri", Entry{ItemID: 1, Flags: ItemReference}, Entry{ItemID: 1}, true},
		{"name is not UTF-8", Entry{ItemID: 1, Name: "\xff"}, Entry{}, false},
		{"namespace is not UTF-8", Entry{ItemID: 1, SourceNamespace: "\xc3"}, Entry{}, false},
	}
	for _, c := range badEntries {
		if _, err := EncodeReply(5, &ListReply{NextCursor: CursorLastPage, Entries: []Entry{c.in}}); err == nil {
			t.Errorf("encode %s accepted", c.name)
		}
		// The decoder sees the same entry laid out by hand, bypassing the
		// encoder's own checks (str's UTF-8 validation included, for the two
		// cases that build invalid UTF-8 on purpose).
		var w stackWriter
		w.u64(c.in.ItemID)
		w.u8(c.in.Kind)
		w.u16(c.in.Flags)
		w.u64(c.in.Size)
		w.blob([]byte(c.in.Name))
		w.u8(c.in.NativeEncoding)
		w.blob(c.in.NativeName)
		w.blob([]byte(c.in.ReferenceURI))
		w.blob([]byte(c.in.SourceNamespace))
		b, _ := w.bytes()
		got, err := decodeEntry(b)
		switch {
		case !c.decodes && err == nil:
			t.Errorf("decode %s accepted", c.name)
		case c.decodes && err != nil:
			t.Errorf("decode %s: %v", c.name, err)
		case c.decodes && !reflect.DeepEqual(got, c.want):
			t.Errorf("decode %s: got %+v, want %+v", c.name, got, c.want)
		}
	}

	// A page with one unusable entry (item_id 0) among good ones keeps the
	// rest instead of losing the whole copy over it (owner's answer to the
	// bullet list). The encoder refuses to ever produce this (Entry.check),
	// so the page is laid out by hand, the same way EncodeReply and
	// ListReply.put would have -- one item_id 0 is the only thing another
	// implementation could put here that this f4 itself never would.
	rawEntry := func(e Entry) []byte {
		var w stackWriter
		w.u64(e.ItemID)
		w.u8(e.Kind)
		w.u16(e.Flags)
		w.u64(e.Size)
		w.blob([]byte(e.Name))
		w.u8(e.NativeEncoding)
		w.blob(e.NativeName)
		w.blob([]byte(e.ReferenceURI))
		w.blob([]byte(e.SourceNamespace))
		b, err := w.bytes()
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	var mw stackWriter
	mw.u8(5)
	mw.i8(int8(StatusOK))
	mw.u64(CursorLastPage)
	mw.u32(3)
	mw.blob(rawEntry(Entry{ItemID: 1, Kind: KindFile, Name: "a"}))
	mw.blob(rawEntry(Entry{Kind: KindFile, Name: "bad"})) // item_id 0
	mw.blob(rawEntry(Entry{ItemID: 2, Kind: KindFile, Name: "b"}))
	mixedStack, err := mw.bytes()
	if err != nil {
		t.Fatal(err)
	}
	mixedFrame, err := DecodeReply(mixedStack)
	if err != nil {
		t.Fatal(err)
	}
	mixedList, err := DecodeListReply(mixedFrame)
	if err != nil || len(mixedList.Entries) != 2 || mixedList.Entries[0].ItemID != 1 || mixedList.Entries[1].ItemID != 2 {
		t.Errorf("mixed page: %+v, %v", mixedList, err)
	}

	// A later version may put more fields into an entry; v1 ignores them.
	b, err := entries[0].encode()
	if err != nil {
		t.Fatal(err)
	}
	e, err := decodeEntry(append([]byte{1, 2, 3}, b...))
	if err != nil || !reflect.DeepEqual(e, entries[0]) {
		t.Errorf("entry with a tail: %+v, %v", e, err)
	}
	// But a cut entry is an error.
	if _, err := decodeEntry(b[1:]); err == nil {
		t.Error("cut entry accepted")
	}
}

func TestReadReply(t *testing.T) {
	q := &ReadRequest{Offer: seqID(0), ItemID: 1, Length: 4}
	good := []ReadReply{
		{ObservedSize: 10, Flags: ReadSizeKnown, Data: []byte("abcd")},
		{ObservedSize: 0, Flags: 0, Data: []byte("ab")}, // short read without EOF
		{ObservedSize: 2, Flags: ReadEOF | ReadSizeKnown, Data: nil},
	}
	for _, d := range good {
		stack, err := EncodeReply(1, &d)
		if err != nil {
			t.Fatal(err)
		}
		f, err := DecodeReply(stack)
		if err != nil {
			t.Fatal(err)
		}
		got, err := DecodeReadReply(f, q)
		if err != nil || got.ObservedSize != d.ObservedSize || got.Flags != d.Flags || !bytes.Equal(got.Data, d.Data) {
			t.Errorf("round trip %+v: %+v, %v", d, got, err)
		}
	}

	if _, err := EncodeReply(1, &ReadReply{Data: nil}); !errors.Is(err, ErrInvalid) {
		t.Errorf("empty without EOF: %v", err)
	}
	if _, err := EncodeReply(1, &ReadReply{ObservedSize: 3, Flags: ReadEOF}); !errors.Is(err, ErrInvalid) {
		t.Errorf("observed_size without SIZE_KNOWN: %v", err)
	}

	// More bytes than asked for.
	stack, _ := EncodeReply(1, &ReadReply{Flags: ReadEOF, Data: []byte("abcde")})
	f, _ := DecodeReply(stack)
	if _, err := DecodeReadReply(f, q); !errors.Is(err, ErrTooLong) {
		t.Errorf("five bytes for four: %v", err)
	}
}

func TestEvent(t *testing.T) {
	good := []Event{
		{Binding: seqID(1), Offer: seqID(2), X: 0, Y: 0, Modifiers: 0x1f, Flags: EventModifiersKnown},
		{Binding: seqID(1), Offer: seqID(2), X: -1, Y: -1},
		{X: math.MaxInt16, Y: 5},
	}
	for _, ev := range good {
		stack, err := EncodeEvent(&ev)
		if err != nil {
			t.Fatal(err)
		}
		k, back, _, err := ParseFrame(Frame(FrameEvent, stack, ST))
		if err != nil || k != FrameEvent {
			t.Fatalf("ParseFrame: %v, %v", k, err)
		}
		got, err := DecodeEvent(back)
		if err != nil || got != ev {
			t.Errorf("round trip %+v: %+v, %v", ev, got, err)
		}
		for i := 1; i < len(stack); i++ {
			if _, err := DecodeEvent(stack[i:]); err == nil {
				t.Errorf("cut %d accepted", i)
			}
		}
	}
	if good[1].PositionKnown() || !good[0].PositionKnown() {
		t.Error("PositionKnown")
	}

	// The encoder still refuses all of these (check, unchanged); a decoder
	// only ever meets them from another implementation and normalizes them
	// in place instead (owner's answers 5 and 6).
	bad := []struct {
		name string
		in   Event
		want Event
	}{
		{"reserved flag", Event{Flags: 2}, Event{Flags: 0}},
		{"modifiers not known", Event{Modifiers: 1}, Event{}},
		{"half unknown", Event{X: -1, Y: 3}, Event{X: -1, Y: -1}},
		{"negative", Event{X: -2, Y: -2}, Event{X: -1, Y: -1}},
		{"reserved bit kept with known modifiers",
			Event{Modifiers: 0x1f, Flags: EventModifiersKnown | 2},
			Event{Modifiers: 0x1f, Flags: EventModifiersKnown}},
	}
	for _, c := range bad {
		if _, err := EncodeEvent(&c.in); !errors.Is(err, ErrInvalid) {
			t.Errorf("encode %s: %v", c.name, err)
		}
		var w stackWriter
		w.u8(InputDND)
		w.id(c.in.Binding)
		w.id(c.in.Offer)
		w.i16(c.in.X)
		w.i16(c.in.Y)
		w.u32(c.in.Modifiers)
		w.u16(c.in.Flags)
		b, _ := w.bytes()
		got, err := DecodeEvent(b)
		c.want.Binding, c.want.Offer = c.in.Binding, c.in.Offer
		if err != nil || got != c.want {
			t.Errorf("decode %s: got %+v, %v; want %+v", c.name, got, err, c.want)
		}
	}

	// binding and offer still decode even with a tail below them, so the
	// caller can CLOSE the right offer at once instead of waiting out its
	// lease (owner's answer 1).
	ev := Event{Binding: seqID(1), Offer: seqID(2), X: -1, Y: -1}
	stack, err := EncodeEvent(&ev)
	if err != nil {
		t.Fatal(err)
	}
	tailed := append([]byte{9, 9, 9}, stack...)
	got, err := DecodeEvent(tailed)
	if !errors.Is(err, ErrTrailing) || got.Binding != ev.Binding || got.Offer != ev.Offer {
		t.Errorf("event with a tail: %+v, %v", got, err)
	}

	if _, err := DecodeEvent([]byte{'K'}); !errors.Is(err, ErrNotDND) {
		t.Errorf("another event: %v", err)
	}
}

func TestParseFrame(t *testing.T) {
	stack, _ := EncodeRequest(0, &CloseRequest{Reason: CloseProcessed})
	apc := Frame(FrameRequest, stack, ST)
	if !strings.HasSuffix(apc, "\x1b\\") {
		t.Fatalf("ST APC %q", apc)
	}
	k, back, term, err := ParseFrame(apc)
	if err != nil || k != FrameRequest || term != ST || !bytes.Equal(back, stack) {
		t.Fatalf("ST: %v %v %v", k, term, err)
	}

	// Missing padding is tolerated (§ 14.3's string without "==").
	if k, back, _, err := ParseFrame("\x1b_far2lYQpiAwAAAA\x07"); err != nil || k != FrameReply ||
		!bytes.Equal(back, []byte{'a', '\n', 'b', 3, 0, 0, 0}) {
		t.Errorf("unpadded: %v, % x, %v", k, back, err)
	}

	for _, s := range []string{
		"\x1b_far2l1\x07", "\x1b_far2l0\x07", "\x1b_far2lok\x07", "\x1b_far2lok\x1b\\",
		"\x1b_far2l:\x07", "\x1b_far2l:!!!!\x07", "\x1b_Gf=100\x07", "\x1b_far2l:AAAA",
	} {
		if _, _, _, err := ParseFrame(s); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
}

func TestMaxReadData(t *testing.T) {
	for _, frame := range []uint32{MinMaxFrame, 4097, 4098, 4099, 5000, DefaultMaxFrame} {
		for _, term := range []Terminator{BEL, ST} {
			n := MaxReadData(frame, term)
			if n == 0 {
				t.Fatalf("frame %d: no room", frame)
			}
			fits := func(size uint32) bool {
				stack, err := EncodeReply(1, &ReadReply{Flags: ReadEOF, Data: make([]byte, size)})
				if err != nil {
					t.Fatal(err)
				}
				return int64(FrameLen(FrameReply, len(stack), term)) <= int64(frame)
			}
			if !fits(n) || fits(n+1) {
				t.Errorf("frame %d %q: MaxReadData %d is not the largest that fits", frame, term.String(), n)
			}
		}
	}
	if MaxReadData(20, BEL) != 0 || MaxReadData(0, ST) != 0 {
		t.Error("tiny frames must leave no room")
	}
	// The proposed profile: 32 KiB of data fit in a 64 KiB frame (§ 7).
	if MaxReadData(DefaultMaxFrame, ST) < DefaultMaxChunk {
		t.Error("the default chunk does not fit the default frame")
	}
}

// TestMaxErrorText checks the exact byte counts the owner's answer 3 works
// out by hand for BindFrameLimit (512): 369 bytes of message at ST, 372 at
// BEL, and that the result is in fact the largest that still fits.
func TestMaxErrorText(t *testing.T) {
	cases := []struct {
		term Terminator
		want int
	}{{ST, 369}, {BEL, 372}}
	for _, c := range cases {
		got := MaxErrorText(BindFrameLimit, c.term)
		if got != c.want {
			t.Errorf("MaxErrorText(%d, %q) = %d, want %d", BindFrameLimit, c.term.String(), got, c.want)
		}
		fits := func(n int) bool {
			stack, err := EncodeError(1, StatusIOError, strings.Repeat("x", n))
			if err != nil {
				t.Fatal(err)
			}
			return FrameLen(FrameReply, len(stack), c.term) <= BindFrameLimit
		}
		if !fits(got) || fits(got+1) {
			t.Errorf("MaxErrorText(%d, %q) = %d is not the largest that fits", BindFrameLimit, c.term.String(), got)
		}
	}
	if MaxErrorText(10, BEL) != 0 || MaxErrorText(0, ST) != 0 {
		t.Error("tiny frames must leave no room")
	}
}
