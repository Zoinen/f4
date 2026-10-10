//go:build linux

package dirwatch

import (
	"sync"

	"golang.org/x/sys/unix"
)

// inotifyMask is what makes a panel listing stale: entries appearing,
// disappearing or being renamed, and their size, time or mode changing.
const inotifyMask = unix.IN_CREATE | unix.IN_DELETE | unix.IN_DELETE_SELF |
	unix.IN_MOVED_FROM | unix.IN_MOVED_TO | unix.IN_MOVE_SELF |
	unix.IN_MODIFY | unix.IN_ATTRIB | unix.IN_CLOSE_WRITE

// newNativeSource watches dir with inotify. The reader goroutine polls the
// descriptor with a short timeout so that close can stop it without racing a
// blocked read against closing the descriptor.
func newNativeSource(dir string) (*rawSource, error) {
	fd, err := unix.InotifyInit1(unix.IN_CLOEXEC | unix.IN_NONBLOCK)
	if err != nil {
		return nil, err
	}
	if _, err := unix.InotifyAddWatch(fd, dir, inotifyMask); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	events := make(chan struct{}, 1)
	stop := make(chan struct{})
	finished := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(finished)
		defer func() { _ = unix.Close(fd) }()
		buf := make([]byte, 16*unix.SizeofInotifyEvent+4096)
		// #nosec G115 -- fd is a small non-negative descriptor from inotify_init1.
		fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		for {
			select {
			case <-stop:
				return
			default:
			}
			n, err := unix.Poll(fds, 100)
			if err != nil && err != unix.EINTR {
				return
			}
			if n <= 0 {
				continue
			}
			// Drain what is there; one event value is enough for the lot.
			got := false
			for {
				m, err := unix.Read(fd, buf)
				if m > 0 {
					got = true
				}
				if err != nil || m <= 0 {
					break
				}
			}
			if got {
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
