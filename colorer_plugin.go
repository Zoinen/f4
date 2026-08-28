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
	// maxCachedAttrLines and attrCacheKeepWindow bound how many lines of
	// already-computed colours storeAttrs keeps around. Each entry is a
	// []uint64 the width of its line, and nothing ever evicted it before
	// this: scrolling through a large file accumulated colours for every
	// line ever drawn, hundreds of megabytes on the reference file. See
	// HIGHLIGHT.md, item 3.
	//
	// The window only needs to be larger than a screen by a comfortable
	// margin — a line evicted and then scrolled back to is just a cache
	// miss, and a miss above the parse position costs a re-anchor, which is
	// the designed fallback, not a bug.
	maxCachedAttrLines  = 20000
	attrCacheKeepWindow = 5000

	// How far the session may be fed forward to reach a line. Beyond this
	// the anchor is thrown away and rebuilt, which costs a fixed
	// hlColorerContext lines instead of the whole distance.
	hlColorerForward = 2000
	// How many lines above a viewport are parsed for context when the
	// session is re-anchored. Everything a construct opened further up
	// would have told us is lost; this is the price of a parser whose
	// state cannot be snapshotted. See HIGHLIGHT.md, phase 5.
	hlColorerContext = 300

	// How many lines behind the parse position stay in the wasm session
	// once forgetBehind starts trimming it. Kept equal to hlColorerContext:
	// that is already the margin the rest of this file trusts for a replay,
	// so forgetting does not introduce a second, untested number.
	hlColorerKeepBehind = hlColorerContext

	// How often forgetBehind actually calls into wasm, in lines fed. Calling
	// it on every ParseLine would double the wasm calls a forward scroll
	// makes; batching keeps the cost negligible while still bounding memory
	// well under file size. See HIGHLIGHT.md, item 9 and item 4's log.
	hlColorerForgetEvery = 1000
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

var (
	colorerPoolMu  sync.Mutex
	colorerIdle    *colorer.Session
	colorerIdleDir string

	colorerRegionCacheMu sync.RWMutex
	colorerRegionCache   = make(map[string]colorerRegionCacheEntry)
)

type colorerRegionCacheEntry struct {
	generation uint64
	define     colorer.RegionDefine
	found      bool
}

func cacheColorerRegion(session *colorer.Session, region string,
	generation uint64,
) {
	if session == nil || region == "" {
		return
	}
	rd, err := session.GetRegionDefine(region)
	entry := colorerRegionCacheEntry{generation: generation}
	if err == nil && rd != nil {
		entry.define = *rd
		entry.found = true
	}
	colorerRegionCacheMu.Lock()
	colorerRegionCache[region] = entry
	colorerRegionCacheMu.Unlock()
}

func cacheColorerRenderRegions(session *colorer.Session, generation uint64) {
	cacheColorerRegion(session, colorerBackgroundRegion, generation)
	cacheColorerRegion(session, colorerHorzCrossRegion, generation)
	cacheColorerRegion(session, colorerVertCrossRegion, generation)
}

func cachedColorerRegionDefine(region string) *colorer.RegionDefine {
	generation := ColorerSchemeGeneration()
	colorerRegionCacheMu.RLock()
	entry, present := colorerRegionCache[region]
	colorerRegionCacheMu.RUnlock()
	if !present || entry.generation != generation || !entry.found {
		return nil
	}
	result := entry.define
	return &result
}

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
  <assign name="def:StringEdge" fore="#ff5544"/>
  <assign name="def:CharacterContent" fore="#ec6a2c"/>

  <assign name="def:Comment" fore="#6a6458"/>
  <assign name="def:CommentContent" fore="#6a6458" style="1"/>
  <assign name="def:CommentDoc" fore="#c4b8a8"/>

  <assign name="def:Symbol" fore="#e6b450"/>
  <assign name="def:SymbolStrong" fore="#e6cf70"/>
  <assign name="def:Prefix" fore="#ec6a2c"/>
  <assign name="def:PrefixStrong" fore="#ff5544"/>

  <assign name="def:Operator" fore="#e6b450"/>

  <assign name="def:Keyword" fore="#ff5544" style="1"/>
  <assign name="def:KeywordStrong" fore="#ff5544"/>
  <assign name="def:ClassKeyword" fore="#e6cf70" style="1"/>
  <assign name="def:TypeKeyword" fore="#e6cf70"/>

  <assign name="def:Function"/>
  <assign name="def:Register" fore="#e6b450"/>
  <assign name="def:Constant" fore="#ec6a2c"/>
  <assign name="def:BooleanConstant" fore="#ec6a2c"/>

  <assign name="def:Var" />
  <assign name="def:VarStrong" fore="#e6cf70"/>
  <assign name="def:Identifier" fore="#dbd3c4"/>

  <assign name="def:Directive" fore="#ff5544"/>
  <assign name="def:Param" fore="#e6b450"/>

  <assign name="def:Tag" fore="#ff5544"/>
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

