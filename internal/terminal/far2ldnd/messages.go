package far2ldnd

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// Identifiers proposed by the specification (§ 6). They are not yet agreed
// with far2l upstream; the symbolic names are the normative part.
const (
	InteractDND byte = 'd' // FARTTY_INTERACT_DND, the command family
	InputDND    byte = 'D' // FARTTY_INPUT_DND, the event

	SubBind  byte = 'b'
	SubList  byte = 'l'
	SubRead  byte = 'r'
	SubClose byte = 'c'
)

// Version is the only protocol version this codec speaks.
const Version uint16 = 1

// Status is the second field of every reply (§ 6).
type Status int8

const (
	StatusOK          Status = 1  // success
	StatusIOError     Status = 0  // I/O or internal error
	StatusDenied      Status = -1 // access denied
	StatusOfferGone   Status = -2 // offer revoked, expired or unknown
	StatusUnknownItem Status = -3 // unknown item, parent or cursor
	StatusBadRequest  Status = -4 // malformed request or range
	StatusUnsupported Status = -5 // representation or version not supported
	StatusChanged     Status = -6 // the source changed
	StatusLimit       Status = -7 // negotiated limit exceeded / busy
	StatusCancelled   Status = -8 // request cancelled by closing the offer
)

// Feature bits negotiated by BIND (§ 6.1).
const (
	FeatureStream    uint32 = 1
	FeatureReference uint32 = 2
)

// Item kinds and flags of a LIST entry (§ 6.3).
const (
	KindFile uint8 = 1

	ItemReference    uint16 = 1
	ItemStream       uint16 = 2
	ItemRandomAccess uint16 = 4
	ItemFrozen       uint16 = 8
	ItemSizeKnown    uint16 = 16

	NativeNone     uint8 = 0
	NativePOSIX    uint8 = 1
	NativeUTF16LE  uint8 = 2
	CursorFirst          = uint64(0)
	CursorLastPage       = uint64(math.MaxUint64) // next_cursor of the last page
)

// Flags of a READ reply (§ 6.4).
const (
	ReadEOF       uint8 = 1
	ReadSizeKnown uint8 = 2
)

// Reasons of CLOSE (§ 6.5).
const (
	CloseRejected  uint8 = 0
	CloseProcessed uint8 = 1
	CloseCancelled uint8 = 2
	CloseFailed    uint8 = 3
)

// EventModifiersKnown is bit 0 of INPUT_DND event_flags (§ 6.2).
const EventModifiersKnown uint16 = 1

// Limits of the specification (§ 6, § 6.3, § 7).
const (
	MaxMessageLen   = 1024      // diagnostic text of an error reply
	MaxListEntries  = 64        // entries on one LIST page
	MaxEntryLen     = 16 * 1024 // one LIST entry blob
	MinMaxFrame     = 4096      // lower bound of the negotiated max_frame
	MinMaxChunk     = 1         // lower bound of the negotiated max_chunk
	MinWindow       = 1         // negotiated window range
	MaxWindow       = 32        //
	BindFrameLimit  = 512       // BIND and its reply, before negotiation
	DefaultMaxFrame = 65536     // proposed initial profile
	DefaultMaxChunk = 32768     //
	DefaultWindow   = 4         //
)

// StatusError is a reply whose status is not StatusOK.
type StatusError struct {
	Status  Status
	Message string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("far2ldnd: status %d: %s", e.Status, e.Message)
}

// StatusFor is the reply status for a request DecodeRequest refused. An
// unsupported version or subcommand is a well-formed request this server
// merely cannot serve (-5, StatusUnsupported): the terminal understands DND
// (an empty reply, ErrNoDND, is reserved for "no DND at all" and would
// mislead a client that only asked for something this version does not
// have); anything that does not parse is StatusBadRequest (-4, owner's
// answers 1 and 2).
func StatusFor(err error) Status {
	switch {
	case errors.Is(err, ErrUnsupportedVersion), errors.Is(err, ErrUnknownSubcommand):
		return StatusUnsupported
	default:
		return StatusBadRequest
	}
}

