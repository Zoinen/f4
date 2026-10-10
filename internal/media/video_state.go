package media

// What mpv reports about the film it is playing (docs/VIDEO.md, V5): the
// position, the length, whether it is paused, the volume. mpv answers on the
// IPC socket, but not to the one-shot dial Command uses, so the player keeps
// one connection open, asks to observe the properties and reads the changes
// as they come.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/unxed/vtinput"
)

// PlaybackState is a copy of what mpv last said. A zero Duration means the
// length is not known (yet).
type PlaybackState struct {
	Position time.Duration
	Duration time.Duration
	Paused   bool
	Volume   int
	// Known is set once mpv has reported anything at all.
	Known bool
}

// observedProperties are asked for by number, so a reply can be told apart.
var observedProperties = []string{"time-pos", "duration", "pause", "volume"}

type playbackTracker struct {
	mu    sync.Mutex
	state PlaybackState
}

func (t *playbackTracker) snapshot() PlaybackState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state
}

// apply takes one line of mpv's output: a property-change event updates the
// state, anything else (replies, other events) is ignored.
func (t *playbackTracker) apply(line []byte) {
	var msg struct {
		Event string          `json:"event"`
		Name  string          `json:"name"`
		Data  json.RawMessage `json:"data"`
	}
	if json.Unmarshal(line, &msg) != nil || msg.Event != "property-change" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state.Known = true
	switch msg.Name {
	case "time-pos":
		t.state.Position = mpvSeconds(msg.Data)
	case "duration":
		t.state.Duration = mpvSeconds(msg.Data)
	case "pause":
		var paused bool
		if json.Unmarshal(msg.Data, &paused) == nil {
			t.state.Paused = paused
		}
	case "volume":
		var volume float64
		if json.Unmarshal(msg.Data, &volume) == nil {
			t.state.Volume = int(volume + 0.5)
		}
	}
}

// mpvSeconds reads a number of seconds; null (nothing playing yet) is zero.
func mpvSeconds(raw json.RawMessage) time.Duration {
	var s float64
	if json.Unmarshal(raw, &s) != nil || s < 0 {
		return 0
	}
	return time.Duration(s * float64(time.Second))
}

// observe reads mpv's answers off conn until it closes: it asks for the
// properties, then feeds every line to the tracker.
func (t *playbackTracker) observe(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	for i, name := range observedProperties {
		payload, err := json.Marshal(map[string]any{"command": []any{"observe_property", i + 1, name}})
		if err != nil {
			return
		}
		_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
		if _, err := conn.Write(append(payload, '\n')); err != nil {
			return
		}
	}
	_ = conn.SetWriteDeadline(time.Time{})
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		t.apply(sc.Bytes())
	}
}

// startObserving connects to the socket, which mpv creates a moment after it
// starts, and keeps the tracker fed until the player goes.
func (p *videoPlayer) startObserving() {
	p.mu.Lock()
	sock := p.sock
	p.mu.Unlock()
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for {
			conn, err := net.DialTimeout("unix", sock, 200*time.Millisecond)
			if err == nil {
				p.tracker.observe(conn)
				return
			}
			select {
			case <-p.done:
				return
			default:
			}
			if time.Now().After(deadline) {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
}

// State is what mpv last reported; the zero value when it has not, or for a
// nil player.
func (p *videoPlayer) State() PlaybackState {
	if p == nil {
		return PlaybackState{}
	}
	return p.tracker.snapshot()
}

// formatClock writes a time as H:MM:SS, or M:SS under an hour.
func formatClock(d time.Duration) string {
	total := int(d / time.Second)
	if total < 0 {
		total = 0
	}
	h, m, s := total/3600, total/60%60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// statusText is the line the viewer shows on the right of its title bar:
// "1:05 / 42:10 │ vol 80", with the pause marked.
func (s PlaybackState) statusText() string {
	if !s.Known {
		return ""
	}
	var parts []string
	clock := formatClock(s.Position)
	if s.Duration > 0 {
		clock += " / " + formatClock(s.Duration)
	}
	parts = append(parts, clock)
	if s.Paused {
		parts = append(parts, "paused")
	}
	parts = append(parts, fmt.Sprintf("vol %d", s.Volume))
	return " " + strings.Join(parts, " │ ") + " "
}

// VideoSiblings is the other films next to the one playing, for the keys that
// walk through them (docs/VIDEO.md V6); the same on both video frames.
type VideoSiblings struct {
	Paths []string
	Index int
	// OnStep is called with the film to switch to; the host closes the frame
	// and opens that one.
	OnStep func(path string)
}

// step handles PgDn/PgUp/End/Home: the next, previous, last and first film. It
// reports whether the key was one of these and there was somewhere to go.
func (s *VideoSiblings) step(vk uint16) bool {
	if s == nil || s.OnStep == nil || len(s.Paths) < 2 || s.Index < 0 || s.Index >= len(s.Paths) {
		return false
	}
	var target int
	switch vk {
	case vtinput.VK_NEXT:
		target = (s.Index + 1) % len(s.Paths)
	case vtinput.VK_PRIOR:
		target = (s.Index - 1 + len(s.Paths)) % len(s.Paths)
	case vtinput.VK_HOME:
		target = 0
	case vtinput.VK_END:
		target = len(s.Paths) - 1
	default:
		return false
	}
	if target == s.Index {
		return true
	}
	s.OnStep(s.Paths[target])
	return true
}