func activeColorerScheme() (string, uint64) {
	schemeMu.Lock()
	name, generation := schemeName, schemeGeneration
	schemeMu.Unlock()
	if name == "" {
		name = "default"
	}
	return name, generation
}

const colorerBackgroundRegion = "def:Text"

// We need a helper to get region definition globally from the active scheme.
func colorerGetRegionDefine(region string) *colorer.RegionDefine {
	if cached := cachedColorerRegionDefine(region); cached != nil {
		return cached
	}
	configsDir := ColorerConfigsDir()
	session, err := acquireColorerSession(configsDir)
	if err != nil {
		return nil
	}
	defer releaseColorerSession(session, configsDir)

	activeScheme, generation := activeColorerScheme()
	_ = session.SetHRD("rgb", activeScheme)

	rd, _ := session.GetRegionDefine(region)
	entry := colorerRegionCacheEntry{generation: generation}
	if rd != nil {
		entry.define = *rd
		entry.found = true
	}
	colorerRegionCacheMu.Lock()
	colorerRegionCache[region] = entry
	colorerRegionCacheMu.Unlock()
	return rd
}

func ColorerEditorBaseAttr(base uint64) uint64 {
	if !AppConfig.EditorColorerBackground {
		return base
	}
	if !strings.EqualFold(AppConfig.EditorHighlighter, "Colorer") {
		return base
	}

	// A render must never initialize a second WASM session synchronously while
	// the editor's own Colorer session is starting. The first frame uses the f4
	// palette; the background session fills this cache and requests a redraw.
	rd := cachedColorerRegionDefine(colorerBackgroundRegion)
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
	if ev != nil {
		ch.SetLineSource(ev.lineTextForHighlight)
	}

	go func() {
		session, err := acquireColorerSession(ch.configsDir)
		if err != nil {
			vtui.DebugLog("COLORER: Failed to init session: %v", err)
			ch.useFallback(ev)
			return
		}

		activeScheme, generation := activeColorerScheme()
		session.SetHRD("rgb", activeScheme)
		cacheColorerRenderRegions(session, generation)

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
			// With no session of its own this object has nothing left to
			// add, and while it sits in ev.highlighter the editor treats
			// the engine behind it as if it were Colorer: no state chain,
			// no walker. Hand the engine over instead.
			if fb := ch.fallback; fb != nil && ev.highlighter == vtui.Highlighter(ch) {
				ch.fallback = nil
				ev.highlighter = fb
			}
			ev.invalidateStates(0)
		}
		vtui.FrameManager.Redraw()
	})
}

type ColorerHighlighter struct {
	session    *colorer.Session
	fallback   vtui.Highlighter
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

	// lineAt hands over the text of any logical line, so re-anchoring can
	// read the context it needs straight from the document instead of the
	// highlighter keeping a second copy of the file.
	lineAt func(idx int) (string, bool)
	// anchor is the first line the session has seen since the last reset.
	// Lines below it are not in its cache, and it cannot be walked back to
	// them: parsedIdx - anchor is the session's own line number, and the
	// two must never drift apart.
	anchor int

	// forgottenUpTo is the last line forgetBehind released the session's
	// hold on. Lines below it may already be gone from the wasm heap; lines
	// at or above it are still there whether or not forgetBehind has ever
	// run, which is why it starts at the same value as anchor.
	forgottenUpTo int
	// forgetDisabled is set once colorer_forget_before turns out not to be
	// exported by the embedded wasm (an older colorer4go). Forgetting is
	// an optimization, not a correctness requirement, so the highlighter
	// falls back to the old behaviour — a reset stays the only thing that
	// releases memory — instead of logging the same failure forever.
	forgetDisabled bool
}