// Request is one of *BindRequest, *ListRequest, *ReadRequest, *CloseRequest.
type Request interface {
	Subcommand() byte
	// noReplyAllowed reports whether RID 0 (no reply) is allowed (§ 6).
	noReplyAllowed() bool
	put(w *stackWriter)
	pop(r *stackReader)
	check() error
}

// BindRequest switches drop reception on or off (§ 6.1).
type BindRequest struct {
	Version        uint16
	Enable         bool
	Binding        ID
	MaxFrame       uint32
	MaxChunk       uint32
	Window         uint16
	WantedFeatures uint32
}

// ListRequest asks for one page of an offer (§ 6.3).
type ListRequest struct {
	Offer    ID
	ParentID uint64
	Cursor   uint64
}

// ReadRequest asks for one range of an item (§ 6.4).
type ReadRequest struct {
	Offer  ID
	ItemID uint64
	Offset uint64
	Length uint32
}

// CloseRequest releases an offer (§ 6.5).
type CloseRequest struct {
	Offer  ID
	Reason uint8
}

func (*BindRequest) Subcommand() byte  { return SubBind }
func (*ListRequest) Subcommand() byte  { return SubList }
func (*ReadRequest) Subcommand() byte  { return SubRead }
func (*CloseRequest) Subcommand() byte { return SubClose }

func (q *BindRequest) noReplyAllowed() bool { return !q.Enable }
func (*ListRequest) noReplyAllowed() bool   { return false }
func (*ReadRequest) noReplyAllowed() bool   { return false }
func (*CloseRequest) noReplyAllowed() bool  { return true }

func (q *BindRequest) put(w *stackWriter) {
	w.u16(q.Version)
	w.u8(boolByte(q.Enable))
	w.id(q.Binding)
	w.u32(q.MaxFrame)
	w.u32(q.MaxChunk)
	w.u16(q.Window)
	w.u32(q.WantedFeatures)
}

func (q *BindRequest) pop(r *stackReader) {
	q.Version = r.u16()
	if r.err == nil && q.Version != Version {
		// The layout of the rest belongs to that version; do not guess it.
		r.err = fmt.Errorf("%w: %d", ErrUnsupportedVersion, q.Version)
		return
	}
	switch r.u8() {
	case 0:
		q.Enable = false
	case 1:
		q.Enable = true
	default:
		r.invalid("BIND enable is neither 0 nor 1")
	}
	q.Binding = r.id()
	q.MaxFrame = r.u32()
	q.MaxChunk = r.u32()
	q.Window = r.u16()
	q.WantedFeatures = r.u32()
}

// check validates a BIND request itself, on either side of the wire: what
// it asks for is a profile to negotiate down, not a ceiling the request
// must already fit under (owner's answer 4). Only the protocol's own floor
// applies here; checkLimits' upper bound on window is a rule of the
// *reply* alone (see its own comment) and dndBind (far2l_dnd.go) already
// clamps max_frame/max_chunk down unconditionally with min() and never
// even reads q.Window, so there is nothing an upper bound here would
// protect.
func (q *BindRequest) check() error {
	if q.Version != Version {
		return fmt.Errorf("%w: %d", ErrUnsupportedVersion, q.Version)
	}
	if !q.Enable {
		if q.MaxFrame != 0 || q.MaxChunk != 0 || q.Window != 0 || q.WantedFeatures != 0 {
			return fmt.Errorf("%w: BIND disable with non-zero limits or features", ErrInvalid)
		}
		return nil
	}
	return checkRequestLimits(q.MaxFrame, q.MaxChunk, q.Window)
}

// checkRequestLimits is the floor a BIND request's own numbers must clear.
func checkRequestLimits(frame, chunk uint32, window uint16) error {
	switch {
	case frame < MinMaxFrame:
		return fmt.Errorf("%w: max_frame %d below %d", ErrInvalid, frame, MinMaxFrame)
	case chunk < MinMaxChunk:
		return fmt.Errorf("%w: max_chunk %d below %d", ErrInvalid, chunk, MinMaxChunk)
	case window < MinWindow:
		return fmt.Errorf("%w: window %d below %d", ErrInvalid, window, MinWindow)
	}
	return nil
}

