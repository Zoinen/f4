package terminal

// far2l_dnd_bridge.go connects DNDClient (far2l_dnd_client.go) to the real
// far2l channel f4 itself runs inside, as opposed to the self-contained RID
// table it keeps when it owns the wire outright (a nested TerminalView's
// Pty, when f4 hosts another f4).
//
// vtui already owns that real channel (os.Stdout out, far2lIDCounter in)
// for its own clipboard/image extensions (far2l_extensions.go), under one
// RID space that belongs to the wire, not to whichever f4 feature happens
// to use it. Giving DNDClient a second, independent RID counter on the very
// same wire would let vtui's request and this client's request collide on
// the same byte, since nothing on the far2l host side of the wire knows
// there ought to be two counters instead of one. A bridged DNDClient keeps
// none of its own: every request goes out, and every reply comes back,
// through vtui.Far2lInteractTimeout, whose far2lIDCounter is the wire's
// only RID counter (status/1628.md, step 4, has the fuller trace of how
// this was found).
//
// One consequence follows straight from that: vtui.Far2lInteractTimeout
// only ever offers a fixed timeout, not a context.Context, so a bridged
// round trip's ctx cancellation cannot free a RID early the way a
// self-contained one does (far2l_dnd_client.go's file comment, § 8 of the
// specification) -- there is no RID here for this client to hold onto or
// give back in the first place. Cancelling ctx before the call only skips
// starting it; cancelling it after only makes this client stop waiting
// sooner than the timeout, never sooner than vtui's own channel actually
// answers. Accepted as this step's limitation, not fixed here.
import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/unxed/f4/internal/terminal/far2ldnd"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// defaultBridgeTimeout is RealDNDClient's round trip timeout: generous
// enough for a human on the other end of a real far2l-compatible terminal
// (a slow network session, a busy host), short enough that a program that
// never answers DND/BIND does not wedge whatever called it forever.
const defaultBridgeTimeout = 5 * time.Second

var (
	realDNDClientOnce sync.Once
	realDNDClient     *DNDClient
)

// RealDNDClient is the one DNDClient bound to the real far2l channel this
// process itself runs inside. There is exactly one such channel per
// process -- the same one vtui's own clipboard/image extensions already
// share (see the file comment) -- so this is a singleton by construction,
// not by convenience; a nested TerminalView (f4 hosting another f4) still
// gets its own DNDClient from NewDNDClient instead, one per Pty. Getting
// this client does not, by itself, negotiate anything: a caller still
// calls Bind on it, exactly as on any other DNDClient, whenever it actually
// wants this process to receive drops from the terminal it runs inside
// (status/1628.md, step 5).
func RealDNDClient() *DNDClient {
	realDNDClientOnce.Do(func() {
		realDNDClient = NewBridgedDNDClient(defaultBridgeTimeout)
	})
	return realDNDClient
}

// ErrDNDBridgeTimeout is roundTrip's error when the real far2l channel does
// not answer within the bridge's timeout.
var ErrDNDBridgeTimeout = errors.New("terminal: far2l DND request over the real channel timed out")

// far2lBridge is a DNDClient's connection to the real far2l channel, in
// place of its own out/pending/nextRID (see the file comment).
type far2lBridge struct {
	timeout time.Duration
	// call is a hook for tests; nil uses vtui.Far2lInteractTimeout, the
	// real channel this bridge exists to share rather than duplicate.
	call func(stk *vtinput.Far2lStack, wait bool, timeout time.Duration) *vtinput.Far2lStack
}

// NewBridgedDNDClient makes a DNDClient that shares the real far2l channel
// instead of owning a Pty of its own; see the file comment for why it
// cannot keep its own RID table there. INPUT_DND on this channel needs one
// more piece this constructor does not wire up by itself: something has to
// hand every input event to DispatchBridgedEvent, because dispatchEvent's
// own far2l "reply"/"ok" handling (vtui's framemanager.go) already resolves
// this client's replies before an EventFilter would ever see them, but
// INPUT_DND is neither "reply" nor "ok" and is never intercepted that way.
func NewBridgedDNDClient(timeout time.Duration) *DNDClient {
	return &DNDClient{bridge: &far2lBridge{timeout: timeout}}
}

