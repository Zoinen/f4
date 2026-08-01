package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"github.com/vmihailenco/msgpack/v5"
)

const (
	extUiProtocolVersion = 2
	extUiMaxMessageSize  = 64 * 1024 * 1024
)

func extUiNewNonce() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", buf[:]), nil
}

func extUiSendMessage(w io.Writer, msg map[string]any) error {
	payload, err := msgpack.Marshal(msg)
	if err != nil {
		return err
	}
	if len(payload) > extUiMaxMessageSize {
		return fmt.Errorf("extui message too large: %d bytes", len(payload))
	}

	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(payload)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err = w.Write(payload)
	return err
}

type extUiMessageSender struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *extUiMessageSender) Send(msg map[string]any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return extUiSendMessage(s.w, msg)
}

func extUiReadMessage(r io.Reader) (map[string]any, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n == 0 {
		return nil, fmt.Errorf("empty extui message")
	}
	if n > extUiMaxMessageSize {
		return nil, fmt.Errorf("extui message too large: %d bytes", n)
	}

	payload := make([]byte, n)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}

	var msg map[string]any
	if err := msgpack.Unmarshal(payload, &msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func extUiString(msg map[string]any, key string) string {
	if v, ok := msg[key].(string); ok {
		return v
	}
	return ""
}

func extUiBool(msg map[string]any, key string) bool {
	if v, ok := msg[key].(bool); ok {
		return v
	}
	return false
}

func extUiInt(msg map[string]any, key string) int {
	return extUiAnyInt(msg[key])
}

func extUiAnyInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	}
	return 0
}

type ExtUiRenderer struct {
	mu   sync.Mutex
	conn net.Conn
	send *extUiMessageSender

	cursorX, cursorY int
	cursorVis        bool
	cursorShape      vtui.CursorShape
	cursorDirty      bool

	palette      [256]uint32
	paletteValid bool

	pendingPalette []uint32
	pendingFrame   map[string]any
	pendingScene   map[string]any
	closed         bool
}

func NewExtUiRenderer(conn net.Conn, sender *extUiMessageSender) *ExtUiRenderer {
	return &ExtUiRenderer{conn: conn, send: sender, cursorDirty: true}
}

func (r *ExtUiRenderer) SetPalette(pal *[256]uint32) {
	if pal == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	changed := !r.paletteValid
	if !changed {
		for i, c := range pal {
			if r.palette[i] != c {
				changed = true
				break
			}
		}
	}
	if !changed {
		return
	}

	r.palette = *pal
	r.paletteValid = true
	colors := make([]uint32, len(pal))
	copy(colors, pal[:])
	r.pendingPalette = colors
}

func (r *ExtUiRenderer) SetCursor(x, y int, visible bool, shape vtui.CursorShape) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cursorX == x && r.cursorY == y && r.cursorVis == visible && r.cursorShape == shape {
		return
	}
	r.cursorX, r.cursorY = x, y
	r.cursorVis = visible
	r.cursorShape = shape
	r.cursorDirty = true
}
func (r *ExtUiRenderer) SetWindowTitle(title string) {
	_ = r.send.Send(map[string]any{
		"type":  "title",
		"title": title,
	})
}

func (r *ExtUiRenderer) Render(buf, shadow []vtui.CharInfo, width, height int, forceRedraw bool) {
	if width <= 0 || height <= 0 || len(buf) == 0 {
		return
	}

	needsRedraw := forceRedraw
	if !needsRedraw {
		for i := 0; i < width*height && i < len(buf) && i < len(shadow); i++ {
			if buf[i] != shadow[i] {
				needsRedraw = true
				break
			}
		}
	}
	if !needsRedraw {
		return
	}

	limit := width * height
	if limit > len(buf) {
		limit = len(buf)
	}
	cells := make([][3]uint64, 0, limit)
	if forceRedraw || len(shadow) < limit {
		for i := 0; i < limit; i++ {
			c := buf[i]
			cells = append(cells, [3]uint64{uint64(i), c.Char, c.Attributes})
		}
	} else {
		for i := 0; i < limit; i++ {
			c := buf[i]
			if c != shadow[i] {
				cells = append(cells, [3]uint64{uint64(i), c.Char, c.Attributes})
			}
		}
	}

	r.mu.Lock()
	r.pendingFrame = map[string]any{
		"type":   "frame",
		"width":  width,
		"height": height,
		"full":   forceRedraw || len(shadow) < limit,
		"cells":  cells,
	}
	r.mu.Unlock()
}

func (r *ExtUiRenderer) SetSemanticScene(scene map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pendingScene = scene
}

