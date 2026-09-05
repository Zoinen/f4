package main

// Playing video, the short way round.
//
// The long way — decode with ffmpeg into a stream of RGBA and push the frames
// into the overlay ourselves — is the one that ends with f4 owning the timing,
// the seeking and the audio. It is also the one that has to exist before
// anybody can watch anything. So this is mpv, given the overlay window to draw
// into, which is a day's work rather than a week's and puts a working player
// in front of people while the other one is written.
//
// mpv draws into f4's own overlay window through --wid, which is what makes
// this more than "f4 launches a player". The window is the one the X session
// already keeps over the terminal: it follows the terminal when it moves, it
// comes down when the terminal loses the focus, and it goes when f4 does.
// Nothing is left behind and nothing floats over the wrong application.
//
// Playback carries on while the terminal is not on top, the way a player
// behaves; [Video] PauseOnFocusLoss is for whoever disagrees.

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/internal/ttyx"
	"github.com/unxed/vtui"
)

// videoExtensions is what F3 offers to play. Deliberately a list and not a
// sniff: the extension is what says the file is meant to be a video, and
// opening every unknown file in a player would be a surprise.
var videoExtensions = map[string]bool{
	".mp4": true, ".mkv": true, ".webm": true, ".avi": true, ".mov": true,
	".m4v": true, ".mpg": true, ".mpeg": true, ".wmv": true, ".flv": true,
	".ts": true, ".m2ts": true, ".ogv": true, ".3gp": true, ".vob": true,
	".mts": true, ".divx": true, ".asf": true, ".rm": true, ".rmvb": true,
}

// IsVideoFile reports whether the name is one f4 would offer to play.
func IsVideoFile(path string) bool {
	return videoExtensions[strings.ToLower(filepath.Ext(path))]
}

// videoPlayer owns one mpv process and the window it draws into.
type videoPlayer struct {
	mu   sync.Mutex
	cmd  *exec.Cmd
	ov   *ttyx.Overlay
	sock string
	done chan struct{}
}

// videoArgs builds mpv's command line. It is a plain function so that the
// shape of it can be checked without mpv, an X server or a video.
func videoArgs(path, socket string, wid uint32) []string {
	return []string{
		// Draw into f4's window rather than one of mpv's own.
		"--wid=" + strconv.FormatUint(uint64(wid), 10),
		// No window furniture of any kind: the window belongs to f4.
		"--no-border",
		"--no-osc",
		"--no-input-default-bindings",
		// The keyboard belongs to the terminal, so mpv is driven through
		// the socket instead of through its own bindings.
		"--input-ipc-server=" + socket,
		// Stop at the end rather than closing the window f4 owns.
		"--keep-open=yes",
		"--idle=no",
		// The picture is scaled to the window, which is the rectangle of
		// the viewer, and the rest of the window is left alone.
		"--keepaspect=yes",
		"--",
		path,
	}
}

// startVideoPlayer launches mpv over the terminal. It returns nil and a reason
// when it cannot, which the caller shows to the user rather than swallowing.
func startVideoPlayer(path string, rect ttyx.Rect) (*videoPlayer, error) {
	bin, ok := toolMPV.Find()
	if !ok {
		return nil, fmt.Errorf("%s", toolMPV.MissingMessage())
	}
	sess := sharedTTYXSession()
	if sess == nil {
		return nil, fmt.Errorf("video needs a local X session, and there is none here")
	}
	ov, err := sess.NewOverlay()
	if err != nil {
		return nil, fmt.Errorf("no window to play into: %w", err)
	}
	if err := ov.Place(rect); err != nil {
		ov.Close()
		return nil, fmt.Errorf("the player window could not be placed: %w", err)
	}

	sock := filepath.Join(os.TempDir(), fmt.Sprintf("f4-mpv-%d-%d.sock", os.Getpid(), time.Now().UnixNano()))
	cmd := exec.Command(bin, videoArgs(path, sock, uint32(ov.Window()))...)
	cmd.Env = toolEnv()
	cmd.Stdout, cmd.Stderr = nil, nil
	if err := cmd.Start(); err != nil {
		ov.Close()
		return nil, fmt.Errorf("%s would not start: %w", filepath.Base(bin), err)
	}

	p := &videoPlayer{cmd: cmd, ov: ov, sock: sock, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(p.done)
	}()
	vtui.DebugLog("VIDEO: %s playing %s into window %d at %+v", bin, path, ov.Window(), rect)
	return p, nil
}

// Done is closed when the player exits on its own.
func (p *videoPlayer) Done() <-chan struct{} {
	if p == nil {
		return nil
	}
	return p.done
}

// Place moves the picture, for a terminal that was resized or dragged.
func (p *videoPlayer) Place(rect ttyx.Rect) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	_ = p.ov.Place(rect)
}

// Command sends one mpv command down the socket. Everything the viewer offers
// goes through here, because the player window has no keyboard of its own: it
// is override-redirect and shaped out of the input region, which is what lets
// the terminal underneath keep working.
func (p *videoPlayer) Command(args ...any) {
	if p == nil {
		return
	}
	p.mu.Lock()
	sock := p.sock
	p.mu.Unlock()

	payload, err := json.Marshal(map[string]any{"command": args})
	if err != nil {
		return
	}
	conn, err := net.DialTimeout("unix", sock, 200*time.Millisecond)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetWriteDeadline(time.Now().Add(200 * time.Millisecond))
	_, _ = conn.Write(append(payload, '\n'))
}

func (p *videoPlayer) TogglePause()     { p.Command("cycle", "pause") }
func (p *videoPlayer) Seek(seconds int) { p.Command("seek", seconds, "relative") }
func (p *videoPlayer) Volume(delta int) { p.Command("add", "volume", delta) }

// SetPaused is what the focus rule uses. Playback carries on by default while
// the terminal is not on top, the way a player behaves.
func (p *videoPlayer) SetPaused(paused bool) {
	v := "no"
	if paused {
		v = "yes"
	}
	p.Command("set_property_string", "pause", v)
}

// Close stops the player and takes its window away.
func (p *videoPlayer) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	cmd, ov, sock := p.cmd, p.ov, p.sock
	p.cmd = nil
	p.mu.Unlock()
	if cmd == nil {
		return
	}

	// Ask first: mpv writes its watch-later state on a clean quit.
	p.Command("quit")
	select {
	case <-p.done:
	case <-time.After(time.Second):
		_ = cmd.Process.Kill()
		<-p.done
	}
	ov.Close()
	_ = os.Remove(sock)
}