func (b *far2lBridge) interact(stk *vtinput.Far2lStack, wait bool) *vtinput.Far2lStack {
	call := b.call
	if call == nil {
		call = vtui.Far2lInteractTimeout
	}
	return call(stk, wait, b.timeout)
}

// bridgeBody strips the placeholder RID far2ldnd.EncodeRequest always
// writes as the stack's last byte (far2ldnd/stack.go): on this channel the
// real RID is vtui's far2lIDCounter's, appended in its place by
// vtui.Far2lInteractTimeout itself when it pushes onto the
// vtinput.Far2lStack this returns.
func bridgeBody(stack []byte) *vtinput.Far2lStack {
	body := vtinput.Far2lStack(append([]byte(nil), stack[:len(stack)-1]...))
	return &body
}

func (b *far2lBridge) roundTrip(ctx context.Context, q far2ldnd.Request) (far2ldnd.ReplyFrame, error) {
	if err := ctx.Err(); err != nil {
		return far2ldnd.ReplyFrame{}, err
	}
	// The placeholder RID's value is never read back out again -- see
	// bridgeBody -- so any non-zero byte does as well as another.
	stack, err := far2ldnd.EncodeRequest(1, q)
	if err != nil {
		return far2ldnd.ReplyFrame{}, err
	}
	reply := b.interact(bridgeBody(stack), true)
	if reply == nil {
		return far2ldnd.ReplyFrame{}, ErrDNDBridgeTimeout
	}
	// vtui's dispatchEvent (framemanager.go) already popped its own RID
	// off the front of *reply before handing it to the waiter this call
	// is; far2ldnd.DecodeReply still expects to find one -- it is the
	// header every reply carries on the wire -- so a placeholder goes back
	// in its place, its value just as unread on the way in as it was on
	// the way out.
	full := append(append([]byte(nil), []byte(*reply)...), 0)
	return far2ldnd.DecodeReply(full)
}

func (b *far2lBridge) sendNoReply(q far2ldnd.Request) error {
	// far2ldnd reserves RID 0 for a request that expects no reply at all
	// (§ 6); this channel cannot keep that convention, since
	// vtui.Far2lInteractTimeout always allocates a real, non-zero id from
	// far2lIDCounter whether or not it is told to wait. That is harmless
	// here: checkRID only restricts what a zero RID may carry, and this
	// channel never asks EncodeRequest for one. The real terminal may
	// still answer such a request; that answer is simply never collected
	// -- Far2lInteractTimeout does not register a waiter for it when wait
	// is false (far2l_extensions.go).
	stack, err := far2ldnd.EncodeRequest(1, q)
	if err != nil {
		return err
	}
	b.interact(bridgeBody(stack), false)
	return nil
}

// DispatchBridgedEvent feeds one EventFilter-visible input event to c's
// INPUT_DND handling, and reports whether it was a far2l "f2l"-family event
// at all -- not whether c's OnEvent chose to act on it -- so it is safe to
// call unconditionally from an EventFilter chain (internal/app/bootstrap.go)
// on every event that reaches one.
//
// Only Far2lEventType events with Far2lCommand "event" ever carry a
// far2ldnd frame by the time they get here: vtinput.ParseFar2lAPC already
// turns every key/mouse/resize "f2l" event it recognises into that event's
// own vtinput.EventType (KeyEventType, MouseEventType, ...), and every
// "far2l"-prefixed reply/ok never reaches an EventFilter at all --
// dispatchEvent answers those itself (see the file comment). So an event
// still shaped like this here is either INPUT_DND or some future far2l
// "f2l" extension this client correctly has no opinion on, which is why
// far2ldnd.ErrNotDND (checked before c.HandleFrame, which would otherwise
// swallow it the same way) answers false, not true.
func (c *DNDClient) DispatchBridgedEvent(ev *vtinput.InputEvent) bool {
	if ev == nil || ev.Type != vtinput.Far2lEventType || ev.Far2lCommand != "event" {
		return false
	}
	if _, err := far2ldnd.DecodeEvent(ev.Far2lData); errors.Is(err, far2ldnd.ErrNotDND) {
		return false
	}
	if err := c.HandleFrame(far2ldnd.FrameEvent, ev.Far2lData); err != nil {
		vtui.DebugLog("DND: bridged INPUT_DND event rejected: %v", err)
	}
	return true
}