func (r *ExtUiRenderer) Flush() {
	var messages []map[string]any

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	if r.pendingPalette != nil {
		messages = append(messages, map[string]any{
			"type":   "palette",
			"colors": r.pendingPalette,
		})
		r.pendingPalette = nil
	}
	if r.pendingFrame != nil {
		messages = append(messages, r.pendingFrame)
		r.pendingFrame = nil
	}
	if r.pendingScene != nil {
		messages = append(messages, r.pendingScene)
		r.pendingScene = nil
	}
	if r.cursorDirty {
		messages = append(messages, map[string]any{
			"type":    "cursor",
			"x":       r.cursorX,
			"y":       r.cursorY,
			"visible": r.cursorVis,
			"shape":   int(r.cursorShape),
		})
		r.cursorDirty = false
	}
	r.mu.Unlock()

	for _, msg := range messages {
		if err := r.send.Send(msg); err != nil {
			vtui.DebugLog("EXTUI_RENDERER: send failed: %v", err)
			r.mu.Lock()
			r.closed = true
			r.mu.Unlock()
			return
		}
	}
}

type ExtUiHost struct {
	mu     sync.Mutex
	conn   net.Conn
	send   *extUiMessageSender
	reader *vtinput.Reader
	cols   int
	rows   int
}

func RunExternalUI(cols, rows int, execPath string, args []string) error {
	nonce, err := extUiNewNonce()
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to create extui listener: %w", err)
	}
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmdArgs := append([]string{}, args...)
	cmdArgs = append(cmdArgs,
		"--f4-ext-connect="+listener.Addr().String(),
		"--f4-ext-nonce="+nonce,
		"--f4-ext-cols="+strconv.Itoa(cols),
		"--f4-ext-rows="+strconv.Itoa(rows),
	)

	cmd := exec.CommandContext(ctx, execPath, cmdArgs...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start external UI %q: %w", execPath, err)
	}
	defer func() {
		cancel()
		_ = cmd.Wait()
	}()

	connCh := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			errCh <- err
			return
		}
		connCh <- conn
	}()

	var conn net.Conn
	select {
	case conn = <-connCh:
	case err := <-errCh:
		return fmt.Errorf("extui connection failed: %w", err)
	case <-time.After(10 * time.Second):
		return fmt.Errorf("extui did not connect within 10s")
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	hello, err := extUiReadMessage(conn)
	if err != nil {
		return fmt.Errorf("failed to read extui hello: %w", err)
	}
	if extUiString(hello, "type") != "hello" || extUiString(hello, "nonce") != nonce {
		return fmt.Errorf("invalid extui hello")
	}

	clientCols := extUiInt(hello, "cols")
	clientRows := extUiInt(hello, "rows")

	pixelW := extUiInt(hello, "pixelWidth")
	pixelH := extUiInt(hello, "pixelHeight")
	cellW := extUiInt(hello, "cellWidth")
	cellH := extUiInt(hello, "cellHeight")

	if pixelW > 0 && cellW > 0 {
		clientCols = pixelW / cellW
	}
	if pixelH > 0 && cellH > 0 {
		clientRows = pixelH / cellH
	}

	if clientCols > 0 {
		cols = clientCols
	}
	if clientRows > 0 {
		rows = clientRows
	}

	sender := &extUiMessageSender{w: conn}
	if err := sender.Send(map[string]any{
		"type":     "hello",
		"nonce":    nonce,
		"protocol": extUiProtocolVersion,
		"cols":     cols,
		"rows":     rows,
		"app":      vtui.AppName,
	}); err != nil {
		return fmt.Errorf("failed to send extui hello: %w", err)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return err
	}

	host := &ExtUiHost{conn: conn, send: sender, cols: cols, rows: rows}
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(cols, rows)
	scr.Renderer = NewExtUiRenderer(conn, sender)
	vtui.FrameManager.Init(scr)

	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	host.reader = reader

	vtui.GetTerminalSize = func() (int, int, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		return host.cols, host.rows, nil
	}

	go host.readLoop()
	SetupUI()
	vtui.FrameManager.Run(reader)
	_ = sender.Send(map[string]any{"type": "quit"})
	return nil
}

func (h *ExtUiHost) readLoop() {
	for {
		msg, err := extUiReadMessage(h.conn)
		if err != nil {
			vtui.DebugLog("EXTUI_HOST: read loop stopped: %v", err)
			if h.reader != nil {
				h.reader.Close()
			}
			return
		}
		h.handleMessage(msg)
	}
}

