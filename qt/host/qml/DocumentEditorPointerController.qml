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

    property int lastMouseColumn: 0
    property int lastMouseRow: 0
    property var pendingMouseMove: null

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
        const cellWidth = Math.max(1, fontMetrics.advanceWidth("M"))
        const column = Math.max(0, Math.floor((mouse.x - 10) / cellWidth))
        const modelIndex = Math.floor(
                    (documentList.contentY + mouse.y) / rowHeight)
        const windowIndex = modelIndex - viewportController.loadedSlotStart
        const rows = viewportController.displayedRows
        const absoluteRow = windowIndex >= 0 && windowIndex < rows.length
                ? viewportController.rowExtent(windowIndex)
                : Number(frame.viewportStart || 0)
                  + Math.floor(mouse.y / rowHeight)
        const row = Math.max(0, Math.floor(
                    absoluteRow - Number(frame.viewportStart || 0)))
        lastMouseColumn = column
        lastMouseRow = row
        const buttons = phase === "release" ? Qt.NoButton
                                               : (mouse.buttons || mouse.button)
        return {
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
    }

    function flushMouseMove() {
        if (pendingMouseMove === null)
            return
        const actionMap = pendingMouseMove
        pendingMouseMove = null
        hostWindow.action(actionMap, true)
    }

    function sendMouse(mouse, phase, moved, doubleClick) {
        const actionMap = mouseAction(mouse, phase, moved, doubleClick)
        if (actionMap === null)
            return
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
        if (frame.kind !== "editor")
            return
        mouseMoveTimer.stop()
        flushMouseMove()
        hostWindow.action({
            "target": hostWindow.cleanText(frame.id),
            "action": "editor.mouse",
            "phase": "release",
            "button": "none",
            "column": lastMouseColumn,
            "row": lastMouseRow
        }, true)
    }

    function reset() {
        mouseMoveTimer.stop()
        pendingMouseMove = null
    }

    Timer {
        id: mouseMoveTimer
        interval: 0
        onTriggered: controller.flushMouseMove()
    }
}
