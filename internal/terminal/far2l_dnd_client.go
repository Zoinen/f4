package terminal

// The application side of the far2l drag-and-drop protocol (unxed/f4#1628,
// specification v0.2): f4 running as a program inside a far2l-compatible
// terminal -- its own embedded TerminalView (a nested f4) or a real one.
// dndServer in far2l_dnd.go is the other end of this same wire; DNDClient
// sends the exact APC frames it parses and parses the exact frames it
// writes, through the far2ldnd codec both sides share.
//
// DNDClient owns one binding: BIND switches drop reception on, an unsolicited
// INPUT_DND event names an offer, and LIST/READ pull its entries and bytes
// one request at a time (this step keeps to window=1, the only profile
// dndServer grants today; a non-blocking pipelined dispatcher for window>1 is
// later work -- status/1628.md, step 4). CLOSE releases the offer, whether
// the transfer finished or the user cancelled it.
//
// § 8 of the specification is the one rule this file exists to keep: giving
// up on a call -- the caller's context is cancelled, e.g. Esc or a lost
// window focus -- must never free its RID before the matching reply (or a
// permanent Shutdown) actually retires it. A RID reused too early could catch
// a stale reply meant for the abandoned request. roundTrip below returns to
// a cancelled caller without touching the pending table; deliver alone frees
// a RID, and only once the full reply for it is in hand.
import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/unxed/f4/internal/terminal/far2ldnd"
)

// ErrDNDClientNoFreeRID: all 255 RIDs are waiting on a reply. § 8 allows
// CLOSE alone to fall back to RID 0 (best-effort, no reply) in this case;
// every other request must wait for the table to drain instead.
var ErrDNDClientNoFreeRID = errors.New("terminal: no free far2l RID")

// DNDClientLimits is the profile a client asks for in BIND (§ 7).
type DNDClientLimits struct {
	MaxFrame uint32
	MaxChunk uint32
	Window   uint16
}

// DefaultDNDClientLimits asks for the proposed initial profile with window=1:
// this step's dispatcher serves one request at a time (see the file comment)
// and must not advertise a window it cannot actually keep in flight.
var DefaultDNDClientLimits = DNDClientLimits{
	MaxFrame: far2ldnd.DefaultMaxFrame,
	MaxChunk: far2ldnd.DefaultMaxChunk,
	Window:   1,
}

// dndReply is what a matched reply resolves a pending call with: the decoded
// frame and whatever far2ldnd.DecodeReply made of it -- a *far2ldnd.StatusError
// and far2ldnd.ErrNoDND are delivered here as ordinary results, not dropped.
type dndReply struct {
	frame far2ldnd.ReplyFrame
	err   error
}

// DNDClient is one binding's worth of far2l DnD client state, bound to one
// far2l channel: real process stdout, or a nested TerminalView's Pty when f4
// hosts another f4. Its zero value is not ready; construct with
// NewDNDClient. Two bindings sharing one channel would fight over the same
// RID table, so each channel gets its own client.
type DNDClient struct {
	// wmu serializes the bytes written to out: § 7 forbids interleaving
	// another frame's bytes inside one APC, and nothing else here
	// guarantees requests are written one at a time.
	wmu sync.Mutex
	out io.Writer

	// OnEvent is called, from a goroutine of its own, whenever INPUT_DND
	// names an offer for the binding currently in force. It runs off the
	// goroutine that fed the event to HandleFrame so it may call back into
	// this client (List, Read, ...) without deadlocking that delivery.
	// A nil OnEvent silently discards the event.
	OnEvent func(far2ldnd.Event)

	// newBinding is a hook for tests; nil uses crypto/rand like the
	// terminal's own offer ids (far2l_dnd.go).
	newBinding func() (far2ldnd.ID, error)

	// bridge, when set, replaces out/pending/nextRID below entirely: every
	// round trip shares the real far2l channel's own RID counter instead
	// of this client's, through vtui.Far2lInteractTimeout. See
	// far2l_dnd_bridge.go (NewBridgedDNDClient) for why a second,
	// independent RID counter cannot share that channel safely.
	bridge *far2lBridge

	mu      sync.Mutex // guards the fields below
	bound   bool
	binding far2ldnd.ID
	granted far2ldnd.BindReply

	rmu     sync.Mutex              // guards pending and nextRID, separately from mu:
	pending map[uint8]chan dndReply // HandleFrame must never wait on a caller
	nextRID uint8
	// freed is closed (and replaced) whenever a RID is retired, to wake the
	// calls waiting for room in the window.
	freed chan struct{}
}

