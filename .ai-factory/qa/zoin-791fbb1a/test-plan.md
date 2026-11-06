## Test Plan: upstream merge into zoin

Date: 2026-10-18. Environment: local macOS, Qt 6.11.1 with video/FFmpeg on.

### Goal and Scope

Smoke-test the merged core with the current Qt host. Use only the isolated
profile and fixture files under `.diagnostics/upstream-smoke/`.
In scope: panels, folders, editor/save, viewer, menu and settings launch.
Out of scope: real device transfers, production SSH files, releases, full CI.

### Preconditions and Acceptance

Build succeeds with CGO_ENABLED=0 and the system Go cache. No real user data
may be modified. High-priority cases in test-cases.md must pass; report native
suite failures and archive alignment separately rather than calling them green.

### Checklist

- High: navigation into a folder retains the other panel.
- High: F4/edit/F2/Escape saves bytes and restores panels.
- Medium: F3 renders saved content; settings opens and cancels.
- Supporting: Go package tests, Qt CTest selection and archive dependency check.
