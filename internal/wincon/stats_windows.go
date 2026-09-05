//go:build windows

package wincon

import "sync/atomic"

// counters is the live form. atomic.Uint64 rather than a plain one because f4
// is built for 32-bit ARM, where a bare 64-bit atomic has to be aligned by
// hand and a field that moves in the struct silently stops being so.
type counters struct {
	applies     atomic.Uint64
	moves       atomic.Uint64
	regions     atomic.Uint64
	invalidates atomic.Uint64
	paints      atomic.Uint64
	blank       atomic.Uint64
}

func (c *counters) snapshot() Stats {
	return Stats{
		Applies:     c.applies.Load(),
		Moves:       c.moves.Load(),
		Regions:     c.regions.Load(),
		Invalidates: c.invalidates.Load(),
		Paints:      c.paints.Load(),
		Blank:       c.blank.Load(),
	}
}
