package terminal

// The terminal side of the far2l drag-and-drop protocol (unxed/f4#1628,
// specification v0.2): f4 as the terminal of the programs running inside it.
//
// A program switches drop reception on with DND/BIND. When the user drops
// something on the terminal, the owner of the view hands the dropped objects
// over as a DNDSource through OfferDrop; the terminal publishes them as an
// offer and tells the program with one INPUT_DND event. The program then pulls
// the list with DND/LIST and byte ranges with DND/READ, and releases the offer
// with DND/CLOSE. The wire format lives in far2ldnd; this file keeps the
// state: the binding, the offers, their leases and read positions.
//
// Requests are taken in the order they arrived by a worker that exists only
// while there is something to serve. LIST and READ are served concurrently, up
// to the window the binding was granted (§ 7: the terminal grants at most
// dndWindow, and never more than the program asked for); BIND and CLOSE are
// barriers -- they wait for everything before them to finish and nothing
// starts until they are done -- so a CLOSE really means "no read of this
// offer is running any more" and a new BIND never overlaps the old binding's
// reads. A sequential item is only ever read after the previous range came
// back (the program's obligation, and checked by the offset rule in
// dndRead), so concurrent READs are of different items or of random-access
// ones. The source is read outside the state lock, so a slow source never
// stalls the GUI or the parser, and a revocation that lands meanwhile turns
// the READ into -8.

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/unxed/f4/internal/terminal/far2ldnd"
)

// DNDSource is the read-only provider behind one drop: the objects the user
// dropped, fixed for the lifetime of the offer (§ 5, § 9.2). The terminal
// never asks it for a path; it reads only the items it listed.
type DNDSource interface {
	// Entries lists the dropped objects in the order LIST pages them. It is
	// called once, by OfferDrop.
	Entries() []far2ldnd.Entry
	// ReadAt reads up to len(p) bytes of the item at off. A short read
	// without eof is allowed; n == 0 is only allowed with eof. An error
	// wrapping ErrDNDSourceChanged is reported as "source changed" (-6).
	ReadAt(itemID uint64, p []byte, off uint64) (n int, eof bool, err error)
	// Close releases the source. The terminal calls it exactly once, when
	// the offer ends, and never while a ReadAt is running.
	Close()
}

var (
	// ErrDNDSourceChanged is what a DNDSource wraps when it detects that the
	// object changed under the transfer.
	ErrDNDSourceChanged = errors.New("terminal: drop source changed")
	// ErrDNDNotBound: no program in the terminal accepts drops.
	ErrDNDNotBound = errors.New("terminal: no DND binding")
	// ErrDNDTooManyOffers: the local limit of live offers is reached.
	ErrDNDTooManyOffers = errors.New("terminal: too many live DND offers")
)

// The terminal's own profile (§ 7).
const (
	dndFeatures    = far2ldnd.FeatureStream | far2ldnd.FeatureReference
	dndWindow      = 4 // LIST/READ requests served at once, at most
	dndIdleSeconds = 600
	dndMaxOffers   = 8
	dndQueueLimit  = 64 // requests waiting for the worker
)

// dndServer is the DnD state of one terminal view. Its zero value is ready.
type dndServer struct {
	// mu guards the state below and serializes the DnD frames written to
	// the program, so a BIND reply is always written before the first event
	// for that binding (§ 6.1).
	mu      sync.Mutex
	bound   bool
	request far2ldnd.BindRequest // the BIND that is in force, as asked
	granted far2ldnd.BindReply   // and as answered
	offers  map[far2ldnd.ID]*dndOffer

	// Hooks for tests.
	now   func() time.Time
	newID func() (far2ldnd.ID, error)

	// qmu guards the queue of decoded requests. It is separate from mu so
	// the parser never waits behind a write to the program.
	qmu     sync.Mutex
	queue   [][]byte
	running bool

	// pmu guards inflight, the LIST/READ requests being served right now;
	// pcond wakes the worker when one finishes.
	pmu      sync.Mutex
	pcond    *sync.Cond
	inflight int
}

// dndReadTimeout bounds one DNDSource.ReadAt. It is a variable so that tests
// can shorten it.
var dndReadTimeout = 30 * time.Second

type dndOffer struct {
	src      DNDSource
	entries  []far2ldnd.Entry // masked to the negotiated features
	index    map[uint64]int   // item_id -> entries index
	next     map[uint64]uint64
	lastUse  time.Time
	busy     int  // READs running outside the lock
	closed   bool // revoked or closed; no more reads
	released bool // src.Close has been called
}

