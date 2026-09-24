# Project Structure

Every file tracked in the repository. Regenerate with
`scripts/filelist_update.sh` after adding or moving files.

    .
    ├── .agents
    │   ├── agents
    │   │   ├── best-practices-sidecar.md
    │   │   ├── commit-preparer.md
    │   │   ├── docs-auditor.md
    │   │   ├── implement-coordinator.md
    │   │   ├── implement-worker.md
    │   │   ├── loop-critic.md
    │   │   ├── loop-evaluator.md
    │   │   ├── loop-invariant-prep.md
    │   │   ├── loop-orchestrator.md
    │   │   ├── loop-perf-prep.md
    │   │   ├── loop-planner.md
    │   │   ├── loop-producer.md
    │   │   ├── loop-refiner.md
    │   │   ├── loop-test-prep.md
    │   │   ├── plan-coordinator.md
    │   │   ├── plan-polisher.md
    │   │   ├── review-sidecar.md
    │   │   ├── rules-sidecar.md
    │   │   └── security-sidecar.md
    │   └── skills
    │       ├── aif
    │       │   ├── references
    │       │   │   ├── config-template.yaml
    │       │   │   └── update-config.mjs
    │       │   └── SKILL.md
    │       ├── aif-architecture
    │       │   ├── references
    │       │   │   └── architecture.md
    │       │   └── SKILL.md
    │       ├── aif-archive
    │       │   └── SKILL.md
    │       ├── aif-best-practices
    │       │   └── SKILL.md
    │       ├── aif-build-automation
    │       │   ├── references
    │       │   │   ├── BEST-PRACTICES.md
    │       │   │   ├── DOC-INTEGRATION.md
    │       │   │   └── SUMMARY-FORMAT.md
    │       │   ├── SKILL.md
    │       │   └── templates
    │       │       ├── justfile-go
    │       │       ├── justfile-gradle
    │       │       ├── justfile-maven
    │       │       ├── justfile-node
    │       │       ├── justfile-php
    │       │       ├── justfile-python
    │       │       ├── justfile-ruby
    │       │       ├── justfile-rust
    │       │       ├── magefile-basic.go
    │       │       ├── magefile-full.go
    │       │       ├── makefile-go.mk
    │       │       ├── makefile-gradle.mk
    │       │       ├── makefile-maven.mk
    │       │       ├── makefile-node.mk
    │       │       ├── makefile-php.mk
    │       │       ├── makefile-python.mk
    │       │       ├── makefile-ruby.mk
    │       │       ├── makefile-rust.mk
    │       │       ├── taskfile-go.yml
    │       │       ├── taskfile-gradle.yml
    │       │       ├── taskfile-maven.yml
    │       │       ├── taskfile-node.yml
    │       │       ├── taskfile-php.yml
    │       │       ├── taskfile-python.yml
    │       │       ├── taskfile-ruby.yml
    │       │       └── taskfile-rust.yml
    │       ├── aif-ci
    │       │   ├── references
    │       │   │   ├── AUDIT-REPORT.md
    │       │   │   ├── BEST-PRACTICES.md
    │       │   │   ├── GITLAB-PATTERNS.md
    │       │   │   ├── SERVICE-CONTAINERS.md
    │       │   │   └── TOOL-COMMANDS.md
    │       │   ├── SKILL.md
    │       │   └── templates
    │       │       ├── github
    │       │       │   ├── go.yml
    │       │       │   ├── java.yml
    │       │       │   ├── node.yml
    │       │       │   ├── php.yml
    │       │       │   ├── python.yml
    │       │       │   └── rust.yml
    │       │       └── gitlab
    │       │           ├── go.yml
    │       │           ├── java.yml
    │       │           ├── node.yml
    │       │           ├── php.yml
    │       │           ├── python.yml
    │       │           └── rust.yml
    │       ├── aif-commit
    │       │   └── SKILL.md
    │       ├── aif-docs
    │       │   ├── references
    │       │   │   └── REVIEW-CHECKLISTS.md
    │       │   ├── SKILL.md
    │       │   └── templates
    │       │       └── html-template.html
    │       ├── aif-evolve
    │       │   └── SKILL.md
    │       ├── aif-explore
    │       │   ├── references
    │       │   │   └── ULTRA-RESEARCH-FORMAT.md
    │       │   └── SKILL.md
    │       ├── aif-fix
    │       │   └── SKILL.md
    │       ├── aif-grounded
    │       │   └── SKILL.md
    │       ├── aif-implement
    │       │   ├── references
    │       │   │   ├── IMPLEMENTATION-GUIDE.md
    │       │   │   └── LOGGING-GUIDE.md
    │       │   └── SKILL.md
    │       ├── aif-improve
    │       │   ├── references
    │       │   │   ├── CHECK-MODE.md
    │       │   │   ├── EXAMPLES.md
    │       │   │   ├── LIST-MODE.md
    │       │   │   └── VALIDATOR.md
    │       │   └── SKILL.md
    │       ├── aif-loop
    │       │   ├── references
    │       │   │   ├── ACTIVE-TIME-BUDGET.md
    │       │   │   ├── CONTEXT-MANAGEMENT.md
    │       │   │   ├── CRITERIA-TEMPLATES.md
    │       │   │   ├── PHASE-CONTRACTS.md
    │       │   │   ├── RULE-SCHEMA.md
    │       │   │   └── TERMINAL-REPORT.md
    │       │   └── SKILL.md
    │       ├── aif-plan
    │       │   ├── references
    │       │   │   ├── EXAMPLES.md
    │       │   │   ├── TASK-FORMAT.md
    │       │   │   └── ULTRA-FORMAT.md
    │       │   └── SKILL.md
    │       ├── aif-qa
    │       │   ├── references
    │       │   │   ├── CHANGE-SUMMARY.md
    │       │   │   ├── TEST-CASES.md
    │       │   │   └── TEST-PLAN.md
    │       │   ├── SKILL.md
    │       │   └── templates
    │       │       ├── CHANGE-SUMMARY.md
    │       │       ├── TEST-CASES.md
    │       │       └── TEST-PLAN.md
    │       ├── aif-qa-check
    │       │   ├── SKILL.md
    │       │   └── templates
    │       │       └── QA-CHECK.md
    │       ├── aif-review
    │       │   ├── references
    │       │   │   ├── CHECK-MODE.md
    │       │   │   ├── SEVERITY.md
    │       │   │   └── VALIDATOR.md
    │       │   └── SKILL.md
    │       ├── aif-roadmap
    │       │   └── SKILL.md
    │       ├── aif-rules
    │       │   └── SKILL.md
    │       ├── aif-rules-check
    │       │   ├── references
    │       │   │   └── RULES-CHECK-CONTRACT.md
    │       │   └── SKILL.md
    │       ├── aif-security-checklist
    │       │   ├── references
    │       │   │   ├── AUTH-PATTERNS.md
    │       │   │   ├── PROMPT-INJECTION.md
    │       │   │   └── RACE-CONDITIONS.md
    │       │   ├── scripts
    │       │   │   └── audit.sh
    │       │   └── SKILL.md
    │       ├── aif-skill-generator
    │       │   ├── references
    │       │   │   ├── BEST-PRACTICES.md
    │       │   │   ├── EXAMPLES.md
    │       │   │   ├── LEARN-MODE.md
    │       │   │   ├── SECURITY-SCANNING.md
    │       │   │   └── SPECIFICATION.md
    │       │   ├── scripts
    │       │   │   ├── cleanup-blocked-skill.py
    │       │   │   ├── search-skills.py
    │       │   │   ├── security-scan.py
    │       │   │   └── validate.sh
    │       │   ├── SKILL.md
    │       │   └── templates
    │       │       ├── basic.md
    │       │       ├── dynamic-context.md
    │       │       ├── research.md
    │       │       ├── task.md
    │       │       └── visual.md
    │       ├── aif-verify
    │       │   ├── references
    │       │   │   ├── CONTEXT-GATES-AND-OWNERSHIP.md
    │       │   │   └── GATE-RESULT-CONTRACT.md
    │       │   └── SKILL.md
    │       ├── golang-code-style
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   └── details.md
    │       │   └── SKILL.md
    │       ├── golang-concurrency
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── channels-and-select.md
    │       │   │   ├── pipelines.md
    │       │   │   └── sync-primitives.md
    │       │   └── SKILL.md
    │       ├── golang-context
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── cancellation.md
    │       │   │   ├── http-services.md
    │       │   │   └── values-tracing.md
    │       │   └── SKILL.md
    │       ├── golang-database
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── performance.md
    │       │   │   ├── scanning.md
    │       │   │   ├── testing.md
    │       │   │   └── transactions.md
    │       │   └── SKILL.md
    │       ├── golang-design-patterns
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── architecture.md
    │       │   │   ├── clean-architecture.md
    │       │   │   ├── data-handling.md
    │       │   │   ├── ddd.md
    │       │   │   ├── hexagonal-architecture.md
    │       │   │   └── resource-management.md
    │       │   └── SKILL.md
    │       ├── golang-documentation
    │       │   ├── assets
    │       │   │   └── templates
    │       │   │       ├── CHANGELOG.md
    │       │   │       ├── CONTRIBUTING.md
    │       │   │       ├── llms.txt
    │       │   │       └── README.md
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── application.md
    │       │   │   ├── code-comments.md
    │       │   │   ├── library.md
    │       │   │   └── project-docs.md
    │       │   └── SKILL.md
    │       ├── golang-error-handling
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── error-creation.md
    │       │   │   ├── error-handling.md
    │       │   │   └── error-wrapping.md
    │       │   └── SKILL.md
    │       ├── golang-naming
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── functions-methods.md
    │       │   │   ├── identifiers.md
    │       │   │   ├── packages-files.md
    │       │   │   ├── testing.md
    │       │   │   └── types-errors.md
    │       │   └── SKILL.md
    │       ├── golang-performance
    │       │   ├── assets
    │       │   │   └── prometheus-alerts.yml
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── caching.md
    │       │   │   ├── cpu.md
    │       │   │   ├── io-networking.md
    │       │   │   ├── memory.md
    │       │   │   ├── observability.md
    │       │   │   └── runtime.md
    │       │   └── SKILL.md
    │       ├── golang-security
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   ├── references
    │       │   │   ├── architecture.md
    │       │   │   ├── checklist.md
    │       │   │   ├── cookies.md
    │       │   │   ├── cryptography.md
    │       │   │   ├── filesystem.md
    │       │   │   ├── injection.md
    │       │   │   ├── logging.md
    │       │   │   ├── memory-safety.md
    │       │   │   ├── network.md
    │       │   │   ├── secrets.md
    │       │   │   ├── third-party.md
    │       │   │   └── threat-modeling.md
    │       │   └── SKILL.md
    │       └── golang-testing
    │           ├── evals
    │           │   └── evals.json
    │           ├── references
    │           │   ├── benchmarks.md
    │           │   ├── coverage.md
    │           │   ├── examples.md
    │           │   ├── helpers.md
    │           │   ├── http-testing.md
    │           │   ├── integration-testing.md
    │           │   └── mocking.md
    │           └── SKILL.md
    ├── .ai-factory
    │   ├── ARCHITECTURE.md
    │   ├── config.yaml
    │   ├── DESCRIPTION.md
    │   ├── patches
    │   │   ├── 2026-09-10-23.51.md
    │   │   ├── 2026-09-11-00.25.md
    │   │   ├── 2026-09-11-02.03.md
    │   │   ├── 2026-09-11-16.18.md
    │   │   ├── 2026-09-11-18.00.md
    │   │   ├── 2026-09-11-23.59.md
    │   │   ├── 2026-09-11-adaptive-settings-fields.md
    │   │   ├── 2026-09-11-choice-caption-width.md
    │   │   ├── 2026-09-11-menu-press-drag.md
    │   │   ├── 2026-09-11-multiline-semantic-inputs.md
    │   │   ├── 2026-09-11-settings-help-alignment.md
    │   │   ├── 2026-09-12-00.25.md
    │   │   ├── 2026-09-12-00.48.md
    │   │   ├── 2026-09-12-01.03.md
    │   │   ├── 2026-09-12-02.10.md
    │   │   ├── 2026-09-12-03.00.md
    │   │   ├── 2026-09-12-12.41.md
    │   │   ├── 2026-09-12-13.30.md
    │   │   ├── 2026-09-12-14.30.md
    │   │   ├── 2026-09-12-18.50.md
    │   │   ├── 2026-09-12-column-text-pixel-grid.md
    │   │   ├── 2026-09-12-console-paste.md
    │   │   ├── 2026-09-12-device-breadcrumb-labels.md
    │   │   ├── 2026-09-12-device-qualified-paths.md
    │   │   ├── 2026-09-12-f9-path-chrome.md
    │   │   ├── 2026-09-12-menu-cursor-lifecycle.md
    │   │   ├── 2026-09-12-panel-refocus.md
    │   │   ├── 2026-09-12-quick-search-pink.md
    │   │   ├── 2026-09-12-terminal-short-output.md
    │   │   ├── 2026-09-13-ai-qualified-paths.md
    │   │   ├── 2026-09-13-connection-dialog.md
    │   │   ├── 2026-09-13-folder-history-dates.md
    │   │   ├── 2026-09-13-history-columns-hover.md
    │   │   ├── 2026-09-13-history-first-visible-position.md
    │   │   ├── 2026-09-13-history-keyboard-scroll.md
    │   │   ├── 2026-09-13-history-viewport.md
    │   │   ├── 2026-09-13-macos-traffic-light-startup.md
    │   │   ├── 2026-09-13-menu-label-quotes.md
    │   │   ├── 2026-09-13-native-menu-decoration.md
    │   │   ├── 2026-09-14-command-line-empty-baseline.md
    │   │   ├── 2026-09-14-gallery-fullscreen-chrome.md
    │   │   ├── 2026-09-14-gallery-viewport-handoffs.md
    │   │   ├── 2026-09-14-macos-native-button-relayout.md
    │   │   ├── 2026-09-14-panel-sort-cursor-anchor.md
    │   │   ├── 2026-09-15-23.08.md
    │   │   ├── 2026-09-15-middle-click-tabs.md
    │   │   ├── 2026-09-16-00.08.md
    │   │   ├── 2026-09-16-02.21.md
    │   │   ├── 2026-09-16-03.03.md
    │   │   ├── 2026-09-16-04.24.md
    │   │   ├── 2026-09-16-23.54.md
    │   │   ├── 2026-09-16-folder-preview-resume.md
    │   │   ├── 2026-09-16-folder-style.md
    │   │   ├── 2026-09-17-01.37.md
    │   │   ├── 2026-09-17-02.01.md
    │   │   ├── 2026-09-17-02.35.md
    │   │   ├── 2026-09-17-23.35.md
    │   │   ├── 2026-09-17-wheel-zoom-burst.md
    │   │   ├── 2026-09-18-console-dialog-minimum-size.md
    │   │   ├── 2026-09-18-drag-cursor-acknowledgement.md
    │   │   ├── 2026-09-18-drag-release-position.md
    │   │   ├── 2026-09-18-envman-checkboxes.md
    │   │   ├── 2026-09-18-envman-key-hints.md
    │   │   ├── 2026-09-18-envman-name-row.md
    │   │   ├── 2026-09-18-envman-native-body.md
    │   │   ├── 2026-09-18-envman-resize-capture.md
    │   │   ├── 2026-09-18-list-cursor-repeat.md
    │   │   ├── 2026-09-18-live-selection-status.md
    │   │   ├── 2026-09-18-overlay-stack-order.md
    │   │   ├── 2026-09-18-palette-input.md
    │   │   ├── 2026-09-18-panel-status-width.md
    │   │   ├── 2026-09-18-quick-search-status-placement.md
    │   │   ├── 2026-09-18-selection-shortcuts.md
    │   │   ├── 2026-09-18-settings-scrollbar-gutter.md
    │   │   ├── 2026-09-18-shared-footer-actions.md
    │   │   ├── 2026-09-18-unified-native-settings.md
    │   │   └── 2026-09-19-drive-menu-text-layout.md
    │   ├── plans
    │   │   ├── feature-restructure-into-internal-packages
    │   │   │   ├── HANDOFF.md
    │   │   │   ├── index.md
    │   │   │   ├── phase-01-baseline-and-barriers.md
    │   │   │   ├── phase-02-repository-root.md
    │   │   │   ├── phase-03-subsystems.md
    │   │   │   ├── phase-04-shared-primitives.md
    │   │   │   ├── phase-05-leaf-packages.md
    │   │   │   ├── phase-06-hosts-and-services.md
    │   │   │   ├── phase-07-view-and-terminal.md
    │   │   │   ├── phase-08-fileops-and-editor.md
    │   │   │   ├── phase-09-panel-and-cmdline.md
    │   │   │   ├── phase-10-composition-root.md
    │   │   │   ├── phase-11-ci-and-docs.md
    │   │   │   └── PR-BODY.md
    │   │   ├── gui-settings-integration.md
    │   │   ├── zoin_branch-gallery-quick-view.md
    │   │   ├── zoin-folder-preview-first-frame.md
    │   │   ├── zoin-folder-previews.md
    │   │   └── zoin-gallery-cache-settings.md
    │   └── rules
    │       └── base.md
    ├── .ai-factory.json
    ├── .gitattributes
    ├── .github
    │   ├── actions
    │   │   ├── affected-packages
    │   │   │   └── action.yml
    │   │   └── shard-packages
    │   │       └── action.yml
    │   ├── assets
    │   │   └── screenshot.png
    │   ├── codecov.yml
    │   └── workflows
    │       ├── build.yml
    │       ├── go-cache-salt
    │       └── quick.yml
    ├── .gitignore
    ├── .gitmodules
    ├── .golangci-strict.yml
    ├── .golangci.yml
    ├── .mcp.json
    ├── AGENTS.md
    ├── artifacts
    │   ├── native-openconsole-probe-static.json
    │   ├── native-openconsole-probe-static.json.sessions
    │   │   └── 80x25.raw
    │   ├── native-openconsole-probe.json
    │   ├── native-openconsole-probe.json.sessions
    │   │   ├── 121x40.raw
    │   │   ├── 1x1.raw
    │   │   └── 80x25.raw
    │   └── README.md
    ├── assets
    │   └── icon
    │       └── AppIcon.icon
    │           ├── Assets
    │           │   ├── f4-content.svg
    │           │   └── f4-shell.svg
    │           └── icon.json
    ├── ci
    │   ├── audit-portable-qt-linux.sh
    │   ├── audit-portable-qt-windows.ps1
    │   ├── audit-static-go-linux.sh
    │   ├── build-portable-qt-linux.sh
    │   ├── build-qwindowkit.ps1
    │   ├── build-qwindowkit.sh
    │   ├── package-embedded-qt-host.py
    │   ├── patch-qt-qmltools-recipe.py
    │   ├── patches
    │   │   └── qwindowkit-default-maximize-hint.patch
    │   └── remove-elf-interpreter.py
    ├── cmd
    │   └── f4
    │       ├── architecture_test.go
    │       ├── command_palette_coverage_test.go
    │       ├── frame_manager_capture_test.go
    │       ├── go2xp_shim.go
    │       ├── hardcoded_strings_test.go
    │       ├── main.go
    │       ├── rsrc_windows_amd64.syso
    │       ├── rsrc_windows_arm64.syso
    │       └── winescape_gate_test.go
    ├── docs
    │   ├── COLORS.md
    │   ├── CONPTY_FUTURE_IDEAS.md
    │   ├── CONPTY_GATE_REQUIREMENTS.md
    │   ├── CONPTY_GATE_STATUS.md
    │   ├── CONPTY_LINE_WRAP_FINDINGS.md
    │   ├── CONPTY_NATIVE_AGENT.md
    │   ├── CONPTY_NATIVE_AUDIT.md
    │   ├── CONPTY_NATIVE_PROBE.md
    │   ├── CONPTY_NATIVE_TEST.md
    │   ├── CONSOLE_MODES.md
    │   ├── CURSOR.md
    │   ├── DRAGDROP.md
    │   ├── FFI.md
    │   ├── FISH_PLUS_S2S.md
    │   ├── FISH+.md
    │   ├── FUSE.md
    │   ├── HIGHLIGHT.md
    │   ├── HIGHLIGHTING.md
    │   ├── HISTORY_IMPORT.md
    │   ├── HISTORY_PERFORMANCE.md
    │   ├── I18N.md
    │   ├── IDEAS.md
    │   ├── IMAGES_PLAN.md
    │   ├── issue-561-solutions.md
    │   ├── ISSUES
    │   │   ├── ISSUE_1179_7Z_SFX_DETECTION_AND_TESTING.md
    │   │   ├── ISSUE_1184_DOCUMENTS_OPEN_AS_ARCHIVES.md
    │   │   ├── ISSUE_165_CONPTY_SYNC_MARKER.md
    │   │   ├── ISSUE_215_WINDOWS_UPDATE_ELEVATION.md
    │   │   ├── ISSUE_247_QUEUE_PROGRESS_THROTTLE.md
    │   │   ├── ISSUE_248_PTY_REDRAW_COALESCING.md
    │   │   ├── ISSUE_249_MC_CTRL_O.md
    │   │   ├── ISSUE_260_SUDO_ARGUMENT_INHERITANCE.md
    │   │   ├── ISSUE_261_COLOR_AREAS.md
    │   │   ├── ISSUE_262_REMOTE_VFS_PATHS.md
    │   │   ├── ISSUE_264_UPDATE_CHECK_VERSIONS.md
    │   │   ├── ISSUE_266_MULTI_SELECTION_ATTRIBUTES.md
    │   │   ├── ISSUE_277_COLORER_FEATURES.md
    │   │   ├── ISSUE_278_SORT_ORDER_COLLATION.md
    │   │   ├── ISSUE_309_PLUGRING_ASSET_URLS.md
    │   │   ├── ISSUE_397_WINDOWS_CONSOLE_LAYOUT_CRASH.md
    │   │   ├── ISSUE_411_HOTKEY_DIALOG_COMMIT.md
    │   │   ├── ISSUE_453_OVERWRITE_DIALOG_LOCALIZATION.md
    │   │   ├── ISSUE_492_RIGHT_CTRL_BINDINGS.md
    │   │   ├── ISSUE_493_HOTKEY_LIST_SELECTION.md
    │   │   ├── ISSUE_511_MULTIPLEXER_CHORDS.md
    │   │   ├── ISSUE_523_GZIP_LOGICAL_FILE.md
    │   │   ├── ISSUE_526_COMMAND_LINE_QUOTING.md
    │   │   ├── ISSUE_546_VIEWER_GRAPHEME_CLUSTERS_FOLLOWUP.md
    │   │   ├── ISSUE_546_VIEWER_GRAPHEME_CLUSTERS.md
    │   │   ├── ISSUE_601_STARTUP_BACKEND_SELECTION.md
    │   │   ├── ISSUE_606_FUSE_LOCK_GRANULARITY.md
    │   │   ├── ISSUE_608_COPY_DIALOG_LAYOUT.md
    │   │   ├── ISSUE_651_MENU_OVERFLOW_CHECKLIST.md
    │   │   ├── ISSUE_677_ATOMIC_CONFIG_WRITES.md
    │   │   ├── ISSUE_689_EDITOR_SYNTAX_FADE.md
    │   │   ├── ISSUE_693_STATIC_LINUX_FFI.md
    │   │   ├── ISSUE_703_COPY_WINDOW_TITLE_FOLLOWUP.md
    │   │   ├── ISSUE_703_COPY_WINDOW_TITLE.md
    │   │   ├── ISSUE_722_COPY_ACCESS_RIGHTS.md
    │   │   ├── ISSUE_722_COPY_OPTIONS.md
    │   │   ├── ISSUE_724_SSH_HOST_KEY_VERIFICATION.md
    │   │   ├── ISSUE_725_SSH_AGENT_FORWARDING.md
    │   │   ├── ISSUE_727_S3_UPLOAD_API.md
    │   │   ├── ISSUE_744_CI_MERGED_RUN_FAILURES.md
    │   │   ├── ISSUE_793_MENU_COMMAND_PALETTE_ITEM.md
    │   │   ├── ISSUE_807_BOOKMARKS_HOTKEY.md
    │   │   ├── ISSUE_814_UNLISTABLE_FOLDER_ENTRY.md
    │   │   ├── ISSUE_816_ARCHIVE_PASSWORD_DIALOG_FOLLOWUP.md
    │   │   ├── ISSUE_833_CODEPAGE_AUTODETECTION.md
    │   │   ├── ISSUE_834_FILE_TITLE_DISAMBIGUATION.md
    │   │   ├── ISSUE_87_NESTED_TERMINAL_INPUT.md
    │   │   ├── ISSUE_91_FREEBSD_CONSOLE_DIAGNOSIS.md
    │   │   ├── ISSUE_915_ARCHIVE_TEST_TOTAL_PROGRESS.md
    │   │   ├── ISSUE_95_TERMINAL_TAB_COMPLETION_FOLLOWUP.md
    │   │   └── ISSUE_95_TERMINAL_TAB_COMPLETION.md
    │   ├── KEYMAP.md
    │   ├── L10N_REPORT_GUIDE.md
    │   ├── LUA.md
    │   ├── MACKEYS.md
    │   ├── MACROS.md
    │   ├── PINNED_CONSOLE.md
    │   ├── PINNED_HOST_FACTS.md
    │   ├── PLAYER.md
    │   ├── PLUGIN_PLAN.md
    │   ├── PLUGINS.md
    │   ├── PLUGRING.md
    │   ├── PORTABILITY_BSD.md
    │   ├── PORTABLE_BUILD_POLICY.md
    │   ├── PROMPT.md
    │   ├── QT_DRAGDROP.md
    │   ├── REVIEW.md
    │   ├── SETTINGS_CENTER.md
    │   ├── settings-center-fields.json
    │   ├── SPREADSHEET.md
    │   ├── SYNC_DIRS.md
    │   ├── TERMINAL.md
    │   ├── TEST_OPTIMIZATION_PLAN.md
    │   ├── TTYX.md
    │   ├── USER_MENU.md
    │   ├── UX_GUIDELINES.md
    │   ├── VFS.md
    │   ├── VIDEO.md
    │   ├── VTML.md
    │   ├── VTVIBE.md
    │   ├── WINCON_805_HANDOVER.md
    │   ├── WINCON.md
    │   └── WINE.md
    ├── embedded.go
    ├── f4.example.ini
    ├── go.mod
    ├── go.sum
    ├── highlight.ini
    ├── internal
    │   ├── action
    │   │   ├── order.go
    │   │   ├── registry_order_test.go
    │   │   └── registry.go
    │   ├── app
    │   │   ├── action_copy_window_title_test.go
    │   │   ├── action_copyname_parent_test.go
    │   │   ├── action_marked_clipboard_test.go
    │   │   ├── action_menu_test.go
    │   │   ├── action_menu_visibility_test.go
    │   │   ├── action_menu.go
    │   │   ├── action_registry_test.go
    │   │   ├── action_restore_selection_test.go
    │   │   ├── action_shortcut_conflict_test.go
    │   │   ├── actions_coverage_helpers_test.go
    │   │   ├── actions_framework_test.go
    │   │   ├── actions_framework.go
    │   │   ├── actions_low_coverage_extra_test.go
    │   │   ├── actions_table_order_test.go
    │   │   ├── actions_table.go
    │   │   ├── actions_test.go
    │   │   ├── actions_view_by_type_test.go
    │   │   ├── actions.go
    │   │   ├── ai_chat_panel_coverage_test.go
    │   │   ├── ai_chat_panel_test.go
    │   │   ├── ai_chat_panel.go
    │   │   ├── api_test.go
    │   │   ├── api.go
    │   │   ├── apply_command_test.go
    │   │   ├── arkanoid_coverage_test.go
    │   │   ├── arkanoid_test.go
    │   │   ├── arkanoid.go
    │   │   ├── attributes_test.go
    │   │   ├── autosave_settings_test.go
    │   │   ├── background_jobs_window_test.go
    │   │   ├── background_jobs_window.go
    │   │   ├── bom_test.go
    │   │   ├── bookmarks_dialog_test.go
    │   │   ├── bookmarks_test.go
    │   │   ├── bootstrap_backend_test.go
    │   │   ├── bootstrap_backend.go
    │   │   ├── bootstrap_coverage_test.go
    │   │   ├── bootstrap_ctrlhandler_other.go
    │   │   ├── bootstrap_ctrlhandler_windows_test.go
    │   │   ├── bootstrap_ctrlhandler_windows.go
    │   │   ├── bootstrap_detach_unix_test.go
    │   │   ├── bootstrap_detach_unix.go
    │   │   ├── bootstrap_detach_windows_test.go
    │   │   ├── bootstrap_detach_windows.go
    │   │   ├── bootstrap_gui_test.go
    │   │   ├── bootstrap_nestedinput_other.go
    │   │   ├── bootstrap_nestedinput_test.go
    │   │   ├── bootstrap_nestedinput_windows_test.go
    │   │   ├── bootstrap_nestedinput_windows.go
    │   │   ├── bootstrap_session_test.go
    │   │   ├── bootstrap_settings_test.go
    │   │   ├── bootstrap_settings.go
    │   │   ├── bootstrap_startupdir_terminal_test.go
    │   │   ├── bootstrap_startupdir_test.go
    │   │   ├── bootstrap_startupfile_terminal_test.go
    │   │   ├── bootstrap_sudo_test.go
    │   │   ├── bootstrap_unicode_test.go
    │   │   ├── bootstrap.go
    │   │   ├── catalog_fixture_test.go
    │   │   ├── child_env_test.go
    │   │   ├── child_env_universal_linux_test.go
    │   │   ├── cloudfox_real_archive_test.go
    │   │   ├── cloudfox_real_cross_cloud_test.go
    │   │   ├── cloudfox_real_large_f5_test.go
    │   │   ├── cloudfox_real_ui_test.go
    │   │   ├── codepage_issue875_sticky_test.go
    │   │   ├── codepage_issue875_test.go
    │   │   ├── colorer_coverage_more_test.go
    │   │   ├── colorer_download_test.go
    │   │   ├── colorer_settings_test.go
    │   │   ├── colorer_settings.go
    │   │   ├── colorer_type_settings_coverage_test.go
    │   │   ├── colorer_type_settings.go
    │   │   ├── colorstyle_firstrun_test.go
    │   │   ├── colorstyle_firstrun.go
    │   │   ├── command_history_paths_test.go
    │   │   ├── command_palette_direct_frames_test.go
    │   │   ├── command_palette_direct_frames.go
    │   │   ├── command_palette_direct_panels_test.go
    │   │   ├── command_palette_drives_test.go
    │   │   ├── command_palette_drives.go
    │   │   ├── command_palette_dynamic_test.go
    │   │   ├── command_palette_frames.go
    │   │   ├── command_palette_help_test.go
    │   │   ├── command_palette_help.go
    │   │   ├── command_palette_i18n_test.go
    │   │   ├── command_palette_i18n.go
    │   │   ├── command_palette_input_test.go
    │   │   ├── command_palette_macros.go
    │   │   ├── command_palette_menu_leaves_test.go
    │   │   ├── command_palette_menu_test.go
    │   │   ├── command_palette_modal.go
    │   │   ├── command_palette_panels.go
    │   │   ├── command_palette_prefixes.go
    │   │   ├── command_palette_search_test.go
    │   │   ├── command_palette_search.go
    │   │   ├── command_palette_test.go
    │   │   ├── command_palette_ui_test.go
    │   │   ├── command_palette_ui.go
    │   │   ├── command_palette_workspace.go
    │   │   ├── command_palette.go
    │   │   ├── command_prefix_registry_test.go
    │   │   ├── commandline_workspace_test.go
    │   │   ├── compare_folders_ui_coverage_test.go
    │   │   ├── compare_folders_ui.go
    │   │   ├── config_test.go
    │   │   ├── console_passthrough_test.go
    │   │   ├── copy_dialog_options_coverage_test.go
    │   │   ├── copy_dialog_options_more_coverage_test.go
    │   │   ├── copy_dialog_options.go
    │   │   ├── coverage_helpers_test.go
    │   │   ├── debug_hangdump_unix.go
    │   │   ├── debug_hangdump_windows.go
    │   │   ├── debug_log_test.go
    │   │   ├── debug.go
    │   │   ├── delete_trash_test.go
    │   │   ├── diagnostics_test.go
    │   │   ├── diagnostics.go
    │   │   ├── dialog_copy_resize_test.go
    │   │   ├── dialog_layout_languages_test.go
    │   │   ├── dialog_layouts_test.go
    │   │   ├── disasm_editor_test.go
    │   │   ├── document_reader_test.go
    │   │   ├── dragdrop_test.go
    │   │   ├── drive_bookmarks_test.go
    │   │   ├── drive_menu_options_test.go
    │   │   ├── drop_scene_test.go
    │   │   ├── editor_actions_test.go
    │   │   ├── editor_binary_open_test.go
    │   │   ├── editor_host_test.go
    │   │   ├── editor_hotkeys_test.go
    │   │   ├── editor_keys_test.go
    │   │   ├── editor_projection_test.go
    │   │   ├── editor_save_wait_test.go
    │   │   ├── editor_test_helpers_test.go
    │   │   ├── f4_commands_test.go
    │   │   ├── f4_commands.go
    │   │   ├── farmenu_file_test.go
    │   │   ├── fast_find_overlay_test.go
    │   │   ├── file_associations_dispatch_test.go
    │   │   ├── file_associations_test.go
    │   │   ├── file_ops_panel_test.go
    │   │   ├── file_panel_sorting_regression_test.go
    │   │   ├── find_file_test.go
    │   │   ├── find_file.go
    │   │   ├── fkeys_hidden_panels_test.go
    │   │   ├── folder_history_actions_test.go
    │   │   ├── folder_history_navigation_test.go
    │   │   ├── folder_history_panel_test.go
    │   │   ├── framewatch_vtui.go
    │   │   ├── framewatch.go
    │   │   ├── grabber_mouse_test.go
    │   │   ├── grabber_test.go
    │   │   ├── grabber.go
    │   │   ├── gui_font_combo_dialog_test.go
    │   │   ├── help_host_test.go
    │   │   ├── help_keys_ar_test.go
    │   │   ├── help_keys_he_test.go
    │   │   ├── help_keys_ru_test.go
    │   │   ├── help_keys_test.go
    │   │   ├── help_keys_tr_test.go
    │   │   ├── help_topics.go
    │   │   ├── history_benchmark_test.go
    │   │   ├── history_bridge.go
    │   │   ├── history_dialog_test.go
    │   │   ├── history_dialog.go
    │   │   ├── history_hint_test.go
    │   │   ├── history_import_test.go
    │   │   ├── history_performance_test.go
    │   │   ├── hotkeys_ui_test.go
    │   │   ├── hotkeys_ui.go
    │   │   ├── image_gallery_panel_test.go
    │   │   ├── image_view_panel_test.go
    │   │   ├── issue1218_keybar_test.go
    │   │   ├── issue54_test.go
    │   │   ├── issue561_test.go
    │   │   ├── issue631_test.go
    │   │   ├── issue821_test.go
    │   │   ├── issue856_mouse_capture_test.go
    │   │   ├── issue95_followup_test.go
    │   │   ├── keybar_injected_test.go
    │   │   ├── keymap_host_test.go
    │   │   ├── keymap_suspend.go
    │   │   ├── lang_host_test.go
    │   │   ├── lang.go
    │   │   ├── local_language_files_test.go
    │   │   ├── macro_ctrlletter_test.go
    │   │   ├── macro_dispatch.go
    │   │   ├── macro_host_test.go
    │   │   ├── macro_host.go
    │   │   ├── macro_plugin_calls.go
    │   │   ├── macro_reload_action_test.go
    │   │   ├── macro_test.go
    │   │   ├── main_menu_bar_keys_test.go
    │   │   ├── main_menu_dropdown_test.go
    │   │   ├── main_test.go
    │   │   ├── managed_execution_test.go
    │   │   ├── media_app.go
    │   │   ├── menu_history_action.go
    │   │   ├── menu_history_test.go
    │   │   ├── mock_failing_vfs_test.go
    │   │   ├── mock_pty_test.go
    │   │   ├── nativeui.go
    │   │   ├── navigation_mode_test.go
    │   │   ├── panel_actions_test.go
    │   │   ├── panel_menu_test.go
    │   │   ├── panel_plugins_test.go
    │   │   ├── panel_shortcuts_test.go
    │   │   ├── panel_visibility_test.go
    │   │   ├── panels_app_commands_coverage_test.go
    │   │   ├── panels_app_commands.go
    │   │   ├── panels_frame_app_test.go
    │   │   ├── panels_frame_drivecursor_windows_test.go
    │   │   ├── path_hints_test.go
    │   │   ├── path_identity_history_test.go
    │   │   ├── plughost_app.go
    │   │   ├── plugin_contributions_test.go
    │   │   ├── plugin_contributions.go
    │   │   ├── plugin_hotkeys_test.go
    │   │   ├── plugring_policy_test.go
    │   │   ├── plugring_rows_test.go
    │   │   ├── plugring_test.go
    │   │   ├── plugring_ui_test.go
    │   │   ├── plugring_ui.go
    │   │   ├── portable_paths_test.go
    │   │   ├── portable_test.go
    │   │   ├── process_environment_host_test.go
    │   │   ├── process_environment_host.go
    │   │   ├── pty_windows_panel_test.go
    │   │   ├── qt_document_lifecycle_test.go
    │   │   ├── qt_document_lifecycle.go
    │   │   ├── qt_document_loading_test.go
    │   │   ├── qt_document_speedup_benchmark_test.go
    │   │   ├── qt_document_viewport_test.go
    │   │   ├── qt_keybar_icons_test.go
    │   │   ├── qt_semantic_menu_wrapper_test.go
    │   │   ├── qt_semantic_window_protocol_test.go
    │   │   ├── qt_semantic.go
    │   │   ├── qt_version_test.go
    │   │   ├── queue_helpers_test.go
    │   │   ├── queue_semantic_test.go
    │   │   ├── quickview_api.go
    │   │   ├── quickview_binding_test.go
    │   │   ├── rpc_commands_test.go
    │   │   ├── runner_unix_panel_test.go
    │   │   ├── search_history_test.go
    │   │   ├── semantic_actions_test.go
    │   │   ├── semantic_test.go
    │   │   ├── settings_host_coverage_test.go
    │   │   ├── settings_host.go
    │   │   ├── settings_routes.go
    │   │   ├── settings_save_route_test.go
    │   │   ├── settings_save.go
    │   │   ├── share_dialog_coverage_test.go
    │   │   ├── share_dialog_test.go
    │   │   ├── share_dialog.go
    │   │   ├── sheet_actions_test.go
    │   │   ├── sheet_actions.go
    │   │   ├── sheet_compare_coverage_test.go
    │   │   ├── sheet_dialogs_coverage_test.go
    │   │   ├── sheet_dialogs.go
    │   │   ├── sheet_frame_coverage_extra_test.go
    │   │   ├── sheet_frame_test.go
    │   │   ├── sheet_frame.go
    │   │   ├── sheet_palette_test.go
    │   │   ├── sheet_palette.go
    │   │   ├── shell_integration_test.go
    │   │   ├── shutdown_cancel_test.go
    │   │   ├── simple_exec_test.go
    │   │   ├── sort_groups_test.go
    │   │   ├── split_update_test.go
    │   │   ├── sqlite_actions_test.go
    │   │   ├── sqlite_actions.go
    │   │   ├── static_direct_actions_test.go
    │   │   ├── static_direct_actions.go
    │   │   ├── sync_dirs_coverage_test.go
    │   │   ├── sync_dirs_ui.go
    │   │   ├── temp_panel_test.go
    │   │   ├── term_app_other.go
    │   │   ├── term_app_windows.go
    │   │   ├── term_app.go
    │   │   ├── terminal_log_offset_test.go
    │   │   ├── terminal_workspace_test.go
    │   │   ├── testdata
    │   │   │   └── action_order.golden
    │   │   ├── text_editor_bridge_test.go
    │   │   ├── theme_host_test.go
    │   │   ├── title_test.go
    │   │   ├── title_unix.go
    │   │   ├── title_windows.go
    │   │   ├── title.go
    │   │   ├── translator_test.go
    │   │   ├── updater_issue635_test.go
    │   │   ├── updater_repro_lock_other_test.go
    │   │   ├── updater_repro_lock_windows_test.go
    │   │   ├── updater_repro_test.go
    │   │   ├── updater_test.go
    │   │   ├── updater.go
    │   │   ├── upstream_compat.go
    │   │   ├── user_menu_ini_test.go
    │   │   ├── user_menu_subst_test.go
    │   │   ├── viewer_app.go
    │   │   ├── viewer_editor_history_test.go
    │   │   ├── viewer_keys_test.go
    │   │   ├── vtvibe_ap_coverage_test.go
    │   │   ├── vtvibe_ap_test.go
    │   │   ├── vtvibe_ap.go
    │   │   ├── vtvibe_host_coverage_test.go
    │   │   ├── vtvibe_host_extra_coverage_test.go
    │   │   ├── vtvibe_host_test.go
    │   │   ├── vtvibe_host.go
    │   │   ├── win32_backend_test.go
    │   │   ├── window_toggle_other.go
    │   │   ├── window_toggle_test.go
    │   │   ├── window_toggle_windows.go
    │   │   ├── window_toggle.go
    │   │   ├── workspace_routing_test.go
    │   │   ├── workspace_session_test.go
    │   │   ├── worktree_identity_test.go
    │   │   └── worktree_identity.go
    │   ├── appcmd
    │   │   └── commands.go
    │   ├── cmdline
    │   │   ├── apply_batch_test.go
    │   │   ├── apply_batch.go
    │   │   ├── apply_output_test.go
    │   │   ├── apply_output.go
    │   │   ├── apply_resources_test.go
    │   │   ├── apply_resources.go
    │   │   ├── apply_shortname_other.go
    │   │   ├── apply_shortname_windows_test.go
    │   │   ├── apply_shortname_windows.go
    │   │   ├── apply_subst_test.go
    │   │   ├── apply_subst.go
    │   │   ├── apply_transcript.go
    │   │   ├── autocomplete.go
    │   │   ├── line_semantic_test.go
    │   │   ├── line_test.go
    │   │   ├── line.go
    │   │   ├── main_test.go
    │   │   ├── prompt_test.go
    │   │   ├── prompt_unix.go
    │   │   ├── prompt_windows.go
    │   │   ├── prompt.go
    │   │   ├── qt_semantic.go
    │   │   ├── quotes_test.go
    │   │   ├── quotes.go
    │   │   ├── quoting_test.go
    │   │   ├── quoting.go
    │   │   ├── resolve_other.go
    │   │   ├── resolve_windows_test.go
    │   │   └── resolve_windows.go
    │   ├── colorer
    │   │   ├── configs
    │   │   │   └── base
    │   │   │       └── hrd
    │   │   │           └── rgb
    │   │   │               └── radiola.hrd
    │   │   └── embedded.go
    │   ├── config
    │   │   ├── appearance_settings_test.go
    │   │   ├── atomic_test.go
    │   │   ├── atomic.go
    │   │   ├── colorstyle_configured_test.go
    │   │   ├── config_test.go
    │   │   ├── config.go
    │   │   ├── cursor_style_test.go
    │   │   ├── fallback_language_test.go
    │   │   ├── grouping_test.go
    │   │   ├── grouping.go
    │   │   ├── options_test.go
    │   │   ├── options.go
    │   │   ├── overlay_test.go
    │   │   ├── overlay.go
    │   │   ├── proxy_settings_test.go
    │   │   ├── schema.go
    │   │   ├── serialize_roundtrip_test.go
    │   │   └── settings.go
    │   ├── dialog
    │   │   ├── about_os_other.go
    │   │   ├── about_os_unix.go
    │   │   ├── about_os_windows.go
    │   │   ├── about_test.go
    │   │   ├── about.go
    │   │   ├── attributes_mixed_test.go
    │   │   ├── attributes_unix.go
    │   │   ├── attributes_windows_test.go
    │   │   ├── attributes_windows.go
    │   │   ├── attributes.go
    │   │   ├── caption.go
    │   │   ├── config_editor_test.go
    │   │   ├── config_editor.go
    │   │   ├── envman_help_test.go
    │   │   ├── file_resize_test.go
    │   │   ├── file_test.go
    │   │   ├── file.go
    │   │   ├── goto.go
    │   │   ├── help
    │   │   │   ├── ar.hlf
    │   │   │   ├── be.hlf
    │   │   │   ├── bn.hlf
    │   │   │   ├── cs.hlf
    │   │   │   ├── de.hlf
    │   │   │   ├── en.hlf
    │   │   │   ├── es.hlf
    │   │   │   ├── et.hlf
    │   │   │   ├── fi.hlf
    │   │   │   ├── he.hlf
    │   │   │   ├── hi.hlf
    │   │   │   ├── hu.hlf
    │   │   │   ├── hy.hlf
    │   │   │   ├── ja.hlf
    │   │   │   ├── ka.hlf
    │   │   │   ├── ko.hlf
    │   │   │   ├── lt.hlf
    │   │   │   ├── lv.hlf
    │   │   │   ├── pl.hlf
    │   │   │   ├── README.md
    │   │   │   ├── ru.hlf
    │   │   │   ├── tr.hlf
    │   │   │   ├── uk.hlf
    │   │   │   └── zh.hlf
    │   │   ├── help_lang_test.go
    │   │   ├── help_languages.go
    │   │   ├── help_search_test.go
    │   │   ├── help_search.go
    │   │   ├── help_semantic.go
    │   │   ├── help_test.go
    │   │   ├── help.go
    │   │   ├── history_import_other.go
    │   │   ├── history_import_test.go
    │   │   ├── history_import_windows.go
    │   │   ├── history_import.go
    │   │   ├── hotkey_capture.go
    │   │   ├── label.go
    │   │   ├── main_test.go
    │   │   ├── path.go
    │   │   ├── profile_transfer_test.go
    │   │   ├── settings_codepage.go
    │   │   ├── settings_portable_test.go
    │   │   ├── settings_portable.go
    │   │   ├── settings_proxy_coverage_test.go
    │   │   ├── settings_proxy_test.go
    │   │   ├── settings_proxy.go
    │   │   ├── usermenu_import_test.go
    │   │   └── usermenu_import.go
    │   ├── editor
    │   │   ├── base64.go
    │   │   ├── buffer_async_test.go
    │   │   ├── buffer_async.go
    │   │   ├── buffer_mapped_unix.go
    │   │   ├── buffer_mapped_windows.go
    │   │   ├── buffer_mapped.go
    │   │   ├── buffer_prepare.go
    │   │   ├── colorer_async.go
    │   │   ├── colorer_check_test.go
    │   │   ├── colorer_check.go
    │   │   ├── colorer_cpu_amd64_test.go
    │   │   ├── colorer_cpu_amd64.go
    │   │   ├── colorer_cpu_other.go
    │   │   ├── colorer_cpu_test.go
    │   │   ├── colorer_cpu.go
    │   │   ├── colorer_diagnostics_test.go
    │   │   ├── colorer_downloader.go
    │   │   ├── colorer_outline_frame_test.go
    │   │   ├── colorer_outline_frame.go
    │   │   ├── colorer_outline_test.go
    │   │   ├── colorer_outline.go
    │   │   ├── colorer_pair_search.go
    │   │   ├── colorer_pairs_test.go
    │   │   ├── colorer_pairs.go
    │   │   ├── colorer_params_test.go
    │   │   ├── colorer_params.go
    │   │   ├── colorer_plugin_test.go
    │   │   ├── colorer_reload.go
    │   │   ├── colorer_setups.go
    │   │   ├── colorer_text.go
    │   │   ├── colorer_type_settings_test.go
    │   │   ├── colorer_type_settings.go
    │   │   ├── colorer_types_coverage_test.go
    │   │   ├── colorer_types_test.go
    │   │   ├── colorer_types.go
    │   │   ├── colorer_window.go
    │   │   ├── colorer.go
    │   │   ├── crosshair_test.go
    │   │   ├── crosshair.go
    │   │   ├── document_reader_test.go
    │   │   ├── editor_base64_test.go
    │   │   ├── editor_codepage_test.go
    │   │   ├── editor_delta_test.go
    │   │   ├── editor_duplicate_line_test.go
    │   │   ├── editor_fade_test.go
    │   │   ├── editor_features_test.go
    │   │   ├── editor_find_all_test.go
    │   │   ├── editor_highlight_budget_test.go
    │   │   ├── editor_index_status_test.go
    │   │   ├── editor_long_line_render_test.go
    │   │   ├── editor_mmap_test.go
    │   │   ├── editor_move_line_test.go
    │   │   ├── editor_multicursor_edit_test.go
    │   │   ├── editor_multicursor_move_test.go
    │   │   ├── editor_multicursor_occurrence_test.go
    │   │   ├── editor_multicursor_select_test.go
    │   │   ├── editor_multicursor_test.go
    │   │   ├── editor_occurrence_test.go
    │   │   ├── editor_restore_keys_test.go
    │   │   ├── editor_save_as_test.go
    │   │   ├── editor_save_inplace_test.go
    │   │   ├── editor_search_lazy_test.go
    │   │   ├── editor_search_remote_test.go
    │   │   ├── editor_search_zerocopy_test.go
    │   │   ├── editor_shiftdel_test.go
    │   │   ├── editor_target_line_test.go
    │   │   ├── editor_veto_test.go
    │   │   ├── editor_view_ads_test.go
    │   │   ├── editor_view_test.go
    │   │   ├── editor_wrap_memory_test.go
    │   │   ├── editor_wrap_safety_test.go
    │   │   ├── escape_colorer_test.go
    │   │   ├── external_command.go
    │   │   ├── external_editor_process_unix_test.go
    │   │   ├── external_editor_test.go
    │   │   ├── external_freebsd_test.go
    │   │   ├── external_freebsd.go
    │   │   ├── external_unix.go
    │   │   ├── external_windows.go
    │   │   ├── fade.go
    │   │   ├── findall.go
    │   │   ├── goto_test.go
    │   │   ├── grapheme.go
    │   │   ├── host.go
    │   │   ├── index_status.go
    │   │   ├── main_test.go
    │   │   ├── mapped_file_test.go
    │   │   ├── menubar_test.go
    │   │   ├── multicursor.go
    │   │   ├── navigation_trace_test.go
    │   │   ├── protocol_helpers_test.go
    │   │   ├── qt_editor_document_projection_test.go
    │   │   ├── qt_editor_index_lifecycle_test.go
    │   │   ├── qt_editor_native_selection_render_test.go
    │   │   ├── qt_editor_opening_test.go
    │   │   ├── qt_editor_pointer_trace_test.go
    │   │   ├── qt_editor_reflow_test.go
    │   │   ├── qt_editor_reflow.go
    │   │   ├── qt_editor_save_regression_test.go
    │   │   ├── qt_editor_short_selection_test.go
    │   │   ├── qt_editor_text_projection.go
    │   │   ├── qt_semantic_window_render.go
    │   │   ├── qt_semantic.go
    │   │   ├── replace_confirm.go
    │   │   ├── save_as.go
    │   │   ├── search_remote.go
    │   │   ├── secondary_carets_test.go
    │   │   ├── semantic_helpers_test.go
    │   │   ├── semantic_model_test.go
    │   │   ├── sort.go
    │   │   ├── status_coverage_test.go
    │   │   ├── status.go
    │   │   ├── surface_benchmark_test.go
    │   │   ├── surface_recorder_test.go
    │   │   ├── url_links_hover_test.go
    │   │   ├── view_coverage_contract_test.go
    │   │   ├── view_lowlevel_coverage_test.go
    │   │   ├── view_semantic_coverage_test.go
    │   │   ├── view_semantic.go
    │   │   ├── view.go
    │   │   ├── viewport_helpers_test.go
    │   │   ├── viewport_test.go
    │   │   ├── viewport.go
    │   │   ├── window_protocol_test.go
    │   │   ├── window_render_test.go
    │   │   └── wrap_safety.go
    │   ├── filemask
    │   │   ├── mask_test.go
    │   │   └── mask.go
    │   ├── fileops
    │   │   ├── access_rights_test.go
    │   │   ├── access_rights.go
    │   │   ├── archive_index_fallback.go
    │   │   ├── archive_index_test.go
    │   │   ├── archive_index.go
    │   │   ├── buttons_test.go
    │   │   ├── buttons.go
    │   │   ├── codepage.go
    │   │   ├── compare_folders_test.go
    │   │   ├── compare.go
    │   │   ├── copy_options_test.go
    │   │   ├── copy_options.go
    │   │   ├── deletion_message_test.go
    │   │   ├── dialog_reporter_test.go
    │   │   ├── document_read.go
    │   │   ├── export_test.go
    │   │   ├── file_mask_far2l_test.go
    │   │   ├── file_mask_test.go
    │   │   ├── file_op_dialog_test.go
    │   │   ├── file_op_tracker_test.go
    │   │   ├── file_ops_coverage_test.go
    │   │   ├── file_ops_safety_test.go
    │   │   ├── file_ops_test.go
    │   │   ├── file_ops_transfer_name_test.go
    │   │   ├── host_test.go
    │   │   ├── host.go
    │   │   ├── identity_test.go
    │   │   ├── identity.go
    │   │   ├── issue149_test.go
    │   │   ├── issue815_test.go
    │   │   ├── local.go
    │   │   ├── main_test.go
    │   │   ├── mask.go
    │   │   ├── ops_case_test.go
    │   │   ├── ops_dialog.go
    │   │   ├── ops.go
    │   │   ├── path_identity_test.go
    │   │   ├── pump.go
    │   │   ├── qt_queue_pause_test.go
    │   │   ├── qt_queue_semantic_test.go
    │   │   ├── qt_queue_semantic.go
    │   │   ├── queue_manager_test.go
    │   │   ├── queue.go
    │   │   ├── real_device_copy_test.go
    │   │   ├── report_contract_test.go
    │   │   ├── report.go
    │   │   ├── rights_extra.go
    │   │   ├── security_other.go
    │   │   ├── security_windows_test.go
    │   │   ├── security_windows.go
    │   │   ├── state_key_test.go
    │   │   ├── state_test.go
    │   │   ├── state.go
    │   │   ├── symlink_coverage_test.go
    │   │   ├── symlink.go
    │   │   ├── sync_test.go
    │   │   ├── sync.go
    │   │   ├── tracker.go
    │   │   └── transfer_identity_integration_test.go
    │   ├── fusefs
    │   │   ├── bench-all.sh
    │   │   ├── BENCH.md
    │   │   ├── bench.sh
    │   │   ├── bridge_test.go
    │   │   ├── bridge.go
    │   │   ├── cli_test.go
    │   │   ├── cli.go
    │   │   ├── FUSE.md
    │   │   ├── fusefs.go
    │   │   ├── mountspec.go
    │   │   ├── node_fuse_test.go
    │   │   ├── node_fuse.go
    │   │   ├── node_unsupported.go
    │   │   ├── platform_other.go
    │   │   ├── platform_unix.go
    │   │   ├── registry.go
    │   │   ├── staged_test.go
    │   │   └── writers_test.go
    │   ├── gui
    │   │   ├── assets
    │   │   │   └── icon
    │   │   │       ├── f4-16.svg
    │   │   │       ├── f4-24.svg
    │   │   │       ├── f4-30.svg
    │   │   │       ├── f4-32.svg
    │   │   │       ├── f4-36.svg
    │   │   │       ├── f4-42.svg
    │   │   │       ├── f4.svg
    │   │   │       ├── generated
    │   │   │       │   ├── f4-1024.png
    │   │   │       │   ├── f4-128.png
    │   │   │       │   ├── f4-16.png
    │   │   │       │   ├── f4-24.png
    │   │   │       │   ├── f4-256.png
    │   │   │       │   ├── f4-28.png
    │   │   │       │   ├── f4-30.png
    │   │   │       │   ├── f4-32.png
    │   │   │       │   ├── f4-36.png
    │   │   │       │   ├── f4-42.png
    │   │   │       │   ├── f4-48.png
    │   │   │       │   ├── f4-512.png
    │   │   │       │   ├── f4-56.png
    │   │   │       │   ├── f4-64.png
    │   │   │       │   ├── f4.icns
    │   │   │       │   └── f4.ico
    │   │   │       └── README.md
    │   │   ├── backend_ffi.go
    │   │   ├── backend_stub.go
    │   │   ├── backend_test.go
    │   │   ├── backend.go
    │   │   ├── font_catalog_test.go
    │   │   ├── font_catalog_unix_test.go
    │   │   ├── font_catalog_unix.go
    │   │   ├── font_catalog_windows_test.go
    │   │   ├── font_catalog_windows.go
    │   │   ├── font_catalog.go
    │   │   ├── font_combo.go
    │   │   ├── font_notwindows.go
    │   │   ├── font_test.go
    │   │   ├── font_windows_test.go
    │   │   ├── font_windows.go
    │   │   ├── font.go
    │   │   ├── icon_darwin_test.go
    │   │   ├── icon_darwin.go
    │   │   ├── icon_unix.go
    │   │   ├── icon_windows_coverage_test.go
    │   │   ├── icon_windows_test.go
    │   │   ├── icon_windows.go
    │   │   ├── run_unix_test.go
    │   │   ├── run_unix.go
    │   │   ├── run_windows.go
    │   │   ├── runtime_mode_test.go
    │   │   ├── runtime_mode.go
    │   │   ├── window.go
    │   │   ├── winepath_other.go
    │   │   ├── winepath_windows_test.go
    │   │   ├── winepath_windows.go
    │   │   └── worktree.go
    │   ├── hideconsole
    │   │   ├── go.mod
    │   │   └── hideconsole.go
    │   ├── history
    │   │   ├── edit_test.go
    │   │   ├── edit.go
    │   │   ├── far2l.go
    │   │   ├── far3_merge.go
    │   │   ├── far3_test.go
    │   │   ├── far3.go
    │   │   ├── history_coverage_test.go
    │   │   ├── menu.go
    │   │   ├── navigation_trace_test.go
    │   │   ├── path_identity_integration_test.go
    │   │   ├── paths.go
    │   │   ├── persistence_test.go
    │   │   ├── pins.go
    │   │   ├── provider.go
    │   │   └── viewedit.go
    │   ├── i18n
    │   │   ├── lang
    │   │   │   ├── ar.lng
    │   │   │   ├── be.lng
    │   │   │   ├── bn.lng
    │   │   │   ├── coverage_baseline.txt
    │   │   │   ├── cs.lng
    │   │   │   ├── de.lng
    │   │   │   ├── en.lng
    │   │   │   ├── es.lng
    │   │   │   ├── et.lng
    │   │   │   ├── fi.lng
    │   │   │   ├── he.lng
    │   │   │   ├── hi.lng
    │   │   │   ├── hu.lng
    │   │   │   ├── hy.lng
    │   │   │   ├── ja.lng
    │   │   │   ├── ka.lng
    │   │   │   ├── ko.lng
    │   │   │   ├── lt.lng
    │   │   │   ├── lv.lng
    │   │   │   ├── pl.lng
    │   │   │   ├── README.md
    │   │   │   ├── ru.lng
    │   │   │   ├── tr.lng
    │   │   │   ├── uk.lng
    │   │   │   └── zh.lng
    │   │   ├── lang_bidi_test.go
    │   │   ├── lang_consistency_test.go
    │   │   ├── lang_contamination_test.go
    │   │   ├── lang_fallback_priority_test.go
    │   │   ├── lang_homoglyphs_test.go
    │   │   ├── lang_packs_test.go
    │   │   ├── lang_scripts_test.go
    │   │   ├── lang_test.go
    │   │   ├── lang.go
    │   │   ├── language_list_test.go
    │   │   ├── languages.go
    │   │   ├── msg_keys_test.go
    │   │   ├── packs.go
    │   │   └── settings_translations_test.go
    │   ├── ini
    │   │   ├── ini_test.go
    │   │   └── ini.go
    │   ├── keymap
    │   │   ├── farkeys_selection_test.go
    │   │   ├── farkeys.go
    │   │   ├── hotkeys.go
    │   │   ├── icons.go
    │   │   ├── input_translation_test.go
    │   │   ├── input.go
    │   │   ├── keymap_test.go
    │   │   ├── kitty.go
    │   │   ├── mackeys_test.go
    │   │   ├── mackeys.go
    │   │   ├── remap.go
    │   │   ├── terminal_mouse_offset_test.go
    │   │   ├── translate_kitty_test.go
    │   │   ├── ttyx_keys_test.go
    │   │   └── ttyx.go
    │   ├── luaplug
    │   │   ├── convert_test.go
    │   │   ├── convert.go
    │   │   ├── f4rpc.go
    │   │   ├── ffi_test.go
    │   │   ├── ffi.go
    │   │   ├── goid.go
    │   │   ├── luastate_test.go
    │   │   ├── runtime_test.go
    │   │   ├── runtime.go
    │   │   └── sandbox.go
    │   ├── macro
    │   │   ├── engine.go
    │   │   ├── export_test.go
    │   │   ├── export.go
    │   │   ├── lua_api.go
    │   │   ├── lua_test.go
    │   │   ├── lua.go
    │   │   ├── plugin_calls_test.go
    │   │   ├── plugin_calls.go
    │   │   ├── reload_test.go
    │   │   └── reload.go
    │   ├── media
    │   │   ├── application.go
    │   │   ├── audio_decode_contract_test.go
    │   │   ├── audio_decode_test.go
    │   │   ├── audio_decode.go
    │   │   ├── audio_engine_contract_test.go
    │   │   ├── audio_oto.go
    │   │   ├── audio_stub.go
    │   │   ├── audio.go
    │   │   ├── blit_test.go
    │   │   ├── image_bmp.go
    │   │   ├── image_console_stats_test.go
    │   │   ├── image_console_stats.go
    │   │   ├── image_decode_test.go
    │   │   ├── image_decode.go
    │   │   ├── image_external_test.go
    │   │   ├── image_external.go
    │   │   ├── image_formats_test.go
    │   │   ├── image_gallery_test.go
    │   │   ├── image_gallery.go
    │   │   ├── image_native_darwin_test.go
    │   │   ├── image_native_darwin.go
    │   │   ├── image_preview_test.go
    │   │   ├── image_preview.go
    │   │   ├── image_qoi_coverage_test.go
    │   │   ├── image_qoi.go
    │   │   ├── image_slideshow_test.go
    │   │   ├── image_slideshow.go
    │   │   ├── image_test.go
    │   │   ├── image_transform_test.go
    │   │   ├── image_transform.go
    │   │   ├── image_view_orient_test.go
    │   │   ├── image_view_overlay_test.go
    │   │   ├── image_view_test.go
    │   │   ├── image_view.go
    │   │   ├── image.go
    │   │   ├── main_test.go
    │   │   ├── overlay_console_key_test.go
    │   │   ├── overlay_console.go
    │   │   ├── overlay_x11_test.go
    │   │   ├── overlay_x11.go
    │   │   ├── tools_test.go
    │   │   ├── tools.go
    │   │   ├── video_test.go
    │   │   ├── video_view_coverage_test.go
    │   │   ├── video_view.go
    │   │   └── video.go
    │   ├── mediatiming
    │   │   ├── qt_media_timing_test.go
    │   │   └── qt_media_timing.go
    │   ├── nativeui
    │   │   ├── adapter.go
    │   │   ├── dialog_key_hints_test.go
    │   │   ├── incremental_test.go
    │   │   ├── list_item_states_test.go
    │   │   ├── menu_wrapper_test.go
    │   │   ├── model_test.go
    │   │   ├── overlay_order_test.go
    │   │   ├── panel_status_test.go
    │   │   ├── path_bar_test.go
    │   │   ├── quickview_model_test.go
    │   │   ├── scene_help_test.go
    │   │   ├── scene_incremental.go
    │   │   ├── scene.go
    │   │   └── settings_semantic_test.go
    │   ├── navtrace
    │   │   ├── document_test.go
    │   │   ├── document.go
    │   │   ├── incremental.go
    │   │   ├── qt_navigation_benchmark_clock_fallback.go
    │   │   ├── qt_navigation_benchmark_clock_raw.go
    │   │   ├── qt_navigation_benchmark_clock_windows_test.go
    │   │   ├── qt_navigation_benchmark_clock_windows.go
    │   │   ├── qt_navigation_benchmark_test.go
    │   │   └── qt_navigation_benchmark.go
    │   ├── netproxy
    │   │   ├── coverage_test.go
    │   │   ├── keepalive_linux_test.go
    │   │   ├── netproxy_test.go
    │   │   └── netproxy.go
    │   ├── numeric
    │   │   ├── memory.go
    │   │   ├── numeric_test.go
    │   │   ├── numeric.go
    │   │   └── size.go
    │   ├── panel
    │   │   ├── actions.go
    │   │   ├── activation_renderer_test.go
    │   │   ├── apply_coverage_extra_test.go
    │   │   ├── apply_shutdown.go
    │   │   ├── apply.go
    │   │   ├── associations_editor.go
    │   │   ├── associations_ui.go
    │   │   ├── associations.go
    │   │   ├── audio_decode_panel_test.go
    │   │   ├── autofilter.go
    │   │   ├── background_jobs_session_test.go
    │   │   ├── bookmarks_dialog_test.go
    │   │   ├── bookmarks_dialog.go
    │   │   ├── bookmarks.go
    │   │   ├── bridge_texteditor.go
    │   │   ├── bridge_visren_coverage_test.go
    │   │   ├── bridge_visren.go
    │   │   ├── cmd_session_test.go
    │   │   ├── commandline_multiline_test.go
    │   │   ├── commandline_multiline.go
    │   │   ├── commandline_visibility_test.go
    │   │   ├── commandline_visibility.go
    │   │   ├── commandline_workspace_test.go
    │   │   ├── commandline_workspace.go
    │   │   ├── console_rows_test.go
    │   │   ├── console.go
    │   │   ├── context_editors_test.go
    │   │   ├── coverage_player_helpers_test.go
    │   │   ├── device_presentation_test.go
    │   │   ├── document_geometry_test.go
    │   │   ├── document_host.go
    │   │   ├── drives_bookmarks_ui.go
    │   │   ├── drives_bookmarks.go
    │   │   ├── drives_menu_unix.go
    │   │   ├── drives_menu_windows.go
    │   │   ├── drives_menu.go
    │   │   ├── drop_dialog_test.go
    │   │   ├── edit_command_test.go
    │   │   ├── exec.go
    │   │   ├── file_panel_test.go
    │   │   ├── frame_coverage_test.go
    │   │   ├── frame_dragdrop.go
    │   │   ├── frame_externalui.go
    │   │   ├── frame_gallery_menu_test.go
    │   │   ├── frame_gallery_menu.go
    │   │   ├── frame_manager_test_helpers_test.go
    │   │   ├── frame_procenv.go
    │   │   ├── frame_translator.go
    │   │   ├── frame_workspace_terminal.go
    │   │   ├── frame_workspace.go
    │   │   ├── frame.go
    │   │   ├── fuse_list_coverage_extra_test.go
    │   │   ├── fuse_list_coverage_test.go
    │   │   ├── fuse_list.go
    │   │   ├── fuse_mount_coverage_test.go
    │   │   ├── fuse_mount.go
    │   │   ├── gallery_session.go
    │   │   ├── grouping_menu.go
    │   │   ├── grouping_test.go
    │   │   ├── grouping_view.go
    │   │   ├── grouping.go
    │   │   ├── highlight_sort_test.go
    │   │   ├── hints.go
    │   │   ├── host_input_modes_other.go
    │   │   ├── host_input_modes_test.go
    │   │   ├── host_input_modes_windows.go
    │   │   ├── host_input_modes.go
    │   │   ├── host.go
    │   │   ├── hotkey_conditions.go
    │   │   ├── info_panel_test.go
    │   │   ├── info_usage.go
    │   │   ├── info.go
    │   │   ├── issue1131_test.go
    │   │   ├── issue1184_test.go
    │   │   ├── issue1218_keybar_test.go
    │   │   ├── issue863_terminal_test.go
    │   │   ├── issue915_keybar_test.go
    │   │   ├── kitty_passthrough_test.go
    │   │   ├── kitty_passthrough.go
    │   │   ├── list_access_other_test.go
    │   │   ├── list_access_test.go
    │   │   ├── list_access_windows_test.go
    │   │   ├── list_access.go
    │   │   ├── list_reconnect.go
    │   │   ├── list_size_test.go
    │   │   ├── list_size.go
    │   │   ├── list.go
    │   │   ├── lookup.go
    │   │   ├── main_test.go
    │   │   ├── media_registration_test.go
    │   │   ├── media_registry.go
    │   │   ├── media_revision_test.go
    │   │   ├── menu_semantic.go
    │   │   ├── menubar_dropdown_test.go
    │   │   ├── mock_pty_test.go
    │   │   ├── navigation_trace_test.go
    │   │   ├── panels_frame_pty_test.go
    │   │   ├── panels_frame_test.go
    │   │   ├── paste_test.go
    │   │   ├── paste.go
    │   │   ├── pins_coverage_test.go
    │   │   ├── pins.go
    │   │   ├── player_coverage_extra_test.go
    │   │   ├── player_coverage_test.go
    │   │   ├── player_test.go
    │   │   ├── player.go
    │   │   ├── plugin_hotkeys.go
    │   │   ├── plugins.go
    │   │   ├── prefixes_registry_windows_test.go
    │   │   ├── prefixes_test.go
    │   │   ├── prefixes.go
    │   │   ├── press_key_test.go
    │   │   ├── process_environment_panel_test.go
    │   │   ├── progress_overlay_test.go
    │   │   ├── prompt_coverage_extra_test.go
    │   │   ├── prompt.go
    │   │   ├── pty_geometry.go
    │   │   ├── qt_commandline_drop_test.go
    │   │   ├── qt_commandline_drop.go
    │   │   ├── qt_document_lifecycle.go
    │   │   ├── qt_macos_locations_darwin.go
    │   │   ├── qt_macos_locations_other.go
    │   │   ├── qt_macos_query_vfs_darwin_test.go
    │   │   ├── qt_macos_query_vfs_darwin.go
    │   │   ├── qt_macos_values_darwin.go
    │   │   ├── qt_panel_refresh_test.go
    │   │   ├── qt_panel_refresh.go
    │   │   ├── qt_semantic_dragdrop_conflict_test.go
    │   │   ├── qt_semantic_dragdrop_test.go
    │   │   ├── qt_semantic_dragdrop.go
    │   │   ├── qt_semantic_incremental.go
    │   │   ├── qt_semantic_panel_status_test.go
    │   │   ├── qt_semantic_panel_status.go
    │   │   ├── qt_semantic_test.go
    │   │   ├── qt_semantic_workspace_drag_test.go
    │   │   ├── qt_semantic.go
    │   │   ├── qt_split_test.go
    │   │   ├── qt_split.go
    │   │   ├── qt_windows_locations_menu_stub.go
    │   │   ├── qt_windows_locations_menu_windows_test.go
    │   │   ├── qt_windows_locations_menu_windows.go
    │   │   ├── quick_view_panel_test.go
    │   │   ├── quick_view_provider_test.go
    │   │   ├── quickview_colors_test.go
    │   │   ├── quickview_coverage_test.go
    │   │   ├── quickview_preview_test.go
    │   │   ├── quickview_preview.go
    │   │   ├── quickview_qml_contract_test.go
    │   │   ├── quickview.go
    │   │   ├── reconnect_test.go
    │   │   ├── remote.go
    │   │   ├── selection_extension_test.go
    │   │   ├── selection_extension.go
    │   │   ├── selection_journal_test.go
    │   │   ├── selection_panel_test.go
    │   │   ├── selection_shortcuts_test.go
    │   │   ├── semantic_helpers_test.go
    │   │   ├── semantic_model_test.go
    │   │   ├── semantic_uri_navigation_test.go
    │   │   ├── session_cmd.go
    │   │   ├── settings_test.go
    │   │   ├── settings.go
    │   │   ├── shell_session_test.go
    │   │   ├── sort_groups_test.go
    │   │   ├── sort.go
    │   │   ├── state.go
    │   │   ├── temp_coverage_test.go
    │   │   ├── temp.go
    │   │   ├── terminal_helpers_test.go
    │   │   ├── terminal_overlay_test.go
    │   │   ├── terminal_redraw_target_test.go
    │   │   ├── terminal_redraw_test.go
    │   │   ├── transfer_dialog_test.go
    │   │   ├── upstream_semantic_test.go
    │   │   ├── uri_navigation_test.go
    │   │   ├── user_menu_ui_test.go
    │   │   ├── usermenu_coverage_test.go
    │   │   ├── usermenu_execution_test.go
    │   │   ├── usermenu_farfile.go
    │   │   ├── usermenu_import_test.go
    │   │   ├── usermenu_import.go
    │   │   ├── usermenu_ini.go
    │   │   ├── usermenu_script_coverage_test.go
    │   │   ├── usermenu_script.go
    │   │   ├── usermenu_subst.go
    │   │   ├── usermenu_ui_coverage_extra_test.go
    │   │   ├── usermenu_ui.go
    │   │   ├── usermenu.go
    │   │   ├── workspace_startup_test.go
    │   │   └── workspace.go
    │   ├── paneltest
    │   │   ├── doc.go
    │   │   ├── frame.go
    │   │   └── mock_pty.go
    │   ├── piecetable
    │   │   ├── concurrent_test.go
    │   │   ├── lineindex_equivalence_test.go
    │   │   ├── lineindex_test.go
    │   │   ├── lineindex.go
    │   │   ├── piecetable_test.go
    │   │   └── piecetable.go
    │   ├── plughost
    │   │   ├── application.go
    │   │   ├── contributions.go
    │   │   ├── extui_coverage_test.go
    │   │   ├── extui_test.go
    │   │   ├── extui.go
    │   │   ├── fastpath_regression_test.go
    │   │   ├── ffi_test.go
    │   │   ├── ffi.go
    │   │   ├── host_coverage_test.go
    │   │   ├── host.go
    │   │   ├── identity_test.go
    │   │   ├── incremental_helpers_test.go
    │   │   ├── incremental_test.go
    │   │   ├── lifecycle_coverage_test.go
    │   │   ├── manager_coverage_test.go
    │   │   ├── manager_lifecycle_test.go
    │   │   ├── manager_names_test.go
    │   │   ├── manager.go
    │   │   ├── media_timing.go
    │   │   ├── menu_items.go
    │   │   ├── menu_retention_test.go
    │   │   ├── navigation_trace_test.go
    │   │   ├── panel_providers.go
    │   │   ├── permissions_test.go
    │   │   ├── permissions_ui_test.go
    │   │   ├── permissions_ui.go
    │   │   ├── permissions.go
    │   │   ├── plugring_meta_test.go
    │   │   ├── plugring_meta.go
    │   │   ├── plugring.go
    │   │   ├── presentation_fixture_test.go
    │   │   ├── presentation_integration_test.go
    │   │   ├── presentation.go
    │   │   ├── qt_embedded_qt_host_payload_stub.go
    │   │   ├── qt_embedded_qt_host_payload_test.go
    │   │   ├── qt_embedded_qt_host_payload.go
    │   │   ├── qt_embedded_qt_host_test.go
    │   │   ├── qt_embedded_qt_host.go
    │   │   ├── qt_extui_directory_preview_test.go
    │   │   ├── qt_extui_directory_preview_wire.go
    │   │   ├── qt_extui_directory_preview.go
    │   │   ├── qt_extui_media_test.go
    │   │   ├── qt_extui_media_windows_test.go
    │   │   ├── qt_extui_media.go
    │   │   ├── qt_platform_ipc_test.go
    │   │   ├── qt_platform_ipc.go
    │   │   ├── rollback_scene_test.go
    │   │   ├── rpc_commands.go
    │   │   ├── rpc_panel_coverage_test.go
    │   │   ├── rpc_panel.go
    │   │   ├── rpc_vfs_coverage_test.go
    │   │   ├── rpc_vfs_test.go
    │   │   ├── rpc_vfs.go
    │   │   ├── scaffold_test.go
    │   │   ├── scaffold.go
    │   │   ├── semantic_helpers_test.go
    │   │   ├── settings_permissions_test.go
    │   │   ├── terminal_redraw_test.go
    │   │   ├── transport_lua_rpc_test.go
    │   │   ├── transport_lua_test.go
    │   │   ├── transport_lua.go
    │   │   ├── transport_rpc_coverage_test.go
    │   │   ├── transport_rpc_test.go
    │   │   ├── transport_rpc.go
    │   │   ├── transport_wazero_test.go
    │   │   └── transport_wazero.go
    │   ├── semantic
    │   │   ├── capabilities.go
    │   │   ├── document_viewport.go
    │   │   ├── fields_test.go
    │   │   ├── fields.go
    │   │   ├── menu_patch_test.go
    │   │   ├── menu_patch.go
    │   │   ├── qt_semantic_window_render.go
    │   │   ├── qt_semantic.go
    │   │   ├── scene_patch.go
    │   │   ├── scene.go
    │   │   ├── time.go
    │   │   ├── values.go
    │   │   ├── workspace.go
    │   │   └── wrapped_rows.go
    │   ├── settings
    │   │   ├── benchmark_test.go
    │   │   ├── catalog.go
    │   │   ├── center_profile_test.go
    │   │   ├── center_semantic_test.go
    │   │   ├── center_semantic.go
    │   │   ├── center_test.go
    │   │   ├── center.go
    │   │   ├── choice_help_test.go
    │   │   ├── choice_help.go
    │   │   ├── choice_help.tsv
    │   │   ├── chord_coverage_test.go
    │   │   ├── chord.go
    │   │   ├── collection_palette_test.go
    │   │   ├── collection_ui.go
    │   │   ├── core.go
    │   │   ├── edit_test.go
    │   │   ├── edit.go
    │   │   ├── enter_test.go
    │   │   ├── extra.go
    │   │   ├── grouping_test.go
    │   │   ├── help.go
    │   │   ├── host.go
    │   │   ├── hotkeys_persistence_test.go
    │   │   ├── inventory_test.go
    │   │   ├── main_test.go
    │   │   ├── manual_save_test.go
    │   │   ├── mouse_test.go
    │   │   ├── operations.go
    │   │   ├── plugin_catalog.go
    │   │   ├── plugin_providers_coverage_test.go
    │   │   ├── plugins.go
    │   │   ├── providers.go
    │   │   ├── radios_semantic.go
    │   │   ├── radios_test.go
    │   │   ├── radios.go
    │   │   ├── record_dialog_test.go
    │   │   ├── record_dialog.go
    │   │   ├── records.go
    │   │   ├── russian_test.go
    │   │   ├── scoped_test.go
    │   │   ├── search_first_test.go
    │   │   ├── search_layout_test.go
    │   │   ├── startup_test.go
    │   │   ├── trace_test.go
    │   │   ├── trace.go
    │   │   ├── transactions_test.go
    │   │   ├── user_menu_test.go
    │   │   └── user_menu.go
    │   ├── settingstest
    │   │   └── russian.go
    │   ├── sheet
    │   │   ├── cell.go
    │   │   ├── coverage_contract_test.go
    │   │   ├── expr.go
    │   │   ├── sheet_test.go
    │   │   ├── sheet.go
    │   │   ├── store.go
    │   │   └── xlsx.go
    │   ├── stallwatch
    │   │   ├── stallwatch_test.go
    │   │   └── stallwatch.go
    │   ├── sysinfo
    │   │   ├── cpu_darwin.go
    │   │   ├── cpu_linux.go
    │   │   ├── cpu_other.go
    │   │   ├── cpu_windows.go
    │   │   ├── cpu.go
    │   │   ├── drive_icons.go
    │   │   ├── drives_unix.go
    │   │   ├── drives_windows.go
    │   │   ├── drives.go
    │   │   ├── fs_darwin.go
    │   │   ├── fs_linux.go
    │   │   ├── fs_other.go
    │   │   ├── fs_windows_test.go
    │   │   ├── fs_windows.go
    │   │   ├── fs.go
    │   │   ├── gpu_darwin.go
    │   │   ├── gpu_linux.go
    │   │   ├── gpu_other.go
    │   │   ├── gpu_windows.go
    │   │   ├── gpu.go
    │   │   ├── mem_linux.go
    │   │   ├── mem_other.go
    │   │   ├── mem_windows.go
    │   │   ├── mem.go
    │   │   ├── qt_drives_darwin_test.go
    │   │   └── qt_drives_unix_test.go
    │   ├── terminal
    │   │   ├── ansi_sync_test.go
    │   │   ├── ansi_test.go
    │   │   ├── ansi.go
    │   │   ├── application.go
    │   │   ├── attr.go
    │   │   ├── backend.go
    │   │   ├── child_env_test.go
    │   │   ├── child_env.go
    │   │   ├── child_process.go
    │   │   ├── clipboard_async.go
    │   │   ├── clipboard_test.go
    │   │   ├── clipboard.go
    │   │   ├── conpty_package_test.go
    │   │   ├── conpty_package.go
    │   │   ├── console_buffer_other.go
    │   │   ├── console_buffer_windows.go
    │   │   ├── console_cmdline_test.go
    │   │   ├── console_cmdline.go
    │   │   ├── console_host_windows.go
    │   │   ├── console_overlay_other.go
    │   │   ├── console_overlay_windows_test.go
    │   │   ├── console_overlay_windows.go
    │   │   ├── console_palette_windows_test.go
    │   │   ├── console_palette_windows.go
    │   │   ├── console_scroll_other.go
    │   │   ├── console_scroll_test.go
    │   │   ├── console_scroll_windows.go
    │   │   ├── console_scroll.go
    │   │   ├── console_spawn_other.go
    │   │   ├── console_spawn_windows.go
    │   │   ├── far2l_auth_test.go
    │   │   ├── far2l_auth.go
    │   │   ├── far2l_image_test.go
    │   │   ├── far2l_image.go
    │   │   ├── graphics_compat_test.go
    │   │   ├── graphics_compat.go
    │   │   ├── graphics_probe_test.go
    │   │   ├── graphics_probe_windows.go
    │   │   ├── graphics_probe.go
    │   │   ├── jobs_test.go
    │   │   ├── jobs.go
    │   │   ├── kitty_coverage_test.go
    │   │   ├── kitty_metrics_test.go
    │   │   ├── kitty_placements_test.go
    │   │   ├── kitty_placements.go
    │   │   ├── kitty_test.go
    │   │   ├── kitty.go
    │   │   ├── local_command_capture_test.go
    │   │   ├── local_command_capture.go
    │   │   ├── local_command_inline_other.go
    │   │   ├── local_command_inline_windows.go
    │   │   ├── log_console_other.go
    │   │   ├── log_console_windows.go
    │   │   ├── log_vfs_coverage_test.go
    │   │   ├── log_vfs_test.go
    │   │   ├── log_vfs.go
    │   │   ├── main_test.go
    │   │   ├── native_command_other.go
    │   │   ├── native_command_windows.go
    │   │   ├── overlay.go
    │   │   ├── pe_subsystem_test.go
    │   │   ├── pe_subsystem.go
    │   │   ├── portable_pty_windows_test.go
    │   │   ├── process_environment_runtime_unix.go
    │   │   ├── process_environment_runtime_windows.go
    │   │   ├── process_environment_test.go
    │   │   ├── process_environment.go
    │   │   ├── pty_bsd_dragonfly.go
    │   │   ├── pty_bsd_freebsd.go
    │   │   ├── pty_bsd_test.go
    │   │   ├── pty_bsd.go
    │   │   ├── pty_cloexec_test.go
    │   │   ├── pty_darwin.go
    │   │   ├── pty_diag_unix_test.go
    │   │   ├── pty_diag_unix.go
    │   │   ├── pty_diag_windows.go
    │   │   ├── pty_linux.go
    │   │   ├── pty_logical_lines_solaris.go
    │   │   ├── pty_logical_lines_unix.go
    │   │   ├── pty_logical_lines.go
    │   │   ├── pty_pollable_test.go
    │   │   ├── pty_ptm_netbsd.go
    │   │   ├── pty_ptm_openbsd.go
    │   │   ├── pty_ptm.go
    │   │   ├── pty_solaris.go
    │   │   ├── pty_test.go
    │   │   ├── pty_windows_test.go
    │   │   ├── pty_windows.go
    │   │   ├── pty_wine_other.go
    │   │   ├── pty_wine_shell_test.go
    │   │   ├── pty_wine_shell.go
    │   │   ├── pty_wine_windows.go
    │   │   ├── pty.go
    │   │   ├── qt_pty_windows_async_test.go
    │   │   ├── qt_terminal_semantic_live_test.go
    │   │   ├── qt_terminal_semantic_window_test.go
    │   │   ├── qt_terminal_semantic.go
    │   │   ├── redraw_stop_test.go
    │   │   ├── redraw.go
    │   │   ├── runner_test.go
    │   │   ├── runner_unix_test.go
    │   │   ├── runner_unix.go
    │   │   ├── runner_windows_test.go
    │   │   ├── runner_windows.go
    │   │   ├── runner.go
    │   │   ├── selection_test.go
    │   │   ├── selection.go
    │   │   ├── session_attach_identity_test.go
    │   │   ├── session_attach_payload_test.go
    │   │   ├── session_daemon_test.go
    │   │   ├── session_unix_coverage_extra_test.go
    │   │   ├── session_unix_coverage_test.go
    │   │   ├── session_unix_test.go
    │   │   ├── session_unix.go
    │   │   ├── session_windows.go
    │   │   ├── shellmode_test.go
    │   │   ├── shellmode.go
    │   │   ├── sixel_layers_test.go
    │   │   ├── sixel_terminal_test.go
    │   │   ├── sixel_terminal.go
    │   │   ├── sixel_test.go
    │   │   ├── sixel.go
    │   │   ├── solaris_pty_alloc_test.go
    │   │   ├── solaris_pty_backend_test.go
    │   │   ├── solaris_pty.go
    │   │   ├── solaris_streams_mock_linux_test.go
    │   │   ├── solaris_streams_mock_other_test.go
    │   │   ├── solaris_streams_mock_test.go
    │   │   ├── solaris_streams_test.go
    │   │   ├── solaris_streams.go
    │   │   ├── ttyx_probe_coverage_unix_test.go
    │   │   ├── ttyx_probe_parse.go
    │   │   ├── ttyx_probe_test.go
    │   │   ├── ttyx_probe_unix.go
    │   │   ├── ttyx_probe_windows.go
    │   │   ├── ttyx_probe.go
    │   │   ├── ttyx_session.go
    │   │   ├── view_reflow_test.go
    │   │   ├── view_reflow.go
    │   │   ├── view_semantic_test.go
    │   │   ├── view_test.go
    │   │   ├── view.go
    │   │   ├── window_helpers_test.go
    │   │   ├── wineprobe_escape_other.go
    │   │   ├── wineprobe_escape_windows.go
    │   │   ├── wineprobe_other.go
    │   │   ├── wineprobe_test.go
    │   │   ├── wineprobe_windows.go
    │   │   ├── wineprobe.go
    │   │   └── zzz_pty_leak_check_test.go
    │   ├── testutil
    │   │   ├── dialog.go
    │   │   ├── doc.go
    │   │   ├── frame_test.go
    │   │   ├── frame.go
    │   │   ├── input.go
    │   │   ├── main.go
    │   │   ├── numeric.go
    │   │   ├── paths_coverage_test.go
    │   │   ├── paths.go
    │   │   ├── race_disabled.go
    │   │   ├── race_enabled.go
    │   │   ├── rpc.go
    │   │   └── subprocess.go
    │   ├── textlayout
    │   │   ├── cluster.go
    │   │   ├── mapping_test.go
    │   │   ├── mapping.go
    │   │   ├── wrap_incremental_test.go
    │   │   ├── wrap_projection_test.go
    │   │   ├── wrap_test.go
    │   │   └── wrap.go
    │   ├── textsearch
    │   │   └── search.go
    │   ├── theme
    │   │   ├── colors_cursor_test.go
    │   │   ├── colors_test.go
    │   │   ├── colors.go
    │   │   ├── colorspace_test.go
    │   │   ├── colorspace.go
    │   │   ├── farcolor_test.go
    │   │   ├── farcolor.go
    │   │   ├── file_icon.go
    │   │   ├── highlight_files_test.go
    │   │   ├── highlight.go
    │   │   ├── style_combo_colors_test.go
    │   │   ├── style_completeness_test.go
    │   │   ├── style_custom_test.go
    │   │   ├── style_default_dark_test.go
    │   │   ├── style_indicator_test.go
    │   │   ├── style_overrides_test.go
    │   │   ├── style_test.go
    │   │   ├── style.go
    │   │   ├── styles
    │   │   │   ├── classic.ini
    │   │   │   ├── default_dark.ini
    │   │   │   ├── modern.ini
    │   │   │   ├── radiola.ini
    │   │   │   └── radiola.md
    │   │   └── table.go
    │   ├── toast
    │   │   └── toast.go
    │   ├── ttyx
    │   │   ├── coverage_edges_test.go
    │   │   ├── coverage_state_test.go
    │   │   ├── keys_coverage_test.go
    │   │   ├── keys.go
    │   │   ├── openfor_test.go
    │   │   ├── overlay_lifecycle_test.go
    │   │   ├── overlay.go
    │   │   ├── session_state_test.go
    │   │   ├── session.go
    │   │   ├── state_coverage_test.go
    │   │   ├── ttyx_events_test.go
    │   │   ├── ttyx_test.go
    │   │   ├── watch_coverage_test.go
    │   │   └── watch.go
    │   ├── unpack
    │   │   ├── coverage_test.go
    │   │   ├── unpack_test.go
    │   │   └── unpack.go
    │   ├── update
    │   │   ├── assets_test.go
    │   │   ├── cli_test.go
    │   │   ├── cli.go
    │   │   ├── coverage_test.go
    │   │   ├── elevation_manual_windows_test.go
    │   │   ├── elevation_other.go
    │   │   ├── elevation_windows.go
    │   │   ├── helper_args.go
    │   │   ├── libc_default_test.go
    │   │   ├── libc_default.go
    │   │   ├── libc_musl_test.go
    │   │   ├── libc_musl.go
    │   │   ├── selfexec_linux_test.go
    │   │   ├── selfexec_linux.go
    │   │   ├── selfexec_other.go
    │   │   ├── selfexec_termux.go
    │   │   ├── selfexec_test.go
    │   │   ├── selfexec.go
    │   │   ├── update_test.go
    │   │   └── update.go
    │   ├── viewer
    │   │   ├── application.go
    │   │   ├── backend_test.go
    │   │   ├── backend.go
    │   │   ├── binary.go
    │   │   ├── colorizer.go
    │   │   ├── content_revision_test.go
    │   │   ├── disasm_test.go
    │   │   ├── disasm.go
    │   │   ├── document_reader_test.go
    │   │   ├── highlight_test.go
    │   │   ├── highlight.go
    │   │   ├── links_test.go
    │   │   ├── links.go
    │   │   ├── main_test.go
    │   │   ├── menubar_test.go
    │   │   ├── mode.go
    │   │   ├── protocol_helpers_test.go
    │   │   ├── qt_document_viewer_loading_test.go
    │   │   ├── qt_document_viewer_projection.go
    │   │   ├── qt_semantic_window_render.go
    │   │   ├── qt_semantic.go
    │   │   ├── qt_viewer_document_projection_test.go
    │   │   ├── qt_viewer_end_navigation.go
    │   │   ├── qt_viewer_end_tail_test.go
    │   │   ├── qt_viewer_navigation_intent_test.go
    │   │   ├── range_loading_test.go
    │   │   ├── reader_profile_test.go
    │   │   ├── search_coverage_test.go
    │   │   ├── search.go
    │   │   ├── semantic_model_test.go
    │   │   ├── tail_test.go
    │   │   ├── task_helpers_test.go
    │   │   ├── text_test.go
    │   │   ├── text.go
    │   │   ├── title.go
    │   │   ├── topbar_test.go
    │   │   ├── topbar.go
    │   │   ├── unwrapped_seek_test.go
    │   │   ├── view_coverage_extra_test.go
    │   │   ├── view_semantic_contract_test.go
    │   │   ├── view_semantic.go
    │   │   ├── view_test.go
    │   │   ├── view.go
    │   │   ├── viewport.go
    │   │   ├── window_protocol_test.go
    │   │   ├── window_render_test.go
    │   │   ├── wordnav_test.go
    │   │   └── wordnav.go
    │   ├── vtvibe
    │   │   ├── ap_test.go
    │   │   ├── ap.go
    │   │   ├── coverage_test.go
    │   │   ├── defaults.go
    │   │   ├── memtree.go
    │   │   ├── pack_test.go
    │   │   ├── pack.go
    │   │   ├── provider_test.go
    │   │   ├── provider.go
    │   │   ├── session_test.go
    │   │   ├── session.go
    │   │   ├── vfs_coverage_test.go
    │   │   └── vfs.go
    │   ├── wincon
    │   │   ├── blit_test.go
    │   │   ├── coverage_test.go
    │   │   ├── geometry.go
    │   │   ├── layered_test.go
    │   │   ├── layered.go
    │   │   ├── overlay_other.go
    │   │   ├── overlay_state_test.go
    │   │   ├── overlay_state.go
    │   │   ├── overlay_windows_test.go
    │   │   ├── overlay_windows.go
    │   │   ├── stats_windows.go
    │   │   ├── stats.go
    │   │   ├── wincon_test.go
    │   │   ├── windowlongptr_32.go
    │   │   └── windowlongptr_64.go
    │   └── winshell
    │       ├── broker_windows.go
    │       ├── client_stub.go
    │       ├── client_windows_integration_test.go
    │       ├── client_windows.go
    │       ├── context_windows.go
    │       ├── icon_windows.go
    │       ├── native_windows_test.go
    │       ├── native_windows.go
    │       ├── register_stub.go
    │       ├── register_windows.go
    │       ├── sta_windows.go
    │       ├── types.go
    │       ├── uri_test.go
    │       ├── uri.go
    │       ├── vfs_test.go
    │       └── vfs.go
    ├── LICENSE
    ├── packaging
    │   ├── linux
    │   │   └── f4.desktop
    │   └── macos
    │       └── Info.plist
    ├── plugins
    │   ├── android
    │   │   ├── adb_integration_test.go
    │   │   ├── adb_sync_test.go
    │   │   ├── adb_sync.go
    │   │   ├── adb_transport_test.go
    │   │   ├── adb_transport.go
    │   │   ├── command_runner_info_test.go
    │   │   ├── coverage_contract_test.go
    │   │   ├── device_test.go
    │   │   ├── device.go
    │   │   ├── fish_pool_test.go
    │   │   ├── fish_pool.go
    │   │   ├── info_paths_test.go
    │   │   ├── info_test.go
    │   │   ├── info.go
    │   │   ├── manager_test.go
    │   │   ├── manager.go
    │   │   ├── pathutil_test.go
    │   │   ├── pathutil.go
    │   │   ├── README.md
    │   │   ├── sync_vfs_paths_test.go
    │   │   ├── sync_vfs_test.go
    │   │   ├── sync_vfs.go
    │   │   ├── uri_test.go
    │   │   └── uri.go
    │   ├── archive
    │   │   ├── archive_materialize_unix_test.go
    │   │   ├── archive_plugin_test.go
    │   │   ├── archive_test.go
    │   │   ├── archive_write_regression_test.go
    │   │   ├── archive.go
    │   │   ├── clone_test.go
    │   │   ├── compressed_regular_test.go
    │   │   ├── extraction_security_test.go
    │   │   ├── issue1179_sfx_test.go
    │   │   ├── issue1179_volumes_test.go
    │   │   ├── issue1184_test.go
    │   │   ├── issue1186_sfx_zip_test.go
    │   │   ├── issue1186_split_zip_test.go
    │   │   ├── issue815_f3_test.go
    │   │   ├── issue816_multivolume_test.go
    │   │   ├── issue816_password_retry_test.go
    │   │   ├── issue915_total_progress_test.go
    │   │   ├── materialize.go
    │   │   ├── multivolume_rar.go
    │   │   ├── nested_detection_test.go
    │   │   ├── password_coverage_test.go
    │   │   ├── password_test.go
    │   │   ├── password.go
    │   │   ├── production_regression_test.go
    │   │   ├── provider_special_unix_test.go
    │   │   ├── provider_test.go
    │   │   ├── provider.go
    │   │   ├── remove_directory_test.go
    │   │   ├── repro_test.go
    │   │   ├── sfx_test.go
    │   │   ├── sfx.go
    │   │   ├── vfs_nested_test.go
    │   │   ├── vfs_test.go
    │   │   ├── vfs.go
    │   │   ├── zip_encoding_test.go
    │   │   ├── zip_encoding.go
    │   │   └── zipcrypto_checkbyte_test.go
    │   ├── chroma
    │   │   ├── chroma_test.go
    │   │   └── chroma.go
    │   ├── cloudfox
    │   │   ├── cloud_vfs_coverage_test.go
    │   │   ├── cloud_vfs_share_test.go
    │   │   ├── cloud_vfs_test.go
    │   │   ├── cloud_vfs.go
    │   │   ├── connection_action_test.go
    │   │   ├── context_editor_test.go
    │   │   ├── credential_scope_test.go
    │   │   ├── credential_scope.go
    │   │   ├── dialog_coverage_extra_test.go
    │   │   ├── dialog_google_test.go
    │   │   ├── dialog_s3_test.go
    │   │   ├── dialog.go
    │   │   ├── manager.go
    │   │   ├── oauth.go
    │   │   ├── password_prompt_test.go
    │   │   ├── password_prompt.go
    │   │   ├── plugin_contributions_test.go
    │   │   ├── plugin_test.go
    │   │   ├── plugin.go
    │   │   ├── provider_capabilities_test.go
    │   │   ├── provider_google_production_test.go
    │   │   ├── provider_google_real_native_integration_test.go
    │   │   ├── provider_google_share_test.go
    │   │   ├── provider_google_share.go
    │   │   ├── provider_google_test.go
    │   │   ├── provider_google.go
    │   │   ├── provider_helpers_test.go
    │   │   ├── provider_helpers.go
    │   │   ├── provider_mutation_test.go
    │   │   ├── provider_real_diagnostics_test.go
    │   │   ├── provider_real_saved_integration_test.go
    │   │   ├── provider_real_semantics_test.go
    │   │   ├── provider_real_sharing_integration_test.go
    │   │   ├── provider_real_upload_cancellation_test.go
    │   │   ├── provider_s3_discovery_core_test.go
    │   │   ├── provider_s3_discovery_regression_test.go
    │   │   ├── provider_s3_real_discovery_test.go
    │   │   ├── provider_s3_share_test.go
    │   │   ├── provider_s3_share.go
    │   │   ├── provider_s3_test.go
    │   │   ├── provider_s3.go
    │   │   ├── provider_webdav_edge_test.go
    │   │   ├── provider_webdav_integration_test.go
    │   │   ├── provider_webdav_share_test.go
    │   │   ├── provider_webdav_share.go
    │   │   ├── provider_webdav_test.go
    │   │   ├── provider_webdav.go
    │   │   ├── provider_yandex_cache.go
    │   │   ├── provider_yandex_info.go
    │   │   ├── provider_yandex_production_test.go
    │   │   ├── provider_yandex_share_test.go
    │   │   ├── provider_yandex_share.go
    │   │   ├── provider_yandex_test.go
    │   │   ├── provider_yandex.go
    │   │   ├── secrets_test.go
    │   │   ├── secrets.go
    │   │   ├── session.go
    │   │   ├── settings_center_test.go
    │   │   ├── settings_center.go
    │   │   ├── settings_russian_test.go
    │   │   ├── store_lock_unix.go
    │   │   ├── store_lock_windows.go
    │   │   ├── store_test.go
    │   │   ├── store.go
    │   │   ├── test_main_test.go
    │   │   ├── types.go
    │   │   ├── uri_test.go
    │   │   ├── uri.go
    │   │   ├── vault.go
    │   │   └── yandex_code_prompt.go
    │   ├── dummy_internal
    │   │   ├── dummy_internal_test.go
    │   │   └── dummy_internal.go
    │   ├── dummy_lua
    │   │   ├── plugin.lua
    │   │   └── README.md
    │   ├── dummy_rpc
    │   │   ├── main_test.go
    │   │   └── main.go
    │   ├── envman
    │   │   ├── codec_test.go
    │   │   ├── codec.go
    │   │   ├── commands_test.go
    │   │   ├── commands.go
    │   │   ├── dialogs.go
    │   │   ├── environment_document.go
    │   │   ├── far3_import_other.go
    │   │   ├── far3_import_test.go
    │   │   ├── far3_import_ui.go
    │   │   ├── far3_import_windows_test.go
    │   │   ├── far3_import_windows.go
    │   │   ├── far3_import.go
    │   │   ├── manager_footer.go
    │   │   ├── manager_frame.go
    │   │   ├── manager_ops.go
    │   │   ├── manager_resize_test.go
    │   │   ├── manager_semantic_test.go
    │   │   ├── manager_semantic.go
    │   │   ├── manager_ui.go
    │   │   ├── messages.go
    │   │   ├── model_test.go
    │   │   ├── model.go
    │   │   ├── plugin_test.go
    │   │   ├── plugin.go
    │   │   ├── README.md
    │   │   ├── settings_center.go
    │   │   ├── settings_russian_test.go
    │   │   ├── settings_test.go
    │   │   ├── settings.go
    │   │   ├── strings.go
    │   │   ├── ui_test.go
    │   │   └── vfs_io.go
    │   ├── id3editor
    │   │   ├── plugin_contributions_test.go
    │   │   ├── plugin_handle_test.go
    │   │   ├── plugin_paths_test.go
    │   │   ├── plugin_test.go
    │   │   └── plugin.go
    │   ├── ios
    │   │   ├── afc_vfs_contract_test.go
    │   │   ├── afc_vfs_test.go
    │   │   ├── afc_vfs.go
    │   │   ├── apps.go
    │   │   ├── core_access_coverage_test.go
    │   │   ├── core_access_stub.go
    │   │   ├── core_access_supported_test.go
    │   │   ├── core_access_supported.go
    │   │   ├── core_access.go
    │   │   ├── core_tunnel_supported.go
    │   │   ├── core_vfs_test.go
    │   │   ├── core_vfs.go
    │   │   ├── coverage_edges_test.go
    │   │   ├── coverage_plugins_test.go
    │   │   ├── coverage_test.go
    │   │   ├── device_path.go
    │   │   ├── internal
    │   │   │   ├── afcproto
    │   │   │   │   ├── client_test.go
    │   │   │   │   ├── client.go
    │   │   │   │   ├── doc.go
    │   │   │   │   ├── errors.go
    │   │   │   │   ├── file.go
    │   │   │   │   ├── path.go
    │   │   │   │   ├── protocol_test.go
    │   │   │   │   ├── protocol.go
    │   │   │   │   └── types.go
    │   │   │   └── corefileservice
    │   │   │       ├── doc.go
    │   │   │       ├── fileservice_test.go
    │   │   │       └── fileservice.go
    │   │   ├── ios_integration_test.go
    │   │   ├── LICENSE.go-ios
    │   │   ├── manager_test.go
    │   │   ├── manager.go
    │   │   ├── native_source.go
    │   │   ├── plugin_test.go
    │   │   ├── plugin.go
    │   │   ├── README.md
    │   │   ├── selectors_test.go
    │   │   ├── selectors.go
    │   │   ├── services_test.go
    │   │   ├── services.go
    │   │   ├── uri_test.go
    │   │   └── uri.go
    │   ├── mediainfo
    │   │   ├── analyzer.go
    │   │   ├── backend_test.go
    │   │   ├── cache_test.go
    │   │   ├── cache.go
    │   │   ├── config_dialog_test.go
    │   │   ├── config_dialog.go
    │   │   ├── coverage_contract_more_test.go
    │   │   ├── coverage_test.go
    │   │   ├── dialog_theme_test.go
    │   │   ├── dialog_util_coverage_test.go
    │   │   ├── dialog.go
    │   │   ├── exif_report_test.go
    │   │   ├── format_gaps_test.go
    │   │   ├── locale.go
    │   │   ├── macro_test.go
    │   │   ├── macro.go
    │   │   ├── matroska_bounds_test.go
    │   │   ├── model.go
    │   │   ├── open_coverage_test.go
    │   │   ├── open_test.go
    │   │   ├── open.go
    │   │   ├── parse_audio.go
    │   │   ├── parse_ebu_stl.go
    │   │   ├── parse_heif.go
    │   │   ├── parse_image.go
    │   │   ├── parse_iso_coverage_test.go
    │   │   ├── parse_iso.go
    │   │   ├── parse_matroska.go
    │   │   ├── parse_riff.go
    │   │   ├── parse_subtitle.go
    │   │   ├── parse_tiff_limits_test.go
    │   │   ├── parse_tiff.go
    │   │   ├── plugin_language_switch_test.go
    │   │   ├── plugin_test.go
    │   │   ├── plugin.go
    │   │   ├── quickview_provider_test.go
    │   │   ├── quickview_provider.go
    │   │   ├── README.md
    │   │   ├── render_limits_test.go
    │   │   ├── render.go
    │   │   ├── report_text.go
    │   │   ├── report_view.go
    │   │   ├── settings_center.go
    │   │   ├── settings_russian_test.go
    │   │   ├── settings_test.go
    │   │   ├── settings.go
    │   │   ├── source.go
    │   │   ├── subtitle_limits_test.go
    │   │   └── util.go
    │   ├── netfox
    │   │   ├── connection_action_test.go
    │   │   ├── context_editor_test.go
    │   │   ├── coverage_test.go
    │   │   ├── crypto_test.go
    │   │   ├── crypto.go
    │   │   ├── dev
    │   │   │   ├── README.md
    │   │   │   └── unxed_f4_issue_316.json
    │   │   ├── dialog_test.go
    │   │   ├── dialog.go
    │   │   ├── fish_clone_session_test.go
    │   │   ├── fish_dialer_test.go
    │   │   ├── fish_pool.go
    │   │   ├── fish_reconnect_entry_test.go
    │   │   ├── fish_reconnect_test.go
    │   │   ├── fish_vfs_device_paths_test.go
    │   │   ├── fish_vfs_test.go
    │   │   ├── fish_vfs.go
    │   │   ├── fishplus
    │   │   │   ├── cancel_test.go
    │   │   │   ├── cand
    │   │   │   ├── exec_test.go
    │   │   │   ├── exec.go
    │   │   │   ├── fs_test.go
    │   │   │   ├── fs.go
    │   │   │   ├── hash_test.go
    │   │   │   ├── hash.go
    │   │   │   ├── helper.ps1
    │   │   │   ├── helper.sh
    │   │   │   ├── job_test.go
    │   │   │   ├── job.go
    │   │   │   ├── keepalive_test.go
    │   │   │   ├── keepalive.go
    │   │   │   ├── ls_test.go
    │   │   │   ├── ls.go
    │   │   │   ├── mutate_test.go
    │   │   │   ├── mutate.go
    │   │   │   ├── patch_test.go
    │   │   │   ├── patch.go
    │   │   │   ├── paths_test.go
    │   │   │   ├── paths.go
    │   │   │   ├── random_fixture_test.go
    │   │   │   ├── read_test.go
    │   │   │   ├── read.go
    │   │   │   ├── script_pwsh_test.go
    │   │   │   ├── script_test.go
    │   │   │   ├── script.go
    │   │   │   ├── search_test.go
    │   │   │   ├── search.go
    │   │   │   ├── session_pwsh_test.go
    │   │   │   ├── session_test.go
    │   │   │   ├── session.go
    │   │   │   ├── sizes
    │   │   │   ├── WINDOWS_PORT.md
    │   │   │   ├── write_test.go
    │   │   │   └── write.go
    │   │   ├── ftp_clone_test.go
    │   │   ├── ftp_vfs_contract_test.go
    │   │   ├── ftp_vfs.go
    │   │   ├── lang_test.go
    │   │   ├── netfox_test.go
    │   │   ├── netfox.go
    │   │   ├── plugin_contributions_test.go
    │   │   ├── proxy_dialog_coverage_test.go
    │   │   ├── proxy_dialog.go
    │   │   ├── registry.go
    │   │   ├── settings_center_test.go
    │   │   ├── settings_center.go
    │   │   ├── settings_russian_test.go
    │   │   ├── sftp_command_test.go
    │   │   ├── sftp_dial_test.go
    │   │   ├── sftp_uri.go
    │   │   ├── sftp_vfs_coverage_extra_test.go
    │   │   ├── sftp_vfs.go
    │   │   ├── ssh_agent_forwarding_test.go
    │   │   ├── ssh_agent_unix.go
    │   │   ├── ssh_agent_windows_test.go
    │   │   ├── ssh_agent_windows.go
    │   │   ├── ssh_dial_test.go
    │   │   ├── ssh_dial.go
    │   │   ├── ssh_keepalive_test.go
    │   │   ├── ssh_known_hosts_test.go
    │   │   ├── ssh_known_hosts.go
    │   │   ├── ssh_pty.go
    │   │   ├── vfs_abs_test.go
    │   │   └── vfs.go
    │   ├── sqlite
    │   │   ├── coverage_test.go
    │   │   ├── locale.go
    │   │   ├── plugin_test.go
    │   │   ├── plugin.go
    │   │   ├── ui_test.go
    │   │   └── ui.go
    │   └── visren
    │       ├── config_test.go
    │       ├── config.go
    │       ├── coverage_contract_test.go
    │       ├── coverage_test.go
    │       ├── dialog_coverage_extra_test.go
    │       ├── dialog_test.go
    │       ├── dialog.go
    │       ├── editor_test.go
    │       ├── editor.go
    │       ├── engine_test.go
    │       ├── LICENSE.upstream
    │       ├── masks.go
    │       ├── metadata_test.go
    │       ├── metadata.go
    │       ├── model.go
    │       ├── plugin_language_switch_test.go
    │       ├── plugin_test.go
    │       ├── plugin.go
    │       ├── rename_test.go
    │       ├── rename.go
    │       ├── replace.go
    │       ├── settings_center_test.go
    │       ├── settings_center.go
    │       ├── settings_russian_test.go
    │       ├── transforms.go
    │       └── word_div_prompt_test.go
    ├── plugring
    │   ├── hello_plugring.lua
    │   └── index.yaml
    ├── qt
    │   └── host
    │       ├── cmake
    │       │   ├── f4-qt-host-target
    │       │   │   └── CMakeLists.txt
    │       │   ├── f4-zoin-gallery-portable-hook.cmake
    │       │   ├── F4HostQmlFiles.cmake
    │       │   └── SplitMsvcLinkResponse.cmake
    │       ├── CMakeLists.txt
    │       ├── conanfile.py
    │       ├── icons
    │       │   ├── lucide
    │       │   │   ├── app-window.svg
    │       │   │   ├── archive.svg
    │       │   │   ├── arrow-down-a-z.svg
    │       │   │   ├── arrow-down-wide-narrow.svg
    │       │   │   ├── arrow-down.svg
    │       │   │   ├── arrow-left-from-line.svg
    │       │   │   ├── arrow-left.svg
    │       │   │   ├── arrow-right-from-line.svg
    │       │   │   ├── arrow-up.svg
    │       │   │   ├── binary.svg
    │       │   │   ├── blocks.svg
    │       │   │   ├── book-open.svg
    │       │   │   ├── check.svg
    │       │   │   ├── chevron-down.svg
    │       │   │   ├── chevron-right.svg
    │       │   │   ├── chevron-up.svg
    │       │   │   ├── circle-check.svg
    │       │   │   ├── circle-pause.svg
    │       │   │   ├── circle-play.svg
    │       │   │   ├── circle-question-mark.svg
    │       │   │   ├── circle-x.svg
    │       │   │   ├── clock-3.svg
    │       │   │   ├── cloud.svg
    │       │   │   ├── columns-2.svg
    │       │   │   ├── columns-3.svg
    │       │   │   ├── copy.svg
    │       │   │   ├── database.svg
    │       │   │   ├── disc.svg
    │       │   │   ├── eye.svg
    │       │   │   ├── file-archive.svg
    │       │   │   ├── file-clock.svg
    │       │   │   ├── file-code.svg
    │       │   │   ├── file-cog.svg
    │       │   │   ├── file-lock.svg
    │       │   │   ├── file-pen-line.svg
    │       │   │   ├── file-plus.svg
    │       │   │   ├── file-terminal.svg
    │       │   │   ├── file-text.svg
    │       │   │   ├── file-type.svg
    │       │   │   ├── file.svg
    │       │   │   ├── flask-conical.svg
    │       │   │   ├── folder-clock.svg
    │       │   │   ├── folder-input.svg
    │       │   │   ├── folder-kanban.svg
    │       │   │   ├── folder-lock.svg
    │       │   │   ├── folder-plus.svg
    │       │   │   ├── folder-root.svg
    │       │   │   ├── folder-symlink.svg
    │       │   │   ├── folder-up.svg
    │       │   │   ├── folder.svg
    │       │   │   ├── git-fork.svg
    │       │   │   ├── globe.svg
    │       │   │   ├── grid-3x3.svg
    │       │   │   ├── hard-drive.svg
    │       │   │   ├── house.svg
    │       │   │   ├── image.svg
    │       │   │   ├── images.svg
    │       │   │   ├── keyboard.svg
    │       │   │   ├── languages.svg
    │       │   │   ├── layout-dashboard.svg
    │       │   │   ├── library.svg
    │       │   │   ├── LICENSE
    │       │   │   ├── list-checks.svg
    │       │   │   ├── list-restart.svg
    │       │   │   ├── list-tree.svg
    │       │   │   ├── list.svg
    │       │   │   ├── loader-circle.svg
    │       │   │   ├── locate-fixed.svg
    │       │   │   ├── log-out.svg
    │       │   │   ├── maximize-2.svg
    │       │   │   ├── memory-stick.svg
    │       │   │   ├── menu.svg
    │       │   │   ├── monitor.svg
    │       │   │   ├── music.svg
    │       │   │   ├── network.svg
    │       │   │   ├── palette.svg
    │       │   │   ├── panel-left.svg
    │       │   │   ├── panel-right.svg
    │       │   │   ├── panels-top-left.svg
    │       │   │   ├── pencil.svg
    │       │   │   ├── plug.svg
    │       │   │   ├── plus.svg
    │       │   │   ├── refresh-cw.svg
    │       │   │   ├── rotate-ccw.svg
    │       │   │   ├── rows-3.svg
    │       │   │   ├── rows-4.svg
    │       │   │   ├── save.svg
    │       │   │   ├── search.svg
    │       │   │   ├── smartphone.svg
    │       │   │   ├── SOURCE.md
    │       │   │   ├── space.svg
    │       │   │   ├── sparkles.svg
    │       │   │   ├── square-terminal.svg
    │       │   │   ├── tablet.svg
    │       │   │   ├── text-wrap.svg
    │       │   │   ├── trash-2.svg
    │       │   │   ├── triangle-alert.svg
    │       │   │   ├── usb-flash-drive.svg
    │       │   │   ├── video.svg
    │       │   │   └── x.svg
    │       │   └── streamline
    │       │       ├── android-logo.svg
    │       │       ├── apple-logo.svg
    │       │       └── microsoft-windows-logo.svg
    │       ├── packaging
    │       │   ├── f4-qt-host-Info.plist.in
    │       │   ├── f4-qt-host.rc.in
    │       │   └── qt.conf.in
    │       ├── qml
    │       │   ├── ActivityBoundedCursorBlink.qml
    │       │   ├── ApplicationMenuPopup.qml
    │       │   ├── AutocompletePopup.qml
    │       │   ├── CommandLineView.qml
    │       │   ├── ConfigDialogButton.qml
    │       │   ├── ConsoleRunRow.qml
    │       │   ├── DialogButton.qml
    │       │   ├── DialogCheckBox.qml
    │       │   ├── DialogComboBox.qml
    │       │   ├── DialogMultiLineEdit.qml
    │       │   ├── DialogOverlay.qml
    │       │   ├── DialogProgressBar.qml
    │       │   ├── DialogRadioButton.qml
    │       │   ├── DialogResizeHandle.qml
    │       │   ├── DialogTextField.qml
    │       │   ├── DocumentEditorPointerController.qml
    │       │   ├── DocumentHeader.qml
    │       │   ├── DocumentRowDelegate.qml
    │       │   ├── DocumentRowPool.qml
    │       │   ├── DocumentSurface.qml
    │       │   ├── DocumentTerminalSelectionController.qml
    │       │   ├── DocumentViewportController.qml
    │       │   ├── DocumentWindowPresenter.qml
    │       │   ├── EnvironmentProfilesBody.qml
    │       │   ├── F4Button.qml
    │       │   ├── F4CheckBox.qml
    │       │   ├── F4ComboBox.qml
    │       │   ├── F4HostWindow.qml
    │       │   ├── F4RadioButton.qml
    │       │   ├── F4ScrollBar.qml
    │       │   ├── F4Slider.qml
    │       │   ├── F4TextField.qml
    │       │   ├── F4ToolTip.qml
    │       │   ├── FilePanelChrome.qml
    │       │   ├── FilePanelView.qml
    │       │   ├── GalleryPanelHost.qml
    │       │   ├── GalleryPanelHostAdapter.qml
    │       │   ├── GalleryPanelInputRouter.qml
    │       │   ├── GallerySettingsPage.qml
    │       │   ├── GalleryViewerHost.qml
    │       │   ├── GenericDialog.qml
    │       │   ├── HelpContent.qml
    │       │   ├── HostPixelAlignedImage.qml
    │       │   ├── HostPresentationUtilities.qml
    │       │   ├── HostThemePalette.qml
    │       │   ├── InfoPanelView.qml
    │       │   ├── KeyBarActionButton.qml
    │       │   ├── KeyBarView.qml
    │       │   ├── main.qml
    │       │   ├── NativeSettingsPage.qml
    │       │   ├── OperationsQueueController.qml
    │       │   ├── OperationsQueueSurface.qml
    │       │   ├── OverlayHost.qml
    │       │   ├── PanelPairSurface.qml
    │       │   ├── PanelRendererMenu.qml
    │       │   ├── PanelSplitter.qml
    │       │   ├── PanelsSurface.qml
    │       │   ├── PanelStatusMetric.qml
    │       │   ├── PanelStatusOverlay.qml
    │       │   ├── QueueActionButton.qml
    │       │   ├── QueueSummaryItem.qml
    │       │   ├── QuickViewController.qml
    │       │   ├── QuickViewPanelView.qml
    │       │   ├── SemanticChoiceGroup.qml
    │       │   ├── SemanticDialogLayout.qml
    │       │   ├── SemanticGroupViewport.qml
    │       │   ├── SemanticListPointer.qml
    │       │   ├── SemanticMenuBar.qml
    │       │   ├── SemanticMenuItemDelegate.qml
    │       │   ├── SemanticMenuPopup.qml
    │       │   ├── SemanticWidgetDelegate.qml
    │       │   ├── SettingsDialogBody.qml
    │       │   ├── ShellInteractionController.qml
    │       │   ├── ShellSceneStore.qml
    │       │   ├── ShellShortcuts.qml
    │       │   ├── ShellSurfaceHost.qml
    │       │   ├── ShellTitleBar.qml
    │       │   ├── TerminalBackdrop.qml
    │       │   ├── TerminalColorsPage.qml
    │       │   ├── TerminalPaletteModel.qml
    │       │   ├── ThemeBooleanOption.qml
    │       │   ├── ThemeColorEditorPane.qml
    │       │   ├── ThemeColorWheel.qml
    │       │   ├── ThemeDraftModel.qml
    │       │   ├── ThemeEditor.qml
    │       │   ├── ThemeEditorContent.qml
    │       │   ├── ThemeEditorFooter.qml
    │       │   ├── ThemeRenderTypeComboBox.qml
    │       │   ├── ToastView.qml
    │       │   └── WorkspaceTabs.qml
    │       ├── README.md
    │       ├── src
    │       │   ├── DummyQWK.h
    │       │   ├── ExtUiMessageDecoder.cpp
    │       │   ├── ExtUiMessageDecoder.h
    │       │   ├── ExtUiProtocol.cpp
    │       │   ├── ExtUiProtocol.h
    │       │   ├── ExtUiScenePatchReducer.cpp
    │       │   ├── ExtUiSceneReducer.cpp
    │       │   ├── ExtUiSceneReducer.h
    │       │   ├── ExtUiSnapshotReducer.cpp
    │       │   ├── ExtUiStateStores.cpp
    │       │   ├── ExtUiStateStores.h
    │       │   ├── ExtUiTransport.cpp
    │       │   ├── ExtUiTransport.h
    │       │   ├── F4ApplicationIcon.cpp
    │       │   ├── F4ApplicationIcon.h
    │       │   ├── F4DirectoryPreviewProvider.cpp
    │       │   ├── F4DirectoryPreviewProvider.h
    │       │   ├── F4GalleryBridge.cpp
    │       │   ├── F4GalleryBridge.h
    │       │   ├── F4GalleryBridgeBenchmark.cpp
    │       │   ├── F4GalleryBridgeCatalog.cpp
    │       │   ├── F4GalleryBridgeCatalogAppend.cpp
    │       │   ├── F4GalleryBridgeDragDrop.cpp
    │       │   ├── F4GalleryBridgeIntents.cpp
    │       │   ├── F4GalleryBridgeMetadataResponse.cpp
    │       │   ├── F4GalleryBridgePanelState.cpp
    │       │   ├── F4GalleryBridgePanelSync.cpp
    │       │   ├── F4GalleryBridgeProtocol.cpp
    │       │   ├── F4GalleryBridgeQuickView.cpp
    │       │   ├── F4GalleryBridgeRowsResponse.cpp
    │       │   ├── F4GalleryBridgeSceneSync.cpp
    │       │   ├── F4GallerySourceDescriptor.h
    │       │   ├── F4IconProvider.cpp
    │       │   ├── F4IconProvider.h
    │       │   ├── F4ImageSourceProvider.cpp
    │       │   ├── F4ImageSourceProvider.h
    │       │   ├── F4NativeDragVisuals.h
    │       │   ├── F4QuickViewPreferences.h
    │       │   ├── F4TextRenderingPolicy.cpp
    │       │   ├── F4TextRenderingPolicy.h
    │       │   ├── F4ThemePersistence.cpp
    │       │   ├── F4ThemePersistence.h
    │       │   ├── F4WorktreeIdentity.cpp
    │       │   ├── F4WorktreeIdentity.h
    │       │   ├── MacApplicationMenu.h
    │       │   ├── MacApplicationMenu.mm
    │       │   ├── MacPlatformServices.h
    │       │   ├── MacPlatformServices.mm
    │       │   ├── MacSystemButtonLayout.h
    │       │   ├── MacSystemButtonLayout.mm
    │       │   ├── main.cpp
    │       │   ├── NavigationBenchmarkTrace.h
    │       │   ├── PanelCatalogModel.cpp
    │       │   ├── PanelCatalogModel.h
    │       │   ├── PanelIntentController.cpp
    │       │   ├── PanelIntentController.h
    │       │   ├── PanelSessionRegistry.cpp
    │       │   ├── PanelSessionRegistry.h
    │       │   ├── PointerRowAnchor.h
    │       │   ├── QtMediaClient.cpp
    │       │   ├── QtMediaClient.h
    │       │   ├── QtMediaClientWire.cpp
    │       │   ├── QtMediaClientWire.h
    │       │   ├── QtShellController.cpp
    │       │   ├── QtShellController.h
    │       │   ├── QtShellControllerFrameApply.cpp
    │       │   ├── QtShellControllerFrameApply.h
    │       │   ├── QtShellControllerFrameTrace.cpp
    │       │   ├── QtShellControllerPanelFrames.cpp
    │       │   ├── QtShellControllerSceneFrames.cpp
    │       │   ├── QtShellControllerStateStores.cpp
    │       │   ├── ScenePixelAlignment.cpp
    │       │   ├── ScenePixelAlignment.h
    │       │   ├── SemanticChildrenModel.cpp
    │       │   ├── SemanticChildrenModel.h
    │       │   ├── SemanticOverlayModel.cpp
    │       │   ├── SemanticOverlayModel.h
    │       │   ├── ShellStateStore.cpp
    │       │   ├── ShellStateStore.h
    │       │   ├── StaticQmlPluginImports.cpp
    │       │   ├── StaticQtPluginImports.cpp
    │       │   ├── ViewerCoordinator.cpp
    │       │   ├── ViewerCoordinator.h
    │       │   ├── VtuiGridItem.cpp
    │       │   ├── VtuiGridItem.h
    │       │   ├── WindowGeometryPersistence.cpp
    │       │   └── WindowGeometryPersistence.h
    │       └── tests
    │           ├── data
    │           │   └── mixed-dpi-screens.json
    │           ├── ExtUiProtocolTests.cpp
    │           ├── ExtUiSceneReducerTests.cpp
    │           ├── F4DocumentSurfaceTests.cpp
    │           ├── F4GalleryBridgeTests.cpp
    │           ├── F4GalleryPointerTests.cpp
    │           ├── F4IconProviderTests.cpp
    │           ├── F4OperationsQueueTests.cpp
    │           ├── F4PanelSplitterTests.cpp
    │           ├── F4QuickViewSurfaceTests.cpp
    │           ├── F4TextRenderingPolicyTests.cpp
    │           ├── F4WindowsApplicationIconTests.cpp
    │           ├── F4WorktreeIdentityTests.cpp
    │           ├── MacApplicationIconTests.mm
    │           ├── MacApplicationMenuTests.mm
    │           ├── MacPlatformServicesTests.mm
    │           ├── MacSystemButtonLayoutTests.mm
    │           ├── manual
    │           │   ├── DocumentRepeaterPerfProbe.qml
    │           │   ├── ListViewIncrementalRebaseProbe.qml
    │           │   ├── ListViewPrependRebaseProbe.qml
    │           │   ├── ListViewStableSlotPoolProbe.qml
    │           │   └── MsgpackSceneDecodeProbe.cpp
    │           ├── PanelCatalogModelTests.cpp
    │           ├── PanelIntentControllerTests.cpp
    │           ├── PanelSessionRegistryTests.cpp
    │           ├── QmlProfilerPlugins.cpp
    │           ├── QtMediaClientTests.cpp
    │           ├── QtShellControllerProductionTests.cpp
    │           ├── QtShellControllerTests.cpp
    │           ├── QWindowKitTitleBarTests.cpp
    │           ├── SemanticPresentationTypes.cpp
    │           ├── ShellStateStoreTests.cpp
    │           ├── StaticQtImagePluginImports.cpp
    │           ├── StaticQtTestPluginImports.cpp
    │           ├── TestExtUiStateController.cpp
    │           ├── TestExtUiStateController.h
    │           ├── TestGalleryPanel.qml
    │           ├── ViewerCoordinatorTests.cpp
    │           └── WindowGeometryPersistenceTests.cpp
    ├── README.md
    ├── run-f4-gallery.sh
    ├── run-f4-gogpu.sh
    ├── run-f4-qml.sh
    ├── scripts
    │   ├── filelist_update.sh
    │   ├── test_plugins.sh
    │   └── test_resurrect.sh
    ├── sdk
    │   ├── extui
    │   │   ├── cmd
    │   │   │   └── generate-fixtures
    │   │   │       └── main.go
    │   │   ├── envelope_test.go
    │   │   ├── envelope.go
    │   │   ├── fixtures_generate.go
    │   │   ├── model_coverage_test.go
    │   │   ├── model_test.go
    │   │   ├── model.go
    │   │   ├── operations_queue_model_test.go
    │   │   ├── patch.go
    │   │   ├── protocol_v4.md
    │   │   ├── terminal_palette_test.go
    │   │   └── testdata
    │   │       └── v4_envelopes.msgpack
    │   ├── f4plugin
    │   │   ├── metadata.go
    │   │   ├── plugin_test.go
    │   │   └── plugin.go
    │   ├── f4rpc
    │   │   ├── mux_test.go
    │   │   └── mux.go
    │   ├── f4settings
    │   │   ├── localization_test.go
    │   │   ├── settings_test.go
    │   │   ├── settings.go
    │   │   └── struct_provider.go
    │   └── lua
    │       └── f4rpc.lua
    ├── skills-lock.json
    ├── third_party
    │   ├── vtui
    │   │   ├── .github
    │   │   │   └── workflows
    │   │   │       └── ci.yml
    │   │   ├── .gitignore
    │   │   ├── .golangci.yml
    │   │   ├── ansi_writer_syscons_test.go
    │   │   ├── ansi_writer.go
    │   │   ├── ARCH_PROPOSALS.md
    │   │   ├── ARCHITECTURE.md
    │   │   ├── autocomplete_test.go
    │   │   ├── autocomplete_trigger_test.go
    │   │   ├── autocomplete.go
    │   │   ├── autolayout_test.go
    │   │   ├── autolayout.go
    │   │   ├── AUTOLAYOUT.md
    │   │   ├── automation_test.go
    │   │   ├── backend_info_test.go
    │   │   ├── backend_info.go
    │   │   ├── backspace_test.go
    │   │   ├── backspace.go
    │   │   ├── bar_test.go
    │   │   ├── bar.go
    │   │   ├── baseframe_test.go
    │   │   ├── baseframe.go
    │   │   ├── basewindow_test.go
    │   │   ├── basewindow.go
    │   │   ├── bidi_mirror_table.go
    │   │   ├── bidi_test.go
    │   │   ├── bidi.go
    │   │   ├── bindings
    │   │   │   ├── c
    │   │   │   │   ├── cabi
    │   │   │   │   │   └── main.go
    │   │   │   │   ├── CMakeLists.txt
    │   │   │   │   ├── examples
    │   │   │   │   │   └── hello.c
    │   │   │   │   ├── include
    │   │   │   │   │   ├── vtui_constants.h
    │   │   │   │   │   └── vtui.h
    │   │   │   │   ├── README.md
    │   │   │   │   └── src
    │   │   │   │       └── vtui.c
    │   │   │   ├── CMakeLists.txt
    │   │   │   ├── cpp
    │   │   │   │   ├── CMakeLists.txt
    │   │   │   │   ├── examples
    │   │   │   │   │   └── hello.cpp
    │   │   │   │   ├── include
    │   │   │   │   │   └── vtui.hpp
    │   │   │   │   └── README.md
    │   │   │   ├── lua
    │   │   │   │   ├── examples
    │   │   │   │   │   └── hello.lua
    │   │   │   │   ├── README.md
    │   │   │   │   ├── rockspec
    │   │   │   │   │   └── vtui-scm-1.rockspec
    │   │   │   │   ├── src
    │   │   │   │   │   └── vtui_lua.c
    │   │   │   │   ├── tests
    │   │   │   │   │   └── test_vtui.lua
    │   │   │   │   └── vtui.lua
    │   │   │   ├── node
    │   │   │   │   ├── examples
    │   │   │   │   │   ├── hello.js
    │   │   │   │   │   └── hello.ts
    │   │   │   │   ├── index.js
    │   │   │   │   ├── package.json
    │   │   │   │   ├── README.md
    │   │   │   │   ├── session.js
    │   │   │   │   ├── test
    │   │   │   │   │   └── test.js
    │   │   │   │   ├── ui.js
    │   │   │   │   └── vtui.d.ts
    │   │   │   ├── php
    │   │   │   │   ├── composer.json
    │   │   │   │   ├── examples
    │   │   │   │   │   └── hello.php
    │   │   │   │   ├── README.md
    │   │   │   │   ├── src
    │   │   │   │   │   └── Vtui.php
    │   │   │   │   └── tests
    │   │   │   │       └── test_vtui.php
    │   │   │   ├── python
    │   │   │   │   ├── examples
    │   │   │   │   │   ├── async_demo.py
    │   │   │   │   │   └── hello.py
    │   │   │   │   ├── README.md
    │   │   │   │   ├── tests
    │   │   │   │   │   └── test_vtui.py
    │   │   │   │   └── vtui
    │   │   │   │       ├── __init__.py
    │   │   │   │       ├── _props.py
    │   │   │   │       ├── async_session.py
    │   │   │   │       ├── session.py
    │   │   │   │       └── ui.py
    │   │   │   └── README.md
    │   │   ├── bindings_integration_test.go
    │   │   ├── bindings.md
    │   │   ├── box_runes.go
    │   │   ├── button_test.go
    │   │   ├── button.go
    │   │   ├── cellspan_test.go
    │   │   ├── checkbox_test.go
    │   │   ├── checkbox.go
    │   │   ├── checkgroup.go
    │   │   ├── clipboard_goclip_test.go
    │   │   ├── clipboard_gui_test.go
    │   │   ├── clipboard_test.go
    │   │   ├── clipboard_unix.go
    │   │   ├── clipboard_windows.go
    │   │   ├── clipboard.go
    │   │   ├── clusters_test.go
    │   │   ├── cmd
    │   │   │   ├── fontprobe
    │   │   │   │   └── main.go
    │   │   │   ├── test-app
    │   │   │   │   ├── main_test.go
    │   │   │   │   └── main.go
    │   │   │   ├── vtui-cast
    │   │   │   │   ├── main_test.go
    │   │   │   │   └── main.go
    │   │   │   ├── vtui-dialog
    │   │   │   │   ├── main_test.go
    │   │   │   │   └── main.go
    │   │   │   ├── vtui-gen
    │   │   │   │   ├── main_test.go
    │   │   │   │   └── main.go
    │   │   │   ├── vtui-host
    │   │   │   │   └── main.go
    │   │   │   ├── vtui-lint
    │   │   │   │   ├── main_test.go
    │   │   │   │   └── main.go
    │   │   │   ├── vtui-replay
    │   │   │   │   ├── main_test.go
    │   │   │   │   └── main.go
    │   │   │   ├── vtui-wasm
    │   │   │   │   └── main.go
    │   │   │   └── vuic
    │   │   │       ├── main_test.go
    │   │   │       └── main.go
    │   │   ├── colors_test.go
    │   │   ├── colors.go
    │   │   ├── combobox_color_test.go
    │   │   ├── combobox_layout_test.go
    │   │   ├── combobox_test.go
    │   │   ├── combobox.go
    │   │   ├── commands.go
    │   │   ├── common_dialogs_test.go
    │   │   ├── common_dialogs.go
    │   │   ├── conhost_altscreen_windows_test.go
    │   │   ├── conhost_altscreen_windows.go
    │   │   ├── console_freebsd.go
    │   │   ├── console_other.go
    │   │   ├── crash_report_pid_unix.go
    │   │   ├── crash_report_pid_windows.go
    │   │   ├── crash_report_stub.go
    │   │   ├── crash_report_test.go
    │   │   ├── crash_report.go
    │   │   ├── cursor_style.go
    │   │   ├── debug_test.go
    │   │   ├── debug.go
    │   │   ├── desktop_test.go
    │   │   ├── desktop.go
    │   │   ├── dialog_test.go
    │   │   ├── docs
    │   │   │   ├── shell_scripting.md
    │   │   │   └── widgets.md
    │   │   ├── document_lifecycle_test.go
    │   │   ├── dragdrop_test.go
    │   │   ├── dragdrop.go
    │   │   ├── DRAGDROP.md
    │   │   ├── dynamictext_test.go
    │   │   ├── dynamictext.go
    │   │   ├── ebiten_dragdrop.go
    │   │   ├── ebiten_host.go
    │   │   ├── ebiten_keys.go
    │   │   ├── ebiten_renderer_test.go
    │   │   ├── ebiten_renderer.go
    │   │   ├── ebiten_stub.go
    │   │   ├── edit_cluster_boundary_test.go
    │   │   ├── edit_multiline_test.go
    │   │   ├── edit_multiline.go
    │   │   ├── edit_semantic_input_test.go
    │   │   ├── edit_test.go
    │   │   ├── edit_words.go
    │   │   ├── edit.go
    │   │   ├── events.go
    │   │   ├── eventsink_test.go
    │   │   ├── examples
    │   │   │   └── shell
    │   │   │       └── demo.sh
    │   │   ├── factory_test.go
    │   │   ├── factory.go
    │   │   ├── far2l_extensions_test.go
    │   │   ├── far2l_extensions.go
    │   │   ├── filelist_update.sh
    │   │   ├── filelist.md
    │   │   ├── frame.go
    │   │   ├── framemanager_caret_test.go
    │   │   ├── framemanager_hidebars_test.go
    │   │   ├── framemanager_paste_test.go
    │   │   ├── framemanager_paste.go
    │   │   ├── framemanager_test.go
    │   │   ├── framemanager.go
    │   │   ├── fuzzy_test.go
    │   │   ├── fuzzy.go
    │   │   ├── go.mod
    │   │   ├── go.sum
    │   │   ├── gogpu_customchar_test.go
    │   │   ├── gogpu_dnd_test.go
    │   │   ├── gogpu_dnd.go
    │   │   ├── gogpu_ffi_stub.go
    │   │   ├── gogpu_ffi.go
    │   │   ├── gogpu_glyph_table.go
    │   │   ├── gogpu_glyphgen_test.go
    │   │   ├── gogpu_host_test.go
    │   │   ├── gogpu_host.go
    │   │   ├── gogpu_keys_test.go
    │   │   ├── gogpu_profile.go
    │   │   ├── gogpu_renderer_test.go
    │   │   ├── gogpu_renderer.go
    │   │   ├── gogpu_scroll_other.go
    │   │   ├── gogpu_scroll_windows.go
    │   │   ├── gogpu_stub.go
    │   │   ├── graphics_external_test.go
    │   │   ├── graphics_far2l_test.go
    │   │   ├── graphics_far2l.go
    │   │   ├── graphics_frame_test.go
    │   │   ├── graphics_image.go
    │   │   ├── graphics_kitty_test.go
    │   │   ├── graphics_kitty.go
    │   │   ├── graphics_native_test.go
    │   │   ├── graphics_native.go
    │   │   ├── graphics_probe_test.go
    │   │   ├── graphics_probe_unix.go
    │   │   ├── graphics_probe_windows.go
    │   │   ├── graphics_probe.go
    │   │   ├── graphics_scale.go
    │   │   ├── graphics_sixel_cursor_test.go
    │   │   ├── graphics_sixel_layered_test.go
    │   │   ├── graphics_sixel_layered.go
    │   │   ├── graphics_sixel_quality_test.go
    │   │   ├── graphics_sixel_tabrow_test.go
    │   │   ├── graphics_sixel_test.go
    │   │   ├── graphics_sixel_truecolor_test.go
    │   │   ├── graphics_sixel_truecolor.go
    │   │   ├── graphics_sixel.go
    │   │   ├── graphics_test.go
    │   │   ├── graphics.go
    │   │   ├── GRAPHICS.md
    │   │   ├── grid_nav.go
    │   │   ├── group_test.go
    │   │   ├── group.go
    │   │   ├── groupbox.go
    │   │   ├── grow_test.go
    │   │   ├── gui_api_fallback.go
    │   │   ├── gui_api.go
    │   │   ├── gui_boxdraw_test.go
    │   │   ├── gui_boxdraw.go
    │   │   ├── gui_font_native_stub.go
    │   │   ├── gui_font_native_windows_test.go
    │   │   ├── gui_font_native_windows.go
    │   │   ├── gui_font_scripts_test.go
    │   │   ├── gui_font_scripts.go
    │   │   ├── gui_font_test.go
    │   │   ├── gui_font.go
    │   │   ├── help_engine_test.go
    │   │   ├── help_engine.go
    │   │   ├── help_resize_test.go
    │   │   ├── help_view_mouse_test.go
    │   │   ├── help_view_test.go
    │   │   ├── help_view.go
    │   │   ├── highlight_test.go
    │   │   ├── highlight.go
    │   │   ├── history_test.go
    │   │   ├── internal
    │   │   │   ├── hideconsole
    │   │   │   │   ├── go.mod
    │   │   │   │   └── hideconsole.go
    │   │   │   └── uba
    │   │   │       ├── bracket.go
    │   │   │       ├── core.go
    │   │   │       ├── LICENSE.x-text
    │   │   │       ├── uba_test.go
    │   │   │       └── uba.go
    │   │   ├── ISSUE_205_SOLUTION_REVIEW.md
    │   │   ├── ISSUE_283_SOLUTION_REVIEW.md
    │   │   ├── keybar_test.go
    │   │   ├── keybar.go
    │   │   ├── keys_common_test.go
    │   │   ├── keys_common.go
    │   │   ├── keys_special.go
    │   │   ├── label_test.go
    │   │   ├── label.go
    │   │   ├── layout_test.go
    │   │   ├── layout_validator_test.go
    │   │   ├── layout_validator.go
    │   │   ├── layout.go
    │   │   ├── LAYOUT.md
    │   │   ├── LICENSE
    │   │   ├── listbox_test.go
    │   │   ├── listbox.go
    │   │   ├── localization_test.go
    │   │   ├── localization.go
    │   │   ├── lookup_test.go
    │   │   ├── menubar_test.go
    │   │   ├── menubar.go
    │   │   ├── mouse_gesture_test.go
    │   │   ├── mouse_gesture.go
    │   │   ├── multilineedit_semantic_test.go
    │   │   ├── multilineedit_semantic.go
    │   │   ├── multilineedit_test.go
    │   │   ├── multilineedit.go
    │   │   ├── OPTIMIZATIONS.md
    │   │   ├── painter.go
    │   │   ├── palette_batch_test.go
    │   │   ├── palette_test.go
    │   │   ├── palette.go
    │   │   ├── panic_bridge_test.go
    │   │   ├── panic_bridge.go
    │   │   ├── PLATFORMS.md
    │   │   ├── progressbar_test.go
    │   │   ├── progressbar.go
    │   │   ├── properties_gen.go
    │   │   ├── properties_test.go
    │   │   ├── properties.go
    │   │   ├── protocol_test.go
    │   │   ├── protocol.go
    │   │   ├── radiobutton.go
    │   │   ├── radiogroup_test.go
    │   │   ├── radiogroup.go
    │   │   ├── README.md
    │   │   ├── REVIEW.md
    │   │   ├── rowprovider_test.go
    │   │   ├── runewidth_test.go
    │   │   ├── runewidth.go
    │   │   ├── screenbuf_cursor_test.go
    │   │   ├── screenbuf_test.go
    │   │   ├── screenbuf.go
    │   │   ├── screenobject_test.go
    │   │   ├── screenobject.go
    │   │   ├── screenshot.png
    │   │   ├── scrollbar_test.go
    │   │   ├── scrollbar_widget_test.go
    │   │   ├── scrollbar.go
    │   │   ├── scrollview_test.go
    │   │   ├── scrollview.go
    │   │   ├── semantic_help_test.go
    │   │   ├── semantic_help.go
    │   │   ├── semantic_table_test.go
    │   │   ├── semantic_table.go
    │   │   ├── semantic_test.go
    │   │   ├── semantic.go
    │   │   ├── separator.go
    │   │   ├── session_test.go
    │   │   ├── shutdown_test.go
    │   │   ├── sizespec.go
    │   │   ├── spacer.go
    │   │   ├── standard_dialogs_layout_test.go
    │   │   ├── statusline_test.go
    │   │   ├── statusline.go
    │   │   ├── step_test.go
    │   │   ├── strings.go
    │   │   ├── symbols.go
    │   │   ├── sys_darwin.go
    │   │   ├── sys_unix.go
    │   │   ├── sys_windows.go
    │   │   ├── table_dialog_test.go
    │   │   ├── table_dialog.go
    │   │   ├── table_test.go
    │   │   ├── table.go
    │   │   ├── tasks_test.go
    │   │   ├── tasks.go
    │   │   ├── terminal_env_console_altscreen_test.go
    │   │   ├── terminal_env_test.go
    │   │   ├── terminal_env_unix.go
    │   │   ├── terminal_env_windows.go
    │   │   ├── terminal_env.go
    │   │   ├── test_main_test.go
    │   │   ├── testdata
    │   │   │   ├── hello.golden.json
    │   │   │   └── hello.vui
    │   │   ├── testing.go
    │   │   ├── text_decorations_test.go
    │   │   ├── text_decorations.go
    │   │   ├── text_test.go
    │   │   ├── text_utils_test.go
    │   │   ├── text_utils.go
    │   │   ├── text.go
    │   │   ├── textseg_test.go
    │   │   ├── textseg.go
    │   │   ├── TEXTSEG.md
    │   │   ├── treeview_test.go
    │   │   ├── treeview.go
    │   │   ├── types.go
    │   │   ├── UI_TESTING.md
    │   │   ├── UNICODE_PLAN.md
    │   │   ├── UX_GUIDELINES.md
    │   │   ├── validator_test.go
    │   │   ├── validator.go
    │   │   ├── vmenu_cancel_test.go
    │   │   ├── vmenu_submenu_test.go
    │   │   ├── vmenu_test.go
    │   │   ├── vmenu_window_test.go
    │   │   ├── vmenu_window.go
    │   │   ├── vmenu.go
    │   │   ├── vocabulary.json
    │   │   ├── vocabulary.schema.json
    │   │   ├── vreactive
    │   │   │   ├── animator_test.go
    │   │   │   ├── animator.go
    │   │   │   ├── bindings_test.go
    │   │   │   ├── bindings.go
    │   │   │   ├── computed_test.go
    │   │   │   ├── computed.go
    │   │   │   ├── easing_test.go
    │   │   │   ├── easing.go
    │   │   │   ├── property_test.go
    │   │   │   ├── property.go
    │   │   │   ├── README.md
    │   │   │   └── statemachine.go
    │   │   ├── vtext_test.go
    │   │   ├── vtext.go
    │   │   ├── vtui_test.go
    │   │   ├── vui_layout.go
    │   │   ├── vui_loader.go
    │   │   ├── vui_test.go
    │   │   ├── vui.schema.json
    │   │   ├── wayland_host_test.go
    │   │   ├── wayland_host.go
    │   │   ├── wayland_present_test.go
    │   │   ├── wayland_renderer.go
    │   │   ├── wayland_stub.go
    │   │   ├── wheel_scroll_test.go
    │   │   ├── wheel_scroll.go
    │   │   ├── WIDTH_NEGOTIATION.md
    │   │   ├── win32_console_common.go
    │   │   ├── win32_console_stub.go
    │   │   ├── win32_console_test.go
    │   │   ├── win32_console_windows.go
    │   │   ├── win32_dnd_stub.go
    │   │   ├── win32_dnd_test.go
    │   │   ├── win32_dnd_windows.go
    │   │   ├── win32_droptarget_other_windows.go
    │   │   ├── win32_droptarget_windows.go
    │   │   ├── win32_gui_common.go
    │   │   ├── win32_gui_renderer.go
    │   │   ├── win32_gui_resize_windows_test.go
    │   │   ├── win32_gui_stub.go
    │   │   ├── win32_gui_test.go
    │   │   ├── win32_gui_windows.go
    │   │   ├── window.go
    │   │   ├── word_nav_test.go
    │   │   ├── word_nav.go
    │   │   ├── WORDNAV.md
    │   │   ├── workspace_altnumber_test.go
    │   │   ├── x11_host_test.go
    │   │   ├── x11_host.go
    │   │   ├── x11_keys_shared.go
    │   │   ├── x11_keys_test.go
    │   │   ├── x11_render_common.go
    │   │   ├── x11_renderer.go
    │   │   ├── x11_shm_fallback.go
    │   │   ├── x11_shm_unix.go
    │   │   ├── x11_stub.go
    │   │   ├── x11_xdnd_test.go
    │   │   ├── x11_xdnd.go
    │   │   ├── xlat_tables.go
    │   │   ├── xlat_test.go
    │   │   └── xlat.go
    │   └── ZoinGallery
    ├── time.txt
    ├── tools
    │   ├── analyze_navigation_benchmark.py
    │   ├── conpty_probe_child.py
    │   ├── conpty_probe.py
    │   ├── conptyreconcile
    │   │   ├── capture.go
    │   │   ├── clear_probe_windows.go
    │   │   ├── command_compare_windows.go
    │   │   ├── command_probe_windows.go
    │   │   ├── command_suite_windows.go
    │   │   ├── control_stream_test.go
    │   │   ├── control_stream.go
    │   │   ├── edge_probe_windows.go
    │   │   ├── emitter.go
    │   │   ├── empty_probe_windows.go
    │   │   ├── gate_nonwindows.go
    │   │   ├── gate_windows.go
    │   │   ├── gate.go
    │   │   ├── go.mod
    │   │   ├── go.sum
    │   │   ├── hash.go
    │   │   ├── host_constants.go
    │   │   ├── host_history_test.go
    │   │   ├── host_history.go
    │   │   ├── host_stream_chunking_test.go
    │   │   ├── host_stream_chunking.go
    │   │   ├── host_stream_test.go
    │   │   ├── host_stream.go
    │   │   ├── lifecycle_probe_windows.go
    │   │   ├── line_diff.go
    │   │   ├── logical_lines_test.go
    │   │   ├── logical_lines.go
    │   │   ├── main.go
    │   │   ├── native_probe_nonwindows.go
    │   │   ├── native_probe_windows.go
    │   │   ├── native_probe.go
    │   │   ├── payload_assertions_test.go
    │   │   ├── payload_assertions.go
    │   │   ├── pinned_host_nonwindows.go
    │   │   ├── pinned_host_windows.go
    │   │   ├── pinned_host.go
    │   │   ├── probe.go
    │   │   ├── quirk_probe_windows.go
    │   │   ├── reflow_probe_windows.go
    │   │   ├── reflow_probe.go
    │   │   ├── scroll_probe_windows.go
    │   │   ├── scrollback_test.go
    │   │   ├── scrollback.go
    │   │   ├── seeds.go
    │   │   ├── semantic_probe_windows.go
    │   │   └── semantic_probe.go
    │   ├── f4imgprobe
    │   │   ├── main.go
    │   │   ├── README.txt
    │   │   ├── windowlongptr_32.go
    │   │   └── windowlongptr_64.go
    │   ├── find_hardcoded.go
    │   ├── fishplus_probe.sh
    │   ├── fishplus_testlab
    │   │   ├── fishclient.py
    │   │   ├── test_patch.py
    │   │   └── TESTLAB.md
    │   ├── hardcode
    │   │   ├── hardcode_test.go
    │   │   └── hardcode.go
    │   ├── hardcoded_baseline.txt
    │   ├── icons
    │   │   ├── go.mod
    │   │   ├── go.sum
    │   │   ├── main_test.go
    │   │   ├── main.go
    │   │   └── third_party
    │   │       └── oksvg
    │   │           ├── .gitignore
    │   │           ├── definitions.go
    │   │           ├── draw.go
    │   │           ├── go.mod
    │   │           ├── icon_cursor.go
    │   │           ├── LICENSE
    │   │           ├── path_cursor.go
    │   │           ├── path_style.go
    │   │           ├── public.go
    │   │           ├── README.md
    │   │           ├── svg_icon.go
    │   │           ├── svg_path.go
    │   │           └── utils.go
    │   ├── langfmt
    │   │   ├── main_test.go
    │   │   └── main.go
    │   ├── sanitize_native_probe_report.ps1
    │   ├── test_analyze_navigation_benchmark.py
    │   ├── test_runner.sh
    │   ├── ttytest
    │   │   ├── analyze_log.py
    │   │   ├── README.md
    │   │   ├── scenarios.py
    │   │   └── ttytest.py
    │   ├── verify_native_probe_artifacts.ps1
    │   ├── vtui-screen
    │   │   ├── main_test.go
    │   │   ├── main.go
    │   │   └── README.md
    │   ├── wine_color_probe
    │   │   ├── main_other.go
    │   │   └── main.go
    │   └── wine_syscall_probe
    │       ├── go.mod
    │       ├── main.go
    │       └── probe_amd64.s
    └── vfs
        ├── bulk_copy_test.go
        ├── codepages_forced_test.go
        ├── codepages_iconv_unix.go
        ├── codepages_issue875_test.go
        ├── codepages_test.go
        ├── codepages_unix_test.go
        ├── codepages_unix.go
        ├── codepages_utf8_system_test.go
        ├── codepages_windows_test.go
        ├── codepages_windows.go
        ├── codepages.go
        ├── contributions.go
        ├── destination_overwrite_test.go
        ├── device_path_test.go
        ├── device_path.go
        ├── device_size_test.go
        ├── disks_unix_test.go
        ├── disks_unix.go
        ├── disks_vfs_coverage_test.go
        ├── disks_vfs_test.go
        ├── disks_vfs.go
        ├── disks_windows_test.go
        ├── disks_windows.go
        ├── hidden_rule_test.go
        ├── hidden_rule.go
        ├── hidden_unix.go
        ├── hidden_windows_test.go
        ├── hidden_windows.go
        ├── hostfs
        │   ├── errno_windows.go
        │   ├── hostfs_posix_test.go
        │   ├── hostfs_posix.go
        │   ├── hostfs_windows_coverage_test.go
        │   ├── hostfs_windows_test.go
        │   ├── hostfs_windows.go
        │   └── hostfs_winescape.go
        ├── hostmode
        │   ├── hostmode_test.go
        │   └── hostmode.go
        ├── hostpath
        │   ├── hostpath_posix.go
        │   └── hostpath_windows.go
        ├── isabs_test.go
        ├── lock_manager_test.go
        ├── metadata_test.go
        ├── metadata.go
        ├── name_order.go
        ├── null_vfs_test.go
        ├── null_vfs.go
        ├── os_vfs_contract_coverage_test.go
        ├── os_vfs_dot_test.go
        ├── os_vfs_junction_stub.go
        ├── os_vfs_junction_test.go
        ├── os_vfs_listing_other_test.go
        ├── os_vfs_listing_test.go
        ├── os_vfs_listing_windows_test.go
        ├── os_vfs_listing.go
        ├── os_vfs_noreplace_test.go
        ├── os_vfs_physical_other.go
        ├── os_vfs_physical_test.go
        ├── os_vfs_physical_unix.go
        ├── os_vfs_physical_windows.go
        ├── os_vfs_platform_unix.go
        ├── os_vfs_platform_windows.go
        ├── os_vfs_posix_atim.go
        ├── os_vfs_posix_atimespec.go
        ├── os_vfs_preview_windows.go
        ├── os_vfs_readdir_other.go
        ├── os_vfs_readdir_windows_test.go
        ├── os_vfs_readdir_windows.go
        ├── os_vfs_reparse_other.go
        ├── os_vfs_reparse_windows_test.go
        ├── os_vfs_reparse_windows.go
        ├── os_vfs_search_test.go
        ├── os_vfs_search.go
        ├── os_vfs_symlink_test.go
        ├── os_vfs_test.go
        ├── os_vfs_unix_test.go
        ├── os_vfs_windows_test.go
        ├── os_vfs_windows.go
        ├── os_vfs.go
        ├── patch_inplace_test.go
        ├── personality.go
        ├── privileges_windows.go
        ├── prompt_hold_test.go
        ├── prompt_hold.go
        ├── quick_view_test.go
        ├── quick_view.go
        ├── read_access_test.go
        ├── registry_vfs_windows_test.go
        ├── registry_vfs_windows.go
        ├── rename_noreplace_darwin.go
        ├── rename_noreplace_linux.go
        ├── rename_noreplace_test.go
        ├── rename_noreplace_unix.go
        ├── rename_noreplace_windows.go
        ├── rename_noreplace.go
        ├── reparse_test.go
        ├── reparse.go
        ├── scanner_test.go
        ├── scanner.go
        ├── session_identity_test.go
        ├── settings.go
        ├── share_test.go
        ├── share.go
        ├── sudo_askpass_test.go
        ├── sudo_askpass_unix.go
        ├── sudo_askpass_windows.go
        ├── sudo_child_env_test.go
        ├── sudo_child_env.go
        ├── sudo_client_platform_unix.go
        ├── sudo_client_platform_windows.go
        ├── sudo_client_windows_test.go
        ├── sudo_client.go
        ├── sudo_dispatcher_unix_test.go
        ├── sudo_dispatcher_unix.go
        ├── sudo_dispatcher_windows.go
        ├── sudo_ipc_unix.go
        ├── sudo_ipc_windows.go
        ├── sudo_msg.go
        ├── sudo_test.go
        ├── trash_darwin_test.go
        ├── trash_darwin.go
        ├── trash_freedesktop_test.go
        ├── trash_freedesktop.go
        ├── trash_test.go
        ├── trash_windows.go
        ├── trash.go
        ├── uri_provider_test.go
        ├── uri_provider.go
        ├── utils_test.go
        ├── utils.go
        └── vfs.go

    286 directories, 3437 files