// checkLimits is what a BIND *reply* must satisfy, on either side of the
// wire: unlike the request, the granted window is capped at MaxWindow, a
// structural limit of this side of the channel (255 RIDs shared with
// clipboard and image far2l calls, owner's answer 4) rather than a profile
// either side is free to exceed and let the other clamp.
func checkLimits(frame, chunk uint32, window uint16) error {
	switch {
	case frame < MinMaxFrame:
		return fmt.Errorf("%w: max_frame %d below %d", ErrInvalid, frame, MinMaxFrame)
	case chunk < MinMaxChunk:
		return fmt.Errorf("%w: max_chunk %d below %d", ErrInvalid, chunk, MinMaxChunk)
	case window < MinWindow || window > MaxWindow:
		return fmt.Errorf("%w: window %d outside %d..%d", ErrInvalid, window, MinWindow, MaxWindow)
	}
	return nil
}

func (q *ListRequest) put(w *stackWriter) {
	w.id(q.Offer)
	w.u64(q.ParentID)
	w.u64(q.Cursor)
}

func (q *ListRequest) pop(r *stackReader) {
	q.Offer = r.id()
	q.ParentID = r.u64()
	q.Cursor = r.u64()
}

func (*ListRequest) check() error { return nil }

func (q *ReadRequest) put(w *stackWriter) {
	w.id(q.Offer)
	w.u64(q.ItemID)
	w.u64(q.Offset)
	w.u32(q.Length)
}

func (q *ReadRequest) pop(r *stackReader) {
	q.Offer = r.id()
	q.ItemID = r.u64()
	q.Offset = r.u64()
	q.Length = r.u32()
}

// check covers what one READ can be judged by alone; the upper bound of the
// length is the negotiated max_chunk and belongs to the dispatcher.
func (q *ReadRequest) check() error {
	switch {
	case q.ItemID == 0:
		return fmt.Errorf("%w: READ of item 0", ErrInvalid)
	case q.Length == 0:
		// Unlike FISH+, zero does not mean "read everything".
		return fmt.Errorf("%w: READ of zero bytes", ErrInvalid)
	case q.Offset > math.MaxUint64-uint64(q.Length):
		return fmt.Errorf("%w: READ offset+length overflows", ErrInvalid)
	}
	return nil
}

func (q *CloseRequest) put(w *stackWriter) {
	w.id(q.Offer)
	w.u8(q.Reason)
}

func (q *CloseRequest) pop(r *stackReader) {
	q.Offer = r.id()
	q.Reason = r.u8()
}

func (q *CloseRequest) check() error {
	if q.Reason > CloseFailed {
		return fmt.Errorf("%w: CLOSE reason %d", ErrInvalid, q.Reason)
	}
	return nil
}

func boolByte(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}

// EncodeRequest builds the stack of an interaction request:
// RID, 'd', subcommand, arguments. RID 0 asks for no reply and is allowed
// only for CLOSE and for switching a binding off.
func EncodeRequest(rid uint8, q Request) ([]byte, error) {
	if err := checkRID(rid, q); err != nil {
		return nil, err
	}
	if err := q.check(); err != nil {
		return nil, err
	}
	var w stackWriter
	w.u8(rid)
	w.u8(InteractDND)
	w.u8(q.Subcommand())
	q.put(&w)
	return w.bytes()
}

func checkRID(rid uint8, q Request) error {
	if rid == 0 && !q.noReplyAllowed() {
		return fmt.Errorf("%w: RID 0 for DND/%c", ErrInvalid, q.Subcommand())
	}
	return nil
}

