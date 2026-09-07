# Issue #677 — solution review

## Findings

1. `cmd/f4/config.go` truncates the live `config.ini` and discards write
   errors. A crash or full filesystem can leave an empty/partial configuration.
2. `cmd/f4/colorer_downloader.go` removes the installed schema tree before the
   downloaded ZIP is validated. Its member path is joined directly, so a
   crafted archive can escape the destination through `..` or an absolute
   path. A failed download also destroys a working installation.
3. `plugins/netfox/vfs.go` ignores JSON and filesystem errors. A damaged file is
   interpreted as an empty database, and the next save can erase all saved
   connections. `netfoxWriter.Close` also saves an empty profile when its JSON
   is invalid.
4. `plugins/visren/config.go` truncates its settings file in place, so a crash
   during save loses the previous preference.
5. `bookmarks.go`, `user_menu_ini.go`, `user_menu_ui.go`, and
   `file_associations.go` all use a predictable `<target>.tmp`. Concurrent
   writers can overwrite one another's temporary file, and a stale temporary
   file can be mistaken for an in-progress save.

## Alternatives

### A — Patch each call site independently

Replace direct writes with `CreateTemp` and rename in every file, and add a
one-off Colorer path check. This is the smallest diff, but it duplicates the
durability rules and makes it likely that the next local settings writer will
reintroduce the same bug.

### B — Shared atomic writer plus staged Colorer installation (selected)

Add a small same-directory atomic writer in `cmd/f4` and reuse it for all
local INI/settings writers. It creates a unique temporary file, writes and
syncs it, closes it, then renames it over the target; failures clean up the
temporary file and leave the old target untouched. Give NetFox and VisRen the
same transaction pattern in their packages. For Colorer, validate every
member path before extraction, extract to a new staging directory, and swap it
with the old tree only after extraction succeeds, retaining a rollback path.

### C — Persistent database / journal for all settings

Move settings and plugin profiles to a journaled store with checksums and
recovery. This would handle concurrent processes more comprehensively, but it
is a large refactor, changes on-disk compatibility, and is explicitly outside
the scope of this ticket.

## Three-pass review of option B

### Pass 1 — correctness

- Same-directory rename gives readers either the old complete file or the new
  complete file, never a truncated file.
- A unique temp name removes the fixed-`.tmp` collision.
- NetFox must fail closed on invalid JSON and invalid writer input; read-only
  listing may fall back to an empty view, but mutating operations must not.
- Colorer must reject absolute paths and `..` components before opening a
  member, and must not touch the installed tree until the staged tree is
  complete.

### Pass 2 — adversarial review

- A ZIP entry cannot escape through a symlink because extraction happens in a
  fresh directory and symlink entries are rejected.
- Cancellation, short writes, close failures, malformed archives, and rename
  failures must remove only staging/temp files and preserve the prior install.
- Existing file modes are intentionally kept compatible for ordinary INI
  files; sensitive plugin configs use `0600`.
- A concurrent save may be last-writer-wins, but every winner is a complete
  serialization and no writer can delete another writer's temp file.

### Pass 3 — regression surface

- Add tests for malformed NetFox JSON preservation, invalid NetFox writer input,
  atomic temp cleanup, Colorer traversal rejection, and failed staged install
  preserving the old catalog.
- Run focused package tests, `go vet`, a race-focused test subset, then the
  full repository tests. Build and run the ARM64 binary locally as a final
  smoke check.

## Decision

Use option B. Keep the on-disk formats unchanged, make failures visible to
callers where the APIs already return errors, and document larger journal/
cross-process coordination ideas without implementing them here.
