## Test Cases: zoin upstream merge

### TC-001: folder navigation (High, Positive)

Open fixture `data` in both panels; double-click `subfolder` in the right
panel. Expected: nested.txt appears; left panel remains at data. PASS in live UI.

### TC-002: editor save and return (High, Regression)

Select left prompt.txt, F4, insert X, F2, Escape. Expected: panels restored;
disk bytes are `Xold path\nsecond line\n`. PASS in UI and independent file read.

### TC-003: viewer (Medium, Positive)

Press F3 on saved fixture. Expected: both lines, including X, render; Escape
returns to panels. PASS in live UI.

### TC-004: settings (Medium, Positive)

F9, Options, Settings, Cancel. Expected: Settings Center opens, controls render,
and cancellation restores panels without changing settings. PASS in live UI.

### Supporting results

Go editor/viewer/config/settings/theme/fileops/panel/netfox and Android tests
pass; selected app/plughost tests pass. Qt host builds with video and FFmpeg.
Extended Qt run: protocol/catalog/intents/document/gesture tests pass; queue has
one assertion failure, QuickView hits 90s timeout, GalleryBridge has an icon
assertion and a SIGSEGV. Their native sources were not modified by this merge;
the worktree contains pre-existing unstaged Qt/ZoinGallery changes.
Archive alignment: 12 mismatches also reproduced in upstream-only checkout;
explicit policy exception requested before commit/push.
