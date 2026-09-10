package terminal

import (
	"bytes"
	"strings"
	"testing"
)

// The questions the probe asks are answered on standard input, and so is the
// far2l announcement that was sent a moment earlier. Whichever answer arrives
// second is read and dropped, and when that is the acknowledgement the
// clipboard loses far2l in both directions for the rest of the session
// (#922). The announcement is repeated so the reader gets one of its own.
func TestRestoreFar2lAfterProbe_AsksAgainWhenTheAckWasEaten(t *testing.T) {
	t.Cleanup(func() { takeProbeInput() })

	noteProbeInput("\x1b_far2lok\x07")
	noteProbeInput("\x1b[4;856;1319t")

	var out bytes.Buffer
	if !restoreFar2lAfterProbe(&out) {
		t.Fatal("the acknowledgement was in what the probe read, so far2l has to be announced again")
	}
	if got := out.String(); got != far2lAnnounce {
		t.Fatalf("wrote %q, want the far2l announcement %q", got, far2lAnnounce)
	}
}

// A terminal that never acknowledged must not be written to: the announcement
// would be a stray escape sequence to something that has no idea what it is.
func TestRestoreFar2lAfterProbe_SilentWithoutAnAck(t *testing.T) {
	t.Cleanup(func() { takeProbeInput() })

	noteProbeInput("\x1b[4;856;1319t")

	var out bytes.Buffer
	if restoreFar2lAfterProbe(&out) {
		t.Error("nothing acknowledged far2l, so nothing should be announced")
	}
	if out.Len() != 0 {
		t.Errorf("wrote %q to a terminal that never acknowledged far2l", out.String())
	}
}

// What one probe read must not be judged again by the next one, or a single
// acknowledgement would keep re-announcing far2l for the life of the process.
func TestRestoreFar2lAfterProbe_ConsumesWhatItRead(t *testing.T) {
	t.Cleanup(func() { takeProbeInput() })

	noteProbeInput("\x1b_far2lok\x07")

	var first, second bytes.Buffer
	if !restoreFar2lAfterProbe(&first) {
		t.Fatal("the first pass should have found the acknowledgement")
	}
	if restoreFar2lAfterProbe(&second) {
		t.Error("the acknowledgement was answered once already")
	}
	if strings.Contains(second.String(), "far2l") {
		t.Errorf("announced far2l a second time: %q", second.String())
	}
}
