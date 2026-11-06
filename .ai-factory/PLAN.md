# Implementation Plan: File groups overview and focused editor

Branch: zoin (existing worktree; no branch changes)
Created: 2026-10-18
Status: implementation in progress

## Original Request

Так, смотри, вообще организация диалога, файловых групп мне вообще не нравится. Всё намешано, четыре экрана настроек, всё подряд. Вообще этим пользоваться невозможно и неудобно. Давай будем исходить из того, что нужно пользователю при первом открытии увидеть самое главное, а именно таблицу с файловыми ассоциациями, они же раскраски, они же иконки. Поэтому основное и главное, что должно быть на этом экране с настройками, — это список. Пусть будет список, несколько колонок, список ассоциаций, ну, вернее, групп наших. В какой-то колонке мы должны сразу показывать расширение или там ещё какие-то маски. То есть колонка должна быть именно: как мы определяем, что надо применить данную группу. Иконка у неё должна быть сразу же слева, если она поаплайнена, если она присутствует. Если нет, то там пустое место. Цвет, если у неё, у этой строчки задан, опять-таки он сразу должен быть, живая превью его, в том числе с курсором, с выделением, позволять надо выделять в нём. То есть, по сути, это эмуляция, такая простенькая панель, то есть должно рендериться полностью по правилам вот этой как раз всей системы. Порядок должен иметь значение, то есть пересортировка вверх-вниз, может поменять, там, итоговую иконку или раскраску. Потом смотри, нужно использовать разделители между глобальными настройками и настройками данной темы. Переключатель тем тоже, да, переключатель нужен обязательно. И если у нас темы динамические, они по-хорошему должны быть динамические. То есть у нас в темах могут быть не только иконки, у нас могут быть ещё и просто разные наборы раскрасок, и это всё дело должно быть полностью настраиваемо через конфиг. Просто мы дефолтные какие-то шипим. По Enter или F4 на группе надо показывать уже отдельный диалог его редактирования Со всеми деталями. Но опять-таки и в нем тоже надо поразумнее организовывать. Допустим, если группа раскраски включается или выключается, у нас это group box, да, то checkbox должен быть прямо в заголовке group box, который его включает и выключает, а не просто от балды. И вообще вот эти всякие детали, мелочи продумать с точки зрения UI/UX, как сделать наиболее удобно.

Clarification:

Переключатель темы в File Groups должен сразу менять тему открытых панелей или только выбирать набор правил для редактирования и предпросмотра?

Только набор для редактирования и предпросмотра

## Settings

- Mode: fast, branchless planning; preserve all existing dirty work.
- Testing: yes, required by repository rules.
- Logging: standard; opt-in VTUI_DEBUG at draft, theme-context and save boundaries;
  never log commands, user filenames or per-painted-row evaluation.
- Docs: yes; update FILE_GROUPS, SETTINGS_CENTER and ExtUI extension documentation.
- No commit, push, PR or application/profile modifications in this planning task.

## Evidence and ownership

- `internal/settings/collection_ui.go:addCollections` creates a five-row,
  single-column table, then appends the selected record's fields inline.
- `internal/settings/file_groups_ui.go:addFileGroupTools` appends reset controls,
  samples, match output and actions onto the same scrollable page.
- `fileGroupRecordLabel` mixes scope, source, feature flags and name in one cell.
- `internal/filegroups/catalog.go:ParseContribution` reads icon catalogs from INI;
  `IconTheme` descriptors currently contain only ID/name/assets/fallbacks.
- `Snapshot.ForIconTheme` already supports arbitrary scope IDs, universal-first
  ordering and compiled predicate reuse. The persisted field `IconTheme` scopes
  all features despite its historical name. Keep that storage compatibility.
- `internal/theme/file_groups.go:LoadFileGroups` and `SemanticStyle` already
  provide the real cascade and four-state appearance. Current text-only Test
  match builds these explicitly; the overview must consume a retained snapshot.
- `sdk/extui/model.go:ControlModel.ItemIcons` is Lucide-name-only and is not a
  substitute for theme icon references or four-state row appearance.
- `settings` owns interaction/drafts, `filegroups` owns policy and configuration,
  `theme` owns colour rendering, Qt owns artwork/native lookup. App only wires.

## UX contract

### Main page: overview first

The page has a compact theme/search toolbar, one dominant expanding table, a
compact action bar, and the existing Settings Apply/OK/Cancel footer. No group
properties, command editors, reset-field selectors or multiline match dumps
appear below the list. Avoid outer page scrolling; only the table scrolls.

