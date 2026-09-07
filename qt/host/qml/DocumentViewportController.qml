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
    required property Item middleAutoScrollController
    property var frame: ({})
    property bool embedded: false
    property bool interactionActive: true
    property bool showsConsoleTopBar: false
    property bool terminalSurface: false
    property real rowHeight: 20
    property bool standaloneViewport: false
    property real standaloneViewportWidth: 0
    property real standaloneViewportHeight: 0
    property real documentCellWidth: 1

    property alias displayedRows: windowPresenter.displayedRows
    property alias windowInitialized: windowPresenter.windowInitialized
    property alias rebasingWindow: windowPresenter.rebasingWindow
    property alias rowTextSuspended: windowPresenter.rowTextSuspended
    property bool windowRequestPending: false
    property var pendingWindowIntent: null
    property real requestedExtent: 0
    property real requestedFraction: 0
    property real requestedGeneration: 0
    property real resumeVelocity: 0
    property bool requestPreservesLiveAnchor: true
    property bool wheelGestureActive: false
    property bool middleAutoScrollAllowed: false
    property string middleAutoScrollDocumentKey: ""
    property real middleAutoScrollLayoutRevision: 0
    property alias stableTopExtent: windowPresenter.stableTopExtent
    property alias stableTopFraction: windowPresenter.stableTopFraction
    property alias lastViewportStart: windowPresenter.lastViewportStart
    property real wheelTarget: 0
    property real queuedScrollBarPosition: -1
    property alias appliedWindowSignature: windowPresenter.appliedWindowSignature
    property alias appliedDocumentKey: windowPresenter.appliedDocumentKey
    property alias appliedLayoutRevision: windowPresenter.appliedLayoutRevision
    property alias appliedWindowGeneration: windowPresenter.appliedWindowGeneration
    property real canceledWindowGeneration: 0
    property alias appliedScrollLeft: windowPresenter.appliedScrollLeft
    property alias appliedViewportStart: windowPresenter.appliedViewportStart
    property alias appliedFrame: windowPresenter.appliedFrame
    property alias currentLoadError: windowPresenter.currentLoadError
    property alias loadErrorDocumentKey: windowPresenter.loadErrorDocumentKey
    property bool componentReady: false
    property bool frameWindowSyncPending: false
    property alias loadedSlotStart: windowPresenter.loadedSlotStart
    property alias loadedSlotEnd: windowPresenter.loadedSlotEnd
    property alias poolSlotWriteCount: windowPresenter.poolSlotWriteCount
    property alias latestWindowRows: windowPresenter.latestWindowRows
    property int reportedViewportRows: 0
    property string reportedViewportTarget: ""
    property string reportedViewportAction: ""
    property int reportedViewportColumns: 0
    property real geometryRevision: 0
    property bool terminalFollowTailIntent: true
    property bool terminalFollowTailInitialized: false

    readonly property alias rowsModel: windowPresenter.rowsModel
    readonly property bool middleAutoScrollActive:
        middleAutoScrollController.scrollingMode
    readonly property bool kineticActive:
        documentList.flicking || documentList.dragging
        || wheelAnimation.running || wheelGestureActive
        || middleAutoScrollActive
        || terminalSelectionController.autoScrollRunning
    readonly property alias hasWindowProtocol: windowPresenter.hasWindowProtocol
    readonly property alias documentKey: windowPresenter.documentKey
    readonly property alias presentationFrame: windowPresenter.presentationFrame
    readonly property alias loadError: windowPresenter.loadError
    readonly property alias contentExtent: windowPresenter.contentExtent
    readonly property alias contentExtentKnown: windowPresenter.contentExtentKnown

    visible: false; width: 0; height: 0

    function completeViewportRows() {
        if (rowHeight <= 0 || documentList.height <= 0)
            return 0
        return Math.max(1, Math.floor(
                            (documentList.height + 0.001) / rowHeight))
    }

    function clearNativeViewport() {
        // Standalone geometry belongs to the window session, not to a file.
        // Closing a document must not clear the next document's layout.
        if (standaloneViewport)
            return
        if (reportedViewportTarget !== "" && reportedViewportTarget !== "app"
                && reportedViewportRows > 0) {
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
        if (standaloneViewport) {
            const columns = Math.max(1, Math.floor(
                standaloneViewportWidth / Math.max(1, documentCellWidth)))
            const rows = Math.max(1, Math.floor(
                (standaloneViewportHeight + 0.001) / Math.max(1, rowHeight)))
            if (standaloneViewportWidth <= 0 || standaloneViewportHeight <= 0
                    || (columns === reportedViewportColumns
                        && rows === reportedViewportRows))
                return
            reportedViewportColumns = columns
            reportedViewportRows = rows
            reportedViewportAction = "document.viewport"
            reportedViewportTarget = "app"
            ++geometryRevision
            hostWindow.action({
                "target": "app", "action": "document.viewport",
                "scope": "standalone", "columns": columns, "rows": rows,
                "geometryRevision": geometryRevision
            }, true)
            return
        }
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
                && reportedViewportTarget !== "app"
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

    function sourceRows() { return windowPresenter.sourceRows() }
    function windowSignature(rows) { return windowPresenter.windowSignature(rows) }
    function clamp(value, minimum, maximum) { return windowPresenter.clamp(value, minimum, maximum) }
    function rowExtent(index, rows) { return windowPresenter.rowExtent(index, rows) }
    function rowEndExtent(index, rows) { return windowPresenter.rowEndExtent(index, rows) }
    function itemForModelIndex(index) { return windowPresenter.itemForModelIndex(index) }
    function modelIndexAtContentY(y) { return windowPresenter.modelIndexAtContentY(y) }
    function textProbeX() { return windowPresenter.textProbeX() }
    function modelCoordinateForIndex(index) { return windowPresenter.modelCoordinateForIndex(index) }
    function windowIndexAtViewportY(y) { return windowPresenter.windowIndexAtViewportY(y) }
    function rowItem(index) { return windowPresenter.rowItem(index) }
    function topState() { return windowPresenter.topState() }
    function extentAtContentY(y) { return windowPresenter.extentAtContentY(y) }
    function captureTopState() { windowPresenter.captureTopState() }
    function indexForExtent(extent, rows) { return windowPresenter.indexForExtent(extent, rows) }
    function setPoolSlot(slot, row) { return windowPresenter.setPoolSlot(slot, row) }
    function clearPoolSlot(slot) { return windowPresenter.clearPoolSlot(slot) }
    function ensurePoolCapacity(capacity) { windowPresenter.ensurePoolCapacity(capacity) }
    function clearLoadedSlots() { windowPresenter.clearLoadedSlots() }
    function recenterRows(rows, extent, fraction, forceReplacement) {
        windowPresenter.recenterRows(rows, extent, fraction, forceReplacement)
    }
    function mergeRowsWithoutRebase(rows, retainLiveUnion) {
        return windowPresenter.mergeRowsWithoutRebase(rows, retainLiveUnion)
    }
    function compactWindowIfIdle() { windowPresenter.compactWindowIfIdle() }
    function minimumLoadedY() { return windowPresenter.minimumLoadedY() }
    function maximumLoadedY() { return windowPresenter.maximumLoadedY() }

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
        return windowPresenter.frameReachesContentEnd()
    }
    function placeAtExtent(extent, fraction) {
        windowPresenter.placeAtExtent(extent, fraction)
    }
    function visibleExtentSpan() { return windowPresenter.visibleExtentSpan() }

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

    function syncScrollBar() { windowPresenter.syncScrollBar() }

    function sendWindowRequest(extent, fraction, velocity,
                               preserveLiveAnchor, forceRequest) {
        if (!interactionActive || !hasWindowProtocol)
            return false
        if (standaloneViewport && (!windowInitialized
                || documentKey !== appliedDocumentKey
                || Number(frame.layoutRevision || 0) !== appliedLayoutRevision))
            return false
        const total = Math.max(0, contentExtent)
        const boundedTarget = clamp(Number(extent || 0), 0, total)
        const target = Math.floor(boundedTarget)
        if (windowRequestPending) {
            const nextFraction = clamp(Number(fraction || 0), 0, 0.999999)
            if (target === requestedExtent) {
                pendingWindowIntent = null
                return false
            }
            // One active request, one replaceable destination. Do not make
            // the IPC/decode/model queues process every intermediate drag.
            pendingWindowIntent = {"extent": target, "fraction": nextFraction,
                "velocity": velocity, "preserveLiveAnchor": preserveLiveAnchor}
            return true
        }
        const current = standaloneViewport ? appliedViewportStart
                                          : Number(frame.viewportStart || 0)
        const requestedFollowTail = terminalSurface
                && (terminalFollowTailIntent
                    || requestReachesContentEnd(target))
        const followStateChanged = terminalSurface
                && requestedFollowTail !== semanticFrameFollowsTail()
        if (forceRequest !== true && Math.abs(target - current) < 0.000001
                && !followStateChanged)
            return false

        windowRequestPending = true
        currentLoadError = ""
        requestedExtent = target
        requestedFraction = clamp(Number(fraction || 0), 0, 0.999999)
        requestedGeneration = (standaloneViewport
            ? Math.max(requestedGeneration, Number(frame.windowGeneration || 0))
            : Number(frame.windowGeneration || 0)) + 1
        resumeVelocity = Number(velocity || 0)
        requestPreservesLiveAnchor = preserveLiveAnchor !== false
        const actionMap = {
            "target": hostWindow.cleanText(frame.id),
            "action": hostWindow.cleanText(presentationFrame.scrollAction) !== ""
                      ? hostWindow.cleanText(presentationFrame.scrollAction)
                      : presentationFrame.kind === "editor"
                        ? "editor.scroll" : "viewer.scrollWindow"
        }
        if (documentKey !== "") {
            actionMap.documentKey = documentKey
            // Retain the old alias while mixed-version hosts exist.
            actionMap.contentKey = documentKey
        }
        actionMap.generation = requestedGeneration
        if (standaloneViewport)
            actionMap.layoutRevision = appliedLayoutRevision
        if (terminalSurface) {
            actionMap.followTail = requestedFollowTail
            setTerminalFollowTailIntent(requestedFollowTail, false)
        }
        if (presentationFrame.scrollUnit === "rows")
            actionMap.visualRow = Math.floor(target)
        else
            actionMap.offset = Math.floor(target)
        hostWindow.action(actionMap, true)
        return true
    }

    function dispatchPendingWindowIntent() {
        if (pendingWindowIntent === null)
            return false
        const pending = pendingWindowIntent
        pendingWindowIntent = null
        windowRequestPending = false
        // Even a return to the currently displayed page must cancel the
        // just-completed different destination in the core.
        return sendWindowRequest(pending.extent, pending.fraction,
                                 pending.velocity, pending.preserveLiveAnchor, true)
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
        // Prefetch eligibility belongs to the loaded window, not the visible
        // viewport. At the terminal tail the viewport can start at row 2
        // while the loaded window already starts at row 0. Treating row 2 as
        // "more content before" repeatedly asks Go for row 2; follow-tail
        // clamps that impossible request back to the same authoritative tail.
        const windowAtStart = rowExtent(0, displayedRows) <= 0.000001
        const windowAtEnd = contentExtentKnown && contentExtent > 0
                && rowEndExtent(displayedRows.length - 1, displayedRows)
                   >= contentExtent - 0.000001
        if ((rowsBefore <= threshold && !windowAtStart)
                || (rowsAfter <= threshold && !windowAtEnd)) {
            sendWindowRequest(state.extent, state.fraction,
                              documentList.verticalVelocity)
        }
    }

    function resetRequestTransientState() {
        stopMotion()
        terminalSelectionController.resetForDocumentChange()
        editorPointerController.reset()
        windowRequestPending = false
        pendingWindowIntent = null
        requestedExtent = 0
        requestedFraction = 0
        requestedGeneration = 0
        canceledWindowGeneration = 0
        resumeVelocity = 0
        requestPreservesLiveAnchor = true
        wheelGestureActive = false
        queuedScrollBarPosition = -1
    }

    function resetDocumentTransientState() {
        windowPresenter.resetDocumentTransientState()
    }

    function invalidateStandalonePresentation() {
        windowPresenter.invalidateStandalonePresentation()
    }

    function beginMiddleAutoScroll() {
        if (!middleAutoScrollAllowed || !standaloneViewport
                || !windowInitialized || middleAutoScrollActive)
            return false

        // The middle gesture becomes the sole owner of contentY. Retire a
        // wheel/flick destination before recording the presented document
        // epoch from which subsequent frame-driven requests are addressed.
        cancelPendingIntent()
        middleAutoScrollDocumentKey = appliedDocumentKey
        middleAutoScrollLayoutRevision = appliedLayoutRevision
        middleAutoScrollController.start()
        return middleAutoScrollActive
    }

    function endMiddleAutoScroll(commitPosition) {
        if (!middleAutoScrollActive)
            return false
        const shouldCommit = commitPosition === true
                && middleAutoScrollController.scrollingStarted
                && windowInitialized && !rebasingWindow
                && appliedDocumentKey === middleAutoScrollDocumentKey
                && appliedLayoutRevision === middleAutoScrollLayoutRevision
        const state = shouldCommit ? topState() : null
        middleAutoScrollController.end()
        middleAutoScrollDocumentKey = ""
        middleAutoScrollLayoutRevision = 0
        if (state !== null)
            sendWindowRequest(state.extent, state.fraction, 0, true)
        return true
    }

    function cancelPendingIntent() {
        if (!standaloneViewport)
            return
        if (windowRequestPending)
            canceledWindowGeneration = Math.max(canceledWindowGeneration,
                                                 requestedGeneration)
        windowRequestPending = false
        pendingWindowIntent = null
        queuedScrollBarPosition = -1
        resumeVelocity = 0
        wheelGestureActive = false
        stopMotion()
    }

    function applyFrameWindow() { windowPresenter.applyFrameWindow() }

    function handleWheel(wheel) {
        if (!interactionActive) {
            wheel.accepted = false
            return
        }
        endMiddleAutoScroll(true)
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

    function stopRequestWindowTimer() { requestWindowTimer.stop() }
    function scheduleWindowRequest() { requestWindowTimer.restart() }
    function scheduleScrollBarRequest() { scrollBarRequestTimer.restart() }

    function stopMotion() {
        endMiddleAutoScroll(false)
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
        if (middleAutoScrollActive && !rebasingWindow
                && !frameWindowSyncPending) {
            // Autoscroll can outlive an IPC round trip. Send the first
            // destination immediately; while it is in flight,
            // sendWindowRequest keeps exactly one replaceable latest intent.
            const middleState = topState()
            sendWindowRequest(middleState.extent, middleState.fraction,
                              0, true)
        }
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
        if (rebasingWindow || wheelGestureActive || middleAutoScrollActive)
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
        if (middleAutoScrollActive
                && (documentKey !== middleAutoScrollDocumentKey
                    || Number(frame.layoutRevision || 0)
                       !== middleAutoScrollLayoutRevision))
            endMiddleAutoScroll(false)
        scheduleFrameWindowSync()
        scheduleNativeViewportSync()
    }
    onRowHeightChanged: scheduleNativeViewportSync()
    onStandaloneViewportWidthChanged: scheduleNativeViewportSync()
    onStandaloneViewportHeightChanged: scheduleNativeViewportSync()
    onDocumentCellWidthChanged: scheduleNativeViewportSync()
    onEmbeddedChanged: scheduleNativeViewportSync()
    onMiddleAutoScrollAllowedChanged: {
        if (!middleAutoScrollAllowed)
            endMiddleAutoScroll(true)
    }
    onInteractionActiveChanged: {
        if (interactionActive) {
            scheduleNativeViewportSync()
            return
        }
        clearNativeViewport()
        if (standaloneViewport)
            scheduleNativeViewportSync()
        stopMotion()
        editorPointerController.reset()
        terminalSelectionController.cancelInteraction()
        wheelGestureActive = false
        windowRequestPending = false
        pendingWindowIntent = null
        requestedGeneration = 0
        requestPreservesLiveAnchor = true
        resumeVelocity = 0
        queuedScrollBarPosition = -1
        if (standaloneViewport)
            invalidateStandalonePresentation()
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

    DocumentWindowPresenter {
        id: windowPresenter
        hostWindow: controller.hostWindow
        documentList: controller.documentList
        documentScrollBar: controller.documentScrollBar
        coordinator: controller
        frame: controller.frame
        terminalSurface: controller.terminalSurface
        rowHeight: controller.rowHeight
        standaloneViewport: controller.standaloneViewport
        geometryRevision: controller.geometryRevision
        kineticActive: controller.kineticActive
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
                    || (controller.windowRequestPending
                        && !controller.standaloneViewport))
                return
            const position = controller.queuedScrollBarPosition
            controller.queuedScrollBarPosition = -1
            controller.sendWindowRequest(
                        position * controller.contentExtent, 0, 0, false)
        }
    }
}
