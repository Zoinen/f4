package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	colorer "github.com/unxed/colorer4go"
	"github.com/unxed/vtui"
)

const (
	maxCachedAttrLines  = 8192
	attrCacheKeepWindow = 2048
)

// The style bits an hrd assign carries, as StyledRegion defines them.
const (
	colorerStyleBold      = 1
	colorerStyleItalic    = 2
	colorerStyleUnderline = 4
	colorerStyleStrikeout = 8
)

func applyColorerStyle(base uint64, style *colorer.RegionDefine) uint64 {
	attr := base
	if style.IsForeSet {
		attr = vtui.SetRGBFore(attr, style.Fore)
	}
	if style.IsBackSet {
		attr = vtui.SetRGBBack(attr, style.Back)
	}
	if style.Style&colorerStyleBold != 0 {
		attr |= vtui.ForegroundIntensity
	}
	if style.Style&colorerStyleUnderline != 0 {
		attr |= vtui.CommonLvbUnderscore
	}
	if style.Style&colorerStyleStrikeout != 0 {
		attr |= vtui.CommonLvbStrikeout
	}
	return attr
}

// colorerLineRuneCount reports how many indexing units Colorer uses for a
// line, which is also how many attributes the editor needs for it.
//
// Colorer keeps a line in its legacy UnicodeString, one element per code
// point. Its UTF-8 decoder in strings/legacy/CString.cpp writes a whole
// decoded code point into a single wchar_t, 32 bits wide under wasi-sdk, and
// never builds a surrogate pair; strings/legacy/Character.h states that the
// library has no surrogate support at all. Region offsets are therefore rune
// indices. They are not UTF-16 unit indices, which is what this file used to
// assume, and the difference showed as colours sliding one position left
// after every astral character on the line and staying there.
//
// Malformed UTF-8 is the one place the two counts can still drift: Go yields
// one replacement rune per bad byte, while Colorer's decoder swallows the
// continuation bytes of a truncated sequence. See REVIEW.md.
func colorerLineRuneCount(line string) int {
	return utf8.RuneCountInString(line)
}

// colorerRegionRunes fits a region Colorer reported onto the attribute slice
// of a line holding lineRunes runes. The offsets pass through unchanged,
// because they are already rune indices; only the clamping is work. A
// negative end means the region runs to the end of the line, which the caller
// also paints the rest of the row with, so it is reported through toEOL.
func colorerRegionRunes(start, end, lineRunes int) (int, int, bool) {
	toEOL := end < 0
	if toEOL || end > lineRunes {
		end = lineRunes
	}
	if start < 0 {
		start = 0
	}
	if start > lineRunes {
		start = lineRunes
	}
	if end < start {
		end = start
	}
	return start, end, toEOL
}

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

func colorerNeedsRewind(parsedIdx, upTo int) bool {
	return parsedIdx < 0 || parsedIdx > upTo
}

var (
	colorerPoolMu  sync.Mutex
	colorerIdle    *colorer.Session
	colorerIdleDir string
)

