pragma ComponentBehavior: Bound

import QtQuick

WheelHandler {
    required property Item viewport
    property real rowHeight: 31
    target: null
    acceptedDevices: PointerDevice.Mouse | PointerDevice.TouchPad
    onWheel: event => {
        const delta = event.pixelDelta.y !== 0 ? event.pixelDelta.y
            : event.angleDelta.y / 120 * rowHeight * 3
        if (delta !== 0) {
            viewport.cancelFlick()
            viewport.contentY = Math.max(0, Math.min(
                Math.max(0, viewport.contentHeight - viewport.height), viewport.contentY - delta))
        }
        // An open menu owns wheel input even at its boundaries. Never scroll
        // the file panel behind it or change the current entry by a wheel step.
        event.accepted = true
    }
}
