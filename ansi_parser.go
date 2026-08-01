package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/unxed/vtui"
)

type ParserState int

var DefaultTermAttr = vtui.SetIndexBoth(0, 7, 0) // Light Gray on Black (standard ANSI indices)

const (
	StateGround ParserState = iota
	StateEsc
	StateEscIntermediate
	StateCSI
	StateOSC
	StateAPC
)

// AnsiParser converts a stream of bytes into ScreenBuf operations.
type AnsiParser struct {
	State        ParserState
	Params       []string
	CurParam     strings.Builder
	Intermediate string
	Attr         uint64
	term         *TerminalView
	pty          PtyBackend
	runeBuf      []byte
	lastRune     rune
}

func NewAnsiParser(t *TerminalView, p PtyBackend) *AnsiParser {
	return &AnsiParser{
		term: t,
		pty:  p,
		Attr: DefaultTermAttr,
	}
}

func (p *AnsiParser) Process(data []byte) {
	if p == nil || len(data) == 0 {
		return
	}

	// Heuristics: Hide background sync commands.
	// We MUST use bytes.* functions to avoid string() casts which destroy
	// incomplete UTF-8 sequences (e.g. when ConPTY splits a wide character chunk).
	if bytes.HasPrefix(data, []byte(" set +H; cd ")) {
		idx := bytes.Index(data, []byte("}\r\n"))
		if idx != -1 {
			data = data[idx+3:]
		}
	}

	if bytes.HasPrefix(data, []byte(" set +H; { printf")) {
		idx := bytes.Index(data, []byte("}\r\n"))
		if idx != -1 {
			data = data[idx+3:]
		}
	}

	if bytes.HasPrefix(data, []byte(" { printf")) {
		idx := bytes.Index(data, []byte("}\r\n"))
		if idx != -1 {
			data = data[idx+3:]
		}
	}

	for {
		startIdx := bytes.Index(data, []byte(" cd '"))
		if startIdx == -1 {
			break
		}
		endIdx := bytes.Index(data[startIdx:], []byte("' # f4_sync"))
		if endIdx != -1 {
			actualEnd := startIdx + endIdx + len("' # f4_sync")
			if actualEnd < len(data) && data[actualEnd] == '\r' {
				actualEnd++
			}
			if actualEnd < len(data) && data[actualEnd] == '\n' {
				actualEnd++
			}
			vtui.DebugLog("ANSI_PARSER: Excising background Unix CD sync")
			newData := make([]byte, 0, startIdx+5+len(data)-actualEnd)
			newData = append(newData, data[:startIdx]...)
			newData = append(newData, []byte("\r\x1b[2K")...)
			newData = append(newData, data[actualEnd:]...)
			data = newData
			continue
		}
		break
	}

	// Windows f4 sync commands excision
	for {
		startIdx := bytes.Index(data, []byte("cd /d \""))
		if startIdx == -1 {
			break
		}

		endIdx := bytes.Index(data[startIdx:], []byte("\" & "))
		if endIdx != -1 {
			if bytes.HasPrefix(data[startIdx+endIdx:], []byte("\" & rem f4_sync")) {
				actualEnd := startIdx + endIdx + len("\" & rem f4_sync")
				if actualEnd < len(data) && data[actualEnd] == '\r' {
					actualEnd++
				}
				if actualEnd < len(data) && data[actualEnd] == '\n' {
					actualEnd++
				}
				if actualEnd < len(data) && data[actualEnd] == '\r' {
					actualEnd++
				}
				if actualEnd < len(data) && data[actualEnd] == '\n' {
					actualEnd++
				}
				vtui.DebugLog("ANSI_PARSER: Excising background Windows CD sync")
				newData := make([]byte, 0, startIdx+5+len(data)-actualEnd)
				newData = append(newData, data[:startIdx]...)
				newData = append(newData, []byte("\r\x1b[2K")...)
				newData = append(newData, data[actualEnd:]...)
				data = newData
				continue
			} else {
				actualEnd := startIdx + endIdx + 4
				vtui.DebugLog("ANSI_PARSER: Excising technical Windows CD sync from buffer")
				newData := make([]byte, 0, startIdx+len(data)-actualEnd)
				newData = append(newData, data[:startIdx]...)
				newData = append(newData, data[actualEnd:]...)
				data = newData
				continue
			}
		}
		break
	}

	if len(data) == 0 {
		return
	}

	for _, b := range data {
		// vtui.DebugLog("PARSER: Byte 0x%02X State %v", b, p.State)
		switch p.State {
		case StateGround:
			if b == 0x1b {
				p.State = StateEsc
				p.runeBuf = p.runeBuf[:0]
			} else if b < 0x80 {
				r := rune(b)
				p.term.PutChar(r, p.Attr)
				p.lastRune = r
				p.runeBuf = p.runeBuf[:0]
			} else {
				p.runeBuf = append(p.runeBuf, b)
				if utf8.FullRune(p.runeBuf) {
					r, _ := utf8.DecodeRune(p.runeBuf)
					p.term.PutChar(r, p.Attr)
					p.lastRune = r
					p.runeBuf = p.runeBuf[:0]
				} else if len(p.runeBuf) >= 4 {
					// Invalid sequence or too long, flush as is
					p.term.PutChar(rune(p.runeBuf[0]), p.Attr)
					p.runeBuf = p.runeBuf[1:]
				}
			}
		case StateEsc:
			if b == '[' {
				p.State = StateCSI
				p.Params = nil
				p.CurParam.Reset()
				p.Intermediate = ""
			} else if b == ']' {
				p.State = StateOSC
				p.Params = nil
				p.CurParam.Reset()
			} else if b == '_' {
				p.CurParam.Reset()
				p.State = StateAPC
			} else if b == '\\' {
				// String Terminator (ST)
				p.State = StateGround
			} else if b >= 0x20 && b <= 0x2F {
				// Промежуточные байты (например, '(' в ESC ( B)
				p.State = StateEscIntermediate
			} else {
				p.handleEsc(b)
				p.State = StateGround
			}
		case StateEscIntermediate:
			if b >= 0x20 && b <= 0x2F {
				// Продолжаем собирать промежуточные байты
			} else {
				// b — финальный байт (0x30-0x7E), завершаем последовательность
				p.State = StateGround
			}
		case StateCSI:
			if b >= 0x30 && b <= 0x39 { // '0'-'9'
				p.CurParam.WriteByte(b)
			} else if b == ';' {
				p.Params = append(p.Params, p.CurParam.String())
				p.CurParam.Reset()
			} else if b >= 0x3C && b <= 0x3F { // < = > ?
				p.CurParam.WriteByte(b)
			} else if b >= 0x20 && b <= 0x2F {
				// Intermediate bytes
				p.Intermediate += string(b)
			} else if b >= 0x40 && b <= 0x7E {
				p.Params = append(p.Params, p.CurParam.String())
				p.handleCSI(b)
				p.State = StateGround
			}
		case StateOSC:
			if b == 0x07 { // BEL
				p.handleOSC()
				p.State = StateGround
			} else if b == 0x1b { // ESC
				p.handleOSC()
				p.State = StateEsc
			} else {
				p.CurParam.WriteByte(b)
			}
		case StateAPC:
			if b == 0x07 { // BEL
				p.handleAPC()
				p.State = StateGround
			} else if b == 0x1b { // ESC
				p.handleAPC()
				p.State = StateEsc
			} else {
				p.CurParam.WriteByte(b)
			}
		}
	}
	p.term.FlushLog()
}
func (p *AnsiParser) handleEsc(cmd byte) {
	// vtui.DebugLog("ANSI_PARSER: ESC %c", cmd)
	switch cmd {
	case '7':
		p.term.SaveCursor()
	case '8':
		p.term.RestoreCursor()
	case 'M': // Reverse Index
		p.term.ReverseIndex()
	case 'D': // Index
		p.term.Index()
	case 'E': // Next Line
		p.term.NextLine()
	case 'c': // RIS - Reset to Initial State
		p.term.ResetBuffer(p.term.Width, p.term.Height)
	case '=', '>':
		// Application/Numeric Keypad Mode - пока просто поглощаем
	}
}