Suggested columns, left to right:

1. Icon + sample filename (live appearance and marker).
2. Group name.
3. Match condition: masks plus concise attribute/size/date restrictions.
4. Enabled features: Highlight / Icons / Sort / Associations indicators.
5. Source/status: Built-in / Modified / User / Disabled.

Use labelled nonselectable separators for **Universal** and **Theme: <name>**.
Universal rules appear first, then the chosen theme's rules. Empty sections
show a lightweight empty state; no catch-all row or fake group is inserted.
Table headers do not sort executable rules: visible order is evaluation order.
Search filters/highlights rows without altering stored order or precedence.
Condition summaries expose full text on demand, not only an extension.

The icon gutter keeps its width when no effective icon exists. This preview
does not invent a generic icon outside the normal resolver. Disabled groups
remain editable and visibly disabled without contributing rules.

Preview marking is temporary UI state, keyed by group ID: Space/Insert and
Shift ranges exercise normal/selected/cursor/selected-cursor states, without
changing Enabled or modifying real panel marks. Keep one independent cursor.
Separate source/status cells remain legible if user appearance is low contrast.

Actions: Add, Edit, Duplicate, Delete/Disable, Move up/down, More. Enter/F4 and
double-click on a data row open the group editor. Separators cannot be opened,
marked or reordered. Reordering is limited to one scope, refreshes resolved
preview, and preserves cursor/marks/scroll by stable ID. Moving to another scope
is an explicit editor operation, not an accidental drag across a divider.

### Real preview, not a swatch

Each row has a concrete sample VFSItem and known-metadata flags. Evaluate it
against the entire active ordered universal + selected-theme snapshot, using
the same colour cascade, UseDefaults, continuation, sort and icon precedence
as panels. Do not simply paint the group's own NormalColor.

Provide a representative filename for straightforward extension/name masks;
retain explicit sample metadata for complex predicates. For ambiguous regex,
negative masks, date/attribute/size conditions, show a nonmatching/deferred badge
and allow editing a sample rather than pretending one can always be generated.
Use a small expandable inspector for the selected row: sample, contributing
group IDs per feature and effective output. A later overriding group or a prior
universal match must be visible in that explanation. Never execute associations
or load directory contents to produce a sample.

Compile once per valid draft revision, evaluate outside paint, refresh only
affected preview revisions. Reuse existing bounded artwork/native-icon caches;
native OS lookup only when the preview theme selects the Native catalog.

### Theme sets and isolated editor context

The selector changes the **editing/preview theme only**, not the application's
saved theme, open-panel theme or committed association context. Drafts for all
themes remain in memory. Changing the selector neither saves nor discards them.
The current application theme can be identified in the selector without being
forced to follow the editing context.

Introduce declarative `[FileGroupTheme.<stable-id>]` descriptors in the same
layered document: Name and optional IconCatalog (a catalog ID). Built-ins have
stable IDs generic/lucide/zoin/system; defaults ship as INI, user overrides and
additional theme sets stay profile-local. Theme sets can contain highlighting,
icons, sorting and associations, including colour-only sets. Several sets may
reuse one artwork catalog; a colour-only set needs no artwork catalog. Absence
of an IconCatalog must not silently borrow another theme's matching rules.
This is not the application's global UI colour-palette editor.

Keep existing v3 groups' `IconTheme` as the scope key for compatibility; expose
it as Theme in UI. Keep Icon.<catalog> resource choices separate from scope.
Read pre-descriptor profiles by materializing known descriptors in memory;
do not rewrite on startup. Unknown theme IDs are preserved and surfaced with
their stable ID/unavailable-catalog state. Explicit fallback artwork does not
activate other themes' groups. Descriptor layering/reset, unknown fields,
comments, atomic saves, external edit checks and backup policy match groups.
Validate format-version needs before publishing new semantics; do not silently
alter the meaning of files readable by an older executable.

Qt and terminal use a shared theme-set registry; frontend catalogs are optional
capabilities, not a hardcoded UI enum. Theme management in More supports creating
an empty set, duplicating a set with fresh rule/command IDs, renaming its label,
and removing/disabling a set with an explicit affected-groups confirmation.
Deleting the application's active theme is not an implicit theme switch: prevent
it while active and explain why. Keep unknown unavailable sets editable.

While browsing another theme, preview changes for that set stay in the table;
edits to universal/application-active rules may retain existing reversible panel
appearance preview. Switching the preview selector never switches real panels.
Associations always remain committed-only until Settings Apply.

