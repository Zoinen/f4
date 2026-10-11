package app

import (
	"context"
	"sync"
	"time"

	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/terminal/far2ldnd"
	"github.com/unxed/vtui"
)

// The terminal is told that this program takes drops (far2l DND/BIND,
// unxed/f4#1628) only once it has acknowledged the far2l extensions:
// without that acknowledgement the request would be ignored, or printed on
// the screen by a terminal that does not know the protocol. The
// acknowledgement is an asynchronous event that vtui consumes itself and
// never passes to the event filter, so it is polled for.
const (
	terminalDNDPollEvery  = 200 * time.Millisecond
	terminalDNDGiveUpAt   = 30 * time.Second
	terminalDNDBindLimit  = 10 * time.Second
	terminalDNDUnbindWait = 500 * time.Millisecond
)

// terminalDNDClient is the part of terminal.DNDClient the binding needs.
type terminalDNDClient interface {
	Bind(ctx context.Context, wantedFeatures uint32, limits terminal.DNDClientLimits) (far2ldnd.BindReply, error)
	Unbind(ctx context.Context) error
}

// terminalDNDStop switches the binding off; set by startTerminalDNDBinding,
// called by the deferred cleanup of Main.
var terminalDNDStop = func() {}

// startTerminalDNDBinding binds client as soon as negotiated reports that
// the terminal acknowledged the far2l extensions and returns the function
// that undoes it. A terminal that never acknowledges (a plain terminal, a
// native window) is never sent anything.
func startTerminalDNDBinding(client terminalDNDClient, negotiated func() bool) (stop func()) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var mu sync.Mutex
	bound := false
	go func() {
		defer close(done)
		tick := time.NewTicker(terminalDNDPollEvery)
		defer tick.Stop()
		giveUp := time.NewTimer(terminalDNDGiveUpAt)
		defer giveUp.Stop()
		for !negotiated() {
			select {
			case <-ctx.Done():
				return
			case <-giveUp.C:
				return
			case <-tick.C:
			}
		}
		bindCtx, bindCancel := context.WithTimeout(ctx, terminalDNDBindLimit)
		defer bindCancel()
		_, err := client.Bind(bindCtx, far2ldnd.FeatureStream|far2ldnd.FeatureReference, terminal.DefaultDNDClientLimits)
		if err != nil {
			// A terminal without DND (far2ldnd.ErrNoDND) or one that refuses the
			// profile is an ordinary outcome, not a fault.
			vtui.DebugLog("DND: BIND not granted by the terminal: %v", err)
			return
		}
		mu.Lock()
		bound = true
		mu.Unlock()
		vtui.DebugLog("DND: bound to the terminal")
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
			mu.Lock()
			wasBound := bound
			mu.Unlock()
			if !wasBound {
				return
			}
			// The event loop that delivers the reply may already be gone at
			// exit, so the wait is short; the terminal drops the binding with
			// the connection anyway.
			unbound := make(chan struct{})
			go func() {
				defer close(unbound)
				uctx, ucancel := context.WithTimeout(context.Background(), terminalDNDUnbindWait)
				defer ucancel()
				_ = client.Unbind(uctx)
			}()
			select {
			case <-unbound:
			case <-time.After(terminalDNDUnbindWait):
			}
		})
	}
}
