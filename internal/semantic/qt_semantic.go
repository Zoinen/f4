package semantic

import (
	"encoding/binary"
	"fmt"
	"github.com/unxed/f4/internal/numeric"
	"github.com/unxed/f4/sdk/extui"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"hash"
	"path/filepath"
	"reflect"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

func WriteSemanticFingerprintString(h hash.Hash, value string) {
	var size [8]byte
	binary.LittleEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = h.Write(size[:])
	_, _ = h.Write([]byte(value))
}

func SemanticWriteUint64(h hash.Hash, value uint64) {
	var encoded [8]byte
	binary.LittleEndian.PutUint64(encoded[:], value)
	_, _ = h.Write(encoded[:])
}

func SemanticEditPositions(edit *vtui.Edit, text string) (cursor, selectionStart, selectionEnd int) {
	runes := []rune(text)
	readRuneIndex := func(name string, fallback int) int {
		if edit == nil {
			return fallback
		}
		value := reflect.ValueOf(edit)
		if value.Kind() != reflect.Pointer || value.IsNil() {
			return fallback
		}
		field := value.Elem().FieldByName(name)
		if !field.IsValid() || field.Kind() != reflect.Int {
			return fallback
		}
		index := int(field.Int())
		if index < 0 {
			return index
		}
		if index > len(runes) {
			return len(runes)
		}
		return index
	}
	toUTF16 := func(runeIndex int) int {
		if runeIndex < 0 {
			return -1
		}
		return len(utf16.Encode(runes[:runeIndex]))
	}

	cursor = toUTF16(readRuneIndex("curPos", len(runes)))
	selectionStart = toUTF16(readRuneIndex("selStart", -1))
	selectionEnd = toUTF16(readRuneIndex("selEnd", -1))
	return cursor, selectionStart, selectionEnd
}

type SemanticSurfaceWindow struct {
	Ready        bool
	LoadError    string
	Rows         []extui.TextRowModel
	Start        int64
	End          int64
	ViewportRow  int
	ViewportRows int
	ViewportSpan int64
}

func SemanticWindowBufferRows(viewportRows int) int {
	// Keep one complete viewport before and after the visible viewport.  The
	// native surface therefore has three screens of data and can continue a
	// high-velocity flick while the next bounded window is prepared.  This is
	// still O(viewport), independent of document size.
	return max(8, viewportRows)
}

func RunsFromCells(cells []vtui.CharInfo) []extui.RunModel {
	if len(cells) == 0 {
		return nil
	}
	var runs []extui.RunModel
	var b strings.Builder
	var attr uint64
	haveRun := false
	flush := func() {
		if !haveRun {
			return
		}
		runs = append(runs, RunModel(b.String(), attr))
		b.Reset()
	}
	pairedFillers := 0
	for _, cell := range cells {
		if cell.Char == vtui.WideCharFiller {
			// A complete wide glyph already occupies the filler's display
			// column in Qt, so its paired marker must not become another cell.
			// Horizontal clipping can, however, leave the continuation marker
			// as the first visible cell. Preserve that orphan as a blank or all
			// following text shifts left by one column.
			if pairedFillers > 0 {
				pairedFillers--
				continue
			}
			cell.Char = ' '
		} else {
			pairedFillers = max(0, vtui.StringWidth(vtui.CellString(cell.Char))-1)
		}
		if !haveRun {
			attr = cell.Attributes
			haveRun = true
		} else if cell.Attributes != attr {
			flush()
			attr = cell.Attributes
			haveRun = true
		}
		b.WriteString(vtui.CellString(cell.Char))
	}
	flush()
	return runs
}

func RunModel(text string, attr uint64) extui.RunModel {
	return extui.RunModel{
		Text:       text,
		Attr:       attr,
		Foreground: SemanticAttrColor(attr, true),
		Background: SemanticAttrColor(attr, false),
		Bold:       attr&vtui.ForegroundIntensity != 0,
		Underline:  attr&vtui.CommonLvbUnderscore != 0,
		Strikeout:  attr&vtui.CommonLvbStrikeout != 0,
	}
}

type semanticRenderedSurface struct {
	Rows             [][]extui.RunModel
	CursorPrefixRuns []extui.RunModel
	CursorX          int
	CursorY          int
	CursorVisible    bool
	CursorShape      string
}

func SemanticRenderSurface(x1, y1, x2, y2 int, render func(*vtui.ScreenBuf)) semanticRenderedSurface {
	result := semanticRenderedSurface{CursorX: -1, CursorY: -1}
	if render == nil || x2 < x1 || y2 < y1 {
		return result
	}
	scr := vtui.NewScreenBuf()
	scr.AllocBuf(max(1, x2+1), max(1, y2+1))
	scr.ThemePalette = &vtui.ThemePalette
	render(scr)
	cursorX, cursorY, visible, shape := scr.GetCursorStateForTesting()
	result.Rows = make([][]extui.RunModel, 0, y2-y1+1)
	for y := y1; y <= y2; y++ {
		cells := make([]vtui.CharInfo, 0, x2-x1+1)
		for x := x1; x <= x2; x++ {
			cells = append(cells, scr.GetCell(x, y))
		}
		result.Rows = append(result.Rows, RunsFromCells(cells))
		if visible && y == cursorY && cursorX >= x1 && cursorX <= x2 {
			prefixLength := min(cursorX-x1, len(cells))
			result.CursorPrefixRuns = RunsFromCells(cells[:prefixLength])
		}
	}
	if visible && cursorX >= x1 && cursorX <= x2 && cursorY >= y1 && cursorY <= y2 {
		result.CursorX = cursorX - x1
		result.CursorY = cursorY - y1
		result.CursorVisible = true
		switch shape {
		case vtui.CursorShapeBlock:
			result.CursorShape = "block"
		default:
			result.CursorShape = "underline"
		}
	}
	return result
}

func SemanticRowsWithRenderedRunsAt(rows []extui.TextRowModel,
	rendered [][]extui.RunModel, firstRow int) []extui.TextRowModel {
	if len(rendered) == 0 || len(rows) == 0 {
		return rows
	}
	result := append([]extui.TextRowModel(nil), rows...)
	for index, runs := range rendered {
		rowIndex := firstRow + index
		if rowIndex < 0 || rowIndex >= len(result) {
			continue
		}
		result[rowIndex].Text = ""
		result[rowIndex].Runs = runs
	}
	return result
}

func SemanticAttrColor(attr uint64, foreground bool) string {
	reverse := attr&vtui.CommonLvbReverse != 0
	if reverse {
		foreground = !foreground
	}
	var rgb uint32
	if foreground {
		if attr&vtui.IsFgRGB != 0 {
			rgb = vtui.GetRGBFore(attr)
		} else {
			rgb = vtui.ThemePalette[vtui.GetIndexFore(attr)]
		}
		if attr&vtui.ForegroundDim != 0 {
			rgb = ((rgb>>16&0xff)/2)<<16 | ((rgb>>8&0xff)/2)<<8 | (rgb&0xff)/2
		}
	} else if attr&vtui.IsBgRGB != 0 {
		rgb = vtui.GetRGBBack(attr)
	} else {
		rgb = vtui.ThemePalette[vtui.GetIndexBack(attr)]
	}
	return fmt.Sprintf("#%06x", rgb&0xffffff)
}

func CellRune(ch uint64) rune {
	if ch == 0 || ch > utf8.MaxRune || (ch >= 0xD800 && ch <= 0xDFFF) {
		return ' '
	}
	return rune(ch)
}

func SemanticBaseName(v interface{ Base(string) string }, path string) string {
	if path == "" {
		return ""
	}
	if v != nil {
		return v.Base(path)
	}
	return filepath.Base(path)
}

func SemanticLocalPath(v vfs.VFS, path string) string {
	if v == nil || path == "" {
		return ""
	}
	provider, ok := v.(vfs.LocalPathProvider)
	if !ok {
		return ""
	}
	localPath, err := provider.LocalPath(path)
	if err != nil {
		return ""
	}
	return localPath
}

func String(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func SemanticAcceptWindowGeneration(action map[string]any, acknowledged uint64,
	highWater *uint64,
) (generation uint64, accepted bool) {
	latest := acknowledged
	if highWater != nil && *highWater > latest {
		latest = *highWater
	}
	if raw, present := action["generation"]; present {
		requested := Int64(raw)
		if requested <= 0 {
			return 0, false
		}
		generation = uint64(requested)
		if generation <= latest {
			return generation, false
		}
	} else {
		if latest == ^uint64(0) {
			return latest, false
		}
		generation = latest + 1
	}
	if highWater != nil {
		*highWater = generation
	}
	return generation, true
}

func Int(v any) int {
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
		value, _ := numeric.BoundedInt64ToInt(n)
		return value
	case uint:
		value, _ := numeric.BoundedUint64ToInt(uint64(n))
		return value
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		value, _ := numeric.BoundedUint64ToInt(uint64(n))
		return value
	case uint64:
		value, _ := numeric.BoundedUint64ToInt(n)
		return value
	case float32:
		return int(n)
	case float64:
		return int(n)
	}
	return 0
}

func Int64(v any) int64 {
	switch n := v.(type) {
	case int:
		return int64(n)
	case int8:
		return int64(n)
	case int16:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case uint:
		return int64(n)
	case uint8:
		return int64(n)
	case uint16:
		return int64(n)
	case uint32:
		return int64(n)
	case uint64:
		if n > uint64(^uint64(0)>>1) {
			return 0
		}
		return int64(n)
	case float32:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}

func StringSlice(v any) []string {
	switch values := v.(type) {
	case []string:
		return append([]string(nil), values...)
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return out
	}
	return nil
}

func SemanticIntSlice(v any) []int {
	switch values := v.(type) {
	case []int:
		return append([]int(nil), values...)
	case []any:
		out := make([]int, 0, len(values))
		for _, value := range values {
			out = append(out, Int(value))
		}
		return out
	}
	return nil
}

func Bool(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	if n, ok := v.(int); ok {
		return n != 0
	}
	if f, ok := v.(float64); ok {
		return f != 0
	}
	return false
}
