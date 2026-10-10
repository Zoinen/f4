package vtui

import "testing"

func TestClampToByte(t *testing.T) {
	cases := []struct {
		in   float32
		want byte
	}{
		{-10, 0},
		{0, 0},
		{254.6, 255},
		{255, 255},
		{300, 255},
		{100.4, 100},
		{100.5, 101},
	}
	for _, c := range cases {
		if got := clampToByte(c.in); got != c.want {
			t.Errorf("clampToByte(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func sumWeights(taps []resampleTap) float64 {
	var total float64
	for _, tp := range taps {
		total += float64(tp.weight)
	}
	return total
}

func TestBuildResampleTaps_Identity(t *testing.T) {
	// Equal source and destination length must use the "grow" branch
	// (dstLen < srcLen is false) and resolve to an exact identity mapping.
	taps := buildResampleTaps(5, 5)
	if len(taps) != 5 {
		t.Fatalf("expected 5 tap lists, got %d", len(taps))
	}
	for i, list := range taps {
		if len(list) != 1 || list[0].idx != i || list[0].weight != 1 {
			t.Errorf("index %d: expected single identity tap, got %+v", i, list)
		}
	}
}

func TestBuildResampleTaps_Grow(t *testing.T) {
	taps := buildResampleTaps(2, 4)
	if len(taps) != 4 {
		t.Fatalf("expected 4 tap lists, got %d", len(taps))
	}
	for i, list := range taps {
		if len(list) == 0 || len(list) > 2 {
			t.Fatalf("index %d: unexpected tap count %d", i, len(list))
		}
		for _, tp := range list {
			if tp.idx < 0 || tp.idx > 1 {
				t.Errorf("index %d: source idx %d out of range", i, tp.idx)
			}
		}
		if w := sumWeights(list); w < 0.999 || w > 1.001 {
			t.Errorf("index %d: weights sum to %v, want ~1", i, w)
		}
	}
}

func TestBuildResampleTaps_Shrink(t *testing.T) {
	taps := buildResampleTaps(4, 2)
	if len(taps) != 2 {
		t.Fatalf("expected 2 tap lists, got %d", len(taps))
	}
	want := [][]resampleTap{
		{{idx: 0, weight: 0.5}, {idx: 1, weight: 0.5}},
		{{idx: 2, weight: 0.5}, {idx: 3, weight: 0.5}},
	}
	for i, list := range taps {
		if len(list) != len(want[i]) {
			t.Fatalf("index %d: expected %d taps, got %d (%+v)", i, len(want[i]), len(list), list)
		}
		for j, tp := range list {
			if tp.idx != want[i][j].idx || tp.weight != want[i][j].weight {
				t.Errorf("index %d tap %d: got %+v, want %+v", i, j, tp, want[i][j])
			}
		}
	}
}

func makeSolidSurface(w, h int, r, g, b, a byte, opaque bool) *ImageSurface {
	s := NewImageSurface(w, h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := y*s.Stride + x*4
			s.Pix[o], s.Pix[o+1], s.Pix[o+2], s.Pix[o+3] = r, g, b, a
		}
	}
	s.Opaque = opaque
	return s
}

func TestScaleSurface_InvalidAndDegenerateInputs(t *testing.T) {
	valid := makeSolidSurface(2, 2, 10, 20, 30, 255, true)

	if got := ScaleSurface(nil, 2, 2); got != nil {
		t.Errorf("nil src: expected nil, got %+v", got)
	}
	if got := ScaleSurface(&ImageSurface{Width: 2, Height: 2}, 2, 2); got != nil {
		t.Errorf("invalid src (no Pix): expected nil, got %+v", got)
	}
	if got := ScaleSurface(valid, 0, 2); got != nil {
		t.Errorf("w=0: expected nil, got %+v", got)
	}
	if got := ScaleSurface(valid, 2, -1); got != nil {
		t.Errorf("h<0: expected nil, got %+v", got)
	}
}

func TestScaleSurface_SameSizeReturnsSrcUnchanged(t *testing.T) {
	src := makeSolidSurface(3, 3, 1, 2, 3, 255, true)
	out := ScaleSurface(src, 3, 3)
	if out != src {
		t.Fatalf("expected the exact same surface pointer back for a no-op scale")
	}
}

func TestScaleSurface_OpaqueShrinkUsesExactBoxAverage(t *testing.T) {
	// Four uniform 2x2 blocks arranged in a 4x4 opaque surface: shrinking to
	// 2x2 should average each block back to its own exact color, which is
	// only possible through the integer box filter path.
	src := NewImageSurface(4, 4)
	blocks := [2][2][4]byte{
		{{255, 0, 0, 255}, {0, 255, 0, 255}},
		{{0, 0, 255, 255}, {128, 128, 128, 255}},
	}
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			c := blocks[y/2][x/2]
			o := y*src.Stride + x*4
			src.Pix[o], src.Pix[o+1], src.Pix[o+2], src.Pix[o+3] = c[0], c[1], c[2], c[3]
		}
	}
	src.Opaque = true

	out := ScaleSurface(src, 2, 2)
	if out == nil {
		t.Fatal("expected a scaled surface, got nil")
	}
	if !out.Opaque {
		t.Error("expected output to remain flagged Opaque")
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			want := blocks[y][x]
			o := y*out.Stride + x*4
			got := [4]byte{out.Pix[o], out.Pix[o+1], out.Pix[o+2], out.Pix[o+3]}
			if got != want {
				t.Errorf("pixel (%d,%d): got %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestScaleSurface_GrowUsesFloatPathAndPreservesColor(t *testing.T) {
	// A 1x1 opaque source upscaled to 2x2 does not satisfy the box filter's
	// "shrink" precondition, so it must fall through to the float resampler,
	// which should simply replicate the single source pixel exactly.
	src := makeSolidSurface(1, 1, 10, 20, 30, 255, true)
	out := ScaleSurface(src, 2, 2)
	if out == nil {
		t.Fatal("expected a scaled surface, got nil")
	}
	for y := 0; y < 2; y++ {
		for x := 0; x < 2; x++ {
			o := y*out.Stride + x*4
			got := [4]byte{out.Pix[o], out.Pix[o+1], out.Pix[o+2], out.Pix[o+3]}
			want := [4]byte{10, 20, 30, 255}
			if got != want {
				t.Errorf("pixel (%d,%d): got %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestScaleSurfaceFloat_BlendsPremultipliedAlpha(t *testing.T) {
	// pixel0: opaque red. pixel1: fully transparent blue, which must
	// contribute nothing to the shrunk color once alpha is un-premultiplied.
	src := NewImageSurface(2, 1)
	src.Pix[0], src.Pix[1], src.Pix[2], src.Pix[3] = 255, 0, 0, 255
	src.Pix[4], src.Pix[5], src.Pix[6], src.Pix[7] = 0, 0, 255, 0
	src.Opaque = false // forces the float path even though it's a shrink

	out := ScaleSurface(src, 1, 1)
	if out == nil {
		t.Fatal("expected a scaled surface, got nil")
	}
	want := [4]byte{255, 0, 0, 128}
	got := [4]byte{out.Pix[0], out.Pix[1], out.Pix[2], out.Pix[3]}
	if got != want {
		t.Errorf("blended pixel: got %v, want %v", got, want)
	}
}

func TestFitInside(t *testing.T) {
	cases := []struct {
		name                   string
		srcW, srcH, boxW, boxH int
		wantW, wantH           int
	}{
		{"invalid srcW", 0, 10, 10, 10, 0, 0},
		{"invalid srcH", 10, 0, 10, 10, 0, 0},
		{"invalid boxW", 10, 10, 0, 10, 0, 0},
		{"invalid boxH", 10, 10, 10, 0, 0, 0},
		{"width constrained", 100, 50, 200, 200, 200, 100},
		{"height constrained", 100, 200, 200, 100, 50, 100},
		{"clamp height to 1", 1000, 1, 10, 10, 10, 1},
		{"clamp width to 1", 1, 1000, 10, 10, 1, 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			w, h := FitInside(c.srcW, c.srcH, c.boxW, c.boxH)
			if w != c.wantW || h != c.wantH {
				t.Errorf("FitInside(%d,%d,%d,%d) = (%d,%d), want (%d,%d)",
					c.srcW, c.srcH, c.boxW, c.boxH, w, h, c.wantW, c.wantH)
			}
		})
	}
}
