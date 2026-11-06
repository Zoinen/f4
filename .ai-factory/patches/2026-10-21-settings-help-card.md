# Settings contextual help card and scroll alignment

**Date:** 2026-10-21
**Files:** SettingsHelpCard.qml, SettingsDialogBody.qml, SemanticWidgetDelegate.qml; F4IconProvider.cpp; icons/lucide/info.svg; internal/settings/center_semantic.go; internal/nativeui/scene.go; sdk/extui/model.go

## Change

Render contextual help as an adaptively docked rounded card with an information icon,
bold heading, muted wrapped description and darker timing text without a chip.
Keep the original right-side placement when the content area is at least 720
logical pixels wide, and bottom placement below that threshold.
Long descriptions have their own viewport. Preserve the legacy full text and
export optional localized helpTiming metadata instead of detecting English UI
strings in QML. The terminal presentation is unchanged.

## Related defect found during verification

At Cocoa DPR 175%, the existing fractional-scroll check sometimes read geometry
before the afterAnimating alignment pass. grabWindow can return the preceding
frame there. The check now waits for the same exact pixel-alignment invariant;
production alignment is unchanged. Visual QA also caught an unavailable info
asset falling back to the file icon. The SVG and provider allowlist now include it.

## Verification

Go tests cover help metadata and retention of legacy text. Settings, nativeui
and ExtUI package suites pass with CGO_ENABLED=0. Qt checks cover card structure,
timing colour, bottom placement, resize, retained controls and pixel alignment.
The full offscreen suite passes (89 tests, 2 opt-in profiling skips); focused
Cocoa checks pass at 175% (6 tests), including the formerly failing scroll check.

## Prevention

Keep localized content metadata separate from presentation. Rendering checks
must wait for the frame's layout/alignment pass. Assert icon identity as well as
successful loading: a successfully loaded fallback is not the requested artwork.
