//go:build !linux && !windows && !darwin && !dragonfly && !freebsd && !netbsd && !openbsd

package dirwatch

import "errors"

// newNativeSource: no OS notification mechanism is used here; Watch
// falls back to polling.
func newNativeSource(string) (*rawSource, error) {
	return nil, errors.New("dirwatch: no native watcher on this OS")
}