func ensureFonokaiSchema(configsDir string) {
	catalogPath := filepath.Join(configsDir, "base", "catalog.xml")
	if _, err := os.Stat(catalogPath); os.IsNotExist(err) {
		return
	}

	hrdDir := filepath.Join(configsDir, "base", "hrd", "rgb")
	hrdPath := filepath.Join(hrdDir, "fonokai.hrd")

	_ = os.MkdirAll(hrdDir, 0755)

	fonokaiHRDContent := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE hrd PUBLIC "-//Cail Lomecb//DTD Colorer HRD take5//EN"
  "http://colorer.sf.net/2003/hrd.dtd">
<hrd xmlns="http://colorer.sf.net/2003/hrd">

  <assign name="def:Text" fore="#dbd3c4" back="#37322c"/>
  <assign name="def:HorzCross" fore="#dbd3c4" back="#2e2a24"/>
  <assign name="def:VertCross" fore="#dbd3c4" back="#2e2a24"/>

  <assign name="def:Number" fore="#e6cf70"/>
  <assign name="def:NumberDec" fore="#e6cf70"/>
  <assign name="def:NumHex" fore="#e6cf70"/>
  <assign name="def:NumberBin" fore="#e6cf70"/>
  <assign name="def:NumberOct" fore="#e6cf70"/>
  <assign name="def:NumberFloat" fore="#e6cf70"/>
  <assign name="def:NumberSuffix" fore="#e6b450"/>

  <assign name="def:String" fore="#ec6a2c"/>
  <assign name="def:StringContent" fore="#ec6a2c"/>
  <assign name="def:StringEdge" fore="#a04020"/>
  <assign name="def:CharacterContent" fore="#ec6a2c"/>

  <assign name="def:Comment" fore="#6a6458"/>
  <assign name="def:CommentContent" fore="#6a6458" style="1"/>
  <assign name="def:CommentDoc" fore="#c4b8a8"/>

  <assign name="def:Symbol" fore="#e6b450"/>
  <assign name="def:SymbolStrong" fore="#e6cf70"/>
  <assign name="def:Prefix" fore="#ec6a2c"/>
  <assign name="def:PrefixStrong" fore="#a04020"/>

  <assign name="def:Operator" fore="#e6b450"/>

  <assign name="def:Keyword" fore="#a04020" style="1"/>
  <assign name="def:KeywordStrong" fore="#a04020"/>
  <assign name="def:ClassKeyword" fore="#e6cf70" style="1"/>
  <assign name="def:TypeKeyword" fore="#e6cf70"/>

  <assign name="def:Function"/>
  <assign name="def:Register" fore="#e6b450"/>
  <assign name="def:Constant" fore="#ec6a2c"/>
  <assign name="def:BooleanConstant" fore="#ec6a2c"/>

  <assign name="def:Var" />
  <assign name="def:VarStrong" fore="#e6cf70"/>
  <assign name="def:Identifier" fore="#dbd3c4"/>

  <assign name="def:Directive" fore="#a04020"/>
  <assign name="def:Param" fore="#e6b450"/>

  <assign name="def:Tag" fore="#a04020"/>
  <assign name="def:OpenTag" fore="#ec6a2c"/>
  <assign name="def:CloseTag" fore="#ec6a2c"/>

  <assign name="def:Label" fore="#e6cf70"/>
  <assign name="def:LabelStrong" fore="#37322c" back="#e6b450"/>

  <assign name="def:Insertion" fore="#dbd3c4" back="#2e2a24"/>
  <assign name="def:InsertionStart" fore="#37322c" back="#e6cf70"/>
  <assign name="def:InsertionEnd" fore="#37322c" back="#e6cf70"/>

  <assign name="def:Error" fore="#eeeeec" back="#c44500"/>
  <assign name="def:ErrorText" fore="#e6cf70"/>
  <assign name="def:TODO" fore="#eeeeec" back="#a04020"/>
  <assign name="def:Debug" fore="#eeeeec" back="#a04020"/>

  <assign name="def:Path" fore="#e6b450"/>
  <assign name="def:URL" fore="#ec6a2c"/>
  <assign name="def:EMail" fore="#ec6a2c"/>

  <assign name="def:Date" fore="#e6cf70"/>
  <assign name="def:Time" fore="#e6cf70"/>

  <assign name="def:PairStart" fore="#37322c" back="#e6b450"/>
  <assign name="def:PairEnd" fore="#37322c" back="#e6b450"/>
</hrd>
`
	_ = os.WriteFile(hrdPath, []byte(fonokaiHRDContent), 0644)

	catalogRGBPath := filepath.Join(configsDir, "base", "hrd", "catalog-rgb.xml")
	data, err := os.ReadFile(catalogRGBPath)
	if err == nil {
		content := string(data)
		if !strings.Contains(content, "name=\"Fonokai\"") {
			entry := "\n        <hrd class=\"rgb\" name=\"Fonokai\" description=\"Fonokai\">\n            <location link=\"&hrd;/rgb/fonokai.hrd\"/>\n        </hrd>\n"
			_ = os.WriteFile(catalogRGBPath, []byte(content+entry), 0644)
		}
	}
}

func acquireColorerSession(configsDir string) (*colorer.Session, error) {
	ensureFonokaiSchema(configsDir)

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
	colorerPoolMu.Lock()
	if colorerIdle == nil {
		colorerIdle = session
		colorerIdleDir = configsDir
		colorerPoolMu.Unlock()
		return
	}
	colorerPoolMu.Unlock()
	go session.Close()
}

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

func ColorerConfigsDir() string {
	if custom := strings.TrimSpace(AppConfig.EditorColorerCatalog); custom != "" {
		return custom
	}
	return filepath.Join(GetF4ConfigDir(), "colorer", "configs")
}

func ResetColorerRegions() {
	// No-op now that region graph scanning is offloaded to C++
}

type ColorerScheme struct {
	Name        string
	Description string
}

func ListColorerSchemes() []ColorerScheme {
	// Fast path: return cached list if already loaded.
	schemesCacheMu.RLock()
	if cachedSchemes != nil {
		defer schemesCacheMu.RUnlock()
		return cachedSchemes
	}
	schemesCacheMu.RUnlock()

	// Slow path: load schemes from disk.
	configsDir := ColorerConfigsDir()
	session, err := acquireColorerSession(configsDir)
	if err != nil {
		return nil
	}
	defer releaseColorerSession(session, configsDir)

	instances, err := session.EnumHRDInstances("rgb")
	if err != nil {
		return nil
	}
	var schemes []ColorerScheme
	for _, inst := range instances {
		schemes = append(schemes, ColorerScheme{
			Name:        inst.Name,
			Description: inst.Description,
		})
	}

	// Store in cache.
	schemesCacheMu.Lock()
	if cachedSchemes == nil {
		cachedSchemes = schemes
	}
	schemesCacheMu.Unlock()

	return cachedSchemes
}

// ResetColorerSchemesCache clears the cached scheme list.
// Useful for tests that need to simulate a fresh environment.
func ResetColorerSchemesCache() {
	schemesCacheMu.Lock()
	cachedSchemes = nil
	schemesCacheMu.Unlock()
}

func colorerSchemeLabel(scheme ColorerScheme) string {
	if label := strings.TrimSpace(scheme.Description); label != "" {
		return label
	}
	return scheme.Name
}

var (
	schemeMu         sync.Mutex
	schemeName       string
	schemeGeneration uint64

	// cachedSchemes holds the list of Colorer schemes loaded from disk.
	// It is populated lazily on the first call to ListColorerSchemes().
	cachedSchemes  []ColorerScheme
	schemesCacheMu sync.RWMutex
)

func SetColorerScheme(name string) {
	schemeMu.Lock()
	unchanged := name == schemeName
	schemeMu.Unlock()
	if unchanged {
		return
	}

	schemeMu.Lock()
	schemeName = name
	schemeGeneration++
	schemeMu.Unlock()

	vtui.DebugLog("COLORER: Color style %q activated", name)
}

func ResetColorerScheme() {
	schemeMu.Lock()
	schemeName = ""
	schemeGeneration++
	schemeMu.Unlock()
}

func ColorerSchemeGeneration() uint64 {
	schemeMu.Lock()
	defer schemeMu.Unlock()
	return schemeGeneration
}

const colorerBackgroundRegion = "def:Text"

// We need a helper to get region definition globally from the active scheme.
func colorerGetRegionDefine(region string) *colorer.RegionDefine {
	configsDir := ColorerConfigsDir()
	session, err := acquireColorerSession(configsDir)
	if err != nil {
		return nil
	}
	defer releaseColorerSession(session, configsDir)

	schemeMu.Lock()
	activeScheme := schemeName
	schemeMu.Unlock()

	if activeScheme == "" {
		activeScheme = "default"
	}
	_ = session.SetHRD("rgb", activeScheme)

	rd, _ := session.GetRegionDefine(region)
	return rd
}

func ColorerEditorBaseAttr(base uint64) uint64 {
	if !AppConfig.EditorColorerBackground {
		return base
	}
	if !strings.EqualFold(AppConfig.EditorHighlighter, "Colorer") {
		return base
	}

	rd := colorerGetRegionDefine(colorerBackgroundRegion)
	if rd == nil {
		return base
	}

	attr := base
	if rd.IsForeSet {
		attr = vtui.SetRGBFore(attr, rd.Fore)
	}
	if rd.IsBackSet {
		attr = vtui.SetRGBBack(attr, rd.Back)
	}
	return attr
}

func newColorerHighlighter(ev *EditorView, filename, firstLine string, fallback vtui.Highlighter) *ColorerHighlighter {
	SetColorerScheme(AppConfig.EditorColorerScheme)

	ch := &ColorerHighlighter{
		fallback:   fallback,
		filename:   filename,
		firstLine:  firstLine,
		starting:   true,
		configsDir: ColorerConfigsDir(),
	}

	go func() {
		session, err := acquireColorerSession(ch.configsDir)
		if err != nil {
			vtui.DebugLog("COLORER: Failed to init session: %v", err)
			ch.useFallback(ev)
			return
		}

		schemeMu.Lock()
		activeScheme := schemeName
		schemeMu.Unlock()

		if activeScheme == "" {
			activeScheme = "default"
		}
		session.SetHRD("rgb", activeScheme)

		selected, sErr := session.SelectType(filename, firstLine)
		vtui.DebugLog("COLORER: SelectType(%q, len=%d) -> selected=%v, err=%v", filename, len(firstLine), selected, sErr)
		if sErr != nil || !selected {
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
				releaseColorerSession(session, ch.configsDir)
				return
			}
			if closer, ok := ch.fallback.(io.Closer); ok {
				closer.Close()
			}
			ch.fallback = nil
			ch.session = session
			ch.starting = false
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
	bgCache    map[int]uint64
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
			return nil, nil
		}
		if ch.fallback != nil {
			return ch.fallback.Highlight(line, prevState, baseAttr)
		}
		return nil, nil
	}

	logIdx := colorerLineIndex(prevState, len(ch.lines))

	gen := ColorerSchemeGeneration()
	if !ch.baseKnown || ch.baseAttr != baseAttr || ch.schemeGen != gen {
		ch.baseAttr = baseAttr
		ch.baseKnown = true
		ch.schemeGen = gen
		ch.attrCache = nil
		ch.bgCache = nil

		schemeMu.Lock()
		activeScheme := schemeName
		schemeMu.Unlock()

		if activeScheme == "" {
			activeScheme = "default"
		}
		ch.session.SetHRD("rgb", activeScheme)
	}

	if logIdx < len(ch.lines) && ch.lines[logIdx] == line {
		if attrs, ok := ch.attrCache[logIdx]; ok {
			return attrs, logIdx
		}
	} else {
		if logIdx < len(ch.lines) {
			ch.lines = ch.lines[:logIdx]
			ch.dropCacheFrom(logIdx)
		}
		ch.lines = append(ch.lines, line)
	}

	if ch.parsedIdx != logIdx {
		ch.resync(logIdx)
	}

	regions, err := ch.session.ParseLine(line)
	ch.parsedIdx = logIdx + 1
	if err != nil {
		vtui.DebugLog("COLORER: ParseLine failed at line %d: %v", logIdx, err)
		return nil, logIdx
	}

	lineRunes := colorerLineRuneCount(line)
	attrs := make([]uint64, lineRunes)
	for i := range attrs {
		attrs[i] = baseAttr
	}

	eolBg := baseAttr
	for _, reg := range regions {
		if !AppConfig.EditorColorerSyntax {
			continue
		}

		start, end, toEOL := colorerRegionRunes(reg.Start, reg.End, lineRunes)
		rd := colorer.RegionDefine{
			Fore:      reg.Fore,
			Back:      reg.Back,
			Style:     reg.Style,
			IsForeSet: reg.IsForeSet,
			IsBackSet: reg.IsBackSet,
		}

		if toEOL {
			eolBg = applyColorerStyle(eolBg, &rd)
		}
		for i := start; i < end; i++ {
			attrs[i] = applyColorerStyle(attrs[i], &rd)
		}
	}

	ch.storeAttrs(logIdx, attrs, eolBg)
	return attrs, logIdx
}

func (ch *ColorerHighlighter) resync(upTo int) {
	if upTo > len(ch.lines) {
		upTo = len(ch.lines)
	}
	if upTo < 0 {
		upTo = 0
	}

	from := ch.parsedIdx
	if colorerNeedsRewind(from, upTo) {
		ch.session.Reset()
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

func (ch *ColorerHighlighter) GetLineBackground(idx int, defaultAttr uint64) uint64 {
	if ch.bgCache == nil {
		return defaultAttr
	}
	if bg, ok := ch.bgCache[idx]; ok {
		return bg
	}
	return defaultAttr
}

func (ch *ColorerHighlighter) storeAttrs(idx int, attrs []uint64, bg uint64) {
	if ch.attrCache == nil {
		ch.attrCache = make(map[int][]uint64)
	}
	if ch.bgCache == nil {
		ch.bgCache = make(map[int]uint64)
	}
	if len(ch.attrCache) >= maxCachedAttrLines {
		for key := range ch.attrCache {
			if key < idx-attrCacheKeepWindow || key > idx+attrCacheKeepWindow {
				delete(ch.attrCache, key)
				delete(ch.bgCache, key)
			}
		}
		if len(ch.attrCache) >= maxCachedAttrLines {
			ch.attrCache = make(map[int][]uint64)
			ch.bgCache = make(map[int]uint64)
		}
	}
	ch.attrCache[idx] = attrs
	ch.bgCache[idx] = bg
}

func (ch *ColorerHighlighter) dropCacheFrom(idx int) {
	for key := range ch.attrCache {
		if key >= idx {
			delete(ch.attrCache, key)
		}
	}
	for key := range ch.bgCache {
		if key >= idx {
			delete(ch.bgCache, key)
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
