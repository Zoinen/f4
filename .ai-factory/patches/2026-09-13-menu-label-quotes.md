# Literal quotes in translated menu labels

## Problem and root cause

English and Russian panel mode and disk-menu entries used quoted INI values.
The language parser deliberately preserves value text, so the quote delimiters
were displayed literally by every frontend.

## Solution

Remove accidental outer quotes from the 16 affected translation values, without
changing parser semantics or stripping legitimate punctuation in renderers.
No runtime code or layout changes are needed.

## Verification and prevention

The regression scan `rg -n '^(Panel\.View\.|DiskMenu\.(MacOS|Shell))[^=]*=\s*"' internal/i18n/lang/en.lng internal/i18n/lang/ru.lng`
reported 16 entries before the fix and none afterwards. Run i18n and INI package
tests and verify native menu titles after rebuilding. Diagnostics are recorded
in the verification output, not added to the menu rendering hot path.
Further coverage could enforce unquoted labels across all language packs.

## Tags

`#i18n` `#menus` `#ini`