func (p *AnsiParser) handleCSI(cmd byte) {
	// vtui.DebugLog("ANSI_PARSER: CSI %s %c (args: %v, intermediate: %q)", p.CurParam.String(), cmd, p.Params, p.Intermediate)
	args := make([]int, len(p.Params))
	// If there are no arguments, args will be an empty slice.
	// This is important for correct handling of default commands.
	for i, s := range p.Params {
		s = strings.TrimLeft(s, "?<=>")
		val, _ := strconv.Atoi(s)
		args[i] = val
	}

	switch cmd {
	case 'm':
		if len(args) == 0 {
			p.handleSGR(args, 0) // Default reset
		} else {
			for i := 0; i < len(args); {
				consumed := p.handleSGR(args, i)
				i += consumed
			}
		}
	case 'H', 'f':
		row, col := 1, 1
		if len(args) > 0 && args[0] != 0 {
			row = args[0]
		}
		if len(args) > 1 && args[1] != 0 {
			col = args[1]
		}
		p.term.SetCursor(col-1, row-1)
	case 'J':
		mode := 0
		if len(args) > 0 {
			mode = args[0]
		}
		p.term.EraseDisplay(mode, p.Attr)
	case 'K':
		mode := 0
		if len(args) > 0 {
			mode = args[0]
		}
		p.term.EraseLine(mode, p.Attr)
	case 'r': // DECSTBM - Set Top and Bottom Margins
		top, bottom := 1, p.term.Height
		if len(args) > 0 && args[0] != 0 {
			top = args[0]
		}
		if len(args) > 1 && args[1] != 0 {
			bottom = args[1]
		}
		p.term.ScrollTop = top - 1
		p.term.ScrollBottom = bottom - 1
		p.term.SetCursor(0, 0)
	case 'c':
		if p.pty != nil {
			p.pty.Write([]byte("\x1b[?1;2c"))
		}
	case 't':
		if len(args) > 0 && p.pty != nil {
			if args[0] == 18 {
				resp := fmt.Sprintf("\x1b[8;%d;%dt", p.term.Height, p.term.Width)
				p.pty.Write([]byte(resp))
			} else if args[0] == 14 {
				resp := fmt.Sprintf("\x1b[4;%d;%dt", p.term.Height*16, p.term.Width*8)
				p.pty.Write([]byte(resp))
			} else if args[0] == 16 {
				resp := fmt.Sprintf("\x1b[6;16;8t")
				p.pty.Write([]byte(resp))
			}
		}
	case 'h', 'l': // DECSET / DECRST
		isSet := cmd == 'h'
		for _, s := range p.Params {
			s = strings.TrimLeft(s, "?")
			switch s {
			case "1":
				p.term.ApplicationCursorKeys = isSet
			case "7":
				p.term.AutoWrap = isSet
			case "1000":
				if isSet {
					p.term.MouseTrackingMode = 1000
				} else {
					p.term.MouseTrackingMode = 0
				}
			case "1002":
				if isSet {
					p.term.MouseTrackingMode = 1002
				} else {
					p.term.MouseTrackingMode = 0
				}
			case "1003":
				if isSet {
					p.term.MouseTrackingMode = 1003
				} else {
					p.term.MouseTrackingMode = 0
				}
			case "1006":
				p.term.MouseSGRMode = isSet
			case "1049", "47":
				p.term.SetAltScreen(isSet)
				if isSet {
					p.term.EraseDisplay(2, p.Attr)
				}
			case "9001":
				p.term.Win32InputMode = isSet
			case "2004":
				p.term.BracketedPasteMode = isSet
			}
		}
	case 'L': // Insert blank lines
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.ScrollDown(p.term.CursorY, p.term.ScrollBottom, n)
	case 'M': // Delete lines
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.scrollUp(p.term.CursorY, p.term.ScrollBottom, n)
	case 'P': // Delete characters
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.DeleteCharacters(n, p.Attr)
	case '@': // Insert blank characters
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.InsertBlankCharacters(n, p.Attr)
	case 'S': // Scroll up (text moves up)
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.scrollUp(p.term.ScrollTop, p.term.ScrollBottom, n)
	case 'T': // Scroll down (text moves down)
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.ScrollDown(p.term.ScrollTop, p.term.ScrollBottom, n)
	case 'A':
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.SetCursor(p.term.CursorX, p.term.CursorY-n)
	case 'B':
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.SetCursor(p.term.CursorX, p.term.CursorY+n)
	case 'C':
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.SetCursor(p.term.CursorX+n, p.term.CursorY)
	case 'D':
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.SetCursor(p.term.CursorX-n, p.term.CursorY)
	case 'G', '`':
		col := 1
		if len(args) > 0 && args[0] != 0 {
			col = args[0]
		}
		p.term.SetCursor(col-1, p.term.CursorY)
	case 'd':
		row := 1
		if len(args) > 0 && args[0] != 0 {
			row = args[0]
		}
		p.term.SetCursor(p.term.CursorX, row-1)
	case 'n': // DSR - Device Status Report
		if len(args) > 0 {
			if args[0] == 5 {
				if p.pty != nil {
					p.pty.Write([]byte("\x1b[0n"))
				}
			} else if args[0] == 6 {
				if p.pty != nil {
					resp := fmt.Sprintf("\x1b[%d;%dR", p.term.CursorY+1, p.term.CursorX+1)
					p.pty.Write([]byte(resp))
				}
			}
		}
	case 's':
		if len(p.Params) == 0 || (len(p.Params) == 1 && p.Params[0] == "") {
			p.term.SaveCursor()
		}
	case 'u':
		s0 := ""
		if len(p.Params) > 0 {
			s0 = p.Params[0]
		}
		if s0 == "" {
			p.term.RestoreCursor()
		} else if strings.HasPrefix(s0, "=") {
			flags, _ := strconv.Atoi(s0[1:])
			mode := 1
			if len(p.Params) > 1 {
				mode, _ = strconv.Atoi(p.Params[1])
			}
			if mode == 1 {
				p.term.KittyFlags = flags
			} else if mode == 2 {
				p.term.KittyFlags |= flags
			} else if mode == 3 {
				p.term.KittyFlags &= ^flags
			}
		} else if strings.HasPrefix(s0, ">") {
			flags, _ := strconv.Atoi(s0[1:])
			if len(p.term.KittyFlagsStack) >= 32 {
				p.term.KittyFlagsStack = p.term.KittyFlagsStack[1:] // Limit stack size
			}
			p.term.KittyFlagsStack = append(p.term.KittyFlagsStack, p.term.KittyFlags)
			p.term.KittyFlags = flags
		} else if strings.HasPrefix(s0, "<") {
			count, _ := strconv.Atoi(s0[1:])
			if count <= 0 {
				count = 1
			}
			for i := 0; i < count; i++ {
				if len(p.term.KittyFlagsStack) == 0 {
					p.term.KittyFlags = 0
					break
				}
				last := len(p.term.KittyFlagsStack) - 1
				p.term.KittyFlags = p.term.KittyFlagsStack[last]
				p.term.KittyFlagsStack = p.term.KittyFlagsStack[:last]
			}
		} else if strings.HasPrefix(s0, "?") {
			if p.pty != nil {
				resp := fmt.Sprintf("\x1b[?%du", p.term.KittyFlags)
				p.pty.Write([]byte(resp))
			}
		} else {
			p.term.RestoreCursor()
		}
	case 'b': // REP - Repeat last character
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.RepeatLastChar(n, p.lastRune, p.Attr)
	case 'X': // ECH - Erase Character
		n := 1
		if len(args) > 0 && args[0] != 0 {
			n = args[0]
		}
		p.term.EraseCharacter(n, p.Attr)
	case 'p': // DECRQM
		if p.Intermediate == "$" {
			p.handleDECRQM(args)
		}
	}
}

