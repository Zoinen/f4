package wincon

// What the pump thread did, counted rather than logged.
//
// The pump thread must not write a log line: its input queue is attached to
// conhost's, so anything it does that can block — and a write to a file can —
// is a way of stopping the console it is drawing over. It counts instead, and
// whoever is composing frames reads the counters and prints them, at most one
// line a second. That also keeps the log small enough to be read by a person
// and pasted into an issue.

// Stats is one reading of the counters.
type Stats struct {
	// Applies is the number of wake-ups the pump thread answered.
	Applies uint64
	// Moves is how many of them showed or moved the window.
	Moves uint64
	// Regions is how many of them reshaped it.
	Regions uint64
	// Invalidates is how many of them asked for a repaint.
	Invalidates uint64
	// Paints is how many WM_PAINT the window served.
	Paints uint64
	// Blank is how many of those found no frame buffer to paint, which is
	// what a black rectangle looks like from in here.
	Blank uint64
}

// Sub is the difference between two readings, for a report about a period
// rather than about the whole run.
func (s Stats) Sub(prev Stats) Stats {
	return Stats{
		Applies:     s.Applies - prev.Applies,
		Moves:       s.Moves - prev.Moves,
		Regions:     s.Regions - prev.Regions,
		Invalidates: s.Invalidates - prev.Invalidates,
		Paints:      s.Paints - prev.Paints,
		Blank:       s.Blank - prev.Blank,
	}
}

// Empty says nothing happened in this period.
func (s Stats) Empty() bool {
	return s == Stats{}
}
