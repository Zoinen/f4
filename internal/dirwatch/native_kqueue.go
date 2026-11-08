//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package dirwatch

import (
	"os"
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

// newNativeSource watches dir with kqueue. Closing the wake pipe interrupts
// the idle kernel wait immediately, without closing a descriptor still in use.
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
	wakeReader, wakeWriter, err := os.Pipe()
	if err != nil {
		_ = unix.Close(kq)
		_ = unix.Close(dfd)
		return nil, err
	}
	var changes [2]unix.Kevent_t
	unix.SetKevent(&changes[0], dfd, unix.EVFILT_VNODE, unix.EV_ADD|unix.EV_CLEAR)
	changes[0].Fflags = kqueueNotes
	unix.SetKevent(&changes[1], int(wakeReader.Fd()), unix.EVFILT_READ, unix.EV_ADD|unix.EV_CLEAR)
	if _, err := unix.Kevent(kq, changes[:], nil, nil); err != nil {
		_ = wakeReader.Close()
		_ = wakeWriter.Close()
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
			_ = wakeReader.Close()
		}()
		out := make([]unix.Kevent_t, 4)
		for {
			select {
			case <-stop:
				return
			default:
			}
			n, err := unix.Kevent(kq, nil, out, nil)
			if err != nil && err != unix.EINTR {
				return
			}
			select {
			case <-stop:
				return
			default:
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
			once.Do(func() {
				close(stop)
				_ = wakeWriter.Close()
			})
			<-finished
		},
	}, nil
}
