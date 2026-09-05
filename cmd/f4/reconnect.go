package main

import (
	"context"
	"fmt"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// reconnectTimeout bounds one attempt at rebuilding a connection. It is
// deliberately longer than a request timeout: the user has just been asked
// whether to wait and answered yes, so giving up quickly would only make them
// answer again.
const reconnectTimeout = 30 * time.Second

// reconnectChoice is what the user answered.
type reconnectChoice int

const (
	// reconnectRetry — the connection was rebuilt and the operation may be
	// attempted again. It is only ever handed to a caller that said its
	// operation is safe to repeat.
	reconnectRetry reconnectChoice = iota
	// reconnectOffline — leave the panel where it is with what it already
	// shows. The session stays broken and the next operation asks again.
	reconnectOffline
	// reconnectLeave — the user is done with this file system.
	reconnectLeave
)

// reconnectAsk shows the question. It is a variable so that a test can answer
// it without a screen; nothing else replaces it.
var reconnectAsk = func(title, message string, buttons []string, onChoice func(int)) {
	dlg := vtui.ShowMessage(title, message, buttons)
	dlg.OnResult = onChoice
}

// reconnectPostUI hands work back to the UI thread. Also a variable, and for
// the same reason: a test has no frame manager to post to.
var reconnectPostUI = func(f func()) { vtui.FrameManager.PostTask(f) }

// reconnectRunner performs the attempt itself, off the UI thread.
var reconnectRunner = func(r vfs.SessionReconnector, done func(error)) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), reconnectTimeout)
		defer cancel()
		err := r.Reconnect(ctx)
		reconnectPostUI(func() { done(err) })
	}()
}

// reconnectorFor reports the file system to offer a reconnect on, or nil when
// there is nothing to offer: an error that is not a lost session, a file
// system that does not live on a connection, or one that cannot open a second
// one. Answering nil is the normal case and means the caller reports the error
// the way it always did.
func reconnectorFor(fs vfs.VFS, err error) vfs.SessionReconnector {
	if fs == nil || err == nil {
		return nil
	}
	r, ok := fs.(vfs.SessionReconnector)
	if !ok {
		return nil
	}
	if !r.SessionLost(err) || !r.CanReconnect() {
		return nil
	}
	return r
}

// sessionKeyOf identifies the connection a file system speaks through, so that
// everything running on it can be found again. A file system that does not
// share a connection answers as itself, which is an identity nothing else will
// match; one that has no session at all is nil, and nil matches nothing.
func sessionKeyOf(fs vfs.VFS) any {
	if fs == nil {
		return nil
	}
	if id, ok := fs.(vfs.SessionIdentity); ok {
		return id.SessionKey()
	}
	return nil
}

// offerReconnect asks what to do about a connection that died under an
// operation, and reports whether it took responsibility for the error. When it
// returns false nothing was shown and the caller reports the error itself.
//
// what names the operation in the message, in a form that fits after "while":
// "reading the directory", "saving the file". retryable says whether repeating
// it is honest — a read is, a half finished write is not, because nobody knows
// how much of it arrived before the session died. A caller that says false
// still gets the reconnect offered, since the panel is worth having back; what
// it does not get is reconnectRetry, and the dialog says so rather than
// silently doing less than it looks like it does.
//
// done runs on the UI thread, once, with the choice the user made. For
// reconnectRetry the error is nil; a reconnect that failed is reported as
// reconnectOffline with the error that stopped it, because a panel that could
// not be rebuilt is exactly a panel left as it was.
func offerReconnect(fs vfs.VFS, err error, what string, retryable bool, done func(reconnectChoice, error)) bool {
	r := reconnectorFor(fs, err)
	if r == nil {
		return false
	}
	// The far side keeps nothing of a session that dropped, so whatever was
	// running there is already gone. The registry is told now rather than
	// after a successful reconnect: the work is equally dead if the user
	// chooses to work offline, and a job left in the list waiting for an
	// answer that cannot arrive is worse than one that says it was lost.
	lost := GlobalBackgroundJobs.SessionLost(sessionKeyOf(fs))

	msg := fmt.Sprintf("The connection was lost while %s:\n%v", what, err)
	if lost == 1 {
		msg += "\n\nOne background job was running on it and is gone."
	} else if lost > 1 {
		msg += fmt.Sprintf("\n\n%d background jobs were running on it and are gone.", lost)
	}
	if retryable {
		msg += "\n\nReconnecting starts a new session and repeats the operation."
	} else {
		msg += "\n\nReconnecting starts a new session, but this operation cannot" +
			"\nbe resumed and has to be started again by hand."
	}
	buttons := []string{"&Reconnect", "Work &offline", "&Close panel"}
	reconnectAsk(" Connection lost ", msg, buttons, func(code int) {
		switch code {
		case 0:
			reconnectRunner(r, func(rerr error) {
				if rerr != nil {
					done(reconnectOffline, rerr)
					return
				}
				if !retryable {
					done(reconnectOffline, nil)
					return
				}
				done(reconnectRetry, nil)
			})
		case 2:
			done(reconnectLeave, nil)
		default:
			// Anything else, Escape included, leaves things as they are. That
			// is the answer that changes nothing, which is what a dialog the
			// user dismissed should mean.
			done(reconnectOffline, nil)
		}
	})
	return true
}

// offerPanelReconnect is the panel's use of it: a directory listing is a read,
// so repeating it is honest, and repeating it is also all it takes to get the
// panel back.
func (fp *FileSystemPanel) offerPanelReconnect(err error, keepEntries bool) bool {
	return offerReconnect(fp.vfs, err, "reading the directory", true, func(c reconnectChoice, rerr error) {
		switch c {
		case reconnectRetry:
			fp.readDirectoryEx(keepEntries)
		case reconnectLeave:
			fp.leaveVFS()
		default:
			shown := err
			if rerr != nil {
				shown = rerr
			}
			fp.updateTitle(shown)
			vtui.ShowMessage(" Error ", fmt.Sprintf("Failed to read directory:\n%v", shown), []string{"&Ok"})
		}
	})
}

// leaveVFS drops the file system the panel is standing in and shows the one
// underneath, which is what entering ".." at the root of a virtual file system
// does. A panel whose file system has no parent stays where it is: there is
// nothing to fall back to, and closing it would leave the frame with a hole.
func (fp *FileSystemPanel) leaveVFS() {
	parent := fp.vfs.ParentVFS()
	if parent == nil {
		return
	}
	oldPath := fp.vfs.GetPath()
	fp.cancelProviderOpen()
	fp.vfs.Close()
	fp.vfs = parent
	fp.showCurrentVFSLoadingRows()
	if fp.providerEntryName != "" {
		fp.pendingSelection = fp.providerEntryName
		fp.providerEntryName = ""
	} else {
		fp.pendingSelection = fp.vfs.Base(oldPath)
	}
	fp.ReadDirectory()
}