// DecodeRequest parses the stack of an interaction request. The RID is
// returned whenever it could be read, also with an error, so that the
// terminal can answer with StatusFor(err). ErrNotDND means the stack belongs
// to another family and should go to its own handler.
func DecodeRequest(stack []byte) (rid uint8, q Request, err error) {
	r := stackReader{b: stack}
	rid = r.u8()
	cmd := r.u8()
	if r.err != nil {
		return rid, nil, r.err
	}
	if cmd != InteractDND {
		return rid, nil, ErrNotDND
	}
	sub := r.u8()
	switch sub {
	case SubBind:
		q = new(BindRequest)
	case SubList:
		q = new(ListRequest)
	case SubRead:
		q = new(ReadRequest)
	case SubClose:
		q = new(CloseRequest)
	default:
		if r.err != nil {
			return rid, nil, r.err
		}
		return rid, nil, fmt.Errorf("%w: %q", ErrUnknownSubcommand, sub)
	}
	q.pop(&r)
	if err := r.end(); err != nil {
		return rid, q, err
	}
	if err := checkRID(rid, q); err != nil {
		return rid, q, err
	}
	// CLOSE is a cleanup barrier that only really needs the offer; refusing
	// it over a reason outside the protocol's own range would leave the
	// offer occupied until its lease expires, and a RID-0 sender would never
	// even learn of the refusal (owner's answer 9). Clamp before the shared
	// check below, which stays strict for whoever composes the request with
	// the encoder.
	if c, ok := q.(*CloseRequest); ok && c.Reason > CloseFailed {
		c.Reason = CloseFailed
	}
	return rid, q, q.check()
}

// BindReply is the body of a successful BIND that switched reception on.
type BindReply struct {
	Version     uint16
	Binding     ID
	MaxFrame    uint32
	MaxChunk    uint32
	Window      uint16
	IdleSeconds uint32
	Features    uint32
}

// Entry is one object of a LIST page (§ 6.3). The entry is a stack of its own
// inside a blob.
type Entry struct {
	ItemID          uint64
	Kind            uint8
	Flags           uint16
	Size            uint64 // meaningful with ItemSizeKnown, zero otherwise
	Name            string // UTF-8 display name, never a destination path
	NativeEncoding  uint8
	NativeName      []byte
	ReferenceURI    string
	SourceNamespace string
}

// ListReply is the body of a successful LIST.
type ListReply struct {
	NextCursor uint64
	Entries    []Entry
}

// ReadReply is the body of a successful READ.
type ReadReply struct {
	ObservedSize uint64
	Flags        uint8
	Data         []byte
}

// Reply is one of *BindReply, *ListReply, *ReadReply, or nil for the empty
// body of CLOSE and of switching a binding off.
type Reply interface {
	put(w *stackWriter) error
}

func (b *BindReply) put(w *stackWriter) error {
	if b.Version != Version {
		return fmt.Errorf("%w: %d", ErrUnsupportedVersion, b.Version)
	}
	if err := checkLimits(b.MaxFrame, b.MaxChunk, b.Window); err != nil {
		return err
	}
	if b.Features&(FeatureStream|FeatureReference) == 0 {
		return fmt.Errorf("%w: BIND reply without STREAM or REFERENCE", ErrInvalid)
	}
	w.u16(b.Version)
	w.id(b.Binding)
	w.u32(b.MaxFrame)
	w.u32(b.MaxChunk)
	w.u16(b.Window)
	w.u32(b.IdleSeconds)
	w.u32(b.Features)
	return nil
}

func (l *ListReply) put(w *stackWriter) error {
	if err := checkPage(l.NextCursor, uint64(len(l.Entries))); err != nil {
		return err
	}
	w.u64(l.NextCursor)
	w.u32(uint32(len(l.Entries))) //nolint:gosec // at most MaxListEntries, checked above
	for i := range l.Entries {
		b, err := l.Entries[i].encode()
		if err != nil {
			return fmt.Errorf("entry %d: %w", i, err)
		}
		w.blob(b)
	}
	return nil
}

func checkPage(next, count uint64) error {
	if count > MaxListEntries {
		return fmt.Errorf("%w: %d LIST entries, limit %d", ErrTooLong, count, MaxListEntries)
	}
	if count == 0 && next != CursorLastPage {
		return fmt.Errorf("%w: empty LIST page that is not the last", ErrInvalid)
	}
	return nil
}

