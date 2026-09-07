# Editor syntax highlighting on large files

Design document and work queue. Rewritten from scratch on 2026-08-13; it
replaces the earlier version, whose numbering had drifted across several
attempts. Everything below describes the code as it stands now.

Origin: issue #458, "Тормоза в редакторе". Reference file for every
measurement in this document: `objdump -d install/far2l > far2l.s`, about 40 MB
and 600 000 lines, opened with F4.

---

## 1. How the editor draws text

Four things cooperate, and all four run on the UI thread.

**Piece table** (`piecetable/piecetable.go`) holds the text. For an unedited
file it is one piece over the loading buffer, so `GetRange` is cheap. It can
return `piecetable.ErrLoading` when the data has not arrived yet.

**Line index** (`piecetable/lineindex.go`) maps line numbers to byte offsets.
It is built in the background by `EditorView.StartIndexing`
(`editor_view.go`), which scans the file in 64 KB chunks off-thread and
publishes batches of offsets through `vtui.FrameManager.PostTask`. Until it
finishes, `li.LineCount()` keeps growing. `ev.indexing` says whether it is
still running. The scroll bar, `Ctrl+End` and any restore of a saved position
all wait for it.

**Wrap engine** (`textlayout/wrap.go`) turns logical lines into visual rows.
Word wrap is off by default, in which case its row bookkeeping is a trivial
`rowOffsets[i] = i` loop. It is not involved in any problem described here.

**Render loop** (`EditorView.DisplayObject`, `editor_view.go`) walks the
logical lines from the top of the viewport, asks the highlighter for the
attributes of each one, and paints. Everything it calls is synchronous: time
spent there is a frame the user waits for.

### Two kinds of highlighter

They share the `vtui.Highlighter` interface

    Highlight(line string, prevState any, baseAttr uint64) ([]uint64, any)

but they are not the same kind of object, and the whole design turns on the
difference.

**Chroma-style — state is a value.** `Highlight` returns a state that fully
describes where the lexer stands. Feed it back for the next line and the
result is correct; keep it in a slice and any line can be resumed later. The
editor stores these in `ev.lineStates`, dense from line 0, where
`lineStates[i]` is the state *after* line `i`.

**Colorer — state is a position.** `ColorerHighlighter` (`colorer_plugin.go`)
drives a wasm session from `github.com/unxed/colorer4go`. The value it returns
is a line number, nothing more. The real state is a parse cache inside the
session, and the C++ wrapper (`colorer_wrapper.cpp`) only ever appends:
`colorer_parse_line` pushes the line into `line_source.lines` and parses with
`TPM_CACHE_UPDATE` at `lno = lines.size()`. Consequences, all load-bearing:

- the session can only be fed **forward**, one line at a time, in order;
- going backwards is impossible; the only way back is
  `colorer_reset_session`, which clears the line vector and the parse cache
  (and keeps the selected file type — verified against v_2.8.0 configs with
  colorer4go v0.1.9);
- everything ever fed to the session stays in wasm memory until that reset.
  A full pass over the reference file puts all 40 MB into the wasm heap as
  UTF-16, plus the parse cache;
- there is no snapshot. The state cannot be saved, copied or restored.

---

## 2. What was wrong, and where it stands

`Ctrl+End` on the reference file froze the editor for 30 to 40 seconds. The
render loop built `ev.lineStates` from line 0 up to the first visible line,
inside the draw path, because a chain has no other way to reach line 600 000.

Fixed, in order:

1. **Never highlight from the draw path.** A gap larger than
   `syncHighlightGapLimit` (50 lines) is not caught up synchronously any more.
   Chroma-style highlighters get one stateless call so the jump lands on
   coloured text immediately; the chain catches up behind it.
2. **A throttled walker.** `startHighlighting` runs slices of at most
   `hlSliceBudget` (4 ms) of UI time each, spaced by a duty cycle
   (`highlightDuty`, `highlightIdleGap`), yielding to the line indexer while it
   is still building. Slices are scheduled through `PostTask` and their
   decisions are made inside the slice, on the thread that owns the fields.
3. **Colorer out of the walker** (`usesStateChain`). See section 3.
4. **Colorer drawn from an anchor** (`HighlightLine`). See section 3.

Reported after (4): the tail of the file appears and colours immediately, with
both highlighters. One problem left from the user's point of view, and it is
item 1 of the queue: **after opening the file the editor shows an empty window
for several seconds** before anything appears.

