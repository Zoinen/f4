# Issue #277 — the rest of FarColorer

The issue asks for everything FarColorer offers, with all its settings. The
list below is taken from the plugin's own sources, which colorer4go carries in
`colorer/src/pcolorer2` (`FarEditorSet.cpp`, `FarEditor.cpp`,
`FarHrcSettings.cpp`), and is worked through one step at a time, each step
usable on its own.

## FarColorer, and where f4 stands

| FarColorer | f4 |
| --- | --- |
| Enabled | `EditorHighlighter = Colorer` |
| Show cross (off / always / if the scheme has it) | `EditorCrossMode`, `EditorCrosshair` |
| Syntax | `EditorColorerSyntax` |
| Change editor background | `EditorColorerBackground` |
| HRD for TrueMod (class `rgb`) | `EditorColorerScheme` |
| HRD for console (class `console`), TrueMod switch | not applicable: f4 always draws RGB |
| Catalog | `EditorColorerCatalog` (a directory, not catalog.xml) |
| User file of schemes (UserHrcPath) | `EditorColorerUserHrc` — step 1 |
| User file of color styles (UserHrdPath) | `EditorColorerUserHrd` — step 1 |
| Reload / test load with the error shown (`TestLoadBase`) | step 2 |
| Pairs: highlight the bracket pair under the cursor | step 3 |
| Menu: match pair, select pair, select block | step 3 |
| Menu: list functions, find errors, locate function; old outline view | step 4 |
| Menu: list types (favourites, hotkeys), select region | step 5 |
| Per-type HRC parameters dialog, UserHrcSettingsPath | step 6 |
| Viewer colouring (off / quick view / all) | step 7 |

## Step 1 — user schemes and colour styles (done)

The case from the issue's comment: a folder of user schemes and a file of
colour styles, as in Far. colorer4go gained `WithUserHRC` / `WithUserHRD`;
f4 stores both paths, offers them in the Colorer settings dialog and the
Settings Center, and keys sessions and caches by the whole source. See
HIGHLIGHT.md 3.8 for the limits the wasm sandbox and Colorer's legacy strings
impose.

Found on the way and fixed with it: the Settings Center style list read
catalog.xml with `encoding/xml`, which stops at the external entities the
installed catalog uses (`&catalog-rgb;`), so it listed no styles at all.

## Step 2 — load before use, and say what is wrong (done)

OK, Reload and a new "Check all schemes" button load the configuration first
and show what Colorer could not load or reported; the Settings Center has the
same as commands. colorer4go gained `Session.FileTypes` and
`Session.LoadFileType`, which also give step 5 its list of types. See
HIGHLIGHT.md 3.9.

## Step 3a — the pair under the cursor (done)

colorer4go's `ParseLinePairs` returns pair regions; f4 matches them as
`BaseEditor::searchPair` does and draws the pair under the cursor, behind the
new `EditorColorerPairs` setting. See HIGHLIGHT.md 3.10.

## Step 3b — match pair, select pair, select block (done)

Three editor actions search the whole file for the match, queueing lines the
cache has not reached to the worker. See HIGHLIGHT.md 3.10.

## Step 4a — list functions, find errors, locate function (done)

colorer4go's `Session.LineOutline`; f4 collects the whole file's outline
through the worker, lists it and locates functions, and has the old outline
view setting. See HIGHLIGHT.md 3.11.

## Step 4b — the outliner's own keys (done)

FarColorer's filter, Tab completion, Ctrl+Up/Down preview, Ctrl+Left/Right
levels and Ctrl+Enter insert in the outline list; FarColorer's commands in a
Colorer submenu of the editor's F11 menu. See HIGHLIGHT.md 3.11.

## Step 5 — list of types, favourites, hotkeys, select region (done)

colorer4go's `SetFileType`, `FileType`, `WithHRCSettings`, `FileTypeParam`
and `SetFileTypeParam`; f4's list of types, its profile in HrcSettings.ini and
select region. See HIGHLIGHT.md 3.12.

## Step 6a — file type settings dialog, user HRC settings (done)

colorer4go's `FileTypeParams`, `ResetFileTypeParam` and `WithUserHRCSettings`;
f4's HRC settings dialog and the user HRC settings path. See HIGHLIGHT.md
3.13.

## Step 6b — make the parameters count (done)

show-cross (cross mode "By file type"), maxlinelength, fullback, default-fore
and default-back. backparse and cross-zorder are not applied, for the
reasons in HIGHLIGHT.md 3.13.

## Step 7a — highlighting in the quick view (done)

The quick view panel highlights text with the editor's highlighter, Chroma
or Colorer, behind `ViewerHighlighting` (off by default / quick view). See
HIGHLIGHT.md 3.14.

## Step 7b — highlighting in the viewer (done)

"All viewers": the viewer highlights the lines on screen with up to 100 lines
before them as context, off the UI thread, with Chroma or Colorer. See
HIGHLIGHT.md 3.14.

## F11 menu: update highlighting, reload base, configure (done)

The last three items of FarColorer's menu. See HIGHLIGHT.md 3.11.