func (e *Entry) check() error {
	switch {
	case e.ItemID == 0:
		return fmt.Errorf("%w: item_id 0", ErrInvalid)
	case e.Flags&ItemSizeKnown == 0 && e.Size != 0:
		return fmt.Errorf("%w: size without SIZE_KNOWN", ErrInvalid)
	case e.Flags&(ItemRandomAccess|ItemFrozen) != 0 && e.Flags&ItemStream == 0:
		return fmt.Errorf("%w: RANDOM_ACCESS or FROZEN without STREAM", ErrInvalid)
	case e.NativeEncoding == NativeNone && len(e.NativeName) != 0:
		return fmt.Errorf("%w: native_name without native_encoding", ErrInvalid)
	case e.Flags&ItemReference == 0 && e.ReferenceURI != "":
		return fmt.Errorf("%w: reference_uri without REFERENCE", ErrInvalid)
	case e.Flags&ItemReference != 0 && e.ReferenceURI == "":
		// A flag without a URI promises an empty representation (owner's
		// answer 7); decodeEntry's normalize clears the flag instead of
		// refusing this from another implementation, but this encoder never
		// produces it in the first place.
		return fmt.Errorf("%w: REFERENCE without reference_uri", ErrInvalid)
	}
	return nil
}

func (e *Entry) encode() ([]byte, error) {
	if err := e.check(); err != nil {
		return nil, err
	}
	var w stackWriter
	w.u64(e.ItemID)
	w.u8(e.Kind)
	w.u16(e.Flags)
	w.u64(e.Size)
	w.str(e.Name)
	w.u8(e.NativeEncoding)
	w.blob(e.NativeName)
	w.str(e.ReferenceURI)
	w.str(e.SourceNamespace)
	b, err := w.bytes()
	if err == nil && len(b) > MaxEntryLen {
		return nil, fmt.Errorf("%w: LIST entry of %d bytes, limit %d", ErrTooLong, len(b), MaxEntryLen)
	}
	return b, err
}

// normalize fixes up, in place, whatever an entry's fields disagree on but
// that carries no meaning of its own once dropped (owner's answer 7): the
// encoder (check and encode, above) never produces any of this, so v1 f4
// only ever meets it from another implementation. Feature-negotiated
// consistency -- a representation flag this binding never asked for in
// BIND -- is not this package's to enforce either: decodeEntry has no
// access to what was granted, so that check belongs to whoever consumes
// ListReply with the BindReply already in hand (DNDClient, step 4).
func (e *Entry) normalize() {
	if e.Flags&ItemSizeKnown == 0 {
		e.Size = 0
	}
	if e.Flags&ItemStream == 0 {
		e.Flags &^= ItemRandomAccess | ItemFrozen
	}
	if e.NativeEncoding == NativeNone {
		e.NativeName = nil
	}
	switch {
	case e.Flags&ItemReference == 0:
		e.ReferenceURI = ""
	case e.ReferenceURI == "":
		e.Flags &^= ItemReference
	}
}

// decodeEntry parses one entry blob. Bytes left after the known fields are a
// tail a later version may add; v1 ignores it (§ 6.3). item_id 0 is the one
// field normalize cannot fix up -- the entry cannot be told apart from any
// other later, so DecodeListReply drops just this entry and keeps the rest
// of the page (owner's answer to the bullet list following the nine
// questions).
func decodeEntry(b []byte) (Entry, error) {
	r := stackReader{b: b}
	var e Entry
	e.ItemID = r.u64()
	e.Kind = r.u8()
	e.Flags = r.u16()
	e.Size = r.u64()
	e.Name = r.str(MaxEntryLen)
	e.NativeEncoding = r.u8()
	e.NativeName = r.blob(MaxEntryLen)
	e.ReferenceURI = r.str(MaxEntryLen)
	e.SourceNamespace = r.str(MaxEntryLen)
	if r.err != nil {
		return Entry{}, r.err
	}
	if len(e.NativeName) == 0 {
		e.NativeName = nil
	}
	if e.ItemID == 0 {
		return Entry{}, fmt.Errorf("%w: item_id 0", ErrInvalid)
	}
	e.normalize()
	return e, nil
}

