//go:build windows

package winshell

import (
	"fmt"

	"github.com/unxed/f4/vfs"
)

// RegisterDefaultProvider exposes persistent windows:// history/session paths.
// Registration itself is synchronous and does not start the broker.
func RegisterDefaultProvider() error {
	if existing := vfs.FindURIProvider(URIFromParsingName("Desktop")); existing != nil {
		if _, owned := existing.(*uriProvider); owned {
			return nil
		}
		return fmt.Errorf("register Windows Shell URI provider: scheme %q is already owned", Scheme)
	}
	return vfs.RegisterURIProvider(&uriProvider{client: DefaultClient})
}
