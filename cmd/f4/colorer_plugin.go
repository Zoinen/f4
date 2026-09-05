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
	embedded "github.com/unxed/f4"
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

	// How many uncoloured lines one worker job may take on, counting the
	// line that missed the cache. One job per line meant one full UI round
	// trip — worker, PostTask, whole-screen redraw — per line, and the
	// viewport visibly filled with colour line by line. A batch colours a
	// screen in a single round trip; the cap only has to exceed any
	// realistic terminal height.
	hlColorerBatchLines = 200

	// How much line text one batch snapshot may copy on the UI render
	// thread. Lines are cut at 64 KB each, so a line count alone bounds the
	// snapshot at ~12.8 MB — fine as a parse budget, not as a synchronous
	// copy inside a frame. Ordinary code is a few dozen bytes per line and
	// never notices this; only files with enormous lines trade batch depth
	// for a bounded frame.
	hlColorerBatchBytes = 256 * 1024
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

func ensureRadiolaSchema(configsDir string) {
	catalogPath := filepath.Join(configsDir, "base", "catalog.xml")
	if _, err := os.Stat(catalogPath); os.IsNotExist(err) {
		return
	}

	hrdDir := filepath.Join(configsDir, "base", "hrd", "rgb")
	hrdPath := filepath.Join(hrdDir, "radiola.hrd")

	_ = os.MkdirAll(hrdDir, 0700)
	_ = os.WriteFile(hrdPath, []byte(embedded.RadiolaHRD), 0600)
	_ = os.Chmod(hrdPath, 0600)

	catalogRGBPath := filepath.Join(configsDir, "base", "hrd", "catalog-rgb.xml")
	data, err := os.ReadFile(catalogRGBPath)
	if err == nil {
		content := string(data)
		if !strings.Contains(content, "name=\"Radiola\"") {
			entry := "\n        <hrd class=\"rgb\" name=\"Radiola\" description=\"Radiola\">\n            <location link=\"&hrd;/rgb/radiola.hrd\"/>\n        </hrd>\n"
			// #nosec G703 -- configsDir is the user-selected Colorer catalog root and every appended component is fixed here.
			_ = os.WriteFile(catalogRGBPath, []byte(content+entry), 0600)
			_ = os.Chmod(catalogRGBPath, 0600)
		}
	}
}

