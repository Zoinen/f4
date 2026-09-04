pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls
Item {
    id: controller

    required property ApplicationWindow hostWindow
    required property ListView documentList
    required property ScrollBar documentScrollBar
    required property DocumentTerminalSelectionController terminalSelectionController
    required property DocumentEditorPointerController editorPointerController
    property var frame: ({})
    property bool embedded: false
    property bool interactionActive: true
    property bool showsConsoleTopBar: false
    property bool terminalSurface: false
    property real rowHeight: 20

    property var displayedRows: []
    property bool windowInitialized: false
    property bool rebasingWindow: false
    property bool windowRequestPending: false
    property real requestedExtent: 0
    property real requestedFraction: 0
    property real requestedGeneration: 0
    property real resumeVelocity: 0
    property bool requestPreservesLiveAnchor: true
    property bool wheelGestureActive: false
    property real stableTopExtent: 0
    property real stableTopFraction: 0
    property real lastViewportStart: -1
    property real wheelTarget: 0
    property real queuedScrollBarPosition: -1
    property string appliedWindowSignature: ""
    property string appliedDocumentKey: ""
    property bool componentReady: false
    property bool frameWindowSyncPending: false
    property int loadedSlotStart: 0
    property int loadedSlotEnd: 0
    property alias poolSlotWriteCount: rowPoolController.slotWriteCount
    property var latestWindowRows: []
    property int reportedViewportRows: 0
    property string reportedViewportTarget: ""
    property string reportedViewportAction: ""
    property bool terminalFollowTailIntent: true
    property bool terminalFollowTailInitialized: false

    readonly property alias rowsModel: rowPoolController.rowsModel
    readonly property bool kineticActive:
        documentList.flicking || documentList.dragging
        || wheelAnimation.running || wheelGestureActive
        || terminalSelectionController.autoScrollRunning
    readonly property bool hasWindowProtocol:
        hostWindow.cleanText(frame.scrollUnit) !== ""
        && frame.windowRows !== undefined
    readonly property string documentKey:
        hostWindow.cleanText(frame.documentKey)
    readonly property real contentExtent:
        Math.max(0, Number(frame.contentExtent || 0))
    readonly property bool contentExtentKnown:
        frame.contentExtentKnown !== false

    visible: false; width: 0; height: 0

    function completeViewportRows() {
        if (rowHeight <= 0 || documentList.height <= 0)
            return 0
        return Math.max(1, Math.floor(
                            (documentList.height + 0.001) / rowHeight))
    }

    function clearNativeViewport() {
        if (reportedViewportTarget !== "" && reportedViewportRows > 0) {
            hostWindow.action({
                "target": reportedViewportTarget,
                "action": reportedViewportAction !== ""
                          ? reportedViewportAction : "document.viewport",
                "rows": 0
            }, true)
        }
        reportedViewportTarget = ""
        reportedViewportAction = ""
        reportedViewportRows = 0
    }

    function syncNativeViewport() {
        if (!interactionActive
                || (!terminalSurface && (embedded || !showsConsoleTopBar))) {
            clearNativeViewport()
            return
        }
        const target = hostWindow.cleanText(frame.id)
        const rows = completeViewportRows()
        const viewportAction = terminalSurface
                ? "terminal.viewport" : "document.viewport"
        if (target === "" || rows <= 0)
            return
        if (target === reportedViewportTarget
                && rows === reportedViewportRows
                && viewportAction === reportedViewportAction)
            return
        if (reportedViewportTarget !== ""
                && (target !== reportedViewportTarget
                    || viewportAction !== reportedViewportAction)) {
            hostWindow.action({
                "target": reportedViewportTarget,
                "action": reportedViewportAction !== ""
                          ? reportedViewportAction : "document.viewport",
                "rows": 0
            }, true)
        }
        reportedViewportTarget = target
        reportedViewportAction = viewportAction
        reportedViewportRows = rows
        hostWindow.action({
            "target": target,
            "action": viewportAction,
            "rows": rows
        }, true)
    }

    function scheduleNativeViewportSync() {
        nativeViewportSyncTimer.restart()
    }

    function sourceRows() {
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
        return frame.scrollUnit === "rows"
                ? Number(row.visualRow || 0) : Number(row.offset || 0)
    }

    function rowEndExtent(index, rows) {
        const source = rows || displayedRows
        if (!source || index < 0 || index >= source.length)
            return rowExtent(index, source)
        const row = source[index] || ({})
        if (frame.scrollUnit === "rows")
            return Number(row.visualRow || 0) + 1
        const end = Number(row.endOffset || 0)
        if (end > Number(row.offset || 0))
            return end
        if (index + 1 < source.length)
            return Number((source[index + 1] || ({})).offset || 0)
        return Number(frame.windowEnd || row.offset || 0)
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
        const raw = clamp(Number(contentY || 0) / rowHeight
                          - loadedSlotStart, 0, displayedRows.length)
        if (raw >= displayedRows.length)
            return rowEndExtent(displayedRows.length - 1)
        const index = Math.floor(raw)
        const fraction = raw - index
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

    function recenterRows(rows, extent, fraction) {
        const source = rows || []
        ensurePoolCapacity(rowPoolController.capacityFor(source))
        clearLoadedSlots()
        const start = Math.max(0, Math.floor(
                     (rowPoolController.count - source.length) / 2))
        for (let index = 0; index < source.length; ++index)
            setPoolSlot(start + index, source[index])
        displayedRows = source
        loadedSlotStart = start
        loadedSlotEnd = start + source.length

        // The pool, its content extent and its viewport are one transaction.
        // Deferring placement to a second animation tick paints empty pool
        // slots (or the preceding document) before the actual first page.
        documentList.forceLayout()
        placeAtExtent(extent, fraction)
        documentList.forceLayout()
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

        const unionStart = Math.min(loadedSlotStart, nextStart)
        const unionEnd = Math.max(loadedSlotEnd, nextEnd)
        const unionRows = []
        for (let index = 0; index < nextRows.length; ++index)
            setPoolSlot(nextStart + index, nextRows[index])

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
            return true
        }

        for (let slot = loadedSlotStart; slot < loadedSlotEnd; ++slot) {
            if (slot < nextStart || slot >= nextEnd)
                clearPoolSlot(slot)
        }
        displayedRows = nextRows
        loadedSlotStart = nextStart
        loadedSlotEnd = nextEnd
        return true
    }

    function compactWindowIfIdle() {
        if (kineticActive || wheelGestureActive || rebasingWindow
                || !latestWindowRows || latestWindowRows.length === 0)
            return
        const state = topState()
        rebasingWindow = true
        recenterRows(latestWindowRows, state.extent, state.fraction)
        rebasingWindow = false
        captureTopState()
        syncScrollBar()
    }

    function minimumLoadedY() {
        return loadedSlotStart * rowHeight
    }

    function maximumLoadedY() {
        return Math.max(minimumLoadedY(),
                        loadedSlotEnd * rowHeight - documentList.height)
    }

    function semanticFrameFollowsTail() {
        if (!terminalSurface)
            return false
        if (frame.followTail !== undefined)
            return frame.followTail === true
        if (!contentExtentKnown || contentExtent <= 0)
            return true
        const viewportEnd = Number(frame.viewportStart || 0)
                + Number(frame.viewportSpan || 0)
        return viewportEnd >= contentExtent - 0.000001
    }

    function frameReachesContentEnd() {
        if (!contentExtentKnown || contentExtent <= 0)
            return false
        if (terminalSurface && terminalFollowTailInitialized)
            return terminalFollowTailIntent
        const viewportEnd = Number(frame.viewportStart || 0)
                + Number(frame.viewportSpan || 0)
        return viewportEnd >= contentExtent - 0.000001
    }

    function placeAtExtent(extent, fraction) {
        if (frameReachesContentEnd()) {
            documentList.contentY = maximumLoadedY()
            wheelTarget = documentList.contentY
            return
        }
        let index = indexForExtent(extent, displayedRows)
        if (index < 0) {
            index = clamp(Number(frame.viewportRow || 0), 0,
                          Math.max(0, displayedRows.length - 1))
        }
        documentList.contentY = (loadedSlotStart + index + fraction)
                * rowHeight
        wheelTarget = documentList.contentY
    }

    function visibleExtentSpan() {
        if (!displayedRows || displayedRows.length === 0)
            return 0
        const top = Math.max(0, documentList.contentY)
        return Math.max(0, extentAtContentY(top + documentList.height)
                           - extentAtContentY(top))
    }

    function requestReachesContentEnd(extent) {
        if (!terminalSurface || !contentExtentKnown)
            return false
        if (contentExtent <= 0)
            return true
        const span = Math.max(visibleExtentSpan(),
                              Number(frame.viewportSpan || 0))
        return Number(extent || 0) + span >= contentExtent - 0.000001
    }

    function setTerminalFollowTailIntent(followTail, notifyCore) {
        if (!terminalSurface)
            return
        const desired = followTail === true
        const changed = !terminalFollowTailInitialized
                || terminalFollowTailIntent !== desired
        terminalFollowTailIntent = desired
        terminalFollowTailInitialized = true
        if (!changed || notifyCore !== true
                || hostWindow.cleanText(frame.id) === "")
            return
        hostWindow.action({
            "target": hostWindow.cleanText(frame.id),
            "action": "terminal.followTail",
            "followTail": desired
        }, true)
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

    function sendWindowRequest(extent, fraction, velocity,
                               preserveLiveAnchor) {
        if (!interactionActive || !hasWindowProtocol || windowRequestPending)
            return false
        const total = Math.max(0, contentExtent)
        const target = clamp(Number(extent || 0), 0, total)
        const current = Number(frame.viewportStart || 0)
        const requestedFollowTail = terminalSurface
                && (terminalFollowTailIntent
                    || requestReachesContentEnd(target))
        const followStateChanged = terminalSurface
                && requestedFollowTail !== semanticFrameFollowsTail()
        if (Math.abs(target - current) < 0.000001
                && Math.abs(fraction) < 0.000001
                && !followStateChanged)
            return false

        windowRequestPending = true
        requestedExtent = target
        requestedFraction = clamp(Number(fraction || 0), 0, 0.999999)
        requestedGeneration = Number(frame.windowGeneration || 0) + 1
        resumeVelocity = Number(velocity || 0)
        requestPreservesLiveAnchor = preserveLiveAnchor !== false
        const actionMap = {
            "target": hostWindow.cleanText(frame.id),
            "action": hostWindow.cleanText(frame.scrollAction) !== ""
                      ? hostWindow.cleanText(frame.scrollAction)
                      : frame.kind === "editor"
                        ? "editor.scroll" : "viewer.scrollWindow"
        }
        if (documentKey !== "")
            actionMap.contentKey = documentKey
        actionMap.generation = requestedGeneration
        if (terminalSurface) {
            actionMap.followTail = requestedFollowTail
            setTerminalFollowTailIntent(requestedFollowTail, false)
        }
        if (frame.scrollUnit === "rows")
            actionMap.visualRow = Math.floor(target)
        else
            actionMap.offset = Math.floor(target)
        hostWindow.action(actionMap, true)
        return true
    }

    function maybeRequestWindow() {
        if (!interactionActive || rebasingWindow || frameWindowSyncPending
                || windowRequestPending
                || !hasWindowProtocol || !displayedRows
                || displayedRows.length === 0)
            return
        const state = topState()
        const visibleRows = Math.max(
                    1, Math.ceil(documentList.height / rowHeight))
        const extraRows = Math.max(0, displayedRows.length - visibleRows)
        const threshold = Math.max(2, Math.floor(extraRows / 4))
        const rowsBefore = state.index
        const rowsAfter = displayedRows.length - state.index - visibleRows
        const atStart = state.extent <= 0.000001
        const atEnd = contentExtent > 0
                && state.extent + visibleExtentSpan()
                   >= contentExtent - 0.000001
        if ((rowsBefore <= threshold && !atStart)
                || (rowsAfter <= threshold && !atEnd)) {
            sendWindowRequest(state.extent, state.fraction,
                              documentList.verticalVelocity)
        }
    }

    function resetDocumentTransientState() {
        stopMotion()
        terminalSelectionController.resetForDocumentChange()
        editorPointerController.reset()
        windowRequestPending = false
        requestedExtent = 0
        requestedFraction = 0
        requestedGeneration = 0
        resumeVelocity = 0
        requestPreservesLiveAnchor = true
        wheelGestureActive = false
        queuedScrollBarPosition = -1
        appliedWindowSignature = ""
        lastViewportStart = -1
        windowInitialized = false
    }

    function applyFrameWindow() {
        const nextRows = sourceRows()
        const nextSignature = windowSignature(nextRows)
        const nextDocumentKey = documentKey
        const documentChanged = windowInitialized
                && nextDocumentKey !== appliedDocumentKey
        if (!terminalSurface) {
            terminalFollowTailInitialized = false
        } else if (!terminalFollowTailInitialized || documentChanged) {
            terminalFollowTailIntent = semanticFrameFollowsTail()
            terminalFollowTailInitialized = true
        } else if (frame.altScreen === true) {
            terminalFollowTailIntent = true
        }
        if (documentChanged)
            resetDocumentTransientState()

        const wasInitialized = windowInitialized
        const oldState = windowInitialized ? topState()
                                         : { "extent": Number(
                                                 frame.viewportStart || 0),
                                             "fraction": 0 }
        const generation = Number(frame.windowGeneration || 0)
        const acknowledged = windowRequestPending
                && generation >= requestedGeneration
        if (terminalSurface && acknowledged)
            setTerminalFollowTailIntent(semanticFrameFollowsTail(), false)
        const viewportChanged = windowInitialized
                && Number(frame.viewportStart || 0) !== lastViewportStart
        if (windowInitialized && !acknowledged && !viewportChanged
                && nextSignature === appliedWindowSignature) {
            syncScrollBar()
            return
        }
        let targetExtent = oldState.extent
        let targetFraction = oldState.fraction
        const acceptsViewportChange = !terminalSurface
                || terminalFollowTailIntent || acknowledged
        if (!windowInitialized || (viewportChanged && acceptsViewportChange)) {
            targetExtent = Number(frame.viewportStart || 0)
            targetFraction = acknowledged ? requestedFraction : 0
        }
        if (acknowledged) {
            targetExtent = Number(frame.viewportStart || requestedExtent)
            targetFraction = requestedFraction
            if (requestPreservesLiveAnchor
                    && indexForExtent(oldState.extent, nextRows) >= 0) {
                targetExtent = oldState.extent
                targetFraction = oldState.fraction
            }
        }

        latestWindowRows = nextRows
        rebasingWindow = true
        const keepLiveCoordinates = wasInitialized && kineticActive
                && acknowledged && requestPreservesLiveAnchor
        const mergedOverlap = wasInitialized
                && mergeRowsWithoutRebase(nextRows, keepLiveCoordinates)
        if (!mergedOverlap)
            recenterRows(nextRows, targetExtent, targetFraction)
        else if (!keepLiveCoordinates)
            placeAtExtent(targetExtent, targetFraction)
        appliedWindowSignature = nextSignature
        appliedDocumentKey = nextDocumentKey
        stableTopExtent = targetExtent
        stableTopFraction = targetFraction
        lastViewportStart = Number(frame.viewportStart || 0)
        rebasingWindow = false
        windowInitialized = true
        if (acknowledged) {
            windowRequestPending = false
            resumeVelocity = 0
        }
        syncScrollBar()
        if (!windowRequestPending && queuedScrollBarPosition >= 0)
            scrollBarRequestTimer.restart()
        else
            requestWindowTimer.restart()
    }

    function handleWheel(wheel) {
        if (!interactionActive) {
            wheel.accepted = false
            return
        }
        wheelGestureActive = true
        wheelCommitTimer.restart()
        const pixelY = Number(wheel.pixelDelta.y || 0)
        const minY = minimumLoadedY()
        const maxY = maximumLoadedY()
        if (pixelY !== 0) {
            wheelAnimation.stop()
            documentList.contentY = clamp(
                        documentList.contentY - pixelY, minY, maxY)
            wheelTarget = documentList.contentY
        } else {
            const steps = Number(wheel.angleDelta.y || 0) / 120
            const base = wheelAnimation.running
                    ? wheelAnimation.to : documentList.contentY
            wheelTarget = clamp(base - steps * rowHeight * 3, minY, maxY)
            wheelAnimation.stop()
            wheelAnimation.from = documentList.contentY
            wheelAnimation.to = wheelTarget
            wheelAnimation.restart()
        }
        if (terminalSurface) {
            setTerminalFollowTailIntent(requestReachesContentEnd(
                extentAtContentY(wheelTarget)), true)
        }
        wheel.accepted = true
    }

    function scheduleFrameWindowSync() {
        if (!componentReady || hostWindow.cleanText(frame.kind) === "")
            return
        frameWindowSyncPending = true
        hostWindow.update()
    }

    function stopMotion() {
        documentList.cancelFlick()
        wheelAnimation.stop()
        wheelCommitTimer.stop()
        requestWindowTimer.stop()
        scrollBarRequestTimer.stop()
    }

    function contentYChanged() {
        if (!rebasingWindow && windowInitialized) {
            const bounded = clamp(documentList.contentY,
                                  minimumLoadedY(), maximumLoadedY())
            if (Math.abs(bounded - documentList.contentY) > 0.001) {
                documentList.contentY = bounded
                return
            }
        }
        captureTopState()
        syncScrollBar()
        if (terminalSurface
                && (wheelGestureActive
                    || documentList.dragging || documentList.flicking)) {
            const state = topState()
            setTerminalFollowTailIntent(
                        requestReachesContentEnd(state.extent), true)
        }
        requestWindowTimer.restart()
    }

    function movementEnded() {
        if (rebasingWindow || wheelGestureActive)
            return
        compactWindowIfIdle()
        const state = topState()
        if (!sendWindowRequest(state.extent, state.fraction, 0))
            requestWindowTimer.restart()
    }

    function scrollBarPositionChanged() {
        if (!documentScrollBar.pressed)
            return
        if (terminalSurface) {
            setTerminalFollowTailIntent(requestReachesContentEnd(
                documentScrollBar.position * contentExtent), true)
        }
        queuedScrollBarPosition = documentScrollBar.position
        scrollBarRequestTimer.restart()
    }

    function scrollBarPressedChanged() {
        if (!documentScrollBar.pressed && queuedScrollBarPosition >= 0)
            scrollBarRequestTimer.restart()
    }

    onFrameChanged: {
        scheduleFrameWindowSync()
        scheduleNativeViewportSync()
    }
    onRowHeightChanged: scheduleNativeViewportSync()
    onEmbeddedChanged: scheduleNativeViewportSync()
    onInteractionActiveChanged: {
        if (interactionActive) {
            scheduleNativeViewportSync()
            return
        }
        clearNativeViewport()
        stopMotion()
        editorPointerController.reset()
        terminalSelectionController.cancelInteraction()
        wheelGestureActive = false
        windowRequestPending = false
        requestedGeneration = 0
        requestPreservesLiveAnchor = true
        resumeVelocity = 0
        queuedScrollBarPosition = -1
    }
    Component.onCompleted: {
        componentReady = true
        scheduleFrameWindowSync()
        scheduleNativeViewportSync()
    }

    Connections {
        target: controller.hostWindow
        enabled: controller.frameWindowSyncPending
        function onAfterAnimating() {
            // Bindings and incoming stream updates have settled, but this
            // frame has not been synchronized to the render thread yet.
            // Commit rows and placement together, once, before it can paint.
            controller.frameWindowSyncPending = false
            controller.applyFrameWindow()
        }
    }

    DocumentRowPool {
        id: rowPoolController
        hostWindow: controller.hostWindow
        rowHeight: controller.rowHeight
        viewportHeight: controller.documentList.height
    }

    NumberAnimation {
        id: wheelAnimation
        target: controller.documentList
        property: "contentY"
        duration: 130
        easing.type: Easing.OutCubic
        onFinished: requestWindowTimer.restart()
    }

    Timer {
        id: wheelCommitTimer
        interval: 180
        onTriggered: {
            if (wheelAnimation.running) {
                restart()
                return
            }
            controller.wheelGestureActive = false
            controller.compactWindowIfIdle()
            const state = controller.topState()
            if (controller.terminalSurface) {
                controller.setTerminalFollowTailIntent(
                    controller.requestReachesContentEnd(state.extent), true)
            }
            if (!controller.sendWindowRequest(state.extent,
                                               state.fraction, 0))
                requestWindowTimer.restart()
        }
    }

    Timer {
        id: nativeViewportSyncTimer
        interval: 0
        onTriggered: controller.syncNativeViewport()
    }

    Timer {
        id: requestWindowTimer
        interval: 12
        onTriggered: controller.maybeRequestWindow()
    }

    Timer {
        id: scrollBarRequestTimer
        interval: 0
        onTriggered: {
            if (controller.queuedScrollBarPosition < 0
                    || controller.windowRequestPending)
                return
            const position = controller.queuedScrollBarPosition
            controller.queuedScrollBarPosition = -1
            controller.sendWindowRequest(
                        position * controller.contentExtent, 0, 0, false)
        }
    }
}
