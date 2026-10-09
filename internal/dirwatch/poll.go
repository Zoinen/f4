package dirwatch

import (
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// snapshot is a comparable summary of a directory: every entry's name, size
// and modification time, in name order.
func snapshot(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		// A vanished or unreadable directory is a change worth reporting
		// once; the marker keeps it from being reported again and again.
		return "!" + err.Error()
	}
	names := make([]string, 0, len(entries))
	byName := make(map[string]os.DirEntry, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
		byName[e.Name()] = e
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		b.WriteString(name)
		if info, err := byName[name].Info(); err == nil {
			b.WriteByte('\x00')
			b.WriteString(strconv.FormatInt(info.Size(), 10))
			b.WriteByte('\x00')
			b.WriteString(strconv.FormatInt(info.ModTime().UnixNano(), 10))
			b.WriteByte('\x00')
			b.WriteString(info.Mode().String())
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// newPollSource compares snapshots of dir every interval.
func newPollSource(dir string, interval time.Duration) *rawSource {
	events := make(chan struct{}, 1)
	stop := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		last := snapshot(dir)
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				cur := snapshot(dir)
				if cur == last {
					continue
				}
				last = cur
				select {
				case events <- struct{}{}:
				default:
				}
			}
		}
	}()
	return &rawSource{
		events: events,
		close: func() {
			close(stop)
			<-finished
		},
	}
}
