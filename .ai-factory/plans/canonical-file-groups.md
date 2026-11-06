# Canonical file groups

## Settings

- Tests: yes
- Docs: yes
- No commit, push, profile rewrite or unrelated cleanup.

## Design

One canonical factory group per concept. Colour styles contribute palette-specific
highlighting variants, not additional groups. A variant retains its historical
predicate and cascade order where these differ from the group's classification
condition. Variant conditions are visible/editable; they are compiled once.
Legacy profile rules remain independent Imported records, never silently deleted
or promoted to factory defaults. Existing factory IDs migrate by explicit aliases.

## Tasks

- [x] 1. Add palette highlighting variants and canonical factory composition, with parity and uniqueness tests.
- [x] 2. Migrate old palette group overrides losslessly; preserve imported provenance, order and backups.
- [x] 3. Expose palette values and distinct highlighting conditions in the editor; separate imported/user/built-in sections.
- [x] 4. Verify Go/Qt regressions, document the format, build and smoke-test without publishing the real profile.

Go short tests passed for filegroups, theme, settings, nativeui, panel, app and
extui. Qt OperationsQueue: 73 passed, 2 opt-in profiling skips at 175% scale.
Built Go and Qt host; live editor section navigation and cancellation passed.
The real highlight.ini checksum remained unchanged throughout verification.