### Separate group editor

Open a modal child editor with Name, Enabled and Scope at the top and a compact
live sample. Use a small section navigator/tabs: Conditions, Appearance,
Sorting, Associations. Do not place every expanded section on one tall form.
Navigation/control state survives switching sections.

- Conditions: masks first; syntax/case next; advanced attribute, size and date
  predicates in an expandable section. Unknown/imported conditions remain editable.
- Appearance: Highlighting and Icons are distinct group boxes with the enabling
  checkbox in each box's actual title. Highlighting offers a four-state matrix,
  expression editing, swatches, marker and advanced continuation/UseDefaults.
  Icons offers catalog/reference, preview, picker, native override and inheritance.
- Sorting: its own checkable group box, enabled number control and explanation
  of unclassified/directory behavior.
- Associations: checkable group box; named command-set list and focused editor
  with six independently enabled slots. Commands are never executed here.
- Disabled feature bodies are dimmed/noninteractive but keep draft values.
  Inherited/overridden/explicitly empty values remain distinguishable. Reset is
  adjacent to its field/feature; destructive group reset is in More with context.
  Enabled booleans do not replace the separate inheritance/reset operation.
- Use consistent gutters, label/control alignment and keyboard mnemonics.
  Header checkboxes must be real accessible controls, not drawn glyphs.

Child OK validates and merges its scratch copy into the parent's draft; no disk
save. Child Cancel/Escape reverts only this child's edits and preview, retaining
prior parent edits. Settings Apply saves the complete draft once, after whole-
document validation, then advances the baseline. Settings Cancel/Escape/close
reverts unapplied changes across all themes. Child Enter on lists opens their
editor; Enter inside multiline command input remains newline. F4/Enter on the
overview never accidentally trigger Settings Apply. Up/Down stay in the active
editor/list; Tab/Shift+Tab explicitly move between panes.

## Tasks

### Phase 1: Contracts and declarative themes

- [x] T1: Add layered theme-set descriptors, registry and isolated editor context
  in `internal/filegroups` and `internal/settings`; catalog references are separate
  from rule scope. Seed descriptors from INI defaults/contributions and migrate
  existing profiles in memory. Add duplicate/reset/unknown-catalog/version tests.
  Log registry changes and validation failures through VTUI_DEBUG only.
- [x] T2 (depends on T1): Introduce retained row-preview projections in settings
  using compiled snapshots and theme.FileHighlighter. Include all four colour
  states, effective icon provenance, sample metadata and per-feature contributors.
  Extend typed optional ExtUI table-row presentation fields using existing style
  types; old clients keep ordinary cells. Test row/icon/style wire round trips.
  Log compile revisions/errors, never per-frame row work or sample contents.

### Phase 2: Go-owned overview and child transactions

- [x] T3 (depends on T1, T2): Replace only File groups' generic inline collection
  rendering with a dedicated overview (`internal/settings/file_groups_overview.go`).
  Add real columns, scope dividers, full-height list, keyboard marking, search,
  compact actions and stable-ID anchors. No change to other collection pages.
  Test boundaries, Enter/F4, reordering effects, marking and empty/filtered lists.
  Log navigation/reorder identity boundaries without command/sample text.
- [x] T4 (depends on T3): Add child editor and scratch transaction in settings
  (`file_groups_editor.go` and `file_groups_preview.go` as needed). Factor reusable
  predicate/feature controls out of the old inline flow without changing storage.
  Implement section navigation, nested command sets, contextual resets and real
  checkbox-in-title semantics via a reusable frontend-neutral control contract.
  Test child OK/Cancel, parent Apply/Cancel, invalid drafts and failed saves.
  Log transaction transitions/validation errors, not user command values.
- [ ] T5 (depends on T3, T4): Implement theme-set management and sample inspector;
  retain hidden drafts and editor theme context through rebuilds. Verify global
  application theme and committed associations never follow the selector. Test
  universal priority, native/plugin flag, all six slots, external file edits and
  active-set delete safeguards. Log theme IDs and transaction revisions only.

### Phase 3: Qt rendering and end-to-end verification

- [x] T6 (depends on T2, T4): Render optional file-group table row appearance in
  `qt/host/qml/SemanticTable*.qml`/actual owning table delegate, resolve typed icon
  references through the central panel icon resolver, and add reusable checkable
  group headers in semantic rendering. Export via `sdk/extui/model.go` and Go's
  semantic adapter; no QML filename matching, command policy or INI parsing.
  Test blank icon gutters, aspect ratio, DPR and all four colour states. Gate
  diagnostic output with existing host tracing, no frame-by-frame logs.
