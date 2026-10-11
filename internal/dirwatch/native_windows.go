//go:build windows

package dirwatch

import (
	"sync"

	"golang.org/x/sys/windows"
)

// windowsFilter is what changes a panel listing: names of files and
// directories, attributes, size and last-write time.
const windowsFilter = windows.FILE_NOTIFY_CHANGE_FILE_NAME | windows.FILE_NOTIFY_CHANGE_DIR_NAME |
	windows.FILE_NOTIFY_CHANGE_ATTRIBUTES | windows.FILE_NOTIFY_CHANGE_SIZE |
	windows.FILE_NOTIFY_CHANGE_LAST_WRITE

// newNativeSource watches dir with a change-notification handle
// (FindFirstChangeNotification), which is signalled on any of the changes
// above without saying which; that is all the debounce loop needs. The reader
// waits with a short timeout so that close can stop it.
func newNativeSource(dir string) (*rawSource, error) {
	h, err := windows.FindFirstChangeNotification(dir, false, windowsFilter)
	if err != nil {
		return nil, err
	}
	events := make(chan struct{}, 1)
	stop := make(chan struct{})
	finished := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(finished)
		defer func() { _ = windows.FindCloseChangeNotification(h) }()
		for {
			select {
			case <-stop:
				return
			default:
			}
			r, err := windows.WaitForSingleObject(h, 100)
			if err != nil {
				return
			}
			switch r {
			case windows.WAIT_OBJECT_0:
				select {
				case events <- struct{}{}:
				default:
				}
				if err := windows.FindNextChangeNotification(h); err != nil {
					return
				}
			case uint32(windows.WAIT_TIMEOUT):
			default:
				return
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