func (d *ReadReply) put(w *stackWriter) error {
	if err := d.check(); err != nil {
		return err
	}
	w.u64(d.ObservedSize)
	w.u8(d.Flags)
	w.blob(d.Data) // length:u32, data:bytes[length]
	return nil
}

func (d *ReadReply) check() error {
	switch {
	case d.Flags&ReadSizeKnown == 0 && d.ObservedSize != 0:
		return fmt.Errorf("%w: observed_size without SIZE_KNOWN", ErrInvalid)
	case len(d.Data) == 0 && d.Flags&ReadEOF == 0:
		return fmt.Errorf("%w: empty READ reply without EOF", ErrInvalid)
	}
	return nil
}

// EncodeReply builds the stack of a successful reply: RID, status 1, body.
// A nil body is the empty body of CLOSE and of switching a binding off.
func EncodeReply(rid uint8, body Reply) ([]byte, error) {
	if rid == 0 {
		return nil, fmt.Errorf("%w: reply to RID 0", ErrInvalid)
	}
	var w stackWriter
	w.u8(rid)
	w.i8(int8(StatusOK))
	if body != nil {
		if err := body.put(&w); err != nil {
			return nil, err
		}
	}
	return w.bytes()
}

// TruncateText cuts s to at most limit bytes on a UTF-8 rune boundary,
// replacing invalid UTF-8 with "?" first. A diagnostic message carries no
// meaning of its own beyond being readable (unlike a LIST entry's file
// name, which TruncateText is never used on): shortening it is more useful
// than refusing to send it at all (owner's "общий принцип" and answer 3).
func TruncateText(s string, limit int) string {
	s = strings.ToValidUTF8(s, "?")
	if len(s) <= limit {
		return s
	}
	s = s[:limit]
	for len(s) > 0 && !utf8.ValidString(s) {
		s = s[:len(s)-1]
	}
	return s
}

// EncodeError builds the stack of a failed reply: RID, status, message.
// message is shortened to this package's own flat ceiling (MaxMessageLen)
// rather than refused (owner's answer 3): a caller that also knows the
// frame this reply has to fit -- BindFrameLimit before BIND, the negotiated
// max_frame after -- shortens further first with MaxErrorText and
// TruncateText, since EncodeError itself has no way to learn that number.
// An EncodeError that could still fail over its message would leave the
// caller's RID hung forever, unable to tell it the request failed at all.
func EncodeError(rid uint8, status Status, message string) ([]byte, error) {
	switch {
	case rid == 0:
		return nil, fmt.Errorf("%w: reply to RID 0", ErrInvalid)
	case status == StatusOK:
		return nil, fmt.Errorf("%w: error reply with status 1", ErrInvalid)
	}
	var w stackWriter
	w.u8(rid)
	w.i8(int8(status))
	w.str(TruncateText(message, MaxMessageLen))
	return w.bytes()
}

// ReplyFrame is a reply split into its header and its still undecoded body.
// The body's layout depends on the request, so it is decoded by one of the
// Decode*Reply functions once the RID has been matched to its request.
type ReplyFrame struct {
	RID    uint8
	Status Status
	body   []byte
}

// DecodeReply parses the header of a reply. An error reply comes back as a
// *StatusError (with the frame, so the RID can still be released); a reply
// that is empty after the RID is ErrNoDND.
func DecodeReply(stack []byte) (ReplyFrame, error) {
	r := stackReader{b: stack}
	f := ReplyFrame{RID: r.u8()}
	if r.err != nil {
		return f, r.err
	}
	if len(r.b) == 0 {
		return f, ErrNoDND
	}
	f.Status = Status(r.i8())
	if f.Status != StatusOK {
		msg := r.str(MaxMessageLen)
		if err := r.end(); err != nil {
			return f, err
		}
		return f, &StatusError{Status: f.Status, Message: msg}
	}
	f.body = r.b
	return f, nil
}