func (d *dndServer) clock() time.Time {
	if d.now != nil {
		return d.now()
	}
	return time.Now()
}

func (d *dndServer) makeID() (far2ldnd.ID, error) {
	if d.newID != nil {
		return d.newID()
	}
	var id far2ldnd.ID
	// A fixed fallback is forbidden (§ 5): no secure randomness, no offer.
	if _, err := rand.Read(id[:]); err != nil {
		return far2ldnd.ID{}, fmt.Errorf("terminal: DND offer id: %w", err)
	}
	return id, nil
}

// revokeLocked ends an offer. The source is released at once unless a READ
// is still reading it; that READ releases it when it returns.
func (o *dndOffer) revokeLocked() {
	o.closed = true
	if o.busy == 0 && !o.released {
		o.released = true
		o.src.Close()
	}
}

func (d *dndServer) unbindLocked() {
	for id, o := range d.offers {
		delete(d.offers, id)
		o.revokeLocked()
	}
	d.bound = false
	d.request = far2ldnd.BindRequest{}
	d.granted = far2ldnd.BindReply{}
}

// expireLocked drops the offers nobody touched for idle_seconds (§ 7).
func (d *dndServer) expireLocked() {
	now := d.clock()
	idle := time.Duration(d.granted.IdleSeconds) * time.Second
	for id, o := range d.offers {
		if o.busy == 0 && now.Sub(o.lastUse) > idle {
			delete(d.offers, id)
			o.revokeLocked()
		}
	}
}

// frameLimitLocked is the largest request APC accepted: 512 bytes before a
// binding has negotiated anything, max_frame after (§ 7).
func (d *dndServer) frameLimitLocked() int {
	if d.bound {
		return int(d.granted.MaxFrame) //nolint:gosec // at most DefaultMaxFrame
	}
	return far2ldnd.BindFrameLimit
}

// DropBound reports whether a program in the terminal accepts drops.
func (tv *TerminalView) DropBound() bool {
	d := &tv.dnd
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.bound
}

// OfferDrop publishes src to the program bound for drops and sends it the
// INPUT_DND event. x and y are the zero-based cell of the drop, negative
// when unknown; modifiers are far2l dwControlKeyState bits, meaningful only
// with modifiersKnown. On success the terminal owns src and closes it when
// the offer ends; on error src stays with the caller.
func (tv *TerminalView) OfferDrop(src DNDSource, x, y int, modifiers uint32, modifiersKnown bool) (far2ldnd.ID, error) {
	d := &tv.dnd
	d.mu.Lock()
	defer d.mu.Unlock()
	d.expireLocked()
	if !d.bound {
		return far2ldnd.ID{}, ErrDNDNotBound
	}
	if len(d.offers) >= dndMaxOffers {
		return far2ldnd.ID{}, ErrDNDTooManyOffers
	}
	o, err := newDNDOffer(src, d.granted.Features)
	if err != nil {
		return far2ldnd.ID{}, err
	}
	id, err := d.makeID()
	if err != nil {
		return far2ldnd.ID{}, err
	}
	if _, taken := d.offers[id]; taken {
		return far2ldnd.ID{}, errors.New("terminal: DND offer id collision")
	}
	ev := far2ldnd.Event{Binding: d.request.Binding, Offer: id, X: -1, Y: -1}
	if x >= 0 && y >= 0 && x <= math.MaxInt16 && y <= math.MaxInt16 {
		ev.X, ev.Y = int16(x), int16(y) //nolint:gosec // range checked above
	}
	if modifiersKnown {
		ev.Modifiers = modifiers
		ev.Flags = far2ldnd.EventModifiersKnown
	}
	stack, err := far2ldnd.EncodeEvent(&ev)
	if err != nil {
		return far2ldnd.ID{}, err
	}
	o.lastUse = d.clock()
	if d.offers == nil {
		d.offers = make(map[far2ldnd.ID]*dndOffer)
	}
	d.offers[id] = o
	tv.dndWriteLocked(far2ldnd.FrameEvent, stack)
	return id, nil
}

