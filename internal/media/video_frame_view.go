package media

// The video viewer for a screen that has no window to play in (docs/VIDEO.md,
// V3): the frame source's pictures, drawn by the terminal's own graphics where
// it has any and in coloured half blocks where it has none. It is silent - the
// sound is mpv's, on the X rung - and the picture is sized to what the screen
// can carry: a frame no bigger than 640x360 at ten a second through a graphics
// protocol, one pixel per half a cell at six a second in text.

import (
	"context"
	"image"
	"path/filepath"
	"sync"
	"time"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/viewer"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

const (
	frameViewGraphicsFPS = 10
	frameViewTextFPS     = 6
	frameViewMaxWidth    = 640
	frameViewMaxHeight   = 360
)

// FrameVideoView plays a video by drawing frames.
type FrameVideoView struct {
	vtui.BaseFrame
	topBar *viewer.TopBar

	vfs  vfs.VFS
	Path string

	mu      sync.Mutex
	frame   *VideoFrame // the latest picture
	paused  bool
	ended   bool
	failure error
	cancel  context.CancelFunc
	gen     int            // which run the frame belongs to; a restart bumps it
	runs    sync.WaitGroup // the run goroutines, for tests to wait out

	// What the current run was started for, to notice a resized screen.
	runCols, runRows int
	runGraphics      bool

	gfxKey  string
	OnClose func()

	// Siblings, when set, lets PgUp/PgDn/Home/End walk through the films of
	// the panel.
	Siblings *VideoSiblings
}

// NewFrameVideoView makes the frame; playback starts when it is first drawn,
// because the size of the pictures depends on the area it is drawn in.
func NewFrameVideoView(v vfs.VFS, Path string) *FrameVideoView {
	fv := &FrameVideoView{vfs: v, Path: Path}
	fv.gfxKey = "f4.framevideo:" + Path
	fv.topBar = viewer.NewTopBar(
		func() string {
			base := filepath.Base(fv.Path)
			if fv.vfs != nil {
				base = fv.vfs.Base(fv.Path)
			}
			return " " + base
		},
		fv.statusText,
	)
	return fv
}

func (fv *FrameVideoView) statusText() string {
	fv.mu.Lock()
	defer fv.mu.Unlock()
	text := " " + formatClock(fv.position())
	switch {
	case fv.failure != nil:
		text += " │ " + fv.failure.Error()
	case fv.ended:
		text += " │ end"
	case fv.paused:
		text += " │ paused"
	}
	return text + " "
}

// position is where in the film the latest picture is. Callers hold mu.
func (fv *FrameVideoView) position() time.Duration {
	if fv.frame == nil {
		return 0
	}
	return fv.frame.PTS
}

// specFor works out what to ask the frame source for on this screen.
func frameViewSpec(scr *vtui.ScreenBuf, cols, rows int, start time.Duration) (VideoFrameSpec, bool) {
	if scr.SupportsGraphics() {
		cw, ch := scr.Graphics().CellSize()
		if cw <= 0 || ch <= 0 {
			cw, ch = ImageViewFallbackCellW, ImageViewFallbackCellH
		}
		w, h := cols*cw, rows*ch
		if w > frameViewMaxWidth || h > frameViewMaxHeight {
			f := min(float64(frameViewMaxWidth)/float64(w), float64(frameViewMaxHeight)/float64(h))
			w, h = int(float64(w)*f), int(float64(h)*f)
		}
		return VideoFrameSpec{Width: max(w&^1, 2), Height: max(h&^1, 2), FPS: frameViewGraphicsFPS, Start: start}, true
	}
	return VideoFrameSpec{Width: max(cols, 1), Height: max(rows*2, 2), FPS: frameViewTextFPS, Start: start}, false
}

// begin starts (or restarts) the run that feeds the view, from start.
func (fv *FrameVideoView) begin(scr *vtui.ScreenBuf, cols, rows int, start time.Duration) {
	spec, graphics := frameViewSpec(scr, cols, rows, start)
	ctx, cancel := context.WithCancel(context.Background())

	fv.mu.Lock()
	if fv.cancel != nil {
		fv.cancel()
	}
	fv.cancel = cancel
	fv.gen++
	gen := fv.gen
	fv.ended, fv.failure = false, nil
	fv.runCols, fv.runRows, fv.runGraphics = cols, rows, graphics
	fv.mu.Unlock()

	fv.runs.Add(1)
	go func() {
		defer fv.runs.Done()
		fv.run(ctx, gen, spec)
	}()
}

// run reads the frames of one start and shows each at its time.
func (fv *FrameVideoView) run(ctx context.Context, gen int, spec VideoFrameSpec) {
	src, err := OpenVideoFrames(ctx, fv.Path, spec)
	if err != nil {
		fv.finish(gen, err)
		return
	}
	defer src.Close()

	var played time.Duration
	last := time.Now()
	for f := range src.Frames {
		target := f.PTS - spec.Start
		for {
			now := time.Now()
			if !fv.isPaused() {
				played += now.Sub(last)
			}
			last = now
			if played >= target {
				break
			}
			wait := min(target-played, 40*time.Millisecond)
			if fv.isPaused() {
				wait = 40 * time.Millisecond
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
		}
		frame := f
		fv.mu.Lock()
		if fv.gen != gen {
			fv.mu.Unlock()
			return
		}
		fv.frame = &frame
		fv.mu.Unlock()
		fv.redraw()
	}
	fv.finish(gen, src.Err())
}

func (fv *FrameVideoView) finish(gen int, err error) {
	fv.mu.Lock()
	if fv.gen == gen {
		fv.ended, fv.failure = true, err
	}
	fv.mu.Unlock()
	fv.redraw()
}

func (fv *FrameVideoView) isPaused() bool {
	fv.mu.Lock()
	defer fv.mu.Unlock()
	return fv.paused
}

func (fv *FrameVideoView) redraw() {
	if fm := vtui.FrameManager; fm != nil {
		fm.Redraw()
	}
}

// seekTo restarts playback at the given time.
func (fv *FrameVideoView) seekTo(scr *vtui.ScreenBuf, at time.Duration) {
	fv.mu.Lock()
	cols, rows := fv.runCols, fv.runRows
	fv.mu.Unlock()
	if scr == nil || cols <= 0 || rows <= 0 {
		return
	}
	fv.begin(scr, cols, rows, max(at, 0))
}

func (fv *FrameVideoView) SetPosition(x1, y1, x2, y2 int) {
	fv.ScreenObject.SetPosition(x1, y1, x2, y2)
	if fv.topBar != nil {
		fv.topBar.SetPosition(x1, y1, x2, y1)
	}
}

func (fv *FrameVideoView) ResizeConsole(w, h int) { fv.SetPosition(0, 0, w-1, h-2) }

func (fv *FrameVideoView) Show(scr *vtui.ScreenBuf) {
	fv.ScreenObject.Show(scr)
	if fv.topBar != nil {
		fv.topBar.Show(scr)
	}
	x1, y1, x2, y2 := fv.GetPosition()
	top := y1 + 1
	scr.FillRect(x1, top, x2, y2, ' ', imageViewBackAttr())
	cols, rows := x2-x1+1, y2-top+1
	if cols <= 0 || rows <= 0 {
		return
	}

	fv.mu.Lock()
	started := fv.cancel != nil
	restart := started && (fv.runCols != cols || fv.runRows != rows || fv.runGraphics != scr.SupportsGraphics())
	at := fv.position()
	frame := fv.frame
	fv.mu.Unlock()
	if !started || restart {
		fv.begin(scr, cols, rows, at)
	}
	if frame == nil {
		return
	}
	fv.draw(scr, frame, x1, top, cols, rows)
}

// draw puts one picture in the area: through the graphics protocol when the
// screen has one, in half blocks when it has not.
func (fv *FrameVideoView) draw(scr *vtui.ScreenBuf, frame *VideoFrame, x1, top, cols, rows int) {
	if !scr.SupportsGraphics() {
		img := &image.RGBA{Pix: frame.Pix, Stride: frame.Width * 4, Rect: image.Rect(0, 0, frame.Width, frame.Height)}
		cells := HalfBlockArt(img, cols, rows)
		for row := 0; row < rows; row++ {
			scr.Write(x1, top+row, cells[row*cols:(row+1)*cols])
		}
		return
	}
	surf := vtui.NewImageSurfaceFromPix(frame.Width, frame.Height, frame.Width*4, frame.Pix)
	if surf == nil {
		return
	}
	surf.Opaque = true
	cw, ch := scr.Graphics().CellSize()
	if cw <= 0 || ch <= 0 {
		cw, ch = ImageViewFallbackCellW, ImageViewFallbackCellH
	}
	// Fit the picture into the area, keeping its proportions.
	scale := min(float64(cols*cw)/float64(frame.Width), float64(rows*ch)/float64(frame.Height))
	p := vtui.ImagePlacement{Surface: surf}
	p.Cols = CellsFor(int(float64(frame.Width)*scale+0.5), cw, cols)
	p.Rows = CellsFor(int(float64(frame.Height)*scale+0.5), ch, rows)
	p.Col = x1 + (cols-p.Cols)/2
	p.Row = top + (rows-p.Rows)/2
	scr.Graphics().DrawImage(fv.gfxKey, p)
}

func (fv *FrameVideoView) ProcessKey(e *vtinput.InputEvent) bool {
	if e == nil || !e.KeyDown {
		return false
	}
	ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	step := 10 * time.Second
	if e.ControlKeyState&vtinput.ShiftPressed != 0 {
		step = time.Second
	}
	if fv.Siblings.step(e.VirtualKeyCode) {
		return true
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_ESCAPE, vtinput.VK_F10, vtinput.VK_F3:
		fv.Close()
		return true
	case vtinput.VK_SPACE:
		fv.mu.Lock()
		fv.paused = !fv.paused
		fv.mu.Unlock()
		fv.redraw()
		return true
	case vtinput.VK_RIGHT, vtinput.VK_LEFT:
		fv.mu.Lock()
		at := fv.position()
		fv.mu.Unlock()
		switch {
		case ctrl && e.VirtualKeyCode == vtinput.VK_LEFT:
			at = 0
		case ctrl:
			return true // no length known to go to the end of
		case e.VirtualKeyCode == vtinput.VK_RIGHT:
			at += step
		default:
			at -= step
		}
		fv.seekTo(vtui.FrameManager.Screen(), at)
		return true
	}
	return false
}

// Close stops the run and closes the frame.
func (fv *FrameVideoView) Close() {
	fv.mu.Lock()
	if fv.cancel != nil {
		fv.cancel()
	}
	fv.gen++
	fv.mu.Unlock()
	fv.BaseFrame.Close()
	if fv.OnClose != nil {
		fv.OnClose()
	}
}

func (fv *FrameVideoView) GetKeyLabels() *vtui.KeySet {
	return &vtui.KeySet{
		Normal: vtui.KeyBarLabels{
			"", "", "", "", "", "", "", "", "", i18n.Msg("KeyBar.F10"),
		},
	}
}

func (fv *FrameVideoView) GetType() vtui.FrameType { return vtui.TypeUser + 9 }
