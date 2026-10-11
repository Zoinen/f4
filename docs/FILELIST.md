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
    │       │   ├── SKILL.md
    │       │   └── references
    │       │       ├── config-template.yaml
    │       │       └── update-config.mjs
    │       ├── aif-architecture
    │       │   ├── SKILL.md
    │       │   └── references
    │       │       └── architecture.md
    │       ├── aif-archive
    │       │   └── SKILL.md
    │       ├── aif-best-practices
    │       │   └── SKILL.md
    │       ├── aif-build-automation
    │       │   ├── SKILL.md
    │       │   ├── references
    │       │   │   ├── BEST-PRACTICES.md
    │       │   │   ├── DOC-INTEGRATION.md
    │       │   │   └── SUMMARY-FORMAT.md
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
    │       │   ├── SKILL.md
    │       │   ├── references
    │       │   │   ├── AUDIT-REPORT.md
    │       │   │   ├── BEST-PRACTICES.md
    │       │   │   ├── GITLAB-PATTERNS.md
    │       │   │   ├── SERVICE-CONTAINERS.md
    │       │   │   └── TOOL-COMMANDS.md
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
    │       │   ├── SKILL.md
    │       │   ├── references
    │       │   │   └── REVIEW-CHECKLISTS.md
    │       │   └── templates
    │       │       └── html-template.html
    │       ├── aif-evolve
    │       │   └── SKILL.md
    │       ├── aif-explore
    │       │   ├── SKILL.md
    │       │   └── references
    │       │       └── ULTRA-RESEARCH-FORMAT.md
    │       ├── aif-fix
    │       │   └── SKILL.md
    │       ├── aif-grounded
    │       │   └── SKILL.md
    │       ├── aif-implement
    │       │   ├── SKILL.md
    │       │   └── references
    │       │       ├── IMPLEMENTATION-GUIDE.md
    │       │       └── LOGGING-GUIDE.md
    │       ├── aif-improve
    │       │   ├── SKILL.md
    │       │   └── references
    │       │       ├── CHECK-MODE.md
    │       │       ├── EXAMPLES.md
    │       │       ├── LIST-MODE.md
    │       │       └── VALIDATOR.md
    │       ├── aif-loop
    │       │   ├── SKILL.md
    │       │   └── references
    │       │       ├── ACTIVE-TIME-BUDGET.md
    │       │       ├── CONTEXT-MANAGEMENT.md
    │       │       ├── CRITERIA-TEMPLATES.md
    │       │       ├── PHASE-CONTRACTS.md
    │       │       ├── RULE-SCHEMA.md
    │       │       └── TERMINAL-REPORT.md
    │       ├── aif-plan
    │       │   ├── SKILL.md
    │       │   └── references
    │       │       ├── EXAMPLES.md
    │       │       ├── TASK-FORMAT.md
    │       │       └── ULTRA-FORMAT.md
    │       ├── aif-qa
    │       │   ├── SKILL.md
    │       │   ├── references
    │       │   │   ├── CHANGE-SUMMARY.md
    │       │   │   ├── TEST-CASES.md
    │       │   │   └── TEST-PLAN.md
    │       │   └── templates
    │       │       ├── CHANGE-SUMMARY.md
    │       │       ├── TEST-CASES.md
    │       │       └── TEST-PLAN.md
    │       ├── aif-qa-check
    │       │   ├── SKILL.md
    │       │   └── templates
    │       │       └── QA-CHECK.md
    │       ├── aif-review
    │       │   ├── SKILL.md
    │       │   └── references
    │       │       ├── CHECK-MODE.md
    │       │       ├── SEVERITY.md
    │       │       └── VALIDATOR.md
    │       ├── aif-roadmap
    │       │   └── SKILL.md
    │       ├── aif-rules
    │       │   └── SKILL.md
    │       ├── aif-rules-check
    │       │   ├── SKILL.md
    │       │   └── references
    │       │       └── RULES-CHECK-CONTRACT.md
    │       ├── aif-security-checklist
    │       │   ├── SKILL.md
    │       │   ├── references
    │       │   │   ├── AUTH-PATTERNS.md
    │       │   │   ├── PROMPT-INJECTION.md
    │       │   │   └── RACE-CONDITIONS.md
    │       │   └── scripts
    │       │       └── audit.sh
    │       ├── aif-skill-generator
    │       │   ├── SKILL.md
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
    │       │   └── templates
    │       │       ├── basic.md
    │       │       ├── dynamic-context.md
    │       │       ├── research.md
    │       │       ├── task.md
    │       │       └── visual.md
    │       ├── aif-verify
    │       │   ├── SKILL.md
    │       │   └── references
    │       │       ├── CONTEXT-GATES-AND-OWNERSHIP.md
    │       │       └── GATE-RESULT-CONTRACT.md
    │       ├── golang-code-style
    │       │   ├── SKILL.md
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   └── references
    │       │       └── details.md
    │       ├── golang-concurrency
    │       │   ├── SKILL.md
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   └── references
    │       │       ├── channels-and-select.md
    │       │       ├── pipelines.md
    │       │       └── sync-primitives.md
    │       ├── golang-context
    │       │   ├── SKILL.md
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   └── references
    │       │       ├── cancellation.md
    │       │       ├── http-services.md
    │       │       └── values-tracing.md
    │       ├── golang-database
    │       │   ├── SKILL.md
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   └── references
    │       │       ├── performance.md
    │       │       ├── scanning.md
    │       │       ├── testing.md
    │       │       └── transactions.md
    │       ├── golang-design-patterns
    │       │   ├── SKILL.md
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   └── references
    │       │       ├── architecture.md
    │       │       ├── clean-architecture.md
    │       │       ├── data-handling.md
    │       │       ├── ddd.md
    │       │       ├── hexagonal-architecture.md
    │       │       └── resource-management.md
    │       ├── golang-documentation
    │       │   ├── SKILL.md
    │       │   ├── assets
    │       │   │   └── templates
    │       │   │       ├── CHANGELOG.md
    │       │   │       ├── CONTRIBUTING.md
    │       │   │       ├── README.md
    │       │   │       └── llms.txt
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   └── references
    │       │       ├── application.md
    │       │       ├── code-comments.md
    │       │       ├── library.md
    │       │       └── project-docs.md
    │       ├── golang-error-handling
    │       │   ├── SKILL.md
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   └── references
    │       │       ├── error-creation.md
    │       │       ├── error-handling.md
    │       │       └── error-wrapping.md
    │       ├── golang-naming
    │       │   ├── SKILL.md
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   └── references
    │       │       ├── functions-methods.md
    │       │       ├── identifiers.md
    │       │       ├── packages-files.md
    │       │       ├── testing.md
    │       │       └── types-errors.md
    │       ├── golang-performance
    │       │   ├── SKILL.md
    │       │   ├── assets
    │       │   │   └── prometheus-alerts.yml
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   └── references
    │       │       ├── caching.md
    │       │       ├── cpu.md
    │       │       ├── io-networking.md
    │       │       ├── memory.md
    │       │       ├── observability.md
    │       │       └── runtime.md
    │       ├── golang-security
    │       │   ├── SKILL.md
    │       │   ├── evals
    │       │   │   └── evals.json
    │       │   └── references
    │       │       ├── architecture.md
    │       │       ├── checklist.md
    │       │       ├── cookies.md
    │       │       ├── cryptography.md
    │       │       ├── filesystem.md
    │       │       ├── injection.md
    │       │       ├── logging.md
    │       │       ├── memory-safety.md
    │       │       ├── network.md
    │       │       ├── secrets.md
    │       │       ├── third-party.md
    │       │       └── threat-modeling.md
    │       └── golang-testing
    │           ├── SKILL.md
    │           ├── evals
    │           │   └── evals.json
    │           └── references
    │               ├── benchmarks.md
    │               ├── coverage.md
    │               ├── examples.md
    │               ├── helpers.md
    │               ├── http-testing.md
    │               ├── integration-testing.md
    │               └── mocking.md
    ├── .ai-factory
    │   ├── ARCHITECTURE.md
    │   ├── DESCRIPTION.md
    │   ├── config.yaml
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
    │   │   ├── 2026-09-19-12.58.md
    │   │   ├── 2026-09-19-20.22.md
    │   │   ├── 2026-09-19-drive-menu-text-layout.md
    │   │   ├── 2026-09-20-00.00.md
    │   │   ├── 2026-09-20-13.17.md
    │   │   ├── 2026-09-21-00.45.md
    │   │   ├── 2026-09-21-00.57.md
    │   │   ├── 2026-09-21-01.16.md
    │   │   ├── 2026-09-21-01.17.md
    │   │   ├── 2026-09-21-01.36.md
    │   │   ├── 2026-09-21-01.38.md
    │   │   ├── 2026-09-21-02.09.md
    │   │   ├── 2026-09-21-02.19.md
    │   │   ├── 2026-09-21-16.54.md
    │   │   ├── 2026-09-21-23.06.md
    │   │   ├── 2026-09-21-23.52.md
    │   │   ├── 2026-09-21-23.58.md
    │   │   ├── 2026-09-22-00.17.md
    │   │   ├── 2026-09-22-01.15.md
    │   │   ├── 2026-09-22-01.49.md
    │   │   ├── 2026-09-22-14.33.md
    │   │   ├── 2026-09-22-15.22.md
    │   │   ├── 2026-09-22-15.36.md
    │   │   ├── 2026-09-22-16.26.md
    │   │   ├── 2026-09-22-16.36.md
    │   │   ├── 2026-09-22-16.50.md
    │   │   ├── 2026-09-22-16.54.md
    │   │   ├── 2026-09-22-16.59.md
    │   │   ├── 2026-09-22-17.28.md
    │   │   ├── 2026-09-22-18.00.md
    │   │   ├── 2026-09-22-18.10.md
    │   │   ├── 2026-09-22-18.15.md
    │   │   ├── 2026-09-22-18.18.md
    │   │   ├── 2026-09-22-18.26.md
    │   │   ├── 2026-09-22-18.39.md
    │   │   ├── 2026-09-22-18.49.md
    │   │   ├── 2026-09-22-19.22.md
    │   │   ├── 2026-09-22-19.27.md
    │   │   ├── 2026-09-22-19.34.md
    │   │   ├── 2026-09-22-19.43.md
    │   │   ├── 2026-09-22-20.02.md
    │   │   ├── 2026-09-22-20.06.md
    │   │   ├── 2026-09-22-20.15.md
    │   │   ├── 2026-09-22-23.16.md
    │   │   ├── 2026-09-22-23.21.md
    │   │   ├── 2026-09-22-23.25.md
    │   │   ├── 2026-09-22-columns-wrap-spacing.md
    │   │   ├── 2026-09-22-zoom-round-trip.md
    │   │   ├── 2026-09-23-01.16.md
    │   │   ├── 2026-09-23-01.35.md
    │   │   ├── 2026-09-23-01.47.md
    │   │   ├── 2026-09-23-02.36.md
    │   │   ├── 2026-09-23-10.23.md
    │   │   ├── 2026-09-23-17.33.md
    │   │   ├── 2026-09-23-22.39.md
    │   │   ├── 2026-09-23-release-gates.md
    │   │   ├── 2026-09-24-01.04.md
    │   │   ├── 2026-09-24-01.17.md
    │   │   ├── 2026-09-25-compact-icons.md
    │   │   ├── 2026-09-26-panel-capacity.md
    │   │   ├── 2026-09-27-01.00.md
    │   │   ├── 2026-09-27-02.22.md
    │   │   ├── 2026-09-27-font-dropdown-text.md
    │   │   ├── 2026-09-27-interface-font-chrome.md
    │   │   ├── 2026-09-27-menu-bar-focus.md
    │   │   ├── 2026-09-28-00.40.md
    │   │   ├── 2026-09-28-case-alias-transfer.md
    │   │   ├── 2026-09-28-panel-column-font.md
    │   │   ├── 2026-09-30-search-first-focus.md
    │   │   ├── 2026-09-30-settings-sections.md
    │   │   ├── 2026-10-02-editor-return-catalog.md
    │   │   ├── 2026-10-02-search-first-autocomplete-workspace.md
    │   │   ├── 2026-10-02-search-first-retained-command.md
    │   │   ├── 2026-10-02-sort-direction.md
    │   │   ├── 2026-10-03-viewer-flying-selection-fill.md
    │   │   ├── 2026-10-03-viewer-transition-caption-size.md
    │   │   ├── 2026-10-03-viewer-transition-grid-caption.md
    │   │   ├── 2026-10-03-viewer-transition-panel-chrome.md
    │   │   ├── 2026-10-04-23.20.md
    │   │   ├── 2026-10-04-fishplus-binary-cancellation.md
    │   │   ├── 2026-10-06-02.58.md
    │   │   ├── 2026-10-06-linux-video-shader-interface.md
    │   │   ├── 2026-10-07-02.10.md
    │   │   ├── 2026-10-07-13.26.md
    │   │   ├── 2026-10-07-23.48.md
    │   │   ├── 2026-10-07-paused-video-presentation-cache.md
    │   │   ├── 2026-10-07-viewer-flight-hardware-sampling.md
    │   │   ├── 2026-10-08-01.22.md
    │   │   ├── 2026-10-08-03.44.md
    │   │   ├── 2026-10-08-13.17.md
    │   │   ├── 2026-10-08-13.48.md
    │   │   ├── 2026-10-08-14.24.md
    │   │   ├── 2026-10-08-14.55.md
    │   │   ├── 2026-10-08-15.24.md
    │   │   ├── 2026-10-08-17.21.md
    │   │   ├── 2026-10-08-autocomplete-multiline.md
    │   │   ├── 2026-10-08-autocomplete-wrap-indent.md
    │   │   ├── 2026-10-08-filmstrip-thumbnail-aspect.md
    │   │   ├── 2026-10-08-masonry-cursor-caption.md
    │   │   ├── 2026-10-08-native-menu-panel-shortcuts.md
    │   │   ├── 2026-10-08-panel-selection-boundary.md
    │   │   ├── 2026-10-08-panel-sort-direction-session.md
    │   │   ├── 2026-10-08-panel-sort-refresh-direction.md
    │   │   ├── 2026-10-08-quick-search-no-match.md
    │   │   ├── 2026-10-08-shift-home-end-inclusive.md
    │   │   ├── 2026-10-08-viewer-tab-osd.md
    │   │   ├── 2026-10-08-windowed-video-and-linux-audio-ci.md
    │   │   ├── 2026-10-09-dialog-command-line-layout.md
    │   │   ├── 2026-10-09-find-file-dialog-workspace.md
    │   │   ├── 2026-10-10-android-device-list-cache.md
    │   │   ├── 2026-10-10-android-immediate-panel-switch.md
    │   │   └── 2026-10-10-qt-redundant-panel-updates.md
    │   ├── plans
    │   │   ├── gui-settings-integration.md
    │   │   ├── viewer-flight-layout.md
    │   │   ├── zoin-folder-preview-first-frame.md
    │   │   ├── zoin-folder-previews.md
    │   │   ├── zoin-gallery-cache-settings.md
    │   │   └── zoin_branch-gallery-quick-view.md
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
    │       ├── mojibake-guard.yml
    │       ├── openwrt.yml
    │       ├── quick.yml
    │       └── sandbox.yml
    ├── .gitignore
    ├── .gitmodules
    ├── .golangci-strict.yml
    ├── .golangci.yml
    ├── .mcp.json
    ├── AGENTS.md
    ├── LICENSE
    ├── README.md
    ├── artifacts
    │   └── README.md
    ├── assets
    │   └── icon
    │       ├── AppIcon.icns
    │       └── AppIcon.icon
    │           ├── Assets
    │           │   ├── f4-content.svg
    │           │   └── f4-shell.svg
    │           └── icon.json
    ├── ci
    │   ├── arm64_glibc_archives.py
    │   ├── audit-portable-qt-linux.sh
    │   ├── audit-portable-qt-windows.ps1
    │   ├── audit-static-go-linux.sh
    │   ├── build-portable-qt-linux.sh
    │   ├── build-qwindowkit.ps1
    │   ├── build-qwindowkit.sh
    │   ├── configure-conan-remote.sh
    │   ├── package-embedded-qt-host.py
    │   ├── package-go-release.py
    │   ├── package-qt-release.py
    │   ├── package-release.py
    │   ├── patch-bzip2-recipe.py
    │   ├── patch-bzip2-recipe.sh
    │   ├── patch-libffi-recipe.py
    │   ├── patch-libffi-recipe.sh
    │   ├── patch-libgettext-recipe.py
    │   ├── patch-libgettext-recipe.sh
    │   ├── patch-libiconv-recipe.py
    │   ├── patch-libiconv-recipe.sh
    │   ├── patch-qt-dependencies.py
    │   ├── patch-qt-qmltools-recipe.py
    │   ├── patch-qt-recipe.sh
    │   ├── patches
    │   │   ├── libiconv-1.17-fix-error-function-declaration-without-prototype.patch
    │   │   └── qwindowkit-default-maximize-hint.patch
    │   ├── remove-elf-interpreter.py
    │   ├── test_arm64_glibc_archives.py
    │   ├── test_package_release_scripts.py
    │   ├── test_patch_qt_dependencies.py
    │   ├── test_portable_conan_upload.py
    │   ├── test_portable_qt_linux.py
    │   └── upload-conan-packages.py
    ├── cmd
    │   ├── f4
    │   │   ├── android_deps_test.go
    │   │   ├── architecture_test.go
    │   │   ├── cloudfox_deps_test.go
    │   │   ├── command_palette_coverage_test.go
    │   │   ├── frame_manager_capture_test.go
    │   │   ├── go2xp_shim.go
    │   │   ├── hardcoded_strings_test.go
    │   │   ├── ios_deps_test.go
    │   │   ├── lite_deps_test.go
    │   │   ├── main.go
    │   │   ├── rsrc_windows_amd64.syso
    │   │   ├── rsrc_windows_arm64.syso
    │   │   └── winescape_gate_test.go
    │   └── f4-gui-launcher
    │       ├── main_other.go
    │       ├── main_windows.go
    │       ├── main_windows_test.go
    │       └── testdata
    │           └── stub
    │               └── main.go
    ├── docs
    │   ├── ARCHIVE_DEPENDENCIES.md
    │   ├── BOOKMARKS.md
    │   ├── COLORS.md
    │   ├── CONPTY_GATE_REQUIREMENTS.md
    │   ├── CONPTY_GATE_STATUS.md
    │   ├── CONPTY_NATIVE_AGENT.md
    │   ├── CONPTY_NATIVE_AUDIT.md
    │   ├── CONPTY_NATIVE_PROBE.md
    │   ├── CONPTY_NATIVE_TEST.md
    │   ├── CONSOLE_MODES.md
    │   ├── CURSOR.md
    │   ├── DOCKER.md
    │   ├── DOTNET.md
    │   ├── DRAGDROP.md
    │   ├── FAR2L_DND.md
    │   ├── FFI.md
    │   ├── FILE_FIELDS.md
    │   ├── FISH+.md
    │   ├── FISH_PLUS_S2S.md
    │   ├── FUSE.md
    │   ├── HIGHLIGHT.md
    │   ├── HIGHLIGHTING.md
    │   ├── HISTORY_IMPORT.md
    │   ├── HISTORY_PERFORMANCE.md
    │   ├── I18N.md
    │   ├── ISSUES
    │   │   ├── CI_WINDOWS_DESKTOP_EOL.md
    │   │   ├── ISSUE_1400_PANEL_MODES.md
    │   │   ├── ISSUE_1716_SFTP_SAVE_OVERWRITE.md
    │   │   ├── ISSUE_1721_EXTERNAL_EDITOR_EPERM.md
    │   │   ├── ISSUE_266_MULTI_SELECTION_ATTRIBUTES.md
    │   │   ├── ISSUE_722_COPY_ACCESS_RIGHTS.md
    │   │   └── ISSUE_722_COPY_OPTIONS.md
    │   ├── KEYMAP.md
    │   ├── KUBERNETES.md
    │   ├── L10N_REPORT_GUIDE.md
    │   ├── LUA.md
    │   ├── MACKEYS.md
    │   ├── MACROS.md
    │   ├── MERMAID.md
    │   ├── MONGODB.md
    │   ├── NESTED_ARCHIVES.md
    │   ├── NETFOX.md
    │   ├── OPENWRT.md
    │   ├── PDF.md
    │   ├── PINNED_CONSOLE.md
    │   ├── PINNED_HOST_FACTS.md
    │   ├── PLAYER.md
    │   ├── PLUGINS.md
    │   ├── PLUGRING.md
    │   ├── PORTABILITY_BSD.md
    │   ├── PORTABLE_BUILD_POLICY.md
    │   ├── PROMPT.md
    │   ├── QT_DRAGDROP.md
    │   ├── REVIEW.md
    │   ├── SERVICES.md
    │   ├── SETTINGS_CENTER.md
    │   ├── SPREADSHEET.md
    │   ├── SYNC_DIRS.md
    │   ├── TERMINAL.md
    │   ├── TERMINAL_JUNK_LOG.md
    │   ├── TTYX.md
    │   ├── UPDATER.md
    │   ├── UPSTREAM.md
    │   ├── USER_MENU.md
    │   ├── UX_GUIDELINES.md
    │   ├── VFS.md
    │   ├── VIDEO.md
    │   ├── VTML.md
    │   ├── VTVIBE.md
    │   ├── WINCON.md
    │   ├── WINCON_805_HANDOVER.md
    │   ├── WINE.md
    │   └── settings-center-fields.json
    ├── embedded.go
    ├── f4.example.ini
    ├── flake.lock
    ├── flake.nix
    ├── go.mod
    ├── go.sum
    ├── highlight.ini
    ├── infra
    │   └── conan-server
    │       ├── Caddyfile
    │       ├── README.md
    │       └── conan-caddy-deploy-hook.sh
    ├── internal
    │   ├── action
    │   │   ├── order.go
    │   │   ├── registry.go
    │   │   └── registry_order_test.go
    │   ├── app
    │   │   ├── action_copy_window_title_test.go
    │   │   ├── action_copyname_parent_test.go
    │   │   ├── action_delete_cursor_test.go
    │   │   ├── action_enabled_test.go
    │   │   ├── action_labelkeys_test.go
    │   │   ├── action_marked_clipboard_test.go
    │   │   ├── action_menu.go
    │   │   ├── action_menu_refresh_test.go
    │   │   ├── action_menu_test.go
    │   │   ├── action_menu_visibility_test.go
    │   │   ├── action_registry_test.go
    │   │   ├── action_restore_selection_test.go
    │   │   ├── action_shortcut_conflict_test.go
    │   │   ├── actions.go
    │   │   ├── actions_copy_conflict_test.go
    │   │   ├── actions_coverage_batch15_test.go
    │   │   ├── actions_coverage_batch17_test.go
    │   │   ├── actions_coverage_batch19_test.go
    │   │   ├── actions_coverage_batch24_test.go
    │   │   ├── actions_coverage_batch26_test.go
    │   │   ├── actions_coverage_helpers_test.go
    │   │   ├── actions_framework.go
    │   │   ├── actions_framework_test.go
    │   │   ├── actions_low_coverage_extra_test.go
    │   │   ├── actions_table.go
    │   │   ├── actions_table_order_test.go
    │   │   ├── actions_test.go
    │   │   ├── actions_view_by_type_test.go
    │   │   ├── ai_chat_panel.go
    │   │   ├── ai_chat_panel_coverage_test.go
    │   │   ├── ai_chat_panel_test.go
    │   │   ├── android_plugin_menu.go
    │   │   ├── android_plugin_menu_test.go
    │   │   ├── api.go
    │   │   ├── api_test.go
    │   │   ├── apply_command_test.go
    │   │   ├── arkanoid.go
    │   │   ├── arkanoid_coverage_test.go
    │   │   ├── arkanoid_test.go
    │   │   ├── attributes_test.go
    │   │   ├── autosave_settings_test.go
    │   │   ├── background_jobs_window.go
    │   │   ├── background_jobs_window_test.go
    │   │   ├── backspace_commandline_test.go
    │   │   ├── bom_test.go
    │   │   ├── bookmarks_dialog_test.go
    │   │   ├── bookmarks_test.go
    │   │   ├── bootstrap.go
    │   │   ├── bootstrap_backend.go
    │   │   ├── bootstrap_backend_test.go
    │   │   ├── bootstrap_coverage_batch34_test.go
    │   │   ├── bootstrap_coverage_batch37_test.go
    │   │   ├── bootstrap_coverage_batch42_test.go
    │   │   ├── bootstrap_coverage_batch43_test.go
    │   │   ├── bootstrap_coverage_batch45_test.go
    │   │   ├── bootstrap_coverage_batch46_test.go
    │   │   ├── bootstrap_coverage_batch47_test.go
    │   │   ├── bootstrap_coverage_extra_test.go
    │   │   ├── bootstrap_coverage_test.go
    │   │   ├── bootstrap_ctrlhandler_other.go
    │   │   ├── bootstrap_ctrlhandler_windows.go
    │   │   ├── bootstrap_ctrlhandler_windows_test.go
    │   │   ├── bootstrap_detach_unix.go
    │   │   ├── bootstrap_detach_unix_test.go
    │   │   ├── bootstrap_detach_windows.go
    │   │   ├── bootstrap_detach_windows_test.go
    │   │   ├── bootstrap_gui_test.go
    │   │   ├── bootstrap_nestedinput_other.go
    │   │   ├── bootstrap_nestedinput_test.go
    │   │   ├── bootstrap_nestedinput_windows.go
    │   │   ├── bootstrap_nestedinput_windows_test.go
    │   │   ├── bootstrap_session_test.go
    │   │   ├── bootstrap_settings.go
    │   │   ├── bootstrap_settings_test.go
    │   │   ├── bootstrap_startupdir_terminal_test.go
    │   │   ├── bootstrap_startupdir_test.go
    │   │   ├── bootstrap_startupfile_terminal_test.go
    │   │   ├── bootstrap_sudo_test.go
    │   │   ├── bootstrap_tty_test.go
    │   │   ├── bootstrap_unicode_test.go
    │   │   ├── calculator_ui.go
    │   │   ├── calculator_ui_test.go
    │   │   ├── calendar_ui.go
    │   │   ├── calendar_ui_test.go
    │   │   ├── catalog_fixture_test.go
    │   │   ├── child_env_test.go
    │   │   ├── child_env_universal_linux_test.go
    │   │   ├── clipboard_image_test.go
    │   │   ├── cloud_storage_lite.go
    │   │   ├── cloud_storage_lite_test.go
    │   │   ├── cloudfox_real_archive_test.go
    │   │   ├── cloudfox_real_cross_cloud_test.go
    │   │   ├── cloudfox_real_large_f5_test.go
    │   │   ├── cloudfox_real_ui_test.go
    │   │   ├── codepage_issue875_sticky_test.go
    │   │   ├── codepage_issue875_test.go
    │   │   ├── colorer_coverage_more_test.go
    │   │   ├── colorer_download_test.go
    │   │   ├── colorer_settings.go
    │   │   ├── colorer_settings_test.go
    │   │   ├── colorer_type_settings.go
    │   │   ├── colorer_type_settings_coverage_test.go
    │   │   ├── colorstyle_firstrun.go
    │   │   ├── colorstyle_firstrun_test.go
    │   │   ├── command_history_paths_test.go
    │   │   ├── command_palette.go
    │   │   ├── command_palette_assign.go
    │   │   ├── command_palette_assign_test.go
    │   │   ├── command_palette_direct_frames.go
    │   │   ├── command_palette_direct_frames_test.go
    │   │   ├── command_palette_direct_panels_test.go
    │   │   ├── command_palette_drives.go
    │   │   ├── command_palette_drives_test.go
    │   │   ├── command_palette_dynamic_test.go
    │   │   ├── command_palette_frames.go
    │   │   ├── command_palette_help.go
    │   │   ├── command_palette_help_test.go
    │   │   ├── command_palette_i18n.go
    │   │   ├── command_palette_i18n_test.go
    │   │   ├── command_palette_input_test.go
    │   │   ├── command_palette_macros.go
    │   │   ├── command_palette_menu_leaves_test.go
    │   │   ├── command_palette_menu_test.go
    │   │   ├── command_palette_modal.go
    │   │   ├── command_palette_panels.go
    │   │   ├── command_palette_prefixes.go
    │   │   ├── command_palette_search.go
    │   │   ├── command_palette_search_test.go
    │   │   ├── command_palette_test.go
    │   │   ├── command_palette_ui.go
    │   │   ├── command_palette_ui_test.go
    │   │   ├── command_palette_workspace.go
    │   │   ├── command_prefix_registry_test.go
    │   │   ├── commandline_workspace_test.go
    │   │   ├── compare_content_ui.go
    │   │   ├── compare_content_ui_test.go
    │   │   ├── compare_folders_ui.go
    │   │   ├── compare_folders_ui_coverage_test.go
    │   │   ├── config_test.go
    │   │   ├── console_passthrough_test.go
    │   │   ├── copy_dialog_options.go
    │   │   ├── copy_dialog_options_coverage_test.go
    │   │   ├── copy_dialog_options_more_coverage_test.go
    │   │   ├── copy_move_prompt_test.go
    │   │   ├── coverage_batch35_test.go
    │   │   ├── coverage_helpers_test.go
    │   │   ├── ctrl_row_labelkeys_test.go
    │   │   ├── debug.go
    │   │   ├── debug_hangdump_unix.go
    │   │   ├── debug_hangdump_windows.go
    │   │   ├── debug_log_test.go
    │   │   ├── delete_trash_test.go
    │   │   ├── diagnostics.go
    │   │   ├── diagnostics_test.go
    │   │   ├── dialog_copy_layout_test.go
    │   │   ├── dialog_copy_resize_test.go
    │   │   ├── dialog_layout_languages_test.go
    │   │   ├── dialog_layouts_test.go
    │   │   ├── dialog_outer_border.go
    │   │   ├── dialog_outer_border_test.go
    │   │   ├── dirsize_marked_test.go
    │   │   ├── disasm_editor_test.go
    │   │   ├── document_reader_test.go
    │   │   ├── dotnet_info.go
    │   │   ├── dotnet_info_test.go
    │   │   ├── dragdrop_test.go
    │   │   ├── drive_bookmarks_test.go
    │   │   ├── drive_menu_options_test.go
    │   │   ├── drive_tools_move_test.go
    │   │   ├── drop_scene_test.go
    │   │   ├── editor_actions_test.go
    │   │   ├── editor_binary_open_test.go
    │   │   ├── editor_events.go
    │   │   ├── editor_far3_keys_path_test.go
    │   │   ├── editor_host_test.go
    │   │   ├── editor_hotkeys_test.go
    │   │   ├── editor_keys_test.go
    │   │   ├── editor_projection_test.go
    │   │   ├── editor_save_wait_test.go
    │   │   ├── editor_search_profile_test.go
    │   │   ├── editor_test_helpers_test.go
    │   │   ├── f4_commands.go
    │   │   ├── f4_commands_test.go
    │   │   ├── farmenu_file_test.go
    │   │   ├── fast_find_overlay_test.go
    │   │   ├── fastfind_keep_actions_test.go
    │   │   ├── file_associations_dispatch_test.go
    │   │   ├── file_associations_test.go
    │   │   ├── file_ops_panel_test.go
    │   │   ├── file_panel_sorting_regression_test.go
    │   │   ├── find_file.go
    │   │   ├── find_file_semantic_test.go
    │   │   ├── find_file_test.go
    │   │   ├── fish_server.go
    │   │   ├── fish_server_test.go
    │   │   ├── fkeys_hidden_panels_test.go
    │   │   ├── folder_history_actions_test.go
    │   │   ├── folder_history_navigation_test.go
    │   │   ├── folder_history_panel_test.go
    │   │   ├── folder_history_qt_test.go
    │   │   ├── framewatch.go
    │   │   ├── framewatch_vtui.go
    │   │   ├── git_actions.go
    │   │   ├── grabber.go
    │   │   ├── grabber_mouse_test.go
    │   │   ├── grabber_test.go
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
    │   │   ├── history_bridge_coverage_test.go
    │   │   ├── history_dialog.go
    │   │   ├── history_dialog_test.go
    │   │   ├── history_hint_test.go
    │   │   ├── history_import_test.go
    │   │   ├── history_performance_test.go
    │   │   ├── hotkeys_ui.go
    │   │   ├── hotkeys_ui_test.go
    │   │   ├── image_gallery_panel_test.go
    │   │   ├── image_view_panel_test.go
    │   │   ├── ios_plugin_menu.go
    │   │   ├── ios_plugin_menu_test.go
    │   │   ├── issue1218_keybar_test.go
    │   │   ├── issue1229_test.go
    │   │   ├── issue1233_test.go
    │   │   ├── issue1237_test.go
    │   │   ├── issue1239_test.go
    │   │   ├── issue1268_test.go
    │   │   ├── issue408_reveal_test.go
    │   │   ├── issue413_test.go
    │   │   ├── issue54_test.go
    │   │   ├── issue561_test.go
    │   │   ├── issue631_test.go
    │   │   ├── issue821_test.go
    │   │   ├── issue856_mouse_capture_test.go
    │   │   ├── issue95_followup_test.go
    │   │   ├── keybar_folder_labels_test.go
    │   │   ├── keybar_injected_test.go
    │   │   ├── keymap_host_test.go
    │   │   ├── keymap_suspend.go
    │   │   ├── lang.go
    │   │   ├── lang_host_test.go
    │   │   ├── lite_build.go
    │   │   ├── lite_build_lite.go
    │   │   ├── local_language_files_test.go
    │   │   ├── macro_ctrlletter_test.go
    │   │   ├── macro_dispatch.go
    │   │   ├── macro_host.go
    │   │   ├── macro_host_test.go
    │   │   ├── macro_menu_items.go
    │   │   ├── macro_menu_items_test.go
    │   │   ├── macro_plugin_calls.go
    │   │   ├── macro_reload_action_test.go
    │   │   ├── macro_test.go
    │   │   ├── main_menu_bar_keys_test.go
    │   │   ├── main_menu_dropdown_test.go
    │   │   ├── main_test.go
    │   │   ├── managed_execution_test.go
    │   │   ├── markdown_view.go
    │   │   ├── markdown_view_test.go
    │   │   ├── media_app.go
    │   │   ├── menu_enabled_refresh_test.go
    │   │   ├── menu_history_action.go
    │   │   ├── menu_history_test.go
    │   │   ├── menu_hotkeys_test.go
    │   │   ├── menu_responsiveness_test.go
    │   │   ├── mock_failing_vfs_test.go
    │   │   ├── mock_pty_test.go
    │   │   ├── nativeui.go
    │   │   ├── navigation_mode_test.go
    │   │   ├── navigation_toggle_test.go
    │   │   ├── netbrowse_actions.go
    │   │   ├── new_file_layout_test.go
    │   │   ├── panel_actions_test.go
    │   │   ├── panel_menu_test.go
    │   │   ├── panel_plugin_keys_test.go
    │   │   ├── panel_plugins_test.go
    │   │   ├── panel_shortcuts_test.go
    │   │   ├── panel_visibility_test.go
    │   │   ├── panels_app_commands.go
    │   │   ├── panels_app_commands_coverage_test.go
    │   │   ├── panels_frame_app_test.go
    │   │   ├── panels_frame_drivecursor_windows_test.go
    │   │   ├── path_hints_test.go
    │   │   ├── path_identity_history_test.go
    │   │   ├── pdf_text.go
    │   │   ├── pdf_text_test.go
    │   │   ├── player_panel_actions_test.go
    │   │   ├── plughost_app.go
    │   │   ├── plugin_contributions.go
    │   │   ├── plugin_contributions_test.go
    │   │   ├── plugin_hotkeys_test.go
    │   │   ├── plugring_install_load_test.go
    │   │   ├── plugring_policy_test.go
    │   │   ├── plugring_rows_test.go
    │   │   ├── plugring_test.go
    │   │   ├── plugring_ui.go
    │   │   ├── plugring_ui_test.go
    │   │   ├── portable_paths_test.go
    │   │   ├── portable_test.go
    │   │   ├── process_environment_host.go
    │   │   ├── process_environment_host_test.go
    │   │   ├── proclist_actions.go
    │   │   ├── procname_linux.go
    │   │   ├── procname_linux_test.go
    │   │   ├── procname_other.go
    │   │   ├── pty_windows_panel_test.go
    │   │   ├── qt_document_lifecycle.go
    │   │   ├── qt_document_lifecycle_test.go
    │   │   ├── qt_document_loading_test.go
    │   │   ├── qt_document_speedup_benchmark_test.go
    │   │   ├── qt_document_viewport_test.go
    │   │   ├── qt_keybar_icons_test.go
    │   │   ├── qt_semantic.go
    │   │   ├── qt_semantic_menu_wrapper_test.go
    │   │   ├── qt_semantic_window_protocol_test.go
    │   │   ├── qt_version_test.go
    │   │   ├── queue_helpers_test.go
    │   │   ├── queue_semantic_test.go
    │   │   ├── quickview_api.go
    │   │   ├── quickview_binding_test.go
    │   │   ├── quit_dialog_keys_test.go
    │   │   ├── release_version_symbol_test.go
    │   │   ├── remote_file_execution_test.go
    │   │   ├── rpc_commands_test.go
    │   │   ├── runner_unix_panel_test.go
    │   │   ├── search_history_test.go
    │   │   ├── select_group_layout_test.go
    │   │   ├── semantic_actions_test.go
    │   │   ├── semantic_test.go
    │   │   ├── settings_coverage_batch16_test.go
    │   │   ├── settings_host.go
    │   │   ├── settings_host_coverage_batch49_test.go
    │   │   ├── settings_host_coverage_test.go
    │   │   ├── settings_host_runtime_coverage_batch50_test.go
    │   │   ├── settings_host_test.go
    │   │   ├── settings_routes.go
    │   │   ├── settings_save.go
    │   │   ├── settings_save_coverage_batch48_test.go
    │   │   ├── settings_save_route_test.go
    │   │   ├── share_dialog.go
    │   │   ├── share_dialog_coverage_test.go
    │   │   ├── share_dialog_test.go
    │   │   ├── sheet_actions.go
    │   │   ├── sheet_actions_coverage_test.go
    │   │   ├── sheet_actions_test.go
    │   │   ├── sheet_compare_coverage_test.go
    │   │   ├── sheet_dialogs.go
    │   │   ├── sheet_dialogs_coverage_test.go
    │   │   ├── sheet_frame.go
    │   │   ├── sheet_frame_coverage_extra_test.go
    │   │   ├── sheet_frame_test.go
    │   │   ├── sheet_palette.go
    │   │   ├── sheet_palette_test.go
    │   │   ├── shell_integration_test.go
    │   │   ├── shutdown_cancel_test.go
    │   │   ├── simple_exec_test.go
    │   │   ├── sort_groups_test.go
    │   │   ├── split_update_test.go
    │   │   ├── sqlite_actions.go
    │   │   ├── sqlite_actions_test.go
    │   │   ├── startup_coverage_batch18_test.go
    │   │   ├── static_direct_actions.go
    │   │   ├── static_direct_actions_test.go
    │   │   ├── svcmgr_actions.go
    │   │   ├── sync_dirs_coverage_test.go
    │   │   ├── sync_dirs_ui.go
    │   │   ├── temp_panel_test.go
    │   │   ├── term_app.go
    │   │   ├── term_app_coverage_batch32_test.go
    │   │   ├── term_app_other.go
    │   │   ├── term_app_windows.go
    │   │   ├── terminal_dnd_bind.go
    │   │   ├── terminal_log_offset_test.go
    │   │   ├── terminal_workspace_test.go
    │   │   ├── testdata
    │   │   │   └── action_order.golden
    │   │   ├── text_editor_bridge_test.go
    │   │   ├── theme_host_test.go
    │   │   ├── title.go
    │   │   ├── title_test.go
    │   │   ├── title_unix.go
    │   │   ├── title_windows.go
    │   │   ├── translator_test.go
    │   │   ├── updater.go
    │   │   ├── updater_issue635_test.go
    │   │   ├── updater_repro_lock_other_test.go
    │   │   ├── updater_repro_lock_windows_test.go
    │   │   ├── updater_repro_test.go
    │   │   ├── updater_test.go
    │   │   ├── upstream_compat.go
    │   │   ├── user_menu_ini_test.go
    │   │   ├── user_menu_subst_test.go
    │   │   ├── viewer_app.go
    │   │   ├── viewer_editor_history_test.go
    │   │   ├── viewer_keys_test.go
    │   │   ├── viewer_search_profile_test.go
    │   │   ├── viewer_search_workspace_test.go
    │   │   ├── vtvibe_ap.go
    │   │   ├── vtvibe_ap_coverage_test.go
    │   │   ├── vtvibe_ap_reject.go
    │   │   ├── vtvibe_ap_review.go
    │   │   ├── vtvibe_ap_review_test.go
    │   │   ├── vtvibe_ap_test.go
    │   │   ├── vtvibe_ap_undo.go
    │   │   ├── vtvibe_ap_undo_test.go
    │   │   ├── vtvibe_host.go
    │   │   ├── vtvibe_host_coverage_test.go
    │   │   ├── vtvibe_host_extra_coverage_test.go
    │   │   ├── vtvibe_host_test.go
    │   │   ├── win32_backend_test.go
    │   │   ├── window_toggle.go
    │   │   ├── window_toggle_other.go
    │   │   ├── window_toggle_test.go
    │   │   ├── window_toggle_windows.go
    │   │   ├── workspace_routing_test.go
    │   │   ├── workspace_session_test.go
    │   │   ├── worktree_identity.go
    │   │   ├── worktree_identity_coverage_test.go
    │   │   └── worktree_identity_test.go
    │   ├── appcmd
    │   │   └── commands.go
    │   ├── cmdline
    │   │   ├── apply_batch.go
    │   │   ├── apply_batch_test.go
    │   │   ├── apply_output.go
    │   │   ├── apply_output_test.go
    │   │   ├── apply_resources.go
    │   │   ├── apply_resources_test.go
    │   │   ├── apply_shortname_other.go
    │   │   ├── apply_shortname_windows.go
    │   │   ├── apply_shortname_windows_test.go
    │   │   ├── apply_subst.go
    │   │   ├── apply_subst_test.go
    │   │   ├── apply_transcript.go
    │   │   ├── autocomplete.go
    │   │   ├── filename_template.go
    │   │   ├── filename_template_test.go
    │   │   ├── line.go
    │   │   ├── line_semantic_test.go
    │   │   ├── line_test.go
    │   │   ├── main_test.go
    │   │   ├── prompt.go
    │   │   ├── prompt_test.go
    │   │   ├── prompt_unix.go
    │   │   ├── prompt_windows.go
    │   │   ├── qt_semantic.go
    │   │   ├── quotes.go
    │   │   ├── quotes_test.go
    │   │   ├── quoting.go
    │   │   ├── quoting_test.go
    │   │   ├── resolve_other.go
    │   │   ├── resolve_windows.go
    │   │   ├── resolve_windows_test.go
    │   │   └── selection_benchmark_test.go
    │   ├── colorer
    │   │   ├── configs
    │   │   │   └── base
    │   │   │       └── hrd
    │   │   │           └── rgb
    │   │   │               └── radiola.hrd
    │   │   └── embedded.go
    │   ├── config
    │   │   ├── appearance_settings_test.go
    │   │   ├── atomic.go
    │   │   ├── atomic_test.go
    │   │   ├── clipboard_image_test.go
    │   │   ├── colorstyle_configured_test.go
    │   │   ├── config.go
    │   │   ├── config_glyph_style_test.go
    │   │   ├── config_test.go
    │   │   ├── cursor_style_test.go
    │   │   ├── fallback_language_test.go
    │   │   ├── grouping.go
    │   │   ├── grouping_test.go
    │   │   ├── options.go
    │   │   ├── options_test.go
    │   │   ├── overlay.go
    │   │   ├── overlay_test.go
    │   │   ├── proxy_settings_test.go
    │   │   ├── schema.go
    │   │   ├── serialize_roundtrip_test.go
    │   │   ├── settings.go
    │   │   └── terminal_history_test.go
    │   ├── dialog
    │   │   ├── about.go
    │   │   ├── about_os_other.go
    │   │   ├── about_os_unix.go
    │   │   ├── about_os_windows.go
    │   │   ├── about_test.go
    │   │   ├── attributes.go
    │   │   ├── attributes_created_time_test.go
    │   │   ├── attributes_mixed_test.go
    │   │   ├── attributes_mtime_validation_test.go
    │   │   ├── attributes_recursive_test.go
    │   │   ├── attributes_unix.go
    │   │   ├── attributes_windows.go
    │   │   ├── attributes_windows_test.go
    │   │   ├── caption.go
    │   │   ├── config_editor.go
    │   │   ├── config_editor_coverage_batch36_test.go
    │   │   ├── config_editor_test.go
    │   │   ├── envman_help_test.go
    │   │   ├── file.go
    │   │   ├── file_layout_test.go
    │   │   ├── file_resize_test.go
    │   │   ├── file_test.go
    │   │   ├── goto.go
    │   │   ├── goto_test.go
    │   │   ├── help
    │   │   │   ├── README.md
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
    │   │   │   ├── ru.hlf
    │   │   │   ├── tr.hlf
    │   │   │   ├── uk.hlf
    │   │   │   └── zh.hlf
    │   │   ├── help.go
    │   │   ├── help_lang_test.go
    │   │   ├── help_languages.go
    │   │   ├── help_search.go
    │   │   ├── help_search_test.go
    │   │   ├── help_semantic.go
    │   │   ├── help_test.go
    │   │   ├── help_zoom_rewrap_test.go
    │   │   ├── history_import.go
    │   │   ├── history_import_other.go
    │   │   ├── history_import_test.go
    │   │   ├── history_import_windows.go
    │   │   ├── hotkey_capture.go
    │   │   ├── hotkey_capture_test.go
    │   │   ├── label.go
    │   │   ├── main_test.go
    │   │   ├── mask.go
    │   │   ├── new_file.go
    │   │   ├── new_file_test.go
    │   │   ├── path.go
    │   │   ├── profile_transfer_test.go
    │   │   ├── settings_codepage.go
    │   │   ├── settings_portable.go
    │   │   ├── settings_portable_test.go
    │   │   ├── settings_proxy.go
    │   │   ├── settings_proxy_coverage_test.go
    │   │   ├── settings_proxy_test.go
    │   │   ├── usermenu_import.go
    │   │   └── usermenu_import_test.go
    │   ├── diffview
    │   │   ├── diffview.go
    │   │   └── diffview_test.go
    │   ├── dirwatch
    │   │   ├── dirwatch.go
    │   │   ├── dirwatch_test.go
    │   │   ├── native_kqueue.go
    │   │   ├── native_linux.go
    │   │   ├── native_other.go
    │   │   ├── native_windows.go
    │   │   └── poll.go
    │   ├── dotnet
    │   │   ├── attrargs.go
    │   │   ├── attrargs_test.go
    │   │   ├── il.go
    │   │   ├── il_test.go
    │   │   ├── metadata.go
    │   │   ├── metadata_test.go
    │   │   ├── report.go
    │   │   └── signature.go
    │   ├── editor
    │   │   ├── amount_words.go
    │   │   ├── amount_words_test.go
    │   │   ├── base64.go
    │   │   ├── buffer_async.go
    │   │   ├── buffer_async_test.go
    │   │   ├── buffer_mapped.go
    │   │   ├── buffer_mapped_unix.go
    │   │   ├── buffer_mapped_windows.go
    │   │   ├── buffer_prepare.go
    │   │   ├── calculator.go
    │   │   ├── colorer.go
    │   │   ├── colorer_async.go
    │   │   ├── colorer_check.go
    │   │   ├── colorer_check_test.go
    │   │   ├── colorer_cpu.go
    │   │   ├── colorer_cpu_amd64.go
    │   │   ├── colorer_cpu_amd64_test.go
    │   │   ├── colorer_cpu_other.go
    │   │   ├── colorer_cpu_test.go
    │   │   ├── colorer_diagnostics_test.go
    │   │   ├── colorer_downloader.go
    │   │   ├── colorer_lite.go
    │   │   ├── colorer_outline.go
    │   │   ├── colorer_outline_frame.go
    │   │   ├── colorer_outline_frame_test.go
    │   │   ├── colorer_outline_test.go
    │   │   ├── colorer_pair_search.go
    │   │   ├── colorer_pairs.go
    │   │   ├── colorer_pairs_test.go
    │   │   ├── colorer_params.go
    │   │   ├── colorer_params_test.go
    │   │   ├── colorer_plugin_test.go
    │   │   ├── colorer_reload.go
    │   │   ├── colorer_setups.go
    │   │   ├── colorer_text.go
    │   │   ├── colorer_type_settings.go
    │   │   ├── colorer_type_settings_test.go
    │   │   ├── colorer_types.go
    │   │   ├── colorer_types_coverage_test.go
    │   │   ├── colorer_types_test.go
    │   │   ├── colorer_userhrd_location_test.go
    │   │   ├── colorer_userpath_test.go
    │   │   ├── colorer_window.go
    │   │   ├── crosshair.go
    │   │   ├── crosshair_test.go
    │   │   ├── document_reader_test.go
    │   │   ├── editor_base64_test.go
    │   │   ├── editor_calculator_test.go
    │   │   ├── editor_codepage_test.go
    │   │   ├── editor_coverage_batch27_test.go
    │   │   ├── editor_coverage_batch28_test.go
    │   │   ├── editor_delta_test.go
    │   │   ├── editor_duplicate_line_test.go
    │   │   ├── editor_fade_test.go
    │   │   ├── editor_features_test.go
    │   │   ├── editor_find_all_test.go
    │   │   ├── editor_hex_colors_test.go
    │   │   ├── editor_highlight_budget_test.go
    │   │   ├── editor_indent_test.go
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
    │   │   ├── editor_save_as_dialog_test.go
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
    │   │   ├── events.go
    │   │   ├── events_test.go
    │   │   ├── external_command.go
    │   │   ├── external_ctty_linux.go
    │   │   ├── external_ctty_other.go
    │   │   ├── external_editor_ctty_linux_test.go
    │   │   ├── external_editor_process_unix_test.go
    │   │   ├── external_editor_test.go
    │   │   ├── external_freebsd.go
    │   │   ├── external_freebsd_test.go
    │   │   ├── external_unix.go
    │   │   ├── external_windows.go
    │   │   ├── fade.go
    │   │   ├── far3_keys.go
    │   │   ├── far3_keys_test.go
    │   │   ├── findall.go
    │   │   ├── goto_test.go
    │   │   ├── grapheme.go
    │   │   ├── host.go
    │   │   ├── indent.go
    │   │   ├── index_status.go
    │   │   ├── issue1230_flicker_test.go
    │   │   ├── issue1230_test.go
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
    │   │   ├── qt_editor_reflow.go
    │   │   ├── qt_editor_reflow_test.go
    │   │   ├── qt_editor_save_regression_test.go
    │   │   ├── qt_editor_short_selection_test.go
    │   │   ├── qt_editor_text_projection.go
    │   │   ├── qt_semantic.go
    │   │   ├── qt_semantic_window_render.go
    │   │   ├── replace_confirm.go
    │   │   ├── save_as.go
    │   │   ├── search_progress.go
    │   │   ├── search_progress_test.go
    │   │   ├── search_remote.go
    │   │   ├── search_reveal.go
    │   │   ├── search_reveal_test.go
    │   │   ├── secondary_carets_test.go
    │   │   ├── semantic_helpers_test.go
    │   │   ├── semantic_model_test.go
    │   │   ├── sort.go
    │   │   ├── sort_test.go
    │   │   ├── status.go
    │   │   ├── status_coverage_test.go
    │   │   ├── surface_benchmark_test.go
    │   │   ├── surface_recorder_test.go
    │   │   ├── url_links_hover_test.go
    │   │   ├── view.go
    │   │   ├── view_coverage_batch23_test.go
    │   │   ├── view_coverage_contract_test.go
    │   │   ├── view_helpers_coverage_batch21_test.go
    │   │   ├── view_lowlevel_coverage_test.go
    │   │   ├── view_mdsplit.go
    │   │   ├── view_mdsplit_test.go
    │   │   ├── view_semantic.go
    │   │   ├── view_semantic_coverage_test.go
    │   │   ├── viewport.go
    │   │   ├── viewport_helpers_test.go
    │   │   ├── viewport_test.go
    │   │   ├── window_protocol_test.go
    │   │   ├── window_render_test.go
    │   │   ├── wrap_mark_attr_test.go
    │   │   └── wrap_safety.go
    │   ├── filemask
    │   │   ├── mask.go
    │   │   └── mask_test.go
    │   ├── fileops
    │   │   ├── access_rights.go
    │   │   ├── access_rights_test.go
    │   │   ├── archive_index.go
    │   │   ├── archive_index_fallback.go
    │   │   ├── archive_index_test.go
    │   │   ├── buttons.go
    │   │   ├── buttons_test.go
    │   │   ├── codepage.go
    │   │   ├── compare.go
    │   │   ├── compare_folders_test.go
    │   │   ├── copy_options.go
    │   │   ├── copy_options_test.go
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
    │   │   ├── host.go
    │   │   ├── host_test.go
    │   │   ├── identity.go
    │   │   ├── identity_test.go
    │   │   ├── issue149_test.go
    │   │   ├── issue815_test.go
    │   │   ├── local.go
    │   │   ├── main_test.go
    │   │   ├── mask.go
    │   │   ├── ops.go
    │   │   ├── ops_case_test.go
    │   │   ├── ops_dialog.go
    │   │   ├── path_identity_test.go
    │   │   ├── pump.go
    │   │   ├── qt_queue_pause_test.go
    │   │   ├── qt_queue_semantic.go
    │   │   ├── qt_queue_semantic_test.go
    │   │   ├── queue.go
    │   │   ├── queue_coverage_test.go
    │   │   ├── queue_manager_test.go
    │   │   ├── real_device_copy_test.go
    │   │   ├── report.go
    │   │   ├── report_contract_test.go
    │   │   ├── rights_extra.go
    │   │   ├── same_device_move_test.go
    │   │   ├── security_other.go
    │   │   ├── security_windows.go
    │   │   ├── security_windows_test.go
    │   │   ├── state.go
    │   │   ├── state_key_test.go
    │   │   ├── state_test.go
    │   │   ├── symlink.go
    │   │   ├── symlink_coverage_test.go
    │   │   ├── sync.go
    │   │   ├── sync_test.go
    │   │   ├── tracker.go
    │   │   └── transfer_identity_integration_test.go
    │   ├── frameborder
    │   │   ├── frameborder.go
    │   │   └── frameborder_test.go
    │   ├── fusefs
    │   │   ├── BENCH.md
    │   │   ├── FUSE.md
    │   │   ├── bench-all.sh
    │   │   ├── bench.sh
    │   │   ├── bridge.go
    │   │   ├── bridge_test.go
    │   │   ├── cli.go
    │   │   ├── cli_coverage_signal_test.go
    │   │   ├── cli_coverage_test.go
    │   │   ├── cli_test.go
    │   │   ├── coverage_helpers_test.go
    │   │   ├── fusefs.go
    │   │   ├── mountspec.go
    │   │   ├── node_fuse.go
    │   │   ├── node_fuse_test.go
    │   │   ├── node_unsupported.go
    │   │   ├── platform_other.go
    │   │   ├── platform_unix.go
    │   │   ├── registry.go
    │   │   ├── staged_test.go
    │   │   └── writers_test.go
    │   ├── gui
    │   │   ├── assets
    │   │   │   └── icon
    │   │   │       ├── README.md
    │   │   │       ├── embed.go
    │   │   │       ├── f4-16.svg
    │   │   │       ├── f4-24.svg
    │   │   │       ├── f4-30.svg
    │   │   │       ├── f4-32.svg
    │   │   │       ├── f4-36.svg
    │   │   │       ├── f4-42.svg
    │   │   │       ├── f4.svg
    │   │   │       └── generated
    │   │   │           ├── f4-1024.png
    │   │   │           ├── f4-128.png
    │   │   │           ├── f4-16.png
    │   │   │           ├── f4-24.png
    │   │   │           ├── f4-256.png
    │   │   │           ├── f4-28.png
    │   │   │           ├── f4-30.png
    │   │   │           ├── f4-32.png
    │   │   │           ├── f4-36.png
    │   │   │           ├── f4-42.png
    │   │   │           ├── f4-48.png
    │   │   │           ├── f4-512.png
    │   │   │           ├── f4-56.png
    │   │   │           ├── f4-64.png
    │   │   │           ├── f4.icns
    │   │   │           └── f4.ico
    │   │   ├── backend.go
    │   │   ├── backend_ffi.go
    │   │   ├── backend_stub.go
    │   │   ├── backend_test.go
    │   │   ├── backends.go
    │   │   ├── backends_lite.go
    │   │   ├── backends_lite_test.go
    │   │   ├── backends_test.go
    │   │   ├── backends_tty.go
    │   │   ├── backends_tty_test.go
    │   │   ├── font.go
    │   │   ├── font_catalog.go
    │   │   ├── font_catalog_test.go
    │   │   ├── font_catalog_unix.go
    │   │   ├── font_catalog_unix_test.go
    │   │   ├── font_catalog_windows.go
    │   │   ├── font_catalog_windows_test.go
    │   │   ├── font_combo.go
    │   │   ├── font_notwindows.go
    │   │   ├── font_test.go
    │   │   ├── font_windows.go
    │   │   ├── font_windows_test.go
    │   │   ├── icon_darwin.go
    │   │   ├── icon_darwin_test.go
    │   │   ├── icon_stub.go
    │   │   ├── icon_unix.go
    │   │   ├── icon_windows.go
    │   │   ├── icon_windows_coverage_test.go
    │   │   ├── icon_windows_test.go
    │   │   ├── liteguard.go
    │   │   ├── run_unix.go
    │   │   ├── run_unix_test.go
    │   │   ├── run_windows.go
    │   │   ├── runtime_mode.go
    │   │   ├── runtime_mode_test.go
    │   │   ├── window.go
    │   │   ├── winepath_other.go
    │   │   ├── winepath_windows.go
    │   │   ├── winepath_windows_test.go
    │   │   └── worktree.go
    │   ├── hideconsole
    │   │   ├── go.mod
    │   │   └── hideconsole.go
    │   ├── history
    │   │   ├── edit.go
    │   │   ├── edit_test.go
    │   │   ├── far2l.go
    │   │   ├── far3.go
    │   │   ├── far3_merge.go
    │   │   ├── far3_test.go
    │   │   ├── history_coverage_test.go
    │   │   ├── menu.go
    │   │   ├── navigation_trace_test.go
    │   │   ├── path_identity_integration_test.go
    │   │   ├── paths.go
    │   │   ├── paths_netfox_test.go
    │   │   ├── persistence_test.go
    │   │   ├── pins.go
    │   │   ├── provider.go
    │   │   ├── shell.go
    │   │   ├── shell_test.go
    │   │   └── viewedit.go
    │   ├── hostwidth
    │   │   ├── hostwidth.go
    │   │   └── hostwidth_test.go
    │   ├── i18n
    │   │   ├── lang
    │   │   │   ├── README.md
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
    │   │   │   ├── ru.lng
    │   │   │   ├── tr.lng
    │   │   │   ├── uk.lng
    │   │   │   └── zh.lng
    │   │   ├── lang.go
    │   │   ├── lang_bidi_test.go
    │   │   ├── lang_consistency_test.go
    │   │   ├── lang_contamination_test.go
    │   │   ├── lang_fallback_priority_test.go
    │   │   ├── lang_homoglyphs_test.go
    │   │   ├── lang_packs_test.go
    │   │   ├── lang_ru_complete_test.go
    │   │   ├── lang_scripts_test.go
    │   │   ├── lang_test.go
    │   │   ├── langfs.go
    │   │   ├── langfs_extralite.go
    │   │   ├── language_list_test.go
    │   │   ├── languages.go
    │   │   ├── msg_keys_test.go
    │   │   ├── packs.go
    │   │   └── settings_translations_test.go
    │   ├── ini
    │   │   ├── ini.go
    │   │   └── ini_test.go
    │   ├── install
    │   │   ├── cli.go
    │   │   ├── cli_test.go
    │   │   ├── desktop.go
    │   │   ├── desktop_test.go
    │   │   ├── install.go
    │   │   └── install_test.go
    │   ├── keymap
    │   │   ├── farkeys.go
    │   │   ├── farkeys_selection_test.go
    │   │   ├── hotkeys.go
    │   │   ├── icons.go
    │   │   ├── input.go
    │   │   ├── input_translation_test.go
    │   │   ├── keybar_combined_rows_test.go
    │   │   ├── keybar_labels_enabled_test.go
    │   │   ├── keymap_test.go
    │   │   ├── kitty.go
    │   │   ├── kitty_coverage_batch39_test.go
    │   │   ├── kitty_coverage_extra_test.go
    │   │   ├── mackeys.go
    │   │   ├── mackeys_test.go
    │   │   ├── navigation_toggle_test.go
    │   │   ├── remap.go
    │   │   ├── terminal_mouse_offset_test.go
    │   │   ├── translate_kitty_test.go
    │   │   ├── ttyx.go
    │   │   └── ttyx_keys_test.go
    │   ├── luaplug
    │   │   ├── convert.go
    │   │   ├── convert_test.go
    │   │   ├── f4rpc.go
    │   │   ├── ffi.go
    │   │   ├── ffi_test.go
    │   │   ├── goid.go
    │   │   ├── luastate_test.go
    │   │   ├── runtime.go
    │   │   ├── runtime_deadline_test.go
    │   │   ├── runtime_test.go
    │   │   └── sandbox.go
    │   ├── macro
    │   │   ├── coverage_extra_test.go
    │   │   ├── engine.go
    │   │   ├── engine_test.go
    │   │   ├── export.go
    │   │   ├── export_test.go
    │   │   ├── lua.go
    │   │   ├── lua_api.go
    │   │   ├── lua_api_coverage_batch19_test.go
    │   │   ├── lua_compat.go
    │   │   ├── lua_events_test.go
    │   │   ├── lua_extralite.go
    │   │   ├── lua_mf_extra.go
    │   │   ├── lua_mf_extra_test.go
    │   │   ├── lua_regex.go
    │   │   ├── lua_test.go
    │   │   ├── lua_timer.go
    │   │   ├── lua_types.go
    │   │   ├── plugin_calls.go
    │   │   ├── plugin_calls_test.go
    │   │   ├── reload.go
    │   │   └── reload_test.go
    │   ├── mdmath
    │   │   ├── convert.go
    │   │   ├── mdmath_test.go
    │   │   └── prepare.go
    │   ├── media
    │   │   ├── ansi_art.go
    │   │   ├── ansi_art_test.go
    │   │   ├── application.go
    │   │   ├── audio.go
    │   │   ├── audio_context_reuse_test.go
    │   │   ├── audio_decode.go
    │   │   ├── audio_decode_bad_stream_coverage_test.go
    │   │   ├── audio_decode_contract_test.go
    │   │   ├── audio_decode_coverage_extra_test.go
    │   │   ├── audio_decode_coverage_test.go
    │   │   ├── audio_decode_external_coverage_test.go
    │   │   ├── audio_decode_flac_coverage_test.go
    │   │   ├── audio_decode_test.go
    │   │   ├── audio_engine_contract_test.go
    │   │   ├── audio_format.go
    │   │   ├── audio_format_test.go
    │   │   ├── audio_oto.go
    │   │   ├── audio_stub.go
    │   │   ├── blit_test.go
    │   │   ├── image.go
    │   │   ├── image_bmp.go
    │   │   ├── image_bmp_coverage_test.go
    │   │   ├── image_console_stats.go
    │   │   ├── image_console_stats_test.go
    │   │   ├── image_decode.go
    │   │   ├── image_decode_test.go
    │   │   ├── image_encode.go
    │   │   ├── image_encode_test.go
    │   │   ├── image_external.go
    │   │   ├── image_external_test.go
    │   │   ├── image_formats_test.go
    │   │   ├── image_gallery.go
    │   │   ├── image_gallery_coverage_test.go
    │   │   ├── image_gallery_test.go
    │   │   ├── image_native_darwin.go
    │   │   ├── image_native_darwin_test.go
    │   │   ├── image_preview.go
    │   │   ├── image_preview_coverage_test.go
    │   │   ├── image_preview_parsing_coverage_test.go
    │   │   ├── image_preview_success_coverage_test.go
    │   │   ├── image_preview_test.go
    │   │   ├── image_qoi.go
    │   │   ├── image_qoi_coverage_test.go
    │   │   ├── image_quick_preview_coverage_test.go
    │   │   ├── image_slideshow.go
    │   │   ├── image_slideshow_test.go
    │   │   ├── image_test.go
    │   │   ├── image_transform.go
    │   │   ├── image_transform_test.go
    │   │   ├── image_view.go
    │   │   ├── image_view_coverage_extra_test.go
    │   │   ├── image_view_halfblocks_test.go
    │   │   ├── image_view_orient_test.go
    │   │   ├── image_view_overlay_test.go
    │   │   ├── image_view_test.go
    │   │   ├── main_test.go
    │   │   ├── overlay_console.go
    │   │   ├── overlay_console_key_test.go
    │   │   ├── overlay_x11.go
    │   │   ├── overlay_x11_test.go
    │   │   ├── tools.go
    │   │   ├── tools_test.go
    │   │   ├── video.go
    │   │   ├── video_frame_view.go
    │   │   ├── video_frame_view_test.go
    │   │   ├── video_frames.go
    │   │   ├── video_frames_test.go
    │   │   ├── video_state.go
    │   │   ├── video_state_test.go
    │   │   ├── video_test.go
    │   │   ├── video_view.go
    │   │   └── video_view_coverage_test.go
    │   ├── mediatiming
    │   │   ├── qt_media_timing.go
    │   │   └── qt_media_timing_test.go
    │   ├── menuhotkeys
    │   │   ├── menuhotkeys.go
    │   │   └── menuhotkeys_test.go
    │   ├── mermaid
    │   │   ├── class.go
    │   │   ├── mermaid.go
    │   │   ├── mermaid_test.go
    │   │   ├── more.go
    │   │   └── sequence.go
    │   ├── nativeui
    │   │   ├── adapter.go
    │   │   ├── dialog_key_hints_test.go
    │   │   ├── directory_preview_projection_test.go
    │   │   ├── incremental_test.go
    │   │   ├── list_item_states_test.go
    │   │   ├── menu_wrapper_test.go
    │   │   ├── model_test.go
    │   │   ├── overlay_order_test.go
    │   │   ├── panel_status_test.go
    │   │   ├── path_bar_test.go
    │   │   ├── quickview_model_test.go
    │   │   ├── scene.go
    │   │   ├── scene_columns_test.go
    │   │   ├── scene_help_test.go
    │   │   ├── scene_incremental.go
    │   │   └── settings_semantic_test.go
    │   ├── navtrace
    │   │   ├── document.go
    │   │   ├── document_test.go
    │   │   ├── incremental.go
    │   │   ├── qt_navigation_benchmark.go
    │   │   ├── qt_navigation_benchmark_clock_fallback.go
    │   │   ├── qt_navigation_benchmark_clock_raw.go
    │   │   ├── qt_navigation_benchmark_clock_windows.go
    │   │   ├── qt_navigation_benchmark_clock_windows_test.go
    │   │   └── qt_navigation_benchmark_test.go
    │   ├── netproxy
    │   │   ├── coverage_test.go
    │   │   ├── keepalive_linux_test.go
    │   │   ├── netproxy.go
    │   │   ├── netproxy_gap_test.go
    │   │   └── netproxy_test.go
    │   ├── numeric
    │   │   ├── memory.go
    │   │   ├── numeric.go
    │   │   ├── numeric_test.go
    │   │   └── size.go
    │   ├── numwords
    │   │   ├── numwords.go
    │   │   └── numwords_test.go
    │   ├── panel
    │   │   ├── actions.go
    │   │   ├── actions_coverage_test.go
    │   │   ├── activation_renderer_test.go
    │   │   ├── android_navigation_test.go
    │   │   ├── apply.go
    │   │   ├── apply_coverage_batch29_test.go
    │   │   ├── apply_coverage_extra_test.go
    │   │   ├── apply_dialog.go
    │   │   ├── apply_layout_test.go
    │   │   ├── apply_shutdown.go
    │   │   ├── associations.go
    │   │   ├── associations_editor.go
    │   │   ├── associations_ui.go
    │   │   ├── audio_decode_panel_test.go
    │   │   ├── autofilter.go
    │   │   ├── autofilter_exact_test.go
    │   │   ├── background_jobs_session_test.go
    │   │   ├── base64_file.go
    │   │   ├── base64_file_coverage_test.go
    │   │   ├── base64_file_test.go
    │   │   ├── bookmarks.go
    │   │   ├── bookmarks_dialog.go
    │   │   ├── bookmarks_dialog_test.go
    │   │   ├── bookmarks_plugin.go
    │   │   ├── bridge_texteditor.go
    │   │   ├── bridge_visren.go
    │   │   ├── bridge_visren_coverage_test.go
    │   │   ├── clipboard_image.go
    │   │   ├── clipboard_image_test.go
    │   │   ├── cmd_session_test.go
    │   │   ├── column_resize_test.go
    │   │   ├── commandline_multiline.go
    │   │   ├── commandline_multiline_test.go
    │   │   ├── commandline_visibility.go
    │   │   ├── commandline_visibility_test.go
    │   │   ├── commandline_workspace.go
    │   │   ├── commandline_workspace_test.go
    │   │   ├── console.go
    │   │   ├── console_rows_test.go
    │   │   ├── context_editors_test.go
    │   │   ├── coverage_player_helpers_test.go
    │   │   ├── custom_column_modes_test.go
    │   │   ├── device_presentation_test.go
    │   │   ├── directory_cache.go
    │   │   ├── dirwatch.go
    │   │   ├── dirwatch_test.go
    │   │   ├── document_geometry_test.go
    │   │   ├── document_host.go
    │   │   ├── drive_menu_headings_test.go
    │   │   ├── drives_bookmarks.go
    │   │   ├── drives_bookmarks_test.go
    │   │   ├── drives_bookmarks_ui.go
    │   │   ├── drives_bookmarks_ui_test.go
    │   │   ├── drives_menu.go
    │   │   ├── drives_menu_ampersand_test.go
    │   │   ├── drives_menu_unix.go
    │   │   ├── drives_menu_windows.go
    │   │   ├── drives_tools_order.go
    │   │   ├── drives_tools_order_test.go
    │   │   ├── drives_tools_visibility.go
    │   │   ├── drives_tools_visibility_test.go
    │   │   ├── drop_dialog_test.go
    │   │   ├── edit_command_test.go
    │   │   ├── entry_totals_cache_test.go
    │   │   ├── exec.go
    │   │   ├── file_clipboard.go
    │   │   ├── file_clipboard_test.go
    │   │   ├── file_fields.go
    │   │   ├── file_fields_test.go
    │   │   ├── file_panel_test.go
    │   │   ├── folder_event_test.go
    │   │   ├── frame.go
    │   │   ├── frame_command.go
    │   │   ├── frame_command_darwin_test.go
    │   │   ├── frame_command_dispatch_test.go
    │   │   ├── frame_command_test.go
    │   │   ├── frame_coverage_batch22_test.go
    │   │   ├── frame_coverage_batch25_test.go
    │   │   ├── frame_coverage_test.go
    │   │   ├── frame_dragdrop.go
    │   │   ├── frame_dragdrop_nested.go
    │   │   ├── frame_dragdrop_term.go
    │   │   ├── frame_dragdrop_vfs.go
    │   │   ├── frame_externalui.go
    │   │   ├── frame_gallery_menu.go
    │   │   ├── frame_gallery_menu_test.go
    │   │   ├── frame_helpers_coverage_test.go
    │   │   ├── frame_history_coverage_test.go
    │   │   ├── frame_manager_test_helpers_test.go
    │   │   ├── frame_procenv.go
    │   │   ├── frame_progress.go
    │   │   ├── frame_remoteopen.go
    │   │   ├── frame_remoteopen_test.go
    │   │   ├── frame_shell_darwin_test.go
    │   │   ├── frame_translator.go
    │   │   ├── frame_workspace.go
    │   │   ├── frame_workspace_terminal.go
    │   │   ├── fuse_list.go
    │   │   ├── fuse_list_coverage_extra_test.go
    │   │   ├── fuse_list_coverage_test.go
    │   │   ├── fuse_mount.go
    │   │   ├── fuse_mount_coverage_test.go
    │   │   ├── gallery_session.go
    │   │   ├── grouping.go
    │   │   ├── grouping_menu.go
    │   │   ├── grouping_menu_coverage_test.go
    │   │   ├── grouping_test.go
    │   │   ├── grouping_view.go
    │   │   ├── header_label_zone_test.go
    │   │   ├── highlight_sort_test.go
    │   │   ├── hints.go
    │   │   ├── host.go
    │   │   ├── host_console_replies.go
    │   │   ├── host_console_replies_test.go
    │   │   ├── host_coverage_test.go
    │   │   ├── host_input_modes.go
    │   │   ├── host_input_modes_other.go
    │   │   ├── host_input_modes_test.go
    │   │   ├── host_input_modes_windows.go
    │   │   ├── hotkey_conditions.go
    │   │   ├── info.go
    │   │   ├── info_panel_test.go
    │   │   ├── info_usage.go
    │   │   ├── insert_dir_path_test.go
    │   │   ├── issue1131_focus_test.go
    │   │   ├── issue1131_test.go
    │   │   ├── issue1184_test.go
    │   │   ├── issue1218_keybar_test.go
    │   │   ├── issue1234_test.go
    │   │   ├── issue1320_keybar_test.go
    │   │   ├── issue1804_test.go
    │   │   ├── issue863_terminal_test.go
    │   │   ├── issue915_keybar_test.go
    │   │   ├── kitty_passthrough.go
    │   │   ├── kitty_passthrough_test.go
    │   │   ├── layout_selection_coverage_test.go
    │   │   ├── lazy_progress_test.go
    │   │   ├── list.go
    │   │   ├── list_access.go
    │   │   ├── list_access_other_test.go
    │   │   ├── list_access_test.go
    │   │   ├── list_access_windows_test.go
    │   │   ├── list_reconnect.go
    │   │   ├── list_reload_cursor_test.go
    │   │   ├── list_size.go
    │   │   ├── list_size_test.go
    │   │   ├── lone_alt.go
    │   │   ├── lone_alt_test.go
    │   │   ├── lookup.go
    │   │   ├── main_test.go
    │   │   ├── managed_exec_debounce_test.go
    │   │   ├── media_registration_test.go
    │   │   ├── media_registry.go
    │   │   ├── media_revision_test.go
    │   │   ├── menu_cache_test.go
    │   │   ├── menu_marks.go
    │   │   ├── menu_semantic.go
    │   │   ├── menubar_dropdown_test.go
    │   │   ├── menukeys.go
    │   │   ├── menukeys_test.go
    │   │   ├── mock_pty_test.go
    │   │   ├── namecompare.go
    │   │   ├── namecompare_extralite.go
    │   │   ├── navigate_elevated_async_test.go
    │   │   ├── navigation.go
    │   │   ├── navigation_guards_coverage_test.go
    │   │   ├── navigation_trace_test.go
    │   │   ├── navigation_vfs_coverage_test.go
    │   │   ├── panels_frame_pty_test.go
    │   │   ├── panels_frame_test.go
    │   │   ├── paste.go
    │   │   ├── paste_test.go
    │   │   ├── pins.go
    │   │   ├── pins_coverage_test.go
    │   │   ├── player.go
    │   │   ├── player_coverage_extra_test.go
    │   │   ├── player_coverage_test.go
    │   │   ├── player_test.go
    │   │   ├── plugin_default_hotkeys_test.go
    │   │   ├── plugin_hotkey_dialog.go
    │   │   ├── plugin_hotkey_dialog_test.go
    │   │   ├── plugin_hotkeys.go
    │   │   ├── plugin_menu_commands_test.go
    │   │   ├── plugin_menu_visibility.go
    │   │   ├── plugin_menu_visibility_test.go
    │   │   ├── plugins.go
    │   │   ├── plugins_closekey_test.go
    │   │   ├── plugins_swap_test.go
    │   │   ├── prefixes.go
    │   │   ├── prefixes_registry_windows_test.go
    │   │   ├── prefixes_test.go
    │   │   ├── press_key_test.go
    │   │   ├── proactive_kitty_protocol_test.go
    │   │   ├── process_environment_panel_test.go
    │   │   ├── progress_overlay_test.go
    │   │   ├── prompt.go
    │   │   ├── prompt_coverage_extra_test.go
    │   │   ├── pty_geometry.go
    │   │   ├── pty_reader.go
    │   │   ├── pty_reader_test.go
    │   │   ├── qt_commandline_drop.go
    │   │   ├── qt_commandline_drop_test.go
    │   │   ├── qt_document_lifecycle.go
    │   │   ├── qt_macos_locations_darwin.go
    │   │   ├── qt_macos_locations_other.go
    │   │   ├── qt_macos_query_vfs_darwin.go
    │   │   ├── qt_macos_query_vfs_darwin_test.go
    │   │   ├── qt_macos_values_darwin.go
    │   │   ├── qt_panel_refresh.go
    │   │   ├── qt_panel_refresh_test.go
    │   │   ├── qt_semantic.go
    │   │   ├── qt_semantic_dragdrop.go
    │   │   ├── qt_semantic_dragdrop_conflict_test.go
    │   │   ├── qt_semantic_dragdrop_test.go
    │   │   ├── qt_semantic_incremental.go
    │   │   ├── qt_semantic_panel_status.go
    │   │   ├── qt_semantic_panel_status_test.go
    │   │   ├── qt_semantic_test.go
    │   │   ├── qt_semantic_workspace_drag_test.go
    │   │   ├── qt_split.go
    │   │   ├── qt_split_test.go
    │   │   ├── qt_windows_locations_menu_stub.go
    │   │   ├── qt_windows_locations_menu_windows.go
    │   │   ├── qt_windows_locations_menu_windows_test.go
    │   │   ├── quick_view_panel_test.go
    │   │   ├── quick_view_provider_test.go
    │   │   ├── quick_view_refused_test.go
    │   │   ├── quickview.go
    │   │   ├── quickview_colors_test.go
    │   │   ├── quickview_coverage_test.go
    │   │   ├── quickview_dir.go
    │   │   ├── quickview_preview.go
    │   │   ├── quickview_preview_test.go
    │   │   ├── quickview_qml_contract_test.go
    │   │   ├── reconnect_test.go
    │   │   ├── remote.go
    │   │   ├── remote_pty_backoff_test.go
    │   │   ├── selection_extension.go
    │   │   ├── selection_extension_test.go
    │   │   ├── selection_journal_test.go
    │   │   ├── selection_panel_test.go
    │   │   ├── selection_shortcuts_test.go
    │   │   ├── semantic_helpers_test.go
    │   │   ├── semantic_model_test.go
    │   │   ├── semantic_uri_navigation_test.go
    │   │   ├── session_cmd.go
    │   │   ├── settings.go
    │   │   ├── settings_test.go
    │   │   ├── shell_session_test.go
    │   │   ├── shell_start_dir_test.go
    │   │   ├── smb_target_test.go
    │   │   ├── sort.go
    │   │   ├── sort_groups_test.go
    │   │   ├── sort_natural.go
    │   │   ├── sort_natural_test.go
    │   │   ├── state.go
    │   │   ├── strict_autofilter_test.go
    │   │   ├── symlink_target.go
    │   │   ├── symlink_target_test.go
    │   │   ├── temp.go
    │   │   ├── temp_coverage_test.go
    │   │   ├── temp_dragout_test.go
    │   │   ├── terminal_helpers_test.go
    │   │   ├── terminal_only_workspace_test.go
    │   │   ├── terminal_overlay_test.go
    │   │   ├── terminal_redraw_target_test.go
    │   │   ├── terminal_redraw_test.go
    │   │   ├── tools_options_dialog.go
    │   │   ├── tools_options_dialog_test.go
    │   │   ├── transfer_dialog_test.go
    │   │   ├── tree.go
    │   │   ├── tree_persist.go
    │   │   ├── tree_persist_test.go
    │   │   ├── tree_test.go
    │   │   ├── upstream_semantic_test.go
    │   │   ├── uri_navigation_test.go
    │   │   ├── user_menu_ui_test.go
    │   │   ├── usermenu.go
    │   │   ├── usermenu_coverage_test.go
    │   │   ├── usermenu_execution_test.go
    │   │   ├── usermenu_farfile.go
    │   │   ├── usermenu_import.go
    │   │   ├── usermenu_import_test.go
    │   │   ├── usermenu_ini.go
    │   │   ├── usermenu_layout_test.go
    │   │   ├── usermenu_script.go
    │   │   ├── usermenu_script_coverage_test.go
    │   │   ├── usermenu_subst.go
    │   │   ├── usermenu_subst_test.go
    │   │   ├── usermenu_ui.go
    │   │   ├── usermenu_ui_coverage_extra_test.go
    │   │   ├── viewmodes.go
    │   │   ├── viewmodes_columns.go
    │   │   ├── viewmodes_dialog.go
    │   │   ├── viewmodes_dialog_coverage_test.go
    │   │   ├── viewmodes_test.go
    │   │   ├── wheel.go
    │   │   ├── wheel_test.go
    │   │   ├── workspace.go
    │   │   ├── workspace_coverage_batch38_test.go
    │   │   └── workspace_startup_test.go
    │   ├── paneltest
    │   │   ├── doc.go
    │   │   ├── frame.go
    │   │   └── mock_pty.go
    │   ├── pdftext
    │   │   ├── doc.go
    │   │   ├── extract.go
    │   │   ├── images.go
    │   │   ├── parse.go
    │   │   ├── pdftext_test.go
    │   │   └── text.go
    │   ├── piecetable
    │   │   ├── concurrent_test.go
    │   │   ├── lineindex.go
    │   │   ├── lineindex_edgecases_test.go
    │   │   ├── lineindex_equivalence_test.go
    │   │   ├── lineindex_test.go
    │   │   ├── piecetable.go
    │   │   └── piecetable_test.go
    │   ├── plughost
    │   │   ├── application.go
    │   │   ├── contributions.go
    │   │   ├── extui.go
    │   │   ├── extui_coverage_test.go
    │   │   ├── extui_test.go
    │   │   ├── fastpath_regression_test.go
    │   │   ├── ffi.go
    │   │   ├── ffi_test.go
    │   │   ├── host.go
    │   │   ├── host_coverage_test.go
    │   │   ├── identity_test.go
    │   │   ├── incremental_helpers_test.go
    │   │   ├── incremental_test.go
    │   │   ├── lifecycle_coverage_test.go
    │   │   ├── manager.go
    │   │   ├── manager_coverage_test.go
    │   │   ├── manager_lifecycle_test.go
    │   │   ├── manager_names_test.go
    │   │   ├── media_timing.go
    │   │   ├── menu_items.go
    │   │   ├── menu_retention_test.go
    │   │   ├── navigation_trace_test.go
    │   │   ├── panel_providers.go
    │   │   ├── permissions.go
    │   │   ├── permissions_test.go
    │   │   ├── permissions_ui.go
    │   │   ├── permissions_ui_test.go
    │   │   ├── plugin_entrypoints.go
    │   │   ├── plugins_full.go
    │   │   ├── plugins_internal.go
    │   │   ├── plugins_internal_extralite.go
    │   │   ├── plugins_lite.go
    │   │   ├── plugins_observer.go
    │   │   ├── plugins_observer_extralite.go
    │   │   ├── plugring.go
    │   │   ├── plugring_firstparty.go
    │   │   ├── plugring_firstparty_test.go
    │   │   ├── plugring_meta.go
    │   │   ├── plugring_meta_test.go
    │   │   ├── presentation.go
    │   │   ├── presentation_fixture_test.go
    │   │   ├── presentation_integration_test.go
    │   │   ├── qt_embedded_qt_host.go
    │   │   ├── qt_embedded_qt_host_payload.go
    │   │   ├── qt_embedded_qt_host_payload_stub.go
    │   │   ├── qt_embedded_qt_host_payload_test.go
    │   │   ├── qt_embedded_qt_host_test.go
    │   │   ├── qt_extui_directory_preview.go
    │   │   ├── qt_extui_directory_preview_test.go
    │   │   ├── qt_extui_directory_preview_wire.go
    │   │   ├── qt_extui_media.go
    │   │   ├── qt_extui_media_test.go
    │   │   ├── qt_extui_media_windows_test.go
    │   │   ├── qt_platform_ipc.go
    │   │   ├── qt_platform_ipc_test.go
    │   │   ├── rollback_scene_test.go
    │   │   ├── rpc_commands.go
    │   │   ├── rpc_panel.go
    │   │   ├── rpc_panel_coverage_test.go
    │   │   ├── rpc_panel_keys_test.go
    │   │   ├── rpc_vfs.go
    │   │   ├── rpc_vfs_coverage_test.go
    │   │   ├── rpc_vfs_errors.go
    │   │   ├── rpc_vfs_errors_test.go
    │   │   ├── rpc_vfs_test.go
    │   │   ├── scaffold.go
    │   │   ├── scaffold_test.go
    │   │   ├── semantic_helpers_test.go
    │   │   ├── settings_permissions_test.go
    │   │   ├── terminal_redraw_test.go
    │   │   ├── transport_lua.go
    │   │   ├── transport_lua_extralite.go
    │   │   ├── transport_lua_rpc_test.go
    │   │   ├── transport_lua_test.go
    │   │   ├── transport_rpc.go
    │   │   ├── transport_rpc_coverage_test.go
    │   │   ├── transport_rpc_test.go
    │   │   ├── transport_wazero.go
    │   │   ├── transport_wazero_extralite.go
    │   │   ├── transport_wazero_test.go
    │   │   ├── ui_guard.go
    │   │   └── ui_guard_test.go
    │   ├── semantic
    │   │   ├── capabilities.go
    │   │   ├── document_viewport.go
    │   │   ├── fields.go
    │   │   ├── fields_test.go
    │   │   ├── menu_patch.go
    │   │   ├── menu_patch_test.go
    │   │   ├── qt_semantic.go
    │   │   ├── qt_semantic_window_render.go
    │   │   ├── scene.go
    │   │   ├── scene_patch.go
    │   │   ├── time.go
    │   │   ├── values.go
    │   │   ├── workspace.go
    │   │   └── wrapped_rows.go
    │   ├── settings
    │   │   ├── benchmark_test.go
    │   │   ├── catalog.go
    │   │   ├── center.go
    │   │   ├── center_profile_test.go
    │   │   ├── center_semantic.go
    │   │   ├── center_semantic_test.go
    │   │   ├── center_test.go
    │   │   ├── choice_help.go
    │   │   ├── choice_help.tsv
    │   │   ├── choice_help_test.go
    │   │   ├── chord.go
    │   │   ├── chord_coverage_test.go
    │   │   ├── clipboard_image_test.go
    │   │   ├── collection_palette_test.go
    │   │   ├── collection_ui.go
    │   │   ├── config_docs.go
    │   │   ├── config_docs_test.go
    │   │   ├── core.go
    │   │   ├── drive_pages.go
    │   │   ├── drive_pages_test.go
    │   │   ├── drive_tools.go
    │   │   ├── edit.go
    │   │   ├── edit_test.go
    │   │   ├── enter_test.go
    │   │   ├── extra.go
    │   │   ├── grouping_test.go
    │   │   ├── help.go
    │   │   ├── host.go
    │   │   ├── hotkeys_persistence_test.go
    │   │   ├── inventory_test.go
    │   │   ├── issue320_test.go
    │   │   ├── main_test.go
    │   │   ├── manual_save_test.go
    │   │   ├── mouse_test.go
    │   │   ├── operations.go
    │   │   ├── operations_catalog_coverage_batch51_test.go
    │   │   ├── operations_coverage_test.go
    │   │   ├── plugin_catalog.go
    │   │   ├── plugin_providers_coverage_test.go
    │   │   ├── plugins.go
    │   │   ├── providers.go
    │   │   ├── radios.go
    │   │   ├── radios_semantic.go
    │   │   ├── radios_test.go
    │   │   ├── record_dialog.go
    │   │   ├── record_dialog_test.go
    │   │   ├── records.go
    │   │   ├── records_coverage_test.go
    │   │   ├── russian_test.go
    │   │   ├── scoped_test.go
    │   │   ├── search_first_test.go
    │   │   ├── search_layout_test.go
    │   │   ├── startup_test.go
    │   │   ├── terminal_history_help_test.go
    │   │   ├── terminal_history_test.go
    │   │   ├── trace.go
    │   │   ├── trace_test.go
    │   │   ├── transactions_test.go
    │   │   ├── user_menu.go
    │   │   └── user_menu_test.go
    │   ├── settingstest
    │   │   └── russian.go
    │   ├── sheet
    │   │   ├── cell.go
    │   │   ├── coverage_contract_test.go
    │   │   ├── expr.go
    │   │   ├── sheet.go
    │   │   ├── sheet_test.go
    │   │   ├── store.go
    │   │   ├── store_lite.go
    │   │   ├── store_lite_test.go
    │   │   └── xlsx.go
    │   ├── stallwatch
    │   │   ├── stallwatch.go
    │   │   └── stallwatch_test.go
    │   ├── sysinfo
    │   │   ├── cpu.go
    │   │   ├── cpu_darwin.go
    │   │   ├── cpu_linux.go
    │   │   ├── cpu_linux_test.go
    │   │   ├── cpu_other.go
    │   │   ├── cpu_windows.go
    │   │   ├── drive_icons.go
    │   │   ├── drives.go
    │   │   ├── drives_test.go
    │   │   ├── drives_unix.go
    │   │   ├── drives_unix_test.go
    │   │   ├── drives_windows.go
    │   │   ├── fs.go
    │   │   ├── fs_darwin.go
    │   │   ├── fs_linux.go
    │   │   ├── fs_linux_test.go
    │   │   ├── fs_other.go
    │   │   ├── fs_resolve.go
    │   │   ├── fs_resolve_test.go
    │   │   ├── fs_windows.go
    │   │   ├── fs_windows_test.go
    │   │   ├── gpu.go
    │   │   ├── gpu_darwin.go
    │   │   ├── gpu_linux.go
    │   │   ├── gpu_linux_test.go
    │   │   ├── gpu_other.go
    │   │   ├── gpu_windows.go
    │   │   ├── mem.go
    │   │   ├── mem_linux.go
    │   │   ├── mem_linux_test.go
    │   │   ├── mem_other.go
    │   │   ├── mem_windows.go
    │   │   ├── mounts_linux.go
    │   │   ├── mounts_linux_test.go
    │   │   ├── mounts_other.go
    │   │   ├── qt_drives_darwin_test.go
    │   │   └── qt_drives_unix_test.go
    │   ├── tarindexcache
    │   │   ├── tarindexcache.go
    │   │   └── tarindexcache_test.go
    │   ├── terminal
    │   │   ├── ansi.go
    │   │   ├── ansi_coverage_extra_test.go
    │   │   ├── ansi_sync_test.go
    │   │   ├── ansi_test.go
    │   │   ├── application.go
    │   │   ├── attr.go
    │   │   ├── backend.go
    │   │   ├── child_env.go
    │   │   ├── child_env_test.go
    │   │   ├── child_process.go
    │   │   ├── clipboard.go
    │   │   ├── clipboard_async.go
    │   │   ├── clipboard_image.go
    │   │   ├── clipboard_image_test.go
    │   │   ├── clipboard_test.go
    │   │   ├── conpty_package.go
    │   │   ├── conpty_package_test.go
    │   │   ├── console_buffer_other.go
    │   │   ├── console_buffer_windows.go
    │   │   ├── console_cmdline.go
    │   │   ├── console_cmdline_test.go
    │   │   ├── console_host_windows.go
    │   │   ├── console_overlay_other.go
    │   │   ├── console_overlay_windows.go
    │   │   ├── console_overlay_windows_test.go
    │   │   ├── console_palette_windows.go
    │   │   ├── console_palette_windows_test.go
    │   │   ├── console_scroll.go
    │   │   ├── console_scroll_other.go
    │   │   ├── console_scroll_test.go
    │   │   ├── console_scroll_windows.go
    │   │   ├── console_spawn_other.go
    │   │   ├── console_spawn_windows.go
    │   │   ├── far2l_auth.go
    │   │   ├── far2l_auth_test.go
    │   │   ├── far2l_dnd.go
    │   │   ├── far2l_dnd_bridge.go
    │   │   ├── far2l_dnd_client.go
    │   │   ├── far2l_dnd_client_test.go
    │   │   ├── far2l_dnd_proxy.go
    │   │   ├── far2l_dnd_test.go
    │   │   ├── far2l_image.go
    │   │   ├── far2l_image_test.go
    │   │   ├── far2ldnd
    │   │   │   ├── far2ldnd_test.go
    │   │   │   ├── frame.go
    │   │   │   ├── messages.go
    │   │   │   └── stack.go
    │   │   ├── file_clipboard.go
    │   │   ├── file_clipboard_unix.go
    │   │   ├── file_clipboard_windows.go
    │   │   ├── graphics_compat.go
    │   │   ├── graphics_compat_test.go
    │   │   ├── graphics_probe.go
    │   │   ├── graphics_probe_test.go
    │   │   ├── graphics_probe_windows.go
    │   │   ├── image_clipboard.go
    │   │   ├── image_clipboard_darwin.go
    │   │   ├── image_clipboard_unix.go
    │   │   ├── image_clipboard_windows.go
    │   │   ├── iterm2.go
    │   │   ├── iterm2_test.go
    │   │   ├── jobs.go
    │   │   ├── jobs_test.go
    │   │   ├── kitty.go
    │   │   ├── kitty_coverage_test.go
    │   │   ├── kitty_diacritics.go
    │   │   ├── kitty_metrics_test.go
    │   │   ├── kitty_placeholder.go
    │   │   ├── kitty_placeholder_test.go
    │   │   ├── kitty_placements.go
    │   │   ├── kitty_placements_test.go
    │   │   ├── kitty_test.go
    │   │   ├── local_command_capture.go
    │   │   ├── local_command_capture_test.go
    │   │   ├── local_command_inline_other.go
    │   │   ├── local_command_inline_windows.go
    │   │   ├── log_console_other.go
    │   │   ├── log_console_windows.go
    │   │   ├── log_vfs.go
    │   │   ├── log_vfs_coverage_test.go
    │   │   ├── log_vfs_test.go
    │   │   ├── main_test.go
    │   │   ├── managed_exec.go
    │   │   ├── managed_exec_test.go
    │   │   ├── native_command_other.go
    │   │   ├── native_command_windows.go
    │   │   ├── overlay.go
    │   │   ├── pe_subsystem.go
    │   │   ├── pe_subsystem_test.go
    │   │   ├── portable_pty_windows_test.go
    │   │   ├── process_environment.go
    │   │   ├── process_environment_runtime_unix.go
    │   │   ├── process_environment_runtime_windows.go
    │   │   ├── process_environment_test.go
    │   │   ├── pty.go
    │   │   ├── pty_bsd.go
    │   │   ├── pty_bsd_dragonfly.go
    │   │   ├── pty_bsd_freebsd.go
    │   │   ├── pty_bsd_test.go
    │   │   ├── pty_cloexec_test.go
    │   │   ├── pty_darwin.go
    │   │   ├── pty_diag_unix.go
    │   │   ├── pty_diag_unix_test.go
    │   │   ├── pty_diag_windows.go
    │   │   ├── pty_linux.go
    │   │   ├── pty_logical_lines.go
    │   │   ├── pty_logical_lines_solaris.go
    │   │   ├── pty_logical_lines_unix.go
    │   │   ├── pty_pollable_test.go
    │   │   ├── pty_ptm.go
    │   │   ├── pty_ptm_netbsd.go
    │   │   ├── pty_ptm_openbsd.go
    │   │   ├── pty_solaris.go
    │   │   ├── pty_test.go
    │   │   ├── pty_windows.go
    │   │   ├── pty_windows_test.go
    │   │   ├── pty_wine_other.go
    │   │   ├── pty_wine_shell.go
    │   │   ├── pty_wine_shell_test.go
    │   │   ├── pty_wine_windows.go
    │   │   ├── qt_pty_windows_async_test.go
    │   │   ├── qt_terminal_semantic.go
    │   │   ├── qt_terminal_semantic_live_test.go
    │   │   ├── qt_terminal_semantic_window_test.go
    │   │   ├── redraw.go
    │   │   ├── redraw_stop_test.go
    │   │   ├── runner.go
    │   │   ├── runner_test.go
    │   │   ├── runner_unix.go
    │   │   ├── runner_unix_test.go
    │   │   ├── runner_windows.go
    │   │   ├── runner_windows_test.go
    │   │   ├── selection.go
    │   │   ├── selection_test.go
    │   │   ├── server_diagnostics.go
    │   │   ├── session_attach_identity_test.go
    │   │   ├── session_attach_payload_test.go
    │   │   ├── session_daemon_test.go
    │   │   ├── session_unix.go
    │   │   ├── session_unix_coverage_extra_test.go
    │   │   ├── session_unix_coverage_test.go
    │   │   ├── session_unix_test.go
    │   │   ├── session_windows.go
    │   │   ├── shell_history.go
    │   │   ├── shell_history_test.go
    │   │   ├── shell_start_darwin.go
    │   │   ├── shell_start_other.go
    │   │   ├── shellmode.go
    │   │   ├── shellmode_test.go
    │   │   ├── sixel.go
    │   │   ├── sixel_layers_test.go
    │   │   ├── sixel_terminal.go
    │   │   ├── sixel_terminal_test.go
    │   │   ├── sixel_test.go
    │   │   ├── solaris_pty.go
    │   │   ├── solaris_pty_alloc_test.go
    │   │   ├── solaris_pty_backend_test.go
    │   │   ├── solaris_streams.go
    │   │   ├── solaris_streams_mock_linux_test.go
    │   │   ├── solaris_streams_mock_other_test.go
    │   │   ├── solaris_streams_mock_test.go
    │   │   ├── solaris_streams_test.go
    │   │   ├── ttyx_probe.go
    │   │   ├── ttyx_probe_coverage_unix_test.go
    │   │   ├── ttyx_probe_parse.go
    │   │   ├── ttyx_probe_test.go
    │   │   ├── ttyx_probe_unix.go
    │   │   ├── ttyx_probe_windows.go
    │   │   ├── ttyx_session.go
    │   │   ├── view.go
    │   │   ├── view_coverage_extra_test.go
    │   │   ├── view_defaultcolors.go
    │   │   ├── view_defaultcolors_test.go
    │   │   ├── view_history.go
    │   │   ├── view_history_test.go
    │   │   ├── view_reflow.go
    │   │   ├── view_reflow_test.go
    │   │   ├── view_semantic_test.go
    │   │   ├── view_test.go
    │   │   ├── window_helpers_test.go
    │   │   ├── wineprobe.go
    │   │   ├── wineprobe_escape_other.go
    │   │   ├── wineprobe_escape_windows.go
    │   │   ├── wineprobe_other.go
    │   │   ├── wineprobe_test.go
    │   │   ├── wineprobe_windows.go
    │   │   └── zzz_pty_leak_check_test.go
    │   ├── testutil
    │   │   ├── dialog.go
    │   │   ├── doc.go
    │   │   ├── frame.go
    │   │   ├── frame_test.go
    │   │   ├── input.go
    │   │   ├── main.go
    │   │   ├── numeric.go
    │   │   ├── paths.go
    │   │   ├── paths_coverage_test.go
    │   │   ├── race_disabled.go
    │   │   ├── race_enabled.go
    │   │   ├── rpc.go
    │   │   └── subprocess.go
    │   ├── textdiff
    │   │   ├── textdiff.go
    │   │   └── textdiff_test.go
    │   ├── textlayout
    │   │   ├── cluster.go
    │   │   ├── mapping.go
    │   │   ├── mapping_test.go
    │   │   ├── wrap.go
    │   │   ├── wrap_incremental_test.go
    │   │   ├── wrap_projection_test.go
    │   │   └── wrap_test.go
    │   ├── textsearch
    │   │   ├── search.go
    │   │   └── search_test.go
    │   ├── theme
    │   │   ├── color_validate.go
    │   │   ├── color_validate_test.go
    │   │   ├── colors.go
    │   │   ├── colors_background_inheritance_test.go
    │   │   ├── colors_background_test.go
    │   │   ├── colors_cursor_test.go
    │   │   ├── colors_test.go
    │   │   ├── colorspace.go
    │   │   ├── colorspace_test.go
    │   │   ├── custom_refresh.go
    │   │   ├── custom_refresh_groups_test.go
    │   │   ├── custom_refresh_test.go
    │   │   ├── farcolor.go
    │   │   ├── farcolor_test.go
    │   │   ├── file_icon.go
    │   │   ├── highlight.go
    │   │   ├── highlight_cache_test.go
    │   │   ├── highlight_edgecases_test.go
    │   │   ├── highlight_files_test.go
    │   │   ├── highlight_hidden_dirs_test.go
    │   │   ├── highlight_usedefaults_test.go
    │   │   ├── style.go
    │   │   ├── style_combo_colors_test.go
    │   │   ├── style_completeness_test.go
    │   │   ├── style_custom_test.go
    │   │   ├── style_default_dark_test.go
    │   │   ├── style_indicator_test.go
    │   │   ├── style_missing_section_test.go
    │   │   ├── style_overrides_test.go
    │   │   ├── style_test.go
    │   │   ├── styles
    │   │   │   ├── classic.ini
    │   │   │   ├── default_dark.ini
    │   │   │   ├── mc_dark.ini
    │   │   │   ├── mc_default.ini
    │   │   │   ├── modern.ini
    │   │   │   ├── radiola.ini
    │   │   │   └── radiola.md
    │   │   └── table.go
    │   ├── toast
    │   │   └── toast.go
    │   ├── ttyx
    │   │   ├── coverage_edges_test.go
    │   │   ├── coverage_no_display_test.go
    │   │   ├── coverage_reject_test.go
    │   │   ├── coverage_state_test.go
    │   │   ├── keys.go
    │   │   ├── keys_coverage_test.go
    │   │   ├── openfor_test.go
    │   │   ├── overlay.go
    │   │   ├── overlay_coverage_extra_test.go
    │   │   ├── overlay_lifecycle_test.go
    │   │   ├── session.go
    │   │   ├── session_state_test.go
    │   │   ├── state_coverage_test.go
    │   │   ├── ttyx_events_test.go
    │   │   ├── ttyx_test.go
    │   │   ├── watch.go
    │   │   └── watch_coverage_test.go
    │   ├── unpack
    │   │   ├── coverage_test.go
    │   │   ├── formats.go
    │   │   ├── formats_lite.go
    │   │   ├── formats_lite_test.go
    │   │   ├── times.go
    │   │   ├── times_other.go
    │   │   ├── times_test.go
    │   │   ├── times_windows.go
    │   │   ├── unpack.go
    │   │   ├── unpack_errors_test.go
    │   │   ├── unpack_sevenzip_test.go
    │   │   └── unpack_test.go
    │   ├── update
    │   │   ├── assets_test.go
    │   │   ├── backup.go
    │   │   ├── backup_test.go
    │   │   ├── channel_switch_test.go
    │   │   ├── cli.go
    │   │   ├── cli_test.go
    │   │   ├── coverage_test.go
    │   │   ├── edition.go
    │   │   ├── edition_lite.go
    │   │   ├── elevation_manual_windows_test.go
    │   │   ├── elevation_other.go
    │   │   ├── elevation_windows.go
    │   │   ├── helper_args.go
    │   │   ├── libc_default.go
    │   │   ├── libc_default_test.go
    │   │   ├── libc_musl.go
    │   │   ├── libc_musl_test.go
    │   │   ├── release_audit.go
    │   │   ├── release_audit_test.go
    │   │   ├── release_os.go
    │   │   ├── release_os_default.go
    │   │   ├── release_os_win7.go
    │   │   ├── selfexec.go
    │   │   ├── selfexec_linux.go
    │   │   ├── selfexec_linux_test.go
    │   │   ├── selfexec_other.go
    │   │   ├── selfexec_termux.go
    │   │   ├── selfexec_test.go
    │   │   ├── startcheck.go
    │   │   ├── startcheck_other.go
    │   │   ├── startcheck_test.go
    │   │   ├── startcheck_windows.go
    │   │   ├── update.go
    │   │   └── update_test.go
    │   ├── viewer
    │   │   ├── ansi.go
    │   │   ├── ansi_test.go
    │   │   ├── application.go
    │   │   ├── backend.go
    │   │   ├── backend_test.go
    │   │   ├── binary.go
    │   │   ├── colorizer.go
    │   │   ├── content_revision_test.go
    │   │   ├── disasm.go
    │   │   ├── disasm_test.go
    │   │   ├── document_reader_test.go
    │   │   ├── events.go
    │   │   ├── events_test.go
    │   │   ├── highlight.go
    │   │   ├── highlight_test.go
    │   │   ├── links.go
    │   │   ├── links_test.go
    │   │   ├── main_test.go
    │   │   ├── menubar_test.go
    │   │   ├── mode.go
    │   │   ├── protocol_helpers_test.go
    │   │   ├── qt_document_viewer_loading_test.go
    │   │   ├── qt_document_viewer_projection.go
    │   │   ├── qt_semantic.go
    │   │   ├── qt_semantic_window_render.go
    │   │   ├── qt_viewer_document_projection_test.go
    │   │   ├── qt_viewer_end_navigation.go
    │   │   ├── qt_viewer_end_tail_test.go
    │   │   ├── qt_viewer_navigation_intent_test.go
    │   │   ├── range_loading_test.go
    │   │   ├── reader_profile_test.go
    │   │   ├── search.go
    │   │   ├── search_coverage_test.go
    │   │   ├── search_reveal.go
    │   │   ├── search_reveal_test.go
    │   │   ├── semantic_model_test.go
    │   │   ├── tail_test.go
    │   │   ├── task_helpers_test.go
    │   │   ├── text.go
    │   │   ├── text_test.go
    │   │   ├── title.go
    │   │   ├── topbar.go
    │   │   ├── topbar_test.go
    │   │   ├── unwrapped_seek_test.go
    │   │   ├── view.go
    │   │   ├── view_coverage_extra_test.go
    │   │   ├── view_semantic.go
    │   │   ├── view_semantic_contract_test.go
    │   │   ├── view_test.go
    │   │   ├── viewport.go
    │   │   ├── window_protocol_test.go
    │   │   ├── window_render_test.go
    │   │   ├── wordnav.go
    │   │   └── wordnav_test.go
    │   ├── vtvibe
    │   │   ├── agent.go
    │   │   ├── agent_test.go
    │   │   ├── agent_tools.go
    │   │   ├── ap
    │   │   │   ├── apply.go
    │   │   │   ├── cases_test.go
    │   │   │   ├── crlf_test.go
    │   │   │   ├── dryrun_test.go
    │   │   │   ├── errors.go
    │   │   │   ├── fsutil.go
    │   │   │   ├── legacy_cases_test.go
    │   │   │   ├── modify.go
    │   │   │   ├── modresult_test.go
    │   │   │   ├── only_test.go
    │   │   │   ├── parse.go
    │   │   │   ├── preview.go
    │   │   │   ├── preview_test.go
    │   │   │   ├── report.go
    │   │   │   ├── run_tests_test.go
    │   │   │   ├── search.go
    │   │   │   ├── structure.go
    │   │   │   ├── testdata
    │   │   │   │   └── reftests
    │   │   │   │       ├── expected
    │   │   │   │       │   ├── 01_basic.cpp
    │   │   │   │       │   ├── 02_sequences.py
    │   │   │   │       │   ├── 03_tabs.py
    │   │   │   │       │   ├── 04_spaces.py
    │   │   │   │       │   ├── 05_crlf.txt
    │   │   │   │       │   ├── 06_short_anchor.py
    │   │   │   │       │   ├── 07_empty_lines.py
    │   │   │   │       │   ├── 14_edge_cases.py
    │   │   │   │       │   ├── 15_robustness.js
    │   │   │   │       │   ├── 18_idempotency.py
    │   │   │   │       │   ├── 19_idempotency_noop.py
    │   │   │   │       │   ├── 22_range_replace.py
    │   │   │   │       │   ├── 24_heuristics.py
    │   │   │   │       │   ├── 25_calculator.py
    │   │   │   │       │   ├── 26_implicit_create_file.txt
    │   │   │   │       │   ├── 27_anchor_resolution.py
    │   │   │   │       │   ├── 28_mixed_locators.py
    │   │   │   │       │   ├── 29_anchor_overlap.py
    │   │   │   │       │   ├── 30_locality_heuristic.py
    │   │   │   │       │   ├── 31_redundant_snippet.py
    │   │   │   │       │   ├── 32_intersection_resolution.py
    │   │   │   │       │   ├── 33_snippet_locality.py
    │   │   │   │       │   ├── 34_range_priority_strict.py
    │   │   │   │       │   ├── 37_heuristic_end_eq_content.py
    │   │   │   │       │   ├── 38_deep_scope.py
    │   │   │   │       │   ├── 39_sequential_repeats.py
    │   │   │   │       │   ├── 40_unified_snippet.py
    │   │   │   │       │   ├── 41_indent_trailing_newline.py
    │   │   │   │       │   ├── 43_created.txt
    │   │   │   │       │   ├── 49_sequential_cursor.py
    │   │   │   │       │   ├── 50_identical_snippet_tail.py
    │   │   │   │       │   ├── 52_atomic_src1.txt
    │   │   │   │       │   ├── 54_crlf.txt
    │   │   │   │       │   ├── 56_insert_noop.txt
    │   │   │   │       │   ├── 57_explicit_lf.txt
    │   │   │   │       │   ├── 58_explicit_cr.txt
    │   │   │   │       │   ├── 60_idempotent_create.txt
    │   │   │   │       │   ├── 67_created.txt
    │   │   │   │       │   ├── 68_source.py
    │   │   │   │       │   ├── 70_source.txt
    │   │   │   │       │   ├── 71_source.txt
    │   │   │   │       │   ├── 72_source.txt
    │   │   │   │       │   ├── 73_source.txt
    │   │   │   │       │   ├── 74_source.txt
    │   │   │   │       │   ├── 75_source.py
    │   │   │   │       │   └── new
    │   │   │   │       │       └── created_file.txt
    │   │   │   │       ├── patches
    │   │   │   │       │   ├── 01_basic_replace.ap
    │   │   │   │       │   ├── 02_sequences.ap
    │   │   │   │       │   ├── 03_tabs.ap
    │   │   │   │       │   ├── 04_spaces.ap
    │   │   │   │       │   ├── 05_crlf.ap
    │   │   │   │       │   ├── 06_short_anchor.ap
    │   │   │   │       │   ├── 07_empty_lines.ap
    │   │   │   │       │   ├── 08_error_snippet_not_found.ap
    │   │   │   │       │   ├── 09_error_anchor_not_found.ap
    │   │   │   │       │   ├── 10_error_ambiguous.ap
    │   │   │   │       │   ├── 11_error_invalid_header.ap
    │   │   │   │       │   ├── 12_error_invalid_spec.ap
    │   │   │   │       │   ├── 13_create_file.ap
    │   │   │   │       │   ├── 14_edge_cases.ap
    │   │   │   │       │   ├── 15_robustness.ap
    │   │   │   │       │   ├── 18_idempotency.ap
    │   │   │   │       │   ├── 19_idempotency_noop.ap
    │   │   │   │       │   ├── 21_error_atomic_failure.ap
    │   │   │   │       │   ├── 22_range_replace.ap
    │   │   │   │       │   ├── 23_error_range_ambiguous.ap
    │   │   │   │       │   ├── 24_heuristics.ap
    │   │   │   │       │   ├── 25_calculator_example.ap
    │   │   │   │       │   ├── 26_implicit_create_file.ap
    │   │   │   │       │   ├── 27_anchor_resolution.ap
    │   │   │   │       │   ├── 28_mixed_locators.ap
    │   │   │   │       │   ├── 29_anchor_overlap.ap
    │   │   │   │       │   ├── 30_locality_heuristic.ap
    │   │   │   │       │   ├── 31_redundant_snippet.ap
    │   │   │   │       │   ├── 32_intersection_resolution.ap
    │   │   │   │       │   ├── 33_snippet_locality.ap
    │   │   │   │       │   ├── 34_range_priority_strict.ap
    │   │   │   │       │   ├── 37_heuristic_end_eq_content.ap
    │   │   │   │       │   ├── 38_deep_scope.ap
    │   │   │   │       │   ├── 39_sequential_repeats.ap
    │   │   │   │       │   ├── 40_unified_snippet.ap
    │   │   │   │       │   ├── 41_indent_trailing_newline.ap
    │   │   │   │       │   ├── 42_strict_cursor.ap
    │   │   │   │       │   ├── 43_heuristic_implicit_create.ap
    │   │   │   │       │   ├── 49_sequential_cursor.ap
    │   │   │   │       │   ├── 50_identical_snippet_tail.ap
    │   │   │   │       │   ├── 52_force_success_part.ap
    │   │   │   │       │   ├── 53_force_fail_report.ap
    │   │   │   │       │   ├── 54_crlf_preservation.ap
    │   │   │   │       │   ├── 56_insert_noop.ap
    │   │   │   │       │   ├── 57_explicit_lf.ap
    │   │   │   │       │   ├── 58_explicit_cr.ap
    │   │   │   │       │   ├── 60_idempotent_create.ap
    │   │   │   │       │   ├── 67_actions_after_create.ap
    │   │   │   │       │   ├── 68_idempotency_cursor_desync.ap
    │   │   │   │       │   ├── 69_delete_snippet_tail_not_found.ap
    │   │   │   │       │   ├── 70_recreate_file.ap
    │   │   │   │       │   ├── 71_recreate_idempotency.ap
    │   │   │   │       │   ├── 72_boundary_anchors_range.ap
    │   │   │   │       │   ├── 73_boundary_anchors_single.ap
    │   │   │   │       │   ├── 74_robust_overlap.ap
    │   │   │   │       │   └── 75_multipass_retry.ap
    │   │   │   │       └── src
    │   │   │   │           ├── 01_basic.cpp
    │   │   │   │           ├── 02_sequences.py
    │   │   │   │           ├── 03_tabs.py
    │   │   │   │           ├── 04_spaces.py
    │   │   │   │           ├── 05_crlf.txt
    │   │   │   │           ├── 06_short_anchor.py
    │   │   │   │           ├── 07_empty_lines.py
    │   │   │   │           ├── 08_error_src.py
    │   │   │   │           ├── 09_error_src.py
    │   │   │   │           ├── 10_error_src.py
    │   │   │   │           ├── 14_edge_cases.py
    │   │   │   │           ├── 15_robustness.js
    │   │   │   │           ├── 18_idempotency.py
    │   │   │   │           ├── 19_idempotency_noop.py
    │   │   │   │           ├── 21_atomic_src1.txt
    │   │   │   │           ├── 21_atomic_src2.txt
    │   │   │   │           ├── 22_range_replace.py
    │   │   │   │           ├── 23_error_range_ambiguous.py
    │   │   │   │           ├── 24_heuristics.py
    │   │   │   │           ├── 25_calculator.py
    │   │   │   │           ├── 27_anchor_resolution.py
    │   │   │   │           ├── 28_mixed_locators.py
    │   │   │   │           ├── 29_anchor_overlap.py
    │   │   │   │           ├── 30_locality_heuristic.py
    │   │   │   │           ├── 31_redundant_snippet.py
    │   │   │   │           ├── 32_intersection_resolution.py
    │   │   │   │           ├── 33_snippet_locality.py
    │   │   │   │           ├── 34_range_priority_strict.py
    │   │   │   │           ├── 37_heuristic_end_eq_content.py
    │   │   │   │           ├── 38_deep_scope.py
    │   │   │   │           ├── 39_sequential_repeats.py
    │   │   │   │           ├── 40_unified_snippet.py
    │   │   │   │           ├── 41_indent_trailing_newline.py
    │   │   │   │           ├── 42_strict_cursor.py
    │   │   │   │           ├── 49_sequential_cursor.py
    │   │   │   │           ├── 50_identical_snippet_tail.py
    │   │   │   │           ├── 52_atomic_src1.txt
    │   │   │   │           ├── 52_atomic_src2.txt
    │   │   │   │           ├── 54_crlf.txt
    │   │   │   │           ├── 56_insert_noop.txt
    │   │   │   │           ├── 57_explicit_lf.txt
    │   │   │   │           ├── 58_explicit_cr.txt
    │   │   │   │           ├── 60_idempotent_create.txt
    │   │   │   │           ├── 68_source.py
    │   │   │   │           ├── 69_source.py
    │   │   │   │           ├── 70_source.txt
    │   │   │   │           ├── 71_source.txt
    │   │   │   │           ├── 72_source.txt
    │   │   │   │           ├── 73_source.txt
    │   │   │   │           ├── 74_source.txt
    │   │   │   │           ├── 75_source.py
    │   │   │   │           └── dummy.txt
    │   │   │   ├── types.go
    │   │   │   ├── undo.go
    │   │   │   ├── undo_disk.go
    │   │   │   ├── undo_test.go
    │   │   │   └── util.go
    │   │   ├── ap.go
    │   │   ├── ap_test.go
    │   │   ├── coverage_test.go
    │   │   ├── defaults.go
    │   │   ├── draft.go
    │   │   ├── draft_test.go
    │   │   ├── memtree.go
    │   │   ├── pack.go
    │   │   ├── pack_test.go
    │   │   ├── provider.go
    │   │   ├── provider_test.go
    │   │   ├── providers.go
    │   │   ├── providers_test.go
    │   │   ├── session.go
    │   │   ├── session_test.go
    │   │   ├── vfs.go
    │   │   └── vfs_coverage_test.go
    │   ├── wheel
    │   │   ├── wheel.go
    │   │   └── wheel_test.go
    │   ├── wincon
    │   │   ├── blit_test.go
    │   │   ├── coverage_test.go
    │   │   ├── geometry.go
    │   │   ├── layered.go
    │   │   ├── layered_test.go
    │   │   ├── overlay_other.go
    │   │   ├── overlay_state.go
    │   │   ├── overlay_state_test.go
    │   │   ├── overlay_windows.go
    │   │   ├── overlay_windows_test.go
    │   │   ├── stats.go
    │   │   ├── stats_windows.go
    │   │   ├── wincon_test.go
    │   │   ├── windowlongptr_32.go
    │   │   └── windowlongptr_64.go
    │   ├── wincondrag
    │   │   ├── backend_other.go
    │   │   ├── backend_windows.go
    │   │   ├── doc.go
    │   │   ├── logic.go
    │   │   └── logic_test.go
    │   ├── winex11drag
    │   │   ├── display.go
    │   │   ├── display_test.go
    │   │   ├── doc.go
    │   │   ├── wineconn.go
    │   │   ├── wineconn_test.go
    │   │   ├── x11window.go
    │   │   ├── x11window_test.go
    │   │   ├── x11window_windows.go
    │   │   ├── xauthority.go
    │   │   └── xauthority_test.go
    │   └── winshell
    │       ├── broker_windows.go
    │       ├── client_stub.go
    │       ├── client_windows.go
    │       ├── client_windows_integration_test.go
    │       ├── context_windows.go
    │       ├── icon_windows.go
    │       ├── native_windows.go
    │       ├── native_windows_test.go
    │       ├── register_stub.go
    │       ├── register_windows.go
    │       ├── sta_windows.go
    │       ├── types.go
    │       ├── uri.go
    │       ├── uri_test.go
    │       ├── vfs.go
    │       └── vfs_test.go
    ├── packaging
    │   ├── linux
    │   │   └── org.unxed.f4.desktop
    │   ├── macos
    │   │   └── Info.plist
    │   ├── nix
    │   │   ├── f4-ini-upsert.sh
    │   │   ├── home-manager-module-check.nix
    │   │   ├── home-manager-module.nix
    │   │   └── ini.nix
    │   ├── openwrt
    │   │   └── f4
    │   │       └── Makefile
    │   └── termux
    │       └── build.sh
    ├── plugins
    │   ├── android
    │   │   ├── README.md
    │   │   ├── adb_integration_test.go
    │   │   ├── adb_sync.go
    │   │   ├── adb_sync_test.go
    │   │   ├── adb_transport.go
    │   │   ├── adb_transport_test.go
    │   │   ├── adb_wire_test.go
    │   │   ├── cmd
    │   │   │   └── android-plugin
    │   │   │       ├── main.go
    │   │   │       └── plugring-manifest.json
    │   │   ├── command_runner_info_test.go
    │   │   ├── coverage_contract_test.go
    │   │   ├── coverage_helpers_test.go
    │   │   ├── device.go
    │   │   ├── device_test.go
    │   │   ├── fish_pool.go
    │   │   ├── fish_pool_test.go
    │   │   ├── go.mod
    │   │   ├── info.go
    │   │   ├── info_paths_test.go
    │   │   ├── info_test.go
    │   │   ├── manager.go
    │   │   ├── manager_test.go
    │   │   ├── pathutil.go
    │   │   ├── pathutil_test.go
    │   │   ├── rpc_plugin.go
    │   │   ├── sync_vfs.go
    │   │   ├── sync_vfs_paths_test.go
    │   │   ├── sync_vfs_test.go
    │   │   ├── uri.go
    │   │   └── uri_test.go
    │   ├── archive
    │   │   ├── archive.go
    │   │   ├── archive_materialize_unix_test.go
    │   │   ├── archive_plugin_test.go
    │   │   ├── archive_test.go
    │   │   ├── archive_write_regression_test.go
    │   │   ├── clone_test.go
    │   │   ├── compressed_regular_test.go
    │   │   ├── doc.go
    │   │   ├── enabled_test.go
    │   │   ├── extraction_paths_test.go
    │   │   ├── extraction_security_test.go
    │   │   ├── gzip_view.go
    │   │   ├── gzip_view_test.go
    │   │   ├── issue1179_sfx_test.go
    │   │   ├── issue1179_volumes_test.go
    │   │   ├── issue1184_test.go
    │   │   ├── issue1186_sfx_zip_test.go
    │   │   ├── issue1186_split_zip_test.go
    │   │   ├── issue1186_winrar_aes_test.go
    │   │   ├── issue1187_test.go
    │   │   ├── issue1243_test.go
    │   │   ├── issue1250_marked_test.go
    │   │   ├── issue1250_prompt_test.go
    │   │   ├── issue1250_rar_test.go
    │   │   ├── issue1250_test.go
    │   │   ├── issue1272_test.go
    │   │   ├── issue1383_test.go
    │   │   ├── issue1504_test.go
    │   │   ├── issue815_f3_test.go
    │   │   ├── issue816_multivolume_test.go
    │   │   ├── issue816_password_retry_test.go
    │   │   ├── issue915_total_progress_test.go
    │   │   ├── materialize.go
    │   │   ├── materialize_coverage_test.go
    │   │   ├── multivolume_rar.go
    │   │   ├── nested_detection_test.go
    │   │   ├── nested_matrix_test.go
    │   │   ├── password.go
    │   │   ├── password_coverage_test.go
    │   │   ├── password_test.go
    │   │   ├── production_regression_test.go
    │   │   ├── provider.go
    │   │   ├── provider_special_unix_test.go
    │   │   ├── provider_test.go
    │   │   ├── rar_password.go
    │   │   ├── ratarmount.go
    │   │   ├── ratarmount_test.go
    │   │   ├── remove_directory_test.go
    │   │   ├── repro_test.go
    │   │   ├── seq_reader.go
    │   │   ├── seq_reader_test.go
    │   │   ├── sfx.go
    │   │   ├── sfx_test.go
    │   │   ├── stream.go
    │   │   ├── stream_test.go
    │   │   ├── tar_index.go
    │   │   ├── tar_index_cache_option_test.go
    │   │   ├── testdata
    │   │   │   └── issue1186_winrar_aes_sfx.exe
    │   │   ├── title.go
    │   │   ├── vfs.go
    │   │   ├── vfs_coverage_batch44_test.go
    │   │   ├── vfs_coverage_extra_test.go
    │   │   ├── vfs_nested_test.go
    │   │   ├── vfs_test.go
    │   │   ├── zip_encoding.go
    │   │   ├── zip_encoding_test.go
    │   │   └── zipcrypto_checkbyte_test.go
    │   ├── chroma
    │   │   ├── chroma.go
    │   │   └── chroma_test.go
    │   ├── cloudfox
    │   │   ├── cloud_vfs.go
    │   │   ├── cloud_vfs_coverage_test.go
    │   │   ├── cloud_vfs_share_test.go
    │   │   ├── cloud_vfs_test.go
    │   │   ├── cmd
    │   │   │   └── cloudfox-plugin
    │   │   │       ├── main.go
    │   │   │       └── plugring-manifest.json
    │   │   ├── connection_action_test.go
    │   │   ├── context_editor_test.go
    │   │   ├── credential_scope.go
    │   │   ├── credential_scope_test.go
    │   │   ├── dialog.go
    │   │   ├── dialog_coverage_extra_test.go
    │   │   ├── dialog_google_test.go
    │   │   ├── dialog_s3_test.go
    │   │   ├── go.mod
    │   │   ├── manager.go
    │   │   ├── manager_coverage_extra_test.go
    │   │   ├── oauth.go
    │   │   ├── password_prompt.go
    │   │   ├── password_prompt_test.go
    │   │   ├── plugin.go
    │   │   ├── plugin_contributions_test.go
    │   │   ├── plugin_test.go
    │   │   ├── provider_capabilities_test.go
    │   │   ├── provider_google.go
    │   │   ├── provider_google_production_test.go
    │   │   ├── provider_google_real_native_integration_test.go
    │   │   ├── provider_google_share.go
    │   │   ├── provider_google_share_test.go
    │   │   ├── provider_google_test.go
    │   │   ├── provider_helpers.go
    │   │   ├── provider_helpers_test.go
    │   │   ├── provider_mutation_test.go
    │   │   ├── provider_real_diagnostics_test.go
    │   │   ├── provider_real_saved_integration_test.go
    │   │   ├── provider_real_semantics_test.go
    │   │   ├── provider_real_sharing_integration_test.go
    │   │   ├── provider_real_upload_cancellation_test.go
    │   │   ├── provider_s3.go
    │   │   ├── provider_s3_discovery_core_test.go
    │   │   ├── provider_s3_discovery_regression_test.go
    │   │   ├── provider_s3_real_discovery_test.go
    │   │   ├── provider_s3_share.go
    │   │   ├── provider_s3_share_test.go
    │   │   ├── provider_s3_test.go
    │   │   ├── provider_webdav.go
    │   │   ├── provider_webdav_edge_test.go
    │   │   ├── provider_webdav_integration_test.go
    │   │   ├── provider_webdav_share.go
    │   │   ├── provider_webdav_share_test.go
    │   │   ├── provider_webdav_test.go
    │   │   ├── provider_yandex.go
    │   │   ├── provider_yandex_cache.go
    │   │   ├── provider_yandex_info.go
    │   │   ├── provider_yandex_production_test.go
    │   │   ├── provider_yandex_share.go
    │   │   ├── provider_yandex_share_test.go
    │   │   ├── provider_yandex_test.go
    │   │   ├── rpc_plugin.go
    │   │   ├── secrets.go
    │   │   ├── secrets_test.go
    │   │   ├── session.go
    │   │   ├── settings_center.go
    │   │   ├── settings_center_draft_test.go
    │   │   ├── settings_center_test.go
    │   │   ├── settings_russian_test.go
    │   │   ├── store.go
    │   │   ├── store_lock_unix.go
    │   │   ├── store_lock_windows.go
    │   │   ├── store_test.go
    │   │   ├── test_main_test.go
    │   │   ├── types.go
    │   │   ├── uri.go
    │   │   ├── uri_test.go
    │   │   ├── vault.go
    │   │   ├── yandex_code_prompt.go
    │   │   └── yandex_code_prompt_coverage_test.go
    │   ├── dockerfs
    │   │   ├── client.go
    │   │   ├── contexts.go
    │   │   ├── contexts_test.go
    │   │   ├── dockerfs_test.go
    │   │   ├── endpoint.go
    │   │   ├── endpoint_test.go
    │   │   ├── locale.go
    │   │   ├── pipe.go
    │   │   ├── pipe_windows_test.go
    │   │   ├── plugin.go
    │   │   ├── uri.go
    │   │   └── vfs.go
    │   ├── dotnet
    │   │   ├── dotnet_test.go
    │   │   ├── plugin.go
    │   │   ├── provider.go
    │   │   └── vfs.go
    │   ├── dummy_internal
    │   │   ├── dummy_internal.go
    │   │   └── dummy_internal_test.go
    │   ├── dummy_lua
    │   │   ├── README.md
    │   │   └── plugin.lua
    │   ├── dummy_rpc
    │   │   ├── main.go
    │   │   ├── main_test.go
    │   │   └── panel_keys_test.go
    │   ├── envman
    │   │   ├── README.md
    │   │   ├── codec.go
    │   │   ├── codec_test.go
    │   │   ├── commands.go
    │   │   ├── commands_test.go
    │   │   ├── dialogs.go
    │   │   ├── environment_document.go
    │   │   ├── far3_import.go
    │   │   ├── far3_import_other.go
    │   │   ├── far3_import_test.go
    │   │   ├── far3_import_ui.go
    │   │   ├── far3_import_windows.go
    │   │   ├── far3_import_windows_test.go
    │   │   ├── help.go
    │   │   ├── help_test.go
    │   │   ├── manager_footer.go
    │   │   ├── manager_frame.go
    │   │   ├── manager_ops.go
    │   │   ├── manager_ops_test.go
    │   │   ├── manager_resize_test.go
    │   │   ├── manager_semantic.go
    │   │   ├── manager_semantic_test.go
    │   │   ├── manager_ui.go
    │   │   ├── messages.go
    │   │   ├── model.go
    │   │   ├── model_test.go
    │   │   ├── plugin.go
    │   │   ├── plugin_test.go
    │   │   ├── settings.go
    │   │   ├── settings_center.go
    │   │   ├── settings_center_coverage_test.go
    │   │   ├── settings_russian_test.go
    │   │   ├── settings_test.go
    │   │   ├── strings.go
    │   │   ├── ui_test.go
    │   │   ├── vfs_io.go
    │   │   └── vfs_io_test.go
    │   ├── git
    │   │   ├── README.md
    │   │   ├── branch.go
    │   │   ├── branch_test.go
    │   │   ├── branchview.go
    │   │   ├── branchview_test.go
    │   │   ├── commit.go
    │   │   ├── commit_test.go
    │   │   ├── diff.go
    │   │   ├── diff_test.go
    │   │   ├── help.go
    │   │   ├── hunk.go
    │   │   ├── hunk_deleted_test.go
    │   │   ├── hunk_discard_test.go
    │   │   ├── hunk_new_test.go
    │   │   ├── hunk_test.go
    │   │   ├── hunkview.go
    │   │   ├── log.go
    │   │   ├── log_test.go
    │   │   ├── logdiff.go
    │   │   ├── logdiff_test.go
    │   │   ├── logdifffiles.go
    │   │   ├── logdifffiles_test.go
    │   │   ├── logview.go
    │   │   ├── logview_test.go
    │   │   ├── main_test.go
    │   │   ├── panel.go
    │   │   ├── panel_test.go
    │   │   ├── plugin.go
    │   │   ├── plugin_test.go
    │   │   ├── remote.go
    │   │   ├── stage.go
    │   │   ├── stage_test.go
    │   │   ├── status.go
    │   │   ├── status_test.go
    │   │   ├── untracked.go
    │   │   ├── untracked_dir.go
    │   │   ├── untracked_dir_test.go
    │   │   └── untracked_test.go
    │   ├── id3editor
    │   │   ├── plugin.go
    │   │   ├── plugin_contributions_test.go
    │   │   ├── plugin_handle_test.go
    │   │   ├── plugin_paths_test.go
    │   │   └── plugin_test.go
    │   ├── ide
    │   │   ├── plugin.go
    │   │   └── plugin_test.go
    │   ├── intchecker
    │   │   ├── display.go
    │   │   ├── display_test.go
    │   │   ├── encoding.go
    │   │   ├── encoding_test.go
    │   │   ├── generate.go
    │   │   ├── generate_test.go
    │   │   ├── hashfile.go
    │   │   ├── hashfile_test.go
    │   │   ├── options_test.go
    │   │   ├── plugin.go
    │   │   ├── plugin_test.go
    │   │   ├── progress.go
    │   │   ├── progress_test.go
    │   │   ├── settings.go
    │   │   ├── settings_test.go
    │   │   ├── validate.go
    │   │   ├── validate_test.go
    │   │   └── validate_ui.go
    │   ├── ios
    │   │   ├── LICENSE.go-ios
    │   │   ├── README.md
    │   │   ├── afc_registry_test.go
    │   │   ├── afc_vfs.go
    │   │   ├── afc_vfs_contract_test.go
    │   │   ├── afc_vfs_live_test.go
    │   │   ├── afc_vfs_test.go
    │   │   ├── apps.go
    │   │   ├── cmd
    │   │   │   └── ios-plugin
    │   │   │       ├── main.go
    │   │   │       └── plugring-manifest.json
    │   │   ├── core_access.go
    │   │   ├── core_access_coverage_test.go
    │   │   ├── core_access_stub.go
    │   │   ├── core_access_supported.go
    │   │   ├── core_access_supported_test.go
    │   │   ├── core_tunnel_supported.go
    │   │   ├── core_vfs.go
    │   │   ├── core_vfs_test.go
    │   │   ├── coverage_edges_test.go
    │   │   ├── coverage_plugins_test.go
    │   │   ├── coverage_test.go
    │   │   ├── device_path.go
    │   │   ├── go.mod
    │   │   ├── internal
    │   │   │   ├── afcproto
    │   │   │   │   ├── client.go
    │   │   │   │   ├── client_test.go
    │   │   │   │   ├── doc.go
    │   │   │   │   ├── errors.go
    │   │   │   │   ├── file.go
    │   │   │   │   ├── path.go
    │   │   │   │   ├── protocol.go
    │   │   │   │   ├── protocol_test.go
    │   │   │   │   └── types.go
    │   │   │   └── corefileservice
    │   │   │       ├── doc.go
    │   │   │       ├── fileservice.go
    │   │   │       └── fileservice_test.go
    │   │   ├── ios_integration_test.go
    │   │   ├── manager.go
    │   │   ├── manager_test.go
    │   │   ├── native_source.go
    │   │   ├── plugin.go
    │   │   ├── plugin_test.go
    │   │   ├── rpc_plugin.go
    │   │   ├── selectors.go
    │   │   ├── selectors_test.go
    │   │   ├── services.go
    │   │   ├── services_test.go
    │   │   ├── uri.go
    │   │   └── uri_test.go
    │   ├── k8sfs
    │   │   ├── client.go
    │   │   ├── contexts.go
    │   │   ├── contexts_test.go
    │   │   ├── k8sfs_test.go
    │   │   ├── kubeconfig.go
    │   │   ├── locale.go
    │   │   ├── plugin.go
    │   │   ├── uri.go
    │   │   └── vfs.go
    │   ├── mediainfo
    │   │   ├── README.md
    │   │   ├── analyzer.go
    │   │   ├── backend_test.go
    │   │   ├── cache.go
    │   │   ├── cache_test.go
    │   │   ├── config_dialog.go
    │   │   ├── config_dialog_test.go
    │   │   ├── coverage_contract_more_test.go
    │   │   ├── coverage_test.go
    │   │   ├── dialog.go
    │   │   ├── dialog_theme_test.go
    │   │   ├── dialog_util_coverage_test.go
    │   │   ├── exif_report_test.go
    │   │   ├── format_gaps_test.go
    │   │   ├── locale.go
    │   │   ├── macro.go
    │   │   ├── macro_test.go
    │   │   ├── matroska_bounds_test.go
    │   │   ├── model.go
    │   │   ├── open.go
    │   │   ├── open_coverage_test.go
    │   │   ├── open_enabled_test.go
    │   │   ├── open_test.go
    │   │   ├── parse_audio.go
    │   │   ├── parse_audio_containers_test.go
    │   │   ├── parse_audio_coverage_batch20_test.go
    │   │   ├── parse_ebu_stl.go
    │   │   ├── parse_heif.go
    │   │   ├── parse_image.go
    │   │   ├── parse_image_coverage_test.go
    │   │   ├── parse_iso.go
    │   │   ├── parse_iso_coverage_test.go
    │   │   ├── parse_matroska.go
    │   │   ├── parse_riff.go
    │   │   ├── parse_subtitle.go
    │   │   ├── parse_tiff.go
    │   │   ├── parse_tiff_limits_test.go
    │   │   ├── plugin.go
    │   │   ├── plugin_language_switch_test.go
    │   │   ├── plugin_test.go
    │   │   ├── quickview_provider.go
    │   │   ├── quickview_provider_test.go
    │   │   ├── render.go
    │   │   ├── render_limits_test.go
    │   │   ├── report_text.go
    │   │   ├── report_view.go
    │   │   ├── report_view_semantic.go
    │   │   ├── report_view_semantic_test.go
    │   │   ├── settings.go
    │   │   ├── settings_center.go
    │   │   ├── settings_russian_test.go
    │   │   ├── settings_test.go
    │   │   ├── source.go
    │   │   ├── subtitle_limits_test.go
    │   │   └── util.go
    │   ├── mongofs
    │   │   ├── bson.go
    │   │   ├── conn.go
    │   │   ├── ejson.go
    │   │   ├── jsonutil.go
    │   │   ├── locale.go
    │   │   ├── mongofs_test.go
    │   │   ├── password.go
    │   │   ├── plugin.go
    │   │   ├── uri.go
    │   │   └── vfs.go
    │   ├── multiarc
    │   │   ├── backend_7z.go
    │   │   ├── backend_7z_gap_test.go
    │   │   ├── backend_7z_realexec_test.go
    │   │   ├── backend_7z_test.go
    │   │   ├── backend_7z_write.go
    │   │   ├── backend_7z_write_test.go
    │   │   ├── backend_gzip.go
    │   │   ├── backend_gzip_realexec_test.go
    │   │   ├── backend_gzip_test.go
    │   │   ├── backend_gzip_write.go
    │   │   ├── backend_gzip_write_test.go
    │   │   ├── backend_tar.go
    │   │   ├── backend_tar_realexec_test.go
    │   │   ├── backend_tar_test.go
    │   │   ├── backend_tar_write.go
    │   │   ├── backend_tar_write_test.go
    │   │   ├── backend_zip.go
    │   │   ├── backend_zip_realexec_test.go
    │   │   ├── backend_zip_test.go
    │   │   ├── backend_zip_write.go
    │   │   ├── backend_zip_write_test.go
    │   │   ├── create.go
    │   │   ├── create_command.go
    │   │   ├── create_command_test.go
    │   │   ├── create_realexec_test.go
    │   │   ├── create_test.go
    │   │   ├── fake_archiver_test.go
    │   │   ├── format.go
    │   │   ├── format_test.go
    │   │   ├── multiarc.go
    │   │   ├── provider.go
    │   │   ├── provider_gap_test.go
    │   │   ├── provider_test.go
    │   │   ├── realexec_test.go
    │   │   ├── tools.go
    │   │   ├── tools_realexec_test.go
    │   │   ├── tools_test.go
    │   │   ├── vfs.go
    │   │   ├── vfs_gap_test.go
    │   │   ├── vfs_test.go
    │   │   ├── vfs_write.go
    │   │   ├── vfs_write_gap_test.go
    │   │   ├── vfs_write_test.go
    │   │   ├── write.go
    │   │   └── write_test.go
    │   ├── netbrowse
    │   │   ├── doc.go
    │   │   ├── enum_other.go
    │   │   ├── enum_windows.go
    │   │   ├── enum_windows_test.go
    │   │   ├── main_test.go
    │   │   ├── mdns.go
    │   │   ├── mdns_test.go
    │   │   ├── netbrowse_test.go
    │   │   ├── netvfs.go
    │   │   ├── netvfs_test.go
    │   │   ├── panel.go
    │   │   ├── plugin.go
    │   │   ├── resource.go
    │   │   ├── smb_full.go
    │   │   ├── smb_lite.go
    │   │   ├── smbnet.go
    │   │   ├── smbnet_test.go
    │   │   └── uri.go
    │   ├── netfox
    │   │   ├── connection_action_test.go
    │   │   ├── context_editor_test.go
    │   │   ├── coverage_test.go
    │   │   ├── crypto.go
    │   │   ├── crypto_test.go
    │   │   ├── dev
    │   │   │   ├── README.md
    │   │   │   └── unxed_f4_issue_316.json
    │   │   ├── dialog.go
    │   │   ├── dialog_test.go
    │   │   ├── directory_cache.go
    │   │   ├── directory_cache_test.go
    │   │   ├── fish_clone_session_test.go
    │   │   ├── fish_dialer_lite.go
    │   │   ├── fish_dialer_lite_test.go
    │   │   ├── fish_dialer_test.go
    │   │   ├── fish_native.go
    │   │   ├── fish_native_test.go
    │   │   ├── fish_pool.go
    │   │   ├── fish_pool_coverage_test.go
    │   │   ├── fish_reconnect_entry_test.go
    │   │   ├── fish_reconnect_test.go
    │   │   ├── fish_vfs.go
    │   │   ├── fish_vfs_device_paths_test.go
    │   │   ├── fish_vfs_test.go
    │   │   ├── fishplus
    │   │   │   ├── WINDOWS_PORT.md
    │   │   │   ├── cancel_test.go
    │   │   │   ├── cand
    │   │   │   ├── exec.go
    │   │   │   ├── exec_test.go
    │   │   │   ├── fs.go
    │   │   │   ├── fs_test.go
    │   │   │   ├── hash.go
    │   │   │   ├── hash_test.go
    │   │   │   ├── helper.ps1
    │   │   │   ├── helper.sh
    │   │   │   ├── job.go
    │   │   │   ├── job_test.go
    │   │   │   ├── keepalive.go
    │   │   │   ├── keepalive_test.go
    │   │   │   ├── ls.go
    │   │   │   ├── ls_test.go
    │   │   │   ├── mutate.go
    │   │   │   ├── mutate_test.go
    │   │   │   ├── patch.go
    │   │   │   ├── patch_test.go
    │   │   │   ├── paths.go
    │   │   │   ├── paths_test.go
    │   │   │   ├── random_fixture_test.go
    │   │   │   ├── read.go
    │   │   │   ├── read_test.go
    │   │   │   ├── script.go
    │   │   │   ├── script_pwsh_test.go
    │   │   │   ├── script_test.go
    │   │   │   ├── search.go
    │   │   │   ├── search_test.go
    │   │   │   ├── server.go
    │   │   │   ├── server_fs.go
    │   │   │   ├── server_mutate.go
    │   │   │   ├── server_patch.go
    │   │   │   ├── server_stat_other.go
    │   │   │   ├── server_stat_unix.go
    │   │   │   ├── server_test.go
    │   │   │   ├── session.go
    │   │   │   ├── session_pwsh_test.go
    │   │   │   ├── session_test.go
    │   │   │   ├── sizes
    │   │   │   ├── write.go
    │   │   │   └── write_test.go
    │   │   ├── ftp_clone_test.go
    │   │   ├── ftp_coverage_batch41_test.go
    │   │   ├── ftp_vfs.go
    │   │   ├── ftp_vfs_contract_test.go
    │   │   ├── history.go
    │   │   ├── history_test.go
    │   │   ├── issue419_test.go
    │   │   ├── lang_test.go
    │   │   ├── netfox.go
    │   │   ├── netfox_paths_test.go
    │   │   ├── netfox_test.go
    │   │   ├── netfox_uri_full.go
    │   │   ├── netfox_uri_lite.go
    │   │   ├── panel_info.go
    │   │   ├── plugin_contributions_test.go
    │   │   ├── proxy_dialog.go
    │   │   ├── proxy_dialog_coverage_test.go
    │   │   ├── registry.go
    │   │   ├── s2s_password.go
    │   │   ├── s2s_password_test.go
    │   │   ├── settings_center.go
    │   │   ├── settings_center_test.go
    │   │   ├── settings_russian_test.go
    │   │   ├── sftp_command_coverage_batch40_test.go
    │   │   ├── sftp_command_test.go
    │   │   ├── sftp_coverage_batch30_test.go
    │   │   ├── sftp_dial_test.go
    │   │   ├── sftp_readat_test.go
    │   │   ├── sftp_rename_overwrite_test.go
    │   │   ├── sftp_uri.go
    │   │   ├── sftp_uri_coverage_batch33_test.go
    │   │   ├── sftp_vfs.go
    │   │   ├── sftp_vfs_coverage_batch31_test.go
    │   │   ├── sftp_vfs_coverage_extra_test.go
    │   │   ├── sftp_vfs_gap_test.go
    │   │   ├── smb_backend.go
    │   │   ├── smb_live_test.go
    │   │   ├── smb_provider.go
    │   │   ├── smb_uri.go
    │   │   ├── smb_vfs.go
    │   │   ├── smb_vfs_test.go
    │   │   ├── ssh_agent_forwarding_test.go
    │   │   ├── ssh_agent_unix.go
    │   │   ├── ssh_agent_windows.go
    │   │   ├── ssh_agent_windows_test.go
    │   │   ├── ssh_config.go
    │   │   ├── ssh_config_test.go
    │   │   ├── ssh_dial.go
    │   │   ├── ssh_dial_test.go
    │   │   ├── ssh_fish_dialer.go
    │   │   ├── ssh_keepalive_test.go
    │   │   ├── ssh_known_hosts.go
    │   │   ├── ssh_known_hosts_coverage_test.go
    │   │   ├── ssh_known_hosts_test.go
    │   │   ├── ssh_pty.go
    │   │   ├── uri.go
    │   │   ├── vfs.go
    │   │   ├── vfs_abs_test.go
    │   │   ├── wsl_dialer_windows.go
    │   │   ├── wsl_dialer_windows_test.go
    │   │   ├── wsl_vfs_windows.go
    │   │   └── wsl_vfs_windows_test.go
    │   ├── observer
    │   │   ├── abi.go
    │   │   ├── config.go
    │   │   ├── config_test.go
    │   │   ├── doc.go
    │   │   ├── errors.go
    │   │   ├── fsbridge.go
    │   │   ├── growing_file.go
    │   │   ├── growing_file_test.go
    │   │   ├── hostimports.go
    │   │   ├── isoimg_e2e_test.go
    │   │   ├── observer_test.go
    │   │   ├── panel_enter_test.go
    │   │   ├── password.go
    │   │   ├── password_test.go
    │   │   ├── plugin.go
    │   │   ├── provider.go
    │   │   ├── provider_config_test.go
    │   │   ├── provider_test.go
    │   │   ├── runtime.go
    │   │   ├── testdata
    │   │   │   ├── isoimg
    │   │   │   │   └── compat
    │   │   │   │       ├── StdAfx.h
    │   │   │   │       ├── isz_stub.cpp
    │   │   │   │       ├── trampolines.cpp
    │   │   │   │       └── windows.h
    │   │   │   └── stub
    │   │   │       └── observer_stub.c
    │   │   ├── vfs.go
    │   │   └── wchar.go
    │   ├── pdfview
    │   │   ├── pdfview_test.go
    │   │   ├── plugin.go
    │   │   ├── provider.go
    │   │   └── vfs.go
    │   ├── proclist
    │   │   ├── README.md
    │   │   ├── actions.go
    │   │   ├── actions_test.go
    │   │   ├── actions_unix.go
    │   │   ├── actions_unix_test.go
    │   │   ├── actions_windows.go
    │   │   ├── actions_windows_test.go
    │   │   ├── collector.go
    │   │   ├── collector_darwin.go
    │   │   ├── collector_darwin_test.go
    │   │   ├── collector_linux.go
    │   │   ├── collector_linux_test.go
    │   │   ├── collector_other.go
    │   │   ├── collector_windows.go
    │   │   ├── collector_windows_test.go
    │   │   ├── columns_sync_test.go
    │   │   ├── config_dialog.go
    │   │   ├── config_dialog_test.go
    │   │   ├── details.go
    │   │   ├── details_darwin.go
    │   │   ├── details_darwin_test.go
    │   │   ├── details_environ.go
    │   │   ├── details_environ_test.go
    │   │   ├── details_linux.go
    │   │   ├── details_linux_test.go
    │   │   ├── details_test.go
    │   │   ├── details_windows.go
    │   │   ├── details_windows_test.go
    │   │   ├── main_test.go
    │   │   ├── panel.go
    │   │   ├── panel_test.go
    │   │   ├── plugin.go
    │   │   ├── plugin_test.go
    │   │   ├── priority_raw_darwin.go
    │   │   ├── priority_raw_linux.go
    │   │   ├── settings.go
    │   │   └── settings_test.go
    │   ├── sqlite
    │   │   ├── backend_cli.go
    │   │   ├── backend_cli_parse_test.go
    │   │   ├── backend_cli_test.go
    │   │   ├── backend_default_lite.go
    │   │   ├── backend_driver.go
    │   │   ├── coverage_test.go
    │   │   ├── export.go
    │   │   ├── locale.go
    │   │   ├── plugin.go
    │   │   ├── plugin_test.go
    │   │   ├── provider.go
    │   │   ├── ui.go
    │   │   ├── ui_test.go
    │   │   ├── vfs.go
    │   │   └── vfs_test.go
    │   ├── svcmgr
    │   │   ├── collector_other.go
    │   │   ├── collector_windows.go
    │   │   ├── collector_windows_test.go
    │   │   ├── control.go
    │   │   ├── control_other.go
    │   │   ├── control_windows.go
    │   │   ├── details.go
    │   │   ├── details_other.go
    │   │   ├── details_windows.go
    │   │   ├── doc.go
    │   │   ├── main_test.go
    │   │   ├── panel.go
    │   │   ├── plugin.go
    │   │   ├── properties.go
    │   │   ├── service.go
    │   │   └── svcmgr_test.go
    │   └── visren
    │       ├── LICENSE.upstream
    │       ├── config.go
    │       ├── config_test.go
    │       ├── coverage_contract_test.go
    │       ├── coverage_test.go
    │       ├── dialog.go
    │       ├── dialog_coverage_extra_test.go
    │       ├── dialog_test.go
    │       ├── editor.go
    │       ├── editor_test.go
    │       ├── engine_test.go
    │       ├── masks.go
    │       ├── masks_coverage_test.go
    │       ├── metadata.go
    │       ├── metadata_coverage_test.go
    │       ├── metadata_test.go
    │       ├── model.go
    │       ├── plugin.go
    │       ├── plugin_language_switch_test.go
    │       ├── plugin_test.go
    │       ├── rename.go
    │       ├── rename_test.go
    │       ├── replace.go
    │       ├── settings_center.go
    │       ├── settings_center_test.go
    │       ├── settings_russian_test.go
    │       ├── transforms.go
    │       └── word_div_prompt_test.go
    ├── plugring
    │   ├── hello_plugring.lua
    │   └── index.yaml
    ├── profiler
    │   └── reports
    │       ├── profile-report-editor-search-2026-10-08.md
    │       ├── profile-report-f4-2026-10-07-203344.md
    │       ├── profile-report-f4-windowed-2026-10-08-002658.md
    │       ├── profile-report-viewer-search-2026-10-08.md
    │       ├── profile-report-workspace-switch-2026-10-08.md
    │       ├── profile-report-workspace-switch-fix-2026-10-08.md
    │       └── right-panel-icon-raster-diagnostic-2026-10-08.md
    ├── qt
    │   └── host
    │       ├── CMakeLists.txt
    │       ├── README.md
    │       ├── cmake
    │       │   ├── F4HostQmlFiles.cmake
    │       │   ├── SplitMsvcLinkResponse.cmake
    │       │   ├── f4-qt-host-target
    │       │   │   └── CMakeLists.txt
    │       │   ├── f4-zoin-gallery-portable-hook.cmake
    │       │   └── repair-macos-app-rpath.cmake
    │       ├── conanfile.py
    │       ├── icons
    │       │   ├── lucide
    │       │   │   ├── LICENSE
    │       │   │   ├── SOURCE.md
    │       │   │   ├── app-window.svg
    │       │   │   ├── archive.svg
    │       │   │   ├── arrow-down-a-z.svg
    │       │   │   ├── arrow-down-up.svg
    │       │   │   ├── arrow-down-wide-narrow.svg
    │       │   │   ├── arrow-down.svg
    │       │   │   ├── arrow-left-from-line.svg
    │       │   │   ├── arrow-left.svg
    │       │   │   ├── arrow-right-from-line.svg
    │       │   │   ├── arrow-up.svg
    │       │   │   ├── binary.svg
    │       │   │   ├── blocks.svg
    │       │   │   ├── book-open.svg
    │       │   │   ├── calendar-plus.svg
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
    │       │   │   ├── eye-off.svg
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
    │       │   │   ├── list-checks.svg
    │       │   │   ├── list-restart.svg
    │       │   │   ├── list-tree.svg
    │       │   │   ├── list.svg
    │       │   │   ├── loader-circle.svg
    │       │   │   ├── locate-fixed.svg
    │       │   │   ├── lock-keyhole-open.svg
    │       │   │   ├── lock-keyhole.svg
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
    │       │   │   ├── pause.svg
    │       │   │   ├── pencil.svg
    │       │   │   ├── play-filled.svg
    │       │   │   ├── play.svg
    │       │   │   ├── plug.svg
    │       │   │   ├── plus.svg
    │       │   │   ├── refresh-cw.svg
    │       │   │   ├── repeat.svg
    │       │   │   ├── rotate-ccw.svg
    │       │   │   ├── rows-3.svg
    │       │   │   ├── rows-4.svg
    │       │   │   ├── save.svg
    │       │   │   ├── search.svg
    │       │   │   ├── settings-2.svg
    │       │   │   ├── slash.svg
    │       │   │   ├── smartphone.svg
    │       │   │   ├── space.svg
    │       │   │   ├── sparkles.svg
    │       │   │   ├── square-terminal.svg
    │       │   │   ├── tablet.svg
    │       │   │   ├── text-wrap.svg
    │       │   │   ├── trash-2.svg
    │       │   │   ├── triangle-alert.svg
    │       │   │   ├── usb-flash-drive.svg
    │       │   │   ├── user-round.svg
    │       │   │   ├── users-round.svg
    │       │   │   ├── video.svg
    │       │   │   ├── volume-2.svg
    │       │   │   ├── volume-x.svg
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
    │       │   ├── F4EditableComboBox.qml
    │       │   ├── F4HostWindow.qml
    │       │   ├── F4RadioButton.qml
    │       │   ├── F4ScrollBar.qml
    │       │   ├── F4Slider.qml
    │       │   ├── F4TextField.qml
    │       │   ├── F4ToolTip.qml
    │       │   ├── FileFieldColumnsPopup.qml
    │       │   ├── FileFieldFilterPopup.qml
    │       │   ├── FileFieldToolsOverlay.qml
    │       │   ├── FilePanelChrome.qml
    │       │   ├── FilePanelView.qml
    │       │   ├── FontSettingsPage.qml
    │       │   ├── GalleryPanelHost.qml
    │       │   ├── GalleryPanelHostAdapter.qml
    │       │   ├── GalleryPanelInputRouter.qml
    │       │   ├── GallerySettingsPage.qml
    │       │   ├── GalleryViewerHost.qml
    │       │   ├── GenericDialog.qml
    │       │   ├── GuiSettingsPage.qml
    │       │   ├── HelpContent.qml
    │       │   ├── HostPixelAlignedImage.qml
    │       │   ├── HostPresentationUtilities.qml
    │       │   ├── HostThemePalette.qml
    │       │   ├── HostTypography.qml
    │       │   ├── InfoPanelView.qml
    │       │   ├── KeyBarActionButton.qml
    │       │   ├── KeyBarView.qml
    │       │   ├── NativeSettingsPage.qml
    │       │   ├── OperationsQueueController.qml
    │       │   ├── OperationsQueueSurface.qml
    │       │   ├── OverlayHost.qml
    │       │   ├── PanelPairSurface.qml
    │       │   ├── PanelRendererMenu.qml
    │       │   ├── PanelSplitter.qml
    │       │   ├── PanelStatusMetric.qml
    │       │   ├── PanelStatusOverlay.qml
    │       │   ├── PanelsSurface.qml
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
    │       │   ├── WorkspaceTabs.qml
    │       │   └── main.qml
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
    │       │   ├── F4GalleryBridgeFileFields.cpp
    │       │   ├── F4GalleryBridgeGroupPage.cpp
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
    │       │   ├── F4PanelPreferences.h
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
    │       │   ├── WindowGeometryPersistence.h
    │       │   └── main.cpp
    │       └── tests
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
    │           ├── PanelCatalogModelTests.cpp
    │           ├── PanelIntentControllerTests.cpp
    │           ├── PanelSessionRegistryTests.cpp
    │           ├── QWindowKitTitleBarTests.cpp
    │           ├── QmlProfilerPlugins.cpp
    │           ├── QtMediaClientTests.cpp
    │           ├── QtShellControllerProductionTests.cpp
    │           ├── QtShellControllerTests.cpp
    │           ├── SemanticPresentationTypes.cpp
    │           ├── ShellStateStoreTests.cpp
    │           ├── StaticQtImagePluginImports.cpp
    │           ├── StaticQtTestPluginImports.cpp
    │           ├── TestExtUiStateController.cpp
    │           ├── TestExtUiStateController.h
    │           ├── TestGalleryPanel.qml
    │           ├── ViewerCoordinatorTests.cpp
    │           ├── WindowGeometryPersistenceTests.cpp
    │           ├── data
    │           │   └── mixed-dpi-screens.json
    │           └── manual
    │               ├── DocumentRepeaterPerfProbe.qml
    │               ├── ListViewIncrementalRebaseProbe.qml
    │               ├── ListViewPrependRebaseProbe.qml
    │               ├── ListViewStableSlotPoolProbe.qml
    │               └── MsgpackSceneDecodeProbe.cpp
    ├── run-f4-gallery.sh
    ├── run-f4-gogpu.sh
    ├── run-f4-qml.sh
    ├── scripts
    │   ├── build_ipk.sh
    │   ├── build_isoimg_test_iso.sh
    │   ├── build_isoimg_test_wasm.sh
    │   ├── build_observer_test_wasm.sh
    │   ├── check_archive_deps.sh
    │   ├── check_isoimg_wasm.sh
    │   ├── check_release_version.sh
    │   ├── filelist_update.sh
    │   ├── import_mc_theme.py
    │   ├── openwrt_smoke.py
    │   ├── test_plugins.sh
    │   └── test_resurrect.sh
    ├── sdk
    │   ├── extui
    │   │   ├── cmd
    │   │   │   └── generate-fixtures
    │   │   │       └── main.go
    │   │   ├── envelope.go
    │   │   ├── envelope_test.go
    │   │   ├── file_fields.go
    │   │   ├── file_fields_test.go
    │   │   ├── fixtures_generate.go
    │   │   ├── model.go
    │   │   ├── model_coverage_test.go
    │   │   ├── model_test.go
    │   │   ├── operations_queue_model_test.go
    │   │   ├── patch.go
    │   │   ├── protocol_v4.md
    │   │   ├── terminal_palette_test.go
    │   │   └── testdata
    │   │       └── v4_envelopes.msgpack
    │   ├── f4plugin
    │   │   ├── metadata.go
    │   │   ├── plugin.go
    │   │   └── plugin_test.go
    │   ├── f4rpc
    │   │   ├── mux.go
    │   │   └── mux_test.go
    │   ├── f4settings
    │   │   ├── localization_test.go
    │   │   ├── settings.go
    │   │   ├── settings_test.go
    │   │   └── struct_provider.go
    │   └── lua
    │       └── f4rpc.lua
    ├── skills-lock.json
    ├── third_party
    │   ├── ZoinGallery
    │   └── vtui
    │       ├── .github
    │       │   └── workflows
    │       │       └── ci.yml
    │       ├── .gitignore
    │       ├── .golangci.yml
    │       ├── ARCHITECTURE.md
    │       ├── ARCH_PROPOSALS.md
    │       ├── AUTOLAYOUT.md
    │       ├── DRAGDROP.md
    │       ├── GRAPHICS.md
    │       ├── ISSUE_205_SOLUTION_REVIEW.md
    │       ├── ISSUE_283_SOLUTION_REVIEW.md
    │       ├── LAYOUT.md
    │       ├── LICENSE
    │       ├── OPTIMIZATIONS.md
    │       ├── PLATFORMS.md
    │       ├── README.md
    │       ├── REVIEW.md
    │       ├── SCREEN_DUMP.md
    │       ├── TEXTSEG.md
    │       ├── UI_TESTING.md
    │       ├── UNICODE_PLAN.md
    │       ├── UX_GUIDELINES.md
    │       ├── WIDTH_NEGOTIATION.md
    │       ├── WORDNAV.md
    │       ├── ansi_writer.go
    │       ├── ansi_writer_syscons_test.go
    │       ├── autocomplete.go
    │       ├── autocomplete_test.go
    │       ├── autocomplete_trigger_test.go
    │       ├── autolayout.go
    │       ├── autolayout_test.go
    │       ├── automation_test.go
    │       ├── backend_info.go
    │       ├── backend_info_test.go
    │       ├── backspace.go
    │       ├── backspace_test.go
    │       ├── bar.go
    │       ├── bar_test.go
    │       ├── baseframe.go
    │       ├── baseframe_test.go
    │       ├── basewindow.go
    │       ├── basewindow_test.go
    │       ├── bidi.go
    │       ├── bidi_mirror_table.go
    │       ├── bidi_test.go
    │       ├── bindings
    │       │   ├── CMakeLists.txt
    │       │   ├── README.md
    │       │   ├── c
    │       │   │   ├── CMakeLists.txt
    │       │   │   ├── README.md
    │       │   │   ├── cabi
    │       │   │   │   └── main.go
    │       │   │   ├── examples
    │       │   │   │   └── hello.c
    │       │   │   ├── include
    │       │   │   │   ├── vtui.h
    │       │   │   │   └── vtui_constants.h
    │       │   │   └── src
    │       │   │       └── vtui.c
    │       │   ├── cpp
    │       │   │   ├── CMakeLists.txt
    │       │   │   ├── README.md
    │       │   │   ├── examples
    │       │   │   │   └── hello.cpp
    │       │   │   └── include
    │       │   │       └── vtui.hpp
    │       │   ├── lua
    │       │   │   ├── README.md
    │       │   │   ├── examples
    │       │   │   │   └── hello.lua
    │       │   │   ├── rockspec
    │       │   │   │   └── vtui-scm-1.rockspec
    │       │   │   ├── src
    │       │   │   │   └── vtui_lua.c
    │       │   │   ├── tests
    │       │   │   │   └── test_vtui.lua
    │       │   │   └── vtui.lua
    │       │   ├── node
    │       │   │   ├── README.md
    │       │   │   ├── examples
    │       │   │   │   ├── hello.js
    │       │   │   │   └── hello.ts
    │       │   │   ├── index.js
    │       │   │   ├── package.json
    │       │   │   ├── session.js
    │       │   │   ├── test
    │       │   │   │   └── test.js
    │       │   │   ├── ui.js
    │       │   │   └── vtui.d.ts
    │       │   ├── php
    │       │   │   ├── README.md
    │       │   │   ├── composer.json
    │       │   │   ├── examples
    │       │   │   │   └── hello.php
    │       │   │   ├── src
    │       │   │   │   └── Vtui.php
    │       │   │   └── tests
    │       │   │       └── test_vtui.php
    │       │   └── python
    │       │       ├── README.md
    │       │       ├── examples
    │       │       │   ├── async_demo.py
    │       │       │   └── hello.py
    │       │       ├── tests
    │       │       │   └── test_vtui.py
    │       │       └── vtui
    │       │           ├── __init__.py
    │       │           ├── _props.py
    │       │           ├── async_session.py
    │       │           ├── session.py
    │       │           └── ui.py
    │       ├── bindings.md
    │       ├── bindings_integration_test.go
    │       ├── box_runes.go
    │       ├── button.go
    │       ├── button_test.go
    │       ├── cellspan_test.go
    │       ├── checkbox.go
    │       ├── checkbox_test.go
    │       ├── checkgroup.go
    │       ├── clipboard.go
    │       ├── clipboard_goclip_test.go
    │       ├── clipboard_gui_test.go
    │       ├── clipboard_test.go
    │       ├── clipboard_unix.go
    │       ├── clipboard_windows.go
    │       ├── clusters_test.go
    │       ├── cmd
    │       │   ├── fontprobe
    │       │   │   └── main.go
    │       │   ├── test-app
    │       │   │   ├── main.go
    │       │   │   └── main_test.go
    │       │   ├── vtui-cast
    │       │   │   ├── main.go
    │       │   │   └── main_test.go
    │       │   ├── vtui-dialog
    │       │   │   ├── main.go
    │       │   │   └── main_test.go
    │       │   ├── vtui-gen
    │       │   │   ├── main.go
    │       │   │   └── main_test.go
    │       │   ├── vtui-host
    │       │   │   └── main.go
    │       │   ├── vtui-lint
    │       │   │   ├── main.go
    │       │   │   └── main_test.go
    │       │   ├── vtui-replay
    │       │   │   ├── main.go
    │       │   │   └── main_test.go
    │       │   ├── vtui-wasm
    │       │   │   └── main.go
    │       │   └── vuic
    │       │       ├── main.go
    │       │       └── main_test.go
    │       ├── colors.go
    │       ├── colors_test.go
    │       ├── combobox.go
    │       ├── combobox_color_test.go
    │       ├── combobox_layout_test.go
    │       ├── combobox_test.go
    │       ├── commands.go
    │       ├── common_dialogs.go
    │       ├── common_dialogs_test.go
    │       ├── conhost_altscreen_windows.go
    │       ├── conhost_altscreen_windows_test.go
    │       ├── console_freebsd.go
    │       ├── console_other.go
    │       ├── crash_report.go
    │       ├── crash_report_pid_unix.go
    │       ├── crash_report_pid_windows.go
    │       ├── crash_report_stub.go
    │       ├── crash_report_test.go
    │       ├── cursor_style.go
    │       ├── debug.go
    │       ├── debug_test.go
    │       ├── desktop.go
    │       ├── desktop_test.go
    │       ├── dialog_test.go
    │       ├── docs
    │       │   ├── LUNOBOT
    │       │   │   └── 926.md
    │       │   ├── shell_scripting.md
    │       │   └── widgets.md
    │       ├── document_lifecycle_test.go
    │       ├── dragdrop.go
    │       ├── dragdrop_test.go
    │       ├── dynamictext.go
    │       ├── dynamictext_test.go
    │       ├── ebiten_dragdrop.go
    │       ├── ebiten_host.go
    │       ├── ebiten_keys.go
    │       ├── ebiten_renderer.go
    │       ├── ebiten_renderer_test.go
    │       ├── ebiten_stub.go
    │       ├── edit.go
    │       ├── edit_cluster_boundary_test.go
    │       ├── edit_multiline.go
    │       ├── edit_multiline_test.go
    │       ├── edit_semantic_input_test.go
    │       ├── edit_test.go
    │       ├── edit_words.go
    │       ├── events.go
    │       ├── eventsink_test.go
    │       ├── examples
    │       │   └── shell
    │       │       └── demo.sh
    │       ├── factory.go
    │       ├── factory_test.go
    │       ├── far2l_extensions.go
    │       ├── far2l_extensions_test.go
    │       ├── filelist.md
    │       ├── filelist_update.sh
    │       ├── frame.go
    │       ├── framemanager.go
    │       ├── framemanager_caret_test.go
    │       ├── framemanager_hidebars_test.go
    │       ├── framemanager_paste.go
    │       ├── framemanager_paste_test.go
    │       ├── framemanager_test.go
    │       ├── fuzzy.go
    │       ├── fuzzy_test.go
    │       ├── go.mod
    │       ├── go.sum
    │       ├── gogpu_customchar_test.go
    │       ├── gogpu_dnd.go
    │       ├── gogpu_dnd_test.go
    │       ├── gogpu_ffi.go
    │       ├── gogpu_ffi_stub.go
    │       ├── gogpu_glyph_table.go
    │       ├── gogpu_glyphgen_test.go
    │       ├── gogpu_host.go
    │       ├── gogpu_host_test.go
    │       ├── gogpu_keys_test.go
    │       ├── gogpu_profile.go
    │       ├── gogpu_renderer.go
    │       ├── gogpu_renderer_test.go
    │       ├── gogpu_scroll_other.go
    │       ├── gogpu_scroll_windows.go
    │       ├── gogpu_stub.go
    │       ├── graphics.go
    │       ├── graphics_external_test.go
    │       ├── graphics_far2l.go
    │       ├── graphics_far2l_test.go
    │       ├── graphics_frame_test.go
    │       ├── graphics_image.go
    │       ├── graphics_kitty.go
    │       ├── graphics_kitty_test.go
    │       ├── graphics_native.go
    │       ├── graphics_native_test.go
    │       ├── graphics_probe.go
    │       ├── graphics_probe_test.go
    │       ├── graphics_probe_unix.go
    │       ├── graphics_probe_windows.go
    │       ├── graphics_scale.go
    │       ├── graphics_sixel.go
    │       ├── graphics_sixel_cursor_test.go
    │       ├── graphics_sixel_layered.go
    │       ├── graphics_sixel_layered_test.go
    │       ├── graphics_sixel_quality_test.go
    │       ├── graphics_sixel_tabrow_test.go
    │       ├── graphics_sixel_test.go
    │       ├── graphics_sixel_truecolor.go
    │       ├── graphics_sixel_truecolor_test.go
    │       ├── graphics_test.go
    │       ├── grid_nav.go
    │       ├── group.go
    │       ├── group_test.go
    │       ├── groupbox.go
    │       ├── grow_test.go
    │       ├── gui_api.go
    │       ├── gui_api_fallback.go
    │       ├── gui_boxdraw.go
    │       ├── gui_boxdraw_test.go
    │       ├── gui_font.go
    │       ├── gui_font_native_stub.go
    │       ├── gui_font_native_windows.go
    │       ├── gui_font_native_windows_test.go
    │       ├── gui_font_scripts.go
    │       ├── gui_font_scripts_test.go
    │       ├── gui_font_test.go
    │       ├── help_engine.go
    │       ├── help_engine_test.go
    │       ├── help_resize_test.go
    │       ├── help_view.go
    │       ├── help_view_mouse_test.go
    │       ├── help_view_test.go
    │       ├── highlight.go
    │       ├── highlight_test.go
    │       ├── history_test.go
    │       ├── indicator_background_test.go
    │       ├── internal
    │       │   ├── hideconsole
    │       │   │   ├── go.mod
    │       │   │   └── hideconsole.go
    │       │   └── uba
    │       │       ├── LICENSE.x-text
    │       │       ├── bracket.go
    │       │       ├── core.go
    │       │       ├── uba.go
    │       │       └── uba_test.go
    │       ├── keybar.go
    │       ├── keybar_test.go
    │       ├── keys_common.go
    │       ├── keys_common_test.go
    │       ├── keys_special.go
    │       ├── label.go
    │       ├── label_test.go
    │       ├── layout.go
    │       ├── layout_test.go
    │       ├── layout_validator.go
    │       ├── layout_validator_test.go
    │       ├── listbox.go
    │       ├── listbox_test.go
    │       ├── localization.go
    │       ├── localization_test.go
    │       ├── lookup_test.go
    │       ├── menubar.go
    │       ├── menubar_test.go
    │       ├── mouse_gesture.go
    │       ├── mouse_gesture_test.go
    │       ├── multilineedit.go
    │       ├── multilineedit_semantic.go
    │       ├── multilineedit_semantic_test.go
    │       ├── multilineedit_test.go
    │       ├── painter.go
    │       ├── palette.go
    │       ├── palette_batch_test.go
    │       ├── palette_test.go
    │       ├── panic_bridge.go
    │       ├── panic_bridge_test.go
    │       ├── progressbar.go
    │       ├── progressbar_test.go
    │       ├── properties.go
    │       ├── properties_gen.go
    │       ├── properties_test.go
    │       ├── protocol.go
    │       ├── protocol_test.go
    │       ├── radiobutton.go
    │       ├── radiogroup.go
    │       ├── radiogroup_test.go
    │       ├── rowprovider_test.go
    │       ├── runewidth.go
    │       ├── runewidth_test.go
    │       ├── screenbuf.go
    │       ├── screenbuf_cursor_color_test.go
    │       ├── screenbuf_cursor_test.go
    │       ├── screenbuf_test.go
    │       ├── screendump.go
    │       ├── screendump_test.go
    │       ├── screenobject.go
    │       ├── screenobject_test.go
    │       ├── screenshot.png
    │       ├── scrollbar.go
    │       ├── scrollbar_test.go
    │       ├── scrollbar_widget_test.go
    │       ├── scrollview.go
    │       ├── scrollview_test.go
    │       ├── semantic.go
    │       ├── semantic_help.go
    │       ├── semantic_help_test.go
    │       ├── semantic_table.go
    │       ├── semantic_table_test.go
    │       ├── semantic_test.go
    │       ├── separator.go
    │       ├── session_test.go
    │       ├── shutdown_test.go
    │       ├── sizespec.go
    │       ├── spacer.go
    │       ├── standard_dialogs_layout_test.go
    │       ├── statusline.go
    │       ├── statusline_test.go
    │       ├── step_test.go
    │       ├── strings.go
    │       ├── symbols.go
    │       ├── sys_darwin.go
    │       ├── sys_unix.go
    │       ├── sys_windows.go
    │       ├── table.go
    │       ├── table_dialog.go
    │       ├── table_dialog_test.go
    │       ├── table_test.go
    │       ├── tasks.go
    │       ├── tasks_test.go
    │       ├── terminal_env.go
    │       ├── terminal_env_console_altscreen_test.go
    │       ├── terminal_env_test.go
    │       ├── terminal_env_unix.go
    │       ├── terminal_env_windows.go
    │       ├── test_main_test.go
    │       ├── testdata
    │       │   ├── hello.golden.json
    │       │   └── hello.vui
    │       ├── testing.go
    │       ├── text.go
    │       ├── text_decorations.go
    │       ├── text_decorations_test.go
    │       ├── text_test.go
    │       ├── text_utils.go
    │       ├── text_utils_test.go
    │       ├── textseg.go
    │       ├── textseg_test.go
    │       ├── treeview.go
    │       ├── treeview_test.go
    │       ├── types.go
    │       ├── validator.go
    │       ├── validator_test.go
    │       ├── vmenu.go
    │       ├── vmenu_cancel_test.go
    │       ├── vmenu_submenu_test.go
    │       ├── vmenu_test.go
    │       ├── vmenu_window.go
    │       ├── vmenu_window_test.go
    │       ├── vocabulary.json
    │       ├── vocabulary.schema.json
    │       ├── vreactive
    │       │   ├── README.md
    │       │   ├── animator.go
    │       │   ├── animator_test.go
    │       │   ├── bindings.go
    │       │   ├── bindings_test.go
    │       │   ├── computed.go
    │       │   ├── computed_test.go
    │       │   ├── easing.go
    │       │   ├── easing_test.go
    │       │   ├── property.go
    │       │   ├── property_test.go
    │       │   └── statemachine.go
    │       ├── vtext.go
    │       ├── vtext_test.go
    │       ├── vtui_test.go
    │       ├── vui.schema.json
    │       ├── vui_layout.go
    │       ├── vui_loader.go
    │       ├── vui_test.go
    │       ├── wayland_host.go
    │       ├── wayland_host_test.go
    │       ├── wayland_present_test.go
    │       ├── wayland_renderer.go
    │       ├── wayland_stub.go
    │       ├── wheel_scroll.go
    │       ├── wheel_scroll_test.go
    │       ├── win32_console_common.go
    │       ├── win32_console_stub.go
    │       ├── win32_console_test.go
    │       ├── win32_console_windows.go
    │       ├── win32_dnd_stub.go
    │       ├── win32_dnd_test.go
    │       ├── win32_dnd_windows.go
    │       ├── win32_droptarget_other_windows.go
    │       ├── win32_droptarget_windows.go
    │       ├── win32_gui_common.go
    │       ├── win32_gui_renderer.go
    │       ├── win32_gui_resize_windows_test.go
    │       ├── win32_gui_stub.go
    │       ├── win32_gui_test.go
    │       ├── win32_gui_windows.go
    │       ├── window.go
    │       ├── word_nav.go
    │       ├── word_nav_test.go
    │       ├── workspace_activation.go
    │       ├── workspace_altnumber_test.go
    │       ├── x11_host.go
    │       ├── x11_host_test.go
    │       ├── x11_keys_shared.go
    │       ├── x11_keys_test.go
    │       ├── x11_render_common.go
    │       ├── x11_renderer.go
    │       ├── x11_shm_fallback.go
    │       ├── x11_shm_unix.go
    │       ├── x11_stub.go
    │       ├── x11_xdnd.go
    │       ├── x11_xdnd_test.go
    │       ├── xlat.go
    │       ├── xlat_tables.go
    │       └── xlat_test.go
    ├── time.txt
    ├── tools
    │   ├── analyze_navigation_benchmark.py
    │   ├── conpty_probe.py
    │   ├── conpty_probe_child.py
    │   ├── conptyreconcile
    │   │   ├── capture.go
    │   │   ├── clear_probe_windows.go
    │   │   ├── command_compare_windows.go
    │   │   ├── command_probe_windows.go
    │   │   ├── command_suite_windows.go
    │   │   ├── command_timing_windows.go
    │   │   ├── control_stream.go
    │   │   ├── control_stream_test.go
    │   │   ├── edge_probe_windows.go
    │   │   ├── emitter.go
    │   │   ├── empty_probe_windows.go
    │   │   ├── gate.go
    │   │   ├── gate_nonwindows.go
    │   │   ├── gate_windows.go
    │   │   ├── go.mod
    │   │   ├── go.sum
    │   │   ├── hash.go
    │   │   ├── host_constants.go
    │   │   ├── host_history.go
    │   │   ├── host_history_test.go
    │   │   ├── host_stream.go
    │   │   ├── host_stream_chunking.go
    │   │   ├── host_stream_chunking_test.go
    │   │   ├── host_stream_test.go
    │   │   ├── lifecycle_probe_windows.go
    │   │   ├── line_diff.go
    │   │   ├── logical_lines.go
    │   │   ├── logical_lines_test.go
    │   │   ├── main.go
    │   │   ├── native_probe.go
    │   │   ├── native_probe_nonwindows.go
    │   │   ├── native_probe_windows.go
    │   │   ├── passthrough_probe_test.go
    │   │   ├── passthrough_probe_windows.go
    │   │   ├── payload_assertions.go
    │   │   ├── payload_assertions_test.go
    │   │   ├── pinned_host.go
    │   │   ├── pinned_host_nonwindows.go
    │   │   ├── pinned_host_windows.go
    │   │   ├── probe.go
    │   │   ├── quirk_probe_windows.go
    │   │   ├── reflow_probe.go
    │   │   ├── reflow_probe_windows.go
    │   │   ├── scroll_probe_windows.go
    │   │   ├── scrollback.go
    │   │   ├── scrollback_test.go
    │   │   ├── seeds.go
    │   │   ├── semantic_probe.go
    │   │   └── semantic_probe_windows.go
    │   ├── f4imgprobe
    │   │   ├── README.txt
    │   │   ├── main.go
    │   │   ├── windowlongptr_32.go
    │   │   └── windowlongptr_64.go
    │   ├── find_hardcoded.go
    │   ├── fishplus_probe.sh
    │   ├── fishplus_testlab
    │   │   ├── TESTLAB.md
    │   │   ├── fishclient.py
    │   │   └── test_patch.py
    │   ├── hardcode
    │   │   ├── hardcode.go
    │   │   └── hardcode_test.go
    │   ├── hardcoded_baseline.txt
    │   ├── icons
    │   │   ├── go.mod
    │   │   ├── go.sum
    │   │   ├── main.go
    │   │   ├── main_test.go
    │   │   └── third_party
    │   │       └── oksvg
    │   │           ├── .gitignore
    │   │           ├── LICENSE
    │   │           ├── README.md
    │   │           ├── definitions.go
    │   │           ├── draw.go
    │   │           ├── go.mod
    │   │           ├── icon_cursor.go
    │   │           ├── path_cursor.go
    │   │           ├── path_style.go
    │   │           ├── public.go
    │   │           ├── svg_icon.go
    │   │           ├── svg_path.go
    │   │           └── utils.go
    │   ├── langfmt
    │   │   ├── main.go
    │   │   └── main_test.go
    │   ├── releasecheck
    │   │   ├── main.go
    │   │   └── main_test.go
    │   ├── sanitize_native_probe_report.ps1
    │   ├── test_analyze_navigation_benchmark.py
    │   ├── test_runner.sh
    │   ├── ttytest
    │   │   ├── README.md
    │   │   ├── analyze_log.py
    │   │   ├── scenarios.py
    │   │   └── ttytest.py
    │   ├── verify_native_probe_artifacts.ps1
    │   ├── vtui-screen
    │   │   ├── README.md
    │   │   ├── main.go
    │   │   └── main_test.go
    │   ├── wine_color_probe
    │   │   ├── main.go
    │   │   └── main_other.go
    │   └── wine_syscall_probe
    │       ├── go.mod
    │       ├── main.go
    │       └── probe_amd64.s
    └── vfs
        ├── birthtime_linux.go
        ├── birthtime_linux_test.go
        ├── birthtime_other.go
        ├── bulk_copy_test.go
        ├── codepages.go
        ├── codepages_cjk.go
        ├── codepages_forced_test.go
        ├── codepages_iconv_unix.go
        ├── codepages_issue875_test.go
        ├── codepages_nocjk.go
        ├── codepages_test.go
        ├── codepages_unix.go
        ├── codepages_unix_test.go
        ├── codepages_utf8_system_test.go
        ├── codepages_windows.go
        ├── codepages_windows_test.go
        ├── contributions.go
        ├── destination_overwrite_test.go
        ├── device_path.go
        ├── device_path_test.go
        ├── device_size_test.go
        ├── disks_unix.go
        ├── disks_unix_gap_test.go
        ├── disks_unix_test.go
        ├── disks_vfs.go
        ├── disks_vfs_coverage_test.go
        ├── disks_vfs_test.go
        ├── disks_windows.go
        ├── disks_windows_test.go
        ├── file_mask.go
        ├── file_mask_test.go
        ├── hidden_rule.go
        ├── hidden_rule_test.go
        ├── hidden_unix.go
        ├── hidden_windows.go
        ├── hidden_windows_test.go
        ├── hostfs
        │   ├── errno_windows.go
        │   ├── hostfs_posix.go
        │   ├── hostfs_posix_test.go
        │   ├── hostfs_windows.go
        │   ├── hostfs_windows_coverage_test.go
        │   ├── hostfs_windows_test.go
        │   └── hostfs_winescape.go
        ├── hostmode
        │   ├── hostmode.go
        │   ├── hostmode_lite.go
        │   └── hostmode_test.go
        ├── hostpath
        │   ├── hostpath_posix.go
        │   └── hostpath_windows.go
        ├── isabs_test.go
        ├── junction_buffer.go
        ├── junction_buffer_test.go
        ├── junction_other.go
        ├── junction_windows.go
        ├── junction_windows_test.go
        ├── lock_manager_test.go
        ├── metadata.go
        ├── metadata_test.go
        ├── mount_linux.go
        ├── mount_linux_test.go
        ├── mount_other.go
        ├── name_order.go
        ├── null_vfs.go
        ├── null_vfs_test.go
        ├── os_vfs.go
        ├── os_vfs_birthtime_bsd_test.go
        ├── os_vfs_birthtime_linux_test.go
        ├── os_vfs_birthtime_windows_test.go
        ├── os_vfs_contract_coverage_test.go
        ├── os_vfs_dangling_link_test.go
        ├── os_vfs_display_error_test.go
        ├── os_vfs_dot_test.go
        ├── os_vfs_elevation_test.go
        ├── os_vfs_junction_stub.go
        ├── os_vfs_junction_test.go
        ├── os_vfs_listing.go
        ├── os_vfs_listing_other_test.go
        ├── os_vfs_listing_test.go
        ├── os_vfs_listing_windows_test.go
        ├── os_vfs_noreplace_test.go
        ├── os_vfs_open_readonly_test.go
        ├── os_vfs_physical_other.go
        ├── os_vfs_physical_test.go
        ├── os_vfs_physical_unix.go
        ├── os_vfs_physical_windows.go
        ├── os_vfs_platform_darwin.go
        ├── os_vfs_platform_unix.go
        ├── os_vfs_platform_windows.go
        ├── os_vfs_posix_atim.go
        ├── os_vfs_posix_atimespec.go
        ├── os_vfs_preview_windows.go
        ├── os_vfs_readdir_other.go
        ├── os_vfs_readdir_windows.go
        ├── os_vfs_readdir_windows_test.go
        ├── os_vfs_reparse_other.go
        ├── os_vfs_reparse_windows.go
        ├── os_vfs_reparse_windows_test.go
        ├── os_vfs_search.go
        ├── os_vfs_search_test.go
        ├── os_vfs_share_windows_test.go
        ├── os_vfs_symlink_test.go
        ├── os_vfs_test.go
        ├── os_vfs_unix_test.go
        ├── os_vfs_windows.go
        ├── os_vfs_windows_test.go
        ├── panel_keys_test.go
        ├── patch_inplace_test.go
        ├── personality.go
        ├── privileges_windows.go
        ├── prompt_hold.go
        ├── prompt_hold_test.go
        ├── pua.go
        ├── pua_test.go
        ├── quick_view.go
        ├── quick_view_test.go
        ├── read_access_test.go
        ├── registry_vfs_windows.go
        ├── registry_vfs_windows_test.go
        ├── rename_noreplace.go
        ├── rename_noreplace_darwin.go
        ├── rename_noreplace_linux.go
        ├── rename_noreplace_linux_test.go
        ├── rename_noreplace_test.go
        ├── rename_noreplace_unix.go
        ├── rename_noreplace_windows.go
        ├── reparse.go
        ├── reparse_test.go
        ├── scanner.go
        ├── scanner_test.go
        ├── session_identity_test.go
        ├── settings.go
        ├── share.go
        ├── share_test.go
        ├── sudo_askpass_test.go
        ├── sudo_askpass_unix.go
        ├── sudo_askpass_windows.go
        ├── sudo_cancel_test.go
        ├── sudo_child_env.go
        ├── sudo_child_env_test.go
        ├── sudo_client.go
        ├── sudo_client_platform_unix.go
        ├── sudo_client_platform_windows.go
        ├── sudo_client_windows_test.go
        ├── sudo_dispatcher_unix.go
        ├── sudo_dispatcher_unix_test.go
        ├── sudo_dispatcher_windows.go
        ├── sudo_elevated.go
        ├── sudo_elevated_dispatcher.go
        ├── sudo_elevated_dispatcher_test.go
        ├── sudo_elevated_test.go
        ├── sudo_frame.go
        ├── sudo_ipc_unix.go
        ├── sudo_ipc_windows.go
        ├── sudo_launch_other.go
        ├── sudo_launch_windows.go
        ├── sudo_msg.go
        ├── sudo_notexist_test.go
        ├── sudo_test.go
        ├── sudo_unmount_bsd.go
        ├── sudo_unmount_other.go
        ├── sudo_without_elevation_test.go
        ├── trash.go
        ├── trash_darwin.go
        ├── trash_darwin_test.go
        ├── trash_freedesktop.go
        ├── trash_freedesktop_test.go
        ├── trash_test.go
        ├── trash_windows.go
        ├── trash_windows_posix_test.go
        ├── trash_xdg.go
        ├── trash_xdg_test.go
        ├── unc_smb.go
        ├── unc_smb_test.go
        ├── uri_provider.go
        ├── uri_provider_test.go
        ├── utils.go
        ├── utils_test.go
        ├── vfs.go
        └── vfs_test.go

    344 directories, 4708 files
