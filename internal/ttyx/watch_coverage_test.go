package ttyx

import (
	"github.com/jezek/xgb/xproto"
	"testing"
)

func TestPumpWithoutConnectionReturns(t *testing.T) {
	var session Session
	session.pump()
}

func TestNotifyKeepsOnePendingToken(t *testing.T) {
	session := &Session{changed: make(chan struct{}, 1)}
	session.notify()
	session.notify()

	select {
	case <-session.Changed():
	default:
		t.Fatal("notify did not publish a token")
	}
	select {
	case <-session.Changed():
		t.Fatal("notify published more than one pending token")
	default:
	}
}

func TestDispatchFocusAndWindowLifecycle(t *testing.T) {
	session := &Session{win: 7, alive: true, changed: make(chan struct{}, 1)}

	session.dispatch(xproto.FocusInEvent{Mode: xproto.NotifyModeNormal, Detail: xproto.NotifyDetailNonlinear})
	if !session.Focused() {
		t.Fatal("FocusIn did not focus the session")
	}
	<-session.Changed()

	session.dispatch(xproto.FocusOutEvent{Mode: xproto.NotifyModeNormal, Detail: xproto.NotifyDetailNonlinear})
	if session.Focused() {
		t.Fatal("FocusOut did not unfocus the session")
	}
	<-session.Changed()

	session.dispatch(xproto.ConfigureNotifyEvent{Window: session.win})
	session.dispatch(xproto.UnmapNotifyEvent{Window: session.win})
	if session.Focused() {
		t.Fatal("UnmapNotify did not unfocus the session")
	}

	session.dispatch(xproto.DestroyNotifyEvent{Window: session.win})
	if session.Alive() {
		t.Fatal("DestroyNotify left the session alive")
	}
	select {
	case <-session.Changed():
	default:
		t.Fatal("DestroyNotify did not notify listeners")
	}

	session.dispatch(xproto.DestroyNotifyEvent{Window: session.win + 1})
}

func TestUnregisterOverlayRemovesEveryMatchingEntry(t *testing.T) {
	first := &Overlay{}
	second := &Overlay{}
	session := &Session{overlays: []*Overlay{first, second, first}}

	session.unregisterOverlay(first)
	if len(session.overlays) != 1 || session.overlays[0] != second {
		t.Fatalf("remaining overlays = %v", session.overlays)
	}
}