func (p *AnsiParser) handleDECRQM(args []int) {
	if len(p.Params) == 0 || p.pty == nil {
		return
	}

	// Ensure there is an actual mode number provided (not just an empty param or prefix)
	modeStr := strings.TrimLeft(p.Params[0], "?<=>")
	if modeStr == "" {
		return
	}

	mode := args[0]
	isDecPrivate := len(p.Params) > 0 && strings.HasPrefix(p.Params[0], "?")

	state := 0 // 0 = not recognized
	if isDecPrivate {
		switch mode {
		case 1:
			if p.term.ApplicationCursorKeys {
				state = 1
			} else {
				state = 2
			}
		case 47, 1049:
			if p.term.UseAltScreen {
				state = 1
			} else {
				state = 2
			}
		case 2004:
			if p.term.BracketedPasteMode {
				state = 1
			} else {
				state = 2
			}
		case 9001:
			if p.term.Win32InputMode {
				state = 1
			} else {
				state = 2
			}
		}
		resp := fmt.Sprintf("\x1b[?%d;%d$y", mode, state)
		p.pty.Write([]byte(resp))
	} else {
		resp := fmt.Sprintf("\x1b[%d;%d$y", mode, state)
		p.pty.Write([]byte(resp))
	}
}

func (p *AnsiParser) handleOSC() {
	s := p.CurParam.String()
	// vtui.DebugLog("ANSI_PARSER: OSC payload: %q", s)
	p.CurParam.Reset()
	if s == "" {
		return
	}

	parts := strings.SplitN(s, ";", 2)
	if len(parts) < 2 {
		return
	}

	cmd, err := strconv.Atoi(parts[0])
	if err != nil {
		return
	}

	if cmd == 0 || cmd == 2 {
		vtui.DebugLog("ANSI_OSC_TRACE: Received window title change: %q", parts[1])
		p.term.Title = parts[1]
		if p.term.OnTitleChange != nil {
			p.term.OnTitleChange(p.term.Title)
		}
		return
	}
	if cmd == 133 {
		// В последовательности OSC 133;C BEL, cmd это 133, а аргумент 'C' находится в parts[1]
		if len(parts) > 1 {
			p.term.HandleOSC133(parts[1])
		}
		return
	}

	if cmd == 52 {
		subparts := strings.SplitN(parts[1], ";", 2)
		if len(subparts) == 2 {
			if subparts[1] == "?" {
				if p.pty != nil {
					go func(subCmd string) {
						allowed := false
						if vtui.GlobalClipboardAccessManager != nil {
							auth := vtui.GlobalClipboardAccessManager.Authorize("Terminal_OSC52_Read")
							if auth == 1 || auth == 2 {
								allowed = true
							}
						}
						if allowed {
							clip := vtui.GetClipboard()
							b64 := base64.StdEncoding.EncodeToString([]byte(clip))
							p.pty.Write([]byte(fmt.Sprintf("\x1b]52;%s;%s\x07", subCmd, b64)))
						}
					}(subparts[0])
				}
			} else {
				decoded, err := base64.StdEncoding.DecodeString(subparts[1])
				if err == nil {
					vtui.SetClipboard(string(decoded))
				}
			}
		}
		return
	}

	if cmd == 4 {
		subparts := strings.SplitN(parts[1], ";", 2)
		if len(subparts) < 2 {
			return
		}

		idx, _ := strconv.Atoi(subparts[0])
		if idx < 0 || idx >= 256 {
			return
		}

		colorStr := subparts[1]
		var rgbVal uint32
		parsed := false

		if strings.HasPrefix(colorStr, "#") && len(colorStr) >= 7 {
			v, err := strconv.ParseUint(colorStr[1:7], 16, 32)
			if err == nil {
				rgbVal = uint32(v)
				parsed = true
			}
		} else if strings.HasPrefix(colorStr, "rgb:") {
			// format rgb:RR/GG/BB
			rgbParts := strings.Split(colorStr[4:], "/")
			if len(rgbParts) == 3 {
				r, _ := strconv.ParseUint(rgbParts[0], 16, 8)
				g, _ := strconv.ParseUint(rgbParts[1], 16, 8)
				b, _ := strconv.ParseUint(rgbParts[2], 16, 8)
				rgbVal = uint32((r << 16) | (g << 8) | b)
				parsed = true
			}
		}

		if parsed {
			p.term.Palette[idx] = rgbVal
		}
	}
}

