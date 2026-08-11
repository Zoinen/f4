# Review queue

Doubts, shortcuts and things deliberately left alone, written down here so
they can be reviewed in one sitting instead of being rediscovered one at a
time. Nothing here blocks anything; anything that grows a plan of its own
moves out of this file.

## The keepalive interval is not the one the host was configured with

A minute is a guess that fits the common defaults. A host with a shorter
`ClientAliveInterval`, or a NAT with a thirty second table, drops the session
anyway, and there is nothing in the protocol that would let the client ask. A
setting is the obvious answer and should wait until a host that needs it turns
up.

## Finding a session dead is still all that happens

The keepalive marks a session broken and stops. That moves the discovery
earlier, which is worth having on its own, but the user still has to open the
site again. Picking up where it was is the rest of step 14.
## Two archives share one state namespace

`vfsStateNamespace` names a remote site by the title the panel already shows and
everything else by its Go type. That tells a local file from an archive member
and one host from another, but two different archives are one namespace: the
same path inside two zips shares a saved position. Naming an archive by the file
it lives in needs the path of that file, which a mounted VFS does not expose
today.

## A site renamed is a site forgotten

The key holds `user@host` as it was typed. Connecting to the same machine by IP
one day and by name the next gives two namespaces, and every saved position
starts again. A host key would be the honest identifier; it is also one the user
never sees, so the panel would show one thing and the state file another.

## AsyncBuffer can hand out a short read without saying so

`AsyncBuffer.Read` clips what it copies to the length of the chunk it found
and returns no error when the result is shorter than asked for. A chunk is
stored short whenever the underlying `ReadAt` returned fewer bytes together
with `io.EOF`, which for a remote file is not only the end of the file.
Anything that advances by `len(data)` then loses its alignment silently, and
the line offsets it records are wrong from that point on. Not observed in the
wild; found while reading the code for Step 13a.

## A chunk that failed to load is dropped without a trace

`AsyncBuffer.fetchChunk` logs the failure and clears the fetching flag, so the
next read retries. That is the right behaviour for a hiccup and the wrong one
for a host that is gone: nothing counts the retries and nothing tells the
user.

## A mouse click still cancels a pending restore unconditionally

`ProcessMouse` drops the restore whenever a button is down. A click does place
the cursor, so that is defensible, but it is not the same test the keyboard now
applies, and a click that lands on the cursor's own position cancels the jump
for nothing.

## Missing Help Files

The translation packs (`.lng` files) exist for several languages that are currently missing their corresponding help files (`.hlf`).
These languages are: `be`, `fi`, `hu`, `hy`, `ja`, `ka`.
As a fallback, `f4` will display the English help for these languages, but they should eventually be translated.
## Automated Layout Verification

An automated layout validation suite has been implemented in `f4/dialog_layouts_test.go`.
It acts as a single source of truth by iterating over all actions from `action_registry.go` and verifying their layout constraints (overlaps, border collisions, and multi-language overflows) across all available translation packs.

- When creating or modifying dialogs, developers no longer need to write custom test logic; the layout test suite automatically discovers them as long as they are registered as standard actions.
## File Association Test Race Condition Fixed
We resolved a test-flake race condition in `TestFileAssociation_PickerRunsChosenCommand` where asynchronous panel refreshes (`f4/file_associations_dispatch_test.go`) would overwrite manually-mocked panel entries with actual repository root files (such as `.git`). This was fixed by physically creating the mock files and directories within the temporary test directory on disk before triggering the refresh.

## The indexer's batch size is a constant nobody has measured

500 line offsets per batch, 64 KB per read. Both were picked to keep the UI
thread from being flooded by a local file, and neither has been measured
against a link where a read is a round trip.### Postponed far2l Color Porting (Roadmap / Future Tasks)

The following color entities and features from `far2l` have been explicitly postponed during the initial alignment phase and remain on our future roadmap:
- **Explicit Disabled Colors:** Transition to dedicated disabled color slots from themes/palette instead of using the dynamically computed `DimColor` fallback.
- **Granular Lists & Comboboxes:** Support custom coloring for `Dialog.List.*` and `Dialog.Combo.*` sub-elements.
- **Default Buttons:** Map and apply styling for `Dialog.DefaultButton.*` elements.
- **Editor & Viewer Selection:** Map and support dedicated selection slots `Editor.Text.Selected` / `Viewer.Text.Selected` (currently falls back to standard text selection).
- **Secondary Widgets:** Support `Clock`, `Panel.ScreensNumber`, and `Panel.DragText` slots.
### Localization debt and open questions

The state of the localization work lives in `L10N_PLAN.md`. Points that need a
human decision at review time:

