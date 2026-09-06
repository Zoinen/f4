pragma ComponentBehavior: Bound

import QtQuick
import QtQuick.Controls

Item {
    id: controller

    required property ApplicationWindow hostWindow
    required property ListView documentList
    required property FontMetrics fontMetrics
    required property DocumentViewportController viewportController
    property var frame: ({})
    property real rowHeight: 20
    property real textHorizontalInset: 10
    property real textViewportWidth: 0
    property real documentCellWidth: 1

    property int lastMouseColumn: 0
    property int lastMouseRow: 0
    property var pendingMouseMove: null
    property var lastMouseAction: null
    property var activePointer: null
    property string pointerDocumentKey: ""
    property real pointerLayoutRevision: 0
    property string lastEdgeEndpoint: ""
    readonly property bool pointerAtEdge: activePointer !== null
        && viewportController.standaloneViewport
        && (activePointer.y < 0 || activePointer.y >= documentList.height
            || activePointer.x < textHorizontalInset
            || activePointer.x >= textHorizontalInset + textViewportWidth)

    visible: false
    width: 0
    height: 0

    function mouseButton(buttons) {
        if ((buttons & Qt.LeftButton) !== 0)
            return "left"
        if ((buttons & Qt.RightButton) !== 0)
            return "right"
        if ((buttons & Qt.MiddleButton) !== 0)
            return "middle"
        return "none"
    }

    function mouseAction(mouse, phase, moved, doubleClick) {
        if (frame.kind !== "editor")
            return null
        const cellWidth = Math.max(1, viewportController.standaloneViewport
            ? documentCellWidth : fontMetrics.advanceWidth("M"))
        const rawColumn = Math.floor((mouse.x - textHorizontalInset) / cellWidth)
        const column = viewportController.standaloneViewport
                ? rawColumn : Math.max(0, rawColumn)
        // A held edge points into the current overscan, never an arbitrary
        // distance inferred from an off-window physical mouse coordinate.
        const pointerY = viewportController.standaloneViewport
                ? Math.max(-2 * rowHeight, Math.min(mouse.y,
                            documentList.height + 2 * rowHeight)) : mouse.y
        const windowIndex = viewportController.windowIndexAtViewportY(pointerY)
        const rows = viewportController.displayedRows
        const absoluteRow = windowIndex >= 0 && windowIndex < rows.length
                ? viewportController.rowExtent(windowIndex)
                : Number(frame.viewportStart || 0)
                  + Math.floor(mouse.y / rowHeight)
        const row = Math.max(0, Math.floor(
                    absoluteRow - (viewportController.standaloneViewport
                                   ? viewportController.appliedViewportStart
                                   : Number(frame.viewportStart || 0))))
        lastMouseColumn = column
        lastMouseRow = row
        const buttons = phase === "release" ? Qt.NoButton
                                               : (mouse.buttons || mouse.button)
        const action = {
            "target": hostWindow.cleanText(frame.id),
            "action": "editor.mouse",
            "phase": phase,
            "button": mouseButton(buttons),
            "column": column,
            "row": row,
            "moved": moved === true,
            "doubleClick": doubleClick === true,
            "shift": (mouse.modifiers & Qt.ShiftModifier) !== 0,
            "ctrl": (mouse.modifiers & Qt.ControlModifier) !== 0,
            "alt": (mouse.modifiers & Qt.AltModifier) !== 0
        }
        if (viewportController.standaloneViewport) {
            if (!viewportController.windowInitialized || rows.length === 0)
                return null
            const displayedRow = rows[Math.max(0, Math.min(
                                        windowIndex, rows.length - 1))]
            action.documentKey = viewportController.appliedDocumentKey
            action.layoutRevision = viewportController.appliedLayoutRevision
            action.rowOffset = Number(displayedRow.offset || 0)
            action.scrollLeft = viewportController.appliedScrollLeft
        }
        lastMouseAction = action
        return action
    }

    function flushMouseMove() {
        if (pendingMouseMove === null)
            return
        const actionMap = pendingMouseMove
        pendingMouseMove = null
        hostWindow.action(actionMap, true)
    }

    function sendMouse(mouse, phase, moved, doubleClick) {
        if (phase === "release")
            activePointer = null
        const actionMap = mouseAction(mouse, phase, moved, doubleClick)
        if (actionMap === null)
            return
        if (phase === "press")
            viewportController.cancelPendingIntent()
        if (viewportController.standaloneViewport && phase !== "release") {
            const endpoint = endpointSignature(actionMap)
            const repeatedEndpoint = moved === true && endpoint === lastEdgeEndpoint
            activePointer = { "x": mouse.x, "y": mouse.y,
                "buttons": mouse.buttons || mouse.button,
                "button": mouse.button, "modifiers": mouse.modifiers }
            pointerDocumentKey = viewportController.appliedDocumentKey
            pointerLayoutRevision = viewportController.appliedLayoutRevision
            lastEdgeEndpoint = endpoint
            if (pointerAtEdge)
                hostWindow.update()
            if (repeatedEndpoint)
                return
        }
        if (moved === true) {
            // Only the newest pointer endpoint can become visible in this
            // presentation frame, so coalesce native drag samples locally.
            pendingMouseMove = actionMap
            mouseMoveTimer.restart()
            return
        }
        mouseMoveTimer.stop()
        flushMouseMove()
        hostWindow.action(actionMap, true)
    }

    function releaseMouse() {
        activePointer = null
        if (frame.kind !== "editor")
            return
        mouseMoveTimer.stop()
        flushMouseMove()
        const action = Object.assign({}, lastMouseAction || {}, {
            "target": hostWindow.cleanText(frame.id),
            "action": "editor.mouse",
            "phase": "release",
            "button": "none",
            "column": lastMouseColumn,
            "row": lastMouseRow
        })
        hostWindow.action(action, true)
    }

    function reset() {
        mouseMoveTimer.stop()
        pendingMouseMove = null
        lastMouseAction = null
        activePointer = null
        lastEdgeEndpoint = ""
    }

    function endpointSignature(action) {
        return action.documentKey + ":" + action.layoutRevision + ":"
                + action.rowOffset + ":" + action.column + ":" + action.scrollLeft
                + ":" + action.button + ":" + action.shift + ":" + action.ctrl
                + ":" + action.alt
    }

    function continueEdgeSelection() {
        if (!pointerAtEdge)
            return
        if (viewportController.documentKey !== pointerDocumentKey
                || Number(frame.layoutRevision || 0) !== pointerLayoutRevision) {
            reset()
            return
        }
        if (viewportController.frameWindowSyncPending) {
            // The viewport commits later at this same animation boundary.
            // Resolve the next edge fragment only from that committed frame.
            hostWindow.update()
            return
        }
        const action = mouseAction(activePointer, "move", true, false)
        if (action === null)
            return
        const endpoint = endpointSignature(action)
        if (endpoint === lastEdgeEndpoint)
            return
        lastEdgeEndpoint = endpoint
        pendingMouseMove = null
        mouseMoveTimer.stop()
        hostWindow.action(action, true)
        hostWindow.update()
    }

    onFrameChanged: {
        if (activePointer !== null
                && (hostWindow.cleanText(frame.documentKey || frame.id)
                    !== pointerDocumentKey
                    || Number(frame.layoutRevision || 0) !== pointerLayoutRevision))
            reset()
    }

    Connections {
        target: controller.hostWindow
        enabled: controller.pointerAtEdge
        function onAfterAnimating() { controller.continueEdgeSelection() }
    }

    Timer {
        id: mouseMoveTimer
        interval: 0
        onTriggered: controller.flushMouseMove()
    }
}
