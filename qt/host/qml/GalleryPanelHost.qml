import QtQuick
import ZoinGallery 1.0 as ZG

FocusScope {
    id: host

    property int side: 0
    property var panel: ({})
    // Layout-only scene patches arrive independently from the row-free panel
    // snapshot. Keeping this small overlay separate avoids invalidating every
    // panel binding just because the renderer mode changed.
    property var layoutState: null
    property var bridge: null
    property var keySink: null
    property string mouseWheelMode: "gui"
    property var theme: ({})
    property var metrics: ({})
    property var hostCapabilities: ({
        cursor: true,
        open: true,
        selection: true,
        viewer: true
    })
    property bool panelActive: false
    // f4's command line remains authoritative even while Gallery owns the
    // visual focus. Once text has been entered, Return must submit that text
    // through the terminal protocol instead of opening the current tile.
    property bool commandLineHasText: false
    // Fast-find is another authoritative f4 keyboard mode: arrows cycle or
    // leave the search, Space extends it, and Return accepts it.
    property bool fastFindActive: false
    // A printable key is sent to Go before the semantic scene produced from
    // that key can come back over the socket. Keep commander input ownership
    // locally during that gap so a rapidly typed Space/arrow/Insert cannot be
    // mistaken for a Gallery selection/navigation command.
    property bool pendingCommanderInput: false
    property int pendingCommanderInputTimeoutMs: 2000
    // A pointer press moves Gallery's session cursor immediately,
    // while the expensive semantic cursor+activation action is intentionally
    // deferred until mouse-up. Keep that new cursor visible on an inactive
    // side during the round-trip; otherwise mouse-down appears to select
    // nothing (or briefly exposes the panel's previous cursor on activation).
    property bool pendingPointerActivation: false
    property string pendingPointerActivationPanelId: ""
    property int pendingPointerActivationTimeoutMs: 6000
    property real devicePixelRatio: 1.0
    property real defaultListDensity: 22
    // GalleryViewer drives these only while its shared thumbnail transition is
    // active. Keeping the state on the persistent host lets the source tile be
    // suppressed without coupling the reusable panel to f4's window hierarchy.
    property bool viewerTransitionActive: false
    property string viewerTransitionEntryId: ""
    // Key ownership is decided on key-down. Scene updates can change the
    // command-line state before key-up (Return clears it immediately), so
    // remember every key sent to Go and always send its matching release.
    property var forwardedKeysDown: ({})
    // Each visual side owns exactly one virtualized viewport. Directory and
    // workspace changes replace its native session; they never retain or
    // pre-create another GalleryPanel object tree off screen.
    readonly property var galleryPanel: embeddedGalleryPanel
    property var panelSession: null
    // Reuse one immutable empty overlay so an ordinary directory update does
    // not look like a new quick-search map to the visible delegates.
    readonly property var emptyQuickSearchMatches: ({})

    // The embedding shell uses this presentation-only signal to hand cursor
    // ownership from the formerly active side to this one in the same
    // mouse-down turn. cursorRequested still owns the single deferred
    // semantic cursor+activation action sent on mouse-up.
    signal pointerActivationPreviewRequested(int side)

    readonly property var effectiveHostCapabilities: ({
        cursor: hostCapabilities.cursor,
        open: hostCapabilities.open,
        selection: hostCapabilities.selection,
        viewer: hostCapabilities.viewer,
        // A populated f4 command line owns panel-navigation/editing keys even
        // though the embedded gallery remains the active-focus surface.
        galleryOwnsPanelInput: !commanderInputActive,
        // The reusable panel must decline Return before the event can bubble
        // to this host; an active-focus child otherwise consumes it first.
        galleryOwnsReturn: !commanderInputActive
    })

    readonly property bool commanderInputActive: commandLineHasText
                                                 || fastFindActive
                                                 || pendingCommanderInput

    readonly property var session: panelSession
    readonly property bool benchmarkTracingEnabled:
        bridge
        && typeof bridge.navigationBenchmarkEnabled !== "undefined"
        && bridge.navigationBenchmarkEnabled === true
    readonly property bool benchmarkTraceOutputEnabled:
        bridge
        && typeof bridge.benchmarkTraceEnabled !== "undefined"
        && bridge.benchmarkTraceEnabled === true
    readonly property double catalogRevision: Number(panel.catalogRevision || 0)
    readonly property var requestedRendererState: rendererStateFor(panel)
    readonly property string requestedPresentationMode:
        String(requestedRendererState.galleryLayoutMode || "masonry")
    readonly property int requestedColumnCount:
        Math.max(2, Math.min(3,
                            Number(requestedRendererState.galleryColumnCount
                                   || 2)))
    readonly property bool densityAdjustable:
        densityIsAdjustable(requestedPresentationMode)
    readonly property real requestedDensity: densityFor(requestedRendererState)
    readonly property real currentDensity:
        galleryPanel && typeof galleryPanel.density !== "undefined"
        ? Number(galleryPanel.density) : requestedDensity
    readonly property real minimumDensity:
        minimumDensityFor(requestedPresentationMode)
    readonly property real maximumDensity:
        maximumDensityFor(requestedPresentationMode)
    readonly property real densityStep:
        densityStepFor(requestedPresentationMode)
    // This is the presentation state already committed to the active native
    // renderer. f4's external Details header consumes the same values, so it
    // can never get one frame ahead of the GalleryPanel body.
    property string appliedPresentationMode: "masonry"
    property var appliedColumnSchema: []
    property string appliedRendererConfigSignature: ""
    property bool applyingRendererState: false

    // Qt converts each semantic panel snapshot into fresh JavaScript array and
    // object wrappers. Compare the small Details schema by value so a cached
    // and fresh snapshot with identical columns keeps the renderer's existing
    // schema identity and does not trigger a redundant layout reset.
    function rendererValuesEqual(left, right) {
        if (left === right)
            return true
        if (left === null || left === undefined
                || right === null || right === undefined)
            return false
        if (typeof left !== typeof right)
            return false
        if (typeof left !== "object")
            return Number.isNaN(left) && Number.isNaN(right)

        const leftIsArray = Array.isArray(left)
        if (leftIsArray !== Array.isArray(right))
            return false
        if (leftIsArray) {
            if (left.length !== right.length)
                return false
            for (let index = 0; index < left.length; ++index) {
                if (!rendererValuesEqual(left[index], right[index]))
                    return false
            }
            return true
        }

        const leftKeys = Object.keys(left)
        const rightKeys = Object.keys(right)
        if (leftKeys.length !== rightKeys.length)
            return false
        for (let keyIndex = 0; keyIndex < leftKeys.length; ++keyIndex) {
            const key = leftKeys[keyIndex]
            if (!Object.prototype.hasOwnProperty.call(right, key)
                    || !rendererValuesEqual(left[key], right[key]))
                return false
        }
        return true
    }

    function panelId(panelState) {
        const semanticPanelId = String(panelState && panelState.id || "")
        if (semanticPanelId !== "")
            return semanticPanelId
        // Protocol scenes always carry a semantic ID. Keep the reusable QML
        // component compatible with lightweight embedders/tests that provide
        // only a non-empty presentation map: those historically consumed the
        // bridge's side session directly. Do not instantiate this fallback for
        // the empty object that exists before a Loader supplies its panel.
        if (!panelState || Object.keys(panelState).length === 0)
            return ""
        return "@legacy-side:" + side
    }

    function legacyPanelId() {
        return "@legacy-side:" + side
    }

    function rendererStateFor(panelState) {
        const base = panelState || ({})
        const overlay = layoutState
        if (!overlay)
            return base
        const baseId = String(base.id || "")
        const overlayId = String(overlay.id || "")
        if (baseId === "" || overlayId !== baseId
                || Number(overlay.catalogRevision || 0)
                   !== Number(base.catalogRevision || 0))
            return base
        return Object.assign({}, base, overlay)
    }

    function presentationModeFor(panelState) {
        return String(panelState && panelState.galleryLayoutMode || "masonry")
    }

    function columnCountFor(panelState) {
        return Math.max(2, Math.min(3,
                    Number(panelState && panelState.galleryColumnCount || 2)))
    }

    function densityIsAdjustable(mode) {
        return mode === "masonry" || mode === "columns"
                || mode === "details" || mode === "grid"
                || mode === "icons"
    }

    function defaultDensityFor(mode) {
        if (mode === "columns" || mode === "details")
            return Math.max(22, defaultListDensity)
        return mode === "grid" ? 160 : mode === "icons" ? 64 : 150
    }

    function minimumDensityFor(mode) {
        if (mode === "columns" || mode === "details")
            return 22
        return mode === "grid" ? 96 : mode === "icons" ? 18 : 30
    }

    function maximumDensityFor(mode) {
        if (mode === "columns" || mode === "details")
            return 72
        return mode === "grid" ? 320 : mode === "icons" ? 256 : 500
    }

    function densityStepFor(mode) {
        return mode === "columns" || mode === "details" ? 2
             : mode === "grid" ? 8 : mode === "icons" ? 2 : 20
    }

    function boundedDensityFor(mode, supplied) {
        const numeric = Number(supplied)
        const value = Number.isFinite(numeric) && numeric > 0
                ? Math.round(numeric) : defaultDensityFor(mode)
        return Math.max(minimumDensityFor(mode),
                        Math.min(maximumDensityFor(mode), value))
    }

    function densityFor(panelState) {
        const mode = presentationModeFor(panelState)
        return boundedDensityFor(
                    mode, panelState && panelState.galleryDensity || 0)
    }

    function presentationDensitiesFor(panelState) {
        const supplied = panelState && panelState.galleryDensities
                ? panelState.galleryDensities : ({})
        const result = ({
            "masonry": boundedDensityFor("masonry", supplied.masonry),
            "columns": boundedDensityFor("columns", supplied.columns),
            "details": boundedDensityFor("details", supplied.details),
            "grid": boundedDensityFor("grid", supplied.grid),
            "icons": boundedDensityFor("icons", supplied.icons)
        })
        const currentMode = presentationModeFor(panelState)
        result[currentMode] = densityFor(panelState)
        return result
    }

    function traceRendererApply(stage, mode) {
        if (!benchmarkTraceOutputEnabled || !bridge
                || typeof bridge.recordBenchmarkStage !== "function")
            return
        bridge.recordBenchmarkStage(side, "renderer.apply." + stage, {
            "rendererMode": mode
        })
    }

    function tracePanelSwitch(stage, panelState) {
        if (!benchmarkTraceOutputEnabled || !bridge
                || typeof bridge.recordBenchmarkStage !== "function")
            return
        bridge.recordBenchmarkStage(side, "panel.switch." + stage, {
            "panelId": panelId(panelState),
            "catalogRevision": Number(panelState
                                      && panelState.catalogRevision || 0)
        })
    }

    function rendererConfigSignature(panelState) {
        const state = panelState || ({})
        const densities = presentationDensitiesFor(state)
        return [presentationModeFor(state),
                columnCountFor(state), densityFor(state),
                state.separateFileExtensions === true ? 1 : 0,
                densities.masonry, densities.columns, densities.details,
                densities.grid, densities.icons,
                JSON.stringify(state.galleryColumns || [])].join("|")
    }

    function applyRendererStateTo(target, panelState, publishState) {
        if (!target)
            return
        const state = panelState || ({})
        const mode = presentationModeFor(state)
        const columnCount = columnCountFor(state)
        const density = densityFor(state)
        const nextPresentationDensities = presentationDensitiesFor(state)
        const nextColumnSchema = state.galleryColumns || []
        const modeChanged = typeof target.presentationMode !== "undefined"
                && target.presentationMode !== mode
        const columnCountChanged = typeof target.columnCount !== "undefined"
                && Number(target.columnCount) !== columnCount
        const densityChanged = typeof target.density !== "undefined"
                && Math.abs(Number(target.density) - density) > 0.0001
        const hostModeChanged = publishState === true
                && appliedPresentationMode !== mode
        const needsLayoutTransaction = modeChanged || columnCountChanged
                || densityChanged || hostModeChanged
        let transactionStarted = false
        applyingRendererState = true
        try {
            if (needsLayoutTransaction)
                traceRendererApply("begin", mode)
            if (needsLayoutTransaction
                    && typeof target.beginPresentationStateUpdate
                            === "function") {
                target.beginPresentationStateUpdate(
                            modeChanged || hostModeChanged)
                transactionStarted = true
            }
            if (needsLayoutTransaction)
                traceRendererApply("transaction-started", mode)
            if (publishState === true) {
                appliedPresentationMode = mode
                if (!rendererValuesEqual(appliedColumnSchema,
                                         nextColumnSchema)) {
                    appliedColumnSchema = nextColumnSchema
                }
                appliedRendererConfigSignature =
                        rendererConfigSignature(state)
            }
            // The helper returns a fresh JavaScript object. Assign it only
            // when a numeric value actually changed; GalleryPanel treats a
            // new object identity as a density update and synchronously
            // resynchronizes/prewarms every presentation cache.
            if (typeof target.presentationDensities !== "undefined"
                    && !rendererValuesEqual(target.presentationDensities,
                                             nextPresentationDensities)) {
                target.presentationDensities = nextPresentationDensities
            }
            if (needsLayoutTransaction)
                traceRendererApply("host-state-published", mode)
            // Details delegates must see their final schema and filename
            // policy during the one native rewrap which creates them.
            if (typeof target.columnSchema !== "undefined"
                    && !rendererValuesEqual(target.columnSchema,
                                             nextColumnSchema)) {
                target.columnSchema = nextColumnSchema
            }
            if (typeof target.separateFileExtensions !== "undefined") {
                target.separateFileExtensions =
                        state.separateFileExtensions === true
            }
            if (needsLayoutTransaction)
                traceRendererApply("schema-applied", mode)
            if (modeChanged)
                target.presentationMode = mode
            if (needsLayoutTransaction)
                traceRendererApply("mode-applied", mode)
            if (columnCountChanged)
                target.columnCount = columnCount
            // setPresentationMode() restores that mode's remembered density.
            // Apply the semantic value again after a mode switch so every
            // presentation receives its current saved zoom in this transaction.
            if (densityChanged || modeChanged)
                target.density = density
            if (needsLayoutTransaction)
                traceRendererApply("density-applied", mode)
        } finally {
            if (needsLayoutTransaction)
                traceRendererApply("transaction-ending", mode)
            if (transactionStarted) {
                target.endPresentationStateUpdate(
                            modeChanged || hostModeChanged)
            }
            if (needsLayoutTransaction)
                traceRendererApply("end", mode)
            applyingRendererState = false
        }
    }

    function sessionForPanelState(panelState) {
        if (!bridge)
            return null
        // A side owns exactly one virtualized model. Directory and workspace
        // changes replace its small materialized row window in place; QML
        // never retains complete panel sessions behind semantic identities.
        return bridge.sessionForSide(side)
    }

    function refreshPanelSession() {
        tracePanelSwitch("begin", panel)
        const exactSession = sessionForPanelState(panel)
        tracePanelSwitch("session-resolved", panel)
        if (panelSession !== exactSession)
            panelSession = exactSession
        tracePanelSwitch("session-applied", panel)
        return exactSession
    }

    function previewDensity(value) {
        if (!densityAdjustable || !galleryPanel
                || typeof galleryPanel.density === "undefined")
            return
        galleryPanel.density = Math.max(minimumDensity,
            Math.min(maximumDensity, Number(value)))
    }

    function commitDensity(value) {
        if (!densityAdjustable)
            return
        const normalized = Math.round(Math.max(minimumDensity,
            Math.min(maximumDensity, Number(value))))
        previewDensity(normalized)
        if (bridge && normalized !== requestedDensity)
            bridge.requestGalleryDensity(side, requestedPresentationMode,
                                         normalized)
    }

    function applyRendererState() {
        applyRendererStateTo(galleryPanel, requestedRendererState, true)
    }

    function forwardBenchmarkStage(stage, metadata) {
        if (!benchmarkTracingEnabled || !bridge
                || typeof bridge.recordBenchmarkStage !== "function")
            return
        var fields = ({})
        if (metadata) {
            const keys = Object.keys(metadata)
            for (let keyIndex = 0; keyIndex < keys.length; ++keyIndex)
                fields[keys[keyIndex]] = metadata[keys[keyIndex]]
        }
        fields.hostSide = side
        fields.hostPanelPath = String(panel.path || "")
        fields.hostPanelLoading = panel.loading === true
        fields.hostCatalogRevision = Number(panel.catalogRevision || 0)
        fields.hostCursorEntryId = String(panel.cursorEntryId || "")
        fields.hostCursorIndex = Number(panel.cursor === undefined
                                        ? -1 : panel.cursor)
        fields.hostPresentationMode = requestedPresentationMode
        bridge.recordBenchmarkStage(side, String(stage || "unknown"), fields)
    }

    function forwardHostBenchmarkState() {
        if (!benchmarkTracingEnabled || !galleryPanel)
            return
        const state = typeof galleryPanel.benchmarkState === "function"
                ? galleryPanel.benchmarkState({}) : ({})
        forwardBenchmarkStage("host.panel.changed", state)
    }

    onPanelChanged: {
        if (pendingPointerActivation
                && pendingPointerActivationPanelId !== panelId(panel))
            finishPendingPointerActivation()
        refreshPanelSession()
        const nextRendererState = rendererStateFor(panel)
        if (rendererConfigSignature(nextRendererState)
                !== appliedRendererConfigSignature) {
            applyRendererStateTo(galleryPanel, nextRendererState, true)
        }
        if (benchmarkTracingEnabled)
            Qt.callLater(forwardHostBenchmarkState)
    }
    onBridgeChanged: refreshPanelSession()
    onSideChanged: refreshPanelSession()
    // QML invalidates requestedRendererState after emitting layoutState's
    // change signal. Read the new overlay directly here; consuming the bound
    // property can otherwise apply the previous mode and make every shortcut
    // appear one transition late.
    onLayoutStateChanged: applyRendererStateTo(
                              galleryPanel, rendererStateFor(panel), true)
    onDefaultListDensityChanged: applyRendererState()
    Component.onCompleted: {
        refreshPanelSession()
        applyRendererState()
    }

    function currentItemImageGeometry(targetItem) {
        if (!targetItem || !galleryPanel
                || typeof galleryPanel.currentItemImageGeometry !== "function")
            return Qt.rect(0, 0, 0, 0)
        return galleryPanel.currentItemImageGeometry(targetItem)
    }

    function currentItemImageSource() {
        if (!galleryPanel
                || typeof galleryPanel.currentItemImageSource !== "function")
            return ""
        return galleryPanel.currentItemImageSource()
    }

    onPanelActiveChanged: {
        if (panelActive)
            finishPendingPointerActivation()
        else
            finishPendingCommanderInput()
    }

    onCommandLineHasTextChanged: {
        if (commandLineHasText)
            acknowledgePendingCommanderInput()
    }

    onFastFindActiveChanged: {
        if (fastFindActive)
            acknowledgePendingCommanderInput()
    }

    function hasCommanderTextModifiers(modifiers) {
        // Ctrl/Meta combinations are commander shortcuts, not text entry.
        // Alt+printable is included because it starts f4 fast-find.
        return !(modifiers & (Qt.ControlModifier | Qt.MetaModifier))
    }

    function hasPrintableText(text) {
        if (!text || text.length === 0)
            return false
        for (var index = 0; index < text.length; ++index) {
            var code = text.charCodeAt(index)
            if (code >= 0x20 && code !== 0x7f
                    && !(code >= 0x80 && code <= 0x9f))
                return true
        }
        return false
    }

    function noteForwardedText(text, modifiers) {
        // The optimistic latch only bridges the first idle-to-commander scene
        // transition. Re-arming it while Go already reports an authoritative
        // command/fast-find mode would outlive the final Backspace that empties
        // that mode and temporarily steal Gallery's idle Space/arrows.
        if (commandLineHasText || fastFindActive
                || !text || text.length === 0
                || !hasCommanderTextModifiers(modifiers))
            return
        pendingCommanderInput = true
        // A test/embedder may change the timeout immediately before the first
        // character. Apply it synchronously before restart rather than waiting
        // for the Timer binding to be reevaluated on a later QML turn.
        pendingCommanderInputTimer.interval = Math.max(
                    1, pendingCommanderInputTimeoutMs)
        pendingCommanderInputTimer.restart()
    }

    function noteForwardedKey(key, text, modifiers) {
        if (commandLineHasText || fastFindActive
                || !hasCommanderTextModifiers(modifiers))
            return
        // macOS can deliver automation/native key presses with a valid Qt key
        // code but an empty event.text. This is called only after the host has
        // decided to forward the key, so Space remains Gallery-owned while
        // idle but safely extends an already pending command/fast-find input.
        if (hasPrintableText(text)
                || (key >= Qt.Key_Space && key <= Qt.Key_AsciiTilde)) {
            pendingCommanderInput = true
            pendingCommanderInputTimer.interval = Math.max(
                        1, pendingCommanderInputTimeoutMs)
            pendingCommanderInputTimer.restart()
        }
    }

    function acknowledgePendingCommanderInput() {
        pendingCommanderInput = false
        pendingCommanderInputTimer.stop()
    }

    function finishPendingCommanderInput() {
        pendingCommanderInput = false
        pendingCommanderInputTimer.stop()
    }

    function beginPendingPointerActivation(semanticPanelId) {
        if (panelActive)
            return
        pendingPointerActivationPanelId = String(semanticPanelId || "")
        pendingPointerActivation = true
        pendingPointerActivationTimer.restart()
    }

    function finishPendingPointerActivation() {
        pendingPointerActivation = false
        pendingPointerActivationPanelId = ""
        pendingPointerActivationTimer.stop()
    }

    Timer {
        id: pendingCommanderInputTimer
        interval: Math.max(1, host.pendingCommanderInputTimeoutMs)
        repeat: false
        onTriggered: {
            // Normally the authoritative command-line/fast-find property has
            // acknowledged the input already. Avoid a sticky keyboard mode if
            // Go rejects the text or no scene follows (for example in tests).
            host.pendingCommanderInput = false
        }
    }

    Connections {
        target: host.keySink
        ignoreUnknownSignals: true
        function onCommanderTextInputForwarded(text, modifiers) {
            // Both persistent panel hosts share the hidden grid. Only the
            // active gallery may adopt text sent by that global key sink.
            if (host.panelActive)
                host.noteForwardedText(text, modifiers)
        }
    }

    function ownsZoomShortcut(event) {
        var zoomKey = event.key === Qt.Key_Plus || event.key === Qt.Key_Equal
                || event.key === Qt.Key_Minus || event.key === Qt.Key_0
        if (!zoomKey)
            return false
        var modifiers = event.modifiers
                & (Qt.ShiftModifier | Qt.ControlModifier
                   | Qt.AltModifier | Qt.MetaModifier)
        var control = Boolean(modifiers & Qt.ControlModifier)
        var meta = Boolean(modifiers & Qt.MetaModifier)
        if (control === meta || (modifiers & Qt.AltModifier))
            return false
        if (modifiers & Qt.ShiftModifier)
            return event.key === Qt.Key_Plus || event.key === Qt.Key_Equal
        return true
    }

    function handleZoom(event) {
        if (!galleryPanel || !ownsZoomShortcut(event))
            return false
        if (event.key === Qt.Key_0) {
            const defaultDensity = defaultDensityFor(
                        requestedPresentationMode)
            if (typeof galleryPanel.resetDensity === "function")
                galleryPanel.resetDensity(defaultDensity)
            else
                commitDensity(defaultDensity)
            return true
        }

        const zoomIn = event.key === Qt.Key_Plus
                || event.key === Qt.Key_Equal
        const zoomOut = event.key === Qt.Key_Minus
        if (!zoomIn && !zoomOut)
            return false

        // GalleryPanel delegates stepping to MasonryLayout. Grid and Icons
        // cross one cell-count interval; compact text modes change row pitch
        // by their two-pixel step.
        if (typeof galleryPanel.stepDensity === "function") {
            galleryPanel.stepDensity(zoomIn)
            return true
        }

        // Compatibility with an older reusable renderer. Production uses the
        // discrete path above; this fallback merely preserves safe local zoom
        // for lightweight test/custom embedders.
        const step = densityStep
        const currentValue = typeof galleryPanel.density !== "undefined"
                ? galleryPanel.density : galleryPanel.thumbnailHeight
        const current = Math.max(minimumDensity, Math.min(maximumDensity,
                              Math.round(Number(currentValue
                                                || requestedDensity))))
        const target = zoomIn ? Math.min(maximumDensity, current + step)
                              : Math.max(minimumDensity, current - step)

        if (target !== current) {
            // Reuse GalleryPanel's pinch commit path: it clamps using the
            // renderer's own per-mode contract, refreshes the visible decode
            // plan once, and emits one final persistence intent to Go.
            if (typeof galleryPanel.beginThumbnailPinch === "function"
                    && typeof galleryPanel.updateThumbnailPinch === "function"
                    && typeof galleryPanel.finishThumbnailPinch === "function") {
                galleryPanel.beginThumbnailPinch()
                galleryPanel.updateThumbnailPinch(target / current)
                galleryPanel.finishThumbnailPinch()
            } else {
                // Compatibility with the pre-density reusable module.
                galleryPanel.thumbnailHeight = target
            }
        }
        return true
    }

    function isPasteShortcut(event) {
        var modifiers = event.modifiers
                & (Qt.ShiftModifier | Qt.ControlModifier
                   | Qt.AltModifier | Qt.MetaModifier)
        var commandModifier = modifiers
                & (Qt.ControlModifier | Qt.MetaModifier)
        if (event.key === Qt.Key_V && commandModifier
                && !(modifiers & (Qt.AltModifier | Qt.ShiftModifier)))
            return true
        return event.key === Qt.Key_Insert
                && modifiers === Qt.ShiftModifier
    }

    function rememberForwardedKey(key) {
        var keys = Object.assign({}, forwardedKeysDown)
        keys[String(key)] = true
        forwardedKeysDown = keys
    }

    function takeForwardedKey(key) {
        var name = String(key)
        if (!forwardedKeysDown[name])
            return false
        var keys = Object.assign({}, forwardedKeysDown)
        delete keys[name]
        forwardedKeysDown = keys
        return true
    }

    function forwardQtKey(event, down) {
        if (!keySink)
            return
        // Preserve the platform's repeat bit through the semantic Gallery
        // surface. VtuiGridItem suppresses Qt's synthetic repeat release but
        // forwards each repeat press with repeat=true to Go.
        if (typeof keySink.sendQtKeyEvent === "function") {
            keySink.sendQtKeyEvent(event.key, event.text, down,
                                   event.modifiers, event.nativeScanCode,
                                   event.isAutoRepeat)
        } else {
            // Compatibility for lightweight embedders and QML test sinks.
            keySink.sendQtKey(event.key, event.text, down,
                              event.modifiers, event.nativeScanCode)
        }
    }

    Timer {
        id: pendingPointerActivationTimer
        interval: Math.max(1, host.pendingPointerActivationTimeoutMs)
        repeat: false
        onTriggered: {
            // The active-scene acknowledgement normally clears this latch.
            // Drop it eventually if the action is rejected or the host goes
            // away, so an inactive panel cannot keep a stale cursor forever.
            host.finishPendingPointerActivation()
        }
    }

    function forwardConsoleWheel(x, y, angleDeltaY, modifiers) {
        if (!galleryPanel || !host.keySink
                || typeof host.keySink.sendQtWheelAt !== "function")
            return
        const point = galleryPanel.mapToItem(host.keySink, x, y)
        host.keySink.sendQtWheelAt(point.x, point.y, angleDeltaY, modifiers)
    }

    function forwardConsoleMouseButton(x, y, button, down, modifiers) {
        if (!galleryPanel || !host.keySink
                || typeof host.keySink.sendQtMouseAt !== "function")
            return
        const point = galleryPanel.mapToItem(host.keySink, x, y)
        host.keySink.sendQtMouseAt(point.x, point.y, button, down, modifiers)
    }

    function commanderOwnsKey(event) {
        // Bare Shift owns the lifetime of Zoin Gallery's range-selection
        // preview. Let the child observe its press/release instead of sending
        // the modifier itself to the commander grid.
        if (event.key === Qt.Key_Shift)
            return false
        // Conventional gallery zoom remains local to the gallery.
        if (event.key === Qt.Key_Plus || event.key === Qt.Key_Equal
                || event.key === Qt.Key_Minus || event.key === Qt.Key_0)
            return !ownsZoomShortcut(event)

        // f4's command line remains the keyboard target while Gallery keeps
        // visual focus. In particular, Space is command text and arrows edit
        // or browse the command/history; none may mutate the gallery catalog.
        if (commanderInputActive)
            return true

        var routingModifiers = event.modifiers
                & (Qt.ShiftModifier | Qt.ControlModifier
                   | Qt.AltModifier | Qt.MetaModifier)
        var spatialKey = event.key === Qt.Key_Left || event.key === Qt.Key_Right
                || event.key === Qt.Key_Up || event.key === Qt.Key_Down
        var pageKey = event.key === Qt.Key_PageUp
                || event.key === Qt.Key_PageDown
        var edgeKey = event.key === Qt.Key_Home || event.key === Qt.Key_End
        // Shift+spatial navigation preserves normal f4 selection semantics,
        // but uses the masonry geometry. Ctrl/Alt combinations remain f4
        // commander shortcuts (panel resizing, history, etc.).
        if ((spatialKey || pageKey || edgeKey)
                && (routingModifiers === Qt.NoModifier
                    || routingModifiers === Qt.ShiftModifier))
            return false
        if (routingModifiers === Qt.NoModifier
                && (event.key === Qt.Key_Return || event.key === Qt.Key_Enter))
            return commandLineHasText
        if (routingModifiers === Qt.NoModifier
                && (event.key === Qt.Key_Space || event.key === Qt.Key_Insert))
            return false

        // Text/fast-find input, navigation outside the masonry contract, and
        // commander shortcuts remain owned by f4.
        return true
    }

    Keys.priority: Keys.BeforeItem
    Keys.onPressed: (event) => {
        if (isPasteShortcut(event)) {
            if (keySink) keySink.sendClipboardPaste()
            event.accepted = true
        } else if (handleZoom(event)) {
            event.accepted = true
        } else if (commanderOwnsKey(event)) {
            forwardQtKey(event, true)
            rememberForwardedKey(event.key)
            // VtuiGridItem emits the same notification synchronously. Keep a
            // direct fallback for lightweight test/custom key sinks that only
            // implement sendQtKey.
            noteForwardedKey(event.key, event.text, event.modifiers)
            if (event.key === Qt.Key_Return || event.key === Qt.Key_Enter
                    || event.key === Qt.Key_Escape)
                finishPendingCommanderInput()
            event.accepted = true
        }
    }
    Keys.onReleased: (event) => {
        if (isPasteShortcut(event)) {
            event.accepted = true
        } else if (forwardedKeysDown[String(event.key)]) {
            forwardQtKey(event, false)
            if (!event.isAutoRepeat)
                takeForwardedKey(event.key)
            event.accepted = true
        } else if (commanderOwnsKey(event)) {
            // A key-down such as Tab/F3 can move focus to another persistent
            // surface before key-up. That new surface has no local press
            // record, but Go must still receive the commander key release.
            forwardQtKey(event, false)
            event.accepted = true
        }
    }

    ZG.GalleryPanel {
        id: embeddedGalleryPanel
        objectName: "embeddedGalleryPanel"
        anchors.fill: parent
        session: host.session
        presentationDensities: ({})
        theme: host.theme
        metrics: host.metrics
        // A 500 ms brick interpolation after an auxiliary metadata batch only
        // makes an already-ready panel look as if it is rebuilding.
        animateLayoutChanges: false
        // Standalone ZoinGallery talks about previewable images. f4 owns a
        // general file catalog, so expose an empty state only after the
        // authoritative folder load has completed.
        emptyStateEnabled: host.session !== null
                           && host.panel.loading !== true
                           && host.panel.catalogProvisional !== true
        emptyStateText: qsTr("Folder is empty")
        scrollBarsReady: host.panel.catalogProvisional !== true
        quickSearchMatches: host.panel.fastFind === true
                            ? (host.panel.fastFindMatches
                               || host.emptyQuickSearchMatches)
                            : host.emptyQuickSearchMatches
        quickSearchMatchColor: {
            const supplied = String(host.panel.fastFindMatchColor || "")
            if (supplied !== "")
                return supplied
            return host.theme && host.theme.quickSearchMatch !== undefined
                    ? host.theme.quickSearchMatch : foregroundColor
        }
        // f4 keeps the Details column header outside the reusable viewport so
        // every unified layout shares one exact geometry and interaction
        // surface.
        showDetailsHeader: false
        hostCapabilities: host.effectiveHostCapabilities
        devicePixelRatio: host.devicePixelRatio
        viewerTransitionActive: host.viewerTransitionActive
        viewerTransitionEntryId: host.viewerTransitionEntryId
        showCursor: host.panelActive
                    || (host.pendingPointerActivation
                        && host.pendingPointerActivationPanelId
                           === host.panelId(host.panel))
        focus: true
        autoFocus: false
        mouseWheelMode: host.mouseWheelMode
        benchmarkTracingEnabled: host.benchmarkTracingEnabled

        onActivateRequested: {
            if (host.bridge && !host.panelActive)
                host.bridge.requestActivate(host.side)
        }
        onCursorRequested: (entryId, index, deferCommit) => {
            if (!host.bridge)
                return
            if (!host.panelActive) {
                host.beginPendingPointerActivation(host.panelId(host.panel))
                host.pointerActivationPreviewRequested(host.side)
            }
            host.bridge.requestCursor(
                        host.side, entryId, index, host.catalogRevision,
                        deferCommit,
                        deferCommit
                        && (embeddedGalleryPanel.keyboardShiftSelectionActive
                            || embeddedGalleryPanel.keyboardToggleSelectionActive))
        }
        onOpenRequested: (entryId, index, isImage, autoRepeat) => {
            if (host.bridge) {
                host.bridge.requestOpen(host.side, entryId, index, isImage,
                                        host.catalogRevision, autoRepeat)
            }
        }
        onSelectionRequested: (mode, entryIds) => {
            if (host.bridge) {
                host.bridge.requestSelection(host.side, mode, entryIds,
                                             host.catalogRevision)
            }
        }
        onSelectionTransactionRequested: (changes, cursorEntryId,
                                           cursorIndex) => {
            if (host.bridge) {
                host.bridge.requestSelectionTransaction(
                            host.side, changes, cursorEntryId, cursorIndex,
                            host.catalogRevision)
            }
        }
        onMetadataVisibleRangeChanged: (firstRow, lastRow) => {
            if (host.bridge) {
                host.bridge.reportMetadataVisibleRange(
                            host.side, firstRow, lastRow,
                            host.catalogRevision)
            }
        }
        onBenchmarkStage: (stage, metadata) => {
            host.forwardBenchmarkStage(stage, metadata)
        }
        onConsoleWheelRequested: (x, y, angleDeltaY, modifiers) => {
            host.forwardConsoleWheel(x, y, angleDeltaY, modifiers)
        }
        onConsoleMouseButtonRequested: (x, y, button, down, modifiers) => {
            host.forwardConsoleMouseButton(x, y, button, down, modifiers)
        }
    }

    Connections {
        target: host.galleryPanel
        ignoreUnknownSignals: true

        function onPresentationModeChanged() {
            if (host.applyingRendererState || !host.bridge
                    || typeof host.galleryPanel.presentationMode === "undefined")
                return
            const mode = String(host.galleryPanel.presentationMode)
            if (mode !== host.requestedPresentationMode) {
                host.bridge.requestGalleryLayout(host.side, mode,
                                                 Number(host.galleryPanel.columnCount || 0))
            }
        }

        function onColumnCountChanged() {
            if (host.applyingRendererState || !host.bridge
                    || host.requestedPresentationMode !== "columns")
                return
            const columns = Number(host.galleryPanel.columnCount || 0)
            if (columns !== host.requestedColumnCount) {
                host.bridge.requestGalleryLayout(host.side, "columns", columns)
            }
        }

        function onDensityChangeRequested(mode, density, finalChange) {
            if (host.applyingRendererState || !host.bridge
                    || finalChange !== true || !host.densityAdjustable)
                return
            const normalizedMode = String(mode || host.requestedPresentationMode)
            if (!host.densityIsAdjustable(normalizedMode))
                return
            const normalizedDensity = Math.round(Number(density || 0))
            if (normalizedDensity > 0
                    && (normalizedMode !== host.requestedPresentationMode
                        || normalizedDensity !== host.requestedDensity)) {
                host.bridge.requestGalleryDensity(
                            host.side, normalizedMode, normalizedDensity)
            }
        }

        function onSortRequested(sortMode, contextMenu) {
            if (!host.bridge)
                return
            host.bridge.requestSort(host.side, String(sortMode || ""),
                                    contextMenu === true)
        }
    }
}
