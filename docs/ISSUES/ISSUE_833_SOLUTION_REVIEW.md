# Issue #833 — code-page auto-detection and per-file selection

## Report and observed cause

Issue #833 asks to repair Autodetect code page, expose the editor and viewer
defaults in settings, and keep the last manually selected code page for each
file. The attached screenshot is the existing code-page menu with the
auto-detect toggle and default selection.

The current detector accepts valid UTF-8, otherwise compares only the current
system ANSI and OEM decoders. On a Linux C.UTF-8 locale those are Windows
1252 and CP437, so Russian CP866, Windows-1251, and KOI8-R samples are
misclassified as ANSI. The normal Editor and Viewer state provider has no
code-page field either; only Quick View stores an explicit override. Viewer
header reads also pass the complete buffer to detection instead of limiting it
to the number of bytes actually returned.

## Three candidate solutions

### A. Keep the current two-candidate score and add a UI-only settings form

Add the requested checkboxes/combos to settings, but continue comparing only
system ANSI and OEM. This is small and does not add dependencies, but it still
cannot recognize a supported non-system code page and reproduces the reported
failure on this machine. Per-file state would also need a separate change.

### B. Detect among supported decoders, preserve ambiguity, and store explicit overrides

Evaluate every supported single-byte decoder (including system aliases)
against the real bytes returned by the VFS. Use text quality first, then a
bounded language-profile score and a small Russian n-gram tie-breaker only when
the candidate decodings are close; retain the configured default when the
evidence is ambiguous, because single-byte encodings are not mathematically
identifiable from arbitrary bytes. Check BOM and UTF-8 first.
Store an explicit code-page override in the shared file state, use it in both
Editor and Viewer, and clear it when the user chooses Auto-detect. Expose the
existing editor defaults and a matching viewer dialog without changing the
default values for existing users.

This handles CP866/1251/KOI8-R and the other code pages already offered by the
UI, does not require an external executable, keeps Quick View's independent
override, and gives users a deterministic escape hatch for mixed-encoding
files. It also fixes the short/partial VFS read path used by Viewer.

### C. Delegate detection to an external or new charset detector

Invoke uchardet/file or add a universal detector dependency. This may
recognize more encodings, but an external binary is unavailable on many
platforms and common pure-Go detectors do not cover every code page f4 offers
(notably CP866). It also makes the result harder to align with the configured
system aliases and adds a failure mode before any UI can open.

## Selection and three-pass review

Solution B is selected.

1. **Correctness pass:** BOM and strict UTF-8 remain authoritative; supported
   legacy decoders are compared on a trimmed sample; weak or tied evidence
   falls back to the configured default. A manually chosen code page overrides
   detection only for that file until Auto-detect is selected.
2. **Lifecycle pass:** the shared file-state key keeps the override across
   Editor ↔ Viewer transitions and restarts, while Quick View keeps its
   existing separate field. Binary heuristics still run after a remembered
   code page is applied, and Viewer backends are rebuilt when a selected code
   page changes.
3. **Regression/scope pass:** settings are persisted independently for Editor
   and Viewer; UI layout is validated in English and the available language
   packs. Tests cover all detector paths, partial reads, per-file restoration,
   clearing an override, and switching between Editor and Viewer. Native
   ARM64 ANSI TTY validation uses real CP866/CP1251/KOI8-R fixtures and a
   persisted manual override.

The implementation will keep detection conservative where bytes are
ambiguous, generalize the fix across Editor, Viewer, and Quick View, and avoid
changing binary handling or explicit code-page conversion semantics.
