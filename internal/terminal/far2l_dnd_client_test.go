package terminal

// The application side of the DnD protocol, tested directly against the
// terminal side from step 2 (dndEnv, far2l_dnd_test.go) in one process: no
// real terminal, no subprocess, just the far2ldnd wire between a DNDClient
// and a TerminalView's dndServer.

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/unxed/f4/internal/terminal/far2ldnd"
)

// syncWire pumps one request through the terminal side and delivers its one
// reply before Write returns, so a DNDClient call made from the test's own
// goroutine never blocks: by the time roundTrip reaches its select, the
// reply is already sitting in the channel.
type syncWire struct {
	t *testing.T
	e *dndEnv
	c *DNDClient
}

func (w *syncWire) Write(b []byte) (int, error) {
	w.t.Helper()
	w.e.pty.Reset()
	w.e.p.Process(b)
	frame := w.e.wait(w.t, 1)[0]
	kind, stack, _, err := far2ldnd.ParseFrame(frame)
	if err != nil {
		return 0, err
	}
	if err := w.c.HandleFrame(kind, stack); err != nil {
		return 0, err
	}
	return len(b), nil
}

func newSyncClient(t *testing.T, e *dndEnv) *DNDClient {
	t.Helper()
	w := &syncWire{t: t, e: e}
	c := NewDNDClient(w)
	w.c = c
	return c
}

// TestDNDClientFullCycle drives BIND, the resulting INPUT_DND, a full
// ListAll, a READ of the whole content and CLOSE against the real dndServer,
// entirely through the wire format both sides share.
func TestDNDClientFullCycle(t *testing.T) {
	e := newDNDEnv(t)
	client := newSyncClient(t, e)

	dropped := make(chan far2ldnd.Event, 1)
	client.OnEvent = func(ev far2ldnd.Event) { dropped <- ev }

	granted, err := client.Bind(context.Background(), far2ldnd.FeatureStream, DefaultDNDClientLimits)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if granted.Features&far2ldnd.FeatureStream == 0 {
		t.Fatalf("BIND reply without STREAM: %+v", granted)
	}
	if got, ok := client.Bound(); !ok || got != granted {
		t.Fatalf("Bound() = %+v, %v; want %+v, true", got, ok, granted)
	}

	// The server publishes the offer and its event synchronously; capture
	// it before any client.* call resets the pty for its own reply.
	e.pty.Reset()
	src := oneFile(1, streamFlags, "hello, dnd")
	offerID, err := e.tv.OfferDrop(src, 3, 4, 0, false)
	if err != nil {
		t.Fatalf("OfferDrop: %v", err)
	}
	evFrame := e.wait(t, 1)[0]
	k, stack, _, err := far2ldnd.ParseFrame(evFrame)
	if err != nil || k != far2ldnd.FrameEvent {
		t.Fatalf("event frame: %v", err)
	}
	if err := client.HandleFrame(k, stack); err != nil {
		t.Fatalf("HandleFrame(event): %v", err)
	}
	ev := <-dropped
	if ev.Offer != offerID || ev.Binding != granted.Binding || ev.X != 3 || ev.Y != 4 {
		t.Fatalf("INPUT_DND %+v, want offer %x binding %x at (3,4)", ev, offerID, granted.Binding)
	}

	entries, err := client.ListAll(context.Background(), offerID)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(entries) != 1 || entries[0].Name != "f" || entries[0].ItemID != 1 {
		t.Fatalf("ListAll = %+v", entries)
	}

	read, err := client.Read(context.Background(), offerID, entries[0].ItemID, 0, granted.MaxChunk)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(read.Data) != "hello, dnd" || read.Flags&far2ldnd.ReadEOF == 0 {
		t.Fatalf("Read = %+v", read)
	}

	if err := client.Close(context.Background(), offerID, far2ldnd.CloseProcessed); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// CLOSE is idempotent, including on an offer already released (§ 6.5).
	if err := client.Close(context.Background(), offerID, far2ldnd.CloseProcessed); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	if err := client.Unbind(context.Background()); err != nil {
		t.Fatalf("Unbind: %v", err)
	}
	if _, ok := client.Bound(); ok {
		t.Fatal("Bound() after Unbind")
	}
}

