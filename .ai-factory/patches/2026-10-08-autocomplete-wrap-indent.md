# Hanging indentation in multiline command suggestions

**Date:** 2026-10-08
**Severity:** low
**Files:** qt/host/qml/AutocompletePopup.qml, qt/host/tests/F4QuickViewSurfaceTests.cpp

## Problem and Root Cause

Soft-wrapped continuation lines aligned like explicit command line breaks,
making the two cases difficult to distinguish.

## Solution

Render each explicit logical line separately and indent only its subsequent
visual lines by two font-measured spaces using Text.lineLaidOut. Reduce the
continuation width by the same amount. Preserve escaping and prefix highlighting
without changing accepted command text.

## Verification

The regression failed before implementation and passes afterward. It forces a
narrow logical line, verifies wrapping and positive continuation indentation,
and verifies explicit lines retain the same left origin. The related suite has
6 passes at DPR 1.75; logging uses [FIX:autocomplete-multiline].

## Prevention

Distinguish logical lines from visual lines when adding hanging indentation.
Retest explicit breaks, wrapping, variable row height and popup navigation.

## Tags

`#qml` `#autocomplete` `#word-wrap`
