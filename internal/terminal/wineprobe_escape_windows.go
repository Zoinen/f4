//go:build windows

package terminal

import (
	"fmt"

	winescape "github.com/unxed/libwinescape/go"

	"github.com/unxed/f4/vfs/hostmode"
)

// winescapeFacts reports what the file layer's personality was decided from:
// the three libwinescape probes, the UseWinescape setting, and the answer
// itself. Without these lines "is posix mode on, and why" could only be read
// out of the debug log, which is a poor way to ask a user running an unusual
// Windows -- Wine, ReactOS, a nine-year-old build -- what their machine says.
//
// hostmode.Posix() decides the personality the first time it is called, so
// asking it here settles it for this process. That is harmless: the probe
// prints and f4 exits without starting the UI.
func winescapeFacts() [][2]string {
	return [][2]string{
		{"winescape.IsWine", fmt.Sprint(winescape.IsWine())},
		{"winescape.Available", fmt.Sprint(winescape.Available())},
		{"winescape.HostOS", orEmptyMarker(winescape.HostOS())},
		{"UseWinescape setting", fmt.Sprint(hostmode.Allowed())},
		{"hostmode.Posix", fmt.Sprint(hostmode.Posix())},
	}
}