// NewDNDClient makes a client that writes its request frames to out. out is
// typically os.Stdout (a real terminal behind this process) or another
// TerminalView's Pty (f4 nested inside f4); Write must not be shared with
// any other far2l request source that expects its own RIDs answered on this
// same client, since DecodeReply frames neither side owns are simply ignored
// (see deliver).
func NewDNDClient(out io.Writer) *DNDClient {
	return &DNDClient{out: out, pending: make(map[uint8]chan dndReply)}
}

// Bound reports the binding currently in force, if any.
func (c *DNDClient) Bound() (far2ldnd.BindReply, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.granted, c.bound
}

func (c *DNDClient) makeBinding() (far2ldnd.ID, error) {
	if c.newBinding != nil {
		return c.newBinding()
	}
	var id far2ldnd.ID
	// A fixed fallback is forbidden, as it is for the terminal's own offer
	// ids: a predictable binding lets another client hijack drop delivery.
	if _, err := rand.Read(id[:]); err != nil {
		return far2ldnd.ID{}, fmt.Errorf("terminal: DND client binding: %w", err)
	}
	return id, nil
}

// Bind negotiates a fresh binding and switches drop reception on. The
// binding is chosen here, by the application, as the specification requires
// (§ 5); a second Bind while already bound starts a new generation, which
// revokes the terminal's offers of the old one (§ 6.1) exactly as a repeat
// BIND with a different binding does.
func (c *DNDClient) Bind(ctx context.Context, wantedFeatures uint32, limits DNDClientLimits) (far2ldnd.BindReply, error) {
	binding, err := c.makeBinding()
	if err != nil {
		return far2ldnd.BindReply{}, err
	}
	q := &far2ldnd.BindRequest{
		Version: far2ldnd.Version, Enable: true, Binding: binding,
		MaxFrame: limits.MaxFrame, MaxChunk: limits.MaxChunk, Window: limits.Window,
		WantedFeatures: wantedFeatures,
	}
	f, err := c.roundTrip(ctx, q)
	if err != nil {
		return far2ldnd.BindReply{}, err
	}
	b, err := far2ldnd.DecodeBindReply(f, q)
	if err != nil {
		return far2ldnd.BindReply{}, err
	}
	c.mu.Lock()
	c.bound, c.binding, c.granted = true, binding, b
	c.mu.Unlock()
	return b, nil
}

// Unbind switches the current binding off. Calling it while not bound is a
// success that does nothing, like a repeated far2l BIND disable (§ 6.1).
func (c *DNDClient) Unbind(ctx context.Context) error {
	c.mu.Lock()
	bound, binding := c.bound, c.binding
	c.mu.Unlock()
	if !bound {
		return nil
	}
	q := &far2ldnd.BindRequest{Version: far2ldnd.Version, Enable: false, Binding: binding}
	f, err := c.roundTrip(ctx, q)
	if err != nil {
		return err
	}
	if _, err := far2ldnd.DecodeBindReply(f, q); err != nil {
		return err
	}
	c.mu.Lock()
	if c.binding == binding {
		c.bound, c.binding, c.granted = false, far2ldnd.ID{}, far2ldnd.BindReply{}
	}
	c.mu.Unlock()
	return nil
}

// List asks for one page of offer's root (§ 6.3); parent_id is always 0 in
// the base v1 profile.
func (c *DNDClient) List(ctx context.Context, offer far2ldnd.ID, cursor uint64) (far2ldnd.ListReply, error) {
	q := &far2ldnd.ListRequest{Offer: offer, ParentID: 0, Cursor: cursor}
	f, err := c.roundTrip(ctx, q)
	if err != nil {
		return far2ldnd.ListReply{}, err
	}
	return far2ldnd.DecodeListReply(f)
}

// ListAll is the dispatcher a caller needs instead of hand-rolling cursor
// bookkeeping: it pages offer's root until next_cursor says the last page,
// in request order. It stops at the first error, keeping whatever entries
// were already collected.
func (c *DNDClient) ListAll(ctx context.Context, offer far2ldnd.ID) ([]far2ldnd.Entry, error) {
	var all []far2ldnd.Entry
	cursor := far2ldnd.CursorFirst
	for {
		page, err := c.List(ctx, offer, cursor)
		if err != nil {
			return all, err
		}
		all = append(all, page.Entries...)
		if page.NextCursor == far2ldnd.CursorLastPage {
			return all, nil
		}
		cursor = page.NextCursor
	}
}

