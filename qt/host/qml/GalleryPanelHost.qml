import QtQuick
import ZoinGallery 1.0 as ZG

FocusScope {
    id: host

    property int side: 0
    property var panel: ({})
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
    // A GalleryPanel owns its viewport delegates and scroll/layout caches.
    // Retain one complete QML view per semantic panel identity so switching a
    // workspace never rebinds a 5k-row session into an already-painted view.
    // Inactive views stay laid out at opacity zero and become visible through
    // one atomic property change.
    property var galleryPanel: null
    property var galleryPanelsById: ({})
    property string activeGalleryPanelId: ""
    property bool galleryPanelContainerReady: false

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

    // Sessions are retained by semantic panel identity, not by visual side.
    // While an inactive workspace is prepared, the currently painted host
    // therefore keeps its old model. Changing both panel objects later swaps
    // to two already-complete sessions in one QML transaction.
    readonly property var session: bridge
        ? (typeof bridge.sessionForPanel === "function"
           ? bridge.sessionForPanel(String(panel.id || ""), side)
           : bridge.sessionForSide(side))
        : null
    readonly property bool benchmarkTracingEnabled:
        bridge
        && typeof bridge.navigationBenchmarkEnabled !== "undefined"
        && bridge.navigationBenchmarkEnabled === true
    readonly property double catalogRevision: Number(panel.catalogRevision || 0)
    readonly property string requestedPresentationMode:
        String(panel.galleryLayoutMode || "masonry")
    readonly property int requestedColumnCount:
        Math.max(2, Math.min(3, Number(panel.galleryColumnCount || 2)))
    readonly property real requestedDensity: {
        const supplied = Number(panel.galleryDensity || 0)
        if (supplied > 0)
            return Math.round(supplied)
        if (requestedPresentationMode === "columns"
                || requestedPresentationMode === "details")
            return Math.max(22, defaultListDensity)
        return requestedPresentationMode === "grid" ? 160
             : requestedPresentationMode === "icons" ? 64 : 150
    }
    readonly property real currentDensity:
        galleryPanel && typeof galleryPanel.density !== "undefined"
        ? Number(galleryPanel.density) : requestedDensity
    readonly property real minimumDensity:
        requestedPresentationMode === "columns"
        || requestedPresentationMode === "details" ? 22
        : requestedPresentationMode === "grid" ? 96
        : requestedPresentationMode === "icons" ? 18 : 30
    readonly property real maximumDensity:
        requestedPresentationMode === "columns"
        || requestedPresentationMode === "details" ? 72
        : requestedPresentationMode === "grid" ? 320
        : requestedPresentationMode === "icons" ? 256 : 500
    readonly property real densityStep:
        requestedPresentationMode === "columns"
        || requestedPresentationMode === "details" ? 2
        : requestedPresentationMode === "grid" ? 8
        : requestedPresentationMode === "icons" ? 2 : 20
    // This is the presentation state already committed to the active native
    // renderer. f4's external Details header consumes the same values, so it
    // can never get one frame ahead of the retained GalleryPanel body.
    property string appliedPresentationMode: "masonry"
    property var appliedColumnSchema: []
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

    function presentationModeFor(panelState) {
        return String(panelState && panelState.galleryLayoutMode || "masonry")
    }

    function columnCountFor(panelState) {
        return Math.max(2, Math.min(3,
                    Number(panelState && panelState.galleryColumnCount || 2)))
    }

    function densityFor(panelState) {
        const mode = presentationModeFor(panelState)
        const supplied = Number(panelState && panelState.galleryDensity || 0)
        if (supplied > 0)
            return Math.round(supplied)
        if (mode === "columns" || mode === "details")
            return Math.max(22, defaultListDensity)
        return mode === "grid" ? 160 : mode === "icons" ? 64 : 150
    }

    function applyRendererStateTo(target, panelState, publishState) {
        if (!target)
            return
        const state = panelState || ({})
        const mode = presentationModeFor(state)
        const columnCount = columnCountFor(state)
        const density = densityFor(state)
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
            if (needsLayoutTransaction
                    && typeof target.beginPresentationStateUpdate
                            === "function") {
                target.beginPresentationStateUpdate(
                            modeChanged || hostModeChanged)
                transactionStarted = true
            }
            if (publishState === true) {
                appliedPresentationMode = mode
                if (!rendererValuesEqual(appliedColumnSchema,
                                         nextColumnSchema)) {
                    appliedColumnSchema = nextColumnSchema
                }
            }
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
            if (modeChanged)
                target.presentationMode = mode
            if (columnCountChanged)
                target.columnCount = columnCount
            if (densityChanged)
                target.density = density
        } finally {
            if (transactionStarted) {
                target.endPresentationStateUpdate(
                            modeChanged || hostModeChanged)
            }
            applyingRendererState = false
        }
    }

    function sessionForPanelId(semanticPanelId) {
        if (!bridge)
            return null
        if (semanticPanelId === legacyPanelId())
            return bridge.sessionForSide(side)
        return typeof bridge.sessionForPanel === "function"
                ? bridge.sessionForPanel(semanticPanelId, side)
                : bridge.sessionForSide(side)
    }

    function ensureGalleryPanel(panelState, activate) {
        if (!galleryPanelContainerReady)
            return null
        const semanticPanelId = panelId(panelState)
        if (semanticPanelId === "")
            return null
        const exactSession = sessionForPanelId(semanticPanelId)
        // A retained viewport keeps its model pointer for its whole lifetime.
        // Do not manufacture one around a mutable side fallback while the
        // identity cache is still being prepared.
        if (!exactSession)
            return null

        var view = galleryPanelsById[semanticPanelId]
        if (!view) {
            view = galleryPanelComponent.createObject(galleryPanelContainer, {
                "semanticPanelId": semanticPanelId,
                "cachedPanel": panelState,
                "fixedSession": exactSession
            })
            if (!view)
                return null
            const next = Object.assign({}, galleryPanelsById)
            next[semanticPanelId] = view
            galleryPanelsById = next
        } else {
            view.cachedPanel = panelState
            // Recovery for a viewport constructed by an older/late startup
            // path.  Normal cache hits keep the same QObject and never rebind.
            if (view.fixedSession !== exactSession)
                view.fixedSession = exactSession
        }
        applyRendererStateTo(view, panelState, activate === true)
        if (activate === true) {
            activeGalleryPanelId = semanticPanelId
            galleryPanel = view
        }
        return view
    }

    function warmCachedGalleryPanels() {
        if (!bridge
                || typeof bridge.cachedPanelPresentations !== "function")
            return
        const cached = bridge.cachedPanelPresentations(side) || []
        for (let index = 0; index < cached.length; ++index)
            ensureGalleryPanel(cached[index], false)
    }

    function previewDensity(value) {
        if (!galleryPanel || typeof galleryPanel.density === "undefined")
            return
        galleryPanel.density = Math.max(minimumDensity,
            Math.min(maximumDensity, Number(value)))
    }

    function commitDensity(value) {
        const normalized = Math.round(Math.max(minimumDensity,
            Math.min(maximumDensity, Number(value))))
        previewDensity(normalized)
        if (bridge && normalized !== requestedDensity)
            bridge.requestGalleryDensity(side, requestedPresentationMode,
                                         normalized)
    }

    function applyRendererState() {
        applyRendererStateTo(galleryPanel, panel, true)
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
        const view = ensureGalleryPanel(panel, true)
        if (!view) {
            activeGalleryPanelId = ""
            galleryPanel = null
            appliedPresentationMode = "masonry"
            appliedColumnSchema = []
        }
        if (benchmarkTracingEnabled)
            Qt.callLater(forwardHostBenchmarkState)
    }
    onDefaultListDensityChanged: applyRendererState()
    Component.onCompleted: {
        galleryPanelContainerReady = true
        warmCachedGalleryPanels()
        ensureGalleryPanel(panel, true)
    }

    Connections {
        target: host.bridge
        ignoreUnknownSignals: true
        function onPanelCachePrepared(panelSide, panelState) {
            if (Number(panelSide) !== host.side)
                return
            const preparedId = host.panelId(panelState)
            const activate = preparedId !== ""
                    && preparedId === host.panelId(host.panel)
            host.ensureGalleryPanel(panelState, activate)
        }
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
        if (!panelActive)
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
        const mode = requestedPresentationMode
        const minimum = mode === "columns" || mode === "details" ? 22
                      : mode === "grid" ? 96
                      : mode === "icons" ? 18 : 30
        const maximum = mode === "columns" || mode === "details" ? 72
                      : mode === "grid" ? 320
                      : mode === "icons" ? 256 : 500
        const defaultDensity = mode === "columns" || mode === "details"
                ? Math.max(22, Math.round(defaultListDensity))
                : mode === "grid" ? 160
                : mode === "icons" ? 64 : 150
        const step = mode === "columns" || mode === "details" ? 2
                   : mode === "grid" ? 8
                   : mode === "icons" ? 2 : 20
        const currentValue = typeof galleryPanel.density !== "undefined"
                ? galleryPanel.density : galleryPanel.thumbnailHeight
        const current = Math.max(minimum, Math.min(maximum,
                              Math.round(Number(currentValue
                                                || defaultDensity))))
        var target = current
        if (event.key === Qt.Key_Plus || event.key === Qt.Key_Equal) {
            target = Math.min(maximum, current + step)
        } else if (event.key === Qt.Key_Minus) {
            target = Math.max(minimum, current - step)
        } else if (event.key === Qt.Key_0) {
            target = Math.max(minimum, Math.min(maximum, defaultDensity))
        } else {
            return false
        }

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

    Item {
        id: galleryPanelContainer
        anchors.fill: parent
    }

    Component {
        id: galleryPanelComponent

        ZG.GalleryPanel {
            id: cachedGalleryPanel
            property string semanticPanelId: ""
            property var cachedPanel: ({})
            property var fixedSession: null
            readonly property bool activeForHost:
                host.activeGalleryPanelId === semanticPanelId

            objectName: activeForHost
                        ? "embeddedGalleryPanel"
                        : "cachedGalleryPanel-" + semanticPanelId
            anchors.fill: parent
            z: activeForHost ? 1 : 0
            visible: true
            opacity: activeForHost ? 1 : 0
            enabled: activeForHost
            session: fixedSession
            theme: host.theme
            metrics: host.metrics
            // f4 receives exact metadata incrementally and swaps retained
            // workspaces atomically. A 500 ms brick interpolation after an
            // auxiliary metadata batch only makes a ready panel look as if it
            // is rebuilding, so embedded panels keep deterministic geometry.
            animateLayoutChanges: false
            // Standalone ZoinGallery talks about previewable images.  f4 owns
            // a general file catalog, so expose an empty state only after the
            // authoritative folder load has completed.
            emptyStateEnabled: activeForHost
                               && fixedSession !== null
                               && cachedPanel.loading !== true
                               && cachedPanel.catalogProvisional !== true
            emptyStateText: qsTr("Folder is empty")
            // Match spans are transient. A compact state merge may briefly
            // retain an older map while fast-find is being closed; never
            // paint it on a hidden/cached workspace.
            quickSearchMatches: activeForHost
                                && host.panel
                                && host.panel.fastFind === true
                                ? (host.panel.fastFindMatches || ({})) : ({})
            quickSearchMatchColor: {
                const supplied = activeForHost
                        ? String(host.panel.fastFindMatchColor || "") : ""
                if (supplied !== "")
                    return supplied
                return host.theme
                        && host.theme.quickSearchMatch !== undefined
                        ? host.theme.quickSearchMatch : foregroundColor
            }
            // f4 keeps the Details column header outside the reusable
            // viewport so every unified layout shares one exact geometry and
            // interaction surface.
            showDetailsHeader: false
            hostCapabilities: host.effectiveHostCapabilities
            devicePixelRatio: host.devicePixelRatio
            viewerTransitionActive: activeForHost
                                    && host.viewerTransitionActive
            viewerTransitionEntryId: activeForHost
                                     ? host.viewerTransitionEntryId : ""
            // Keep an off-screen workspace in the exact cursor appearance it
            // will have when revealed. Lucide image-provider URLs include the
            // semantic tint; hiding the cursor here used to change that URL on
            // activation and made the selected icon arrive one frame late.
            showCursor: activeForHost ? host.panelActive
                                      : cachedPanel.active === true
            // Only the currently exposed cached viewport participates in the
            // host FocusScope. The others remain polished but inert.
            focus: activeForHost
            autoFocus: false
            mouseWheelMode: host.mouseWheelMode
            benchmarkTracingEnabled: activeForHost
                                     && host.benchmarkTracingEnabled

            Component.onCompleted: {
                host.applyRendererStateTo(cachedGalleryPanel,
                                          cachedGalleryPanel.cachedPanel,
                                          false)
            }
            onCachedPanelChanged: {
                if (!activeForHost) {
                    host.applyRendererStateTo(
                                cachedGalleryPanel,
                                cachedGalleryPanel.cachedPanel,
                                false)
                }
            }
            onActiveForHostChanged: {
                if (!activeForHost)
                    return
                host.galleryPanel = cachedGalleryPanel
            }

            onActivateRequested: {
                if (activeForHost && !host.panelActive)
                    host.bridge.requestActivate(host.side)
            }
            onCursorRequested: (entryId, index, deferCommit) => {
                if (activeForHost) {
                    host.bridge.requestCursor(host.side, entryId, index,
                                              host.catalogRevision,
                                              deferCommit)
                }
            }
            onOpenRequested: (entryId, index, isImage, autoRepeat) => {
                if (activeForHost) {
                    host.bridge.requestOpen(host.side, entryId, index, isImage,
                                            host.catalogRevision, autoRepeat)
                }
            }
            onSelectionRequested: (mode, entryIds) => {
                if (activeForHost) {
                    host.bridge.requestSelection(host.side, mode, entryIds,
                                                 host.catalogRevision)
                }
            }
            onMetadataVisibleRangeChanged: (firstRow, lastRow) => {
                if (activeForHost && host.bridge) {
                    host.bridge.reportMetadataVisibleRange(
                                host.side, firstRow, lastRow,
                                host.catalogRevision)
                }
            }
            onBenchmarkStage: (stage, metadata) => {
                if (activeForHost)
                    host.forwardBenchmarkStage(stage, metadata)
            }
            onConsoleWheelRequested: (x, y, angleDeltaY, modifiers) => {
                if (activeForHost)
                    host.forwardConsoleWheel(x, y, angleDeltaY, modifiers)
            }
            onConsoleMouseButtonRequested: (x, y, button, down, modifiers) => {
                if (activeForHost) {
                    host.forwardConsoleMouseButton(x, y, button, down,
                                                   modifiers)
                }
            }
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
                    || finalChange !== true)
                return
            const normalizedMode = String(mode || host.requestedPresentationMode)
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