// asyncWire, unlike syncWire, only forwards bytes to the terminal side: it
// never waits for or delivers the reply itself. The test pumps replies in by
// hand, at the moment it chooses -- the only way to make a call's own ctx
// cancellation race a real, still-outstanding reply.
type asyncWire struct {
	e *dndEnv
}

func (w *asyncWire) Write(b []byte) (int, error) {
	w.e.p.Process(b)
	return len(b), nil
}

// TestDNDClientCancelRetainsRID cancels a List in flight and checks, by the
// rule of § 8, that its RID stays reserved until the late reply actually
// arrives -- never freed just because the caller stopped waiting. It then
// completes the § 8 cancellation sequence with a real CLOSE.
func TestDNDClientCancelRetainsRID(t *testing.T) {
	e := newDNDEnv(t)
	e.bind(t, dndBindingA, 65536, 32768, far2ldnd.FeatureStream)
	offerID := e.offer(t, oneFile(1, streamFlags, "x"))
	e.pty.Reset()

	w := &asyncWire{e: e}
	client := NewDNDClient(w)
	client.mu.Lock()
	client.bound, client.binding = true, dndBindingA
	client.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		reply far2ldnd.ListReply
		err   error
	}
	done := make(chan result, 1)
	go func() {
		reply, err := client.List(ctx, offerID, far2ldnd.CursorFirst)
		done <- result{reply, err}
	}()

	// Wait for the request to actually reach the wire before cancelling,
	// so the race is genuine: the server has not answered yet either way,
	// since dndDrain answers asynchronously.
	deadline := e.wait(t, 1)
	cancel()
	res := <-done
	if !errors.Is(res.err, context.Canceled) {
		t.Fatalf("List after cancel: reply=%+v err=%v, want context.Canceled", res.reply, res.err)
	}

	client.rmu.Lock()
	pending := len(client.pending)
	client.rmu.Unlock()
	if pending != 1 {
		t.Fatalf("pending RIDs after cancel = %d, want 1 (held, not freed -- § 8)", pending)
	}

	// The late reply now arrives; it must be absorbed without panicking and
	// must free the RID it was reserved for.
	k, stack, _, err := far2ldnd.ParseFrame(deadline[0])
	if err != nil || k != far2ldnd.FrameReply {
		t.Fatalf("late reply frame: %v", err)
	}
	if err := client.HandleFrame(k, stack); err != nil {
		t.Fatalf("HandleFrame(late reply): %v", err)
	}
	client.rmu.Lock()
	pending = len(client.pending)
	client.rmu.Unlock()
	if pending != 0 {
		t.Fatalf("pending RIDs after the late reply = %d, want 0 (released)", pending)
	}

	// § 8 step 2: the caller completes the cancellation with CLOSE. This
	// uses a fresh RID of its own and must succeed normally.
	e.pty.Reset()
	closeDone := make(chan error, 1)
	go func() { closeDone <- client.Close(context.Background(), offerID, far2ldnd.CloseCancelled) }()
	closeFrame := e.wait(t, 1)[0]
	k, stack, _, err = far2ldnd.ParseFrame(closeFrame)
	if err != nil || k != far2ldnd.FrameReply {
		t.Fatalf("close reply frame: %v", err)
	}
	if err := client.HandleFrame(k, stack); err != nil {
		t.Fatalf("HandleFrame(close reply): %v", err)
	}
	if err := <-closeDone; err != nil {
		t.Fatalf("Close after cancel: %v", err)
	}

	client.rmu.Lock()
	pending = len(client.pending)
	client.rmu.Unlock()
	if pending != 0 {
		t.Fatalf("pending RIDs after Close = %d, want 0", pending)
	}
}