That one is not about highlighting at all. `DisplayObject` paints a blank
rectangle and returns while `ev.targetLine != -1`, which is the state of an
editor whose saved cursor position has not been reached by the line indexer
yet. On a 40 MB file the indexer needs seconds to get there, and the reasoning
behind the blank — that painting the top of the file and then jumping looks
like a flicker — trades a flicker for several seconds of nothing.

---

## 3. Design decisions

### 3.1 Colorer keeps no state chain

`usesStateChain(h)` in `editor_view.go` returns false for
`*ColorerHighlighter`, and both `startHighlighting` and `highlightSlice` refuse
to run for it.

*Why.* Walking the file for Colorer builds nothing — the chain would hold
consecutive integers. It is also actively harmful: the walk feeds every line
to the session, so the wasm heap fills with the whole file, and it leaves the
parse position ahead of the viewport, so each frame has to rewind it. Before
this change, the render path and the walker took turns dragging the session in
opposite directions; the discarded work is what made `Esc` arrive fifteen
seconds after a held `PgDn`.

### 3.2 Colorer is addressed by line number, from an anchor

`ColorerHighlighter.HighlightLine(idx, line, baseAttr)` is what the render loop
calls. A cache hit returns the stored attributes; a miss queues work and
returns nil — the line stays plain until the result lands. Internally
(`colorer_async.go`):

- `colorerContextPlan(parsedIdx, idx)` — pure, and the whole decision. If the
  wanted line is ahead of the session and no further than `hlColorerForward`
  (2000) lines, feed the session forward. Otherwise reset and restart
  `hlColorerContext` (300) lines above the target.
- `queueLine` runs on the UI thread but never calls into Colorer. It snapshots
  the context lines and the whole uncoloured run starting at `idx` — bounded
  by `hlColorerBatchLines` and `hlColorerBatchBytes`, stopping at the first
  line already coloured or not yet loaded — into one immutable `colorerJob`.
  One job per screen, not per line: the per-line version coloured the viewport
  visibly line by line, one worker round trip and one full redraw each.
- A single worker goroutine owns the session (`runWorker`). It replays the
  context, parses the batch, and posts all the attributes back in one
  `PostTask`, which stores them and triggers one redraw. `workGeneration`
  invalidates results that were overtaken by an edit or a cancel.
- A parse error on the first batch line — the one the viewport asked for —
  disables highlighting for the file. An error on a *prefetched* line only
  cuts the batch short: the finished attributes are posted (`partial`), and
  both sides force a re-anchor, because the session's position after a failed
  `ParseLine` is unknown.

*Why an anchor.* A jump then costs the same at line 500 and at line 500 000,
which is the only way a 600 000-line file can behave. Sequential scrolling
downwards still feeds the same session forward and stays exactly correct.

*What it costs.* A construct opened more than `hlColorerContext` lines above
the viewport is invisible to the parser after a jump, so the first screen after
`Ctrl+End` can be coloured as if a block comment had never started. Scrolling
down to it from above gives the exact answer. This is accepted deliberately —
it is the same trade already accepted for the stateless Chroma call on a jump.

### 3.3 One source of line text

`EditorView.lineTextForHighlight(idx) (string, bool)` returns a line as the
highlighters see it: **line terminator included** (the parse state of the next
line depends on it) and cut at 64 KB so no parser is handed a megabyte of
binary. It feeds both the render loop and, through `ch.lineAt`, the context
lines of a re-anchor, so those are byte for byte the same text. `ok == false`
means the text is not available yet; leave the line plain and come back next
frame.

### 3.4 Invalidation

Colours are cached by line number now, so the highlighter cannot notice an edit
on its own. Two hooks:

- `invalidateStates(fromLine)` — every edit path already calls it. Truncates
  the chain and calls `ch.DropFrom(fromLine)`.
- `clearCaches()` — undo, redo, reload. Calls `ch.DropFrom(0)`.

`DropFrom(idx)` drops cached attributes from `idx` on, and if the session has
already parsed past `idx` it throws the session away: its cache cannot be
unwound.

### 3.5 A fallback engine is handed to the editor

When the Colorer session cannot be created, or no schema matches the file,
`useFallback` moves `ch.fallback` into `ev.highlighter` instead of proxying it.
Otherwise a perfectly ordinary Chroma highlighter would be treated as Colorer
by `usesStateChain` and lose both its chain and its walker.

### 3.6 Everything runs on the UI thread — except Colorer parsing