// Read asks for one range of item_id (§ 6.4). Unlike FISH+, length 0 is not
// "read everything"; the codec's check() already refuses it, and offset and
// length still overflowing is refused the same way, before anything is sent.
func (c *DNDClient) Read(ctx context.Context, offer far2ldnd.ID, itemID, offset uint64, length uint32) (far2ldnd.ReadReply, error) {
	q := &far2ldnd.ReadRequest{Offer: offer, ItemID: itemID, Offset: offset, Length: length}
	f, err := c.roundTrip(ctx, q)
	if err != nil {
		return far2ldnd.ReadReply{}, err
	}
	return far2ldnd.DecodeReadReply(f, q)
}

// Close releases offer, whether the transfer finished or the caller gave up
// on it (§ 6.5); reason records which for the terminal's own bookkeeping.
// Repeating it, including after the offer already expired, is a success.
//
// Cancelling a drag (Esc, a lost window focus) does not call Close directly:
// it cancels the ctx of whatever List/Read is in flight (§ 8 step 1, which
// this client keeps by never freeing that RID early -- see the file
// comment), and the caller then calls Close itself (§ 8 step 2) to revoke
// the offer server-side. If every RID is already waiting on a reply, Close
// still gets through: it falls back to RID 0, best-effort, exactly as § 8
// allows.
func (c *DNDClient) Close(ctx context.Context, offer far2ldnd.ID, reason uint8) error {
	q := &far2ldnd.CloseRequest{Offer: offer, Reason: reason}
	f, err := c.roundTrip(ctx, q)
	if errors.Is(err, ErrDNDClientNoFreeRID) {
		return c.sendNoReply(q)
	}
	if err != nil {
		return err
	}
	return far2ldnd.DecodeEmptyReply(f)
}

// Shutdown abandons every RID this client still holds pending, without
// waiting for or expecting their replies, and forgets the current binding.
// It is for tearing the whole client down -- the far2l channel itself is
// gone, e.g. the process is exiting or the underlying Pty closed (§ 12) --
// not for cancelling one drag: that is what passing a cancelled ctx to a
// single List/Read/Close already does (see Close), and it leaves this
// client's other in-flight requests, and their RIDs, alone.
func (c *DNDClient) Shutdown() {
	c.rmu.Lock()
	c.pending = make(map[uint8]chan dndReply)
	c.notifyFreedLocked()
	c.rmu.Unlock()
	c.mu.Lock()
	c.bound, c.binding, c.granted = false, far2ldnd.ID{}, far2ldnd.BindReply{}
	c.mu.Unlock()
}

// HandleFrame feeds one decoded far2l frame -- as far2ldnd.ParseFrame or an
// equivalent real input parser produced it -- to this client. A FrameReply
// resolves the pending call of its RID, if this client has one; a
// FrameReply for a RID nobody here is waiting on, or a FrameEvent for a code
// other than INPUT_DND, is silently ignored, which is what lets this method
// be handed every far2l reply/event frame on a shared channel, not only the
// ones that turn out to be this client's own (§ 8: a late or foreign reply
// must not disturb anyone else's RID). A FrameRequest -- the direction only
// an application sends -- is refused: nothing on this side should ever
// receive one.
func (c *DNDClient) HandleFrame(kind far2ldnd.FrameKind, stack []byte) error {
	switch kind {
	case far2ldnd.FrameReply:
		f, err := far2ldnd.DecodeReply(stack)
		// f.RID reads before any error that can only strike deeper in the
		// stack (DecodeReply pops the RID first), so most malformed
		// replies still retire the RID they claim; one that fails before
		// even that is not attributable to any RID and is simply dropped.
		c.deliver(f.RID, dndReply{frame: f, err: err})
		if err != nil {
			var se *far2ldnd.StatusError
			if errors.As(err, &se) || errors.Is(err, far2ldnd.ErrNoDND) {
				return nil // delivered as the call's own result, not a fault here
			}
			return err
		}
		return nil
	case far2ldnd.FrameEvent:
		ev, err := far2ldnd.DecodeEvent(stack)
		if errors.Is(err, far2ldnd.ErrNotDND) {
			return nil // an f2l event of another kind, not ours
		}
		if err != nil {
			return err
		}
		c.mu.Lock()
		ours := c.bound && ev.Binding == c.binding
		cb := c.OnEvent
		c.mu.Unlock()
		if ours && cb != nil {
			go cb(ev)
		}
		return nil
	default:
		return fmt.Errorf("terminal: DND client got a frame of kind %d, application to terminal only", kind)
	}
}

