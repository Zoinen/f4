pragma ComponentBehavior: Bound

import QtQuick
import ZoinGallery 1.0 as ZG

Item {
    id: adapter

    required property int side
    required property var panel
    required property ZG.GalleryPanel galleryPanel
    property var layoutState: null
    property var bridge: null
    property real defaultListDensity: 22

    property var panelSession: null
    property string appliedPresentationMode: "masonry"
    property var appliedColumnSchema: []
    property string appliedRendererConfigSignature: ""
    property bool applyingRendererState: false
    property bool panelPresentationTransactionActive: false
    property bool panelPresentationTransactionSwitchesMode: false

    signal rendererTransactionStateChanged(bool active)

    readonly property bool benchmarkTracingEnabled:
        bridge
        && typeof bridge.navigationBenchmarkEnabled !== "undefined"
        && bridge.navigationBenchmarkEnabled === true
    readonly property bool benchmarkTraceOutputEnabled:
        bridge
        && typeof bridge.benchmarkTraceEnabled !== "undefined"
        && bridge.benchmarkTraceEnabled === true
    readonly property double catalogRevision:
        Number(panel && panel.catalogRevision || 0)
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

    visible: false
    width: 0
    height: 0

    // Qt converts each semantic panel snapshot into fresh JavaScript array and
    // object wrappers. Compare the small renderer contract by value so an
    // equivalent update keeps the native viewport's existing layout identity.
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
        // Production snapshots always carry an ID. Keep the fixture contract
        // without allocating another session for lightweight test embedders.
        if (!panelState || Object.keys(panelState).length === 0)
            return ""
        return "@legacy-side:" + side
    }

    function rendererStateFor(panelState, suppliedLayoutState) {
        const base = panelState || ({})
        const overlay = suppliedLayoutState === undefined
                ? layoutState : suppliedLayoutState
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

    function publishRendererHostState(state, mode, columnSchema) {
        appliedPresentationMode = mode
        if (!rendererValuesEqual(appliedColumnSchema, columnSchema))
            appliedColumnSchema = columnSchema
        appliedRendererConfigSignature = rendererConfigSignature(state)
    }

    function applyRendererSchema(target, state, densities, columnSchema) {
        if (typeof target.presentationDensities !== "undefined"
                && !rendererValuesEqual(target.presentationDensities,
                                         densities)) {
            target.presentationDensities = densities
        }
        if (typeof target.columnSchema !== "undefined"
                && !rendererValuesEqual(target.columnSchema,
                                         columnSchema)) {
            target.columnSchema = columnSchema
        }
        if (typeof target.separateFileExtensions !== "undefined") {
            target.separateFileExtensions =
                    state.separateFileExtensions === true
        }
    }

    function applyRendererGeometry(target, mode, columnCount, density,
                                   modeChanged, columnCountChanged,
                                   densityChanged) {
        if (modeChanged) {
            if (typeof target.applyPresentationMode === "function")
                target.applyPresentationMode(mode)
            else
                target.presentationMode = mode
        }
        if (columnCountChanged)
            target.columnCount = columnCount
        if (densityChanged || modeChanged)
            target.density = density
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
        rendererTransactionStateChanged(true)
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
            if (publishState === true)
                publishRendererHostState(state, mode, nextColumnSchema)
            applyRendererSchema(target, state, nextPresentationDensities,
                                nextColumnSchema)
            if (needsLayoutTransaction)
                traceRendererApply("host-state-published", mode)
            if (needsLayoutTransaction)
                traceRendererApply("schema-applied", mode)
            applyRendererGeometry(target, mode, columnCount, density,
                                  modeChanged, columnCountChanged,
                                  densityChanged)
            if (needsLayoutTransaction)
                traceRendererApply("mode-applied", mode)
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
            rendererTransactionStateChanged(false)
        }
    }

    function sessionForPanelState(panelState) {
        if (!bridge)
            return null
        // Each side owns exactly one bounded virtualized model. A folder or
        // workspace change replaces its materialized window in place.
        return bridge.sessionForSide(side)
    }

    function refreshPanelSession(panelState) {
        const state = panelState === undefined ? panel : panelState
        tracePanelSwitch("begin", state)
        const exactSession = sessionForPanelState(state)
        tracePanelSwitch("session-resolved", state)
        if (panelSession !== exactSession)
            panelSession = exactSession
        tracePanelSwitch("session-applied", state)
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

    function beginPanelPresentationTransaction(
            panelId, catalogRevision, mode, columnCount, density,
            presentationDensities, columns, separateFileExtensions) {
        if (!galleryPanel)
            return
        if (panelPresentationTransactionActive)
            endPanelPresentationTransaction()

        const state = {
            "id": String(panelId || ""),
            "catalogRevision": Number(catalogRevision || 0),
            "galleryLayoutMode": String(mode || "masonry"),
            "galleryColumnCount": Number(columnCount || 2),
            "galleryDensity": Number(density || 0),
            "galleryDensities": presentationDensities || ({}),
            "galleryColumns": columns || [],
            "separateFileExtensions": separateFileExtensions === true
        }
        const targetMode = presentationModeFor(state)
        const targetColumnCount = columnCountFor(state)
        const targetDensity = densityFor(state)
        const modeChanged = galleryPanel.presentationMode !== targetMode
        const columnCountChanged = Number(galleryPanel.columnCount)
                !== targetColumnCount
        const densityChanged = Math.abs(Number(galleryPanel.density)
                                        - targetDensity) > 0.0001
        const hostModeChanged = appliedPresentationMode !== targetMode

        applyingRendererState = true
        rendererTransactionStateChanged(true)
        panelPresentationTransactionSwitchesMode = modeChanged
                || hostModeChanged
        galleryPanel.beginPresentationStateUpdate(
                    panelPresentationTransactionSwitchesMode)
        panelPresentationTransactionActive = true
        publishRendererHostState(state, targetMode,
                                 state.galleryColumns)
        applyRendererSchema(galleryPanel, state,
                            presentationDensitiesFor(state),
                            state.galleryColumns)
        applyRendererGeometry(galleryPanel, targetMode, targetColumnCount,
                              targetDensity, modeChanged,
                              columnCountChanged, densityChanged)
    }

    function endPanelPresentationTransaction() {
        if (!panelPresentationTransactionActive)
            return
        const switchesMode = panelPresentationTransactionSwitchesMode
        panelPresentationTransactionActive = false
        panelPresentationTransactionSwitchesMode = false
        try {
            galleryPanel.endPresentationStateUpdate(switchesMode)
        } finally {
            applyingRendererState = false
            rendererTransactionStateChanged(false)
        }
    }

    // These entry points receive the facade's source values directly. QML
    // evaluates child bindings lazily; routing the source snapshot explicitly
    // preserves the old same-signal-stack renderer transaction contract.
    function synchronizePanel(panelState, rendererOverlay) {
        refreshPanelSession(panelState)
        const nextRendererState = rendererStateFor(panelState, rendererOverlay)
        if (rendererConfigSignature(nextRendererState)
                !== appliedRendererConfigSignature) {
            applyRendererStateTo(galleryPanel, nextRendererState, true)
        }
        if (benchmarkTracingEnabled)
            Qt.callLater(adapter.forwardHostBenchmarkState)
    }

    function synchronizeLayout(panelState, rendererOverlay) {
        applyRendererStateTo(
                    galleryPanel,
                    rendererStateFor(panelState, rendererOverlay), true)
    }

    function forwardBenchmarkStage(stage, metadata) {
        if (!benchmarkTracingEnabled || !bridge
                || typeof bridge.recordBenchmarkStage !== "function")
            return
        const fields = ({})
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

    onBridgeChanged: refreshPanelSession()
    onSideChanged: refreshPanelSession()

    Connections {
        target: adapter.galleryPanel
        ignoreUnknownSignals: true

        function onPresentationModeChanged() {
            if (adapter.applyingRendererState || !adapter.bridge
                    || typeof adapter.galleryPanel.presentationMode
                            === "undefined")
                return
            const mode = String(adapter.galleryPanel.presentationMode)
            if (mode !== adapter.requestedPresentationMode) {
                adapter.bridge.requestGalleryLayout(
                            adapter.side, mode,
                            Number(adapter.galleryPanel.columnCount || 0))
            }
        }

        function onColumnCountChanged() {
            if (adapter.applyingRendererState || !adapter.bridge
                    || adapter.requestedPresentationMode !== "columns")
                return
            const columns = Number(adapter.galleryPanel.columnCount || 0)
            if (columns !== adapter.requestedColumnCount) {
                adapter.bridge.requestGalleryLayout(
                            adapter.side, "columns", columns)
            }
        }

        function onDensityChangeRequested(mode, density, finalChange) {
            if (adapter.applyingRendererState || !adapter.bridge
                    || finalChange !== true || !adapter.densityAdjustable)
                return
            const normalizedMode = String(
                        mode || adapter.requestedPresentationMode)
            if (!adapter.densityIsAdjustable(normalizedMode))
                return
            const normalizedDensity = Math.round(Number(density || 0))
            if (normalizedDensity > 0
                    && (normalizedMode !== adapter.requestedPresentationMode
                        || normalizedDensity !== adapter.requestedDensity)) {
                adapter.bridge.requestGalleryDensity(
                            adapter.side, normalizedMode, normalizedDensity)
            }
        }

        function onSortRequested(sortMode, contextMenu) {
            if (!adapter.bridge)
                return
            adapter.bridge.requestSort(adapter.side, String(sortMode || ""),
                                       contextMenu === true)
        }
    }

    Connections {
        target: adapter.bridge
        ignoreUnknownSignals: true

        function onPanelPresentationTransactionStarted(
                requestedSide, panelId, catalogRevision, mode, columnCount,
                density, presentationDensities, columns,
                separateFileExtensions) {
            if (Number(requestedSide) !== adapter.side)
                return
            adapter.beginPanelPresentationTransaction(
                        panelId, catalogRevision, mode, columnCount, density,
                        presentationDensities, columns,
                        separateFileExtensions)
        }

        function onPanelPresentationTransactionFinished(requestedSide) {
            if (Number(requestedSide) === adapter.side)
                adapter.endPanelPresentationTransaction()
        }
    }
}
