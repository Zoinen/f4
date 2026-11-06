## Change Summary

262 upstream commits affecting 297 paths are being merged from unxed/main
a546e70cb into zoin (starting at 3beabafbe). Risk: high integration breadth.

### What Changed

Upstream editor, archive, file-operation scheduling, drive menus, clipboard,
SSH and settings updates, including merged PR #2051 (Replace All/save).
Preserve Qt semantic APIs and synchronize the vendored vtui to v0.1.411.

Follow-up merge includes upstream 80f06cb39 (25 additional commits fetched
after approval). Preserve connection metadata while applying saved start
folders, including SFTP's FISH fallback; adapt upstream rule-list identity
invalidation to the fork's synchronized metadata-aware highlight cache.
The upstream rule-replacement regression failed before adaptation and passed
afterward. Isolate the new tree scrolling fixture from whole-volume traversal.

The user explicitly approved keeping upstream archive dependency versions
despite the inherited alignment-check failure before committing/pushing.

### Evidence

`git log HEAD..origin/main` and merge conflicts identify the affected areas.
Existing unstaged Qt, ZoinGallery, screenshots and diagnostics are outside the
merge commit. Archive dependency alignment fails equally in upstream-only
checkout: 12 inherited mismatches, with no newer published chain available.

### Risks and Recommendations

Verify actual folder navigation, editor save/return, viewer and settings.
Check RPC write buffering, persistent rectangular selection, and ANSI log
snapshot offsets. Broader native tests must be reported separately from smoke.