// SetLineSource gives the highlighter a way to read the document.
func (ch *ColorerHighlighter) SetLineSource(fn func(idx int) (string, bool)) {
	ch.lineAt = fn
}

// Highlight is the vtui.Highlighter entry point. It is still needed while
// the session is loading or has been given up on; the editor draws Colorer
// with a live session through HighlightLine instead, so this delegates
// nothing in that case. See HIGHLIGHT.md, phase 5c and item 2.
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
	return nil, nil
}

// syncScheme picks up a change of colour scheme or of the editor's base
// attribute. Only the colours are affected, never the parse position.
func (ch *ColorerHighlighter) syncScheme(baseAttr uint64) {
	activeScheme, gen := activeColorerScheme()
	if ch.baseKnown && ch.baseAttr == baseAttr && ch.schemeGen == gen {
		return
	}
	ch.baseAttr = baseAttr
	ch.baseKnown = true
	ch.schemeGen = gen
	ch.attrCache = nil
	ch.bgCache = nil

	ch.session.SetHRD("rgb", activeScheme)
	cacheColorerRenderRegions(ch.session, gen)
}

// attrsFor turns the regions of one parsed line into cell attributes, and
// reports the background the line carries past its last character.
func (ch *ColorerHighlighter) attrsFor(line string, regions []colorer.Region, baseAttr uint64) ([]uint64, uint64) {
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
	return attrs, eolBg
}

// HighlightLine colours one line addressed by its real number in the document.
//
// This is the path the editor draws through. It replaces the state chain that
// a Colorer session cannot provide: instead of carrying a state from line to
// line, the session is parked next to the viewport and fed forward from there.
func (ch *ColorerHighlighter) HighlightLine(idx int, line string, baseAttr uint64) []uint64 {
	if ch.session == nil || idx < 0 {
		return nil
	}

	ch.syncScheme(baseAttr)
	if attrs, ok := ch.attrCache[idx]; ok {
		return attrs
	}

	ch.ensureContext(idx)
	if ch.parsedIdx != idx {
		// The context could not be read — the line index is still growing,
		// or the piece table is still loading. Plain for this frame.
		return nil
	}

	regions, err := ch.session.ParseLine(line)
	ch.parsedIdx = idx + 1
	if err != nil {
		vtui.DebugLog("COLORER: ParseLine failed at line %d: %v", idx, err)
		return nil
	}

	attrs, eolBg := ch.attrsFor(line, regions, baseAttr)
	ch.storeAttrs(idx, attrs, eolBg)
	ch.forgetBehind()
	return attrs
}

// colorerContextPlan decides how the session gets to idx: fed forward from
// where it stands, or reset and restarted from a fixed run of context lines.
//
// Feeding forward is what sequential scrolling gets, and it keeps every
// construct opened above intact. A jump, or any move backwards — the session
// cannot be rewound — costs the same whether it is over a hundred lines or
// half a million, which is the whole point of the anchor.
func colorerContextPlan(parsedIdx, idx int) (start int, reset bool) {
	if parsedIdx <= idx && idx-parsedIdx <= hlColorerForward {
		return parsedIdx, false
	}
	start = idx - hlColorerContext
	if start < 0 {
		start = 0
	}
	return start, true
}

