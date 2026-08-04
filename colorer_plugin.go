package main

import (
	"context"
	"io"
	"strings"
	"sync"

	colorer "github.com/unxed/colorer4go"
	"github.com/unxed/vtui"
)

// Colorer reports region names like "def:Comment", "html:htmlOpenTag" or
// "smarty:OpenTag". A name is resolved to a color in three steps: an exact
// lookup, then the longest key that is a prefix of the name (so
// "def:CommentContent" inherits "def:comment"), and finally the longest
// generic marker contained in the name (so "html:htmlOpenTag" is a tag).
var ColorerColorMap = map[string]uint32{
	"def:comment":   0x555753, // Gray
	"def:directive": 0xAD7FA8, // Purple
	"def:keyword":   0x729FCF, // Light Blue
	"def:symbol":    0xD3D7CF, // Light Gray
	"def:string":    0x8AE234, // Green
	"def:character": 0x8AE234, // Green
	"def:number":    0xAD7FA8, // Purple
	"def:float":     0xAD7FA8, // Purple
	"def:var":       0xE9B96E, // Sand
	"def:function":  0xFCE94F, // Yellow
	"def:type":      0x729FCF, // Light Blue
	"def:class":     0x8CC4FF, // Sky Blue
	"def:method":    0xFCE94F, // Yellow
	"def:parameter": 0xE9B96E, // Sand
	"def:constant":  0xAD7FA8, // Purple
	"def:label":     0xE9B96E, // Sand
	"def:pair":      0xFCE94F, // Yellow
	"def:outlined":  0xFFFFFF, // White
	"def:insertion": 0xFCAF3E, // Orange
	"def:error":     0xFF0000, // Red
	"def:todo":      0xFCAF3E, // Orange
	"def:date":      0xAD7FA8, // Purple

	"comment":      0x555753, // Gray
	"keyword":      0x729FCF, // Light Blue
	"string":       0x8AE234, // Green
	"character":    0x8AE234, // Green
	"number":       0xAD7FA8, // Purple
	"float":        0xAD7FA8, // Purple
	"operator":     0xFFFFFF, // White
	"symbol":       0xD3D7CF, // Light Gray
	"function":     0xFCE94F, // Yellow
	"identifier":   0xEEEEEC, // Near White
	"constant":     0xAD7FA8, // Purple
	"type":         0x729FCF, // Light Blue
	"class":        0x8CC4FF, // Sky Blue
	"method":       0xFCE94F, // Yellow
	"parameter":    0xE9B96E, // Sand
	"preprocessor": 0xAD7FA8, // Purple
	"directive":    0xAD7FA8, // Purple
	"error":        0xFF0000, // Red
	"var":          0xE9B96E, // Sand
	"tag":          0x729FCF, // Light Blue
	"attribute":    0xFCE94F, // Yellow
	"entity":       0xAD7FA8, // Purple
	"pair":         0xFCE94F, // Yellow
	"outline":      0xFFFFFF, // White
	"insertion":    0xFCAF3E, // Orange
	"label":        0xE9B96E, // Sand
	"todo":         0xFCAF3E, // Orange
	"header":       0x8CC4FF, // Sky Blue
}

const (
	// Highlighted lines are cached to avoid re-parsing the file on every
	// frame. The cache is bounded, so huge files do not eat all the memory.
	maxCachedAttrLines  = 8192
	attrCacheKeepWindow = 2048
)

var (
	colorerKeysMu   sync.Mutex
	colorerKeys     []string
	colorerKeysSize = -1
	colorerNames    = make(map[string]int64)
)

// colorerColorKeys returns the map keys ordered from the longest to the
// shortest one, so that the most specific key always wins. Map iteration
// order is random in Go, so an ordered list is required to make the
// resolution deterministic.
func colorerColorKeys() []string {
	colorerKeysMu.Lock()
	defer colorerKeysMu.Unlock()
	if colorerKeysSize != len(ColorerColorMap) {
		keys := make([]string, 0, len(ColorerColorMap))
		for key := range ColorerColorMap {
			keys = append(keys, key)
		}
		sortColorerKeys(keys)
		colorerKeys = keys
		colorerKeysSize = len(ColorerColorMap)
		colorerNames = make(map[string]int64)
	}
	return colorerKeys
}