- [ ] T7 (depends on T5, T6): Test terminal-only with CGO_ENABLED=0; Qt at 100% and
  175%; narrow window; key repeat; touchpad scrolling while hover/help updates;
  selection/scroll anchoring after row edits and theme changes. Add a 10k-rule/
  entry workload appropriate to the affected path to assert no paint-time mask
  compilation, file reads, repeated full catalog transmission or row rebuilds.
  Log timing summaries only in opt-in test/performance runs.
- [ ] T8 (depends on T7): Update format/editor/ExtUI docs and managed INI help;
  verify default appearance, associations, selected app theme and contributor
  UseDefaults/Symlink/Junction/Hidden/tree behavior remain unchanged. Build and
  smoke-test the application without Apply on the user's real profile; use an
  isolated profile for persistence cases. Record actual checks and limitations.
  Log migration/save diagnostics through existing configurable channels.

## Verification checkpoint (2026-10-18)

Implemented: independent declarative theme sets and editor context; dominant
overview with stable-ID rows, evaluated four-state samples and marking;
separate scratch child editor; checkboxes in feature headers; adjacent resets;
theme management and sample/cached-panel inspector; optional Qt contracts and
independent preview catalog resolver. No profile Apply or commit/push performed.

Passed: CGO_ENABLED=0 short tests for settings, filegroups, nativeui, theme,
i18n, cmd/f4, app and panel; go vet for changed Go packages; 10,000-entry scoped
appearance refresh without directory reload. Qt overview/header checks pass at
100%/175%; settings scrolling/identity/resize checks pass at 175%; icon suite
38 passed, 1 desktop-only test skipped on offscreen. Go and video/FFmpeg Qt host
build successfully. Live overview/child smoke used an isolated portable profile.

At the initial checkpoint, the child retained the
existing expression controls rather than the planned dedicated colour matrix
and pinned live sample; fine-grained inherit/override/clear status badges and
arbitrary sample metadata editing need further polish. The full Qt operations
suite has 70 passing tests, 1 failing search-dialog workspace-activation test
and 2 opt-in skips; the failure also reproduces alone at 100%. Its cause has not
been attributed to this change. Docs and managed help described implemented UX.

## Follow-up: compact matrix, pinned sample and Qt regression

Requested follow-up implemented: four inline expression rows with aligned labels,
effective colour swatches and resets; pinned four-state sample outside the
scrolling property page, retained across section changes. The sample includes
base palette and complete icon provenance; no compilation during paint. Invalid
new colour expressions and invalid masks retain the last valid projection.
Legacy stored colour expressions remain tolerant unless edited.

The failed Qt test clicked an offscreen clipped tab (isVisible was insufficient).
It now reveals the tab, verifies viewport/window containment and retains its
workspace activation assertion for both viewer and editor. No production input
routing changes were needed. Regression passes at 100% and 175%.

Validation: CGO_ENABLED=0 short settings/theme/nativeui/filegroups/i18n/cmd tests;
go vet; complete Qt Operations suite 72 passed, 0 failed, 2 opt-in skips at 175%;
matrix/pinned sample and search regression also pass at 100%. Real Qt smoke
confirmed matrix, sample and section navigation without Apply. Existing docs
updated using aif-docs. Go and Qt host rebuilt; no commit/push.

Other optional refinements (arbitrary sample metadata editing and explicit
inherit/override/clear badges) remain outside this follow-up; T5 stays unchecked.

## Commit Plan

Implementation checkpoints only; commits/push/PR are not authorized by this plan:

1. T1-T2: `feat: add declarative file-group themes and preview contracts`.
2. T3-T5: `feat: replace inline file-group settings with overview and editor`.
3. T6-T8: `feat(qt): render file-group previews and checkable section headers`.

## Acceptance

- Opening File groups immediately shows a large useful table, not editable fields.
- Conditions are readable; icons/colour states are real evaluated output and
  respond correctly to reorder and overlap. No command executes in preview.
- Universal/theme divider rows and selection are unambiguous and keyboard-safe.
- Theme selection edits/previews that set only, without changing application theme.
- Config-defined colour-only and shared-catalog themes work without a code enum.
- Group details open by Enter/F4, with independent child cancellation and properly
  placed enabling checkboxes. No feature, unknown value or inheritance is lost.
- Fast scrolling does not reset position; vertical navigation does not escape.
- Existing colors, associations, profile data and frontend boundaries are retained.