// newDNDOffer checks the source's objects against the base v1 profile and
// masks their representations to the negotiated features (§ 6.3). An object
// that cannot be offered is an error of the drop, never silently lost (§ 4).
func newDNDOffer(src DNDSource, features uint32) (*dndOffer, error) {
	in := src.Entries()
	if len(in) > math.MaxInt32 {
		return nil, fmt.Errorf("terminal: DND offer of %d objects", len(in))
	}
	o := &dndOffer{
		src:     src,
		entries: make([]far2ldnd.Entry, len(in)),
		index:   make(map[uint64]int, len(in)),
		next:    make(map[uint64]uint64),
	}
	for i, e := range in {
		if e.Kind != far2ldnd.KindFile {
			return nil, fmt.Errorf("terminal: DND object %d is of kind %d, only regular files are offered", i, e.Kind)
		}
		if _, dup := o.index[e.ItemID]; dup {
			return nil, fmt.Errorf("terminal: DND item_id %d offered twice", e.ItemID)
		}
		e.NativeName = append([]byte(nil), e.NativeName...)
		if features&far2ldnd.FeatureReference == 0 {
			e.Flags &^= far2ldnd.ItemReference
			e.ReferenceURI = ""
		}
		if features&far2ldnd.FeatureStream == 0 {
			e.Flags &^= far2ldnd.ItemStream | far2ldnd.ItemRandomAccess | far2ldnd.ItemFrozen
		}
		// The codec is the judge of an entry: item_id, flags, 16 KiB.
		if _, err := far2ldnd.EncodeReply(1, &far2ldnd.ListReply{NextCursor: far2ldnd.CursorLastPage, Entries: []far2ldnd.Entry{e}}); err != nil {
			return nil, fmt.Errorf("terminal: DND object %d: %w", i, err)
		}
		o.entries[i] = e
		o.index[e.ItemID] = i
	}
	return o, nil
}

// dndAccept takes one DnD request APC from the parser. wireLen is the length
// of the whole APC. The limit is checked before the base64 is decoded; a
// frame over it is dropped whole and, when it asked for a reply, answered
// with -7 (§ 7).
func (tv *TerminalView) dndAccept(rid uint8, wireLen int, b64 string) {
	d := &tv.dnd
	d.mu.Lock()
	limit := d.frameLimitLocked()
	d.mu.Unlock()
	if wireLen > limit {
		tv.dndAsyncError(rid, far2ldnd.StatusLimit, fmt.Sprintf("DND frame of %d bytes, limit %d", wireLen, limit))
		return
	}
	stack, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		tv.dndAsyncError(rid, far2ldnd.StatusBadRequest, "DND frame is not base64")
		return
	}
	d.qmu.Lock()
	if len(d.queue) >= dndQueueLimit {
		d.qmu.Unlock()
		tv.dndAsyncError(rid, far2ldnd.StatusLimit, "DND request queue is full")
		return
	}
	d.queue = append(d.queue, stack)
	start := !d.running
	d.running = true
	d.qmu.Unlock()
	if start {
		go tv.dndDrain()
	}
}

// dndAsyncError answers from a goroutine of its own: the caller is the
// parser, which must not wait for the program to read its input.
func (tv *TerminalView) dndAsyncError(rid uint8, status far2ldnd.Status, msg string) {
	if rid == 0 {
		return
	}
	go func() {
		d := &tv.dnd
		d.mu.Lock()
		defer d.mu.Unlock()
		tv.dndErrorLocked(rid, status, msg)
	}()
}

func (tv *TerminalView) dndDrain() {
	d := &tv.dnd
	for {
		d.qmu.Lock()
		if len(d.queue) == 0 {
			d.running = false
			d.qmu.Unlock()
			return
		}
		stack := d.queue[0]
		d.queue[0] = nil
		d.queue = d.queue[1:]
		d.qmu.Unlock()
		tv.dndServe(stack)
	}
}

// dndWaitInflight blocks until fewer than limit LIST/READ requests are being
// served (limit 1 with the barrier flag below means: none at all), then, for
// a concurrent request, counts it in.
func (d *dndServer) dndWaitInflight(limit int, count bool) {
	d.pmu.Lock()
	defer d.pmu.Unlock()
	if d.pcond == nil {
		d.pcond = sync.NewCond(&d.pmu)
	}
	for d.inflight >= limit {
		d.pcond.Wait()
	}
	if count {
		d.inflight++
	}
}

func (d *dndServer) dndDone() {
	d.pmu.Lock()
	d.inflight--
	if d.pcond != nil {
		d.pcond.Broadcast()
	}
	d.pmu.Unlock()
}

// dndGrantedWindow is the window of the binding in force (1 before one).
func (d *dndServer) dndGrantedWindow() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.bound && d.granted.Window > 0 {
		return int(d.granted.Window)
	}
	return 1
}

