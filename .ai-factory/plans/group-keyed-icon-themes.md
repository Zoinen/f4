# Group-keyed icon themes

Docs: yes

## Scope

Icon themes contain artwork keyed by normalized shared group names, not their
own filename rules. Preserve user overrides, command slots, native priority and
colour cascades. Create ordinary universal groups for unmatched artwork.

## Tasks

- [x] 1. Replace icon classification contributions with shared groups and group-keyed catalogs; rename file artwork without touching chrome.
- [x] 2. Resolve implicit group artwork in compiled Go snapshots; migrate old factory icon rules safely, preserving edited groups and profile drafts.
- [x] 3. Use the same group-keyed resolver in Qt and the editor; hide inactive colour-style contributions.
- [x] 4. Add regression coverage, document the convention, build and smoke-test without publishing a profile or committing.