// DecodeBindReply parses and checks the reply to q (§ 6.1): the version and
// the binding must be echoed, every limit must lie between the protocol
// minimum and what q asked for, and the features must be a non-empty subset
// of the wanted ones. Switching off has an empty body.
func DecodeBindReply(f ReplyFrame, q *BindRequest) (BindReply, error) {
	r := stackReader{b: f.body}
	var b BindReply
	if !q.Enable {
		return b, r.end()
	}
	b.Version = r.u16()
	b.Binding = r.id()
	b.MaxFrame = r.u32()
	b.MaxChunk = r.u32()
	b.Window = r.u16()
	b.IdleSeconds = r.u32()
	b.Features = r.u32()
	if err := r.end(); err != nil {
		return BindReply{}, err
	}
	switch {
	case b.Version != q.Version:
		return b, fmt.Errorf("%w: BIND reply version %d, asked %d", ErrInvalid, b.Version, q.Version)
	case b.Binding != q.Binding:
		return b, fmt.Errorf("%w: BIND reply for another binding", ErrInvalid)
	case b.MaxFrame > q.MaxFrame || b.MaxChunk > q.MaxChunk || b.Window > q.Window:
		return b, fmt.Errorf("%w: BIND reply limits above the requested ones", ErrInvalid)
	case b.Features&^q.WantedFeatures != 0:
		return b, fmt.Errorf("%w: BIND reply features %#x beyond wanted %#x", ErrInvalid, b.Features, q.WantedFeatures)
	case b.Features&(FeatureStream|FeatureReference) == 0:
		return b, fmt.Errorf("%w: BIND reply without STREAM or REFERENCE", ErrInvalid)
	}
	if err := checkLimits(b.MaxFrame, b.MaxChunk, b.Window); err != nil {
		return b, err
	}
	// A client does not yet know which terminator its future READ replies
	// will actually close with, so it has to assume the worse of the two
	// (ST, one byte longer than BEL) when checking that max_chunk's reply
	// can even fit inside max_frame (owner's answer 4).
	if b.MaxChunk > MaxReadData(b.MaxFrame, ST) {
		return b, fmt.Errorf("%w: BIND reply max_chunk %d does not fit max_frame %d", ErrInvalid, b.MaxChunk, b.MaxFrame)
	}
	return b, nil
}

// DecodeListReply parses a LIST page.
func DecodeListReply(f ReplyFrame) (ListReply, error) {
	r := stackReader{b: f.body}
	var l ListReply
	l.NextCursor = r.u64()
	count := r.u32()
	if r.err != nil {
		return ListReply{}, r.err
	}
	if err := checkPage(l.NextCursor, uint64(count)); err != nil {
		return ListReply{}, err
	}
	l.Entries = make([]Entry, 0, count)
	for i := uint32(0); i < count; i++ {
		b := r.blob(MaxEntryLen)
		if r.err != nil {
			// The blob itself does not parse: a framing problem the rest of
			// the page cannot be trusted around, unlike one bad entry below.
			return ListReply{}, fmt.Errorf("entry %d: %w", i, r.err)
		}
		e, err := decodeEntry(b)
		if err != nil {
			// One unusable entry (item_id 0 today, decodeEntry's only
			// remaining rejection once normalize absorbs the rest) does not
			// have to cost the whole page and the copy it was fetched for
			// (owner's "отказ на приёме стоит дорого"): skip it and keep
			// going.
			continue
		}
		l.Entries = append(l.Entries, e)
	}
	if err := r.end(); err != nil {
		return ListReply{}, err
	}
	return l, nil
}

// DecodeReadReply parses the reply to q; it may carry at most q.Length
// bytes (r.blob's limit above enforces that on the wire; a reply longer
// than asked is still ErrTooLong). observed_size without SIZE_KNOWN carries
// no meaning and is normalized to 0 rather than refused (owner's answer to
// the bullet list); an empty reply without EOF is still an error, because
// it would stop a reader's progress dead without saying so (d.check, used
// by the encoder, keeps enforcing both).
func DecodeReadReply(f ReplyFrame, q *ReadRequest) (ReadReply, error) {
	r := stackReader{b: f.body}
	var d ReadReply
	d.ObservedSize = r.u64()
	d.Flags = r.u8()
	d.Data = r.blob(uint64(q.Length))
	if err := r.end(); err != nil {
		return ReadReply{}, err
	}
	if d.Flags&ReadSizeKnown == 0 {
		d.ObservedSize = 0
	}
	if len(d.Data) == 0 && d.Flags&ReadEOF == 0 {
		return ReadReply{}, fmt.Errorf("%w: empty READ reply without EOF", ErrInvalid)
	}
	return d, nil
}