The walker's slices, the render loop and every Chroma-style highlighter call
happen there, through `PostTask`. Highlighters are not thread-safe and the
wrap engine mutates its caches as a side effect of what look like reads.
Responsiveness comes from bounding each slice in time.

Colorer's `ParseLine` can execute arbitrary grammar code and is the one thing
that moved off-thread: a single worker goroutine owns the session, the UI only
queues immutable line snapshots and consumes posted results
(`colorer_async.go`). One worker, not a pool — the session is stateful and
line order is its state, so concurrent calls are wrong by construction, and
one owner gives cancellation a single well-defined home.

---

## 4. Approaches that were tried or considered and dropped

**Build a state chain for Colorer by walking the file.** Shipped, then
removed. See 3.1. It cannot work: there is no state to chain, and the walk
damages the session.

**Deep-rewind escape hatch.** When a rewind was too deep, `resync` did
`Reset(); parsedIdx = upTo; return`, leaving the session with no context and
its internal line numbering divorced from the document. Together with the
walker this is what produced "a file opened in the middle never gets colours".
Replaced by the anchor.

**Serialize the Colorer parser state.** The state is a stack of schemas in
libcolorer's `TextParser` cache. Making it serializable is a change to
libcolorer itself, not to the Go binding. Snapshotting the wasm linear memory
instead would mean tens of megabytes per snapshot. Not pursued.

**A second Colorer session for the walker.** Doubles the wasm memory and still
provides no way to move state from one session to the other.

**Filling the Chroma chain gap with nil states after a jump** (re-anchor the
chain heuristically instead of walking). Rejected in favour of checkpoints
(item 7): the nil-gap version permanently mis-states everything above the
anchor, and the checkpoints give exact colours for the price of a bounded
replay. It stays the fallback if checkpoints prove too expensive.

**A background goroutine instead of time-sliced UI tasks.** See 3.6.

---

## 5. Invariants

Breaking any of these produces silent, hard-to-trace mis-colouring.

1. `ch.parsedIdx - ch.anchor` is the session's own line number. Lines are fed
   in order, one at a time, starting at the anchor. Never feed a line out of
   order; never assume the session can go back.
2. Any move backwards, and any forward jump beyond `hlColorerForward`, is a
   reset. There is no third option.
3. `ev.lineStates` is dense from line 0 and `lineStates[i]` is the state
   *after* line `i`. It belongs to Chroma-style highlighters only.
4. Nothing calls `ev.highlighter.Highlight` for a Colorer with a live session.
   The render path uses `HighlightLine`; the walker refuses to run.
5. All highlighter text comes from `lineTextForHighlight`.
6. A slice of background work must be bounded by wall clock, not by a line
   count: one line of Colorer and one line of Chroma differ by two orders of
   magnitude.
7. The line indexer outranks the highlighter. If both want the UI thread, the
   index wins — it is what the scroll bar and every jump are waiting for.

---

## 6. Work queue

Each item is one commit. Do them in order unless a measurement says otherwise;
do not start the next one before the previous is verified. Every item lists
where to look, what "done" means, and what not to touch.

### Item 1 — show the file while the saved position is still being indexed

*Problem.* Opening the reference file leaves the editor blank for several
seconds. `DisplayObject` (`editor_view.go`, the `if ev.targetLine != -1` guard
near the top) fills the text area with spaces and returns. `targetLine` is set
in `actions.go` when a saved state exists for the file, and cleared by the
indexer once it reaches that line — or by any key that moves the cursor.

*Why it is there.* Painting from the top and then jumping when the restore
lands reads as a flicker. That reasoning holds for a small file, where the wait
is a few frames. It does not survive a wait of seconds.

*What to do.* Time-box the blank. Record the moment the editor was created
(or the moment `targetLine` was set) and keep the blank only for
`restoreBlankGrace` — start with 250 ms. After that, draw the document
normally while the restore is still pending; the jump, when it lands, is one
frame and strictly better than an empty window.

*Notes.* While `targetLine != -1` the restore path controls the scroll
position, so the document renders from the top; do not change that. Do not
remove the restore, do not touch `ProcessKey`'s rule about abandoning it.

*Acceptance.* Opening the reference file shows the top of the file within a
frame or two, still uncoloured or partly coloured, and jumps to the saved
position when the index reaches it. `editor_target_line_test.go` still passes.

*Test.* With `targetLine` set and the grace period pushed into the past, one
`ev.Show(scr)` must leave non-blank cells in the text area (`scr.GetCell`);
with the grace period still running, it must not.