func lookupColorerColor(name string) (uint32, bool) {
	nameLower := strings.ToLower(name)

	keys := colorerColorKeys()
	colorerKeysMu.Lock()
	cached, hit := colorerNames[nameLower]
	colorerKeysMu.Unlock()
	if hit {
		if cached < 0 {
			return 0, false
		}
		return uint32(cached), true
	}

	color, found := uint32(0), false
	if exact, ok := ColorerColorMap[nameLower]; ok {
		color, found = exact, true
	}
	if !found {
		for _, key := range keys {
			if strings.HasPrefix(nameLower, key) {
				color, found = ColorerColorMap[key], true
				break
			}
		}
	}
	if !found {
		for _, key := range keys {
			if strings.Contains(nameLower, key) {
				color, found = ColorerColorMap[key], true
				break
			}
		}
	}

	stored := int64(-1)
	if found {
		stored = int64(color)
	}
	colorerKeysMu.Lock()
	colorerNames[nameLower] = stored
	colorerKeysMu.Unlock()

	return color, found
}

func getColorerAttr(name string, baseAttr uint64) uint64 {
	if !AppConfig.EditorColorerSyntax {
		return baseAttr
	}
	if style, ok := colorerSchemeStyle(name); ok {
		return applyColorerStyle(baseAttr, style)
	}
	if colorerSchemeActive() {
		// A color style from the catalog is in charge, and colorer leaves a
		// region the style does not define transparent. Falling back to the
		// built-in colors here is what used to drop foreign colors into a
		// monochrome style.
		return baseAttr
	}
	if color, ok := lookupColorerColor(name); ok {
		return vtui.SetRGBFore(baseAttr, color)
	}
	return baseAttr
}

// colorerUTF16ToRuneIndex maps every UTF-16 code unit offset of the line
// (including the offset right past its end) to the index of the rune it
// belongs to. Colorer stores lines as UTF-16 strings, so it reports region
// bounds in code units and not in bytes.
func colorerUTF16ToRuneIndex(line string) []int {
	index := make([]int, 0, len(line)+1)
	runeIdx := 0
	for _, r := range line {
		index = append(index, runeIdx)
		if r > 0xFFFF {
			// Characters outside the BMP take a surrogate pair.
			index = append(index, runeIdx)
		}
		runeIdx++
	}
	return append(index, runeIdx)
}

// colorerLineIndex turns the state the editor hands back into the index of the
// line that is about to be parsed. The state is the index of the previous
// line, so a missing one means the very first line of the file. Falling back
// to the number of lines seen so far instead made every redraw append another
// copy of line zero to the end of the array: the parser then had to be rewound
// for the next line, and the array grew without bound.
func colorerLineIndex(prevState any, known int) int {
	idx := 0
	if prevIdx, ok := prevState.(int); ok {
		idx = prevIdx + 1
	}
	if idx < 0 {
		idx = 0
	}
	if idx > known {
		idx = known
	}
	return idx
}

// colorerNeedsRewind reports whether the engine has to be restarted from the
// first line in order to reach upTo. Colorer only ever parses forward, so a
// line that is still ahead of the parser can simply be fed to it.
func colorerNeedsRewind(parsedIdx, upTo int) bool {
	return parsedIdx < 0 || parsedIdx > upTo
}

// Starting a session means compiling the WASM module and parsing the whole
// HRC catalog, which takes seconds. An idle session is therefore kept around
// and handed over to the next opened file instead of being destroyed.
var (
	colorerPoolMu  sync.Mutex
	colorerIdle    *colorer.Session
	colorerIdleDir string
)

func acquireColorerSession(configsDir string) (*colorer.Session, error) {
	colorerPoolMu.Lock()
	if colorerIdle != nil && colorerIdleDir == configsDir {
		session := colorerIdle
		colorerIdle = nil
		colorerPoolMu.Unlock()
		session.Reset()
		vtui.DebugLog("COLORER: Reusing a pooled session")
		return session, nil
	}
	colorerPoolMu.Unlock()

	catalogPath := "/base/catalog.xml"
	vtui.DebugLog("COLORER: Initializing session with catalog %q, configs %q", catalogPath, configsDir)
	return colorer.NewSession(context.Background(), catalogPath, configsDir)
}

func releaseColorerSession(session *colorer.Session, configsDir string) {
	if session == nil {
		return
	}
	session.Reset()
	colorerPoolMu.Lock()
	if colorerIdle == nil {
		colorerIdle = session
		colorerIdleDir = configsDir
		colorerPoolMu.Unlock()
		return
	}
	colorerPoolMu.Unlock()
	session.Close()
}

// ResetColorerSessions drops the pooled session, so that the next opened file
// starts a fresh one. The schemas are read when a session is created, which is
// what makes a re-downloaded or relocated catalog take effect.
func ResetColorerSessions() {
	colorerPoolMu.Lock()
	session := colorerIdle
	colorerIdle = nil
	colorerIdleDir = ""
	colorerPoolMu.Unlock()
	if session != nil {
		session.Close()
	}
}

