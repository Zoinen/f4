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
// above without saying which; that is all the debounce loop needs. Wait on
// both the notification and a stop event so Close can wake an idle reader.
func newNativeSource(dir string) (*rawSource, error) {
	h, err := windows.FindFirstChangeNotification(dir, false, windowsFilter)
	if err != nil {
		return nil, err
	}
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		_ = windows.FindCloseChangeNotification(h)
		return nil, err
	}
	events := make(chan struct{}, 1)
	finished := make(chan struct{})
	var once sync.Once
	go func() {
		defer close(finished)
		defer func() { _ = windows.FindCloseChangeNotification(h) }()
		handles := []windows.Handle{stop, h}
		for {
			result, err := windows.WaitForMultipleObjects(handles, false, windows.INFINITE)
			if err != nil || result != windows.WAIT_OBJECT_0+1 {
				return
			}
			select {
			case events <- struct{}{}:
			default:
			}
			if err := windows.FindNextChangeNotification(h); err != nil {
				return
			}
		}
	}()
	return &rawSource{
		events: events,
		native: true,
		close: func() {
			once.Do(func() {
				_ = windows.SetEvent(stop)
				<-finished
				_ = windows.CloseHandle(stop)
			})
		},
	}, nil
}