### Item 2 — delete the dead Colorer state-chain code

Nothing calls it any more; it exists only to confuse the next reader and to be
accidentally reachable.

*Remove from `colorer_plugin.go`:* `ch.lines`, `resync`, `colorerNeedsRewind`,
`colorerLineIndex`, and the body of the `Highlight` shim that uses them. Keep
`Highlight` itself: it is still the `vtui.Highlighter` entry point, and it must
keep working while the session is loading (`starting` → nil) and while a
fallback engine is in place. With a live session it should now delegate
nothing and return nil — `HighlightLine` is the way in.

*Also remove* the tests that only covered the deleted helpers
(`TestColorer_LineIndexComesFromTheEditorState`,
`TestColorer_RewindsOnlyWhenTheParserIsAhead` in `colorer_plugin_test.go`).
Keep the fallback tests.

*Acceptance.* `go build ./... && go test .` unchanged, and `ch` no longer holds
a copy of the document.

### Item 3 — bound the attribute cache

`maxCachedAttrLines` (5 000 000) and `attrCacheKeepWindow` (1 000 000) in
`colorer_plugin.go` are effectively no limit: colours are kept for every line
ever drawn, each as a `[]uint64` the width of the line. Scrolling through the
reference file accumulates hundreds of megabytes.

*What to do.* Shrink them to a window around the viewport — start with 20 000
and 5 000 — and check `storeAttrs`'s eviction actually keeps the map near that
size. Keep the existing exception that protects the first lines of the file, so
`Ctrl+Home` stays instant.

*Cost to expect.* A line evicted from the cache and then scrolled back to is a
miss, and a miss above the parse position is a re-anchor. That is the designed
behaviour; the window only has to be larger than a screen by a comfortable
margin.

*Acceptance.* `TestColorer_AttrCacheIsBounded` extended to push past the new
limit and assert the map stays bounded.

### Item 4 — bound the session on a long forward scroll — superseded by item 9

Re-anchoring resets the session, which is what releases the wasm line vector
and the parse cache. One case never re-anchors: scrolling straight down, which
feeds the session forward for as long as the user holds `PgDn`.

*Resolution.* The forced-re-anchor counter proposed here was never needed:
item 9's upstream `colorer_forget_before` landed and is wired in as
`session.ForgetBefore`, driven by `colorerForgetPlan` in the worker — every
`hlColorerForgetEvery` (1000) lines fed, the session drops everything more
than `hlColorerKeepBehind` (300) lines behind the parse position. A long
forward scroll stays bounded without ever paying a reset's loss of context.

### Item 5 — cheaper editing in a large file

Typing at line 500 000 currently throws the session away on every keystroke
(item 3.4), so the next frame re-anchors: roughly `hlColorerContext` lines of
parsing per character. In a file small enough to be reached from line 0 this
does not arise.

*Measure first.* Type a line into the reference file and see whether it is
felt. If it is: the fix is a smaller anchor while an edit is in flight, not a
different design. Do not reintroduce a state chain.

### Item 6 — progress feedback

While a viewport is uncoloured and the walker is behind it, the user has no
way to tell work from breakage. Show it in the top bar
(`EditorView.topBar`, right-hand side, next to the codepage and cursor
position) — a percentage while `ev.highlighting` is true and the walker is
behind the viewport, nothing otherwise. Colorer has no walker and needs no
indicator.

### Item 7 — checkpoints for Chroma-style highlighters

`ev.lineStates` holds one `any` per line for the whole document, and after a
jump the walker still has to reach the viewport from wherever it stands.

*What to do.* Keep a checkpoint every `Step` (start with 1000) lines —
`checkpoints[k]` is the state entering line `k*Step`, `checkpoints[0]` is nil —
plus a dense window around the viewport. Drawing line L takes the nearest
checkpoint at or below L and replays at most `Step` lines. `invalidateStates`
maps onto checkpoint granularity: drop checkpoints above `fromLine/Step`.

*Why this and not a heuristic re-anchor.* See section 4. Correct colours for a
bounded replay.

*Prerequisite.* Only start this after items 1-3; it touches the same render
path.

### Item 8 — state-only fast path for the walker

The walker calls `Highlight` and throws the attribute slice away — one
allocation per line for nothing. If a highlighter also offers something like
`HighlightState(line string, prev any) any`, discover it with a type assertion
and use it. Chroma-side support lives in vtui, so this may become an upstream
change there.