// TestDNDClientCloseFallsBackToRID0WhenExhausted checks the § 8 p.2 escape
// hatch in isolation, without a real server: with every RID already
// reserved, Close must not block waiting for a free one -- it revokes the
// offer with RID 0, best-effort, and returns at once.
func TestDNDClientCloseFallsBackToRID0WhenExhausted(t *testing.T) {
	var buf bytes.Buffer
	client := NewDNDClient(&buf)
	client.rmu.Lock()
	for i := 1; i <= 255; i++ {
		client.pending[uint8(i)] = make(chan dndReply, 1) //nolint:gosec // 1..255 fits uint8
	}
	client.rmu.Unlock()

	offerID := far2ldnd.ID{0x01}
	if err := client.Close(context.Background(), offerID, far2ldnd.CloseCancelled); err != nil {
		t.Fatalf("Close with RIDs exhausted: %v", err)
	}

	k, stack, _, err := far2ldnd.ParseFrame(buf.String())
	if err != nil || k != far2ldnd.FrameRequest {
		t.Fatalf("frame written under exhaustion: %v", err)
	}
	rid, q, err := far2ldnd.DecodeRequest(stack)
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if rid != 0 {
		t.Fatalf("RID = %d, want 0 (best-effort revoke, § 8 p.2)", rid)
	}
	cq, ok := q.(*far2ldnd.CloseRequest)
	if !ok || cq.Offer != offerID || cq.Reason != far2ldnd.CloseCancelled {
		t.Fatalf("request = %+v (%T)", q, q)
	}
}

// TestDNDClientHandleFrameIgnoresForeign checks the robustness HandleFrame
// needs to sit on a channel shared with other far2l traffic (§ 8: a late or
// foreign reply must not disturb an unrelated RID): a reply for a RID this
// client never reserved, and an f2l event of a code other than INPUT_DND,
// are both silently ignored; a FrameRequest -- the direction only an
// application ever sends -- is refused outright.
func TestDNDClientHandleFrameIgnoresForeign(t *testing.T) {
	var buf bytes.Buffer
	client := NewDNDClient(&buf)
	client.OnEvent = func(far2ldnd.Event) { t.Fatal("OnEvent fired for a non-DND f2l code") }

	foreignReply, err := far2ldnd.EncodeReply(99, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.HandleFrame(far2ldnd.FrameReply, foreignReply); err != nil {
		t.Fatalf("HandleFrame(foreign reply): %v", err)
	}

	// Any f2l event whose last byte -- the event code, popped first -- is
	// not far2ldnd.InputDND ('D') belongs to another kind of event.
	if err := client.HandleFrame(far2ldnd.FrameEvent, []byte{0, 0, 'K'}); err != nil {
		t.Fatalf("HandleFrame(foreign event): %v", err)
	}

	reqStack, err := far2ldnd.EncodeRequest(1, &far2ldnd.CloseRequest{Offer: far2ldnd.ID{1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.HandleFrame(far2ldnd.FrameRequest, reqStack); err == nil {
		t.Fatal("HandleFrame accepted a FrameRequest, the application-to-terminal direction")
	}
}

// chanWire hands each written frame to the test over a channel instead of a
// shared buffer, so a client call running on its own goroutine can be pumped
// from the test goroutine without racing on the transport itself.
type chanWire struct{ frames chan []byte }

func (w *chanWire) Write(b []byte) (int, error) {
	w.frames <- append([]byte(nil), b...)
	return len(b), nil
}

// TestDNDClientBindOldServer checks the § 6.1 old-server signal: a reply
// that is empty once the RID is popped -- what an unknown interact command
// leaves behind -- is far2ldnd.ErrNoDND, not a successful empty BIND.
func TestDNDClientBindOldServer(t *testing.T) {
	w := &chanWire{frames: make(chan []byte, 1)}
	client := NewDNDClient(w)

	done := make(chan error, 1)
	go func() {
		_, err := client.Bind(context.Background(), far2ldnd.FeatureStream, DefaultDNDClientLimits)
		done <- err
	}()

	req := <-w.frames
	_, stack, _, err := far2ldnd.ParseFrame(string(req))
	if err != nil {
		t.Fatalf("parse the request just written: %v", err)
	}
	rid := stack[len(stack)-1] // EncodeRequest lays the RID out last (§ 6)

	if err := client.HandleFrame(far2ldnd.FrameReply, []byte{rid}); err != nil {
		t.Fatalf("HandleFrame(bare-RID reply): %v", err)
	}
	if err := <-done; !errors.Is(err, far2ldnd.ErrNoDND) {
		t.Fatalf("Bind against an old server: %v, want ErrNoDND", err)
	}
}
