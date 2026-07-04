package vtui

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/unxed/vtinput"
)

type QtHost struct {
	mu     sync.Mutex
	conn   net.Conn
	send   *qtMessageSender
	reader *vtinput.Reader
	cols   int
	rows   int
}

func runInQtWindow(cols, rows int, setupApp func()) error {
	hostPath, err := findQtHostPath()
	if err != nil {
		return err
	}

	nonce, err := qtNewNonce()
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("failed to create Qt host listener: %w", err)
	}
	defer listener.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, hostPath,
		"--f4-qt-connect="+listener.Addr().String(),
		"--f4-qt-nonce="+nonce,
		"--f4-qt-cols="+strconv.Itoa(cols),
		"--f4-qt-rows="+strconv.Itoa(rows),
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start Qt host %q: %w", hostPath, err)
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
		return fmt.Errorf("Qt host connection failed: %w", err)
	case <-time.After(10 * time.Second):
		return fmt.Errorf("Qt host did not connect within 10s")
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	hello, err := qtReadMessage(conn)
	if err != nil {
		return fmt.Errorf("failed to read Qt host hello: %w", err)
	}
	if qtString(hello, "type") != "hello" || qtString(hello, "nonce") != nonce {
		return fmt.Errorf("invalid Qt host hello")
	}
	sender := &qtMessageSender{w: conn}
	if err := sender.Send(map[string]any{
		"type":     "hello",
		"nonce":    nonce,
		"protocol": qtProtocolVersion,
		"cols":     cols,
		"rows":     rows,
		"app":      AppName,
	}); err != nil {
		return fmt.Errorf("failed to send Qt host hello: %w", err)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return err
	}

	host := &QtHost{conn: conn, send: sender, cols: cols, rows: rows}
	scr := NewScreenBuf()
	scr.AllocBuf(cols, rows)
	scr.Renderer = NewQtRendererWithSender(conn, sender)
	FrameManager.Init(scr)

	pr, _ := io.Pipe()
	reader := vtinput.NewReader(pr, true)
	host.reader = reader

	GetTerminalSize = func() (int, int, error) {
		host.mu.Lock()
		defer host.mu.Unlock()
		return host.cols, host.rows, nil
	}

	go host.readLoop()
	setupApp()
	FrameManager.Run(reader)
	_ = sender.Send(map[string]any{"type": "quit"})
	return nil
}

func (h *QtHost) readLoop() {
	for {
		msg, err := qtReadMessage(h.conn)
		if err != nil {
			DebugLog("QT_HOST: read loop stopped: %v", err)
			if h.reader != nil {
				h.reader.Close()
			}
			return
		}
		h.handleMessage(msg)
	}
}

func (h *QtHost) handleMessage(msg map[string]any) {
	switch qtString(msg, "type") {
	case "resize":
		cols, rows := qtInt(msg, "cols"), qtInt(msg, "rows")
		if cols <= 0 || rows <= 0 {
			return
		}
		h.mu.Lock()
		changed := h.cols != cols || h.rows != rows
		h.cols, h.rows = cols, rows
		h.mu.Unlock()
		if changed {
			h.sendEvent(&vtinput.InputEvent{Type: vtinput.ResizeEventType, InputSource: "qt"})
		}
	case "key":
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.KeyEventType,
			KeyDown:         qtBool(msg, "down"),
			VirtualKeyCode:  uint16(qtInt(msg, "vk")),
			Char:            rune(qtInt(msg, "char")),
			ControlKeyState: vtinput.ControlKeyState(uint32(qtInt(msg, "mods"))),
			InputSource:     "qt",
		})
	case "text":
		for _, r := range qtString(msg, "text") {
			h.sendEvent(&vtinput.InputEvent{
				Type:            vtinput.KeyEventType,
				KeyDown:         true,
				Char:            r,
				ControlKeyState: vtinput.ControlKeyState(uint32(qtInt(msg, "mods"))),
				InputSource:     "qt_text",
			})
		}
	case "mouse":
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          uint16(qtInt(msg, "x")),
			MouseY:          uint16(qtInt(msg, "y")),
			ButtonState:     uint32(qtInt(msg, "button")),
			MouseEventFlags: uint32(qtInt(msg, "flags")),
			KeyDown:         qtBool(msg, "down"),
			ControlKeyState: vtinput.ControlKeyState(uint32(qtInt(msg, "mods"))),
			InputSource:     "qt",
		})
	case "wheel":
		h.sendEvent(&vtinput.InputEvent{
			Type:            vtinput.MouseEventType,
			MouseX:          uint16(qtInt(msg, "x")),
			MouseY:          uint16(qtInt(msg, "y")),
			WheelDirection:  qtInt(msg, "dir"),
			ControlKeyState: vtinput.ControlKeyState(uint32(qtInt(msg, "mods"))),
			InputSource:     "qt",
		})
	case "paste":
		text := qtString(msg, "text")
		h.sendEvent(&vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: true, InputSource: "qt"})
		for _, r := range text {
			h.sendEvent(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: r, InputSource: "qt_paste"})
		}
		h.sendEvent(&vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: false, InputSource: "qt"})
	case "clipboard_get":
		_ = h.send.Send(map[string]any{
			"type": "clipboard_data",
			"text": GetClipboard(),
		})
	case "clipboard_set":
		SetClipboard(qtString(msg, "text"))
	case "ui_action":
		action := msg
		if nested, ok := msg["action"].(map[string]any); ok {
			action = nested
		}
		if FrameManager != nil {
			FrameManager.PostTask(func() {
				if FrameManager.HandleSemanticAction(action) {
					FrameManager.Redraw()
				}
			})
		}
	case "quit":
		if FrameManager != nil {
			FrameManager.PostTask(func() {
				FrameManager.EmitCommand(CmQuit, nil)
				FrameManager.Stop()
			})
		}
	}
}

func (h *QtHost) sendEvent(ev *vtinput.InputEvent) {
	if h.reader == nil {
		return
	}
	select {
	case h.reader.EventChan <- ev:
	case <-time.After(500 * time.Millisecond):
		DebugLog("QT_HOST: dropped event after blocked queue: %s", ev.String())
	}
}
