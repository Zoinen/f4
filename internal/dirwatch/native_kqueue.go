//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package dirwatch

import (
	"sync"

	"golang.org/x/sys/unix"
)

// kqueue reports the directory itself: an entry added, removed or renamed
// there (NOTE_WRITE on a directory), the directory going away, attributes
// changing. A file changing its content in place does not touch the
// directory's vnode, so such a change reaches the panel with the next reload
// only -- the same limit the listing of names has everywhere kqueue is used.
const kqueueNotes = unix.NOTE_WRITE | unix.NOTE_DELETE | unix.NOTE_RENAME |
	unix.NOTE_ATTRIB | unix.NOTE_EXTEND | unix.NOTE_LINK | unix.NOTE_REVOKE

// newNativeSource watches dir with kqueue; the reader waits with a short
// timeout so that close can stop it.
func newNativeSource(dir string) (*rawSource, error) {
	dfd, err := unix.Open(dir, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	kq, err := unix.Kqueue()
	if err != nil {
		_ = unix.Close(dfd)
		return nil, err
	}
	var change unix.Kevent_t
	unix.SetKevent(&change, dfd, unix.EVFILT_VNODE, unix.EV_ADD|unix.EV_CLEAR)
	change.Fflags = kqueueNotes
	if _, err := unix.Kevent(kq, []unix.Kevent_t{change}, nil, nil); err != nil {
		_ = unix.Close(kq)
		_ = unix.Close(dfd)
		return nil, err
	}

	events := make(chan struct{}, 1)
	stop := make(chan struct{})
	finished := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(finished)
		defer func() {
			_ = unix.Close(kq)
			_ = unix.Close(dfd)
		}()
		timeout := unix.NsecToTimespec(100 * 1000 * 1000)
		out := make([]unix.Kevent_t, 4)
		for {
			select {
			case <-stop:
				return
			default:
			}
			n, err := unix.Kevent(kq, nil, out, &timeout)
			if err != nil && err != unix.EINTR {
				return
			}
			if n > 0 {
				select {
				case events <- struct{}{}:
				default:
				}
			}
		}
	}()
	return &rawSource{
		events: events,
		native: true,
		close: func() {
			once.Do(func() { close(stop) })
			<-finished
		},
	}, nil
}
