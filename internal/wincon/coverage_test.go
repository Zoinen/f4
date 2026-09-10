package wincon

import (
	"testing"
)

func TestSourceStringCoversUnknownAndPseudoSources(t *testing.T) {
	for _, test := range []struct {
		source Source
		want   string
	}{
		{source: SourceNone, want: "nothing"},
		{source: SourcePseudo, want: "a ConPTY pseudo console window (Windows Terminal or another terminal on the far side of a pty)"},
		{source: Source(99), want: "nothing"},
	} {
		if got := test.source.String(); got != test.want {
			t.Errorf("Source(%d).String() = %q, want %q", test.source, got, test.want)
		}
	}
}

func TestClipToClientRejectsEmptyAndDisjointRectangles(t *testing.T) {
	for _, test := range []struct {
		name string
		r    Rect
		w, h int
	}{
		{name: "empty rectangle", r: Rect{}, w: 100, h: 100},
		{name: "empty client", r: Rect{W: 10, H: 10}, w: 0, h: 100},
		{name: "past left edge", r: Rect{X: -20, W: 10, H: 10}, w: 100, h: 100},
		{name: "past top edge", r: Rect{Y: -20, W: 10, H: 10}, w: 100, h: 100},
		{name: "past right edge", r: Rect{X: 100, W: 10, H: 10}, w: 100, h: 100},
		{name: "past bottom edge", r: Rect{Y: 100, W: 10, H: 10}, w: 100, h: 100},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := ClipToClient(test.r, test.w, test.h); ok {
				t.Fatalf("ClipToClient(%+v, %d, %d) accepted a disjoint rectangle", test.r, test.w, test.h)
			}
		})
	}
}

func TestUnionExpandsAroundEverySide(t *testing.T) {
	got, ok := Union([]Rect{
		{X: 10, Y: 10, W: 20, H: 20},
		{X: 2, Y: 4, W: 4, H: 4},
		{X: 40, Y: 50, W: 8, H: 9},
	})
	if !ok || got != (Rect{X: 2, Y: 4, W: 46, H: 55}) {
		t.Fatalf("Union got %+v, ok=%v; want {2 4 46 55}", got, ok)
	}
}

func TestBlitIntoComposesPartialAlpha(t *testing.T) {
	// An opaque blue pixel under a half-transparent red pixel exercises the
	// source-over path and its BGRA channel order.
	dst := []byte{255, 0, 0, 255}
	src := []byte{255, 0, 0, 128}
	blitInto(dst, 1, 1, src, 1, 1, 4, 0, 0)
	if want := []byte{127, 0, 128, 255}; string(dst) != string(want) {
		t.Fatalf("partial alpha composition got %v, want %v", dst, want)
	}
}

func TestTrackStepHidesAnOverlayWhenItIsNoLongerWanted(t *testing.T) {
	ops := trackStep(trackerState{OnScreen: true}, aliveConsole())
	if !ops.Hide || ops.Show || ops.MoveTo || ops.Restack {
		t.Fatalf("got %+v, want only hide", ops)
	}
}

func TestStatsSubAndEmpty(t *testing.T) {
	if !(Stats{}).Empty() {
		t.Fatal("zero stats must be empty")
	}
	current := Stats{Applies: 8, Moves: 7, Regions: 6, Invalidates: 5, Paints: 4, Blank: 3}
	previous := Stats{Applies: 1, Moves: 2, Regions: 3, Invalidates: 4, Paints: 1, Blank: 2}
	want := Stats{Applies: 7, Moves: 5, Regions: 3, Invalidates: 1, Paints: 3, Blank: 1}
	if got := current.Sub(previous); got != want {
		t.Fatalf("Stats.Sub got %+v, want %+v", got, want)
	}
	if current.Sub(previous).Empty() {
		t.Fatal("a non-zero stats delta must not be empty")
	}
}