func (p *AnsiParser) handleAPC() {
	s := p.CurParam.String()
	p.CurParam.Reset()
	if strings.HasPrefix(s, "far2l") {
		p.term.HandleFar2lAPC(s)
	}
}

func (p *AnsiParser) handleSGR(args []int, i int) int {
	if len(args) == 0 {
		p.Attr = DefaultTermAttr
		return 1
	}

	n := args[i]
	switch {
	case n == 0:
		p.Attr = DefaultTermAttr
	case n == 1:
		p.Attr |= vtui.ForegroundIntensity
	case n == 2:
		p.Attr |= vtui.ForegroundDim
	case n == 4:
		p.Attr |= vtui.CommonLvbUnderscore
	case n == 5:
		// Blink - ignored in many TUIs or mapped to intensity
	case n == 7:
		p.Attr |= vtui.CommonLvbReverse
	case n == 9:
		p.Attr |= vtui.CommonLvbStrikeout
	case n == 22:
		p.Attr &= ^(vtui.ForegroundIntensity | vtui.ForegroundDim)
	case n == 24:
		p.Attr &= ^vtui.CommonLvbUnderscore
	case n == 27:
		p.Attr &= ^vtui.CommonLvbReverse
	case n == 29:
		p.Attr &= ^vtui.CommonLvbStrikeout

	case n >= 30 && n <= 37:
		p.Attr = vtui.SetIndexFore(p.Attr, uint8(n-30))
	case n == 38:
		if i+2 < len(args) {
			if args[i+1] == 5 { // 256 colors
				idx := args[i+2]
				if idx >= 0 && idx < 256 {
					p.Attr = vtui.SetIndexFore(p.Attr, uint8(idx))
				}
				return 3
			} else if args[i+1] == 2 && i+4 < len(args) { // TrueColor
				r, g, b := uint32(args[i+2]), uint32(args[i+3]), uint32(args[i+4])
				p.Attr = vtui.SetRGBFore(p.Attr, (r<<16)|(g<<8)|b)
				return 5
			}
		}
	case n == 39:
		p.Attr = vtui.SetIndexFore(p.Attr, vtui.GetIndexFore(DefaultTermAttr))

	case n >= 40 && n <= 47:
		p.Attr = vtui.SetIndexBack(p.Attr, uint8(n-40))
	case n == 48:
		if i+2 < len(args) {
			if args[i+1] == 5 { // 256 colors
				idx := args[i+2]
				if idx >= 0 && idx < 256 {
					p.Attr = vtui.SetIndexBack(p.Attr, uint8(idx))
				}
				return 3
			} else if args[i+1] == 2 && i+4 < len(args) { // TrueColor
				r, g, b := uint32(args[i+2]), uint32(args[i+3]), uint32(args[i+4])
				p.Attr = vtui.SetRGBBack(p.Attr, (r<<16)|(g<<8)|b)
				return 5
			}
		}
	case n == 49:
		p.Attr = vtui.SetIndexBack(p.Attr, vtui.GetIndexBack(DefaultTermAttr))

	case n >= 90 && n <= 97:
		p.Attr = vtui.SetIndexFore(p.Attr, uint8(n-90+8))
	case n >= 100 && n <= 107:
		p.Attr = vtui.SetIndexBack(p.Attr, uint8(n-100+8))
	}
	return 1
}
