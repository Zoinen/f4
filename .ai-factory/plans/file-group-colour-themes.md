# Independent file colour and icon themes

Tests: yes
Docs: yes
No commit/push or unrelated cleanup.

Colour themes map normalized group names to four-state colour expressions.
Icon themes remain artwork catalogs; application UI colour schemes are separate.
Keep group predicates, ordering, commands and feature toggles owned by groups.
Ship Current (extracted current group colours) and Zoin (opaque SVG base colours).
Preserve unknown fields, reversible drafts, atomic saves and external-edit checks.

- [x] 1. Add independent colour-theme domain, document persistence/migration and runtime projection, with tests.
- [x] 2. Extract/ship both declarative colour themes and verify every Zoin SVG colour including opacity.
- [x] 3. Add independent theme selectors and name-based colour editing/preview in File groups; verify Apply/Cancel and icon independence.
- [x] 4. Update docs, run Go/Qt regressions, build and live smoke-test.

Verification: CGO_ENABLED=0 tests passed for filegroups, theme, settings, panel,
app, nativeui, extui and architecture checks; affected-package go vet passed.
Qt OperationsQueue: 73 passed, 2 opt-in profiling skips, at 175% scale.
Live Qt smoke: Current retained existing appearance, Zoin used extracted hues,
Lucide artwork retained Zoin colours, child/parent Cancel rolled back previews.
Built and restarted the app; left File groups open with Colours: Current.
Documentation review: format examples and local links checked; no unrelated
documentation restructuring. The profile is not written until Apply.