// newColorerHighlighter returns a highlighter which works through the fallback
// engine right away and upgrades itself to Colorer as soon as the session is
// ready, so that opening a file is never blocked by the WASM start-up.
func newColorerHighlighter(ev *EditorView, filename, firstLine string, fallback vtui.Highlighter) *ColorerHighlighter {
	SetColorerScheme(AppConfig.EditorColorerScheme)

	ch := &ColorerHighlighter{
		fallback:   fallback,
		filename:   filename,
		firstLine:  firstLine,
		starting:   true,
		configsDir: ColorerConfigsDir(),
	}
	// Colors are resolved through the region parents declared in the schemas,
	// and reading those takes long enough to keep it off the render path.
	StartColorerRegionScan(ch.configsDir)

	go func() {
		session, err := acquireColorerSession(ch.configsDir)
		if err != nil {
			vtui.DebugLog("COLORER: Failed to init session: %v", err)
			ch.useFallback(ev)
			return
		}
		selected, sErr := session.SelectType(filename, firstLine)
		vtui.DebugLog("COLORER: SelectType(%q, len=%d) -> selected=%v, err=%v", filename, len(firstLine), selected, sErr)
		if sErr != nil || !selected {
			// Colorer knows no schema for this file and would paint nothing at
			// all, which is worse than what the fallback engine already shows.
			// The session goes back to the pool and the switch never happens.
			releaseColorerSession(session, ch.configsDir)
			ch.useFallback(ev)
			return
		}

		vtui.FrameManager.PostTask(func() {
			if ch.closed {
				releaseColorerSession(session, ch.configsDir)
				return
			}
			if ev != nil && ev.highlighter != vtui.Highlighter(ch) {
				// The editor moved on to another engine while the session was
				// starting up.
				releaseColorerSession(session, ch.configsDir)
				return
			}
			if closer, ok := ch.fallback.(io.Closer); ok {
				closer.Close()
			}
			ch.fallback = nil
			ch.session = session
			ch.starting = false
			// Nothing the fallback engine produced carries over: its states
			// mean nothing to Colorer, and no line has been parsed yet.
			ch.lines = nil
			ch.attrCache = nil
			ch.parsedIdx = 0
			if ev != nil {
				ev.invalidateStates(0)
			}
			vtui.FrameManager.Redraw()
		})
	}()

	return ch
}

// useFallback hands the file over to the fallback engine once Colorer has said
// it cannot take it. Nothing is colored until that point, so the states of the
// plain phase have to go.
func (ch *ColorerHighlighter) useFallback(ev *EditorView) {
	vtui.FrameManager.PostTask(func() {
		if !ch.starting {
			return
		}
		ch.starting = false
		if ev != nil {
			ev.invalidateStates(0)
		}
		vtui.FrameManager.Redraw()
	})
}

type ColorerHighlighter struct {
	session    *colorer.Session
	fallback   vtui.Highlighter
	lines      []string
	attrCache  map[int][]uint64
	baseAttr   uint64
	baseKnown  bool
	schemeGen  uint64
	parsedIdx  int
	filename   string
	firstLine  string
	configsDir string
	closed     bool
	starting   bool
}

