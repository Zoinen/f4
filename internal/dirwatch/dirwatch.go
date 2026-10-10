// Package dirwatch tells when the contents of one local directory change
// (f4#1668, part 1), so a panel can refresh itself instead of waiting for
// the user to press Ctrl+R.
//
// Where the OS can report changes -- inotify on Linux, kqueue on macOS and the
// BSDs, a change-notification handle on Windows -- Watch uses that; on any
// other OS, and where the mechanism is unavailable or out of watches, it
// falls back to comparing a snapshot of the directory (names, sizes and
// modification times) once per PollInterval.
//
// Raw events are debounced: the callback runs once after Quiet has passed
// with no new event, and no later than MaxWait after the first event of a
// burst, so a directory that changes all the time (a build writing files)
// refreshes at a steady pace instead of once per file. The callback runs on
// the watcher's own goroutine and never concurrently with itself; it must
// hand the work over to the UI thread.
//
// Only a directory the OS reports as a local one should be watched: the
// caller must not pass paths of a remote or plugin VFS.
package dirwatch

import (
	"errors"
	"os"
	"sync"
	"time"
)

// Options tunes a Watcher; zero fields take the defaults below.
type Options struct {
	// Quiet is how long the directory must be silent before the callback
	// runs (default 150ms).
	Quiet time.Duration
	// MaxWait caps how long a continuous burst of events may postpone the
	// callback (default 1s).
	MaxWait time.Duration
	// PollInterval is the snapshot period of the fallback (default 1s).
	PollInterval time.Duration
	// ForcePolling skips the OS notification mechanism.
	ForcePolling bool
}

const (
	defaultQuiet   = 150 * time.Millisecond
	defaultMaxWait = time.Second
	defaultPoll    = time.Second
)

func (o Options) withDefaults() Options {
	if o.Quiet <= 0 {
		o.Quiet = defaultQuiet
	}
	if o.MaxWait <= 0 {
		o.MaxWait = defaultMaxWait
	}
	if o.MaxWait < o.Quiet {
		o.MaxWait = o.Quiet
	}
	if o.PollInterval <= 0 {
		o.PollInterval = defaultPoll
	}
	return o
}

// Watcher is one running watch; Close stops it.
type Watcher struct {
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
	native   bool
}

// Native reports whether the OS notification mechanism is in use rather
// than the polling fallback.
func (w *Watcher) Native() bool { return w.native }

// Close stops the watch and waits until the callback is no longer running.
// It is safe to call more than once. Do not call it from the callback.
func (w *Watcher) Close() {
	w.stopOnce.Do(func() { close(w.stop) })
	<-w.done
}

// rawSource delivers one value on events for every change the OS reports (or
// the snapshot comparison notices); close releases its resources and is
// called once, after the debounce loop has stopped reading.
type rawSource struct {
	events <-chan struct{}
	close  func()
	native bool
}

// Watch starts watching dir and calls onChange after its contents change.
func Watch(dir string, opts Options, onChange func()) (*Watcher, error) {
	if onChange == nil {
		return nil, errors.New("dirwatch: nil callback")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("dirwatch: not a directory: " + dir)
	}
	opts = opts.withDefaults()

	var src *rawSource
	if !opts.ForcePolling {
		if s, err := newNativeSource(dir); err == nil {
			src = s
		}
	}
	if src == nil {
		src = newPollSource(dir, opts.PollInterval)
	}

	w := &Watcher{stop: make(chan struct{}), done: make(chan struct{}), native: src.native}
	go func() {
		defer close(w.done)
		defer src.close()
		debounce(src.events, w.stop, opts.Quiet, opts.MaxWait, onChange)
	}()
	return w, nil
}

// debounce turns bursts of events into single calls of fire: fire runs when
// quiet has passed since the last event of a burst, or maxWait since its
// first, whichever comes first. It returns when stop is closed.
func debounce(events <-chan struct{}, stop <-chan struct{}, quiet, maxWait time.Duration, fire func()) {
	var (
		timer   *time.Timer
		timerC  <-chan time.Time
		started time.Time
	)
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	for {
		select {
		case <-stop:
			return
		case <-events:
			now := time.Now()
			if timerC == nil {
				started = now
			}
			wait := quiet
			if left := maxWait - now.Sub(started); left < wait {
				wait = left
			}
			if wait < 0 {
				wait = 0
			}
			if timer == nil {
				timer = time.NewTimer(wait)
			} else {
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(wait)
			}
			timerC = timer.C
		case <-timerC:
			timerC = nil
			select {
			case <-stop:
				return
			default:
			}
			fire()
		}
	}
}