- `tools/hardcoded_baseline.txt` de-duplicates identical entries, so a second
  copy of an already known caption in the same file and constructor slips
  through the gate. Line numbers would close that hole but would make the
  baseline churn on every unrelated edit. Revisit once the baseline is close
  to empty.
- The scanner knows only a handful of vtui constructors. Menu items, input
  boxes and titles assembled with `fmt.Sprintf` are invisible to it.
- `lang/pl.lng` contained `Action.Editor.Undo` and `Action.Editor.Redo` twice,
  with different hotkey positions. The duplicates were removed and the first
  occurrence kept; the INI parser silently preferred the last one before that.
- `TestExecuteFindFile_MaskMatching` times out. Unrelated to localization, not
  investigated here.
  - The Italian and Dutch translations were deleted rather than
    repaired, because a mixture of four languages cannot be sanitized in a way
    anybody can verify. Roughly 450 usable strings per file went with them. If
    that turns out to be the wrong call, they are one `git revert` away.
  - `actions.go` still maps the language codes `it` and `nl` to display
    names for the help language menu. The cases are dead until those languages
    come back, and were left in place on purpose.
    - `ParseIni` trims whitespace around values, which silently ate the padding
      spaces of every dialog title (`FindFile.Title= Find File`). Some translations
      have since lost their trailing space to whitespace-stripping editors, so the
      padding cannot be restored from the data alone. Either the language files need
      a parser that preserves values, or `NewCenteredDialog` should pad the title
      itself. Deferred as cosmetic, recorded in `L10N_PLAN.md` section 6.
    - The layout failures were all fixed by shortening translations. Some captions
      now sit exactly on the limit (the German font label ends on the last allowed
      column), so a future dialog resize will need a look at `L10N_PLAN.md`
      section 7.1 rather than a guess.
      ## `CloneStateFrom` copies the PTY handle along with the screen

      `TerminalView.CloneStateFrom` assigns `other.pty` to the clone, while the
      clone's own `initPTY` goroutine assigns its freshly created PTY to the same
      field. Which one wins is a race. A PTY is ownership rather than visual state
      and has no business in a state clone. Left alone here because it belongs to
      the terminal umbrella (#425); recorded in `TERMINAL_WINDOWS.md` section 5.

      ## The sync excision also mangles commands a human typed

      The fallback branch in `AnsiParser.Process` removes `cd /d "..." & ` even when
      the `rem f4_sync` marker is absent, so a `cd /d "X" & dir` typed by the user
      loses its first half in the log. Fixing it properly means marking our own
      writes rather than pattern-matching text, which is `TERMINAL_WINDOWS.md`
      section 6.
      ## Colorer and Go disagree on how to count malformed UTF-8

      Colorer's region offsets are rune indices, and the editor now takes them as
      such. On well formed text the two sides count identically. On a truncated or
      stray multi byte sequence they do not: Go yields one replacement rune per bad
      byte, while Colorer's decoder in `strings/legacy/CString.cpp` reads the length
      out of the lead byte and consumes that many bytes regardless of what they are.
      A binary file opened in the editor can therefore still see colours land beside
      the character they belong to, on the damaged line and after it.

      Fixing this means either counting the way Colorer does when building the
      attribute slice, or sanitising the line before handing it over. The second is
      better and belongs with whatever eventually decides how the editor displays
      binary content, so it is not being done here.

      ## Colorer's UTF-8 encoder mangles astral characters

      `Encodings::toBytes` in the vendored Colorer library builds a four byte
      sequence as `0xF0 + (wc >> 14)` with the second byte taking bits 12 and up,
      where the shifts should be 18 and 12. Anything above U+FFFF that leaves the
      library as UTF-8 comes out wrong. Nothing on our path does that today - region
      names are ASCII and lines travel inwards, not outwards - so this is recorded
            rather than patched. It is upstream code and a patch belongs upstream.

      ## The PTY is published once but copied three times

      `initPTY` sets `pf.pty` under `ptyMutex` and, in the same critical section,
      `pf.parser.pty` and `pf.termView.pty`. `PanelsFrame` now reads its own field
      through `localPTY`, but the parser and the terminal view read their copies
      straight from the UI thread, and `NewPanelsFrame` writes `pf.termView.pty` a
      second time, unlocked, right after starting the goroutine. That is the same
      two word interface race that crashed the quit path, only with a smaller
      window and a less obvious symptom, since a torn value there gets a `Write`
      rather than a `Close`.

      The tidy fix is for one owner to hold the PTY and for the other two to ask it,
      which means touching the parser and the view and is more than a crash fix
      should carry. Recorded here so it is done deliberately, with the terminal
      tests in front of whoever does it.