### Item 9 — colorer4go: trim the session window — done

Upstream, in `github.com/unxed/colorer4go`. The wrapper only appends; a
`colorer_forget_before(lno)` (or a ring buffer in `WasmLineSource`) would let
the session drop lines the editor will never ask about again, which makes item
4 unnecessary and removes the only reason a long scroll ever resets. Do
**not** ask for state snapshots — see section 4.

*Resolution.* Delivered as `Session.ForgetBefore`; the worker calls it every
`hlColorerForgetEvery` lines through `colorerForgetPlan` (see item 4). A
session that answers `ForgetBefore` with an error keeps the old
grow-until-reset behaviour for that session only.

---

## 7. How to build and test

    go build ./...
    go vet .

Targeted, while working in this area:

    go test -run 'Colorer|Highlight|State|Editor_' .

Whole package before sending a patch:

    go test .

Two failures are expected in a container and unrelated to this work:
`TestExecuteFileOp_Move_PermissionDenied_Recovery` (runs as root, so the
permission it wants to be denied is granted) and `TestUpdateFailureMessageRepro`
(wants the network). Verify against a clean checkout before blaming a change
for anything else.

Manual check on the reference file, in this order: open it (item 1), `Ctrl+End`,
hold `PgDn` from the top, `Esc` right after releasing it, `Ctrl+Home`, and type
a character deep in the file. With `--debug`, the two loops report

    EDITOR: Indexer stopped: N lines in T, W of it waiting for data, B UI batches
    EDITOR: Highlight walker stopped: N lines, U on the UI thread, T wall clock

Quick check that a problem is about highlighting at all: set
`EditorHighlighter = None` and repeat. If it is still slow, the cost is in the
loader or the line index, and nothing in this document applies.

---

## 8. Constants

`editor_view.go`, walker:

| name | value | meaning |
|---|---|---|
| `hlSliceBudget` | 4 ms | longest stall one slice may put on the UI thread |
| `hlClockStride` | 8 | lines between two clock readings inside a slice |
| `hlDutyIndexing` | 10 % | walker's share while the line index is building |
| `hlDutyVisible` | 50 % | share while the viewport is still uncoloured |
| `hlDutyAhead` | 25 % | share while walking past the viewport |
| `hlIdleMin` / `hlIdleMax` | 1 / 100 ms | bounds on the gap between slices |
| `hlStallIdle` | 10 ms | retry gap when a slice got no data |
| `hlMaxStallSlices` | 100 | give up after this many empty slices |
| `syncHighlightGapLimit` | 50 | largest chain gap still caught up in the draw path |

`colorer_plugin.go`, anchor:

| name | value | meaning |
|---|---|---|
| `hlColorerForward` | 2000 | furthest the session is fed forward instead of re-anchored |
| `hlColorerContext` | 300 | context lines parsed above a new anchor |
| `hlColorerBatchLines` | 200 | longest uncoloured run one worker job takes on |
| `hlColorerBatchBytes` | 256 KB | most line text one batch snapshot copies on the UI thread |
| `hlColorerKeepBehind` | 300 | lines kept behind the parse position when the session is trimmed |
| `hlColorerForgetEvery` | 1000 | lines fed between two `ForgetBefore` wasm calls |
| `maxCachedAttrLines` | 20 000 | attribute cache limit (item 3, done) |
| `attrCacheKeepWindow` | 5000 | eviction window (item 3, done) |

None of these are settings. Do not add them to `AppConfig` before a
measurement asks for it.

---

## 9. Log

- Ctrl+End froze for 30-40 s: the chain was built from line 0 inside the draw
  path.
- Draw path no longer catches up over a large gap; Chroma gets an immediate
  stateless call. Chroma fixed.
- Walker added, then bounded by wall clock and put on a duty cycle, yielding to
  the line indexer.
- Colorer taken out of the walker.
- Colorer drawn from an anchor next to the viewport; `ch.lines` no longer
  needed; fallback engine handed to the editor.
- Reported: the tail of the reference file appears and colours immediately with
  both highlighters. Remaining: the blank window on open, item 1.
- Colorer parsing moved to a worker goroutine owning the session; the UI
  queues immutable snapshots and consumes posted results (`colorer_async.go`).
- One worker job per line made the viewport colour visibly line by line: each
  line cost a full worker→PostTask→redraw round trip, ~40 full render passes
  per screen. Jobs now batch the whole uncoloured run (`hlColorerBatchLines`),
  so a screen colours in one round trip and one redraw.