func (h *ExtUiHost) handleMessage(msg map[string]any) {
	switch extUiString(msg, "type") {
	case "resize":
		cols, rows := extUiInt(msg, "cols"), extUiInt(msg, "rows")
		if cols <= 0 || rows <= 0 {
			return
		}
		h.mu.Lock()
		changed := h.cols != cols || h.rows != rows
		h.cols, h.rows = cols, rows
		h.mu.Unlock()
		if changed {
			h.sendEvent(&vtinput.InputEvent{Type: vtinput.ResizeEventType, InputSource: "extui"})
		}
	case "key":
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         extUiBool(msg, "down"),
			VirtualKeyCode:  uint16(extUiInt(msg, "vk")),
			Char:            rune(extUiInt(msg, "char")),
			ControlKeyState: vtinput.ControlKeyState(uint32(extUiInt(msg, "mods"))),
			InputSource:     "extui",
		})
	case "text":
		for _, r := range extUiString(msg, "text") {
			h.sendEvent(&vtinput.InputEvent{
				Type:            vtinput.KeyEventType,
				KeyDown:         true,
				Char:            r,
				ControlKeyState: vtinput.ControlKeyState(uint32(extUiInt(msg, "mods"))),
				InputSource:     "extui_text",
			})
		}
	case "mouse":
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          int16(extUiInt(msg, "x")),
			MouseY:          int16(extUiInt(msg, "y")),
			ButtonState:     uint32(extUiInt(msg, "button")),
			MouseEventFlags: uint32(extUiInt(msg, "flags")),
			KeyDown:         extUiBool(msg, "down"),
			ControlKeyState: vtinput.ControlKeyState(uint32(extUiInt(msg, "mods"))),
			InputSource:     "extui",
		})
	case "wheel":
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          int16(extUiInt(msg, "x")),
			MouseY:          int16(extUiInt(msg, "y")),
			WheelDirection:  extUiInt(msg, "dir"),
			ControlKeyState: vtinput.ControlKeyState(uint32(extUiInt(msg, "mods"))),
			InputSource:     "extui",
		})
	case "paste":
		text := extUiString(msg, "text")
		h.sendEvent(&vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: true, InputSource: "extui"})
		for _, r := range text {
			h.sendEvent(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r, InputSource: "extui_paste"})
		}
		h.sendEvent(&vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: false, InputSource: "extui"})
	case "clipboard_get":
		_ = h.send.Send(map[string]any{
			"type": "clipboard_data",
			"text": vtui.GetClipboard(),
		})
	case "clipboard_set":
		vtui.SetClipboard(extUiString(msg, "text"))
	case "ui_action":
		action := msg
		if nested, ok := msg["action"].(map[string]any); ok {
			action = nested
		}
		if vtui.FrameManager != nil {
			vtui.FrameManager.PostTask(func() {
				if HandleSemanticAction(action) {
					vtui.FrameManager.Redraw()
				}
			})
		}
	case "quit":
		if vtui.FrameManager != nil {
			vtui.FrameManager.PostTask(func() {
				vtui.FrameManager.EmitCommand(vtui.CmQuit, nil)
				vtui.FrameManager.Stop()
			})
		}
	}
}

func (h *ExtUiHost) sendEvent(ev *vtinput.InputEvent) {
	if h.reader == nil {
		return
	}
	select {
	case h.reader.EventChan <- ev:
	case <-time.After(500 * time.Millisecond):
		vtui.DebugLog("EXTUI_HOST: dropped event after blocked queue: %s", ev.String())
	}
}

func RunExternalUIWithMapping(backend string) error {
	path, err := findExtUiPath(backend)
	if err != nil {
		return err
	}
	return RunExternalUI(100, 30, path, nil)
}

func findExtUiPath(backend string) (string, error) {
	binName := "f4-qt-host"
	if backend != "qt" {
		binName = strings.TrimPrefix(backend, "ext:")
	}
	if runtime.GOOS == "windows" && !strings.HasSuffix(binName, ".exe") {
		binName += ".exe"
	}

	if env := os.Getenv("F4_EXT_UI_PATH"); env != "" {
		if extUiFileExists(env) {
			return env, nil
		}
	}

	var candidates []string
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(exeDir, binName))
		if runtime.GOOS == "darwin" && strings.HasSuffix(exeDir, ".app/Contents/MacOS") {
			candidates = append(candidates, filepath.Join(filepath.Dir(filepath.Dir(exeDir)), "Resources", binName))
		}
	}

	if cwd, err := os.Getwd(); err == nil {
		for _, cfg := range []string{"RelWithDebInfo", "Release", "Debug"} {
			candidates = append(candidates, filepath.Join(cwd, "qt", "host", "build", "bin", cfg, binName))
		}
		candidates = append(candidates,
			filepath.Join(cwd, "qt", "host", "build", "bin", binName),
			filepath.Join(cwd, "qt", "host", "build", binName),
		)
	}

	for _, path := range candidates {
		if extUiFileExists(path) {
			return path, nil
		}
	}
	return "", fmt.Errorf("external UI executable %q not found", binName)
}

func extUiFileExists(path string) bool {
	if path == "" {
		return false
	}
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