// dndServe answers one decoded DnD request: LIST and READ on a goroutine of
// their own while the granted window has room, BIND and CLOSE alone.
func (tv *TerminalView) dndServe(stack []byte) {
	d := &tv.dnd
	rid, q, err := far2ldnd.DecodeRequest(stack)
	if err != nil {
		d.mu.Lock()
		defer d.mu.Unlock()
		tv.dndErrorLocked(rid, far2ldnd.StatusFor(err), err.Error())
		return
	}
	switch q := q.(type) {
	case *far2ldnd.BindRequest:
		d.dndWaitInflight(1, false)
		tv.dndBind(rid, q)
	case *far2ldnd.CloseRequest:
		d.dndWaitInflight(1, false)
		tv.dndClose(rid, q)
	case *far2ldnd.ListRequest:
		d.dndWaitInflight(d.dndGrantedWindow(), true)
		go func() {
			defer d.dndDone()
			tv.dndList(rid, q)
		}()
	case *far2ldnd.ReadRequest:
		d.dndWaitInflight(d.dndGrantedWindow(), true)
		go func() {
			defer d.dndDone()
			tv.dndRead(rid, q)
		}()
	}
}

// dndReset forgets the binding and revokes every offer: far2l extensions
// were switched off (§ 12).
func (tv *TerminalView) dndReset() {
	d := &tv.dnd
	d.mu.Lock()
	defer d.mu.Unlock()
	d.unbindLocked()
}

func (tv *TerminalView) dndBind(rid uint8, q *far2ldnd.BindRequest) {
	d := &tv.dnd
	d.mu.Lock()
	defer d.mu.Unlock()
	if !q.Enable {
		// Only the matching generation goes; a late disable of an old
		// binding leaves the new one alone, and a repeat is harmless.
		if d.bound && d.request.Binding == q.Binding {
			d.unbindLocked()
		}
		tv.dndReplyLocked(rid, nil)
		return
	}
	if d.bound && d.request.Binding == q.Binding {
		if d.request != *q {
			tv.dndErrorLocked(rid, far2ldnd.StatusBadRequest, "BIND repeated for the same binding with other parameters")
			return
		}
		tv.dndReplyLocked(rid, &d.granted)
		return
	}
	features := q.WantedFeatures & dndFeatures
	if features == 0 {
		tv.dndErrorLocked(rid, far2ldnd.StatusUnsupported, "BIND wants neither STREAM nor REFERENCE")
		return
	}
	grant := far2ldnd.BindReply{
		Version:     far2ldnd.Version,
		Binding:     q.Binding,
		MaxFrame:    min(q.MaxFrame, far2ldnd.DefaultMaxFrame),
		Window:      uint16(min(int(q.Window), dndWindow)), //nolint:gosec // at most dndWindow
		IdleSeconds: dndIdleSeconds,
		Features:    features,
	}
	// The chunk shrinks so that a READ reply always fits the frame (§ 7).
	// Every reply this terminal sends is BEL-terminated (dndWriteLocked),
	// but the grant itself is computed against the one-byte-tighter ST
	// bound: DecodeBindReply checks a client's own grant the same
	// conservative way, since nothing in the wire format tells a client in
	// advance which terminator its future READ replies will actually close
	// with (owner's answer 4). ST's bound is never above BEL's for the same
	// max_frame, so this is always at least as tight as the real wire needs.
	grant.MaxChunk = min(q.MaxChunk, far2ldnd.DefaultMaxChunk, far2ldnd.MaxReadData(grant.MaxFrame, far2ldnd.ST))
	// A new binding revokes the offers of the old one (§ 6.1).
	d.unbindLocked()
	d.bound = true
	d.request = *q
	d.granted = grant
	tv.dndReplyLocked(rid, &grant)
}

