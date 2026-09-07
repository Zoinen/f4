pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: presenter

    required property ApplicationWindow hostWindow
    required property ListView documentList
    required property ScrollBar documentScrollBar
    required property var coordinator
    property var frame: ({})
    property bool terminalSurface: false
    property real rowHeight: 20
    property bool standaloneViewport: false
    property real geometryRevision: 0
    property bool kineticActive: false

    // These fields form the presented window transaction. They deliberately
    // live beside the physical row pool and may only advance in
    // applyFrameWindow's render-boundary commit.
    property var displayedRows: []
    property bool windowInitialized: false
    property bool rebasingWindow: false
    property bool rowTextSuspended: false
    property real stableTopExtent: 0
    property real stableTopFraction: 0
    property real lastViewportStart: -1
    property string appliedWindowSignature: ""
    property string appliedDocumentKey: ""
    property real appliedLayoutRevision: 0
    property real appliedWindowGeneration: 0
    property real appliedScrollLeft: 0
    property real appliedViewportStart: 0
    property var appliedFrame: ({})
    property string currentLoadError: ""
    property string loadErrorDocumentKey: ""
    property int loadedSlotStart: 0
    property int loadedSlotEnd: 0
    property var latestWindowRows: []

    readonly property alias rowsModel: rowPoolController.rowsModel
    property alias poolSlotWriteCount: rowPoolController.slotWriteCount
    readonly property bool hasWindowProtocol:
        hostWindow.cleanText(frame.scrollUnit) !== ""
        && (frame.windowRows !== undefined || frame.nativeWindowRows === true)
    readonly property string documentKey:
        hostWindow.cleanText(frame.documentKey || frame.id)
    readonly property var presentationFrame:
        standaloneViewport && windowInitialized
        && documentKey === appliedDocumentKey ? appliedFrame : frame
    readonly property string loadError:
        loadErrorDocumentKey === documentKey ? currentLoadError : ""
    readonly property real contentExtent:
        Math.max(0, Number(presentationFrame.contentExtent || 0))
    readonly property bool contentExtentKnown:
        presentationFrame.contentExtentKnown !== false

    visible: false
    width: 0
    height: 0

    function sourceRows() {
        if (frame.nativeWindowRows === true)
            return qtShell.surfaceRegistry.documentWindowRows(
                documentKey, frame.nativeWindowRevision)
        return hasWindowProtocol ? frame.windowRows || [] : frame.rows || []
    }

    function windowSignature(rows) {
        const contentKey = hostWindow.cleanText(frame.windowContentKey)
        if (contentKey !== "") {
            return hostWindow.cleanText(frame.windowStart) + ":"
                    + hostWindow.cleanText(frame.windowEnd) + ":" + contentKey
        }
        return JSON.stringify(rows || [])
    }

    function clamp(value, minimum, maximum) {
        return Math.max(minimum, Math.min(maximum, value))
    }

    function rowExtent(index, rows) {
        const source = rows || displayedRows
        if (!source || index < 0 || index >= source.length)
            return 0
        const row = source[index] || ({})
        return presentationFrame.scrollUnit === "rows"
                ? Number(row.visualRow || 0) : Number(row.offset || 0)
    }

    function rowEndExtent(index, rows) {
        const source = rows || displayedRows
        if (!source || index < 0 || index >= source.length)
            return rowExtent(index, source)
        const row = source[index] || ({})
        if (presentationFrame.scrollUnit === "rows")
            return Number(row.visualRow || 0) + 1
        const end = Number(row.endOffset || 0)
        if (end > Number(row.offset || 0))
            return end
        if (index + 1 < source.length)
            return Number((source[index + 1] || ({})).offset || 0)
        return Number(presentationFrame.windowEnd || row.offset || 0)
    }

    function itemForModelIndex(modelIndex) {
        if (modelIndex < 0 || modelIndex >= rowPoolController.count)
            return null
        return documentList.itemAtIndex(modelIndex)
    }

    function modelIndexAtContentY(contentY) {
        if (documentList.width <= 0)
            return -1
        const x = Math.max(0, Math.min(documentList.width - 0.001,
                                       textProbeX()))
        // indexAt operates in content coordinates. Probe just inside the
        // viewport edge first; a fractional delegate can leave a subpixel
        // gap in which no item owns the exact boundary.
        const probes = [0.001, -0.001, 0.5, rowHeight * 0.25]
        for (let probe = 0; probe < probes.length; ++probe) {
            const index = documentList.indexAt(x, Number(contentY || 0)
                                                  + probes[probe])
            if (index >= 0)
                return index
        }
        return -1
    }

    function textProbeX() {
        return Math.max(1, Math.min(documentList.width * 0.5,
                                    documentList.width - 1))
    }

    function modelCoordinateForIndex(modelIndex) {
        const direct = itemForModelIndex(modelIndex)
        if (direct !== null)
            return direct.y
        const anchorIndex = modelIndexAtContentY(documentList.contentY)
        const anchor = itemForModelIndex(anchorIndex)
        if (anchor !== null) {
            // Qt's ListView estimator may round a fractional fixed delegate
            // height (22.857 at 175%) to a different stride. Infer that
            // stride from actual neighboring delegates, not rowHeight.
            for (let delta = 1; delta <= 4; ++delta) {
                let neighbor = itemForModelIndex(anchorIndex + delta)
                if (neighbor !== null)
                    return anchor.y + (modelIndex - anchorIndex)
                            * (neighbor.y - anchor.y) / delta
                neighbor = itemForModelIndex(anchorIndex - delta)
                if (neighbor !== null)
                    return anchor.y + (modelIndex - anchorIndex)
                            * (anchor.y - neighbor.y) / delta
            }
        }
        return Number(documentList.originY || 0)
                + modelIndex * rowHeight
    }

    function windowIndexAtViewportY(viewportY) {
        const contentY = documentList.contentY + Number(viewportY || 0)
        const modelIndex = modelIndexAtContentY(contentY)
        if (modelIndex >= 0)
            return modelIndex - loadedSlotStart
        const state = topState()
        return Math.floor(state.index + state.fraction
                          + Number(viewportY || 0) / rowHeight)
    }

    function rowItem(index) {
        // contentY is deliberately read so this binding is refreshed when a
        // cursor row enters or leaves the instantiated ListView viewport.
        const viewportRevision = documentList.contentY
                + documentList.originY + documentList.contentHeight
        if (!isFinite(viewportRevision))
            return null
        return itemForModelIndex(loadedSlotStart + index)
    }

    function topState() {
        if (!displayedRows || displayedRows.length === 0)
            return { "index": 0, "fraction": 0, "extent": 0 }
        const raw = Math.max(0, documentList.contentY) / rowHeight
                - loadedSlotStart
        const index = clamp(Math.floor(raw), 0, displayedRows.length - 1)
        const fraction = clamp(raw - index, 0, 0.999999)
        const start = rowExtent(index)
        const end = rowEndExtent(index)
        return {
            "index": index,
            "fraction": fraction,
            "extent": start + (end - start) * fraction
        }
    }

    function extentAtContentY(contentY) {
        if (!displayedRows || displayedRows.length === 0)
            return 0
        const raw = Math.max(0, Number(contentY || 0)) / rowHeight
                - loadedSlotStart
        const bounded = clamp(raw, 0, displayedRows.length)
        if (bounded >= displayedRows.length)
            return rowEndExtent(displayedRows.length - 1)
        const index = Math.floor(bounded)
        const fraction = bounded - index
        const start = rowExtent(index)
        const end = rowEndExtent(index)
        return start + (end - start) * fraction
    }


    function captureTopState() {
        if (rebasingWindow || !windowInitialized)
            return
        const state = topState()
        stableTopExtent = state.extent
        stableTopFraction = state.fraction
    }

    function indexForExtent(extent, rows) {
        const source = rows || displayedRows
        if (!source || source.length === 0)
            return -1
        for (let index = 0; index < source.length; ++index) {
            const start = rowExtent(index, source)
            const end = rowEndExtent(index, source)
            if (Math.abs(start - extent) < 0.000001
                    || (extent >= start && extent < end))
                return index
        }
        return -1
    }

    function setPoolSlot(slot, row) {
        return rowPoolController.setSlot(slot, row)
    }

    function clearPoolSlot(slot) {
        return rowPoolController.clearSlot(slot)
    }

    function ensurePoolCapacity(capacity) {
        rowPoolController.ensureCapacity(capacity)
    }

    function clearLoadedSlots() {
        for (let slot = loadedSlotStart;
             slot < loadedSlotEnd && slot < rowPoolController.count; ++slot)
            clearPoolSlot(slot)
    }

    function recenterRows(rows, extent, fraction, forceReplacement) {
        rowTextSuspended = false
        const source = rows || []
        ensurePoolCapacity(rowPoolController.capacityFor(source))
        const start = Math.max(0, Math.floor(
                     (rowPoolController.count - source.length) / 2))
        if (standaloneViewport) {
            // Replace occupied slots directly in this transaction. Clearing
            // them first tears down their text runs only to recreate them
            // immediately; only slots outside the new window become empty.
            rowPoolController.replaceWindow(start, source,
                loadedSlotStart, loadedSlotEnd, forceReplacement, true)
        } else {
            clearLoadedSlots()
            for (let index = 0; index < source.length; ++index)
                rowPoolController.setSlot(start + index, source[index], forceReplacement)
        }
        displayedRows = source
        loadedSlotStart = start
        loadedSlotEnd = start + source.length
        rowPoolController.commit()

        // The pool, its content extent and its viewport are one transaction.
        // Deferring placement to a second animation tick paints empty pool
        // slots (or the preceding document) before the actual first page.
        if (!standaloneViewport)
            documentList.forceLayout()
        placeAtExtent(extent, fraction)
        documentList.forceLayout()
        // ListView fixes its own pool bounds during layout and rounds an
        // interior resting position to logical pixels. Our semantic endpoint
        // is inside that pool and may be fractional at the window's DPR.
        // Commit its exact placement after layout, before enabling row text.
        if (standaloneViewport)
            placeAtExtent(extent, fraction)
    }

    function mergeRowsWithoutRebase(nextRows, retainLiveUnion) {
        if (!displayedRows || displayedRows.length === 0) {
            recenterRows(nextRows, Number(frame.viewportStart || 0), 0)
            return true
        }

        let oldIndex = -1
        let nextIndex = -1
        for (let index = 0; index < nextRows.length && nextIndex < 0; ++index) {
            const extent = rowExtent(index, nextRows)
            const found = indexForExtent(extent, displayedRows)
            if (found >= 0
                    && Math.abs(rowExtent(found, displayedRows) - extent)
                       < 0.000001) {
                oldIndex = found
                nextIndex = index
            }
        }
        if (oldIndex < 0)
            return false

        const nextStart = loadedSlotStart + oldIndex - nextIndex
        const nextEnd = nextStart + nextRows.length
        if (nextStart < 0 || nextEnd > rowPoolController.count)
            return false
        if (retainLiveUnion !== true && Math.abs(oldIndex - nextIndex) > 5)
            return false

        const unionStart = Math.min(loadedSlotStart, nextStart)
        const unionEnd = Math.max(loadedSlotEnd, nextEnd)
        const unionRows = []
        if (standaloneViewport)
            rowPoolController.replaceWindow(nextStart, nextRows,
                loadedSlotStart, loadedSlotEnd, false,
                retainLiveUnion !== true)
        else {
            for (let index = 0; index < nextRows.length; ++index)
                setPoolSlot(nextStart + index, nextRows[index])
        }

        if (retainLiveUnion === true) {
            for (let slot = unionStart; slot < unionEnd; ++slot) {
                const incoming = slot - nextStart
                const existing = slot - loadedSlotStart
                const hasIncoming = incoming >= 0 && incoming < nextRows.length
                const hasExisting = existing >= 0
                        && existing < displayedRows.length
                if (hasIncoming)
                    unionRows.push(nextRows[incoming])
                else if (hasExisting)
                    unionRows.push(displayedRows[existing])
                else
                    return false
            }
            displayedRows = unionRows
            loadedSlotStart = unionStart
            loadedSlotEnd = unionEnd
            rowPoolController.commit()
            return true
        }

        if (!standaloneViewport) {
            for (let slot = loadedSlotStart; slot < loadedSlotEnd; ++slot) {
                if (slot < nextStart || slot >= nextEnd)
                    clearPoolSlot(slot)
            }
        }
        displayedRows = nextRows
        loadedSlotStart = nextStart
        loadedSlotEnd = nextEnd
        rowPoolController.commit()
        return true
    }

    function compactWindowIfIdle() {
        if (kineticActive || coordinator.wheelGestureActive || rebasingWindow
                || !latestWindowRows || latestWindowRows.length === 0)
            return
        const state = topState()
        rebasingWindow = true
        recenterRows(latestWindowRows, state.extent, state.fraction)
        rowTextSuspended = false
        rebasingWindow = false
        captureTopState()
        syncScrollBar()
    }

    function minimumLoadedY() {
        return modelCoordinateForIndex(loadedSlotStart)
    }

    function maximumLoadedY() {
        // Use the coordinate immediately after the loaded range. Looking at
        // the last instantiated delegate can be wrong while ListView is
        // rebasing its bounded pool: that delegate may still carry the old
        // local origin, which leaves blank rows below a follow-tail frame.
        const bottom = modelCoordinateForIndex(loadedSlotEnd)
        return Math.max(minimumLoadedY(), bottom - documentList.height)
    }

    function frameReachesContentEnd() {
        if (!contentExtentKnown || contentExtent <= 0)
            return false
        if (terminalSurface && coordinator.terminalFollowTailInitialized)
            return coordinator.terminalFollowTailIntent
        const viewportEnd = Number(presentationFrame.viewportStart || 0)
                + Number(presentationFrame.viewportSpan || 0)
        return viewportEnd >= contentExtent - 0.000001
    }

    function placeAtExtent(extent, fraction) {
        // The terminal's semantic viewport is negotiated using Go's
        // integer viewport span, while the native ListView can expose a
        // larger physical viewport.  A follow-tail frame must therefore be
        // placed at the native viewport's actual bottom, not at the stale
        // semantic viewportStart. Otherwise the last row is visible only
        // because the delegate pool extends below the viewport and the next
        // scroll request starts from the wrong extent.
        const followTailToActualViewport = terminalSurface
                && coordinator.terminalFollowTailIntent
                && contentExtentKnown && contentExtent > 0 && rowHeight > 0
        let targetExtent = extent
        if (followTailToActualViewport) {
            const visibleRows = Math.max(1, documentList.height / rowHeight)
            targetExtent = Math.max(0, contentExtent - visibleRows)
            fraction = 0
        }
        let index = indexForExtent(targetExtent, displayedRows)
        if (index < 0) {
            index = clamp(Number(presentationFrame.viewportRow || 0), 0,
                          Math.max(0, displayedRows.length - 1))
        }
        if (followTailToActualViewport) {
            const start = rowExtent(index, displayedRows)
            fraction = clamp(targetExtent - start, 0, 0.999999)
        }
        const modelIndex = loadedSlotStart + index
        if (frameReachesContentEnd() && !followTailToActualViewport) {
            const maxY = maximumLoadedY()
            const anchorY = modelCoordinateForIndex(modelIndex)
            if (maxY > anchorY) {
                if (loadedSlotEnd > loadedSlotStart) {
                    documentList.positionViewAtIndex(loadedSlotEnd - 1,
                                                     ListView.End)
                    documentList.forceLayout()
                }
                const item = itemForModelIndex(modelIndex)
                const itemY = item !== null ? item.y : anchorY
                documentList.contentY = Math.max(itemY, maxY)
                coordinator.wheelTarget = documentList.contentY
                return
            }
        }
        documentList.positionViewAtIndex(modelIndex, ListView.Beginning)
        documentList.forceLayout()
        const item = itemForModelIndex(modelIndex)
        const itemHeight = item !== null && item.height > 0 ? item.height : rowHeight
        const itemY = item !== null ? item.y : modelCoordinateForIndex(modelIndex)
        documentList.contentY = itemY + fraction * itemHeight
        coordinator.wheelTarget = documentList.contentY
    }

    function visibleExtentSpan() {
        if (!displayedRows || displayedRows.length === 0)
            return 0
        const top = Math.max(0, documentList.contentY)
        return Math.max(0, extentAtContentY(top + documentList.height)
                           - extentAtContentY(top))
    }

    function syncScrollBar() {
        if (!documentScrollBar.visible || documentScrollBar.pressed)
            return
        const extent = Math.max(1, contentExtent)
        const state = topState()
        const span = Math.max(0, visibleExtentSpan())
        documentScrollBar.size = clamp(span / extent, 0, 1)
        documentScrollBar.position = clamp(
                    state.extent / extent, 0, 1 - documentScrollBar.size)
    }

    function resetDocumentTransientState() {
        coordinator.resetRequestTransientState()
        appliedWindowSignature = ""
        lastViewportStart = -1
        windowInitialized = false
    }

    function invalidateStandalonePresentation() {
        resetDocumentTransientState()
        // The physical pool and negotiated window geometry are reusable, but
        // a closed Go document is not. documentKey historically contained a
        // pointer and can be reused by a later allocation, so no presentation
        // identity or scroll position may survive the close boundary.
        appliedDocumentKey = ""
        appliedLayoutRevision = 0
        appliedWindowGeneration = 0
        appliedViewportStart = 0
        appliedScrollLeft = 0
        appliedFrame = ({})
        latestWindowRows = []
        stableTopExtent = 0
        stableTopFraction = 0
        currentLoadError = ""
        loadErrorDocumentKey = ""
    }

    // architecture-check: allow-function-lines atomic row/model/identity/placement render-boundary transaction
    function applyFrameWindow() {
        const traceWindow = standaloneViewport && typeof qtGallery !== "undefined"
                && qtGallery.benchmarkTraceEnabled === true
        const prepareStartedMs = traceWindow ? Date.now() : 0
        if (standaloneViewport && frame.geometryRevision !== undefined
                && Number(frame.geometryRevision) < geometryRevision)
            return
        const nextLayoutRevision = Number(frame.layoutRevision || 0)
        const nextGeneration = Number(frame.windowGeneration || 0)
        if (standaloneViewport && windowInitialized
                && documentKey === appliedDocumentKey
                && nextLayoutRevision > appliedLayoutRevision) {
            // Even a pending new layout cancels demand in the old layout.
            // Its rows and presentation metadata still commit only when ready.
            coordinator.windowRequestPending = false
            coordinator.pendingWindowIntent = null
            coordinator.queuedScrollBarPosition = -1
            coordinator.canceledWindowGeneration = 0
        }
        if (standaloneViewport && hostWindow.cleanText(frame.loadError) !== ""
                && (!windowInitialized || documentKey !== appliedDocumentKey
                    || nextLayoutRevision >= appliedLayoutRevision)) {
            const failedRequest = Number(frame.windowRequestGeneration || 0)
            if (frame.windowRequestGeneration !== undefined
                    && documentKey === appliedDocumentKey
                    && nextLayoutRevision === appliedLayoutRevision
                    && ((coordinator.windowRequestPending && failedRequest < coordinator.requestedGeneration)
                        || (failedRequest > 0
                            && failedRequest <= coordinator.canceledWindowGeneration)))
                return
            if (coordinator.dispatchPendingWindowIntent())
                return
            coordinator.windowRequestPending = false
            currentLoadError = hostWindow.cleanText(frame.loadError)
            loadErrorDocumentKey = documentKey
            coordinator.stopRequestWindowTimer()
            return
        }
        if (standaloneViewport && frame.layoutPending === true)
            return
        if (standaloneViewport && windowInitialized
                && documentKey === appliedDocumentKey
                && (nextLayoutRevision < appliedLayoutRevision
                    || (nextLayoutRevision === appliedLayoutRevision
                        && (nextGeneration < Math.max(appliedWindowGeneration,
                                coordinator.windowRequestPending ? coordinator.requestedGeneration : 0)
                            || (nextGeneration > appliedWindowGeneration
                                && nextGeneration <= coordinator.canceledWindowGeneration)))))
            return
        const nextRows = sourceRows()
        // A retained frame may outlive its native publication while hidden.
        // Never pair its metadata with a newer document's rows.
        if (nextRows === undefined || nextRows === null)
            return
        currentLoadError = ""
        const nextSignature = windowSignature(nextRows)
        const nextDocumentKey = documentKey
        const documentChanged = windowInitialized
                && nextDocumentKey !== appliedDocumentKey
        if (!terminalSurface) {
            coordinator.terminalFollowTailInitialized = false
        } else if (!coordinator.terminalFollowTailInitialized || documentChanged) {
            coordinator.terminalFollowTailIntent = coordinator.semanticFrameFollowsTail()
            coordinator.terminalFollowTailInitialized = true
        } else if (frame.altScreen === true) {
            coordinator.terminalFollowTailIntent = true
        }
        if (documentChanged)
            resetDocumentTransientState()

        const layoutChanged = windowInitialized
                && nextLayoutRevision !== appliedLayoutRevision

        const wasInitialized = windowInitialized
        const oldState = windowInitialized ? topState()
                                         : { "extent": Number(
                                                 frame.viewportStart || 0),
                                             "fraction": 0 }
        const generation = Number(frame.windowGeneration || 0)
        const acknowledged = coordinator.windowRequestPending
                && generation >= coordinator.requestedGeneration
        const pendingEmbeddedIntent = !standaloneViewport
                && coordinator.pendingWindowIntent !== null
                ? coordinator.pendingWindowIntent : null
        if (terminalSurface && acknowledged)
            coordinator.setTerminalFollowTailIntent(coordinator.semanticFrameFollowsTail(), false)
        const viewportChanged = windowInitialized
                && Number(frame.viewportStart || 0) !== lastViewportStart
        if (windowInitialized && !layoutChanged && !acknowledged && !viewportChanged
                && nextSignature === appliedWindowSignature) {
            appliedWindowGeneration = generation
            appliedScrollLeft = Number(frame.scrollLeft || 0)
            appliedFrame = frame
            syncScrollBar()
            return
        }
        let targetExtent = oldState.extent
        let targetFraction = oldState.fraction
        const acceptsViewportChange = !terminalSurface
                || coordinator.terminalFollowTailIntent || acknowledged
        if (!windowInitialized || (viewportChanged && acceptsViewportChange)) {
            targetExtent = Number(frame.viewportStart || 0)
            targetFraction = acknowledged ? coordinator.requestedFraction : 0
        }
        if (acknowledged) {
            const matchesRequestedExtent = Math.abs(Number(frame.viewportStart || 0)
                    - coordinator.requestedExtent) < 0.000001
            targetExtent = Number(frame.viewportStart !== undefined
                    && frame.viewportStart !== null
                    ? frame.viewportStart : coordinator.requestedExtent)
            targetFraction = matchesRequestedExtent ? coordinator.requestedFraction : 0
            if (!viewportChanged && (kineticActive || coordinator.wheelGestureActive)
                    && coordinator.requestPreservesLiveAnchor
                && indexForExtent(oldState.extent, nextRows) >= 0) {
                targetExtent = oldState.extent
                targetFraction = oldState.fraction
            }
            // Embedded Quick View has no separate ACK for a replaceable local
            // wheel destination. Preserve that local motion when the first
            // request is acknowledged; sending a second request here would
            // leave the view pending forever because the test/app may not
            // publish another semantic window immediately.
            if (pendingEmbeddedIntent !== null) {
                targetExtent = Number(pendingEmbeddedIntent.extent || 0)
                targetFraction = Number(pendingEmbeddedIntent.fraction || 0)
            }
        }

        latestWindowRows = nextRows
        appliedFrame = frame
        rebasingWindow = true
        rowTextSuspended = false
        const rowsStartedMs = traceWindow ? Date.now() : 0
        const keepLiveCoordinates = wasInitialized && !layoutChanged && kineticActive
                && acknowledged && coordinator.requestPreservesLiveAnchor
        const mergedOverlap = wasInitialized && !layoutChanged
                && mergeRowsWithoutRebase(nextRows, keepLiveCoordinates)
        if (!mergedOverlap)
            recenterRows(nextRows, targetExtent, targetFraction,
                         standaloneViewport && (!wasInitialized || layoutChanged))
        else if (!keepLiveCoordinates) {
            placeAtExtent(targetExtent, targetFraction)
            documentList.forceLayout()
            if (standaloneViewport)
                placeAtExtent(targetExtent, targetFraction)
        }
        const finishStartedMs = traceWindow ? Date.now() : 0
        appliedWindowSignature = nextSignature
        appliedDocumentKey = nextDocumentKey
        appliedLayoutRevision = nextLayoutRevision
        appliedWindowGeneration = generation
        appliedScrollLeft = Number(frame.scrollLeft || 0)
        appliedViewportStart = Number(frame.viewportStart || 0)
        stableTopExtent = targetExtent
        stableTopFraction = targetFraction
        lastViewportStart = Number(frame.viewportStart || 0)
        rowTextSuspended = false
        rebasingWindow = false
        windowInitialized = true
        if (acknowledged) {
            coordinator.windowRequestPending = false
            coordinator.resumeVelocity = 0
            if (pendingEmbeddedIntent !== null)
                coordinator.pendingWindowIntent = null
        }
        syncScrollBar()
        if (acknowledged && standaloneViewport)
            coordinator.dispatchPendingWindowIntent()
        if (!coordinator.windowRequestPending && coordinator.queuedScrollBarPosition >= 0)
            coordinator.scheduleScrollBarRequest()
        else
            coordinator.scheduleWindowRequest()
        if (traceWindow) {
            qtGallery.recordDocumentWindowCommit({
                "documentKey": appliedDocumentKey, "kind": frame.kind,
                "layoutRevision": appliedLayoutRevision,
                "geometryRevision": frame.geometryRevision,
                "windowGeneration": appliedWindowGeneration,
                "windowStart": frame.windowStart, "windowEnd": frame.windowEnd,
                "windowContentKey": frame.windowContentKey,
                "viewportStart": appliedViewportStart,
                "viewportColumns": frame.viewportColumns,
                "viewportRows": frame.viewportRows, "rowCount": nextRows.length,
                // QML's wall clock has millisecond resolution. These bounded
                // phase durations supplement the native monotonic boundary;
                // they are not used for protocol ordering or presentation.
                "prepareMs": rowsStartedMs - prepareStartedMs,
                "rowsAndPlacementMs": finishStartedMs - rowsStartedMs,
                "finishMs": Date.now() - finishStartedMs
            })
        }
    }

    DocumentRowPool {
        id: rowPoolController
        hostWindow: presenter.hostWindow
        nativeRows: presenter.standaloneViewport
        rowHeight: presenter.rowHeight
        viewportHeight: presenter.documentList.height
    }
}