func acquireColorerSession(configsDir string) (*colorer.Session, error) {
	ensureRadiolaSchema(configsDir)

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

// A pooled Session owns the context it was created with. Colorer parsing gets
// a private context instead, so Esc can interrupt an in-flight ParseLine.
func acquireCancelableColorerSession(ctx context.Context, configsDir string) (*colorer.Session, error) {
	ensureRadiolaSchema(configsDir)

	catalogPath := "/base/catalog.xml"
	vtui.DebugLog("COLORER: Initializing cancellable session with catalog %q, configs %q", catalogPath, configsDir)
	return colorer.NewSession(ctx, catalogPath, configsDir)
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

var colorerBackgroundCache struct {
	sync.Mutex
	generation uint64
	configsDir string
	ready      bool
	loading    bool
	define     *colorer.RegionDefine
}

// We need a helper to get region definition globally from the active scheme.
func colorerGetRegionDefine(region string) *colorer.RegionDefine {
	if cached := cachedColorerRegionDefine(region); cached != nil {
		return cached
	}
	configsDir := ColorerConfigsDir()
	schemeMu.Lock()
	activeScheme := schemeName
	schemeMu.Unlock()
	return colorerGetRegionDefineFor(region, configsDir, activeScheme)
}

func colorerGetRegionDefineFor(region, configsDir, activeScheme string) *colorer.RegionDefine {
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

	sessionCtx, sessionCancel := context.WithCancel(context.Background())
	ch := &ColorerHighlighter{
		fallback:      fallback,
		filename:      filename,
		firstLine:     firstLine,
		starting:      true,
		configsDir:    ColorerConfigsDir(),
		owner:         ev,
		sessionCtx:    sessionCtx,
		sessionCancel: sessionCancel,
	}
	if ev != nil {
		ch.SetLineSource(ev.lineTextForHighlight)
	}

	// Read on the goroutine that starts this work, not inside it: the
	// work outlives the call, and reading the global from it races
	// anything that reassigns vtui.FrameManager meanwhile.
	frames := vtui.FrameManager
	ch.postTask = frames.PostTask
	ch.redraw = frames.Redraw
	go func() {
		session, err := acquireCancelableColorerSession(sessionCtx, ch.configsDir)
		if err != nil {
			vtui.DebugLog("COLORER: Failed to init session: %v", err)
			if !ch.closed {
				ch.useFallback(ev)
			}
			return
		}

		activeScheme, generation := activeColorerScheme()
		session.SetHRD("rgb", activeScheme)
		cacheColorerRenderRegions(session, generation)

		selected, sErr := session.SelectType(filename, firstLine)
		vtui.DebugLog("COLORER: SelectType(%q, len=%d) -> selected=%v, err=%v", filename, len(firstLine), selected, sErr)
		if sErr != nil || !selected {
			session.Close()
			if !ch.closed {
				ch.useFallback(ev)
			}
			return
		}

		frames.PostTask(func() {
			if ch.closed || ch.disabled {
				session.Close()
				return
			}
			if ev != nil && ev.highlighter != vtui.Highlighter(ch) {
				session.Close()
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
			ch.startWorker(session)
			if ev != nil {
				ev.finishColorerWork(ch.startupWorkID)
				ev.invalidateStates(0)
			}
			frames.Redraw()
		})
	}()

	return ch
}

func (ch *ColorerHighlighter) useFallback(ev *EditorView) {
	vtui.FrameManager.PostTask(func() {
		if !ch.starting || ch.closed || ch.disabled {
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
			ev.finishColorerWork(ch.startupWorkID)
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
	parsedIdx  int
	filename   string
	firstLine  string
	configsDir string
	closed     bool
	starting   bool
	owner      *EditorView
	postTask   func(func())
	redraw     func()

	// The session and its worker share this context. The worker is the only
	// goroutine allowed to call ParseLine/Reset/SelectType on the live session;
	// the UI only queues immutable line snapshots and consumes results.
	sessionCtx     context.Context
	sessionCancel  context.CancelFunc
	workerJobs     chan colorerJob
	workerDone     chan struct{}
	pending        bool
	disabled       bool
	forceReset     bool
	workGeneration uint64
	startupWorkID  uint64

	// lineAt hands over the text of any logical line, so re-anchoring can
	// read the context it needs straight from the document instead of the
	// highlighter keeping a second copy of the file.
	lineAt func(idx int) (string, bool)
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

func (ch *ColorerHighlighter) attrsForSyntax(line string, regions []colorer.Region, baseAttr uint64, syntax bool) ([]uint64, uint64) {
	lineRunes := colorerLineRuneCount(line)
	attrs := make([]uint64, lineRunes)
	for i := range attrs {
		attrs[i] = baseAttr
	}

	eolBg := baseAttr
	for _, reg := range regions {
		if !syntax {
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
	if ch.session == nil || idx < 0 || ch.closed || ch.disabled {
		return nil
	}

	if attrs, ok := ch.attrCache[idx]; ok {
		return attrs
	}

	// ParseLine can execute arbitrary Colorer grammar code and is therefore
	// never allowed to run from DisplayObject's UI render path. queueLine makes
	// a bounded snapshot of the required context and the worker does all WASM
	// calls asynchronously.
	ch.queueLine(idx, line, baseAttr)
	return nil
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

// DropFrom forgets everything the highlighter knows from line idx on. An edit
// invalidates the colours below it, and it invalidates the session itself
// whenever the session has already parsed past that line: its cache cannot be
// unwound, only thrown away.
func (ch *ColorerHighlighter) DropFrom(idx int) {
	if idx < 0 {
		idx = 0
	}
	ch.dropCacheFrom(idx)
	// The worker may currently be inside ParseLine. Do not touch its session
	// from the UI; invalidate that result and let the next frame enqueue a
	// fresh anchored snapshot.
	ch.forceReset = true
	ch.workGeneration++
	ch.parsedIdx = 0
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
	ch.disabled = true
	if ch.sessionCancel != nil {
		ch.sessionCancel()
	}
	ch.stopWorker()
	ch.attrCache = nil
	ch.parsedIdx = 0
	if closer, ok := ch.fallback.(io.Closer); ok {
		closer.Close()
	}
	ch.fallback = nil
	// Worker-owned sessions are closed by runWorker. A highlighter created by
	// tests or a caller without a worker still uses the old pooled lifecycle.
	if ch.session != nil && ch.workerDone == nil {
		releaseColorerSession(ch.session, ch.configsDir)
	}
	ch.session = nil
	return nil
}