func (ch *ColorerHighlighter) Highlight(line string, prevState any, baseAttr uint64) ([]uint64, any) {
	if ch.session == nil {
		if ch.starting {
			// The session is still coming up. Painting the whole file with one
			// engine and repainting it with another a moment later is more
			// distracting than waiting, so nothing is colored until Colorer
			// has answered.
			return nil, nil
		}
		if ch.fallback != nil {
			return ch.fallback.Highlight(line, prevState, baseAttr)
		}
		vtui.DebugLog("COLORER: Highlight called with nil session")
		return nil, nil
	}

	logIdx := colorerLineIndex(prevState, len(ch.lines))

	// Cached colors are only valid while both the palette they were computed
	// from and the active color style stay the same.
	gen := ColorerSchemeGeneration()
	if !ch.baseKnown || ch.baseAttr != baseAttr || ch.schemeGen != gen {
		ch.baseAttr = baseAttr
		ch.baseKnown = true
		ch.schemeGen = gen
		ch.attrCache = nil
	}

	if logIdx < len(ch.lines) && ch.lines[logIdx] == line {
		if attrs, ok := ch.attrCache[logIdx]; ok {
			return attrs, logIdx
		}
	} else {
		if logIdx < len(ch.lines) {
			vtui.DebugLog("COLORER: Dropping cache from line %d due to edits", logIdx)
			ch.lines = ch.lines[:logIdx]
			ch.dropCacheFrom(logIdx)
		}
		ch.lines = append(ch.lines, line)
	}

	// The Colorer engine is sequential: it can only parse the line that
	// directly follows the previously parsed one. Restore its state only
	// when the requested line is not the expected one.
	if ch.parsedIdx != logIdx {
		ch.resync(logIdx)
	}

	regions, err := ch.session.ParseLine(line)
	ch.parsedIdx = logIdx + 1
	if err != nil {
		vtui.DebugLog("COLORER: ParseLine failed at line %d: %v", logIdx, err)
		return nil, logIdx
	}

	vtui.DebugLog("--- COLORER HIGHLIGHT LINE %d: %q ---", logIdx, line)

	unitToRune := colorerUTF16ToRuneIndex(line)
	lineUnits := len(unitToRune) - 1
	attrs := make([]uint64, unitToRune[lineUnits])
	for i := range attrs {
		attrs[i] = baseAttr
	}

	for _, reg := range regions {
		start, end := reg.Start, reg.End
		if start < 0 {
			start = 0
		}
		if end > lineUnits {
			end = lineUnits
		}
		if start >= end {
			continue
		}
		startRune := unitToRune[start]
		endRune := unitToRune[end]

		var style colorerRegionStyle
		var useScheme bool
		var fallbackColor uint32
		var useFallback bool

		if AppConfig.EditorColorerSyntax {
			if colorerSchemeActive() {
				style, _ = colorerSchemeStyle(reg.Name)
				useScheme = true
				vtui.DebugLog("  [SCHEME] Reg: %q -> %q (Rune: %d..%d), Style: fore=#%06X (has=%v) back=#%06X (has=%v) mode=%d", reg.Name, line[start:end], startRune, endRune, style.fore, style.hasFore, style.back, style.hasBack, style.style)
			} else {
				fallbackColor, useFallback = lookupColorerColor(reg.Name)
				if useFallback {
					vtui.DebugLog("  [FALLBACK] Reg: %q -> %q (Rune: %d..%d), Color: #%06X", reg.Name, line[start:end], startRune, endRune, fallbackColor)
				}
			}
		}

		for i := startRune; i < endRune && i < len(attrs); i++ {
			if useScheme {
				attrs[i] = applyColorerStyle(attrs[i], style)
			} else if useFallback {
				attrs[i] = vtui.SetRGBFore(attrs[i], fallbackColor)
			}
		}
	}

	ch.storeAttrs(logIdx, attrs)
	return attrs, logIdx
}

// resync brings the engine to the state of the line that is about to be
// parsed. Colorer only ever moves forward, so a line still ahead of the parser
// is simply fed to it; only a line behind it costs a rewind and a replay from
// the very beginning.
func (ch *ColorerHighlighter) resync(upTo int) {
	if upTo > len(ch.lines) {
		upTo = len(ch.lines)
	}
	if upTo < 0 {
		upTo = 0
	}

	from := ch.parsedIdx
	if colorerNeedsRewind(from, upTo) {
		vtui.DebugLog("COLORER: Rewinding the parser from line %d to line %d", ch.parsedIdx, upTo)
		ch.session.Reset()
		// The type is chosen from the first line as well, so the one the
		// session started with is kept for the case of an empty array.
		firstLine := ch.firstLine
		if len(ch.lines) > 0 {
			firstLine = ch.lines[0]
		}
		_, _ = ch.session.SelectType(ch.filename, firstLine)
		from = 0
	}
	for i := from; i < upTo; i++ {
		_, _ = ch.session.ParseLine(ch.lines[i])
	}
	ch.parsedIdx = upTo
}

func (ch *ColorerHighlighter) storeAttrs(idx int, attrs []uint64) {
	if ch.attrCache == nil {
		ch.attrCache = make(map[int][]uint64)
	}
	if len(ch.attrCache) >= maxCachedAttrLines {
		for key := range ch.attrCache {
			if key < idx-attrCacheKeepWindow || key > idx+attrCacheKeepWindow {
				delete(ch.attrCache, key)
			}
		}
		if len(ch.attrCache) >= maxCachedAttrLines {
			ch.attrCache = make(map[int][]uint64)
		}
	}
	ch.attrCache[idx] = attrs
}

func (ch *ColorerHighlighter) dropCacheFrom(idx int) {
	for key := range ch.attrCache {
		if key >= idx {
			delete(ch.attrCache, key)
		}
	}
}

func (ch *ColorerHighlighter) Close() error {
	ch.closed = true
	ch.lines = nil
	ch.attrCache = nil
	ch.parsedIdx = 0
	if closer, ok := ch.fallback.(io.Closer); ok {
		closer.Close()
	}
	ch.fallback = nil
	if ch.session != nil {
		releaseColorerSession(ch.session, ch.configsDir)
		ch.session = nil
	}
	return nil
}