func (tv *TerminalView) dndList(rid uint8, q *far2ldnd.ListRequest) {
	d := &tv.dnd
	d.mu.Lock()
	defer d.mu.Unlock()
	d.expireLocked()
	o := d.offers[q.Offer]
	switch {
	case o == nil:
		tv.dndErrorLocked(rid, far2ldnd.StatusOfferGone, "unknown DND offer")
		return
	case q.ParentID != 0:
		// Base v1 has no tree: the root is the only parent.
		tv.dndErrorLocked(rid, far2ldnd.StatusUnknownItem, "unknown DND parent")
		return
	case q.Cursor != far2ldnd.CursorFirst && q.Cursor >= uint64(len(o.entries)): //nolint:gosec // a length is never negative
		// Valid cursors are 0 and the ones this terminal handed out,
		// which always point at an entry.
		tv.dndErrorLocked(rid, far2ldnd.StatusUnknownItem, "unknown DND cursor")
		return
	}
	o.lastUse = d.clock()
	start := int(q.Cursor) //nolint:gosec // below len(o.entries), checked above
	if start == len(o.entries) {
		tv.dndReplyLocked(rid, &far2ldnd.ListReply{NextCursor: far2ldnd.CursorLastPage})
		return
	}
	// The page grows while it fits both 64 entries and max_frame. Nothing is
	// ever cut to fit: an entry that does not fit alone is -7 (§ 6.3, § 7).
	var best []byte
	for end := start + 1; end <= len(o.entries) && end-start <= far2ldnd.MaxListEntries; end++ {
		next := uint64(end) //nolint:gosec // end > 0
		if end == len(o.entries) {
			next = far2ldnd.CursorLastPage
		}
		stack, err := far2ldnd.EncodeReply(rid, &far2ldnd.ListReply{NextCursor: next, Entries: o.entries[start:end]})
		if err != nil || far2ldnd.FrameLen(far2ldnd.FrameReply, len(stack), far2ldnd.BEL) > int(d.granted.MaxFrame) { //nolint:gosec // at most DefaultMaxFrame
			break
		}
		best = stack
	}
	if best == nil {
		tv.dndErrorLocked(rid, far2ldnd.StatusLimit, "DND entry does not fit max_frame")
		return
	}
	if rid != 0 {
		tv.dndWriteLocked(far2ldnd.FrameReply, best)
	}
}

func (tv *TerminalView) dndRead(rid uint8, q *far2ldnd.ReadRequest) {
	d := &tv.dnd
	d.mu.Lock()
	d.expireLocked()
	o := d.offers[q.Offer]
	if o == nil {
		tv.dndErrorLocked(rid, far2ldnd.StatusOfferGone, "unknown DND offer")
		d.mu.Unlock()
		return
	}
	i, ok := o.index[q.ItemID]
	if !ok {
		tv.dndErrorLocked(rid, far2ldnd.StatusUnknownItem, "unknown DND item")
		d.mu.Unlock()
		return
	}
	e := o.entries[i]
	var msg string
	status := far2ldnd.StatusBadRequest
	switch {
	case q.Length > d.granted.MaxChunk:
		msg = fmt.Sprintf("READ of %d bytes, max_chunk %d", q.Length, d.granted.MaxChunk)
	case e.Flags&far2ldnd.ItemStream == 0:
		status, msg = far2ldnd.StatusUnsupported, "DND item has no STREAM representation"
	case e.Flags&far2ldnd.ItemRandomAccess == 0 && q.Offset != o.next[q.ItemID]:
		// A sequential item is read from 0, each range after the bytes
		// actually received (§ 6.4).
		msg = fmt.Sprintf("sequential DND item read at %d, expected %d", q.Offset, o.next[q.ItemID])
	}
	if msg != "" {
		tv.dndErrorLocked(rid, status, msg)
		d.mu.Unlock()
		return
	}
	o.lastUse = d.clock()
	o.busy++
	d.mu.Unlock()

	// At most one chunk, into a bounded buffer, before the reply is built
	// whole (§ 6.4).
	buf := make([]byte, q.Length)
	type readResult struct {
		n   int
		eof bool
		err error
	}
	done := make(chan readResult, 1)
	go func() {
		n, eof, err := o.src.ReadAt(q.ItemID, buf, q.Offset)
		done <- readResult{n, eof, err}
	}()
	timer := time.NewTimer(dndReadTimeout)
	var res readResult
	select {
	case res = <-done:
		timer.Stop()
	case <-timer.C:
		// A source that never returns must not hold the request worker, and
		// with it every later request, for the whole idle time. The offer is
		// revoked; the hung ReadAt keeps the source open (it must never be
		// closed under a running read) and releases it when it returns.
		d.mu.Lock()
		if d.offers[q.Offer] == o {
			delete(d.offers, q.Offer)
		}
		o.closed = true
		tv.dndErrorLocked(rid, far2ldnd.StatusIOError, "DND source read timed out")
		d.mu.Unlock()
		go func() {
			<-done
			d.mu.Lock()
			defer d.mu.Unlock()
			o.busy--
			if o.busy == 0 && !o.released {
				o.released = true
				o.src.Close()
			}
		}()
		return
	}
	n, eof, err := res.n, res.eof, res.err

	d.mu.Lock()
	defer d.mu.Unlock()
	o.busy--
	if o.closed {
		if o.busy == 0 && !o.released {
			o.released = true
			o.src.Close()
		}
		tv.dndErrorLocked(rid, far2ldnd.StatusCancelled, "DND offer closed during READ")
		return
	}
	o.lastUse = d.clock()
	switch {
	case errors.Is(err, ErrDNDSourceChanged):
		tv.dndErrorLocked(rid, far2ldnd.StatusChanged, err.Error())
		return
	case err != nil:
		tv.dndErrorLocked(rid, far2ldnd.StatusIOError, err.Error())
		return
	case n < 0 || n > len(buf) || (n == 0 && !eof):
		tv.dndErrorLocked(rid, far2ldnd.StatusIOError, fmt.Sprintf("DND source returned %d bytes, eof %v", n, eof))
		return
	}
	reply := far2ldnd.ReadReply{Data: buf[:n]}
	if eof {
		reply.Flags |= far2ldnd.ReadEOF
	}
	if e.Flags&far2ldnd.ItemSizeKnown != 0 {
		reply.Flags |= far2ldnd.ReadSizeKnown
		reply.ObservedSize = e.Size
	}
	if e.Flags&far2ldnd.ItemRandomAccess == 0 {
		o.next[q.ItemID] = q.Offset + uint64(n) //nolint:gosec // 0 <= n <= len(buf), checked above
	}
	tv.dndReplyLocked(rid, &reply)
}

