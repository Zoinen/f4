# Release-gate test and vet repairs

The release branch had several gate failures that were reproducible before the
release build: panel toggling dereferenced a deliberately minimal test frame's
nil command line; the panel-load helper returned before worker-posted metadata
tasks were drained; action order and the Explorer help expectation lagged the
current registrations; and the theme tests inherited the developer's real
farcolors.ini. Go 1.26 also diagnosed test snapshots that copied sync.Once and
vtui's lock-bearing FrameManager.

Keep minimal panel fixtures safe, join directory workers before the final UI
drain, isolate built-in style tests from user files, and replace frame-manager
test instances by pointer rather than copying them. Keep action/message keys
consistent so the i18n near-miss sweep remains meaningful.

Verified: focused regressions, full `CGO_ENABLED=0 go test ./...`, affected
package vet, Darwin/arm64 cross-target vet, `git diff --check`, and workflow
`actionlint`.