// colorerForgetPlan decides whether forgetBehind has enough new lines behind
// the parse position to be worth a wasm call, and where the cut should land.
// Pure, like colorerContextPlan, so the batching threshold is tested without
// a session.
func colorerForgetPlan(parsedIdx, forgottenUpTo int) (keepFrom int, do bool) {
	if parsedIdx-forgottenUpTo < hlColorerForgetEvery {
		return 0, false
	}
	keepFrom = parsedIdx - hlColorerKeepBehind
	if keepFrom <= forgottenUpTo {
		return 0, false
	}
	return keepFrom, true
}

// forgetBehind releases the session's hold on lines that are behind the parse
// position by more than hlColorerKeepBehind, batched so a long forward scroll
// pays for it roughly once every hlColorerForgetEvery lines instead of on
// every ParseLine call.
//
// This is what keeps holding PgDn from filling the wasm heap with the whole
// file now that a reset is not the only way to release it (colorer4go
// v0.1.12, colorer_forget_before / HIGHLIGHT.md item 9). Safe for the same
// reason ForgetBefore documents on its own side: the parser only ever reads
// forward from ch.parsedIdx, so nothing below hlColorerKeepBehind lines back
// from there will be asked for again before the next reset.
func (ch *ColorerHighlighter) forgetBehind() {
	if ch.session == nil || ch.forgetDisabled {
		return
	}
	keepFrom, do := colorerForgetPlan(ch.parsedIdx, ch.forgottenUpTo)
	if !do {
		return
	}
	if err := ch.session.ForgetBefore(keepFrom); err != nil {
		vtui.DebugLog("COLORER: ForgetBefore unsupported, disabling for this session: %v", err)
		ch.forgetDisabled = true
		return
	}
	ch.forgottenUpTo = keepFrom
}

func (ch *ColorerHighlighter) ensureContext(idx int) {
	if ch.session == nil || ch.lineAt == nil || ch.parsedIdx == idx {
		return
	}
	if start, reset := colorerContextPlan(ch.parsedIdx, idx); reset {
		ch.resetSessionAt(start)
	}
	// Falling short here means the document could not be read that far —
	// the line index is still growing, or the piece table is still loading.
	// Re-anchoring would hit the same wall, so leave it for the next frame.
	ch.parseThrough(idx)
	ch.forgetBehind()
}

// parseThrough feeds the session every line up to, but not including, idx.
func (ch *ColorerHighlighter) parseThrough(idx int) {
	for ch.parsedIdx < idx {
		line, ok := ch.lineAt(ch.parsedIdx)
		if !ok {
			return
		}
		if _, err := ch.session.ParseLine(line); err != nil {
			return
		}
		ch.parsedIdx++
	}
}

// resetSessionAt empties the session and declares start to be its first line.
// The reset is what releases the lines and the parse cache the session has
// accumulated, so it is also how this highlighter stays bounded on a large
// file.
func (ch *ColorerHighlighter) resetSessionAt(start int) {
	ch.session.Reset()

	firstLine := ch.firstLine
	if ch.lineAt != nil {
		if line, ok := ch.lineAt(0); ok {
			firstLine = line
		}
	}
	_, _ = ch.session.SelectType(ch.filename, firstLine)

	ch.anchor = start
	ch.parsedIdx = start
	ch.forgottenUpTo = start
}

// DropFrom forgets everything the highlighter knows from line idx on. An edit
// invalidates the colours below it, and it invalidates the session itself
// whenever the session has already parsed past that line: its cache cannot be
// unwound, only thrown away.
func (ch *ColorerHighlighter) DropFrom(idx int) {
	if idx < 0 {
		idx = 0
	}
	ch.dropCacheFrom(idx)
	if ch.session != nil && ch.parsedIdx > idx {
		ch.resetSessionAt(0)
	}
}

func (ch *ColorerHighlighter) GetLineBackground(idx int, defaultAttr uint64) uint64 {
	if ch.bgCache == nil {
		return defaultAttr
	}
	if idx < ch.parsedIdx-100 {
		if bg, ok := ch.bgCache[idx]; ok {
			return bg
		}
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
			// Do not evict lines near top of document (0..2000) so Ctrl+Home always retains instant cached colors
			if key > 2000 && (key < idx-attrCacheKeepWindow || key > idx+attrCacheKeepWindow) {
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