func (tv *TerminalView) dndClose(rid uint8, q *far2ldnd.CloseRequest) {
	d := &tv.dnd
	d.mu.Lock()
	defer d.mu.Unlock()
	if o := d.offers[q.Offer]; o != nil {
		delete(d.offers, q.Offer)
		o.revokeLocked()
	}
	// Closing an unknown or expired offer again is a success (§ 6.5).
	tv.dndReplyLocked(rid, nil)
}

// dndWriteLocked writes one whole DnD frame to the program.
func (tv *TerminalView) dndWriteLocked(k far2ldnd.FrameKind, stack []byte) {
	if tv.Pty != nil {
		_, _ = tv.Pty.Write([]byte(far2ldnd.Frame(k, stack, far2ldnd.BEL)))
	}
}

func (tv *TerminalView) dndReplyLocked(rid uint8, body far2ldnd.Reply) {
	if rid == 0 {
		return
	}
	stack, err := far2ldnd.EncodeReply(rid, body)
	if err != nil {
		tv.dndErrorLocked(rid, far2ldnd.StatusIOError, err.Error())
		return
	}
	tv.dndWriteLocked(far2ldnd.FrameReply, stack)
}

// dndErrorLocked shortens msg to what the *current* frame limit can carry
// before handing it to EncodeError: EncodeError's own ceiling (MaxMessageLen,
// 1024 bytes) is only ever the right bound once BIND has negotiated a large
// enough max_frame, and before that -- or for a BIND request itself, which
// answers within BindFrameLimit -- 1024 bytes of message would not even fit
// the 512-byte reply it has to travel in (owner's answer 3). Every error
// reply goes out as BEL (dndWriteLocked), so that is what bounds the budget
// here.
func (tv *TerminalView) dndErrorLocked(rid uint8, status far2ldnd.Status, msg string) {
	if rid == 0 {
		return
	}
	limit := far2ldnd.MaxErrorText(uint32(tv.dnd.frameLimitLocked()), far2ldnd.BEL) //nolint:gosec // frameLimitLocked is at most DefaultMaxFrame
	stack, err := far2ldnd.EncodeError(rid, status, far2ldnd.TruncateText(msg, limit))
	if err != nil {
		return
	}
	tv.dndWriteLocked(far2ldnd.FrameReply, stack)
}

// dndPeek decodes only the last base64 groups of a request: the stack is
// popped from its end, so the RID and the command are its last two bytes.
// This tells a DnD request apart before the whole frame is decoded.
func dndPeek(b64 string) (rid, cmd uint8, ok bool) {
	tail := b64
	if len(tail) > 8 {
		tail = tail[len(tail)-8:]
	}
	b, err := base64.StdEncoding.DecodeString(tail)
	if err != nil || len(b) < 2 {
		return 0, 0, false
	}
	return b[len(b)-1], b[len(b)-2], true
}