// DecodeEmptyReply checks the empty body of CLOSE.
func DecodeEmptyReply(f ReplyFrame) error {
	r := stackReader{b: f.body}
	return r.end()
}

// Event is INPUT_DND (§ 6.2): a drop happened; the event carries neither the
// list nor the data.
type Event struct {
	Binding   ID
	Offer     ID
	X, Y      int16 // zero-based cell; (-1,-1) when unknown
	Modifiers uint32
	Flags     uint16
}

func (ev *Event) check() error {
	switch {
	case ev.Flags&^EventModifiersKnown != 0:
		return fmt.Errorf("%w: reserved event_flags %#x", ErrInvalid, ev.Flags)
	case ev.Flags&EventModifiersKnown == 0 && ev.Modifiers != 0:
		return fmt.Errorf("%w: modifiers without the known bit", ErrInvalid)
	case (ev.X < 0 || ev.Y < 0) && (ev.X != -1 || ev.Y != -1):
		return fmt.Errorf("%w: position (%d,%d)", ErrInvalid, ev.X, ev.Y)
	}
	return nil
}

// PositionKnown reports whether the event carries a cell position.
func (ev *Event) PositionKnown() bool { return ev.X >= 0 && ev.Y >= 0 }

// normalize fixes up, in place, whatever an event's fields do not carry
// meaning in rather than refusing the event over them (owner's answers 5
// and 6): a sender must never produce any of this (check, above, still
// guards EncodeEvent), so v1 f4 only meets it from another implementation,
// or from a later version's reserved bits it does not understand yet --
// masking them off is the safe default either way. Any coordinate the
// terminal's own current size may since have outgrown (a resize racing the
// drop) is for the application to treat as unknown too; this package has no
// notion of a grid to compare against.
func (ev *Event) normalize() {
	ev.Flags &= EventModifiersKnown
	if ev.Flags&EventModifiersKnown == 0 {
		ev.Modifiers = 0
	}
	if ev.X < 0 || ev.Y < 0 {
		ev.X, ev.Y = -1, -1
	}
}

// EncodeEvent builds the stack of INPUT_DND.
func EncodeEvent(ev *Event) ([]byte, error) {
	if err := ev.check(); err != nil {
		return nil, err
	}
	var w stackWriter
	w.u8(InputDND)
	w.id(ev.Binding)
	w.id(ev.Offer)
	w.i16(ev.X)
	w.i16(ev.Y)
	w.u32(ev.Modifiers)
	w.u16(ev.Flags)
	return w.bytes()
}

// DecodeEvent parses the stack of an f2l event. ErrNotDND means another
// event, for its own handler. binding and offer sit at the top of the
// stack (the first two fields popped after the event code) and decode
// correctly even when a tail is left over below them, so a trailing-bytes
// error (owner's answer 1) still comes back with them set: the application
// can CLOSE(offer, 3) at once instead of leaving the offer occupied until
// its lease expires.
func DecodeEvent(stack []byte) (Event, error) {
	r := stackReader{b: stack}
	code := r.u8()
	if r.err != nil {
		return Event{}, r.err
	}
	if code != InputDND {
		return Event{}, ErrNotDND
	}
	var ev Event
	ev.Binding = r.id()
	ev.Offer = r.id()
	ev.X = r.i16()
	ev.Y = r.i16()
	ev.Modifiers = r.u32()
	ev.Flags = r.u16()
	if r.err != nil {
		return Event{}, r.err
	}
	if err := r.end(); err != nil {
		return Event{Binding: ev.Binding, Offer: ev.Offer}, err
	}
	ev.normalize()
	return ev, nil
}