// roundTrip sends q under a fresh RID and waits for its reply or for ctx to
// end. A cancelled ctx returns without freeing the RID (§ 8): deliver alone
// retires it, whenever the real reply -- or a foreign one that happens to
// reuse the number, which cannot happen while this RID is still reserved --
// finally arrives.
func (c *DNDClient) roundTrip(ctx context.Context, q far2ldnd.Request) (far2ldnd.ReplyFrame, error) {
	if c.bridge != nil {
		return c.bridge.roundTrip(ctx, q)
	}
	// The window bounds the requests the terminal has to hold at once (§ 7):
	// a call beyond it waits for a reply to retire one rather than being
	// dropped or answered -7. BIND itself is outside it -- there is no
	// window before it is answered.
	limit := 0
	if _, isBind := q.(*far2ldnd.BindRequest); !isBind {
		if granted, ok := c.Bound(); ok {
			limit = int(granted.Window)
		}
	}
	rid, ch, err := c.allocRIDInWindow(ctx, limit)
	if err != nil {
		return far2ldnd.ReplyFrame{}, err
	}
	stack, err := far2ldnd.EncodeRequest(rid, q)
	if err != nil {
		c.freeRID(rid)
		return far2ldnd.ReplyFrame{}, err
	}
	if err := c.write(far2ldnd.FrameRequest, stack); err != nil {
		c.freeRID(rid)
		return far2ldnd.ReplyFrame{}, err
	}
	select {
	case res := <-ch:
		return res.frame, res.err
	case <-ctx.Done():
		return far2ldnd.ReplyFrame{}, ctx.Err()
	}
}

// sendNoReply writes q under RID 0: fire-and-forget, allowed only for CLOSE
// and for switching a binding off (far2ldnd.EncodeRequest enforces this).
func (c *DNDClient) sendNoReply(q far2ldnd.Request) error {
	if c.bridge != nil {
		return c.bridge.sendNoReply(q)
	}
	stack, err := far2ldnd.EncodeRequest(0, q)
	if err != nil {
		return err
	}
	return c.write(far2ldnd.FrameRequest, stack)
}

func (c *DNDClient) write(k far2ldnd.FrameKind, stack []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err := c.out.Write([]byte(far2ldnd.Frame(k, stack, far2ldnd.BEL)))
	return err
}

// allocRID reserves the next free RID, wrapping 1..255 (0 is reserved for
// no-reply requests and is never handed out here).
func (c *DNDClient) allocRID() (uint8, chan dndReply, error) {
	c.rmu.Lock()
	defer c.rmu.Unlock()
	if c.pending == nil {
		c.pending = make(map[uint8]chan dndReply)
	}
	for i := 0; i < 255; i++ {
		c.nextRID++
		if c.nextRID == 0 {
			c.nextRID = 1
		}
		if _, busy := c.pending[c.nextRID]; !busy {
			ch := make(chan dndReply, 1)
			c.pending[c.nextRID] = ch
			return c.nextRID, ch, nil
		}
	}
	return 0, nil, ErrDNDClientNoFreeRID
}

func (c *DNDClient) freeRID(rid uint8) {
	c.rmu.Lock()
	delete(c.pending, rid)
	c.notifyFreedLocked()
	c.rmu.Unlock()
}

// notifyFreedLocked wakes every call waiting in allocRIDInWindow.
func (c *DNDClient) notifyFreedLocked() {
	if c.freed != nil {
		close(c.freed)
		c.freed = nil
	}
}

// allocRIDInWindow is allocRID for a call that must not have more than limit
// requests outstanding (limit <= 0: no bound). Cancelled ctx ends the wait;
// nothing has been reserved by then, so nothing is left to retire.
func (c *DNDClient) allocRIDInWindow(ctx context.Context, limit int) (uint8, chan dndReply, error) {
	for {
		c.rmu.Lock()
		if limit <= 0 || len(c.pending) < limit {
			c.rmu.Unlock()
			return c.allocRID()
		}
		if c.freed == nil {
			c.freed = make(chan struct{})
		}
		wait := c.freed
		c.rmu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return 0, nil, ctx.Err()
		}
	}
}

// deliver resolves rid's pending call, if this client has one, and retires
// the RID in the same critical section so a reply can never be waited on
// twice. A rid nobody is waiting on -- an unrelated far2l reply on a shared
// channel, or one this client already gave up tracking via Shutdown -- is
// dropped without a receiver to send to.
func (c *DNDClient) deliver(rid uint8, res dndReply) {
	c.rmu.Lock()
	ch, ok := c.pending[rid]
	if ok {
		delete(c.pending, rid)
		c.notifyFreedLocked()
	}
	c.rmu.Unlock()
	if ok {
		ch <- res
	}
}
