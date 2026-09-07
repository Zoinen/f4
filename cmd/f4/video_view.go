package main

// The frame the video plays inside. It draws almost nothing — the picture is
// mpv's, in a window over the terminal — so what is left is the title, the key
// bar, and turning keys into commands down the socket.
//
// It exists rather than f4 simply launching a player because the window has to
// belong to f4: it is placed where the frame is, it follows the terminal, it
// comes down when the terminal loses the focus, and it goes when the frame is
// closed. A player launched on its own does none of that.

import (
	"path/filepath"

	"github.com/unxed/f4/internal/ttyx"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type VideoView struct {
	vtui.BaseFrame
	topBar *TopBar

	vfs    vfs.VFS
	path   string
	player *videoPlayer

	// paused is what f4 last asked for, so that the focus rule does not
	// unpause a video the reader paused on purpose.
	paused bool

	OnClose func()
}

// NewVideoView starts the player and hands back the frame it lives in.
func NewVideoView(v vfs.VFS, path string) (*VideoView, error) {
	vv := &VideoView{vfs: v, path: path}
	vv.topBar = NewTopBar(
		func() string {
			base := filepath.Base(vv.path)
			if vv.vfs != nil {
				base = vv.vfs.Base(vv.path)
			}
			return " " + base
		},
		func() string { return "" },
	)
	return vv, nil
}

// start is called once the frame knows where it is, because the player window
// is placed where the frame is and the frame does not know that until it has
// been laid out.
func (vv *VideoView) start(scr *vtui.ScreenBuf) bool {
	if vv.player != nil {
		return true
	}
	rect, ok := vv.pictureRect(scr)
	if !ok {
		return false
	}
	p, err := startVideoPlayer(vv.path, rect)
	if err != nil {
		vtui.DebugLog("VIDEO: %v", err)
		vtui.ShowMessage(" Video ", err.Error(), []string{"&Ok"})
		vv.Close()
		return false
	}
	vv.player = p
	// Read on the goroutine that starts this work, not inside it: the
	// work outlives the call, and reading the global from it races
	// anything that reassigns vtui.FrameManager meanwhile.
	frames := vtui.FrameManager
	go func() {
		<-p.Done()
		frames.Redraw()
	}()
	return true
}

// pictureRect is where the picture goes on the screen: the frame's own area,
// in pixels, worked out exactly the way the image overlay works it out.
func (vv *VideoView) pictureRect(scr *vtui.ScreenBuf) (ttyx.Rect, bool) {
	sess := sharedTTYXSession()
	if sess == nil || scr == nil {
		return ttyx.Rect{}, false
	}
	win, err := sess.Geometry()
	if err != nil {
		return ttyx.Rect{}, false
	}
	cols, rows := scr.Width(), scr.Height()
	tw, th, known := hostTextSize(cols, rows)
	grid := nudgeGrid(hostGridRect(win, tw, th, known))

	x1, y1, x2, y2 := vv.GetPosition()
	top := y1
	if vv.topBar != nil {
		top = y1 + 1
	}
	return overlayCellRect(grid, cols, rows, x1, top, x2, y2)
}

func (vv *VideoView) SetPosition(x1, y1, x2, y2 int) {
	vv.ScreenObject.SetPosition(x1, y1, x2, y2)
	if vv.topBar != nil {
		vv.topBar.SetPosition(x1, y1, x2, y1)
	}
}

func (vv *VideoView) ResizeConsole(w, h int) {
	vv.SetPosition(0, 0, w-1, h-2)
}

func (vv *VideoView) Show(scr *vtui.ScreenBuf) {
	vv.ScreenObject.Show(scr)
	if vv.topBar != nil {
		vv.topBar.Show(scr)
	}
	x1, y1, x2, y2 := vv.GetPosition()
	top := y1
	if vv.topBar != nil {
		top = y1 + 1
	}
	scr.FillRect(x1, top, x2, y2, ' ', imageViewBackAttr())

	if !vv.start(scr) {
		return
	}
	// The window moves with the frame, so that a resize of the terminal
	// does not leave the picture where the frame used to be.
	if rect, ok := vv.pictureRect(scr); ok {
		vv.player.Place(rect)
	}
	// The focus rule: playback carries on while the terminal is not on
	// top, unless the reader asked otherwise.
	if AppConfig.VideoPauseOnFocusLoss && !vv.paused {
		if sess := sharedTTYXSession(); sess != nil {
			vv.player.SetPaused(!sess.Focused())
		}
	}
}

func (vv *VideoView) ProcessKey(e *vtinput.InputEvent) bool {
	if e == nil || !e.KeyDown || vv.player == nil {
		return false
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_ESCAPE, vtinput.VK_F10, vtinput.VK_F3:
		vv.Close()
		return true
	case vtinput.VK_SPACE:
		vv.paused = !vv.paused
		vv.player.TogglePause()
		return true
	case vtinput.VK_RIGHT:
		vv.player.Seek(10)
		return true
	case vtinput.VK_LEFT:
		vv.player.Seek(-10)
		return true
	case vtinput.VK_UP:
		vv.player.Volume(5)
		return true
	case vtinput.VK_DOWN:
		vv.player.Volume(-5)
		return true
	}
	return false
}

func (vv *VideoView) Close() {
	vv.player.Close()
	vv.player = nil
	vv.BaseFrame.Close()
	if vv.OnClose != nil {
		vv.OnClose()
	}
}

func (vv *VideoView) GetKeyLabels() *vtui.KeySet {
	return &vtui.KeySet{
		Normal: vtui.KeyBarLabels{
			"", "", "", "", "", "", "", "", "", "Quit",
		},
	}
}

func (vv *VideoView) GetType() vtui.FrameType { return vtui.TypeUser + 8 }
